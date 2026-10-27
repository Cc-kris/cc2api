import { expect, test, type APIRequestContext } from '@playwright/test'
// TASK-033 公开站 SSR/SEO/路由/翻译（R7-02, R9-02～R9-08, R12-01/02/05/06）。SSR 走 Go 服务，默认 :8080。
const SSR = process.env.S2A_SSR_BASE_URL ?? 'http://127.0.0.1:8080'
const ADMIN = process.env.S2A_BASE_URL ?? 'http://127.0.0.1:3100'
const LOCALES = ['zh', 'en', 'ru', 'fr', 'de']
const SLUG = 'ccai-api-practice'
const NEWS = 'ccai-api-gateway-news'
const go = (r: APIRequestContext, p: string, headers: Record<string, string> = {}) =>
  r.get(SSR + p, { headers, maxRedirects: 0, failOnStatusCode: false })
const publicJson = async (r: APIRequestContext, p: string) => (await r.get(SSR + p)).json()
const located = (loc: string) => {
  const u = new URL(loc, SSR)
  return u.pathname + u.search
}
const hreflang = (html: string) => [...html.matchAll(/hreflang="([^"]+)"/g)].map((m) => m[1])

test('R7-02 译文列表继承原文封面，不因翻译丢失', async ({ request }) => {
  const zh = (await publicJson(request, '/api/v1/public/articles?locale=zh-CN&page_size=100')).data.items
  const en = (await publicJson(request, '/api/v1/public/articles?locale=en-US&page_size=100')).data.items
  const bySlug = new Map<string, { coverImage?: string }>(en.map((i: { slug: string }) => [i.slug, i]))
  let pairs = 0
  for (const item of zh) {
    const translated = bySlug.get(item.slug)
    if (!translated) continue
    pairs++
    expect(translated.coverImage ?? null, `${item.slug} 译文封面`).toBe(item.coverImage ?? null)
  }
  expect(pairs, '存在译文对').toBeGreaterThan(0)
})

test('R9-02 仅匹配当前 source_version 的译文对外发布', async ({ request }) => {
  const login = await request.post(`${ADMIN}/api/v1/auth/login`, {
    data: { email: process.env.S2A_E2E_EMAIL ?? 'e2e-task031@sub2api.local', password: process.env.S2A_E2E_PASSWORD ?? 'TASK031-e2e-Passw0rd!' }
  })
  test.skip(!login.ok(), `管理员登录失败 ${login.status()}`)
  const auth = { Authorization: `Bearer ${(await login.json()).data.access_token}` }
  const items = (await (await request.get(`${ADMIN}/api/v1/admin/content/articles?page=1&page_size=100`, { headers: auth })).json()).data.items
  let checked = 0
  for (const item of items) {
    if (checked >= 3) break
    const res = await request.get(`${ADMIN}/api/v1/admin/content/articles/${item.id}/translations`, { headers: auth })
    for (const tr of (await res.json()).data ?? []) {
      if (tr.status !== 'published') continue
      checked++
      const lang = tr.locale.slice(0, 2)
      const html = await (await go(request, `/articles/${lang}/${item.slug}`)).text()
      if (tr.sourceVersion === item.version) {
        expect(html, `${item.slug}/${lang} 当前版本`).toContain(`rel="canonical" href="https://ccai.xyz/articles/${lang}/${item.slug}"`)
      } else {
        expect(html, `${item.slug}/${lang} 旧版本必须回落 noindex`).toContain('content="noindex, follow"')
      }
    }
  }
  expect(checked, '已发布译文数量').toBeGreaterThan(0)
})

test('R9-03 无 JS 原文 HTML：en 英文正文，de 中文正文且德语界面', async ({ request }) => {
  const en = await (await go(request, `/articles/en/${SLUG}`)).text()
  const de = await (await go(request, `/articles/de/${SLUG}`)).text()
  expect(en).not.toMatch(/<script[^>]+src=/)
  expect(en).toContain('Overview')
  expect(en).not.toMatch(/[\u4e00-\u9fa5]{10,}/)
  expect(de).toContain('通过 CCAI 可以用同一套密钥')
  expect(de).toMatch(/<html lang="de-DE"/)
  expect(de).toMatch(/>Nachrichten<|>Artikel<|>Dokumentation</)
})

