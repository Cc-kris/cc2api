import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
// TASK-031 内容管理核心链路 E2E（真实栈：Go 后端 :8080 + Vite :3100 + 真实 PostgreSQL），覆盖 R1-01/R1-02/R1-03/R1-04/R2-01/R5-01/R6-01/R6-05/R9-01/R10-06/R10-12
const BASE = process.env.S2A_BASE_URL ?? 'http://127.0.0.1:3100'
const EMAIL = process.env.S2A_E2E_EMAIL ?? 'e2e-task031@sub2api.local'
const PASSWORD = process.env.S2A_E2E_PASSWORD ?? 'TASK031-e2e-Passw0rd!'
const API = '/api/v1/admin/content'
const stamp = Date.now().toString().slice(-7)
let token = ''
const auth: Record<string, string> = {}
test.beforeAll(async ({ request }) => {
  const res = await request.post(`${BASE}/api/v1/auth/login`, { data: { email: EMAIL, password: PASSWORD } })
  if (!res.ok()) test.skip(true, `真实栈不可用：${BASE} 登录返回 ${res.status()}`)
  token = (await res.json()).data.access_token
  auth.Authorization = `Bearer ${token}`
})
async function api(request: APIRequestContext, method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string, data?: unknown) {
  const res = await request.fetch(`${BASE}${API}${path}`, { method, headers: auth, data })
  return { status: res.status(), body: res.status() === 204 ? null : await res.json().catch(() => null) }
}
const rows = (page: Page) => page.locator('table tbody tr')
const newArticle = (request: APIRequestContext, title: string, body: string) => api(request, 'POST', '/articles', { title, excerpt: 'TASK-031', bodyMarkdown: body, locale: 'zh-CN' })
async function openAdmin(page: Page, tab = '深度解析') {
  await page.addInitScript(([t, u]) => {
    localStorage.setItem('auth_token', t)
    localStorage.setItem('auth_user', JSON.stringify(u))
    localStorage.setItem('token_expires_at', String(Date.now() + 86_400_000))
    // 引导 tour（driver.js）会自动弹出并拦截弹窗按钮点击；模拟“已看过引导”的老用户，
    // 否则每次全新 context 都会重新触发新手引导（storageKey 见 useOnboardingTour.ts）
    localStorage.setItem('admin_guide_2_admin_v4_interactive', 'true')
    localStorage.setItem('admin_guide_guest_user_v4_interactive', 'true')
  }, [token, { id: 2, email: EMAIL, username: 'e2e-task031', role: 'admin', status: 'active' }] as [string, unknown])
  await page.goto(`${BASE}/admin/content`)
  await expect(page.locator('.tabs button').first()).toBeVisible({ timeout: 30_000 })
  // 公告弹窗（z-[120]）会拦截点击，先按真实用户路径逐条关闭
  await page.evaluate(async () => {
    for (let i = 0; i < 3; i++) {
      const popup = Array.from(document.querySelectorAll('div')).find((d) => String(d.className).includes('z-[120]'))
      const btn = popup?.querySelector('button') as HTMLElement | null
      if (!btn) break
      btn.click()
      await new Promise((r) => setTimeout(r, 400))
    }
  })
  // 直接触发 Vue 事件，避免列表轮询重渲染导致 actionability 重试超时
  await page.locator('.tabs button').evaluateAll((els, name) => (els.find((el) => (el.textContent || '').trim() === name) as HTMLElement | undefined)?.click(), tab)
  await expect(page.locator('.tabs button.active', { hasText: tab }).first()).toBeVisible()
}

