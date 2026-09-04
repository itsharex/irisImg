# `internal/api/image.go`

图片相关接口的 Gin 控制器。

- **`POST /api/v1/images`** -- 对外添加图片，**已实现**，由 [API 密钥鉴权中间件](../middleware/apikey.md) 保护，需 `readwrite` 密钥。
- **`POST /api/v1/admin/images`** -- 后台直传图片，**已实现**，由 [JWT 鉴权中间件](../middleware/auth.md) 保护，供内容中心上传，`key_id` 留空（admin 直传）。
- **`GET  /api/v1/admin/images`** -- 后台图片列表，**已实现**，由 [JWT 鉴权中间件](../middleware/auth.md) 保护，供内容中心拉取图片（支持按 `key_id` 过滤、时间升序、分页）。
- **`DELETE /api/v1/admin/images`** -- 后台批量删除图片，**已实现**，由 [JWT 鉴权中间件](../middleware/auth.md) + [HTTPSOnly](../middleware/https.md) 保护，供内容中心多选删除；请求体需携带账号密码做二次确认。
- **`GET  /api/v1/images`** -- 对外占位，任意有效密钥可访问，**目前返回 501**，待语义明确后再实现。

## 类型

### `ImageAPI`

```go
type ImageAPI struct {
    svc     *service.ImageService
    authSvc *service.AuthService
    rec     service.LogRecorder
}
```

由 [`router`](../router/router.md) 通过 `NewImageAPI(svc, authSvc, rec)` 注入；`authSvc` 用于批量删除的账号密码二次确认；`rec` 为 `service.LogRecorder`，用于把图片上传 / 删除业务事件记录到日志中心（`rec` 为 `nil` 时静默跳过，便于测试）。控制器不再直接持有 DAO，便于在 service 层统一编排上传链路。

## 处理函数

### `Create(c *gin.Context)` -- `POST /api/v1/images`

**请求**：`multipart/form-data`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `file` | file | 图片文件，必填。MIME 白名单见 `storage.allowed_mime_types` |

**Header**：`X-API-Key: <readwrite 密钥>`（由 [`middleware.APIKeyAuth`](../middleware/apikey.md) 校验）。

**响应**：200 + 完整 `model.Image`（含 `id / url / stored_path / size / mime_type / width / height / hash / key_id / created_at` 等）。

**错误映射**：

| HTTP | 业务码 | 触发 |
| --- | --- | --- |
| 400 | `CodeBadRequest` | 缺少 `file` 字段、上传内容为空、MIME 不在白名单 |
| 401 / 403 / 429 | 见 [`APIKEY.md`](../../APIKEY.md) | 鉴权失败 / 权限不足 / 限流 |
| 413 | `CodePayloadTooLarge` | 文件超过 `storage.max_upload_size_mb` |
| 500 | `CodeServerError` | 内部错误（落盘 / 落库失败等） |

**关键实现**：

1. 文件读取与错误映射抽到共享 helper `readUploadFile` / `respondUploadError`（见下文），`Create` 与 `CreateAdmin` 复用，避免两处分支漂移。
2. `readUploadFile` 先用 `http.MaxBytesReader(...)` 把 `c.Request.Body` 包一层（上限取自 `svc.MaxBytes()`），超大请求体在更早阶段就被拦下；再 `c.FormFile("file")` 解析出 `*multipart.FileHeader`，若被 `MaxBytesReader` 触发，`errors.As(err, *http.MaxBytesError)` 命中 -> 413。
3. 读完文件全部字节后调 `svc.Upload`，`respondUploadError` 按 sentinel error 分支映射状态码。
4. `key_id` 取自 `c.GetInt(middleware.ContextKeyAPIKeyID)`（中间件保证一定 >0），透传给 service 落库，记录图片由哪把密钥添加。
5. 上传成功后调 `recordUpload(c, filename)`，经 `rec` 记录一条 `model.EventImageUpload`（`model.LevelInfo`）业务事件，消息体为 `"upload image: <filename>"`；`rec` 为 `nil` 时跳过。