test('R9-04 有效译文 self-canonical / index / hreflang 含中文与 x-default', async ({ request }) => {
  const detail = await publicJson(request, `/api/v1/public/articles/zh/${SLUG}`)
  const alts: string[] = (detail.data.alternates ?? []).map((a: { locale: string }) => a.locale.slice(0, 2))
  expect(alts.length, '至少一个已发布译文').toBeGreaterThan(0)
  for (const lang of alts) {
    const html = await (await go(request, `/articles/${lang}/${SLUG}`)).text()
    expect(html, lang).toContain(`rel="canonical" href="https://ccai.xyz/articles/${lang}/${SLUG}"`)
    expect(html, lang).toContain('content="index, follow"')
    const hl = hreflang(html)
    expect(hl, `${lang} hreflang`).toContain('zh')
    expect(hl, `${lang} x-default`).toContain('x-default')
    expect(hl, `${lang} 不含无译文 de`).not.toContain('de')
  }
})

test('R9-05 无译文 de：canonical 指中文、noindex、不进 hreflang/sitemap', async ({ request }) => {
  const html = await (await go(request, `/articles/de/${SLUG}`)).text()
  expect(html).toContain(`rel="canonical" href="https://ccai.xyz/articles/zh/${SLUG}"`)
  expect(html).toContain('content="noindex, follow"')
  expect(hreflang(html)).not.toContain('de')
  expect(await (await request.get(`${SSR}/sitemap.xml`)).text()).not.toContain(`/articles/de/${SLUG}`)
})

test('R9-06 中文 self-canonical，sitemap 含中文与有效译文', async ({ request }) => {
  const html = await (await go(request, `/articles/zh/${SLUG}`)).text()
  expect(html).toContain(`rel="canonical" href="https://ccai.xyz/articles/zh/${SLUG}"`)
  const sitemap = await (await request.get(`${SSR}/sitemap.xml`)).text()
  expect(sitemap).toContain(`/articles/zh/${SLUG}`)
  expect(sitemap).toContain(`/articles/en/${SLUG}`)
})

test('R9-07 长码 locale 301 到短码并保留查询参数', async ({ request }) => {
  for (const [from, to] of [
    [`/articles/zh-CN/${SLUG}?utm=1`, `/articles/zh/${SLUG}?utm=1`],
    [`/news/en-US/${NEWS}?from=pin`, `/news/en/${NEWS}?from=pin`]
  ]) {
    const res = await go(request, from)
    expect(res.status(), from).toBe(301)
    expect(located(res.headers()['location']), from).toBe(to)
  }
})

test('R9-08 无前缀旧详情 301 中文，非法语言 404', async ({ request }) => {
  for (const [from, to] of [[`/articles/${SLUG}`, `/articles/zh/${SLUG}`], [`/news/${NEWS}`, `/news/zh/${NEWS}`]]) {
    const res = await go(request, from)
    expect(res.status(), from).toBe(301)
    expect(located(res.headers()['location']), from).toBe(to)
  }
  expect((await go(request, '/xx/news')).status(), '/xx/news').toBe(404)
})

test('R12-01 无前缀入口恒 301 中文，忽略 cookie 语言', async ({ request }) => {
  for (const [from, to] of [['/', '/zh/'], ['/news', '/zh/news'], ['/articles', '/zh/articles']]) {
    const res = await go(request, from, { cookie: 'galley_locale=en-US' })
    expect(res.status(), from).toBe(301)
    expect(located(res.headers()['location']), from).toBe(to)
  }
})

test('R12-02 URL 语言优先于 cookie 与 localStorage', async ({ request }) => {
  const res = await go(request, `/articles/de/${SLUG}`, { cookie: 'galley_locale=en-US' })
  const html = await res.text()
  expect(res.status()).toBe(200)
  expect(html).toMatch(/<html lang="de-DE"/)
  expect(html).toContain(`rel="canonical" href="https://ccai.xyz/articles/zh/${SLUG}"`)
})

test('R12-05 长码 301 短码，非法 locale 404 且不跳登录', async ({ request }) => {
  for (const [from, to] of [['/zh-CN/', '/zh/'], ['/en-US/news', '/en/news'], ['/de-DE/articles', '/de/articles']]) {
    const res = await go(request, from)
    expect(res.status(), from).toBe(301)
    expect(located(res.headers()['location']), from).toBe(to)
  }
  for (const path of ['/xx/news', '/xx/', `/articles/xx/${SLUG}`]) {
    const res = await go(request, path)
    expect(res.status(), path).toBe(404)
    expect(res.headers()['location'] ?? '', path).not.toContain('/login')
  }
})

test('R12-06 五语言前缀路由 SSR 200，不被嵌入中间件打进 SPA 登录', async ({ request }) => {
  for (const lang of LOCALES) {
    const paths = [`/${lang}/`, `/${lang}/news`, `/${lang}/articles`, `/articles/${lang}/${SLUG}`, `/news/${lang}/${NEWS}`]
    for (const path of paths) {
      const res = await go(request, path)
      const html = await res.text()
      expect(res.status(), path).toBe(200)
      expect(html, path).toContain('<html lang=')
      expect(html, path).not.toContain('/login?redirect')
    }
  }
})
