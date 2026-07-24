# `frontend/app/pages/dashboard.vue`

仪表盘页面，对应路由 `/dashboard`，使用默认布局 `layouts/default.vue`，是后台默认着陆页（登录成功后跳转至此）。

## 职责

- 一次性拉取后端聚合统计 [`useDashboard().overview(30)`](../composables/useDashboard.md)，渲染三大区块：
  1. **统计卡片区**：4 张 [`DashboardStatCard`](../components/dashboard/StatCard.md) - 图片总量、存储占用（`formatBytes`）、APIkey 数（副标题「有效 X · 已吊销 Y」）、日志总量。`grid-cols-2 lg:grid-cols-4`。
  2. **近 N 天新增图片趋势**：白底圆角卡，header 展示标题与合计数，下方复用 [`LogsHistogram`](../components/logs/LogsHistogram.md)（传 `empty-text` / `title-text` / `unit="张"` 适配图片场景）；hover 浮动卡除日期与数量外，还按 `recent_upload_trend` 携带的 `keys` 渲染「当日每个 key 上传数」来源明细，后台直传显示 admin。
  3. **快捷入口**：4 张 `NuxtLink` 卡片链接内容中心 / 日志中心 / APIkey 管理 / 系统配置，图标复用 [`AppSidebar`](../components/layout/AppSidebar.md) 的 heroicons path。
- 三态：加载态用 `animate-pulse` 骨架卡（统计区）+ `LogsHistogram` 内置加载态；错误态 rose 虚框 + 重试按钮（统计区与趋势图各一，均调 `fetchOverview`）；空态由 `LogsHistogram` 处理。
- 顶部「刷新」按钮强制重新拉取。

## 鉴权逻辑

- `definePageMeta({ middleware: 'auth' })`：未登录跳转 `/`。
- 校验集中到 `middleware/auth.ts`，页面内不再重复校验。
- token 注入与 401 自动登出由 `useApi` 全局兜底。

## 数据流

```
onMounted -> fetchOverview()
  └─ useDashboard().overview(30)
       └─ useApi.get('/admin/dashboard?days=30')  (自动带 JWT、解包 data)
            -> stats: DashboardStats | null
                 ├─ 统计卡片：images_total / storage_bytes / apikeys_* / logs_total
                 └─ 趋势图：recent_upload_trend / recent_upload_total
```

## 与其它文件的关系

- composable：[`useDashboard.ts`](../composables/useDashboard.md)（`overview`）、[`useImages.ts`](../composables/useImages.md)（`formatBytes`）。
- 组件：[`DashboardStatCard`](../components/dashboard/StatCard.md)、[`LogsHistogram`](../components/logs/LogsHistogram.md)。
- 图标来源：[`AppSidebar.vue`](../components/layout/AppSidebar.md)（快捷入口 heroicons path 与之一致）。
- 后端接口：`GET /admin/dashboard`，端到端见 [`DASHBOARD.md`](../../../backend/DASHBOARD.md)。

## 修改建议

- 需要页面级数据预取时可改用 `useAsyncData`；当前 SPA 模式下 `onMounted` 拉取即可。
- 可选增强：环境信息只读卡（复用 `useSystemConfig`）、MIME 类型分布、APIkey 活跃度（见 [`DASHBOARD.md`](../../../backend/DASHBOARD.md) 扩展建议）。
