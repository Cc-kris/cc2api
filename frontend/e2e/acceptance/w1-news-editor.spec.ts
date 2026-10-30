import fs from 'node:fs'
import path from 'node:path'
import { expect, test, type Browser, type Page, type TestInfo } from '@playwright/test'
import { BASE, EMAIL, PASSWORD, dismissOverlays, expectClean, guard, openContentTab, realLogin, reloadToTab, shooter, stamp, waitWrite } from '../real/real-helpers'

/**
 * v0.4 W1 · 新闻编辑弹窗 + 富文本编辑器（NE-01 ~ NE-19）
 *
 * 本文件是**验收**而不是回归：断言直接照 `docs/v0.4/v0.4_e2e_acceptance_cases_20261028.md`
 * §W1 与 PRD R2 / 1.3–1.5 写死，产品不达标就 FAIL，不为通过而放宽。
 *
 * 契约（同 real-helpers）：无 route/mock，请求全部打到真实 3100 → 8080 → PostgreSQL；
 * 登录走真实登录页。唯一走接口的动作是**测试数据造数与清理**（界面无「新建新闻」入口，
 * 用真实登录会话拿到的 token 调 admin API 建 ACC-W1-* 数据，结束时删除），断言全部经界面完成。
 *
 * 选择器说明（相对用例文档的调整 + 依据）：
 *  - 文档写 `wangEditor` 类名（`.w-e-text-container`、`.w-e-toolbar`）。实际产品新闻正文用的是
 *    `frontend/src/components/admin/announcements/AnnouncementRichTextEditor.vue`（contentEditable +
 *    execCommand），根节点带 `data-test="news-rich-editor"`。故统一用 `[data-test="news-rich-editor"] .rich-text-content`
 *    取编辑区、`.rich-text-toolbar button` 取工具栏；**这本身就是 NE-08 的 FAIL 证据**。
 *  - 弹窗用 `.news-editor[role="dialog"]`（含 `aria-labelledby="news-editor-title"`），比按文案匹配稳。
 *  - 字段用 `getByLabel(text, { exact: true })`——弹窗字段是 `<label>文案<input></label>` 包裹结构。
 */

const TAB = '新闻动态'
const RUN = stamp
const TITLE = `ACC-W1-${RUN}-主新闻`
const TITLE_LONG = `ACC-W1-${RUN}-长文`
const SLUG = `acc-w1-${RUN}`
const SLUG_LONG = `acc-w1-long-${RUN}`
const OUT = process.env.E2E_REAL_OUT ?? path.resolve(process.cwd(), 'test-results/e2e-real-v04/w1')
const MEASURE = path.resolve(process.cwd(), '../e2e-acceptance-v04/w1-measurements.jsonl')

const RICH_BODY = [
  '<h2>W1 小节标题</h2>',
  '<p>正文段落，含 <strong>加粗</strong> 与 <em>斜体</em> 与 <a href="https://example.com/link">外部链接</a>。</p>',
  '<ul><li>无序一</li><li>无序二</li></ul>',
  '<ol><li>有序一</li></ol>',
  '<blockquote>引用块文字</blockquote>',
  '<p><img src="https://picsum.photos/seed/w1/400/200" alt="W1 配图"></p>',
].join('')

const LONG_TEXT = (() => {
  const unit = '这是一段用于 W1 长文往返验收的中文正文，用来验证保存前后逐字一致。'
  let s = ''
  let i = 0
  while (s.length < 10500) {
    s += `第${++i}段：${unit}\n`
  }
  return s
})()

let seededIds: number[] = []
let token = ''

// ---------------- 公共小工具 ----------------

function record(id: string, data: Record<string, unknown>) {
  fs.mkdirSync(path.dirname(MEASURE), { recursive: true })
  fs.appendFileSync(MEASURE, JSON.stringify({ id, at: new Date().toISOString(), ...data }) + '\n')
}

/** 弹窗根节点 */
const dlg = (page: Page) => page.locator('.news-editor[role="dialog"]')
/** 富文本编辑区（contentEditable） */
const editor = (page: Page) => page.locator('[data-test="news-rich-editor"] .rich-text-content')
const toolbar = (page: Page) => page.locator('[data-test="news-rich-editor"] .rich-text-toolbar')
const footer = (page: Page) => page.locator('.news-editor .modal-fixed-footer')

async function openNewsTab(page: Page) {
  await openContentTab(page, TAB)
  await expect(page.locator('input[placeholder="搜索标题或摘要"]')).toBeVisible({ timeout: 20_000 })
}

/** 打开 ACC-W1 目标新闻的编辑弹窗（真实点击 查询 → 行内 编辑） */
async function openEditorFor(page: Page, title: string) {
  await openNewsTab(page)
  const search = page.locator('input[placeholder="搜索标题或摘要"]')
  await search.fill(title)
  await Promise.all([
    page.waitForResponse((r) => r.url().includes('/admin/content/news?') && r.status() < 400, { timeout: 20_000 }).catch(() => null),
    page.locator('.toolbar button', { hasText: '查询' }).click(),
  ])
  const row = page.locator('table tbody tr', { hasText: title }).first()
  await expect(row, `列表中找不到 ${title}`).toBeVisible({ timeout: 20_000 })
  await row.getByRole('button', { name: '编辑' }).click()
  await expect(dlg(page)).toBeVisible({ timeout: 20_000 })
  await expect(page.getByLabel('标题', { exact: true })).toHaveValue(title, { timeout: 10_000 })
}

async function closeDialogIfOpen(page: Page) {
  if (await dlg(page).isVisible().catch(() => false)) {
    const cancel = footer(page).getByRole('button', { name: '取消' })
    if (await cancel.isVisible().catch(() => false)) await cancel.click()
    else await page.keyboard.press('Escape')
    await expect(dlg(page)).toBeHidden({ timeout: 10_000 })
  }
}

/** 页面内测量：弹窗/两栏/编辑器/译文面板几何 + 可视高度 + 样式 */
async function measureLayout(page: Page) {
  return page.evaluate(() => {
    const q = (s: string) => document.querySelector(s) as HTMLElement | null
    const rect = (el: Element | null) => {
      if (!el) return null
      const r = el.getBoundingClientRect()
      return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height), right: Math.round(r.right), bottom: Math.round(r.bottom) }
    }
    const visibleHeight = (el: Element | null) => {
      if (!el) return 0
      const r = el.getBoundingClientRect()
      let top = r.top
      let bottom = r.bottom
      let p: HTMLElement | null = el.parentElement
      while (p) {
        const cs = getComputedStyle(p)
        if (/(auto|scroll|hidden)/.test(cs.overflowY)) {
          const pr = p.getBoundingClientRect()
          top = Math.max(top, pr.top)
          bottom = Math.min(bottom, pr.bottom)
        }
        p = p.parentElement
      }
      top = Math.max(top, 0)
      bottom = Math.min(bottom, window.innerHeight)
      return Math.max(0, Math.round(bottom - top))
    }
    const dialog = q('.news-editor[role="dialog"]')
    const content = q('[data-test="news-rich-editor"] .rich-text-content')
    const panel = q('.news-translation-panel')
    const foot = q('.news-editor .modal-fixed-footer')
    const cols = [...document.querySelectorAll('.news-editor .two-col-editor > div')].map((el) => ({
      cls: (el as HTMLElement).className,
      rect: rect(el),
    }))
    const labels = [...document.querySelectorAll('.news-editor .form-grid > label')].map((el) => (el.textContent || '').trim())
    const leftColLabels = [...document.querySelectorAll('.news-editor .two-col-editor > div:first-child label, .news-editor .two-col-editor > div:first-child .config-title, .news-editor .editor-config-col .config-title')].map((el) => (el.textContent || '').trim())
    const inputs = [...document.querySelectorAll('.news-editor .form-grid > label')].map((el) => {
      const ctrl = el.querySelector('input,select,textarea') as HTMLElement | null
      return {
        tag: ctrl?.tagName,
        cls: ctrl?.className || '',
        type: (ctrl as HTMLInputElement | null)?.type || '',
        list: (ctrl as HTMLInputElement | null)?.list?.id || '',
        role: ctrl?.getAttribute('role') || '',
      }
    })
    const toolbarButtons = [...document.querySelectorAll('[data-test="news-rich-editor"] .rich-text-toolbar button')].map((b) => ({
      text: (b.textContent || '').trim(),
      aria: b.getAttribute('aria-label'),
    }))
    const footerButtons = [...document.querySelectorAll('.news-editor .modal-fixed-footer button')].map((b) => (b.textContent || '').trim())
    const checkboxGroups = [...document.querySelectorAll('.news-editor input[type="checkbox"]')].map((el) => {
      const lab = el.closest('label')
      return (lab?.textContent || '').trim()
    })
    return {
      viewport: { w: window.innerWidth, h: window.innerHeight },
      docScrollOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
      dialog: rect(dialog),
      dialogRadius: dialog ? getComputedStyle(dialog).borderRadius : '',
      dialogBg: dialog ? getComputedStyle(dialog).backgroundColor : '',
      bodyScrollBody: rect(q('.news-editor .modal-scroll-body')),
      bodyScrollHeight: q('.news-editor .modal-scroll-body')?.scrollHeight ?? 0,
      bodyClientHeight: q('.news-editor .modal-scroll-body')?.clientHeight ?? 0,
      cols,
      labels,
      leftColLabels,
      inputs,
      toolbarButtons,
      footerButtons,
      footer: rect(foot),
      editor: rect(content),
      editorVisible: visibleHeight(content),
      editorScrollH: content?.scrollHeight ?? 0,
      editorClientH: content?.clientHeight ?? 0,
      editorTag: content?.tagName ?? null,
      editorEditable: content?.getAttribute('contenteditable') ?? null,
      panel: rect(panel),
      panelVisible: visibleHeight(panel),
      checkboxLabels: checkboxGroups,
      titleStar: (() => {
        const label = [...document.querySelectorAll('.news-editor .form-grid > label')][0]
        return label ? (label.textContent || '').includes('*') : null
      })(),
    }
  })
}

