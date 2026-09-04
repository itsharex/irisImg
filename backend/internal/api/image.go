package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/Lestine-Yan/irisImg/backend/internal/middleware"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
	"github.com/Lestine-Yan/irisImg/backend/internal/pkg/response"
	"github.com/Lestine-Yan/irisImg/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ImageAPI 是图片相关接口的控制器。
//
// 当前已落地：
//   - POST /images（对外添加图片）：由 API Key 鉴权中间件保护，只读密钥访问 POST 会被 403。
//   - POST /api/v1/admin/images（后台直传）：由 JWT 鉴权中间件保护，供内容中心上传，key_id 留空。
//   - GET /api/v1/admin/images（后台列表）：由 JWT 鉴权中间件保护，供内容中心拉取图片。
//   - DELETE /api/v1/admin/images（后台批量删除）：由 JWT 鉴权中间件保护，供内容中心批量删除，
//     需请求体携带账号密码做二次确认。
//
// GET /images（对外，API Key）暂为占位，待语义明确后再实现。
type ImageAPI struct {
	svc     *service.ImageService
	authSvc *service.AuthService
	rec     service.LogRecorder
}

// NewImageAPI 通过依赖注入构造控制器。authSvc 用于批量删除的账号密码二次确认；
// rec 用于记录图片上传 / 删除业务事件到日志中心。
func NewImageAPI(svc *service.ImageService, authSvc *service.AuthService, rec service.LogRecorder) *ImageAPI {
	return &ImageAPI{svc: svc, authSvc: authSvc, rec: rec}
}

// recordUpload 记录一次图片上传业务事件。rec 为空时直接返回，便于测试。
func (h *ImageAPI) recordUpload(c *gin.Context, filename string) {
	if h.rec == nil {
		return
	}
	h.rec.Record(model.NewEventLog(model.EventImageUpload, model.LevelInfo, "upload image: "+filename, middleware.LogContextFromGin(c)))
}

// recordDelete 记录一次批量删除图片业务事件（破坏性操作，级别 warn）。rec 为空时直接返回，便于测试。
func (h *ImageAPI) recordDelete(c *gin.Context, count int) {
	if h.rec == nil {
		return
	}
	h.rec.Record(model.NewEventLog(model.EventImageDelete, model.LevelWarn,
		fmt.Sprintf("batch delete images: %d", count), middleware.LogContextFromGin(c)))
}

// uploadFormField 是上传字段名，固定为 "file"。
const uploadFormField = "file"

// List 处理 GET /images（对外占位）。任意有效密钥均可访问。
// 对外列表 / 单图查询语义待定，当前显式返回 501 与鉴权失败状态码区分开。
func (h *ImageAPI) List(c *gin.Context) {
	response.Fail(c, http.StatusNotImplemented, response.CodeServerError, "图片列表接口尚未实现（占位）")
}

