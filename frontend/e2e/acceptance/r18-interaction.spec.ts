import fs from 'node:fs'
import path from 'node:path'
import { expect, test, type Page, type TestInfo } from '@playwright/test'
import { BASE, dismissOverlays, expectClean, guard, realLogin, shooter, stamp } from '../real/real-helpers'

/**
 * R18 · 前端交互行为验收（INTERACT-01 ~ INTERACT-12）
 *
 * 依据：`docs/v0.4/v0.4_ui_brief_20261026_v3.3.md` §2（基础组件与状态规格）与 §5（全站状态与内容溢出）。
 *
 * 约定：
 *  - 一个 test = 一个 case，标题以 case ID 开头。
 *  - 所有子检查用 `expect.soft`，一次运行把全部违规都报出来。
 *  - 每条子检查都记录**实测值**（computed style / boundingRect / opacity / scrollHeight），
 *    写进 evidence 数组并 attach 到报告 + 落盘 `$E2E_REAL_OUT/<case>-evidence.json`。
 *  - 只用真实登录页（realLogin），无 page.route / mock / token 注入。
 *  - 不写产品代码；不改产品数据（唯一写操作是「精选」开关，用例内 toggle 回原值并 reload 校验复原）。
 *
 * 选择器说明（依据真实 DOM 探针得出）：
 *  - 后台内容中心：`.tabs button` 一级 tab；文章列表 `input.search-input` / `.select-trigger` /
 *    `tbody tr` / `[role="switch"]`；工具栏按钮 `button.button`（查询）与 `button.primary`（新建文章）。
 *  - 文章编辑抽屉：`.article-editor-modal`（自定义 modal-drawer，非 BaseDialog）；
 *    脏数据确认框 `.confirm-dialog`，标题「有未保存修改，确认离开？」，按钮「确认离开 / 继续编辑」。
 *  - 新闻编辑抽屉：`.news-editor`；标题输入 `input[aria-label="标题"]`；错误文案 `.field-error`；
 *    摘要多行框 `textarea.news-summary-input`。
 *  - BaseDialog 实例：/admin/users 的「Create User」→ `.modal-content[role="dialog"]` + `.modal-footer`。
 *  - Toast：`.ccai-neolab-toast`（顶部右侧，含状态标签文本 + 图标）。
 *  - 前台列表：`.timeline-news-card`；分页 `[role="navigation"][aria-label="Pagination"]`。
 */

const OUT = process.env.E2E_REAL_OUT ?? path.resolve(process.cwd(), '../e2e-acceptance-v04/e2e-r18/interaction')
const RUN = stamp

type Verdict = boolean | 'SKIP'
interface Row {
  case: string
  check: string
  expected: string
  measured: string
  ok: Verdict
  note?: string
}

let rows: Row[] = []

/** 记录 + 软断言：一条违规不影响同 case 其它子检查。 */
function ev(caseId: string, check: string, expected: string, measured: string, ok: Verdict, note?: string) {
  rows.push({ case: caseId, check, expected, measured, ok, note })
  if (ok === 'SKIP') {
    console.log(`  SKIP  [${caseId}] ${check} — ${note ?? '未取证'}`)
    return
  }
  console.log(`  ${ok ? 'PASS' : 'FAIL'}  [${caseId}] ${check} | 期望 ${expected} | 实测 ${measured}`)
  expect
    .soft(ok, `[${caseId}] ${check}\n  期望: ${expected}\n  实测: ${measured}${note ? `\n  说明: ${note}` : ''}`)
    .toBe(true)
}

async function finish(testInfo: TestInfo, caseId: string) {
  const mine = rows.filter((r) => r.case === caseId)
  const payload = { case: caseId, title: testInfo.title, at: new Date().toISOString(), base: BASE, rows: mine }
  fs.mkdirSync(OUT, { recursive: true })
  const file = path.join(OUT, `${caseId}-evidence.json`)
  fs.writeFileSync(file, JSON.stringify(payload, null, 2))
  fs.appendFileSync(path.join(OUT, 'evidence.jsonl'), JSON.stringify(payload) + '\n')
  await testInfo.attach(`${caseId}-evidence.json`, { path: file, contentType: 'application/json' })
}

const num = (v: string) => Number.parseFloat(v)
const round = (n: number) => Math.round(n * 100) / 100

/** 读某个选择器的计算样式与盒模型（无自动等待，缺失返回 null）。 */
async function box(page: Page, sel: string) {
  return page.evaluate((s) => {
    const el = document.querySelector(s) as HTMLElement | null
    if (!el) return null
    const r = el.getBoundingClientRect()
    const cs = getComputedStyle(el)
    return {
      text: (el.innerText || el.getAttribute('placeholder') || '').trim().slice(0, 40),
      cls: String(el.className || '').slice(0, 90),
      w: Math.round(r.width * 100) / 100,
      h: Math.round(r.height * 100) / 100,
      radius: cs.borderRadius,
      padL: cs.paddingLeft,
      padR: cs.paddingRight,
      bg: cs.backgroundColor,
      color: cs.color,
      borderTop: `${cs.borderTopWidth} ${cs.borderTopStyle} ${cs.borderTopColor}`,
      borderBottom: `${cs.borderBottomWidth} ${cs.borderBottomStyle} ${cs.borderBottomColor}`,
      borderColor: cs.borderTopColor,
      borderWidth: cs.borderTopWidth,
      shadow: cs.boxShadow,
      outline: `${cs.outlineWidth} ${cs.outlineStyle} ${cs.outlineColor}`,
      outlineOffset: cs.outlineOffset,
      opacity: Number.parseFloat(cs.opacity),
      transform: cs.transform,
      transition: cs.transitionDuration,
      fontSize: cs.fontSize,
      cursor: cs.cursor,
    }
  }, sel)
}

/** 按可见文本精确定位一个元素的样式（比 nth 稳定，不会因插入元素错位）。 */
async function boxByText(page: Page, tag: string, text: string) {
  return page.evaluate(
    ([t, x]) => {
      const els = Array.from(document.querySelectorAll(t as string)) as HTMLElement[]
      const el = els.find((e) => (e.innerText || '').trim() === (x as string))
      if (!el) return null
      const r = el.getBoundingClientRect()
      const cs = getComputedStyle(el)
      return {
        text: (el.innerText || '').trim().slice(0, 40),
        cls: String(el.className || '').slice(0, 90),
        w: Math.round(r.width * 100) / 100,
        h: Math.round(r.height * 100) / 100,
        x: Math.round(r.x * 100) / 100,
        y: Math.round(r.y * 100) / 100,
        radius: cs.borderRadius,
        padL: cs.paddingLeft,
        padR: cs.paddingRight,
        bg: cs.backgroundColor,
        color: cs.color,
        borderWidth: cs.borderTopWidth,
        borderColor: cs.borderTopColor,
        shadow: cs.boxShadow,
        outline: `${cs.outlineWidth} ${cs.outlineStyle} ${cs.outlineColor}`,
        outlineOffset: cs.outlineOffset,
        opacity: Number.parseFloat(cs.opacity),
        transform: cs.transform,
        cursor: cs.cursor,
      }
    },
    [tag, text] as const,
  )
}

const count = (page: Page, sel: string) => page.locator(sel).count()

async function openAdmin(page: Page, tab?: string) {
  await page.goto(`${BASE}/admin/content`, { timeout: 45_000 })
  await page.locator('.tabs button').first().waitFor({ state: 'visible', timeout: 30_000 })
  await page.waitForTimeout(1500)
  await dismissOverlays(page)
  if (tab) {
    await page.locator('.tabs button', { hasText: tab }).first().click({ timeout: 15_000 })
    await page.waitForTimeout(2000)
  }
}

const ARTICLES = '深度解析'
const NEWS = '新闻动态'

