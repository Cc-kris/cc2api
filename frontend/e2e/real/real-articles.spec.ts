import { expect, test, type Page } from '@playwright/test'
import { expectClean, guard, openContentTab, realLogin, reloadToTab, shooter, stamp, waitWrite } from './real-helpers'

// 流程 A：深度解析文章 新建 → 刷新回读 → 编辑 → 精选开/关 → 删除，全程真实点击 + 真实后端
const TAB = '深度解析'
const title = `E2E-REAL-文章-${stamp}`
const API = '/api/v1/admin/content/articles'
// 精选开关真实请求：POST /api/v1/admin/content/articles/taxonomy/mutate（body.scope=featured_article）
const FEATURED_API = '/admin/content/articles/taxonomy/mutate'

async function search(page: Page, q: string) {
  const box = page.locator('.articles-pane .search-input')
  await box.fill(q)
  await box.press('Enter')
  await page.waitForLoadState('networkidle')
}
const row = (page: Page) => page.locator('.articles-pane table tbody tr', { hasText: title })

test.describe.configure({ mode: 'serial' })

test('A1 新建文章并刷新后仍在', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('articles', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await shot(page, 'list')
  await page.getByRole('button', { name: '+ 新建文章' }).click()
  const modal = page.locator('.article-editor-modal')
  await expect(modal).toBeVisible()
  await modal.locator('.editor-title-input').fill(title)
  await modal.locator('textarea').first().fill('E2E 真实操作摘要')
  await modal.getByRole('button', { name: 'HTML 源码' }).click()
  await modal.locator('textarea.html-source-area').fill(`<h2>真实 E2E</h2><p>${title} 正文</p>`)
  await shot(page, 'editor-filled')
  await waitWrite(page, 'POST', API, () => modal.getByRole('button', { name: '保存', exact: true }).click())
  await expect(modal).toHaveCount(0)
  await reloadToTab(page, TAB)
  await search(page, title)
  await expect(row(page)).toHaveCount(1)
  await shot(page, 'after-reload')
  expectClean(problems)
})

test('A2 编辑摘要保存，刷新后回显', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('articles', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await search(page, title)
  await row(page).getByRole('button', { name: '编辑' }).click()
  const modal = page.locator('.article-editor-modal')
  await expect(modal.locator('.editor-title-input')).toHaveValue(title)
  await modal.getByRole('button', { name: 'HTML 源码' }).click()
  await expect(modal.locator('textarea.html-source-area')).toHaveValue(new RegExp(`${title} 正文`))
  await modal.locator('textarea').first().fill('E2E 摘要-已修改')
  await shot(page, 'edit')
  await waitWrite(page, 'PUT', API, () => modal.getByRole('button', { name: '保存', exact: true }).click())
  await reloadToTab(page, TAB)
  await search(page, title)
  await row(page).getByRole('button', { name: '编辑' }).click()
  await expect(modal.locator('textarea').first()).toHaveValue('E2E 摘要-已修改')
  await shot(page, 'after-reload')
  await modal.getByRole('button', { name: '取消' }).click()
  await expect(modal).toHaveCount(0)
  expectClean(problems)
})

test('A3 精选开/关，刷新后状态保持', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('articles', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await search(page, title)
  const toggle = row(page).locator('.featured-toggle')
  // 业务规则：仅已发布文章可设精选。草稿行开关必须是禁用态（修复前可点、后端回 500）。
  await expect(toggle, '草稿文章的精选开关应禁用').toBeDisabled()
  await shot(page, 'draft-featured-disabled')
  await waitWrite(page, 'POST', '/publish', () => row(page).getByRole('button', { name: '发布', exact: true }).click())
  await expect(toggle).toBeEnabled()
  await waitWrite(page, 'POST', FEATURED_API, () => toggle.check())
  await reloadToTab(page, TAB)
  await search(page, title)
  await expect(row(page).locator('.featured-toggle')).toBeChecked()
  await shot(page, 'featured-on')
  await waitWrite(page, 'POST', FEATURED_API, () => row(page).locator('.featured-toggle').uncheck())
  await reloadToTab(page, TAB)
  await search(page, title)
  await expect(row(page).locator('.featured-toggle')).not.toBeChecked()
  await shot(page, 'featured-off')
  expectClean(problems)
})

test('A4 删除走确认弹窗，刷新后消失', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('articles', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await search(page, title)
  await row(page).getByRole('button', { name: '删除' }).click()
  const dlg = page.locator('.inline-tax-dialog', { hasText: '确认删除这篇文章' })
  await expect(dlg).toBeVisible()
  await shot(page, 'confirm')
  await waitWrite(page, 'DELETE', API, () => dlg.getByRole('button', { name: '确认删除' }).click())
  await reloadToTab(page, TAB)
  await search(page, title)
  await expect(row(page)).toHaveCount(0)
  await shot(page, 'after-reload')
  expectClean(problems)
})