test('R1-01 标签删除二次确认：取消不删，确认后删除并同步 taxonomy', async ({ page, request }) => {
  const name = `TASK031-标签-${stamp}`
  expect((await api(request, 'POST', '/articles/taxonomy/mutate', { scope: 'tag', action: 'add', name })).status).toBe(200)
  await openAdmin(page, '分类与标签')
  await page.locator('.subnav button', { hasText: '标签' }).first().click()
  await page.locator('.toolbar input.search-input').fill(name)
  const row = rows(page).filter({ hasText: name })
  const dialog = page.locator('.taxonomy-dialog')
  await expect(row).toHaveCount(1)
  await row.getByRole('button', { name: '删除' }).click()
  await expect(dialog).toContainText(name)
  await expect(dialog).toContainText('确认删除')
  await dialog.getByRole('button', { name: '取消' }).click()
  await expect(dialog).toHaveCount(0)
  await expect(row).toHaveCount(1)
  await row.getByRole('button', { name: '删除' }).click()
  await dialog.getByRole('button', { name: '确认删除' }).click()
  await expect(rows(page).filter({ hasText: name })).toHaveCount(0)
  expect((await api(request, 'GET', '/articles/taxonomy')).body.data.tags).not.toContain(name)
})

test('R1-02/R1-03 抓取间隔：空值存 NULL 继承全局，0/14/1441 拒绝，15/1440 通过', async ({ request }) => {
  const mk = (interval?: number) => api(request, 'POST', '/news-sources', {
    name: `TASK031-src-${stamp}-${interval ?? 'inherit'}`,
    url: `https://example.com/task031/${stamp}-${interval ?? 'inherit'}.xml`,
    sourceType: 'rss', enabled: false, termsConfirmed: true,
    ...(interval === undefined ? {} : { intervalMinutes: interval }),
  })
  for (const bad of [0, 14, 1441]) expect((await mk(bad)).status, `interval=${bad} 应拒绝`).toBe(400)
  for (const ok of [15, 1440]) {
    const created = await mk(ok)
    expect(created.status, `interval=${ok} 应通过`).toBe(201)
    expect(created.body.data.intervalMinutes).toBe(ok)
  }
  const inherited = await mk()
  expect(inherited.body.data.intervalMinutes, '空值必须存 NULL 而不是被 15 覆盖').toBeNull()
  const listed = (await api(request, 'GET', '/news-sources?page=1&page_size=50')).body.data.items.find((s: any) => s.id === inherited.body.data.id)
  expect(listed.intervalMinutes, '刷新后仍须为 NULL').toBeNull()
})

test('R1-02 UI 回归：留空间隔显示“跟随全局”，独立间隔显示固定值（DEFECT-031-01 不得复现）', async ({ page, request }) => {
  // 自建数据，避免依赖其它用例/worker 的临时源（fullyParallel 下 stamp 不共享）
  const inheritName = `TASK031-ui-inherit-${stamp}`
  const fixedName = `TASK031-ui-fixed45-${stamp}`
  const mk = (name: string, intervalMinutes?: number) => api(request, 'POST', '/news-sources', {
    name,
    url: `https://example.com/task031/${name}.xml`,
    sourceType: 'rss',
    enabled: false,
    termsConfirmed: true,
    ...(intervalMinutes === undefined ? {} : { intervalMinutes }),
  })
  const inherited = await mk(inheritName)
  expect(inherited.status).toBe(201)
  expect(inherited.body.data.intervalMinutes, '留空必须存 NULL，不得被 15 覆盖').toBeNull()
  const fixed = await mk(fixedName, 45)
  expect(fixed.status).toBe(201)
  expect(fixed.body.data.intervalMinutes, '独立间隔必须落库为 45').toBe(45)

  await openAdmin(page, '新闻动态')
  await page.locator('.subnav button', { hasText: '新闻源' }).first().click()
  const inheritRow = rows(page).filter({ hasText: inheritName })
  await expect(inheritRow, '列表应展示刚创建的继承源').toHaveCount(1)
  await expect(inheritRow, 'intervalMinutes=NULL 必须显示跟随全局').toContainText(/跟随全局（\d+ 分钟）/)
  await expect(inheritRow, '不得把全局 fetchCron 显示成源自身间隔').not.toContainText('每 15 分钟抓取一次')
  const fixedRow = rows(page).filter({ hasText: fixedName })
  await expect(fixedRow).toHaveCount(1)
  await expect(fixedRow, '有独立 intervalMinutes 时显示该固定值').toContainText('每 45 分钟抓取一次')
})