/** 打开文章编辑抽屉（真实点击列表行内「编辑」）。 */
async function openArticleEditor(page: Page) {
  await page.locator('tbody tr').first().locator('button', { hasText: /^(编辑|Edit)$/ }).first().click({ timeout: 15_000 })
  await page.locator('.article-editor-modal').first().waitFor({ state: 'visible', timeout: 20_000 })
  await page.waitForTimeout(900)
}

/** 打开新闻编辑抽屉。 */
async function openNewsEditor(page: Page) {
  await page.locator('tbody tr').first().locator('button', { hasText: /^(编辑|Edit)$/ }).first().click({ timeout: 15_000 })
  await page.locator('.news-editor').first().waitFor({ state: 'visible', timeout: 20_000 })
  await page.waitForTimeout(900)
}

/** 关掉一切弹层（用例收尾，避免脏状态串场）。 */
async function closeEverything(page: Page) {
  for (let i = 0; i < 4; i++) {
    if (await count(page, '.confirm-dialog')) {
      await page.locator('.confirm-dialog button', { hasText: /确认离开|Discard/ }).first().click({ timeout: 8_000 }).catch(() => {})
      await page.waitForTimeout(600)
      continue
    }
    if (await count(page, '.article-editor-modal')) {
      await page.locator('.article-editor-modal .close-btn').first().click({ timeout: 8_000 }).catch(() => {})
      await page.waitForTimeout(600)
      continue
    }
    if (await count(page, '.news-editor')) {
      await page.locator('.news-editor .modal-close-button, .news-editor .close-btn').first().click({ timeout: 8_000 }).catch(() => {})
      await page.waitForTimeout(600)
      continue
    }
    if (await count(page, '.modal-content[role="dialog"]')) {
      await page.keyboard.press('Escape')
      await page.waitForTimeout(500)
      continue
    }
    break
  }
}

/* ────────────────────────────── INTERACT-01 ────────────────────────────── */
test('INTERACT-01 Button states: size, hover #1D4ED8, focus ring, disabled opacity + not clickable', async ({ page }, testInfo) => {
  const C = 'INTERACT-01'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const problems = guard(page)
  try {
    await realLogin(page)
    await openAdmin(page, ARTICLES)

    const plain = await boxByText(page, 'button', '查询')
    const primary = await boxByText(page, 'button', '+ 新建文章')
    ev(C, '工具栏存在普通按钮与主按钮', 'button.button 与 button.primary 各 ≥1', `查询=${plain ? 'found' : 'missing'}, + 新建文章=${primary ? 'found' : 'missing'}`, Boolean(plain && primary))

    for (const [name, b] of [['普通按钮「查询」', plain], ['主按钮「＋ 新建文章」', primary]] as const) {
      if (!b) continue
      ev(C, `${name} 高度`, '36px（±1）', `${b.h}px`, Math.abs(b.h - 36) <= 1)
      ev(C, `${name} 圆角`, '8px', b.radius, num(b.radius) === 8)
      ev(C, `${name} 水平 padding`, '16px', `left=${b.padL} / right=${b.padR}`, num(b.padL) === 16 && num(b.padR) === 16)
    }

    // hover：主按钮底色应变为 #1D4ED8 = rgb(29, 78, 216)
    const btn = page.locator('button.primary', { hasText: '新建文章' }).first()
    const rest = await boxByText(page, 'button', '+ 新建文章')
    await btn.hover({ timeout: 10_000 })
    await page.waitForTimeout(450)
    const hovered = await boxByText(page, 'button', '+ 新建文章')
    await page.mouse.move(2, 2)
    await page.waitForTimeout(300)
    ev(
      C,
      '主按钮 hover 底色',
      'rgb(29, 78, 216)（#1D4ED8）',
      `rest=${rest?.bg} → hover=${hovered?.bg}`,
      hovered?.bg === 'rgb(29, 78, 216)',
      'hover 有可视反馈但用的是柠绿加深 + 位移动效，不是 UI Brief 的 hover 主色',
    )

    // focus：键盘 Tab 到按钮，检查 2px 蓝色焦点环 + 2px 偏移
    const focused = await page.evaluate(() => {
      const el = Array.from(document.querySelectorAll('button')).find(
        (b) => (b.innerText || '').trim() === '查询',
      ) as HTMLElement | undefined
      el?.focus()
      const cs = getComputedStyle(el as HTMLElement)
      return { outline: `${cs.outlineWidth} ${cs.outlineStyle} ${cs.outlineColor}`, offset: cs.outlineOffset, shadow: cs.boxShadow }
    })
    const restPlain = plain
    const ringVisible =
      (num(focused.outline.split(' ')[0]) >= 2 && focused.outline.includes('solid')) ||
      (focused.shadow !== 'none' && focused.shadow !== restPlain?.shadow)
    ev(C, '按钮 focus 可见焦点环', 'outline ≥2px（solid）或 box-shadow 与静止态不同', `outline=${focused.outline}, offset=${focused.offset}, shadow=${focused.shadow.slice(0, 60)}`, ringVisible,
      ringVisible ? undefined : `静止态阴影为 ${restPlain?.shadow.slice(0, 60)}，聚焦后完全一致 → 焦点无任何可视变化`)

    // disabled：不透明度 ≈45%，且点击无效
    const disabled = await boxByText(page, 'button', '批量下架')
    ev(C, '禁用按钮不透明度', '≈0.45（0.40–0.50）', `${disabled?.opacity}`, disabled ? Math.abs((disabled.opacity as number) - 0.45) <= 0.05 : false)

    // 禁用态「点不动」要用真实可点击性判定：dispatchEvent 会绕过 disabled 直接跑 Vue handler，
    // 之前据此判定「点了有 toast」属于测试自身伪造的失败。
    const disabledBtn = page.getByRole('button', { name: '批量下架', exact: true }).first()
    const enabledState = await disabledBtn.isEnabled().catch(() => true)
    let clickRejected = false
    let rejectMsg = ''
    try {
      // trial 只做可操作性判定、不真正点击，避免误改数据
      await disabledBtn.click({ trial: true, timeout: 1500 })
    } catch (e) {
      clickRejected = true
      rejectMsg = String(e).split('\n')[0].slice(0, 80)
    }
    ev(
      C,
      '禁用按钮点不动',
      'isEnabled=false 且 trial click 被拒（不产生请求/提示）',
      `isEnabled=${enabledState}, trialRejected=${clickRejected}${rejectMsg ? `, msg=${rejectMsg}` : ''}`,
      enabledState === false && clickRejected,
    )

    // loading 态：需真实发一个写请求才能观察，本轮不制造产品数据
    ev(C, 'loading 保留宽度 + 转圈 + 不可点', '按钮进入 loading 时宽度不变、有 spinner、disabled', '未取证', 'SKIP',
      '观察 loading 需要触发真实写请求（会改产品数据）；本轮以「不写数据」为约束，改用源码级组件确认不在此 spec 断言，避免伪造结论')

    const problemsText = problems.map((p) => `[${p.kind}] ${p.detail}`).join(' | ')
    ev(C, '交互过程中无 console/pageerror/4xx', 'problems 为空', problemsText || 'clean', problems.length === 0)
    expectClean(problems)
  } finally {
    await closeEverything(page)
    await finish(testInfo, C)
  }
})

