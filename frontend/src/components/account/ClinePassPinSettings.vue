<template>
  <div class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="clinepass-pin-settings">
    <div class="flex items-center justify-between gap-4">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.clinepassPin.title') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.clinepassPin.desc') }}
        </p>
      </div>
      <div class="w-44 flex-shrink-0">
        <Select
          :model-value="mode"
          :options="modeOptions"
          data-testid="clinepass-pin-mode"
          @update:model-value="onModeChange"
        />
      </div>
    </div>

    <template v-if="mode !== 'off'">
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <label class="input-label">{{ t('admin.accounts.clinepassPin.upstream') }}</label>
          <input
            v-model="upstream"
            type="text"
            class="input"
            :placeholder="t('admin.accounts.clinepassPin.upstreamPlaceholder')"
            data-testid="clinepass-pin-upstream"
            @input="emitChange"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.clinepassPin.pipelines') }}</label>
          <Select
            :model-value="pipelines"
            :options="pipelineOptions"
            data-testid="clinepass-pin-pipelines"
            @update:model-value="onPipelinesChange"
          />
        </div>
      </div>

      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <label class="input-label">{{ t('admin.accounts.clinepassPin.sort') }}</label>
          <Select
            :model-value="sort"
            :options="sortOptions"
            data-testid="clinepass-pin-sort"
            @update:model-value="onSortChange"
          />
        </div>
      </div>

      <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700">
        <div class="flex items-end gap-2">
          <div class="min-w-0 flex-1">
            <label class="input-label">{{ t('admin.accounts.clinepassPin.probeModel') }}</label>
            <input
              v-model="probeModel"
              type="text"
              class="input"
              :placeholder="t('admin.accounts.clinepassPin.probeModelPlaceholder')"
              data-testid="clinepass-pin-probe-model"
            />
          </div>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="probing || !probeModel.trim()"
            data-testid="clinepass-pin-probe-run"
            @click="runProbe"
          >
            {{ probing ? t('admin.accounts.clinepassPin.probing') : t('admin.accounts.clinepassPin.probeRun') }}
          </button>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.clinepassPin.probeHint') }}
        </p>
        <p v-if="probeError" class="mt-2 text-xs text-red-600 dark:text-red-400" data-testid="clinepass-pin-probe-error">
          {{ probeError }}
        </p>
        <div v-if="probeResult" class="mt-2 space-y-2" data-testid="clinepass-pin-probe-result">
          <div v-if="probeResult.vercel?.length" class="flex flex-wrap items-center gap-1.5">
            <span class="text-xs font-medium text-gray-600 dark:text-gray-300">
              {{ t('admin.accounts.clinepassPin.pipelineVercel') }}:
            </span>
            <button
              v-for="slug in probeResult.vercel"
              :key="`vercel-${slug}`"
              type="button"
              class="rounded border border-gray-300 px-1.5 py-0.5 text-xs text-gray-700 hover:bg-gray-100 dark:border-dark-500 dark:text-gray-200 dark:hover:bg-dark-600"
              @click="pickUpstream(slug)"
            >
              {{ slug }}
            </button>
          </div>
          <div v-if="probeResult.openrouter?.length" class="flex flex-wrap items-center gap-1.5">
            <span class="text-xs font-medium text-gray-600 dark:text-gray-300">
              {{ t('admin.accounts.clinepassPin.pipelineOpenrouter') }}:
            </span>
            <button
              v-for="slug in probeResult.openrouter"
              :key="`openrouter-${slug}`"
              type="button"
              class="rounded border border-gray-300 px-1.5 py-0.5 text-xs text-gray-700 hover:bg-gray-100 dark:border-dark-500 dark:text-gray-200 dark:hover:bg-dark-600"
              @click="pickUpstream(slug)"
            >
              {{ slug }}
            </button>
          </div>
          <p
            v-if="!probeResult.vercel?.length && !probeResult.openrouter?.length"
            class="text-xs text-gray-500 dark:text-gray-400"
          >
            {{ t('admin.accounts.clinepassPin.probeEmpty') }}
          </p>
        </div>
      </div>

      <div>
        <div class="flex items-center justify-between">
          <label class="input-label mb-0">{{ t('admin.accounts.clinepassPin.models') }}</label>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            data-testid="clinepass-pin-model-add"
            @click="addModelRow"
          >
            {{ t('admin.accounts.clinepassPin.modelsAdd') }}
          </button>
        </div>
        <p v-if="!modelRows.length" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.clinepassPin.modelsEmpty') }}
        </p>
        <div
          v-for="(row, idx) in modelRows"
          :key="row.key"
          class="mt-2 flex items-center gap-2"
          data-testid="clinepass-pin-model-row"
        >
          <input
            v-model="row.model"
            type="text"
            class="input"
            :placeholder="t('admin.accounts.clinepassPin.modelNamePlaceholder')"
            @input="emitChange"
          />
          <input
            v-model="row.upstream"
            type="text"
            class="input"
            :placeholder="t('admin.accounts.clinepassPin.modelUpstreamPlaceholder')"
            @input="emitChange"
          />
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :aria-label="t('admin.accounts.clinepassPin.modelsRemove')"
            @click="removeModelRow(idx)"
          >
            &times;
          </button>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import { probeUpstreams, type ClinePassUpstreamProbeResult } from '@/api/admin/clinepass'

