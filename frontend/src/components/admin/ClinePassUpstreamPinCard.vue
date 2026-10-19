<template>
  <div class="card" data-testid="clinepass-platform-pin">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">ClinePass 上游钉定（平台级）</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        所有 ClinePass 账号统一以此处为准。每个模型可以固定到不同渠道；strict 只允许指定渠道，不自动换渠道，preferred 仅表达偏好。
      </p>
    </div>
    <div class="space-y-5 px-6 py-4">
      <p v-if="loading" class="text-sm text-gray-500">加载中…</p>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400" data-testid="pin-error">{{ error }}</p>
      <fieldset :disabled="loading || saving || !loaded" class="space-y-5">
        <div class="grid gap-4 md:grid-cols-3">
          <label class="block text-sm">
            <span class="input-label">平台开关 / 默认模式</span>
            <select v-model="mode" class="input w-full" data-testid="pin-mode">
              <option value="strict">strict（固定渠道）</option>
              <option value="preferred">preferred（偏好渠道）</option>
              <option value="off">off（关闭全部钉定）</option>
            </select>
          </label>
          <label class="block text-sm">
            <span class="input-label">默认上游（可留空，仅使用模型规则）</span>
            <input v-model="upstream" class="input w-full" placeholder="例如 deepseek" data-testid="pin-upstream" />
          </label>
          <label class="block text-sm">
            <span class="input-label">默认模型前缀（逗号分隔）</span>
            <input v-model="matchPrefixesText" class="input w-full" placeholder="例如 deepseek，留空匹配全部" data-testid="pin-prefixes" />
          </label>
        </div>
        <p class="text-xs text-gray-500 dark:text-gray-400">
          默认规则只作用于命中前缀的模型；下方精确模型规则优先于默认规则，不受默认前缀限制。未配置的模型保持原路由；cline-free 模型不会钉定。已有高级路由字段会保留。
        </p>
        <section class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 class="font-medium">逐模型固定渠道</h3>
              <p class="mt-1 text-xs text-gray-500">填写完整模型 ID（可带 cline-pass/），添加多行可为多个模型分别指定渠道。</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" data-testid="pin-add-model" @click="addRow">添加模型</button>
          </div>
          <div class="flex flex-wrap items-end gap-3 rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
            <label class="min-w-0 flex-1 text-sm">
              <span class="input-label">用于探测的 ClinePass 账号（不修改账号配置）</span>
              <select v-model="probeAccountId" class="input w-full" :disabled="probing" data-testid="pin-probe-account">
                <option :value="0">请选择账号</option>
                <option v-for="account in accounts" :key="account.id" :value="account.id">{{ account.name }} (#{{ account.id }})</option>
              </select>
            </label>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="accountsLoading || probing" data-testid="pin-accounts-refresh" @click="loadAccounts">
              {{ accountsLoading ? '加载账号…' : '刷新账号' }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="!probeAccountId || probing || !rows.length" data-testid="pin-probe-all" @click="probeAll">
              {{ probing ? '探测中…' : '探测全部模型' }}
            </button>
            <p class="w-full text-xs text-gray-500">
              探测两条管线的可用渠道，不保存配置，也不代表钉定已被上游执行。结果取决于所选账号与模型映射；点击渠道填入后，仍需保存。探测会发送上游请求。
            </p>
            <p v-if="accountsError" role="alert" class="w-full text-xs text-red-600" data-testid="pin-accounts-error">{{ accountsError }}</p>
          </div>
          <p v-if="!rows.length" class="text-sm text-gray-500">尚无模型规则；可添加多个模型分别固定渠道。</p>
          <div v-for="row in rows" :key="row.key" class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="pin-model-row">
            <div class="grid items-end gap-3 md:grid-cols-12">
              <label class="block text-sm md:col-span-4">
                <span class="input-label">模型 ID</span>
                <input v-model="row.model" class="input w-full" placeholder="例如 deepseek-v4.1-flash" data-testid="pin-model-id" @input="clearProbe(row)" />
              </label>
              <label class="block text-sm md:col-span-3">
                <span class="input-label">固定上游渠道</span>
                <input v-model="row.upstream" class="input w-full" placeholder="输入或从探测结果选择" data-testid="pin-model-upstream" />
              </label>
              <label class="block text-sm md:col-span-2">
                <span class="input-label">模式</span>
                <select v-model="row.mode" class="input w-full" data-testid="pin-model-mode">
                  <option value="">继承默认</option>
                  <option value="strict">strict（固定）</option>
                  <option value="preferred">preferred（偏好）</option>
                  <option value="off">off（不钉定）</option>
                </select>
              </label>
              <label class="block text-sm md:col-span-3">
                <span class="input-label">生效管线</span>
                <select v-model="row.pipelines" class="input w-full" data-testid="pin-model-pipelines">
                  <option value="">继承默认</option>
                  <option value="both">两条管线</option>
                  <option value="vercel">Vercel AI Gateway</option>
                  <option value="openrouter">OpenRouter</option>
                </select>
              </label>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <button type="button" class="btn btn-secondary btn-sm" :disabled="!probeAccountId || !row.model.trim() || probing" data-testid="pin-model-probe" @click="probeOne(row)">探测渠道</button>
              <button type="button" class="btn btn-secondary btn-sm" data-testid="pin-model-remove" @click="removeRow(row)">移除规则</button>
              <span v-if="row.probing" class="text-xs text-gray-500">正在探测两条管线…</span>
              <span v-if="row.probeModel" class="break-all text-xs text-gray-500">探测实际模型：{{ row.probeModel }}</span>
            </div>
            <p v-if="row.probeError" role="alert" class="text-xs text-red-600" data-testid="pin-model-probe-error">{{ row.probeError }}</p>
            <div v-if="row.result" class="space-y-2 text-xs" data-testid="pin-model-probe-result">
              <div v-for="pipeline in pipelineOptions" :key="pipeline.key" class="flex flex-wrap items-center gap-2">
                <span class="font-medium">{{ pipeline.label }}：</span>
                <button v-for="provider in row.result[pipeline.key]" :key="provider" type="button" class="rounded border border-blue-200 px-2 py-1 text-blue-600 dark:border-blue-800 dark:text-blue-300" :aria-pressed="row.upstream === provider" data-testid="pin-provider-option" @click="chooseProvider(row, provider)">{{ provider }}</button>
                <span v-if="!row.result[pipeline.key]?.length" class="text-gray-500">未取得可用渠道</span>
              </div>
              <details v-if="row.result.vercel_raw || row.result.openrouter_raw" class="break-all text-gray-500">
                <summary class="cursor-pointer">探测原始返回 / 诊断</summary>
                <pre class="mt-2 whitespace-pre-wrap">{{ row.result.vercel_raw }}
{{ row.result.openrouter_raw }}</pre>
              </details>
            </div>
          </div>
        </section>
        <p v-if="mode === 'off'" class="text-sm text-amber-600">保存后关闭全部平台钉定，并清除已保存的模型规则。</p>
        <div class="flex flex-wrap items-center gap-3">
          <button type="button" class="btn btn-primary btn-sm" :disabled="saving || probing" data-testid="pin-save" @click="save">{{ saving ? '保存中…' : '保存配置' }}</button>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="saving || probing" data-testid="pin-reload" @click="load">重新加载</button>
          <span v-if="savedMsg" role="status" class="text-sm text-green-600" data-testid="pin-saved">{{ savedMsg }}</span>
        </div>
      </fieldset>
      <button v-if="!loaded && !loading" type="button" class="btn btn-secondary btn-sm" @click="load">重试加载</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { list as listAccounts } from '@/api/admin/accounts'
import {
  getUpstreamPin, probeUpstreams, updateUpstreamPin,
  type ClinePassUpstreamPinConfig, type ClinePassUpstreamProbeResult
} from '@/api/admin/clinepass'

type PinMode = NonNullable<ClinePassUpstreamPinConfig['mode']>
type Pipelines = NonNullable<ClinePassUpstreamPinConfig['pipelines']>
interface ModelRow {
  key: number
  model: string
  upstream: string
  mode: PinMode | ''
  pipelines: Pipelines | ''
  original: ClinePassUpstreamPinConfig
  probing: boolean
  probeVersion: number
  probeModel: string
  probeError: string
  result: ClinePassUpstreamProbeResult | null
}
const loading = ref(false)
const loaded = ref(false)
const saving = ref(false)
const error = ref('')
const savedMsg = ref('')
const mode = ref<PinMode>('off')
const upstream = ref('')
const matchPrefixesText = ref('')
const original = ref<ClinePassUpstreamPinConfig>({})
const rows = ref<ModelRow[]>([])
const accounts = ref<Array<{ id: number; name: string }>>([])
const probeAccountId = ref(0)
const accountsLoading = ref(false)
const accountsError = ref('')
const batchProbing = ref(false)
const probing = computed(() => batchProbing.value || rows.value.some(row => row.probing))
const pipelineOptions = [
  { key: 'vercel' as const, label: 'Vercel AI Gateway' },
  { key: 'openrouter' as const, label: 'OpenRouter' }
]
let nextKey = 0
let alive = true
let accountLoadVersion = 0
const message = (value: unknown): string => value instanceof Error ? value.message : String(value)
const canonicalModel = (model: string): string => model.trim().replace(/^cline-pass\//, '')

function makeRow(model = '', config: ClinePassUpstreamPinConfig = { mode: 'strict', pipelines: 'both' }): ModelRow {
  return {
    key: ++nextKey, model, upstream: config.upstream || '', mode: config.mode || '',
    pipelines: config.pipelines || '', original: config, probing: false, probeVersion: 0,
    probeModel: '', probeError: '', result: null
  }
}
function applyConfig(config: ClinePassUpstreamPinConfig | null) {
  original.value = config || {}
  mode.value = config?.mode || (config ? 'strict' : 'off')
  upstream.value = config?.upstream || ''
  matchPrefixesText.value = (config?.match_prefixes || []).join(', ')
  rows.value = Object.entries(config?.models || {}).map(([model, rule]) => makeRow(model, rule))
}
function addRow() {
  rows.value.push(makeRow())
  // Adding the first rule to an unset platform enables model-only strict routing.
  if (mode.value === 'off') mode.value = 'strict'
}
function clearProbe(row: ModelRow) {
  row.probeVersion++
  row.result = null
  row.probeError = ''
  row.probeModel = ''
}
function removeRow(row: ModelRow) {
  clearProbe(row)
  rows.value = rows.value.filter(item => item.key !== row.key)
}
function chooseProvider(row: ModelRow, provider: string) {
  row.upstream = provider
  row.mode = 'strict'
}
async function load() {
  if (loading.value || saving.value || probing.value) return
  loading.value = true
  error.value = ''
  savedMsg.value = ''
  try {
    const response = await getUpstreamPin()
    if (!alive) return
    applyConfig(response.config)
    loaded.value = true
  } catch (e: unknown) {
    if (alive) error.value = message(e)
  } finally {
    loading.value = false
  }
}
async function loadAccounts() {
  const version = ++accountLoadVersion
  accountsLoading.value = true
  accountsError.value = ''
  try {
    const items: Array<{ id: number; name: string }> = []
    let page = 1
    let pages = 1
    do {
      const response = await listAccounts(page, 100, { platform: 'clinepass', lite: 'true' })
      items.push(...response.items)
      pages = response.pages || 1
      page++
    } while (page <= pages && alive && version === accountLoadVersion)
    if (!alive || version !== accountLoadVersion) return
    accounts.value = items
    if (!items.some(account => account.id === probeAccountId.value)) probeAccountId.value = items[0]?.id || 0
    if (!items.length) accountsError.value = '没有可用于探测的 ClinePass 账号，请先添加账号。'
  } catch (e: unknown) {
    if (alive && version === accountLoadVersion) accountsError.value = message(e)
  } finally {
    if (version === accountLoadVersion) accountsLoading.value = false
  }
}
async function probeRow(row: ModelRow) {
  const model = row.model.trim()
  const accountId = probeAccountId.value
  if (!accountId || !model) {
    row.probeError = '请先选择账号并填写模型 ID。'
    return
  }
  const version = ++row.probeVersion
  row.probing = true
  row.result = null
  row.probeError = ''
  row.probeModel = ''
  const isCurrent = () => alive && row.probeVersion === version && row.model.trim() === model
    && probeAccountId.value === accountId && rows.value.some(item => item.key === row.key)
  try {
    const result = await probeUpstreams(accountId, model)
    if (!isCurrent()) return
    row.result = result
    row.probeModel = result.model
    if (!result.vercel?.length && !result.openrouter?.length) {
      row.probeError = '未探测到可用渠道，请展开诊断查看上游返回；这不等于该模型没有渠道。'
    }
  } catch (e: unknown) {
    if (isCurrent()) row.probeError = message(e)
  } finally {
    row.probing = false
  }
}
async function probeOne(row: ModelRow) {
  if (probing.value) return
  await probeRow(row)
}
async function probeAll() {
  if (probing.value || !probeAccountId.value) return
  batchProbing.value = true
  try {
    for (const row of [...rows.value]) {
      if (!alive) break
      if (rows.value.some(item => item.key === row.key)) await probeRow(row)
    }
  } finally {
    batchProbing.value = false
  }
}
function buildPayload(): ClinePassUpstreamPinConfig {
  if (mode.value === 'off') return { mode: 'off' }
  const models: Record<string, ClinePassUpstreamPinConfig> = {}
  const seen = new Set<string>()
  for (const row of rows.value) {
    const model = row.model.trim()
    const canonical = canonicalModel(model)
    if (!canonical) throw new Error('每条规则都需要填写模型 ID。')
    if (seen.has(canonical)) throw new Error(`模型重复：${model}（裸名称与 cline-pass/ 名称视为同一模型）`)
    seen.add(canonical)
    if (row.mode !== 'off' && model.toLowerCase().startsWith('cline-free')) throw new Error('cline-free 模型不支持钉定。')
    models[model] = { ...row.original, mode: row.mode || undefined, upstream: row.upstream.trim(), pipelines: row.pipelines || undefined }
  }
  return {
    ...original.value, mode: mode.value, upstream: upstream.value.trim(),
    match_prefixes: matchPrefixesText.value.split(/[,，]/).map(value => value.trim()).filter(Boolean), models
  }
}
async function save() {
  if (!loaded.value || saving.value || probing.value) return
  error.value = ''
  savedMsg.value = ''
  let payload: ClinePassUpstreamPinConfig
  try {
    payload = buildPayload()
  } catch (e: unknown) {
    error.value = message(e)
    return
  }
  saving.value = true
  try {
    const response = await updateUpstreamPin(payload)
    if (!alive) return
    applyConfig(response.config)
    savedMsg.value = response.config ? '已保存平台配置' : '已关闭平台钉定'
  } catch (e: unknown) {
    if (alive) error.value = message(e)
  } finally {
    saving.value = false
  }
}
watch([mode, upstream, matchPrefixesText, () => rows.value.map(row => [row.model, row.upstream, row.mode, row.pipelines])], () => { savedMsg.value = '' }, { flush: 'sync', deep: true })
watch(probeAccountId, () => { rows.value.forEach(clearProbe) })
onMounted(() => { void load(); void loadAccounts() })
onBeforeUnmount(() => { alive = false; accountLoadVersion++ })
</script>