// ListAdmin 处理 GET /api/v1/admin/images（JWT 保护），供后台内容中心拉取图片列表。
//
//	Query 参数:
//	  - key_id    可选；缺省=不按密钥过滤（全部），>=1 时只返回该密钥添加的图片。
//	  - order     可选；asc/desc，默认 asc（时间升序，契合内容中心）。
//	  - page      可选；默认 1，<1 视为非法。
//	  - page_size 可选；默认 24，<1 视为非法。
//
//	响应: { items: [model.Image], total, page, page_size }
func (h *ImageAPI) ListAdmin(c *gin.Context) {
	var keyID *int
	if raw := c.Query("key_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id < 1 {
			response.BadRequest(c, "无效的 key_id")
			return
		}
		keyID = &id
	}

	order := c.DefaultQuery("order", "asc")

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		response.BadRequest(c, "无效的 page")
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "24"))
	if err != nil || pageSize < 1 {
		response.BadRequest(c, "无效的 page_size")
		return
	}

	q := model.ImageListQuery{
		KeyID:  keyID,
		Order:  order,
		Offset: (page - 1) * pageSize,
		Limit:  pageSize,
	}
	result, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		response.ServerError(c, "查询图片列表失败："+err.Error())
		return
	}
	response.Success(c, gin.H{
		"items":     result.Items,
		"total":     result.Total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Create 处理 POST /images，落地一张图片：
//
//	请求：multipart/form-data，字段名 "file"，需 readwrite 密钥（由中间件保证）。
//	响应：200 + 完整的 model.Image（含对外 URL、宽高、hash、key_id 等）。
//
// 错误：
//   - 400 缺少文件 / 内容为空 / MIME 不在白名单
//   - 413 超过 storage.max_upload_size_mb
//   - 500 其它内部错误
func (h *ImageAPI) Create(c *gin.Context) {
	filename, content, ok := readUploadFile(c, h.svc.MaxBytes())
	if !ok {
		return
	}

	// 中间件保证此处 key_id 必然存在且 >0。
	keyID := c.GetInt(middleware.ContextKeyAPIKeyID)

	img, err := h.svc.Upload(c.Request.Context(), &model.UploadImageInput{
		Filename: filename,
		Content:  content,
		KeyID:    &keyID,
	})
	if err != nil {
		respondUploadError(c, err)
		return
	}
	h.recordUpload(c, filename)
	response.Success(c, img)
}

// CreateAdmin 处理 POST /api/v1/admin/images，后台 JWT 直传一张图片：
//
//	请求：multipart/form-data，字段名 "file"，由 JWT 鉴权中间件保护（无需 X-API-Key）。
//	响应：200 + 完整的 model.Image（key_id 为 nil，代表 admin 直传）。
//
// 与 Create 的差别仅在不走 API Key 通道、KeyID 传 nil：上传的图片不关联任何密钥，
// 只会在内容中心「全部」里出现，详情里来源展示为 admin。
//
// 错误码与 Create 的业务部分一致（400 / 413 / 500）；401 由 JWT 中间件负责，
// 不涉及 API Key 通道的 403 / 429。
func (h *ImageAPI) CreateAdmin(c *gin.Context) {
	filename, content, ok := readUploadFile(c, h.svc.MaxBytes())
	if !ok {
		return
	}

	img, err := h.svc.Upload(c.Request.Context(), &model.UploadImageInput{
		Filename: filename,
		Content:  content,
		KeyID:    nil,
	})
	if err != nil {
		respondUploadError(c, err)
		return
	}
	h.recordUpload(c, filename)
	response.Success(c, img)
}

// BatchDeleteAdmin 处理 DELETE /api/v1/admin/images，批量删除图片（物理文件 + 元信息记录）。
// 供内容中心多选删除使用。
//
//	请求：JSON body，含账号密码（二次确认）与待删除 ID 列表（非空、每项 >0、上限 100）。
//	响应：200 + { deleted: 实际删除条数, ids: 实际被删除的图片 ID 列表 }。
//
// 与吊销 / 删除密钥同款二次确认：VerifyCredentials 失败返回 403（而非 401），
// 避免触发前端全局登出。不存在的 ID 静默跳过（幂等语义），不构成错误。
//
// 错误：
//   - 400 body 解析失败 / ids 为空 / 超过 100 / 含非正数
//   - 401 JWT 缺失或失效（由中间件返回）
//   - 403 账号密码二次确认失败（或生产环境 HTTPSOnly 拦截）
//   - 500 其它内部错误
func (h *ImageAPI) BatchDeleteAdmin(c *gin.Context) {
	var req model.BatchDeleteImagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.authSvc.VerifyCredentials(req.Username, req.Password); err != nil {
		response.Forbidden(c, "用户名或密码错误")
		return
	}

	deleted, ids, err := h.svc.BatchDelete(c.Request.Context(), req.IDs)
	if err != nil {
		response.ServerError(c, "批量删除图片失败："+err.Error())
		return
	}
	h.recordDelete(c, deleted)
	response.Success(c, model.BatchDeleteImagesResponse{Deleted: deleted, IDs: ids})
}

// readUploadFile 从 multipart 请求里读取 "file" 字段的完整字节，统一处理超大与缺字段错误。
// 失败时已写入响应并返回 ok=false，调用方应直接 return；成功返回原始文件名与内容。
// MaxBytesReader 也在此处套上，把超大请求体拦在 Multipart 解析之前，避免分配大内存。
func readUploadFile(c *gin.Context, maxBytes int64) (filename string, content []byte, ok bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)

	// MaxBytesReader 触发时 FormFile / ReadAll 会返回 "http: request body too large"
	// 之类的错误；用 errors.As 抓 *http.MaxBytesError，更可靠。声明在函数作用域，
	// 供下面两处错误分支共用。
	var maxErr *http.MaxBytesError

	fileHeader, err := c.FormFile(uploadFormField)
	if err != nil {
		if errors.As(err, &maxErr) {
			response.PayloadTooLarge(c, "上传文件过大")
			return "", nil, false
		}
		response.BadRequest(c, "缺少上传文件字段 "+uploadFormField+"："+err.Error())
		return "", nil, false
	}

	src, err := fileHeader.Open()
	if err != nil {
		response.ServerError(c, "打开上传文件失败："+err.Error())
		return "", nil, false
	}
	defer src.Close()

	content, err = io.ReadAll(src)
	if err != nil {
		if errors.As(err, &maxErr) {
			response.PayloadTooLarge(c, "上传文件过大")
			return "", nil, false
		}
		response.ServerError(c, "读取上传文件失败："+err.Error())
		return "", nil, false
	}
	return fileHeader.Filename, content, true
}

// respondUploadError 把 service.Upload 返回的错误映射为对应的 HTTP 响应，
// 供 Create / CreateAdmin 复用，避免两处分支漂移。
func respondUploadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEmptyFile):
		response.BadRequest(c, "上传内容为空")
	case errors.Is(err, service.ErrFileTooLarge):
		response.PayloadTooLarge(c, "上传文件过大")
	case errors.Is(err, service.ErrUnsupportedMime):
		response.BadRequest(c, "不支持的图片类型")
	default:
		response.ServerError(c, "图片上传失败："+err.Error())
	}
}
