import { expect, test, type Page, type TestInfo } from '@playwright/test'
import { expectClean, guard, openContentTab, realLogin, reloadToTab, shooter, stamp, waitWrite } from '../real/real-helpers'

/**
 * v0.4 W3 验收：新闻源 / 抓取规则 / 自动化设置 / 分类标签 / 按钮清单（SRC-01 ~ SRC-13）
 * 依据：docs/v0.4/v0.4_prd_20261026_v3.3.md（1.1–1.5、R1、R3、R4、R14）、
 *      docs/v0.4/v0.4_e2e_acceptance_cases_20261028.md（§0 执行契约、§1 量化口径、W3）
 * 契约：真实后端，无 route/mock/token 注入；一条用例 = 一个 test；子检查用 expect.soft。
 */

const NEWS = '新闻动态'
const TAX = '分类与标签'
const NAME = `ACC-W3-源-${stamp}`
const URL_OK = `https://example.com/acc-w3-${stamp}.xml`

type Ev = string[]

function note(ev: Ev, k: string, v: unknown) {
  ev.push(`${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`)
}

async function attachEv(testInfo: TestInfo, ev: Ev) {
  await testInfo.attach('measured', { body: ev.join('\n'), contentType: 'text/plain' })
}

async function gotoNewsSub(page: Page, label: string) {
  await openContentTab(page, NEWS)
  await page.locator('.subnav button', { hasText: label }).first().click()
  await page.waitForTimeout(600)
}

async function sourceRow(page: Page, name: string) {
  return page.locator('table tbody tr', { hasText: name }).first()
}

async function toastTexts(page: Page): Promise<string[]> {
  return page.locator('.ccai-neolab-toast').allInnerTexts()
}

/** SRC-01 后新增改为弹窗：未打开时先点「+ 新增新闻源」，再在 .source-dialog 内填写。 */
async function fillSourceForm(page: Page, name: string, url: string, interval?: string) {
  const dlg = page.locator('.source-dialog')
  if (!(await dlg.isVisible().catch(() => false))) {
    await page.getByRole('button', { name: /新增新闻源/ }).first().click()
    await dlg.waitFor({ state: 'visible', timeout: 5000 })
  }
  await dlg.locator('input[placeholder*="来源名称"]').fill(name)
  await dlg.locator('input[placeholder*="URL"]').fill(url)
  if (interval !== undefined) await dlg.locator('.interval-input').fill(interval)
}

/** 弹窗已打开时点「保存新闻源」提交；否则点「新增新闻源」（只打开弹窗，不应发 POST）。force 点击并等 POST 响应。 */
async function clickCreate(page: Page, timeout = 8000) {
  const dlgOpen = await page.locator('.source-dialog').isVisible().catch(() => false)
  const btn = dlgOpen
    ? page.locator('.source-dialog').getByRole('button', { name: '保存新闻源' })
    : page.getByRole('button', { name: /新增新闻源/ }).first()
  const disabled = await btn.isDisabled().catch(() => false)
  const [resp] = await Promise.all([
    page
      .waitForResponse((r) => r.url().includes('/news-sources') && r.request().method() === 'POST', { timeout })
      .catch(() => null),
    btn.click({ force: true, timeout: 5000 }).catch(() => null),
  ])
  return { resp, disabled }
}

async function deleteSource(page: Page, name: string) {
  const row = await sourceRow(page, name)
  if (!(await row.count())) return
  await row.getByRole('button', { name: '删除', exact: true }).click()
  const dlg = page.locator('.ui-confirm, [class*="confirm"]').filter({ hasText: '确认删除' }).first()
  if (await dlg.count()) {
    await dlg.getByRole('button', { name: '确认删除' }).click()
    await page.waitForTimeout(800)
  }
}

// ---------------------------------------------------------------- SRC-01
test('SRC-01 新闻源列表页只有列表 + 右上角「+ 新增新闻源」按钮，无内嵌表单', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page)
  await realLogin(page)
  await gotoNewsSub(page, '新闻源')
  await shot(page, 'sources-list')

  // 期望：页面内没有内嵌表单输入
  const inline = page.locator('.source-toolbar input, .source-toolbar textarea, .source-toolbar select')
  const inlineVisible = await inline.evaluateAll((els) =>
    els.filter((e) => (e as HTMLElement).offsetParent !== null).map((e) => ({
      tag: e.tagName,
      type: (e as HTMLInputElement).type || '',
      ph: (e as HTMLInputElement).placeholder || '',
    })),
  )
  note(ev, '内嵌表单可见控件', inlineVisible)
  await expect.soft(inlineVisible.length, `列表页仍有 ${inlineVisible.length} 个内嵌表单控件`).toBe(0)

  // 期望：右上角有「+ 新增新闻源」按钮
  const btn = page.getByRole('button', { name: /新增新闻源/ }).first()
  await expect.soft(btn, '缺少「新增新闻源」按钮').toBeVisible()
  const btnText = (await btn.innerText().catch(() => '')).trim()
  const box = await btn.boundingBox()
  const right = await page.evaluate(() => {
    const sec = document.querySelector('section.workspace') as HTMLElement | null
    if (!sec) return window.innerWidth
    // 以内容区右边缘为准：外框右边减去 border 和 padding（原先用外框，固定多算 24px padding + 2.5px border）
    const cs = getComputedStyle(sec)
    return sec.getBoundingClientRect().right - parseFloat(cs.borderRightWidth) - parseFloat(cs.paddingRight)
  })
  note(ev, '按钮文案', btnText)
  note(ev, '按钮右边缘距内容区右边缘', box ? Math.round(right - (box.x + box.width)) : -1)
  note(ev, '按钮宽高', box ? `${Math.round(box.width)}x${Math.round(box.height)}` : 'none')
  await expect.soft(btnText, '按钮文案应是「+ 新增新闻源」').toMatch(/^\+/)
  await expect.soft(box ? right - (box.x + box.width) : 999, '按钮不靠右（右上角）').toBeLessThanOrEqual(24)

  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-02
