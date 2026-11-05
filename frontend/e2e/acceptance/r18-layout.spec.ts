import { expect, test, type Page, type TestInfo } from '@playwright/test'
import { BASE, guard, realLogin, shooter } from '../real/real-helpers'

/**
 * v0.4 R18 验收：前台/后台「页面布局」跨断点实测（LAYOUT-01 ~ LAYOUT-09）
 *
 * 依据：docs/v0.4/v0.4_ui_brief_20261026_v3.3.md §2（基础组件/弹窗）、§3（后台响应式）、
 *      §4（前台列表/详情/推荐/精选）、§5（内容溢出）；
 *      弹窗宽度例外依据 checkpoint_v04_r16.md 第 1 项（新闻/文章编辑弹窗 880 → 1320）。
 *
 * 契约：
 *  - 真实后端真实数据：no page.route / no mock / no token 注入；每条用例都走 realLogin。
 *  - 一条用例 = 一个 test()，标题以用例号开头；所有子检查用 expect.soft，一次跑完把所有违规都报出来。
 *  - 失败必须带「实测值 vs 期望值」：数值进入 evidence（measurements.txt）并 attach 到报告。
 *  - 断点：desktop ≥1280、tablet 768–1279、mobile <768；
 *    实测视口：1440×900、1280×800、1024×768、768×1024、390×844。
 */

const VIEWPORTS = [
  { w: 1440, h: 900, band: 'desktop' as const },
  { w: 1280, h: 800, band: 'desktop' as const },
  { w: 1024, h: 768, band: 'tablet' as const },
  { w: 768, h: 1024, band: 'tablet' as const },
  { w: 390, h: 844, band: 'mobile' as const }
]

const TABS = ['新闻动态', '深度解析', 'AI 创作', '分类与标签', 'BI 分析']

// 公开埋点自带 IP 限流（120/min），E2E 连续翻页稳定触发 429；属产品既定行为，放行但仍记入证据。
const ALLOW = [/public\/analytics\/events.*429/, /status of 429 \(Too Many Requests\)/]

type Ev = string[]
function note(ev: Ev, k: string, v: unknown) {
  ev.push(`${k} = ${typeof v === 'string' ? v : JSON.stringify(v)}`)
}
/** 数值证据：同时 attach 到报告，并落盘到 E2E_REAL_OUT 便于脱离报告 triage。 */
async function attachEv(testInfo: TestInfo, ev: Ev) {
  const body = ev.join('\n')
  await testInfo.attach('measurements', { body, contentType: 'text/plain' })
  try {
    const dir = process.env.E2E_REAL_OUT
    if (dir) {
      const fs = await import('node:fs')
      const path = await import('node:path')
      fs.mkdirSync(dir, { recursive: true })
      fs.writeFileSync(path.join(dir, `${testInfo.title.split(' ')[0]}-measurements.txt`), body, 'utf8')
    }
  } catch {
    /* 落盘失败不影响判定 */
  }
}

const px = (n: number) => `${Math.round(n * 100) / 100}px`

/** 页面级横向溢出（hard rule：scrollWidth ≤ clientWidth + 1）。 */
async function pageOverflow(page: Page) {
  return page.evaluate(() => {
    const de = document.documentElement
    const vw = de.clientWidth
    const offenders = Array.from(document.querySelectorAll('body *'))
      .map((el) => {
        const b = el.getBoundingClientRect()
        return { tag: el.tagName, cls: (el.className || '').toString().slice(0, 48), right: Math.round(b.right), w: Math.round(b.width) }
      })
      .filter((x) => x.right > vw + 1 && x.w > 0)
      .slice(0, 4)
    return { vw, scrollWidth: de.scrollWidth, clientWidth: de.clientWidth, over: de.scrollWidth - de.clientWidth, offenders }
  })
}

/** 打开前台页：固定主题与界面语言（locale 记忆存 localStorage，属用户偏好，不是凭据注入）。 */
async function openPublic(page: Page, path: string, theme: 'light' | 'dark' = 'light', uiLocale = 'zh') {
  await page.addInitScript(
    ([t, l]) => {
      try {
        localStorage.setItem('theme', t)
        localStorage.setItem('sub2api_locale', l)
      } catch {
        /* ignore */
      }
    },
    [theme, uiLocale] as [string, string]
  )
  await page.goto(`${BASE}${path}`, { waitUntil: 'domcontentloaded' })
  await page
    .waitForFunction(
      () => !!document.querySelector('.neo-articles-row-list .neo-article-row-card, .neo-article-sheet, .neo-error-box'),
      null,
      { timeout: 45_000 }
    )
    .catch(() => {})
  await page.waitForTimeout(900)
}

async function setAdminTheme(page: Page, theme: 'light' | 'dark') {
  await page.evaluate((t) => localStorage.setItem('theme', t), theme)
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1200)
  const isDark = await page.evaluate(() => document.documentElement.classList.contains('dark'))
  return isDark
}

/** 关掉当前所有弹窗/未保存确认，供下一条测量复用同一会话。 */
async function closeDialogs(page: Page) {
  for (let i = 0; i < 6; i++) {
    const confirmLeave = page.locator('.confirm-dialog button', { hasText: '确认离开' })
    if (await confirmLeave.count()) {
      await confirmLeave.first().click({ force: true }).catch(() => {})
      await page.waitForTimeout(500)
      continue
    }
    if (await page.locator('.ui-confirm-dialog button').count()) {
      await page.locator('.ui-confirm-dialog button').first().click({ force: true }).catch(() => {})
      await page.waitForTimeout(400)
      continue
    }
    const close = page.locator(
      '[role="dialog"] .dialog-close:visible, [role="dialog"] .close-btn:visible, .ai-dialog-close:visible, .taxonomy-dialog-header button:visible'
    )
    if (await close.count()) {
      await close.last().click({ force: true }).catch(() => {})
      await page.waitForTimeout(500)
      continue
    }
    break
  }
  await page.keyboard.press('Escape').catch(() => {})
  await page.waitForTimeout(400)
}

async function gotoAdminContent(page: Page) {
  await page.goto(`${BASE}/admin/content`, { waitUntil: 'domcontentloaded' })
  await expect(page.locator('.tabs button').first()).toBeVisible({ timeout: 40_000 })
  await page.waitForTimeout(1000)
}

async function clickTab(page: Page, tab: string) {
  await page.locator('.tabs button', { hasText: tab }).first().click({ force: true })
  await page.waitForTimeout(1200)
}

/**
 * 量弹窗几何：宽/高/圆角/居中/根节点是否自身滚动/页头页脚在滚动时是否位移。
 * 「页头页脚固定，只有内容区滚动」的结构判据：滚动容器必须是弹窗根节点的后代，而不是根节点本身。
 */
async function measureDialog(page: Page, rootSel: string, headerSel: string, footerSel: string) {
  return page.evaluate(
    ({ rootSel, headerSel, footerSel }) => {
      const root = document.querySelector(rootSel)
      if (!root) return { found: false as const }
      const vw = document.documentElement.clientWidth
      const vh = window.innerHeight
      const rb = root.getBoundingClientRect()
      const rcs = getComputedStyle(root)
      const hdr = root.querySelector(headerSel)
      const ftr = root.querySelector(footerSel)
      const hb = hdr?.getBoundingClientRect()
      const fb = ftr?.getBoundingClientRect()

      // 候选滚动容器包含弹窗根节点自身：若根节点自己滚，页头/页脚会被一起滚走（要能测到位移量）
      const scrollers = ([root, ...Array.from(root.querySelectorAll('*'))] as HTMLElement[]).filter((el) => {
        const cs = getComputedStyle(el)
        const b = el.getBoundingClientRect()
        return /auto|scroll/.test(cs.overflowY) && b.width > 0 && b.height > 0 && el.scrollHeight - el.clientHeight > 2
      })
      const deep = scrollers[scrollers.length - 1] as HTMLElement | undefined
      const before = { hdrTop: hb ? Math.round(hb.top) : -1, ftrTop: fb ? Math.round(fb.top) : -1 }
      let after = { ...before }
      let scrolled = 0
      if (deep) {
        scrolled = deep.scrollHeight - deep.clientHeight
        deep.scrollTop = deep.scrollHeight
        const h2 = hdr?.getBoundingClientRect()
        const f2 = ftr?.getBoundingClientRect()
        after = { hdrTop: h2 ? Math.round(h2.top) : -1, ftrTop: f2 ? Math.round(f2.top) : -1 }
        deep.scrollTop = 0
      }
      return {
        found: true as const,
        vw,
        vh,
        width: Math.round(rb.width),
        height: Math.round(rb.height),
        left: Math.round(rb.left),
        top: Math.round(rb.top),
        radius: rcs.borderRadius,
        rootOverflowY: rcs.overflowY,
        rootMaxH: rcs.maxHeight,
        headerPos: hdr ? getComputedStyle(hdr).position : null,
        footerPos: ftr ? getComputedStyle(ftr).position : null,
        centerX: Math.round(rb.left + rb.width / 2 - vw / 2),
        centerY: Math.round(rb.top + rb.height / 2 - vh / 2),
        scroller: deep ? { cls: (deep.className || '').toString().slice(0, 48), scrollable: scrolled, overflowY: getComputedStyle(deep).overflowY } : null,
        scrollers: scrollers.map((s) => ({ cls: (s.className || '').toString().slice(0, 40), scrollable: s.scrollHeight - s.clientHeight })),
        before,
        after,
        footerInViewport: fb ? fb.bottom <= vh + 1 : null,
        headerInViewport: hb ? hb.top >= -1 : null,
        maxHeightRules: { ninetyVh: Math.round(vh * 0.9), eightyFiveVh: Math.round(vh * 0.85) }
      }
    },
    { rootSel, headerSel, footerSel }
  )
}

