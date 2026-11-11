import { expect, type Page, type TestInfo } from '@playwright/test'
import { login, navigate, screenshot, ADMIN_EMAIL, ADMIN_PASSWORD } from './ui'
import { scalar } from './db'

type Seed = Awaited<ReturnType<typeof import('./seed').seed>>
const money = (v: string) => Number(v).toFixed(8)

export async function editChannelPricing(page: Page, s: Seed, testInfo: TestInfo, fields: Record<string, string>, create: boolean) {
  await login(page, ADMIN_EMAIL, ADMIN_PASSWORD, testInfo.project.name === 'mobile-en' ? 'en-US' : 'zh-CN')
  await navigate(page, '/admin/channels/pricing')
  if (create) await page.getByRole('button', { name: /新建渠道|创建渠道|Create Channel/ }).first().click()
  else {
    const row = page.locator('tr').filter({ hasText: s.channel!.name })
      .or(page.locator('div.rounded-lg.border').filter({ hasText: s.channel!.name }))
    await row.first().getByRole('button', { name: /编辑|Edit/ }).click()
  }
  const dialog = page.getByRole('dialog').last()
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: s.platform === 'openai' ? /^OpenAI$/ : /^Anthropic$/, exact: true }).click()
  const groupLabel = dialog.locator('label').filter({ hasText: s.group.name })
  if (!(await groupLabel.locator('input').isChecked())) await groupLabel.locator('input').check()
  const entry = dialog.locator('div.rounded-lg.border').filter({
    hasText: /模型列表.*默认价格|Models.*Default [Pp]rices/s,
  }).last()
  if (await dialog.locator('div.rounded-lg.border').filter({ hasText: /^定价配置$|^Pricing Entry$/ }).count()) {
    await dialog.getByText(/^定价配置$|^Pricing Entry$/).last().click()
  }
  await expect(entry.locator('input[type="number"]').first()).toBeVisible()
  const model = s.platform === 'openai' ? 'gpt-5.5' : 'claude-sonnet-4-6'
  if (!(await entry.getByText(model, { exact: true }).count())) {
    const modelInput = entry.locator('input[type="text"]').first()
    await modelInput.fill(model)
    await modelInput.press('Enter')
  }
  for (const [label, value] of Object.entries(fields)) {
    const input = entry.locator('label').filter({ hasText: new RegExp(label, 'i') }).locator('..').locator('input[type="number"]')
    await input.fill(value)
  }
  const [saved] = await Promise.all([
    page.waitForResponse(r => /\/admin\/channels(?:\/\d+)?$/.test(new URL(r.url()).pathname) && ['POST', 'PUT'].includes(r.request().method())),
    dialog.locator('button[form="channel-form"]').click(),
  ])
  expect(saved.status(), await saved.text()).toBeLessThan(300)
  await expect(dialog).not.toBeVisible()
  await screenshot(page, testInfo, create ? 'channel-pricing-created' : 'channel-pricing-fallback')
}

export async function editImageGroup(page: Page, s: Seed, testInfo: TestInfo, enabled: boolean) {
  await login(page, ADMIN_EMAIL, ADMIN_PASSWORD, testInfo.project.name === 'mobile-en' ? 'en-US' : 'zh-CN')
  await navigate(page, '/admin/groups')
  // Both desktop and mobile lists are windowed; filter before locating a row.
  await page.getByRole('textbox', { name: /搜索分组|Search groups/i }).fill(s.group.name)
  const row = page.locator('tr').filter({ hasText: s.group.name })
    .or(page.locator('div.rounded-lg.border').filter({ hasText: s.group.name }).filter({ has: page.getByRole('button', { name: /编辑|Edit/ }) }))
  await row.first().getByRole('button', { name: /编辑|Edit/ }).click()
  const dialog = page.getByRole('dialog').last()
  await dialog.getByLabel(/允许当前分组生图|Allow image generation/).check()
  const toggle = dialog.locator('#edit-image-count-billing-input')
  if (enabled) await toggle.check(); else await toggle.uncheck()
  await dialog.locator('label').filter({ hasText: /^1K \(\$\)$/ }).locator('..').locator('input[type="number"]').fill('0.04')
  const [saved] = await Promise.all([
    page.waitForResponse(r => r.request().method() === 'PUT' && r.url().endsWith(`/admin/groups/${s.group.id}`)),
    dialog.getByRole('button', { name: /更新|Update/ }).click(),
  ])
  expect(saved.status(), await saved.text()).toBe(200)
  await expect(dialog).not.toBeVisible()
  expect(scalar(`select image_count_billing from groups where id=${s.group.id}`)).toBe(enabled ? 't' : 'f')
  expect(Number(scalar(`select image_price_1k from groups where id=${s.group.id}`))).toBe(0.04)
}

export async function assertLatestCost(keyId: number, expected: string) {
  // The usage worker may finish after the HTTP response. Wait for the new
  // request's own amount rather than reading the preceding request's row.
  await expect.poll(() => {
    const value = scalar(`select coalesce((select actual_cost::text from usage_logs where api_key_id=${keyId} order by id desc limit 1),'')`)
    return value === '' ? '' : money(value)
  }, { timeout: 20000 }).toBe(expected)
  const id = scalar(`select id from usage_logs where api_key_id=${keyId} order by id desc limit 1`)
  expect(money(scalar(`select actual_cost from usage_logs where id=${id}`))).toBe(expected)
  return id
}
