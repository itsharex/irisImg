# 日志中心说明（irisImg 后端）

> irisImg 用一套统一的**日志中心**同时承担「访问日志」与「业务事件审计」：每个 HTTP 请求、每次 panic、每个关键业务动作（登录 / 上传 / 密钥签发·吊销·删除 / 日志清理）都落进同一张 `logs` 表，供后台分页查询、14 天直方图与一键清理。
> 本文档面向使用/调试本服务的人，跨文件讲清楚「日志怎么产生」「怎么异步落库」「日志中心怎么查 / 怎么清」。
> 各 `.go` 文件的逐文件文档见各自目录的 `.md`；密钥鉴权见 [`APIKEY.md`](./APIKEY.md)，登录链路见 [`AUTH.md`](./AUTH.md)。

---

## 1. 整体设计

- **zap 强类型内核**：访问日志与运行时日志统一走 [`internal/pkg/logger`](./internal/pkg/logger.md) 包装的 `*zap.Logger`，用类型化 `zap.Field` 构造器（`zap.String/Int/Duration/Error`）避免 `interface{}` 装箱，输出到 stdout / 文件供运维采集。
- **统一落 `logs` 表**：访问日志（`event=http.request`）与业务事件（`event=image.upload` / `event=image.delete` / `apikey.*` / `auth.*` / `log.clear` / `panic`）写入同一张表，按 `event` 区分；HTTP 字段（method/path/status/duration_ms）仅访问日志有值，其余为 NULL。
- **异步批量写入**：[`service.LogService`](./internal/service/log.md) 持有容量 2048 的缓冲通道，`Record` 非阻塞推入；后台 flusher 协程攒满 200 条或满 1 秒调 `dao.BatchCreate` 一次性落库，使请求处理零 DB 写延迟。
- **request id 串联**：[`middleware.RequestID`](./internal/middleware/requestid.md) 为每个请求生成 / 透传 `X-Request-Id`，写入 `gin.Context` 与 `c.Request.Context()`；同一请求的访问日志、业务事件、panic 经 `request_id` 关联。
- **日志中心查询**：后台 JWT 登录后可分页 + 多维过滤查询、看 14 天每日直方图；清理为敏感操作，需账号密码二次确认。
- **清理留审计**：清空日志后会立即补记一条 `event=log.clear` 的审计事件（由本服务异步落库），故日志中心清空后仍可见「谁清的、清了多少」。

参与的代码文件：

| 角色 | 文件 |
| --- | --- |
| 日志内核 | `internal/pkg/logger/logger.go` |
| 中间件 | `internal/middleware/requestid.go`、`internal/middleware/recovery.go`、`internal/middleware/logger.go` |
| 业务逻辑 | `internal/service/log.go` |
| 控制器 | `internal/api/log.go` |
| Schema | `ent/schema/log.go` |
| DTO | `internal/model/log.go` |
| DAO 接口 | `internal/dao/dao.go`（`LogDAO`） |
| DAO 实现 | `internal/dao/entdao/log.go` |
| 配置 | `config/config.go`、`config/config.yaml`（`logger` 段） |
| 路由装配 | `internal/router/router.go` |
| 统一响应 | `internal/pkg/response/response.go`（复用错误码） |
| 二次确认 | `internal/service/auth.go`（`VerifyCredentials`） |

## 2. 配置

```yaml
logger:
  level: "info"          # debug|info|warn|error，缺省 info
  encoding: "json"       # json|console，缺省 json
  output: "stdout"       # stdout|stderr|<文件路径>，缺省 stdout
  time_format: "iso8601" # iso8601|rfc3339|epoch，缺省 iso8601
```

> 此处控制的是 **zap 输出到 stdout / 文件**的部分（运维采集）。访问日志与业务事件**另**异步落 `logs` 表供日志中心查询，两条通道相互独立。所有字段都有合理默认值。详见 [`config.md`](./config/config.md)。

