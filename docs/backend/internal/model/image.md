# internal/model/image.go

图片元信息的**跨层数据载体**（实体 DTO）。

## 类型：Image

独立于 Ent 生成的 `ent.Image`：DAO 层负责在二者之间转换（见 [`entdao/image.go`](../dao/entdao/image.md) 的 `toModel`），使 service / api 层不直接依赖 Ent，便于替换存储实现。

字段：`ID`、`Filename`、`StoredPath`、`URL`、`Size`、`MimeType`、`Width`、`Height`、`Hash`、`CreatedAt`，均带 `json` 标签，可直接作为 API 响应体。字段语义与 [`ent/schema/image.go`](../../ent/schema/image.md) 一一对应。

此外含一个可空字段：

- `KeyID *int`（`json:"key_id,omitempty"`）：添加该图片的 API 密钥 ID。通过后台 JWT 上传的图片没有关联密钥，此处为 `nil`（序列化时省略）；通过密钥 POST 添加的图片回填中间件注入的 `api_key_id`。对应 schema 的 `key` edge / `key_id` 字段，详见 [`APIKEY.md`](../../APIKEY.md)。

## 类型：UploadImageInput

「上传一张图片」的入参，由 api 层从 HTTP 请求装配后传给 [`service.ImageService.Upload`](../service/image.md)。设计成结构体（而非多参数）便于后续追加字段（如标签、相册 ID）不破坏签名。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `Filename` | string | 客户端给出的原始文件名，仅作展示。真实落盘文件名由 hash + 嗅探的扩展名决定 |
| `Content` | []byte | 完整字节，由 api 层在 `http.MaxBytesReader` 保护下读出 |
| `KeyID` | *int | 添加该图片的 API 密钥 ID。API Key 渠道由中间件保证非空；JWT 直传渠道（暂未实现）传 nil |

## 类型：ImageListQuery / ImageListResult

「查询图片列表」的入参与返回结构，供后台内容中心 `GET /api/v1/admin/images` 使用（见 [`api/image.md`](../api/image.md)）。设计成结构体便于后续追加过滤维度（如 MIME、时间区间）不破坏签名。

`ImageListQuery`：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `KeyID` | *int | 非 nil 时只返回该密钥添加的图片；nil 表示不按密钥过滤（全部） |
| `Order` | string | 排序方向：非 `"desc"` 一律视为升序。空字符串按升序处理，契合内容中心「时间升序」需求 |
| `Offset` | int | 分页偏移 |
| `Limit` | int | 每页条数；<=0 时由 service 兜底为 24 |

`ImageListResult`：`Items []*Image` + `Total int`（符合过滤条件的总数，用于前端计算总页数）。

## 类型：BatchDeleteImagesRequest / BatchDeleteImagesResponse

「批量删除图片」的请求 / 响应 DTO，供后台内容中心 `DELETE /api/v1/admin/images` 使用（见 [`api/image.md`](../api/image.md)）。

`BatchDeleteImagesRequest`（复用吊销 / 删除密钥同款账号密码二次确认机制）：

| 字段 | 类型 | binding | 说明 |
| --- | --- | --- | --- |
| `Username` | string | `required` | 账号，api 层经 `AuthService.VerifyCredentials` 常量时间比对校验，失败返回 403（而非 401，避免触发前端全局登出） |
| `Password` | string | `required` | 密码 |
| `IDs` | []int | `required,min=1,max=100,dive,gt=0` | 待删除的图片主键列表：非空、每项 > 0、上限 100（防误操作巨量删除） |

`BatchDeleteImagesResponse`：`Deleted int`（实际删除条数）+ `IDs []int`（实际被删除的图片 ID 列表）。**不存在的 ID 静默跳过（幂等语义）**，两个口径均按「实际删除」返回，供前端同步本地列表与分页计算。
