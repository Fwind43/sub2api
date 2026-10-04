import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'

enableAutoUnmount(afterEach)

const mocks = vi.hoisted(() => ({
  updateAccount: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn(), showWarning: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isSimpleMode: true })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getManagementCapabilities: vi.fn().mockResolvedValue({ web_search_enabled: false, account_quota_notify_enabled: false }),
      update: mocks.updateAccount,
      checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false })
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function buildOAuthAccount(patch: Record<string, unknown> = {}) {
  return {
    id: 7, name: 'OpenAI OAuth', notes: '', platform: 'openai', type: 'oauth',
    credentials: { access_token: 'oauth-token', chatgpt_account_id: 'acc' }, extra: {},
    proxy_id: null, concurrency: 1, priority: 1, rate_multiplier: 1, status: 'active',
    group_ids: [], expires_at: null, auto_pause_on_expired: false, parent_account_id: null,
    ...patch
  } as any
}

function mountModal(account = buildOAuthAccount()) {
  return mount(EditAccountModal, {
    props: { show: true, account, proxies: [], groups: [] },
    global: {
      stubs: { BaseDialog: BaseDialogStub, Select: true, Icon: true, ProxySelector: true, GroupSelector: true, ModelWhitelistSelector: true }
    }
  })
}

async function submit(wrapper: ReturnType<typeof mountModal>) {
  await wrapper.get('form#edit-account-form').trigger('submit.prevent')
  await flushPromises()
}

describe('EditAccountModal Prism OAuth switch', () => {
  beforeEach(() => {
    mocks.updateAccount.mockReset()
    mocks.updateAccount.mockImplementation(async (_id: number, payload: Record<string, unknown>) => ({ ...buildOAuthAccount(), ...payload }))
  })

  it('persists the Prism switch while preserving unrelated extra fields', async () => {
    const wrapper = mountModal(buildOAuthAccount({ extra: { fixture_flag: true } }))
    await flushPromises()
    await wrapper.get('[data-testid="openai-prism-browser-oauth-toggle"]').setValue(true)
    await submit(wrapper)
    expect(mocks.updateAccount).toHaveBeenCalledTimes(1)
    expect(mocks.updateAccount.mock.calls[0][1].extra).toMatchObject({ fixture_flag: true, openai_prism_browser: true,
      openai_prism_browser_models: ['gpt-6.1-sol', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-6-luna'] })
  })

  it('removes the flag when disabled', async () => {
    const wrapper = mountModal(buildOAuthAccount({ extra: { openai_prism_browser: true, openai_prism_browser_models: ['gpt-6.1-sol'] } }))
    await flushPromises()
    await wrapper.get('[data-testid="openai-prism-browser-oauth-toggle"]').setValue(false)
    await submit(wrapper)
    expect(mocks.updateAccount.mock.calls[0][1].extra.openai_prism_browser).toBeUndefined()
    expect(mocks.updateAccount.mock.calls[0][1].extra.openai_prism_browser_models).toBeUndefined()
  })

  it('limits legacy accounts to the four supported models and persists a subset', async () => {
    const wrapper = mountModal(buildOAuthAccount({ extra: { openai_prism_browser: true } }))
    await flushPromises()
    expect(wrapper.findAll('[data-testid="prism-model-scope"] input:checked')).toHaveLength(4)
    for (const model of ['gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-6-luna']) {
      await wrapper.get(`[data-testid="prism-model-${model}"]`).setValue(false)
    }
    await submit(wrapper)
    expect(mocks.updateAccount.mock.calls[0][1].extra.openai_prism_browser_models).toEqual(['gpt-6.1-sol'])
  })

  it.each([[], ['gpt-4o-audio-preview'], 'malformed'])('does not widen an empty or invalid scope: %j', async (models) => {
    const wrapper = mountModal(buildOAuthAccount({ extra: { openai_prism_browser: true, openai_prism_browser_models: models } }))
    await flushPromises()
    expect(wrapper.findAll('[data-testid="prism-model-scope"] input:checked')).toHaveLength(0)
    await submit(wrapper)
    expect(mocks.updateAccount.mock.calls[0][1].extra.openai_prism_browser_models).toEqual([])
  })

  it('hides Prism for API-key accounts', async () => {
    const wrapper = mountModal(buildOAuthAccount({ type: 'apikey' }))
    await flushPromises()
    expect(wrapper.find('[data-testid="openai-prism-browser-oauth-toggle"]').exists()).toBe(false)
  })
})
