import { describe, expect, it } from 'vitest'
import { resolveLoginRedirect } from '../loginRedirect'

describe('resolveLoginRedirect', () => {
  const fallback = '/admin/dashboard'

  it('preserves private deep links, query values and fragments', () => {
    expect(resolveLoginRedirect('/admin/settings?tab=gateway&name=a%26b#limits', fallback))
      .toBe('/admin/settings?tab=gateway&name=a%26b#limits')
    expect(resolveLoginRedirect('/admin/accounts?page=2', fallback, '#filters'))
      .toBe('/admin/accounts?page=2#filters')
  })

  it.each([
    undefined, null, '', ['/admin/settings'], 'https://evil.example', '//evil.example',
    '/\\evil.example', '/%2f%2fevil.example', '/%5cevil.example', '/admin/\nsettings',
    '/%0dsettings', '/%zz', '/login', '/login/?redirect=/admin/settings', '/register#form',
    '/LOGIN', '/%6cogin', '/admin/../login',
  ])('rejects unsafe or looping target %j', (value) => {
    expect(resolveLoginRedirect(value, fallback)).toBe(fallback)
  })

  it('uses the caller fallback without changing role-specific landing behavior', () => {
    expect(resolveLoginRedirect(undefined, '/dashboard')).toBe('/dashboard')
  })
})