test('R1-04 ≥1 万字正文保存回显不截断（HTML 源码回显 + 重新拉取）', async ({ page, request }) => {
  const title = `TASK031-long-${stamp}`
  const body = `${'正文校验'.repeat(4200)}TAIL031`
  const created = await newArticle(request, title, body)
  expect(created.status).toBe(201)
  await openAdmin(page)
  await page.locator('input[placeholder*="搜索"]').first().fill(title)
  await rows(page).filter({ hasText: title }).getByRole('button', { name: '编辑' }).first().click()
  await page.getByRole('button', { name: 'HTML 源码' }).click()
  const value = await page.locator('textarea.html-source-area').inputValue({ timeout: 20_000 })
  expect(value.length, '源码回显应覆盖全文').toBeGreaterThanOrEqual(10_000)
  expect(value, '回显尾部标记不得截断').toContain('TAIL031')
  await page.locator('.article-editor-modal .modal-drawer-footer button.primary').click()
  await expect(page.locator('.article-editor-modal')).toHaveCount(0, { timeout: 20_000 })
  const saved = (await api(request, 'GET', `/articles/${created.body.data.id}`)).body.data
  expect(saved.bodyMarkdown.length, '保存后全文长度不得缩水').toBeGreaterThanOrEqual(10_000)
  expect(saved.bodyMarkdown, '保存后尾部标记不得丢失').toContain('TAIL031')
})

test('R2-01 1440/1024/390 视口无横向溢出，编辑弹窗宽度不超出视口', async ({ page }) => {
  for (const width of [1440, 1024, 390]) {
    await page.setViewportSize({ width, height: 900 })
    await openAdmin(page)
    expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth), `视口 ${width} 横向溢出`).toBeLessThanOrEqual(1)
  }
  await page.setViewportSize({ width: 1440, height: 900 })
  await rows(page).first().getByRole('button', { name: '编辑' }).click()
  const box = await page.locator('.article-editor-modal').boundingBox()
  expect(box!.width).toBeLessThanOrEqual(1440)
  expect(box!.x).toBeGreaterThanOrEqual(0)
})

test('R5-01 组字阶段 Enter 不生成词条，组字结束后才生成', async ({ page }) => {
  await openAdmin(page)
  await rows(page).first().getByRole('button', { name: '编辑' }).click()
  const input = page.locator('.article-editor-modal .tag-input input').first()
  const chips = page.locator('.article-editor-modal .tag-chip')
  const before = await chips.count()
  await input.evaluate((el) => {
    const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    el.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }))
    set.call(el, 'TASK031组字')
    el.dispatchEvent(new Event('input', { bubbles: true }))
    el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', keyCode: 229, isComposing: true, bubbles: true }))
  })
  await expect(chips, '组字期间 Enter 不应生成词条').toHaveCount(before)
  await input.evaluate((el) => {
    el.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true, data: 'TASK031组字' }))
    el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
  })
  await expect.poll(async () => chips.count(), { message: '组字结束后 Enter 应生成词条' }).toBeGreaterThan(before)
})

