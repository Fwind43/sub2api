<template>
  <AppLayout>
    <div class="space-y-6 p-4 sm:p-6">
      <!-- Header -->
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-gray-100">
            {{ t('admin.iqTest.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.iqTest.description') }}
          </p>
        </div>
        <div class="flex items-center gap-2">
          <button class="btn btn-secondary" :disabled="loading" @click="loadAll">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button class="btn btn-primary" @click="openCreateBank">
            <Icon name="plus" size="md" class="mr-1" />
            {{ t('admin.iqTest.newBank') }}
          </button>
        </div>
      </div>

      <!-- Tabs -->
      <div class="flex gap-1 border-b border-gray-200 dark:border-dark-700">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          class="px-3 py-2 text-sm font-medium transition-colors"
          :class="
            activeTab === tab.key
              ? 'border-b-2 border-primary-500 text-primary-600 dark:text-primary-400'
              : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
          "
          @click="activeTab = tab.key"
        >
          {{ t(tab.label) }}
        </button>
      </div>

      <!-- Banks -->
      <div v-if="activeTab === 'banks'" class="space-y-4">
        <div v-if="!banks.length && !loading" class="rounded-xl border border-dashed border-gray-300 p-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('admin.iqTest.noBanks') }}
        </div>
        <div v-for="bank in banks" :key="bank.id" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-wrap items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-gray-100">{{ bank.name }}</span>
                <span
                  class="rounded-full px-2 py-0.5 text-xs"
                  :class="bank.enabled ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'"
                >
                  {{ bank.enabled ? t('admin.iqTest.enabled') : t('common.disabled') }}
                </span>
                <span class="text-xs text-gray-400">{{ t('admin.iqTest.questionCount') }}: {{ bank.questions?.length ?? 0 }}</span>
              </div>
              <p v-if="bank.description" class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ bank.description }}</p>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <button class="btn btn-secondary text-sm" @click="openPlans(bank)">{{ t('admin.iqTest.plans') }}</button>
              <button class="btn btn-secondary text-sm" @click="openRunDialog(bank)">{{ t('admin.iqTest.runAgainstAccount') }}</button>
              <button class="btn btn-secondary text-sm" @click="openEditBank(bank)">{{ t('common.edit') }}</button>
              <button class="btn btn-danger text-sm" @click="confirmDeleteBank(bank)">{{ t('common.delete') }}</button>
            </div>
          </div>
        </div>
      </div>

      <!-- Runs -->
      <div v-else class="space-y-4">
        <div v-if="!runs.length && !loading" class="rounded-xl border border-dashed border-gray-300 p-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('admin.iqTest.noRuns') }}
        </div>
        <div v-else class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
          <table class="min-w-full text-sm">
            <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th class="px-3 py-2">#</th>
                <th class="px-3 py-2">{{ t('admin.iqTest.bank') }}</th>
                <th class="px-3 py-2">{{ t('admin.iqTest.accountId') }}</th>
                <th class="px-3 py-2">{{ t('admin.iqTest.model') }}</th>
                <th class="px-3 py-2">{{ t('admin.iqTest.score') }}</th>
                <th class="px-3 py-2">{{ t('admin.iqTest.correct') }}</th>
                <th class="px-3 py-2">{{ t('admin.iqTest.latency') }}</th>
                <th class="px-3 py-2">{{ t('admin.iqTest.trigger') }}</th>
                <th class="px-3 py-2">{{ t('admin.iqTest.status') }}</th>
                <th class="px-3 py-2">{{ t('admin.iqTest.startedAt') }}</th>
                <th class="px-3 py-2"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <template v-for="run in runs" :key="run.id">
                <tr class="hover:bg-gray-50 dark:hover:bg-dark-700/40">
                  <td class="px-3 py-2 text-gray-400">{{ run.id }}</td>
                  <td class="px-3 py-2">{{ bankName(run.bank_id) }}</td>
                  <td class="px-3 py-2">{{ run.account_id }}</td>
                  <td class="px-3 py-2">{{ run.model_id || '-' }}</td>
                  <td class="px-3 py-2 font-medium">{{ run.score }}</td>
                  <td class="px-3 py-2">{{ run.correct }}/{{ run.total }}</td>
                  <td class="px-3 py-2">{{ run.latency_ms }} ms</td>
                  <td class="px-3 py-2">{{ run.trigger === 'scheduled' ? t('admin.iqTest.scheduled') : t('admin.iqTest.manual') }}</td>
                  <td class="px-3 py-2">
                    <span :class="run.status === 'success' ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'">
                      {{ run.status === 'success' ? t('admin.iqTest.success') : t('admin.iqTest.failed') }}
                    </span>
                  </td>
                  <td class="px-3 py-2 text-gray-500">{{ formatDateTime(run.started_at) }}</td>
                  <td class="px-3 py-2 text-right">
                    <button class="text-primary-600 hover:underline dark:text-primary-400" @click="toggleRunDetails(run.id)">
                      {{ t('admin.iqTest.viewDetails') }}
                    </button>
                  </td>
                </tr>
                <tr v-if="expandedRuns.has(run.id)">
                  <td colspan="11" class="bg-gray-50 px-3 py-3 dark:bg-dark-800/60">
                    <p v-if="run.error_message" class="mb-2 text-sm text-red-600 dark:text-red-400">{{ run.error_message }}</p>
                    <div v-if="run.details?.length" class="space-y-2">
                      <div v-for="(d, idx) in run.details" :key="idx" class="rounded-lg border border-gray-200 bg-white p-3 text-sm dark:border-dark-700 dark:bg-dark-900">
                        <div class="flex items-center justify-between gap-2">
                          <span class="font-medium text-gray-800 dark:text-gray-200">{{ idx + 1 }}. {{ d.prompt }}</span>
                          <span :class="d.correct ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'">
                            {{ d.correct ? t('admin.iqTest.pass') : t('admin.iqTest.fail') }}
                          </span>
                        </div>
                        <p class="mt-1 text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.response') }}: {{ d.response || '-' }}</p>
                        <p class="text-gray-500 dark:text-gray-500">{{ t('admin.iqTest.expected') }}: {{ d.expected }}</p>
                        <p v-if="d.error_message" class="text-red-500">{{ d.error_message }}</p>
                      </div>
                    </div>
                    <p v-else class="text-sm text-gray-500">{{ t('admin.iqTest.noRuns') }}</p>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Bank editor -->
    <BaseDialog :show="showBankDialog" :title="editingBankId ? t('admin.iqTest.editBank') : t('admin.iqTest.newBank')" width="wide" @close="showBankDialog = false">
      <div class="space-y-4">
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.bankName') }}</label>
            <Input v-model="bankForm.name" :placeholder="t('admin.iqTest.bankName')" />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.bankDescription') }}</label>
            <Input v-model="bankForm.description" :placeholder="t('admin.iqTest.bankDescription')" />
          </div>
          <div class="flex items-end">
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <Toggle v-model="bankForm.enabled" />
              {{ t('admin.iqTest.enabled') }}
            </label>
          </div>
        </div>

        <div class="flex items-center justify-between">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.iqTest.questions') }}</span>
          <button class="btn btn-secondary text-sm" @click="addQuestion">
            <Icon name="plus" size="sm" class="mr-1" />
            {{ t('admin.iqTest.addQuestion') }}
          </button>
        </div>

        <div v-for="(q, idx) in bankForm.questions" :key="idx" class="space-y-2 rounded-xl border border-gray-200 p-3 dark:border-dark-700">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-gray-500">{{ idx + 1 }}</span>
            <button class="text-xs text-red-500 hover:underline" @click="bankForm.questions.splice(idx, 1)">
              {{ t('admin.iqTest.deleteQuestion') }}
            </button>
          </div>
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
            <div>
              <label class="mb-1 block text-xs text-gray-500">{{ t('admin.iqTest.questionType') }}</label>
              <Select v-model="q.type" :options="typeOptions" />
            </div>
            <div>
              <label class="mb-1 block text-xs text-gray-500">{{ t('admin.iqTest.weight') }}</label>
              <Input v-model="q.weight" type="number" placeholder="1" />
            </div>
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-500">{{ t('admin.iqTest.prompt') }}</label>
            <textarea v-model="q.prompt" rows="2" class="input w-full"></textarea>
          </div>
          <div v-if="q.type === 'single_choice'">
            <label class="mb-1 block text-xs text-gray-500">{{ t('admin.iqTest.options') }}</label>
            <textarea v-model="q.optionsText" rows="3" class="input w-full" placeholder="A. ...&#10;B. ..."></textarea>
          </div>
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-xs text-gray-500">{{ t('admin.iqTest.answer') }}</label>
              <Input v-model="q.answer" :placeholder="q.type === 'single_choice' ? 'B' : 'keyword1, keyword2'" />
            </div>
            <div>
              <label class="mb-1 block text-xs text-gray-500">{{ t('admin.iqTest.keywords') }}</label>
              <Input v-model="q.keywordsText" placeholder="keyword1, keyword2" />
            </div>
          </div>
          <p class="text-xs text-gray-400">{{ t('admin.iqTest.answerHelp') }}</p>
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button class="btn btn-secondary" @click="showBankDialog = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="saving" @click="saveBank">{{ t('common.save') }}</button>
        </div>
      </div>
    </BaseDialog>

    <!-- Plans -->
    <BaseDialog :show="showPlansDialog" :title="t('admin.iqTest.plans')" width="wide" @close="showPlansDialog = false">
      <div class="space-y-4">
        <div class="flex items-center justify-between">
          <span class="text-sm text-gray-500 dark:text-gray-400">{{ plansBank?.name }}</span>
          <button class="btn btn-secondary text-sm" @click="addPlan">
            <Icon name="plus" size="sm" class="mr-1" />
            {{ t('admin.iqTest.addPlan') }}
          </button>
        </div>

        <div v-if="planForm" class="grid grid-cols-1 gap-3 rounded-xl border border-primary-200 bg-primary-50/50 p-3 sm:grid-cols-2 dark:border-primary-800 dark:bg-primary-900/20">
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.accountId') }}</label>
            <Input v-model="planForm.account_id" type="number" placeholder="1" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.question') }}</label>
            <Select v-model="planForm.question_id" :options="planQuestionOptions" :placeholder="t('admin.iqTest.selectQuestion')" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.model') }}</label>
            <Input v-model="planForm.model_id" :placeholder="t('admin.iqTest.modelPlaceholder')" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.cronExpression') }}</label>
            <Input v-model="planForm.cron_expression" placeholder="*/30 * * * *" :hint="t('admin.iqTest.cronHelp')" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.maxResults') }}</label>
            <Input v-model="planForm.max_results" type="number" placeholder="50" />
          </div>
          <div class="flex items-center gap-3">
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <Toggle v-model="planForm.enabled" />
              {{ t('admin.iqTest.enabled') }}
            </label>
            <button class="btn btn-primary text-sm" :disabled="saving" @click="savePlan">{{ t('common.save') }}</button>
            <button class="btn btn-secondary text-sm" @click="planForm = null">{{ t('common.cancel') }}</button>
          </div>
        </div>

        <div v-if="!plans.length" class="rounded-lg border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
          {{ t('admin.iqTest.noPlans') }}
        </div>
        <div v-for="plan in plans" :key="plan.id" class="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-gray-200 p-3 text-sm dark:border-dark-700">
          <div class="min-w-0">
            <div class="font-medium text-gray-800 dark:text-gray-200">
              {{ t('admin.iqTest.accountId') }} #{{ plan.account_id }} · {{ plan.model_id || t('admin.iqTest.model') }} · {{ t('admin.iqTest.question') }} #{{ plan.question_id ?? '-' }}
            </div>
            <div class="text-xs text-gray-500">
              cron: {{ plan.cron_expression || '-' }} · {{ t('admin.iqTest.lastRun') }}: {{ plan.last_run_at ? formatDateTime(plan.last_run_at) : '-' }} ·
              {{ t('admin.iqTest.nextRun') }}: {{ plan.next_run_at ? formatDateTime(plan.next_run_at) : '-' }}
            </div>
          </div>
          <div class="flex items-center gap-2">
            <span class="text-xs" :class="plan.enabled ? 'text-green-600' : 'text-gray-400'">{{ plan.enabled ? t('admin.iqTest.enabled') : t('common.disabled') }}</span>
            <button class="btn btn-secondary text-sm" :disabled="running" @click="runPlanNow(plan)">{{ t('admin.iqTest.runNow') }}</button>
            <button class="btn btn-secondary text-sm" @click="editPlan(plan)">{{ t('common.edit') }}</button>
            <button class="btn btn-danger text-sm" @click="removePlan(plan)">{{ t('common.delete') }}</button>
          </div>
        </div>
      </div>
    </BaseDialog>

    <!-- Run for account -->
    <BaseDialog :show="showRunDialog" :title="t('admin.iqTest.runForAccountTitle')" @close="showRunDialog = false">
      <div class="space-y-3">
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.accountId') }}</label>
          <Input v-model="runForm.account_id" type="number" placeholder="1" />
        </div>
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.question') }}</label>
          <Select v-model="runForm.question_id" :options="runQuestionOptions" :placeholder="t('admin.iqTest.selectQuestion')" />
        </div>
        <div>
          <label class="mb-1 block text-xs text-gray-600 dark:text-gray-400">{{ t('admin.iqTest.model') }}</label>
          <Input v-model="runForm.model_id" :placeholder="t('admin.iqTest.modelPlaceholder')" />
        </div>
        <div class="flex justify-end gap-2 pt-2">
          <button class="btn btn-secondary" @click="showRunDialog = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="running" @click="submitRun">{{ t('admin.iqTest.runNow') }}</button>
        </div>
      </div>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteBankConfirm"
      :title="t('common.confirm')"
      :message="t('admin.iqTest.confirmDeleteBank')"
      @confirm="doDeleteBank"
      @cancel="showDeleteBankConfirm = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import type { IQTestBank, IQTestPlan, IQTestQuestion, IQTestRun, IQTestQuestionInput } from '@/types'
