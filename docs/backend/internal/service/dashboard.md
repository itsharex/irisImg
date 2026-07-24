# internal/service/dashboard.go

仪表盘聚合统计的业务逻辑层，一次性汇总首页所需的图片总量、存储占用、APIkey 计数、日志总量与近 N 天上传趋势。只读、无副作用、不记业务事件。

## 类型

- `DashboardService`：持有 `imageDAO` / `apiKeyDAO` / `logDAO` 三个 DAO（沿用本仓库 service 仅依赖 DAO 的既有模式，不依赖其他 service）。
- `NewDashboardService(imageDAO, apiKeyDAO, logDAO) *DashboardService`：构造函数。
- 常量 `dashboardTrendDays = 30`：趋势窗口默认天数。

## 方法

### `Overview(ctx, days) (*model.DashboardOverview, error)`

`days <= 0` 兜底为 `dashboardTrendDays`（30）。聚合步骤：

1. 图片总量 / 存储大小：`imageDAO.Count` / `imageDAO.TotalSize`。
2. APIkey 计数：`apiKeyDAO.List` 一次拉全量后内存按 `Revoked` 分桶，得总数 / 有效 / 已吊销（避免多次 Count 往返）；同批 keys 顺带构建 `id->name` 映射 `nameByID`，供 `uploadTrend` 解析每张图片的来源密钥标签。
3. 日志总量：`logDAO.Count`。
4. 近 N 天上传趋势（含按来源拆分）：调 `uploadTrend(ctx, days, nameByID)`。

任一 DAO 调用出错即整体返回错误（不部分降级），由 handler 转 500。

### `uploadTrend(ctx, days, nameByID) ([]model.DashboardTrendDay, int, error)`

与 [`LogService.Histogram`](./log.md) 同构的按日循环：用 `time.Now().Location()` 构造今日午夜，逐日左闭右开区间调 `imageDAO.CountByRangeGrouped(dayStart, dayEnd)`，缺日因查询返回空分组自然补零，结果按日期升序；同时累加合计。每个分组据 `nameByID` 解析为密钥标签（`KeyID=nil` -> `"admin"`；未命中兜底 `"已删除密钥"`，理论不会发生——删除密钥会级联删除其图片），生成 `[]model.KeyCount` 并按 `Count` 降序，写入 `DashboardTrendDay.Keys`（当日无上传时为 `nil`，序列化 `omitempty` 省略）；当日合计 = 各分组之和，写入 `DashboardTrendDay.Count`。

## 与其它文件的关系

- 依赖：[`dao.ImageDAO`](../dao/dao.md)（`Count`/`TotalSize`/`CountByRangeGrouped`）、[`dao.APIKeyDAO`](../dao/dao.md)（`List`）、[`dao.LogDAO`](../dao/dao.md)（`Count`）。
- 返回：[`model.DashboardOverview`](../model/dashboard.md)。
- 被调方：[`api.DashboardAPI.Overview`](../api/dashboard.md)。
- 装配：[`router.New`](../router/router.md) 构造后注入。
- 端到端说明见 [`DASHBOARD.md`](../../DASHBOARD.md)。
