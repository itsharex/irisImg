# `internal/service/log.go`

日志的业务逻辑层：负责日志的**异步批量落库**与**查询 / 直方图 / 清理**。写入走异步批量，使请求处理零 DB 写延迟；查询、直方图、清理走同步 dao 调用。需要发射业务事件的控制器只依赖窄接口 `LogRecorder`，避免耦合完整 `LogService`。

## 类型与变量

### `LogRecorder`

```go
type LogRecorder interface {
    Record(l *model.Log)
}
```

- 日志记录的**窄接口**，供需要发射业务事件（如 `log.clear` 审计）的控制器依赖，避免依赖完整 `LogService`。
- 仅暴露 `Record`，调用方无法触达查询 / 清理等敏感操作。

### `LogService`

```go
type LogService struct {
    dao      dao.LogDAO
    logger   *logger.Logger
    buf      chan *model.Log
    done     chan struct{}
    flushReq chan chan struct{}
    wg       sync.WaitGroup
}
```

- 持有 [`dao.LogDAO`](../dao/dao.md) 接口，由 [`router`](../router/router.md) 注入。
- `buf` 为容量 `logBufferSize`（2048）的缓冲通道，`Record` 非阻塞写入，后台 `flushLoop` 排空；**该通道永不关闭**，退出经 `done` 通知。
- `done` 为关闭信号通道，`Close` 时 `close(s.done)`；`Record` 与 `flushSync` 都 `select` 它以在关闭后安全返回，杜绝 send on closed channel panic。
- `flushReq` 为 `chan chan struct{}`，`flushSync` 借它请求 flusher 立即排空 + flush 并回信，供 `ClearAll` 在删除前同步落盘在途日志。
- `wg` 跟踪 `flushLoop` 协程，`Close` 在 `close(done)` 后 `wg.Wait()` 等待 flusher 排空残余并落库后退出。
- `logger` 仅用于「缓冲满丢弃」与「批量写失败」的告警，不参与业务日志落库。
- 有可变状态（`buf` / `done` / `flushReq` / `wg`），但写路径单协程消费、读路径走 dao，对外 goroutine 安全。

### 常量

| 常量 | 值 | 含义 |
|------|----|------|
| `logBufferSize` | 2048 | 异步缓冲通道容量，满则丢弃并告警 |
| `logBatchSize` | 200 | 单批最大写入条数，达到即触发 `BatchCreate` |
| `logFlushInterval` | 1s | 定时 flush 间隔，即使不足一批也按秒落库 |
| `logQueryDefault` | 50 | `List` 默认页大小 |
| `logHistogramDays` | 14 | `Histogram` 默认天数 |

## 函数

### `NewLogService(d dao.LogDAO, l *logger.Logger) *LogService`

构造 `LogService` 并**启动后台 `flushLoop` 协程**。调用方须在 DB 关闭前调 `Close` 收尾。

### `Record(l *model.Log)`

非阻塞地把一条日志加入缓冲通道：

1. `nil` 直接返回。
2. `Timestamp` 为零值时兜底为 `time.Now()`；`Level` 为空时兜底为 `model.LevelInfo`。
3. `select { case s.buf <- l: case <-s.done: default: 丢弃并告警 }`：
   - 正常写入 `buf`；
   - **`done` 已关闭（服务关闭中）**时经 `done` 分支直接返回，不触发 send on closed channel panic，故即便有在途 handler 仍在 `Record`，`Close` 也可安全调用；
   - **缓冲满**则丢弃该条并告警（`logger.Warn`，带 `event` 字段），绝不阻塞请求。

### `flushLoop()`（私有）

后台排空缓冲的 flusher 协程，由 `NewLogService` 启动：

- 维护一个 `batch`（cap `logBatchSize`）。
- `select` 四路：
  - `buf`：累计到 `logBatchSize`（200）条立即 flush。
  - `ticker.C`：每 `logFlushInterval`（1s）定时 flush。
  - `flushReq`（`chan chan struct{}`）：先排空通道中已缓冲的日志再 flush，最后 `close(done)` 回信——供 `flushSync` 同步落盘所有在途日志。
  - `done`（关闭信号）：排空剩余缓冲后 flush，再 `return` 退出。
- `flush` 用 `5s` 超时 ctx 调 [`dao.BatchCreate`](../dao/dao.md) 落库，失败经 `logger.Error` 告警（带 `n` 条数），**不重试**。
- 退出前 `defer wg.Done()`，使 `Close` 的 `wg.Wait()` 能感知 flusher 已排空并落库完毕。

### `flushSync()`（私有）

同步排空当前缓冲并落库，供 `ClearAll` 在删除前确保在途日志已落盘：

1. 新建一个 `done chan struct{}` 发往 `s.flushReq`，flusher 收到后排空通道、flush 并 `close(done)` 回信。
2. `select { case s.flushReq <- done: <-done; case <-s.done: }`：若 flusher 已退出（`done` 关闭）则直接返回，不阻塞。

