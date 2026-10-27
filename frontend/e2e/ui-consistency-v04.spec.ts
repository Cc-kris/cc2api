import { expect, test, type Page } from '@playwright/test'

// TASK-036 / UI Brief R14-R15：登录、404 与主要后台页面的公共动作控件回归。
const BASE = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:3100'
const EMAIL = process.env.S2A_E2E_EMAIL ?? 'e2e-task031@sub2api.local'
const PASSWORD = process.env.S2A_E2E_PASSWORD ?? 'TASK031-e2e-Passw0rd!'
let token = ''

async function login(page: Page) {
  await page.addInitScript(([t, email]) => {
    localStorage.setItem('auth_token', t)
    localStorage.setItem('auth_user', JSON.stringify({ id: 2, email, username: 'e2e-task031', role: 'admin', status: 'active' }))
    localStorage.setItem('token_expires_at', String(Date.now() + 86_400_000))
    localStorage.setItem('admin_guide_2_admin_v4_interactive', 'true')
    localStorage.setItem('admin_guide_guest_user_v4_interactive', 'true')
  }, [token, EMAIL])
}

async function expectPrimary(page: Page, selector: string) {
  const control = page.locator(selector).first()
  await expect(control).toBeVisible({ timeout: 30_000 })
  await expect(control).toHaveCSS('background-color', 'rgb(204, 255, 0)')
  await expect(control).toHaveCSS('color', 'rgb(18, 19, 22)')
  await expect(control).toHaveCSS('border-top-color', 'rgb(18, 19, 22)')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
}

test.beforeAll(async ({ request }) => {
  const response = await request.post(`${BASE}/api/v1/auth/login`, { data: { email: EMAIL, password: PASSWORD } })
  test.skip(!response.ok(), `真实栈不可用：登录 ${response.status()}`)
  token = (await response.json()).data.access_token
})

test.beforeEach(async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
})

test('登录与 404 暗色主按钮保持 Neo-Brutalism 规范', async ({ page }) => {
  await page.goto('/login')
  await expectPrimary(page, 'button[type=submit]')
  await page.goto('/not-found-v04-regression')
  await expectPrimary(page, 'a.btn-primary')
})

test('后台选中 Tab、图表切换和分页不再回退旧蓝色', async ({ page }) => {
  await login(page)
  await page.goto('/admin/settings')
  await expectPrimary(page, '.settings-tab-active')
  await page.goto('/admin/dashboard')
  await expectPrimary(page, 'button.bg-white.shadow-sm')
  await page.goto('/admin/users')
  if ((page.viewportSize()?.width ?? 0) >= 640) {
    const control = page.locator('.neo-page-btn[aria-current=page]').first()
    await expect(control).toBeVisible({ timeout: 30_000 })
    await expect(control).toHaveCSS('border-top-width', '2px')
    await expect(control).toHaveCSS('border-radius', '8px')
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})
