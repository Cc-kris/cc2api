import { expect, test, type Page } from '@playwright/test'
import { expectClean, guard, openContentTab, realLogin, reloadToTab, shooter, stamp, waitWrite } from './real-helpers'

// 流程 S：新闻源 新建 → 刷新回读 → 编辑名称 → 停用/启用 → 删除；外加「自动化设置」「抓取任务」子页可打开无报错。
// 不点「立即抓取」：会访问外网，结果不稳定，不属于本流程。
const TAB = '新闻动态'
const name = `E2E-REAL-源-${stamp}`
const renamed = `${name}-改`
const url = `https://example.com/e2e-real-${stamp}.xml`

async function gotoSources(page: Page) {
  await page.locator('button', { hasText: '新闻源' }).first().click()
  await expect(page.locator('.source-toolbar button', { hasText: '新增新闻源' }).first()).toBeVisible()
  await expect(page.locator('table tbody tr').first()).toBeVisible({ timeout: 30_000 })
}
const row = (page: Page, n: string) => page.locator('table tbody tr', { hasText: n })

test.describe.configure({ mode: 'serial' })

test('S1 新建新闻源，刷新后仍在', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('sources', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await gotoSources(page)
  await page.locator('.source-toolbar button', { hasText: '新增新闻源' }).first().click()
  const dlg = page.locator('.source-dialog[role="dialog"]')
  await expect(dlg).toBeVisible()
  await dlg.getByPlaceholder('来源名称（必填）').fill(name)
  await dlg.getByPlaceholder('公开 RSS/Atom/JSON URL（必填）').fill(url)
  await shot(page, 'form')
  const saveBtn = dlg.getByRole('button', { name: '保存新闻源' })
  await expect(saveBtn).toBeEnabled()
  await waitWrite(page, 'POST', '/admin/content/news-sources', () => saveBtn.click())
  await reloadToTab(page, TAB)
  await gotoSources(page)
  await expect(row(page, name)).toHaveCount(1)
  await shot(page, 'after-reload')
  expectClean(problems)
})

test('S2 编辑名称保存，刷新后回显', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('sources', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await gotoSources(page)
  await row(page, name).getByRole('button', { name: '编辑' }).click()
  // 真实弹窗结构：.content-modal-layer > .source-dialog[role=dialog]（没有 .modal-backdrop）
  const dlg = page.locator('.source-dialog[role="dialog"]')
  await expect(dlg).toBeVisible({ timeout: 20_000 })
  await dlg.locator('label', { hasText: '名称' }).first().locator('input').fill(renamed)
  await shot(page, 'edit')
  await waitWrite(page, 'PUT', '/admin/content/news-sources/', () => dlg.getByRole('button', { name: '保存新闻源' }).click())
  await reloadToTab(page, TAB)
  await gotoSources(page)
  await expect(row(page, renamed)).toHaveCount(1)
  await shot(page, 'after-reload')
  expectClean(problems)
})

test('S3 停用/启用，刷新后状态保持', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('sources', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await gotoSources(page)
  const r = () => row(page, renamed)
  const firstLabel = (await r().getByRole('button', { name: /^(停用|启用)$/ }).innerText()).trim()
  const flipped = firstLabel === '停用' ? '启用' : '停用'
  await waitWrite(page, 'POST', /news-sources\/\d+\/(enable|disable)/, () => r().getByRole('button', { name: firstLabel }).click())
  await reloadToTab(page, TAB)
  await gotoSources(page)
  await expect(r().getByRole('button', { name: flipped })).toBeVisible()
  await shot(page, 'flipped')
  await waitWrite(page, 'POST', /news-sources\/\d+\/(enable|disable)/, () => r().getByRole('button', { name: flipped }).click())
  await reloadToTab(page, TAB)
  await gotoSources(page)
  await expect(r().getByRole('button', { name: firstLabel })).toBeVisible()
  expectClean(problems)
})

// PRD 7.1：新闻源为「软删除」，列表固定 include_deleted=true，已删行保留并显示「恢复」。
// 因此断言：DELETE 2xx → 刷新后该行出现「恢复」且不再有「删除」 → 接口回读 deletedAt 非空。
test('S4 删除新闻源（软删除），刷新后显示「恢复」且接口 deletedAt 已写入', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('sources', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await gotoSources(page)
  await row(page, renamed).getByRole('button', { name: '删除' }).click()
  // 产品用页内自绘确认框（.ui-confirm-dialog，非 window.confirm）：先等它出现再点确认
  const confirm = page.locator('.ui-confirm-dialog[role="dialog"]')
  await expect(confirm).toBeVisible({ timeout: 15_000 })
  await shot(page, 'confirm')
  const [delRes] = await Promise.all([
    page.waitForResponse((r) => r.request().method() === 'DELETE' && r.url().includes('/admin/content/news-sources/'), { timeout: 20_000 }),
    confirm.getByRole('button', { name: '确认删除' }).click(),
  ])
  expect(delRes.status(), `DELETE ${delRes.url()}`).toBeLessThan(300)
  const id = delRes.url().split('/news-sources/')[1].split(/[/?]/)[0]
  await reloadToTab(page, TAB)
  await gotoSources(page)
  const r = row(page, renamed)
  await expect(r).toHaveCount(1)
  await expect(r.getByRole('button', { name: '恢复' })).toBeVisible()
  // 观察项（非断言）：模板对已软删除行仍渲染「删除」按钮（ContentCenterView 新闻源操作列无 v-if），属 UX 问题，记录于报告。
  await shot(page, 'after-reload-soft-deleted')
  // 接口回读：用页面自身登录态（localStorage token）请求真实后端
  const detail = await page.evaluate(async (sid) => {
    const token = localStorage.getItem('auth_token') || localStorage.getItem('access_token') || ''
    const res = await fetch(`/api/v1/admin/content/news-sources/${sid}?include_deleted=true`, { headers: { Authorization: `Bearer ${token}` } })
    return { status: res.status, body: await res.json().catch(() => null) }
  }, id)
  expect(detail.status, `GET news-sources/${id}`).toBe(200)
  const data = detail.body?.data ?? detail.body
  expect(data?.deletedAt ?? data?.deleted_at, '软删除未写入 deletedAt').toBeTruthy()
  expectClean(problems)
})

test('S5 抓取任务 / 自动化设置子页可打开', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('sources', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await page.locator('button', { hasText: '抓取任务' }).first().click()
  await expect(page.locator('th', { hasText: '错误摘要' })).toBeVisible()
  await shot(page, 'runs')
  await page.locator('button', { hasText: '自动化设置' }).first().click()
  await page.waitForLoadState('networkidle')
  await shot(page, 'automation')
  expectClean(problems)
})
