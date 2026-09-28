import { describe, expect, it, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import { nextTick } from 'vue'
import type { Account, AccountUsageInfo } from '@/types'
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})
vi.mock('@/utils/usageLoadQueue', () => ({ enqueueUsageRequest: vi.fn() }))
import AccountUsageCell from '../AccountUsageCell.vue'

describe('CommandCode monthly credits display', () => {
  it.each([10, 4.25, 0])('renders actual balance %s including zero', async (balance) => {
    const wrapper = shallowMount(AccountUsageCell, { props: {
      account: { id: 987001, platform: 'commandcode', type: 'apikey', credentials: {}, extra: {} } as Account
    } })
    const vm = wrapper.vm as unknown as { usageInfo: Partial<AccountUsageInfo> }
    vm.usageInfo = { commandcode_monthly_credits: balance }
    await nextTick()
    expect(wrapper.text()).toContain(`US$${balance.toFixed(2)}`)
    expect(wrapper.text()).not.toContain('—')
    wrapper.unmount()
  })
  it.each([null, undefined])('does not invent a balance for %s', async (balance) => {
    const wrapper = shallowMount(AccountUsageCell, { props: {
      account: { id: 987002, platform: 'commandcode', type: 'apikey', credentials: {}, extra: {} } as Account
    } })
    const vm = wrapper.vm as unknown as { usageInfo: Partial<AccountUsageInfo> }
    vm.usageInfo = { commandcode_monthly_credits: balance }
    await nextTick()
    expect(wrapper.text()).not.toContain('US$')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.activeQuery')
    wrapper.unmount()
  })
})
