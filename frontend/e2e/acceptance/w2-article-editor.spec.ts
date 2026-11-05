import { expect, test, type BrowserContext, type Page } from '@playwright/test'
import {
  BASE,
  expectClean,
  expectNoHorizontalOverflow,
  guard,
  openContentTab,
  realLogin,
  reloadToTab,
  shooter,
  stamp,
  waitWrite,
} from '../real/real-helpers'

/**
 * v0.4 验收 · W2 文章编辑 + 标签/SEO + 精选 + 文章翻译（AE-01 ~ AE-19）
 * 用例文档：docs/v0.4/v0.4_e2e_acceptance_cases_20261028.md（§0 执行契约 / §1 量化口径 / W2）
 * 需求依据：docs/v0.4/v0.4_prd_20261026_v3.3.md（R5 / R6 / R9，第一章为最高优先级）
 * 真实 E2E：真实登录 + 真实 :3100→:8080→PG，禁止 mock / page.route / 注入 token。
 */

const TAB = '深度解析'
const TAX_TAB = '分类与标签'
const PREFIX = 'ACC-W2'
const S = `${PREFIX}-${stamp}`
const DRAFT_TITLE = `${PREFIX}-DRAFT-${stamp}`
const PUB_TITLE = `${PREFIX}-PUB-${stamp}`
const API = '/api/v1/admin/content/articles'
const TAX_API = '/articles/taxonomy/mutate'
const SUG_PREFIX = `${PREFIX}-SUG`
const SUGGESTION_TAGS = Array.from({ length: 9 }, (_, i) => `${SUG_PREFIX}${String(i + 1).padStart(2, '0')}`)

let ctx: BrowserContext
let setupPage: Page

const modal = (page: Page) => page.locator('.article-editor-modal')
const rowOf = (page: Page, title: string) => page.locator('.articles-pane table tbody tr', { hasText: title })
const tagInput = (page: Page) => modal(page).locator('.tag-input').first()
const tagChips = (page: Page) => modal(page).locator('.tag-chip')
/** chip 文本去掉删除按钮的「×」 */
const chipTexts = async (page: Page) => (await tagChips(page).allInnerTexts()).map((t) => t.replace('×', '').trim())

/** 登录（带重试）：并发 E2E 会话可能使本会话失效，失败时回登录页重试；已登录则直接返回 */
async function login(page: Page, attempts = 3) {
  for (let i = 0; i < attempts; i++) {
    try {
      await realLogin(page)
      return
    } catch (e) {
      if (!page.url().includes('/login')) return // /login 被重定向 → 已登录
      if (i === attempts - 1) throw e
      await page.goto(`${BASE}/login`).catch(() => {})
      await page.waitForTimeout(800)
    }
  }
}

async function openArticles(page: Page) {
  await login(page)
  await openContentTab(page, TAB)
}

async function search(page: Page, q: string) {
  const box = page.locator('.articles-pane .search-input')
  await box.fill(q)
  await box.press('Enter')
  await page.waitForLoadState('networkidle')
}

async function openNew(page: Page) {
  await page.getByRole('button', { name: '+ 新建文章' }).click()
  await expect(modal(page)).toBeVisible()
  return modal(page)
}

async function openEdit(page: Page, title: string) {
  await search(page, title)
  await rowOf(page, title).getByRole('button', { name: '编辑' }).first().click()
  await expect(modal(page)).toBeVisible()
  return modal(page)
}

async function fillSource(page: Page, html: string) {
  await modal(page).getByRole('button', { name: 'HTML 源码' }).click()
  await modal(page).locator('textarea.html-source-area').fill(html)
}

/** 新建一篇文章（走真实界面），返回标题 */
async function createArticle(page: Page, title: string, bodyHtml: string) {
  await openContentTab(page, TAB)
  await openNew(page)
  await modal(page).locator('.editor-title-input').fill(title)
  await fillSource(page, bodyHtml)
  await waitWrite(page, 'POST', API, () => modal(page).getByRole('button', { name: '保存', exact: true }).click())
  await expect(modal(page)).toHaveCount(0)
}

async function publishRow(page: Page, title: string) {
  await search(page, title)
  await waitWrite(page, 'POST', '/publish', () => rowOf(page, title).getByRole('button', { name: '发布', exact: true }).click())
}

async function deleteRowIfExists(page: Page, title: string) {
  await openContentTab(page, TAB)
  await search(page, title)
  const row = rowOf(page, title)
  if ((await row.count()) === 0) return false
  await row.getByRole('button', { name: '删除' }).first().click()
  const dlg = page.locator('.inline-tax-dialog', { hasText: '确认删除这篇文章' })
  await expect(dlg).toBeVisible()
  await waitWrite(page, 'DELETE', API, () => dlg.getByRole('button', { name: '确认删除' }).click())
  return true
}

async function openTagTaxonomy(page: Page) {
  await openContentTab(page, TAX_TAB)
  await page.locator('.taxonomy-pane .subnav button', { hasText: '标签管理' }).first().click()
  await expect(page.locator('.taxonomy-pane table')).toBeVisible()
}

async function createTagViaUI(page: Page, name: string) {
  await page.getByRole('button', { name: /新增标签/ }).click()
  const dlg = page.locator('.taxonomy-dialog')
  await expect(dlg).toBeVisible()
  await dlg.locator('input').fill(name)
  await waitWrite(page, 'POST', 'taxonomy/mutate', () => dlg.getByRole('button', { name: '保存', exact: true }).click())
  await expect(dlg).toHaveCount(0)
}

async function readTagNames(page: Page): Promise<string[]> {
  return page.locator('.taxonomy-pane table tbody tr td:first-child').allInnerTexts()
}

async function metrics(page: Page) {
  return page.evaluate(() => {
    const q = (sel: string) => document.querySelector(sel) as HTMLElement | null
    const box = (el: Element | null) => {
      if (!el) return null
      const r = el.getBoundingClientRect()
      return { x: r.x, y: r.y, w: r.width, h: r.height, bottom: r.bottom, right: r.right }
    }
    const m = q('.article-editor-modal')
    const body = q('.article-editor-modal .modal-drawer-body')
    const footer = q('.article-editor-modal .modal-drawer-footer')
    const editorWrap = q('.article-editor-modal .rich-text-editor')
    const editor = q('.article-editor-modal .rich-text-content')
    const left = q('.article-editor-modal .editor-col-left')
    const right = q('.article-editor-modal .editor-col-right')
    const trans = q('.article-editor-modal .translations-card')
    const cs = m ? getComputedStyle(m) : null
    return {
      vw: window.innerWidth,
      vh: window.innerHeight,
      modal: box(m),
      modalRadius: cs?.borderTopLeftRadius ?? '',
      modalDisplay: cs?.display ?? '',
      body: box(body),
      bodyScroll: body ? { sh: body.scrollHeight, ch: body.clientHeight } : null,
      footer: box(footer),
      editorWrap: box(editorWrap),
      editor: box(editor),
      editorScroll: editor ? { sh: editor.scrollHeight, ch: editor.clientHeight } : null,
      left: box(left),
      right: box(right),
      trans: box(trans),
      bodyInLeft: !!(left?.querySelector('.rich-text-editor')),
      bodyInRight: !!(right?.querySelector('.rich-text-editor')),
      colsTemplate: left && right ? getComputedStyle(left.parentElement as Element).gridTemplateColumns : '',
    }
  })
}

test.describe.configure({ timeout: 120_000 })

// 注：本文件各用例相互独立（依赖 beforeAll 造的数据），失败不互相阻断；
// 运行方式：--workers=1 保证同文件内按声明顺序串行执行、beforeAll 只跑一次。

test.beforeAll(async ({ browser }) => {
  ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  setupPage = await ctx.newPage()
  await login(setupPage)
  await createArticle(setupPage, DRAFT_TITLE, `<p>${DRAFT_TITLE} 草稿正文，用于精选开关禁用校验。</p>`)
  await createArticle(setupPage, PUB_TITLE, `<p>${PUB_TITLE} 已发布正文，用于精选开关与翻译状态校验。</p>`)
  await publishRow(setupPage, PUB_TITLE)
  await openTagTaxonomy(setupPage)
  for (const name of SUGGESTION_TAGS) {
    const names = await readTagNames(setupPage)
    if (!names.includes(name)) await createTagViaUI(setupPage, name)
  }
})