import type { SelectOption } from '@/components/common/Select.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Input from '@/components/common/Input.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const tabs: { key: 'banks' | 'runs'; label: string }[] = [
  { key: 'banks', label: 'admin.iqTest.banks' },
  { key: 'runs', label: 'admin.iqTest.runs' }
]
const activeTab = ref<'banks' | 'runs'>('banks')

const typeOptions: SelectOption[] = [
  { value: 'single_choice', label: t('admin.iqTest.singleChoice') },
  { value: 'open', label: t('admin.iqTest.open') }
]

const loading = ref(false)
const saving = ref(false)
const running = ref(false)
const banks = ref<IQTestBank[]>([])
const runs = ref<IQTestRun[]>([])
const expandedRuns = reactive(new Set<number>())

// Bank editor state
interface QuestionForm {
  type: string
  prompt: string
  optionsText: string
  answer: string
  keywordsText: string
  weight: string
}
const showBankDialog = ref(false)
const editingBankId = ref<number | null>(null)
const bankForm = reactive({
  name: '',
  description: '',
  enabled: true,
  questions: [] as QuestionForm[]
})

// Plans state
const showPlansDialog = ref(false)
const plansBank = ref<IQTestBank | null>(null)
const plans = ref<IQTestPlan[]>([])
const planForm = ref<null | {
  id: number | null
  account_id: string
  question_id: number | null
  model_id: string
  cron_expression: string
  max_results: string
  enabled: boolean
}>(null)

