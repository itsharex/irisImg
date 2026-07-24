# internal/api/dashboard.go

仪表盘统计接口的 Gin 控制器，挂在 JWT 受保护组下，只读聚合。无敏感操作，无需 HTTPSOnly 与密码二次确认。

## 类型

- `DashboardAPI`：持有 `*service.DashboardService`。
- `NewDashboardAPI(svc) *DashboardAPI`：构造函数。

## 接口

### `Overview(c *gin.Context)` -- `GET /api/v1/admin/dashboard`

- Query 参数 `days`：趋势窗口天数，默认 30，上限 90（防滥用）；非数字 / `<=0` / `>90` 一律回退到 30（不返回 400，因 `days` 为带默认值的可选参数）。
- 调 `svc.Overview(ctx, days)`，成功 `response.Success(c, overview)`；DB 错误 `response.ServerError(c, "查询仪表盘数据失败："+err)`。

响应 `data` 即 [`model.DashboardOverview`](../model/dashboard.md)，其中 `recent_upload_trend` 每个单元携带按来源拆分的 `keys`（`{name, count}`，后台直传为 `admin`），供仪表盘 tooltip 展示。统一响应体见 [`pkg/response`](../pkg/response.md)。

## 与其它文件的关系

- 依赖：[`service.DashboardService`](../service/dashboard.md)。
- 路由注册：[`router.New`](../router/router.md)（`protected.GET("/admin/dashboard", dashboardAPI.Overview)`）。
- 端到端说明见 [`DASHBOARD.md`](../../DASHBOARD.md)。