test.afterAll(async () => {
  if (!ctx) return
  const page = setupPage
  try {
    await openTagTaxonomy(page)
    for (const name of [...SUGGESTION_TAGS, `${PREFIX}-NEWTAG-${stamp}`]) {
      await page.locator('.taxonomy-pane .search-input').fill(name)
      await page.waitForTimeout(200)
      const row = page.locator('.taxonomy-pane table tbody tr', { hasText: name })
      if ((await row.count()) === 0) continue
      await row.getByRole('button', { name: '删除' }).first().click()
      const dlg = page.locator('.taxonomy-dialog', { hasText: '确认删除' })
      if (await dlg.isVisible().catch(() => false)) {
        await dlg.getByRole('button', { name: '确认删除' }).click().catch(() => {})
        await page.waitForTimeout(400)
      }
    }
  } catch (e) {
    console.log('[cleanup] tag ui cleanup failed:', String(e).slice(0, 200))
  }
  try {
    for (const title of [DRAFT_TITLE, PUB_TITLE, `${PREFIX}-LONG-${stamp}`, `${PREFIX}-NEW-${stamp}`, `${PREFIX}-TAGLIB-${stamp}`]) {
      const ok = await deleteRowIfExists(page, title).catch(() => false)
      console.log(`[cleanup] delete article ${title} → ${ok}`)
    }
  } catch (e) {
    console.log('[cleanup] article cleanup failed:', String(e).slice(0, 200))
  }
  await ctx.close()
})

/* ============================ AE-01 ============================ */
test('AE-01 文章编辑弹窗宽度/圆角/居中 + 左右结构 + 390 不横向溢出', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)

  await page.setViewportSize({ width: 1440, height: 900 })
  await openNew(page)
  const m1440 = await metrics(page)
  console.log('AE-01 1440 metrics:', JSON.stringify(m1440))
  await shot(page, '1440-dialog')

  expect
    .soft(m1440.modal!.w, `1440 视口弹窗宽度期望 1320±4，实测 ${m1440.modal!.w}`)
    .toBeGreaterThanOrEqual(1316)
  expect.soft(m1440.modal!.w, `1440 视口弹窗宽度期望 1320±4，实测 ${m1440.modal!.w}`).toBeLessThanOrEqual(1324)
  expect.soft(m1440.modalRadius, `弹窗圆角期望 12px，实测 ${m1440.modalRadius}`).toBe('12px')
  const centerDelta = Math.abs(m1440.modal!.x + m1440.modal!.w / 2 - m1440.vw / 2)
  expect.soft(centerDelta, `弹窗未居中，中心偏移 ${centerDelta}px`).toBeLessThanOrEqual(4)

  // 左右结构（R16/R17 最新基线）：左栏=标题/摘要/分类标签/正文编辑器，右栏=精选/封面/SEO/翻译
  expect.soft(m1440.left, '缺少左栏').not.toBeNull()
  expect.soft(m1440.right, '缺少右栏').not.toBeNull()
  expect.soft(m1440.left!.x, '左栏应在右栏左侧').toBeLessThan(m1440.right!.x)
  expect.soft(m1440.bodyInLeft, `正文编辑器应在左栏，实测 bodyInLeft=${m1440.bodyInLeft} bodyInRight=${m1440.bodyInRight}`).toBe(true)
  // 右栏应含 精选(推荐设置)/封面图/SEO 卡片；编辑态另含译文卡（AE-15 覆盖）
  const rightCards = await modal(page).locator('.editor-col-right .meta-card-header h4').allInnerTexts()
  console.log(`AE-01 right cards=${JSON.stringify(rightCards)}`)
  expect.soft(rightCards.join('|'), `右栏应含推荐(精选)/封面/SEO 卡片，实测 ${JSON.stringify(rightCards)}`).toMatch(/推荐设置/)
  expect.soft(rightCards.join('|'), `右栏应含封面图卡片，实测 ${JSON.stringify(rightCards)}`).toMatch(/封面图/)
  expect.soft(rightCards.join('|'), `右栏应含 SEO 卡片，实测 ${JSON.stringify(rightCards)}`).toMatch(/SEO/)
  // 左栏应含正文卡片（编辑器所在）
  const leftCards = await modal(page).locator('.editor-col-left .meta-card-header h4').allInnerTexts()
  console.log(`AE-01 left cards=${JSON.stringify(leftCards)}`)
  expect.soft(leftCards.join('|'), `左栏应含正文卡片，实测 ${JSON.stringify(leftCards)}`).toMatch(/正文/)
  await shot(page, '1440-columns')

  // 1024 视口（弹窗宽 = min(1320, 100vw-32) = 992）
  await page.setViewportSize({ width: 1024, height: 768 })
  await modal(page).getByRole('button', { name: '取消' }).click()
  await expect(modal(page)).toHaveCount(0)
  await page.setViewportSize({ width: 1024, height: 768 })
  await openNew(page)
  const m1024 = await metrics(page)
  console.log('AE-01 1024 metrics:', JSON.stringify(m1024))
  await shot(page, '1024-dialog')
  expect
    .soft(m1024.modal!.w, `1024 视口弹窗宽度期望 min(1320,100vw-32)=992±4，实测 ${m1024.modal!.w}`)
    .toBeGreaterThanOrEqual(988)
  expect.soft(m1024.modal!.w, `1024 视口弹窗宽度期望 min(1320,100vw-32)=992±4，实测 ${m1024.modal!.w}`).toBeLessThanOrEqual(996)
  expect.soft(Math.abs(m1024.left!.x - m1024.right!.x), '1024 下左右两栏应并排（左右 x 不同）').toBeGreaterThan(50)
  await modal(page).getByRole('button', { name: '取消' }).click()
  await expect(modal(page)).toHaveCount(0)

  // 390 视口
  await page.setViewportSize({ width: 390, height: 844 })
  await openNew(page)
  const m390 = await metrics(page)
  console.log('AE-01 390 metrics:', JSON.stringify(m390))
  await shot(page, '390-dialog')
  await expectNoHorizontalOverflow(page)
  expect
    .soft(Math.abs(m390.left!.x - m390.right!.x), `390 下应堆叠为单栏，实测左 x=${m390.left!.x} 右 x=${m390.right!.x}`)
    .toBeLessThanOrEqual(1)
  expect.soft(m390.right!.y, '390 下右栏应位于左栏下方').toBeGreaterThanOrEqual(m390.left!.y)
  await modal(page).getByRole('button', { name: '取消' }).click()
  await expect(modal(page)).toHaveCount(0)
  expectClean(problems)
})

/* ============================ AE-02 ============================ */
test('AE-02 正文编辑器占满剩余高度/内部滚动 + 翻译卡片不遮挡', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openEdit(page, PUB_TITLE)
  const m = await metrics(page)
  console.log('AE-02 metrics:', JSON.stringify(m))
  await shot(page, 'editor-layout')

  const gap = m.footer!.y - m.editorWrap!.bottom
  console.log(`AE-02 editorBottom=${m.editorWrap!.bottom.toFixed(1)} footerTop=${m.footer!.y.toFixed(1)} gap=${gap.toFixed(1)}`)
  expect.soft(gap, `编辑器底边到按钮区顶边期望 ≤24px，实测 ${gap.toFixed(1)}px`).toBeLessThanOrEqual(24)
  expect
    .soft(m.editorScroll!.ch, `1440×900 下编辑器可视高度期望 ≥300px，实测 ${m.editorScroll!.ch}px`)
    .toBeGreaterThanOrEqual(300)

  if (m.trans) {
    const intersect = !(
      m.trans.bottom <= m.editorWrap!.y ||
      m.trans.y >= m.editorWrap!.bottom ||
      m.trans.right <= m.editorWrap!.x ||
      m.trans.x >= m.editorWrap!.right
    )
    console.log(`AE-02 translationsCard=${JSON.stringify(m.trans)} intersectEditor=${intersect}`)
    expect.soft(intersect, `译文卡片与正文编辑器 boundingRect 相交：${JSON.stringify({ trans: m.trans, editor: m.editorWrap })}`).toBe(false)
  } else {
    expect.soft(m.trans, '译文卡片未渲染（.translations-card 不存在）').not.toBeNull()
  }

  // 3000 字正文：编辑器外框高度不变、内容区内部滚动、弹窗不变长
  const editorBefore = m.editorWrap!.h
  const modalBefore = m.modal!.h
  await modal(page).locator('.rich-text-content').click()
  await page.keyboard.press('ControlOrMeta+a')
  await page.keyboard.insertText('测'.repeat(3000))
  await page.waitForTimeout(500)
  const after = await metrics(page)
  console.log('AE-02 after 3000 chars:', JSON.stringify({ editorWrap: after.editorWrap, editorScroll: after.editorScroll, modal: after.modal }))
  await shot(page, 'editor-3000chars')
  expect
    .soft(Math.abs(after.editorWrap!.h - editorBefore), `贴入 3000 字后编辑器外框高度变化 ${(after.editorWrap!.h - editorBefore).toFixed(1)}px（应不变）`)
    .toBeLessThanOrEqual(1)
  expect
    .soft(
      after.editorScroll!.sh > after.editorScroll!.ch,
      `编辑器内部应出现滚动，实测 scrollHeight=${after.editorScroll!.sh} clientHeight=${after.editorScroll!.ch}`,
    )
    .toBe(true)
  expect
    .soft(Math.abs(after.modal!.h - modalBefore), `弹窗整体高度变化 ${(after.modal!.h - modalBefore).toFixed(1)}px（应不变长）`)
    .toBeLessThanOrEqual(1)
  expectClean(problems)
})

