import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import UserTokenRanking from '../UserTokenRanking.vue'

const getUserSpendingRanking = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin/dashboard', () => ({
  getUserSpendingRanking,
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

describe('UserTokenRanking', () => {
  beforeEach(() => {
    getUserSpendingRanking.mockReset()
    getUserSpendingRanking.mockResolvedValue({
      ranking: [
        { user_id: 7, email: 'alex@example.com', tokens: 1200, requests: 3, actual_cost: 0.12 },
      ],
    })
  })

  it('exposes the user filter action as a keyboard-accessible button', async () => {
    const wrapper = mount(UserTokenRanking, {
      props: {
        active: true,
        startDate: '2026-08-01',
        endDate: '2026-08-29',
      },
    })
    await flushPromises()

    const userButton = wrapper.get('tbody button')
    expect(userButton.attributes('type')).toBe('button')
    expect(userButton.text()).toBe('alex@example.com')

    await userButton.trigger('click')
    expect(wrapper.emitted('userClick')).toEqual([[7]])
  })
})