// Run state
const showRunDialog = ref(false)
const runQuestions = ref<IQTestQuestion[]>([])
const runForm = reactive({ bank_id: 0, account_id: '', model_id: '', question_id: null as number | null })
const runQuestionOptions = computed<SelectOption[]>(() => questionOptionsOf(runQuestions.value))

// Delete confirm
const showDeleteBankConfirm = ref(false)
const deletingBank = ref<IQTestBank | null>(null)

const bankName = (bankId: number) => banks.value.find((b) => b.id === bankId)?.name || `#${bankId}`

const questionOptionsOf = (questions?: IQTestQuestion[]) =>
  (questions || []).map((q) => ({ value: q.id, label: `#${q.id} ${q.prompt.slice(0, 40)}` }))

const planQuestionOptions = computed<SelectOption[]>(() => questionOptionsOf(plansBank.value?.questions))

const loadAll = async () => {
  loading.value = true
  try {
    const [bankList, runList] = await Promise.all([
      adminAPI.iqTest.listBanks(),
      adminAPI.iqTest.listRuns({ limit: 100 })
    ])
    banks.value = bankList
    runs.value = runList
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.loadFailed'))
  } finally {
    loading.value = false
  }
}

const addQuestion = () => {
  bankForm.questions.push({ type: 'single_choice', prompt: '', optionsText: '', answer: '', keywordsText: '', weight: '1' })
}

