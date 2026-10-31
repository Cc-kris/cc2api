import { expect, test, type Page } from '@playwright/test'
import { expectClean, expectNoHorizontalOverflow, guard, openContentTab, realLogin, reloadToTab, shooter, stamp, waitWrite } from './real-helpers'

// 流程 M：AI 创作 → 模型配置：新增（非默认）→ 刷新回读 → 编辑名称 → 刷新回读 → 删除；不碰现有默认配置
// 流程 B：BI 分析：指标可见、改日期触发真实查询、导出可用、无横向溢出
const label = `E2E-AI-${stamp}`
const renamed = `${label}-改`

// 真实 DOM：AI 创作页子 tab 在 nav.ai-subtabs 内（两个按钮：文章生成 / 模型配置）；
// 页面另有一个 .ai-config-required 里的「前往模型配置」按钮，故必须限定容器 + exact，避免 strict mode 冲突。
const aiSubTab = (page: Page) => page.locator('.ai-subtabs').getByRole('button', { name: '模型配置', exact: true })

async function gotoConfig(page: Page) {
  await openContentTab(page, 'AI 创作')
  await aiSubTab(page).click()
  await expect(page.locator('.ai-config-panel')).toBeVisible()
}
const card = (page: Page, n: string) => page.locator('.ai-config-list > li', { hasText: n })
const dialog = (page: Page) => page.locator('.ai-config-dialog')

test.describe.configure({ mode: 'serial' })

test('M1 新增模型配置，刷新后仍在', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('ai-config', info)
  await realLogin(page)
  await gotoConfig(page)
  await shot(page, 'list')
  await page.getByRole('button', { name: '新增配置' }).click()
  const d = dialog(page)
  await expect(d).toBeVisible()
  await d.getByPlaceholder('例如：文章生成主模型').fill(label)
  await d.getByPlaceholder('https://api.example.com/v1').fill('https://api.example.com/v1')
  await d.locator('input[type="password"]').fill('sk-e2e-real-not-a-real-key')
  await d.getByPlaceholder('每行一个模型名称').fill('e2e-model-a\ne2e-model-b')
  // 默认模型：填了 Models 后该字段会变成自定义 Select（组件 common/Select.vue，无原生 <select>，
  // 触发按钮为 .select-trigger，选项 teleport 到 body 的 role=option），故按真实控件点选。
  const defTrigger = d.locator('.select-trigger').last()
  if ((await defTrigger.count()) > 0 && (await defTrigger.isVisible())) {
    await defTrigger.click()
    await page.getByRole('option', { name: 'e2e-model-a', exact: true }).click()
  } else {
    await d.getByPlaceholder('Enter a model name manually').fill('e2e-model-a')
  }
  await shot(page, 'dialog-filled')
  await waitWrite(page, 'POST', '/ai-configs/', () => d.locator('button.primary').last().click())
  await expect(d).toHaveCount(0)
  await reloadToTab(page, 'AI 创作')
  await aiSubTab(page).click()
  await expect(card(page, label)).toHaveCount(1)
  await shot(page, 'after-reload')
  expectClean(problems)
})

test('M2 编辑名称保存，刷新后回显（不出现 409）', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('ai-config', info)
  await realLogin(page)
  await gotoConfig(page)
  await card(page, label).getByRole('button', { name: /编辑|Edit/ }).click()
  const d = dialog(page)
  await expect(d.getByPlaceholder('例如：文章生成主模型')).toHaveValue(label)
  await d.getByPlaceholder('例如：文章生成主模型').fill(renamed)
  await shot(page, 'edit')
  await waitWrite(page, 'POST', '/ai-configs/', () => d.locator('button.primary').last().click())
  await reloadToTab(page, 'AI 创作')
  await aiSubTab(page).click()
  await expect(card(page, renamed)).toHaveCount(1)
  await shot(page, 'after-reload')
  expectClean(problems)
})

test('M3 删除配置（清理），刷新后消失', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('ai-config', info)
  await realLogin(page)
  await gotoConfig(page)
  page.once('dialog', (dl) => dl.accept())
  await waitWrite(page, 'POST', '/ai-configs/', () => card(page, renamed).locator('button.danger-button').click())
  await reloadToTab(page, 'AI 创作')
  await aiSubTab(page).click()
  await expect(card(page, renamed)).toHaveCount(0)
  await shot(page, 'after-reload')
  expectClean(problems)
})

test('B1 BI 指标可见、改日期触发查询、导出可用', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('bi', info)
  await realLogin(page)
  await openContentTab(page, 'BI 分析')
  await page.waitForLoadState('networkidle')
  // 真实 DOM：bi-from/bi-to 只在「范围 = Custom range」时渲染；默认范围是 Last 30 days。
  const rangeSel = page.locator('[data-test="bi-range"]')
  await expect(rangeSel).toBeVisible()
  await rangeSel.locator('button').first().click()
  await page.getByRole('option', { name: 'Custom range', exact: true }).click()
  const from = page.locator('[data-test="bi-from"]')
  await expect(from).toBeVisible()
  await page.waitForLoadState('networkidle')
  await shot(page, 'overview')
  // 指标卡片/图表区必须有非空可见文本（数字或名称），不能是空壳
  const pane = page.locator('section.workspace').first()
  expect((await pane.innerText()).replace(/\s+/g, '').length, 'BI 页面几乎无可见内容').toBeGreaterThan(40)
  await expectNoHorizontalOverflow(page)
  const d = new Date(Date.now() - 14 * 86400_000).toISOString().slice(0, 10)
  const [res] = await Promise.all([
    page.waitForResponse(
      (r) => r.url().includes('/bi/analytics/') && r.request().method() === 'GET',
      { timeout: 20_000 },
    ),
    from.fill(d).then(() => from.dispatchEvent('change')),
  ])
  expect(res.status()).toBeLessThan(300)
  expect(res.url(), '改日期后请求未带上新日期').toContain(`from=${d}`)
  await page.waitForLoadState('networkidle')
  await shot(page, 'filtered-7d')
  const exportBtn = page.locator('[data-test="bi-export"]')
  await expect(exportBtn).toBeEnabled()
  const download = page.waitForEvent('download', { timeout: 15_000 }).catch(() => null)
  await exportBtn.click()
  expect(await download, '点击导出没有产生下载').not.toBeNull()
  await shot(page, 'after-export')
  expectClean(problems)
})
