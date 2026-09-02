<template>
  <UiBaseDialog :open="open" :title="title" content-class="max-w-xl" @close="emit('close')">
    <div class="space-y-5">
      <!-- 警告 -->
      <div class="flex items-start gap-2 rounded-lg bg-rose-50 p-3 text-sm text-rose-700">
        <svg class="mt-0.5 h-5 w-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="1.5"
            d="M12 9v3.75m-9-.75a9 9 0 1118 0 9 9 0 01-18 0zm9 3.75h.008v.008H12v-.008z"
          />
        </svg>
        <p>{{ warning }}</p>
      </div>

      <!-- 账号密码二次确认 -->
      <div class="space-y-3 border-t border-gray-100 pt-4">
        <p class="text-sm font-medium text-gray-700">为确认是你本人操作，请重新输入账号密码</p>
        <div>
          <label for="purge-username" class="sr-only">用户名</label>
          <input
            id="purge-username"
            v-model="form.username"
            type="text"
            placeholder="用户名"
            autocomplete="username"
            class="w-full rounded-xl border border-gray-200 bg-gray-50 px-4 py-2.5 text-gray-900 outline-none transition-colors placeholder:text-gray-400 focus:border-iris-dark focus:bg-white focus:ring-2 focus:ring-iris-dark/10"
          />
        </div>
        <div>
          <label for="purge-password" class="sr-only">密码</label>
          <input
            id="purge-password"
            v-model="form.password"
            type="password"
            placeholder="密码"
            autocomplete="current-password"
            class="w-full rounded-xl border border-gray-200 bg-gray-50 px-4 py-2.5 text-gray-900 outline-none transition-colors placeholder:text-gray-400 focus:border-iris-dark focus:bg-white focus:ring-2 focus:ring-iris-dark/10"
          />
        </div>
        <div v-if="serverError" class="rounded-lg bg-red-50 p-3 text-sm text-red-600">{{ serverError }}</div>
      </div>
    </div>

    <template #footer>
      <button
        type="button"
        class="rounded-lg border border-gray-200 px-3 py-1.5 text-sm text-gray-600 transition hover:bg-gray-50"
        @click="emit('close')"
      >
        取消
      </button>
      <button
        type="button"
        :disabled="!canSubmit || loading"
        class="inline-flex items-center justify-center gap-1.5 rounded-xl px-3 py-2 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-60 bg-rose-50 text-rose-600 hover:bg-rose-100"
        @click="handleSubmit"
      >
        <span v-if="loading" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-rose-200 border-t-rose-600"></span>
        确认清理
      </button>
    </template>
  </UiBaseDialog>
</template>

<script setup lang="ts">
const props = withDefaults(defineProps<{ open: boolean; scope?: 'all' | 'get' }>(), { scope: 'all' })
const emit = defineEmits<{
  close: []
  done: []
}>()

const { purge } = useLogs()
const { user } = useAuth()

const form = reactive({ username: '', password: '' })
const serverError = ref('')
const loading = ref(false)

// 按 scope 切换标题与警告文案：'all' 清空全部；'get' 仅清 info 级 GET 请求日志。
const title = computed(() => (props.scope === 'get' ? '清理GET日志' : '清理全部日志'))
const warning = computed(() =>
  props.scope === 'get'
    ? '此操作将删除全部 info 级别的 GET 请求日志，不可撤销。清理后会保留一条审计记录。'
    : '此操作将清空全部历史日志，不可撤销。清理后会保留一条审计记录。',
)

const canSubmit = computed(() => form.username.trim() !== '' && form.password !== '')

// 每次打开重置表单、预填当前用户名、清空错误。
watch(
  () => props.open,
  (v) => {
    if (v) {
      form.username = user.value?.username ?? ''
      form.password = ''
      serverError.value = ''
      loading.value = false
    }
  },
)

async function handleSubmit() {
  if (loading.value || !canSubmit.value) return
  loading.value = true
  serverError.value = ''
  try {
    await purge({ username: form.username.trim(), password: form.password, scope: props.scope })
    emit('done')
  } catch (err: unknown) {
    serverError.value = err instanceof Error ? err.message : '操作失败，请稍后重试'
  } finally {
    loading.value = false
  }
}
</script>