const openCreateBank = () => {
  editingBankId.value = null
  bankForm.name = ''
  bankForm.description = ''
  bankForm.enabled = true
  bankForm.questions = []
  addQuestion()
  showBankDialog.value = true
}

const openEditBank = async (bank: IQTestBank) => {
  editingBankId.value = bank.id
  bankForm.name = bank.name
  bankForm.description = bank.description
  bankForm.enabled = bank.enabled
  bankForm.questions = []
  try {
    const detail = await adminAPI.iqTest.getBank(bank.id)
    bankForm.questions = (detail.questions || []).map((q) => ({
      type: q.type || 'single_choice',
      prompt: q.prompt,
      optionsText: (q.options || []).join('\n'),
      answer: q.answer,
      keywordsText: (q.keywords || []).join(', '),
      weight: String(q.weight || 1)
    }))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.loadFailed'))
  }
  if (!bankForm.questions.length) addQuestion()
  showBankDialog.value = true
}

const toQuestionInputs = (): IQTestQuestionInput[] =>
  bankForm.questions
    .filter((q) => q.prompt.trim())
    .map((q) => ({
      type: q.type,
      prompt: q.prompt,
      options:
        q.type === 'single_choice'
          ? q.optionsText.split('\n').map((s) => s.trim()).filter(Boolean)
          : [],
      answer: q.answer,
      keywords: q.keywordsText.split(',').map((s) => s.trim()).filter(Boolean),
      weight: Number(q.weight) || 1
    }))

