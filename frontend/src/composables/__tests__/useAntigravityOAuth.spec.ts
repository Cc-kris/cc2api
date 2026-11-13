import { describe, expect, it, vi } from 'vitest'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    antigravity: {
      generateAuthUrl: vi.fn(),
      exchangeCode: vi.fn(),
      refreshAntigravityToken: vi.fn()
    }
  }
}))

import { useAntigravityOAuth } from '@/composables/useAntigravityOAuth'

// This builder is shared by OAuth creation, refresh-token import and reauthorization.
describe('Antigravity subscription metadata', () => {
  it.each(['pro', 'ultra', 'free'])('preserves the upstream %s plan when saving credentials', (plan) => {
    const oauth = useAntigravityOAuth()
    const credentials = oauth.buildCredentials({ plan_type: plan } as never)
    expect(credentials.plan_type).toBe(plan)
  })

  it('does not invent a plan when subscription discovery is unavailable', () => {
    const credentials = useAntigravityOAuth().buildCredentials({} as never)
    expect(credentials).not.toHaveProperty('plan_type')
  })
})
