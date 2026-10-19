import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/admin/clinepass', () => ({
  getUpstreamPin: vi.fn(), updateUpstreamPin: vi.fn(), probeUpstreams: vi.fn()
}))
vi.mock('@/api/admin/accounts', () => ({ list: vi.fn() }))

import ClinePassUpstreamPinCard from '../ClinePassUpstreamPinCard.vue'
import { getUpstreamPin, updateUpstreamPin, probeUpstreams, type ClinePassUpstreamPinConfig, type ClinePassUpstreamProbeResult } from '@/api/admin/clinepass'
import { list as listAccounts } from '@/api/admin/accounts'

const wrappers: ReturnType<typeof mount>[] = []
const mountCard = async (config: ClinePassUpstreamPinConfig | null = null) => {
  vi.mocked(getUpstreamPin).mockResolvedValue({ config })
  const wrapper = mount(ClinePassUpstreamPinCard)
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}
const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
const twoModels: ClinePassUpstreamPinConfig = {
  mode: 'strict', match_prefixes: ['deepseek'],
  models: { 'deepseek-v4-pro': { upstream: 'deepseek' }, 'glm-5.3': { upstream: 'z-ai' } }
}

beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(listAccounts).mockResolvedValue({ items: [{ id: 17, name: 'probe account' }], pages: 1 } as Awaited<ReturnType<typeof listAccounts>>)
  vi.mocked(updateUpstreamPin).mockImplementation(async config => ({ config }))
  vi.mocked(probeUpstreams).mockResolvedValue({ model: 'glm-5.3', vercel: ['z-ai'], openrouter: ['z-ai', 'novita'] })
})
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

