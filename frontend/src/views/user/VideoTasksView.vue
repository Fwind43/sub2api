<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="flex items-center justify-between gap-3">
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">MiniMax H3</h2>
        <div class="flex items-center gap-2">
          <select
            v-model="statusFilter"
            class="input w-auto"
            :aria-label="t('videoTasks.filterGeneration')"
            @change="changeFilter()"
          >
            <option value="">{{ t('videoTasks.allGenerations') }}</option>
            <option v-for="state in generationStates" :key="state" :value="state">{{ statusLabel(state) }}</option>
          </select>
          <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadTasks()">
            <Icon name="refresh" size="sm" class="mr-2" />
            {{ t('common.refresh') }}
          </button>
        </div>
      </div>

      <p v-if="failed" role="status" class="text-sm text-red-700 dark:text-red-400">
        {{ t(tasks.length ? 'videoTasks.refreshFailed' : 'videoTasks.loadFailed') }}
      </p>

      <section class="card overflow-hidden" :aria-label="t('videoTasks.title')">
        <p v-if="loading && !tasks.length" role="status" class="p-6 text-sm text-gray-500 dark:text-dark-300">
          {{ t('common.loading') }}
        </p>
        <EmptyState v-else-if="!failed && !tasks.length" :title="t(statusFilter ? 'videoTasks.emptyFiltered' : 'videoTasks.empty')" />
        <div v-else-if="tasks.length" class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-gray-200 bg-gray-50 text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300">
              <tr>
                <th scope="col" class="min-w-48 px-4 py-3 font-medium">{{ t('videoTasks.taskId') }}</th>
                <th scope="col" class="px-4 py-3 font-medium">{{ t('videoTasks.submittedAt') }}</th>
                <th scope="col" class="px-4 py-3 font-medium">{{ t('videoTasks.generation') }}</th>
                <th scope="col" class="px-4 py-3 font-medium">{{ t('videoTasks.delivery') }}</th>
                <th scope="col" class="px-4 py-3 font-medium">{{ t('videoTasks.billing') }}</th>
                <th scope="col" class="px-4 py-3 font-medium">{{ t('videoTasks.cost') }}</th>
                <th scope="col" class="px-4 py-3 font-medium">{{ t('videoTasks.actions') }}</th>
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
                <td class="whitespace-nowrap px-4 py-3 tabular-nums">
                  <span v-if="task.cost == null" class="text-gray-400">-</span>
                  <span v-else-if="task.billing_status === 'settled'" class="font-medium text-gray-900 dark:text-white">
                    {{ formatCurrency(task.cost) }}
                  </span>
                  <span v-else-if="task.billing_status === 'pending'" class="text-gray-500 dark:text-dark-300">
                    {{ t('videoTasks.estimatedCost', { amount: formatCurrency(task.cost) }) }}
                  </span>
                  <span v-else class="text-gray-400">{{ t('videoTasks.notCharged') }}</span>
                </td>
                <td class="whitespace-nowrap px-4 py-3">
                  <div class="flex items-center gap-2">
                    <button
                      v-if="isReady(task)"
                      type="button"
                      class="btn btn-primary px-2.5 py-1.5 text-xs"
                      :disabled="downloadingId === task.id"
                      @click="downloadVideo(task)"
                    >
                      <Icon name="download" size="sm" class="mr-1" />
                      {{ downloadingId === task.id ? t('videoTasks.downloading') : t('videoTasks.downloadVideo') }}
                    </button>
                    <button
                      v-if="canRefresh(task)"
                      type="button"
                      class="btn btn-secondary px-2.5 py-1.5 text-xs"
                      :disabled="refreshingIds.has(task.id)"
                      :aria-label="t('videoTasks.refreshTask', { id: task.id })"
                      @click="refreshOne(task)"
                    >
                      <Icon name="refresh" size="sm" class="mr-1" :class="{ 'animate-spin': refreshingIds.has(task.id) }" />
                      {{ t('videoTasks.updateStatus') }}
                    </button>
                    <span v-if="!isReady(task) && !canRefresh(task)" class="text-xs text-gray-400">-</span>
                  </div>
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
import { fetchVideoTaskContent, listVideoTasks, refreshVideoTask, type VideoTask } from '@/api/videoTasks'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'
import { formatCurrency, formatDateTime } from '@/utils/format'

const { t, te } = useI18n()
const { copyToClipboard } = useClipboard()
const statusFields = ['status', 'delivery_status', 'billing_status'] as const
const pageSize = 20
const generationStates = ['failed', 'completed', 'pending', 'submitting', 'submission_unknown'] as const
const statusFilter = ref('')
const refreshMs = 15_000
const tasks = ref<VideoTask[]>([])
const page = ref(1)
const total = ref(0)
const loading = ref(true)
const failed = ref(false)
let request: AbortController | undefined
let timer: number | undefined
const appStore = useAppStore()
const refreshingIds = ref(new Set<string>())
const downloadingId = ref('')

function isReady(task: VideoTask) {
  return task.status === 'completed' && task.delivery_status === 'verified' && task.billing_status === 'settled'
}

function canRefresh(task: VideoTask) {
  return task.billing_status === 'pending' && (task.status === 'pending' || task.status === 'completed')
}

async function refreshOne(task: VideoTask) {
  if (refreshingIds.value.has(task.id)) return
  refreshingIds.value = new Set(refreshingIds.value).add(task.id)
  try {
    const updated = await refreshVideoTask(task.id)
    const index = tasks.value.findIndex(item => item.id === updated.id)
    if (index >= 0) tasks.value.splice(index, 1, updated)
  } catch {
    appStore.showError(t('videoTasks.refreshTaskFailed'))
  } finally {
    const next = new Set(refreshingIds.value)
    next.delete(task.id)
    refreshingIds.value = next
    scheduleRefresh()
  }
}

async function downloadVideo(task: VideoTask) {
  if (downloadingId.value) return
  downloadingId.value = task.id
  try {
    const blob = await fetchVideoTaskContent(task.id)
    const url = URL.createObjectURL(new Blob([blob], { type: 'video/mp4' }))
    const link = document.createElement('a')
    link.href = url
    link.download = `${task.id}.mp4`
    document.body.appendChild(link)
    link.click()
    link.remove()
    // Give the browser time to start the download before releasing the blob.
    window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
  } catch {
    appStore.showError(t('videoTasks.downloadFailed'))
  } finally {
    downloadingId.value = ''
  }
}

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
    const result = await listVideoTasks(page.value, pageSize, controller.signal, statusFilter.value)
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

function changeFilter() {
  page.value = 1
  tasks.value = []
  total.value = 0
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