### `CreateAdmin(c *gin.Context)` -- `POST /api/v1/admin/images`

后台直传图片，由 [JWT 鉴权中间件](../middleware/auth.md) 保护，供内容中心管理端上传，无需 `X-API-Key`。

**请求**：`multipart/form-data`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `file` | file | 图片文件，必填。MIME 白名单见 `storage.allowed_mime_types` |

**Header**：`Authorization: Bearer <JWT>`（由 [`middleware.JWTAuth`](../middleware/auth.md) 校验）。

**响应**：200 + 完整 `model.Image`，`key_id` 为 `null`（admin 直传，不关联任何密钥；因 `omitempty` 不出现在 JSON 中）。

**错误映射**：

| HTTP | 业务码 | 触发 |
| --- | --- | --- |
| 400 | `CodeBadRequest` | 缺少 `file` 字段、上传内容为空、MIME 不在白名单 |
| 401 | `CodeUnauthorized` | 未登录 / JWT 失效（由中间件返回） |
| 413 | `CodePayloadTooLarge` | 文件超过 `storage.max_upload_size_mb` |
| 500 | `CodeServerError` | 内部错误（落盘 / 落库失败等） |

**与 `Create` 的差别**：仅在不走 API Key 通道、`KeyID` 传 `nil`。业务流程（嗅探 -> 秒传 -> 落盘 -> 落库）完全一致，复用 `svc.Upload`；上传成功后同样调 `recordUpload(c, filename)` 记录 `model.EventImageUpload`（`LevelInfo`）业务事件。这类图片只会在内容中心「全部」里出现，详情里来源展示为 `admin`。

### `BatchDeleteAdmin(c *gin.Context)` -- `DELETE /api/v1/admin/images`

后台批量删除图片（物理文件 + 元信息记录同删，不可恢复），供内容中心多选删除使用。由 [JWT 鉴权中间件](../middleware/auth.md) + [`HTTPSOnly`](../middleware/https.md)（生产环境由 `apikey.https_only` 开启）保护。

**请求**：JSON body（`model.BatchDeleteImagesRequest`）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `username` | string | 账号（二次确认，必填） |
| `password` | string | 密码（二次确认，必填） |
| `ids` | []int | 待删除的图片 ID 列表：非空、每项 > 0、上限 100 |

**响应**：200 + `{ deleted: 实际删除条数, ids: 实际被删除的图片 ID 列表 }`。不存在的 ID 静默跳过（幂等语义），不构成错误。

**错误映射**：

| HTTP | 业务码 | 触发 |
| --- | --- | --- |
| 400 | `CodeBadRequest` | body 解析失败 / `ids` 为空 / 超过 100 / 含非正数 |
| 401 | `CodeUnauthorized` | 未登录 / JWT 失效（由中间件返回） |
| 403 | `CodeForbidden` | 账号密码二次确认失败（或生产环境 HTTPSOnly 拦截非 HTTPS 请求） |
| 500 | `CodeServerError` | 查询 / 删除失败等内部错误 |

**关键实现**：

1. 与吊销 / 删除密钥（[`api/apikey.md`](apikey.md)）同款二次确认：`authSvc.VerifyCredentials` 常量时间比对，失败返回 **403 而非 401**，避免触发前端 `useApi` 的全局登出。
2. 校验通过后调 `svc.BatchDelete(ctx, req.IDs)`（见 [`service/image.md`](../service/image.md)），删除成功后调 `recordDelete(c, deleted)` 记录 `model.EventImageDelete`（`model.LevelWarn`）业务事件。

### `recordUpload(c *gin.Context, filename string)` -- 辅助方法

上传成功业务事件的统一记录入口。`rec` 为 `nil` 时直接返回；否则调 `rec.Record(model.NewEventLog(model.EventImageUpload, model.LevelInfo, "upload image: "+filename, middleware.LogContextFromGin(c)))`，把请求上下文（操作者 / 来源 IP 等）一并带入日志。`Create` 与 `CreateAdmin` 在 `svc.Upload` 成功后调用。

