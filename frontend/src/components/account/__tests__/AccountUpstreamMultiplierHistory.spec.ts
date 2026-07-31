import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { listMock } = vi.hoisted(() => ({ listMock: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      listUpstreamMultiplierChanges: listMock
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import AccountUpstreamMultiplierHistory from '../AccountUpstreamMultiplierHistory.vue'

describe('AccountUpstreamMultiplierHistory', () => {
  it('loads the latest five changes when expanded', async () => {
    listMock.mockResolvedValue({
      items: [{
        id: 1,
        account_id: 9,
        old_multiplier: '1.0000',
        new_multiplier: '1.2500',
        change_type: 'update',
        effective_at: '2026-07-26T00:00:00Z',
        operator_id: 2,
        operator_name: 'admin',
        reason: '上游价格调整',
        created_at: '2026-07-26T00:00:00Z'
      }],
      total: 1,
      page: 1,
      page_size: 5,
      pages: 1
    })

    const wrapper = mount(AccountUpstreamMultiplierHistory, {
      props: { accountId: 9 },
      global: { stubs: { BaseDialog: true } }
    })
    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(listMock).toHaveBeenCalledWith(9, 1, 5)
    expect(wrapper.text()).toContain('1.2500')
    expect(wrapper.text()).toContain('上游价格调整')
  })
})
