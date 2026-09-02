# `internal/api/log.go`

日志中心接口的 Gin 控制器。控制器层只做参数解析、调用 service、组装响应。这些接口均挂在需 **JWT 登录**的受保护组下，并要求 **HTTPS**（由 [`HTTPSOnly`](../middleware/https.md) 中间件保证）。清理日志为敏感操作，handler 内用账号密码做二次确认，复用 [apikey 吊销 / 删除](apikey.md) 的同款机制。

## 类型

### `LogAPI`

- 字段：`svc *service.LogService`、`authSvc *service.AuthService`（用于清空前的密码二次确认）。
- 由 [`router`](../router/router.md) 通过 `NewLogAPI(svc, authSvc)` 注入。

## 处理函数

### `List(c *gin.Context)` -- `GET /admin/logs`

分页 + 多维过滤查询日志：

1. 解析分页 query：`page` 默认 `1`、`page_size` 默认 `50`（上限 `500`）；非法 -> `response.BadRequest`。
2. `status_class` 白名单校验：仅允许 `""` / `"2xx"` / `"4xx"` / `"5xx"`，非法值直接 `response.BadRequest("无效的 status_class")` 返回 **400**。该字段不再随 query 任意透传，避免空字符串外的非法取值被静默放宽为"不过滤全量"。
3. 组装 [`model.LogQuery`](../model/log.md)：`level` / `event` / `method` / `keyword` / `request_id` 直接透传，`status_class` 透传上一步已校验的值；`api_key_id` 非空时解析为 `*int`（非法 -> 400）；`start` / `end` 用 RFC3339 解析（左闭右开，失败 -> 400）；`Offset = (page-1) * pageSize`。
4. 调 `svc.List`：err -> `response.ServerError`。
5. 成功 `response.Success(c, gin.H{"items", "total", "page", "page_size"})`。

### `Histogram(c *gin.Context)` -- `GET /admin/logs/histogram`

返回最近 **14 天**的每日日志量（固定天数，供直方图 + 趋势线）。调 `svc.Histogram(ctx, 14)`：err -> 500；成功返回 `gin.H{"buckets": [{date, count}], "total": total}`。

### `Clear(c *gin.Context)` -- `DELETE /admin/logs`

按 `scope` 清理日志。需在请求体携带 [`model.DestructiveRequest`](../model/log.md) 做密码二次确认：

1. `c.ShouldBindJSON(&req)` 解析 `DestructiveRequest`（含可选 `scope`）；失败 -> `response.BadRequest`。
2. `authSvc.VerifyCredentials(req.Username, req.Password)` 失败 -> `response.Forbidden("用户名或密码错误")`（**403 `CodeForbidden` 而非 401**，避免触发前端 `useApi` 的全局登出）。
3. 按 `req.Scope` 白名单分发（非法值 -> `response.BadRequest("无效的 scope")` 返回 **400**，风格对齐 `status_class`）：
   - `""` / `"all"`：调 `svc.ClearAll(ctx, lc)` 清空全部，补记一条 `log.clear` 审计事件；
   - `"get"`：调 `svc.ClearInfoGet(ctx, lc)` 仅删 info 级 GET 请求日志，补记一条 `log.clear_get` 审计事件。
   其中 `lc` 为 [`middleware.LogContextFromGin`](../middleware/requestid.md)`(c)`；err -> 500。
4. 成功返回 `gin.H{"deleted": n}`。

## 错误码约定

| 场景 | HTTP | code |
|------|------|------|
| 入参非法（`page` / `page_size` / `status_class` / `start` / `end` / `api_key_id` / `scope`） | 400 | 40000 |
| 密码二次确认失败（Clear） | 403 | 40300 |
| 内部错误（查询 / 直方图 / 清理失败） | 500 | 50000 |

## 修改建议

- 不要在控制器里直接查库或写审计事件——那是 [`service.LogService`](../service/log.md) 的职责。
- 新增管理接口挂到 router 的日志分组下即可，自动继承 JWT + HTTPS 保护。
- 清空操作的密码校验放在控制器层（边界关注点），与 apikey 吊销 / 删除保持一致；service 保持纯粹。
- 直方图天数目前硬编码 `14`，如需可配置应作为 service 参数而非控制器常量。
