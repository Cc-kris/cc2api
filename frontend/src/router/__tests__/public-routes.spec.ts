import { describe, expect, it } from 'vitest'
import router from '@/router'

describe('public content routes', () => {
  it('uses the public root as the Home route', () => {
    const root = router.getRoutes().find((route) => route.path === '/')
    expect(root?.name).toBe('Home')
    expect(root?.meta.requiresAuth).toBe(false)
  })

  it('redirects the public /home alias to the canonical root', () => {
    const homeAlias = router.getRoutes().find((route) => route.path === '/home')
    expect(homeAlias?.redirect).toBe('/')
  })

  it.each([
    ['/news', 'PublicNewsList'],
    ['/news/zh-CN/story', 'PublicNewsDetail'],
    ['/articles', 'PublicArticlesList'],
    ['/articles/en-US/guide', 'PublicArticleDetail'],
    ['/docs/api', 'PublicDocs']
  ])('resolves %s as %s without authentication', (path, name) => {
    const resolved = router.resolve(path)
    expect(resolved.name).toBe(name)
    expect(resolved.meta.requiresAuth).toBe(false)
  })

  it.each(['/model-square', '/dashboard', '/keys', '/admin/dashboard'])('keeps %s protected', (path) => {
    const resolved = router.resolve(path)
    expect(resolved.meta.requiresAuth).not.toBe(false)
  })
})
