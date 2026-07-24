# `frontend/app/components/logs/LogsHistogram.vue`

按日计数直方图（纯 SVG、零第三方绘图依赖；默认渲染近 14 天日志量，亦可复用于仪表盘近 N 天新增图片趋势），Nuxt 自动导入标签为 `<LogsHistogram />`（`components/logs/LogsHistogram.vue` 文件名以目录名 `logs` 开头，Nuxt 去重前缀，注册名 `LogsHistogram` 而非 `LogsLogsHistogram`）。

## 职责

- 四态：加载中 / 错误态（`error` 非空，rose 虚框 + 错误文案 + 重试按钮 emit `retry`）/ 空态（`total === 0`，显示 `emptyText`，默认「近 14 天暂无日志」，仪表盘图片趋势传「近 30 天暂无新增图片」）/ 直方图 + 趋势线。渲染优先级：`loading` > `error` > `total === 0` > 直方图。
- SVG 画布 `viewBox="0 0 700 240"`，内边距常量 `PAD_L=40 / PAD_R=16 / PAD_T=16 / PAD_B=36`。
- 柱（由 `buckets.length` 决定）：恒为 `iris-violet`（`#A48CE6`），**hover 不变色**；hover 由覆盖整个绘图区的透明热区 `<rect>` 接管，按鼠标 x 反算落在哪个等分日期槽（整个槽高均视为该日区域，而非仅柱身），进入某区域时在该槽中线（`cx`）绘制 `iris-sky`（`#86C9EC`）竖直虚线（**跨整张直方图高度**，绘图区顶 `PAD_T` 到底 `PAD_T+plotH`，sky 在白底上可见且与 violet 柱身 / gold 趋势线区分），并弹出浮动信息卡：表头「日期 | 数量（`unit`）」；当 bucket 携带 `keys`（仅仪表盘图片趋势）时，表体额外渲染可滚动来源明细（每个 key 当日上传数，后台直传显示 admin，圆点 key=`iris-violet` / admin=`iris-gold` 区分，按 count 降序）。区域切换时虚线与浮窗均做过渡动画平滑横移。
- 7 日移动平均趋势线：`iris-gold`（`#F4C430`）`polyline` + 圆点；窗口不足 7 时取已有天均值，使趋势线从首日即横跨全宽。
- 4 等分网格线 + Y 轴刻度（0 / 1/4·max / 1/2·max / 3/4·max / max）。
- X 轴日期标签按自适应步长显示（`labelStep = ceil(n/8)`：14 天 step=2 与历史一致、30 天 step=4，末根恒显示），格式 `MM-DD`（`date.slice(5)`）。
- 底部图例：紫色方块 `legendText`（默认「日志量」）+ 金色短线「7 日均值」。

## Props / Emits

- props：`buckets: HistogramBucket[]`、`total: number`、`loading: boolean`、`error: string | null`、`emptyText?: string`（空态文案，默认「近 14 天暂无日志」）、`titleText?: string`（`aria-label` 文案，默认「近 14 天日志量直方图」）、`legendText?: string`（柱状图图例文案，默认「日志量」）、`unit?: string`（浮动卡表头数量单位，默认「条」；仪表盘图片趋势传「张」）。`emptyText` / `titleText` / `legendText` / `unit` 使组件可复用于图片趋势等非日志场景，默认值保持日志中心原行为不变。
- emits：`retry: []`（仅在错误态下点击「重试」按钮时触发，父组件据此重新拉取直方图数据）。

## 实现要点

- 零依赖：仅用原生 SVG 元素（`rect` / `polyline` / `circle` / `line` / `text`）与 Vue 模板渲染，未引入 ECharts/D3 等。
- `max` 取 `Math.max(1, ...counts)`，至少为 1 以避免除零；无数据时走空态分支不渲染柱。
- 柱布局按等分 `slot = plotW / n`，柱宽 `slot * 0.6`，圆角 `rx="3"`。
- 趋势点复用 `bars[i].cx` 作为横坐标，纵坐标按 `v / max * plotH` 反向映射到画布。
- 热区与虚线：透明热区 `<rect>`（`fill="transparent"` + `pointer-events="all"`）盖在绘图区最顶层，`mousemove` 时把鼠标 x 换算到 viewBox 单位反算槽号 `i`（整个槽高均视为该日区域），`mouseleave` 清空悬停；柱身不再直接响应鼠标。虚线 `v-if` 随悬停显隐（与浮窗一致），横坐标 `cursorX`（viewBox 单位）由 `hovered` 派生（`bars[i].cx`），通过 `transform: translateX()` + `transition: transform 0.2s` 在区域间切换时平滑滑动（纯 translate 不依赖 transform-origin，跨浏览器稳定；px 在 SVG 变换中映射为用户单位，与 `bars[i].cx` 同坐标系），首次进入/离开为即时显隐不滑动；`y1=PAD_T / y2=PAD_T+plotH` 跨整张直方图高度。
- 浮动卡定位：`onPlotMove` 用 `svgEl` / `wrapperEl` 的 `getBoundingClientRect()` 取当前槽像素左右边（相对 wrapper）与直方图垂直中点。`transform` 恒为 `translate(-100%,-50%)`（右沿对齐 `left`、垂直居中于中点），`top` 固定为直方图垂直中点、仅 `left` 变化并加 `transition: left 0.2s`，使区域间切换横向平滑滑动（垂直不动）。左侧优先（右沿贴区域左边 -8px，不覆盖当前柱）；左侧空间不足（最左若干根）翻到右侧（左沿贴区域右边 +8px），两种放法都以「右沿 = `left`」表达、transform 不变，故翻边亦为平滑横移而非跳变；最后按 wrapper 宽度夹紧 `left ∈ [tw, ww]` 避免极窄屏溢出。浮窗宽度**固定 `w-[220px]`**（`max-w-full` 兜底极窄屏），避免不同日期 `keys` 多寡 / 名称长短不同时浮窗宽度随内容撑开而漂移（最右侧「今天」常只有 `admin` 一个短 key，内容撑开会明显变窄，表现为「最右侧浮窗缩小」）；`ResizeObserver` 仍实测 `tooltipWidth`（默认回退 200），用于左侧空间判断与翻边（极窄屏 `max-w-full` 缩窄时同步实测值）。浮动卡 `pointer-events-none`，不拦截鼠标事件。
- 配色沿用 iris 色系：柱 `iris-violet`、hover 虚线 `iris-sky`、趋势线 `iris-gold`、图例方块 `bg-iris-violet`、短线 `bg-iris-gold`、浮动卡 `bg-white` + `border-iris-violet/30` + 来源圆点 key `bg-iris-violet` / admin `bg-iris-gold`、数量 `text-iris-dark`；网格线用浅灰 `#F1F5F9`、文字用 `fill-gray-400`。

## 与其它文件的关系

- 父组件：[`pages/logs/index.vue`](../../pages/logs/index.md)（近 14 天日志量）、[`pages/dashboard.vue`](../../pages/dashboard.md)（近 30 天新增图片趋势，传 `empty-text` / `title-text` 适配）。
- 类型来源：[`composables/useLogs`](../../composables/useLogs.md)（`HistogramBucket`，仪表盘经 [`useDashboard`](../../composables/useDashboard.md) 复用同结构）。
