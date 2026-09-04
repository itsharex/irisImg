<template>
  <UiBaseDialog :open="open" title="批量删除图片" content-class="max-w-xl" @close="emit('close')">
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
        <p>此操作将永久删除选中的 {{ ids.length }} 张图片（含物理文件与记录），不可撤销。</p>
      </div>

      <!-- 账号密码二次确认 -->
      <div class="space-y-3 border-t border-gray-100 pt-4">
        <p class="text-sm font-medium text-gray-700">为确认是你本人操作，请重新输入账号密码</p>
        <div>
          <label for="bdel-username" class="sr-only">用户名</label>
          <input
            id="bdel-username"
            v-model="form.username"
            type="text"
            placeholder="用户名"
            autocomplete="username"
            class="w-full rounded-xl border border-gray-200 bg-gray-50 px-4 py-2.5 text-gray-900 outline-none transition-colors placeholder:text-gray-400 focus:border-iris-dark focus:bg-white focus:ring-2 focus:ring-iris-dark/10"
          />
        </div>
        <div>
          <label for="bdel-password" class="sr-only">密码</label>
          <input
            id="bdel-password"
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
        确认删除
      </button>
    </template>
  </UiBaseDialog>
</template>

<script setup lang="ts">
import type { BatchDeleteImagesResult } from '~/composables/useImages'

const props = defineProps<{ open: boolean; ids: number[] }>()
const emit = defineEmits<{
  close: []
  done: [result: BatchDeleteImagesResult]
}>()

const { batchRemove } = useImages()
const { user } = useAuth()

const form = reactive({ username: '', password: '' })
const serverError = ref('')
const loading = ref(false)

const canSubmit = computed(() => form.username.trim() !== '' && form.password !== '' && props.ids.length > 0)

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
    const result = await batchRemove({
      username: form.username.trim(),
      password: form.password,
      ids: props.ids,
    })
    emit('done', result)
  } catch (err: unknown) {
    // 二次确认失败后端返回 403（而非 401），不触发 useApi 的全局登出；
    // 弹窗保持打开，内联报错，用户修正密码后可重试。
    serverError.value = err instanceof Error ? err.message : '操作失败，请稍后重试'
  } finally {
    loading.value = false
  }
}
</script>
