import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { NavigationGuard } from 'vue-router'

const state = vi.hoisted(() => ({
  guard: undefined as NavigationGuard | undefined,
  auth: {
    checkAuth: vi.fn(), isAuthenticated: true, isAdmin: true,
    isSimpleMode: false, hasPendingAuthSession: false, user: { role: 'admin' },
  },
  app: { siteName: 'Sub2API', backendModeEnabled: false, cachedPublicSettings: null },
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    createRouter: (options: Parameters<typeof actual.createRouter>[0]) => {
      const router = actual.createRouter(options)
      const register = router.beforeEach.bind(router)
      router.beforeEach = (guard) => {
        state.guard = guard
        return register(guard)
      }
      return router
    },
  }
})
vi.mock('@/stores/auth', () => ({ useAuthStore: () => state.auth }))
vi.mock('@/stores/app', () => ({ useAppStore: () => state.app }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn() }),
}))
vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({ triggerPrefetch: vi.fn(), cancelPendingPrefetch: vi.fn(), resetPrefetchState: vi.fn() }),
}))

import router from '../index'

// Invoke the actual registered production guard, not a copied simulation.
async function navigate(path: string) {
  const next = vi.fn()
  await state.guard!.call(undefined, router.resolve(path), router.resolve('/'), next)
  return next.mock.calls[0]?.[0]
}

describe('login deep-link production guard', () => {
  beforeEach(() => {
    Object.assign(state.auth, { isAuthenticated: true, isAdmin: true, isSimpleMode: false, user: { role: 'admin' } })
    state.app.backendModeEnabled = false
  })

  it('resumes a saved admin settings URL rather than the dashboard', async () => {
    const target = '/admin/settings?tab=gateway&name=a%26b#limits'
    expect(await navigate('/login?redirect=' + encodeURIComponent(target))).toBe(target)
    expect(await navigate(target)).toBeUndefined()
  })

  it('preserves a fragment inherited across the HTTP redirect', async () => {
    expect(await navigate('/login?redirect=%2Fadmin%2Fsettings#limits')).toBe('/admin/settings#limits')
  })

  it('does not bypass destination admin authorization', async () => {
    Object.assign(state.auth, { isAdmin: false, user: { role: 'user' } })
    const target = await navigate('/login?redirect=%2Fadmin%2Fsettings')
    expect(target).toBe('/admin/settings')
    expect(await navigate(target)).toBe('/dashboard')
  })

  it('leaves an unauthenticated user on login and retains their destination', async () => {
    state.auth.isAuthenticated = false
    state.auth.isAdmin = false
    expect(await navigate('/admin/settings?tab=gateway')).toEqual({
      path: '/login', query: { redirect: '/admin/settings?tab=gateway' },
    })
    expect(await navigate('/login?redirect=%2Fadmin%2Fsettings')).toBeUndefined()
  })

  it('retains backend-mode restrictions without a redirect loop', async () => {
    state.app.backendModeEnabled = true
    Object.assign(state.auth, { isAdmin: false, user: { role: 'user' } })
    expect(await navigate('/login?redirect=%2Fadmin%2Fsettings')).toBeUndefined()
  })

  it.each(['/login', '/login?redirect=%2Flogin', '/login?redirect=https%3A%2F%2Fevil.example'])('keeps default landing for %s', async (path) => {
    expect(await navigate(path)).toBe('/admin/dashboard')
  })
})
