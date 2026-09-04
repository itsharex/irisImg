# `internal/service/image.go`

图片上传与批量删除的业务编排层。控制器把请求字节交过来，本层负责：**大小/MIME 校验 → 算 hash → 去重秒传 → 解析宽高 → 写盘 → 落库**；批量删除则编排 **查现存记录 → 逐张删物理文件 → 批量删记录**。

## 类型与变量

### `ImageService`

```go
type ImageService struct {
    dao         dao.ImageDAO
    saver       *storage.Saver
    cfg         config.StorageConfig
    maxBytes    int64               // cfg.MaxUploadSizeMB 推导
    allowedMime map[string]struct{} // cfg.AllowedMimeTypes 预编译
}
```

由 [`router`](../router/router.md) 注入 dao / saver / cfg；构造期就把白名单与大小上限预计算好，每次上传不再重新解析。

### sentinel 错误

供 api 层用 `errors.Is` 区分映射 HTTP 状态码：

| 错误 | 含义 | API 层映射 |
| --- | --- | --- |
| `ErrEmptyFile` | 上传内容为空 | 400 |
| `ErrFileTooLarge` | 字节数超过 `storage.max_upload_size_mb`（service 内部二次防御） | 413 |
| `ErrUnsupportedMime` | 嗅探出的真实 MIME 不在白名单 | 400 |

## 函数

### `NewImageService(d dao.ImageDAO, s *storage.Saver, cfg config.StorageConfig) *ImageService`

- `cfg.MaxUploadSizeMB <= 0` 时回退到 20，与配置默认值一致。
- 白名单字符串预压低、去空白后入查表 map。

### `(s *ImageService) MaxBytes() int64`

返回字节上限，供 [`api.ImageAPI.Create`](../api/image.md) 给 `http.MaxBytesReader` 设阈值（让超限请求在更早阶段被拦下）。

### `(s *ImageService) Upload(ctx, *model.UploadImageInput) (*model.Image, error)`

主流程：

1. **空文件 / 大小校验** → `ErrEmptyFile` / `ErrFileTooLarge`。
2. **MIME 嗅探**：`http.DetectContentType` 看头部 512 字节，剥掉 `;charset=...` 之类参数后比对白名单。客户端伪造 `Content-Type` 没用。
3. **算 SHA256**，作为文件名 + 去重键。
4. **秒传判定**：`dao.GetByHash(ctx, hash)`，命中直接返回已有记录（**不重复写盘、不重复落库**）；只放过 `dao.ErrNotFound`，其它错误透传。
5. **解析宽高**：`image.DecodeConfig`（标准库注册了 png/jpeg/gif；webp 等未注册的格式会失败但**不阻断**，宽高记 0）。
6. **MIME → 扩展名**：`extFromMime` 把白名单内的 MIME 映射成 `png/jpg/gif/webp` 等；不信任原文件名后缀。
7. **写盘**：调 [`saver.Save`](../pkg/storage.md) 得到相对路径，再调 `saver.PublicURL` 拼出对外 URL。
8. **落库**：组装 `model.Image{Filename, StoredPath, URL, Size, MimeType, Width, Height, Hash, KeyID}` 调 `dao.Create`。

### `(s *ImageService) List(ctx, model.ImageListQuery) (*model.ImageListResult, error)`

查询图片列表：对 `q.Limit<=0` 兜底为 24，调 [`dao.ImageDAO.List`](../dao/dao.md)，组装 `model.ImageListResult{Items, Total}` 返回。过滤（key_id）与排序方向由 dao 层落实，service 只做参数兜底与结果组装。

### `(s *ImageService) BatchDelete(ctx, ids []int) (deleted int, ids []int, err error)`

批量删除图片，供 [`api.ImageAPI.BatchDeleteAdmin`](../api/image.md) 调用。返回 `(实际删除条数, 实际删除的图片 ID 列表, error)`；**不存在的 ID 静默跳过（幂等语义）**，不构成错误。

主流程：

1. `len(ids) == 0` 直接返回 `(0, nil, nil)`（service 层兜底，不依赖 api 层 binding 校验）。
2. `dao.ListByIDs(ctx, ids)` 取**现存**记录（删物理文件必须先拿到 `StoredPath`），混入的不存在 ID 天然被过滤。
3. 逐张 best-effort 删物理文件：`_ = s.saver.Delete(img.StoredPath)`，失败不阻断（[`Saver.Delete`](../pkg/storage.md) 幂等，文件不存在返回 nil）。
4. `dao.DeleteByIDs(ctx, ids)` 一条 `IN` SQL 批量删记录，返回实际删除条数。

**非事务**（低频管理操作，与 `APIKeyService.Delete` 同款权衡）：若第 4 步失败，个别物理文件已删但记录仍在，该图缩略图 404，管理员重删时幂等兜底。结构对称于删除密钥时的级联清理（先 `ListByKeyID` 后 `DeleteByKeyID`）。

### `decodeImageSize` / `extFromMime`

未导出辅助函数：
- 解析失败一律返回 0,0，不让宽高问题阻断上传。
- 扩展名映射兜底取 MIME 子类型；未知格式则用 `bin`。

## 入参

`*model.UploadImageInput`（定义见 [`model/image.md`](../model/image.md)）字段：
- `Filename` — 原始文件名，仅作展示。
- `Content` — 已经过 `http.MaxBytesReader` 保护的完整字节。
- `KeyID` — 由 API Key 鉴权中间件写入；JWT 直传渠道（暂未实现）会传 nil。

## 与其它包的关系

```
api.ImageAPI ──► service.ImageService
                     ├─► dao.ImageDAO       (GetByHash / Create / List / ListByIDs / DeleteByIDs)
                     └─► pkg/storage.Saver  (Save / PublicURL / Delete)
```

## 修改建议

- 想支持流式上传（边读边写边算 hash）：把 `Upload` 的 `in.Content` 改成 `io.Reader`，并在 saver 里同步换成流式接口。当前小于 20 MiB 的小文件全量读入是最简实现。
- 想支持 webp/avif 宽高解析：引入 `golang.org/x/image/webp` 等并 `_` import 注册解码器，无需改业务代码。
- 想去重时复用 `KeyID`：当前秒传直接返回首次创建的记录（包括首次的 `key_id`），不覆盖、不创建新行；如需「记录第二次是谁上传的」，可在这里追加一张 `image_uploads` 关联表。
