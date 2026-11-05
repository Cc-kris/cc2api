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
  const login = await request.post(`${ADMIN}/api/v1/auth/login`, { data: {
    email: process.env.S2A_E2E_EMAIL ?? 'e2e-task031@sub2api.local', password: process.env.S2A_E2E_PASSWORD ?? 'TASK031-e2e-Passw0rd!' } })
  expect(login.ok(), `管理员登录 ${login.status()}`).toBeTruthy()
  const auth = { Authorization: `Bearer ${(await login.json()).data.access_token}` }
  // 自包含夹具：不再依赖后台前 100 篇文章里恰好存在已发布译文，slug 唯一避免并发串扰。
  const slug = `e2e-r9-02-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  const create = await request.post(`${ADMIN}/api/v1/admin/content/articles`, { headers: auth, data: {
    locale: 'zh-CN', slug, title: 'R9-02 夹具中文标题', excerpt: 'R9-02 中文摘要', bodyMarkdown: 'R9-02夹具中文正文',
    seoTitle: 'R9-02 中文 SEO', seoDescription: 'R9-02 中文描述', status: 'draft' } })
  expect(create.ok(), `夹具文章创建 ${create.status()}`).toBeTruthy()
  const article = (await create.json()).data
  let translationId = 0
  try {
    const pub = (await (await request.post(`${ADMIN}/api/v1/admin/content/articles/${article.id}/publish`, { headers: auth })).json()).data
    expect(pub.version, '发布后文章版本递增').toBeGreaterThan(article.version)
    const trRes = await request.post(`${ADMIN}/api/v1/admin/content/articles/${article.id}/translations`, { headers: auth, data: {
      contentType: 'article', contentId: article.id, sourceVersion: pub.version, sourceLocale: 'zh-CN', locale: 'en-US', slug,
      title: 'R9-02 fixture EN', excerpt: 'EN excerpt', bodyMarkdown: 'R9-02 fixture English body', seoTitle: 'EN SEO', seoDescription: 'EN description', status: 'translation_draft' } })
    expect(trRes.ok(), `夹具译文创建 ${trRes.status()}`).toBeTruthy()
    const tr = (await trRes.json()).data
    translationId = tr.id
    expect(tr.sourceVersion, '译文绑定当前版本').toBe(pub.version)
    expect((await request.post(`${ADMIN}/api/v1/admin/content/articles/translations/${tr.id}/publish`, { headers: auth })).status(), '发布译文').toBe(200)
    const current = await (await go(request, `/articles/en/${slug}`)).text()
    expect(current, '当前版本 self-canonical').toContain(`rel="canonical" href="https://ccai.xyz/articles/en/${slug}"`)
    expect(current, '当前版本可索引').toContain('content="index, follow"')
    expect(current, '当前版本渲染英文译文').toContain('R9-02 fixture English body')
    // 版本推进令已发布译文过期：unpublish/publish 各 +1 版本，译文 source_version 停在 pub.version。
    await request.post(`${ADMIN}/api/v1/admin/content/articles/${article.id}/unpublish`, { headers: auth })
    const bumped = (await (await request.post(`${ADMIN}/api/v1/admin/content/articles/${article.id}/publish`, { headers: auth })).json()).data
    const stale = (await (await request.get(`${ADMIN}/api/v1/admin/content/articles/${article.id}/translations`, { headers: auth })).json()).data
      .find((t: { id: number }) => t.id === tr.id)
    expect(stale?.status, '旧版本译文仍为 published（证明是版本门禁而非缺失）').toBe('published')
    expect(stale?.sourceVersion, '译文版本落后当前文章').toBeLessThan(bumped.version)
    const fallback = await (await go(request, `/articles/en/${slug}`)).text()
    expect(fallback, '旧版本不得再对外发布英文译文').not.toContain('R9-02 fixture English body')
    expect(fallback, '旧版本回落中文正文').toContain('R9-02夹具中文正文')
    expect(fallback, '旧版本 canonical 指中文').toContain(`rel="canonical" href="https://ccai.xyz/articles/zh/${slug}"`)
    expect(fallback, '旧版本必须 noindex').toContain('content="noindex, follow"')
  } finally {
    if (translationId) await request.delete(`${ADMIN}/api/v1/admin/content/articles/translations/${translationId}`, { headers: auth })
    await request.delete(`${ADMIN}/api/v1/admin/content/articles/${article.id}`, { headers: auth })
  }
})

test('R9-03 无 JS 原文 HTML：en 英文正文，de 中文正文且德语界面', async ({ request }) => {
  const en = await (await go(request, `/articles/en/${SLUG}`)).text()
  const de = await (await go(request, `/articles/de/${SLUG}`)).text()
  expect(en).not.toMatch(/<script[^>]+src=/)
  expect(en).toContain('Overview')
  const body = en.match(/<section class="content">([\s\S]*?)<\/section>/)?.[1] ?? ''
  expect(body, '英文 SSR 正文不应回落中文').not.toMatch(/[\u4e00-\u9fa5]{10,}/)
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
