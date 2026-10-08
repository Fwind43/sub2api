import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import AccountIQTestPanel from '../AccountIQTestPanel.vue'
import { formatDateTime } from '@/utils/format'

const { listBanksMock, listPlansMock, listRunsMock, getAvailableModelsMock, getBankMock } = vi.hoisted(() => ({
  listBanksMock: vi.fn(),
  listPlansMock: vi.fn(),
  listRunsMock: vi.fn(),
  getAvailableModelsMock: vi.fn(),
  getBankMock: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    iqTest: {
      listBanks: listBanksMock,
      listPlansByAccount: listPlansMock,
      listRunsByAccount: listRunsMock,
      getBank: getBankMock
    },
    accounts: {
      getAvailableModels: getAvailableModelsMock
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: { modelValue: { type: [String, Number, Boolean, null], default: '' }, options: { type: Array, default: () => [] } },
  emits: ['update:modelValue'],
  template: '<select><option v-for="o in options" :key="o.value" :value="o.value">{{ o.label }}</option></select>'
})

const StartedAt = '2026-10-07T14:30:05Z'
const FinishedAt = '2026-10-07T14:30:09Z'
const LastRunAt = '2026-10-07T09:00:00Z'
const NextRunAt = '2026-10-07T09:30:00Z'

// mount closed then open -> the component's watch([show, accountId]) fires load()
const mountPanel = async () => {
  const pinia = createPinia()
  setActivePinia(pinia)
  const wrapper = mount(AccountIQTestPanel, {
    props: { show: false, accountId: 1748 },
    global: {
      plugins: [pinia],
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        ConfirmDialog: true,
        Input: true,
        Toggle: true,
        Icon: true
      }
    }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

const buildPlan = (over: Record<string, unknown> = {}) => ({
  id: 7,
  bank_id: 2,
  question_id: 2,
  account_id: 1748,
  model_id: '',
  cron_expression: '0 0/30 * * *',
  enabled: true,
  max_results: 50,
  reasoning_effort: '',
  last_run_at: LastRunAt,
  next_run_at: NextRunAt,
  created_at: LastRunAt,
  updated_at: LastRunAt,
  ...over
})

const buildRun = () => ({
  id: 20,
  plan_id: 7,
  bank_id: 2,
  question_id: 2,
  account_id: 1748,
  model_id: 'gpt-5.4',
  trigger: 'scheduled',
  status: 'success',
  score: 100,
  total: 1,
  correct: 1,
  latency_ms: 1234,
  error_message: '',
  details: null,
  started_at: StartedAt,
  finished_at: FinishedAt,
  created_at: StartedAt
})

describe('AccountIQTestPanel time display', () => {
  beforeEach(() => {
    localStorage.setItem('sub2api_locale', 'en')
    listBanksMock.mockResolvedValue([])
    listPlansMock.mockResolvedValue([buildPlan()])
    listRunsMock.mockResolvedValue([buildRun()])
    getAvailableModelsMock.mockResolvedValue([])
    getBankMock.mockResolvedValue({ id: 2, questions: [] })
  })

  afterEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('renders formatted start/finish time for each run (not raw ISO)', async () => {
    const wrapper = await mountPanel()
    const text = wrapper.text()
    const expectedStart = formatDateTime(StartedAt)
    const expectedFinish = formatDateTime(FinishedAt)

    expect(expectedStart).not.toBe('')
    expect(expectedStart).toContain('2026')

    expect(text).toContain(expectedStart)
    expect(text).toContain(expectedFinish)
    expect(text).not.toContain(StartedAt)
    expect(text).not.toContain(FinishedAt)
  })

  it('renders last/next run time for each plan', async () => {
    const wrapper = await mountPanel()
    const text = wrapper.text()
    const expectedLast = formatDateTime(LastRunAt)
    const expectedNext = formatDateTime(NextRunAt)

    expect(text).toContain(expectedLast)
    expect(text).toContain(expectedNext)
    expect(text).not.toContain(LastRunAt)
    expect(text).not.toContain(NextRunAt)
  })

  it('falls back to dash when a plan has no run history yet', async () => {
    listPlansMock.mockResolvedValue([buildPlan({ last_run_at: null, next_run_at: null })])
    const wrapper = await mountPanel()
    const text = wrapper.text()
    expect(text).toContain('admin.iqTest.lastRun')
    expect(text).toContain('admin.iqTest.nextRun')
    expect(text).not.toContain(LastRunAt)
  })
})
