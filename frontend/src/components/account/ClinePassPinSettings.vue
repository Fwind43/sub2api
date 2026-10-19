<template>
  <div class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="clinepass-routing-status">
    <p class="text-xs text-gray-500 dark:text-gray-400" data-testid="clinepass-platform-only">
      ClinePass 上游钉定统一以平台设置为准，账号级配置已移除。下方仅展示实际命中与平台规则的对照。
    </p>
    <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="clinepass-lastknown">
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.clinepassPin.lastKnownTitle') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.clinepassPin.lastKnownDesc') }}
          </p>
        </div>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="lastKnownLoading"
          data-testid="clinepass-lastknown-refresh"
          @click="loadLastKnown"
        >
          {{ lastKnownLoading ? t('admin.accounts.clinepassPin.lastKnownLoading') : t('admin.accounts.clinepassPin.lastKnownRefresh') }}
        </button>
      </div>
      <p v-if="lastKnownError" class="mt-2 text-xs text-red-600 dark:text-red-400" data-testid="clinepass-lastknown-error">
        {{ lastKnownError }}
      </p>
      <p
        v-if="!lastKnownLoading && !lastKnown.length && !lastKnownError"
        class="mt-2 text-xs text-gray-500 dark:text-gray-400"
      >
        {{ t('admin.accounts.clinepassPin.lastKnownEmpty') }}
      </p>
      <div
        v-for="item in lastKnown"
        :key="item.model"
        class="mt-2 rounded border border-gray-100 p-2 text-xs dark:border-dark-600"
        data-testid="clinepass-lastknown-row"
      >
        <div class="flex flex-wrap items-center gap-2">
          <span class="font-medium text-gray-700 dark:text-gray-200">{{ item.model }}</span>
          <span class="rounded bg-gray-100 px-1.5 py-0.5 text-gray-700 dark:bg-dark-600 dark:text-gray-200">
            {{ item.provider }}
          </span>
          <span v-if="item.pipeline" class="text-gray-500 dark:text-gray-400">{{ pipelineLabel(item.pipeline) }}</span>
          <span class="text-gray-400">{{ formatObserved(item.observed_at) }}</span>
        </div>
        <div class="mt-1 flex flex-wrap items-center gap-2">
          <span class="text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.clinepassPin.lastKnownExpected') }}:
            {{ expectedForModel(item.model) || t('admin.accounts.clinepassPin.lastKnownNone') }}
          </span>
          <span
            v-if="expectedForModel(item.model)"
            :class="isMatch(item) ? 'text-green-600 dark:text-green-400' : 'text-amber-600 dark:text-amber-400'"
            data-testid="clinepass-lastknown-verdict"
          >
            {{ isMatch(item) ? t('admin.accounts.clinepassPin.lastKnownMatch') : t('admin.accounts.clinepassPin.lastKnownMismatch') }}
          </span>
        </div>
        <p v-if="item.fallbacks?.length" class="mt-1 text-gray-400">{{ item.fallbacks.join(', ') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getUpstreamPin, listLastKnown, type ClinePassLastKnownItem, type ClinePassUpstreamPinConfig } from '@/api/admin/clinepass'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{ accountId: number }>()
const { t } = useI18n()
const platformPin = ref<ClinePassUpstreamPinConfig | null>(null)
const lastKnown = ref<ClinePassLastKnownItem[]>([])
const lastKnownLoading = ref(false)
const lastKnownError = ref('')
let loadVersion = 0

const modelBase = (model: string): string => model.trim().split('/').pop() || model.trim()
const expectedProviders = (model: string): string[] => {
  const pin = platformPin.value
  if (!pin || pin.mode === 'off') return []
  const id = model.trim()
  const base = modelBase(id).toLowerCase()
  if (id.toLowerCase().startsWith('cline-free') || base.startsWith('cline-free')) return []
  const override = Object.entries(pin.models || {}).find(([key]) => key.trim() === id)?.[1]
    || Object.entries(pin.models || {}).find(([key]) => modelBase(key) === modelBase(id))?.[1]
  // Like the backend, only non-empty override fields replace inherited values.
  const config = { ...pin, ...Object.fromEntries(Object.entries(override || {}).filter(([, value]) =>
    Array.isArray(value) ? value.length > 0 : typeof value === 'string' && value !== ''
  )) } as ClinePassUpstreamPinConfig
  if (config.mode === 'off') return []
  const prefixes = config.match_prefixes?.map(p => p.trim().toLowerCase()).filter(Boolean) || []
  if (prefixes.length && !prefixes.some(prefix => base.startsWith(prefix))) return []
  const upstream = config.upstream?.trim()
  // Strict upstream wins over an explicit list, matching outbound injection.
  if (config.mode !== 'preferred' && upstream) return [upstream]
  if (config.only?.length) return config.only
  if (config.exclude?.length && config.upstreams?.length) {
    return config.upstreams.filter(provider => provider === upstream || !config.exclude?.includes(provider))
  }
  return upstream ? [upstream] : []
}
const expectedForModel = (model: string): string => expectedProviders(model).join(', ')
const isMatch = (item: ClinePassLastKnownItem): boolean =>
  expectedProviders(item.model).some(provider => provider.toLowerCase() === item.provider.trim().toLowerCase())
const pipelineLabel = (value: string): string =>
  value === 'planner' ? t('admin.accounts.clinepassPin.lastKnownPipelinePlanner')
    : value === 'direct' ? t('admin.accounts.clinepassPin.lastKnownPipelineDirect') : value
const formatObserved = (value: string): string => formatDateTime(value)

const loadLastKnown = async () => {
  const version = ++loadVersion
  if (!props.accountId) {
    lastKnown.value = []
    platformPin.value = null
    lastKnownError.value = ''
    lastKnownLoading.value = false
    return
  }
  lastKnownLoading.value = true
  lastKnownError.value = ''
  lastKnown.value = []
  platformPin.value = null
  try {
    const [observations, platform] = await Promise.all([listLastKnown(props.accountId), getUpstreamPin()])
    if (version !== loadVersion) return
    lastKnown.value = observations.items || []
    platformPin.value = platform.config || null
  } catch (error: unknown) {
    if (version !== loadVersion) return
    lastKnownError.value = error instanceof Error ? error.message : String(error)
  } finally {
    if (version === loadVersion) lastKnownLoading.value = false
  }
}
watch(() => props.accountId, () => { void loadLastKnown() }, { immediate: true })
</script>
