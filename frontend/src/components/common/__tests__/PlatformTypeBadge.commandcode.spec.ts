import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import PlatformTypeBadge from '../PlatformTypeBadge.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const make = (planType?: string) => mount(PlatformTypeBadge, {
  props: { platform: 'commandcode', type: 'apikey', planType },
  global: { stubs: { PlatformIcon: true, Icon: true, GrokFreeIcon: true } }
})
describe('CommandCode plan badges', () => {
  it('does not infer a plan when none is supplied', () => {
    const wrapper = make()
    expect(wrapper.text()).toMatch(/commandcode/i)
    expect(wrapper.text()).not.toMatch(/GO|Free|Pro|Team/)
  })
  it('renders supplied plan without ChatGPT naming', () => {
    const wrapper = make('pro')
    expect(wrapper.text()).toContain('Pro')
    expect(wrapper.text()).not.toContain('Pro 20x')
  })
  it('reacts to verified plan changes and removal', async () => {
    const wrapper = make()
    await wrapper.setProps({ planType: 'GO' })
    expect(wrapper.text()).toContain('GO')
    await wrapper.setProps({ planType: undefined })
    expect(wrapper.text()).not.toContain('GO')
  })
})