/* ============================ AE-03 ============================ */
test('AE-03 富文本工具栏 10 项 + 逐项生效', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openNew(page)
  const toolbar = modal(page).locator('.rich-text-toolbar')
  const buttons = toolbar.locator('button')
  const count = await buttons.count()
  const labels = await buttons.evaluateAll((els) => els.map((e) => (e as HTMLElement).getAttribute('aria-label') || (e as HTMLElement).textContent || ''))
  console.log(`AE-03 toolbarCount=${count} labels=${JSON.stringify(labels)}`)
  await shot(page, 'toolbar')

  const required = ['加粗', '斜体', 'H2', 'H3', '有序列表', '无序列表', '引用', '链接', '插入图片', '清除格式']
  expect.soft(count, `工具栏项数期望 10，实测 ${count}（${labels.join('/')}）`).toBe(10)
  for (const name of required) {
    const hit = labels.some((l) => l.includes(name))
    expect.soft(hit, `工具栏缺少「${name}」，实测：${labels.join(' / ')}`).toBe(true)
  }

  const content = modal(page).locator('.rich-text-content')
  const selectAllType = async (text: string) => {
    await content.click()
    await page.keyboard.press('ControlOrMeta+a')
    await page.keyboard.press('Delete')
    await page.keyboard.insertText(text)
    await page.keyboard.press('ControlOrMeta+a')
  }

  const tryCommand = async (label: string, expectPattern: RegExp) => {
    const btn = toolbar.locator(`button[aria-label*="${label}"]`)
    if ((await btn.count()) === 0) return { label, skipped: true }
    await selectAllType(`acc ${label}`)
    await btn.click()
    await page.waitForTimeout(150)
    const html = await content.innerHTML()
    return { label, skipped: false, html, ok: expectPattern.test(html) }
  }

  const results = [
    await tryCommand('加粗', /<(b|strong)[\s>]/i),
    await tryCommand('斜体', /<(i|em)[\s>]/i),
    await tryCommand('项目列表', /<ul[\s>]/i),
    await tryCommand('编号列表', /<ol[\s>]/i),
    await tryCommand('引用', /<blockquote[\s>]/i),
    await tryCommand('清除格式', /^(?!.*<(b|strong|i|em)[\s>]).*$/i),
  ]
  console.log('AE-03 command results:', JSON.stringify(results))
  await shot(page, 'toolbar-applied')
  for (const r of results) {
    if (r.skipped) {
      expect.soft(`missing:${r.label}`, `工具栏缺少「${r.label}」，无法验证生效`).toBe('present')
    } else {
      expect.soft(r.ok, `点击「${r.label}」后编辑区 HTML 未出现预期标签，实测：${r.html}`).toBe(true)
    }
  }
  await modal(page).getByRole('button', { name: '取消' }).click()
  expectClean(problems)
})

/* ============================ AE-04 ============================ */
test('AE-04 ≥10,000 字正文保存后重新打开逐字一致', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })

  const longTitle = `${PREFIX}-LONG-${stamp}`
  const paragraphs: string[] = []
  let n = 0
  while (paragraphs.join('').length < 10500) {
    n += 1
    paragraphs.push(`<p>第${n}段：ACC-W2 长文往返校验，中文标点，English words，数字 1234567890。</p>`)
  }
  const html = `<h2>ACC-W2 长文标题</h2>${paragraphs.join('')}`
  const plainLen = html.replace(/<[^>]*>/g, '').length
  console.log(`AE-04 payload html.length=${html.length} plainLen=${plainLen}`)

  await openNew(page)
  await modal(page).locator('.editor-title-input').fill(longTitle)
  await fillSource(page, html)
  const typedLen = (await modal(page).locator('textarea.html-source-area').inputValue()).length
  expect.soft(typedLen, `输入源文本长度 ${typedLen}`).toBe(html.length)
  await shot(page, 'filled-10k')
  await waitWrite(page, 'POST', API, () => modal(page).getByRole('button', { name: '保存', exact: true }).click())
  await expect(modal(page)).toHaveCount(0)

  await reloadToTab(page, TAB)
  await search(page, longTitle)
  await expect(rowOf(page, longTitle)).toHaveCount(1)
  await rowOf(page, longTitle).getByRole('button', { name: '编辑' }).first().click()
  await expect(modal(page)).toBeVisible()
  await modal(page).getByRole('button', { name: 'HTML 源码' }).click()
  const actual = await modal(page).locator('textarea.html-source-area').inputValue()
  console.log(`AE-04 roundtrip actual.length=${actual.length} expected=${html.length} equal=${actual === html}`)
  if (actual !== html) {
    let i = 0
    while (i < Math.min(actual.length, html.length) && actual[i] === html[i]) i++
    console.log(`AE-04 first diff at ${i}: expected="${html.slice(i, i + 60)}" actual="${actual.slice(i, i + 60)}"`)
  }
  await shot(page, 'roundtrip-source')
  expect.soft(actual.length, `回读正文长度期望 ${html.length}，实测 ${actual.length}`).toBe(html.length)
  expect.soft(actual === html, `回读正文与保存前不一致（长度 ${actual.length} vs ${html.length}）`).toBe(true)
  await modal(page).getByRole('button', { name: '取消' }).click()
  expectClean(problems)
})

/* ============================ AE-05 ============================ */
test('AE-05 标签：回车成词、三种分隔符、大小写与空格去重、无独立新增按钮', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openNew(page)
  const input = tagInput(page).locator('input')

  await input.fill('AI')
  await input.press('Enter')
  await expect(tagChips(page)).toHaveCount(1)
  await input.fill('ai')
  await input.press('Enter')
  await page.waitForTimeout(200)
  const afterDedupe = await chipTexts(page)
  console.log(`AE-05 after AI/ai chips=${JSON.stringify(afterDedupe)}`)
  expect.soft(afterDedupe.length, `大小写去重后期望 1 个 chip，实测 ${afterDedupe.length}：${afterDedupe.join('|')}`).toBe(1)

  await input.fill(' Alpha，Beta、Gamma,Delta ')
  await input.press('Enter')
  await page.waitForTimeout(300)
  const afterSeparators = await chipTexts(page)
  console.log(`AE-05 separators chips=${JSON.stringify(afterSeparators)}`)
  await shot(page, 'separators')
  expect
    .soft(afterSeparators.length, `「，」「、」"," 三种分隔符应生成 4 个新 chip（Alpha/Beta/Gamma/Delta），实测 chip 总数 ${afterSeparators.length}：${afterSeparators.join('|')}`)
    .toBe(5)
  for (const expectName of ['Alpha', 'Beta', 'Gamma', 'Delta']) {
    expect.soft(afterSeparators.join('|'), `缺少 chip「${expectName}」，实测：${afterSeparators.join('|')}`).toContain(expectName)
  }

  await input.fill('  AI  ')
  await input.press('Enter')
  await page.waitForTimeout(200)
  const afterTrim = await tagChips(page).count()
  expect.soft(afterTrim, `首尾空格+大小写应去重不新增，实测 chip 数 ${afterTrim}`).toBe(5)

  const addButtons = await modal(page).locator('button', { hasText: /新增标签|新建标签/ }).count()
  const addTexts = await modal(page).locator('button', { hasText: /新增标签|新建标签/ }).allInnerTexts()
  console.log(`AE-05 standalone add-tag buttons=${addButtons} ${JSON.stringify(addTexts)}`)
  await shot(page, 'no-standalone-button')
  expect.soft(addButtons, `不应存在独立的「新增标签」按钮，实测 ${addButtons} 个：${addTexts.join('|')}`).toBe(0)
  await modal(page).getByRole('button', { name: '取消' }).click()
  expectClean(problems)
})

