package api

import (
	"strconv"
	"time"

	"github.com/Lestine-Yan/irisImg/backend/internal/middleware"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
	"github.com/Lestine-Yan/irisImg/backend/internal/pkg/response"
	"github.com/Lestine-Yan/irisImg/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// LogAPI 是日志中心接口的控制器，挂在 JWT 受保护组下。
// 清理日志为敏感操作，handler 内用账号密码做二次确认（复用 apikey 吊销 / 删除的同款机制）。
type LogAPI struct {
	svc     *service.LogService
	authSvc *service.AuthService
}

// NewLogAPI 构造控制器。
func NewLogAPI(svc *service.LogService, authSvc *service.AuthService) *LogAPI {
	return &LogAPI{svc: svc, authSvc: authSvc}
}

// List 处理 GET /admin/logs，分页 + 多维过滤查询日志。
//
//	Query 参数:
//	  - page / page_size: 分页，默认 1 / 50，page_size 上限 500。
//	  - level / event / method / status_class / keyword / request_id / api_key_id: 过滤条件。
//	  - start / end: 时间区间（RFC3339），左闭右开。
func (h *LogAPI) List(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		response.BadRequest(c, "无效的 page")
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if err != nil || pageSize < 1 || pageSize > 500 {
		response.BadRequest(c, "无效的 page_size")
		return
	}

	// status_class 仅允许空 / 2xx / 4xx / 5xx，非法值直接 400，避免被静默放宽为不过滤。
	statusClass := c.Query("status_class")
	switch statusClass {
	case "", "2xx", "4xx", "5xx":
	default:
		response.BadRequest(c, "无效的 status_class")
		return
	}

	q := model.LogQuery{
		Level:       c.Query("level"),
		Event:       c.Query("event"),
		Method:      c.Query("method"),
		StatusClass: statusClass,
		Keyword:     c.Query("keyword"),
		RequestID:   c.Query("request_id"),
		Offset:      (page - 1) * pageSize,
		Limit:       pageSize,
	}
	if raw := c.Query("api_key_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id < 1 {
			response.BadRequest(c, "无效的 api_key_id")
			return
		}
		q.APIKeyID = &id
	}
	if raw := c.Query("start"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "无效的 start（需 RFC3339）")
			return
		}
		q.Start = t
	}
	if raw := c.Query("end"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "无效的 end（需 RFC3339）")
			return
		}
		q.End = t
	}

	items, total, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		response.ServerError(c, "查询日志失败："+err.Error())
		return
	}
	response.Success(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Histogram 处理 GET /admin/logs/histogram，返回最近 14 天的每日日志量（供直方图 + 趋势线）。
// 响应: { buckets: [{date, count}], total }
func (h *LogAPI) Histogram(c *gin.Context) {
	buckets, total, err := h.svc.Histogram(c.Request.Context(), 14)
	if err != nil {
		response.ServerError(c, "查询日志直方图失败："+err.Error())
		return
	}
	response.Success(c, gin.H{
		"buckets": buckets,
		"total":   total,
	})
}

// Clear 处理 DELETE /admin/logs，按 scope 清理日志。
// 需在请求体中携带账号密码做二次确认；失败返回 403（非 401，避免触发前端全局登出）。
// scope 取值：空 / "all" 清空全部（补记 log.clear 审计事件）；
// "get" 仅清 info 级 GET 请求日志（补记 log.clear_get 审计事件），非法值返回 400。
func (h *LogAPI) Clear(c *gin.Context) {
	var req model.DestructiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.authSvc.VerifyCredentials(req.Username, req.Password); err != nil {
		response.Forbidden(c, "用户名或密码错误")
		return
	}

	var n int64
	var err error
	lc := middleware.LogContextFromGin(c)
	switch req.Scope {
	case "", "all":
		n, err = h.svc.ClearAll(c.Request.Context(), lc)
	case "get":
		n, err = h.svc.ClearInfoGet(c.Request.Context(), lc)
	default:
		response.BadRequest(c, "无效的 scope")
		return
	}
	if err != nil {
		response.ServerError(c, "清理日志失败："+err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": n})
}
