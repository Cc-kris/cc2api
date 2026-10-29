import { expect, test, type Page, type TestInfo } from '@playwright/test'
import { BASE, EMAIL, PASSWORD, expectClean, guard, openContentTab, realLogin, shooter, stamp } from '../real/real-helpers'

/**
 * v0.4 W5 验收：前台内容站（PUB-01 ~ PUB-16）
 * 依据：docs/v0.4/v0.4_prd_20261026_v3.3.md（R7/R8/R9/R10）、
 *      docs/v0.4/v0.4_e2e_acceptance_cases_20261028.md（W5 表）
 * 契约：真实后端真实数据，无 route/mock/token 注入；一条用例 = 一个 test；子检查用 expect.soft。
 *
 * 双宿主说明（实测口径，报告里逐条注明）：
 *  - SSR 宿主 http://127.0.0.1:8080：Go 进程内嵌前端 + 前台 SSR（/news 301→/zh/news、/en/news 200）。
 *    PUB-01/02/03/05/11 这类「HTTP/SSR 源码」用例在此宿主判定。
 *  - SPA 宿主 http://127.0.0.1:3100：Vite 开发服务，前台交互 UI（精选区/分页/语言切换/埋点）只有这里能跑 JS。
 *    PUB-04/06/07/08/09/10/12/15/16 及 PUB-13/14 的前台部分在此宿主判定。
 */

const SSR = process.env.S2A_SSR_BASE ?? 'http://127.0.0.1:8080'
const LANGS = ['English', '中文', 'Русский', 'Français', 'Deutsch']
const SLUG_EN = 'ccai-api-practice' // 有已发布英文译文（content_translations status=published, locale=en-US）
const SLUG_ZH_ONLY = 'task031-featured-1-7713902' // 仅中文正文

// 公开埋点接口自带限流（public-analytics-ip 120/min）。E2E 连续翻页会稳定触发 429，
// 这是产品既定行为而非功能错误，故从网络守卫白名单放行，但仍记录到证据里。
const ANALYTICS_429 = [/public\/analytics\/events.*429/, /status of 429 \(Too Many Requests\)/]
const pubGuard = (page: Page) => guard(page, ANALYTICS_429)

/** 翻页取某语种列表全量（后端 page_size 有上限，不能一次拿完）。 */
async function allItems(request: any, loc: string): Promise<{ total: number; items: any[] }> {
  const out: any[] = []
  let total = 0
  for (let p = 1; p <= 50; p++) {
    const d = await (await request.get(`${SSR}/api/v1/public/articles?locale=${loc}&page=${p}&page_size=50`)).json()
    const items = d?.data?.items ?? []
    total = d?.data?.total ?? total
    out.push(...items)
    if (!items.length || out.length >= total) break
  }
  return { total, items: out }
}

type Ev = string[]

function note(ev: Ev, k: string, v: unknown) {
  ev.push(`${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`)
}

async function attachEv(testInfo: TestInfo, ev: Ev) {
  await testInfo.attach('measured', { body: ev.join('\n'), contentType: 'text/plain' })
}