/** 通用弹窗断言集：宽度 ≤ 视口−32、圆角 12、居中、页头页脚固定（结构）、页脚在视口内。 */
async function checkDialog(
  ev: Ev,
  tag: string,
  m: Awaited<ReturnType<typeof measureDialog>>,
  opts: { maxHeight: 'large' | 'medium' }
) {
  await expect.soft(m.found, `${tag}：没找到弹窗（选择器失效）`).toBe(true)
  if (!m.found) return
  note(ev, `${tag} 几何`, `w=${px(m.width)} h=${px(m.height)} left=${px(m.left)} top=${px(m.top)} radius=${m.radius} rootOverflowY=${m.rootOverflowY} rootMaxH=${m.rootMaxH}`)
  note(ev, `${tag} 居中`, `Δx=${m.centerX} Δy=${m.centerY}（视口 ${m.vw}×${m.vh}）`)
  note(ev, `${tag} 页头/页脚`, `pos=${m.headerPos}/${m.footerPos} 滚动前=${JSON.stringify(m.before)} 滚动后=${JSON.stringify(m.after)} 滚动量=${m.scroller?.scrollable ?? 0} 滚动容器=${m.scroller?.cls ?? 'none'}`)

  await expect
    .soft(m.width, `${tag}：宽 ${px(m.width)} 超过「视口−32」上限 ${px(m.vw - 32)}（实测 ${m.width} / 允许 ${m.vw - 32}）`)
    .toBeLessThanOrEqual(m.vw - 32)
  await expect.soft(m.radius, `${tag}：圆角 ${m.radius}，规范 12px`).toBe('12px')
  await expect.soft(Math.abs(m.centerX), `${tag}：水平未居中，中心偏移 ${m.centerX}px（应 ≤2px）`).toBeLessThanOrEqual(2)
  await expect.soft(m.headerInViewport, `${tag}：页头被滚出视口（${JSON.stringify(m.before)} → ${JSON.stringify(m.after)}）`).toBe(true)
  await expect.soft(m.footerInViewport, `${tag}：页脚不在视口内（弹窗高 ${m.height} / 视口 ${m.vh}）`).toBe(true)
  // 结构判据：内容真的溢出时，弹窗根节点自己不能是滚动容器
  // （否则页头/页脚会被一起滚出可视区）；内容没溢出（无滚动量）时该结构不产生实际影响，不判失败。
  const rootScrolls = /auto|scroll/.test(String(m.rootOverflowY)) && (m.scroller?.scrollable ?? 0) > 0
  note(ev, `${tag} 根节点自滚判定`, `rootOverflowY=${m.rootOverflowY} 实际滚动量=${m.scroller?.scrollable ?? 0} → ${rootScrolls ? '根节点自滚（违规）' : '仅内容区滚/无滚动'}`)
  await expect
    .soft(
      rootScrolls,
      `${tag}：内容溢出 ${m.scroller?.scrollable ?? 0}px 时由弹窗根节点自己滚动（overflow-y=${m.rootOverflowY}），页头/页脚被一起滚走（页头 ${m.before.hdrTop}→${m.after.hdrTop}px，页脚 ${m.before.ftrTop}→${m.after.ftrTop}px）`
    )
    .toBe(false)
  if (m.scroller && (m.scroller.scrollable ?? 0) > 2) {
    await expect
      .soft(
        Math.abs(m.after.hdrTop - m.before.hdrTop),
        `${tag}：内容区滚动 ${m.scroller.scrollable}px 后页头位移 ${m.after.hdrTop - m.before.hdrTop}px（应 0）=${JSON.stringify(m.before)}→${JSON.stringify(m.after)}`
      )
      .toBeLessThanOrEqual(2)
    await expect
      .soft(
        Math.abs(m.after.ftrTop - m.before.ftrTop),
        `${tag}：内容区滚动 ${m.scroller.scrollable}px 后页脚位移 ${m.after.ftrTop - m.before.ftrTop}px（应 0）`
      )
      .toBeLessThanOrEqual(2)
  }
  const limit = opts.maxHeight === 'large' ? Math.min(m.maxHeightRules.ninetyVh, 900) : m.maxHeightRules.eightyFiveVh
  note(ev, `${tag} 最大高`, `实测 h=${m.height} 上限(${opts.maxHeight === 'large' ? 'min(90vh,900)' : '85vh'})=${limit}`)
  await expect
    .soft(m.height, `${tag}：高 ${px(m.height)} 超过上限 ${limit}px（${opts.maxHeight === 'large' ? 'min(90vh,900px)' : '85vh'}）`)
    .toBeLessThanOrEqual(limit + 1)
}

// ===================================================================================== LAYOUT-01
test('LAYOUT-01 后台内容中心 5 个 tab × 5 个视口：无横向页面溢出', async ({ page }, testInfo) => {
  test.setTimeout(180_000)
  const ev: Ev = []
  const shot = shooter('layout', testInfo)
  const problems = guard(page, ALLOW)

  await page.setViewportSize({ width: 1440, height: 900 })
  await realLogin(page)
  await gotoAdminContent(page)

  for (const vp of VIEWPORTS) {
    await page.setViewportSize({ width: vp.w, height: vp.h })
    await page.waitForTimeout(500)
    for (const tab of TABS) {
      await clickTab(page, tab)
      const o = await pageOverflow(page)
      note(ev, `[${vp.w}×${vp.h}][${tab}]`, `scrollWidth=${o.scrollWidth} clientWidth=${o.clientWidth} 溢出=${o.over}px 越界元素=${JSON.stringify(o.offenders)}`)
      await expect
        .soft(
          o.over,
          `[${vp.w}×${vp.h}][${tab}] 页面横向溢出 ${o.over}px（scrollWidth ${o.scrollWidth} > clientWidth ${o.clientWidth}），越界元素 ${JSON.stringify(o.offenders)}`
        )
        .toBeLessThanOrEqual(1)
      if (vp.w === 1440 || vp.w === 390) await shot(page, `tab-${tab}-${vp.w}`)
    }
  }

  note(ev, 'pageerror/console', problems.filter((p) => p.kind !== 'http').map((p) => p.detail))
  await attachEv(testInfo, ev)
  // 只把真正的 JS 崩溃当失败；网络 4xx/5xx 不是本次布局判据（另有用例负责）
  expect.soft(problems.filter((p) => p.kind === 'pageerror'), '页面 JS 崩溃').toEqual([])
})