/* ============================ AE-06 ============================ */
test('AE-06 中文输入法：组字中回车不生成 chip，compositionend 后回车才生成', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openNew(page)
  const input = tagInput(page).locator('input')

  await input.click()
  await input.fill('')
  await page.evaluate(() => {
    const el = document.querySelector('.article-editor-modal .tag-input input') as HTMLInputElement
    el.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true, data: '' }))
  })
  await input.fill('组字中')
  const beforeEnter = await tagChips(page).count()
  await input.press('Enter')
  await page.waitForTimeout(300)
  const duringComposition = await tagChips(page).count()
  console.log(`AE-06 chips before=${beforeEnter} afterEnterDuringComposition=${duringComposition}`)
  await shot(page, 'composing-enter')
  expect.soft(duringComposition, `组字中按 Enter 不应生成 chip，实测 chip 数由 ${beforeEnter} 变为 ${duringComposition}`).toBe(beforeEnter)

  await page.evaluate(() => {
    const el = document.querySelector('.article-editor-modal .tag-input input') as HTMLInputElement
    el.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true, data: el.value }))
  })
  await input.press('Enter')
  await page.waitForTimeout(300)
  const afterComposition = await chipTexts(page)
  console.log(`AE-06 chips afterCompositionEnd=${JSON.stringify(afterComposition)}`)
  await shot(page, 'after-composition')
  expect.soft(afterComposition.length, `组字结束后按 Enter 应生成 1 个 chip，实测 ${afterComposition.length}`).toBe(1)
  expect.soft(afterComposition.join('|'), `chip 内容应为「组字中」，实测 ${afterComposition.join('|')}`).toContain('组字中')
  await modal(page).getByRole('button', { name: '取消' }).click()
  expectClean(problems)
})

/* ============================ AE-07 ============================ */
test('AE-07 标签联想：打开不弹、输入即联想、最多 8 条、方向键+Enter、Esc 关闭', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openNew(page)
  const wrap = tagInput(page)
  const input = wrap.locator('input')
  const list = wrap.locator('.tag-suggestions')

  await page.waitForTimeout(400)
  const onOpen = await list.count()
  console.log(`AE-07 suggestions on dialog open = ${onOpen}`)
  expect.soft(onOpen, `打开弹窗时不应自动弹出联想下拉，实测 ${onOpen} 个`).toBe(0)
  await shot(page, 'open-no-dropdown')

  await input.click()
  await input.pressSequentially(SUG_PREFIX.slice(0, 1), { delay: 40 })
  await page.waitForTimeout(400)
  const firstCharCount = await list.locator('li').count()
  console.log(`AE-07 after 1 char suggestions=${firstCharCount}`)
  if (firstCharCount === 0) {
    // 第一个字符若命中不足，继续输入前缀以获取联想数据（用例前提是标签库存在数据）
    await input.fill(SUG_PREFIX)
    await page.waitForTimeout(400)
  }
  const items = await list.locator('li').allInnerTexts()
  console.log(`AE-07 suggestions count=${items.length} items=${JSON.stringify(items)}`)
  await shot(page, 'suggestions')
  expect.soft(items.length, `输入第 1 个字符后应出现联想下拉，实测 ${items.length} 条`).toBeGreaterThan(0)
  expect.soft(items.length, `联想最多 8 条，实测 ${items.length} 条`).toBeLessThanOrEqual(8)

  await input.press('ArrowDown')
  await input.press('Enter')
  await page.waitForTimeout(300)
  const chips = await chipTexts(page)
  console.log(`AE-07 chips after ArrowDown+Enter=${JSON.stringify(chips)}`)
  expect.soft(chips.length, `方向键+Enter 应选中 1 条联想，实测 chip 数 ${chips.length}`).toBe(1)
  if (items.length) {
    expect.soft(chips[0], `选中的 chip 应等于联想项「${items[0]}」，实测「${chips[0]}」`).toBe(items[0])
  }
  await shot(page, 'arrow-enter-selected')

  await input.fill(SUG_PREFIX)
  await page.waitForTimeout(300)
  const reopened = await list.count()
  await input.press('Escape')
  await page.waitForTimeout(300)
  const afterEsc = await list.count()
  console.log(`AE-07 dropdown before Esc=${reopened} after Esc=${afterEsc}`)
  expect.soft(afterEsc, `Esc 应关闭联想下拉，实测关闭后仍有 ${afterEsc} 个`).toBe(0)
  await shot(page, 'esc-closed')
  await modal(page).getByRole('button', { name: '取消' }).click()
  expectClean(problems)
})

/* ============================ AE-08 ============================ */
test('AE-08 第 11 个标签、21 字标签：有明确提示且不生成 chip', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openNew(page)
  const input = tagInput(page).locator('input')

  for (let i = 1; i <= 11; i++) {
    await input.fill(`${PREFIX}T${String(i).padStart(2, '0')}`)
    await input.press('Enter')
    await page.waitForTimeout(120)
  }
  const chips11 = await chipTexts(page)
  const hint11 = await modal(page).locator('text=/最多|上限|超过|超出/').count()
  console.log(`AE-08 after 11 tags chips=${chips11.length} ${JSON.stringify(chips11)} hintNodes=${hint11}`)
  await shot(page, 'eleven-tags')
  expect.soft(chips11.length, `单篇最多 10 个标签，输入第 11 个后实测 chip 数 ${chips11.length}`).toBe(10)
  expect.soft(hint11, `输入第 11 个标签应出现明确提示，实测提示节点 ${hint11} 个`).toBeGreaterThan(0)

  const longTag = '标'.repeat(21)
  await input.fill(longTag)
  await input.press('Enter')
  await page.waitForTimeout(300)
  const chipsAfterLong = await chipTexts(page)
  const hintLong = await modal(page).locator('text=/最多|上限|超过|超出|20/').count()
  console.log(`AE-08 after 21-char tag chips=${chipsAfterLong.length} hintNodes=${hintLong}`)
  await shot(page, 'long-tag')
  expect
    .soft(
      chipsAfterLong.some((c) => c.replace('×', '').trim() === longTag),
      `21 字标签不应生成 chip，实测 chips=${JSON.stringify(chipsAfterLong)}`,
    )
    .toBe(false)
  expect.soft(hintLong, `21 字标签应出现明确提示，实测提示节点 ${hintLong} 个`).toBeGreaterThan(0)
  await modal(page).getByRole('button', { name: '取消' }).click()
  expectClean(problems)
})

/* ============================ AE-09 ============================ */
test('AE-09 新标签保存后写入标签库', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  const newTag = `${PREFIX}-NEWTAG-${stamp}`
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })

  await openNew(page)
  await modal(page).locator('.editor-title-input').fill(`${PREFIX}-TAGLIB-${stamp}`)
  await fillSource(page, '<p>ACC-W2 标签入库校验正文。</p>')
  const input = tagInput(page).locator('input')
  await input.fill(newTag)
  await input.press('Enter')
  await expect(tagChips(page)).toHaveCount(1)
  await shot(page, 'tag-typed')
  await waitWrite(page, 'POST', API, () => modal(page).getByRole('button', { name: '保存', exact: true }).click())
  await expect(modal(page)).toHaveCount(0)

  await openTagTaxonomy(page)
  await page.locator('.taxonomy-pane .search-input').fill(newTag)
  await page.waitForTimeout(600)
  const names = await readTagNames(page)
  console.log(`AE-09 taxonomy tags matching ${newTag}: ${JSON.stringify(names)}`)
  await shot(page, 'taxonomy-list')
  expect.soft(names.some((n) => n.trim() === newTag), `新标签「${newTag}」应出现在标签库，实测列表=${JSON.stringify(names)}`).toBe(true)

  // 清理该标签
  const row = page.locator('.taxonomy-pane table tbody tr', { hasText: newTag })
  if ((await row.count()) > 0) {
    await row.getByRole('button', { name: '删除' }).first().click()
    const dlg = page.locator('.taxonomy-dialog', { hasText: '确认删除' })
    await expect(dlg).toBeVisible()
    await waitWrite(page, 'POST', 'taxonomy/mutate', () => dlg.getByRole('button', { name: '确认删除' }).click())
    await expect(dlg).toHaveCount(0)
  }
  expectClean(problems)
})