/** 用真实剪贴板 HTML 粘贴（失败则退回 innerHTML+input，并在返回值里说明） */
async function pasteHtmlIntoEditor(page: Page, html: string, marker: string): Promise<'clipboard' | 'fallback'> {
  await editor(page).click()
  await page.keyboard.press('ControlOrMeta+A')
  await page.keyboard.press('Delete')
  let copied = false
  try {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
    copied = await page.evaluate(async (h) => {
      try {
        await navigator.clipboard.write([new ClipboardItem({ 'text/html': new Blob([h], { type: 'text/html' }), 'text/plain': new Blob([h], { type: 'text/plain' }) })])
        return true
      } catch {
        return false
      }
    }, html)
  } catch {
    copied = false
  }
  if (copied) {
    await page.keyboard.press('ControlOrMeta+V')
    await page.waitForTimeout(400)
    const got = (await editor(page).innerHTML()).includes(marker)
    if (got) return 'clipboard'
  }
  await page.evaluate(
    ([h]) => {
      const el = document.querySelector('[data-test="news-rich-editor"] .rich-text-content') as HTMLElement
      el.focus()
      el.innerHTML = h as string
      el.dispatchEvent(new Event('input', { bubbles: true }))
    },
    [html],
  )
  await page.waitForTimeout(200)
  return 'fallback'
}

async function apiCall(page: Page, method: string, apiPath: string, data?: unknown) {
  const res = await page.request.fetch(`${BASE}/api/v1${apiPath}`, {
    method,
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    data: data === undefined ? undefined : JSON.stringify(data),
  })
  const text = await res.text()
  let json: any = null
  try {
    json = JSON.parse(text)
  } catch {
    /* 非 JSON */
  }
  return { status: res.status(), json, text }
}

async function seedNews(browser: Browser) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'zh-CN' })
  const page = await ctx.newPage()
  // worker 被重建时 beforeAll 会重跑，登录必须可重试（否则后续用例全被拖死）
  for (let attempt = 1; attempt <= 3; attempt++) {
    await page.goto(`${BASE}/login`)
    await page.locator('#email').fill(EMAIL)
    await page.locator('#password').fill(PASSWORD)
    await page.locator('button[type="submit"]').click()
    const ok = await page
      .waitForURL((u) => !u.pathname.startsWith('/login'), { timeout: 20_000 })
      .then(() => true)
      .catch(() => false)
    if (ok) break
    await page.waitForTimeout(1000 * attempt)
  }
  token = (await page.evaluate(() => localStorage.getItem('auth_token'))) || ''
  const src = await apiCall(page, 'GET', '/admin/content/news-sources?page=1&page_size=1')
  const sourceId = src.json?.data?.items?.[0]?.id ?? 1
  const base = {
    sourceId,
    locale: 'zh-CN',
    status: 'draft',
    publishMode: 'manual',
    sourceName: 'ACC-W1 来源',
    category: '模型发布',
    tags: ['AI', 'acc-w1'],
    seoTitle: 'ACC-W1 SEO 标题',
    seoDescription: 'ACC-W1 SEO 描述',
  }
  const main = await apiCall(page, 'POST', '/admin/content/news', {
    ...base,
    slug: SLUG,
    sourceCanonicalUrl: `https://example.com/${SLUG}`,
    originalUrl: `https://example.com/${SLUG}`,
    title: TITLE,
    summary: 'ACC-W1 摘要：用于新闻编辑弹窗验收',
    bodyHtml: RICH_BODY,
    bodyText: 'W1 小节标题 正文段落，含 加粗 与 斜体 与 外部链接。 无序一 无序二 有序一 引用块文字',
    bodySourceType: 'html_extract',
    publishedAt: new Date().toISOString(),
  })
  expect(main.status, `造数失败：${main.text.slice(0, 300)}`).toBe(201)
  seededIds.push(main.json.data.id)

  const long = await apiCall(page, 'POST', '/admin/content/news', {
    ...base,
    slug: SLUG_LONG,
    sourceCanonicalUrl: `https://example.com/${SLUG_LONG}`,
    originalUrl: `https://example.com/${SLUG_LONG}`,
    title: TITLE_LONG,
    summary: 'ACC-W1 长文摘要',
    bodyHtml: '<p>' + LONG_TEXT.replace(/\n/g, '</p><p>') + '</p>',
    bodyText: LONG_TEXT,
    bodySourceType: 'html_extract',
  })
  expect(long.status, `造数失败：${long.text.slice(0, 300)}`).toBe(201)
  seededIds.push(long.json.data.id)
  record('SEED', { ids: seededIds, seedBodyTextLen: LONG_TEXT.length })
  await ctx.close()
}

async function cleanupNews(browser: Browser) {
  if (!seededIds.length || !token) return
  const ctx = await browser.newContext()
  const page = await ctx.newPage()
  await page.goto(`${BASE}/login`).catch(() => null)
  const results: string[] = []
  for (const id of seededIds) {
    const del = await apiCall(page, 'DELETE', `/admin/content/news/${id}`)
    const after = await apiCall(page, 'GET', `/admin/content/news/${id}?include_deleted=true`)
    results.push(`id=${id} delete=${del.status} deleted_at=${after.json?.data?.deletedAt ?? 'null'}`)
  }
  fs.writeFileSync(path.join(OUT_STATE_DIR(), 'cleanup.txt'), results.join('\n'))
  record('CLEANUP', { results })
  await ctx.close()
}

function OUT_STATE_DIR() {
  const dir = path.resolve(process.cwd(), '../e2e-acceptance-v04')
  fs.mkdirSync(dir, { recursive: true })
  return dir
}

// ---------------- 用例 ----------------

// 用 default + --workers=1：按文件顺序串行执行（数据前依赖成立），但单条失败不会跳过后续用例，
// 验收需要「19 条各自的结论」，不能因为第一条 FAIL 就只留一条证据。
test.describe.configure({ mode: 'default', timeout: 120_000 })

test.beforeAll(async ({ browser }) => {
  await seedNews(browser)
})

test.afterAll(async ({ browser }) => {
  await cleanupNews(browser)
})

test.beforeEach(async ({ page }) => {
  // 产品若弹原生 confirm，一律确认，避免用例被对话框挂死
  page.on('dialog', (d) => d.accept().catch(() => {}))
  await realLogin(page)
})

test.afterEach(async ({ page }) => {
  // 收尾：任何残留的新闻编辑弹窗都关掉（含「未保存确认」），避免污染下一条用例
  await page.keyboard.press('Escape').catch(() => {})
  for (let i = 0; i < 2; i++) {
    const open = await page.locator('.news-editor').isVisible().catch(() => false)
    if (!open) break
    await page.locator('.news-editor .form-actions button', { hasText: '取消' }).first().click({ timeout: 3000 }).catch(() => {})
    await page.waitForTimeout(300)
    // 二次确认弹层
    await page.locator('button', { hasText: /确定|确认|放弃/ }).last().click({ timeout: 2000 }).catch(() => {})
    await page.waitForTimeout(300)
  }
})

