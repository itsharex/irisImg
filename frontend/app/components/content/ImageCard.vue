<template>
  <button
    type="button"
    class="group flex flex-col overflow-hidden rounded-2xl border bg-white text-left transition focus:outline-none focus:ring-2 focus:ring-iris-violet/40"
    :class="borderClass"
    :aria-pressed="selectable ? selected : undefined"
    @click="emit('click', image)"
  >
    <div class="relative aspect-square w-full overflow-hidden bg-gray-50">
      <img
        :src="resolvedUrl"
        :alt="image.filename"
        loading="lazy"
        class="h-full w-full object-cover transition duration-200 group-hover:scale-[1.03]"
      />
      <!-- 多选模式：右下角勾选徽标，两态切换（未选中半透明空圈 / 选中实底勾）。
           常驻占位而非选中才出现，避免切换引起布局跳动，也给用户「可点选」暗示。 -->
      <span
        v-if="selectable"
        class="absolute bottom-2 right-2 flex h-6 w-6 items-center justify-center rounded-full border transition"
        :class="
          selected
            ? 'border-iris-dark bg-iris-dark text-white shadow-sm'
            : 'border-white/70 bg-black/30 text-transparent backdrop-blur-sm'
        "
      >
        <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M4.5 12.75l6 6 9-13.5" />
        </svg>
      </span>
    </div>
    <div class="flex flex-col gap-0.5 px-3 py-2">
      <span class="truncate text-sm font-medium text-gray-800" :title="image.filename">{{ image.filename }}</span>
      <span class="text-xs text-gray-400">{{ formatBytes(image.size) }} · {{ formatDate(image.created_at) }}</span>
    </div>
  </button>
</template>

<script setup lang="ts">
import type { ImageItem } from '~/composables/useImages'

const props = defineProps<{
  image: ImageItem
  /** 是否处于多选模式：显示右下角勾选徽标，hover 提示「点击是选择」。 */
  selectable?: boolean
  /** 多选模式下该卡片是否被选中：主色描边 + 柔和 ring + 实底勾。 */
  selected?: boolean
}>()
const emit = defineEmits<{ (e: 'click', image: ImageItem): void }>()

const resolvedUrl = computed(() => resolveImageUrl(props.image.url))

// 边框样式三态互斥：选中 > 多选未选中 > 默认。
// 选中态与 hover 放在互斥分支而非叠加类，避免 hover 把选中边框撤掉。
const borderClass = computed(() => {
  if (props.selected) return 'border-iris-dark ring-2 ring-iris-violet/30'
  if (props.selectable) return 'border-gray-200 hover:border-iris-dark/50'
  return 'border-gray-200 hover:border-iris-violet/60 hover:shadow-sm'
})
</script>
