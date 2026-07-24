# `frontend/app/composables/useLogs.ts`

日志中心接口封装 + 类型定义，供日志列表、趋势直方图与日志清理使用。

## 导出类型

- `LogLevel`：日志级别字面量联合 `'debug' | 'info' | 'warn' | 'error'`，对应后端 `model.Log.Level`。
- `LogItem`：单条日志，对应后端 `model.Log`（`id / timestamp / level / event / method / path / status / duration_ms / client_ip / request_id / api_key_id / username / message / created_at`）。
- `LogListResponse`：`GET /admin/logs` 的响应 `data`，含 `items / total / page / page_size`。
- `HistogramBucket`：直方图单日计数（`date` 形如 `YYYY-MM-DD` / `count`，可选 `keys` 仅仪表盘图片趋势携带）。
- `KeyCount`：某日某来源的新增图片计数（`name` 密钥标签，后台直传为 `"admin"` / `count`），供仪表盘 tooltip 按来源拆分。
- `HistogramResponse`：`GET /admin/logs/histogram` 的响应 `data`，含 `buckets / total`。
- `PurgeRequest`：清理日志请求体（账号密码二次确认），含 `username / password`。
- `ListLogsParams`：`list()` 的入参（`level / event / method / statusClass / keyword / start / end / page / pageSize`）。

## 导出函数

### `useLogs()`

返回 `{ list, histogram, purge }`，所有接口走后台 JWT 通道（由 [`useApi`](./useApi.md) 自动附带 `Authorization` 头、自动解包 `data`）。

- `list(params)`：调 [`useApi`](./useApi.md) 的 `get('/admin/logs', { query })`，自动附带 JWT。`page` / `pageSize` 默认 `1` / `50`，并映射为后端的 `page` / `page_size`；`statusClass` 映射为下划线 `status_class`；`level` / `event` / `method` / `keyword` / `start` / `end`（RFC3339 时间字符串）原样透传，未传的字段不进入 query。
- `histogram()`：调 [`useApi`](./useApi.md) 的 `get('/admin/logs/histogram')`，返回按日聚合的请求计数，无入参。
- `purge(creds)`：调 [`useApi`](./useApi.md) 的 `api('/admin/logs', { method: 'DELETE', body: creds })`，**走 DELETE 方法**，请求体携带 `username` / `password` 做账号密码二次确认，返回 `{ deleted }`（被删除的条数）。清理为敏感操作，后端密码校验失败返回 **403**（而非 401），从而不触发 `useApi` 的全局登出逻辑。

## 与其它文件的关系

- 底层依赖 [`useApi.ts`](./useApi.md)（`get` 用于查询、`api` 用于 DELETE，二者都自动附带 JWT 并解包 `data`）。
- 日志中心页面尚未落地，落地后将在本段补充对应的 `pages/logs/*` 与 `components/logs/*` 引用。