async function enter(page: Page, title = TITLE) {
  await openEditorFor(page, title)
  await dismissOverlays(page)
}

test('NE-01 弹窗多视口尺寸/圆角/居中/小屏不溢出', async ({ page }, info) => {
  const shot = shooter('w1', info)
  const results: Record<string, unknown> = {}

  await enter(page)
  await shot(page, '1440-dialog')

  for (const vp of [{ width: 1440, height: 900 }, { width: 1024, height: 768 }]) {
    await page.setViewportSize(vp)
    await page.waitForTimeout(400)
    const m = await measureLayout(page)
    results[`${vp.width}`] = { dialog: m.dialog, radius: m.dialogRadius, overflow: m.docScrollOverflow, docW: m.viewport }
    await shot(page, `${vp.width}-layout`)
    const d = m.dialog!
    const expectW = Math.min(1320, vp.width - 32)
    expect.soft(Math.abs(d.w - expectW), `${vp.width} 下弹窗宽应为 min(1320, 100vw-32)=${expectW}（R16 基线），实测 ${d.w}`).toBeLessThanOrEqual(4)
    const centerOffset = Math.abs((d.x + d.w / 2) - vp.width / 2)
    expect.soft(centerOffset, `${vp.width} 下弹窗未居中：中心偏移 ${centerOffset}px`).toBeLessThanOrEqual(2)
    expect.soft(m.dialogRadius, `${vp.width} 下弹窗圆角应为 12px，实测 ${m.dialogRadius}`).toBe('12px')
  }

  await page.setViewportSize({ width: 390, height: 844 })
  await page.waitForTimeout(400)
  const m390 = await measureLayout(page)
  results['390'] = { dialog: m390.dialog, overflow: m390.docScrollOverflow, cols: m390.cols }
  await shot(page, '390-layout')
  expect.soft(m390.docScrollOverflow, `390 视口出现横向溢出 ${m390.docScrollOverflow}px`).toBeLessThanOrEqual(1)
  const sideBySide = m390.cols.length === 2 && Math.abs(m390.cols[0].rect!.y - m390.cols[1].rect!.y) < 8
  expect.soft(sideBySide, `390 下两栏未堆叠为单栏：${JSON.stringify(m390.cols)}`).toBe(false)

  record('NE-01', results)
  expectClean(guard(page))
})

test('NE-02 左右两栏布局与左栏字段顺序', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const m = await measureLayout(page)
  await shot(page, 'two-col')
  record('NE-02', { cols: m.cols, labels: m.labels, inputs: m.inputs })

  expect(m.cols.length, `两栏缺失，只有 ${m.cols.length} 栏`).toBe(2)
  const [left, right] = m.cols
  // R16 基线（2026-10-30 用户诉求 1/3）：左栏 = 标题/摘要/分类/标签 + 正文编辑器（弹性），
  // 右栏 = 精选/封面/SEO/翻译（固定 380）
  expect.soft(left.rect!.w, `左栏（正文栏）应占满剩余宽度（≥600px），实测 ${left.rect!.w}`).toBeGreaterThanOrEqual(600)
  expect.soft(right.rect!.w, `右栏（元信息栏）宽应为 380±8，实测 ${right.rect!.w}`).toBeGreaterThanOrEqual(372)
  expect.soft(right.rect!.w, `右栏（元信息栏）宽应为 380±8，实测 ${right.rect!.w}`).toBeLessThanOrEqual(388)
  expect.soft(Math.abs(left.rect!.y - right.rect!.y), `两栏没有并排：左栏 y=${left.rect!.y}，右栏 y=${right.rect!.y}`).toBeLessThanOrEqual(8)
  // 正文编辑器必须落在左栏内（不被右栏挤压）
  const ed = m.editor!
  expect.soft(ed.x, `正文编辑器 x=${ed.x} 未落在左栏（x=${left.rect!.x}..${left.rect!.x + left.rect!.w}）`).toBeGreaterThanOrEqual(left.rect!.x)
  expect.soft(ed.x + ed.w, `正文编辑器右边界 ${ed.x + ed.w} 越出左栏右边界 ${left.rect!.x + left.rect!.w}`).toBeLessThanOrEqual(left.rect!.x + left.rect!.w + 1)

  const expectedOrder = ['标题', '摘要', '分类', '标签']
  const leftLabels = m.leftColLabels
  const present = leftLabels.filter((l) => expectedOrder.some((e) => l.startsWith(e)))
  for (const name of expectedOrder) {
    expect.soft(leftLabels.some((l) => l.startsWith(name)), `左栏缺字段「${name}」；左栏实有：${leftLabels.join(' / ')}`).toBe(true)
  }
  expect.soft(present, `左栏字段顺序与规范不一致\n规范：${expectedOrder.join(' / ')}\n实测：${present.join(' / ')}`).toEqual(expectedOrder)
  expectClean(guard(page))
})

test('NE-03 分类、状态使用统一 Select 组件', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const probe = await page.evaluate(() => {
    const labels = [...document.querySelectorAll('.news-editor .form-grid > label')]
    const pick = (name: string) => labels.find((l) => (l.textContent || '').trim().startsWith(name)) || null
    const describe = (name: string) => {
      const label = pick(name)
      if (!label) return { found: false }
      const native = label.querySelector('select') as HTMLSelectElement | null
      const datalistInput = label.querySelector('input[list]') as HTMLInputElement | null
      const combobox = label.querySelector('[role="combobox"],[aria-haspopup="listbox"]') as HTMLElement | null
      const plainInput = label.querySelector('input:not([list])') as HTMLInputElement | null
      return {
        found: true,
        nativeSelect: !!native,
        nativeOptions: native ? [...native.options].map((o) => o.textContent) : [],
        datalistId: datalistInput?.list?.id ?? null,
        datalistOptions: datalistInput?.list ? [...(datalistInput.list.options || [])].map((o) => o.value) : [],
        combobox: !!combobox,
        comboboxRole: combobox?.getAttribute('role') ?? null,
        plainInput: !!plainInput,
        html: label.innerHTML.trim().slice(0, 220),
      }
    }
    return { 分类: describe('分类'), 状态: describe('状态') }
  })
  await shot(page, 'category-status')
  record('NE-03', probe)

  for (const [name, d] of Object.entries(probe) as [string, any][]) {
    expect.soft(d.found, `弹窗缺字段「${name}」`).toBe(true)
  }
  const cat: any = probe['分类']
  expect.soft(cat.combobox, `分类不是统一 Select 组件（实测 ${cat.datalistId ? 'input+datalist' : cat.nativeSelect ? '原生 select' : '纯文本框'}）`).toBe(true)
  expect.soft(cat.nativeSelect, '分类仍使用原生 select').toBe(false)
  const status: any = probe['状态']
  expect.soft(status.combobox, `状态字段不是统一 Select（实测 found=${status.found}）`).toBe(true)
  if (status.combobox) {
    await page.getByLabel('状态', { exact: true }).click()
    const opts = await page.locator('[role="option"]').allInnerTexts()
    record('NE-03-status-options', { opts })
    expect.soft(opts, `状态选项应只有 草稿/发布，实测 ${opts.join(',')}`).toEqual(['草稿', '发布'])
  }
  expectClean(guard(page))
})