type PinMode = 'off' | 'strict' | 'preferred'
type PinPipelines = 'both' | 'vercel' | 'openrouter'
type PinSort = '' | 'cost' | 'ttft' | 'tps'

interface ModelRow {
  key: number
  model: string
  upstream: string
}

const props = defineProps<{
  accountId: number
  modelValue: Record<string, unknown> | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: Record<string, unknown> | null): void
}>()

const { t } = useI18n()

const modeOptions = computed(() => [
  { value: 'off', label: t('admin.accounts.clinepassPin.modes.off') },
  { value: 'strict', label: t('admin.accounts.clinepassPin.modes.strict') },
  { value: 'preferred', label: t('admin.accounts.clinepassPin.modes.preferred') }
])

const pipelineOptions = computed(() => [
  { value: 'both', label: t('admin.accounts.clinepassPin.pipelineBoth') },
  { value: 'vercel', label: t('admin.accounts.clinepassPin.pipelineVercel') },
  { value: 'openrouter', label: t('admin.accounts.clinepassPin.pipelineOpenrouter') }
])

const sortOptions = computed(() => [
  { value: '', label: t('admin.accounts.clinepassPin.sortNone') },
  { value: 'cost', label: t('admin.accounts.clinepassPin.sortCost') },
  { value: 'ttft', label: t('admin.accounts.clinepassPin.sortTtft') },
  { value: 'tps', label: t('admin.accounts.clinepassPin.sortTps') }
])

const mode = ref<PinMode>('off')
const upstream = ref('')
const pipelines = ref<PinPipelines>('both')
const sort = ref<PinSort>('')
const modelRows = ref<ModelRow[]>([])
let rowKeySeq = 1

const probeModel = ref('')
const probing = ref(false)
const probeResult = ref<ClinePassUpstreamProbeResult | null>(null)
const probeError = ref('')

const readMode = (v: Record<string, unknown>): PinMode => {
  const m = typeof v.mode === 'string' ? v.mode : ''
  if (m === 'off') return 'off'
  if (m === 'preferred') return 'preferred'
  return Object.keys(v).length > 0 ? 'strict' : 'off'
}

const readModelRows = (v: Record<string, unknown>): ModelRow[] => {
  const models =
    v.models && typeof v.models === 'object' && !Array.isArray(v.models)
      ? (v.models as Record<string, unknown>)
      : {}
  const rows: ModelRow[] = []
  for (const [key, val] of Object.entries(models)) {
    const item = val && typeof val === 'object' && !Array.isArray(val) ? (val as Record<string, unknown>) : {}
    rows.push({
      key: rowKeySeq++,
      model: key,
      upstream: typeof item.upstream === 'string' ? item.upstream : ''
    })
  }
  return rows
}

