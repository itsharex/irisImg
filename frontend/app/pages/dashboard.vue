<template>
  <div class="space-y-6">
    <!-- 顶部标题 + 刷新 -->
    <div class="flex items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-gray-900">仪表盘</h1>
        <p class="mt-1 text-sm text-gray-500">概览你的图片服务运行状况</p>
      </div>
      <button
        type="button"
        class="inline-flex shrink-0 items-center gap-1.5 rounded-xl bg-iris-violet/10 px-3 py-2 text-sm font-medium text-iris-dark transition hover:bg-iris-violet/15"
        :disabled="loading"
        @click="fetchOverview"
      >
        <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="1.5"
            d="M16.023 9.348h4.992V4.356M19.5 9.348a7.5 7.5 0 01-13.5 4.5M7.977 14.652H2.985v4.992M4.5 14.652a7.5 7.5 0 0113.5-4.5"
          />
        </svg>
        刷新
      </button>
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-2 gap-6 lg:grid-cols-4">
      <template v-if="loading">
        <div v-for="i in 4" :key="i" class="rounded-2xl border border-gray-200 bg-white p-6">
          <div class="flex items-start justify-between gap-4">
            <div class="space-y-2">
              <div class="h-4 w-16 rounded bg-gray-200 animate-pulse"></div>
              <div class="h-8 w-24 rounded bg-gray-200 animate-pulse"></div>
            </div>
            <div class="h-10 w-10 rounded-xl bg-gray-200 animate-pulse"></div>
          </div>
        </div>
      </template>
      <template v-else-if="error">
        <div
          class="col-span-2 flex flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-rose-200 bg-rose-50/50 p-8 lg:col-span-4"
        >
          <p class="text-sm text-rose-600">{{ error }}</p>
          <button
            type="button"
            class="rounded-lg border border-gray-200 px-3 py-1.5 text-sm text-gray-600 transition hover:bg-gray-50"
            @click="fetchOverview"
          >
            重试
          </button>
        </div>
      </template>
      <template v-else-if="stats">
        <DashboardStatCard label="图片总量" :value="stats.images_total" :icon="icons.image" />
        <DashboardStatCard
          label="存储占用"
          :value="formatBytes(stats.storage_bytes)"
          :icon="icons.storage"
          hint="已登记图片总大小"
        />
        <DashboardStatCard
          label="APIkey 数"
          :value="stats.apikeys_total"
          :icon="icons.key"
          :hint="`有效 ${stats.apikeys_active} · 已吊销 ${stats.apikeys_revoked}`"
        />
        <DashboardStatCard label="日志总量" :value="stats.logs_total" :icon="icons.doc" />
      </template>
    </div>

    <!-- 近 N 天新增图片趋势 -->
    <div class="rounded-2xl border border-gray-200 bg-white p-6">
      <div class="mb-4 flex items-center justify-between gap-4">
        <div>
          <h2 class="text-base font-semibold text-gray-900">近 {{ days }} 天新增图片</h2>
          <p class="mt-0.5 text-xs text-gray-500">按创建时间统计每日新增</p>
        </div>
        <div class="text-right">
          <p class="text-xs text-gray-500">合计新增</p>
          <p class="text-2xl font-bold text-iris-dark">
            {{ stats ? stats.recent_upload_total : '—' }}
          </p>
        </div>
      </div>
      <LogsHistogram
        :buckets="stats?.recent_upload_trend ?? []"
        :total="stats?.recent_upload_total ?? 0"
        :loading="loading"
        :error="error"
        :empty-text="`近 ${days} 天暂无新增图片`"
        :title-text="`近 ${days} 天新增图片趋势`"
        :legend-text="'新增图片'"
        :unit="'张'"
        @retry="fetchOverview"
      />
    </div>

    <!-- 快捷入口 -->
    <div>
      <h2 class="mb-3 text-base font-semibold text-gray-900">快捷入口</h2>
      <div class="grid grid-cols-2 gap-6 lg:grid-cols-4">
        <NuxtLink
          v-for="entry in quickEntries"
          :key="entry.to"
          :to="entry.to"
          class="group rounded-2xl border border-gray-200 bg-white p-5 transition hover:border-iris-violet hover:shadow-sm"
        >
          <div class="flex items-center gap-3">
            <span
              class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-iris-violet/10 text-iris-dark transition group-hover:bg-iris-violet/15"
            >
              <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" :d="entry.icon" />
              </svg>
            </span>
            <span class="text-sm font-semibold text-gray-900">{{ entry.label }}</span>
          </div>
          <p class="mt-3 text-xs text-gray-500">{{ entry.desc }}</p>
        </NuxtLink>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { formatBytes } from '~/composables/useImages'
import type { DashboardStats } from '~/composables/useDashboard'

definePageMeta({ middleware: 'auth' })

const { overview } = useDashboard()

const stats = ref<DashboardStats | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)

const days = computed(() => stats.value?.days ?? 30)

// heroicons outline path d，与 AppSidebar.vue 的 navItems 保持一致；
// storage 用 circle-stack（数据库）图标，语义对应存储占用。
const icons = {
  image: 'M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 001.5-1.5V6a1.5 1.5 0 00-1.5-1.5H3.75A1.5 1.5 0 002.25 6v12a1.5 1.5 0 001.5 1.5zm10.5-11.25h.008v.008h-.008V8.25zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z',
  storage: 'M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125S3.75 20.028 3.75 17.75V6.375m16.5 0v3.75m-16.5-3.75v3.75m16.5 0v3.75C20.25 16.153 16.556 18 12 18s-8.25-1.847-8.25-4.125v-3.75m16.5 0c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125',
  key: 'M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H7.5v3H4.5v-3H1.5v-3.75l5.879-5.879c.404-.404.526-1 .43-1.563A6 6 0 1120.25 8.25z M15 9a.75.75 0 00-.75.75.75.75 0 01-.75.75.75.75 0 00-.75.75.75.75 0 01-.75.75m0 0a.75.75 0 01-.75-.75.75.75 0 00-.75-.75.75.75 0 01-.75-.75.75.75 0 00-.75-.75',
  doc: 'M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z',
  cog: 'M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z M15 12a3 3 0 11-6 0 3 3 0 016 0z',
}

// 快捷入口：路由 / 标题 / 描述 / 图标，图标复用 AppSidebar navItems 的 heroicons path。
const quickEntries = [
  { to: '/content', label: '内容中心', desc: '管理已上传的图片资源，按 Key 筛选、分页浏览与后台直传', icon: icons.image },
  { to: '/logs', label: '日志中心', desc: '查看访问与业务日志，14 天直方图趋势、多维筛选与清理', icon: icons.doc },
  { to: '/apikeys', label: 'APIkey 管理', desc: '创建与维护 API 密钥，重命名 / 重置 / 吊销 / 删除', icon: icons.key },
  { to: '/settings', label: '系统配置', desc: '查看当前运行配置（存储 / 数据库 / APIkey / 服务端只读快照）', icon: icons.cog },
]

async function fetchOverview() {
  loading.value = true
  error.value = null
  try {
    stats.value = await overview(30)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载仪表盘数据失败'
    stats.value = null
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchOverview()
})
</script>