test('NE-04 标签使用词条式 TagInput（回车生成 chip、大小写去重、× 删除）', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const before = await page.evaluate(() => {
    const labels = [...document.querySelectorAll('.news-editor .form-grid > label')]
    const label = labels.find((l) => (l.textContent || '').trim().startsWith('标签'))!
    return {
      html: label.innerHTML.trim().slice(0, 300),
      chips: label.querySelectorAll('.tag-chip,.chip,[data-test*="tag"]').length,
      placeholder: (label.querySelector('input') as HTMLInputElement | null)?.placeholder ?? '',
    }
  })
  record('NE-04-before', before)
  const tagInput = page.getByLabel('标签', { exact: true })
  await tagInput.click()
  await tagInput.fill('AI')
  await page.keyboard.press('Enter')
  await page.waitForTimeout(300)
  await shot(page, 'tag-enter')
  const afterEnter = await page.evaluate(() => {
    const labels = [...document.querySelectorAll('.news-editor .form-grid > label')]
    const label = labels.find((l) => (l.textContent || '').trim().startsWith('标签'))!
    return { chips: label.querySelectorAll('.tag-chip,.chip,[data-test*="tag"]').length, text: (label.textContent || '').trim().slice(0, 200) }
  })
  record('NE-04-after-enter', afterEnter)

  expect.soft(before.chips, `标签不是词条式（无 chip 元素），实测 placeholder=「${before.placeholder}」innerHTML=${before.html}`).toBeGreaterThan(0)
  expect.soft(afterEnter.chips, `输入 AI 回车后没有生成 chip（chip 数 ${afterEnter.chips}，字段文本「${afterEnter.text}」）`).toBeGreaterThan(0)

  if (afterEnter.chips > 0) {
    await tagInput.fill('ai')
    await page.keyboard.press('Enter')
    await page.waitForTimeout(300)
    const dup = await page.locator('.news-editor .tag-chip').allInnerTexts()
    record('NE-04-dup', { chips: dup })
    expect.soft(dup.length, `大小写去重失败：AI 与 ai 生成了 ${dup.length} 个 chip`).toBe(1)
    const del = page.locator('.news-editor .tag-chip button, .news-editor .tag-chip .tag-remove').first()
    await del.click()
    await page.waitForTimeout(200)
    expect.soft(await page.locator('.news-editor .tag-chip').count(), '× 删除 chip 后仍有 chip').toBe(0)
  }
  expectClean(guard(page))
})

test('NE-05 同步语言多选（English/Русский/Français/Deutsch）并可回显', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const m = await measureLayout(page)
  await shot(page, 'sync-locale')
  record('NE-05', { checkboxLabels: m.checkboxLabels })
  const expected = ['English', 'Русский', 'Français', 'Deutsch']
  expect.soft(m.checkboxLabels.length, `同步语言多选缺失：弹窗内 checkbox 数量 ${m.checkboxLabels.length}，标签「${m.checkboxLabels.join(',')}」`).toBe(expected.length)
  for (const lang of expected) {
    expect.soft(m.checkboxLabels.some((l) => l.includes(lang)), `缺语言选项「${lang}」`).toBe(true)
  }
  if (m.checkboxLabels.length) {
    // 标签可能是「本地化名 + 母语名」，按母语名归一化后再校验顺序
    const order = m.checkboxLabels.map((l) => expected.find((e) => l.includes(e)) || '')
    expect.soft(order, `语言顺序应按 i18n 顺序 ${expected.join('/')}，实测 ${m.checkboxLabels.join('/')}`).toEqual(expected)
  }
  expectClean(guard(page))
})

test('NE-06 译文面板不挤压编辑区（1440×900 与 1024×768）', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const results: Record<string, unknown> = {}
  for (const vp of [{ width: 1440, height: 900 }, { width: 1024, height: 768 }]) {
    await page.setViewportSize(vp)
    await page.waitForTimeout(400)
    // R16 基线：字段在正文之上，小视口下弹窗主体可滚动。
    // 先把正文滚进可视区（用户真实操作路径），再量可视高度；仍要求不被页脚/译文面板遮挡。
    if (vp.width < 1440) {
      await editor(page).scrollIntoViewIfNeeded()
      await page.waitForTimeout(250)
    }
    const m = await measureLayout(page)
    const e = m.editor!
    const p = m.panel
    const overlap = p ? Math.max(0, Math.min(e.bottom, p.bottom) - Math.max(e.y, p.y)) : 0
    results[`${vp.width}x${vp.height}`] = {
      editor: e,
      editorVisible: m.editorVisible,
      panel: p,
      overlapPx: overlap,
      footer: m.footer,
      scrollBody: { sh: m.bodyScrollHeight, ch: m.bodyClientHeight },
    }
    await shot(page, `panel-${vp.width}`)
    expect.soft(overlap, `${vp.width}×${vp.height} 下译文面板与正文编辑器相交 ${overlap}px（编辑器 y=${e.y}..${e.bottom}，面板 y=${p?.y}..${p?.bottom}）`).toBe(0)
    const minVisible = vp.width >= 1440 ? 300 : 200
    expect.soft(m.editorVisible, `${vp.width}×${vp.height} 下编辑器可视高度 ${m.editorVisible}px，应 ≥${minVisible}px`).toBeGreaterThanOrEqual(minVisible)
  }
  record('NE-06', results)
  expectClean(guard(page))
})

test('NE-07 编辑器占满剩余高度 + 3000 字内部滚动', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await page.setViewportSize({ width: 1440, height: 900 })
  await enter(page)
  const before = await measureLayout(page)
  const gap = before.footer!.y - (before.editor!.y + before.editor!.h)
  await shot(page, 'height-before')
  record('NE-07-before', { editor: before.editor, footer: before.footer, gap, dialogH: before.dialog!.h })

  expect.soft(gap, `编辑器底边到按钮区顶边间距 ${gap}px，应 ≤24px（编辑器没占满剩余高度）`).toBeLessThanOrEqual(24)

  await editor(page).click()
  await page.keyboard.press('ControlOrMeta+A')
  await page.keyboard.press('Delete')
  const chunk = '这是 3000 字高度验收的填充文字，用于观察编辑器是否内部滚动。'
  await page.keyboard.insertText(chunk.repeat(120))
  await page.waitForTimeout(500)
  const after = await measureLayout(page)
  const dialogGrew = after.dialog!.h - before.dialog!.h
  await shot(page, 'height-after-3000')
  record('NE-07-after', {
    editorH: after.editor!.h,
    editorScrollH: after.editorScrollH,
    editorClientH: after.editorClientH,
    dialogH: after.dialog!.h,
    dialogGrew,
    textLen: chunk.length * 120,
  })
  expect.soft(Math.abs(after.editor!.h - before.editor!.h), `贴入 3000 字后编辑器外框高度变化 ${after.editor!.h - before.editor!.h}px（应不变）`).toBeLessThanOrEqual(2)
  expect.soft(after.editorScrollH > after.editorClientH, `编辑器内容区未出现内部滚动（scrollHeight=${after.editorScrollH} ≤ clientHeight=${after.editorClientH}）`).toBe(true)
  expect.soft(Math.abs(dialogGrew), `贴入 3000 字后弹窗整体变长 ${dialogGrew}px`).toBeLessThanOrEqual(2)
  expectClean(guard(page))
})

test('NE-08 工具栏 10 项 + 逐项插入对应标签', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const m = await measureLayout(page)
  await shot(page, 'toolbar')
  record('NE-08', { buttons: m.toolbarButtons, editorTag: m.editorTag, editable: m.editorEditable })

  const names = m.toolbarButtons.map((b) => (b.aria || b.text).replace(/\s/g, ''))
  const expectOrder = ['加粗', '斜体', 'H2', 'H3', '有序列表', '无序列表', '引用', '链接', '插入图片', '清除格式']
  expect.soft(names.length, `工具栏项数 ${names.length}，应为 10 项：实测 ${names.join('/')}`).toBe(10)
  for (const need of expectOrder) {
    expect.soft(names.some((n) => n.includes(need)), `工具栏缺「${need}」`).toBe(true)
  }
  const idx = (k: string) => names.findIndex((n) => n.includes(k))
  expect.soft(idx('加粗'), '工具栏顺序不符（加粗应为第 1 项）').toBe(0)

  // 逐项实际插入，检查 HTML 标签（缺项的跳过，已在上面的断言里 FAIL）
  // 允许别名：产品把「有序/无序列表」命名为「编号列表/项目列表」，功能等价，按功能判定。
  const alias: Record<string, string> = { 有序列表: '编号列表', 无序列表: '项目列表' }
  const cases: [string, string, RegExp][] = [
    ['加粗', '加粗测试', /<(strong|b)\b/i],
    ['斜体', '斜体测试', /<(em|i)\b/i],
    ['H2', 'H2 标题测试', /<h2\b/i],
    ['H3', 'H3 标题测试', /<h3\b/i],
    ['有序列表', '有序项', /<ol\b/i],
    ['无序列表', '无序项', /<ul\b/i],
    ['引用', '引用测试', /<blockquote\b/i],
  ]
  const applied: Record<string, string> = {}
  for (const [label, text, re] of cases) {
    let i = idx(label)
    if (i < 0 && alias[label]) i = idx(alias[label])
    if (i < 0) {
      applied[label] = '按钮缺失'
      continue
    }
    // 每次用空文档起步，避免上一步的行内状态（execCommand 的 toggle 语义）污染判定
    await page.evaluate(() => {
      const el = document.querySelector('[data-test="news-rich-editor"] .rich-text-content') as HTMLElement
      el.innerHTML = ''
      el.focus()
    })
    await editor(page).click()
    await page.keyboard.insertText(text)
    await page.keyboard.press('ControlOrMeta+A')
    const btn = toolbar(page).locator('button').nth(i)
    let html = ''
    for (let attempt = 0; attempt < 2; attempt++) {
      await btn.click()
      await page.waitForTimeout(150)
      html = await editor(page).innerHTML()
      if (re.test(html)) break
    }
    applied[label] = re.test(html) ? `OK ${html.slice(0, 90)}` : `NO ${html.slice(0, 90)}`
    expect.soft(re.test(html), `点击「${alias[label] ?? label}」后编辑区没有对应标签（${re}），HTML=${html.slice(0, 200)}`).toBe(true)
  }
  record('NE-08-applied', applied)
  expectClean(guard(page))
})