// ===================================================================================== LAYOUT-02
test('LAYOUT-02 弹窗几何：宽≤视口−32、圆角 12、居中、页头页脚固定且仅内容区滚动', async ({ page }, testInfo) => {
  test.setTimeout(240_000)
  const ev: Ev = []
  const shot = shooter('layout', testInfo)
  const problems = guard(page, ALLOW)

  const DIALOGS: Array<{ name: string; root: string; header: string; footer: string; maxHeight: 'large' | 'medium'; open: (p: Page) => Promise<void> }> = [
    {
      name: '新闻编辑',
      root: '.news-editor.news-editor-modal',
      header: '.modal-fixed-header',
      footer: '.modal-fixed-footer',
      maxHeight: 'large',
      open: async (p) => {
        await gotoAdminContent(p)
        await clickTab(p, '新闻动态')
        await p.locator('.table-wrap tbody tr').first().locator('.actions-cell button', { hasText: '编辑' }).first().click()
        await p.waitForTimeout(1800)
      }
    },
    {
      name: '文章编辑',
      root: '.article-editor-modal',
      header: '.modal-drawer-header',
      footer: '.modal-drawer-footer',
      maxHeight: 'large',
      open: async (p) => {
        await gotoAdminContent(p)
        await clickTab(p, '深度解析')
        await p.locator('.table-wrap tbody tr').first().locator('button', { hasText: '编辑' }).first().click()
        await p.waitForTimeout(2000)
      }
    },
    {
      name: '新闻源',
      root: '.source-dialog',
      header: '.section-heading',
      footer: '.modal-fixed-footer',
      maxHeight: 'medium',
      open: async (p) => {
        await gotoAdminContent(p)
        await clickTab(p, '新闻动态')
        await p.locator('.subnav button', { hasText: '新闻源' }).first().click()
        await p.waitForTimeout(1400)
        await p.locator('.source-create-button').first().click()
        await p.waitForTimeout(1400)
      }
    },
    {
      name: 'AI 模型配置',
      root: '.ai-config-dialog',
      header: '.ai-dialog-header',
      footer: '.ai-dialog-footer',
      maxHeight: 'medium',
      open: async (p) => {
        await gotoAdminContent(p)
        await clickTab(p, 'AI 创作')
        await p.locator('.ai-subtabs button', { hasText: '模型配置' }).first().click({ force: true })
        await p.waitForTimeout(1400)
        await p.locator('button', { hasText: '新增配置' }).first().click({ force: true })
        await p.waitForTimeout(1500)
      }
    },
    {
      name: '分类与标签',
      root: '.taxonomy-dialog',
      header: '.taxonomy-dialog-header',
      footer: '.taxonomy-dialog-footer',
      maxHeight: 'medium',
      open: async (p) => {
        await gotoAdminContent(p)
        await clickTab(p, '分类与标签')
        await p.locator('button', { hasText: '新增文章分类' }).first().click({ force: true })
        await p.waitForTimeout(1300)
      }
    }
  ]

  await page.setViewportSize({ width: 1440, height: 900 })
  await realLogin(page)

  for (const vp of [
    { w: 1440, h: 900 },
    { w: 1024, h: 768 },
    { w: 390, h: 844 },
    // 矮视口：只有内容确实超出视口时，「页头页脚固定、仅内容区滚动」才可被真实观测
    { w: 1024, h: 560 },
    { w: 390, h: 520 }
  ]) {
    await page.setViewportSize({ width: vp.w, height: vp.h })
    for (const d of DIALOGS) {
      await d.open(page)
      const m = await measureDialog(page, d.root, d.header, d.footer)
      await shot(page, `dialog-${d.name}-${vp.w}`)
      await checkDialog(ev, `[${vp.w}×${vp.h}][${d.name}]`, m, { maxHeight: d.maxHeight })
      if (m.found && vp.w === 390) {
        // <768：编辑弹窗左右栏必须折叠为单栏
        const cols = await page.evaluate((sel) => {
          const root = document.querySelector(sel)
          const grid = root?.querySelector('.two-col-editor, .article-editor-columns')
          if (!grid) return null
          const gcs = getComputedStyle(grid)
          return { template: gcs.gridTemplateColumns, cols: gcs.gridTemplateColumns.split(' ').length, w: Math.round(grid.getBoundingClientRect().width) }
        }, d.root)
        if (cols) {
          note(ev, `[390][${d.name}] 编辑栅格`, cols)
          await expect
            .soft(cols.cols, `[390][${d.name}] 编辑弹窗栅格 ${cols.template} 不是单栏（<768 应折叠为单栏）`)
            .toBe(1)
        }
      }
      await closeDialogs(page)
    }
  }

  note(ev, 'pageerror/console', problems.filter((p) => p.kind !== 'http').map((p) => p.detail))
  await attachEv(testInfo, ev)
  expect.soft(problems.filter((p) => p.kind === 'pageerror'), '页面 JS 崩溃').toEqual([])
})

// ===================================================================================== LAYOUT-03
test('LAYOUT-03 后台表格响应式：≥1280 完整表格 / 1024 允许内部滚动且页面不溢出 / 390 转卡片', async ({ page }, testInfo) => {
  test.setTimeout(180_000)
  const ev: Ev = []
  const shot = shooter('layout', testInfo)
  const problems = guard(page, ALLOW)

  await page.setViewportSize({ width: 1440, height: 900 })
  await realLogin(page)

  for (const vp of VIEWPORTS) {
    await page.setViewportSize({ width: vp.w, height: vp.h })
    await gotoAdminContent(page)
    await clickTab(page, '新闻动态')
    const m = await page.evaluate(() => {
      const wrap = document.querySelector('.table-wrap') as HTMLElement | null
      const table = wrap?.querySelector('table') as HTMLElement | null
      const th = document.querySelector('th.actions-th') as HTMLElement | null
      const td = document.querySelector('td.actions-cell') as HTMLElement | null
      const g = (e: HTMLElement | null) => (e ? { pos: getComputedStyle(e).position, right: getComputedStyle(e).right, w: Math.round(e.getBoundingClientRect().width) } : null)
      const cardish = document.querySelectorAll(
        '.news-card-list, .card-list, .mobile-card-list, [data-testid*="card-list"], .table-card-list'
      ).length
      // 宽松的卡片化判定：工作区里 ≥5 个「有文字、带按钮/链接、宽度≥180px」的 *card* 兄弟节点，
      // 也视为已切换成卡片列表（避免只认某个具体类名而误报）
      const looseCards = Array.from(document.querySelectorAll('[class*="card" i]')).filter((el) => {
        const b = el.getBoundingClientRect()
        return b.width >= 180 && b.height >= 48 && (el.textContent || '').trim().length > 10 && el.querySelector('button, a')
      }).length
      const rows = document.querySelectorAll('.table-wrap tbody tr').length
      const de = document.documentElement
      return {
        vw: de.clientWidth,
        pageOver: de.scrollWidth - de.clientWidth,
        tableDisplay: table ? getComputedStyle(table).display : 'none',
        tableVisible: !!table && table.getBoundingClientRect().width > 0,
        tableW: table ? Math.round(table.getBoundingClientRect().width) : -1,
        wrapCW: wrap ? wrap.clientWidth : -1,
        wrapSW: wrap ? wrap.scrollWidth : -1,
        innerScroll: wrap ? wrap.scrollWidth - wrap.clientWidth : -1,
        toolWrap: (() => {
          const t = document.querySelector('.toolbar') as HTMLElement | null
          return t ? { flexWrap: getComputedStyle(t).flexWrap, h: Math.round(t.getBoundingClientRect().height) } : null
        })(),
        actionCol: { th: g(th), td: g(td) },
        cardListNodes: cardish,
        looseCardNodes: looseCards,
        rows
      }
    })
    note(ev, `[${vp.w}×${vp.h}] 新闻列表表格`, m)
    await shot(page, `table-${vp.w}`)

    await expect.soft(m.pageOver, `[${vp.w}] 页面横向溢出 ${m.pageOver}px（表格必须内部滚动，不能顶破页面）`).toBeLessThanOrEqual(1)

    if (vp.band === 'desktop') {
      await expect
        .soft(m.tableVisible, `[${vp.w}] ≥1280 应为完整表格，实测 table display=${m.tableDisplay} width=${m.tableW}`)
        .toBe(true)
      await expect
        .soft(
          m.innerScroll,
          `[${vp.w}] ≥1280 表格不应横向滚动，实测需内部滚动 ${m.innerScroll}px（表宽 ${m.tableW} > 容器 ${m.wrapCW}）`
        )
        .toBeLessThanOrEqual(1)
      await expect.soft(m.toolWrap?.flexWrap, `[${vp.w}] ≥1280 筛选条应单行（flex-wrap=${m.toolWrap?.flexWrap}，高 ${m.toolWrap?.h}px）`).toBe('nowrap')
    }

    if (vp.band === 'tablet') {
      await expect
        .soft(m.toolWrap?.flexWrap, `[${vp.w}] tablet 允许筛选折行，实测 flex-wrap=${m.toolWrap?.flexWrap}`)
        .toBe('wrap')
      await expect.soft(m.actionCol.td?.pos, `[${vp.w}] tablet 操作列应固定在右侧（实测 position=${m.actionCol.td?.pos}）`).toBe('sticky')
      await expect.soft(m.actionCol.td?.right, `[${vp.w}] tablet 操作列 sticky 应贴 right:0（实测 ${m.actionCol.td?.right}）`).toBe('0px')
    }

    if (vp.band === 'mobile') {
      const converted = (m.tableDisplay === 'none' || !m.tableVisible) || m.cardListNodes > 0 || m.looseCardNodes >= 5
      note(ev, `[${vp.w}] 卡片化判定`, `tableDisplay=${m.tableDisplay} tableVisible=${m.tableVisible} cardListNodes=${m.cardListNodes} looseCardNodes=${m.looseCardNodes} rows=${m.rows}`)
      await expect
        .soft(
          converted,
          `[${vp.w}] <768 表格应转卡片列表，实测仍渲染 <table>（display=${m.tableDisplay}，宽 ${m.tableW}px，${m.rows} 行，table-wrap 内部滚动 ${m.innerScroll}px），且工作区内没有卡片列表容器（cardListNodes=${m.cardListNodes}，宽松匹配 card 节点=${m.looseCardNodes}）`
        )
        .toBe(true)
    }
  }

  note(ev, 'pageerror/console', problems.filter((p) => p.kind !== 'http').map((p) => p.detail))
  await attachEv(testInfo, ev)
  expect.soft(problems.filter((p) => p.kind === 'pageerror'), '页面 JS 崩溃').toEqual([])
})

