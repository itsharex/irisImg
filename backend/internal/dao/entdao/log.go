package entdao

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Lestine-Yan/irisImg/backend/ent"
	"github.com/Lestine-Yan/irisImg/backend/ent/accesslog"
	"github.com/Lestine-Yan/irisImg/backend/ent/predicate"
	"github.com/Lestine-Yan/irisImg/backend/internal/dao"
	"github.com/Lestine-Yan/irisImg/backend/internal/model"
)

// logDAO 是 dao.LogDAO 的 Ent 实现。
// ent 预声明了 Log 标识符，故底层实体为 ent.AccessLog（表名经注解定为 logs）；
// 本文件在 model.Log 与 ent.AccessLog 之间转换，上层只感知 model.Log。
type logDAO struct {
	client *ent.Client
}

// NewLogDAO 基于 Ent 客户端构造 dao.LogDAO。
func NewLogDAO(client *ent.Client) dao.LogDAO {
	return &logDAO{client: client}
}

// 编译期断言：logDAO 必须实现 dao.LogDAO。
var _ dao.LogDAO = (*logDAO)(nil)

// Create 落库单条日志。
func (d *logDAO) Create(ctx context.Context, l *model.Log) (*model.Log, error) {
	row, err := d.newCreateBuilder(l).Save(ctx)
	if err != nil {
		return nil, err
	}
	return toLogModel(row), nil
}

// BatchCreate 批量落库日志，供 LogService 异步 flusher 调用。
func (d *logDAO) BatchCreate(ctx context.Context, logs []*model.Log) error {
	if len(logs) == 0 {
		return nil
	}
	builders := make([]*ent.AccessLogCreate, 0, len(logs))
	for _, l := range logs {
		builders = append(builders, d.newCreateBuilder(l))
	}
	_, err := d.client.AccessLog.CreateBulk(builders...).Save(ctx)
	return err
}

// newCreateBuilder 构造单条日志的创建 builder，供 Create / BatchCreate 复用。
// 空字符串的可选字段写 NULL（更利于按 method IS NULL 过滤），level 为空时兜底为 info。
func (d *logDAO) newCreateBuilder(l *model.Log) *ent.AccessLogCreate {
	lvl := accesslog.LevelInfo
	if l.Level != "" {
		lvl = accesslog.Level(l.Level)
	}
	ts := l.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	return d.client.AccessLog.Create().
		SetTimestamp(ts).
		SetLevel(lvl).
		SetEvent(l.Event).
		SetNillableMethod(nillableStr(sanitizeLogText(l.Method))).
		SetNillablePath(nillableStr(sanitizeLogText(l.Path))).
		SetNillableStatus(l.Status).
		SetNillableDurationMs(l.DurationMs).
		SetNillableClientIP(nillableStr(sanitizeLogText(l.ClientIP))).
		SetNillableRequestID(nillableStr(sanitizeLogText(l.RequestID))).
		SetNillableAPIKeyID(l.APIKeyID).
		SetNillableUsername(nillableStr(sanitizeLogText(l.Username))).
		SetMessage(sanitizeLogText(l.Message))
}

// List 按 LogQuery 过滤 / 分页返回日志（按 timestamp 倒序），同时给出总条数。
func (d *logDAO) List(ctx context.Context, q model.LogQuery) ([]*model.Log, int, error) {
	preds := buildLogPreds(q)

	total, err := d.client.AccessLog.Query().Where(preds...).Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := d.client.AccessLog.Query().Where(preds...).Order(ent.Desc(accesslog.FieldTimestamp))
	if q.Offset > 0 {
		query = query.Offset(q.Offset)
	}
	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*model.Log, 0, len(rows))
	for _, r := range rows {
		items = append(items, toLogModel(r))
	}
	return items, total, nil
}

// CountByRange 统计 [start, end) 时间区间的日志条数，供直方图按日聚合。
func (d *logDAO) CountByRange(ctx context.Context, start, end time.Time) (int, error) {
	return d.client.AccessLog.Query().
		Where(accesslog.TimestampGTE(start), accesslog.TimestampLT(end)).
		Count(ctx)
}