## 3. 接口一览

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v1/admin/logs` | JWT + HTTPS | 分页 + 多维过滤查询日志（按 timestamp 倒序） |
| GET | `/api/v1/admin/logs/histogram` | JWT + HTTPS | 最近 14 天每日日志量（缺日补零），供直方图 + 趋势线 |
| DELETE | `/api/v1/admin/logs` | JWT + HTTPS + 密码 | 清空全部日志；请求体需 `{username,password}` 二次确认，失败返 **403** |

> 三个接口均挂在 JWT 受保护组下的 `/admin/logs` 子组，并叠加 [`HTTPSOnly`](./internal/middleware/https.md)（生产由 `apikey.https_only` 开启）。

## 4. 请求链路时序

### 4.1 中间件链

装配见 [`router.New`](./internal/router.md)，顺序固定为：

```
RequestID  ->  Recovery(lg, logSvc)  ->  CORS  ->  Logger(lg, logSvc)  ->  handler
```

- [`RequestID`](./internal/middleware/requestid.md) 最前：后续中间件 / handler 均可从 `gin.Context` 或 `c.Request.Context()` 取到 request id。
- [`Recovery`](./internal/middleware/recovery.md) 捕获 panic：用 zap 记堆栈，并落库一条 `event=panic` 的日志，再返回 500（替换 `gin.Recovery()`，使 panic 也进入日志中心可查）。
- [`Logger`](./internal/middleware/logger.md) 在 `c.Next()` 后置记录：用 zap 输出结构化访问日志，同时把一条 `event=http.request` 的日志异步落库。跳过 `/api/v1/ping` 健康检查以减噪。

### 4.2 单请求：访问日志 + 业务事件

```
client      RequestID      Recovery       Logger(后置)       handler       LogService.Record
  │ GET /x    │              │               │                 │              │
  │ ─────────►│ 生成/透传 rid │               │                 │              │
  │           │ ctx+resp头    │               │                 │              │
  │           │ ─ Next ─────►│ defer recover  │                 │              │
  │           │              │ ─ Next ─────►│ c.Next() ──────►│ 业务逻辑       │
  │           │              │               │                 │ ─ Record(业务事件)─►│ buf
  │           │              │               │ ◄─ handler 返回  │              │
  │           │              │               │ zap 输出 http.request          │
  │           │              │               │ ─ Record(http.request)───────►│ buf
  │ ◄─ 响应 ──│              │               │                 │              │
  │           │              │ panic?        │                 │              │
  │           │              │ ─ Record(panic)────────────────────────────────►│ buf
```

- 访问日志的级别由 HTTP 状态码推导：`≥500` error、`≥400` warn、否则 info（见 `levelFromStatus`）。
- 业务事件由各控制器显式调用 `logSvc.Record(model.NewEventLog(...))`，级别与消息由调用方指定。

### 4.3 异步 flusher 批量落库

```
LogService.buf(容量2048)      flusher 协程                   dao.LogDAO.BatchCreate
  │ Record 推入                  │                            │
  │ ──────────────────────────►│ 攒批(≤200) / 满 1s           │
  │ (满则丢弃 + zap.Warn)       │ ─ BatchCreate(batch) ─────►│ CreateBulk 落库
  │                             │ ◄── err? zap.Error ────────│
  │ Close(): close(done)        │                            │
  │ ──────────────────────────►│ 排空+flush 残余再退出        │