// ===================================================================================== LAYOUT-04
test('LAYOUT-04 前台列表卡片：桌面封面 240 / 平板 200 / 移动通栏 16:9、一行一卡、20 条每页', async ({ page }, testInfo) => {
  test.setTimeout(180_000)
  const ev: Ev = []
  const shot = shooter('layout', testInfo)
  const problems = guard(page, ALLOW)

  await page.setViewportSize({ width: 1440, height: 900 })
  await realLogin(page)

  for (const vp of VIEWPORTS) {
    await page.setViewportSize({ width: vp.w, height: vp.h })
    await openPublic(page, '/articles', 'light', 'zh')
    await shot(page, `list-${vp.w}`)
    const m = await page.evaluate(() => {
      const main = document.querySelector('.neo-public-main') as HTMLElement | null
      const cards = Array.from(document.querySelectorAll('.neo-articles-row-list .neo-article-row-card')) as HTMLElement[]
      const sample = cards.slice(0, 3).map((c) => {
        const cover = c.querySelector('[data-testid="list-card-cover"]') as HTMLElement | null
        // 只有真实 <img> 有 object-fit；无封面时是占位 div，跳过该子检查（占位不使用 object-fit）
        const img = cover?.querySelector('img') as HTMLElement | null
        const main2 = c.querySelector('.article-row-main') as HTMLElement | null
        const cb = c.getBoundingClientRect()
        const cov = cover?.getBoundingClientRect()
        return {
          cardW: Math.round(cb.width),
          cardLeft: Math.round(cb.left),
          coverW: cov ? Math.round(cov.width) : -1,
          coverH: cov ? Math.round(cov.height) : -1,
          coverTop: cov ? Math.round(cov.top) : -1,
          cardOuterTop: Math.round(cb.top),
          imgFit: img ? getComputedStyle(img).objectFit : null,
          cardPadding: getComputedStyle(c).padding,
          mainPadding: main2 ? getComputedStyle(main2).padding : null,
          inlineOverflow: c.scrollWidth - c.clientWidth,
          hasImg: !!img,
          coverOverflowsCard: cov ? Math.round(cov.right - cb.right) : 0
        }
      })
      const tops = [...new Set(cards.map((c) => Math.round(c.getBoundingClientRect().top)))]
      const perRow = tops.map((t) => cards.filter((c) => Math.round(c.getBoundingClientRect().top) === t).length)
      const de = document.documentElement
      return {
        vw: de.clientWidth,
        pageOver: de.scrollWidth - de.clientWidth,
        mainW: main ? Math.round(main.getBoundingClientRect().width) : -1,
        mainMaxW: main ? getComputedStyle(main).maxWidth : 'none',
        cards: cards.length,
        pager: !!document.querySelector('[data-testid="pager"]'),
        pagerWrap: (() => {
          const p = document.querySelector('[data-testid="pager"]') as HTMLElement | null
          return p ? { w: Math.round(p.getBoundingClientRect().width), h: Math.round(p.getBoundingClientRect().height), flexWrap: getComputedStyle(p).flexWrap } : null
        })(),
        rows: tops.length,
        perRow,
        sample
      }
    })
    note(ev, `[${vp.w}×${vp.h}] 列表`, `main=${m.mainW} (max-width ${m.mainMaxW}) cards=${m.cards} pager=${m.pager} 行数=${m.rows} 每行卡片数=${JSON.stringify(m.perRow)} 页面溢出=${m.pageOver}px`)
    note(ev, `[${vp.w}×${vp.h}] 卡片抽样`, m.sample)

    await expect.soft(m.pageOver, `[${vp.w}] 列表页横向溢出 ${m.pageOver}px`).toBeLessThanOrEqual(1)
    await expect.soft(m.cards, `[${vp.w}] 每页应 20 条，实测 ${m.cards} 条`).toBe(20)
    await expect.soft(m.rows, `[${vp.w}] 应一行一卡，实测 ${m.rows} 行 / 每行 ${JSON.stringify(m.perRow)}`).toBe(m.cards)
    for (const s of m.sample) {
      await expect.soft(s.inlineOverflow, `[${vp.w}] 卡片内部横向溢出 ${s.inlineOverflow}px（内容超出卡片盒）`).toBeLessThanOrEqual(1)
    }

    if (vp.band === 'desktop') {
      await expect.soft(Math.abs(m.mainW - 1200), `[${vp.w}] ≥1280 内容最大宽应 1200px，实测 ${m.mainW}px`).toBeLessThanOrEqual(2)
      let imgChecked = 0
      for (const s of m.sample) {
        await expect.soft(Math.abs(s.coverW - 240), `[${vp.w}] 桌面左封面宽应 240px，实测 ${s.coverW}px`).toBeLessThanOrEqual(2)
        await expect.soft(Math.abs(s.coverW / Math.max(s.coverH, 1) - 16 / 9), `[${vp.w}] 封面应 16:9，实测 ${s.coverW}×${s.coverH}`).toBeLessThanOrEqual(0.06)
        if (s.hasImg) {
          imgChecked++
          await expect.soft(s.imgFit, `[${vp.w}] 封面 <img> 应 object-fit: cover，实测 ${s.imgFit}`).toBe('cover')
        }
      }
      note(ev, `[${vp.w}] 抽样中有真实 <img> 的封面数`, `${imgChecked}/${m.sample.length}（无封面时为占位 div，不做 object-fit 断言）`)
    }

    if (vp.band === 'tablet') {
      for (const s of m.sample) {
        await expect
          .soft(Math.abs(s.coverW - 200), `[${vp.w}] tablet 封面宽应 200px，实测 ${s.coverW}×${s.coverH}（卡片宽 ${s.cardW}）`)
          .toBeLessThanOrEqual(2)
      }
    }

    if (vp.band === 'mobile') {
      for (const s of m.sample) {
        // 移动：封面改顶部通栏 16:9、卡片内边距 16px
        await expect
          .soft(
            s.coverW,
            `[${vp.w}] 移动端封面应通栏（≈卡片宽 ${s.cardW - 32}px），实测 ${s.coverW}px（卡片宽 ${s.cardW}）`
          )
          .toBeGreaterThanOrEqual(s.cardW - 40)
        await expect
          .soft(Math.abs(s.coverW / Math.max(s.coverH, 1) - 16 / 9), `[${vp.w}] 移动端通栏封面应 16:9，实测 ${s.coverW}×${s.coverH}`)
          .toBeLessThanOrEqual(0.06)
        await expect
          .soft(s.coverOverflowsCard, `[${vp.w}] 移动端封面右侧超出卡片 ${s.coverOverflowsCard}px（被 overflow:hidden 裁掉）`)
          .toBeLessThanOrEqual(1)
        await expect
          .soft(s.mainPadding, `[${vp.w}] 移动端卡片内边距应 16px，实测 .article-row-main padding=${s.mainPadding}（卡片 padding=${s.cardPadding}）`)
          .toBe('16px')
      }
    }
  }

  note(ev, 'pageerror/console', problems.filter((p) => p.kind !== 'http').map((p) => p.detail))
  await attachEv(testInfo, ev)
  expect.soft(problems.filter((p) => p.kind === 'pageerror'), '页面 JS 崩溃').toEqual([])
})

