import { expect, test } from '@playwright/test'

const admin = { id: 1, username: 'admin', email: 'admin@ccai.test', role: 'admin', status: 'active' }

test.describe('CCAI content center admin flow', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript((user) => {
      localStorage.setItem('auth_token', 'e2e-admin-token')
      localStorage.setItem('auth_user', JSON.stringify(user))
      localStorage.setItem('sub2api_locale', 'zh')
      ;(window as any).__APP_CONFIG__ = { site_name: 'CCAI', backend_mode_enabled: false, model_square_enabled: true }
    }, admin)
    await page.route('**/api/v1/auth/me*', async (route) => route.fulfill({ json: { code: 0, data: admin } }))
    await page.route('**/api/v1/**', async (route) => {
      const path = new URL(route.request().url()).pathname
      if (path.endsWith('/news-sources')) return route.fulfill({ json: { code: 0, data: { items: [{ id: 7, name: 'CCAI Feed', sourceType: 'public_json', language: 'zh-CN', enabled: false, mappingJson: { title: '$.headline' }, intervalMinutes: 15 }], total: 1, hasNext: false } } })
      if (path.endsWith('/automation-settings')) return route.fulfill({ json: { code: 0, data: { enabled: false, timezone: 'Asia/Shanghai', fetchCron: '0 */15 * * * *', publishMode: 'manual_review', dailyFetchLimit: 0, dailyPublishLimit: 0 } } })
      if (path.includes('/news') || path.includes('/articles') || path.includes('/fetch-runs') || path.includes('/analytics')) return route.fulfill({ json: { code: 0, data: { items: [], total: 0, hasNext: false, overview: {}, trends: [], rankings: [], ctr: [] } } })
      return route.fulfill({ json: { code: 0, data: {} } })
    })
  })

  test('opens news, article and BI tabs and validates mapping input', async ({ page }) => {
    await page.goto('/admin/content')
    await expect(page.getByRole('heading', { name: '内容中心' })).toBeVisible()
    await page.getByRole('button', { name: '新闻源' }).click()
    await expect(page.getByText('CCAI Feed')).toBeVisible()
    await page.locator('select').first().selectOption('public_json')
    await page.getByPlaceholder(/JSON 字段映射/).fill('{bad')
    await page.getByRole('button', { name: '新增新闻源' }).click()
    await expect(page.getByText('JSON 字段映射格式无效，请输入合法 JSON')).toBeVisible()
    await page.getByRole('button', { name: '文章' }).click()
    await expect(page.getByRole('button', { name: '文章' })).toBeVisible()
    await page.getByRole('button', { name: 'BI 分析' }).click()
    await expect(page.getByText('BI 分析')).toBeVisible()
  })
})
