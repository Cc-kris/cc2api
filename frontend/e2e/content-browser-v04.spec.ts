import { expect, test, type Page } from '@playwright/test'
// TASK-033 前台浏览器验收（R7-01/03, R9-09, R12-04, R13-01～03, R14-01/02, R15-01/02, COMP-01）
const BASE = new URL(process.env.E2E_BASE_URL ?? 'http://127.0.0.1:3100')
const NEWS = '/news/zh/ccai-api-gateway-news'
const ART = '/articles/zh/ccai-api-practice'
const COVERED = '/articles/zh/llm-routing-strategy-best-practices'
const ratio = (page: Page, sel: string) => page.locator(sel).first().evaluate((e) => getComputedStyle(e).aspectRatio.replace(/\s/g, ''))
const setTheme = (page: Page, dark: boolean) => page.evaluate((d) => document.documentElement.classList.toggle('dark', d), dark)
const widths = async (page: Page) => page.evaluate(() => ({ docW: document.documentElement.scrollWidth, winW: innerWidth }))

test.beforeEach(async ({ context }) => {
  await context.addCookies([{ name: 'galley_locale', value: 'zh-CN', url: BASE.origin }])
})

test('R7-01 列表封面桌面左图 240px/16:9、移动顶部通栏 16:9，详情封面 16:9', async ({ page }) => {
  await page.goto('/news')
  const card = page.locator('.neo-news-spotlight').first()
  const cover = card.locator('[data-testid="spotlight-cover"]')
  await expect(cover).toBeVisible()
  expect(await ratio(page, '[data-testid="spotlight-cover"]')).toBe('16/9')
  const c = (await cover.boundingBox())!
  const title = (await card.locator('.spotlight-title').boundingBox())!
  const cardBox = (await card.boundingBox())!
  if ((page.viewportSize()?.width ?? 1280) >= 1024) {
    expect(Math.round(c.width), '桌面列表封面 240px').toBe(240)
    expect(c.x + c.width, '封面在标题左侧').toBeLessThanOrEqual(title.x + 1)
  } else {
    expect(c.y, '移动端封面在标题上方').toBeLessThan(title.y)
    expect(c.width, '移动端顶部通栏').toBeGreaterThan(cardBox.width * 0.8)
  }
  await page.goto(NEWS)
  const detail = page.locator('[data-testid="detail-cover"]')
  await expect(detail).toBeVisible()
  expect(await ratio(page, '[data-testid="detail-cover"]')).toBe('16/9')
  expect(await detail.locator('img, [role="img"]').count()).toBeGreaterThan(0)
})

test('R7-03 封面加载失败不出现破图，占位保留尺寸', async ({ page }) => {
  await page.route('**/media/**', (route) => route.fulfill({ status: 404, body: '' }))
  await page.goto(NEWS)
  const cover = page.locator('[data-testid="detail-cover"]')
  await expect(cover).toBeVisible()
  await page.waitForTimeout(1000)
  const broken = await page.evaluate(
    () => [...document.querySelectorAll('[data-testid="detail-cover"] img')].filter((i) => (i as HTMLImageElement).offsetParent && (i as HTMLImageElement).naturalWidth === 0).length
  )
  expect(broken, '可见破图数量').toBe(0)
  expect((await cover.boundingBox())!.height, '占位保留尺寸').toBeGreaterThan(50)
})

test('R9-09 详情标题/阅读时间/推荐去重且不含自身', async ({ page }) => {
  await page.goto(NEWS)
  await expect(page.locator('h1')).toBeVisible()
  await expect(page.getByText(/\d+\s*分钟阅读/).first()).toBeVisible()
  const related = page.locator('.more-related-card')
  const n = await related.count()
  expect(n).toBeGreaterThan(0)
  expect(n).toBeLessThanOrEqual(6)
  const hrefs = await related.evaluateAll((els) => els.map((e) => e.getAttribute('href') ?? ''))
  expect(new Set(hrefs).size, '推荐去重').toBe(n)
  expect(hrefs.join(), '不含当前文章').not.toContain('ccai-api-gateway-news')
})

