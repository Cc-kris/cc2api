import { expect, test, type Page } from '@playwright/test'
import { expectClean, expectNoHorizontalOverflow, guard, openContentTab, realLogin, reloadToTab, shooter, stamp, waitWrite } from './real-helpers'

// 流程 N：已抓取新闻 列表 → 打开编辑弹窗 → 改摘要保存 → 刷新回读 → 恢复原值；取消/× 关闭不落库
// 新闻不能手工新建（只能抓取），所以拿列表第一条做编辑，并在最后恢复原摘要，避免污染数据。
const TAB = '新闻动态'
const marker = `[e2e-real-${stamp}]`
let targetTitle = ''
let originalSummary = ''

const newsRows = (page: Page) => page.locator('section.workspace table tbody tr')
const modal = (page: Page) => page.locator('.modal-backdrop', { has: page.locator('[aria-label="关闭新闻编辑"]') })
const summaryBox = (page: Page) => modal(page).locator('textarea.news-summary-input')

async function openTarget(page: Page) {
  await page.getByPlaceholder('搜索标题或摘要').fill(targetTitle.slice(0, 30))
  await page.getByRole('button', { name: '查询' }).click()
  await page.waitForLoadState('networkidle')
  const row = newsRows(page).filter({ hasText: targetTitle.slice(0, 30) }).first()
  await row.getByRole('button', { name: '编辑' }).click()
  await expect(modal(page)).toBeVisible()
}

test.describe.configure({ mode: 'serial' })

test('N1 新闻列表可见且无横向溢出', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('news', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await expect(newsRows(page).first()).toBeVisible()
  targetTitle = ((await newsRows(page).first().locator('.title-link').first().innerText()) || '').split('\n')[0].trim()
  expect(targetTitle.length, '列表第一条新闻标题为空').toBeGreaterThan(3)
  await expectNoHorizontalOverflow(page)
  await shot(page, 'list')
  expectClean(problems)
})

test('N2 编辑摘要并保存，刷新后回显', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('news', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await openTarget(page)
  originalSummary = await summaryBox(page).inputValue()
  await summaryBox(page).fill(`${originalSummary} ${marker}`.trim())
  await shot(page, 'editing')
  await waitWrite(page, 'PUT', '/admin/content/news/', () => modal(page).getByRole('button', { name: '保存新闻' }).click())
  await expect(modal(page)).toHaveCount(0)
  await reloadToTab(page, TAB)
  await openTarget(page)
  await expect(summaryBox(page)).toHaveValue(new RegExp(marker.replace(/[[\]]/g, '\\$&')))
  await shot(page, 'after-reload')
  expectClean(problems)
})

test('N3 取消与 × 关闭不落库', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('news', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await openTarget(page)
  await summaryBox(page).fill('不应保存的内容')
  await modal(page).getByRole('button', { name: '取消' }).click()
  const guardDlg = page.locator('.news-close-guard-dialog')
  if (await guardDlg.isVisible().catch(() => false)) await guardDlg.getByRole('button', { name: '确认离开' }).click()
  await expect(modal(page)).toHaveCount(0)
  await openTarget(page)
  await expect(summaryBox(page)).not.toHaveValue('不应保存的内容')
  await page.locator('[aria-label="关闭新闻编辑"]').click()
  await expect(modal(page)).toHaveCount(0)
  await shot(page, 'closed')
  expectClean(problems)
})

test('N4 恢复原摘要（清理）', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('news', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await openTarget(page)
  await summaryBox(page).fill(originalSummary)
  await waitWrite(page, 'PUT', '/admin/content/news/', () => modal(page).getByRole('button', { name: '保存新闻' }).click())
  await reloadToTab(page, TAB)
  await openTarget(page)
  await expect(summaryBox(page)).toHaveValue(originalSummary)
  await shot(page, 'restored')
  expectClean(problems)
})