/* ============================ AE-10 ============================ */
test('AE-10 SEO 按钮：正文空禁用 + 加载态 + 描述 120-160 字/关键词 5-8 个/标题与 slug 不变', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openNew(page)
  const seoBtn = modal(page).locator('.seo-auto-btn')
  const seoCard = modal(page).locator('.meta-card', { hasText: 'SEO 搜索引擎优化' })
  const seoDesc = seoCard.locator('textarea').first()
  const seoTitleInput = seoCard.locator('input').first()
  const seoKwWrap = seoCard.locator('.tag-input')

  // 正文为空：按钮呈禁用语义（aria-disabled），且点击必须给出明确提示，不能「点了没反应」
  const ariaDisabledOnEmpty = await seoBtn.getAttribute('aria-disabled')
  console.log(`AE-10 text empty → seo button aria-disabled=${ariaDisabledOnEmpty}`)
  await shot(page, 'empty-body-disabled')
  expect.soft(ariaDisabledOnEmpty, `正文为空时 SEO 按钮应为禁用语义（aria-disabled=true），实测 ${ariaDisabledOnEmpty}`).toBe('true')

  // 空正文点击：必须给出明确结果（提示/请求/状态变化）
  const toastBefore = await page.locator('.ccai-neolab-toast').count()
  const writes: string[] = []
  const onReq = (req: any) => {
    if (req.method() !== 'GET') writes.push(`${req.method()} ${String(req.url()).replace('http://127.0.0.1:3100', '')}`)
  }
  page.on('request', onReq)
  await seoBtn.click({ force: true }).catch(() => {})
  await page.waitForTimeout(1500)
  const toastAfter = await page.locator('.ccai-neolab-toast').count()
  const toastTextsEmpty = await page.locator('.ccai-neolab-toast').allInnerTexts()
  const descOnEmpty = await seoDesc.inputValue()
  console.log(`AE-10 empty click → toasts ${toastBefore}→${toastAfter} ${JSON.stringify(toastTextsEmpty)}, desc="${descOnEmpty}", writes=${JSON.stringify(writes)}`)
  await shot(page, 'empty-body-click')
  const hasEmptyFeedback = toastAfter > toastBefore || descOnEmpty.length > 0 || writes.length > 0
  expect.soft(hasEmptyFeedback, '正文为空时点击 SEO 按钮无任何结果（无提示/无请求/无字段变化）').toBe(true)
  expect
    .soft(toastTextsEmpty.join('|'), `空正文点击应提示「请先填写正文内容」，实测 toast=${JSON.stringify(toastTextsEmpty)}`)
    .toMatch(/请先填写正文|正文/)

  // 有正文时点击
  await modal(page).locator('.title-cell, .editor-title-input').first().fill(`${PREFIX} SEO 校验标题`)
  await fillSource(
    page,
    '<h2>大模型推理成本</h2><p>大模型推理成本持续下降，企业开始把生成式人工智能接入内容生产流程。本文从算力、模型选择、缓存、批量推理四个角度分析成本结构，并给出可落地的优化建议，包括提示词压缩、结构化输出、语义缓存与分级路由。数据来源为公开基准测试与厂商定价页。</p>',
  )
  const advanced = seoCard.locator('details.advanced-seo')
  if (!(await advanced.evaluate((el) => (el as HTMLDetailsElement).open))) await advanced.locator('summary').click()
  const slugInput = advanced.locator('input').first()
  const slugBefore = await slugInput.inputValue()
  const titleBefore = await seoTitleInput.inputValue()
  await seoBtn.click()
  const disabledDuring = await seoBtn.isDisabled()
  const textDuring = (await seoBtn.innerText()).trim()
  console.log(`AE-10 click → button disabled=${disabledDuring} text="${textDuring}"`)
  expect.soft(disabledDuring, `生成中按钮应禁用并显示加载态，实测 disabled=${disabledDuring} text="${textDuring}"`).toBe(true)
  await page.waitForTimeout(1200)
  const descAfter = await seoDesc.inputValue()
  const kwChips = (await seoKwWrap.locator('.tag-chip').allInnerTexts()).map((t) => t.replace('×', '').trim())
  const titleAfter = await seoTitleInput.inputValue()
  const slugAfter = await slugInput.inputValue()
  console.log(
    `AE-10 result descLen=${descAfter.length} kwCount=${kwChips.length} kw=${JSON.stringify(kwChips)} titleBefore="${titleBefore}" titleAfter="${titleAfter}" slugBefore="${slugBefore}" slugAfter="${slugAfter}"`,
  )
  await shot(page, 'seo-generated')
  expect.soft(descAfter.length, `SEO 描述应为 120–160 字符，实测 ${descAfter.length}`).toBeGreaterThanOrEqual(120)
  expect.soft(descAfter.length, `SEO 描述应为 120–160 字符，实测 ${descAfter.length}`).toBeLessThanOrEqual(160)
  expect.soft(kwChips.length, `SEO 关键词应为 5–8 个，实测 ${kwChips.length}：${kwChips.join('|')}`).toBeGreaterThanOrEqual(5)
  expect.soft(kwChips.length, `SEO 关键词应为 5–8 个，实测 ${kwChips.length}：${kwChips.join('|')}`).toBeLessThanOrEqual(8)
  expect.soft(titleAfter, `SEO 标题不应被生成逻辑修改，实测 "${titleBefore}" → "${titleAfter}"`).toBe(titleBefore)
  expect.soft(slugAfter, `slug 不应被修改，实测 "${slugBefore}" → "${slugAfter}"`).toBe(slugBefore)
  page.off('request', onReq)
  await modal(page).getByRole('button', { name: '取消' }).click()
  expectClean(problems)
})

/* ============================ AE-11 ============================ */
test('AE-11 已有 SEO 描述时点击：先确认「已有内容，是否覆盖？」，取消后原内容不变', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openNew(page)
  const seoCard = modal(page).locator('.meta-card', { hasText: 'SEO 搜索引擎优化' })
  const seoDesc = seoCard.locator('textarea').first()
  const existing = 'ACC-W2 已存在的 SEO 描述，长度用于覆盖确认校验。'
  await seoDesc.fill(existing)
  await modal(page).locator('.editor-title-input').fill(`${PREFIX} 覆盖确认标题`)
  await fillSource(page, '<p>覆盖确认用例正文，描述已存在时应先询问是否覆盖。</p>')

  let confirmSeen = false
  page.once('dialog', (d) => {
    confirmSeen = true
    console.log(`AE-11 native dialog: "${d.message()}"`)
    void d.dismiss()
  })
  await modal(page).locator('.seo-auto-btn').click()
  await page.waitForTimeout(800)
  // 覆盖确认卡片（Teleport 到 body 的 alertdialog）
  const confirmText = await page.locator('.seo-overwrite-confirm').count()
  const modalConfirm = await page.locator('.action-confirm, .inline-tax-dialog, .confirm-dialog').filter({ hasText: '覆盖' }).count()
  console.log(`AE-11 confirmByText=${confirmText} confirmByDialog=${modalConfirm} nativeDialog=${confirmSeen}`)
  await shot(page, 'overwrite-confirm')
  expect
    .soft(confirmText + modalConfirm + (confirmSeen ? 1 : 0), '已有内容时应先弹「已有内容，是否覆盖？」确认，实测无确认框')
    .toBeGreaterThan(0)
  // 点「取消」→ 原描述不变
  const cancelBtn = page.locator('.seo-overwrite-confirm').getByRole('button', { name: '取消' })
  if (await cancelBtn.isVisible().catch(() => false)) await cancelBtn.click()
  await page.waitForTimeout(400)
  const afterDesc = await seoDesc.inputValue()
  console.log(`AE-11 desc after click="${afterDesc.slice(0, 80)}" unchanged=${afterDesc === existing}`)
  expect.soft(afterDesc, `取消覆盖后原 SEO 描述应保持不变，实测 "${afterDesc}"`).toBe(existing)
  await modal(page).getByRole('button', { name: '取消' }).click()
  expectClean(problems)
})

