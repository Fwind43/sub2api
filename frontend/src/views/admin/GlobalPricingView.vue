<template>
  <div class="space-y-4">
    <!-- Header -->
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('admin.globalPricing.title') }}
        </h2>
        <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-dark-300">
          {{ t('admin.globalPricing.description') }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <button type="button" class="btn btn-secondary" :disabled="reloading" @click="reloadCatalog">
          <Icon name="refresh" size="sm" class="mr-1.5" :class="reloading ? 'animate-spin' : ''" />
          {{ t('admin.globalPricing.reloadCatalog') }}
        </button>
        <button type="button" class="btn btn-primary" @click="openCreate">
          <Icon name="plus" size="sm" class="mr-1.5" />
          {{ t('admin.globalPricing.addModel') }}
        </button>
      </div>
    </div>

    <!-- Search -->
    <div class="relative max-w-sm">
      <span class="pointer-events-none absolute inset-y-0 left-3 flex items-center text-gray-400">
        <Icon name="search" size="sm" />
      </span>
      <input
        v-model="keyword"
        type="text"
        class="w-full rounded-lg border border-gray-300 bg-white py-2 pl-9 pr-3 text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-800 dark:text-white"
        :placeholder="t('admin.globalPricing.searchPlaceholder')"
      />
    </div>

    <!-- Table -->
    <div class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800">
      <div v-if="loading" class="p-8 text-center text-sm text-gray-500 dark:text-dark-300">
        <Icon name="refresh" size="md" class="mx-auto animate-spin" />
      </div>

      <div v-else-if="filteredModels.length === 0" class="p-10 text-center">
        <Icon name="dollar" size="xl" class="mx-auto text-gray-300 dark:text-dark-500" />
        <p class="mt-3 text-sm font-medium text-gray-700 dark:text-gray-200">
          {{ t('admin.globalPricing.empty') }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-300">
          {{ t('admin.globalPricing.emptyHint') }}
        </p>
      </div>

      <table v-else class="min-w-full divide-y divide-gray-200 dark:divide-dark-600">
        <thead class="bg-gray-50 dark:bg-dark-900">
          <tr>
            <th v-for="col in columns" :key="col" class="px-4 py-2.5 text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-300">
              {{ t(`admin.globalPricing.columns.${col}`) }}
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="row in filteredModels" :key="row.model" class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
            <td class="max-w-[22rem] truncate px-4 py-2.5 font-mono text-sm text-gray-900 dark:text-white" :title="row.model">
              {{ row.model }}
            </td>
            <td
              v-for="field in TABLE_PRICE_FIELDS"
              :key="field.json"
              class="whitespace-nowrap px-4 py-2.5 text-right text-sm tabular-nums text-gray-700 dark:text-gray-200"
            >
              {{ formatPrice(row.pricing[field.json]) }}
            </td>
            <td class="whitespace-nowrap px-4 py-2.5 text-right">
              <button type="button" class="btn btn-ghost btn-sm" :title="t('admin.globalPricing.edit')" @click="openEdit(row)">
                <Icon name="edit" size="sm" />
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-sm text-red-600 hover:text-red-700"
                :title="t('admin.globalPricing.delete')"
                @click="deleteTarget = row"
              >
                <Icon name="trash" size="sm" />
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Edit / create dialog -->
    <BaseDialog
      :show="dialogOpen"
      :title="editing ? t('admin.globalPricing.editTitle') : t('admin.globalPricing.addTitle')"
      width="wide"
      @close="closeDialog"
    >
      <div class="space-y-4">
        <div>
          <label class="input-label mb-1.5 block">{{ t('admin.globalPricing.model') }}</label>
          <input
            v-model="form.model"
            type="text"
            :disabled="editing"
            class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 font-mono text-sm text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 disabled:bg-gray-100 disabled:text-gray-500 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
            :placeholder="t('admin.globalPricing.modelPlaceholder')"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-300">{{ t('admin.globalPricing.modelHint') }}</p>
        </div>

        <div>
          <label class="input-label mb-1.5 block">{{ t('admin.globalPricing.mode') }}</label>
          <Select v-model="form.mode" :options="modeOptions" />
        </div>

        <p class="rounded-lg bg-blue-50 px-3 py-2 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">
          {{ t('admin.globalPricing.unitHint') }}
        </p>

        <div>
          <h4 class="mb-2 text-sm font-semibold text-gray-800 dark:text-gray-100">
            {{ t('admin.globalPricing.sectionBasic') }}
          </h4>
          <div class="grid grid-cols-2 gap-3">
            <Input v-model="form.input_price" type="number" :label="t('admin.globalPricing.priceInput')" placeholder="0" />
            <Input v-model="form.output_price" type="number" :label="t('admin.globalPricing.priceOutput')" placeholder="0" />
          </div>
        </div>

        <details class="rounded-lg border border-gray-200 dark:border-dark-600">
          <summary class="cursor-pointer select-none px-3 py-2 text-sm font-medium text-gray-700 dark:text-gray-200">
            {{ t('admin.globalPricing.sectionAdvanced') }}
          </summary>
          <div class="grid grid-cols-2 gap-3 border-t border-gray-200 p-3 dark:border-dark-600">
            <Input v-model="form.cache_write_price" type="number" :label="t('admin.globalPricing.priceCacheWrite')" placeholder="0" />
            <Input v-model="form.cache_write_1h_price" type="number" :label="t('admin.globalPricing.priceCacheWrite1h')" placeholder="0" />
            <Input v-model="form.cache_read_price" type="number" :label="t('admin.globalPricing.priceCacheRead')" placeholder="0" />
            <Input v-model="form.image_input_price" type="number" :label="t('admin.globalPricing.priceImageInput')" placeholder="0" />
            <Input v-model="form.image_output_price" type="number" :label="t('admin.globalPricing.priceImageOutput')" placeholder="0" />
          </div>
        </details>
      </div>

      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeDialog">
          {{ t('admin.globalPricing.cancel') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="saving" @click="submit">
          <Icon v-if="saving" name="refresh" size="sm" class="mr-1.5 animate-spin" />
          {{ t('admin.globalPricing.save') }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="deleteTarget !== null"
      :title="t('admin.globalPricing.deleteConfirmTitle')"
      :message="deleteMessage"
      @confirm="confirmDelete"
      @cancel="deleteTarget = null"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Input from '@/components/common/Input.vue'
import Select from '@/components/common/Select.vue'
import globalPricingAPI from '@/api/admin/globalPricing'
import { mTokToPerToken, perTokenToMTok } from '@/components/admin/channel/types'

const { t } = useI18n()
const appStore = useAppStore()

interface PricingRow {
  model: string
  pricing: Record<string, unknown>
}

interface EditForm {
  model: string
  mode: string
  input_price: string | number
  output_price: string | number
  cache_write_price: string | number
  cache_write_1h_price: string | number
  cache_read_price: string | number
  image_input_price: string | number
  image_output_price: string | number
}

/** 表单字段 ←→ LiteLLM 目录字段。API 存 per-token，界面用 $/MTok。 */
const PRICE_FIELDS = [
  { key: 'input_price', json: 'input_cost_per_token' },
  { key: 'output_price', json: 'output_cost_per_token' },
  { key: 'cache_write_price', json: 'cache_creation_input_token_cost' },
  { key: 'cache_write_1h_price', json: 'cache_creation_input_token_cost_above_1hr' },
  { key: 'cache_read_price', json: 'cache_read_input_token_cost' },
  { key: 'image_input_price', json: 'input_cost_per_image_token' },
  { key: 'image_output_price', json: 'output_cost_per_image_token' }
] as const

const TABLE_PRICE_FIELDS = PRICE_FIELDS.filter(field => !field.json.includes('_above_1hr'))
const columns = ['model', ...['input', 'output', 'cacheWrite', 'cacheRead'], 'actions']

const loading = ref(false)
const reloading = ref(false)
const saving = ref(false)
const keyword = ref('')
const rows = ref<PricingRow[]>([])
const dialogOpen = ref(false)
const editing = ref(false)
const deleteTarget = ref<PricingRow | null>(null)

const form = reactive<EditForm>({
  model: '',
  mode: '',
  input_price: '',
  output_price: '',
  cache_write_price: '',
  cache_write_1h_price: '',
  cache_read_price: '',
  image_input_price: '',
  image_output_price: ''
})

const modeOptions = computed(() => [
  { value: '', label: t('admin.globalPricing.modeUnset') },
  { value: 'chat', label: t('admin.globalPricing.modeChat') },
  { value: 'image_generation', label: t('admin.globalPricing.modeImage') }
])

const filteredModels = computed(() => {
  const term = keyword.value.trim().toLowerCase()
  if (!term) return rows.value
  return rows.value.filter(row => row.model.toLowerCase().includes(term))
})

const deleteMessage = computed(() =>
  t('admin.globalPricing.deleteConfirmMessage', { model: deleteTarget.value?.model ?? '' })
)

function formatPrice(value: unknown): string {
  if (typeof value !== 'number') return t('admin.globalPricing.notConfigured')
  const perMTok = perTokenToMTok(value)
  if (perMTok === null) return t('admin.globalPricing.notConfigured')
  return `$${perMTok}`
}

function readPrice(pricing: Record<string, unknown>, json: string): string {
  const value = pricing[json]
  if (typeof value !== 'number') return ''
  const perMTok = perTokenToMTok(value)
  return perMTok === null ? '' : String(perMTok)
}

async function load() {
  loading.value = true
  try {
    const result = await globalPricingAPI.list()
    rows.value = Object.entries(result.entries ?? {})
      .map(([model, pricing]) => ({ model, pricing: (pricing ?? {}) as Record<string, unknown> }))
      .sort((a, b) => a.model.localeCompare(b.model))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.globalPricing.loadError')))
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.model = ''
  form.mode = ''
  for (const field of PRICE_FIELDS) form[field.key] = ''
}

function openCreate() {
  resetForm()
  editing.value = false
  dialogOpen.value = true
}

function openEdit(row: PricingRow) {
  resetForm()
  form.model = row.model
  const mode = row.pricing.mode
  form.mode = typeof mode === 'string' ? mode : ''
  for (const field of PRICE_FIELDS) form[field.key] = readPrice(row.pricing, field.json)
  editing.value = true
  dialogOpen.value = true
}

function closeDialog() {
  dialogOpen.value = false
}

/** 组装 PUT 请求体；校验失败时返回错误文案。 */
function buildPayload(): Record<string, unknown> | string {
  const payload: Record<string, unknown> = {}
  for (const field of PRICE_FIELDS) {
    const raw = form[field.key]
    if (raw === '' || raw === null || raw === undefined) continue
    const num = Number(raw)
    if (!Number.isFinite(num) || num < 0) return t('admin.globalPricing.priceInvalid')
    payload[field.json] = mTokToPerToken(num)
  }
  if (form.mode !== '') payload.mode = form.mode
  return payload
}

async function submit() {
  const model = form.model.trim()
  if (!model) {
    appStore.showError(t('admin.globalPricing.modelRequired'))
    return
  }
  const payload = buildPayload()
  if (typeof payload === 'string') {
    appStore.showError(payload)
    return
  }
  if (Object.keys(payload).length === 0) {
    appStore.showError(t('admin.globalPricing.priceInvalid'))
    return
  }
  saving.value = true
  try {
    await globalPricingAPI.save(model, payload)
    appStore.showSuccess(t('admin.globalPricing.saved', { model }))
    dialogOpen.value = false
    await load()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.globalPricing.saveError')))
  } finally {
    saving.value = false
  }
}

async function confirmDelete() {
  const row = deleteTarget.value
  if (!row) return
  deleteTarget.value = null
  try {
    await globalPricingAPI.remove(row.model)
    appStore.showSuccess(t('admin.globalPricing.deleted', { model: row.model }))
    await load()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.globalPricing.deleteError')))
  }
}

async function reloadCatalog() {
  reloading.value = true
  try {
    await globalPricingAPI.reload()
    appStore.showSuccess(t('admin.globalPricing.reloaded'))
    await load()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.globalPricing.reloadError')))
  } finally {
    reloading.value = false
  }
}

onMounted(load)
</script>