const saveBank = async () => {
  if (!bankForm.name.trim()) return
  saving.value = true
  try {
    const questions = toQuestionInputs()
    if (editingBankId.value) {
      await adminAPI.iqTest.updateBank(editingBankId.value, {
        name: bankForm.name,
        description: bankForm.description,
        enabled: bankForm.enabled,
        questions,
        replace_questions: true
      })
      appStore.showSuccess(t('admin.iqTest.bankUpdated'))
    } else {
      await adminAPI.iqTest.createBank({
        name: bankForm.name,
        description: bankForm.description,
        enabled: bankForm.enabled,
        questions
      })
      appStore.showSuccess(t('admin.iqTest.bankCreated'))
    }
    showBankDialog.value = false
    await loadAll()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.saveFailed'))
  } finally {
    saving.value = false
  }
}

const confirmDeleteBank = (bank: IQTestBank) => {
  deletingBank.value = bank
  showDeleteBankConfirm.value = true
}

const doDeleteBank = async () => {
  if (!deletingBank.value) return
  showDeleteBankConfirm.value = false
  try {
    await adminAPI.iqTest.deleteBank(deletingBank.value.id)
    appStore.showSuccess(t('admin.iqTest.bankDeleted'))
    await loadAll()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.saveFailed'))
  } finally {
    deletingBank.value = null
  }
}

