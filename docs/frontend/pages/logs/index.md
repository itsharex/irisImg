# `frontend/app/pages/logs/index.vue`

日志中心主页，对应路由 `/logs`，后台「日志中心」导航目标。提供多维筛选、近 14 天趋势直方图、分页日志列表与日志清理入口。

## 职责

- 顶部标题 + 副标题（「查看访问与业务日志，支持多维筛选、按日趋势与清理」）。
- 筛选栏（`grid` 响应式 2/3/4 列）：级别 / 事件 / 方法 / 状态 / 关键字 / 起始日期 / 结束日期 + 「查询」「重置」按钮。
- 直方图卡片：标题「近 14 天日志量」+ 副文案「共 N 条」，右上角并排两个红色清理按钮——「清理全部日志」（触发 `purgeOpen`，清空全部）与「清理GET日志」（触发 `purgeGetOpen`，仅清 info 级 GET 请求日志），分别打开 [`LogsPurgeDialog`](../../components/logs/LogsPurgeDialog.md) 二次确认弹窗（同一组件，`scope` prop 区分模式）；卡片主体渲染 [`LogsHistogram`](../../components/logs/LogsHistogram.md)，透传 `:error="histError"` 与 `@retry="fetchHistogram"`，与表格一致支持加载 / 错误（重试）/ 空 / 图表四态。
- 分页日志栏：顶部「共 N 条」+ [`UiPagination`](../../components/ui/Pagination.md) 页码控件（上一页 / 下一页 + 页码按钮与首末页直达，当前页灰色不可按），主体为 [`LogsTable`](../../components/logs/LogsTable.md)（四态：加载 / 错误（重试）/ 空 / 表格）。
- 组件按目录前缀自动导入，但因文件名以目录名 `logs` 开头，Nuxt 会去重前缀：`components/logs/LogsHistogram.vue` -> `<LogsHistogram />`、`LogsTable.vue` -> `<LogsTable />`、`LogsPurgeDialog.vue` -> `<LogsPurgeDialog />`（注意不是 `LogsLogs*`）。

## 鉴权逻辑

- `definePageMeta({ middleware: 'auth' })`：未登录跳转 `/`。
- 列表 / 直方图 / 清理均走后台 JWT 通道（`GET /admin/logs`、`GET /admin/logs/histogram`、`DELETE /admin/logs`），由 [`useApi`](../../composables/useApi.md) 自动附带 `Authorization` 头。

## 状态管理

| 类别 | 变量 | 类型 | 说明 |
| --- | --- | --- | --- |
| 筛选 | `filters` | `reactive` | `level / event / method / statusClass / keyword / start / end`，日期为本地 `YYYY-MM-DD`，其余为字符串（空表示不限）。 |
| 列表 | `logs` | `ref<LogItem[]>` | 当前页日志条目。 |
| 列表 | `total` | `ref<number>` | 命中总条数，用于分页与「共 N 条」。 |
| 列表 | `page` | `ref<number>` | 当前页码，从 1 起。 |
| 列表 | `loading` | `ref<boolean>` | 列表加载态，控制按钮禁用。 |
| 列表 | `error` | `ref<string \| null>` | 列表错误文案，非空时透传给表格错误态。 |
| 直方图 | `buckets` | `ref<HistogramBucket[]>` | 近 14 天按日计数。 |
| 直方图 | `histTotal` | `ref<number>` | 直方图区间合计。 |
| 直方图 | `histLoading` | `ref<boolean>` | 直方图加载态。 |
| 直方图 | `histError` | `ref<string \| null>` | 直方图错误文案，非空时透传给直方图错误态。 |
| 弹窗 | `purgeOpen` | `ref<boolean>` | 「清理全部日志」弹窗显隐（默认 scope）。 |
| 弹窗 | `purgeGetOpen` | `ref<boolean>` | 「清理GET日志」弹窗显隐（`scope="get"`）。 |
| 派生 | `totalPages` | `computed<number>` | `Math.max(1, Math.ceil(total / PAGE_SIZE))`。 |
| 常量 | `PAGE_SIZE` | `number` | `50`，每页条数。 |