test('NE-09 编辑区可聚焦/输入/撤销（非只读空壳）', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const el = editor(page)
  await el.click()
  await page.keyboard.press('ControlOrMeta+A')
  await page.keyboard.press('Delete')
  await page.keyboard.type('English 与中文混排 123')
  await page.waitForTimeout(200)
  const typed = await el.innerText()
  await shot(page, 'typed')
  await page.keyboard.press('ControlOrMeta+Z')
  await page.waitForTimeout(300)
  const undone = await el.innerText()
  const focused = await page.evaluate(() => (document.activeElement as HTMLElement)?.getAttribute('contenteditable'))
  record('NE-09', { typed, undone, focused })
  expect.soft(typed.includes('English 与中文混排 123'), `编辑区无法输入，实测「${typed.slice(0, 80)}」`).toBe(true)
  expect.soft(focused, '编辑区聚焦后 activeElement 不是 contenteditable').toBe('true')
  expectClean(guard(page))
})

test('NE-10 粘贴 script/onclick/style 后保存不落库', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const evil = '<p onclick="alert(1)" style="color:red">危险段落 <script>window.__w1_xss=1</script><strong>加粗保留</strong></p>'
  const mode = await pasteHtmlIntoEditor(page, evil, '危险段落')
  await shot(page, 'pasted-evil')
  const res = await waitWrite(page, 'PUT', '/admin/content/news/', () => footer(page).getByRole('button', { name: '保存新闻' }).click())
  await shot(page, 'saved')
  const id = seededIds[0]
  const after = await apiCall(page, 'GET', `/admin/content/news/${id}`)
  const body = String(after.json?.data?.bodyHtml ?? '')
  record('NE-10', { pasteMode: mode, status: res?.status ?? null, bodyHtml: body.slice(0, 400) })
  expect.soft(body.length, '保存后 bodyHtml 为空').toBeGreaterThan(0)
  expect.soft(/<script/i.test(body), `bodyHtml 仍含 <script>：${body.slice(0, 200)}`).toBe(false)
  expect.soft(/\son\w+\s*=/i.test(body), `bodyHtml 仍含内联事件：${body.slice(0, 200)}`).toBe(false)
  expect.soft(/\sstyle\s*=/i.test(body), `bodyHtml 仍含 style 属性：${body.slice(0, 200)}`).toBe(false)
  expect.soft(body.includes('加粗保留'), '清洗把正常内容也丢了（应保留 strong 文本）').toBe(true)
  expectClean(guard(page))
})

test('NE-11 插图：6MB/.bmp 拒绝、base64 不内嵌、png 走上传接口', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const uploadUi = await page.evaluate(() => {
    const root = document.querySelector('[data-test="news-rich-editor"]') as HTMLElement | null
    return {
      fileInputs: root?.querySelectorAll('input[type="file"]').length ?? 0,
      buttons: [...(root?.querySelectorAll('.rich-text-toolbar button') ?? [])].map((b) => b.getAttribute('aria-label') || b.textContent),
    }
  })
  await shot(page, 'upload-affordance')
  record('NE-11-ui', uploadUi)
  expect.soft(uploadUi.fileInputs, `编辑器内没有上传图片入口（file input 数 ${uploadUi.fileInputs}，工具栏 ${JSON.stringify(uploadUi.buttons)}）`).toBeGreaterThan(0)
  expect.soft(uploadUi.buttons.some((b) => String(b).includes('图片')), '工具栏没有「插入图片」').toBe(true)

  // base64 图片不得内嵌
  const b64 = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
  const mode = await pasteHtmlIntoEditor(page, `<p>base64 图测试</p><p><img src="${b64}" alt="b64"></p>`, 'base64 图测试')
  await waitWrite(page, 'PUT', '/admin/content/news/', () => footer(page).getByRole('button', { name: '保存新闻' }).click())
  await shot(page, 'saved-base64')
  const after = await apiCall(page, 'GET', `/admin/content/news/${seededIds[0]}`)
  const body = String(after.json?.data?.bodyHtml ?? '')
  record('NE-11-base64', { pasteMode: mode, hasDataImage: body.includes('data:image'), bodyHtml: body.slice(0, 300) })
  expect.soft(body.includes('data:image'), '保存后 bodyHtml 仍内嵌 base64 图片').toBe(false)
  expectClean(guard(page))
})

test('NE-12 ≥10000 字正文往返逐字一致', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page, TITLE_LONG)
  const apiBefore = await apiCall(page, 'GET', `/admin/content/news/${seededIds[1]}`)
  const apiText: string = String(apiBefore.json?.data?.bodyText ?? '')
  const shown = await editor(page).innerText()
  await shot(page, 'long-open')
  const norm = (s: string) => s.replace(/\s+/g, '')
  const shownLen = norm(shown).length
  record('NE-12-open', { apiBodyTextLen: apiText.length, apiNormLen: norm(apiText).length, editorTextLen: shown.length, editorNormLen: shownLen, tail: shown.slice(-60) })
  expect.soft(apiText.length, '造数正文不足 10000 字').toBeGreaterThanOrEqual(10000)
  expect.soft(shownLen, `回显字数与接口不一致：接口(去空白) ${norm(apiText).length} 字 vs 编辑器 ${shownLen} 字`).toBe(norm(apiText).length)
  expect.soft(shown.trim().endsWith(apiText.trim().slice(-40)), '回显尾部被截断').toBe(true)

  await waitWrite(page, 'PUT', '/admin/content/news/', () => footer(page).getByRole('button', { name: '保存新闻' }).click())
  await shot(page, 'long-saved')
  const apiAfter = await apiCall(page, 'GET', `/admin/content/news/${seededIds[1]}`)
  const savedText = String(apiAfter.json?.data?.bodyText ?? '')
  record('NE-12-saved', { apiBodyTextLen: savedText.length, normLen: norm(savedText).length })
  expect.soft(norm(savedText).length, `保存后字数变化：${norm(savedText).length} vs ${norm(apiText).length}`).toBe(norm(apiText).length)

  await reloadToTab(page, TAB)
  await enter(page, TITLE_LONG)
  const reopened = norm(await editor(page).innerText())
  await shot(page, 'long-reopened')
  record('NE-12-reopen', { reopenedLen: reopened.length })
  expect.soft(reopened.length, `重开回显字数 ${reopened.length}，应与 ${norm(apiText).length} 一致`).toBe(norm(apiText).length)
  expectClean(guard(page))
})

test('NE-13 四种关闭方式（无修改）均可直接关闭', async ({ page }, info) => {
  const shot = shooter('w1', info)
  const results: Record<string, string> = {}
  const escWithFocus = async () => {
    // 规范要求 Esc 任何时候都能关闭；这里先给「焦点在弹窗内」的机会，
    // 以便区分「仅焦点在内时生效」与「完全不生效」
    await page.getByLabel('标题', { exact: true }).click()
    await page.keyboard.press('Escape')
    await page.waitForTimeout(400)
    if (await dlg(page).isHidden({ timeout: 1500 }).catch(() => false)) return
  }
  const methods: [string, () => Promise<unknown>][] = [
    ['×', () => page.locator('.news-editor [aria-label="关闭新闻编辑"]').click()],
    ['取消', () => footer(page).getByRole('button', { name: '取消' }).click()],
    ['点遮罩', () => page.mouse.click(4, 4)],
    ['Esc', escWithFocus],
  ]
  for (const [name, act] of methods) {
    await enter(page)
    await act()
    const closed = await dlg(page).isHidden({ timeout: 5000 }).catch(() => false)
    if (!closed) {
      await page.keyboard.press('Escape').catch(() => null)
      await closeDialogIfOpen(page)
    }
    results[name] = closed ? 'PASS' : 'FAIL'
    await shot(page, `close-${name}`)
    expect.soft(closed, `「${name}」未能关闭弹窗`).toBe(true)
  }
  record('NE-13', results)
  expectClean(guard(page))
})

