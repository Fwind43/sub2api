import { mount } from '@vue/test-utils'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { nextTick } from 'vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/clinepass', () => ({ probeUpstreams: vi.fn(), listLastKnown: vi.fn() }))

import ClinePassPinSettings from '../ClinePassPinSettings.vue'
import { listLastKnown } from '@/api/admin/clinepass'

const settle = async () => {
  await nextTick()
  await nextTick()
}

function mountPin(initial: Record<string, unknown> | null) {
  const wrapper = mount(ClinePassPinSettings, {
    props: {
      accountId: 1,
      modelValue: initial,
      'onUpdate:modelValue': (value: Record<string, unknown> | null) => {
        wrapper.setProps({ modelValue: value })
      }
    },
    global: {
      stubs: { Select: true }
    }
  })
  return wrapper
}

describe('ClinePassPinSettings model overrides', () => {
  it('keeps a new empty row after clicking add', async () => {
    const wrapper = mountPin({ mode: 'strict' })
    await settle()
    expect(wrapper.findAll('[data-testid="clinepass-pin-model-row"]')).toHaveLength(0)

    await wrapper.get('[data-testid="clinepass-pin-model-add"]').trigger('click')
    await settle()

    expect(wrapper.findAll('[data-testid="clinepass-pin-model-row"]')).toHaveLength(1)
    wrapper.unmount()
  })

  it('keeps in-progress typing and emits the canonical value', async () => {
    const wrapper = mountPin({ mode: 'strict' })
    await settle()
    await wrapper.get('[data-testid="clinepass-pin-model-add"]').trigger('click')
    await settle()

    const inputs = wrapper.get('[data-testid="clinepass-pin-model-row"]').findAll('input')
    await inputs[0].setValue('deepseek-v4-pro')
    await settle()
    await inputs[1].setValue('morph')
    await settle()

    const rows = wrapper.findAll('[data-testid="clinepass-pin-model-row"]')
    expect(rows).toHaveLength(1)
    const kept = rows[0].findAll('input')
    expect((kept[0].element as HTMLInputElement).value).toBe('deepseek-v4-pro')
    expect((kept[1].element as HTMLInputElement).value).toBe('morph')

    const emitted = wrapper.emitted('update:modelValue') as Array<[Record<string, unknown> | null]>
    expect(emitted[emitted.length - 1][0]).toEqual({
      mode: 'strict',
      models: { 'deepseek-v4-pro': { upstream: 'morph' } }
    })
    wrapper.unmount()
  })

  it('adds another row when rows already exist', async () => {
    const wrapper = mountPin({ mode: 'strict', models: { 'x-model': { upstream: 'morph' } } })
    await settle()
    expect(wrapper.findAll('[data-testid="clinepass-pin-model-row"]')).toHaveLength(1)

    await wrapper.get('[data-testid="clinepass-pin-model-add"]').trigger('click')
    await settle()

    expect(wrapper.findAll('[data-testid="clinepass-pin-model-row"]')).toHaveLength(2)
    wrapper.unmount()
  })

  it('keeps a row being filled from the upstream side first', async () => {
    const wrapper = mountPin({ mode: 'strict' })
    await settle()
    await wrapper.get('[data-testid="clinepass-pin-model-add"]').trigger('click')
    await settle()

    const inputs = wrapper.get('[data-testid="clinepass-pin-model-row"]').findAll('input')
    await inputs[1].setValue('morph')
    await settle()

    // Row still visible even though the model name is not filled yet.
    expect(wrapper.findAll('[data-testid="clinepass-pin-model-row"]')).toHaveLength(1)

    await inputs[0].setValue('x-model')
    await settle()

    const emitted = wrapper.emitted('update:modelValue') as Array<[Record<string, unknown> | null]>
    expect(emitted[emitted.length - 1][0]).toEqual({
      mode: 'strict',
      models: { 'x-model': { upstream: 'morph' } }
    })
    wrapper.unmount()
  })

  it('re-hydrates when the external value changes', async () => {
    const wrapper = mountPin({ mode: 'strict' })
    await settle()
    await wrapper.setProps({ modelValue: { mode: 'strict', models: { 'x-model': { upstream: 'morph' } } } })
    await settle()

    const rows = wrapper.findAll('[data-testid="clinepass-pin-model-row"]')
    expect(rows).toHaveLength(1)
    const kept = rows[0].findAll('input')
    expect((kept[0].element as HTMLInputElement).value).toBe('x-model')
    expect((kept[1].element as HTMLInputElement).value).toBe('morph')
    wrapper.unmount()
  })
})

describe('ClinePassPinSettings last-known upstream', () => {
  beforeEach(() => {
    vi.mocked(listLastKnown).mockReset()
  })

  it('renders the last actually-hit upstream with its timestamp', async () => {
    vi.mocked(listLastKnown).mockResolvedValue({
      account_id: 1,
      items: [
        {
          model: 'glm-5.3-flash',
          provider: 'deepinfra',
          pipeline: 'planner',
          canonical_slug: 'z-ai/glm-5.3-flash',
          fallbacks: ['novita', 'wafer'],
          observed_at: '2026-10-08T09:00:00Z'
        }
      ]
    })
    const wrapper = mountPin({ mode: 'strict', upstream: 'deepinfra' })
    await settle()
    await settle()

    const rows = wrapper.findAll('[data-testid="clinepass-lastknown-row"]')
    expect(rows).toHaveLength(1)
    expect(rows[0].text()).toContain('glm-5.3-flash')
    expect(rows[0].text()).toContain('deepinfra')
    expect(rows[0].text()).toContain('novita, wafer')
    // Pin expectation matches the observed upstream.
    expect(wrapper.get('[data-testid="clinepass-lastknown-verdict"]').text()).toBe(
      'admin.accounts.clinepassPin.lastKnownMatch'
    )
    wrapper.unmount()
  })

  it('flags a mismatch against the configured pin', async () => {
    vi.mocked(listLastKnown).mockResolvedValue({
      account_id: 1,
      items: [
        { model: 'glm-5.3-flash', provider: 'novita', pipeline: 'direct', observed_at: '2026-10-08T09:00:00Z' }
      ]
    })
    const wrapper = mountPin({ mode: 'strict', upstream: 'deepinfra' })
    await settle()
    await settle()

    expect(wrapper.get('[data-testid="clinepass-lastknown-verdict"]').text()).toBe(
      'admin.accounts.clinepassPin.lastKnownMismatch'
    )
    wrapper.unmount()
  })

  it('shows the empty state when nothing was observed yet', async () => {
    vi.mocked(listLastKnown).mockResolvedValue({ account_id: 1, items: [] })
    const wrapper = mountPin({ mode: 'off' })
    await settle()
    await settle()

    expect(wrapper.findAll('[data-testid="clinepass-lastknown-row"]')).toHaveLength(0)
    expect(wrapper.text()).toContain('admin.accounts.clinepassPin.lastKnownEmpty')
    wrapper.unmount()
  })
})
