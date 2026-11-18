<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="flex items-center justify-between gap-3">
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">MiniMax H3</h2>
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadTasks()">
          <Icon name="refresh" size="sm" class="mr-2" />
          {{ t('common.refresh') }}
        </button>
      </div>

      <p v-if="failed" role="status" class="text-sm text-red-700 dark:text-red-400">
        {{ t(tasks.length ? 'videoTasks.refreshFailed' : 'videoTasks.loadFailed') }}
      </p>

      <section class="card overflow-hidden" :aria-label="t('videoTasks.title')">
        <p v-if="loading && !tasks.length" role="status" class="p-6 text-sm text-gray-500 dark:text-dark-300">
          {{ t('common.loading') }}
        </p>
        <EmptyState v-else-if="!failed && !tasks.length" :title="t('videoTasks.empty')" />
        <div v-else-if="tasks.length" class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-gray-200 bg-gray-50 text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300">
              <tr>
                <th scope="col" class="min-w-48 px-4 py-3 font-medium">{{ t('videoTasks.taskId') }}</th>
                <th scope="col" class="px-4 py-3 font-medium">{{ t('videoTasks.submittedAt') }}</th>
                <th scope="col" class="px-4 py-3 font-medium">{{ t('videoTasks.generation') }}</th>
                <th scope="col" class="px-4 py-3 font-medium">{{ t('videoTasks.delivery') }}</th>
                <th scope="col" class="px-4 py-3 font-medium">{{ t('videoTasks.billing') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="task in tasks" :key="task.id">
                <td class="px-4 py-3">
                  <div class="flex items-center gap-2">
                    <code class="break-all text-xs">{{ task.id }}</code>
                    <button
                      type="button"
                      class="btn btn-secondary shrink-0 p-1.5"
                      :aria-label="t('videoTasks.copyTaskId', { id: task.id })"
                      :title="t('common.copy')"
                      @click="copyToClipboard(task.id)"
                    >
                      <Icon name="copy" size="sm" />
                    </button>
                  </div>
                </td>
                <td class="whitespace-nowrap px-4 py-3">
                  <time :datetime="task.created_at">{{ formatDateTime(task.created_at) }}</time>
                </td>
                <td v-for="field in statusFields" :key="field" class="whitespace-nowrap px-4 py-3">
                  <span class="badge" :class="badgeFor(task[field])">{{ statusLabel(task[field]) }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="total > pageSize"
          :total="total"
          :page="page"
          :page-size="pageSize"
          :show-page-size-selector="false"
          @update:page="changePage"
        />
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { listVideoTasks, type VideoTask } from '@/api/videoTasks'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useClipboard } from '@/composables/useClipboard'
import { formatDateTime } from '@/utils/format'

const { t, te } = useI18n()
const { copyToClipboard } = useClipboard()
const statusFields = ['status', 'delivery_status', 'billing_status'] as const
const pageSize = 20
const refreshMs = 15_000
const tasks = ref<VideoTask[]>([])
const page = ref(1)
const total = ref(0)
const loading = ref(true)
const failed = ref(false)
let request: AbortController | undefined
let timer: number | undefined

function statusLabel(status: string) {
  const key = `videoTasks.states.${status}`
  return te(key) ? t(key) : t('common.unknown')
}

function badgeFor(status: string) {
  switch (status) {
    case 'completed':
    case 'verified':
    case 'settled':
      return 'badge-success'
    case 'submitting':
    case 'pending':
      return 'badge-primary'
    case 'submission_unknown':
      return 'badge-warning'
    case 'failed':
      return 'badge-danger'
    default:
      return 'badge-gray'
  }
}

function needsRefresh() {
  return tasks.value.some(task =>
    task.status === 'submitting' || task.status === 'pending' ||
    (task.status === 'completed' && task.billing_status === 'pending'),
  )
}

function scheduleRefresh() {
  window.clearTimeout(timer)
  if (!document.hidden && needsRefresh()) {
    timer = window.setTimeout(() => void loadTasks(), refreshMs)
  }
}

async function loadTasks() {
  window.clearTimeout(timer)
  request?.abort()
  const controller = new AbortController()
  request = controller
  loading.value = true
  try {
    const result = await listVideoTasks(page.value, pageSize, controller.signal)
    if (controller.signal.aborted) return
    tasks.value = result.items
    total.value = result.total
    failed.value = false
  } catch {
    if (!controller.signal.aborted) failed.value = true
  } finally {
    if (request === controller) {
      request = undefined
      loading.value = false
      scheduleRefresh()
    }
  }
}

function changePage(next: number) {
  if (next === page.value) return
  page.value = next
  tasks.value = []
  failed.value = false
  void loadTasks()
}

function visibilityChanged() {
  window.clearTimeout(timer)
  if (!document.hidden && !request && needsRefresh()) void loadTasks()
}

onMounted(() => {
  document.addEventListener('visibilitychange', visibilityChanged)
  void loadTasks()
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', visibilityChanged)
  request?.abort()
  request = undefined
  window.clearTimeout(timer)
})
</script>
