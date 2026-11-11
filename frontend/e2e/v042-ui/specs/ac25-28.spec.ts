import { expect, test } from '@playwright/test'
import { seed } from '../helpers/seed'
import { anthropicBody, gatewayOpenAITry, gatewayTry, imageBody, openaiChatBody, raw } from '../helpers/api'
import { scenario } from '../helpers/mock'
import { scalar } from '../helpers/db'
import { assertLatestCost, editChannelPricing, editImageGroup } from '../helpers/ac25-28'
import { login, navigate, screenshot } from '../helpers/ui'
import type { Browser, Page, TestInfo } from '@playwright/test'

const tokenPrice = (model: string) => ({ models: [model], billing_mode: 'token' })

async function verifyUserUsage(browser: Browser, adminPage: Page, s: Awaited<ReturnType<typeof seed>>, testInfo: TestInfo, amounts: string[], name: string) {
  const context = await browser.newContext({ viewport: adminPage.viewportSize() || undefined })
  const page = await context.newPage()
  try {
    await login(page, s.email, s.password, testInfo.project.name === 'mobile-en' ? 'en-US' : 'zh-CN')
    await navigate(page, '/usage')
    for (const amount of amounts) await expect(page.locator('body')).toContainText(amount)
    await screenshot(page, testInfo, name)
  } finally {
    await context.close()
  }
}

test('AC25 真实渠道界面配置 1h 缓存写价与留空回退', async ({ page, browser }, testInfo) => {
  const s = await seed({ prefix: 'ac25', balance: 10, channel: { model_pricing: [tokenPrice('claude-sonnet-4-6')] } })
  await scenario(s.upstreamKey, { usage: { input: 0, cache_creation_5m: 10000, cache_creation_1h: 10000, output: 100 }, stream: false })
  await editChannelPricing(page, s, testInfo, { '^输出$|^Output$': '15', '^缓存写入$|^Cache Write$': '3.75', '1h.*缓存写入|1h Cache Write': '6' }, false)
  let r = await gatewayTry({ path: '/v1/messages', apiKey: s.key.key, body: anthropicBody('claude-sonnet-4-6') })
  expect(r.status, r.text).toBe(200)
  let id = await assertLatestCost(s.key.id, '0.09900000')
  expect(Number(scalar(`select cache_creation_cost from usage_logs where id=${id}`)).toFixed(8)).toBe('0.09750000')
  await editChannelPricing(page, s, testInfo, { '1h.*缓存写入|1h Cache Write': '' }, false)
  r = await gatewayTry({ path: '/v1/messages', apiKey: s.key.key, body: anthropicBody('claude-sonnet-4-6') })
  expect(r.status, r.text).toBe(200)
  id = await assertLatestCost(s.key.id, '0.07650000')
  await verifyUserUsage(browser, page, s, testInfo, ['$0.09900000', '$0.07650000'], 'AC25-cache-1h-and-fallback')
})

test('AC26 真实渠道界面配置图片输入价与留空回退', async ({ page, browser }, testInfo) => {
  const s = await seed({
    prefix: 'ac26', platform: 'openai', balance: 10,
    accounts: [{ type: 'apikey', platform: 'openai', credentials: { api_key: 'up-ac26-' + Date.now(), base_url: 'http://127.0.0.1:18921' }, extra: { openai_responses_mode: 'force_chat_completions' } }],
    channel: { model_pricing: [{ ...tokenPrice('gpt-5.5'), platform: 'openai' }] },
  })
  await scenario(s.upstreamKey, { usage: { input: 1500, image_input: 1000, output: 200 }, stream: false })
  await editChannelPricing(page, s, testInfo, { '^输入$|^Input$': '5', '^输出$|^Output$': '30', '图片输入|Image Input': '10' }, false)
  let r = await gatewayOpenAITry({ path: '/v1/chat/completions', apiKey: s.key.key, body: openaiChatBody('gpt-5.5') })
  expect(r.status, r.text).toBe(200)
  let id = await assertLatestCost(s.key.id, '0.01850000')
  expect(Number(scalar(`select input_cost from usage_logs where id=${id}`)).toFixed(8)).toBe('0.00250000')
  expect(Number(scalar(`select image_input_cost from usage_logs where id=${id}`)).toFixed(8)).toBe('0.01000000')
  await editChannelPricing(page, s, testInfo, { '图片输入|Image Input': '' }, false)
  r = await gatewayOpenAITry({ path: '/v1/chat/completions', apiKey: s.key.key, body: openaiChatBody('gpt-5.5') })
  expect(r.status, r.text).toBe(200)
  id = await assertLatestCost(s.key.id, '0.01350000')
  await verifyUserUsage(browser, page, s, testInfo, ['$0.01850000', '$0.01350000'], 'AC26-image-input-and-fallback')
})