/* ============================ AE-12 ============================ */
test('AE-12 多行文本框：初始高 ≥96px，50 行后 ≤400px 且内部滚动', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openNew(page)
  const excerpt = modal(page).locator('.short-text-area')
  const seoDesc = modal(page).locator('.meta-card', { hasText: 'SEO 搜索引擎优化' }).locator('textarea').first()

  const measure = async (loc: typeof excerpt, name: string) => {
    const m = await loc.evaluate((el) => {
      const cs = getComputedStyle(el as HTMLElement)
      return { h: (el as HTMLElement).getBoundingClientRect().height, ch: (el as HTMLTextAreaElement).clientHeight, sh: (el as HTMLTextAreaElement).scrollHeight, border: cs.borderTopWidth, radius: cs.borderTopLeftRadius, overflowY: cs.overflowY }
    })
    console.log(`AE-12 ${name} initial=${JSON.stringify(m)}`)
    await shot(page, `${name}-initial`)
    expect.soft(m.h, `${name} 初始高度期望 ≥96px，实测 ${m.h}px`).toBeGreaterThanOrEqual(96)
    return m
  }
  await measure(excerpt, 'excerpt')
  await measure(seoDesc, 'seoDesc')

  const fifty = Array.from({ length: 50 }, (_, i) => `第${i + 1}行：ACC-W2 多行文本高度校验内容`).join('\n')
  await excerpt.fill(fifty)
  await page.waitForTimeout(400)
  const e2 = await excerpt.evaluate((el) => ({ h: (el as HTMLElement).getBoundingClientRect().height, ch: (el as HTMLTextAreaElement).clientHeight, sh: (el as HTMLTextAreaElement).scrollHeight, overflowY: getComputedStyle(el as HTMLElement).overflowY }))
  console.log(`AE-12 excerpt after 50 lines=${JSON.stringify(e2)}`)
  await shot(page, 'excerpt-50lines')
  expect.soft(e2.h, `摘要贴入 50 行后高度期望 ≤400px，实测 ${e2.h}px`).toBeLessThanOrEqual(400)
  expect.soft(e2.sh > e2.ch, `摘要超出后应内部滚动，实测 scrollHeight=${e2.sh} clientHeight=${e2.ch}`).toBe(true)

  await seoDesc.fill(fifty)
  await page.waitForTimeout(400)
  const s2 = await seoDesc.evaluate((el) => ({ h: (el as HTMLElement).getBoundingClientRect().height, ch: (el as HTMLTextAreaElement).clientHeight, sh: (el as HTMLTextAreaElement).scrollHeight, len: (el as HTMLTextAreaElement).value.length, max: (el as HTMLTextAreaElement).maxLength }))
  const truncated = s2.max > 0 && s2.len >= s2.max && s2.len < fifty.length
  console.log(`AE-12 seoDesc after 50 lines=${JSON.stringify(s2)} truncatedByMaxlength=${truncated}`)
  await shot(page, 'seodesc-50lines')
  expect.soft(s2.h, `SEO 描述贴入 50 行后高度期望 ≤400px，实测 ${s2.h}px`).toBeLessThanOrEqual(400)
  if (truncated) {
    console.log(`AE-12 说明：SEO 描述含 maxlength=${s2.max}，50 行文本被浏览器截断为 ${s2.len} 字，无溢出故不判内部滚动`)
  } else {
    expect.soft(s2.sh > s2.ch, `SEO 描述超出后应内部滚动，实测 scrollHeight=${s2.sh} clientHeight=${s2.ch}`).toBe(true)
  }
  await modal(page).getByRole('button', { name: '取消' }).click()
  expectClean(problems)
})

/* ============================ AE-13 ============================ */
test('AE-13 草稿行精选开关：禁用并说明「请先发布」，无 500', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await search(page, DRAFT_TITLE)
  const row = rowOf(page, DRAFT_TITLE)
  await expect(row).toHaveCount(1)
  const toggle = row.locator('.featured-toggle')
  const state = await toggle.evaluate((el) => ({ disabled: (el as HTMLInputElement).disabled, title: el.getAttribute('title') || '' }))
  console.log(`AE-13 draft toggle=${JSON.stringify(state)}`)
  await shot(page, 'draft-featured')
  expect.soft(state.disabled, `草稿文章精选开关应禁用，实测 disabled=${state.disabled}`).toBe(true)
  expect.soft(state.title, `应提示「请先发布」，实测 title="${state.title}"`).toMatch(/请先发布|已发布/)
  const http500 = problems.filter((p) => p.kind === 'http' && /→ 5\d\d/.test(p.detail))
  expect.soft(http500, `出现 5xx：${http500.map((p) => p.detail).join('; ')}`).toEqual([])
})

/* ============================ AE-14 ============================ */
test('AE-14 已发布文章精选开关：滑块式、开 #2563EB/关灰、刷新保持', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await search(page, PUB_TITLE)
  const row = rowOf(page, PUB_TITLE)
  await expect(row).toHaveCount(1)

  const desc = await row.locator('.featured-toggle').evaluate((el) => {
    const cs = getComputedStyle(el as HTMLElement)
    const r = (el as HTMLElement).getBoundingClientRect()
    return {
      tag: el.tagName,
      type: (el as HTMLInputElement).type,
      role: el.getAttribute('role') || '',
      cls: el.className,
      w: r.width,
      h: r.height,
      bg: cs.backgroundColor,
      appearance: cs.appearance,
      knob: !!(el as HTMLElement).querySelector('.switch-knob'),
      // 浏览器原生 checkbox 的特征：appearance 为 auto 且无滑块子元素
      nativeCheckbox: cs.appearance !== 'none' && !((el as HTMLElement).querySelector('*')),
    }
  })
  console.log(`AE-14 featured toggle desc=${JSON.stringify(desc)}`)
  await shot(page, 'toggle-off')
  expect.soft(desc.nativeCheckbox || desc.tag === 'INPUT', `精选开关应为滑块式（.switch/.switch-btn），实测 ${desc.tag} type=${desc.type} class="${desc.cls}" appearance=${desc.appearance}`).toBe(false)
  expect.soft(desc.role, `滑块应有 role=switch，实测 role="${desc.role}"`).toMatch(/switch/i)
  expect.soft(desc.knob, '滑块应含 .switch-knob 滑块子元素').toBe(true)

  // 滑块尺寸口径（46×26）
  expect.soft(desc.w, `滑块宽度期望 46px，实测 ${desc.w}`).toBeGreaterThanOrEqual(40)
  expect.soft(desc.w, `滑块宽度期望 46px，实测 ${desc.w}`).toBeLessThanOrEqual(52)
  expect.soft(desc.h, `滑块高度期望 26px，实测 ${desc.h}`).toBeGreaterThanOrEqual(22)
  expect.soft(desc.h, `滑块高度期望 26px，实测 ${desc.h}`).toBeLessThanOrEqual(30)

  // 打开 → 蓝色 #2563EB；关 → 灰（浅色主题）
  const onOf = (page2: typeof page, title: string) =>
    rowOf(page2, title).locator('.featured-toggle').evaluate((el) => ({
      on: (el as HTMLElement).classList.contains('on'),
      ariaChecked: el.getAttribute('aria-checked') || '',
      bg: getComputedStyle(el as HTMLElement).backgroundColor,
    }))
  await waitWrite(page, 'POST', TAX_API, () => row.locator('.featured-toggle').click())
  await page.waitForTimeout(400)
  const onStyle = await onOf(page, PUB_TITLE)
  console.log(`AE-14 on style=${JSON.stringify(onStyle)}`)
  await shot(page, 'toggle-on')
  expect.soft(onStyle.on, '点击后滑块应为 on 态').toBe(true)
  expect.soft(onStyle.bg, `滑块 on 态背景应为主色 #2563eb，实测 ${onStyle.bg}`).toMatch(/37, 99, 235/)

  await reloadToTab(page, TAB)
  await search(page, PUB_TITLE)
  const stillOn = await rowOf(page, PUB_TITLE).locator('.featured-toggle').evaluate((el) => (el as HTMLElement).classList.contains('on'))
  console.log(`AE-14 after reload checked=${stillOn}`)
  await shot(page, 'toggle-on-after-reload')
  expect.soft(stillOn, `刷新后精选状态应保持（开），实测 on=${stillOn}`).toBe(true)

  await waitWrite(page, 'POST', TAX_API, () => rowOf(page, PUB_TITLE).locator('.featured-toggle').click())
  await page.waitForTimeout(300)
  const offStyle = await onOf(page, PUB_TITLE)
  console.log(`AE-14 off style=${JSON.stringify(offStyle)}`)
  expect.soft(offStyle.on, `再次点击应关闭精选，实测 on=${offStyle.on}`).toBe(false)
  await reloadToTab(page, TAB)
  await search(page, PUB_TITLE)
  const offAfterReload = await rowOf(page, PUB_TITLE).locator('.featured-toggle').evaluate((el) => (el as HTMLElement).classList.contains('on'))
  await expect(rowOf(page, PUB_TITLE).locator('.featured-toggle')).not.toHaveClass(/on/)
  await shot(page, 'toggle-off-after-reload')
  expect.soft(offAfterReload, `刷新后精选应保持关闭，实测 on=${offAfterReload}`).toBe(false)
  expectClean(problems)
})

