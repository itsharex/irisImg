<template>
  <div class="space-y-6">
    <!-- 顶部标题 -->
    <div>
      <h1 class="text-2xl font-bold text-gray-900">日志中心</h1>
      <p class="mt-1 text-sm text-gray-500">查看访问与业务日志，支持多维筛选、按日趋势与清理</p>
    </div>

    <!-- 筛选栏 -->
    <div class="rounded-2xl border border-gray-200 bg-white p-4">
      <div class="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-4">
        <label class="block">
          <span class="mb-1 block text-xs font-medium text-gray-500">级别</span>
          <select v-model="filters.level" class="w-full rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-900 outline-none transition-colors focus:border-iris-dark focus:bg-white focus:ring-2 focus:ring-iris-dark/10">
            <option v-for="o in levelOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-medium text-gray-500">事件</span>
          <select v-model="filters.event" class="w-full rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-900 outline-none transition-colors focus:border-iris-dark focus:bg-white focus:ring-2 focus:ring-iris-dark/10">
            <option v-for="o in eventOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-medium text-gray-500">方法</span>
          <select v-model="filters.method" class="w-full rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-900 outline-none transition-colors focus:border-iris-dark focus:bg-white focus:ring-2 focus:ring-iris-dark/10">
            <option v-for="o in methodOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-medium text-gray-500">状态</span>
          <select v-model="filters.statusClass" class="w-full rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-900 outline-none transition-colors focus:border-iris-dark focus:bg-white focus:ring-2 focus:ring-iris-dark/10">
            <option v-for="o in statusOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-medium text-gray-500">关键字</span>
          <input
            v-model="filters.keyword"
            type="text"
            placeholder="路径 / 详情"
            class="w-full rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-900 outline-none transition-colors placeholder:text-gray-400 focus:border-iris-dark focus:bg-white focus:ring-2 focus:ring-iris-dark/10"
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-medium text-gray-500">起始日期</span>
          <input
            v-model="filters.start"
            type="date"
            class="w-full rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-900 outline-none transition-colors focus:border-iris-dark focus:bg-white focus:ring-2 focus:ring-iris-dark/10"
          />
        </label>
        <label class="block">
          <span class="mb-1 block text-xs font-medium text-gray-500">结束日期</span>
          <input
            v-model="filters.end"
            type="date"
            class="w-full rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-900 outline-none transition-colors focus:border-iris-dark focus:bg-white focus:ring-2 focus:ring-iris-dark/10"
          />
        </label>
        <div class="flex items-end gap-2">
          <button
            type="button"
            class="flex-1 rounded-xl bg-iris-violet/10 px-3 py-2 text-sm font-medium text-iris-dark transition hover:bg-iris-violet/15"
            @click="onSearch"
          >
            查询
          </button>
          <button
            type="button"
            class="rounded-xl border border-gray-200 px-3 py-2 text-sm text-gray-600 transition hover:bg-gray-50"
            @click="onReset"
          >
            重置
          </button>
        </div>
      </div>
    </div>

    <!-- 直方图卡片 + 清理按钮 -->
    <div class="rounded-2xl border border-gray-200 bg-white p-4">
      <div class="mb-2 flex items-center justify-between gap-4">
        <div>
          <h2 class="text-sm font-semibold text-gray-900">近 14 天日志量</h2>
          <p class="mt-0.5 text-xs text-gray-500">共 {{ histTotal }} 条</p>
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-xl bg-rose-50 px-3 py-2 text-sm font-medium text-rose-600 transition hover:bg-rose-100"
            @click="purgeOpen = true"
          >
            <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="1.5"
                d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0"
              />
            </svg>
            清理全部日志
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-xl bg-rose-50 px-3 py-2 text-sm font-medium text-rose-600 transition hover:bg-rose-100"
            @click="purgeGetOpen = true"
          >
            <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="1.5"
                d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0"
              />
            </svg>
            清理GET日志
          </button>
        </div>
      </div>
      <LogsHistogram :buckets="buckets" :total="histTotal" :loading="histLoading" :error="histError" @retry="fetchHistogram" />
    </div>

    <!-- 分页日志栏 -->
    <div>
      <div class="mb-4 flex items-center justify-between gap-4">
        <p class="text-sm text-gray-500">
          共 <span class="font-medium text-gray-700">{{ total }}</span> 条
        </p>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="rounded-lg border border-gray-200 px-3 py-1.5 text-sm text-gray-600 transition hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40"
            :disabled="loading || page <= 1"
            @click="goPage(page - 1)"
          >
            上一页
          </button>
          <span class="text-sm text-gray-500">{{ page }} / {{ totalPages }}</span>
          <button
            type="button"
            class="rounded-lg border border-gray-200 px-3 py-1.5 text-sm text-gray-600 transition hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40"
            :disabled="loading || page >= totalPages"
            @click="goPage(page + 1)"
          >
            下一页
          </button>
        </div>
      </div>

      <LogsTable :logs="logs" :loading="loading" :error="error" @retry="fetchLogs" />
    </div>

    <!-- 清理日志二次确认弹窗（全部 / 仅 info 级 GET） -->
    <LogsPurgeDialog :open="purgeOpen" @close="purgeOpen = false" @done="onPurgedAll" />
    <LogsPurgeDialog scope="get" :open="purgeGetOpen" @close="purgeGetOpen = false" @done="onPurgedGet" />
  </div>