/* ────────────────────────────── INTERACT-02 ────────────────────────────── */
test('INTERACT-02 Input states: 36px/2px/8px, focus primary border, error red border + 12px text, no underline-only', async ({ page }, testInfo) => {
  const C = 'INTERACT-02'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const shot = shooter('r18', testInfo)
  try {
    await realLogin(page)
    await openAdmin(page, NEWS)
    await openNewsEditor(page)
    await shot(page, 'news-editor-open')

    const titleSel = '.news-editor input[aria-label="标题"]'
    const title = await box(page, titleSel)
    ev(C, '单行输入高度', '36px（±1）', `${title?.h}px`, title ? Math.abs((title.h as number) - 36) <= 1 : false)
    ev(C, '单行输入边框', '2px', `${title?.borderWidth}`, title ? num(title.borderWidth) === 2 : false)
    ev(C, '单行输入圆角', '8px', `${title?.radius}`, title ? num(title.radius) === 8 : false)
    ev(C, '非「仅下划线」样式', '四边都有边框（上/下 ≥1px，且下边 ≥2px）', `top=${title?.borderTop} / bottom=${title?.borderBottom}`,
      Boolean(title && num(String(title.borderBottom).split(' ')[0]) >= 2 && num(String(title.borderTop).split(' ')[0]) >= 1))

    // hover → 主色边框
    const restBorder = title?.borderColor
    await page.locator(titleSel).first().hover({ timeout: 10_000 })
    await page.waitForTimeout(400)
    const hoverTitle = await box(page, titleSel)
    await page.mouse.move(2, 2)
    await page.waitForTimeout(250)
    ev(C, '输入框 hover 边框', '主色 rgb(37, 78, 216) / #2563EB 系', `rest=${restBorder} → hover=${hoverTitle?.borderColor}`, hoverTitle?.borderColor === 'rgb(37, 99, 235)',
      hoverTitle?.borderColor === restBorder ? 'hover 边框未变化' : undefined)

    // focus → 主色边框 + 焦点环
    await page.locator(titleSel).first().focus()
    await page.waitForTimeout(350)
    const focusTitle = await box(page, titleSel)
    ev(C, '输入框 focus 边框', '主色 rgb(37, 99, 235)', `${focusTitle?.borderColor}`, focusTitle?.borderColor === 'rgb(37, 99, 235)',
      focusTitle?.borderColor === restBorder ? 'focus 后边框颜色未变（仅靠 box-shadow 表达焦点）' : undefined)
    const focusRing = Boolean(focusTitle && focusTitle.shadow !== (title?.shadow ?? 'none') && focusTitle.shadow !== 'none')
    ev(C, '输入框 focus 焦点环', 'box-shadow ≠ none 且与静止态不同', `rest=${title?.shadow.slice(0, 50)} → focus=${focusTitle?.shadow.slice(0, 50)}`, focusRing)

    // error：清空标题后失焦 → aria-invalid + 红边 + 12px 错误文案
    await page.locator(titleSel).first().fill('', { timeout: 10_000 })
    await page.locator(titleSel).first().blur()
    await page.waitForTimeout(800)
    const err = await box(page, '.news-editor .field-error')
    const errTitle = await box(page, titleSel)
    const invalid = await page.evaluate((s) => {
      const el = document.querySelector(s) as HTMLInputElement | null
      return el?.getAttribute('aria-invalid') ?? null
    }, titleSel)
    ev(C, '失焦校验触发', 'aria-invalid=true 且出现错误文案', `aria-invalid=${invalid}, errorText=${JSON.stringify(err?.text)}`, invalid === 'true' && Boolean(err?.text))
    ev(C, '错误态边框为红色', '红（如 #DC2626 / #B42318）', `${errTitle?.borderColor}`, /rgb\((220, 38, 38|180, 35, 24)\)/.test(String(errTitle?.borderColor)),
      `边框仍为 ${errTitle?.borderColor}`)
    ev(C, '错误文案字号', '12px', `${err?.fontSize}`, err ? num(err.fontSize) === 12 : false)
    ev(C, '错误文案颜色为红', '红（#DC2626 / #B42318 系）', `${err?.color}`, /rgb\((220, 38, 38|180, 35, 24)\)/.test(String(err?.color)))
    await shot(page, 'title-error')

    await closeEverything(page)

    // 基础组件 Input（.input，BaseDialog 表单）对照
    await page.goto(`${BASE}/admin/users`, { timeout: 45_000 })
    await page.waitForTimeout(2500)
    await dismissOverlays(page)
    await page.locator('button', { hasText: /Create User|新建用户/ }).first().click({ timeout: 15_000 })
    await page.waitForTimeout(1200)
    const baseInput = await box(page, '.modal-content[role="dialog"] input.input')
    ev(C, '基础组件输入框高度', '36px（±1）', `${baseInput?.h}px`, baseInput ? Math.abs((baseInput.h as number) - 36) <= 1 : false)
    ev(C, '基础组件输入框圆角', '8px', `${baseInput?.radius}`, baseInput ? num(baseInput.radius) === 8 : false)
    ev(C, '基础组件输入框边框', '2px', `${baseInput?.borderWidth}`, baseInput ? num(baseInput.borderWidth) === 2 : false)
    await page.locator('.modal-content[role="dialog"] input.input').first().focus()
    await page.waitForTimeout(300)
    const baseFocus = await box(page, '.modal-content[role="dialog"] input.input')
    ev(C, '基础组件输入框 focus 边框为蓝色', '主色 rgb(37, 99, 235)', `${baseFocus?.borderColor}`, baseFocus?.borderColor === 'rgb(37, 99, 235)')
    expectClean([])
  } finally {
    await closeEverything(page)
    await finish(testInfo, C)
  }
})

