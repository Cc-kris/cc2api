import { expect, test, type Page } from '@playwright/test'
import { expectClean, guard, openContentTab, realLogin, reloadToTab, shooter, stamp, waitWrite } from './real-helpers'

// 流程 T：分类与标签 —— 分类 新建→刷新→改名→刷新→删除；标签 新建（TagInput，打开时不应主动弹下拉）→刷新→删除
const TAB = '分类与标签'
const cat = `E2E分类${stamp}`
const catRenamed = `${cat}改`
const tag = `e2etag${stamp}`

const dialog = (page: Page) => page.locator('.taxonomy-dialog-footer').locator('..')
async function switchView(page: Page, label: RegExp) {
  await page.locator('.taxonomy-pane .subnav button', { hasText: label }).first().click()
  await page.waitForLoadState('networkidle')
}
async function find(page: Page, q: string) {
  await page.locator('.taxonomy-pane .toolbar input').first().fill(q)
  await page.waitForTimeout(400)
}
const row = (page: Page, n: string) => page.locator('.taxonomy-pane table tbody tr, .taxonomy-pane li', { hasText: n })
const submit = (page: Page) => page.locator('.taxonomy-dialog-footer button.primary')

test.describe.configure({ mode: 'serial' })

test('T1 新建分类，刷新后仍在', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('taxonomy', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await switchView(page, /分类/)
  await shot(page, 'list')
  await page.locator('.taxonomy-pane .toolbar button.button').first().click()
  await dialog(page).locator('input').first().fill(cat)
  await shot(page, 'dialog')
  await waitWrite(page, 'POST', '/admin/content/', () => submit(page).click())
  await reloadToTab(page, TAB)
  await switchView(page, /分类/)
  await find(page, cat)
  await expect(row(page, cat).first()).toBeVisible()
  await shot(page, 'after-reload')
  expectClean(problems)
})

test('T2 分类改名，刷新后回显', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('taxonomy', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await switchView(page, /分类/)
  await find(page, cat)
  await row(page, cat).first().getByRole('button', { name: '编辑' }).click()
  await dialog(page).locator('input').first().fill(catRenamed)
  await waitWrite(page, /^(PUT|POST|PATCH)$/, '/admin/content/', () => submit(page).click())
  await reloadToTab(page, TAB)
  await switchView(page, /分类/)
  await find(page, catRenamed)
  await expect(row(page, catRenamed).first()).toBeVisible()
  await shot(page, 'after-reload')
  expectClean(problems)
})

test('T3 删除分类，刷新后消失', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('taxonomy', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await switchView(page, /分类/)
  await find(page, catRenamed)
  await row(page, catRenamed).first().getByRole('button', { name: '删除' }).click()
  await expect(submit(page)).toHaveText(/确认删除/)
  await shot(page, 'confirm')
  await waitWrite(page, 'POST', '/taxonomy/mutate', () => submit(page).click())
  await reloadToTab(page, TAB)
  await switchView(page, /分类/)
  await find(page, catRenamed)
  await expect(row(page, catRenamed)).toHaveCount(0)
  expectClean(problems)
})

test('T4 新建标签：打开时不主动下拉，保存后刷新仍在，然后删除', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('taxonomy', info)
  await realLogin(page)
  await openContentTab(page, TAB)
  await switchView(page, /标签/)
  await page.locator('.taxonomy-pane .toolbar button.button').first().click()
  const input = dialog(page).locator('input').first()
  await expect(input).toBeVisible()
  await expect(dialog(page).locator('[role="listbox"], .tag-suggestions, .suggestions'), '标签弹窗一打开就弹出了下拉').toHaveCount(0)
  await shot(page, 'dialog-open-no-dropdown')
  await input.fill(tag)
  // 名称已改为普通输入框：Enter 本身就会提交（@keyup.enter），这里只点「保存」，避免重复提交
  await expect(submit(page)).toBeEnabled()
  await waitWrite(page, 'POST', '/admin/content/', () => submit(page).click())
  await reloadToTab(page, TAB)
  await switchView(page, /标签/)
  await find(page, tag)
  await expect(row(page, tag).first()).toBeVisible()
  await shot(page, 'after-reload')
  await row(page, tag).first().getByRole('button', { name: '删除' }).click()
  await waitWrite(page, 'POST', '/taxonomy/mutate', () => submit(page).click())
  await reloadToTab(page, TAB)
  await switchView(page, /标签/)
  await find(page, tag)
  await expect(row(page, tag)).toHaveCount(0)
  expectClean(problems)
})
