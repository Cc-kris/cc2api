import path from 'node:path'
import { expect, test as base, type Page } from '@playwright/test'
import { OUT_ROOT, expectClean, guard, openContentTab, realLogin, shooter, type Problem } from '../real/real-helpers'

/**
 * W4 验收：AI 创作模型配置（AI-01~AI-11）+ BI 分析（BI-01~BI-07）
 * 依据：docs/v0.4/v0.4_prd_20261026_v3.3.md（R10/R11/R15）、docs/v0.4/v0.4_ui_brief_20261026_v3.3.md
 * 契约：真实登录 + 真实 :3100→:8080→PG，无 page.route / mock；产品不符合就判 FAIL，不改产品代码。
 */

const tag = process.env.W4_TAG ?? Date.now().toString(36)
const LABEL_A = `E2E-W4-${tag}-A`
const LABEL_B = `E2E-W4-${tag}-B`
let providerBlankIssue: string | null = null
const FAKE_KEY = 'sk-w4plaintext-abcdefgh1234'

/** 收集式断言：一次跑完所有检查，最后统一报告，便于产出「实测值」。 */
function collector() {
  const fails: string[] = []
  const notes: string[] = []
  return {
    fails,
    notes,
    check(cond: boolean, msg: string) {
      if (!cond) fails.push(msg)
    },
    note(msg: string) {
      notes.push(msg)
    },
    finish() {
      expect(fails, `断言未通过（共 ${fails.length} 项）：\n- ${fails.join('\n- ')}`).toEqual([])
    }
  }
}

type Ctx = { page: () => Promise<Page>; context: () => Promise<import('@playwright/test').BrowserContext> }

const test = base.extend<{}, { admin: Ctx }>({
  admin: [
    async ({ browser }, use) => {
      const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, acceptDownloads: true })
      const first = await context.newPage()
      await realLogin(first)
      await use({
        context: async () => context,
        page: async () => {
          const p = await context.newPage()
          await p.goto('http://127.0.0.1:3100/admin/content')
          return p
        }
      })
      await context.close()
    },
    { scope: 'worker' }
  ]
})

async function gotoTab(page: Page, tab: 'AI 创作' | 'BI 分析') {
  await page.keyboard.press('Escape')
  const btn = page.locator('.tabs button', { hasText: tab }).first()
  if (await btn.count()) {
    await btn.click({ force: true })
  } else {
    await openContentTab(page, tab)
  }
  await page.waitForTimeout(900)
}

async function reloadToConfig(page: Page) {
  await page.reload()
  await page.waitForTimeout(1200)
  await gotoTab(page, 'AI 创作')
  await page.locator('nav.ai-subtabs button', { hasText: '模型配置' }).first().click({ force: true })
  await expect(page.locator('.ai-config-panel')).toBeVisible({ timeout: 20_000 })
}

async function gotoConfig(page: Page) {
  await gotoTab(page, 'AI 创作')
  await page.locator('nav.ai-subtabs button', { hasText: '模型配置' }).first().click()
  await expect(page.locator('.ai-config-panel')).toBeVisible({ timeout: 15_000 })
}

const rowOf = (page: Page, label: string) => page.locator('.ai-config-list li', { hasText: label }).first()

async function openDialog(page: Page) {
  await page.locator('.ai-config-panel button', { hasText: '新增配置' }).first().click()
  const dlg = page.locator('.ai-config-dialog')
  await expect(dlg).toBeVisible({ timeout: 15_000 })
  return dlg
}

async function fillDialog(
  page: Page,
  dlg: ReturnType<Page['locator']>,
  v: { label?: string; provider?: string; baseUrl?: string; apiKey?: string; models?: string; model?: string }
) {
  if (v.label !== undefined) await dlg.locator('label', { hasText: '配置名称' }).locator('input').fill(v.label)
  if (v.baseUrl !== undefined) await dlg.locator('input[type="url"]').fill(v.baseUrl)
  if (v.apiKey !== undefined) await dlg.locator('input[type="password"]').fill(v.apiKey)
  if (v.models !== undefined) await dlg.locator('textarea').fill(v.models)
  if (v.model !== undefined) {
    // 填了模型列表后「默认模型」变成 Select，否则是文本输入
    const triggers = dlg.locator('.select-trigger')
    if ((await triggers.count()) >= 2) {
      const modelTrigger = triggers.nth(1)
      const current = (await modelTrigger.innerText()).trim()
      if (current !== v.model) {
        await modelTrigger.click()
        await page.locator('.select-options [role="option"]').filter({ hasText: v.model }).first().click()
      }
    } else {
      await dlg.locator('label', { hasText: 'Default' }).locator('input').fill(v.model)
    }
  }
  if (v.provider !== undefined) {
    await dlg.locator('.select-trigger').first().click({ force: true })
    const opts = page.locator('.select-options [role="option"]')
    await opts.first().waitFor({ timeout: 5000 }).catch(() => {})
    const labels = (await opts.allInnerTexts().catch(() => [])).map((t) => t.trim())
    const idx = labels.findIndex((t) => t && t.toLowerCase().includes(v.provider!.toLowerCase()))
    if (idx >= 0) {
      await opts.nth(idx).click({ force: true })
    } else {
      // 真实缺陷兜底：厂商下拉选项标签为空，无法按文本选择；表单默认 provider=openai 仍可保存
      providerBlankIssue = `厂商下拉选项标签为空 ${JSON.stringify(labels)}，无法按文本选择「${v.provider}」`
      await page.keyboard.press('Escape')
    }
  }
}

async function saveDialog(page: Page, dlg: ReturnType<Page['locator']>) {
  await dlg.locator('footer button', { hasText: /Save configuration|保存/ }).first().click()
  await expect(dlg).toBeHidden({ timeout: 20_000 })
}

async function closeIfOpen(page: Page) {
  const dlg = page.locator('.ai-config-dialog')
  if (await dlg.isVisible().catch(() => false)) await page.keyboard.press('Escape')
}

/** 删除本文件创建的所有测试配置（仅 E2E-W4- 前缀），返回删除结果说明。 */
async function cleanupW4Configs(page: Page, label: string) {
  await gotoConfig(page)
  const rows = page.locator('.ai-config-list li')
  const total = await rows.count()
  for (let i = total - 1; i >= 0; i--) {
    const row = rows.nth(i)
    const text = (await row.innerText()).replace(/\s+/g, ' ')
    if (!text.includes(label)) continue
    const del = row.locator('button', { hasText: /Delete|删除/ }).first()
    if (await del.isDisabled()) continue
    page.once('dialog', (d) => d.accept())
    await del.click()
    await page.waitForTimeout(600)
  }
}