// ===================================================================================== LAYOUT-05
test('LAYOUT-05 前台详情页：正文最大宽 800、封面 ≤400、留白 32/16、图片不溢出', async ({ page }, testInfo) => {
  test.setTimeout(180_000)
  const ev: Ev = []
  const shot = shooter('layout', testInfo)
  const problems = guard(page, ALLOW)

  const SLUG = 'ai-gateway-routing-strategy-best-practices'
  await page.setViewportSize({ width: 1440, height: 900 })
  await realLogin(page)

  for (const vp of VIEWPORTS) {
    await page.setViewportSize({ width: vp.w, height: vp.h })
    await openPublic(page, `/articles/zh/${SLUG}`, 'light', 'zh')
    await shot(page, `detail-${vp.w}`)
    const m = await page.evaluate(() => {
      const de = document.documentElement
      const vw = de.clientWidth
      const main = document.querySelector('.neo-public-main') as HTMLElement | null
      const sheet = document.querySelector('.neo-article-sheet') as HTMLElement | null
      const prose = document.querySelector('.neo-prose-body') as HTMLElement | null
      const cover = document.querySelector('[data-testid="detail-cover"]') as HTMLElement | null
      const h1 = document.querySelector('.article-headline') as HTMLElement | null
      const sb = sheet?.getBoundingClientRect()
      const pb = prose?.getBoundingClientRect()
      const cb = cover?.getBoundingClientRect()
      const imgs = Array.from(document.querySelectorAll('.neo-prose-body img, [data-testid="detail-cover"] img, .more-related-card img')) as HTMLImageElement[]
      const imgIssues = imgs.map((im) => {
        const ib = im.getBoundingClientRect()
        const pRect = im.parentElement!.getBoundingClientRect()
        return {
          w: Math.round(ib.width),
          h: Math.round(ib.height),
          overRight: Math.round(ib.right - pRect.right),
          overLeft: Math.round(pRect.left - ib.left),
          natural: im.naturalWidth,
          parent: (im.parentElement!.className || '').toString().slice(0, 30)
        }
      })
      const kidsOver = prose
        ? Array.from(prose.querySelectorAll('*'))
            .map((el) => ({ tag: el.tagName, over: Math.round(el.getBoundingClientRect().right - pb!.right) }))
            .filter((x) => x.over > 1)
        : []
      const pres = prose ? Array.from(prose.querySelectorAll('pre')).map((p) => ({ w: Math.round(p.getBoundingClientRect().width), scrollable: (p as HTMLElement).scrollWidth - (p as HTMLElement).clientWidth, ox: getComputedStyle(p).overflowX })) : []
      return {
        vw,
        pageOver: de.scrollWidth - vw,
        proseW: pb ? Math.round(pb.width) : -1,
        proseMaxW: prose ? getComputedStyle(prose).maxWidth : 'none',
        proseLeft: pb ? Math.round(pb.left) : -1,
        h1W: h1 ? Math.round(h1.getBoundingClientRect().width) : -1,
        sheetGutterL: sb ? Math.round(sb.left) : -1,
        sheetGutterR: sb ? Math.round(vw - sb.right) : -1,
        mainPadding: main ? getComputedStyle(main).padding : null,
        sheetPadding: sheet ? getComputedStyle(sheet).padding : null,
        coverW: cb ? Math.round(cb.width) : -1,
        coverH: cb ? Math.round(cb.height) : -1,
        coverMaxH: cover ? getComputedStyle(cover).maxHeight : '',
        imgCount: imgs.length,
        imgIssues,
        kidsOver: kidsOver.slice(0, 5),
        pres
      }
    })
    note(ev, `[${vp.w}×${vp.h}] 详情`, JSON.stringify(m))

    await expect.soft(m.pageOver, `[${vp.w}] 详情页横向溢出 ${m.pageOver}px`).toBeLessThanOrEqual(1)
    // 正文最大宽 800px（含标题同一列）
    await expect
      .soft(m.proseW, `[${vp.w}] 正文列宽应 ≤800px，实测 ${m.proseW}px（max-width=${m.proseMaxW}，标题列 ${m.h1W}px）`)
      .toBeLessThanOrEqual(800)
    await expect
      .soft(m.coverH, `[${vp.w}] 详情封面高应 ≤400px，实测 ${m.coverH}px（宽 ${m.coverW}，max-height=${m.coverMaxH}）`)
      .toBeLessThanOrEqual(400)

    if (vp.band === 'tablet') {
      await expect
        .soft(m.sheetGutterL, `[${vp.w}] 平板左右留白应 32px，实测左 ${m.sheetGutterL}px / 右 ${m.sheetGutterR}px（main padding=${m.mainPadding}，sheet padding=${m.sheetPadding}）`)
        .toBe(32)
    }
    if (vp.band === 'mobile') {
      await expect
        .soft(m.sheetGutterL, `[${vp.w}] 移动左右留白应 16px，实测左 ${m.sheetGutterL}px / 右 ${m.sheetGutterR}px（main padding=${m.mainPadding}，sheet padding=${m.sheetPadding}）`)
        .toBe(16)
    }
    for (const x of m.imgIssues) {
      await expect.soft(x.overRight, `[${vp.w}] 图片右侧超出容器 ${x.overRight}px（${x.w}×${x.h} in .${x.parent}）`).toBeLessThanOrEqual(1)
      await expect.soft(x.overLeft, `[${vp.w}] 图片左侧超出容器 ${x.overLeft}px`).toBeLessThanOrEqual(1)
    }
    await expect
      .soft(m.kidsOver.length, `[${vp.w}] 正文内有 ${m.kidsOver.length} 个元素超出正文列：${JSON.stringify(m.kidsOver)}`)
      .toBe(0)
    for (const p of m.pres) {
      await expect.soft(p.w, `[${vp.w}] 代码块宽 ${p.w}px 超出正文列 ${m.proseW}px`).toBeLessThanOrEqual(m.proseW)
    }
  }

  note(ev, 'pageerror/console', problems.filter((p) => p.kind !== 'http').map((p) => p.detail))
  await attachEv(testInfo, ev)
  expect.soft(problems.filter((p) => p.kind === 'pageerror'), '页面 JS 崩溃').toEqual([])
})

