# internal/model/log.go

日志中心的**跨层数据载体**（实体与请求 / 统计 DTO）。独立于 Ent 生成的 `ent.AccessLog`（Go 侧 schema 类型为 `AccessLog`，表名经注解定为 `logs`）：DAO 层负责二者转换（见 [`entdao/log.go`](../dao/entdao/log.md) 的 `toLogModel` 与 `newCreateBuilder`），使 service / api 层不直接依赖 Ent。

## 常量

### 日志级别

```go
const (
    LevelDebug = "debug"
    LevelInfo  = "info"
    LevelWarn  = "warn"
    LevelError = "error"
)
```

写入 `Log.Level`，供列表按级别过滤。

### 事件类型

```go
const (
    EventHTTPRequest   = "http.request"        // 每条 HTTP 请求（访问日志，由中间件批量落库）
    EventImageUpload   = "image.upload"        // 图片上传
    EventImageDelete   = "image.delete"        // 批量删除图片（warn）
    EventAPIKeyCreate  = "apikey.create"       // 密钥创建
    EventAPIKeyRename  = "apikey.rename"       // 密钥重命名
    EventAPIKeyReset   = "apikey.reset"        // 密钥重置（明文重发）
    EventAPIKeyRevoke  = "apikey.revoke"       // 密钥吊销
    EventAPIKeyDelete  = "apikey.delete"       // 密钥删除
    EventAuthLoginOK   = "auth.login_success"  // 登录成功
    EventAuthLoginFail = "auth.login_failed"   // 登录失败
    EventLogClear      = "log.clear"           // 日志清理（全量清空）
    EventLogClearGet   = "log.clear_get"       // 日志清理（仅 info 级 GET 请求日志）
    EventPanic         = "panic"               // panic 恢复记录
)
```

写入 `Log.Event`，供列表按事件类型过滤、按事件区分审计与系统异常。

## 类型

### `Log`

日志实体，字段与 [`ent/schema/log.go`](../../ent/schema/log.md) 一一对应：`ID`、`Timestamp`、`Level`、`Event`、`Method`、`Path`、`Status`、`DurationMs`、`ClientIP`、`RequestID`、`APIKeyID`、`Username`、`Message`、`CreatedAt`。

- `Method` / `Path` / `Status` / `DurationMs` 为 HTTP 专属字段，仅访问日志（`EventHTTPRequest`）由中间件填充；业务事件日志这些字段留空，故带 `omitempty`。
- `Status`、`DurationMs`、`APIKeyID` 为指针类型：`nil` 表示该条日志无此属性，序列化时省略；`APIKeyID` 仅当请求经由 API 密钥发起时才有值（库中不建 Edge，仅以普通字段记录来源密钥 ID）。
- `RequestID` / `Username` / `ClientIP` 由 `LogContext` 注入，用于跨条日志串联同一请求或同一身份。

### `LogContext`

从 `gin.Context` 抽取的、写日志时附带的关联信息，由 middleware 装配后传给 `NewEventLog`。

| 字段 | 说明 |
|------|------|
| `RequestID` | 请求追踪 ID |
| `Username` | 当前登录用户名（未登录为空） |
| `APIKeyID` | 来源密钥 ID（密钥请求才有，否则 `nil`） |
| `ClientIP` | 客户端 IP |

### `NewEventLog`

构造一条业务事件日志的便捷函数：自动填充 `Timestamp`（`time.Now()`）与来自 `LogContext` 的关联字段，调用方只需提供 `event` / `level` / `msg`。访问日志（含 HTTP 字段）不由此构造，而是由中间件直接组装 `*Log`。

### `LogQuery`

日志列表的过滤 / 分页条件，由 service 层解析查询参数后传入 DAO。

| 字段 | 说明 |
|------|------|
| `Level` | 精确匹配级别，空表示不过滤 |
| `Event` | 精确匹配事件类型，空表示不过滤 |
| `Method` | 精确匹配 HTTP 方法，空表示不过滤 |
| `StatusClass` | `"2xx"` / `"4xx"` / `"5xx"`，空表示不过滤 |
| `Keyword` | 对 `path` / `message` 模糊匹配，空表示不过滤 |
| `RequestID` | 精确匹配 `request_id`，空表示不过滤 |
| `APIKeyID` | 精确匹配来源密钥，`nil` 表示不过滤 |
| `Start` | 时间下界（含），零值表示不过滤 |
| `End` | 时间上界（不含），零值表示不过滤 |
| `Offset` / `Limit` | 分页 |

### `DailyCount`

直方图单日计数，用于日志趋势图。

| 字段 | 说明 |
|------|------|
| `Date` | 日期，`YYYY-MM-DD` |
| `Count` | 当日日志条数 |

### `DestructiveRequest`

清理日志等敏感操作的请求体，含 `Username` / `Password`（均 `binding:"required"`）与可选 `Scope`。复用与 API 密钥吊销 / 删除相同的二次确认机制：后端用 `subtle.ConstantTimeCompare` 校验，**失败返回 403（而非 401）**，避免触发前端全局登出。作为 JWT 登录态之上的二次确认。

| 字段 | 说明 |
|------|------|
| `Username` / `Password` | 二次确认凭据，`binding:"required"` |
| `Scope` | 清理范围：空 / `"all"` 清空全部（默认，向后兼容）；`"get"` 仅清 info 级 GET 请求日志。非法值由控制器返回 400 |

## 调用关系

- 被 [`service.LogService`](../service/log.md) 构造与消费。
- 与 `ent.AccessLog` 的双向转换在 [`dao/entdao`](../dao/entdao/log.md) 完成。
- `LogContext` 由日志中间件从 `gin.Context` 装配后注入 `NewEventLog`。
