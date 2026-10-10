<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        ClinePass 上游钉定（平台级）
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        对所有 ClinePass 账号生效：命中模型前缀的请求会被固定路由到指定上游渠道（例如 deepseek）。
        单个账号的 extra.clinepass_upstream_pin 可覆盖此处配置；账号级 mode=off 可让该账号退出平台级钉定。
      </p>
    </div>
    <div class="space-y-4 p-6">
      <div class="grid gap-4 md:grid-cols-3">
        <label class="block">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">模式</span>
          <select
            v-model="mode"
            class="mt-1 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
          >
            <option value="off">off（不钉定）</option>
            <option value="strict">strict（只走钉定渠道）</option>
            <option value="preferred">preferred（优先钉定渠道）</option>
          </select>
        </label>
        <label class="block">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">上游渠道（upstream）</span>
          <input
            v-model="upstream"
            type="text"
            placeholder="deepseek"
            :disabled="mode === 'off'"
            class="mt-1 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 disabled:opacity-50 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
          />
        </label>
        <label class="block">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">生效模型前缀（match_prefixes）</span>
          <input
            v-model="matchPrefixes"
            type="text"
            placeholder="deepseek"
            :disabled="mode === 'off'"
            class="mt-1 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 disabled:opacity-50 dark:border-dark-600 dark:bg-dark-900 dark:text-white"
          />
          <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">
            逗号分隔；留空=所有模型；cline-free/* 永不钉定
          </span>
        </label>
      </div>
      <div class="flex items-center gap-3">
        <button
          type="button"
          :disabled="saving || loading"
          class="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          @click="save"
        >
          {{ saving ? '保存中…' : '保存' }}
        </button>
        <span v-if="error" class="text-sm text-red-600 dark:text-red-400">{{ error }}</span>
        <span v-else-if="savedMsg" class="text-sm text-green-600 dark:text-green-400">{{ savedMsg }}</span>
        <span v-else-if="loading" class="text-sm text-gray-500 dark:text-gray-400">加载中…</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getUpstreamPin, updateUpstreamPin, type ClinePassUpstreamPinConfig } from '@/api/admin/clinepass'

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const savedMsg = ref('')
const mode = ref<'off' | 'strict' | 'preferred'>('off')
const upstream = ref('deepseek')
const matchPrefixes = ref('deepseek')

function extractConfig(resp: unknown): ClinePassUpstreamPinConfig | null {
  if (!resp || typeof resp !== 'object') return null
  const holder = resp as { config?: ClinePassUpstreamPinConfig | null }
  if ('config' in holder) return holder.config ?? null
  return resp as ClinePassUpstreamPinConfig
}

onMounted(async () => {
  loading.value = true
  error.value = ''
  try {
    const cfg = extractConfig(await getUpstreamPin())
    if (cfg && typeof cfg === 'object') {
      const m = String(cfg.mode ?? '').toLowerCase()
      mode.value = m === 'preferred' ? 'preferred' : m === 'strict' ? 'strict' : 'off'
      if (cfg.upstream) upstream.value = String(cfg.upstream)
      if (Array.isArray(cfg.match_prefixes) && cfg.match_prefixes.length > 0) {
        matchPrefixes.value = cfg.match_prefixes.join(',')
      }
    } else {
      mode.value = 'off'
    }
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
})

async function save() {
  saving.value = true
  error.value = ''
  savedMsg.value = ''
  try {
    const prefixes = matchPrefixes.value
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean)
    const payload: ClinePassUpstreamPinConfig =
      mode.value === 'off'
        ? { mode: 'off' }
        : { mode: mode.value, upstream: upstream.value.trim(), match_prefixes: prefixes }
    const resp = extractConfig(await updateUpstreamPin(payload))
    savedMsg.value = resp ? '已保存并生效' : '已关闭平台级钉定'
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}
</script>
