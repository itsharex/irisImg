<template>
  <nav class="flex items-center gap-1" aria-label="分页">
    <button
      type="button"
      class="rounded-lg border border-gray-200 px-3 py-1.5 text-sm text-gray-600 transition hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40"
      :disabled="loading || page <= 1"
      @click="emit('change', page - 1)"
    >
      上一页
    </button>

    <template v-for="(item, i) in items" :key="`${item}-${i}`">
      <span v-if="item === ELLIPSIS" class="px-1 text-sm text-gray-400">…</span>
      <button
        v-else
        type="button"
        class="h-8 min-w-8 rounded-lg border px-2 text-sm transition"
        :class="
          item === page
            ? 'cursor-not-allowed border-gray-200 bg-gray-100 font-medium text-gray-400'
            : 'border-gray-200 text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40'
        "
        :disabled="loading || item === page"
        :aria-current="item === page ? 'page' : undefined"
        @click="emit('change', item)"
      >
        {{ item }}
      </button>
    </template>

    <button
      type="button"
      class="rounded-lg border border-gray-200 px-3 py-1.5 text-sm text-gray-600 transition hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40"
      :disabled="loading || page >= totalPages"
      @click="emit('change', page + 1)"
    >
      下一页
    </button>
  </nav>
</template>

<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    /** 当前页码（从 1 开始）。 */
    page: number
    /** 总页数。 */
    totalPages: number
    /** 请求进行中时禁用全部按钮，防止连点。 */
    loading?: boolean
    /** 当前页两侧各显示的页码数。 */
    siblingCount?: number
  }>(),
  { loading: false, siblingCount: 3 },
)

const emit = defineEmits<{ change: [page: number] }>()

// 页码序列元素：数字页码或省略号占位。
type PageItem = number | '…'

const ELLIPSIS = '…' as const

// 生成带省略号的页码序列：首末页固定可见可点（即首页/末页直达）；页数不多时全量显示；
// 否则当前页两侧各留 siblingCount 个页码，与首/末页不连续处折叠成省略号。
// 例：totalPages=16、page=7、siblingCount=3 → 1 … 4 5 6 7 8 9 10 … 16
const items = computed<PageItem[]>(() => {
  const totalPages = Math.max(1, Math.floor(props.totalPages))
  const page = Math.min(Math.max(1, Math.floor(props.page)), totalPages)
  const siblingCount = Math.max(0, Math.floor(props.siblingCount))

  // 页数不多时直接全量显示，无需折叠。
  if (totalPages <= siblingCount * 2 + 5) {
    return Array.from({ length: totalPages }, (_, i) => i + 1)
  }

  const result: PageItem[] = [1]
  const start = Math.max(2, page - siblingCount)
  const end = Math.min(totalPages - 1, page + siblingCount)

  if (start > 2) result.push(ELLIPSIS)
  for (let p = start; p <= end; p++) result.push(p)
  if (end < totalPages - 1) result.push(ELLIPSIS)

  result.push(totalPages)
  return result
})
</script>
