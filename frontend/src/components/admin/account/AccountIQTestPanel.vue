<template>
  <BaseDialog :show="show" :title="t('admin.iqTest.runForAccountTitle')" width="wide" @close="emit('close')">
    <div class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.iqTest.accountId') }} #{{ accountId ?? '-' }}
        </div>
        <div class="flex items-center gap-2">
          <button class="btn btn-secondary text-sm" :disabled="loading" @click="load">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button class="btn btn-primary text-sm" :disabled="running || !accountId" @click="runNow">
            <Icon name="play" size="sm" class="mr-1" />
            {{ t('admin.iqTest.runNow') }}
          </button>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.bank') }}</label>
          <Select v-model="selectedBankId" :options="bankOptions" :placeholder="t('admin.iqTest.selectBank')" />
        </div>
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.question') }}</label>
          <Select v-model="selectedQuestionId" :options="questionOptions" :placeholder="t('admin.iqTest.selectQuestion')" />
        </div>
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.model') }}</label>
          <Input v-model="modelId" :placeholder="t('admin.iqTest.modelPlaceholder')" />
        </div>
      </div>

      <div>
        <div class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.iqTest.plans') }}</div>
        <div v-if="!plans.length" class="rounded-lg border border-dashed border-gray-300 p-4 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('admin.iqTest.noPlans') }}
        </div>
        <div v-for="plan in plans" :key="plan.id" class="mb-2 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-gray-200 p-2 text-sm dark:border-dark-700">
          <span class="text-gray-700 dark:text-gray-300">
            {{ t('admin.iqTest.bank') }} #{{ plan.bank_id }} · {{ t('admin.iqTest.question') }} #{{ plan.question_id ?? '-' }} · cron {{ plan.cron_expression || '-' }} ·
            {{ plan.enabled ? t('admin.iqTest.enabled') : t('common.disabled') }}
          </span>
          <button class="btn btn-secondary text-sm" :disabled="running" @click="runPlan(plan)">{{ t('admin.iqTest.runNow') }}</button>
        </div>
      </div>

      <div>
        <div class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.iqTest.runs') }}</div>
        <div v-if="!runs.length" class="rounded-lg border border-dashed border-gray-300 p-4 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('admin.iqTest.noRuns') }}
        </div>
        <div v-for="run in runs" :key="run.id" class="mb-2 rounded-lg border border-gray-200 p-2 text-sm dark:border-dark-700">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="text-gray-700 dark:text-gray-300">
              #{{ run.id }} · {{ t('admin.iqTest.bank') }} #{{ run.bank_id }} · {{ run.model_id || '-' }}
            </span>
            <span class="flex items-center gap-2">
              <span class="font-medium">{{ run.score }}</span>
              <span class="text-xs text-gray-500">{{ run.correct }}/{{ run.total }} · {{ run.latency_ms }} ms</span>
              <span class="text-xs" :class="run.status === 'success' ? 'text-green-600' : 'text-red-600'">
                {{ run.status === 'success' ? t('admin.iqTest.success') : t('admin.iqTest.failed') }}
              </span>
              <button class="text-xs text-primary-600 hover:underline dark:text-primary-400" @click="loadRunDetail(run.id)">
                {{ t('admin.iqTest.viewDetails') }}
              </button>
            </span>
          </div>
          <div v-if="expanded[run.id]" class="mt-2 space-y-1 border-t border-gray-100 pt-2 dark:border-dark-700">
            <p v-if="run.error_message" class="text-xs text-red-500">{{ run.error_message }}</p>
            <div v-for="(d, idx) in run.details || []" :key="idx" class="text-xs text-gray-600 dark:text-gray-400">
              <span :class="d.correct ? 'text-green-600' : 'text-red-500'">{{ d.correct ? '✓' : '✗' }}</span>
              {{ d.prompt }} → {{ d.response || '-' }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { IQTestBank, IQTestPlan, IQTestQuestion, IQTestRun } from '@/types'
import type { SelectOption } from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Input from '@/components/common/Input.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  show: boolean
  accountId: number | null
}>()

const emit = defineEmits<{ (e: 'close'): void }>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const running = ref(false)
const banks = ref<IQTestBank[]>([])
const plans = ref<IQTestPlan[]>([])
const runs = ref<IQTestRun[]>([])
const selectedBankId = ref<number | null>(null)
const selectedQuestionId = ref<number | null>(null)
const runQuestions = ref<IQTestQuestion[]>([])
const modelId = ref('')
const expanded = reactive<Record<number, boolean>>({})

const bankOptions = computed<SelectOption[]>(() =>
  banks.value.map((b) => ({ value: b.id, label: `${b.name} (#${b.id})` }))
)

const questionOptions = computed<SelectOption[]>(() =>
  runQuestions.value.map((q) => ({ value: q.id, label: `#${q.id} ${q.prompt.slice(0, 40)}` }))
)

const load = async () => {
  if (!props.accountId) return
  loading.value = true
  try {
    const [bankList, planList, runList] = await Promise.all([
      adminAPI.iqTest.listBanks(),
      adminAPI.iqTest.listPlansByAccount(props.accountId),
      adminAPI.iqTest.listRunsByAccount(props.accountId, 20)
    ])
    banks.value = bankList
    plans.value = planList
    runs.value = runList
    if (selectedBankId.value == null && bankList.length) selectedBankId.value = bankList[0].id
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.loadFailed'))
  } finally {
    loading.value = false
  }
}

const refreshRuns = async () => {
  if (!props.accountId) return
  runs.value = await adminAPI.iqTest.listRunsByAccount(props.accountId, 20)
}

watch(selectedBankId, async (id) => {
  selectedQuestionId.value = null
  runQuestions.value = []
  if (id == null) return
  try {
    const detail = await adminAPI.iqTest.getBank(id)
    runQuestions.value = detail.questions || []
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.loadFailed'))
  }
})

const runNow = async () => {
  if (!props.accountId || selectedBankId.value == null) return
  const questionId = selectedQuestionId.value
  if (questionId == null) {
    appStore.showError(t('admin.iqTest.questionRequired'))
    return
  }
  running.value = true
  try {
    const run = await adminAPI.iqTest.runForAccount(props.accountId, {
      bank_id: selectedBankId.value,
      question_id: questionId,
      model_id: modelId.value
    })
    appStore.showSuccess(`${t('admin.iqTest.runStarted')}: ${run.score}`)
    await refreshRuns()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.runFailed'))
  } finally {
    running.value = false
  }
}

const runPlan = async (plan: IQTestPlan) => {
  running.value = true
  try {
    const run = await adminAPI.iqTest.runPlanNow(plan.id)
    appStore.showSuccess(`${t('admin.iqTest.runStarted')}: ${run.score}`)
    await refreshRuns()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.runFailed'))
  } finally {
    running.value = false
  }
}

const loadRunDetail = async (runId: number) => {
  if (expanded[runId]) {
    expanded[runId] = false
    return
  }
  try {
    const detail = await adminAPI.iqTest.getRun(runId)
    const idx = runs.value.findIndex((r) => r.id === runId)
    if (idx >= 0) runs.value[idx] = detail
    expanded[runId] = true
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.loadFailed'))
  }
}

watch(
  () => [props.show, props.accountId],
  ([show]) => {
    if (show) {
      selectedBankId.value = null
      selectedQuestionId.value = null
      modelId.value = ''
      load()
    }
  }
)
</script>