const buildValue = (): Record<string, unknown> | null => {
  if (mode.value === 'off') return null
  const out: Record<string, unknown> = { mode: mode.value }
  if (upstream.value.trim()) out.upstream = upstream.value.trim()
  if (pipelines.value !== 'both') out.pipelines = pipelines.value
  if (sort.value) out.sort = sort.value
  const models: Record<string, unknown> = {}
  for (const row of modelRows.value) {
    const name = row.model.trim()
    if (!name) continue
    const item: Record<string, unknown> = {}
    if (row.upstream.trim()) item.upstream = row.upstream.trim()
    if (Object.keys(item).length) models[name] = item
  }
  if (Object.keys(models).length) out.models = models
  return out
}

const stateSignature = (v: Record<string, unknown> | null): string => {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return 'off||both||[]'
  const m = readMode(v)
  const up = typeof v.upstream === 'string' ? v.upstream.trim() : ''
  const p = v.pipelines === 'vercel' || v.pipelines === 'openrouter' ? (v.pipelines as string) : 'both'
  const s = v.sort === 'cost' || v.sort === 'ttft' || v.sort === 'tps' ? (v.sort as string) : ''
  const rows = readModelRows(v).map((r) => ({ m: r.model, u: r.upstream }))
  return [m, up, p, s, JSON.stringify(rows)].join('|')
}

const currentSignature = (): string => {
  return [
    mode.value,
    upstream.value.trim(),
    pipelines.value,
    sort.value,
    JSON.stringify(modelRows.value.map((r) => ({ m: r.model, u: r.upstream })))
  ].join('|')
}

const hydrate = (value: Record<string, unknown> | null | undefined) => {
  const incoming =
    value && typeof value === 'object' && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : null
  if (stateSignature(incoming) === currentSignature()) return
  const v = incoming ?? {}
  mode.value = readMode(v)
  upstream.value = typeof v.upstream === 'string' ? v.upstream : ''
  pipelines.value = v.pipelines === 'vercel' || v.pipelines === 'openrouter' ? (v.pipelines as PinPipelines) : 'both'
  sort.value = v.sort === 'cost' || v.sort === 'ttft' || v.sort === 'tps' ? (v.sort as PinSort) : ''
  modelRows.value = readModelRows(v)
}

watch(() => props.modelValue, hydrate, { immediate: true })

const emitChange = () => {
  emit('update:modelValue', buildValue())
}

const normalizeMode = (v: unknown): PinMode => (v === 'off' || v === 'strict' || v === 'preferred' ? v : 'off')

const onModeChange = (v: unknown) => {
  mode.value = normalizeMode(v)
  emitChange()
}

const onPipelinesChange = (v: unknown) => {
  pipelines.value = v === 'vercel' || v === 'openrouter' ? v : 'both'
  emitChange()
}

const onSortChange = (v: unknown) => {
  sort.value = v === 'cost' || v === 'ttft' || v === 'tps' ? v : ''
  emitChange()
}

const pickUpstream = (slug: string) => {
  upstream.value = slug
  emitChange()
}

const addModelRow = () => {
  modelRows.value.push({ key: rowKeySeq++, model: '', upstream: '' })
  emitChange()
}

const removeModelRow = (index: number) => {
  modelRows.value.splice(index, 1)
  emitChange()
}

const runProbe = async () => {
  const model = probeModel.value.trim()
  if (!model || probing.value) return
  probing.value = true
  probeError.value = ''
  try {
    probeResult.value = await probeUpstreams(props.accountId, model)
  } catch (error: unknown) {
    probeResult.value = null
    const message = error instanceof Error ? error.message : String(error)
    probeError.value = message
  } finally {
    probing.value = false
  }
}
</script>
