import { mount, flushPromises } from '@vue/test-utils'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/api/admin/clinepass', () => ({ getUpstreamPin: vi.fn(), listLastKnown: vi.fn() }))

import ClinePassPinSettings from '../ClinePassPinSettings.vue'
import { getUpstreamPin, listLastKnown } from '@/api/admin/clinepass'

const observation = {
  model: 'cline-pass/deepseek-v4-pro', provider: 'deepseek', pipeline: 'planner',
  canonical_slug: 'deepseek/deepseek-v4-pro', fallbacks: ['novita', 'wafer'],
  observed_at: '2026-10-08T09:00:00Z'
}
const wrappers: ReturnType<typeof mount>[] = []
const mountStatus = (accountId = 1) => {
  const wrapper = mount(ClinePassPinSettings, { props: { accountId } })
  wrappers.push(wrapper)
  return wrapper
}
const verdict = 'admin.accounts.clinepassPin.lastKnown'

describe('ClinePass platform-only routing diagnostics', () => {
  it.each([
    ['deepseek-v3', 'deepseek', true],
    ['cline-pass/deepseek-v4-pro', 'novita', true],
    ['deepseek-v4.1-flash', 'exact', true],
    ['deepseek-v4.1-flash', 'novita', false],
    ['cline-pass/glm-5.3-flash', 'z-ai', true],
    ['glm-disabled-test', 'z-ai', null],
    ['cline-free/glm-5.3-flash', 'z-ai', null]
  ])('uses the same prefix priority for %s / %s', async (model, provider, matched) => {
    vi.mocked(getUpstreamPin).mockResolvedValue({ config: {
      mode: 'strict', match_prefixes: ['fallback'], models: {
        'deepseek*': { upstream: 'deepseek' }, 'deepseek-v4*': { upstream: 'novita' },
        'deepseek-v4.1-flash': { upstream: 'exact' }, 'cline-pass/glm*': { upstream: 'z-ai' },
        'glm-disabled*': { mode: 'off' }
      }
    } })
    vi.mocked(listLastKnown).mockResolvedValue({ account_id: 1, items: [{ ...observation, model, provider }] })
    const wrapper = mountStatus()
    await flushPromises()
    if (matched === null) {
      expect(wrapper.find('[data-testid="clinepass-lastknown-verdict"]').exists()).toBe(false)
    } else {
      expect(wrapper.get('[data-testid="clinepass-lastknown-verdict"]').text()).toBe(verdict + (matched ? 'Match' : 'Mismatch'))
    }
  })
  beforeEach(() => {
    vi.resetAllMocks()
    vi.mocked(getUpstreamPin).mockResolvedValue({
      config: { mode: 'strict', upstream: 'deepseek', match_prefixes: ['deepseek'] }
    })
    vi.mocked(listLastKnown).mockResolvedValue({ account_id: 1, items: [observation] })
  })
  afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

  it('has no account-level pin editor or update event', async () => {
    const wrapper = mountStatus()
    await flushPromises()
    expect(wrapper.find('[data-testid="clinepass-platform-only"]').exists()).toBe(true)
    expect(wrapper.findAll('input, select, textarea')).toHaveLength(0)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(getUpstreamPin).toHaveBeenCalledTimes(1)
    expect(listLastKnown).toHaveBeenCalledWith(1)
  })

  it('compares actual hits against platform settings and preserves metadata', async () => {
    const wrapper = mountStatus()
    await flushPromises()
    const row = wrapper.get('[data-testid="clinepass-lastknown-row"]')
    expect(row.text()).toContain(observation.model)
    expect(row.text()).toContain('deepseek')
    expect(row.text()).toContain('novita, wafer')
    expect(row.text()).toContain('2026')
    expect(wrapper.get('[data-testid="clinepass-lastknown-verdict"]').text()).toBe(verdict + 'Match')
  })

  it('flags a mismatch against the platform upstream', async () => {
    vi.mocked(listLastKnown).mockResolvedValue({ account_id: 1, items: [{ ...observation, provider: 'morph' }] })
    const wrapper = mountStatus()
    await flushPromises()
    expect(wrapper.get('[data-testid="clinepass-lastknown-verdict"]').text()).toBe(verdict + 'Mismatch')
  })

  it.each([null, { mode: 'off' as const }])('does not report pinning when platform is %j', async config => {
    vi.mocked(getUpstreamPin).mockResolvedValue({ config })
    const wrapper = mountStatus()
    await flushPromises()
    expect(wrapper.find('[data-testid="clinepass-lastknown-verdict"]').exists()).toBe(false)
  })

  it.each(['glm-5.3', 'cline-free/deepseek-v4-pro'])('does not compare an unpinned model %s', async model => {
    vi.mocked(listLastKnown).mockResolvedValue({ account_id: 1, items: [{ ...observation, model }] })
    const wrapper = mountStatus()
    await flushPromises()
    expect(wrapper.find('[data-testid="clinepass-lastknown-verdict"]').exists()).toBe(false)
  })

  it('compares an explicit model rule outside the default prefix gate', async () => {
    vi.mocked(getUpstreamPin).mockResolvedValue({ config: {
      mode: 'strict', upstream: 'deepseek', match_prefixes: ['deepseek'],
      models: { 'cline-pass/glm-5.3': { upstream: 'z-ai' } }
    } })
    vi.mocked(listLastKnown).mockResolvedValue({
      account_id: 1, items: [{ ...observation, model: 'glm-5.3', provider: 'z-ai' }]
    })
    const wrapper = mountStatus()
    await flushPromises()
    expect(wrapper.get('[data-testid="clinepass-lastknown-verdict"]').text()).toBe(verdict + 'Match')
  })

  it('uses platform per-model overrides and explicit allow lists', async () => {
    vi.mocked(getUpstreamPin).mockResolvedValue({ config: {
      mode: 'preferred', upstream: 'morph', models: { 'deepseek-v4-pro': { only: ['morph', 'deepseek'] } }
    } })
    const wrapper = mountStatus()
    await flushPromises()
    expect(wrapper.get('[data-testid="clinepass-lastknown-verdict"]').text()).toBe(verdict + 'Match')
  })

  it('follows strict upstream priority even when an explicit allow list differs', async () => {
    vi.mocked(getUpstreamPin).mockResolvedValue({ config: {
      mode: 'strict', upstream: 'morph', only: ['deepseek']
    } })
    const wrapper = mountStatus()
    await flushPromises()
    expect(wrapper.get('[data-testid="clinepass-lastknown-verdict"]').text()).toBe(verdict + 'Mismatch')
  })

  it('honors a platform per-model off override', async () => {
    vi.mocked(getUpstreamPin).mockResolvedValue({ config: {
      mode: 'strict', upstream: 'deepseek', models: { 'deepseek-v4-pro': { mode: 'off' } }
    } })
    const wrapper = mountStatus()
    await flushPromises()
    expect(wrapper.find('[data-testid="clinepass-lastknown-verdict"]').exists()).toBe(false)
  })

  it('refreshes the platform rule along with observations', async () => {
    const wrapper = mountStatus()
    await flushPromises()
    vi.mocked(getUpstreamPin).mockResolvedValue({ config: { mode: 'strict', upstream: 'morph' } })
    await wrapper.get('[data-testid="clinepass-lastknown-refresh"]').trigger('click')
    await flushPromises()
    expect(getUpstreamPin).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="clinepass-lastknown-verdict"]').text()).toBe(verdict + 'Mismatch')
  })

  it('renders an empty state when no hit has been observed', async () => {
    vi.mocked(listLastKnown).mockResolvedValue({ account_id: 1, items: [] })
    const wrapper = mountStatus()
    await flushPromises()
    expect(wrapper.findAll('[data-testid="clinepass-lastknown-row"]')).toHaveLength(0)
    expect(wrapper.text()).toContain(verdict + 'Empty')
  })

  it('reports a platform read failure rather than a misleading verdict', async () => {
    vi.mocked(getUpstreamPin).mockRejectedValue(new Error('platform unavailable'))
    const wrapper = mountStatus()
    await flushPromises()
    expect(wrapper.get('[data-testid="clinepass-lastknown-error"]').text()).toBe('platform unavailable')
    expect(wrapper.find('[data-testid="clinepass-lastknown-verdict"]').exists()).toBe(false)
  })

  it('ignores stale results when switching accounts', async () => {
    let resolveOld!: (value: Awaited<ReturnType<typeof listLastKnown>>) => void
    vi.mocked(listLastKnown).mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    const wrapper = mountStatus()
    await wrapper.setProps({ accountId: 2 })
    await flushPromises()
    resolveOld({ account_id: 1, items: [{ ...observation, provider: 'stale-provider' }] })
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale-provider')
    expect(listLastKnown).toHaveBeenLastCalledWith(2)
  })
})