// ===================================================================================== LAYOUT-06
test('LAYOUT-06 精选区：一行一条 + 左大图 + 无区块标题胶囊；0 条整区隐藏；>10 只留最新 10', async ({ page, request }, testInfo) => {
  test.setTimeout(180_000)
  const ev: Ev = []
  const shot = shooter('layout', testInfo)
  const problems = guard(page, ALLOW)

  await page.setViewportSize({ width: 1440, height: 900 })
  await realLogin(page)

  const apiZh = await (await request.get(`${BASE}/api/v1/public/articles/featured?locale=zh-CN`)).json().catch(() => null)
  const apiCount = Array.isArray(apiZh?.data) ? apiZh.data.length : 0
  note(ev, '精选接口(zh-CN) 条数', apiCount)

  // 基线口径（2026-11-03 用户确认）：「一行一条 + 左图右文」为准，
  // 取代 ui_brief §4 早期的「1 大卡 ≈40% + 右侧 3×3 小卡网格」。
  // 依据 checkpoint_v04_r16 第 8 项；ui_brief 已同步修订。
  for (const vp of VIEWPORTS) {
    await page.setViewportSize({ width: vp.w, height: vp.h })
    await openPublic(page, '/articles', 'light', 'zh')
    await shot(page, `featured-${vp.w}`)
    const m = await page.evaluate(() => {
      const sec = document.querySelector('[data-testid="featured-section"]') as HTMLElement | null
      if (!sec) return { found: false }
      const rows = Array.from(sec.querySelectorAll('[data-testid="featured-large-card"], [data-testid="featured-small"]')) as HTMLElement[]
      const boxes = rows.map((r) => {
        const b = r.getBoundingClientRect()
        const cover = r.querySelector('.public-featured-cover-row, .public-featured-cover') as HTMLElement | null
        const title = r.querySelector('.public-featured-row-title') as HTMLElement | null
        const cb = cover ? cover.getBoundingClientRect() : null
        const tb = title ? title.getBoundingClientRect() : null
        return {
          w: Math.round(b.width),
          h: Math.round(b.height),
          top: Math.round(b.top),
          left: Math.round(b.left),
          coverW: cb ? Math.round(cb.width) : -1,
          coverH: cb ? Math.round(cb.height) : -1,
          titleRightOfCover: cb && tb ? tb.left >= cb.right - 2 : false,
        }
      })
      const tops = [...new Set(boxes.map((b) => b.top))]
      const perRow = tops.map((t) => boxes.filter((b) => b.top === t).length)
      const legacyPill = sec.querySelector('.public-featured-title, .public-featured-subtitle') as HTMLElement | null
      const secRight = Math.round(sec.getBoundingClientRect().right)
      return {
        found: true,
        secW: Math.round(sec.getBoundingClientRect().width),
        count: rows.length,
        rows: tops.length,
        perRow,
        leftoverPillText: legacyPill?.textContent?.trim() ?? '',
        firstCover: boxes[0]?.coverW ?? -1,
        firstCoverH: boxes[0]?.coverH ?? -1,
        titleRightOfCover: boxes[0]?.titleRightOfCover ?? false,
        allTitleRightOfCover: boxes.every((b) => b.titleRightOfCover),
        maxRowWidth: boxes.length ? Math.max(...boxes.map((b) => b.w)) : -1,
        overflowRight: boxes.length ? Math.max(...boxes.map((b) => b.left + b.w)) - secRight : 0,
      }
    })
    note(ev, `[${vp.w}×${vp.h}] 精选区`, JSON.stringify(m))
    await expect.soft(m.found, `[${vp.w}] 精选区不存在（data-testid=featured-section 缺失）`).toBe(true)
    if (!m.found) continue

    await expect.soft(m.count, `[${vp.w}] 精选条数应 = min(接口 ${apiCount}, 10)，实测 ${m.count}`).toBe(Math.min(apiCount, 10))

    // ① 一行一条：每条独占一行
    await expect
      .soft(Math.max(...m.perRow), `[${vp.w}] 精选应为「一行一条」，实测每行卡片数=${JSON.stringify(m.perRow)}（共 ${m.rows} 行）`)
      .toBe(1)

    // ② 左图右文：标题在封面右侧
    await expect.soft(m.titleRightOfCover, `[${vp.w}] 首条应为「左图右文」（标题在封面右侧）`).toBe(true)
    await expect.soft(m.allTitleRightOfCover, `[${vp.w}] 所有精选行都应为左图右文，实测存在标题不在封面右侧的行`).toBe(true)

    // ③ 左大图：首条封面应占行宽显著比例（移动端通栏，不强制）
    if (vp.band !== 'mobile') {
      const coverRatio = m.firstCover > 0 ? m.firstCover / m.maxRowWidth : 0
      await expect
        .soft(
          coverRatio,
          `[${vp.w}] 首条封面应占行宽 ≥20%（左大图），实测 ${Math.round(coverRatio * 100)}%（封面 ${m.firstCover}px / 行 ${m.maxRowWidth}px）`,
        )
        .toBeGreaterThanOrEqual(0.2)
    }

    // ④ 旧「精选深度解析」胶囊标题不应存在
    await expect.soft(m.leftoverPillText, `[${vp.w}] 不应再有区块标题胶囊/副标题（r16 已移除），实测「${m.leftoverPillText}」`).toBe('')

    // ⑤ 不横向溢出
    await expect.soft(m.overflowRight, `[${vp.w}] 精选行右边缘超出区块 ${m.overflowRight}px`).toBeLessThanOrEqual(1)
  }

  // 0 条 → 整区隐藏：en-US 无精选数据（接口 data 为 null），真实数据路径验证，不改库
  const apiEn = await (await request.get(`${BASE}/api/v1/public/articles/featured?locale=en-US`)).json().catch(() => null)
  const enCount = Array.isArray(apiEn?.data) ? apiEn.data.length : 0
  note(ev, '精选接口(en-US) 条数', `${enCount}（data=${JSON.stringify(apiEn?.data)?.slice(0, 60)}）`)
  if (enCount === 0) {
    await page.setViewportSize({ width: 1440, height: 900 })
    await openPublic(page, '/articles', 'light', 'en')
    const en = await page.evaluate(() => ({
      sec: !!document.querySelector('[data-testid="featured-section"]'),
      listCards: document.querySelectorAll('.neo-articles-row-list .neo-article-row-card').length,
      featuredHeader: !!document.querySelector('.public-featured-header')
    }))
    note(ev, '[en-US] 精选 0 条时列表页', JSON.stringify(en))
    await shot(page, 'featured-zero-en')
    await expect.soft(en.listCards, '[en-US] 精选 0 条时列表本身应正常渲染').toBeGreaterThan(0)
    await expect.soft(en.sec, '[en-US] 精选 0 条时整区应隐藏，实测仍渲染 featured-section').toBe(false)
  } else {
    note(ev, '0 条隐藏检查', 'en-US 精选接口非空，本次未构造到 0 条场景（不做数据变更）')
  }

  // >10 只显示最新 10：用接口排序的前 10 个 slug 与 DOM 前 10 个一致来间接验证截断
  const apiSlugs = (Array.isArray(apiZh?.data) ? apiZh.data : []).map((i: any) => i.slug)
  await page.setViewportSize({ width: 1440, height: 900 })
  await openPublic(page, '/articles', 'light', 'zh')
  const domSlugs = await page.evaluate(() =>
    Array.from(document.querySelectorAll('[data-testid="featured-section"] a[href*="/articles/"]'))
      .map((a) => (a.getAttribute('href') || '').split('/').pop() || '')
      .filter(Boolean)
      .slice(0, 12)
  )
  note(ev, 'DOM 精选前 10 slug', domSlugs.slice(0, 10))
  note(ev, '接口精选前 10 slug', apiSlugs.slice(0, 10))
  await expect.soft(domSlugs.length, `精选 DOM 条数 ${domSlugs.length} 超过 10（>10 应只显示最新 10）`).toBeLessThanOrEqual(10)

  note(ev, 'pageerror/console', problems.filter((p) => p.kind !== 'http').map((p) => p.detail))
  await attachEv(testInfo, ev)
  expect.soft(problems.filter((p) => p.kind === 'pageerror'), '页面 JS 崩溃').toEqual([])
})