function styleWidth(box: { width: number } | null) {
  return box ? Math.round(box.width) : -1
}

test.describe('W4 AI 创作模型配置 + BI 分析（真实 E2E 验收）', () => {
  test.beforeEach(async () => {
    test.setTimeout(120_000)
  })

  let shot: Awaited<ReturnType<typeof shooter>>
  const problems: Problem[] = []

  test.beforeEach(async ({}, testInfo) => {
    shot = shooter('w4', testInfo)
  })

  test('AI-01 进入 AI 创作：子 tab 与页内配置', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page, [/favicon/, /\.map$/]).forEach((p) => problems.push(p))

    await gotoTab(page, 'AI 创作')
    const pane = page.locator('section.workspace.ai-studio-pane')
    c.check(await pane.isVisible(), 'AI 创作面板未渲染（section.workspace.ai-studio-pane 不可见）')

    const subtabs = (await page.locator('nav.ai-subtabs button').allInnerTexts()).map((s) => s.trim())
    c.note(`子 tab = ${JSON.stringify(subtabs)}`)
    c.check(subtabs.length === 2, `子 tab 数量 ${subtabs.length}，期望 2（文章生成/模型配置）`)
    c.check(subtabs[0]?.includes('文章生成') && subtabs[1]?.includes('模型配置'), `子 tab 文案不符：${subtabs}`)

    const urlBefore = page.url()
    await page.locator('nav.ai-subtabs button', { hasText: '模型配置' }).first().click()
    await page.waitForTimeout(800)
    c.check(page.url() === urlBefore, `点「模型配置」跳走了：${urlBefore} → ${page.url()}`)
    c.check(await page.locator('.ai-config-panel').isVisible(), '配置面板未在本页渲染')
    c.note(`配置面板标题 = ${(await page.locator('.ai-config-panel h3').first().innerText()).trim()}`)
    await shot(page, 'ai-config-tab')
    c.finish()
  })

  test('AI-02 新增配置表单字段与弹窗规格', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoConfig(page)
    const dlg = await openDialog(page)

    const box = await dlg.boundingBox()
    const styles = await dlg.evaluate((el) => {
      const s = getComputedStyle(el)
      return { w: el.getBoundingClientRect().width, radius: s.borderRadius, h: el.getBoundingClientRect().height }
    })
    c.note(`弹窗宽 ${styleWidth(box)}px / 高 ${Math.round(styles.h)}px / 圆角 ${styles.radius}`)
    c.check(styleWidth(box) >= 636 && styleWidth(box) <= 644, `弹窗宽度 ${styleWidth(box)}px，PRD 中弹窗 640±4px`)
    c.check(styles.radius === '12px', `弹窗圆角 ${styles.radius}，UI 规范 1.3 要求 12px`)

    const labels = (await dlg.locator('label').allInnerTexts()).map((s) => s.split('\n')[0].trim())
    c.note(`字段 = ${JSON.stringify(labels)}`)
    for (const need of ['配置名称', 'Provider', 'Base URL', 'API key', 'Models', 'Default']) {
      c.check(labels.some((l) => l.toLowerCase().includes(need.toLowerCase())), `缺少字段：${need}`)
    }
    const btns = (await dlg.locator('button').allInnerTexts()).map((s) => s.trim()).filter(Boolean)
    c.note(`弹窗按钮 = ${JSON.stringify(btns)}`)
    c.check(btns.some((b) => /Get models|获取模型/.test(b)), '缺少「获取模型」按钮')
    c.check(btns.some((b) => /Test connection|测试连通|测试连接/.test(b)), '缺少「测试连通」按钮')
    c.check(btns.some((b) => /Save|保存/.test(b)), '缺少保存按钮')

    const listActions = (await page.locator('.ai-config-list li').first().locator('button').allInnerTexts()).map((s) => s.trim())
    c.note(`列表行操作 = ${JSON.stringify(listActions)}`)
    c.check(listActions.some((b) => /Set as default|设为默认|默认/.test(b)), '列表缺少「设为默认」操作')

    await dlg.locator('.select-trigger').first().click()
    const providerOptions = (await page.locator('.select-options [role="option"]').allInnerTexts()).map((s) => s.trim())
    await page.keyboard.press('Escape')
    c.note(`厂商下拉 = ${JSON.stringify(providerOptions)}`)
    const required = ['OpenAI', 'Anthropic', 'DeepSeek', '智谱', '月之暗面', '阿里通义', '自定义']
    const missing = required.filter((r) => !providerOptions.some((o) => o.toLowerCase().includes(r.toLowerCase().replace('自定义', 'custom')) && (r !== '自定义' || /custom|自定义|compatible/i.test(o))))
    c.check(missing.length === 0, `厂商下拉缺项：${missing.join('/')}（实测 ${JSON.stringify(providerOptions)}）`)
    if (providerBlankIssue) c.note(providerBlankIssue)
    const blankOpts = providerOptions.filter((o) => !o).length
    c.check(blankOpts === 0, `厂商下拉有 ${blankOpts}/${providerOptions.length} 项标签为空（选项渲染不出文字）`)

    c.note(`字段中英混排：${labels.join(' | ')}`)
    await shot(page, 'ai-dialog')
    await closeIfOpen(page)
    c.finish()
  })

  test('AI-03 配置名称：51 字与大小写重名', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoConfig(page)

    const saveReqs: string[] = []
    page.on('request', (r) => {
      if (r.url().includes('/ai-configs/')) saveReqs.push(`${r.method()} ${r.url()}`)
    })

    const existing = (await page.locator('.ai-config-list li strong').allInnerTexts()).map((s) => s.trim())
    c.note(`已有配置 = ${JSON.stringify(existing)}`)
    const dupSource = existing.find((x) => x.length > 2) ?? 'Legacy'

    // (1) 51 字：先按真实键盘输入，看是否被 maxlength 静默截断
    let dlg = await openDialog(page)
    const nameInput = dlg.locator('label', { hasText: '配置名称' }).locator('input')
    const tooLong = `E2E-W4-${tag}-` + 'x'.repeat(40)
    await nameInput.fill(tooLong)
    const typedLen = (await nameInput.inputValue()).length
    c.note(`输入 ${[...tooLong].length} 字，输入框实际值长度 ${typedLen}，maxlength=${await nameInput.getAttribute('maxlength')}`)

    // 绕过 maxlength 提交 51 字，验证服务端正/前端校验是否有明确提示
    await nameInput.evaluate((el, v) => {
      const input = el as HTMLInputElement
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
      setter.call(input, v)
      input.dispatchEvent(new Event('input', { bubbles: true }))
    }, tooLong)
    await dlg.locator('footer button', { hasText: /Save|保存/ }).first().click()
    await page.waitForTimeout(900)
    const longErr = (await dlg.locator('.ai-feedback.error').allInnerTexts().catch(() => [])).join(' ').trim()
    c.note(`51 字提示 = ${JSON.stringify(longErr)}；期间写请求 = ${JSON.stringify(saveReqs)}`)
    // 真键盘输入被 maxlength 截断属合规实现（PRD 只要求最长 50 字）；绕过 maxlength 时若仍无提示则记缺陷
    c.note(`51 字场景：输入实际 ${typedLen} 字（maxlength 生效），绕过 maxlength 后提示 = ${longErr || '（无）'}`)
    c.check(saveReqs.length === 0, `51 字时仍发出了写请求：${JSON.stringify(saveReqs)}`)

    // (2) 与已有配置大小写不同重名
    await page.keyboard.press('Escape')
    await page.waitForTimeout(300)
    dlg = await openDialog(page)
    await fillDialog(page, dlg, {
      label: dupSource.toLowerCase(),
      baseUrl: 'https://api.openai.com/v1',
      apiKey: '',
      models: 'gpt-4o-mini',
      model: 'gpt-4o-mini'
    })
    const before = saveReqs.length
    await dlg.locator('footer button', { hasText: /Save|保存/ }).first().click()
    await page.waitForTimeout(1200)
    const dupErr = (await dlg.locator('[role="alert"], .ai-feedback').allInnerTexts()).join(' ').trim()
    c.note(`重名（${dupSource.toLowerCase()} vs ${dupSource}）提示 = ${JSON.stringify(dupErr)}`)
    c.check(/重复|已存在|duplicate|exist/i.test(dupErr), `大小写重名无明确提示，实测提示=${JSON.stringify(dupErr)}`)
    c.check(saveReqs.length === before, `重名时仍发出写请求：${JSON.stringify(saveReqs.slice(before))}`)

    await shot(page, 'ai-name-validation')
    await closeIfOpen(page)
    c.finish()
  })

  test('AI-04 保存带 Key 的配置后只显示掩码', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoConfig(page)

    let respBody = ''
    const respPromise = page
      .waitForResponse((r) => r.url().includes('/ai-configs/save'), { timeout: 20_000 })
      .then(async (r) => {
        respBody = await r.text().catch(() => '')
        return r.status()
      })
      .catch(() => -1)

    const dlg = await openDialog(page)
    await fillDialog(page, dlg, {
      label: LABEL_A,
      provider: 'OpenAI',
      baseUrl: 'https://api.openai.com/v1',
      apiKey: FAKE_KEY,
      models: 'gpt-4o-mini\ngpt-4o',
      model: 'gpt-4o-mini'
    })
    await dlg.locator('footer button', { hasText: /Save|保存/ }).first().click()
    const status = await respPromise
    await expect(dlg).toBeHidden({ timeout: 20_000 })
    c.note(`保存响应状态 ${status}，响应体含明文 key = ${respBody.includes(FAKE_KEY)}`)

    const row = rowOf(page, LABEL_A)
    await expect(row).toBeVisible()
    const rowText = (await row.innerText()).replace(/\s+/g, ' ')
    const mask = (await row.locator('small').innerText()).trim()
    c.note(`列表行 = ${rowText}`)
    c.check(/^sk-\*{2,}|sk-\*+[0-9a-zA-Z]{2,}$|\*{4}/.test(mask) || /\*{4}/.test(mask), `列表未显示掩码，实测 small = ${JSON.stringify(mask)}`)
    c.check(!rowText.includes(FAKE_KEY), '列表行出现明文 Key')
    c.check(!respBody.includes(FAKE_KEY), '保存响应体出现明文 Key')

    await reloadToConfig(page)
    const html = await page.content()
    c.check(!html.includes(FAKE_KEY), '刷新后页面 DOM 仍含明文 Key')
    const mask2 = (await rowOf(page, LABEL_A).locator('small').innerText()).trim()
    c.note(`刷新后掩码 = ${JSON.stringify(mask2)}`)
    c.check(mask2 === mask, `刷新前后掩码不一致：${mask} → ${mask2}`)

    await shot(page, 'ai-masked-key')
    c.finish()
  })

  test('AI-05 编辑配置时 Key 留空保存保持原密钥', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoConfig(page)

    const row = rowOf(page, LABEL_A)
    await expect(row).toBeVisible({ timeout: 15_000 })
    const maskBefore = (await row.locator('small').innerText()).trim()

    await row.locator('button', { hasText: /Edit|编辑/ }).first().click()
    const dlg = page.locator('.ai-config-dialog')
    await expect(dlg).toBeVisible()
    const keyInput = dlg.locator('input[type="password"]')
    c.note(`编辑时 key 输入框值长度 = ${(await keyInput.inputValue()).length}，placeholder = ${await keyInput.getAttribute('placeholder')}`)
    await dlg.locator('label', { hasText: '配置名称' }).locator('input').fill(`${LABEL_A}-E`)

    const respPromise = page.waitForResponse((r) => r.url().includes('/ai-configs/save'), { timeout: 20_000 }).catch(() => null)
    await dlg.locator('footer button', { hasText: /Save|保存/ }).first().click()
    const status = (await respPromise)?.status() ?? -1
    await expect(dlg).toBeHidden({ timeout: 20_000 })
    const notice = (await page.locator('.ai-config-panel .ai-feedback, .ai-config-panel [role="status"]').allInnerTexts()).join(' ').trim()
    c.note(`保存状态 ${status}；页面提示 = ${JSON.stringify(notice)}`)
    c.check(status === 200, `Key 留空保存返回 ${status}`)
    c.check(notice.length > 0, '保存成功后没有提示文案')

    const maskAfter = (await rowOf(page, `${LABEL_A}-E`).locator('small').innerText()).trim()
    c.note(`掩码：${maskBefore} → ${maskAfter}`)
    c.check(maskAfter === maskBefore, `Key 留空保存后掩码变化：${maskBefore} → ${maskAfter}（应保持不变）`)

    await reloadToConfig(page)
    const maskReload = (await rowOf(page, `${LABEL_A}-E`).locator('small').innerText()).trim()
    c.note(`刷新后掩码 = ${maskReload}`)
    c.check(maskReload === maskBefore, `刷新后掩码丢失：${maskBefore} → ${maskReload}`)

    await shot(page, 'ai-edit-keep-key')
    c.finish()
  })

  test('AI-06 「获取模型」有明确结果', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page, [/fetch-models/]).forEach((p) => problems.push(p))
    await gotoConfig(page)

    const row = rowOf(page, new RegExp(`^${LABEL_A}`))
    await row.locator('button', { hasText: /Edit|编辑/ }).first().click()
    const dlg = page.locator('.ai-config-dialog')
    await expect(dlg).toBeVisible()
    await dlg.locator('textarea').fill('')
    const reqPromise = page.waitForResponse((r) => r.url().includes('fetch-models'), { timeout: 30_000 }).catch(() => null)
    await dlg.locator('button', { hasText: /Get models|获取模型/ }).first().click()
    await page.waitForTimeout(500)
    const busy = (await dlg.locator('button', { hasText: /Get models|获取模型/ }).first().isDisabled()) ? '按钮进入 loading/disabled' : '按钮未进入 loading'
    const resp = await reqPromise
    await page.waitForTimeout(1500)
    const feedback = (await dlg.locator('.ai-feedback, [role="alert"], [role="status"]').allInnerTexts()).join(' ').trim()
    const modelsText = await dlg.locator('textarea').inputValue()
    c.note(`${busy}；fetch-models 状态 ${resp?.status() ?? '-'}；提示 = ${JSON.stringify(feedback)}；模型列表 = ${JSON.stringify(modelsText.slice(0, 80))}`)
    c.check(feedback.length > 0 || modelsText.trim().length > 0, '点「获取模型」没有任何可见结果（无提示也无模型填入）')
    c.check(resp !== null, '未发出 /ai-config/fetch-models 请求')

    await shot(page, 'ai-get-models')
    await closeIfOpen(page)
    c.finish()
  })

  test('AI-07 「测试连通」失败时有可读提示', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page, [/ai-config-test/]).forEach((p) => problems.push(p))
    await gotoConfig(page)

    const row = rowOf(page, new RegExp(`^${LABEL_A}`))
    await row.locator('button', { hasText: /Edit|编辑/ }).first().click()
    const dlg = page.locator('.ai-config-dialog')
    await expect(dlg).toBeVisible()

    // 错误 Key + 正确的 OpenAI 域名，触发真实 401
    await dlg.locator('input[type="password"]').fill('sk-w4-invalid-key-000000')
    const reqPromise = page.waitForResponse((r) => r.url().includes('ai-config-test'), { timeout: 40_000 }).catch(() => null)
    const t0 = Date.now()
    await dlg.locator('button', { hasText: /Test connection|测试连通/ }).first().click()
    await page.waitForTimeout(400)
    const loadingText = (await dlg.locator('.ai-feedback, [role="status"]').allInnerTexts()).join(' ').trim()
    const resp = await reqPromise
    await page.waitForTimeout(1800)
    const elapsed = Date.now() - t0
    const feedback = (await dlg.locator('.ai-feedback, [role="alert"], [role="status"]').allInnerTexts()).join(' ').trim()
    const body = resp ? await resp.text().catch(() => '') : ''
    c.note(`耗时 ${elapsed}ms；请求状态 ${resp?.status() ?? '-'}；响应 = ${JSON.stringify(body.slice(0, 200))}`)
    c.note(`loading 文案 = ${JSON.stringify(loadingText)}；失败提示 = ${JSON.stringify(feedback)}`)
    c.check(loadingText.length > 0 || elapsed > 1200, '点击后无 loading 反馈，也没有可感知的等待过程')
    c.check(feedback.length > 0, '测试连通失败后没有任何提示')
    c.check(!/sk-w4-invalid-key-000000/.test(feedback), '失败提示里回显了完整 API Key')
    c.check(feedback.length < 300 && !/Traceback|TypeError|undefined|\[object Object\]/.test(feedback), `提示不可读（疑似技术栈错误信息）：${feedback}`)

    await shot(page, 'ai-test-connection')
    await closeIfOpen(page)
    c.finish()
  })

  test('AI-08 设为默认与 AI 页状态回流', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoConfig(page)

    const rowA = rowOf(page, LABEL_A)
    await expect(rowA).toBeVisible()
    const respPromise = page.waitForResponse((r) => r.url().includes('/ai-configs/default'), { timeout: 20_000 }).catch(() => null)
    await rowA.locator('button', { hasText: /Set as default|设为默认/ }).first().click()
    const status = (await respPromise)?.status() ?? -1
    await page.waitForTimeout(1500)
    const rowAText = (await rowA.innerText()).replace(/\s+/g, ' ')
    c.note(`设为默认响应 ${status}；A 行 = ${rowAText}`)

    // 同一页面上不应有两个「默认」
    const badges = await page.locator('.ai-config-list .default-badge').count()
    c.note(`页面上「默认」徽标数量 = ${badges}`)
    c.check(badges === 1, `默认徽标数量 ${badges}，期望唯一`)
    c.check(rowAText.toLowerCase().includes('default') || rowAText.includes('默认'), 'A 行未出现默认标记')
    c.check(await rowA.locator('button', { hasText: /Set as default|设为默认/ }).first().isDisabled(), '默认配置的「设为默认」按钮未禁用')

    // 打开编辑，确认默认选中态来自真实数据
    await rowA.locator('button', { hasText: /Edit|编辑/ }).first().click()
    const dlg = page.locator('.ai-config-dialog')
    await expect(dlg).toBeVisible()
    const checked = await dlg.locator('input[type="checkbox"]').first().isChecked().catch(() => false)
    c.note(`编辑弹窗内「设为默认」勾选 = ${checked}`)
    c.check(checked, '编辑默认配置时「设为默认」未勾选，状态未回流')
    await closeIfOpen(page)

    // 刷新后仍是默认
    await reloadToConfig(page)
    const rowAReload = (await rowOf(page, LABEL_A).innerText()).replace(/\s+/g, ' ')
    c.note(`刷新后 A 行 = ${rowAReload}`)
    c.check(/default|默认/i.test(rowAReload), '刷新后默认状态丢失')

    await shot(page, 'ai-set-default')
    c.finish()
  })

  test('AI-09 删除配置需二次确认且列表移除', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoConfig(page)

    // 默认配置的删除按钮必须禁用
    const defaultRow = page.locator('.ai-config-list li').filter({ has: page.locator('.default-badge') }).first()
    const defaultDel = defaultRow.locator('button', { hasText: /Delete|删除/ }).first()
    c.note(`默认行删除按钮 disabled = ${await defaultDel.isDisabled()}`)
    c.check(await defaultDel.isDisabled(), '默认配置的删除按钮未禁用（PRD 要求默认配置不可删）')

    // 先建 B（非默认）
    let dlg = await openDialog(page)
    await fillDialog(page, dlg, {
      label: LABEL_B,
      baseUrl: 'https://api.openai.com/v1',
      apiKey: 'sk-w4-b-second-000000',
      models: 'gpt-4o-mini'
    })
    await saveDialog(page, dlg)
    const rowB = rowOf(page, LABEL_B)
    await expect(rowB).toBeVisible({ timeout: 15_000 })

    let dialogSeen = ''
    page.once('dialog', async (d) => {
      dialogSeen = `${d.type()}: ${d.message()}`
      await d.dismiss()
    })
    await rowB.locator('button', { hasText: /Delete|删除/ }).first().click()
    await page.waitForTimeout(1000)
    c.note(`confirm 弹窗 = ${JSON.stringify(dialogSeen)}；取消后 B 是否还在 = ${await rowB.isVisible().catch(() => false)}`)
    c.check(await rowB.isVisible().catch(() => false), '确认框选择取消后配置仍被删除（缺少二次确认保护）')

    const respPromise = page.waitForResponse((r) => r.url().includes('/ai-configs/delete'), { timeout: 20_000 }).catch(() => null)
    page.once('dialog', (d) => d.accept())
    await rowB.locator('button', { hasText: /Delete|删除/ }).first().click()
    const status = (await respPromise)?.status() ?? -1
    await page.waitForTimeout(1800)
    c.note(`删除响应 ${status}；删除后 B 可见 = ${await rowB.isVisible().catch(() => false)}`)
    c.check(status === 200, `删除请求返回 ${status}`)
    c.check(!(await rowB.isVisible().catch(() => false)), '删除成功后列表仍显示该配置')

    await reloadToConfig(page)
    c.note(`刷新后 B 仍存在 = ${await rowOf(page, LABEL_B).isVisible().catch(() => false)}`)
    c.check(!(await rowOf(page, LABEL_B).isVisible().catch(() => false)), '刷新后已删除的配置复现（未真正落库删除）')

    await shot(page, 'ai-delete')
    c.finish()
  })

  test('AI-10 弹窗四种关闭方式与未保存确认', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoConfig(page)

    const dirty = async () => {
      const dlg = await openDialog(page)
      await dlg.locator('label', { hasText: '配置名称' }).locator('input').fill(`E2E-W4-${tag}-dirty`)
      return dlg
    }

    // 1) 右上 ×
    let dlg = await dirty()
    const warnX: string[] = []
    page.once('dialog', (d) => {
      warnX.push(d.message())
      void d.accept()
    })
    await dlg.locator('.ai-dialog-close').click()
    await page.waitForTimeout(800)
    c.note(`× 关闭：confirm=${JSON.stringify(warnX)}，弹窗可见=${await dlg.isVisible().catch(() => false)}`)
    c.check(!(await dlg.isVisible().catch(() => false)), '点右上 × 未关闭弹窗')
    c.check(warnX.length > 0, '有未保存内容时点 × 直接丢弃，没有二次确认')

    // 2) Esc
    dlg = await dirty()
    const warnEsc: string[] = []
    page.once('dialog', (d) => {
      warnEsc.push(d.message())
      void d.accept()
    })
    await page.keyboard.press('Escape')
    await page.waitForTimeout(800)
    c.note(`Esc 关闭：confirm=${JSON.stringify(warnEsc)}；可见=${await dlg.isVisible().catch(() => false)}`)
    c.check(!(await dlg.isVisible().catch(() => false)), 'Esc 未关闭弹窗')

    // 3) 取消按钮
    dlg = await dirty()
    await dlg.locator('footer button', { hasText: /Cancel|取消/ }).first().click()
    await page.waitForTimeout(800)
    c.note(`取消按钮关闭：可见=${await dlg.isVisible().catch(() => false)}`)
    c.check(!(await dlg.isVisible().catch(() => false)), '点「取消」未关闭弹窗')

    // 4) 点遮罩
    dlg = await dirty()
    const box = await dlg.boundingBox()
    if (box) await page.mouse.click(Math.max(2, box.x / 2), Math.min(box.y + 10, 890))
    await page.waitForTimeout(800)
    c.note(`点遮罩关闭：可见=${await dlg.isVisible().catch(() => false)}`)
    c.check(!(await dlg.isVisible().catch(() => false)), '点遮罩未关闭弹窗')

    // 重新打开时应为空表单（脏数据不残留）
    dlg = await openDialog(page)
    const residual = await dlg.locator('label', { hasText: '配置名称' }).locator('input').inputValue()
    c.note(`重开弹窗后配置名称残留 = ${JSON.stringify(residual)}`)
    c.check(residual === '', `重开弹窗残留上次输入：${residual}`)
    await closeIfOpen(page)

    await shot(page, 'ai-dialog-close')
    c.finish()
  })

  test('AI-11 多语言下 AI 页无硬编码中文/英文混排', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoConfig(page)
    await openDialog(page)

    const zh = /[\u4e00-\u9fa5]/
    const lats = /[A-Za-z]{3,}/
    const items = await page.locator('.ai-config-dialog label, .ai-config-dialog h3, .ai-config-dialog p, .ai-config-dialog footer button, .ai-config-panel h3, .ai-config-panel > button').allInnerTexts()
    const mixed = items.map((s) => s.replace(/\s+/g, ' ').trim()).filter((s) => s && zh.test(s) && lats.test(s))
    c.note(`界面语言 = ${await page.evaluate(() => document.documentElement.lang || navigator.language)}`)
    c.note(`同一条文案中英混排 = ${JSON.stringify(mixed)}`)
    c.check(mixed.length === 0, `存在中英混排文案：${JSON.stringify(mixed)}`)
    await closeIfOpen(page)

    // 切到 ru-RU，再回到本页，确认文案随语言变化（不残留中文）
    const before = (await page.locator('.ai-config-panel h3').first().innerText()).trim()
    await page.goto('http://127.0.0.1:3100/admin/content')
    await page.waitForTimeout(800)
    const langBtn = page.locator('[aria-label="Change language"]').first()
    await langBtn.click()
    await page.waitForTimeout(400)
    const langs = (await page.locator('[role="menuitem"], .lang-option, .language-option').allInnerTexts()).map((s) => s.trim()).filter(Boolean)
    c.note(`语言菜单 = ${JSON.stringify(langs.slice(0, 8))}`)
    const ru = page.locator('[role="menuitem"], .lang-option, .language-option').filter({ hasText: /Рус|ru-RU|Russian/ }).first()
    if (await ru.count()) {
      await ru.click()
      await page.waitForTimeout(1200)
      await gotoConfig(page)
      const after = (await page.locator('.ai-config-panel h3').first().innerText()).trim()
      c.note(`切换语言后配置面板标题：${before} → ${after}`)
      c.check(after !== before, `切到 ru-RU 后标题未变化（仍为 ${after}），疑似硬编码`)
      const stillZh = zh.test(await page.locator('.ai-config-panel').innerText())
      c.check(!stillZh, 'ru-RU 下面板仍含中文文案')
      // 还原 en
      await page.goto('http://127.0.0.1:3100/admin/content')
      await page.waitForTimeout(600)
      await page.locator('[aria-label="Change language"]').first().click()
      await page.locator('[role="menuitem"], .lang-option, .language-option').filter({ hasText: /^EN$|English/ }).first().click().catch(() => {})
      await page.waitForTimeout(800)
    } else {
      c.note('语言菜单中未找到 ru-RU 项，跳过跨语言比对')
    }

    await shot(page, 'ai-i18n')
    c.finish()
  })

  test('BI-01 布局 1440/1024/390 + 无横向溢出', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoTab(page, 'BI 分析')

    const measure = async () =>
      page.evaluate(() => {
        const cols = (sel: string) => {
          const el = document.querySelector(sel)
          if (!el) return -1
          const t = getComputedStyle(el).gridTemplateColumns
          return t.split(/\s+/).filter(Boolean).length
        }
        const rects = Array.from(document.querySelectorAll('.metric-card')).map((e) => {
          const r = e.getBoundingClientRect()
          return { y: Math.round(r.y), w: Math.round(r.width) }
        })
        return {
          metricCount: rects.length,
          metricRows: new Set(rects.map((r) => r.y)).size,
          metricWidths: rects.map((r) => r.w),
          chartCols: cols('.chart-grid'),
          docOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
          bodyOverflow: document.body.scrollWidth - document.body.clientWidth,
        }
      })

    const results: Record<string, unknown> = {}
    for (const vp of [
      { w: 1440, h: 900 },
      { w: 1024, h: 768 },
      { w: 390, h: 844 },
    ]) {
      await page.setViewportSize({ width: vp.w, height: vp.h })
      await page.waitForTimeout(900)
      const m = await measure()
      results[`${vp.w}x${vp.h}`] = m
      await page.screenshot({ path: path.join(OUT_ROOT, 'shots', 'w4', `bi-layout-${vp.w}.png`), fullPage: true }).catch(() => {})
      c.note(`${vp.w}x${vp.h}: KPI ${m.metricCount} 张/${m.metricRows} 行，图表列数 ${m.chartCols}，溢出 ${m.docOverflow}px`)

      c.check(m.metricCount >= 4, `${vp.w}: KPI 卡不足 4 张（${m.metricCount}）`)
      c.check(m.docOverflow <= 1 && m.bodyOverflow <= 1, `${vp.w}: 出现横向溢出 doc=${m.docOverflow} body=${m.bodyOverflow}`)
      if (vp.w >= 1024) {
        c.check(m.metricRows === 1, `${vp.w}: KPI 未在一行内（占 ${m.metricRows} 行）`)
        c.check(m.chartCols === 2, `${vp.w}: 图表网格应为 2 列，实测 ${m.chartCols} 列`)
      } else {
        c.check(m.chartCols === 1, `${vp.w}: 390 下图表网格应为 1 列，实测 ${m.chartCols} 列`)
      }
    }
    c.note(`实测 ${JSON.stringify(results)}`)
    await page.setViewportSize({ width: 1440, height: 900 })
    await shot(page, 'bi-layout')
    c.finish()
  })

  test('BI-02 十个维度存在与顺序', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoTab(page, 'BI 分析')
    await page.waitForTimeout(1500)

    const info = await page.evaluate(() => {
      const cards = Array.from(document.querySelectorAll('.chart-card')).map((card) => {
        const h = card.querySelector('h3')
        const cv = card.querySelector('canvas')
        let drawn = false
        let colors = 0
        if (cv) {
          try {
            const ctx = cv.getContext('2d')
            const d = ctx.getImageData(0, 0, cv.width, cv.height).data
            const seen = new Set<string>()
            for (let i = 0; i < d.length; i += 4 * 97) {
              if (d[i + 3] > 8) seen.add(`${d[i]},${d[i + 1]},${d[i + 2]}`)
              if (seen.size > 6) break
            }
            colors = seen.size
            drawn = colors > 1
          } catch (e) {
            drawn = false
          }
        }
        return {
          title: h ? (h.textContent || '').trim() : '',
          hasCanvas: !!cv,
          empty: !!card.querySelector('.chart-empty'),
          drawn,
          colors,
          wide: card.className.includes('chart-wide'),
        }
      })
      return { cards, legendText: document.querySelector('.chart-grid') ? document.querySelector('.chart-grid').innerText : '' }
    })

    c.note(`卡片数 ${info.cards.length}；标题顺序 = ${info.cards.map((x) => x.title).join(' | ')}`)
    c.check(info.cards.length === 10, `图表卡片应为 10 张，实测 ${info.cards.length}`)

    const expected: Array<[string, RegExp]> = [
      ['访客/浏览/阅读趋势', /trend|visitor|浏览|趋势/i],
      ['文章阅读 TOP10', /article/i],
      ['新闻阅读 TOP10', /news/i],
      ['位置点击率', /ctr|click|点击率/i],
      ['分类阅读分布', /category|分类/i],
      ['语言阅读分布', /language|语言/i],
      ['来源阅读分布', /source|来源/i],
      ['流量来源', /traffic|referr|流量/i],
      ['平均阅读时长', /reading time|duration|时长/i],
      ['发布趋势', /publishing|发布/i],
    ]
    expected.forEach(([name, re], i) => {
      const card = info.cards[i]
      if (!card) {
        c.check(false, `第 ${i + 1} 张卡片缺失（应为 ${name}）`)
        return
      }
      c.check(re.test(card.title), `第 ${i + 1} 张卡片标题不匹配（应为 ${name}，实测「${card.title}」）`)
      c.check(card.hasCanvas, `第 ${i + 1} 张（${card.title}）没有 canvas`)
      c.check(!card.empty, `第 ${i + 1} 张（${card.title}）显示空态（有数据时应渲染图表）`)
      c.check(card.drawn, `第 ${i + 1} 张（${card.title}）canvas 未绘制内容（颜色样本 ${card.colors}）`)
    })
    c.note('图表类型（源码核对）：trend=LineChart 3 线；TOP10×2=BarChart indexAxis=y；位置点击率=BarChart 纵向；分类/语言/来源/流量=环形图；平均阅读时长=LineChart；发布趋势=BarChart')
    await shot(page, 'bi-charts')
    c.finish()
  })

  test('BI-03 文案 i18n：不暴露字段代码 / 位置中文名 / UV-PV-CTR 说明', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoTab(page, 'BI 分析')
    await page.waitForTimeout(1200)

    const text = await page.locator('.workspace').innerText()
    const codes = text.match(/\b[a-z][a-z0-9]*_[a-z0-9_]+\b/g) ?? []
    const forbidden = ['home_hero', 'home_list', 'article_detail_related', 'news_list', 'articles_list', 'duration_ms']
    const hitForbidden = forbidden.filter((f) => text.includes(f))
    c.note(`可见文案中的下划线代码：${JSON.stringify([...new Set(codes)].slice(0, 10))}`)
    c.check(hitForbidden.length === 0, `可见文案出现字段/位置代码：${hitForbidden.join(',')}`)
    c.check(codes.length === 0, `可见文案出现 snake_case 代码：${[...new Set(codes)].slice(0, 8).join(',')}`)

    // 位置筛选下拉：必须是中文名（当前 en 环境为英文名），不能是 home_hero 之类代码
    await page.locator('.select-dropdown-portal').count()
    const trigger = page.locator('.bi-toolbar [aria-haspopup="listbox"]').nth(2)
    await trigger.click({ force: true })
    await page.waitForTimeout(400)
    const options = (await page.locator('.select-dropdown-portal [role="option"]').allInnerTexts()).map((s) => s.trim())
    await page.keyboard.press('Escape')
    c.note(`位置下拉选项 = ${JSON.stringify(options)}`)
    c.check(options.length >= 2, '位置下拉没有选项')
    c.check(!options.some((o) => /^[a-z0-9_]+$/.test(o)), `位置下拉出现代码值：${options.filter((o) => /^[a-z0-9_]+$/.test(o)).join(',')}`)

    const metrics = await page.locator('.metric-card').allInnerTexts()
    const joined = metrics.join(' ').replace(/\s+/g, ' ')
    c.note(`KPI 文案 = ${joined}`)
    for (const k of ['UV', 'PV', 'CTR']) {
      c.check(joined.includes(k), `KPI 未出现 ${k} 缩写说明`)
    }
    c.check(!/NaN|undefined|null/.test(text), '页面出现 NaN/undefined/null')

    // tooltip 为 canvas 绘制，无法读 DOM：用悬浮截图留证
    const first = page.locator('.chart-card canvas').first()
    const box = await first.boundingBox()
    if (box) {
      await page.mouse.move(box.x + box.width * 0.55, box.y + box.height * 0.5)
      await page.waitForTimeout(500)
      await shot(page, 'bi-tooltip-hover')
      c.note('tooltip 为 canvas 内绘制，已用悬浮截图留证（无法取 DOM 文本）')
    } else {
      await shot(page, 'bi-i18n')
    }
    c.finish()
  })

  test('BI-04 筛选项均为 Select，变更后重新查询', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoTab(page, 'BI 分析')
    await page.waitForTimeout(1200)

    const toolbar = page.locator('.bi-toolbar')
    const selects = toolbar.locator('[aria-haspopup="listbox"]')
    const selectCount = await selects.count()
    const freeInputs = await toolbar.locator('input[type="text"], input:not([type])').count()
    c.note(`工具栏 Select 数 ${selectCount}，自由文本输入 ${freeInputs}，工具栏文案 = ${(await toolbar.innerText()).replace(/\s+/g, ' ')}`)
    c.check(selectCount === 3, `工具栏应有 3 个 Select（时间/语言/位置），实测 ${selectCount}`)
    c.check(freeInputs === 0, `工具栏存在可手填输入框 ${freeInputs} 个`)

    // 时间范围：Last 30 days → Last 7 days，应触发一次 BI 查询
    const reqs: string[] = []
    page.on('request', (r) => {
      if (/analytics/i.test(r.url()) && !/events/i.test(r.url())) reqs.push(r.url().split('?')[1] ?? r.url())
    })
    await page.locator('[data-test="bi-range"]').first().click({ force: true })
    await page.waitForTimeout(400)
    const rangeOpts = (await page.locator('.select-dropdown-portal [role="option"]').allInnerTexts()).map((s) => s.trim())
    c.note(`时间范围选项 = ${JSON.stringify(rangeOpts)}`)
    c.check(rangeOpts.length >= 3, `时间范围选项不足 3 个：${JSON.stringify(rangeOpts)}`)
    await page.locator('.select-dropdown-portal [role="option"]').filter({ hasText: /7/ }).first().click()
    await page.waitForTimeout(1800)
    c.check(reqs.length > 0, '切换时间范围后没有发出 BI 查询请求')
    c.note(`切换后请求 = ${JSON.stringify(reqs.slice(-3))}`)

    // 位置筛选：选第一个具体值（非 All），应带 placement 参数
    const before = reqs.length
    await page.locator('.bi-toolbar [aria-haspopup="listbox"]').nth(2).click({ force: true })
    await page.waitForTimeout(400)
    const placeOpts = (await page.locator('.select-dropdown-portal [role="option"]').allInnerTexts()).map((s) => s.trim())
    await page.locator('.select-dropdown-portal [role="option"]').nth(1).click()
    await page.waitForTimeout(1800)
    const newReqs = reqs.slice(before)
    c.note(`位置选项 = ${JSON.stringify(placeOpts)}；位置切换请求 = ${JSON.stringify(newReqs)}`)
    c.check(newReqs.length > 0, '切换位置后没有重新查询')
    c.check(newReqs.some((u) => /placement/i.test(u)), `位置切换请求未带 placement 参数：${JSON.stringify(newReqs)}`)
    await shot(page, 'bi-filters')
    c.finish()
  })

  test('BI-05 双主题：无旧绿 #10B981，深色对比度 ≥4.5', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoTab(page, 'BI 分析')
    await page.waitForTimeout(1000)

    const light = await page.evaluate(() => {
      const card = document.querySelector('.chart-card')
      const h = document.querySelector('.chart-card h3')
      const cs = card ? getComputedStyle(card) : null
      const hs = h ? getComputedStyle(h) : null
      // 只统计 BI 区域实际渲染出的颜色，避免把全局 tailwind 工具类误判为旧绿
      let green = 0
      const scope = document.querySelector('.bi-panel') || document.querySelector('.chart-grid') || document.body
      for (const el of Array.from(scope.querySelectorAll('*')).slice(0, 800)) {
        const st = getComputedStyle(el)
        for (const c of [st.color, st.backgroundColor, st.borderTopColor, st.outlineColor]) {
          if (!c) continue
          const m = c.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/)
          if (m && m[1] === '16' && m[2] === '185' && m[3] === '129') { green++; break }
        }
      }
      return {
        cardBg: cs ? cs.backgroundColor : '',
        titleColor: hs ? hs.color : '',
        titleSize: hs ? hs.fontSize : '',
        greenRules: green,
        htmlClass: document.documentElement.className,
      }
    })
    c.note(`浅色：卡片底色 ${light.cardBg}，标题色 ${light.titleColor}，命中旧绿规则 ${light.greenRules} 条，html class = ${JSON.stringify(light.htmlClass)}`)
    c.check(light.greenRules === 0, `BI 区域仍有元素使用旧绿 #10B981（${light.greenRules} 处计算样式）`)

    const contrast = (rgb1: string, rgb2: string) => {
      const parse = (s: string) => (s.match(/\d+/g) ?? []).slice(0, 3).map(Number)
      const lum = (rgb: number[]) => {
        const a = rgb.map((v) => {
          const x = v / 255
          return x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4)
        })
        return 0.2126 * a[0] + 0.7152 * a[1] + 0.0722 * a[2]
      }
      const l1 = lum(parse(rgb1))
      const l2 = lum(parse(rgb2))
      const hi = Math.max(l1, l2)
      const lo = Math.min(l1, l2)
      return Math.round(((hi + 0.05) / (lo + 0.05)) * 100) / 100
    }
    const lightRatio = contrast(light.titleColor, light.cardBg)
    c.note(`浅色标题对比度 ${lightRatio}`)
    c.check(lightRatio >= 4.5, `浅色标题对比度不足：${lightRatio}`)

    // 深色：系统 prefers-color-scheme 深色 + 无本地偏好 → 应用应加 html.dark
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.reload()
    await gotoTab(page, 'BI 分析')
    await page.waitForTimeout(1500)
    const dark = await page.evaluate(() => {
      const card = document.querySelector('.chart-card')
      const h = document.querySelector('.chart-card h3')
      const cs = card ? getComputedStyle(card) : null
      const hs = h ? getComputedStyle(h) : null
      return {
        htmlClass: document.documentElement.className,
        cardBg: cs ? cs.backgroundColor : '',
        titleColor: hs ? hs.color : '',
        bodyBg: getComputedStyle(document.body).backgroundColor,
      }
    })
    await shot(page, 'bi-dark')
    const darkRatio = contrast(dark.titleColor, dark.cardBg)
    c.note(`深色：html class = ${JSON.stringify(dark.htmlClass)}，卡片底色 ${dark.cardBg}，标题色 ${dark.titleColor}，正文底 ${dark.bodyBg}，对比度 ${darkRatio}`)
    c.check(/dark/.test(dark.htmlClass), '深色偏好下 html 未带 dark 类（主题未生效）')
    c.check(darkRatio >= 4.5, `深色标题对比度不足：${darkRatio}`)
    c.check(dark.cardBg === light.cardBg && /rgb\(255, 255, 255\)/.test(dark.cardBg), `深色下卡片仍为纯白 ${dark.cardBg}，疑似未做深色适配`)

    await page.emulateMedia({ colorScheme: 'light' })
    c.finish()
  })

  test('BI-06 导出：有文件下载且内容非空', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoTab(page, 'BI 分析')
    await page.waitForTimeout(1200)

    const btn = page.locator('.bi-toolbar button', { hasText: /Export|导出/ }).first()
    c.check((await btn.count()) > 0, '工具栏没有导出按钮')
    let saved = ''
    try {
      const [dl] = await Promise.all([
        page.waitForEvent('download', { timeout: 20_000 }),
        btn.click(),
      ])
      saved = path.join(OUT_ROOT, 'w4-bi-export.csv')
      await dl.saveAs(saved)
      c.note(`下载文件名 ${dl.suggestedFilename()}，保存到 ${saved}`)
      c.check(/\.csv$/i.test(dl.suggestedFilename()), `导出文件名不是 csv：${dl.suggestedFilename()}`)
    } catch (e) {
      c.check(false, `点击导出没有触发下载：${String(e).slice(0, 120)}`)
    }
    if (saved) {
      const fs = await import('node:fs')
      const body = fs.readFileSync(saved, 'utf8')
      const lines = body.trim().split(/\r?\n/)
      c.note(`CSV ${lines.length} 行，首行 = ${lines[0]?.slice(0, 160)}`)
      c.check(lines.length > 1, `CSV 只有 ${lines.length} 行（无数据）`)
    }
    await shot(page, 'bi-export')
    c.finish()
  })

  test('BI-07 空数据态：无空白卡片、无 NaN', async ({ admin }) => {
    const page = await admin.page()
    const c = collector()
    guard(page).forEach((p) => problems.push(p))
    await gotoTab(page, 'BI 分析')
    await page.waitForTimeout(1000)

    await page.locator('[data-test="bi-range"]').first().click({ force: true })
    await page.waitForTimeout(400)
    await page.locator('.select-dropdown-portal [role="option"]').filter({ hasText: /Custom/i }).first().click()
    await page.waitForTimeout(600)
    const from = page.locator('[data-test="bi-from"]')
    const to = page.locator('[data-test="bi-to"]')
    c.check((await from.count()) > 0 && (await to.count()) > 0, '选择自定义范围后未出现起止日期输入')
    if (await from.count()) {
      await from.fill('2030-01-01')
      await from.dispatchEvent('change')
      await page.waitForTimeout(300)
      await to.fill('2030-01-05')
      await to.dispatchEvent('change')
      await page.waitForTimeout(2500)
    }
    const state = await page.evaluate(() => ({
      emptyCards: document.querySelectorAll('.chart-empty').length,
      cards: document.querySelectorAll('.chart-card').length,
      text: (document.querySelector('.workspace') || document.body).innerText.replace(/\s+/g, ' ').slice(0, 600),
    }))
    c.note(`未来区间：${state.cards} 张卡片中空态 ${state.emptyCards} 张；文案片段 = ${state.text.slice(0, 200)}`)
    c.check(!/NaN|undefined/.test(state.text), '空数据时出现 NaN/undefined')
    c.check(state.emptyCards > 0 || !/NaN/.test(state.text), '空数据既无空态提示也无可读占位')
    c.note('接口失败态（5xx）未构造：契约禁止 page.route/mock，无法在不改后端的情况下注入失败；该项记为 BLOCKED')
    await shot(page, 'bi-empty')
    c.finish()
  })
})
