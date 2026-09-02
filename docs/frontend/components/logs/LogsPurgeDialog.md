# `frontend/app/components/logs/LogsPurgeDialog.vue`

清理日志的危险操作弹窗，Nuxt 自动导入标签为 `<LogsPurgeDialog />`（文件名以目录名 `logs` 开头，Nuxt 去重前缀，注册名 `LogsPurgeDialog` 而非 `LogsLogsPurgeDialog`）。

## 职责

- 双模式（由 `scope` prop 切换）：`'all'`（默认）清空全部历史日志；`'get'` 仅删除 info 级 GET 请求日志。两种模式均不可撤销，清理后会保留一条审计记录。
- 顶部 rose 警告条按 `scope` 切换文案提示后果。
- 账号密码二次确认表单（`username` 预填当前登录用户，可改；`password`）。
- 提交调 [`useLogs`](../../composables/useLogs.md).`purge(creds)`（`scope` 随 body 透传），成功 emit `done`。

## Props / Emits

- props：`open: boolean`；`scope?: 'all' | 'get'`（默认 `'all'`）。
- emits：`close()`、`done()`。

## 实现要点

- `title` / `warning` 计算属性按 `scope` 切换：`'all'` -> 标题「清理全部日志」、警告「此操作将清空全部历史日志……」；`'get'` -> 标题「清理GET日志」、警告「此操作将删除全部 info 级别的 GET 请求日志……」。
- `canSubmit` 计算属性：用户名与密码均非空才允许提交。
- `watch(open)` 在每次打开时预填 `user.username`、清空密码与 `serverError`、复位 `loading`。
- 确认按钮用 `rose-600` 危险色，提交中显示 `animate-spin` 的 `rose` spinner 并禁用按钮。
- `handleSubmit` 失败时把 `err.message` 写入 `serverError` 就地展示；清理为敏感操作，后端密码校验失败返回 **403**（而非 401），从而不触发 [`useApi`](../../composables/useApi.md) 的全局登出。
- 二次确认交互（预填用户名 + 账号密码 + 就地错误）复用 [`apikeys/RevokeDeleteDialog`](../apikeys/RevokeDeleteDialog.md) 删除密钥的同款模式。

## 与其它文件的关系

- 父组件：[`pages/logs/index.vue`](../../pages/logs/index.md)（挂载两份实例：「清理全部日志」按钮触发 `purgeOpen`（默认 scope），「清理GET日志」按钮触发 `purgeGetOpen`（`scope="get"`）；`@done` 后各自刷新直方图与列表）。
- 外壳：[`ui/BaseDialog`](../ui/BaseDialog.md)。当前用户名取自 [`useAuth`](../../composables/useAuth.md)。