const openPlans = async (bank: IQTestBank) => {
  plansBank.value = bank
  planForm.value = null
  showPlansDialog.value = true
  try {
    const [detail, planList] = await Promise.all([adminAPI.iqTest.getBank(bank.id), adminAPI.iqTest.listPlansByBank(bank.id)])
    plansBank.value = detail
    plans.value = planList
  } catch (error: any) {
    plans.value = []
    appStore.showError(error?.message || t('admin.iqTest.loadFailed'))
  }
}

const addPlan = () => {
  planForm.value = {
    id: null,
    account_id: '',
    question_id: null,
    model_id: '',
    cron_expression: '',
    max_results: '50',
    enabled: true
  }
}

const editPlan = (plan: IQTestPlan) => {
  planForm.value = {
    id: plan.id,
    account_id: String(plan.account_id),
    question_id: plan.question_id ?? null,
    model_id: plan.model_id,
    cron_expression: plan.cron_expression,
    max_results: String(plan.max_results || 50),
    enabled: plan.enabled
  }
}

const savePlan = async () => {
  const form = planForm.value
  if (!form || !plansBank.value) return
  if (!form.account_id) {
    appStore.showError(t('admin.iqTest.accountRequired'))
    return
  }
  if (form.question_id == null) {
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
        max_results: Number(form.max_results) || 50
      })
      appStore.showSuccess(t('admin.iqTest.planUpdated'))
    } else {
      await adminAPI.iqTest.createPlan({
        bank_id: plansBank.value.id,
        question_id: Number(form.question_id),
        account_id: Number(form.account_id),
        model_id: form.model_id,
        cron_expression: form.cron_expression,
        enabled: form.enabled,
        max_results: Number(form.max_results) || 50
      })
      appStore.showSuccess(t('admin.iqTest.planCreated'))
    }
    planForm.value = null
    plans.value = await adminAPI.iqTest.listPlansByBank(plansBank.value.id)
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.saveFailed'))
  } finally {
    saving.value = false
  }
}

const removePlan = async (plan: IQTestPlan) => {
  try {
    await adminAPI.iqTest.deletePlan(plan.id)
    appStore.showSuccess(t('admin.iqTest.planDeleted'))
    if (plansBank.value) plans.value = await adminAPI.iqTest.listPlansByBank(plansBank.value.id)
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.saveFailed'))
  }
}

const runPlanNow = async (plan: IQTestPlan) => {
  running.value = true
  try {
    const run = await adminAPI.iqTest.runPlanNow(plan.id)
    appStore.showSuccess(`${t('admin.iqTest.runStarted')}: ${run.score}`)
    runs.value = await adminAPI.iqTest.listRuns({ limit: 100 })
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.runFailed'))
  } finally {
    running.value = false
  }
}

const openRunDialog = async (bank: IQTestBank) => {
  runForm.bank_id = bank.id
  runForm.account_id = ''
  runForm.model_id = ''
  runForm.question_id = null
  runQuestions.value = []
  showRunDialog.value = true
  try {
    const detail = await adminAPI.iqTest.getBank(bank.id)
    runQuestions.value = detail.questions || []
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.loadFailed'))
  }
}

const submitRun = async () => {
  if (!runForm.account_id) {
    appStore.showError(t('admin.iqTest.accountRequired'))
    return
  }
  const questionId = runForm.question_id
  if (questionId == null) {
    appStore.showError(t('admin.iqTest.questionRequired'))
    return
  }
  running.value = true
  try {
    const run = await adminAPI.iqTest.runBank({
      bank_id: runForm.bank_id,
      question_id: questionId,
      account_id: Number(runForm.account_id),
      model_id: runForm.model_id
    })
    appStore.showSuccess(`${t('admin.iqTest.runStarted')}: ${run.score}`)
    showRunDialog.value = false
    activeTab.value = 'runs'
    runs.value = await adminAPI.iqTest.listRuns({ limit: 100 })
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.iqTest.runFailed'))
  } finally {
    running.value = false
  }
}

const toggleRunDetails = (runId: number) => {
  if (expandedRuns.has(runId)) expandedRuns.delete(runId)
  else expandedRuns.add(runId)
}

onMounted(loadAll)
</script>
