# `frontend/app/composables/useImages.ts`

图片资源请求封装（列表 / 上传 / 批量删除）+ 类型定义 + 图片 URL / 体积 / 时间格式化工具，供内容中心使用。

## 导出类型

- `ImageItem`：单张图片元信息，对应后端 `model.Image`（`id / filename / stored_path / url / size / mime_type / width / height / hash / created_at / key_id`）。
- `ImageListResponse`：`GET /admin/images` 的响应 `data`，含 `items / total / page / page_size`。
- `ListImagesParams`：`list()` 的入参（`keyId / order / page / pageSize`）。
- `BatchDeleteImagesRequest`：`batchRemove()` 的入参（`username / password / ids`），账号密码为后端二次确认字段，`ids` 非空、每项 > 0、上限 100。
- `BatchDeleteImagesResult`：批量删除响应，`deleted` 为实际删除条数、`ids` 为实际被删除的图片 ID（不存在的 ID 静默跳过）。

## 导出函数

### `useImages()`

返回：

- `list(params)`：调 [`useApi`](./useApi.md) 的 `get('/admin/images', { query })`，自动附带 JWT。`keyId` 不传则不按密钥过滤；`order` 默认 `asc`（时间升序）。
- `upload(file)`：调 [`useApi`](./useApi.md) 的 `post('/admin/images', FormData)`，走后台 JWT 直传通道（`POST /admin/images`），自动附带 JWT。`FormData` 字段名固定 `file`；ofetch 自动设置 `multipart/form-data` 边界，**不要手动设 Content-Type**。返回新增的 `ImageItem`（`key_id` 为 `null`，即 admin 直传，不关联任何密钥）。与对外 `POST /images`（API Key 鉴权）解耦。
- `batchRemove(params)`：调 [`useApi`](./useApi.md) 的 `api('/admin/images', { method: 'DELETE', body })`，走后台 JWT 通道批量删除（`DELETE /admin/images`，物理文件与记录同删，不可恢复）。**二次确认失败后端返回 403（而非 401），不会触发 `useApi` 的全局登出**，调用方（[`BatchDeleteDialog`](../components/content/batch-delete-dialog.md)）可在弹窗内联展示错误让用户重试。

### `resolveImageUrl(url)`

把后端返回的图片 URL 解析成浏览器可加载的完整地址：

- 后端 `storage.public_base_url` 为空时，`url` 形如 `/imgs/2026/07/<hash>.png`（相对路径）。前端 SPA 跑在另一端口（如 :3000），需拼上后端 origin（从 `runtimeConfig.public.apiBase` 去掉 `/api/v1` 推导）。
- `url` 已是 `http(s)://` 绝对地址时原样返回。后端 `NewSaver` 会对裸域名 `public_base_url` 自动补 `https://`，故非空配置产出的 `url` 总是带协议的绝对地址，能命中本分支原样返回；若后端意外返回无协议的裸域名，本函数会把它误当相对路径拼成 `/img.example.com/...`，故配置务必带协议或留空。

### `formatBytes(n)` / `formatDate(s)`

体积（B / KB / MB / GB）与时间（`YYYY-MM-DD HH:mm` 本地时间）格式化，供卡片与详情弹窗复用。

## 与其它文件的关系

- 被 [`pages/content/index.vue`](../pages/content/index.md) 与 [`components/content/ImageCard.vue`](../components/content/image-card.md)、[`ImageDetailDialog.vue`](../components/content/image-detail-dialog.md)、[`UploadPanel.vue`](../components/content/upload-panel.md)、[`BatchDeleteDialog.vue`](../components/content/batch-delete-dialog.md) 使用。
- 底层依赖 [`useApi.ts`](./useApi.md)。