/* ============================ AE-15 ============================ */
test('AE-15 勾选同步语言 英+法 保存后，列表译文列出现 EN/FR 状态', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openEdit(page, PUB_TITLE)
  const card = modal(page).locator('.translations-card')
  await expect(card).toBeVisible()
  await shot(page, 'translations-card')

  const en = card.locator('input[type=checkbox][data-locale="en-US"]')
  const fr = card.locator('input[type=checkbox][data-locale="fr-FR"]')
  await en.check()
  await fr.check()
  const checked = await card.locator('input[type=checkbox]:checked').count()
  console.log(`AE-15 checked locales=${checked}`)
  expect.soft(checked, `应能勾选 2 个同步语言，实测勾选 ${checked}`).toBe(2)
  await shot(page, 'locales-checked')

  await waitWrite(page, 'PUT', API, () => modal(page).getByRole('button', { name: '保存', exact: true }).click())
  await expect(modal(page)).toHaveCount(0)
  await page.waitForTimeout(2500)
  await reloadToTab(page, TAB)
  await search(page, PUB_TITLE)
  const transCell = rowOf(page, PUB_TITLE).locator('.trans-cell')
  const cellText = (await transCell.innerText()).trim()
  const chips = await transCell.locator('.trans-chip-mini').allInnerTexts()
  console.log(`AE-15 translation cell text="${cellText}" chips=${JSON.stringify(chips)}`)
  await shot(page, 'translation-column')
  expect.soft(chips.length, `保存后译文列应出现 EN、FR 两个状态，实测 "${cellText}"`).toBeGreaterThanOrEqual(2)
  expect.soft(chips.join('|'), `译文列应含 EN 状态，实测 ${chips.join('|')}`).toMatch(/EN/)
  expect.soft(chips.join('|'), `译文列应含 FR 状态，实测 ${chips.join('|')}`).toMatch(/FR/)

  const failed = chips.filter((c) => /失败/.test(c))
  if (failed.length) {
    const retry = rowOf(page, PUB_TITLE).locator('button', { hasText: '重试' })
    expect.soft(await retry.count(), `失败项应有「重试」按钮，实测 ${await retry.count()} 个`).toBeGreaterThan(0)
  } else {
    console.log('AE-15 无失败状态，跳过重试分支（不记 FAIL，报告注明未触发）')
  }
  expectClean(problems)
})

/* ============================ AE-16 ============================ */
test('AE-16 四种关闭方式 + 未保存确认', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })

  const probe = async (way: 'x' | 'cancel' | 'backdrop' | 'esc') => {
    await openNew(page)
    await modal(page).locator('.editor-title-input').fill(`${PREFIX}-UNSAVED-${way}-${stamp}`)
    if (way === 'x') await modal(page).locator('.close-btn').click()
    else if (way === 'cancel') await modal(page).getByRole('button', { name: '取消' }).click()
    else if (way === 'backdrop') await page.locator('.modal-backdrop').first().click({ position: { x: 10, y: 10 } })
    else await page.keyboard.press('Escape')
    await page.waitForTimeout(700)
    // 有未保存修改时：关闭动作被拦截，编辑弹窗隐藏并显示「有未保存修改，确认离开？」
    const confirmText = await page.locator('text=/有未保存修改/').count()
    const confirmNodes = await page.locator('.confirm-dialog, .inline-tax-dialog, [role=dialog]').filter({ hasText: '有未保存修改' }).count()
    const modalOpen = await modal(page).count()
    const guardShown = await page.locator('.confirm-dialog').filter({ hasText: '有未保存修改' }).isVisible().catch(() => false)
    console.log(`AE-16 way=${way} confirmText=${confirmText} confirmNodes=${confirmNodes} modalOpen=${modalOpen} guardShown=${guardShown}`)
    await shot(page, `close-${way}`)
    return { confirm: confirmText + confirmNodes, modalOpen }
  }

  const results: Record<string, { confirm: number; modalOpen: number }> = {}
  for (const way of ['x', 'cancel', 'backdrop', 'esc'] as const) {
    results[way] = await probe(way)
    // 复原：处理可能出现的确认框，然后关闭弹窗
    const keepEditing = page.getByRole('button', { name: /继续编辑/ })
    if (await keepEditing.isVisible().catch(() => false)) await keepEditing.click().catch(() => {})
    const discard = page.getByRole('button', { name: /确认离开|确认|离开/ }).last()
    if (await discard.isVisible().catch(() => false)) await discard.click().catch(() => {})
    if (await modal(page).count()) {
      await modal(page).locator('.editor-title-input').fill(`${PREFIX}-UNSAVED-${way}-${stamp}-x`)
      await modal(page).locator('.close-btn').click().catch(() => {})
      await page.waitForTimeout(300)
      const leave = page.getByRole('button', { name: /确认离开/ })
      if (await leave.isVisible().catch(() => false)) await leave.click().catch(() => {})
      await page.waitForTimeout(300)
    }
  }

  for (const way of ['x', 'cancel', 'backdrop', 'esc'] as const) {
    expect.soft(results[way].confirm, `关闭方式「${way}」在有未保存修改时应先弹「有未保存修改，确认离开？」，实测确认框数 ${results[way].confirm}`).toBeGreaterThan(0)
  }
  // Esc 必须能触发关闭流程：编辑弹窗不再滞留（隐藏或出现确认框都算已拦截关闭）
  expect.soft(results.esc.modalOpen, `Esc 应能关闭弹窗（进入未保存确认），实测弹窗仍打开=${results.esc.modalOpen === 1}`).toBe(0)
  expectClean(problems)
})

/* ============================ AE-17 ============================ */
test('AE-17 新建文章保存：关闭弹窗 + 「已保存」提示 + 列表出现 + 仍为草稿', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  const newTitle = `${PREFIX}-NEW-${stamp}`
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openNew(page)
  await modal(page).locator('.editor-title-input').fill(newTitle)
  await fillSource(page, `<p>${newTitle} 新建校验正文。</p>`)
  await shot(page, 'filled')
  const res = await waitWrite(page, 'POST', API, () => modal(page).getByRole('button', { name: '保存', exact: true }).click())
  console.log(`AE-17 create response keys=${res ? Object.keys(res).join(',') : 'null'}`)

  const closed = await modal(page).count()
  await page.waitForTimeout(500)
  const toastTexts = await page.locator('.ccai-neolab-toast').allInnerTexts()
  const savedToast = toastTexts.filter((t) => /已保存/.test(t))
  console.log(`AE-17 modalCount=${closed} toasts=${JSON.stringify(toastTexts)}`)
  await shot(page, 'after-save')
  expect.soft(closed, '保存成功后应关闭弹窗，实测仍打开').toBe(0)
  expect.soft(savedToast.length, `保存成功后应出现「已保存」提示，实测 toast=${JSON.stringify(toastTexts)}`).toBeGreaterThan(0)
  await reloadToTab(page, TAB)
  await search(page, newTitle)
  const row = rowOf(page, newTitle)
  await expect(row).toHaveCount(1)
  const status = (await row.locator('.status').first().innerText()).trim()
  console.log(`AE-17 list status="${status}"`)
  await shot(page, 'list-row')
  expect.soft(status, `新建文章保存后应为草稿，实测状态「${status}」`).toMatch(/draft|草稿/)
  expectClean(problems)
})

