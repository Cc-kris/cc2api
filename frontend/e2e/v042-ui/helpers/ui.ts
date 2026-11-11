import { expect, type Page } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
export const BASE = 'http://127.0.0.1:8280'
export const ADMIN_EMAIL = 'admin@v042e2e.local'
export const ADMIN_PASSWORD = 'V042-e2e-Admin-Passw0rd!'
export const USER_EMAIL = 'e2e-ac01@v042e2e.local'
export const USER_PASSWORD = 'E2e-Passw0rd!'
export async function login(page: Page, email: string, password: string, locale: 'zh-CN'|'en-US' = 'zh-CN') {
  await page.addInitScript((l) => {
    localStorage.setItem('sub2api_locale', l)
    // Tours are unrelated to this acceptance flow and otherwise intercept real
    // controls in fresh admin contexts, especially on mobile.
    const getItem = Storage.prototype.getItem
    Storage.prototype.getItem = function (key) {
      if (/^(admin_guide|user_guide)_/.test(String(key))) return 'true'
      return getItem.call(this, key)
    }
  }, locale === 'zh-CN' ? 'zh' : 'en')
  await page.goto(BASE + '/login', { waitUntil: 'domcontentloaded' })
  // An authenticated session can redirect /login to the dashboard before the form renders.
  await Promise.race([
    page.locator('#email').waitFor({ state: 'visible', timeout: 15000 }).catch(() => null),
    page.waitForURL(u => !u.pathname.startsWith('/login'), { timeout: 15000 }).catch(() => null),
  ])
  if (!new URL(page.url()).pathname.startsWith('/login')) {
    await page.locator('main').waitFor({ state: 'visible' })
    return
  }
  await page.locator('#email').fill(email, { timeout: 15000 })
  await page.locator('#password').fill(password)
  await page.locator('button[type="submit"]').click()
  const throttled = await Promise.race([
    page.waitForURL(u => !u.pathname.startsWith('/login')).then(() => false),
    page.getByText(/Too many requests|请求过于频繁/).waitFor({ state: 'visible' }).then(() => true),
  ]).catch(() => false)
  if (throttled) {
    await page.waitForTimeout(61000)
    await page.locator('button[type="submit"]').click()
  }
  await page.waitForURL(u => !u.pathname.startsWith('/login'))
  await page.locator('main').waitFor({ state: 'visible' })
}
export async function navigate(page: Page, path: string) {
  await page.goto(BASE + path, {waitUntil:'domcontentloaded'}).catch(() => {})
  await page.waitForTimeout(300)
  if (new URL(page.url()).pathname === path) {
    await closeMobileDrawer(page)
    return
  }
  const link = page.locator(`a[href="${path}"]`).first()
  const parentLabels: Record<string, RegExp> = {
    '/admin/channels/pricing': /^渠道管理$|^Channels$/,
    '/admin/orders': /^订单管理$|^Orders$/,
  }
  const parent = parentLabels[path] ? page.getByRole('button', { name: parentLabels[path] }) : null
  const inViewport = async () => {
    const box = await (parent || link).boundingBox().catch(() => null)
    const width = page.viewportSize()?.width ?? 1440
    return !!box && box.x + box.width > 0 && box.x < width
  }
  if (!(await inViewport())) {
    const menu = page.getByRole('button', {name:/Toggle menu|切换菜单|菜单/}).first()
    if (await menu.isVisible().catch(() => false)) await menu.click()
    await expect.poll(inViewport, { timeout: 5000 }).toBe(true)
  }
  if (parent) {
    await parent.click({ timeout: 10000 })
  }
  await link.click({ timeout: 10000 })
  await page.waitForURL(u => u.pathname === path)
  await closeMobileDrawer(page)
}
async function closeMobileDrawer(page: Page) {
  const overlay = page.locator('.fixed.inset-0.z-30')
  if (await overlay.isVisible().catch(() => false)) {
    // The sidebar covers the left of the backdrop. Click the exposed right
    // edge through its real closeMobile handler, not the header toggle below it.
    const box = await overlay.boundingBox()
    if (!box) throw new Error('Visible sidebar backdrop has no bounding box')
    await overlay.click({ position: { x: box.width - 8, y: box.height / 2 } })
    await expect(overlay).toBeHidden()
  }
  if ((page.viewportSize()?.width ?? 1440) < 1024) {
    // The backdrop fade can finish before the sidebar slide. Do not capture
    // evidence or interact with content while that transition is still running.
    await expect.poll(async () => {
      const box = await page.getByRole('complementary').boundingBox()
      return !box || box.x + box.width <= 0
    }).toBe(true)
  }
}
export async function screenshot(page: Page, testInfo: any, name: string) {
  const body = await page.screenshot({fullPage:true, animations:'disabled'})
  await testInfo.attach(name, {body, contentType:'image/png'})
  const dir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../shots', testInfo.project.name)
  await mkdir(dir, { recursive: true })
  await writeFile(path.join(dir, `${name}.png`), body)
}