### `recordDelete(c *gin.Context, count int)` -- 辅助方法

批量删除业务事件的统一记录入口。`rec` 为 `nil` 时直接返回；否则调 `rec.Record(model.NewEventLog(model.EventImageDelete, model.LevelWarn, fmt.Sprintf("batch delete images: %d", count), middleware.LogContextFromGin(c)))`。删除属破坏性操作，级别用 `warn`（对照 apikey 的 revoke/delete 事件）。`BatchDeleteAdmin` 在 `svc.BatchDelete` 成功后调用。

### `List(c *gin.Context)` -- `GET /api/v1/images`（占位）

显式返回 `501 Not Implemented`（`code = CodeServerError`，message「图片列表接口尚未实现（占位）」），便于前端 / 调用方与鉴权失败的 401/403/429 区分。

### `ListAdmin(c *gin.Context)` -- `GET /api/v1/admin/images`

后台图片列表，由 [JWT 鉴权中间件](../middleware/auth.md) 保护，供内容中心拉取图片。

**Query 参数**：

| 参数 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- |
| `key_id` | int | 缺省=全部 | >=1 时只返回该密钥添加的图片 |
| `order` | string | `asc` | `asc`/`desc`，时间升序契合内容中心 |
| `page` | int | 1 | 页码，<1 非法 |
| `page_size` | int | 24 | 每页条数，<1 非法 |

**响应**：200 + `{ items: [model.Image], total, page, page_size }`。

**错误映射**：

| HTTP | 业务码 | 触发 |
| --- | --- | --- |
| 400 | `CodeBadRequest` | page / page_size / key_id 非法 |
| 401 | `CodeUnauthorized` | 未登录 / JWT 失效（由中间件返回） |
| 500 | `CodeServerError` | 查询失败 |

**关键实现**：`page` / `page_size` -> `offset=(page-1)*page_size`、`limit=page_size`，组装 `model.ImageListQuery` 调 `svc.List`。

## 与其它包的关系

```
images 组 ── APIKeyAuth ──► ImageAPI.Create ──► ImageService.Upload ──► dao.ImageDAO + pkg/storage.Saver
                                              │
                                              ├─ c.GetInt(ContextKeyAPIKeyID) -> model.UploadImageInput.KeyID
                                              └─ recordUpload -> rec.Record(EventImageUpload, LevelInfo, filename)

admin/images 组 ── JWTAuth + HTTPSOnly ──► ImageAPI.CreateAdmin ──► ImageService.Upload ──► dao.ImageDAO + pkg/storage.Saver
                                                     │
                                                     ├─ KeyID = nil（admin 直传）
                                                     └─ recordUpload -> rec.Record(EventImageUpload, LevelInfo, filename)

admin/images 组 ── JWTAuth + HTTPSOnly ──► ImageAPI.BatchDeleteAdmin
                                                     │
                                                     ├─ authSvc.VerifyCredentials（403 on fail，不触发前端登出）
                                                     ├─ ImageService.BatchDelete ──► dao.ListByIDs + saver.Delete + dao.DeleteByIDs
                                                     └─ recordDelete -> rec.Record(EventImageDelete, LevelWarn, "batch delete images: N")
```

## 注意

- 控制器**不再做权限/限流判定**，这些全部在 [`middleware.APIKeyAuth`](../middleware/apikey.md) 内完成。
- `MaxBytesReader` 的兜底校验在 service 层用 `len(content) > MaxBytes` 又做一次，避免接入新调用路径时遗漏。
- `recordUpload` 仅在上传**成功**路径上触发，失败分支由 `respondUploadError` 直接写响应返回，不记录业务事件。
- 落盘 / URL 生成的细节见 [`service/image.md`](../service/image.md) 与 [`pkg/storage.md`](../pkg/storage.md)；运维侧的 Nginx 静态反代约定见 [`IMAGE.md`](../../IMAGE.md)。