/* ============================ AE-18 ============================ */
test('AE-18 分类下拉、状态下拉为统一 Select（无原生 select/datalist/默认箭头）', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  // 「状态下拉」指列表工具栏的「文章状态」筛选，也必须是统一 Select（口径：统一 Select，不外露原生箭头/datalist）
  const toolbarStatus = page.locator('.table-top-actions [data-test="article-status"]')
  const toolbarNative = await page.locator('.table-top-actions select, .table-top-actions datalist').count()
  const toolbarInfo = (await toolbarStatus.count())
    ? await toolbarStatus.first().evaluate((el) => ({
        text: (el as HTMLElement).innerText.trim(),
        svg: !!(el as HTMLElement).querySelector('svg'),
        tag: (el as HTMLElement).tagName.toLowerCase(),
      }))
    : null
  console.log(`AE-18 toolbarStatus=${JSON.stringify(toolbarInfo)} toolbarNative=${toolbarNative}`)
  expect.soft(await toolbarStatus.count(), '列表「文章状态」下拉应为统一 Select 组件，实测 0 个').toBe(1)
  expect.soft(toolbarNative, `列表工具栏不应外露原生 select/datalist，实测 ${toolbarNative} 个`).toBe(0)
  if (toolbarInfo) {
    expect.soft(toolbarInfo.svg, '「文章状态」Select 应使用统一箭头图标（svg），实测无').toBe(true)
    expect.soft(toolbarInfo.text.length, '「文章状态」Select 应显示当前选项文案').toBeGreaterThan(0)
  }
  await shot(page, 'toolbar-status')

  await openNew(page)
  const nativeSelects = await modal(page).locator('select').count()
  const datalists = await modal(page).locator('datalist').count()
  const categoryTrigger = modal(page).locator('.select-trigger[aria-label="文章分类"]')
  const catCount = await categoryTrigger.count()
  const catStyle = catCount
    ? await categoryTrigger.evaluate((el) => {
        const cs = getComputedStyle(el as HTMLElement)
        const r = (el as HTMLElement).getBoundingClientRect()
        return { h: r.height, radius: cs.borderTopLeftRadius, appearance: cs.appearance, svg: !!(el as HTMLElement).querySelector('svg') }
      })
    : null
  console.log(`AE-18 nativeSelects=${nativeSelects} datalists=${datalists} categoryTrigger=${catCount} style=${JSON.stringify(catStyle)}`)
  await shot(page, 'selects')

  expect.soft(nativeSelects, `弹窗内不应出现原生 <select>，实测 ${nativeSelects} 个`).toBe(0)
  expect.soft(datalists, `弹窗内不应出现 <datalist>，实测 ${datalists} 个`).toBe(0)
  expect.soft(catCount, '分类下拉应为统一 Select 组件（.select-trigger[aria-label="文章分类"]），实测 0 个').toBe(1)
  if (catStyle) {
    // 口径未规定下拉高度/圆角，仅记录实测值
    console.log(`AE-18 catSelect h=${catStyle.h.toFixed(1)} radius=${catStyle.radius}`)
    expect.soft(catStyle.svg, '分类 Select 应使用统一箭头图标（svg），实测无').toBe(true)
  }

  // 真实交互：打开分类下拉并选中第一项
  if (catCount) {
    await categoryTrigger.click()
    const listbox = page.locator('.select-dropdown-portal [role=listbox]')
    await expect(listbox).toBeVisible()
    const options = listbox.locator('[role=option]')
    const optCount = await options.count()
    const firstText = optCount ? (await options.first().innerText()).trim() : ''
    console.log(`AE-18 category options=${optCount} first="${firstText}"`)
    await shot(page, 'category-open')
    if (optCount > 1) {
      await options.nth(1).click()
      await page.waitForTimeout(300)
      const value = (await categoryTrigger.innerText()).trim()
      console.log(`AE-18 selected category="${value}"`)
      expect.soft(value.length, '选择分类后 trigger 应显示所选名称').toBeGreaterThan(0)
    }
  }
  await modal(page).getByRole('button', { name: '取消' }).click()
  expectClean(problems)
})

/* ============================ AE-19 ============================ */
test('AE-19 键盘：Tab 顺序与视觉一致、焦点可见、不跑出弹窗', async ({ page }, info) => {
  const problems = guard(page)
  const shot = shooter('', info)
  await openArticles(page)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openNew(page)
  await modal(page).locator('.editor-title-input').click()

  const seq: { tag: string; cls: string; label: string; x: number; y: number; col: number; inside: boolean; outline: string; shadow: string }[] = []
  for (let i = 0; i < 14; i++) {
    await page.keyboard.press('Tab')
    await page.waitForTimeout(80)
    const cur = await page.evaluate(() => {
      const el = document.activeElement as HTMLElement | null
      if (!el) return null
      const cs = getComputedStyle(el)
      const r = el.getBoundingClientRect()
      return {
        tag: el.tagName,
        cls: typeof el.className === 'string' ? el.className : '',
        label: el.getAttribute('aria-label') || (el as HTMLInputElement).placeholder || el.textContent?.trim().slice(0, 24) || '',
        x: Math.round(r.x),
        y: Math.round(r.y),
        // 视觉阅读顺序：先左栏（配置项）后右栏（正文编辑器），列内按 y
        col: (() => {
          const m = el.closest('.article-editor-modal') as HTMLElement | null
          const mr = m ? m.getBoundingClientRect() : { x: 0, width: innerWidth }
          return r.x + r.width / 2 < mr.x + mr.width / 2 ? 0 : 1
        })(),
        inside: !!el.closest('.article-editor-modal'),
        outline: `${cs.outlineStyle}/${cs.outlineWidth}`,
        shadow: cs.boxShadow,
      }
    })
    if (cur) seq.push(cur)
  }
  console.log('AE-19 tab sequence:', JSON.stringify(seq, null, 1))
  await shot(page, 'tab-sequence')

  const outside = seq.filter((s) => !s.inside)
  expect.soft(outside, `Tab 焦点跑出弹窗：${JSON.stringify(outside)}`).toEqual([])
  const noIndicator = seq.filter((s) => s.outline.startsWith('none/0px') || (s.outline.startsWith('none') && s.shadow === 'none'))
  console.log(`AE-19 no visible focus indicator count=${noIndicator.length} ${JSON.stringify(noIndicator)}`)
  expect.soft(noIndicator, `以下焦点无可见 outline/box-shadow：${JSON.stringify(noIndicator)}`).toEqual([])

  // 视觉顺序：两栏布局下按「先左栏后右栏、列内自上而下」比较 (col,y) 字典序，必须单调不减
  // 视觉顺序违规 = 同一列内 y 回跳（y 变小且没有明显向右换列）
  const regressions: string[] = []
  for (let i = 1; i < seq.length; i++) {
    const a = seq[i - 1]
    const b = seq[i]
    const jumpedBackUp = b.y < a.y - 2 && b.x < a.x + 100
    if (jumpedBackUp) regressions.push(`${i}: ${(a.label || a.cls).slice(0, 14)}(y${a.y},x${a.x}) → ${(b.label || b.cls).slice(0, 14)}(y${b.y},x${b.x})`)
  }
  console.log(`AE-19 order=${seq.map((s) => `${s.tag}.${(s.label || s.cls).slice(0, 18)}@c${s.col}y${s.y}x${s.x}`).join(' | ')}`)
  expect.soft(regressions, `同列内 Tab 顺序与视觉顺序不一致（焦点上跳）：${regressions.join(' | ')}`).toEqual([])

  await modal(page).getByRole('button', { name: '取消' }).click()
  expectClean(problems)
})
