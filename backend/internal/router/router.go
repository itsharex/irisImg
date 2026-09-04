package router

import (
	"net"

	"github.com/Lestine-Yan/irisImg/backend/config"
	"github.com/Lestine-Yan/irisImg/backend/internal/api"
	"github.com/Lestine-Yan/irisImg/backend/internal/dao"
	"github.com/Lestine-Yan/irisImg/backend/internal/middleware"
	"github.com/Lestine-Yan/irisImg/backend/internal/pkg/jwt"
	"github.com/Lestine-Yan/irisImg/backend/internal/pkg/logger"
	"github.com/Lestine-Yan/irisImg/backend/internal/pkg/ratelimit"
	"github.com/Lestine-Yan/irisImg/backend/internal/pkg/storage"
	"github.com/Lestine-Yan/irisImg/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// New 组装并返回 gin.Engine，同时返回 LogService 供调用方在优雅关闭时 flush 异步日志缓冲。
//
// 依赖在这里手动注入，规模变大后可以引入 wire 等工具自动生成。
//
// imageDAO / apiKeyDAO / logDAO 由调用方（main）基于已打开的数据库构造后注入；
// saver 由调用方基于配置构造；lg 是贯穿全链路的 zap 结构化日志。
func New(cfg *config.Config, imageDAO dao.ImageDAO, apiKeyDAO dao.APIKeyDAO, logDAO dao.LogDAO, saver *storage.Saver, trustedProxies []*net.IPNet, lg *logger.Logger) (*gin.Engine, *service.LogService) {
	r := gin.New()

	// LogService 先于中间件链构造：Logger / Recovery 中间件需要它异步落库访问日志 / panic。
	logSvc := service.NewLogService(logDAO, lg)

	// 中间件链：RequestID 最前（后续中间件 / handler 均可取 request id）->
	// Recovery 捕获 panic 并落库 -> CORS -> Logger 结构化访问日志 + 异步落库。
	r.Use(
		middleware.RequestID(),
		middleware.Recovery(lg, logSvc),
		middleware.CORS(cfg.CORS.AllowOrigins),
		middleware.Logger(lg, logSvc),
	)

	// 静态图片服务：开发期由后端直接 serve 落盘目录，前端可加载 /imgs/<rel> 缩略图与大图。
	// 生产建议由 Nginx 反代 /imgs/（见 docs/backend/IMAGE.md），此处兜底，不影响 Nginx 优先拦截。
	//
	// 带「图片扩展名白名单」前置过滤（serveImages）：仅放行 .png/.jpg/.jpeg/.gif/.webp 等，
	// 拒绝 .yaml/.db/.go。即便 storage.root_dir 被误配成工作目录或 backend/ 本身，未认证访客
	// 也无法经 /imgs 下载 config.yaml / irisImg.db / 源码。.. 逃逸防护复用 http.FileServer。
	// 详见 static.go 与 docs/backend/internal/router/static.md。
	imgServe := serveImages(cfg.Storage.RootDir, allowedImageExtensions(cfg.Storage.AllowedMimeTypes))
	r.GET("/imgs/*filepath", imgServe)
	r.HEAD("/imgs/*filepath", imgServe)

	// 依赖装配：config -> jwt.Manager / service -> api
	jwtMgr := jwt.NewManager(cfg.Auth.JWT)
	authSvc := service.NewAuthService(cfg.Auth, jwtMgr)
	authAPI := api.NewAuthAPI(authSvc, logSvc)

	apiKeySvc := service.NewAPIKeyService(apiKeyDAO, imageDAO, saver)
	apiKeyAPI := api.NewAPIKeyAPI(apiKeySvc, authSvc, logSvc)

	imageSvc := service.NewImageService(imageDAO, saver, cfg.Storage)
	imageAPI := api.NewImageAPI(imageSvc, authSvc, logSvc)

	logAPI := api.NewLogAPI(logSvc, authSvc)

	systemSvc := service.NewSystemService(cfg)
	systemAPI := api.NewSystemAPI(systemSvc)

	dashboardSvc := service.NewDashboardService(imageDAO, apiKeyDAO, logDAO)
	dashboardAPI := api.NewDashboardAPI(dashboardSvc)

	// 按密钥维度限流的内存令牌桶，默认阈值来自配置。
	rateStore := ratelimit.NewStore(cfg.APIKey.RateLimitPerMinute)

	// 路由分组：所有业务接口统一挂在 /api/v1 下
	v1 := r.Group("/api/v1")
	{
		v1.GET("/ping", api.Ping)

		// 公开的认证入口
		v1.POST("/auth/login", authAPI.Login)

		// 需要 JWT 登录后才能访问的受保护接口
		protected := v1.Group("", middleware.JWTAuth(jwtMgr))
		{
			protected.GET("/auth/me", authAPI.Me)

			// 密钥管理接口：受 JWT 保护 + 强制 HTTPS（生产由配置开启）。
			// 吊销 / 删除为敏感操作，handler 内部还会校验请求体携带的账号密码做二次确认。
			keys := protected.Group("/apikeys", middleware.HTTPSOnly(cfg.APIKey.HTTPSOnly, trustedProxies))
			{
				keys.POST("", apiKeyAPI.Create)
				keys.GET("", apiKeyAPI.List)
				keys.PATCH("/:id", apiKeyAPI.Rename)
				keys.POST("/:id/reset", apiKeyAPI.Reset)
				keys.POST("/:id/revoke", apiKeyAPI.Revoke)
				keys.DELETE("/:id", apiKeyAPI.Delete)
			}

			// 图片管理接口（后台）：受 JWT 保护，供内容中心拉取图片列表、后台直传上传、批量删除。
			// 与对外 /images（API Key 鉴权）解耦，避免后台页面被迫注入 X-API-Key。
			// 路径用 /admin/images 而非 /images，后者已被 APIKeyAuth 组占用，重复注册会冲突。
			// 批量删除为敏感操作（物理删文件不可恢复），组上强制 HTTPS（生产由配置开启），
			// handler 内部还会校验请求体携带的账号密码做二次确认，与 /apikeys、/admin/logs 同款防护。
			adminImages := protected.Group("/admin/images", middleware.HTTPSOnly(cfg.APIKey.HTTPSOnly, trustedProxies))
			{
				adminImages.GET("", imageAPI.ListAdmin)
				adminImages.POST("", imageAPI.CreateAdmin)
				adminImages.DELETE("", imageAPI.BatchDeleteAdmin)
			}

			// 日志中心接口：受 JWT 保护 + 强制 HTTPS（生产由配置开启）。
			// 清理日志为敏感操作，handler 内部还会校验请求体携带的账号密码做二次确认。
			logs := protected.Group("/admin/logs", middleware.HTTPSOnly(cfg.APIKey.HTTPSOnly, trustedProxies))
			{
				logs.GET("", logAPI.List)
				logs.GET("/histogram", logAPI.Histogram)
				logs.DELETE("", logAPI.Clear)
			}

			// 系统配置只读接口：受 JWT 保护，返回当前 config 的非敏感快照。
			// 不支持修改 / 热更新，配置变更需改 config 文件并重启。
			protected.GET("/system/config", systemAPI.Config)

			// 仪表盘统计接口：受 JWT 保护，只读聚合，一次性返回首页所需的图片总量 / 存储大小 /
			// APIkey 计数 / 日志总量 / 近 N 天上传趋势。无需 HTTPSOnly 与二次确认。
			protected.GET("/admin/dashboard", dashboardAPI.Overview)
		}

		// 图片接口：由 API 密钥鉴权中间件保护（独立于 JWT）。
		// GET 任意有效密钥可访问，POST 需读写密钥；POST 已落地，GET 仍为占位。
		images := v1.Group("/images", middleware.APIKeyAuth(apiKeySvc, rateStore))
		{
			images.GET("", imageAPI.List)
			images.POST("", imageAPI.Create)
		}
	}

	return r, logSvc
}