test('NE-14 有修改时四种关闭方式都弹未保存确认', async ({ page }, info) => {
  const shot = shooter('w1', info)
  const results: Record<string, string> = {}
  const methods: [string, () => Promise<unknown>][] = [
    ['×', () => page.locator('.news-editor [aria-label="关闭新闻编辑"]').click()],
    ['取消', () => footer(page).getByRole('button', { name: '取消' }).click()],
    ['点遮罩', () => page.mouse.click(4, 4)],
    ['Esc', () => page.keyboard.press('Escape')],
  ]
  for (const [name, act] of methods) {
    await enter(page)
    const titleBox = page.getByLabel('标题', { exact: true })
    const original = await titleBox.inputValue()
    await titleBox.fill(`${original}-dirty`)
    await page.waitForTimeout(200)
    await act()
    await page.waitForTimeout(500)
    const confirm = page.locator('text=/未保存修改|确认离开/').first()
    const hasConfirm = await confirm.isVisible().catch(() => false)
    const dialogStillOpen = await dlg(page).isVisible().catch(() => false)
    await shot(page, `dirty-${name}`)
    results[name] = `confirm=${hasConfirm} dialogOpen=${dialogStillOpen}`
    expect.soft(hasConfirm, `「${name}」在有修改时没有弹「有未保存修改，确认离开？」`).toBe(true)
    if (hasConfirm) {
      const keep = page.getByRole('button', { name: /继续编辑/ }).first()
      if (await keep.isVisible().catch(() => false)) {
        await keep.click()
        await page.waitForTimeout(300)
        expect.soft(await titleBox.inputValue(), `「${name}」选继续编辑后内容丢失`).toBe(`${original}-dirty`)
      }
    }
    await closeDialogIfOpen(page).catch(() => null)
    await page.keyboard.press('Escape').catch(() => null)
    await page.waitForTimeout(200)
  }
  record('NE-14', results)
  expectClean(guard(page))
})

test('NE-15 改标题保存：关弹窗 + 顶部「已保存」+ 列表刷新，草稿仍草稿', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const newTitle = `${TITLE}-已改`
  const apiBefore = await apiCall(page, 'GET', `/admin/content/news/${seededIds[0]}`)
  const statusBefore = String(apiBefore.json?.data?.status ?? '')
  await page.getByLabel('标题', { exact: true }).fill(newTitle)
  await waitWrite(page, 'PUT', '/admin/content/news/', () => footer(page).getByRole('button', { name: '保存新闻' }).click())
  // toast 与弹窗关闭都是异步渲染，isVisible()/isHidden() 不等待 → 显式 waitFor 后再取样
  const toastLoc = page.locator('.ccai-neolab-toast', { hasText: '已保存' }).first()
  await toastLoc.waitFor({ state: 'visible', timeout: 8_000 }).catch(() => null)
  await shot(page, 'saved-toast')
  const toastVisible = await toastLoc.isVisible().catch(() => false)
  const toastText = await page.locator('.ccai-neolab-toast').allInnerTexts().catch(() => [])
  const closed = await dlg(page)
    .waitFor({ state: 'hidden', timeout: 10_000 })
    .then(() => true)
    .catch(() => false)
  const apiAfter = await apiCall(page, 'GET', `/admin/content/news/${seededIds[0]}`)
  const statusAfter = String(apiAfter.json?.data?.status ?? '')

  await reloadToTab(page, TAB)
  const search = page.locator('input[placeholder="搜索标题或摘要"]')
  await search.fill(newTitle)
  // 等查询请求真正返回再判断：isVisible() 不等待，全量跑时后端稍慢就会误判
  await Promise.all([
    page
      .waitForResponse(
        (r) => r.url().includes('/admin/content/news') && r.request().method() === 'GET',
        { timeout: 15_000 },
      )
      .catch(() => null),
    page.locator('.toolbar button', { hasText: '查询' }).click(),
  ])
  const rowVisible = await page
    .locator('table tbody tr', { hasText: newTitle })
    .first()
    .waitFor({ state: 'visible', timeout: 10_000 })
    .then(() => true)
    .catch(() => false)
  await shot(page, 'list-refreshed')
  record('NE-15', { newTitle, toastVisible, toastText, closed, statusBefore, statusAfter, rowVisible, titleInApi: apiAfter.json?.data?.title })
  expect.soft(closed, '保存后弹窗未关闭').toBe(true)
  expect.soft(toastVisible, `保存后顶部没有「已保存」提示（toast: ${JSON.stringify(toastText)}）`).toBe(true)
  expect.soft(apiAfter.json?.data?.title, '接口标题未更新').toBe(newTitle)
  expect.soft(rowVisible, '列表刷新后看不到新标题').toBe(true)
  expect.soft(statusAfter, `草稿保存后状态被改成 ${statusAfter}`).toBe(statusBefore)
  // 复原标题与正文，避免影响后续用例
  const restore = await apiCall(page, 'PUT', `/admin/content/news/${seededIds[0]}`, {
    sourceId: apiAfter.json?.data?.sourceId,
    slug: SLUG,
    locale: 'zh-CN',
    title: TITLE,
    summary: 'ACC-W1 摘要：用于新闻编辑弹窗验收',
    bodyHtml: RICH_BODY,
    bodyText: 'W1 小节标题 正文段落，含 加粗 与 斜体 与 外部链接。 无序一 无序二 有序一 引用块文字',
    bodySourceType: 'html_extract',
    sourceCanonicalUrl: `https://example.com/${SLUG}`,
    originalUrl: `https://example.com/${SLUG}`,
    sourceName: 'ACC-W1 来源',
    category: '模型发布',
    tags: ['AI', 'acc-w1'],
    status: statusBefore,
    publishMode: 'manual',
    publishedAt: new Date().toISOString(),
  })
  record('NE-15-restore', { status: restore.status, body: restore.text.slice(0, 200) })
  expectClean(guard(page))
})

test('NE-16 底部按钮顺序/尺寸/颜色 + 标题必填红星与失焦校验', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const m = await measureLayout(page)
  await shot(page, 'footer')
  const btnInfo = await page.evaluate(() => {
    const btns = [...document.querySelectorAll('.news-editor .modal-fixed-footer button')] as HTMLElement[]
    return btns.map((b) => {
      const cs = getComputedStyle(b)
      const r = b.getBoundingClientRect()
      return {
        text: (b.textContent || '').trim(),
        x: Math.round(r.x),
        h: Math.round(r.height),
        radius: cs.borderRadius,
        paddingX: cs.paddingLeft,
        bg: cs.backgroundColor,
        color: cs.color,
        border: cs.borderColor,
        cls: b.className,
      }
    })
  })
  record('NE-16', { footerButtons: m.footerButtons, btnInfo, titleStar: m.titleStar })

  expect.soft(m.footerButtons, `底部按钮应为 左「取消」右「保存」，实测 ${JSON.stringify(m.footerButtons)}`).toEqual(['取消', '保存'])
  const cancel = btnInfo.find((b) => b.text === '取消')
  const save = btnInfo.find((b) => b.text.includes('保存'))
  if (cancel && save) expect.soft(cancel.x, '「取消」不在「保存」左侧').toBeLessThan(save.x)
  for (const b of btnInfo) {
    expect.soft(b.h, `按钮「${b.text}」高 ${b.h}px，规范 36px`).toBeGreaterThanOrEqual(34)
    expect.soft(b.h, `按钮「${b.text}」高 ${b.h}px，规范 36px`).toBeLessThanOrEqual(38)
    expect.soft(b.radius, `按钮「${b.text}」圆角 ${b.radius}，规范 8px`).toBe('8px')
  }

  expect.soft(m.titleStar, '标题（必填）没有红星 *').toBe(true)

  // 清空标题后失焦，应即时提示
  const titleBox = page.getByLabel('标题', { exact: true })
  await titleBox.fill('')
  await page.locator('.news-editor h2').first().click()
  await page.waitForTimeout(600)
  const hint = await page.evaluate(() => {
    const root = document.querySelector('.news-editor') as HTMLElement
    const bad = root.querySelectorAll('.input-invalid,[role="alert"],.error,.field-error').length
    const text = root.innerText
    return { bad, hasHint: /标题.{0,8}(必填|不能为空|请输入)/.test(text) }
  })
  await shot(page, 'title-empty-blur')
  record('NE-16-title-empty', hint)
  expect.soft(hint.bad > 0 || hint.hasHint, '清空标题失焦后没有任何即时校验提示').toBe(true)
  expectClean(guard(page).filter((p) => !/admin\/content\/news/.test(p.detail)))
})

