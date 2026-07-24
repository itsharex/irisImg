<template>
  <div ref="wrapperEl" class="relative">
    <!-- 加载态 -->
    <div
      v-if="loading"
      class="flex h-[240px] items-center justify-center rounded-xl border border-dashed border-gray-200 bg-white/60"
    >
      <span class="text-sm text-gray-400">加载中…</span>
    </div>

    <!-- 错误态 -->
    <div
      v-else-if="error"
      class="flex h-[240px] flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-rose-200 bg-rose-50/50"
    >
      <p class="text-sm text-rose-600">{{ error }}</p>
      <button
        type="button"
        class="rounded-lg border border-gray-200 px-3 py-1.5 text-sm text-gray-600 transition hover:bg-gray-50"
        @click="emit('retry')"
      >
        重试
      </button>
    </div>

    <!-- 空态 -->
    <div
      v-else-if="total === 0"
      class="flex h-[240px] items-center justify-center rounded-xl border border-dashed border-gray-300 bg-white/60"
    >
      <span class="text-sm text-gray-400">{{ emptyText }}</span>
    </div>

    <!-- 直方图 + 趋势线 -->
    <svg
      v-else
      ref="svgEl"
      class="w-full"
      viewBox="0 0 700 240"
      preserveAspectRatio="xMidYMid meet"
      role="img"
      :aria-label="titleText"
    >
      <!-- 网格线 + Y 轴刻度 -->
      <g v-for="(g, i) in gridLines" :key="`g-${i}`">
        <line :x1="PAD_L" :y1="g.y" :x2="VB_W - PAD_R" :y2="g.y" stroke="#F1F5F9" stroke-width="1" />
        <text :x="PAD_L - 6" :y="g.y + 3" text-anchor="end" class="fill-gray-400 text-[10px]">{{ g.label }}</text>
      </g>

      <!-- 柱状图：恒为 iris-violet，hover 由顶层透明热区接管，柱身不再直接响应鼠标 -->
      <g>
        <rect
          v-for="(b, i) in bars"
          :key="`b-${i}`"
          :x="b.x"
          :y="b.y"
          :width="b.w"
          :height="b.h"
          rx="3"
          fill="#A48CE6"
        />
      </g>

      <!-- 悬停竖直虚线：跨整张直方图高度（绘图区顶到底），v-if 随悬停显隐；区域间切换时保持挂载、translateX 平滑滑动。
           颜色取 tailwind.config.ts 的 iris-sky (#86C9EC)：白底上可见，且与 iris-violet 柱身 / iris-gold 趋势线区分；改色需同步 config -->
      <line
        v-if="hoveredBar"
        x1="0"
        x2="0"
        :y1="PAD_T"
        :y2="PAD_T + plotH"
        stroke="#86C9EC"
        stroke-width="1.5"
        stroke-dasharray="3 3"
        pointer-events="none"
        :style="dashedLineStyle"
      />

      <!-- 趋势线（7 日移动平均） -->
      <polyline
        v-if="trendPath"
        :points="trendPath"
        fill="none"
        stroke="#F4C430"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      <circle
        v-for="(p, i) in trendPoints"
        :key="`t-${i}`"
        :cx="p.x"
        :cy="p.y"
        r="2.5"
        fill="#F4C430"
      />

      <!-- X 轴日期标签（隔行显示，避免拥挤） -->
      <text
        v-for="(b, i) in bars"
        v-show="i % labelStep === 0 || i === bars.length - 1"
        :key="`x-${i}`"
        :x="b.cx"
        :y="VB_H - 10"
        text-anchor="middle"
        class="fill-gray-400 text-[10px]"
      >{{ b.label }}</text>

      <!-- 透明热区：按日期等分整个绘图区，鼠标落在某区域（整个槽高）即视为悬停该日 -->
      <rect
        :x="PAD_L"
        :y="PAD_T"
        :width="plotW"
        :height="plotH"
        fill="transparent"
        pointer-events="all"
        @mousemove="onPlotMove"
        @mouseleave="onPlotLeave"
      />
    </svg>

    <!-- 悬停浮动信息卡：固定宽度 w-[220px]，避免不同日期 keys 多寡/名称长短不同时浮窗宽度漂移
         （最右侧「今天」常只有 admin 一个短 key，内容撑开会明显变窄）；max-w-full 兜底极窄屏。
         固定在直方图垂直中点，贴当前区域左侧（左侧无空间时翻到右侧，仍不覆盖当前柱），区域切换平滑横移 -->
    <div
      v-if="hoveredBar"
      ref="tooltipEl"
      class="pointer-events-none absolute z-30 w-[220px] max-w-full rounded-xl border border-iris-violet/30 bg-white px-3 py-2 shadow-lg"
      :style="tooltipStyle"
    >
      <div class="flex items-center justify-between gap-4">
        <span class="text-xs font-semibold text-gray-900">{{ hoveredBar.date }}</span>
        <span class="text-xs font-semibold tabular-nums text-iris-dark">{{ hoveredBar.count }} {{ unit }}</span>
      </div>
      <div
        v-if="hoveredBar.keys && hoveredBar.keys.length"
        class="mt-1.5 max-h-[180px] space-y-1 overflow-y-auto border-t border-gray-100 pt-1.5"
      >
        <div
          v-for="(k, ki) in hoveredBar.keys"
          :key="ki"
          class="flex items-center justify-between gap-3"
        >
          <span class="flex min-w-0 items-center gap-1.5">
            <span
              class="inline-block h-2 w-2 shrink-0 rounded-sm"
              :class="k.name === 'admin' ? 'bg-iris-gold' : 'bg-iris-violet'"
            ></span>
            <span class="truncate text-xs text-gray-700">{{ k.name }}</span>
          </span>
          <span class="shrink-0 text-xs tabular-nums text-gray-600">{{ k.count }}</span>
        </div>
      </div>
    </div>

    <!-- 图例 -->
    <div class="mt-2 flex items-center justify-end gap-4 text-xs text-gray-500">
      <span class="flex items-center gap-1.5">
        <span class="inline-block h-2.5 w-2.5 rounded-sm bg-iris-violet"></span>{{ legendText }}
      </span>
      <span class="flex items-center gap-1.5">
        <span class="inline-block h-0.5 w-4 bg-iris-gold"></span>7 日均值
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { HistogramBucket } from '~/composables/useLogs'

const props = withDefaults(defineProps<{
  buckets: HistogramBucket[]
  total: number
  loading: boolean
  error: string | null
  /** 空态文案，默认为日志中心原文案；仪表盘图片趋势可传「近 30 天暂无新增图片」。 */
  emptyText?: string
  /** aria-label 文案，默认为日志中心原文案。 */
  titleText?: string
  /** 柱状图图例文案，默认「日志量」；仪表盘图片趋势可传「新增图片」。 */
  legendText?: string
  /** 浮动信息卡表头数量单位，默认「条」；仪表盘图片趋势传「张」。 */
  unit?: string
}>(), {
  emptyText: '近 14 天暂无日志',
  titleText: '近 14 天日志量直方图',
  legendText: '日志量',
  unit: '条',
})
const emit = defineEmits<{ retry: [] }>()

// SVG 视口与内边距常量。
const VB_W = 700
const VB_H = 240
const PAD_L = 40
const PAD_R = 16
const PAD_T = 16
const PAD_B = 36

const plotW = VB_W - PAD_L - PAD_R
const plotH = VB_H - PAD_T - PAD_B

const hovered = ref<number | null>(null)
// 虚线横坐标（viewBox 单位），由 hovered 槽中线派生；v-if 控制显隐，区域间切换由 transform 过渡平滑滑动。
const cursorX = computed(() => (hovered.value !== null ? bars.value[hovered.value]?.cx ?? 0 : 0))

const wrapperEl = ref<HTMLElement | null>(null)
const svgEl = ref<SVGSVGElement | null>(null)
const tooltipEl = ref<HTMLElement | null>(null)

// 浮窗像素几何（均相对 wrapper）：当前热区左右边 + 直方图垂直中点 + 浮窗实测宽度。
const regionGeom = ref<{ left: number; right: number } | null>(null)
const plotMiddlePx = ref(0)
const tooltipWidth = ref(0)

const hoveredBar = computed(() => (hovered.value !== null ? bars.value[hovered.value] ?? null : null))

// 虚线：translateX 滑动。纯 translate 不依赖 transform-origin，跨浏览器稳定；
// px 在 SVG 变换中映射为用户单位（viewBox 单位），与 bars[i].cx 同坐标系。
const dashedLineStyle = computed(() => ({
  transform: `translateX(${cursorX.value}px)`,
  transition: 'transform 0.2s ease',
}))

// 浮窗定位：transform 恒为 translate(-100%,-50%)（右沿对齐 left、垂直居中于中点），
// 仅 left 变化并加 transition，使区域间切换横向平滑滑动。
// 左侧优先（右沿贴当前区域左边 -gap）；左侧空间不足（最左若干根）翻到右侧（左沿贴区域右边 +gap），
// 两种放法都以「right = left」表达，transform 不变，故翻边亦为平滑横移而非跳变。
// 最后按 wrapper 宽度夹紧，避免极窄屏溢出。
const tooltipStyle = computed(() => {
  const g = regionGeom.value
  if (!g) return { display: 'none' }
  const gap = 8
  const tw = tooltipWidth.value || 200
  const ww = wrapperEl.value?.clientWidth ?? 0
  let left: number
  if (g.left - gap - tw >= 0) {
    left = g.left - gap // 左侧放置：浮窗右沿 = 区域左边 - gap
  } else {
    left = g.right + gap + tw // 右侧放置：浮窗左沿 = 区域右边 + gap（right = left - tw）
  }
  if (ww > 0) left = Math.max(tw, Math.min(left, ww)) // 夹紧到 [tw, ww]，保证 [left-tw, left] ⊂ [0, ww]
  return {
    left: `${left}px`,
    top: `${plotMiddlePx.value}px`,
    transform: 'translate(-100%, -50%)',
    transition: 'left 0.2s ease, top 0.2s ease',
  }
})

// 透明热区 mousemove：按鼠标 x 反算落在哪个等分槽，整个槽高均视为该日区域。
function onPlotMove(e: MouseEvent) {
  const svg = svgEl.value
  const wrapper = wrapperEl.value
  if (!svg || !wrapper) return
  const n = props.buckets.length
  if (n === 0) return
  const sRect = svg.getBoundingClientRect()
  const wRect = wrapper.getBoundingClientRect()
  // 鼠标 x 换算到 viewBox 单位
  const px = ((e.clientX - sRect.left) / sRect.width) * VB_W
  const slot = plotW / n
  const rel = px - PAD_L
  if (rel < 0 || rel >= plotW) return // 落在绘图区外（Y 轴标签区等）不处理
  const i = Math.min(n - 1, Math.max(0, Math.floor(rel / slot)))
  if (i !== hovered.value) {
    hovered.value = i // cursorX 由 hovered 派生，区域间切换时 transform 过渡平滑滑动
  }
  // 当前槽像素包围盒 + 直方图垂直中点（相对 wrapper），供浮窗定位
  const scale = sRect.width / VB_W
  regionGeom.value = {
    left: (sRect.left - wRect.left) + (PAD_L + slot * i) * scale,
    right: (sRect.left - wRect.left) + (PAD_L + slot * (i + 1)) * scale,
  }
  plotMiddlePx.value = (sRect.top - wRect.top) + (PAD_T + plotH / 2) * (sRect.height / VB_H)
}

function onPlotLeave() {
  hovered.value = null
}

// 浮窗挂载/卸载及内容尺寸变化时实测宽度，用于左侧空间判断与翻边。
watch(tooltipEl, (el, _old, onCleanup) => {
  if (!el) return
  tooltipWidth.value = el.offsetWidth
  const ro = new ResizeObserver(() => {
    tooltipWidth.value = el.offsetWidth
  })
  ro.observe(el)
  onCleanup(() => ro.disconnect())
}, { flush: 'post' })

const counts = computed(() => props.buckets.map(b => b.count))
// 至少为 1，避免除零；无数据时直方图走空态分支不会渲染。
const max = computed(() => Math.max(1, ...counts.value))

const bars = computed(() =>
  props.buckets.map((b, i) => {
    const n = props.buckets.length || 1
    const slot = plotW / n
    const cx = PAD_L + slot * (i + 0.5)
    const w = slot * 0.6
    const h = (b.count / max.value) * plotH
    const y = PAD_T + plotH - h
    return {
      x: cx - w / 2,
      y,
      w,
      h,
      cx,
      count: b.count,
      date: b.date,
      label: b.date.slice(5), // YYYY-MM-DD -> MM-DD
      keys: b.keys, // 仅仪表盘图片趋势携带，日志直方图为 undefined
    }
  }),
)

// X 轴日期标签隔行步长：按桶数自适应，使标签数稳定在约 8 个以内，避免 30 天时拥挤。
// 14 天时 step=2 与历史行为一致；30 天时 step=4。
const labelStep = computed(() => Math.max(1, Math.ceil((props.buckets.length || 1) / 8)))

// 7 日移动平均：窗口不足 7 时取已有天均值，使趋势线从首日即横跨全宽。
const trend = computed(() => {
  const c = counts.value
  const window = 7
  return c.map((_, i) => {
    const start = Math.max(0, i - window + 1)
    const slice = c.slice(start, i + 1)
    return slice.reduce((a, b) => a + b, 0) / slice.length
  })
})

const trendPoints = computed(() =>
  trend.value.map((v, i) => ({
    x: bars.value[i].cx,
    y: PAD_T + plotH - (v / max.value) * plotH,
  })),
)

const trendPath = computed(() =>
  trendPoints.value.map(p => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '),
)

// 4 等分网格线 + 刻度（0 / 1/4 / 1/2 / 3/4 / max）。
const gridLines = computed(() => {
  const steps = 4
  const lines: { y: number; label: number }[] = []
  for (let i = 0; i <= steps; i++) {
    const val = (max.value * i) / steps
    lines.push({
      y: PAD_T + plotH - (val / max.value) * plotH,
      label: Math.round(val),
    })
  }
  return lines
})
</script>