```

- `Record` 用 `select { case buf <- l: case <-done: default: 丢弃 }`：`done` 关闭后经 done 分支安全返回**绝不 panic**，缓冲满则丢弃并 `zap.Warn`，**绝不阻塞请求**。
- flusher 每 1 秒或满 200 条触发一次 `BatchCreate`，单次调用带 5 秒超时 ctx；收到 `flushReq`（`ClearAll` 的 `flushSync`）时先排空 `buf` 再 flush 并回信。
- `Close()` 关闭 `done` 通道通知 flusher，flusher 排空 `buf` 残余并 flush 落库后退出，`Close` 等其结束才返回。可安全在 `Record` 仍被调用时关闭。

### 4.4 调用关系

| 调用方 | 被调方 | 时机 |
| --- | --- | --- |
| `middleware.Logger` | `LogService.Record` | 每个非 ping 请求结束后（`http.request`） |
| `middleware.Recovery` | `LogService.Record` | 捕获到 panic 时（`panic`） |
| `api.LogAPI` / `api.AuthAPI` / `api.APIKeyAPI` / `api.ImageAPI` | `LogService.Record` | 业务事件发生时（`auth.*` / `apikey.*` / `image.upload` / `image.delete` 等） |
| `LogService.flushLoop` | `dao.LogDAO.BatchCreate` | 攒满 200 条 / 满 1 秒 / `flushReq`（ClearAll 同步 flush）/ 关闭时 |
| `api.LogAPI.List` | `LogService.List` -> `dao.LogDAO.List` | 日志中心分页查询 |
| `api.LogAPI.Histogram` | `LogService.Histogram` -> `dao.LogDAO.CountByRange` | 直方图按日聚合 |
| `api.LogAPI.Clear` | `AuthService.VerifyCredentials` -> `LogService.ClearAll`（`flushSync` -> `dao.LogDAO.ClearAll` -> `Record(log.clear)`） | 清理日志（含二次确认 + 先 flush 再清空 + 审计补记） |

## 5. 数据模型

### 5.1 `logs` 表字段（[`ent/schema/log.go`](./ent/schema/log.md)）

> ent 预声明了 `Log` 标识符，故 Go 侧 schema 类型用 `AccessLog`；实际表名经 `entsql.Annotation{Table: "logs"}` 定为 `logs`。`api_key_id` **不建 Edge**，避免高频插入的外键开销，仅以普通字段记录来源密钥 ID。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `timestamp` | time | 日志发生时间，默认 `now`、不可变；用于排序 / 直方图 / 时间范围过滤 |
| `level` | enum(debug/info/warn/error) | 日志级别，默认 info；访问日志按 HTTP 状态推导 |
| `event` | string | 事件类型，非空（见下表） |
| `method` | string? | HTTP 方法，仅访问日志有值（NULL 利于 `IS NULL` 过滤） |
| `path` | string? | 请求路径，供关键字模糊匹配 |
| `status` | *int? | HTTP 状态码，非请求类事件为 NULL |
| `duration_ms` | *int? | 请求耗时（毫秒），仅访问日志有值 |
| `client_ip` | string? | 客户端 IP |
| `request_id` | string? | 请求追踪 ID，关联同一请求的多条日志 |
| `api_key_id` | *int? | 来源 API 密钥 ID，无关联时为 NULL |
| `username` | string? | 操作者用户名（JWT 登录用户） |
| `message` | string | 可读描述 / 错误信息，默认 `""` |
| `created_at` | time | 记录落库时间，默认 `now` |

索引：`timestamp`、`level`、`event`、`request_id`、`api_key_id`。无 Edge。

### 5.2 事件类型常量（[`model/log.go`](./internal/model/log.md)）

| 常量 | 值 | 产生方 |
| --- | --- | --- |
| `EventHTTPRequest` | `http.request` | `middleware.Logger` |
| `EventPanic` | `panic` | `middleware.Recovery` |
| `EventImageUpload` | `image.upload` | `api.ImageAPI` |
| `EventImageDelete` | `image.delete` | `api.ImageAPI`（批量删除图片，warn） |
| `EventAPIKeyCreate` | `apikey.create` | `api.APIKeyAPI` |
| `EventAPIKeyRename` | `apikey.rename` | `api.APIKeyAPI` |
| `EventAPIKeyReset` | `apikey.reset` | `api.APIKeyAPI` |
| `EventAPIKeyRevoke` | `apikey.revoke` | `api.APIKeyAPI` |
| `EventAPIKeyDelete` | `apikey.delete` | `api.APIKeyAPI` |
| `EventAuthLoginOK` | `auth.login_success` | `api.AuthAPI` |
| `EventAuthLoginFail` | `auth.login_failed` | `api.AuthAPI` |
| `EventLogClear` | `log.clear` | `service.LogService.ClearAll`（审计补记） |

### 5.3 关键方法

| 层 | 方法 | 职责 |
| --- | --- | --- |
| `logger.Logger` | `Debug/Info/Warn/Error(ctx, msg, fields...)` | 类型化字段记录；自动从 ctx 注入 `request_id` |
| `logger.Logger` | `Named` / `With` / `Sync` / `Zap` | 派生子 logger / 附加固定字段 / flush 缓冲 / 取原生 zap |
| `logger` | `ContextWithRequestID(ctx, id)` | 把 request id 写入 ctx（`RequestID` 中间件调用） |
| `middleware` | `LogContextFromGin(c)` | 从 gin.Context 抽取 request id / username / api_key_id / client_ip |
| `service.LogService` | `Record(l)` | 非阻塞推入缓冲；关闭后经 done 安全返回，满则丢弃 + 告警 |
| `service.LogService` | `List(ctx, q)` / `Histogram(ctx, days)` / `ClearAll(ctx, lc)` | 同步查询 / 直方图 / 清理（flushSync 后清空再补记 `log.clear`） |
| `service.LogService` | `Close()` | 关闭 done 通知 flusher 排空+flush 残余后退出（须在 DB 关闭之前） |
| `dao.LogDAO` | `Create` / `BatchCreate` / `List` / `CountByRange` / `ClearAll` | 持久化抽象，唯一实现 `entdao.logDAO` |

## 6. 错误码表

日志中心复用全局错误码（见 [`response.md`](./internal/pkg/response.md)），不新增：

| code | HTTP | 常量 | 含义 |
| --- | --- | --- | --- |
| 0 | 200 | `CodeOK` | 成功 |
| 40000 | 400 | `CodeBadRequest` | 入参非法（page / page_size 越界、`api_key_id` 非正整数、`start`/`end` 非 RFC3339、请求体非合法 JSON） |
| 40100 | 401 | `CodeUnauthorized` | JWT 未登录 / 无效（管理接口） |
| 40300 | 403 | `CodeForbidden` | 清理日志密码二次确认失败 / 未走 HTTPS |
| 40400 | 404 | `CodeNotFound` | 日志中心接口当前不触发（列表空返回空数组、直方图空返回零、清空空表返回 `deleted:0`）；列出以与全局错误码体系对齐 |
| 50000 | 500 | `CodeServerError` | 查询 / 直方图 / 清理时 DB 错误 |

> **403 而非 401**：清理日志的密码二次确认失败返回 403。前端 `useApi` 会把 401 当作 JWT 失效并全局登出；403 则交由调用方就地提示「用户名或密码错误」，与密钥吊销 / 删除的同款机制一致。

## 7. 安全与排错

- **密码二次确认返 403 不触发登出**：`DELETE /admin/logs` 复用 `model.DestructiveRequest` + `AuthService.VerifyCredentials`（`subtle.ConstantTimeCompare` 防时序攻击），失败返 403。前端按 403 就地报错，不会清登录态。
- **缓冲满丢弃 / 关闭后安全**：缓冲通道容量 2048，`Record` 用 `select { case buf <- l: case <-done: default: 丢弃 }`；`done` 关闭后经 done 分支直接返回，**绝不触发 send on closed channel panic**，故 `main` 在 `srv.Shutdown` 超时后仍可安全调用 `Close`（即便有在途 handler 仍在 `Record`）。极端场景下缓冲满则丢弃并 `zap.Warn`，**绝不阻塞请求**。被丢弃的日志仅在 stdout 可见，不入库。
- **`Close` 须先于 DB 关闭 / Shutdown 超时不 Fatalf**：优雅关闭顺序为 `srv.Shutdown`（超时只记错误不 `Fatalf`，避免 `os.Exit` 跳过 `logSvc.Close` 与 defer）-> `logSvc.Close()`（关闭 `done`，flusher 排空+flush 残余到 DB）-> `defer dbClient.Close()`（见 [`cmd/server/main.go`](./cmd/server/main.md)）。若先关 DB 再 Close，残余日志会因 DB 已关而 flush 失败。
- **清理留 `log.clear` 审计 / 先 flush 再清空**：`ClearAll` 先 `flushSync`（同步排空缓冲并落库）确保在途日志已落盘，再物理清空全部日志，最后 `Record` 一条 `event=log.clear` 的审计事件（消息含删除条数）。先 flush 再删可避免缓冲中的旧日志在清空后被 flusher 重新写回库；该审计事件走异步缓冲，故清空后日志中心只留这条审计记录--既能证明「日志被清过」，也能看到「谁清的、清了多少」。
- **日志注入防护（CWE-117）**：DAO 写入边界对 `method` / `path` / `client_ip` / `request_id` / `username` / `message` 统一过 `sanitizeLogText`，把 CR/LF 替换为空格，防止用户可控字符串在日志中心伪造换行造成日志注入。覆盖所有日志来源，控制器无需重复处理。
- **时间范围过滤对齐 `time.Local`**：`start` / `end` 在 DAO 层用 `q.Start.In(time.Local)` / `q.End.In(time.Local)` 对齐服务器本地时区后再比较。modernc 驱动按 `t.String()` 文本绑定 `time.Time`、SQLite 按字节序比较；存储用 `time.Now()`（本地时区），查询参数须用同一时区偏移才能保证字节序与时刻序一致。
- **`status_class` 白名单**：`buildLogPreds` 仅识别 `2xx` / `4xx` / `5xx` 三种取值并翻译为对应状态码区间谓词（`2xx`->[200,300)、`4xx`->[400,500)、`5xx`->[500,600)），其他值（含空串）忽略不报错；前端过滤请用这三者之一。
- **`/api/v1/ping` 不入日志中心**：健康检查高频且无业务价值，`middleware.Logger` 显式跳过，但仍会走 zap stdout 输出（若需静默可在中间件内一并跳过）。
- **`api_key_id` 不建 Edge**：日志高频写入，外键约束会拖慢插入；仅以普通 `*int` 字段记录来源密钥 ID，密钥被删除后该字段保留为悬空 ID（不级联、不报错）。
- **直方图按日聚合走 N 次 `CountByRange`**：14 天即 14 次 count 查询，依赖 `timestamp` 索引；数据量极大时可改为 SQL `GROUP BY date` 单次聚合。
- **内存缓冲不抗多实例与重启**：进程重启时缓冲内未 flush 的日志丢失；多实例部署需换共享队列（如 Redis Stream / Kafka）。

## 8. 示例

```bash
# 0) 后台登录拿 JWT（见 AUTH.md）
TOKEN="eyJ..."

