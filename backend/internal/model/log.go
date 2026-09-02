package model

import "time"

// 日志级别常量。
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// 事件类型常量。访问日志统一用 EventHTTPRequest；其余为业务事件 / 系统事件。
const (
	EventHTTPRequest   = "http.request"
	EventImageUpload   = "image.upload"
	EventAPIKeyCreate  = "apikey.create"
	EventAPIKeyRename  = "apikey.rename"
	EventAPIKeyReset   = "apikey.reset"
	EventAPIKeyRevoke  = "apikey.revoke"
	EventAPIKeyDelete  = "apikey.delete"
	EventAuthLoginOK   = "auth.login_success"
	EventAuthLoginFail = "auth.login_failed"
	EventLogClear      = "log.clear"
	EventLogClearGet   = "log.clear_get"
	EventPanic         = "panic"
)

// Log 是日志中心的跨层数据载体（实体），独立于 Ent 生成的 ent.Log。
// DAO 层负责二者转换，使 service / api 层不直接依赖 Ent。
type Log struct {
	ID         int       `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Level      string    `json:"level"`
	Event      string    `json:"event"`
	Method     string    `json:"method,omitempty"`
	Path       string    `json:"path,omitempty"`
	Status     *int      `json:"status,omitempty"`
	DurationMs *int      `json:"duration_ms,omitempty"`
	ClientIP   string    `json:"client_ip,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
	APIKeyID   *int      `json:"api_key_id,omitempty"`
	Username   string    `json:"username,omitempty"`
	Message    string    `json:"message,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// LogContext 是从请求上下文抽取的、写日志时附带的关联信息。
// 由 middleware.LogContextFromGin 从 gin.Context 装配，传给 NewEventLog。
type LogContext struct {
	RequestID string
	Username  string
	APIKeyID  *int
	ClientIP  string
}

// NewEventLog 构造一条业务事件日志：填充时间戳与上下文关联字段，
// 调用方只需提供事件类型 / 级别 / 可读消息。访问日志（含 HTTP 字段）由中间件直接组装 *Log。
func NewEventLog(event, level, msg string, lc LogContext) *Log {
	return &Log{
		Timestamp: time.Now(),
		Level:     level,
		Event:     event,
		Message:   msg,
		RequestID: lc.RequestID,
		Username:  lc.Username,
		APIKeyID:  lc.APIKeyID,
		ClientIP:  lc.ClientIP,
	}
}

// LogQuery 描述日志列表的过滤 / 分页条件。
type LogQuery struct {
	Level       string    // 精确匹配级别，空表示不过滤
	Event       string    // 精确匹配事件类型，空表示不过滤
	Method      string    // 精确匹配 HTTP 方法，空表示不过滤
	StatusClass string    // "2xx"/"4xx"/"5xx"，空表示不过滤
	Keyword     string    // 对 path / message 模糊匹配，空表示不过滤
	RequestID   string    // 精确匹配 request_id，空表示不过滤
	APIKeyID    *int      // 精确匹配来源密钥，nil 表示不过滤
	Start       time.Time // 时间下界（含），零值表示不过滤
	End         time.Time // 时间上界（不含），零值表示不过滤
	Offset      int
	Limit       int
}

// DailyCount 是直方图单日计数。
type DailyCount struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Count int    `json:"count"`
}

// DestructiveRequest 是清理日志等敏感操作的请求体（账号密码二次确认）。
// 复用与 apikey 吊销 / 删除相同的二次确认机制：后端用 subtle.ConstantTimeCompare 校验，
// 失败返回 403（而非 401），避免触发前端全局登出。
type DestructiveRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	// Scope 清理范围：空 / "all" 清空全部（默认，向后兼容）；"get" 仅清 info 级 GET 请求日志。
	Scope string `json:"scope,omitempty"`
}
