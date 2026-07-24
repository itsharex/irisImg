# `frontend/app/composables/useDashboard.ts`

仪表盘统计接口封装 + 类型定义，供仪表盘页面拉取首页聚合数据。

## 导出类型

- `DashboardStats`：`GET /admin/dashboard` 的响应 `data`，对应后端 `model.DashboardOverview`：
  - `images_total: number`、`storage_bytes: number`
  - `apikeys_total / apikeys_active / apikeys_revoked: number`
  - `logs_total: number`
  - `recent_upload_trend: HistogramBucket[]`（复用 [`useLogs`](./useLogs.md) 的 `HistogramBucket`，结构 `{date, count, keys?}`；`keys` 为按来源拆分，供仪表盘 tooltip 展示「当日每个 key 上传数」，后台直传显示 admin）
  - `recent_upload_total: number`、`days: number`

## 导出函数

### `useDashboard()`

返回 `{ overview }`，走后台 JWT 通道（由 [`useApi`](./useApi.md) 自动附带 `Authorization` 头、自动解包 `data`）。

- `overview(days = 30)`：调 [`useApi`](./useApi.md) 的 `get('/admin/dashboard', { query: { days } })`，一次性返回聚合统计。`days` 默认 30（后端上限 90，非法值由后端回退到 30）。

## 与其它文件的关系

- 底层依赖 [`useApi.ts`](./useApi.md)（`get` 自动附带 JWT 并解包 `data`）。
- 复用类型 [`useLogs.ts`](./useLogs.md)（`HistogramBucket`）。
- 消费方：[`pages/dashboard.vue`](../pages/dashboard.md)。
- 后端接口见 [`DASHBOARD.md`](../../../backend/DASHBOARD.md)。
