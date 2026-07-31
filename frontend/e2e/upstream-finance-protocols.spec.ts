import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

const admin = { id: 1, username: 'owner', email: 'owner@example.com', role: 'admin', balance: 0, status: 'active' }
const config = {
  capabilities: ['account_usage', 'funding_transactions'],
  authentication: { type: 'bearer', credential_source: 'account_api_key' },
  operations: {
    account_usage: { method: 'GET', path: '/v1/usage', mapping: { list_cost: '$.list_cost', actual_cost: '$.actual_cost' } },
    funding_transactions: { method: 'GET', path: '/v1/funds', mapping: { transactions: '$.data' } },
  },
  cost_mode: 'cumulative_list_and_actual', unit_semantics: 'fiat_currency', counter_scope: 'account', redact_paths: ['$.token'],
}

async function fulfill(route: Route, data: unknown) { await route.fulfill({ json: { code: 0, message: 'success', data } }) }

async function mockProtocols(page: Page) {
  await page.addInitScript((savedAdmin) => {
    localStorage.setItem('auth_token', 'protocol-e2e-token')
    localStorage.setItem('auth_user', JSON.stringify(savedAdmin))
    localStorage.setItem('admin_guide_1_admin_v4_interactive', 'true')
    ;(window as any).__APP_CONFIG__ = { site_name: 'CCAI', version: 'e2e', model_square_enabled: true, custom_menu_items: [] }
  }, admin)
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/auth/me')) return fulfill(route, admin)
    if (path.endsWith('/admin/upstream-finance-protocols') && route.request().method() === 'GET') return fulfill(route, { items: [{ id: 7, code: 'vendor_x', name: 'Vendor X', protocol_type: 'http_json', status: 'published', current_version_id: 71, current_version: { id: 71, protocol_id: 7, version: 1, config, validation_status: 'valid' }, updated_at: '2026-07-30T00:00:00Z' }], total: 1, page: 1, page_size: 100 })
    if (path.endsWith('/admin/upstream-finance-protocols/7/versions')) return fulfill(route, [{ id: 71, protocol_id: 7, version: 1, config, validation_status: 'valid', published_at: '2026-07-30T00:00:00Z' }])
    if (path.endsWith('/admin/upstream-finance-protocols/7/copy')) return fulfill(route, { id: 8, code: 'vendor_x_copy', name: 'Vendor X 副本', protocol_type: 'http_json', status: 'draft', current_version: { id: 81, protocol_id: 8, version: 1, config, validation_status: 'valid' }, updated_at: '2026-07-30T00:00:00Z' })
    return fulfill(route, {})
  })
}

test.describe('Upstream finance protocols', () => {
  test.beforeEach(async ({ page }) => { await mockProtocols(page) })

  test('copies and edits a generic protocol through structured fields', async ({ page }) => {
    await page.goto('/admin/upstreams/finance-protocols')
    await expect(page.getByRole('heading', { name: '上游财务协议' })).toBeVisible({ timeout: 15_000 })
    await expect(page.getByText('Vendor X')).toBeVisible()
    await page.getByRole('button', { name: '复制为草稿' }).click()

    await expect(page.getByText('计费语义与能力')).toBeVisible()
    await expect(page.getByRole('heading', { name: '账号累计费用' })).toBeVisible()
    await expect(page.getByRole('heading', { name: '充值交易' })).toBeVisible()
    await expect(page.getByText('不填写或保存真实密钥')).toBeVisible()
    await expect(page.locator('input[type="password"]')).toHaveCount(0)

    const copyRequest = page.waitForRequest(request => request.url().endsWith('/api/v1/admin/upstream-finance-protocols/7/copy'))
    await page.getByRole('button', { name: '保存草稿' }).click()
    const request = await copyRequest
    const payload = request.postDataJSON()
    expect(payload.code).toBe('vendor_x_copy')
    expect(payload.config.authentication.credential_source).toBe('account_api_key')
    expect(payload.config.counter_scope).toBe('account')
    expect(JSON.stringify(payload)).not.toContain('protocol-e2e-token')

    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze()
    expect(results.violations.filter(item => ['critical', 'serious'].includes(item.impact || ''))).toEqual([])
  })
})