/** 打开 SPA 公开页并固定主题（theme 存 localStorage，前台 detectTheme 读取）。 */
async function openPublic(page: Page, path: string, theme: 'light' | 'dark' = 'light') {
  await page.addInitScript((t) => {
    try {
      localStorage.setItem('theme', t)
    } catch {
      /* ignore */
    }
  }, theme)
  await page.goto(`${BASE}${path}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(1500)
}

/** WCAG 相对亮度 / 对比度，供页面内 evaluate 复用（作为字符串注入）。 */
const CONTRAST_SRC = `
function parseRGB(s){
  const m = String(s).match(/rgba?\\(([^)]+)\\)/)
  if(!m) return null
  const p = m[1].split(',').map(x=>parseFloat(x))
  return { r:p[0], g:p[1], b:p[2], a: p.length>3 ? p[3] : 1 }
}
function lum(c){
  const f = (v)=>{ v/=255; return v<=0.03928 ? v/12.92 : Math.pow((v+0.055)/1.055,2.4) }
  return 0.2126*f(c.r)+0.7152*f(c.g)+0.0722*f(c.b)
}
function ratio(fg,bg){
  const a=lum(fg), b=lum(bg)
  const hi=Math.max(a,b), lo=Math.min(a,b)
  return (hi+0.05)/(lo+0.05)
}
function effBg(el){
  let n = el
  while(n && n !== document.documentElement){
    const cs = getComputedStyle(n)
    const c = parseRGB(cs.backgroundColor)
    if(c && c.a > 0.05) return c
    n = n.parentElement
  }
  const c = parseRGB(getComputedStyle(document.body).backgroundColor)
  return c && c.a > 0.05 ? c : { r:255, g:255, b:255, a:1 }
}
`

// ---------------------------------------------------------------- PUB-01
test('PUB-01 语言前缀直访 200 / 无前缀 301', async ({ request }, testInfo) => {
  const ev: Ev = []
  const cases = ['/en/news', '/zh/articles', '/en/', '/ru/news', '/fr/articles', '/de/articles']
  for (const p of cases) {
    const r = await request.get(`${SSR}${p}`, { maxRedirects: 0, failOnStatusCode: false })
    note(ev, `直访 ${p}`, `status=${r.status()} loc=${r.headers()['location'] ?? ''}`)
    await expect.soft(r.status(), `${p} 应 200（不能 404）`).toBe(200)
  }
  for (const p of ['/news', '/articles']) {
    const r = await request.get(`${SSR}${p}`, { maxRedirects: 0, failOnStatusCode: false })
    const loc = r.headers()['location'] ?? ''
    note(ev, `无前缀 ${p}`, `status=${r.status()} loc=${loc}`)
    await expect.soft(r.status(), `${p} 应 301`).toBe(301)
    await expect.soft(loc, `${p} 应跳 /zh/...`).toContain('/zh/')
  }
  await attachEv(testInfo, ev)
})

// ---------------------------------------------------------------- PUB-02
test('PUB-02 /articles/en/<译文slug> 源码含英文正文（SSR）', async ({ request }, testInfo) => {
  const ev: Ev = []
  const res = await request.get(`${SSR}/articles/en/${SLUG_EN}`, { failOnStatusCode: false })
  const html = await res.text()
  note(ev, 'status', res.status())
  note(ev, 'html lang', (html.match(/<html[^>]*lang="([^"]*)"/) ?? [])[1] ?? 'none')
  const h1 = (html.match(/<h1[^>]*>([\s\S]*?)<\/h1>/) ?? [])[1] ?? ''
  const bodyHtml = (html.match(/<section class="content">([\s\S]*?)<\/section>/) ?? [])[1] ?? ''
  const body = bodyHtml.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
  note(ev, 'h1', h1.trim())
  note(ev, '正文前 120 字', body.slice(0, 120))
  const cjk = (body.match(/[\u4e00-\u9fff]/g) ?? []).length
  note(ev, '正文汉字数', cjk)

  await expect.soft(res.status(), '详情页应 200').toBe(200)
  await expect.soft(body.length, '源码正文为空（SSR 未生效）').toBeGreaterThan(50)
  await expect.soft(cjk, '英文详情页正文仍是中文，说明 SSR 未取到英文译文').toBeLessThanOrEqual(2)
  await expect.soft(/[A-Za-z]{8,}/.test(body), '源码正文不含英文句子').toBe(true)
  await attachEv(testInfo, ev)
})

// ---------------------------------------------------------------- PUB-03
test('PUB-03 无译文语言：正文回落中文、界面文案跟随 URL 语言、不跳转', async ({ request }, testInfo) => {
  const ev: Ev = []
  const expectNav: Record<string, string> = { ru: 'Новости', fr: 'Actualités', de: 'Nachrichten' }
  for (const loc of ['ru', 'fr', 'de']) {
    const res = await request.get(`${SSR}/articles/${loc}/${SLUG_EN}`, { maxRedirects: 0, failOnStatusCode: false })
    const html = await res.text()
    const lang = (html.match(/<html[^>]*lang="([^"]*)"/) ?? [])[1] ?? ''
    const body = ((html.match(/<section class="content">([\s\S]*?)<\/section>/) ?? [])[1] ?? '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ')
    const nav = ((html.match(/<nav>([\s\S]*?)<\/nav>/) ?? [])[1] ?? '').replace(/<[^>]+>/g, '|')
    const cjk = (body.match(/[\u4e00-\u9fff]/g) ?? []).length
    note(ev, `${loc} status/lang`, `${res.status()} / ${lang}`)
    note(ev, `${loc} nav`, nav.slice(0, 60))
    note(ev, `${loc} 正文汉字数`, cjk)
    await expect.soft(res.status(), `${loc} 应 200 不跳转`).toBe(200)
    await expect.soft(lang.toLowerCase().startsWith(loc), `${loc} html lang 应为 ${loc}`).toBe(true)
    await expect.soft(cjk, `${loc} 无译文时正文应回落中文`).toBeGreaterThan(3)
    await expect.soft(nav, `${loc} 界面文案应本地化`).toContain(expectNav[loc])
  }
  await attachEv(testInfo, ev)
})

// ---------------------------------------------------------------- PUB-04
test('PUB-04 列表分页：每页 20 条 + 分页控件 + page 参数边界', async ({ page, request }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  const problems = pubGuard(page)

  // SSR 宿主：分页参数是否生效
  const ssrEmpty = await (async () => {
    const r = await request.get(`${SSR}/zh/articles?page=999`, { failOnStatusCode: false })
    const h = await r.text()
    return { status: r.status(), articles: (h.match(/<article>/g) ?? []).length, hasPager: /class="[^"]*pager/.test(h) }
  })()
  note(ev, 'SSR /zh/articles?page=999', ssrEmpty)
  const ssrP1 = await (async () => {
    const h = await (await request.get(`${SSR}/zh/articles`)).text()
    return (h.match(/<article>/g) ?? []).length
  })()
  note(ev, 'SSR /zh/articles 第 1 页条数', ssrP1)
  await expect.soft(ssrP1, 'SSR 列表第 1 页应 20 条').toBe(20)
  await expect.soft(ssrEmpty.hasPager, 'SSR 列表页没有分页控件（上一页/页码/下一页）').toBe(true)

  // SPA 宿主：卡片数 / 分页控件 / 参数
  await openPublic(page, '/articles')
  await shot(page, 'list-page1')
  const rows = page.locator('.neo-articles-row-list .article-row, .neo-articles-row-list > *')
  const rowCount = await rows.count()
  note(ev, 'SPA 列表第 1 页卡片数', rowCount)
  await expect.soft(rowCount, 'SPA 列表每页应 20 条').toBe(20)

  const pager = page.locator('[data-testid="pager"]')
  const pagerVisible = await pager.isVisible().catch(() => false)
  note(ev, 'SPA 分页控件可见', pagerVisible)
  await expect.soft(pagerVisible, 'SPA 列表缺少分页控件').toBe(true)
  if (pagerVisible) {
    const nums = await pager.locator('.pager-num').allInnerTexts()
    const cls = await pager.locator('.pager-num').evaluateAll((els) => els.map((e) => ({ t: e.textContent?.trim(), c: e.className, color: getComputedStyle(e).color, bg: getComputedStyle(e).backgroundColor })))
    note(ev, '页码', nums)
    note(ev, '当前页样式', cls.filter((x) => x.c.includes('is-active') || x.c.includes('current')))
    await expect.soft(nums.length, '页码最多 7 个').toBeLessThanOrEqual(7)
    const cur = cls.find((x) => x.c.includes('is-active') || x.c.includes('current'))
    note(ev, '当前页颜色', cur ? `${cur.color} / bg ${cur.bg}` : 'none')
  }

  // ?page=2 刷新后仍在第 2 页
  await page.goto(`${BASE}/articles?page=2`)
  await page.waitForTimeout(1500)
  const active2 = await page.locator('[data-testid="pager"] .pager-num.is-active, [data-testid="pager"] .pager-num.pager-current').first().innerText().catch(() => '')
  const url2 = page.url()
  note(ev, '?page=2 刷新后 URL', url2)
  note(ev, '?page=2 刷新后当前页', active2)
  await shot(page, 'list-page2')
  await expect.soft(active2.trim(), '?page=2 刷新后应停在第 2 页').toBe('2')

  // page=999 应跳最后一页（而非空列表）
  await page.goto(`${BASE}/articles?page=999`)
  await page.waitForTimeout(1500)
  const rows999 = await page.locator('.neo-articles-row-list .article-row, .neo-articles-row-list > *').count()
  note(ev, 'page=999 卡片数', rows999)
  await shot(page, 'list-page999')
  await expect.soft(rows999, 'page=999 应落到最后一页（有内容）而不是空列表').toBeGreaterThan(0)

  // page=0 / abc 视为第 1 页
  for (const bad of ['0', 'abc']) {
    await page.goto(`${BASE}/articles?page=${bad}`)
    await page.waitForTimeout(1200)
    const act = await page.locator('[data-testid="pager"] .pager-num.is-active, [data-testid="pager"] .pager-num.pager-current').first().innerText().catch(() => '')
    note(ev, `page=${bad} 当前页`, act.trim())
    await expect.soft(act.trim(), `page=${bad} 应视为第 1 页`).toBe('1')
  }

  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- PUB-05
test('PUB-05 英文列表总数 = 全部文章数', async ({ request }, testInfo) => {
  const ev: Ev = []
  const en = await (await request.get(`${SSR}/api/v1/public/articles?locale=en-US&page=1&page_size=1`)).json()
  const zh = await (await request.get(`${SSR}/api/v1/public/articles?locale=zh-CN&page=1&page_size=1`)).json()
  const ru = await (await request.get(`${SSR}/api/v1/public/articles?locale=ru-RU&page=1&page_size=1`)).json()
  const enTotal = en?.data?.total
  const zhTotal = zh?.data?.total
  const ruTotal = ru?.data?.total
  note(ev, 'API total en/zh/ru', `${enTotal} / ${zhTotal} / ${ruTotal}`)

  // "全部文章数" = 五个语种列表里出现过的全部文章 slug 并集（数据会随验收过程变化，故动态计算）
  const union = new Set<string>()
  for (const loc of ['zh-CN', 'en-US', 'ru-RU', 'fr-FR', 'de-DE']) {
    const d = await allItems(request, loc)
    for (const it of d.items) union.add(it.slug)
  }
  note(ev, '五语种 slug 并集（全部文章数）', union.size)
  await expect.soft(enTotal, '英文列表总数应等于全部文章数').toBe(union.size)

  // 列表页实际渲染条数应能覆盖全部文章（多页累加），且没有译文的显示中文
  const enList = await allItems(request, 'en-US')
  const items = enList.items
  note(ev, '英文接口实际返回条数', items.length)
  await expect.soft(items.length, '英文接口返回条数应等于 total').toBe(enTotal)
  const cjkTitles = items.filter((i: any) => /[\u4e00-\u9fff]/.test(i?.title ?? '')).length
  note(ev, '其中标题为中文（无译文回退）条数', cjkTitles)
  await attachEv(testInfo, ev)
})

// ---------------------------------------------------------------- PUB-06
test('PUB-06 封面：列表卡左 240px 16:9、详情 ≤400px、无封面占位（双主题）', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  const problems = pubGuard(page)

  for (const theme of ['light', 'dark'] as const) {
    await openPublic(page, '/articles', theme)
    await shot(page, `list-${theme}`)
    const covers = await page.evaluate(() => {
      const rows = Array.from(document.querySelectorAll('.neo-articles-row-list .article-row, .neo-articles-row-list > *'))
      const sample = rows.slice(0, 5).map((r) => {
        const img = r.querySelector('img, .list-card-cover, .article-cover, figure')
        if (!img) return { hasCover: false, rowText: (r.textContent ?? '').slice(0, 30) }
        const b = img.getBoundingClientRect()
        const cs = getComputedStyle(img.querySelector('img') ?? img)
        return { hasCover: true, tag: img.className || img.tagName, w: Math.round(b.width), h: Math.round(b.height), fit: cs.objectFit }
      })
      const placeholder = document.querySelectorAll('.public-featured-cover-placeholder, [class*="cover-placeholder"]').length
      return { rows: rows.length, sample, placeholder }
    })
    note(ev, `列表[${theme}] 行数/封面抽样`, covers)
    if (covers.rows > 0) {
      const withCover = covers.sample.filter((s: any) => s.hasCover)
      note(ev, `列表[${theme}] 有封面的行数`, withCover.length)
      await expect.soft(withCover.length, '列表卡片左侧应有封面（240px 16:9），当前行内没有封面元素').toBeGreaterThan(0)
      for (const c of withCover as any[]) {
        await expect.soft(Math.abs(c.w - 240), `封面宽应 240px，实测 ${c.w}`).toBeLessThanOrEqual(8)
        await expect.soft(Math.abs(c.w / Math.max(c.h, 1) - 16 / 9), '封面比例应 16:9').toBeLessThanOrEqual(0.12)
        await expect.soft(c.fit, '封面应 object-fit: cover').toBe('cover')
      }
    }

    // 详情页封面高度
    await openPublic(page, `/articles/zh/${SLUG_ZH_ONLY}`, theme)
    await shot(page, `detail-${theme}`)
    const d = await page.evaluate(() => {
      const el = document.querySelector('[data-testid="detail-cover"], .article-cover, figure img') as HTMLElement | null
      if (!el) return { has: false }
      const b = el.getBoundingClientRect()
      return { has: true, h: Math.round(b.height), w: Math.round(b.width) }
    })
    note(ev, `详情[${theme}] 封面`, d)
    if (d.has) await expect.soft((d as any).h, '详情页顶部封面高应 ≤400px').toBeLessThanOrEqual(400)
  }

  // 占位图：无封面文章不应留空白
  await openPublic(page, '/articles', 'light')
  const ph = await page.evaluate(() => {
    const els = Array.from(document.querySelectorAll('.article-row, .public-featured-small'))
    const noImg = els.filter((e) => !e.querySelector('img'))
    return { rows: els.length, noImg: noImg.length, placeholder: document.querySelectorAll('[class*="placeholder"]').length }
  })
  note(ev, '无封面行/占位元素', ph)
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- PUB-07
test('PUB-07 精选区结构：柠绿胶囊标题 + 副标题 + 查看全部 + 1 大 9 小 + 倒序', async ({ page, request }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  const problems = pubGuard(page)
  await openPublic(page, '/articles')
  await shot(page, 'featured')

  const sec = page.locator('[data-testid="featured-section"]')
  await expect.soft(await sec.count(), '缺少精选区（data-testid=featured-section）').toBeGreaterThan(0)

  const head = await page.evaluate(() => {
    const s = document.querySelector('[data-testid="featured-section"]') as HTMLElement | null
    if (!s) return null
    const pill = s.querySelector('.public-featured-title, [class*="pill"], [class*="eyebrow"]') as HTMLElement | null
    const cs = pill ? getComputedStyle(pill) : null
    const viewAll = Array.from(s.querySelectorAll('a,button')).map((e) => (e.textContent ?? '').trim()).filter((t) => /查看全部|View all|Voir tout|Alle|Смотреть/.test(t))
    return {
      pillText: pill?.textContent?.trim() ?? '',
      pillBg: cs?.backgroundColor ?? '',
      pillColor: cs?.color ?? '',
      pillRadius: cs?.borderRadius ?? '',
      subtitles: Array.from(s.querySelectorAll('[class*="subtitle"], [class*="sub-title"], p')).map((e) => (e.textContent ?? '').trim()).filter(Boolean).slice(0, 3),
      viewAll,
      largeCount: s.querySelectorAll('[data-testid="featured-large-card"]').length,
      smallCount: s.querySelectorAll('[data-testid="featured-small-grid"] > *').length,
      gridCols: s.querySelector('[data-testid="featured-small-grid"]') ? getComputedStyle(s.querySelector('[data-testid="featured-small-grid"]') as HTMLElement).gridTemplateColumns : '',
    }
  })
  note(ev, '精选区头部/数量', head)
  if (head) {
    await expect.soft(head.pillBg.replace(/\s/g, ''), '胶囊标题底色应柠绿 rgb(204,255,0)').toMatch(/204,\s*255,\s*0|#ccff00/i)
    await expect.soft(head.pillColor.replace(/\s/g, ''), '胶囊标题文字应 #121316').toMatch(/18,\s*19,\s*22/)
    await expect.soft(head.pillRadius, '胶囊标题应为全圆角').toMatch(/999|9999|50%/)
    await expect.soft(head.subtitles.length, '缺少副标题').toBeGreaterThan(0)
    await expect.soft(head.viewAll.length, '缺少「查看全部 →」').toBeGreaterThan(0)
    await expect.soft(head.largeCount, '应有 1 张大卡').toBe(1)
    await expect.soft(head.smallCount, '小卡最多 9 张').toBeLessThanOrEqual(9)
    await expect.soft(Number(head.gridCols.split(' ').length), '小卡应为 3 列').toBe(3)
  }

  const largeW = await page.evaluate(() => {
    const l = document.querySelector('[data-testid="featured-large-card"]') as HTMLElement | null
    const s = document.querySelector('[data-testid="featured-section"]') as HTMLElement | null
    if (!l || !s) return null
    return { large: l.getBoundingClientRect().width, section: s.getBoundingClientRect().width }
  })
  note(ev, '大卡宽度/区块宽度', largeW)
  if (largeW) {
    const pct = (largeW.large / largeW.section) * 100
    note(ev, '大卡占比 %', Math.round(pct * 10) / 10)
    await expect.soft(Math.abs(pct - 40), '大卡应约 40% 宽').toBeLessThanOrEqual(8)
  }

  // 最多 10 篇 + 按 featured_at 倒序
  const api = await (await request.get(`${SSR}/api/v1/public/articles/featured?locale=zh-CN`)).json()
  const list = Array.isArray(api?.data) ? api.data : api?.data?.items ?? []
  note(ev, 'featured 接口条数', list.length)
  note(ev, 'featured 接口前 3 标题', list.slice(0, 3).map((i: any) => i.title?.slice(0, 20)))
  await expect.soft(list.length, '精选最多 10 篇').toBeLessThanOrEqual(10)
  const domTitles = await page.evaluate(() => Array.from(document.querySelectorAll('[data-testid="featured-section"] h2, [data-testid="featured-section"] h3')).map((e) => (e.textContent ?? '').trim()))
  note(ev, 'DOM 精选标题顺序', domTitles.map((t) => t.slice(0, 20)))
  const apiTitles = list.map((i: any) => (i.title ?? '').trim())
  const inOrder = apiTitles.length > 0 && domTitles.length > 0 && apiTitles[0].startsWith(domTitles[0].slice(0, 12))
  note(ev, 'DOM 首条与接口首条一致', inOrder)
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- PUB-08
test('PUB-08 精选卡片对比度 ≥4.5（深/浅双主题色值）', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  for (const theme of ['dark', 'light'] as const) {
    await openPublic(page, '/articles', theme)
    await shot(page, `featured-${theme}`)
    const data = await page.evaluate((src) => {
      const fn = new Function(`${src} return { parseRGB, lum, ratio, effBg }`)() as any
      const cards = Array.from(document.querySelectorAll('[data-testid="featured-section"] [data-testid="featured-large-card"], [data-testid="featured-section"] .public-featured-small'))
      const out = cards.slice(0, 10).map((c) => {
        const title = c.querySelector('h2, h3') as HTMLElement | null
        if (!title) return null
        const fg = fn.parseRGB(getComputedStyle(title).color)
        const cs = getComputedStyle(c)
        const bgOwn = fn.parseRGB(cs.backgroundColor)
        const bg = bgOwn && bgOwn.a > 0.05 ? bgOwn : fn.effBg(title)
        return {
          title: (title.textContent ?? '').slice(0, 24),
          fg: `rgb(${fg.r},${fg.g},${fg.b})`,
          bg: bg ? `rgb(${bg.r},${bg.g},${bg.b})` : 'none',
          ratio: fg && bg ? Math.round(fn.ratio(fg, bg) * 100) / 100 : null,
        }
      }).filter(Boolean)
      return out
    }, CONTRAST_SRC)
    note(ev, `[${theme}] 卡片标题/底色/对比度`, data)
    for (const d of data as any[]) {
      await expect.soft(d.ratio ?? 0, `[${theme}]「${d.title}」对比度 ${d.ratio} < 4.5（fg ${d.fg} / bg ${d.bg}）`).toBeGreaterThanOrEqual(4.5)
    }
    if (theme === 'dark' && (data as any[])[0]) {
      note(ev, '[dark] 实测卡片底', (data as any[])[0].bg)
      note(ev, '[dark] 实测标题色', (data as any[])[0].fg)
    }
    if (theme === 'light' && (data as any[])[0]) {
      note(ev, '[light] 实测卡片底', (data as any[])[0].bg)
      note(ev, '[light] 实测标题色', (data as any[])[0].fg)
    }
  }
  await attachEv(testInfo, ev)
})

// ---------------------------------------------------------------- PUB-09
test('PUB-09 精选卡片悬停上浮+阴影加深，点击进入详情', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  const problems = pubGuard(page)
  await openPublic(page, '/articles')
  const card = page.locator('[data-testid="featured-large-card"]').first()
  await expect.soft(await card.count(), '缺少大卡，无法验证悬停').toBeGreaterThan(0)
  if (await card.count()) {
    const before = await card.evaluate((el) => {
      const cs = getComputedStyle(el)
      return { transform: cs.transform, shadow: cs.boxShadow }
    })
    await card.hover()
    await page.waitForTimeout(700)
    const after = await card.evaluate((el) => {
      const cs = getComputedStyle(el)
      return { transform: cs.transform, shadow: cs.boxShadow }
    })
    note(ev, '悬停前', before)
    note(ev, '悬停后', after)
    await shot(page, 'hover')
    await expect.soft(after.transform, '悬停应有 transform 变化（上浮）').not.toBe(before.transform)
    await expect.soft(after.shadow, '悬停阴影应加深').not.toBe(before.shadow)

    await Promise.all([page.waitForURL(/\/articles\/(zh|en|ru|fr|de)\//, { timeout: 15000 }).catch(() => null), card.click()])
    await page.waitForTimeout(1500)
    note(ev, '点击后 URL', page.url())
    await shot(page, 'after-click')
    await expect.soft(page.url(), '点击精选卡应进入详情页').toMatch(/\/articles\/(zh|en|ru|fr|de)(-[A-Za-z]{2,3})?\/[^/?#]+/)
  }
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- PUB-10
test('PUB-10 390×844 精选区：大卡占满一行、小卡 1 列、无横向溢出', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  await page.setViewportSize({ width: 390, height: 844 })
  await openPublic(page, '/articles')
  await page.waitForTimeout(800)
  await shot(page, 'mobile')
  const m = await page.evaluate(() => {
    const l = document.querySelector('[data-testid="featured-large-card"]') as HTMLElement | null
    const g = document.querySelector('[data-testid="featured-small-grid"]') as HTMLElement | null
    const cols = g ? getComputedStyle(g).gridTemplateColumns.split(' ').length : -1
    return {
      largeW: l ? Math.round(l.getBoundingClientRect().width) : -1,
      viewportW: document.documentElement.clientWidth,
      cols,
      overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    }
  })
  note(ev, '390 视口测量', m)
  await expect.soft(m.largeW, '大卡应占满一行').toBeGreaterThanOrEqual(m.viewportW - 48)
  await expect.soft(m.cols, '小卡在 390 下应为 1 列').toBe(1)
  await expect.soft(m.overflow, '390 视口横向溢出').toBeLessThanOrEqual(1)
  await attachEv(testInfo, ev)
})

// ---------------------------------------------------------------- PUB-11
test('PUB-11 详情页「更多深度解析」6 条不含当前 + 上一篇/下一篇可点', async ({ page, request }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  const problems = pubGuard(page)

  // 取中文列表第 2 篇（内部文章，理论上上一篇/下一篇都存在）作为被测详情
  const zhList = await allItems(request, 'zh-CN')
  const interior = zhList.items[1]
  const interiorSlug = interior?.slug ?? SLUG_EN
  note(ev, '被测详情 slug（列表第2篇）', interiorSlug)

  // SSR 宿主源码
  const html = await (await request.get(`${SSR}/articles/zh/${interiorSlug}`)).text()
  note(ev, 'SSR 源码含 更多深度解析', /更多深度解析/.test(html))
  note(ev, 'SSR 源码含 上一篇', /上一篇/.test(html))
  await expect.soft(/更多深度解析/.test(html), 'SSR 详情页缺少「更多深度解析」区块').toBe(true)

  // SPA 宿主实测
  await openPublic(page, `/articles/zh/${interiorSlug}`)
  await page.waitForTimeout(1500)
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
  await page.waitForTimeout(1200)
  await shot(page, 'detail-related')
  const info = await page.evaluate(() => {
    const head = Array.from(document.querySelectorAll('h1,h2,h3,div,span')).find((e) => /更多深度解析|More Deep Dive/.test((e.textContent ?? '').trim()) && (e.textContent ?? '').trim().length < 40)
    const isArticle = (a: Element) => /\/articles\/[^/?#]+\/[^/?#]+/.test(a.getAttribute('href') ?? '')
    let block: HTMLElement | null = (head as HTMLElement | null)?.closest('section') ?? null
    if (!block || block.querySelectorAll('a').length < 2) {
      let b: HTMLElement | null = head as HTMLElement | null
      for (let i = 0; i < 4 && b; i++) {
        b = b.parentElement
        if (b && Array.from(b.querySelectorAll('a')).filter(isArticle).length >= 2) { block = b; break }
      }
    }
    const seen = new Set<string>()
    const links = (block ? Array.from(block.querySelectorAll('a')).filter(isArticle) : []).filter((a) => {
      const h = a.getAttribute('href') ?? ''
      if (seen.has(h)) return false
      seen.add(h)
      return true
    })
    const self = location.pathname
    return {
      headFound: !!head,
      headText: head?.textContent?.trim() ?? '',
      blockTag: block ? `${block.tagName}.${(block.className || '').toString().split(' ')[0]}` : null,
      linkCount: links.length,
      links: links.slice(0, 8).map((a) => ({ href: a.getAttribute('href'), text: (a.textContent ?? '').trim().slice(0, 24) })),
      selfPath: self,
      prevNext: (() => {
        const nav = document.querySelector('.detail-prev-next-grid')
        if (!nav) return { nav: false, cards: 0, clickable: 0, empty: 0, items: [] as Array<{ tag: string; href: string | null; text: string }> }
        const cards = Array.from(nav.querySelectorAll('.prev-next-card'))
        return {
          nav: true,
          cards: cards.length,
          clickable: cards.filter((c) => c.tagName === 'A').length,
          empty: cards.filter((c) => c.classList.contains('pn-empty')).length,
          items: cards.map((c) => ({ tag: c.tagName, href: c.getAttribute('href'), text: (c.textContent ?? '').replace(/\s+/g, ' ').trim().slice(0, 40) })),
        }
      })(),
    }
  })
  note(ev, 'SPA 相关推荐/上下篇', info)
  await expect.soft(info.headFound, 'SPA 详情页缺少「更多深度解析」').toBe(true)
  const pn = info.prevNext as any
  note(ev, '上下篇卡片', `cards=${pn.cards} clickable=${pn.clickable} empty=${pn.empty}`)
  await expect.soft(pn.nav, '缺少「上一篇/下一篇」导航').toBe(true)
  await expect.soft(pn.cards, '上一篇/下一篇应有两张卡').toBe(2)
  await expect.soft(pn.clickable, '中间文章的上一篇/下一篇都应可点').toBe(2)
  const recLinks = info.links.filter((l) => l.href && l.href.includes('/articles/'))
  note(ev, '相关推荐链接数', recLinks.length)
  await expect.soft(recLinks.length, '相关推荐应固定 6 条').toBe(6)
  const containsSelf = recLinks.some((l) => (l.href ?? '').includes(info.selfPath.split('/').pop() ?? '###'))
  note(ev, '相关推荐是否含当前文章', containsSelf)
  await expect.soft(containsSelf, '相关推荐不应包含当前文章').toBe(false)
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- PUB-12
test('PUB-12 语言切换器：五项顺序一致、切换写 cookie 并跳前缀 URL', async ({ page, context, request }, testInfo) => {
  test.slow() // 3 个页面 × 正反两轮切换 + 各 2.5s 等待，默认 30s 装不下
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  const problems = pubGuard(page)

  for (const path of ['/', '/news', '/articles']) {
    await openPublic(page, path)
    // 语言菜单 v-if 渲染，且桌面/移动两套切换器同时存在于 DOM → 只操作「可见」那一套
    const trigger = page.locator('.lang-switch-btn:visible').first()
    await trigger.click()
    const menu = page.locator('.lang-menu:visible').first()
    await menu.waitFor({ state: 'visible', timeout: 5000 }).catch(() => {})
    const rawOpts = await menu.locator('li, [role="option"]').evaluateAll((els) =>
      els.map((e) => (e.textContent ?? '').replace(/\s+/g, ' ').trim())
    )
    // 桌面 + 移动两套菜单同时在 DOM 里（其中一个仅 CSS 隐藏），文本会成对重复 → 去重后比对顺序
    const opts = Array.from(new Set(rawOpts.filter(Boolean)))
    note(ev, `${path} 菜单原始项`, rawOpts)
    note(ev, `${path} 去重后选项`, opts)
    await expect.soft(opts, `${path} 语言选项/顺序应为 ${LANGS.join('/')}`).toEqual(LANGS)

    // 当前语言已是 en，点 English 会被 `code === locale` 提前 return；先切 Русский 验证真实切换
    await menu.locator('li, [role="option"]').filter({ hasText: 'Русский' }).first().click()
    await page.waitForTimeout(2500)
    const pathname = new URL(page.url()).pathname.replace(/\/$/, '') || '/'
    const cookies = await context.cookies()
    const cookieLocale = cookies.find((c) => c.name === 'galley_locale')?.value ?? ''
    const storageLocale = await page.evaluate(() => localStorage.getItem('sub2api_locale') ?? '')
    const htmlLang = await page.evaluate(() => document.documentElement.lang)
    note(ev, `${path} 切 English 后 pathname`, pathname)
    note(ev, `${path} cookie galley_locale`, cookieLocale)
    note(ev, `${path} localStorage sub2api_locale`, storageLocale)
    note(ev, `${path} html lang`, htmlLang)
    await shot(page, `lang-${path.replace(/\//g, '_') || 'home'}`)

    await expect.soft(cookieLocale, '切换后应写 cookie galley_locale=ru').toMatch(/^ru/)
    await expect.soft(storageLocale, '切换后应写 localStorage sub2api_locale=ru').toMatch(/^ru/)
    await expect.soft(htmlLang.toLowerCase(), '切换后 html lang 应变 ru').toMatch(/^ru/)

    // 目标前缀 URL：'/'→'/ru'，'/news'→'/ru/news'，'/articles'→'/ru/articles'
    const expectedRu = path === '/' ? '/ru' : `/ru${path}`
    await expect.soft(pathname, `${path} 切 Русский 应跳前缀 ${expectedRu}，实测 ${pathname}`).toBe(expectedRu)
    const ssrRu = await request.get(`${SSR}${expectedRu}`).catch(() => null)
    note(ev, `${expectedRu} 在 SSR 宿主状态码`, ssrRu?.status() ?? 'request-failed')
    await expect.soft(ssrRu?.status() ?? 0, `切换目标 ${expectedRu} 在 SSR 宿主应 200`).toBe(200)

    // 回到原页（cookie 已是 ru）再切回 English，验证反向切换同样落地 /en 前缀
    await openPublic(page, path)
    const trigger2 = page.locator('.lang-switch-btn:visible').first()
    await trigger2.click()
    const menu2 = page.locator('.lang-menu:visible').first()
    await menu2.waitFor({ state: 'visible', timeout: 5000 }).catch(() => {})
    await menu2.locator('li, [role="option"]').filter({ hasText: 'English' }).first().click()
    await page.waitForTimeout(2500)
    const pathname2 = new URL(page.url()).pathname.replace(/\/$/, '') || '/'
    const cookie2 = (await context.cookies()).find((c) => c.name === 'galley_locale')?.value ?? ''
    const expectedEn = path === '/' ? '/en' : `/en${path}`
    note(ev, `${path} 切回 English pathname`, pathname2)
    note(ev, `${path} 切回 English cookie`, cookie2)
    await expect.soft(cookie2, '切回 English 应写 cookie galley_locale=en').toMatch(/^en/)
    await expect.soft(pathname2, `${path} 切 English 应跳前缀 ${expectedEn}，实测 ${pathname2}`).toBe(expectedEn)
    const ssrEn = await request.get(`${SSR}${expectedEn}`).catch(() => null)
    note(ev, `${expectedEn} 在 SSR 宿主状态码`, ssrEn?.status() ?? 'request-failed')
    await expect.soft(ssrEn?.status() ?? 0, `切换目标 ${expectedEn} 在 SSR 宿主应 200`).toBe(200)
  }
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- PUB-13
test('PUB-13 全站样式扫描：无原生 select/单下边框输入框/#10B981、按钮输入框 36±1', async ({ page }, testInfo) => {
  test.slow() // 7 个页面（浅色/深色）逐个扫描 + 截图，默认 30s 装不下
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  const problems = pubGuard(page)

  const scan = async (label: string) => {
    const r = await page.evaluate(() => {
      const vis = (e: Element) => {
        const el = e as HTMLElement
        const b = el.getBoundingClientRect()
        const cs = getComputedStyle(el)
        return b.width > 0 && b.height > 0 && cs.visibility !== 'hidden' && cs.display !== 'none'
      }
      const desc = (e: Element) => {
        const el = e as HTMLElement
        return `${el.tagName.toLowerCase()}${el.id ? '#' + el.id : ''}.${(el.className && typeof el.className === 'string' ? el.className.split(/\s+/).slice(0, 2).join('.') : '')}`
      }
      const nativeSelects: string[] = []
      document.querySelectorAll('select').forEach((s) => {
        const cs = getComputedStyle(s)
        if (vis(s) && cs.appearance !== 'none' && cs.webkitAppearance !== 'none') nativeSelects.push(`${desc(s)} appearance=${cs.appearance}`)
      })
      const underlineOnly: string[] = []
      const badInputs: string[] = []
      document.querySelectorAll('input, textarea').forEach((i) => {
        if (!vis(i)) return
        const cs = getComputedStyle(i)
        const bt = parseFloat(cs.borderTopWidth)
        const bb = parseFloat(cs.borderBottomWidth)
        const bl = parseFloat(cs.borderLeftWidth)
        const br = parseFloat(cs.borderRightWidth)
        if (bt === 0 && bl === 0 && br === 0 && bb > 0) underlineOnly.push(`${desc(i)} border=${bt}/${br}/${bb}/${bl}`)
        const h = (i as HTMLElement).getBoundingClientRect().height
        if (Math.abs(h - 36) > 1) badInputs.push(`${desc(i)} h=${Math.round(h)}`)
      })
      const badButtons: string[] = []
      document.querySelectorAll('button, [role="button"], a.btn, .btn').forEach((b) => {
        if (!vis(b)) return
        const h = (b as HTMLElement).getBoundingClientRect().height
        if (Math.abs(h - 36) > 1) badButtons.push(`${desc(b)} h=${Math.round(h)}`)
      })
      const green: string[] = []
      document.querySelectorAll('*').forEach((e) => {
        if (!vis(e)) return
        const cs = getComputedStyle(e)
        const hit = [cs.color, cs.backgroundColor, cs.borderTopColor].some((c) => /16,\s*185,\s*129/.test(c) || /#10b981/i.test(c))
        if (hit) green.push(`${desc(e)} color=${cs.color} bg=${cs.backgroundColor}`)
      })
      return { nativeSelects, underlineOnly, badInputs: badInputs.slice(0, 12), badInputCount: badInputs.length, badButtons: badButtons.slice(0, 12), badButtonCount: badButtons.length, green: green.slice(0, 12), greenCount: green.length }
    })
    ev.push(`--- ${label}`)
    note(ev, '原生 select', r.nativeSelects)
    note(ev, '仅下边框输入框', r.underlineOnly)
    note(ev, `按钮高度非 36±1（前 12/${r.badButtonCount}）`, r.badButtons)
    note(ev, `输入框高度非 36±1（前 12/${r.badInputCount}）`, r.badInputs)
    note(ev, `#10B981 命中（前 12/${r.greenCount}）`, r.green)
    return r
  }

  // 前台各页（双主题）
  for (const theme of ['light', 'dark'] as const) {
    for (const p of ['/', '/news', '/articles', `/articles/zh/${SLUG_ZH_ONLY}`]) {
      await openPublic(page, p, theme)
      await shot(page, `scan${p.replace(/\//g, '_') || '_home'}-${theme}`)
      await scan(`前台 ${p} [${theme}]`)
    }
  }

  // 后台各 tab（浅色为主，另测深色一次）
  await realLogin(page)
  for (const tab of ['新闻动态', '分类与标签', '文章管理', '内容设置']) {
    const has = await page.locator('.tabs button', { hasText: tab }).count().catch(() => 0)
    if (!has) {
      note(ev, `后台 tab 不存在`, tab)
      continue
    }
    await openContentTab(page, tab)
    await page.waitForTimeout(900)
    await shot(page, `admin-${tab}`)
    await scan(`后台 ${tab} [light]`)
  }
  await page.evaluate(() => localStorage.setItem('theme', 'dark'))
  await page.reload()
  await page.waitForTimeout(1500)
  await shot(page, 'admin-dark')
  await scan('后台 [dark]')

  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- PUB-14
test('PUB-14 全站正文对比度抽样（每页前 30 个文本节点，双主题）', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  for (const theme of ['light', 'dark'] as const) {
    for (const p of ['/', '/news', '/articles', `/articles/zh/${SLUG_ZH_ONLY}`]) {
      await openPublic(page, p, theme)
      await shot(page, `contrast${p.replace(/\//g, '_') || '_home'}-${theme}`)
      const r = await page.evaluate((src) => {
        const fn = new Function(`${src} return { parseRGB, ratio, effBg }`)() as any
        const nodes: HTMLElement[] = []
        const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
        let n: Node | null = walker.nextNode()
        while (n && nodes.length < 30) {
          const t = (n.textContent ?? '').trim()
          const p = n.parentElement
          if (t.length >= 2 && p && !/^(SCRIPT|STYLE|NOSCRIPT)$/.test(p.tagName) && p.getBoundingClientRect().height > 0) nodes.push(p)
          n = walker.nextNode()
        }
        return nodes.map((el) => {
          const cs = getComputedStyle(el)
          const fg = fn.parseRGB(cs.color)
          const bg = fn.effBg(el)
          const size = parseFloat(cs.fontSize)
          const bold = Number(cs.fontWeight) >= 600
          const large = size >= 24 || (size >= 18.66 && bold)
          return { text: (el.textContent ?? '').trim().slice(0, 26), color: cs.color, bg: bg ? `rgb(${bg.r},${bg.g},${bg.b})` : 'none', size, large, ratio: fg && bg ? Math.round(fn.ratio(fg, bg) * 100) / 100 : null }
        })
      }, CONTRAST_SRC)
      const bad = (r as any[]).filter((x) => x.ratio !== null && x.ratio < (x.large ? 3 : 4.5))
      note(ev, `[${theme}] ${p} 抽样 ${r.length} 条，违规 ${bad.length}`, bad.slice(0, 8))
      for (const b of bad as any[]) {
        await expect.soft(b.ratio, `[${theme}] ${p}「${b.text}」对比度 ${b.ratio} < ${b.large ? 3 : 4.5}（fg ${b.color} / bg ${b.bg}）`).toBeGreaterThanOrEqual(b.large ? 3 : 4.5)
      }
    }
  }
  await attachEv(testInfo, ev)
})

// ---------------------------------------------------------------- PUB-15
test('PUB-15 前台键盘导航：Tab 覆盖导航/语言切换/卡片/分页且焦点可见', async ({ page }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  const problems = pubGuard(page)
  await openPublic(page, '/articles')
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())

  const seen: any[] = []
  for (let i = 0; i < 60; i++) {
    await page.keyboard.press('Tab')
    const cur = await page.evaluate(() => {
      const a = document.activeElement as HTMLElement | null
      if (!a || a === document.body) return null
      const cs = getComputedStyle(a)
      const b = a.getBoundingClientRect()
      return {
        tag: a.tagName.toLowerCase(),
        cls: typeof a.className === 'string' ? a.className.slice(0, 60) : '',
        text: (a.textContent ?? '').trim().slice(0, 24),
        href: a.getAttribute('href') ?? '',
        outlineW: parseFloat(cs.outlineWidth) || 0,
        outlineStyle: cs.outlineStyle,
        shadow: cs.boxShadow !== 'none',
        top: Math.round(b.top),
        left: Math.round(b.left),
      }
    })
    if (cur) seen.push(cur)
  }
  note(ev, 'Tab 序列（前 40）', seen.slice(0, 40))
  const noFocusRing = seen.filter((s) => s.outlineW === 0 && !s.shadow)
  note(ev, '无可见焦点样式数量/总点击数', `${noFocusRing.length}/${seen.length}`)
  await shot(page, 'keyboard-end')

  const idx = (pred: (s: any) => boolean) => seen.findIndex(pred)
  const navIdx = idx((s) => s.href === '/news' || s.href === '/articles' || s.cls.includes('nav-sub-btn'))
  const langIdx = idx((s) => s.cls.includes('lang') || /English|中文|Русский/.test(s.text))
  const cardIdx = idx((s) => s.href.includes('/articles/') && !s.cls.includes('nav-sub-btn'))
  const pagerIdx = idx((s) => s.cls.includes('pager') || s.href.includes('page='))
  note(ev, '顺序索引 nav/lang/card/pager', [navIdx, langIdx, cardIdx, pagerIdx])
  await expect.soft(seen.length, 'Tab 走不到任何可聚焦元素').toBeGreaterThan(3)
  await expect.soft(langIdx, 'Tab 无法到达语言切换器').toBeGreaterThanOrEqual(0)
  await expect.soft(cardIdx, 'Tab 无法到达卡片').toBeGreaterThanOrEqual(0)
  await expect.soft(pagerIdx, 'Tab 无法到达分页').toBeGreaterThanOrEqual(0)
  if (navIdx >= 0 && langIdx >= 0) await expect.soft(navIdx < langIdx, 'Tab 顺序应为导航→语言切换').toBe(true)
  if (langIdx >= 0 && cardIdx >= 0) await expect.soft(langIdx < cardIdx, 'Tab 顺序应为语言切换→卡片').toBe(true)
  if (cardIdx >= 0 && pagerIdx >= 0) await expect.soft(cardIdx < pagerIdx, 'Tab 顺序应为卡片→分页').toBe(true)
  await expect.soft(noFocusRing.length, '存在无可见焦点的可聚焦元素').toBeLessThanOrEqual(Math.ceil(seen.length * 0.2))

  await attachEv(testInfo, ev)
  expectClean(problems)
})

// ---------------------------------------------------------------- PUB-16
test('PUB-16 详情页停留 ≥10s 有埋点上报（referrer/停留时长/语言）', async ({ page, request }, testInfo) => {
  const ev: Ev = []
  const shot = shooter('w5', testInfo)
  const problems = pubGuard(page)
  const payloads: any[] = []
  page.on('request', (req) => {
    if (req.url().includes('/api/v1/public/analytics/events') && req.method() === 'POST') {
      const raw = req.postData() ?? ''
      const flat = raw.includes('[') ? JSON.parse(raw) : [JSON.parse(raw)]
      for (const p of flat) payloads.push(p)
    }
  })
  const zhList16 = await allItems(request, 'zh-CN')
  const interiorSlug = zhList16.items[1]?.slug ?? SLUG_EN
  note(ev, '埋点探针详情 slug', interiorSlug)
  // 带外链 referer 进入，才能验证埋点上报 referrer（直接从 about:blank 进入没有 document.referrer）
  await page.goto(`${BASE}/articles/zh/${interiorSlug}`, { waitUntil: 'domcontentloaded', referer: 'https://www.google.com/' })
  await page.waitForTimeout(12000)
  await page.evaluate(() => window.scrollTo(0, 600))
  await page.waitForTimeout(2000)
  await shot(page, 'analytics-after-12s')
  note(ev, '埋点事件数', payloads.length)
  note(ev, '事件类型', payloads.map((p) => p.eventName ?? p.eventName ?? p.event ?? null))
  note(ev, '载荷样例', payloads.slice(0, 6))
  await expect.soft(payloads.length, '停留 12s 后没有埋点上报').toBeGreaterThan(0)
  const joined = JSON.stringify(payloads)
  note(ev, '含语言字段', /"locale"\s*:\s*"/.test(joined))
  note(ev, '含停留时长字段', /duration_ms|read_seconds|dwell/.test(joined))
  note(ev, '含 referrer 字段', /referrer/i.test(joined))
  await expect.soft(/duration_ms|read_seconds/.test(joined), '埋点载荷缺少停留时长（page_heartbeat/page_leave 的 duration_ms 或 read_seconds）').toBe(true)
  await expect.soft(/referrer/i.test(joined), '埋点载荷缺少 referrer（外链来源时应带 referrerHost）').toBe(true)
  await expect.soft(/"locale"\s*:\s*"/.test(joined), '埋点载荷缺少语言 locale').toBe(true)
  await attachEv(testInfo, ev)
  expectClean(problems)
})

// 凭据常量导出占位，避免未使用告警（EMAIL/PASSWORD 由 realLogin 内部使用同一来源）
void EMAIL
void PASSWORD
void stamp