# 1) 分页查询日志：第 1 页、每页 20 条，只看 5xx 且关键字含 /images
curl -G http://localhost:8080/api/v1/admin/logs \
     -H "Authorization: Bearer $TOKEN" \
     --data-urlencode "page=1" \
     --data-urlencode "page_size=20" \
     --data-urlencode "status_class=5xx" \
     --data-urlencode "keyword=/images"
# -> {"code":0,...,"data":{"items":[...],"total":N,"page":1,"page_size":20}}

# 2) 按 request_id 串联同一请求的全部日志
curl -G http://localhost:8080/api/v1/admin/logs \
     -H "Authorization: Bearer $TOKEN" \
     --data-urlencode "request_id=<X-Request-Id>"

# 3) 14 天直方图（供前端画趋势线）
curl http://localhost:8080/api/v1/admin/logs/histogram \
     -H "Authorization: Bearer $TOKEN"
# -> {"code":0,...,"data":{"buckets":[{"date":"2026-07-08","count":123},...],"total":1234}}

# 4) 清空全部日志（敏感操作：账号密码二次确认，失败返 403）
curl -X DELETE http://localhost:8080/api/v1/admin/logs \
     -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"admin123"}'
# -> {"code":0,...,"data":{"deleted":1234}}
# 密码错误 -> 403 CodeForbidden（前端不会登出）
```

## 9. 扩展与修改建议

- **改缓冲 / 批次参数**：调 `service/log.go` 顶部常量 `logBufferSize` / `logBatchSize` / `logFlushInterval`；写入峰值远高于落库速度时优先调大缓冲，避免丢弃。
- **加日志归档 / TTL**：当前仅支持一键清空；如需按天保留，可在 `LogDAO` 加 `DeleteBefore(t)` 并由定时任务调用，配套在 `LogService` 暴露。
- **直方图性能优化**：数据量大时把 `Histogram` 的 N 次 `CountByRange` 换成单条 `GROUP BY date(timestamp)` 聚合查询，需在 `LogDAO` 新增方法。
- **多实例部署**：把 `buf` 通道换成 Redis Stream / Kafka，`flusher` 改为消费共享队列；`Record` 仍保持非阻塞语义。
- **按事件分流输出**：如需把 `panic` / `log.clear` 单独推送告警，可在 `Record` 前按 `event` 路由到额外 sink，不影响 `logs` 主表写入。
- **新增事件类型**：在 `model/log.go` 加常量，由对应控制器调 `logSvc.Record(model.NewEventLog(...))`；`logs` 表 `event` 为自由字符串无需迁移，但建议在文档与本表同步登记。
