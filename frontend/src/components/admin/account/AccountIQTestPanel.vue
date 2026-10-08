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
          <button class="btn btn-secondary text-sm" @click="openPlanForm(null)">
            <Icon name="plus" size="sm" class="mr-1" />
            {{ t('admin.iqTest.addPlan') }}
          </button>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
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
          <Select v-model="modelId" :options="modelOptions" :placeholder="t('admin.iqTest.modelDefault')" />
        </div>
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.reasoningEffort') }}</label>
          <Select v-model="effort" :options="effortOptions" :placeholder="t('admin.iqTest.reasoningEffortDefault')" />
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
          <span class="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-gray-500 dark:text-gray-400">
            <span>{{ t('admin.iqTest.lastRun') }}: {{ plan.last_run_at ? formatDateTime(plan.last_run_at) : '-' }}</span>
            <span>{{ t('admin.iqTest.nextRun') }}: {{ plan.next_run_at ? formatDateTime(plan.next_run_at) : '-' }}</span>
          </span>
          <button class="btn btn-secondary text-sm" :disabled="running" @click="runPlan(plan)">{{ t('admin.iqTest.runNow') }}</button>
          <button class="btn btn-secondary text-sm" @click="openPlanForm(plan)">{{ t('common.edit') }}</button>
          <button class="btn btn-danger text-sm" @click="askRemovePlan(plan)">{{ t('common.delete') }}</button>
        </div>
      </div>

      <div>
        <div v-if="planForm" class="mb-3 space-y-3 rounded-xl border border-primary-200 bg-primary-50/50 p-3 dark:border-primary-800 dark:bg-primary-900/20">
        <div class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ planForm.id ? t('admin.iqTest.editPlan') : t('admin.iqTest.addPlan') }}
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.bank') }}</label>
            <Select v-model="planForm.bank_id" :options="bankOptions" @change="loadPlanQuestions" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.question') }}</label>
            <Select v-model="planForm.question_id" :options="planQuestionOptions" :placeholder="t('admin.iqTest.selectQuestion')" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.model') }}</label>
            <Select v-model="planForm.model_id" :options="modelOptions" :placeholder="t('admin.iqTest.modelDefault')" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.reasoningEffort') }}</label>
            <Select v-model="planForm.reasoning_effort" :options="effortOptions" :placeholder="t('admin.iqTest.reasoningEffortDefault')" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.cronExpression') }}</label>
            <Input v-model="planForm.cron_expression" placeholder="0 3 * * *" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.maxResults') }}</label>
            <Input v-model="planForm.max_results" type="number" placeholder="50" />
          </div>
        </div>
        <div class="flex items-center justify-between">
          <label class="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
            <Toggle v-model="planForm.enabled" />
            {{ t('admin.iqTest.enabled') }}
          </label>
          <div class="flex items-center gap-2">
            <button class="btn btn-secondary text-sm" @click="planForm = null">{{ t('common.cancel') }}</button>
            <button class="btn btn-primary text-sm" :disabled="saving" @click="savePlanForm">{{ t('common.save') }}</button>
          </div>
        </div>
      </div>

<div class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.iqTest.runs') }}</div>
        <div v-if="!runs.length" class="rounded-lg border border-dashed border-gray-300 p-4 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('admin.iqTest.noRuns') }}
        </div>
        <div v-for="run in runs" :key="run.id" class="mb-2 rounded-lg border border-gray-200 p-2 text-sm dark:border-dark-700">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="text-gray-700 dark:text-gray-300">
              #{{ run.id }} · {{ t('admin.iqTest.bank') }} #{{ run.bank_id }} · {{ run.model_id || '-' }}
            </span>
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.iqTest.startedAt') }}: {{ run.started_at ? formatDateTime(run.started_at) : '-' }}
              <template v-if="run.finished_at"> · {{ t('admin.iqTest.finishedAt') }}: {{ formatDateTime(run.finished_at) }}</template>
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
    <ConfirmDialog
      :show="showPlanDeleteConfirm"
      :title="t('common.confirm')"
      :message="t('admin.iqTest.confirmDeletePlan')"
      @confirm="doRemovePlan"
      @cancel="showPlanDeleteConfirm = false"
    />
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
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Input from '@/components/common/Input.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { REASONING_EFFORT_LEVELS } from '@/constants/channel'
import { formatDateTime } from '@/utils/format'

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
const effort = ref('')
const modelOptions = ref<SelectOption[]>([{ value: '', label: t('admin.iqTest.modelDefault') }])
const expanded = reactive<Record<number, boolean>>({})

const bankOptions = computed<SelectOption[]>(() =>
  banks.value.map((b) => ({ value: b.id, label: `${b.name} (#${b.id})` }))
)

const questionOptions = computed<SelectOption[]>(() =>
  runQuestions.value.map((q) => ({ value: q.id, label: `#${q.id} ${q.prompt.slice(0, 40)}` }))
)