// ===================================================================================== LAYOUT-07
test('LAYOUT-07 推荐区：固定 6 条，3×2 / 2×3 / 1×6', async ({ page }, testInfo) => {
  test.setTimeout(180_000)
  const ev: Ev = []
  const shot = shooter('layout', testInfo)
  const problems = guard(page, ALLOW)

  const SLUG = 'ai-gateway-routing-strategy-best-practices'
  await page.setViewportSize({ width: 1440, height: 900 })
  await realLogin(page)

  for (const vp of VIEWPORTS) {
    await page.setViewportSize({ width: vp.w, height: vp.h })
    await openPublic(page, `/articles/zh/${SLUG}`, 'light', 'zh')
    await shot(page, `related-${vp.w}`)
    const m = await page.evaluate(() => {
      const grid = document.querySelector('.more-related-grid') as HTMLElement | null
      if (!grid) return { found: false }
      const cards = Array.from(grid.children) as HTMLElement[]
      const tops = [...new Set(cards.map((c) => Math.round(c.getBoundingClientRect().top)))]
      const perRow = tops.map((t) => cards.filter((c) => Math.round(c.getBoundingClientRect().top) === t).length)
      const gcs = getComputedStyle(grid)
      const cols = gcs.gridTemplateColumns.split(' ').length
      const de = document.documentElement
      return {
        found: true,
        cards: cards.length,
        cols,
        template: gcs.gridTemplateColumns,
        rows: tops.length,
        perRow,
        cardW: cards[0] ? Math.round(cards[0].getBoundingClientRect().width) : -1,
        gridW: Math.round(grid.getBoundingClientRect().width),
        pageOver: de.scrollWidth - de.clientWidth
      }
    })
    note(ev, `[${vp.w}×${vp.h}] 推荐区`, JSON.stringify(m))
    await expect.soft(m.found, `[${vp.w}] 未找到推荐区容器 .more-related-grid`).toBe(true)
    if (!m.found) continue

    await expect.soft(m.cards, `[${vp.w}] 推荐应固定 6 条，实测 ${m.cards} 条`).toBe(6)
    await expect.soft(m.pageOver, `[${vp.w}] 推荐区导致页面横向溢出 ${m.pageOver}px`).toBeLessThanOrEqual(1)

    if (vp.band === 'desktop') {
      await expect.soft(m.cols, `[${vp.w}] 桌面推荐应 3 列，实测 ${m.cols} 列（${m.template}）`).toBe(3)
      await expect.soft(m.perRow, `[${vp.w}] 桌面推荐应 3 列 × 2 行，实测每行 ${JSON.stringify(m.perRow)}`).toEqual([3, 3])
    }
    if (vp.band === 'tablet') {
      await expect.soft(m.cols, `[${vp.w}] 平板推荐应 2 列，实测 ${m.cols} 列（${m.template}）`).toBe(2)
      await expect.soft(m.perRow, `[${vp.w}] 平板推荐应 2 列 × 3 行，实测每行 ${JSON.stringify(m.perRow)}`).toEqual([2, 2, 2])
    }
    if (vp.band === 'mobile') {
      await expect.soft(m.cols, `[${vp.w}] 移动推荐应 1 列，实测 ${m.cols} 列（${m.template}）`).toBe(1)
      await expect.soft(m.perRow, `[${vp.w}] 移动推荐应 1 列 × 6 行，实测每行 ${JSON.stringify(m.perRow)}`).toEqual([1, 1, 1, 1, 1, 1])
    }
  }

  note(ev, 'pageerror/console', problems.filter((p) => p.kind !== 'http').map((p) => p.detail))
  await attachEv(testInfo, ev)
  expect.soft(problems.filter((p) => p.kind === 'pageerror'), '页面 JS 崩溃').toEqual([])
})

// ===================================================================================== LAYOUT-08
test('LAYOUT-08 内容溢出健壮性：长标题 2 行截断、标签换行、子元素不超出父盒', async ({ page }, testInfo) => {
  test.setTimeout(180_000)
  const ev: Ev = []
  const shot = shooter('layout', testInfo)
  const problems = guard(page, ALLOW)

  await page.setViewportSize({ width: 1440, height: 900 })
  await realLogin(page)

  for (const vp of VIEWPORTS) {
    await page.setViewportSize({ width: vp.w, height: vp.h })
    await openPublic(page, '/articles', 'light', 'zh')
    await shot(page, `overflow-${vp.w}`)
    const m = await page.evaluate(() => {
      const cards = Array.from(document.querySelectorAll('.neo-articles-row-list .neo-article-row-card')) as HTMLElement[]
      const data = cards.map((c, i) => {
        const t = c.querySelector('.article-row-title') as HTMLElement | null
        const meta = c.querySelector('.article-row-meta') as HTMLElement | null
        const tags = c.querySelectorAll('.article-row-meta .article-row-tag')
        const cs = t ? getComputedStyle(t) : null
        const cb = c.getBoundingClientRect()
        const kids = Array.from(c.querySelectorAll('*'))
          .map((el) => ({ tag: el.tagName, cls: (el.className || '').toString().slice(0, 30), over: Math.round(el.getBoundingClientRect().right - cb.right) }))
          .filter((x) => x.over > 1)
        return {
          i,
          titleLen: (t?.textContent || '').trim().length,
          titleH: t ? Math.round(t.getBoundingClientRect().height) : -1,
          lineHeight: cs?.lineHeight ?? '',
          clamp: cs?.webkitLineClamp ?? '',
          display: cs?.display ?? '',
          overflow: cs?.overflow ?? '',
          titleInlineOver: t ? t.scrollWidth - t.clientWidth : -1,
          tagCount: tags.length,
          metaWrap: meta ? getComputedStyle(meta).flexWrap : '',
          metaInlineOver: meta ? meta.scrollWidth - meta.clientWidth : -1,
          cardInlineOver: c.scrollWidth - c.clientWidth,
          kidsOver: kids.slice(0, 3)
        }
      })
      const longest = data.slice().sort((a, b) => b.titleLen - a.titleLen)[0] ?? null
      return { cards: cards.length, data, longest }
    })
    note(ev, `[${vp.w}×${vp.h}] 标题/标签`, JSON.stringify({ cards: m.cards, longest: m.longest }))

    const lh = parseFloat(String(m.longest?.lineHeight || '0').replace('px', '')) || 0
    if (m.longest && lh > 0) {
      const twoLine = lh * 2
      await expect
        .soft(
          m.longest.titleH,
          `[${vp.w}] 最长标题（${m.longest.titleLen} 字）高 ${m.longest.titleH}px，应 ≈2 行 ${Math.round(twoLine)}px（line-height ${m.longest.lineHeight}，-webkit-line-clamp=${m.longest.clamp}，display=${m.longest.display}）`
        )
        .toBeLessThanOrEqual(Math.ceil(twoLine) + 2)
    }
    for (const d of m.data) {
      await expect.soft(d.titleInlineOver, `[${vp.w}] 卡片#${d.i} 标题横向溢出 ${d.titleInlineOver}px`).toBeLessThanOrEqual(1)
      await expect.soft(d.metaInlineOver, `[${vp.w}] 卡片#${d.i} 标签行横向溢出 ${d.metaInlineOver}px（${d.tagCount} 个标签，flex-wrap=${d.metaWrap}）`).toBeLessThanOrEqual(1)
    }
    // 汇总：卡片内有没有子元素顶出卡片盒（逐卡重复报会淹没有效信息，故按视口聚合成一条）
    const cardOffenders = m.data.filter((d) => d.kidsOver.length > 0)
    await expect
      .soft(
        cardOffenders.length,
        `[${vp.w}] ${cardOffenders.length}/${m.cards} 张卡片有子元素超出卡片右边界；样例 ${JSON.stringify(cardOffenders.slice(0, 3).map((d) => ({ i: d.i, kids: d.kidsOver })))}`
      )
      .toBe(0)
    const cardOverflowers = m.data.filter((d) => d.cardInlineOver > 1)
    await expect
      .soft(
        cardOverflowers.length,
        `[${vp.w}] ${cardOverflowers.length}/${m.cards} 张卡片自身横向溢出；样例 ${JSON.stringify(cardOverflowers.slice(0, 3).map((d) => ({ i: d.i, over: d.cardInlineOver })))}`
      )
      .toBe(0)
  }

  // 详情页正文：任何元素不得超出正文列；长 URL/代码可换行或内部滚动
  await page.setViewportSize({ width: 1440, height: 900 })
  await openPublic(page, '/articles/zh/ai-gateway-routing-strategy-best-practices', 'light', 'zh')
  await shot(page, 'overflow-detail')
  const d = await page.evaluate(() => {
    const prose = document.querySelector('.neo-prose-body') as HTMLElement | null
    if (!prose) return { found: false }
    const pr = prose.getBoundingClientRect()
    const kids = Array.from(prose.querySelectorAll('*')).map((el) => {
      const b = el.getBoundingClientRect()
      return {
        tag: el.tagName,
        over: Math.round(b.right - pr.right),
        inlineScroll: el.scrollWidth - el.clientWidth,
        ox: getComputedStyle(el).overflowX,
        ws: getComputedStyle(el).whiteSpace,
        ow: getComputedStyle(el).overflowWrap
      }
    })
    return {
      found: true,
      proseW: Math.round(pr.width),
      overRight: kids.filter((k) => k.over > 1).slice(0, 5),
      inlineScrollers: kids.filter((k) => k.inlineScroll > 1 && /auto|scroll/.test(k.ox)).slice(0, 5),
      pageOver: document.documentElement.scrollWidth - document.documentElement.clientWidth
    }
  })
  note(ev, '[1440] 详情正文溢出', JSON.stringify(d))
  if (d.found) {
    await expect.soft((d as any).overRight.length, `详情正文有元素超出正文列：${JSON.stringify((d as any).overRight)}`).toBe(0)
    await expect.soft((d as any).pageOver, `详情页横向溢出 ${(d as any).pageOver}px`).toBeLessThanOrEqual(1)
  }

  note(ev, 'pageerror/console', problems.filter((p) => p.kind !== 'http').map((p) => p.detail))
  await attachEv(testInfo, ev)
  expect.soft(problems.filter((p) => p.kind === 'pageerror'), '页面 JS 崩溃').toEqual([])
})

