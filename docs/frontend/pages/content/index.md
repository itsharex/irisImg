# `frontend/app/pages/content/index.vue`

内容中心主页，对应路由 `/content`，展示已上传的图片资源，支持按 API Key 筛选、时间升序分页浏览，点击图片查看详情，多选模式批量删除。

## 职责

- 顶部展示「内容中心」标题，标题右侧为按钮组：
  - **默认态**：「多选」+「上传图片」。点击「多选」进入多选模式；「上传图片」切换 [`ContentUploadPanel`](../../components/content/upload-panel.md) 上传栏的显隐。
  - **多选态**：「删除（N）」（rose 危险色，空选中时禁用）+「取消」退出多选。
- 上传栏：拖拽 / 点击选择图片，走 JWT 后台直传（`POST /admin/images`），**不关联密钥**（`key_id` 为空，即 admin 直传）；上传的图只出现在「全部」里。
- 左侧按钮栏：顶部「全部」+ 各 API Key 的 `name` 按钮，点击切换筛选；已吊销密钥标灰提示。切换时重置到第 1 页。
- 右侧图片网格：按所选 Key 过滤、时间升序、每页 24 张，支持上一页 / 下一页。提供加载 / 空 / 错误态与重试。多选模式下卡片展示右下角勾选徽标与选中态边框（见 [`ImageCard`](../../components/content/image-card.md)）。
- 点击图片卡片：默认模式弹出 [`ContentImageDetailDialog`](../../components/content/image-detail-dialog.md) 查看大图与全元数据；多选模式切换选中 / 取消选中，不打开详情。
- 批量删除：多选态点「删除」弹出 [`ContentBatchDeleteDialog`](../../components/content/batch-delete-dialog.md)（账号密码二次确认），成功后刷新列表。

## 鉴权逻辑

- `definePageMeta({ middleware: 'auth' })`：未登录跳转 `/`。
- 列表、上传与批量删除均走 JWT 通道（`GET /admin/images`、`POST /admin/images`、`DELETE /admin/images`），由 [`useApi`](../../composables/useApi.md) 自动附带 `Authorization`，与对外 API Key 通道解耦。

## 数据流

- `onMounted` 并行拉取：
  - `GET /apikeys`（经 [`useApi`](../../composables/useApi.md)）→ 取 `items`，建立 `id → name` 映射，供左侧栏与详情弹窗解析来源 Key 名称。
  - `GET /admin/images`（经 [`useImages`](../../composables/useImages.md)）→ 默认全部、`order=asc`、`page=1`、`page_size=24`。
- `selectedKeyId` 或 `page` 变化时重新拉取图片列表；切换 Key 时 `page` 重置为 1。
- 上传栏 `@uploaded` 回调：admin 直传图 `key_id` 为空、只在不按密钥过滤时可见，故若当前在按密钥筛选则先 `selectKey(null)`（重置第 1 页并拉取），否则直接 `fetchImages()` 刷新当前页。
- 详情弹窗的来源 Key 名称由前端 `keyNameById` 映射解析，无需后端联表；`key_id` 为空时来源显示 `admin`。

## 多选模式状态机

- 状态：`multiSelect`（是否处于多选模式）、`selectedIds`（选中的图片 ID 数组，**跨页累计**）、`showDeleteConfirm`（删除确认弹窗显隐）。
- `enterMultiSelect()`：进入多选模式、清空选中，并收起上传面板（两种模式互斥不叠屏）。
- `exitMultiSelect()`：退出并清空全部多选状态（含确认弹窗）。
- `onCardClick(img)` 点击分叉：多选时 `toggleSelect(img)`（再点一次取消选中），否则 `openDetail(img)`。
- 翻页（`goPage`）**保留**选中（跨页累计多选，操作栏始终显示累计数）；切换 Key 筛选（`selectKey`）时**自动退出多选**（跨筛选维度保留选中违背直觉、误删风险高）。
- 删除完成 `onDeleted(result)`：关闭弹窗、清空选中、**保留多选模式**（便于连续分批删除，点「取消」才退出）；按删除后的总数本地计算 `maxPage = ceil((total - deleted) / PAGE_SIZE)` 收敛页码（当前页被删空时回退），再 `fetchImages()` 刷新。
- 状态行在多选且有选中时显示「· 已选 N 张」。

## 与其它文件的关系

```
content/index.vue
  ├── useImages.ts                              → GET /admin/images、POST /admin/images（upload）、DELETE /admin/images（batchRemove）
  ├── useApi.ts                                 → GET /apikeys
  ├── components/content/UploadPanel.vue        （上传栏，emit uploaded）
  ├── components/content/ImageCard.vue          （网格单元，传入 selectable / selected）
  ├── components/content/ImageDetailDialog.vue  （详情弹窗）
  └── components/content/BatchDeleteDialog.vue  （批量删除确认弹窗，emit done）
layouts/default.vue（承载侧边栏与用户区）
```