test('NE-17 输入框 2px/8 圆角 + 深色对比度 ≥4.5 + 编辑器可读', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  const light = await page.evaluate(() => {
    const label = [...document.querySelectorAll('.news-editor .form-grid > label')].find((l) => (l.textContent || '').trim().startsWith('标题'))!
    const input = label.querySelector('input') as HTMLElement
    const cs = getComputedStyle(input)
    const r = input.getBoundingClientRect()
    return { borderWidth: cs.borderTopWidth, radius: cs.borderRadius, h: Math.round(r.height), borderColor: cs.borderTopColor, bg: cs.backgroundColor, color: cs.color }
  })
  await shot(page, 'light-theme')
  record('NE-17-light', light)
  expect.soft(light.borderWidth, `输入框边框 ${light.borderWidth}，规范 2px`).toBe('2px')
  expect.soft(light.radius, `输入框圆角 ${light.radius}，规范 8px`).toBe('8px')

  // 深色开关在后台侧栏底部（AppSidebar 的 button.sidebar-link，文案 nav.darkMode/lightMode）；
  // 弹窗遮罩会挡住点击，所以先关弹窗 → 点侧栏开关 → 重开弹窗测量
  const setDark = async (on: boolean) => {
    const isDark = await page.evaluate(() => document.documentElement.classList.contains('dark'))
    if (isDark === on) return
    await page.locator('button.sidebar-link', { hasText: /深色模式|浅色模式|Dark Mode|Light Mode/ }).first().click()
    await page.waitForTimeout(500)
  }
  await footer(page).getByRole('button', { name: '取消' }).click()
  await dlg(page).waitFor({ state: 'hidden', timeout: 5000 }).catch(() => {})
  await setDark(true)
  await enter(page)
  await shot(page, 'dark-theme')
  const darkOn = await page.evaluate(() => document.documentElement.classList.contains('dark'))

  const dark = await page.evaluate(() => {
    const lum = (c: string) => {
      const m = c.match(/[\d.]+/g)
      if (!m) return null
      const [r, g, b] = m.slice(0, 3).map(Number).map((v) => {
        const s = v / 255
        return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
      })
      return 0.2126 * r + 0.7152 * g + 0.0722 * b
    }
    const contrast = (fg: string, bg: string) => {
      const a = lum(fg)
      const b = lum(bg)
      if (a === null || b === null) return 0
      return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)
    }
    const bgOf = (el: Element | null) => {
      let cur: Element | null = el
      while (cur) {
        const c = getComputedStyle(cur).backgroundColor
        if (c && !/rgba\(.*,\s*0\)/.test(c) && c !== 'transparent') return c
        cur = cur.parentElement
      }
      return 'rgb(255,255,255)'
    }
    const label = [...document.querySelectorAll('.news-editor .form-grid > label')].find((l) => (l.textContent || '').trim().startsWith('标题'))!
    const input = label.querySelector('input') as HTMLElement
    const editorEl = document.querySelector('[data-test="news-rich-editor"] .rich-text-content') as HTMLElement
    const toolbarBtn = document.querySelector('[data-test="news-rich-editor"] .rich-text-toolbar button') as HTMLElement
    const darkRows = [...document.querySelectorAll('.news-editor label, .news-editor .editor-label')].slice(0, 4).map((el) => ({
      text: (el.textContent || '').trim().slice(0, 12),
      fg: getComputedStyle(el).color,
      bg: bgOf(el),
      ratio: +contrast(getComputedStyle(el).color, bgOf(el)).toFixed(2),
    }))
    return {
      input: {
        fg: getComputedStyle(input).color,
        bg: bgOf(input),
        ratio: +contrast(getComputedStyle(input).color, bgOf(input)).toFixed(2),
        border: getComputedStyle(input).borderTopColor,
        bgLum: +(lum(bgOf(input)) ?? -1).toFixed(4),
      },
      editor: {
        fg: getComputedStyle(editorEl).color,
        bg: bgOf(editorEl),
        ratio: +contrast(getComputedStyle(editorEl).color, bgOf(editorEl)).toFixed(2),
        border: getComputedStyle(editorEl).borderTopColor,
        bgLum: +(lum(bgOf(editorEl)) ?? -1).toFixed(4),
      },
      toolbar: toolbarBtn
        ? { fg: getComputedStyle(toolbarBtn).color, bg: bgOf(toolbarBtn), ratio: +contrast(getComputedStyle(toolbarBtn).color, bgOf(toolbarBtn)).toFixed(2) }
        : null,
      labels: darkRows,
      bodyBg: getComputedStyle(document.body).backgroundColor,
    }
  })
  record('NE-17-dark', { darkOn, ...dark })
  expect.soft(darkOn, '点击主题按钮后 html 未加 .dark').toBe(true)
  // 深色主题必须真正覆盖弹窗：只加 .dark 类但输入框/正文区底色仍是浅色，等于深色模式不可用
  expect.soft(dark.input.bgLum, `深色模式下输入框背景仍为 ${dark.input.bg}（亮度 ${dark.input.bgLum}），深色主题未覆盖新闻编辑弹窗`).toBeLessThan(0.5)
  expect.soft(dark.editor.bgLum, `深色模式下正文区背景仍为 ${dark.editor.bg}（亮度 ${dark.editor.bgLum}），深色主题未覆盖编辑器`).toBeLessThan(0.5)
  expect.soft(dark.input.ratio, `深色下输入框文字对比度 ${dark.input.ratio}，应 ≥4.5（fg=${dark.input.fg}, bg=${dark.input.bg}）`).toBeGreaterThanOrEqual(4.5)
  expect.soft(dark.editor.ratio, `深色下正文区文字对比度 ${dark.editor.ratio}，应 ≥4.5（fg=${dark.editor.fg}, bg=${dark.editor.bg}）`).toBeGreaterThanOrEqual(4.5)
  expect.soft(dark.toolbar?.ratio ?? 0, `深色下工具栏按钮对比度 ${dark.toolbar?.ratio}，应 ≥4.5`).toBeGreaterThanOrEqual(4.5)
  for (const l of dark.labels) {
    expect.soft(l.ratio, `深色下标签「${l.text}」对比度 ${l.ratio} < 4.5`).toBeGreaterThanOrEqual(4.5)
  }

  // 守卫断言在还原主题前做（回首页会产生与新闻无关的请求噪声）
  expectClean(guard(page).filter((p) => !/\/api\/v1\/public\//.test(p.detail)))
  await footer(page).getByRole('button', { name: '取消' }).click()
  await dlg(page).waitFor({ state: 'hidden', timeout: 5000 }).catch(() => {})
  await setDark(false)
})

test('NE-18 Tab 焦点顺序 左栏字段→编辑器→取消→保存，且不逃出弹窗', async ({ page }, info) => {
  const shot = shooter('w1', info)
  await enter(page)
  await page.getByLabel('标题', { exact: true }).click()
  const seq: string[] = []
  const outside: string[] = []
  for (let i = 0; i < 140; i++) {
    await page.keyboard.press('Tab')
    const cur = await page.evaluate(() => {
      const el = document.activeElement as HTMLElement | null
      if (!el) return { desc: 'null', inDialog: false }
      const dlgEl = document.querySelector('.news-editor[role="dialog"]')
      const inDialog = !!(dlgEl && dlgEl.contains(el))
      const labelText = (el.closest('label')?.textContent || '').trim().slice(0, 16)
      const desc = `${el.tagName.toLowerCase()}${labelText ? `[${labelText}]` : ''}:${(el.getAttribute('aria-label') || el.textContent || '').trim().slice(0, 14)}`
      return { desc, inDialog }
    })
    seq.push(cur.desc)
    if (!cur.inDialog) outside.push(`${i + 1}:${cur.desc}`)
  }
  await shot(page, 'tab-sequence')
  record('NE-18', { seq, outside })
  const joined = seq.join(' | ')
  // 编辑器在 Tab 序列里表现为 div:新闻正文…（contenteditable 容器 aria-label 为「新闻正文」，class 不含 rich-text-content）
  const iEditor = seq.findIndex((s) => /rich-text-content|contenteditable|新闻正文/.test(s))
  const iCancel = seq.findIndex((s) => s.includes('取消'))
  const iSave = seq.findIndex((s) => s.includes('保存'))
  expect.soft(outside.length, `焦点跑出弹窗：${outside.join(', ')}`).toBe(0)
  expect.soft(iEditor, `Tab 未经过正文编辑器：${joined}`).toBeGreaterThan(0)
  expect.soft(iCancel, `Tab 未经过「取消」：${joined}`).toBeGreaterThan(0)
  expect.soft(iSave, `Tab 未经过「保存」：${joined}`).toBeGreaterThan(0)
  expect.soft(iCancel < iSave, `Tab 顺序应为 取消 → 保存，实测 取消@${iCancel} 保存@${iSave}`).toBe(true)
  const focusVisible = await page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null
    if (!el) return 'none'
    const cs = getComputedStyle(el)
    return `${cs.outlineStyle}/${cs.outlineWidth}/${cs.boxShadow.slice(0, 24)}`
  })
  record('NE-18-focus-ring', { focusVisible })
  // 契约口径：outline 或 box-shadow 可见（不为 none / 0）。Chrome 的 outline-style:auto 即为默认聚焦环，算可见
  expect.soft(/auto|solid|rgb|inset/.test(focusVisible) && !/^none\/0px/.test(focusVisible), `焦点不可见（outline/box-shadow = ${focusVisible}）`).toBe(true)
  expectClean(guard(page))
})