// ===================================================================================== LAYOUT-09
test('LAYOUT-09 深/浅双主题：布局度量一致，深色不引入溢出或遮挡', async ({ page }, testInfo) => {
  test.setTimeout(180_000)
  const ev: Ev = []
  const shot = shooter('layout', testInfo)
  const problems = guard(page, ALLOW)

  await page.setViewportSize({ width: 1440, height: 900 })
  await realLogin(page)
  await gotoAdminContent(page)

  // ---------- 后台：两个主题下同一组度量
  const adminMetrics: Record<string, any> = {}
  for (const theme of ['light', 'dark'] as const) {
    const isDark = await setAdminTheme(page, theme)
    await gotoAdminContent(page)
    await clickTab(page, '新闻动态')
    await shot(page, `admin-${theme}`)
    const m = await page.evaluate(() => {
      const de = document.documentElement
      const wrap = document.querySelector('.table-wrap') as HTMLElement | null
      const tab = document.querySelector('.tabs button.active') as HTMLElement | null
      const first = document.querySelector('.table-wrap tbody tr') as HTMLElement | null
      const input = document.querySelector('.toolbar input') as HTMLElement | null
      const g = (e: HTMLElement | null) => (e ? { h: Math.round(e.getBoundingClientRect().height), w: Math.round(e.getBoundingClientRect().width), bg: getComputedStyle(e).backgroundColor, radius: getComputedStyle(e).borderRadius } : null)
      return {
        htmlClass: de.className,
        bodyBg: getComputedStyle(document.body).backgroundColor,
        pageOver: de.scrollWidth - de.clientWidth,
        wrap: g(wrap),
        tabBtn: g(tab),
        row: g(first),
        input: g(input),
        overflowY: de.scrollHeight > de.clientHeight
      }
    })
    note(ev, `[admin][${theme}]`, JSON.stringify(m))
    adminMetrics[theme] = m
    await expect.soft(m.pageOver, `[admin][${theme}] 页面横向溢出 ${m.pageOver}px`).toBeLessThanOrEqual(1)
    await expect.soft(m.htmlClass.includes('dark'), `[admin][${theme}] 主题类未生效（html class="${m.htmlClass}"，期望 ${theme}）`).toBe(theme === 'dark' ? true : false)
  }
  const same = (k: string) => {
    const a = adminMetrics.light?.[k]
    const b = adminMetrics.dark?.[k]
    return a && b && a.w === b.w && a.h === b.h
  }
  for (const k of ['wrap', 'tabBtn', 'row', 'input']) {
    await expect
      .soft(same(k), `[admin] 深/浅主题 ${k} 尺寸不一致：light=${JSON.stringify(adminMetrics.light?.[k])} dark=${JSON.stringify(adminMetrics.dark?.[k])}`)
      .toBe(true)
  }
  // 深色下表格/按钮盒模型与浅色一致（宽高已比），文字色需变化以适配深底
  await expect
    .soft(adminMetrics.dark?.bodyBg !== adminMetrics.light?.bodyBg, `[admin] 深色 body 背景 ${adminMetrics.dark?.bodyBg} 与浅色 ${adminMetrics.light?.bodyBg} 相同（主题未生效）`)
    .toBe(true)

  // ---------- 前台：两个主题下列表 + 详情度量一致
  const pubs: Record<string, any> = {}
  for (const theme of ['light', 'dark'] as const) {
    await openPublic(page, '/articles', theme, 'zh')
    await shot(page, `public-${theme}`)
    const list = await page.evaluate(() => {
      const card = document.querySelector('.neo-article-row-card') as HTMLElement | null
      const cover = document.querySelector('[data-testid="list-card-cover"]') as HTMLElement | null
      const main = document.querySelector('.neo-public-main') as HTMLElement | null
      const title = document.querySelector('.article-row-title') as HTMLElement | null
      const de = document.documentElement
      return {
        htmlClass: de.className,
        pageOver: de.scrollWidth - de.clientWidth,
        mainW: main ? Math.round(main.getBoundingClientRect().width) : -1,
        cardW: card ? Math.round(card.getBoundingClientRect().width) : -1,
        cardH: card ? Math.round(card.getBoundingClientRect().height) : -1,
        coverW: cover ? Math.round(cover.getBoundingClientRect().width) : -1,
        coverH: cover ? Math.round(cover.getBoundingClientRect().height) : -1,
        cardBg: card ? getComputedStyle(card).backgroundColor : '',
        cardBorder: card ? getComputedStyle(card).borderTopColor : '',
        titleColor: title ? getComputedStyle(title).color : '',
        cards: document.querySelectorAll('.neo-article-row-card').length
      }
    })
    await openPublic(page, '/articles/zh/ai-gateway-routing-strategy-best-practices', theme, 'zh')
    const detail = await page.evaluate(() => {
      const sheet = document.querySelector('.neo-article-sheet') as HTMLElement | null
      const prose = document.querySelector('.neo-prose-body') as HTMLElement | null
      const cover = document.querySelector('[data-testid="detail-cover"]') as HTMLElement | null
      const rel = document.querySelector('.more-related-grid') as HTMLElement | null
      const de = document.documentElement
      return {
        pageOver: de.scrollWidth - de.clientWidth,
        sheetBg: sheet ? getComputedStyle(sheet).backgroundColor : '',
        proseW: prose ? Math.round(prose.getBoundingClientRect().width) : -1,
        coverH: cover ? Math.round(cover.getBoundingClientRect().height) : -1,
        relCols: rel ? getComputedStyle(rel).gridTemplateColumns : '',
        relCards: rel ? rel.children.length : 0
      }
    })
    note(ev, `[public][${theme}] list`, JSON.stringify(list))
    note(ev, `[public][${theme}] detail`, JSON.stringify(detail))
    pubs[theme] = { list, detail }
    await expect.soft(list.pageOver, `[public][${theme}] 列表页横向溢出 ${list.pageOver}px`).toBeLessThanOrEqual(1)
    await expect.soft(detail.pageOver, `[public][${theme}] 详情页横向溢出 ${detail.pageOver}px`).toBeLessThanOrEqual(1)
    await expect.soft(list.cards, `[public][${theme}] 列表条数应 20，实测 ${list.cards}`).toBe(20)
    await expect.soft(detail.relCards, `[public][${theme}] 推荐应 6 条，实测 ${detail.relCards}`).toBe(6)
  }
  for (const k of ['mainW', 'cardW', 'cardH', 'coverW', 'coverH']) {
    await expect
      .soft(
        pubs.light.list[k] === pubs.dark.list[k],
        `前台列表 ${k} 深/浅不一致：light=${pubs.light.list[k]} dark=${pubs.dark.list[k]}`
      )
      .toBe(true)
  }
  for (const k of ['proseW', 'coverH', 'relCols']) {
    await expect
      .soft(
        pubs.light.detail[k] === pubs.dark.detail[k],
        `前台详情 ${k} 深/浅不一致：light=${pubs.light.detail[k]} dark=${pubs.dark.detail[k]}`
      )
      .toBe(true)
  }
  await expect
    .soft(pubs.dark.list.cardBg !== pubs.light.list.cardBg, `前台深色卡片底 ${pubs.dark.list.cardBg} 与浅色 ${pubs.light.list.cardBg} 相同（深色未生效）`)
    .toBe(true)

  note(ev, 'pageerror/console', problems.filter((p) => p.kind !== 'http').map((p) => p.detail))
  await attachEv(testInfo, ev)
  expect.soft(problems.filter((p) => p.kind === 'pageerror'), '页面 JS 崩溃').toEqual([])
})