test('SRC-02 新增新闻源弹窗规格与字段（640±4、圆角 12、R3 字段清单）', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page)
  await realLogin(page)
  await gotoNewsSub(page, '新闻源')

  // 点「新增新闻源」：期望弹出中弹窗
  const { resp: cResp, disabled: cDisabled } = await clickCreate(page)
  note(ev, '新增按钮 disabled', cDisabled)
  note(ev, '点新增后是否发 POST', cResp ? cResp.status() : '无请求')
  await page.waitForTimeout(600)
  const createDialogs = page.locator('[role="dialog"]')
  const nCreate = await createDialogs.count()
  note(ev, '点击新增后的弹窗数量', nCreate)
  await shot(page, 'create-after-click')
  await expect.soft(nCreate, '点「新增新闻源」没有弹出弹窗（仍是内嵌表单）').toBe(1)

  // 无论是否弹窗，都用现有唯一表单核对 R3 字段（新增=内嵌表单 / 编辑=弹窗）
  const dlg = page.locator('.source-dialog')
  if (await dlg.count()) {
    const m = await dlg.evaluate((el) => {
      const cs = getComputedStyle(el)
      const r = el.getBoundingClientRect()
      return { w: Math.round(r.width), h: Math.round(r.height), radius: cs.borderTopLeftRadius, bg: cs.backgroundColor }
    })
    note(ev, '弹窗宽度/圆角', m)
    await expect.soft(Math.abs(m.w - 640), `弹窗宽 ${m.w}，应 640±4`).toBeLessThanOrEqual(4)
    await expect.soft(m.radius, `弹窗圆角 ${m.radius}，应 12px`).toBe('12px')
  }
  // 用列表首行「编辑」打开实际表单核对字段（新增弹窗的遮罩会挡住列表，先关掉）
  if (await dlg.isVisible().catch(() => false)) {
    await dlg.getByRole('button', { name: '取消' }).click()
    await dlg.waitFor({ state: 'hidden', timeout: 5000 }).catch(() => null)
  }
  // 数据隔离：列表按 created_at desc 排序，其它用例/上一轮跑完可能留下已被软删的行占据首行；
  // 这里显式选「操作列有『编辑』」的第一行（软删行只渲染「恢复」），不依赖库里已有数据的新旧。
  const row = page
    .locator('table tbody tr')
    .filter({ has: page.getByRole('button', { name: '编辑', exact: true }) })
    .first()
  await expect(row, '列表里没有可编辑的新闻源行（全部被软删或为空）').toHaveCount(1)
  await row.getByRole('button', { name: '编辑', exact: true }).click()
  await page.waitForTimeout(600)
  await shot(page, 'edit-dialog')

  const labels = await dlg.locator('.form-grid > label').evaluateAll((els) =>
    els.map((e) => (e.firstChild?.textContent || '').trim()).filter(Boolean),
  )
  note(ev, '表单字段标签', labels)
  const stars = await dlg.locator('.required-star, label:has-text("*")').count()
  note(ev, '必填红星个数', stars)
  const footer = await dlg.locator('.form-actions button').evaluateAll((els) => els.map((e) => e.textContent?.trim()))
  note(ev, '底部按钮顺序', footer)
  const placeholders = await dlg.locator('input').evaluateAll((els) => els.map((e) => (e as HTMLInputElement).placeholder))
  note(ev, '间隔 placeholder', placeholders.find((p) => p.includes('跟随全局')) ?? '(无)')

  const joined = labels.join('|')
  await expect.soft(/名称/.test(joined), '缺「名称」字段').toBe(true)
  await expect.soft(/(链接|URL)/i.test(joined), '缺「链接 URL」字段').toBe(true)
  await expect.soft(/类型/.test(joined), '缺「类型（RSS/网页，Select）」字段').toBe(true)
  await expect.soft(/抓取规则/.test(joined), '缺「抓取规则（Select）」字段').toBe(true)
  await expect.soft(/默认分类/.test(joined), '缺「默认分类（下拉，单选）」字段（现为纯文本 input）').toBe(true)
  await expect.soft(/状态/.test(joined), '缺「状态（开关，默认启用）」字段').toBe(true)
  await expect.soft(stars, '必填项没有红星 *').toBeGreaterThan(0)
  await expect.soft(footer.join(','), '底部按钮应「左取消 右保存」').toBe('取消,保存新闻源')
  // 抓取规则不存在时的提示
  const hint = await dlg.locator('text=请先在抓取规则里创建').count()
  note(ev, '无规则提示节点数', hint)
  await expect.soft(hint, '没有「请先在抓取规则里创建」提示').toBeGreaterThan(0)

  // 关闭弹窗
  const cancel = dlg.getByRole('button', { name: '取消' })
  if (await cancel.count()) await cancel.click()
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-03
test('SRC-03 抓取间隔范围校验：0/14/1441 报错，15/1440 成功', async ({ page }, testInfo) => {
  test.setTimeout(120_000) // 5 个取值 × (填表+提交+刷新+回读)，30s 预算不够
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  // 本用例故意提交 0/14/1441，后端返回 400 是预期结果；只放行新闻源创建接口的 400 及其对应的浏览器 console 报错
  const problems = guard(page, [/^POST \/api\/v1\/admin\/content\/news-sources → 400$/, /status of 400 \(Bad Request\)/])
  await realLogin(page)
  await gotoNewsSub(page, '新闻源')

  const cases: { v: string; ok: boolean }[] = [
    { v: '0', ok: false },
    { v: '14', ok: false },
    { v: '15', ok: true },
    { v: '1440', ok: true },
    { v: '1441', ok: false },
  ]
  const created: string[] = []
  for (const c of cases) {
    const nm = `${NAME}-间隔${c.v}`
    await fillSourceForm(page, nm, `https://example.com/acc-w3-i${c.v}-${stamp}.xml`, c.v)
    const { resp } = await clickCreate(page)
    await page.waitForTimeout(900)
    // 错误提示要在刷新前读：刷新后弹窗和 toast 都会消失
    // 只读弹窗内提示 + toast：列表行里的 source.lastError（历史抓取错误）不属于本次校验反馈
    const errText = (
      await page.locator('.source-dialog .error-text, .ccai-neolab-toast').allInnerTexts()
    )
      .join(' / ')
      .slice(0, 200)
    await page.reload()
    await gotoNewsSub(page, '新闻源')
    const appeared = await page.locator('table tbody tr', { hasText: nm }).count()
    note(ev, `间隔${c.v}`, `请求=${resp ? resp.status() : '无请求'} 行出现=${appeared} 提示=${errText || '(无)'}`)
    if (appeared) created.push(nm)
    if (!c.ok) {
      // 期望：被拒绝（400）+ 范围类提示文案；0 被接受=缺陷
      await expect.soft(appeared, `间隔 ${c.v} 应被拒绝并报范围错误，实际创建成功`).toBe(0)
      await expect.soft(resp ? resp.status() : 0, `间隔 ${c.v} 应返回 400 范围错误`).toBe(400)
      await expect.soft(errText, `间隔 ${c.v} 的错误提示应说明范围`).toMatch(/范围|1[45]|分钟|interval/i)
    } else {
      await expect.soft(appeared, `间隔 ${c.v} 应保存成功`).toBeGreaterThan(0)
    }
    await shot(page, `interval-${c.v}`)
  }
  // 清理
  for (const nm of created) await deleteSource(page, nm)
  note(ev, '清理删除条数', created.length)
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-04
test('SRC-04 间隔 45 保存后回显 45，不被全局 15 覆盖；下次抓取=上次+45', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page)
  await realLogin(page)
  await gotoNewsSub(page, '新闻源')

  const nm = `${NAME}-45`
  await fillSourceForm(page, nm, `https://example.com/acc-w3-45-${stamp}.xml`, '45')
  const [createResp] = await Promise.all([
    page.waitForResponse((r) => r.url().includes('/news-sources') && r.request().method() === 'POST', { timeout: 20000 }).catch(() => null),
    page.locator('.source-dialog').getByRole('button', { name: '保存新闻源' }).click(),
  ])
  const createBody = createResp ? await createResp.json().catch(() => null) : null
  note(ev, '创建接口 intervalMinutes', createBody?.data?.intervalMinutes ?? '(读不到)')
  await page.waitForTimeout(1000)

  await reloadToTab(page, NEWS)
  await page.locator('.subnav button', { hasText: '新闻源' }).first().click()
  await page.waitForTimeout(800)
  const row = await sourceRow(page, nm)
  const schedText = (await row.locator('td').nth(2).innerText().catch(() => '')).replace(/\n/g, ' ')
  note(ev, '列表调度文案', schedText)
  await expect.soft(schedText, '列表调度应显示「每 45 分钟抓取一次」').toContain('45')
  await shot(page, 'list-45')

  await row.getByRole('button', { name: '编辑', exact: true }).click()
  await page.waitForTimeout(500)
  const dlg = page.locator('.source-dialog')
  const val = await dlg.locator('label:has-text("间隔") input').inputValue()
  note(ev, '重开弹窗间隔值', val)
  await expect.soft(val, '间隔回显应为 45').toBe('45')
  await shot(page, 'reopen-45')
  const cancel = dlg.getByRole('button', { name: '取消' })
  if (await cancel.count()) await cancel.click()

  await deleteSource(page, nm)
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-05
test('SRC-05 清空间隔保存后显示「跟随全局（15 分钟）」，接口返回 null', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page)
  await realLogin(page)
  await gotoNewsSub(page, '新闻源')

  const nm = `${NAME}-空间隔`
  await fillSourceForm(page, nm, `https://example.com/acc-w3-null-${stamp}.xml`, '45')
  await Promise.all([
    page.waitForResponse((r) => r.url().includes('/news-sources') && r.request().method() === 'POST', { timeout: 20000 }).catch(() => null),
    page.locator('.source-dialog').getByRole('button', { name: '保存新闻源' }).click(),
  ])
  await page.waitForTimeout(900)
  await page.reload()
  await gotoNewsSub(page, '新闻源')

  const row = await sourceRow(page, nm)
  await row.getByRole('button', { name: '编辑', exact: true }).click()
  await page.waitForTimeout(500)
  const dlg = page.locator('.source-dialog')
  const intervalInput = dlg.locator('label:has-text("间隔") input')
  await intervalInput.fill('')
  const [resp] = await Promise.all([
    page.waitForResponse((r) => r.url().includes('/news-sources/') && r.request().method() === 'PUT', { timeout: 20000 }).catch(() => null),
    dlg.getByRole('button', { name: '保存新闻源' }).click(),
  ])
  const body = resp ? await resp.json().catch(() => null) : null
  note(ev, 'PUT 返回 intervalMinutes', body?.data?.intervalMinutes === null ? 'null' : JSON.stringify(body?.data?.intervalMinutes))
  await page.waitForTimeout(800)
  await shot(page, 'saved-empty')

  await page.reload()
  await gotoNewsSub(page, '新闻源')
  const text = ((await (await sourceRow(page, nm)).locator('td').nth(2).innerText().catch(() => '')) || '').replace(/\n/g, ' ')
  note(ev, '列表调度文案（清空后）', text)
  await expect.soft(text, '清空后应显示「跟随全局（15 分钟）」').toContain('跟随全局（15 分钟）')
  const iv = body?.data?.intervalMinutes
  note(ev, '接口 intervalMinutes 是否为 null', iv === null ? 'null' : JSON.stringify(iv))
  await expect.soft(iv === null, '接口应返回 intervalMinutes=null（继承全局）').toBe(true)
  await shot(page, 'list-empty')

  await deleteSource(page, nm)
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-06
test('SRC-06 必填项留空：红星 + 失焦校验 + 提交前整体校验且不发请求', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page)
  await realLogin(page)
  await gotoNewsSub(page, '新闻源')

  // v0.4 改版：新增/编辑表单在弹窗内（旧版是工具栏内联），断言口径同步到 .source-dialog
  await page.getByRole('button', { name: /新增新闻源/ }).first().click()
  const dlg = page.locator('.source-dialog')
  await dlg.waitFor({ state: 'visible', timeout: 8000 })

  const dlgStars = await dlg.locator('.required-star').count()
  note(ev, '弹窗内红星个数', dlgStars)
  await expect.soft(dlgStars, '必填项没有红星标记 *').toBeGreaterThan(0)

  const nameInput = dlg.locator('input[placeholder*="来源名称"]')
  await nameInput.click()
  await nameInput.fill('x')
  await nameInput.fill('')
  await nameInput.blur()
  await page.waitForTimeout(400)
  const blurErr = (await page.locator('.field-errors, .error-text').allInnerTexts()).join(' / ')
  note(ev, '失焦后错误文案', blurErr || '(无)')
  await expect.soft(blurErr, '名称清空失焦后应立即提示必填').toMatch(/必填/)
  await shot(page, 'blur-error')

  // 提交前整体校验：点提交不得发出 POST
  let posted = 0
  const onReq = (r: any) => {
    if (r.method() === 'POST' && r.url().includes('/news-sources')) posted += 1
  }
  page.on('request', onReq)
  await clickCreate(page, 4000)
  await page.waitForTimeout(1200)
  page.off('request', onReq)
  const errsAll = (await page.locator('.field-errors, .error-text').allInnerTexts()).join(' / ')
  note(ev, '提交后错误文案', errsAll || '(无)')
  note(ev, '提交期间 POST 次数', posted)
  await expect.soft(posted, '必填为空仍发出了创建请求').toBe(0)
  await expect.soft(errsAll, '提交后没有整体校验提示').toMatch(/必填|请填写/)
  await shot(page, 'submit-error')

  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-07
test('SRC-07 弹窗四种关闭方式 + 未保存修改确认', async ({ page }, testInfo) => {
  test.setTimeout(60_000) // 建源 + 四轮开关弹窗 + 收尾删除，30s 预算不够
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page)
  await realLogin(page)
  await gotoNewsSub(page, '新闻源')
  // 自建数据，避免复用他人脏行（软删行会触发 GET /news-sources/{id} 404）
  const nm = `${NAME}-关闭`
  await fillSourceForm(page, nm, `https://example.com/acc-w3-close-${stamp}.xml`)
  await clickCreate(page)
  await page.waitForTimeout(1000)
  const row = await sourceRow(page, nm)
  note(ev, '自建源是否出现', await row.count())
  await expect.soft(await row.count(), '自建新闻源未出现在列表').toBeGreaterThan(0)
  const dlg = page.locator('.source-dialog')

  const open = async () => {
    await row.getByRole('button', { name: '编辑', exact: true }).click()
    await page.waitForTimeout(500)
    await dlg.locator('input').first().click()
  }

  // 1) ×
  await open()
  const closeBtn = dlg.locator('.dialog-close')
  note(ev, '有 × 按钮', await closeBtn.count())
  if (await closeBtn.count()) await closeBtn.click()
  await page.waitForTimeout(400)
  note(ev, '点 × 后弹窗数', await dlg.count())
  await expect.soft(await dlg.count(), '点 × 未能关闭弹窗').toBe(0)
  await shot(page, 'close-x')

  // 2) Esc（焦点在弹窗内）
  await open()
  await page.keyboard.press('Escape')
  await page.waitForTimeout(500)
  const afterEsc = await dlg.count()
  note(ev, 'Esc 后弹窗数', afterEsc)
  await expect.soft(afterEsc, 'Esc 未能关闭弹窗').toBe(0)
  await shot(page, 'close-esc')
  if (await dlg.count()) await dlg.getByRole('button', { name: '取消' }).click()

  // 3) 点遮罩
  await open()
  await page.mouse.click(5, 5)
  await page.waitForTimeout(500)
  const afterMask = await dlg.count()
  note(ev, '点遮罩后弹窗数', afterMask)
  await expect.soft(afterMask, '点遮罩未能关闭弹窗').toBe(0)
  await shot(page, 'close-mask')
  if (await dlg.count()) await dlg.getByRole('button', { name: '取消' }).click()

  // 4) 取消
  await open()
  await dlg.getByRole('button', { name: '取消' }).click()
  await page.waitForTimeout(400)
  note(ev, '取消后弹窗数', await dlg.count())
  await expect.soft(await dlg.count(), '点取消未能关闭弹窗').toBe(0)

  // 5) 有未保存修改时的确认
  await open()
  await dlg.locator('input').first().fill('ACC-W3-未保存')
  await page.keyboard.press('Escape')
  await page.waitForTimeout(600)
  const confirmText = (await page.locator('body').innerText()).includes('有未保存修改')
  note(ev, '修改后 Esc 出现「有未保存修改」确认', confirmText)
  await expect.soft(confirmText, '有未保存修改时关闭没有确认提示').toBe(true)
  await shot(page, 'unsaved-confirm')
  // 收尾：未保存修改要先过确认弹层（继续编辑保留内容 → 取消再确认离开才关闭）
  const guardDlg = page.locator('.source-close-guard-dialog')
  if (await guardDlg.count()) {
    await guardDlg.getByRole('button', { name: '继续编辑' }).first().click()
    await page.waitForTimeout(300)
    note(ev, '继续编辑后弹窗仍在', await dlg.count())
    const kept = await dlg.locator('input').first().inputValue()
    await expect.soft(kept, '继续编辑后已填内容被清空').toBe('ACC-W3-未保存')
    await dlg.getByRole('button', { name: '取消' }).first().click()
    await page.waitForTimeout(300)
    await expect.soft(await guardDlg.count(), '有未保存修改点取消没有确认弹层').toBeGreaterThan(0)
    await guardDlg.getByRole('button', { name: '确认离开' }).first().click()
    await page.waitForTimeout(400)
  }
  await expect.soft(await dlg.count(), '确认离开后弹窗仍未关闭').toBe(0)

  await deleteSource(page, nm)
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-08
test('SRC-08 新增/编辑/删除新闻源：关闭+提示+刷新；软删除行只显示「恢复」', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page)
  await realLogin(page)
  await gotoNewsSub(page, '新闻源')

  const nm = `${NAME}-CRUD`
  const nm2 = `${nm}-改`
  await fillSourceForm(page, nm, `https://example.com/acc-w3-crud-${stamp}.xml`)
  const [resp] = await Promise.all([
    page.waitForResponse((r) => r.url().includes('/news-sources') && r.request().method() === 'POST', { timeout: 20000 }).catch(() => null),
    page.locator('.source-dialog').getByRole('button', { name: '保存新闻源' }).click(),
  ])
  await page.waitForTimeout(900)
  const toasts = await toastTexts(page)
  note(ev, '新增接口状态', resp ? resp.status() : '无请求')
  note(ev, '新增后 toast', toasts)
  await expect.soft(toasts.join('|'), '新增成功没有提示').toMatch(/已保存|成功|已创建|新闻源/)
  const row = await sourceRow(page, nm)
  note(ev, '新增后列表出现该行', await row.count())
  await expect.soft(await row.count(), '新增后列表未刷新出新行').toBeGreaterThan(0)
  await shot(page, 'created')

  // 编辑改名
  await row.getByRole('button', { name: '编辑', exact: true }).click()
  await page.waitForTimeout(400)
  const dlg = page.locator('.source-dialog')
  await dlg.locator('input').first().fill(nm2)
  await Promise.all([
    page.waitForResponse((r) => r.url().includes('/news-sources/') && r.request().method() === 'PUT', { timeout: 20000 }).catch(() => null),
    dlg.getByRole('button', { name: '保存新闻源' }).click(),
  ])
  await page.waitForTimeout(900)
  note(ev, '保存后弹窗是否关闭', (await dlg.count()) === 0)
  await expect.soft(await dlg.count(), '保存成功后弹窗未关闭').toBe(0)
  note(ev, '保存后 toast', await toastTexts(page))
  await expect.soft((await toastTexts(page)).join('|'), '保存成功没有「已保存」提示').toMatch(/已保存/)
  await page.reload()
  await gotoNewsSub(page, '新闻源')
  const renamed = await sourceRow(page, nm2)
  note(ev, '改名回读', await renamed.count())
  await expect.soft(await renamed.count(), '改名后刷新未持久化').toBeGreaterThan(0)
  await shot(page, 'renamed')

  // 删除：有确认；确认后行显示「恢复」且不再有「删除」
  await renamed.getByRole('button', { name: '删除', exact: true }).click()
  await page.waitForTimeout(500)
  const confirmVisible = await page.locator('text=确认删除这个新闻源').count()
  note(ev, '删除确认框出现', confirmVisible)
  await expect.soft(confirmVisible, '删除没有二次确认').toBeGreaterThan(0)
  await shot(page, 'delete-confirm')
  const okBtn = page.getByRole('button', { name: '确认删除' }).first()
  if (await okBtn.count()) await okBtn.click()
  await page.waitForTimeout(1000)
  const after = await sourceRow(page, nm2)
  const btns = (await after.locator('td').last().innerText().catch(() => '')).replace(/\n/g, ' ')
  note(ev, '删除后该行操作列', btns)
  await expect.soft(btns, '软删除后仍有「删除」按钮').not.toContain('删除')
  await expect.soft(btns, '软删除后缺少「恢复」按钮').toContain('恢复')
  await shot(page, 'deleted-row')

  // 还原数据
  const restoreBtn = after.getByRole('button', { name: '恢复' })
  if (await restoreBtn.count()) {
    await restoreBtn.click()
    await page.waitForTimeout(800)
  }
  await deleteSource(page, nm2)
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-09
test('SRC-09 自动化设置区块：滑块开关 + 2px/圆角8 输入框 + Select + 色值（双主题）+ 刷新保持', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page)
  await realLogin(page)
  await gotoNewsSub(page, '自动化设置')

  const panel = page.locator('.settings')
  await expect.soft(await panel.count(), '没有自动化设置区块').toBeGreaterThan(0)

  // 1) 开关必须是滑块
  // 轨道色有 150ms background-color 过渡：数据到达后开关会从灰切蓝，采样必须等过渡结束
  // （否则会读到中间值 rgb(38,99,235) 之类）
  await panel.locator('label.switch').first().waitFor({ state: 'visible' })
  await page.waitForTimeout(400)
  const sw = await panel.locator('label.switch').first().evaluate((el) => {
    const cb = el.querySelector('input[type=checkbox]') as HTMLInputElement | null
    const track = el.querySelector('span') as HTMLElement | null
    const cs = track ? getComputedStyle(track) : null
    const r = track?.getBoundingClientRect()
    return {
      cbSize: cb ? `${Math.round(cb.getBoundingClientRect().width)}x${Math.round(cb.getBoundingClientRect().height)}` : 'none',
      track: r ? `${Math.round(r.width)}x${Math.round(r.height)}` : 'none',
      radius: cs?.borderRadius ?? 'none',
      bg: cs?.backgroundColor ?? 'none',
      checked: cb?.checked,
    }
  })
  note(ev, '开关实测', sw)
  const cbW = Number(String(sw.cbSize).split('x')[0])
  // 滑块式开关：原生 checkbox 应视觉隐藏（≈0 尺寸），由 span 充当轨道（圆角大、有底色）
  await expect.soft(cbW, `开关不是滑块式：原生 checkbox 可见 ${sw.cbSize}`).toBeLessThanOrEqual(2)
  await expect.soft(sw.radius, `开关无滑块轨道（轨道圆角 ${sw.radius}，应≥9999 或一半高度）`).not.toBe('0px')
  if (sw.checked) await expect.soft(sw.bg, `开态应为蓝色 #2563EB，实测 ${sw.bg}`).toBe('rgb(37, 99, 235)')

  // 2) 输入框 2px / 圆角 8 / 高 36±1
  const inputs = await panel.locator('input:not([type=checkbox])').evaluateAll((els) =>
    els.map((e) => {
      const cs = getComputedStyle(e)
      const r = e.getBoundingClientRect()
      return { h: Math.round(r.height), radius: cs.borderTopLeftRadius, bw: cs.borderTopWidth }
    }),
  )
  note(ev, '输入框实测(前6)', inputs.slice(0, 6))
  inputs.forEach((i, idx) => {
    expect.soft(Math.abs(i.h - 36), `输入框#${idx} 高 ${i.h}，应 36±1`).toBeLessThanOrEqual(1)
    expect.soft(i.radius, `输入框#${idx} 圆角 ${i.radius}，应 8px`).toBe('8px')
    expect.soft(i.bw, `输入框#${idx} 边框 ${i.bw}，应 2px`).toBe('2px')
  })

  // 3) 不得有原生 select
  const nativeSel = await panel.locator('select').evaluateAll((els) =>
    els.filter((e) => getComputedStyle(e).appearance !== 'none' && (e as HTMLElement).offsetParent !== null).length,
  )
  note(ev, '原生可见 select 数', nativeSel)
  await expect.soft(nativeSel, '自动化设置使用了原生 select').toBe(0)

  // 4) 文字色值
  const colors = await panel.evaluate((el) => {
    const ok = ['rgb(18, 19, 22)', 'rgb(107, 114, 128)']
    const all = Array.from(el.querySelectorAll('label, span, small, h3, p')) as HTMLElement[]
    const bad = all
      .filter((n) => (n.textContent || '').trim().length > 0 && n.offsetParent !== null)
      .map((n) => ({ t: (n.textContent || '').trim().slice(0, 16), c: getComputedStyle(n).color }))
      .filter((x) => !ok.includes(x.c))
    return { total: all.length, bad: bad.slice(0, 8) }
  })
  note(ev, '文字色值异常项', colors)
  await expect.soft(colors.bad.length, `文字色值不符 1.1：${JSON.stringify(colors.bad)}`).toBe(0)

  // 5) 切换 + 刷新保持
  const cb = panel.locator('label.switch input[type=checkbox]').first()
  const before = await cb.isChecked()
  await panel.locator('label.switch').first().click()
  await page.waitForTimeout(200)
  const mid = await cb.isChecked()
  const [saveResp] = await Promise.all([
    page.waitForResponse((r) => r.url().includes('/automation') && r.request().method() === 'PUT', { timeout: 15000 }).catch(() => null),
    page.getByRole('button', { name: /保存自动化设置/ }).click(),
  ])
  note(ev, '保存自动化响应', saveResp ? saveResp.status() : '无请求')
  if (saveResp) note(ev, '保存自动化响应体', (await saveResp.text().catch(() => '')).slice(0, 300))
  await page.waitForTimeout(900)
  await reloadToTab(page, NEWS)
  await page.locator('.subnav button', { hasText: '自动化设置' }).first().click()
  await page.waitForTimeout(900)
  const after = await page.locator('.settings label.switch input[type=checkbox]').first().isChecked()
  note(ev, '开关 前/切换后/刷新后', `${before}/${mid}/${after}`)
  await expect.soft(after, '切换开关后刷新状态未保持').toBe(mid)
  await shot(page, 'automation-after-reload')
  if (after !== before) {
    await page.locator('.settings label.switch').first().click()
    await page.waitForTimeout(200)
    await page.getByRole('button', { name: /保存自动化设置/ }).click()
    await page.waitForTimeout(800)
  }
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-10
test('SRC-10 目标语言开关列表：英/俄/法/德，保存后刷新回显', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page)
  await realLogin(page)
  await gotoNewsSub(page, '自动化设置')

  const group = page.locator('.locale-checkbox-group')
  const labels = (await group.locator('.locale-checkbox span').allInnerTexts()).map((s) => s.trim())
  note(ev, '目标语言项', labels)
  note(ev, '目标语言项数', labels.length)
  await expect.soft(labels.length, '目标语言项数应为 4（英/俄/法/德）').toBe(4)
  // 1.6：语言名按当前界面语言显示「本地化名 + 母语名」；先按母语名归一化校验顺序，
  //      中文界面再额外校验中文名（E2E 默认界面语言为 en，此时本地化名即英文名）。
  const native = ['English', 'Русский', 'Français', 'Deutsch']
  const order = labels.map((l) => native.find((n) => l.includes(n)) || '')
  note(ev, '按母语名归一化顺序', order)
  await expect.soft(order, `语言顺序应为 ${native.join('/')}，实测 ${labels.join('|')}`).toEqual(native)
  const uiLang = String((await page.locator('html').getAttribute('lang')) || '').toLowerCase()
  const zh = ['英', '俄', '法', '德']
  const zhOk = zh.every((z) => labels.join('|').includes(z))
  note(ev, '界面语言', uiLang)
  note(ev, '是否使用中文语言名', zhOk)
  if (uiLang.startsWith('zh')) {
    await expect.soft(zhOk ? 'zh' : 'native', `中文界面语言名未本地化：${labels.join('|')}`).toBe('zh')
  }

  const en = group.locator('.locale-checkbox').first()
  const before = await en.locator('input').isChecked()
  await en.click()
  await page.waitForTimeout(200)
  const mid = await en.locator('input').isChecked()
  await page.getByRole('button', { name: /保存自动化设置/ }).click()
  await page.waitForTimeout(1000)
  await reloadToTab(page, NEWS)
  await page.locator('.subnav button', { hasText: '自动化设置' }).first().click()
  await page.waitForTimeout(900)
  const after = await page.locator('.locale-checkbox-group .locale-checkbox').first().locator('input').isChecked()
  note(ev, '英语勾选 前/后/刷新后', `${before}/${mid}/${after}`)
  await expect.soft(after, '目标语言勾选后刷新未回显').toBe(mid)
  await shot(page, 'locales-after-reload')
  if (after !== before) {
    await page.locator('.locale-checkbox-group .locale-checkbox').first().click()
    await page.waitForTimeout(200)
    await page.getByRole('button', { name: /保存自动化设置/ }).click()
    await page.waitForTimeout(800)
  }
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-11
test('SRC-11 分类管理：新建/改名/删除都在弹窗内完成，删除有确认，刷新持久化', async ({ page }, testInfo) => {
  test.setTimeout(120_000) // 新建/改名/删除各一轮「刷新+回读」，30s 预算不够
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page)
  await realLogin(page)
  await openContentTab(page, TAX)
  await page.locator('.subnav button', { hasText: '文章分类' }).first().click()
  await page.waitForTimeout(700)

  const nm = `ACC-W3-分类-${stamp}`
  const nm2 = `${nm}-改`
  await page.getByRole('button', { name: /新增文章分类/ }).first().click()
  await page.waitForTimeout(500)
  const dlg = page.locator('.taxonomy-dialog')
  note(ev, '新建弹窗出现', await dlg.count())
  await expect.soft(await dlg.count(), '新建分类没有弹窗').toBeGreaterThan(0)
  await shot(page, 'create-dialog')
  await dlg.locator('input').first().fill(nm)
  await waitWrite(page, 'POST', 'taxonomy/mutate', () => dlg.getByRole('button', { name: /保存|创建/ }).first().click())
  await expect(dlg).toHaveCount(0)
  note(ev, '新建后弹窗关闭', (await dlg.count()) === 0)

  // 分类列表 10 条/页，历史数据会把新建项挤到第 2 页；回读时用搜索框定位，避免分页干扰
  const reopen = async (q = '') => {
    await page.reload()
    await openContentTab(page, TAX)
    await page.locator('.subnav button', { hasText: '文章分类' }).first().click()
    const searchBox = page.locator('.taxonomy-pane input.search-input').first()
    await expect(searchBox).toBeVisible()
    // refresh() 会异步恢复上次的 taxonomyView/taxonomySearch；先等旧值回显，避免新输入被覆盖。
    await page.waitForTimeout(900)
    if (q) {
      await searchBox.fill(q)
      await expect(searchBox).toHaveValue(q)
    }
  }

  await reopen(nm)
  const row = page.locator('table tbody tr', { hasText: nm })
  note(ev, '新建后刷新回读', await row.count())
  await expect.soft(await row.count(), '新建分类刷新后未持久化').toBeGreaterThan(0)

  await row.first().getByRole('button', { name: '编辑' }).click()
  await page.waitForTimeout(500)
  await dlg.locator('input').first().fill(nm2)
  await waitWrite(page, 'POST', 'taxonomy/mutate', () => dlg.getByRole('button', { name: /保存|创建/ }).first().click())
  await expect(dlg).toHaveCount(0)
  await reopen(nm2)
  const renamed = await page.locator('table tbody tr', { hasText: nm2 }).count()
  note(ev, '改名后回读', renamed)
  await expect.soft(renamed, '改名刷新后未持久化').toBeGreaterThan(0)

  const row2 = page.locator('table tbody tr', { hasText: nm2 }).first()
  await row2.getByRole('button', { name: '删除' }).click()
  await page.waitForTimeout(600)
  const delText = await page.locator('.taxonomy-dialog').innerText().catch(() => '')
  note(ev, '删除确认文案', delText.replace(/\n/g, ' ').slice(0, 80))
  await expect.soft(delText, '删除没有明确确认弹窗').toMatch(/删除/)
  await shot(page, 'delete-confirm')
  await waitWrite(page, 'POST', 'taxonomy/mutate', () => page.locator('.taxonomy-dialog').getByRole('button', { name: /确认删除|删除/ }).last().click())
  await expect(page.locator('.taxonomy-dialog')).toHaveCount(0)
  await reopen(nm2)
  const left = await page.locator('table tbody tr', { hasText: nm2 }).count()
  note(ev, '删除后残留行', left)
  await expect.soft(left, '删除后刷新仍存在该分类').toBe(0)
  await shot(page, 'after-delete')
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-12
test('SRC-12 标签管理：新建不自动弹下拉，中文多字完整保存，删除有确认', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page)
  await realLogin(page)
  await openContentTab(page, TAX)
  await page.locator('.subnav button', { hasText: '标签管理' }).first().click()
  await page.waitForTimeout(700)

  const nm = '大模型推理'
  await page.getByRole('button', { name: /新增标签/ }).first().click()
  await page.waitForTimeout(800)
  const listboxes = await page.locator('[role=listbox], .tag-suggest, .dropdown-menu, [class*=suggest]').evaluateAll((els) =>
    els.filter((e) => (e as HTMLElement).offsetParent !== null).length,
  )
  note(ev, '打开后自动可见下拉/建议面板数', listboxes)
  await expect.soft(listboxes, '打开新建标签时自动弹出了下拉/建议面板').toBe(0)
  await shot(page, 'tag-create')

  const dlg = page.locator('.taxonomy-dialog')
  const inp = dlg.locator('input').first()
  await inp.fill(nm)
  note(ev, '输入框回显值', await inp.inputValue())
  await dlg.getByRole('button', { name: /保存|创建/ }).first().click()
  await page.waitForTimeout(1200)
  await page.reload()
  await openContentTab(page, TAX)
  await page.locator('.subnav button', { hasText: '标签管理' }).first().click()
  await page.waitForTimeout(800)
  // 列表分页，用搜索框按名称过滤后再核对（避免新标签落在第 2 页造成假失败）
  const search = page.locator('input[placeholder*="搜索"], input[placeholder*="查询"]').first()
  if (await search.count()) {
    await search.fill(nm)
    await page.waitForTimeout(800)
  }
  const names = (await page.locator('table tbody tr td:first-child').allInnerTexts()).map((s) => s.trim())
  note(ev, '标签列表首列(前8)', names.slice(0, 8))
  const row = page.locator('table tbody tr', { hasText: nm })
  note(ev, '多字标签完整保存', await row.count())
  await expect.soft(await row.count(), `标签「${nm}」未完整保存（可能被截断）`).toBeGreaterThan(0)

  await row.first().getByRole('button', { name: '删除' }).click()
  await page.waitForTimeout(600)
  const txt = await page.locator('.taxonomy-dialog').innerText().catch(() => '')
  note(ev, '标签删除确认文案', txt.replace(/\n/g, ' ').slice(0, 80))
  await expect.soft(txt, '删除标签没有确认弹窗').toMatch(/删除/)
  await shot(page, 'tag-delete-confirm')
  await page.locator('.taxonomy-dialog').getByRole('button', { name: /确认删除|删除/ }).last().click()
  await page.waitForTimeout(1000)
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- SRC-13
test('SRC-13 内容中心按钮清单：逐个点击都有可观察结果，无失效按钮', async ({ page }, testInfo) => {
  test.setTimeout(300000)
  const ev: Ev = []
  const shot = shooter('w3', testInfo)
  const problems = guard(page, [/\/fetch/, /news-sources → 400/, /Failed to load resource/])
  await realLogin(page)
  await openContentTab(page, NEWS)
  // 关键：默认 30s 等待会让"逐一点击"在弹层/长请求上累计超时，这里收敛等待上限
  page.setDefaultTimeout(6000)
  page.setDefaultNavigationTimeout(20000)

  type Row = { where: string; name: string; result: string }
  const rows: Row[] = []
  const deadline = Date.now() + 190000

  /** 关闭任何拦截点击的遮罩/弹层（不动数据：一律走取消/Esc）。 */
  const clearOverlays = async () => {
    for (let i = 0; i < 3; i++) {
      const cancel = page.locator('.taxonomy-dialog, .ui-confirm, [role=dialog], .content-modal-layer').getByRole('button', { name: /取消|关闭|继续编辑/ })
      if (await cancel.count().catch(() => 0)) await cancel.first().click({ timeout: 1500 }).catch(() => {})
      await page.keyboard.press('Escape').catch(() => {})
      await page.waitForTimeout(120)
      const blocking = await page.locator('.content-modal-layer:visible, .ui-confirm:visible').count().catch(() => 0)
      if (!blocking) break
    }
  }

  const scanAndClick = async (where: string) => {
    const btns = page.locator('section.workspace button:visible')
    const n = await btns.count()
    const seen = new Set<string>()
    for (let i = 0; i < n; i++) {
      if (Date.now() > deadline) {
        rows.push({ where, name: '(剩余按钮)', result: `未点击：时间预算耗尽（已记录 ${seen.size} 个文案）` })
        break
      }
      await clearOverlays()
      const b = btns.nth(i)
      const name = ((await b.innerText().catch(() => '')) || '').trim().replace(/\s+/g, ' ').slice(0, 20)
      if (!name || /上一页|下一页|收起|展开/.test(name)) continue
      if (/^\d+$/.test(name)) continue // 分页页码按钮由翻页能力覆盖，不计入清单
      // 禁用按钮点击本就无响应（表单未填完时的保存/提交），属预期，不计为失效
      if (await b.isDisabled().catch(() => false)) {
        rows.push({ where, name, result: 'disabled：当前不可点（未计入失效）' })
        continue
      }
      // 同一文案的按钮只点一次（按文案去重=按钮清单口径），避免逐行重复点击拖垮用例
      if (seen.has(name)) continue
      seen.add(name)
      const inSubnav = await b.evaluate((el) => !!el.closest('.subnav')).catch(() => false)
      if (inSubnav) continue // 子导航页签由外层逐页点击覆盖，不在本页清单内重复计入
      const clsBefore = await b.getAttribute('class').catch(() => '')
      const urlBefore = page.url()
      const beforeText = await page.locator('section.workspace').innerText()
      let net = 0
      // 请求与响应都计数：POST 切换类操作的响应很快，只盯 response 容易漏；两路都记避免假无反应
      const onApiReq = (r: any) => { if (r.url().includes('/api/')) net += 1 }
      const onResp = (r: any) => { if (r.url().includes('/api/')) net += 1 }
      page.on('request', onApiReq)
      page.on('response', onResp)
      const toastsBefore = (await toastTexts(page)).length
      let clicked = 'ok'
      await b.click({ timeout: 3000, force: true }).catch((e) => { clicked = String(e.message).split('\n')[0].slice(0, 60) })
      await page.waitForTimeout(800)
      page.off('request', onApiReq)
      page.off('response', onResp)
      const leftPage = !page.url().includes('/admin/content')
      const clsAfter = await b.getAttribute('class').catch(() => '')
      const clsChanged = clsBefore !== clsAfter
      const urlChanged = page.url() !== urlBefore
      const dialogs = await page.locator('[role=dialog], .taxonomy-dialog, .source-dialog, .inline-tax-dialog, .ui-confirm').count()
      const toastsAfter = (await toastTexts(page)).length
      const afterText = await page.locator('section.workspace').innerText().catch(() => '')
      // 「启用/停用」这类切换文案字数相同，长度差恒为 0，必须按全文比对
      const textChanged = afterText !== beforeText
      const textDelta = afterText.length - beforeText.length
      const menus = await page.locator('[role=menu]:visible, .dropdown-menu:visible, .menu-popover:visible, .actions-menu:visible, .popover:visible').count().catch(() => 0)
      const observable = menus > 0 || dialogs > 0 || net > 0 || toastsAfter > toastsBefore || textChanged || clicked !== 'ok' || leftPage || clsChanged || urlChanged
      rows.push({ where, name, result: `点击=${clicked} 弹窗=${dialogs} 菜单=${menus} 请求=${net} 提示=${toastsAfter - toastsBefore} 文本Δ=${textDelta}${textChanged ? '(内容变化)' : ''}${clsChanged ? ' 选中态变化' : ''}${urlChanged ? ' URL变化' : ''}${leftPage ? ' 跳转' : ''} → ${observable ? '有结果' : '无反应'}` })
      if (leftPage) {
        await page.goto('/admin/content', { waitUntil: 'domcontentloaded' })
        await page.waitForTimeout(600)
        await page.locator('.subnav button', { hasText: where.split('/')[1] }).first().click().catch(() => {})
        await page.waitForTimeout(600)
        continue
      }
      const cancel = page.locator('.taxonomy-dialog, .ui-confirm, [role=dialog]').getByRole('button', { name: /取消|关闭/ })
      if (await cancel.count()) await cancel.first().click().catch(() => {})
      await page.keyboard.press('Escape').catch(() => {})
      await page.waitForTimeout(200)
    }
  }

  for (const sub of ['已抓取新闻', '新闻源', '抓取任务', '自动化设置']) {
    await page.locator('.subnav button', { hasText: sub }).first().click().catch(() => {})
    await page.waitForTimeout(800)
    await scanAndClick(`${NEWS}/${sub}`)
    await shot(page, `btns-${sub}`)
  }
  await openContentTab(page, TAX)
  for (const sub of ['文章分类', '新闻分类', '标签管理']) {
    await page.locator('.subnav button', { hasText: sub }).first().click().catch(() => {})
    await page.waitForTimeout(700)
    await scanAndClick(`${TAX}/${sub}`)
    await shot(page, `btns-${sub}`)
  }

  const dead = rows.filter((r) => r.result.includes('无反应'))
  note(ev, '按钮清单', rows)
  note(ev, '无反应按钮数', dead.length)
  // 把清单写进证据目录，供报告 §3 引用（不在断言之后生成，避免失败时无产物）
  const outDir = process.env.E2E_REAL_OUT || ''
  if (outDir) {
    const fs = await import('node:fs')
    const lines = ['# SRC-13 内容中心按钮清单（逐一点击实测）', '', '| 页面 | 按钮 | 实测 |', '|---|---|---|']
    for (const r of rows) lines.push(`| ${r.where} | ${r.name || '(图标按钮)'} | ${r.result} |`)
    fs.mkdirSync(`${outDir}/w3`, { recursive: true })
    fs.writeFileSync(`${outDir}/w3/SRC-13-button-inventory.md`, lines.join('\n') + '\n')
  }
  await expect.soft(dead.length, `存在无反应按钮：${JSON.stringify(dead)}`).toBe(0)
  await attachEv(testInfo, ev)
  expectClean(problems)
})
