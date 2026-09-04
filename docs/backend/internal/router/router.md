# `internal/router/router.go`

整个后端唯一的依赖装配点和路由注册点。`cmd/server/main.go` 打开数据库、构造 DAO、构造 storage.Saver 后调用 `router.New(cfg, imageDAO, apiKeyDAO, logDAO, saver, lg)` 拿到 `*gin.Engine` 与一个 `*service.LogService`：前者用于启动 HTTP 服务，后者供 main 在优雅关闭阶段调用 flush，确保异步访问日志缓冲全部落库。

## 函数

### `New(cfg *config.Config, imageDAO dao.ImageDAO, apiKeyDAO dao.APIKeyDAO, logDAO dao.LogDAO, saver *storage.Saver, trustedProxies []*net.IPNet, lg *logger.Logger) (*gin.Engine, *service.LogService)`

- 使用 `gin.New()` 而非 `gin.Default()`，自己挂中间件以保留控制权。中间件链按顺序为 `RequestID -> Recovery -> CORS -> Logger`：
  - `middleware.RequestID()` 最先挂载，为每个请求生成/透传 request id，后续中间件与 handler 均可取用。
  - `middleware.Recovery(lg, logSvc)` 捕获 panic 并经 `logSvc` 异步落库一条 panic 事件。
  - `middleware.CORS(cfg.CORS.AllowOrigins)` 跨域中间件，按 [`cors.allow_origins`](../config/config.md) 白名单收紧：开发 `*`、生产留空关闭或配确切域名（release 模式 [`Validate`](../config/config.md) 拒 `*`）。见 [`cors.md`](../middleware/cors.md)。
  - `middleware.Logger(lg, logSvc)` 输出 zap 结构化访问日志，同时把访问记录交 `logSvc` 异步落库。
- `r.GET/HEAD("/imgs/*filepath", serveImages(...))`：开发期由后端直接 serve 图片落盘目录，供前端加载 `/imgs/<rel>`；生产环境建议由 Nginx 反代 `/imgs/`（见 [`IMAGE.md`](../../IMAGE.md)），此处仅兜底。**带图片扩展名白名单前置过滤**（[`serveImages`](./static.md)，由 `cfg.Storage.AllowedMimeTypes` 折算）：仅放行 `.png/.jpg/.jpeg/.gif/.webp` 等，拒绝 `.yaml/.db/.go`，即便 `root_dir` 被误配成工作目录也不会未认证暴露 config/数据库/源码；`..` 逃逸防护复用 `http.FileServer`。
- `imageDAO` / `apiKeyDAO` / `logDAO` 由调用方基于已打开的数据库注入（见 [`dao.md`](../dao/dao.md)）。
- `saver` 由调用方基于 `cfg.Storage` 提前构造（见 [`pkg/storage.md`](../pkg/storage.md)），启动期 `MkdirAll` 暴露路径/权限问题。
- `lg` 是贯穿全链路的 zap 结构化日志器，供中间件、service、api 共享。
- **依赖装配**（按 dao -> service -> api 的顺序）：
  1. `jwtMgr := jwt.NewManager(cfg.Auth.JWT)`
  2. `authSvc := service.NewAuthService(cfg.Auth, jwtMgr)` -> `authAPI := api.NewAuthAPI(authSvc, logSvc)`
  3. `apiKeySvc := service.NewAPIKeyService(apiKeyDAO, imageDAO, saver)` -> `apiKeyAPI := api.NewAPIKeyAPI(apiKeySvc, authSvc, logSvc)`（`imageDAO`/`saver` 供删除密钥级联清理，`authSvc` 供吊销/删除密码二次确认）
  4. `imageSvc := service.NewImageService(imageDAO, saver, cfg.Storage)` -> `imageAPI := api.NewImageAPI(imageSvc, authSvc, logSvc)`（`authSvc` 供批量删除的账号密码二次确认）
  5. `logSvc := service.NewLogService(logDAO, lg)`（先于中间件链与各 API 构造，Logger/Recovery 中间件需要它异步落库）-> `logAPI := api.NewLogAPI(logSvc, authSvc)`（`authSvc` 供清理日志的密码二次确认）
  6. `systemSvc := service.NewSystemService(cfg)` -> `systemAPI := api.NewSystemAPI(systemSvc)`（只读 config 快照，不依赖 dao / storage / logger，handler 不记业务事件）
  7. `dashboardSvc := service.NewDashboardService(imageDAO, apiKeyDAO, logDAO)` -> `dashboardAPI := api.NewDashboardAPI(dashboardSvc)`（只读聚合统计，复用三个已注入的 DAO，不记业务事件；端到端见 [`DASHBOARD.md`](../../DASHBOARD.md)）
  8. `rateStore := ratelimit.NewStore(cfg.APIKey.RateLimitPerMinute)` -- 按密钥维度限流的内存令牌桶。