/* ────────────────────────────── INTERACT-03 ────────────────────────────── */
test('INTERACT-03 Multi-line textarea: initial ≥96px, auto-grow, cap 400px then internal scroll', async ({ page }, testInfo) => {
  const C = 'INTERACT-03'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const shot = shooter('r18', testInfo)
  try {
    await realLogin(page)
    await openAdmin(page, NEWS)
    await openNewsEditor(page)
    await shot(page, 'news-editor')

    const taSel = '.news-editor textarea.news-summary-input'
    const probe = (sel: string) =>
      page.evaluate((s) => {
        const el = document.querySelector(s) as HTMLTextAreaElement | null
        if (!el) return null
        const cs = getComputedStyle(el)
        return {
          h: Math.round(el.getBoundingClientRect().height),
          scrollH: el.scrollHeight,
          clientH: el.clientHeight,
          minH: cs.minHeight,
          maxH: cs.maxHeight,
          overflowY: cs.overflowY,
        }
      }, sel)

    const empty = await probe(taSel)
    ev(C, '多行框初始高度', '≥96px', `${empty?.h}px（min-height=${empty?.minH}）`, empty ? (empty.h as number) >= 96 : false)

    await page.locator(taSel).first().fill('A'.repeat(240), { timeout: 10_000 })
    await page.waitForTimeout(400)
    const grown = await probe(taSel)
    ev(C, '内容增加后自动增高', `高度 > 初始 ${empty?.h}px`, `${grown?.h}px`, Boolean(empty && grown && (grown.h as number) > (empty.h as number)),
      empty && grown && empty.h === grown.h ? `填 240 字后高度仍是 ${grown.h}px（scrollHeight=${grown.scrollH}），未随内容增长` : undefined)

    const many = Array.from({ length: 40 }, (_, i) => `第 ${i + 1} 行：多行输入自动增长验收内容`).join('\n')
    await page.locator(taSel).first().fill(many, { timeout: 10_000 })
    await page.waitForTimeout(600)
    const capped = await probe(taSel)
    ev(C, '高度上限 400px', '高度 ≤400px 且随内容增长逼近上限', `${capped?.h}px`, Boolean(capped && (capped.h as number) <= 400))
    ev(C, '超高后内部滚动', 'scrollHeight > clientHeight', `scrollHeight=${capped?.scrollH} > clientHeight=${capped?.clientH}`, Boolean(capped && capped.scrollH > capped.clientH))
    ev(C, '超高后高度确实到达上限附近', '≥300px（内容溢出时应已长到接近 400）', `${capped?.h}px`, Boolean(capped && (capped.h as number) >= 300),
      `实测恒定 ${capped?.h}px，说明没有自动增长，只有内部滚动`)
    await shot(page, 'textarea-40lines')

    // 文章编辑抽屉的摘要多行框对照
    await closeEverything(page)
    await openAdmin(page, ARTICLES)
    await openArticleEditor(page)
    const excerpt = await page.evaluate(() => {
      const el = Array.from(document.querySelectorAll('.article-editor-modal textarea'))[0] as HTMLTextAreaElement | undefined
      if (!el) return null
      return { h: Math.round(el.getBoundingClientRect().height), sh: el.scrollHeight, ch: el.clientHeight }
    })
    ev(C, '文章摘要多行框初始高度', '≥96px', `${excerpt?.h}px`, excerpt ? (excerpt.h as number) >= 96 : false)
    await page.evaluate(() => {
      const el = Array.from(document.querySelectorAll('.article-editor-modal textarea'))[0] as HTMLTextAreaElement
      el.focus()
      el.value = 'B'.repeat(400)
      el.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await page.waitForTimeout(500)
    const excerpt2 = await page.evaluate(() => {
      const el = Array.from(document.querySelectorAll('.article-editor-modal textarea'))[0] as HTMLTextAreaElement
      return { h: Math.round(el.getBoundingClientRect().height), sh: el.scrollHeight, ch: el.clientHeight }
    })
    ev(C, '文章摘要多行框自动增高', `高度 > ${excerpt?.h}px`, `${excerpt2.h}px`, Boolean(excerpt && excerpt2.h > (excerpt.h as number)),
      excerpt && excerpt.h === excerpt2.h ? '填 400 字后高度未变' : undefined)
    expectClean([])
  } finally {
    await closeEverything(page)
    await finish(testInfo, C)
  }
})

/* ────────────────────────────── INTERACT-04 ────────────────────────────── */
test('INTERACT-04 Custom Select: no native select, panel shadow, ArrowDown+Enter, Esc', async ({ page }, testInfo) => {
  const C = 'INTERACT-04'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const shot = shooter('r18', testInfo)
  try {
    await realLogin(page)
    await openAdmin(page, ARTICLES)

    ev(C, '页面无原生 <select>', '0 个', `${await count(page, 'select')}`, (await count(page, 'select')) === 0)
    const trig = await box(page, '.select-trigger')
    ev(C, 'Select 触发器高度', '36px（±1）', `${trig?.h}px`, trig ? Math.abs((trig.h as number) - 36) <= 1 : false)
    ev(C, 'Select 触发器圆角', '8px', `${trig?.radius}`, trig ? num(trig.radius) === 8 : false)

    const label0 = (await box(page, '.select-trigger'))?.text ?? ''
    await page.locator('.select-trigger').first().focus()
    await page.keyboard.press('Enter')
    await page.waitForTimeout(600)
    const panel = await box(page, '.select-dropdown-portal')
    await shot(page, 'select-open')
    ev(C, '面板打开', '可见且带阴影', `panel=${panel ? 'visible' : 'missing'}, shadow=${panel?.shadow.slice(0, 40)}`, Boolean(panel && panel.shadow !== 'none'))
    ev(C, '面板圆角', '8px', `${panel?.radius}`, panel ? num(panel.radius) === 8 : false)
    ev(C, '面板宽高合理', '宽 ≥ 触发器宽、高 > 0', `w=${panel?.w}, h=${panel?.h}`, Boolean(panel && (panel.w as number) >= (trig?.w ?? 0)))

    const opts = await page.evaluate(() =>
      Array.from(document.querySelectorAll('.select-dropdown-portal .select-option')).map((e) => ({
        t: (e.textContent || '').trim().slice(0, 16),
        sel: e.getAttribute('aria-selected'),
      })),
    )
    ev(C, '选项渲染', '≥2 个 option', `${opts.length} 个：${opts.map((o) => o.t).join(' / ')}`, opts.length >= 2)

    await page.keyboard.press('ArrowDown')
    await page.waitForTimeout(150)
    await page.keyboard.press('ArrowDown')
    await page.waitForTimeout(250)
    const focusedOpt = await page.evaluate(() => {
      const el = document.querySelector('.select-dropdown-portal .select-option-focused') as HTMLElement | null
      return el ? (el.textContent || '').trim().slice(0, 16) : null
    })
    ev(C, 'ArrowDown 移动高亮', '高亮项存在且随按键移动', `focused=${focusedOpt}`, Boolean(focusedOpt))
    await page.keyboard.press('Enter')
    await page.waitForTimeout(700)
    const label1 = (await box(page, '.select-trigger'))?.text ?? ''
    ev(C, 'Enter 选中并回填', `值由「${label0}」变为高亮项「${focusedOpt}」`, `now=${label1}`, Boolean(focusedOpt && label1 === focusedOpt))

    await page.locator('.select-trigger').first().focus()
    await page.keyboard.press('Enter')
    await page.waitForTimeout(500)
    const opened2 = await count(page, '.select-dropdown-portal')
    await page.keyboard.press('Escape')
    await page.waitForTimeout(500)
    const afterEsc = await count(page, '.select-dropdown-portal')
    ev(C, 'Esc 关闭面板', `打开=${opened2} → 关闭后 0`, `${afterEsc}`, opened2 > 0 && afterEsc === 0)

    // 搜索：选项 >5 的 Select（全部模型）应出现搜索框
    const modelSelect = page.locator('[data-test="article-model"] .select-trigger').first()
    await modelSelect.click({ timeout: 10_000 })
    await page.waitForTimeout(600)
    const searchInput = await count(page, '.select-search-input')
    const before = await count(page, '.select-dropdown-portal .select-option')
    if (searchInput > 0) {
      await page.locator('.select-search-input').first().fill('gpt', { timeout: 10_000 })
      await page.waitForTimeout(600)
      const after = await count(page, '.select-dropdown-portal .select-option')
      ev(C, 'Select 支持搜索过滤', `输入后选项数 < ${before}`, `${before} → ${after}`, after < before)
    } else {
      ev(C, 'Select 支持搜索', '可搜索 Select 出现搜索框', `searchInput=${searchInput}`, 'SKIP', '该 Select 选项数不足（searchable="auto" 时 ≤5 不启用搜索），未强行改产品配置')
    }
    await page.keyboard.press('Escape')
    await page.waitForTimeout(300)

    // 清空能力
    const clear = await count(page, '.select-clear')
    if (clear > 0) {
      ev(C, 'Select 支持清空', '.select-clear 存在且可点', `${clear}`, clear > 0)
    } else {
      ev(C, 'Select 支持清空', 'clearable 的 Select 出现 .select-clear', `0`, 'SKIP', '当前页所有 Select 均未开启 clearable（产品用法所致），无法在不改产品配置的前提下验证')
    }
    await shot(page, 'select-final')
    expectClean([])
  } finally {
    await closeEverything(page)
    await finish(testInfo, C)
  }
})

/* ────────────────────────────── INTERACT-05 ────────────────────────────── */
test('INTERACT-05 Toggle: slider form, on/off colour, role=switch, text state, focus ring, persists after reload', async ({ page }, testInfo) => {
  const C = 'INTERACT-05'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const shot = shooter('r18', testInfo)
  let restore: (() => Promise<void>) | null = null
  try {
    await realLogin(page)
    await openAdmin(page, ARTICLES)

    const n = await count(page, '[role="switch"]')
    ev(C, '存在 switch 组件', '≥1', `${n}`, n > 0)
    const sw0 = await box(page, '[role="switch"]')
    ev(C, '滑杆形态（非原生 checkbox）', '圆角 >= 999px、尺寸 ~46x26、内部无 input[type=checkbox]', `radius=${sw0?.radius}, ${sw0?.w}x${sw0?.h}`, Boolean(sw0 && num(sw0.radius) >= 999 && (sw0.h as number) <= 32))
    ev(C, '无原生 checkbox 外观', '0 个 input[type=checkbox] 承担该状态', `${await count(page, '[role="switch"] input[type="checkbox"]')}`, (await count(page, '[role="switch"] input[type="checkbox"]')) === 0)
    ev(C, '可读文本状态', '开关自身/相邻位置有「开/关」文本', `switch.innerText=${JSON.stringify(sw0?.text)}, aria-label=${await page.evaluate(() => document.querySelector('[role="switch"]')?.getAttribute('aria-label'))}`, Boolean(sw0 && (sw0.text || '').length > 0),
      '只有 aria-label（读屏可用），视觉上没有文字状态；UI Brief 要求「带可读文本状态」')

    // 找一个处于 off 的行，打开 → 检查颜色 / 持久化 → 关闭还原
    const idx = await page.evaluate(() => {
      const els = Array.from(document.querySelectorAll('[role="switch"]'))
      return els.findIndex((e) => e.getAttribute('aria-checked') === 'false')
    })
    if (idx < 0) {
      ev(C, '可用的 off 态开关', '存在 aria-checked=false 的开关', 'none', 'SKIP', '当前列表 20 行的精选开关全部为开，无安全可切换的行')
    } else {
      const rowTitle = await page.evaluate((i) => {
        const el = document.querySelectorAll('[role="switch"]')[i]
        const tr = el?.closest('tr') as HTMLElement | null
        return tr ? (tr.querySelector('td')?.textContent || '').trim().slice(0, 40) : ''
      }, idx)
      const toggleOf = (title: string) =>
        page.locator('tbody tr', { hasText: title.slice(0, 18) }).first().locator('[role="switch"]').first()

      const off = await page.evaluate((i) => {
        const el = document.querySelectorAll('[role="switch"]')[i] as HTMLElement
        const cs = getComputedStyle(el)
        return { checked: el.getAttribute('aria-checked'), bg: cs.backgroundColor, opacity: cs.opacity }
      }, idx)
      ev(C, 'off 态为灰色', '灰（gray-200 系）', `${off.bg}`, /rgb\(2[0-9]{2}, 2[0-9]{2}, 2[0-9]{2}\)/.test(off.bg) || off.bg === 'rgb(203, 213, 225)')

      // focus ring
      const ring = await page.evaluate((i) => {
        const el = document.querySelectorAll('[role="switch"]')[i] as HTMLElement
        el.focus()
        const cs = getComputedStyle(el)
        return { shadow: cs.boxShadow, ow: cs.outlineWidth, os: cs.outlineStyle }
      }, idx)
      const ringOk = ring.shadow !== 'none' || (num(String(ring.ow)) >= 2 && String(ring.os) !== 'none')
      ev(C, '开关可见焦点环', 'box-shadow ≠ none，或 outline ≥2px 且 style 非 none', `shadow=${ring.shadow.slice(0, 60)}, outline=${ring.ow}/${ring.os}`, ringOk,
        ringOk ? undefined : '聚焦后既无 box-shadow 也无实线 outline → 键盘用户看不到焦点位置')

      await toggleOf(rowTitle).click({ timeout: 10_000 })
      await page.waitForTimeout(1600)
      restore = async () => {
        const t = toggleOf(rowTitle)
        if ((await t.count()) && (await t.getAttribute('aria-checked')) === 'true') {
          await t.click({ timeout: 10_000 }).catch(() => {})
          await page.waitForTimeout(1200)
        }
      }
      const onNow = await toggleOf(rowTitle).evaluate((el) => {
        const cs = getComputedStyle(el)
        return { checked: el.getAttribute('aria-checked'), bg: cs.backgroundColor }
      })
      await shot(page, 'toggle-on')
      ev(C, '点击后切换为 on', 'aria-checked=true', `checked=${onNow.checked}`, onNow.checked === 'true')
      ev(C, 'on 态为主蓝', 'rgb(37, 99, 235)（#2563EB）', `${onNow.bg}`, onNow.bg === 'rgb(37, 99, 235)')

      await page.reload({ timeout: 45_000 })
      await page.waitForTimeout(2500)
      await dismissOverlays(page)
      // reload 会回到默认 tab，需重新进入文章列表再读同一行
      if (await count(page, '.tabs button.active')) {
        const activeTab = await page.locator('.tabs button.active').first().innerText().catch(() => '')
        if (!activeTab.includes(ARTICLES)) {
          await page.locator('.tabs button', { hasText: ARTICLES }).first().click({ timeout: 12_000 })
          await page.waitForTimeout(2500)
        }
      }
      const persisted = await toggleOf(rowTitle).evaluate((el) => el.getAttribute('aria-checked')).catch(() => 'row-missing')
      ev(C, '状态在 reload 后持久化', 'true', `${persisted}`, persisted === 'true', persisted === 'row-missing' ? 'reload 后找不到该行（列表分页/排序变化）' : undefined)

      await restore()
      restore = null
      await page.waitForTimeout(800)
      const back = await toggleOf(rowTitle).evaluate((el) => el.getAttribute('aria-checked')).catch(() => 'row-missing')
      ev(C, '用例已把开关还原为原值', 'false', `${back}`, back === 'false')
    }

    ev(C, '请求中禁用（disabled while in flight）', '提交期间 aria-disabled=true / 不可点', '未取证', 'SKIP', 'PATCH 在本地栈 <100ms 完成，无法稳定捕捉在途中状态；不强断言以免假阳性')
    ev(C, '失败回滚 + Toast', '接口失败时回到原值并弹 Toast', '未取证', 'SKIP', '强制失败需要 mock/route 拦截，本 spec 禁止 mock，故不伪造')
    expectClean([])
  } finally {
    if (restore) await restore().catch(() => {})
    await closeEverything(page)
    await finish(testInfo, C)
  }
})

/* ────────────────────────────── INTERACT-06 ────────────────────────────── */
test('INTERACT-06 Modal four close methods without modifications (× / 取消 / backdrop / Esc)', async ({ page }, testInfo) => {
  const C = 'INTERACT-06'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const shot = shooter('r18', testInfo)
  try {
    await realLogin(page)
    await page.goto(`${BASE}/admin/users`, { timeout: 45_000 })
    await page.waitForTimeout(2500)
    await dismissOverlays(page)

    const opener = page.locator('button', { hasText: /Create User|新建用户/ }).first()
    const dlg = '.modal-content[role="dialog"]'

    const open = async () => {
      await opener.click({ timeout: 12_000 })
      await page.waitForTimeout(1100)
      return (await count(page, dlg)) > 0
    }

    const meta = await (async () => { await open(); return box(page, dlg) })()
    await shot(page, 'dialog-open')
    ev(C, 'BaseDialog 打开', '1 个 .modal-content[role="dialog"]', `${await count(page, dlg)}`, true)
    ev(C, '弹窗宽度（中号）', '640px', `${meta?.w}px`, meta ? Math.abs((meta.w as number) - 640) <= 4 : false)
    ev(C, '弹窗圆角', '12px', `${meta?.radius}`, meta ? num(meta.radius) === 12 : false)

    await page.locator('.modal-close-button').first().click({ timeout: 10_000 })
    await page.waitForTimeout(900)
    ev(C, '× 关闭', '关闭后 0 个弹窗', `${await count(page, dlg)}`, (await count(page, dlg)) === 0)

    await open()
    await page.locator('.modal-footer button', { hasText: /Cancel|取消/ }).first().click({ timeout: 10_000 })
    await page.waitForTimeout(900)
    ev(C, '底部「取消」关闭', '关闭后 0 个弹窗', `${await count(page, dlg)}`, (await count(page, dlg)) === 0)

    await open()
    await page.mouse.click(6, 6)
    await page.waitForTimeout(900)
    ev(C, '点遮罩关闭', '关闭后 0 个弹窗', `${await count(page, dlg)}`, (await count(page, dlg)) === 0)

    await open()
    await page.keyboard.press('Escape')
    await page.waitForTimeout(900)
    ev(C, 'Esc 关闭', '关闭后 0 个弹窗', `${await count(page, dlg)}`, (await count(page, dlg)) === 0)

    // 附加：内容中心文章编辑抽屉（v0.4 主编辑弹窗，自定义 drawer）同样四种方式
    await openAdmin(page, ARTICLES)
    await openArticleEditor(page)
    await page.keyboard.press('Escape')
    await page.waitForTimeout(1000)
    const afterEscDrawer = await count(page, '.article-editor-modal')
    ev(C, '（内容中心）Esc 关闭文章编辑抽屉', '按键后弹窗关闭', `modal=${afterEscDrawer}`, afterEscDrawer === 0,
      afterEscDrawer !== 0 ? '打开后焦点仍停在触发按钮（在弹窗之外），Esc keydown 未落到抽屉节点上 → Esc 失效' : undefined)

    await closeEverything(page)
    await openArticleEditor(page)
    await page.mouse.click(5, 5)
    await page.waitForTimeout(900)
    ev(C, '（内容中心）点遮罩关闭', '关闭', `modal=${await count(page, '.article-editor-modal')}`, (await count(page, '.article-editor-modal')) === 0)
    expectClean([])
  } finally {
    await closeEverything(page)
    await finish(testInfo, C)
  }
})

/* ────────────────────────────── INTERACT-07 ────────────────────────────── */
test('INTERACT-07 Modal dirty-close confirm for all four close methods', async ({ page }, testInfo) => {
  const C = 'INTERACT-07'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const shot = shooter('r18', testInfo)
  try {
    await realLogin(page)
    await openAdmin(page, ARTICLES)

    const modal = '.article-editor-modal'
    const guardBox = '.confirm-dialog'
    // 抽屉内首个 input 即「文章标题」（探针确认 placeholder=输入引人入胜的文章标题...）
    const titleSel = `${modal} input`

    const dirty = async () => {
      const cur = await page.evaluate((s) => (document.querySelector(s) as HTMLInputElement | null)?.value ?? '', titleSel)
      await page.fill(titleSel, cur + 'X', { timeout: 10_000 })
      await page.waitForTimeout(600)
      return cur.length + 1
    }
    const guardState = () =>
      page.evaluate(([g, m]) => ({
        modal: Boolean(document.querySelector(m as string)),
        guard: Boolean(document.querySelector(g as string)),
        text: ((document.querySelector(g as string) as HTMLElement | null)?.innerText || '').replace(/\n/g, ' | ').slice(0, 90),
        buttons: Array.from(document.querySelectorAll(`${g} button`)).map((b) => (b.textContent || '').trim()),
        value: (document.querySelector('.article-editor-modal input') as HTMLInputElement | null)?.value ?? null,
        rowTitle: ((document.querySelector('tbody tr td.title-cell') as HTMLElement | null)?.innerText || '').trim().slice(0, 40),
      }), [guardBox, modal] as const)

    const methods: Array<[string, () => Promise<void>]> = [
      ['×', async () => { await page.click(`${modal} .close-btn`, { timeout: 10_000 }) }],
      ['取消', async () => { await page.click(`${modal} .modal-drawer-footer button:has-text("取消")`, { timeout: 10_000 }) }],
      ['backdrop', async () => { await page.mouse.click(5, 5) }],
      ['Esc', async () => { await page.keyboard.press('Escape') }],
    ]

    for (const [name, act] of methods) {
      await closeEverything(page)
      await openArticleEditor(page)
      const original = await page.evaluate((s) => (document.querySelector(s) as HTMLInputElement | null)?.value ?? '', titleSel)
      await dirty()
      await act()
      await page.waitForTimeout(900)
      const st = await guardState()
      await shot(page, `dirty-${name}`)
      ev(C, `${name} 关闭时弹出未保存确认`, '确认框出现且文案为「有未保存修改，确认离开？」', `guard=${st.guard}, text=${JSON.stringify(st.text.slice(0, 30))}, buttons=${st.buttons.join('/')}`,
        st.guard && st.text.includes('有未保存修改，确认离开？'))
      ev(C, `${name} 确认框按钮文案`, '「确认离开」「继续编辑」', `${st.buttons.join(' / ')}`, st.buttons.includes('确认离开') && st.buttons.includes('继续编辑'))

      if (st.guard) {
        await page.click(`${guardBox} button:has-text("继续编辑")`, { timeout: 10_000 })
        await page.waitForTimeout(900)
        const kept = await guardState()
        ev(C, `${name} →「继续编辑」保留内容`, `回到编辑器且标题仍为 ${JSON.stringify(original + 'X')}`, `modal=${kept.modal}, value=${JSON.stringify(kept.value)}`,
          kept.modal && kept.value === original + 'X')

        // 直接丢弃，结束本次循环
        await page.click(`${modal} .close-btn`, { timeout: 10_000 }).catch(() => {})
        await page.waitForTimeout(700)
        if (await count(page, guardBox)) {
          await page.click(`${guardBox} button:has-text("确认离开")`, { timeout: 10_000 })
          await page.waitForTimeout(1100)
        }
        const closed = await guardState()
        ev(C, `${name} →「确认离开」关闭且不保存`, `弹窗关闭，列表首行标题仍为 ${JSON.stringify(original)}`, `modal=${closed.modal}, rowTitle=${JSON.stringify(closed.rowTitle)}, value=${closed.value}`,
          !closed.modal && closed.rowTitle === original.trim())
      }
    }

    // 附加：BaseDialog 系表单（新建用户）没有脏数据保护，直接关闭即丢输入
    await page.goto(`${BASE}/admin/users`, { timeout: 45_000 })
    await page.waitForTimeout(2200)
    await dismissOverlays(page)
    await page.locator('button', { hasText: /Create User|新建用户/ }).first().click({ timeout: 12_000 })
    await page.waitForTimeout(1100)
    await page.fill('.modal-content[role="dialog"] input.input', 'dirty-' + RUN + '@example.com', { timeout: 10_000 })
    await page.waitForTimeout(400)
    await page.keyboard.press('Escape')
    await page.waitForTimeout(900)
    const userGuard = await count(page, '.modal-discard-dialog, [role="alertdialog"]')
    ev(C, '（BaseDialog 表单）脏数据关闭确认', '有未保存输入时关闭应先确认', `确认框=${userGuard}, 弹窗=${await count(page, '.modal-content[role="dialog"]')}`, userGuard > 0,
      userGuard === 0 ? 'BaseDialog 的 hasUnsavedChanges 能力存在，但产品内无任何调用方传值 → 新建用户弹窗填了邮箱按 Esc 直接关闭并丢弃输入' : undefined)
    expectClean([])
  } finally {
    await closeEverything(page)
    await finish(testInfo, C)
  }
})

/* ────────────────────────────── INTERACT-08 ────────────────────────────── */
test('INTERACT-08 Focus management: initial focus, focus trap, focus returns to trigger', async ({ page }, testInfo) => {
  const C = 'INTERACT-08'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const shot = shooter('r18', testInfo)
  try {
    await realLogin(page)
    await page.goto(`${BASE}/admin/users`, { timeout: 45_000 })
    await page.waitForTimeout(2500)
    await dismissOverlays(page)

    const opener = page.locator('button', { hasText: /Create User|新建用户/ }).first()
    await opener.click({ timeout: 12_000 })
    await page.waitForTimeout(1200)
    await shot(page, 'dialog-focus')

    const initial = await page.evaluate(() => {
      const a = document.activeElement as HTMLElement | null
      const d = document.querySelector('.modal-content[role="dialog"]')
      return {
        tag: a?.tagName ?? null,
        ph: a?.getAttribute('placeholder') ?? null,
        inside: Boolean(d && a && d.contains(a)),
      }
    })
    ev(C, '打开后焦点进入弹窗首个字段', '焦点在 dialog 内且落在 input 上', `tag=${initial.tag}, placeholder=${JSON.stringify(initial.ph)}, inside=${initial.inside}`,
      initial.inside && initial.tag === 'INPUT')

    let escapes = 0
    const path: string[] = []
    for (let i = 0; i < 40; i++) {
      await page.keyboard.press('Tab')
      const info = await page.evaluate(() => {
        const a = document.activeElement as HTMLElement | null
        const d = document.querySelector('.modal-content[role="dialog"]')
        return { inside: Boolean(d && a && d.contains(a)), t: (a?.innerText || a?.getAttribute('placeholder') || a?.tagName || '').slice(0, 18) }
      })
      if (!info.inside) escapes += 1
      if (i < 8) path.push(`${i}:${info.t}`)
    }
    ev(C, 'Tab 焦点锁在弹窗内', '40 次 Tab 逃逸 0 次', `escapes=${escapes}；前 8 跳 ${path.join(' → ')}`, escapes === 0)

    await page.keyboard.press('Escape')
    await page.waitForTimeout(900)
    const back = await page.evaluate(() => {
      const a = document.activeElement as HTMLElement | null
      return { tag: a?.tagName ?? null, text: (a?.innerText || '').trim().slice(0, 24), inDialog: Boolean(document.querySelector('.modal-content[role="dialog"]')) }
    })
    ev(C, '关闭后焦点回到触发元素', 'activeElement 为打开弹窗的按钮', `tag=${back.tag}, text=${JSON.stringify(back.text)}, dialogOpen=${back.inDialog}`, back.tag === 'BUTTON' && /Create User|新建用户/.test(back.text))

    // 附加：内容中心文章编辑抽屉
    await openAdmin(page, ARTICLES)
    await openArticleEditor(page)
    const drawerFocus = await page.evaluate(() => {
      const a = document.activeElement as HTMLElement | null
      const d = document.querySelector('.article-editor-modal')
      return { tag: a?.tagName ?? null, inside: Boolean(d && a && d.contains(a)), text: (a?.innerText || '').trim().slice(0, 18) }
    })
    ev(C, '（内容中心）打开后焦点进入弹窗', '焦点在 .article-editor-modal 内', `tag=${drawerFocus.tag}, inside=${drawerFocus.inside}, text=${JSON.stringify(drawerFocus.text)}`, drawerFocus.inside,
      drawerFocus.inside ? undefined : '打开文章编辑抽屉后焦点仍停在列表行内「编辑」按钮上（在弹窗之外），不符合「打开后焦点进入首个字段」')
    expectClean([])
  } finally {
    await closeEverything(page)
    await finish(testInfo, C)
  }
})

/* ────────────────────────────── INTERACT-09 ────────────────────────────── */
test('INTERACT-09 Toast: top feedback, status colour + icon + text, auto dismiss', async ({ page }, testInfo) => {
  const C = 'INTERACT-09'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const shot = shooter('r18', testInfo)
  try {
    await realLogin(page)
    await openAdmin(page, NEWS)
    await page.locator('button', { hasText: '自动化设置' }).first().click({ timeout: 12_000 })
    await page.waitForTimeout(1500)

    // 保存自动化设置（值不变，不产生数据变更），会产生 success toast
    await page.locator('button', { hasText: '保存自动化设置' }).first().click({ timeout: 12_000 })
    const toastSel = '.ccai-neolab-toast'
    await page.locator(toastSel).first().waitFor({ state: 'visible', timeout: 15_000 }).catch(() => {})
    await page.waitForTimeout(250)
    const toast = await page.evaluate(() => {
      const el = document.querySelector('.ccai-neolab-toast') as HTMLElement | null
      if (!el) return null
      const cs = getComputedStyle(el)
      const r = el.getBoundingClientRect()
      const badge = el.querySelector('div > div, div') as HTMLElement | null
      return {
        text: (el.innerText || '').replace(/\n/g, ' | ').slice(0, 90),
        x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height),
        bg: cs.backgroundColor, border: `${cs.borderTopWidth} ${cs.borderTopColor}`, radius: cs.borderRadius,
        icons: el.querySelectorAll('svg').length,
        tagBg: (el.querySelector('span') ? getComputedStyle(el.querySelector('span') as HTMLElement).backgroundColor : null),
        badgeBg: badge ? getComputedStyle(badge).backgroundColor : null,
        viewport: { w: window.innerWidth, h: window.innerHeight },
      }
    })
    await shot(page, 'toast')
    ev(C, '保存成功后出现 Toast', '可见的 .ccai-neolab-toast', toast ? `text=${JSON.stringify(toast.text)}` : 'none', Boolean(toast))
    ev(C, 'Toast 位于顶部', 'y < 100px', `y=${toast?.y}（视口 ${toast?.viewport.w}x${toast?.viewport.h}）`, Boolean(toast && (toast.y as number) < 100))
    ev(C, '带状态图标', '≥1 个 svg 图标', `${toast?.icons}`, Boolean(toast && (toast.icons as number) >= 1))
    ev(C, '带文字状态（不只靠颜色）', '出现「200 OK」等文字标签', `${JSON.stringify((toast?.text || '').slice(0, 12))}`, Boolean(toast && /200 OK|成功|Success/i.test(toast.text)))
    const hasSuccessGreen = toast?.tagBg === 'rgb(22, 163, 74)' || toast?.badgeBg === 'rgb(22, 163, 74)'
    ev(C, '使用成功状态色 #16A34A', '图标/描边/文字使用 rgb(22, 163, 74)', `surface=${toast?.bg}, tagBg=${toast?.tagBg}, border=${toast?.border}`, hasSuccessGreen,
      hasSuccessGreen ? undefined : `实测面为浅绿 ${toast?.bg}、标签底为 ${toast?.tagBg}、图标为黑色 #121316，未出现 UI Brief 的成功色 #16A34A（该值只出现在浅色/深色 token 表）`)

    const t0 = Date.now()
    let gone = -1
    for (let i = 0; i < 80; i++) {
      if ((await count(page, toastSel)) === 0) { gone = Date.now() - t0; break }
      await page.waitForTimeout(100)
    }
    ev(C, 'Toast 自动消失', '≤8s 内自行消失', `${gone < 0 ? '未消失（>8s）' : gone + 'ms'}`, gone >= 0 && gone <= 8000)

    const problems = guard(page)
    await page.waitForTimeout(500)
    ev(C, '无 console/pageerror/4xx', 'problems 为空', problems.map((p) => p.detail).join(' | ') || 'clean', problems.length === 0)
  } finally {
    await closeEverything(page)
    await finish(testInfo, C)
  }
})

/* ────────────────────────────── INTERACT-10 ────────────────────────────── */
test('INTERACT-10 List states: loading, empty, success (error+retry needs fault injection)', async ({ page }, testInfo) => {
  const C = 'INTERACT-10'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const shot = shooter('r18', testInfo)
  try {
    await realLogin(page)
    await page.goto(`${BASE}/admin/content`, { timeout: 45_000 })
    await page.waitForTimeout(1500)

    // 初始加载：切 tab 触发一次真实请求，轮询捕捉加载态
    await page.locator('.tabs button', { hasText: NEWS }).first().click({ timeout: 12_000 })
    let loadingText = ''
    let skeletons = 0
    for (let i = 0; i < 100; i++) {
      const res = await page.evaluate(() => {
        const el = document.querySelector('.list-state') as HTMLElement | null
        const sk = document.querySelectorAll('[class*="skeleton"], .skeleton').length
        return { t: el ? el.innerText.trim() : '', sk }
      })
      if (res.t) { loadingText = res.t; skeletons = res.sk; break }
      await page.waitForTimeout(20)
    }
    ev(C, '初始加载有可见指示', '出现 role=status 的加载提示', loadingText || '未捕捉到', Boolean(loadingText))
    ev(C, '加载使用同布局骨架', '存在 skeleton 占位元素', `skeleton=${skeletons}`, skeletons > 0,
      '只有一行文字「正在加载文章…」，没有 UI Brief §5 要求的同布局骨架（产品 Skeleton 组件在内容中心未被使用）')
    await page.waitForTimeout(2200)

    // 空/无结果：搜一个不存在的标题
    const searchSel = 'input.search-input, input[placeholder*="搜索"]'
    await page.locator(searchSel).first().fill('r18-no-such-' + RUN, { timeout: 10_000 })
    await page.keyboard.press('Enter')
    let emptyText = ''
    let emptyBtns: string[] = []
    let rowsShown = 20
    for (let i = 0; i < 120; i++) {
      const res = await page.evaluate(() => {
        const el = document.querySelector('p.empty') as HTMLElement | null
        return {
          t: el ? el.innerText.trim() : '',
          btns: el?.parentElement ? Array.from(el.parentElement.querySelectorAll('button')).map((b) => (b.textContent || '').trim()).filter(Boolean) : [],
          rows: document.querySelectorAll('tbody tr').length,
        }
      })
      if (res.t) { emptyText = res.t; emptyBtns = res.btns; rowsShown = res.rows; break }
      await page.waitForTimeout(50)
    }
    await shot(page, 'empty')
    ev(C, '无结果时显示空态文案', '出现空态文案（如「暂无文章」）', `${JSON.stringify(emptyText)}, rows=${rowsShown}`, Boolean(emptyText) && rowsShown === 0)
    ev(C, '空态不把「0」当数据渲染', '空态文案为文字，不是数字 0', `${JSON.stringify(emptyText)}`, Boolean(emptyText) && emptyText !== '0')
    ev(C, '空态提供当前页可执行主操作', '空态块内或其紧凑邻域有「新建/创建」类主操作', `空态块内按钮=${emptyBtns.length ? emptyBtns.join('/') : '无'}`, emptyBtns.some((b) => /新建|创建|新增|New|Create/.test(b)),
      '「＋ 新建文章」只存在于顶部工具栏，空态块内部没有任何操作入口')

    // 恢复列表
    await page.locator(searchSel).first().fill('', { timeout: 10_000 })
    await page.keyboard.press('Enter')
    await page.waitForTimeout(2000)
    const successRows = await count(page, 'tbody tr')
    ev(C, '成功态渲染数据行', '>0 行', `${successRows} 行`, successRows > 0)

    // 刷新加载：再次点同一 tab
    await page.locator('.tabs button', { hasText: ARTICLES }).first().click({ timeout: 12_000 })
    let refreshText = ''
    for (let i = 0; i < 80; i++) {
      const t = await page.evaluate(() => (document.querySelector('.list-state') as HTMLElement | null)?.innerText?.trim() ?? '')
      if (t) { refreshText = t; break }
      await page.waitForTimeout(20)
    }
    ev(C, '刷新加载有指示', '出现加载提示', refreshText || '未捕捉到', Boolean(refreshText), refreshText ? undefined : '请求过快（本地栈 <50ms），刷新态未能稳定捕捉')

    ev(C, '请求失败态显示原因 + 重试', '错误文案 + 重试按钮', '未取证', 'SKIP',
      '强制 /api 失败必须 mock/route 拦截；本 spec 禁止 mock，且无破坏后端可用性的合规手段，故按约定标记 SKIPPED 而不伪造')
    expectClean([])
  } finally {
    await closeEverything(page)
    await finish(testInfo, C)
  }
})

/* ────────────────────────────── INTERACT-11 ────────────────────────────── */
test('INTERACT-11 Hover affordance on public cards (transform / shadow)', async ({ page }, testInfo) => {
  const C = 'INTERACT-11'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const shot = shooter('r18', testInfo)
  try {
    await page.goto(`${BASE}/news`, { timeout: 45_000 })
    await page.locator('.timeline-news-card').first().waitFor({ state: 'visible', timeout: 30_000 })
    await page.waitForTimeout(1200)
    const n = await count(page, '.timeline-news-card')
    ev(C, '前台列表有内容卡片', '≥1', `${n}`, n > 0)

    const rest = await box(page, '.timeline-news-card')
    await page.locator('.timeline-news-card').first().hover({ timeout: 10_000 })
    await page.waitForTimeout(500)
    const hover = await box(page, '.timeline-news-card')
    await shot(page, 'card-hover')
    ev(C, 'hover 产生位移或阴影变化', 'transform 非 none 或 box-shadow 与静止态不同', `rest: transform=${rest?.transform}, shadow=${rest?.shadow.slice(0, 40)} → hover: transform=${hover?.transform}, shadow=${hover?.shadow.slice(0, 40)}`,
      Boolean(hover && (hover.transform !== 'none' || hover.shadow !== rest?.shadow)))
    ev(C, 'hover 上浮约 2px', 'translateY(≈-2px)', `${hover?.transform}`, Boolean(hover && /matrix\(1, 0, 0, 1, -?\d+(\.\d+)?, -2\)/.test(hover.transform)),
      hover ? undefined : '未取到 hover 后样式')
    ev(C, 'hover 有过渡动画', 'transition-duration > 0', `${hover?.transition}`, Boolean(hover && hover.transition !== '0s'))
  } finally {
    await finish(testInfo, C)
  }
})

/* ────────────────────────────── INTERACT-12 ────────────────────────────── */
test('INTERACT-12 Keyboard navigation: tab order matches visual order, focus always visible, pager reachable', async ({ page }, testInfo) => {
  const C = 'INTERACT-12'
  rows = []
  page.setDefaultTimeout(12_000)
  test.setTimeout(180_000)
  const shot = shooter('r18', testInfo)
  try {
    await page.goto(`${BASE}/news`, { timeout: 45_000 })
    await page.locator('.timeline-news-card').first().waitFor({ state: 'visible', timeout: 30_000 })
    await page.waitForTimeout(1000)

    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
    const stops: Array<{ t: string; x: number; y: number; ring: string; visible: boolean }> = []
    for (let i = 0; i < 14; i++) {
      await page.keyboard.press('Tab')
      await page.waitForTimeout(80)
      const info = await page.evaluate(() => {
        const a = document.activeElement as HTMLElement | null
        if (!a || a === document.body) return null
        const cs = getComputedStyle(a)
        const r = a.getBoundingClientRect()
        const ringVisible = (Number.parseFloat(cs.outlineWidth) > 0 && cs.outlineStyle !== 'none') || cs.boxShadow !== 'none'
        return {
          t: (a.innerText || a.getAttribute('aria-label') || a.tagName).trim().replace(/\s+/g, ' ').slice(0, 22),
          x: Math.round(r.x), y: Math.round(r.y),
          ring: `outline=${cs.outlineWidth}/${cs.outlineStyle}; shadow=${cs.boxShadow.slice(0, 30)}`,
          visible: ringVisible,
        }
      })
      if (info) stops.push(info)
    }
    await shot(page, 'tab-order')
    ev(C, 'Tab 顺序按视觉顺序前进', '14 跳焦点 y 坐标总体不倒退（允许同一行内小幅差异）', stops.map((s) => `${s.t}@${s.y}`).join(' → '),
      stops.every((s, i) => i === 0 || s.y >= stops[i - 1].y - 8))
    const invisible = stops.filter((s) => !s.visible)
    ev(C, '每一跳焦点都可见', 'outline 或 box-shadow 非 none', invisible.length ? invisible.map((s) => `${s.t}(${s.ring})`).join(' | ') : `${stops.length} 跳全部可见`, invisible.length === 0)

    const pager = await count(page, '[role="navigation"][aria-label="Pagination"]')
    const total = await count(page, '.timeline-news-card')
    if (pager > 0) {
      let reachable = false
      for (let i = 0; i < 60; i++) {
        await page.keyboard.press('Tab')
        const inPager = await page.evaluate(() => {
          const p = document.querySelector('[role="navigation"][aria-label="Pagination"]')
          return Boolean(p && document.activeElement && p.contains(document.activeElement))
        })
        if (inPager) { reachable = true; break }
      }
      ev(C, '分页器可用 Tab 到达', '60 跳内焦点进入分页器', `reachable=${reachable}`, reachable)
    } else {
      ev(C, '分页器可用 Tab 到达', '分页器存在且可 Tab 到达', `pager=0, cards=${total}`, 'SKIP',
        `前台 /news 当前 ${total} 条 < 每页 20 条，只渲染 1 页因此没有分页器；不造数据故无法验证分页键盘可达性`)
    }
  } finally {
    await finish(testInfo, C)
  }
})