test('AC27 界面按张开关、无法解析按一张、混合请求仍按 token', async ({ page, browser }, testInfo) => {
  const s = await seed({ prefix: 'ac27', platform: 'openai', balance: 10, group: { allow_image_generation: true } })
  await editImageGroup(page, s, testInfo, true)
  await scenario(s.upstreamKey, { images: 3, usage: { input: 1000, output: 100 } })
  let r = await gatewayOpenAITry({ path: '/v1/images/generations', apiKey: s.key.key, body: imageBody('gpt-image-1', { size: '1024x1024' }) })
  expect(r.status, r.text).toBe(200)
  let id = await assertLatestCost(s.key.id, '0.12000000')
  expect(scalar(`select image_count from usage_logs where id=${id}`)).toBe('3')
  await scenario(s.upstreamKey, { images: 'unknown', usage: { input: 1000, output: 100 } })
  r = await gatewayOpenAITry({ path: '/v1/images/generations', apiKey: s.key.key, body: imageBody('gpt-image-1', { size: '1024x1024' }) })
  expect(r.status, r.text).toBe(200)
  id = await assertLatestCost(s.key.id, '0.04000000')
  expect(scalar(`select image_count from usage_logs where id=${id}`)).toBe('1')
  await scenario(s.upstreamKey, { usage: { input: 1000, output: 100 }, stream: false })
  r = await gatewayOpenAITry({ path: '/v1/responses', apiKey: s.key.key, body: { model: 'gpt-5.5', input: 'mixed request', modalities: ['text', 'image'] } })
  expect(r.status, r.text).toBe(200)
  id = await assertLatestCost(s.key.id, '0.00800000')
  expect(scalar(`select billing_mode from usage_logs where id=${id}`)).toBe('token')
  await editImageGroup(page, s, testInfo, false)
  await scenario(s.upstreamKey, { images: 3, usage: { input: 1000, output: 100 } })
  r = await gatewayOpenAITry({ path: '/v1/images/generations', apiKey: s.key.key, body: imageBody('gpt-image-1', { size: '1024x1024' }) })
  expect(r.status, r.text).toBe(200)
  id = await assertLatestCost(s.key.id, '0.12000000')
  expect(scalar(`select billing_mode from usage_logs where id=${id}`)).toBe('image')
  await verifyUserUsage(browser, page, s, testInfo, ['$0.12000000', '$0.04000000'], 'AC27-image-count-switch-and-fallback')
})

test('AC28 Gemini 两张 inlineData 与一张 fileData 共三张', async ({ page }, testInfo) => {
  const s = await seed({ prefix: 'ac28', platform: 'gemini', balance: 10 })
  await scenario(s.upstreamKey, { gemini_images: true, usage: { input: 10, output: 5 } })
  const r = await raw('POST', 'http://127.0.0.1:8280/v1beta/models/gemini-3-pro-image-preview:generateContent', { contents: [{ role: 'user', parts: [{ text: 'three images' }] }], generationConfig: { imageConfig: { imageSize: '1K' } } }, undefined, { authorization: 'Bearer ' + s.key.key })
  expect(r.status, r.text).toBe(200)
  const id = await assertLatestCost(s.key.id, '0.40200000')
  expect(scalar(`select image_count from usage_logs where id=${id}`)).toBe('3')
  await login(page, s.email, s.password, testInfo.project.name === 'mobile-en' ? 'en-US' : 'zh-CN')
  await navigate(page, '/usage')
  await expect(page.locator('body')).toContainText('$0.40200000')
  await screenshot(page, testInfo, 'AC28-gemini-three-images')
})