test('NE-19 保存后前台新闻详情渲染标题层级/列表/引用/图片/链接（浅+深）', async ({ page }, info) => {
  const shot = shooter('w1', info)
  // 先用接口把正文复原成 RICH_BODY（NE-10/11 保存过清洗后的内容），再发布
  const restore = await apiCall(page, 'PUT', `/admin/content/news/${seededIds[0]}`, {
    sourceId: 210,
    slug: SLUG,
    locale: 'zh-CN',
    title: TITLE,
    summary: 'ACC-W1 摘要：用于新闻编辑弹窗验收',
    bodyHtml: RICH_BODY,
    bodyText: 'W1 小节标题 正文段落，含 加粗 与 斜体 与 外部链接。 无序一 无序二 有序一 引用块文字',
    bodySourceType: 'html_extract',
    sourceCanonicalUrl: `https://example.com/${SLUG}`,
    originalUrl: `https://example.com/${SLUG}`,
    sourceName: 'ACC-W1 来源',
    category: '模型发布',
    tags: ['AI', 'acc-w1'],
    status: 'draft',
    publishMode: 'manual',
    publishedAt: new Date().toISOString(),
  })
  record('NE-19-restore', { status: restore.status, body: restore.text.slice(0, 200) })
  // 发布 ACC-W1 主新闻
  const pub = await apiCall(page, 'POST', `/admin/content/news/${seededIds[0]}/publish`, { publishAt: null })
  record('NE-19-publish', { status: pub.status, body: pub.text.slice(0, 200) })
  expect.soft(pub.status, `发布失败 ${pub.status}：${pub.text.slice(0, 200)}`).toBeLessThan(300)

  const url = `${BASE}/news/zh-CN/${SLUG}`
  // 先看匿名 JSON 到底下发什么（这是前台 SPA 的数据源）
  const pubJson = await page.request.get(`${BASE}/api/v1/public/news/zh-CN/${SLUG}`)
  const pubData = (await pubJson.json().catch(() => null))?.data ?? {}
  record('NE-19-public-api', {
    status: pubJson.status(),
    keys: Object.keys(pubData),
    hasBodyHtml: 'bodyHtml' in pubData,
    hasBodyText: 'bodyText' in pubData,
    bodyTextHead: String(pubData.bodyText ?? '').slice(0, 80),
  })
  await page.goto(url)
  await page.waitForFunction(() => {
    const t = document.body.innerText || ''
    return t.includes('W1 小节标题') || t.includes('引用块文字')
  }, { timeout: 20_000 }).catch(() => null)
  await shot(page, 'public-light')
  const dom = await page.evaluate(() => {
    const body = document.body
    const txt = body.innerText || ''
    const h2 = [...document.querySelectorAll('h2')].find((h) => (h.textContent || '').includes('W1 小节标题'))
    const li = [...document.querySelectorAll('li')].find((l) => (l.textContent || '').includes('无序一'))
    const bq = [...document.querySelectorAll('blockquote')].find((b) => (b.textContent || '').includes('引用块文字'))
    const a = [...document.querySelectorAll('a')].find((x) => (x.textContent || '').includes('外部链接'))
    const img = [...document.querySelectorAll('img')].find((x) => (x as HTMLImageElement).alt?.includes('W1 配图') || (x.getAttribute('src') || '').includes('picsum'))
    const strong = [...document.querySelectorAll('strong,b')].find((x) => (x.textContent || '').includes('加粗'))
    const desc = (el: Element | null) => {
      if (!el) return null
      const cs = getComputedStyle(el)
      const r = el.getBoundingClientRect()
      return { tag: el.tagName, fontSize: cs.fontSize, fontWeight: cs.fontWeight, color: cs.color, w: Math.round(r.width), h: Math.round(r.height), text: (el.textContent || '').trim().slice(0, 20) }
    }
    return {
      hasText: { h2: !!h2, li: !!li, bq: !!bq, a: !!a, img: !!img, strong: !!strong },
      h1s: [...document.querySelectorAll('h1')].map((h) => (h.textContent || '').trim().slice(0, 60)),
      h1: desc(document.querySelector('h1')),
      h2: desc(h2),
      li: desc(li),
      bq: desc(bq),
      a: desc(a),
      img: desc(img),
      strong: desc(strong),
      bodyLen: txt.length,
      titleInPage: txt.includes('主新闻'),
      dark: document.documentElement.classList.contains('dark'),
    }
  })
  record('NE-19-light', dom)

  for (const k of ['h2', 'li', 'bq', 'a', 'img'] as const) {
    expect.soft(dom.hasText[k], `前台详情缺 ${k} 渲染`) .toBe(true)
  }
  if (dom.h2 && dom.h1) {
    expect.soft(parseFloat(dom.h2.fontSize), `h2(${dom.h2.fontSize}) 未大于正文/未形成层级（h1=${dom.h1.fontSize}）`).toBeLessThan(parseFloat(dom.h1.fontSize))
  }
  if (dom.img) expect.soft(dom.img.w, '前台图片宽度为 0（未渲染）').toBeGreaterThan(20)

  // 前台深色
  await page.locator('button[aria-label*="深色"], button[aria-label*="主题"], button[title*="深色"]').first().click({ timeout: 5000 }).catch(async () => {
    await page.evaluate(() => document.documentElement.classList.add('dark'))
  })
  await page.waitForTimeout(500)
  await shot(page, 'public-dark')
  const darkDom = await page.evaluate(() => {
    const lum = (c: string) => {
      const m = c.match(/[\d.]+/g)
      if (!m) return null
      const [r, g, b] = m.slice(0, 3).map(Number).map((v) => {
        const s = v / 255
        return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
      })
      return 0.2126 * r + 0.7152 * g + 0.0722 * b
    }
    const contrast = (fg: string, bg: string) => {
      const a = lum(fg)
      const b = lum(bg)
      if (a === null || b === null) return 0
      return +(((Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)).toFixed(2))
    }
    const bgOf = (el: Element | null) => {
      let cur: Element | null = el
      while (cur) {
        const c = getComputedStyle(cur).backgroundColor
        if (c && !/rgba\(.*,\s*0\)/.test(c) && c !== 'transparent') return c
        cur = cur.parentElement
      }
      return 'rgb(255,255,255)'
    }
    const pick = (sel: string) => document.querySelector(sel)
    const targets = ['h1', 'h2', 'p', 'li', 'blockquote']
    return targets.map((s) => {
      const el = pick(s)
      if (!el) return { s, ratio: null }
      return { s, fg: getComputedStyle(el).color, bg: bgOf(el), ratio: contrast(getComputedStyle(el).color, bgOf(el)) }
    })
  })
  record('NE-19-dark', { dark: true, darkDom })
  for (const t of darkDom) {
    if (t.ratio === null) continue
    expect.soft(t.ratio as number, `前台深色下 ${t.s} 对比度 ${t.ratio} < 4.5（fg=${t.fg}, bg=${t.bg}）`).toBeGreaterThanOrEqual(4.5)
  }
  expectClean(guard(page).filter((p) => !/\/api\/v1\/public\//.test(p.detail)))
})