test('R6-01/R6-05 精选以 featured_at 为唯一真源：设置即写入时间，列表按时间倒序', async ({ request }) => {
  const made: number[] = []
  for (const i of [1, 2]) {
    const created = await newArticle(request, `TASK031-featured-${i}-${stamp}`, `精选校验 ${i}`)
    expect((await api(request, 'POST', `/articles/${created.body.data.id}/publish`)).status, '仅已发布文章可精选').toBe(200)
    made.push(created.body.data.id)
  }
  for (const id of made) expect((await api(request, 'POST', '/articles/taxonomy/mutate', { scope: 'featured_article', action: 'set', name: String(id) })).status).toBe(200)
  const adminItems = (await api(request, 'GET', '/articles?page=1&page_size=100')).body.data.items
  const byId = new Map(adminItems.map((a: any) => [a.id, a]))
  const firstAt = (byId.get(made[0]) as any)?.featuredAt
  const secondAt = (byId.get(made[1]) as any)?.featuredAt
  expect(firstAt, '设为精选必须写入 featured_at').toBeTruthy()
  expect(secondAt, '设为精选必须写入 featured_at').toBeTruthy()
  expect(new Date(secondAt).getTime(), '后设置的精选时间应更新').toBeGreaterThan(new Date(firstAt).getTime())
  const times = adminItems.filter((a: any) => a.featuredAt).map((a: any) => new Date(a.featuredAt).getTime())
  expect(times, '后台列表 featured_at 必须递减').toEqual([...times].sort((a, b) => b - a))
  expect((await api(request, 'GET', `/articles/${made[1]}`)).body.data.isFeatured).toBe(true)
  // 前台精选（真实上限在公共接口：MaxPublicFeaturedArticles=10），按设置时间倒序
  const publicRes = await request.get(`${BASE}/api/v1/public/articles/featured`)
  expect(publicRes.ok(), '公共精选接口应可匿名访问').toBe(true)
  const publicFeatured = (await publicRes.json()).data as any[]
  expect(publicFeatured.length, '前台最多展示 10 条精选').toBeLessThanOrEqual(10)
  const ids = publicFeatured.map((a: any) => a.id)
  expect(ids, '新设置的精选应出现在前台').toEqual(expect.arrayContaining([made[0], made[1]]))
  expect(ids.indexOf(made[1]), '最新设置的精选应排在更前').toBeLessThan(ids.indexOf(made[0]))
})

test('R10-06/R10-12 未配置加密/默认模型时返回明确错误且不落明文 Key', async ({ request }) => {
  const cfg = await api(request, 'GET', '/articles/ai-configs')
  test.skip(cfg.status === 200, '本环境已配置加密密钥，改由配置管理用例覆盖')
  expect(cfg.status).toBe(503)
  expect(cfg.body.reason).toBe('encryption_not_configured')
  const write = await api(request, 'POST', '/articles/ai-configs/create', { label: 'TASK031', provider: 'openai', baseUrl: 'https://x.test/v1', apiKey: 'sk-task031-plain', models: ['m'], defaultModel: 'm' })
  expect(JSON.stringify(write.body), '错误响应不得回显明文 Key').not.toContain('sk-task031-plain')
  expect([409, 503]).toContain(write.status)
  const article = (await api(request, 'GET', '/articles?page=1&page_size=1')).body.data.items[0]
  const seo = await api(request, 'POST', `/articles/${article.id}/seo-autofill`)
  expect([400, 503]).toContain(seo.status)
  expect(JSON.stringify(seo.body)).toMatch(/model_not_configured|encryption_not_configured/)
})

test('R9-01 翻译任务：缺模型被拒，指定模型后入队为 queued 而非假成功', async ({ request }) => {
  const article = (await api(request, 'GET', '/articles?page=1&page_size=1')).body.data.items[0]
  test.skip(!article, '缺少可用于翻译的文章')
  expect((await api(request, 'POST', `/articles/${article.id}/translations/tasks`, { locales: ['en-US'], model: '' })).status, '缺模型必须 400').toBe(400)
  const queued = await api(request, 'POST', `/articles/${article.id}/translations/tasks`, { locales: ['de-DE'], model: 'MiniMax-M3' })
  expect(queued.status).toBe(202)
  expect(queued.body.data.status, '只能入队，不能直接 published').toBe('queued')
  expect(queued.body.data.targetLocale).toBe('de-DE')
  const translations = (await api(request, 'GET', `/articles/${article.id}/translations`)).body.data
  const list = Array.isArray(translations) ? translations : translations.items ?? []
  expect(list.filter((t: any) => t.locale === 'de-DE' && t.status === 'published')).toHaveLength(0)
})