const effortLabel = (level: string) => (level === 'xhigh' ? 'XHigh' : level.charAt(0).toUpperCase() + level.slice(1))
const effortOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.iqTest.reasoningEffortDefault') },
  ...REASONING_EFFORT_LEVELS.map((level) => ({ value: level, label: effortLabel(level) }))
])

const load = async () => {
  if (!props.accountId) return
  loading.value = true
  try {
    const [bankList, planList, runList, modelList] = await Promise.all([
      adminAPI.iqTest.listBanks(),
      adminAPI.iqTest.listPlansByAccount(props.accountId),
      adminAPI.iqTest.listRunsByAccount(props.accountId, 20),
      adminAPI.accounts.getAvailableModels(props.accountId)
    ])
    banks.value = bankList
    plans.value = planList
    runs.value = runList
    modelOptions.value = [
      { value: '', label: t('admin.iqTest.modelDefault') },
      ...modelList.map((model) => ({ value: model.id, label: model.display_name || model.id }))
    ]
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
      model_id: modelId.value,
      reasoning_effort: effort.value
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

const showPlanDeleteConfirm = ref(false)
const deletingPlan = ref<IQTestPlan | null>(null)
const saving = ref(false)
const planForm = ref<null | {
  id: number | null
  bank_id: number | null
  question_id: number | null
  model_id: string
  reasoning_effort: string
  cron_expression: string
  max_results: string
  enabled: boolean
}>(null)
const planQuestions = ref<IQTestQuestion[]>([])

const planQuestionOptions = computed<SelectOption[]>(() =>
  planQuestions.value.map((q) => ({ value: q.id, label: `#${q.id} ${q.prompt.slice(0, 40)}` }))
)

const openPlanForm = (plan: IQTestPlan | null) => {
  planQuestions.value = []
  if (plan) {
    planForm.value = {
      id: plan.id,
      bank_id: plan.bank_id,
      question_id: plan.question_id ?? null,
      model_id: plan.model_id,
      reasoning_effort: plan.reasoning_effort || '',
      cron_expression: plan.cron_expression,
      max_results: String(plan.max_results || 50),
      enabled: plan.enabled
    }
    void loadPlanQuestions()
  } else {
    planForm.value = {
      id: null,
      bank_id: selectedBankId.value,
      question_id: null,
      model_id: '',
      reasoning_effort: '',
      cron_expression: '',
      max_results: '50',
      enabled: true
    }
    if (selectedBankId.value != null) void loadPlanQuestions()
  }
}

const loadPlanQuestions = async () => {
  const bankId = planForm.value?.bank_id
  planQuestions.value = []
  if (bankId == null) return
  try {
    const detail = await adminAPI.iqTest.getBank(bankId)
    planQuestions.value = detail.questions || []
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.loadFailed'))
  }
}

const savePlanForm = async () => {
  const form = planForm.value
  if (!form || !props.accountId) return
  if (form.bank_id == null || form.question_id == null) {
    appStore.showError(t('admin.iqTest.questionRequired'))
    return
  }
  saving.value = true
  try {
    if (form.id) {
      await adminAPI.iqTest.updatePlan(form.id, {
        question_id: Number(form.question_id),
        model_id: form.model_id,
        cron_expression: form.cron_expression,
        enabled: form.enabled,
        max_results: Number(form.max_results) || 50,
        reasoning_effort: form.reasoning_effort
      })
      appStore.showSuccess(t('admin.iqTest.planUpdated'))
    } else {
      await adminAPI.iqTest.createPlan({
        bank_id: form.bank_id,
        question_id: Number(form.question_id),
        account_id: props.accountId,
        model_id: form.model_id,
        cron_expression: form.cron_expression,
        enabled: form.enabled,
        max_results: Number(form.max_results) || 50,
        reasoning_effort: form.reasoning_effort
      })
      appStore.showSuccess(t('admin.iqTest.planCreated'))
    }
    planForm.value = null
    plans.value = await adminAPI.iqTest.listPlansByAccount(props.accountId)
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.saveFailed'))
  } finally {
    saving.value = false
  }
}

const askRemovePlan = (plan: IQTestPlan) => {
  deletingPlan.value = plan
  showPlanDeleteConfirm.value = true
}

const doRemovePlan = async () => {
  const plan = deletingPlan.value
  showPlanDeleteConfirm.value = false
  if (!plan || !props.accountId) return
  try {
    await adminAPI.iqTest.deletePlan(plan.id)
    appStore.showSuccess(t('admin.iqTest.planDeleted'))
    plans.value = await adminAPI.iqTest.listPlansByAccount(props.accountId)
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.saveFailed'))
  } finally {
    deletingPlan.value = null
  }
}

watch(
  () => [props.show, props.accountId],
  ([show]) => {
    if (show) {
      selectedBankId.value = null
      selectedQuestionId.value = null
      modelId.value = ''
      effort.value = ''
      planForm.value = null
      load()
    }
  }
)
</script>