// Count 返回日志总量，供仪表盘统计。
func (d *logDAO) Count(ctx context.Context) (int64, error) {
	n, err := d.client.AccessLog.Query().Count(ctx)
	if err != nil {
		return 0, err
	}
	return int64(n), nil
}

// ClearAll 清空全部日志，返回删除条数。
func (d *logDAO) ClearAll(ctx context.Context) (int64, error) {
	n, err := d.client.AccessLog.Delete().Exec(ctx)
	if err != nil {
		return 0, err
	}
	return int64(n), nil
}

// ClearInfoGet 删除全部 info 级 GET 请求日志，返回删除条数。
// 复用 buildLogPreds 组装 Level+Method 谓词；审计 / 业务事件 method 为 NULL，不会被命中。
func (d *logDAO) ClearInfoGet(ctx context.Context) (int64, error) {
	preds := buildLogPreds(model.LogQuery{Level: model.LevelInfo, Method: http.MethodGet})
	n, err := d.client.AccessLog.Delete().Where(preds...).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return int64(n), nil
}

// buildLogPreds 把 LogQuery 翻译为 Ent 谓词。
func buildLogPreds(q model.LogQuery) []predicate.AccessLog {
	var preds []predicate.AccessLog
	if q.Level != "" {
		preds = append(preds, accesslog.LevelEQ(accesslog.Level(q.Level)))
	}
	if q.Event != "" {
		preds = append(preds, accesslog.EventEQ(q.Event))
	}
	if q.Method != "" {
		preds = append(preds, accesslog.MethodEQ(q.Method))
	}
	if q.RequestID != "" {
		preds = append(preds, accesslog.RequestIDEQ(q.RequestID))
	}
	if q.APIKeyID != nil {
		preds = append(preds, accesslog.APIKeyIDEQ(*q.APIKeyID))
	}
	if !q.Start.IsZero() {
		// 对齐到服务器本地时区再比较：modernc 按 t.String() 文本绑定 time.Time，SQLite 按字节序比较，
		// 存储用 time.Now()（本地时区），查询参数须用同一时区偏移才能保证字节序与时刻序一致。
		preds = append(preds, accesslog.TimestampGTE(q.Start.In(time.Local)))
	}
	if !q.End.IsZero() {
		preds = append(preds, accesslog.TimestampLT(q.End.In(time.Local)))
	}
	switch q.StatusClass {
	case "2xx":
		preds = append(preds, accesslog.StatusGTE(200), accesslog.StatusLT(300))
	case "4xx":
		preds = append(preds, accesslog.StatusGTE(400), accesslog.StatusLT(500))
	case "5xx":
		preds = append(preds, accesslog.StatusGTE(500), accesslog.StatusLT(600))
	}
	if q.Keyword != "" {
		preds = append(preds, accesslog.Or(accesslog.PathContains(q.Keyword), accesslog.MessageContains(q.Keyword)))
	}
	return preds
}

// nillableStr 把空字符串转为 nil 指针，使可选字段写 NULL。
func nillableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// sanitizeLogText 把 CR/LF 替换为空格，防止用户可控字符串（用户名 / 路径 / 消息等）
// 在日志中心伪造换行造成日志注入（CWE-117）。在 DAO 写入边界统一处理，覆盖所有日志来源。
func sanitizeLogText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, s)
}

// toLogModel 将 Ent 实体转换为跨层的 model.Log。
func toLogModel(e *ent.AccessLog) *model.Log {
	if e == nil {
		return nil
	}
	return &model.Log{
		ID:         e.ID,
		Timestamp:  e.Timestamp,
		Level:      string(e.Level),
		Event:      e.Event,
		Method:     e.Method,
		Path:       e.Path,
		Status:     e.Status,
		DurationMs: e.DurationMs,
		ClientIP:   e.ClientIP,
		RequestID:  e.RequestID,
		APIKeyID:   e.APIKeyID,
		Username:   e.Username,
		Message:    e.Message,
		CreatedAt:  e.CreatedAt,
	}
}