</template>

<script setup lang="ts">
import type { LogItem, HistogramBucket, ListLogsParams } from '~/composables/useLogs'

definePageMeta({ middleware: 'auth' })

const PAGE_SIZE = 50
const { list, histogram } = useLogs()

// 筛选状态
const filters = reactive({
  level: '',
  event: '',
  method: '',
  statusClass: '',
  keyword: '',
  start: '', // YYYY-MM-DD
  end: '',
})

// 列表状态
const logs = ref<LogItem[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref<string | null>(null)

// 直方图状态
const buckets = ref<HistogramBucket[]>([])
const histTotal = ref(0)
const histLoading = ref(false)
const histError = ref<string | null>(null)

// 清理弹窗（全部 / 仅 info 级 GET）
const purgeOpen = ref(false)
const purgeGetOpen = ref(false)

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))

const levelOptions = [
  { value: '', label: '全部级别' },
  { value: 'debug', label: 'Debug' },
  { value: 'info', label: 'Info' },
  { value: 'warn', label: 'Warn' },
  { value: 'error', label: 'Error' },
]

const eventOptions = [
  { value: '', label: '全部事件' },
  { value: 'http.request', label: 'HTTP 请求' },
  { value: 'image.upload', label: '图片上传' },
  { value: 'apikey.create', label: '创建密钥' },
  { value: 'apikey.rename', label: '重命名密钥' },
  { value: 'apikey.reset', label: '重置密钥' },
  { value: 'apikey.revoke', label: '吊销密钥' },
  { value: 'apikey.delete', label: '删除密钥' },
  { value: 'auth.login_success', label: '登录成功' },
  { value: 'auth.login_failed', label: '登录失败' },
  { value: 'log.clear', label: '清理全部日志' },
  { value: 'log.clear_get', label: '清理GET日志' },
  { value: 'panic', label: 'Panic' },
]

const methodOptions = [
  { value: '', label: '全部方法' },
  { value: 'GET', label: 'GET' },
  { value: 'POST', label: 'POST' },
  { value: 'PATCH', label: 'PATCH' },
  { value: 'DELETE', label: 'DELETE' },
]

const statusOptions = [
  { value: '', label: '全部状态' },
  { value: '2xx', label: '2xx 成功' },
  { value: '4xx', label: '4xx 客户端错误' },
  { value: '5xx', label: '5xx 服务端错误' },
]

// 把筛选条件 + 页码组装成请求参数；日期从本地 YYYY-MM-DD 转为 RFC3339（起=当日 0 点，终=次日 0 点，左闭右开）。
function buildQuery(p: number): ListLogsParams {
  const q: ListLogsParams = { page: p, pageSize: PAGE_SIZE }
  if (filters.level) q.level = filters.level
  if (filters.event) q.event = filters.event
  if (filters.method) q.method = filters.method
  if (filters.statusClass) q.statusClass = filters.statusClass
  if (filters.keyword.trim()) q.keyword = filters.keyword.trim()
  if (filters.start) q.start = new Date(`${filters.start}T00:00:00`).toISOString()
  if (filters.end) {
    const d = new Date(`${filters.end}T00:00:00`)
    d.setDate(d.getDate() + 1)
    q.end = d.toISOString()
  }
  return q
}

async function fetchLogs() {
  loading.value = true
  error.value = null
  try {
    const data = await list(buildQuery(page.value))
    logs.value = data.items ?? []
    total.value = data.total ?? 0
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载日志失败'
    logs.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

async function fetchHistogram() {
  histLoading.value = true
  histError.value = null
  try {
    const data = await histogram()
    buckets.value = data.buckets ?? []
    histTotal.value = data.total ?? 0
  } catch (e) {
    histError.value = e instanceof Error ? e.message : '加载直方图失败'
    buckets.value = []
    histTotal.value = 0
  } finally {
    histLoading.value = false
  }
}

function onSearch() {
  page.value = 1
  fetchLogs()
}

function onReset() {
  filters.level = ''
  filters.event = ''
  filters.method = ''
  filters.statusClass = ''
  filters.keyword = ''
  filters.start = ''
  filters.end = ''
  page.value = 1
  fetchLogs()
}

function goPage(p: number) {
  if (p < 1 || p > totalPages.value || p === page.value) return
  page.value = p
  fetchLogs()
}

function onPurgedAll() {
  purgeOpen.value = false
  refreshAfterPurge()
}

function onPurgedGet() {
  purgeGetOpen.value = false
  refreshAfterPurge()
}

// 清理后直方图与列表都需刷新（列表回到第 1 页，并能看到补记的 log.clear / log.clear_get 审计事件）。
function refreshAfterPurge() {
  fetchHistogram()
  page.value = 1
  fetchLogs()
}

onMounted(() => {
  fetchHistogram()
  fetchLogs()
})
</script>
