import fs from 'node:fs'
import path from 'node:path'
import { expect, type Page, type Response, type TestInfo } from '@playwright/test'

/**
 * v0.4 内容中心「真实操作」E2E 公共工具。契约（不得弱化）：
 *  1. 禁止 page.route / mock：请求全部打到真实 :3100 → :8080 → PostgreSQL。
 *  2. 走真实登录页表单，不注入 token。
 *  3. 用例结束时 /api/ 下不得有 4xx/5xx、console.error、pageerror（白名单须写明原因）。
 *  4. 写操作必须 reload 后从界面读回（持久化），不能只看接口返回。
 *  5. 关键步骤截图：test-results/e2e-real-v04/<flow>/<case>-NN-<step>.png
 */
export const BASE = process.env.S2A_BASE_URL ?? 'http://127.0.0.1:3100'
export const EMAIL = process.env.S2A_E2E_EMAIL ?? 'e2e-task031@sub2api.local'
export const PASSWORD = process.env.S2A_E2E_PASSWORD ?? 'TASK031-e2e-Passw0rd!'
export const OUT_ROOT = process.env.E2E_REAL_OUT ?? path.resolve(process.cwd(), 'test-results/e2e-real-v04')
export const stamp = Date.now().toString(36)

export interface Problem { kind: 'http' | 'console' | 'pageerror'; detail: string }

/** 挂网络/控制台守卫；返回的数组会随页面事件持续增长。 */
export function guard(page: Page, allow: RegExp[] = []): Problem[] {
  const problems: Problem[] = []
  const allowed = (s: string) => allow.some((r) => r.test(s))
  page.on('response', (res: Response) => {
    if (!res.url().includes('/api/') || res.status() < 400) return
    const detail = `${res.request().method()} ${res.url().replace(BASE, '')} → ${res.status()}`
    if (!allowed(detail)) problems.push({ kind: 'http', detail })
  })
  page.on('console', (msg) => {
    if (msg.type() !== 'error') return
    const detail = msg.text().slice(0, 300)
    if (!allowed(detail)) problems.push({ kind: 'console', detail })
  })
  page.on('pageerror', (err) => {
    const detail = String(err?.message || err).slice(0, 300)
    if (!allowed(detail)) problems.push({ kind: 'pageerror', detail })
  })
  return problems
}

export function expectClean(problems: Problem[]) {
  const msg = problems.map((p) => `[${p.kind}] ${p.detail}`).join('\n')
  expect(problems, `真实操作过程中出现错误：\n${msg}`).toEqual([])
}

/** 截图器：按 flow/用例编号落盘，并 attach 到 Playwright 报告。 */
export function shooter(flow: string, testInfo: TestInfo) {
  const dir = path.join(OUT_ROOT, flow)
  fs.mkdirSync(dir, { recursive: true })
  const caseTag = testInfo.title.split(' ')[0]
  let n = 0
  return async (page: Page, step: string) => {
    n += 1
    const file = path.join(dir, `${caseTag}-${String(n).padStart(2, '0')}-${step}.png`)
    await page.screenshot({ path: file, fullPage: false })
    await testInfo.attach(`${caseTag}-${n}-${step}`, { path: file, contentType: 'image/png' })
  }
}

/** 去掉 dev 浮层 / 公告弹窗等非业务遮挡（公告按真实用户点击关闭）。 */
export async function dismissOverlays(page: Page) {
  await page.evaluate(() => document.querySelector('vite-plugin-checker-error-overlay')?.remove())
  for (let i = 0; i < 3; i++) {
    const btn = page.locator('div[class*="z-[120]"] button').first()
    if (!(await btn.isVisible().catch(() => false))) break
    await btn.click()
    await page.waitForTimeout(300)
  }
}

/** 真实登录页登录；只预置“已看过新手引导”（老用户状态），不注入凭据。 */
export async function realLogin(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem('admin_guide_2_admin_v4_interactive', 'true')
    localStorage.setItem('admin_guide_guest_user_v4_interactive', 'true')
  })
  await page.goto(`${BASE}/login`)
  await page.locator('#email').fill(EMAIL)
  await page.locator('#password').fill(PASSWORD)
  await page.locator('button[type="submit"]').click()
  await page.waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 30_000 })
}

/** 打开内容中心并真实点击切到一级 tab。 */
export async function openContentTab(page: Page, tab: string) {
  if (!page.url().includes('/admin/content')) await page.goto(`${BASE}/admin/content`)
  await expect(page.locator('.tabs button').first()).toBeVisible({ timeout: 30_000 })
  await dismissOverlays(page)
  await page.locator('.tabs button', { hasText: tab }).first().click()
  await expect(page.locator('.tabs button.active', { hasText: tab }).first()).toBeVisible()
}

/** reload 后回到同一 tab（持久化校验）。 */
export async function reloadToTab(page: Page, tab: string) {
  await page.reload()
  await openContentTab(page, tab)
}

/** 点击触发写请求并断言 2xx；返回 JSON（若有）。 */
export async function waitWrite(page: Page, method: string | RegExp, urlPart: string | RegExp, action: () => Promise<unknown>) {
  const match = (u: string) => (typeof urlPart === 'string' ? u.includes(urlPart) : urlPart.test(u))
  const methodOk = (m: string) => (typeof method === 'string' ? m === method : method.test(m))
  const [res] = await Promise.all([
    page.waitForResponse((r) => methodOk(r.request().method()) && match(r.url()), { timeout: 20_000 }),
    action(),
  ])
  const body = await res.text().catch(() => '')
  expect(res.status(), `${method} ${res.url()} 返回 ${res.status()}：${body.slice(0, 300)}`).toBeLessThan(300)
  try { return JSON.parse(body) } catch { return null }
}

/** 断言页面无横向溢出。 */
export async function expectNoHorizontalOverflow(page: Page) {
  const over = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(over, '页面出现横向溢出').toBeLessThanOrEqual(1)
}