## 关键函数

| 函数 | 作用 |
| --- | --- |
| `buildQuery(p)` | 把 `filters` + 页码组装成 `ListLogsParams`（`page / pageSize` 必填，其余仅在前端非空时写入）；日期从本地 `YYYY-MM-DD` 转 RFC3339：`start` = 当日 `00:00:00`，`end` = 次日 `00:00:00`（左闭右开）。 |
| `fetchLogs()` | 调 [`useLogs`](../../composables/useLogs.md).`list(buildQuery(page))`，写入 `logs / total`；失败写 `error` 并清空列表。 |
| `fetchHistogram()` | 调 [`useLogs`](../../composables/useLogs.md).`histogram()`，起始清空 `histError`，成功写入 `buckets / histTotal`；失败写 `histError`（不再静默吞错）并清空 `buckets / histTotal`。 |
| `onSearch()` | `page` 重置为 1 后 `fetchLogs()`。 |
| `onReset()` | 清空 `filters` 全部字段、`page` 重置为 1 后 `fetchLogs()`。 |
| `goPage(p)` | 越界或与当前页相同时直接返回，否则 `page = p` 后 `fetchLogs()`。 |
| `onPurgedAll()` / `onPurgedGet()` | 各自关闭对应清理弹窗后调 `refreshAfterPurge()`。 |
| `refreshAfterPurge()` | 刷新直方图，`page` 重置为 1 后 `fetchLogs()`（清理后列表回到第 1 页，可见补记的 `log.clear` / `log.clear_get` 审计事件）。 |

## 数据流

- `onMounted` 并行触发 `fetchHistogram()` 与 `fetchLogs()`（首屏 `page=1`、无筛选）。
- 筛选栏「查询」走 `onSearch`（回到第 1 页），「重置」走 `onReset`（清空筛选并回到第 1 页）。
- 分页走 `UiPagination` 的 `@change` → `goPage(p)`：上一页 / 下一页、页码按钮与首末页直达共用同一入口；`loading` 时组件整体禁用，越界 / 重复页由 `goPage` 守卫拦截。
- 表格 `@retry` 直接复用 `fetchLogs()` 重拉当前页。
- 直方图 `@retry` 直接复用 `fetchHistogram()` 重拉趋势；`histError` 非空时直方图进入错误态并显示重试入口。
- 清理弹窗 `@done` 走 `onPurgedAll` / `onPurgedGet`：弹窗内部完成账号密码二次确认（`DELETE /admin/logs`，`scope` 区分全部 / 仅 info 级 GET）后通知父页刷新直方图与列表。

## 与其它文件的关系

```
pages/logs/index.vue
  ├── composables/useLogs.ts                    -> list / histogram / purge（JWT 通道）
  ├── components/logs/LogsHistogram.vue         （<LogsHistogram />，近 14 天直方图）
  ├── components/logs/LogsTable.vue             （<LogsTable />，四态列表，emit retry）
  └── components/logs/LogsPurgeDialog.vue       （<LogsPurgeDialog />，清理二次确认，双实例：默认 / scope="get"，emit done）
layouts/default.vue（承载侧边栏与用户区）
```

## 修改建议

- 新增筛选维度时同步扩展 `filters`、`buildQuery` 与对应下拉 `options`，并确认 [`useLogs`](../../composables/useLogs.md).`ListLogsParams` 与后端 `GET /admin/logs` query 字段一致。
- 调整每页条数改 `PAGE_SIZE` 即可，`totalPages` 与分页禁用条件会自动跟随。
- 清理为敏感操作，弹窗内已做账号密码二次确认；如需追加二次输入或审计展示，集中在 `LogsPurgeDialog` 内改动，父页仅消费 `@done`。