### `Close()`

`close(s.done)` 通知 flusher 退出 + `wg.Wait()` 等待其排空剩余缓冲并 flush 落库。**不关闭 `buf` 通道**（关闭会与 `Record` 的写入竞态触发 panic）。**必须在 DB 关闭之前调用**，否则残余日志将随 DB 句柄失效而丢失。

### `List(ctx, q model.LogQuery) ([]*model.Log, int, error)`

同步分页查询（按 `timestamp` 倒序）。`q.Limit <= 0` 时兜底为 `logQueryDefault`（50）。直接转发 [`dao.List`](../dao/dao.md)。

### `Histogram(ctx, days int) ([]model.DailyCount, int, error)`

返回最近 `days` 天的每日计数与总条数：

1. `days <= 0` 时兜底为 `logHistogramDays`（14）。
2. 以本地时区「今天 0 点」为锚，从 `days-1` 天前到今天，逐日调 [`dao.CountByRange`](../dao/dao.md) 取 `[dayStart, dayEnd)` 区间计数。
3. **缺日补零**：无日志的日期仍产出 `DailyCount{Date, Count: 0}`，保证前端直方图连续。
4. 结果按日期**升序**返回，同时累加 `total`。

> 逐日查询共 `days` 次 dao 调用；天窗较大时可考虑改 dao 一次性 `GROUP BY date` 聚合（见修改建议）。

### `ClearAll(ctx, lc model.LogContext) (int64, error)`

清空全部日志并**补记一条 `log.clear` 审计事件**：

1. 先 `flushSync()` 同步排空在途缓冲并落库--避免缓冲中的旧日志在清空后被 flusher 重新写回库（即清空后旧日志不重现）。
2. 同步调 [`dao.ClearAll`](../dao/dao.md) 物理删除，返回删除条数 `n`。
3. 经 `Record(model.NewEventLog(model.EventLogClear, model.LevelInfo, "cleared N logs", lc))` 异步补记审计。
4. 因为审计是在清空**之后**入缓冲的，清空动作完成后日志中心**仍可见这条 `log.clear` 记录**。

### `ClearInfoGet(ctx, lc model.LogContext) (int64, error)`

删除全部 **info 级 GET 请求日志**并**补记一条 `log.clear_get` 审计事件**，流程与 `ClearAll` 完全一致：

1. 先 `flushSync()` 同步排空在途缓冲并落库（同 `ClearAll` 的动机）。
2. 同步调 [`dao.ClearInfoGet`](../dao/dao.md) 条件删除（谓词为 `Level = info AND method = 'GET'`），返回删除条数 `n`。
3. 经 `Record(model.NewEventLog(model.EventLogClearGet, model.LevelInfo, "cleared N info GET logs", lc))` 异步补记审计。
4. 审计事件本身 method 为 NULL，**不会被自己的删除条件命中**；warn/error 级、非 GET 方法与业务事件日志全部保留。

## 与其它包的关系

```
api.LogAPI ──────────────► service.LogService ──► dao.LogDAO (List / Histogram / ClearAll / ClearInfoGet 同步)
其它控制器 ──(LogRecorder)─►        │              └─► dao.BatchCreate (flushLoop 异步)
                                   ├─► pkg/logger (缓冲满 / 批量写失败告警)
                                   └─► flushLoop 协程 ──► dao.BatchCreate
router ──(NewLogService)──────────►│
                                   └─► Close() 须在 DB 关闭前调用
```

- 写路径（`Record` / `ClearAll` / `ClearInfoGet` 的审计）**异步**：经 `buf` 通道由 `flushLoop` 批量落库。
- 读路径（`List` / `Histogram`）与 `ClearAll` / `ClearInfoGet` 的删除**同步**：直接走 dao，结果立即可见。
- 控制器层细节见 [`../../api/log.md`](../../api/log.md)，dao 接口与实现见 [`../dao/dao.md`](../dao/dao.md)。

## 修改建议

- `Histogram` 当前逐日 `CountByRange` 共 `days` 次 dao 调用；若天数变大成为瓶颈，可在 dao 增加 `GROUP BY DATE(timestamp)` 的聚合查询，service 端只做缺日补零。
- 缓冲满时策略为「丢弃 + 告警」以保请求延迟；若日志不可丢，可改为阻塞写入或落盘溢出文件，但会引入背压，需评估对请求链路的影响。
- `Close` 的「DB 关闭前调用」是隐式契约；可在 `router` 的关闭编排里用注释或顺序断言固化，避免依赖装配顺序变更时悄悄破坏。
- `flushLoop` 批量写失败不重试；若需 at-least-once，可保留失败 batch 并在 `Close` 前重试，但需权衡重复写入风险。
