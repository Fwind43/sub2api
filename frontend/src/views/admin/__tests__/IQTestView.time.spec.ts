import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import IQTestView from '../IQTestView.vue'
import { formatDateTime } from '@/utils/format'

const { listBanksMock, listRunsMock } = vi.hoisted(() => ({
  listBanksMock: vi.fn(),
  listRunsMock: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    iqTest: {
      listBanks: listBanksMock,
      listRuns: listRunsMock,
      getBank: vi.fn().mockResolvedValue({ id: 2, questions: [] }),
      runBank: vi.fn()
    },
    accounts: {
      getAvailableModels: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const AppLayoutStub = defineComponent({ name: 'AppLayout', template: '<div><slot /></div>' })

const StartedAt = '2026-10-07T14:30:05Z'
const FinishedAt = '2026-10-07T14:30:09Z'

const mountView = async () => {
  const pinia = createPinia()
  setActivePinia(pinia)
  const wrapper = mount(IQTestView, {
    global: {
      plugins: [pinia],
      stubs: {
        AppLayout: AppLayoutStub,
        BaseDialog: true,
        ConfirmDialog: true,
        Input: true,
        Select: true,
        Toggle: true,
        Icon: true
      }
    }
  })
  await flushPromises()
  return wrapper
}

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

describe('IQTestView run time display', () => {
  beforeEach(() => {
    localStorage.setItem('sub2api_locale', 'en')
    listBanksMock.mockResolvedValue([])
    listRunsMock.mockResolvedValue([buildRun()])
  })

  afterEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('shows a formatted Started At value for each run (not raw ISO)', async () => {
    const wrapper = await mountView()
    // switch to the runs tab
    const vm = wrapper.vm as unknown as { activeTab: string }
    vm.activeTab = 'runs'
    await flushPromises()

    const text = wrapper.text()
    const expected = formatDateTime(StartedAt)
    expect(expected).toContain('2026')
    expect(text).toContain(expected)
    expect(text).not.toContain(StartedAt)
  })

  it('renders the Finished At column header and value', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as unknown as { activeTab: string }
    vm.activeTab = 'runs'
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('admin.iqTest.finishedAt')
    const expectedFinish = formatDateTime(FinishedAt)
    expect(expectedFinish).toContain('2026')
    expect(text).toContain(expectedFinish)
    expect(text).not.toContain(FinishedAt)
  })
})