- `logSvc` 同时作为 **LogRecorder** 注入到 `authAPI` / `apiKeyAPI` / `imageAPI`，使这些 handler 能在登录、密钥增删改、图片上传等业务节点发射结构化业务事件，统一由 `logSvc` 异步落库。
- **路由注册**：所有业务接口挂在 `/api/v1` 下。
  - 公开：`GET /ping`、`POST /auth/login`
  - 受保护（`middleware.JWTAuth(jwtMgr)`）：`GET /auth/me`；再套 `adminImages := protected.Group("/admin/images", middleware.HTTPSOnly(cfg.APIKey.HTTPSOnly, trustedProxies))` 挂后台图片接口（**JWT + HTTPS** 双重保护）供内容中心：`GET ""`（图片列表）、`POST ""`（后台直传上传，`key_id` 留空）、`DELETE ""`（批量删除，敏感操作，handler 内部校验请求体携带的账号密码做二次确认）；再套 `keys := protected.Group("/apikeys", middleware.HTTPSOnly(cfg.APIKey.HTTPSOnly, trustedProxies))` 挂密钥管理接口（**JWT + HTTPS** 双重保护），其中吊销（`POST /:id/revoke`）与删除（`DELETE /:id`）为敏感操作，handler 内部还会校验请求体携带的账号密码做二次确认；再套 `logs := protected.Group("/admin/logs", middleware.HTTPSOnly(cfg.APIKey.HTTPSOnly, trustedProxies))` 挂日志中心接口（**JWT + HTTPS** 双重保护），其中清理（`DELETE ""`）为敏感操作，handler 内部同样校验账号密码做二次确认。`GET /system/config` 同样挂在 protected 下（**仅 JWT**，未套 HTTPSOnly），返回当前 config 的非敏感只读快照（auth 段不暴露）。`GET /admin/dashboard` 同样挂在 protected 下（**仅 JWT**，未套 HTTPSOnly），一次性返回仪表盘聚合统计（图片总量 / 存储占用 / APIkey 计数 / 日志总量 / 近 N 天上传趋势，见 [`DASHBOARD.md`](../../DASHBOARD.md)）。`/admin/images` 与对外 `/images`（API Key 鉴权）路径不同，避免同方法同路径重复注册冲突。
  - API 密钥保护（`middleware.APIKeyAuth(apiKeySvc, rateStore)`，独立于 JWT）：`images` 组。

## 路由地图

```
/imgs/*filepath                   GET/HEAD    serveImages(cfg.Storage.RootDir, 扩展名白名单)
                                              （开发期 serve，生产由 Nginx 反代；仅放行图片扩展名）

/api/v1
├── GET  /ping                   公开        api.Ping
├── POST /auth/login             公开        AuthAPI.Login
├── ── (中间件: JWTAuth)
│   ├── GET  /auth/me            受保护      AuthAPI.Me
│   ├── ── (中间件: HTTPSOnly)   /admin/images
│   │   ├── GET    ""            JWT+HTTPS   ImageAPI.ListAdmin（内容中心图片列表）
│   │   ├── POST   ""            JWT+HTTPS   ImageAPI.CreateAdmin (后台直传，key_id=nil)
│   │   └── DELETE ""            JWT+HTTPS   ImageAPI.BatchDeleteAdmin（批量删除，需密码）
│   ├── ── (中间件: HTTPSOnly)   /apikeys
│   │   ├── POST   ""            JWT+HTTPS   APIKeyAPI.Create
│   │   ├── GET    ""            JWT+HTTPS   APIKeyAPI.List
│   │   ├── PATCH  "/:id"        JWT+HTTPS   APIKeyAPI.Rename
│   │   ├── POST   "/:id/reset"  JWT+HTTPS   APIKeyAPI.Reset
│   │   ├── POST   "/:id/revoke" JWT+HTTPS   APIKeyAPI.Revoke（软吊销，需密码）
│   │   └── DELETE "/:id"        JWT+HTTPS   APIKeyAPI.Delete（硬删+级联删图，需密码）
│   ├── ── (中间件: HTTPSOnly)   /admin/logs
│   │   ├── GET    ""            JWT+HTTPS   LogAPI.List（分页查询访问/业务日志）
│   │   ├── GET    "/histogram"  JWT+HTTPS   LogAPI.Histogram（按时间桶聚合统计）
│   │   └── DELETE ""            JWT+HTTPS   LogAPI.Clear（清空日志，需密码）
│   ├── GET  /system/config      受保护      SystemAPI.Config（只读配置快照，无入参）
│   └── GET  /admin/dashboard    受保护      DashboardAPI.Overview（聚合统计，?days=30，见 DASHBOARD.md）
└── ── (中间件: APIKeyAuth)      /images
    ├── GET  ""                  API Key     ImageAPI.List   (占位)
    └── POST ""                  API Key     ImageAPI.Create (已实现，需 readwrite)
```

API 密钥鉴权链路与权限矩阵的完整说明见 [`APIKEY.md`](../../APIKEY.md)；图片上传链路与 Nginx 反代约定见 [`IMAGE.md`](../../IMAGE.md)；访问/业务日志落库与日志中心接口说明见 [`LOG.md`](../../LOG.md)；仪表盘聚合统计见 [`DASHBOARD.md`](../../DASHBOARD.md)。

## 修改建议

- **保持 router 是依赖装配的唯一入口**：新加业务包时，把 `New` 写进各自的构造函数，由 router 在这里组装；不要在 handler 或 service 里直接 `new` 出依赖。
- **`logSvc` 必须最先构造**：Logger / Recovery 中间件在请求链路中就会调用它异步落库，若它晚于中间件注册会出现空指针；新增依赖时留意构造顺序。
- **优雅关闭须 flush `logSvc`**：`New` 返回的 `*service.LogService` 交由 main 持有，进程退出前应调用其 flush/关闭方法，避免异步缓冲中的访问日志丢失。
- 受保护路由全部挂到 `protected := v1.Group("", middleware.JWTAuth(jwtMgr))` 下；新增公开路由直接挂 `v1`。
- 新增需要写业务事件的 handler 时，通过构造函数注入 `logSvc`（即 `LogRecorder` 接口）发射事件，不要在 handler 里直接操作 `dao.LogDAO`。
- 项目变大、依赖关系复杂后，可以引入 [`google/wire`](https://github.com/google/wire) 等工具做编译期 DI，但当前规模手动注入是最直观、最低心智成本的做法。
