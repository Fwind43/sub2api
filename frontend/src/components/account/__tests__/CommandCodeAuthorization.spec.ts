import { mount } from '@vue/test-utils'
import { describe, it, expect, vi } from 'vitest'
import CommandCodeAuthorization from '../CommandCodeAuthorization.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
async function start() {
  const wrapper = mount(CommandCodeAuthorization)
  await wrapper.get('button').trigger('click')
  const state = wrapper.get('[data-testid="commandcode-receiver-command"]').text().split(' ').at(-1)!
  const callback = { state, apiKey: 'synthetic-test-key' }
  return { wrapper, callback }
}
describe('CommandCode GO authorization', () => {
  it('receives a matching callback once and clears sensitive input', async () => {
    const { wrapper, callback } = await start()
    expect(wrapper.get('input').attributes('type')).toBe('password')
    await wrapper.get('input').setValue(JSON.stringify(callback))
    await wrapper.findAll('button')[1].trigger('click')
    expect(wrapper.emitted('authorized')).toHaveLength(1)
    expect(wrapper.emitted('authorized')![0][0]).toMatchObject({ apiKey: 'synthetic-test-key' })
    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.get('[role="status"]').text()).toContain('accepted')
    expect(wrapper.html()).not.toContain('synthetic-test-key')
    wrapper.unmount()
  })
  it('rejects mismatched state without emitting credentials', async () => {
    const { wrapper, callback } = await start()
    callback.state = 'wrong'
    await wrapper.get('input').setValue(JSON.stringify(callback))
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('authorized')).toBeUndefined()
    expect(wrapper.get('[role="alert"]').text()).toContain('errors.state')
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('')
    wrapper.unmount()
  })
  it('cancels pending authorization and rotates state on restart', async () => {
    const { wrapper, callback } = await start()
    await wrapper.get('input').setValue(JSON.stringify(callback))
    await wrapper.findAll('button')[2].trigger('click')
    expect(wrapper.find('input').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    const command = wrapper.get('[data-testid="commandcode-receiver-command"]').text()
    expect(command).not.toContain(callback.state)
    expect(wrapper.get('a').attributes('download')).toBe('commandcode-login.mjs')
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('')
    expect(wrapper.emitted('authorized')).toBeUndefined()
    wrapper.unmount()
  })
})
