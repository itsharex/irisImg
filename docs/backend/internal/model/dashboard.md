# internal/model/dashboard.go

仪表盘聚合统计接口的返回 DTO，承载首页所需的全部指标。由 [`service.DashboardService.Overview`](../service/dashboard.md) 组装，经 [`api.DashboardAPI.Overview`](../api/dashboard.md) 返回给前端仪表盘页面。

## 类型

### `DashboardOverview`

| 字段 | 类型 | 说明 |
|------|------|------|
| `ImagesTotal` | `int64` | 图片总量（无过滤） |
| `StorageBytes` | `int64` | 全部图片 `size` 之和（字节），取自 DB `SUM(size)`，空表为 0 |
| `APIKeysTotal` | `int` | 密钥总数（含已吊销；已删除为物理删除，不在统计内） |
| `APIKeysActive` | `int` | 未吊销的有效密钥数 |
| `APIKeysRevoked` | `int` | 已吊销密钥数 |
| `LogsTotal` | `int64` | 日志总量 |
| `RecentUploadTrend` | `[]DashboardTrendDay` | 近 N 天每日新增图片（升序、缺日补零），每单元额外携带按来源拆分的 `Keys`，供仪表盘 tooltip 展示「当日每个 key 上传数」 |
| `RecentUploadTotal` | `int` | 近 N 天新增图片合计 |
| `Days` | `int` | 趋势窗口天数（默认 30），回显给前端文案 |

## 关联类型

### `DashboardTrendDay`

仪表盘趋势单日单元，比 [`DailyCount`](./log.md) 多携带按来源拆分。复用 `{Date, Count}` 语义并追加可选 `Keys`：前端 `LogsHistogram` 对 `{date, count}` 结构兼容，仅当 `Keys` 非空时在 tooltip 中渲染来源明细（日志直方图不携带 `Keys`）。

| 字段 | 类型 | 说明 |
|------|------|------|
| `Date` | `string` | 日期，`YYYY-MM-DD` |
| `Count` | `int` | 当日新增图片合计（各来源之和） |
| `Keys` | `[]KeyCount` | 当日按来源拆分（按 `Count` 降序）；当日无上传时为 `nil`（`omitempty` 序列化省略） |

### `KeyCount`

某日某来源（密钥 / 后台直传）的新增图片计数，名称已解析，供仪表盘 tooltip 渲染。

| 字段 | 类型 | 说明 |
|------|------|------|
| `Name` | `string` | 密钥标签；后台 JWT 直传的图片无关联密钥，统一为 `"admin"` |
| `Count` | `int` | 该来源当日新增图片数 |

### `KeyGroupCount`

DAO 按密钥分组的新增图片计数，`KeyID` 未解析名称，由 service 据 `apiKeyDAO.List` 构建 id->name 映射后解析为 `KeyCount`。

| 字段 | 类型 | 说明 |
|------|------|------|
| `KeyID` | `*int` | 添加该图片的密钥 ID，`nil` 表示后台 JWT 直传 |
| `Count` | `int` | 该密钥当日新增图片数 |

## 设计要点

- 趋势单元用 `DashboardTrendDay`（在 [`DailyCount`](./log.md) 的 `{Date, Count}` 基础上追加可选 `Keys`），结构兼容日志直方图 buckets，前端可共用 `LogsHistogram` 组件；`Keys` 仅仪表盘携带，日志直方图不携带，tooltip 不渲染来源明细。
- `KeyGroupCount` 只在 DAO/service 内部流转（DAO 只感知 `KeyID` 外键，不感知密钥标签），不对外暴露；service 解析名称后转为 `KeyCount` 进入响应。
- 存储大小取 DB `SUM(size)` 而非文件系统遍历：`images.size` 字段已是单一事实来源，一条 SQL 完成；文件系统遍历慢且含孤儿文件，仅适合作可选的「磁盘实际占用」辅助指标。
- 近 N 天趋势以 `Image.CreatedAt` 为准，**不**沿用 `image.upload` 日志事件数：秒传（同 hash 已存在）也会记一次事件导致重复计数，且日志可被 ClearAll 清空不持久。

## 与其它文件的关系

- 组装方：[`internal/service/dashboard.go`](../service/dashboard.md)。
- 暴露方：[`internal/api/dashboard.go`](../api/dashboard.md)（`GET /admin/dashboard`）。
- 复用类型：[`model.DailyCount`](./log.md)（`KeyGroupCount`/`KeyCount`/`DashboardTrendDay` 定义在本文件）。
- 端到端说明见 [`DASHBOARD.md`](../../DASHBOARD.md)。