test('R12-04 五语言切换跳对应短码前缀且不丢当前详情', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto(NEWS)
  await page.click('.lang-switch-btn')
  await expect(page.locator('.lang-menu .lang-option')).toHaveCount(5)
  await page.locator('.lang-menu .lang-option', { hasText: 'Deutsch' }).click()
  await page.waitForURL(/\/news\/de\//)
  expect(page.url()).toContain('ccai-api-gateway-news')
  await page.click('.lang-switch-btn')
  await page.locator('.lang-menu .lang-option', { hasText: 'English' }).click()
  await page.waitForURL(/\/news\/en\//)
})

test('R13-01 精选 1 大 + 最多 9 小，三视口网格与无横向滚动', async ({ page }) => {
  await page.goto('/articles')
  await expect(page.locator('[data-testid="featured-large-card"]')).toHaveCount(1)
  const small = await page.locator('[data-testid="featured-small-grid"] > *').count()
  expect(small, '小卡 1～9').toBeGreaterThan(0)
  expect(small, '小卡 1～9').toBeLessThanOrEqual(9)
  for (const w of [1440, 1024, 390]) {
    await page.setViewportSize({ width: w, height: 900 })
    await page.waitForTimeout(250)
    const { docW, winW } = await widths(page)
    expect(docW, `${w}px 横向滚动`).toBeLessThanOrEqual(winW + 1)
    const cols = await page.locator('[data-testid="featured-small-grid"]').evaluate((e) => getComputedStyle(e).gridTemplateColumns.split(' ').length)
    expect([1, 2, 3], `${w}px 栅格列数`).toContain(cols)
  }
})

test('R13-02 精选徽章柠绿底深色字，暗色主题下仍一致', async ({ page }) => {
  await page.goto('/articles')
  for (const dark of [false, true]) {
    await setTheme(page, dark)
    const badge = await page.evaluate(() => {
      const hit = [...document.querySelectorAll('*')].find(
        (e) => getComputedStyle(e).backgroundColor === 'rgb(204, 255, 0)' && (e.textContent ?? '').trim().length > 0
      )
      return hit ? { color: getComputedStyle(hit).color, text: (hit.textContent ?? '').trim().slice(0, 12) } : null
    })
    expect(badge, `dark=${dark} 柠绿徽章存在`).not.toBeNull()
    expect(badge!.color, `dark=${dark} 深色字`).toBe('rgb(18, 19, 22)')
  }
})

test('R13-03 0 篇精选整段隐藏，长标题两行截断', async ({ page }) => {
  await page.route('**/api/v1/public/articles/featured*', (route) => route.fulfill({ json: { code: 0, message: 'success', data: [] } }))
  await page.goto('/articles')
  await expect(page.locator('[data-testid="featured-section"]')).toHaveCount(0)
  await page.unroute('**/api/v1/public/articles/featured*')
  await page.goto('/articles')
  const clamp = await page.locator('[data-testid="featured-large-card"] [class*="title"]').first().evaluate((e) => getComputedStyle(e).webkitLineClamp)
  expect(clamp).toBe('2')
})

test('R14-01 公开四类页面双主题无 #10B981 旧绿渲染残留', async ({ page }) => {
  const LEGACY = 'rgb(16, 185, 129)'
  for (const path of ['/news', '/articles', NEWS, ART]) {
    for (const dark of [false, true]) {
      await page.goto(path)
      await setTheme(page, dark)
      const hits = await page.evaluate((legacy) => {
        const out: string[] = []
        for (const el of document.querySelectorAll('[class], button, a, span, div')) {
          const s = getComputedStyle(el)
          for (const prop of ['color', 'backgroundColor', 'borderTopColor', 'borderBottomColor'] as const) {
            if (s[prop] === legacy) out.push(`${el.tagName}.${String(el.className).slice(0, 40)}:${prop}`)
          }
        }
        return [...new Set(out)]
      }, LEGACY)
      expect(hits, `${path} dark=${dark}`).toEqual([])
    }
  }
})

test('R14-02 四类页面双主题三视口无横向滚动', async ({ page }) => {
  for (const path of ['/news', '/articles', NEWS, ART]) {
    for (const dark of [false, true]) {
      await page.goto(path)
      await setTheme(page, dark)
      for (const w of [1440, 1024, 390]) {
        await page.setViewportSize({ width: w, height: 900 })
        await page.waitForTimeout(150)
        const { docW, winW } = await widths(page)
        expect(docW, `${path} ${w}px dark=${dark}`).toBeLessThanOrEqual(winW + 1)
      }
    }
  }
})

test('R15-01/02 后台下拉、输入框、表格统一组件化', async ({ page, request }) => {
  const login = await request.post(`${BASE.origin}/api/v1/auth/login`, {
    data: { email: process.env.S2A_E2E_EMAIL ?? 'e2e-task031@sub2api.local', password: process.env.S2A_E2E_PASSWORD ?? 'TASK031-e2e-Passw0rd!' }
  })
  test.skip(!login.ok(), `管理员登录失败 ${login.status()}`)
  const token = (await login.json()).data.access_token
  await page.addInitScript((t) => {
    localStorage.setItem('auth_token', t)
    localStorage.setItem('auth_user', JSON.stringify({ id: 2, email: 'e2e', username: 'e2e', role: 'admin', status: 'active' }))
    localStorage.setItem('token_expires_at', String(Date.now() + 864e5))
    localStorage.setItem('admin_guide_2_admin_v4_interactive', 'true')
  }, token)
  await page.goto('/admin/content')

  const nativeSelects = page.locator('select')
  const dropdown = page.locator('.select-dropdown-portal:visible')
  const triggerOf = (name: string) => page.locator(`[data-test="${name}"]`).locator('button[aria-haspopup="listbox"]')

  // 1. 已抓取新闻：无原生 select，news 状态下拉由 common/Select 接管
  const newsStatus = triggerOf('news-status')
  await expect(newsStatus).toBeVisible({ timeout: 30_000 })
  await expect(nativeSelects, '内容中心不得渲染原生 select').toHaveCount(0)

  // 1b. 布局不变量：工具栏下拉与输入框等高、无横向溢出
  const layout = await page.evaluate(() => {
    const rect = (el: Element | null) => (el ? el.getBoundingClientRect() : null)
    const trigger = document.querySelector('.toolbar [data-test="news-status"] button')
    const input = document.querySelector('.toolbar input')
    const t = rect(trigger)
    const i = rect(input)
    return {
      triggerH: t ? Math.round(t.height) : null,
      inputH: i ? Math.round(i.height) : null,
      triggerW: t ? Math.round(t.width) : null,
      overflowX: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    }
  })
  expect(layout.triggerH, '下拉触发区高度与输入框同高（±6px）').not.toBeNull()
  expect(Math.abs((layout.triggerH as number) - (layout.inputH as number)), `下拉 ${layout.triggerH} vs 输入框 ${layout.inputH}`).toBeLessThanOrEqual(6)
  expect(layout.triggerW, '下拉宽度受控（不会被 flex 拉到撑破工具栏）').toBeLessThanOrEqual(200)
  expect(layout.overflowX, '内容中心无横向溢出').toBeLessThanOrEqual(0)

  // 2. 真实鼠标点击：点开 trigger → 点选项「已发布」→ 断言 trigger 值 / aria-selected / 按 status 重拉
  const newsRequests: string[] = []
  page.on('request', (r) => {
    if (r.url().includes('/admin/content/news?') && /[?&]status=published(&|$)/.test(r.url())) newsRequests.push(r.url())
  })
  await newsStatus.click()
  await expect(newsStatus).toHaveAttribute('aria-expanded', 'true')
  await expect(dropdown).toBeVisible()
  await dropdown.getByRole('option', { name: '已发布', exact: true }).click()
  await expect(newsStatus, '鼠标选中后 trigger 显示所选值').toContainText('已发布')
  await expect(newsStatus).toHaveAttribute('aria-expanded', 'false')
  await expect.poll(() => newsRequests.length, {
    message: '鼠标选择后按 status=published 重新拉取新闻列表',
    timeout: 10_000,
  }).toBeGreaterThan(0)
  await newsStatus.click()
  await expect(newsStatus).toHaveAttribute('aria-expanded', 'true')
  await expect(dropdown.getByRole('option', { name: '已发布', exact: true }), '选中项 aria-selected').toHaveAttribute('aria-selected', 'true')
  // 真实鼠标展开后直接按 Escape（不人为 focus，Safari 下 button 不会被鼠标聚焦）：下拉必须关闭
  await page.keyboard.press('Escape')
  await expect(newsStatus, 'Escape 关闭下拉（无需事先聚焦）').toHaveAttribute('aria-expanded', 'false')
  await expect(dropdown).toHaveCount(0)
  await expect(newsStatus, 'Escape 不清空已选值').toContainText('已发布')
  // 不重复关闭、不干扰页面其它键盘监听：再次 Escape 后下拉不会重开，路由保持不变
  await page.keyboard.press('Escape')
  await expect(dropdown).toHaveCount(0)
  await expect(page).toHaveURL(/\/admin\/content/)

  // 3. 深度解析：真实键盘（Enter 打开 → ArrowDown 移动 → Enter 确认），并断言 aria-selected
  await page.getByRole('button', { name: '深度解析', exact: true }).click()
  const articleStatus = triggerOf('article-status')
  await expect(articleStatus).toBeVisible()
  await expect(nativeSelects, '深度解析无原生 select').toHaveCount(0)
  const articleRequests: string[] = []
  page.on('request', (r) => {
    if (r.url().includes('/admin/content/articles?') && /[?&]status=published(&|$)/.test(r.url())) articleRequests.push(r.url())
  })
  await expect(articleStatus).toHaveAttribute('aria-expanded', 'false')
  await articleStatus.focus()
  await page.keyboard.press('Enter')
  await expect(articleStatus, 'Enter 打开下拉').toHaveAttribute('aria-expanded', 'true')
  await expect(dropdown).toBeVisible()
  // 打开后初始高亮索引由组件决定，真实键盘逐格下移直到落在「已发布」
  let activeText = ''
  for (let i = 0; i < 4; i += 1) {
    await page.keyboard.press('ArrowDown')
    const id = await articleStatus.getAttribute('aria-activedescendant')
    if (!id) continue
    activeText = (await page.locator(`[id="${id}"]`).innerText()).trim()
    if (activeText === '已发布') break
  }
  expect(activeText, 'ArrowDown 可移动到「已发布」').toBe('已发布')
  const activeId = await articleStatus.getAttribute('aria-activedescendant')
  expect(activeId, 'ArrowDown 后有活动选项').toBeTruthy()
  await page.keyboard.press('Enter')
  await expect(articleStatus, 'Enter 确认后 trigger 值更新').toContainText('已发布')
  await expect(articleStatus).toHaveAttribute('aria-expanded', 'false')
  await expect.poll(() => articleRequests.length, {
    message: '键盘选择后按 status=published 重新拉取文章列表',
    timeout: 10_000,
  }).toBeGreaterThan(0)
  // Space 打开 → aria-selected 为当前值
  await page.keyboard.press(' ')
  await expect(articleStatus, 'Space 打开下拉').toHaveAttribute('aria-expanded', 'true')
  await expect(dropdown.getByRole('option', { name: '已发布', exact: true }), '键盘确认后 aria-selected').toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('Escape')
  await expect(dropdown).toHaveCount(0)

  // 4. 文章编辑弹窗（新建）与其余视图同样不得出现原生 select
  await page.getByRole('button', { name: '+ 新建文章', exact: true }).click()
  await expect(page.locator('[data-test="article-category"]')).toBeVisible()
  await expect(nativeSelects, '文章编辑弹窗无原生 select').toHaveCount(0)
  await page.locator('.modal-backdrop .close-btn').first().click()
  await expect(page.locator('.modal-backdrop')).toHaveCount(0)

  // 5. 新闻源 / 自动化设置 / 分类与标签 / AI 创作 / BI 分析
  await page.getByRole('button', { name: '新闻动态', exact: true }).click()
  for (const sub of ['新闻源', '自动化设置']) {
    await page.getByRole('button', { name: sub, exact: true }).click()
    await expect(nativeSelects, `${sub} 无原生 select`).toHaveCount(0)
  }
  await page.getByRole('button', { name: '新闻源', exact: true }).click()
  await expect(page.locator('[data-test="news-source-type"]')).toBeVisible()
  await page.getByRole('button', { name: '自动化设置', exact: true }).click()
  await expect(page.locator('[data-test="automation-publish-mode"]')).toBeVisible()

  for (const tab of ['分类与标签', 'AI 创作', 'BI 分析']) {
    await page.getByRole('button', { name: tab, exact: true }).click()
    await expect(nativeSelects, `${tab} 无原生 select`).toHaveCount(0)
  }
  await page.getByRole('button', { name: 'AI 创作', exact: true }).click()
  await page.getByRole('button', { name: '文章生成', exact: true }).click()
  const aiGrid = page.locator('.ai-studio-grid')
  if (await aiGrid.count()) {
    // 有默认模型：brief 目标语言必须是 common/Select 且可交互
    const briefLang = triggerOf('brief-language')
    await expect(briefLang).toBeVisible()
    await briefLang.click()
    await expect(dropdown).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(dropdown).toHaveCount(0)
  } else {
    // 无默认模型：必须给出引导态，且同样不得出现原生 select
    await expect(page.locator('.ai-config-required')).toBeVisible()
  }
  await expect(nativeSelects, 'AI 创作无原生 select').toHaveCount(0)

  // 6. 输入框与表格仍为统一组件（R15-02）
  await page.getByRole('button', { name: '深度解析', exact: true }).click()
  const input = page.locator('.toolbar input, input.search-input').first()
  const border = await input.evaluate((e) => {
    const s = getComputedStyle(e)
    return { top: s.borderTopWidth, bottom: s.borderBottomWidth }
  })
  expect(border.top, '输入框非仅下划线').not.toBe('0px')
  expect(await page.locator('table tbody td').first().evaluate((e) => getComputedStyle(e).borderBottomWidth), '表格分隔线').not.toBe('0px')
})

test('COMP-01 五语言×双主题×三视口冒烟无脚本错误', async ({ page, context }) => {
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  for (const locale of ['zh-CN', 'en-US', 'ru-RU', 'fr-FR', 'de-DE']) {
    await context.addCookies([{ name: 'galley_locale', value: locale, url: BASE.origin }])
    for (const dark of [false, true]) {
      await page.goto('/news')
      await setTheme(page, dark)
      for (const w of [1440, 1024, 390]) {
        await page.setViewportSize({ width: w, height: 900 })
        await page.waitForTimeout(120)
        const { docW, winW } = await widths(page)
        expect(docW, `${locale} dark=${dark} ${w}px`).toBeLessThanOrEqual(winW + 1)
      }
      expect(await page.locator('#app, main').count(), `${locale} 主内容渲染`).toBeGreaterThan(0)
    }
  }
  expect(errors, '运行期脚本错误').toEqual([])
})
