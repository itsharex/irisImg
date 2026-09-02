# internal/dao/entdao/log.go

[`dao.LogDAO`](../dao.md) 的 Ent 实现。ent 预声明了 `Log` 标识符，故底层实体为 `ent.AccessLog`（表名经注解定为 `logs`）；本文件在 [`model.Log`](../../model/log.md) 与 `ent.AccessLog` 之间转换，上层只感知 `model.Log`。

## 类型

- `logDAO`：持有 `*ent.Client` 的非导出实现类型。
- `NewLogDAO(client *ent.Client) dao.LogDAO`：构造函数。
- 编译期断言 `var _ dao.LogDAO = (*logDAO)(nil)` 保证接口一致。

## 方法

逐一实现 `LogDAO` 接口：

| 方法 | 实现要点 |
|------|---------|
| `Create` | `newCreateBuilder(l).Save(ctx)` 落库单条日志，回填自增 ID 与时间戳 |
| `BatchCreate` | 空切片直接返回 nil；否则对每条日志复用 `newCreateBuilder` 构造 builder，`AccessLog.CreateBulk(...).Save(ctx)` 批量落库，供 `LogService` 异步 flusher 调用 |
| `List` | `buildLogPreds(q)` 翻译过滤条件；先 `Count` 取总数，再 `Order(ent.Desc(accesslog.FieldTimestamp))` 按 timestamp 倒序分页（`q.Offset`/`q.Limit` 为正才生效），逐行经 `toLogModel` 返回 |
| `CountByRange` | `Where(accesslog.TimestampGTE(start), accesslog.TimestampLT(end)).Count(ctx)`，统计 [start, end) 区间条数，供直方图按日聚合 |
| `Count` | `AccessLog.Query().Count(ctx)` 返回日志总量（`int` -> `int64`），供仪表盘统计 |
| `ClearAll` | `AccessLog.Delete().Exec(ctx)` 清空全部日志，返回删除条数（`int64`） |
| `ClearInfoGet` | 复用 `buildLogPreds(model.LogQuery{Level: "info", Method: "GET"})` 组装 Level+Method 谓词后 `AccessLog.Delete().Where(preds...).Exec(ctx)` 条件删除，返回删除条数；审计 / 业务事件 method 为 NULL，不会被命中 |

## 辅助函数

- `newCreateBuilder(l *model.Log) *ent.AccessLogCreate`：构造单条日志的创建 builder，供 `Create` / `BatchCreate` 复用。`level` 为空时兜底为 `accesslog.LevelInfo`；`timestamp` 为零值时取 `time.Now()`；用户可控的文本字段（method / path / client_ip / request_id / username / message）先经 `sanitizeLogText` 净化（把 CR/LF 替换为空格，防日志注入 CWE-117），空字符串的可选字段再经 `nillableStr` 转 nil 指针，使列写 NULL（更利于按 `method IS NULL` 过滤）；`status` / `duration_ms` / `api_key_id` 用 `SetNillable*` 写入。
- `buildLogPreds(q model.LogQuery) []predicate.AccessLog`：把 `LogQuery` 翻译为 Ent 谓词，支持的过滤维度：

  | 维度 | 谓词 |
  |------|------|
  | `Level` | `accesslog.LevelEQ(accesslog.Level(q.Level))` |
  | `Event` | `accesslog.EventEQ(q.Event)` |
  | `Method` | `accesslog.MethodEQ(q.Method)` |
  | `RequestID` | `accesslog.RequestIDEQ(q.RequestID)` |
  | `APIKeyID`（非 nil） | `accesslog.APIKeyIDEQ(*q.APIKeyID)` |
  | `Start`（非零） | `accesslog.TimestampGTE(q.Start.In(time.Local))` |
  | `End`（非零） | `accesslog.TimestampLT(q.End.In(time.Local))` |
  | `StatusClass` | `2xx` -> `Status∈[200,300)`、`4xx` -> `[400,500)`、`5xx` -> `[500,600)` |
  | `Keyword` | `Or(PathContains(keyword), MessageContains(keyword))`，对 path + message 模糊匹配 |

  > 时间范围过滤一律对齐到服务器本地时区（`q.Start.In(time.Local)` / `q.End.In(time.Local)`）：modernc 驱动按 `t.String()` 文本绑定 `time.Time`，SQLite 又按字节序比较，而存储侧用的是 `time.Now()`（本地时区）；查询参数须采用同一时区偏移，才能保证字节序与时刻序一致，避免边界漏行/错行。

- `nillableStr(s string) *string`：空字符串转 nil 指针，使可选字段写 NULL。
- `sanitizeLogText(s string) string`：把 CR/LF 替换为空格，防止用户可控字符串（用户名 / 路径 / 消息等）在日志中心伪造换行造成日志注入（CWE-117）。在 DAO 写入边界统一处理，覆盖所有日志来源；由 `newCreateBuilder` 对 method / path / client_ip / request_id / username / message 调用。
- `toLogModel(e *ent.AccessLog) *model.Log`：把 Ent 实体转换为跨层的 [`model.Log`](../../model/log.md)，使 service / api 不直接依赖 Ent；入参为 nil 时返回 nil。

## 错误与转换

- 与同包 [`image.go`](image.md) / [`apikey.go`](apikey.md) 不同，本文件不使用 `wrapErr`：日志查询无「不存在」语义，`Create` / `BatchCreate` / `List` / `CountByRange` / `Count` / `ClearAll` / `ClearInfoGet` 直接透传底层 Ent 错误。
- `toLogModel` 见上节，承担 Ent -> model 的字段映射。

## 调用关系

被 `service.LogService` 依赖（通过 `dao.LogDAO` 接口）；由 [`cmd/server/main.go`](../../../cmd/server.md) 构造后经 [`router.New`](../../router/router.md) 注入。
