# `frontend/app/components/content/BatchDeleteDialog.vue`

内容中心「批量删除图片」的确认弹窗。Nuxt 自动导入标签为 `<ContentBatchDeleteDialog />`。结构参照 [`components/logs/LogsPurgeDialog.vue`](../logs/logs-purge-dialog.md)（同为账号密码二次确认的敏感操作弹窗）。

## 职责

- 弹出 [`UiBaseDialog`](../ui/BaseDialog.md)（标题「批量删除图片」），展示 rose 警告（永久删除选中的 N 张图片，含物理文件与记录，不可撤销；N 取 `ids.length`）。
- 账号密码二次确认：预填当前登录用户名（仍可修改），密码留空。提交调 [`useImages`](../../composables/useImages.md) 的 `batchRemove({ username, password, ids })`，即后端 `DELETE /admin/images`。
- 成功后 emit `done(result)`（`result` 为 `BatchDeleteImagesResult`，含实际删除条数与 ID 列表），由父页面刷新列表。
- 失败时弹窗**保持打开**，内联展示错误：二次确认失败后端返回 403（而非 401），不触发 `useApi` 的全局登出，用户修正密码后可直接重试。

## Props / Emits

- props：`open: boolean`（弹窗显隐）、`ids: number[]`（待删除的图片 ID 列表，即父页面多选的 `selectedIds`）。
- emits：
  - `close()` — 取消 / ESC / 遮罩关闭（`BaseDialog` 已处理）；
  - `done(result: BatchDeleteImagesResult)` — 删除成功。

## 交互细节

- `watch(open)`：每次打开重置表单、预填用户名、清空错误与 loading。
- 提交按钮在用户名 / 密码为空或 `ids` 为空时禁用；loading 时展示 spinner。
- `ids` 为空时父页面的删除按钮本身已禁用，此处的空 `ids` 校验是第二道防线。

## 与其它文件的关系

- 父组件：[`pages/content/index.vue`](../../pages/content/index.md)（多选模式的 `selectedIds` 与删除回调 `onDeleted`）。
- 依赖 [`useImages`](../../composables/useImages.md) 的 `batchRemove`、[`useAuth`](../../composables/useAuth.md) 的 `user`（预填用户名）、[`UiBaseDialog`](../ui/BaseDialog.md)。