describe('ClinePass platform model routing editor', () => {
  it('saves multiple independent prefixes and an exact override', async () => {
    const w = await mountCard()
    for (const [model, provider] of [['deepseek*', 'deepseek'], ['glm*', 'z-ai'], ['deepseek-v4.1-flash', 'novita']]) {
      await w.get('[data-testid="pin-add-model"]').trigger('click')
      const row = w.findAll('[data-testid="pin-model-row"]').at(-1)!
      await row.get('[data-testid="pin-model-id"]').setValue(model)
      await row.get('[data-testid="pin-model-upstream"]').setValue(provider)
    }
    await w.get('[data-testid="pin-save"]').trigger('click')
    await flushPromises()
    expect(updateUpstreamPin).toHaveBeenCalledWith(expect.objectContaining({
      models: {
        'deepseek*': expect.objectContaining({ upstream: 'deepseek' }),
        'glm*': expect.objectContaining({ upstream: 'z-ai' }),
        'deepseek-v4.1-flash': expect.objectContaining({ upstream: 'novita' })
      }
    }))
    expect(w.text()).toContain('最长前缀')
  })

  it.each(['*', 'cline-pass/*', 'glm*flash', 'glm**'])('rejects invalid model pattern %s before save or probe', async model => {
    const w = await mountCard({ mode: 'strict', models: { [model]: { upstream: 'a' } } })
    await w.get('[data-testid="pin-save"]').trigger('click')
    await w.get('[data-testid="pin-model-probe"]').trigger('click')
    await flushPromises()
    expect(updateUpstreamPin).not.toHaveBeenCalled()
    expect(probeUpstreams).not.toHaveBeenCalled()
    expect(w.text()).toContain('前缀')
  })

  it('probes a concrete model inside a prefix without changing the saved prefix', async () => {
    const w = await mountCard({ mode: 'strict', models: { 'glm*': { upstream: 'z-ai' } } })
    await w.get('[data-testid="pin-model-probe"]').trigger('click')
    expect(probeUpstreams).not.toHaveBeenCalled()
    await w.get('[data-testid="pin-model-probe-id"]').setValue('deepseek-v4.1-flash')
    await w.get('[data-testid="pin-model-probe"]').trigger('click')
    expect(probeUpstreams).not.toHaveBeenCalled()
    await w.get('[data-testid="pin-model-probe-id"]').setValue('cline-pass/glm-5.3-flash')
    await w.get('[data-testid="pin-model-probe"]').trigger('click')
    await flushPromises()
    expect(probeUpstreams).toHaveBeenCalledWith(17, 'cline-pass/glm-5.3-flash')
    await w.get('[data-testid="pin-save"]').trigger('click')
    await flushPromises()
    expect(updateUpstreamPin).toHaveBeenCalledWith(expect.objectContaining({ models: { 'glm*': expect.objectContaining({ upstream: 'z-ai' }) } }))
  })

  it('blocks duplicate prefix aliases before saving', async () => {
    const w = await mountCard({ mode: 'strict', models: { 'glm*': { upstream: 'a' }, 'cline-pass/glm*': { upstream: 'b' } } })
    await w.get('[data-testid="pin-save"]').trigger('click')
    expect(updateUpstreamPin).not.toHaveBeenCalled()
    expect(w.text()).toContain('模型重复')
  })
  it('adds model-only strict rules and saves separate providers on both pipelines', async () => {
    const w = await mountCard()
    for (const [model, provider] of [['deepseek-v4-pro', 'deepseek'], ['glm-5.3', 'z-ai']]) {
      await w.get('[data-testid="pin-add-model"]').trigger('click')
      const row = w.findAll('[data-testid="pin-model-row"]').at(-1)!
      await row.get('[data-testid="pin-model-id"]').setValue(model)
      await row.get('[data-testid="pin-model-upstream"]').setValue(provider)
    }
    await w.get('[data-testid="pin-save"]').trigger('click')
    await flushPromises()
    expect(updateUpstreamPin).toHaveBeenCalledWith(expect.objectContaining({
      mode: 'strict', upstream: '', models: {
        'deepseek-v4-pro': { mode: 'strict', upstream: 'deepseek', pipelines: 'both' },
        'glm-5.3': { mode: 'strict', upstream: 'z-ai', pipelines: 'both' }
      }
    }))
    expect(w.findAll('[data-testid="pin-model-row"]')).toHaveLength(2)
    expect(w.text()).toContain('已保存平台配置')
  })

  it('probes with the selected account, exposes both pipelines, and only edits the draft', async () => {
    const w = await mountCard(twoModels)
    const row = w.findAll('[data-testid="pin-model-row"]')[1]!
    await row.get('[data-testid="pin-model-probe"]').trigger('click')
    await flushPromises()
    expect(probeUpstreams).toHaveBeenCalledWith(17, 'glm-5.3')
    expect(row.get('[data-testid="pin-model-probe-result"]').text()).toContain('OpenRouter')
    const provider = row.findAll('[data-testid="pin-provider-option"]').find(button => button.text() === 'novita')!
    await provider.trigger('click')
    expect((row.get('[data-testid="pin-model-upstream"]').element as HTMLInputElement).value).toBe('novita')
    expect((row.get('[data-testid="pin-model-mode"]').element as HTMLSelectElement).value).toBe('strict')
    expect(updateUpstreamPin).not.toHaveBeenCalled()
    await w.get('[data-testid="pin-save"]').trigger('click')
    await flushPromises()
    expect(vi.mocked(updateUpstreamPin).mock.calls[0]![0].models?.['glm-5.3']?.upstream).toBe('novita')
  })

  it('probes all models serially and continues after a per-model failure', async () => {
    const first = deferred<ClinePassUpstreamProbeResult>()
    vi.mocked(probeUpstreams).mockReturnValueOnce(first.promise)
    const w = await mountCard(twoModels)
    await w.get('[data-testid="pin-probe-all"]').trigger('click')
    expect(probeUpstreams).toHaveBeenCalledTimes(1)
    expect(w.get('[data-testid="pin-save"]').attributes('disabled')).toBeDefined()
    first.reject(new Error('first relay unavailable'))
    await flushPromises()
    expect(probeUpstreams).toHaveBeenNthCalledWith(2, 17, 'glm-5.3')
    expect(w.text()).toContain('first relay unavailable')
    expect(w.findAll('[data-testid="pin-model-probe-result"]')).toHaveLength(1)
    expect(w.get('[data-testid="pin-save"]').attributes('disabled')).toBeUndefined()
    expect(updateUpstreamPin).not.toHaveBeenCalled()
  })

  it('rejects an obsolete probe response after the model changes', async () => {
    const pending = deferred<ClinePassUpstreamProbeResult>()
    vi.mocked(probeUpstreams).mockReturnValueOnce(pending.promise)
    const w = await mountCard(twoModels)
    const row = w.findAll('[data-testid="pin-model-row"]')[0]!
    await row.get('[data-testid="pin-model-probe"]').trigger('click')
    await row.get('[data-testid="pin-model-id"]').setValue('changed-model')
    pending.resolve({ model: 'deepseek-v4-pro', vercel: ['stale'] })
    await flushPromises()
    expect(row.find('[data-testid="pin-model-probe-result"]').exists()).toBe(false)
    expect(w.get('[data-testid="pin-save"]').attributes('disabled')).toBeUndefined()
  })

  it('preserves advanced fields when loading, editing and saving', async () => {
    const config: ClinePassUpstreamPinConfig = {
      ...twoModels, only: ['default'], order: ['default'], sort: 'cost', pipelines: 'openrouter',
      models: { 'glm-5.3': { only: ['z-ai'], exclude: ['novita'], upstreams: ['z-ai', 'novita'], sort: 'ttft' } }
    }
    const w = await mountCard(config)
    await w.get('[data-testid="pin-save"]').trigger('click')
    await flushPromises()
    expect(updateUpstreamPin).toHaveBeenCalledWith(expect.objectContaining({
      only: ['default'], order: ['default'], sort: 'cost', pipelines: 'openrouter',
      models: { 'glm-5.3': expect.objectContaining({ only: ['z-ai'], exclude: ['novita'], upstreams: ['z-ai', 'novita'], sort: 'ttft' }) }
    }))
  })

  it('blocks bare/prefixed duplicate models before saving', async () => {
    const w = await mountCard({ mode: 'strict', models: { glm: { upstream: 'a' }, 'cline-pass/glm': { upstream: 'b' } } })
    await w.get('[data-testid="pin-save"]').trigger('click')
    expect(updateUpstreamPin).not.toHaveBeenCalled()
    expect(w.text()).toContain('模型重复')
  })

  it('shows raw diagnostics for empty probe results without claiming no channels exist', async () => {
    vi.mocked(probeUpstreams).mockResolvedValue({ model: 'glm-5.3', vercel_raw: 'relay diagnostic' })
    const w = await mountCard(twoModels)
    await w.findAll('[data-testid="pin-model-probe"]')[1]!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('这不等于该模型没有渠道')
    expect(w.text()).toContain('relay diagnostic')
    expect(updateUpstreamPin).not.toHaveBeenCalled()
  })

  it('loads all pages of clinepass accounts', async () => {
    vi.mocked(listAccounts).mockResolvedValueOnce({ items: [{ id: 17, name: 'first' }], pages: 2 } as Awaited<ReturnType<typeof listAccounts>>)
      .mockResolvedValueOnce({ items: [{ id: 18, name: 'second' }], pages: 2 } as Awaited<ReturnType<typeof listAccounts>>)
    const w = await mountCard(twoModels)
    expect(listAccounts).toHaveBeenNthCalledWith(2, 2, 100, { platform: 'clinepass', lite: 'true' })
    await w.get('[data-testid="pin-probe-account"]').setValue(18)
    await w.findAll('[data-testid="pin-model-probe"]')[1]!.trigger('click')
    await flushPromises()
    expect(probeUpstreams).toHaveBeenCalledWith(18, 'glm-5.3')
  })
})
