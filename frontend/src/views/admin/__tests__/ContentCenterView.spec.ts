import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { api, editorControl } = vi.hoisted(() => ({
  api: {
    listNews: vi.fn(),
    getNews: vi.fn(),
    updateNews: vi.fn(),
    showToast: vi.fn(),
    analyticsOverview: vi.fn(),
    analyticsTrends: vi.fn(),
    analyticsRankings: vi.fn(),
    analyticsCTR: vi.fn(),
    analyticsDistribution: vi.fn(),
    analyticsExport: vi.fn(),
  },
  editorControl: { editor: null as any },
}))

vi.mock('@/api/admin/content', () => ({
  default: new Proxy(api, {
    get(target, key: string) {
      if (key in target) return target[key as keyof typeof target]
      return vi.fn(async () => ({ data: [] }))
    },
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showToast: api.showToast }),
}))

vi.mock('@/components/common/Pagination.vue', async () => {
  const { defineComponent: component, h: createElement } = await import('vue')
  return { default: component({
    props: ['page', 'total', 'pageSize', 'showPageSizeSelector'],
    emits: ['update:page'],
    setup(props, { emit }) {
      return () => createElement('button', {
        type: 'button',
        'data-test': 'next-page',
        disabled: props.page * props.pageSize >= props.total,
        onClick: () => emit('update:page', props.page + 1),
      }, '下一页')
    },
  }) }
})

vi.mock('@/components/layout/AppLayout.vue', async () => {
  const { defineComponent: component, h: createElement } = await import('vue')
  return {
    default: component({
      setup(_, { slots }) {
        return () => createElement('div', slots.default?.())
      },
    }),
  }
})

vi.mock('vue-chartjs', async () => {
  const { defineComponent: component, h: createElement } = await import('vue')
  // 把图表 labels 渲染成可断言的文本，便于 UI 验收（真实 canvas 文本不可读）
  const ChartStub = component({
    props: { data: { type: Object, default: () => ({}) }, options: { type: Object, default: () => ({}) } },
    setup(props) {
      return () => createElement('div', { class: 'chart-stub' }, JSON.stringify({
        labels: props.data?.labels || [],
        datasets: (props.data as any)?.datasets?.map((d: any) => d.label) || [],
      }))
    },
  })
  return { Line: ChartStub, Bar: ChartStub, Doughnut: ChartStub }
})

vi.mock('@wangeditor/editor-for-vue', async () => {
  const { defineComponent: component, h: createElement } = await import('vue')
  return {
    Editor: component({
      props: { modelValue: { type: String, default: '' } },
      emits: ['update:modelValue', 'onCreated', 'onChange'],
      setup(props, { emit }) {
        return () => createElement('div', [
          createElement('textarea', {
            'data-test': 'news-rich-editor',
            value: props.modelValue,
            onInput: (event: Event) => {
              emit('update:modelValue', (event.target as HTMLTextAreaElement).value)
              emit('onChange')
            },
          }),
          createElement('button', {
            type: 'button',
            onClick: () => emit('onCreated', editorControl.editor),
          }, '创建编辑器'),
        ])
      },
    }),
    Toolbar: component({ setup: () => () => createElement('div') }),
  }
})

import { createI18n } from 'vue-i18n'
import contentV04Messages from '@/i18n/locales/contentV04'
import Select from '@/components/common/Select.vue'
import ContentCenterView from '../ContentCenterView.vue'

const persistedNews = {
  id: 1381,
  title: '长正文回显验收新闻',
  slug: 'long-body-1381',
  locale: 'zh-CN',
  status: 'draft',
  summary: '',
  bodyHtml: '',
}

function response(data: unknown) {
  return Promise.resolve({ data })
}

function makeEditor() {
  let editorHtml = '<p>wangEditor 初始内容</p>'
  return {
    getHtml: vi.fn(() => editorHtml),
    setHtml: vi.fn((html: string) => { editorHtml = html }),
    destroy: vi.fn(),
  }
}

async function openNewsEditor(wrapper: ReturnType<typeof mount>) {
  const editButton = wrapper.findAll('button').find((button) => button.text().trim() === '编辑')
  expect(editButton, 'news row should have its edit action').toBeTruthy()
  await editButton!.trigger('click')
  await flushPromises()
  const editor = wrapper.get('[data-test="news-rich-editor"]')
  await wrapper.get('button').exists()
  return editor
}

function createTestI18n() {
  return createI18n({
    legacy: false,
    locale: 'zh',
    fallbackLocale: 'zh',
    missingWarn: false,
    fallbackWarn: false,
    messages: { zh: contentV04Messages.zh },
  })
}

async function mountContentCenter() {
  const wrapper = mount(ContentCenterView, {
    global: { plugins: [createTestI18n()] },
  })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  persistedNews.bodyHtml = ''
  api.listNews.mockReset().mockImplementation(() => response({ items: [{ ...persistedNews }], total: 1 }))
  api.getNews.mockReset().mockImplementation(async () => response({ ...persistedNews }))
  api.updateNews.mockReset().mockImplementation(async (_id: number, body: any) => {
    persistedNews.bodyHtml = body.bodyHtml
    Object.assign(persistedNews, body)
    return response({ ...persistedNews })
  })
  api.showToast.mockReset()
  api.analyticsOverview.mockReset().mockImplementation(async () =>
    response({ uniqueVisitors: 12, pageViews: 34, reads: 21, impressions: 100, clicks: 7, ctr: 0.07 }),
  )
  api.analyticsTrends.mockReset().mockImplementation(async () =>
    response([
      { date: '2026-10-26', uniqueVisitors: 5, pageViews: 12, reads: 9, impressions: 40, clicks: 3, avgReadSeconds: 42.5, publishedArticles: 2, publishedNews: 1 },
      { date: '2026-10-27', uniqueVisitors: 7, pageViews: 22, reads: 12, impressions: 60, clicks: 4, avgReadSeconds: 51.8, publishedArticles: 1, publishedNews: 3 },
    ]),
  )
  api.analyticsRankings.mockReset().mockImplementation(async () =>
    response({
      items: [
        { contentType: 'article', contentId: 1, title: '深度文章A', reads: 30 },
        { contentType: 'news', contentId: 2, title: '新闻B', reads: 18 },
      ],
      total: 2,
      page: 1,
      page_size: 10,
    }),
  )
  api.analyticsCTR.mockReset().mockImplementation(async () =>
    response([{ placement: 'home_hero', impressions: 100, clicks: 9, ctr: 0.09 }]),
  )
  api.analyticsDistribution.mockReset().mockImplementation(async (dim: string) => {
    if (dim === 'locale') return response([{ key: 'zh-CN', count: 8, ratio: 0.8 }])
    if (dim === 'referrer') return response([{ key: 'search', count: 6, ratio: 0.6 }])
    if (dim === 'source') return response([{ key: 'Reuters', count: 4, ratio: 0.4 }])
    return response([{ key: '科技', count: 11, ratio: 1 }])
  })
  editorControl.editor = makeEditor()
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
    configurable: true,
    value: vi.fn(),
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('ContentCenterView news body editing integration', () => {
  it('uses server pagination totals and requests the selected 20-item page', async () => {
    api.listNews.mockImplementation(async (params: any) => response({
      items: params.page === 2 ? [{ ...persistedNews, title: '第二页新闻' }] : [{ ...persistedNews }],
      total: 45, page: params.page, page_size: 20, pages: 3,
    }))
    const wrapper = await mountContentCenter()
    const requestsBefore = api.listNews.mock.calls.length

    await wrapper.get('[data-test="next-page"]').trigger('click')
    await flushPromises()

    expect(api.listNews.mock.calls).toHaveLength(requestsBefore + 1)
    expect(api.listNews.mock.calls.at(-1)?.[0]).toMatchObject({ page: 2, page_size: 20 })
    expect(wrapper.text()).toContain('第二页新闻')
    wrapper.unmount()
  })

  it('loads, edits, saves, and reopens a 10,000-character rich body without truncation', async () => {
    const text = '长正文验证内容。'.repeat(1_250)
    expect(text.length).toBe(10_000)
    const originalHtml = `<p>${text}</p>`
    persistedNews.bodyHtml = originalHtml

    const wrapper = await mountContentCenter()
    const editor = await openNewsEditor(wrapper)
    expect((editor.element as HTMLTextAreaElement).value).toBe(originalHtml)

    const editedHtml = `<p>${text}末尾追加</p>`
    await editor.setValue(editedHtml)
    await wrapper.get('button').exists()
    const saveButton = wrapper.findAll('button').find((button) => button.text().trim() === '保存新闻')
    expect(saveButton).toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(api.updateNews).toHaveBeenCalledTimes(1)
    expect(api.updateNews.mock.calls[0][0]).toBe(1381)
    expect(api.updateNews.mock.calls[0][1].bodyHtml).toBe(editedHtml)
    expect(persistedNews.bodyHtml).toBe(editedHtml)
    expect((wrapper.get('[data-test="news-rich-editor"]').element as HTMLTextAreaElement).value).toBe(editedHtml)
    expect(api.getNews).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('converts historical Markdown into editable HTML and saves the complete rendered text', async () => {
    persistedNews.bodyHtml = ''
    const markdown = `## 历史 Markdown 标题\n\n${'历史正文段落。'.repeat(700)}\n\n**加粗结尾**`
    api.getNews.mockImplementation(async () => response(
      persistedNews.bodyHtml
        ? { ...persistedNews }
        : { ...persistedNews, bodyMarkdown: markdown, bodyHtml: '' },
    ))

    const wrapper = await mountContentCenter()
    const editor = await openNewsEditor(wrapper)
    const initialHtml = (editor.element as HTMLTextAreaElement).value
    expect(initialHtml).toContain('<h2>历史 Markdown 标题</h2>')
    expect(initialHtml).toContain('<strong>加粗结尾</strong>')
    expect(initialHtml).toContain('历史正文段落。')

    const amendedHtml = `${initialHtml}<p>保存后追加段落</p>`
    await editor.setValue(amendedHtml)
    const saveButton = wrapper.findAll('button').find((button) => button.text().trim() === '保存新闻')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(api.updateNews.mock.calls[0][1].bodyHtml).toBe(amendedHtml)
    expect((wrapper.get('[data-test="news-rich-editor"]').element as HTMLTextAreaElement).value).toBe(amendedHtml)
    wrapper.unmount()
  })
  it('文章列表按语言展示译文状态，弹窗可按勾选语言批量创建翻译任务', async () => {
    const article = { id: 501, title: '多语言验收文章', slug: 'multi-lang-501', locale: 'zh-CN', status: 'published', version: 3 }
    api.listArticles = vi.fn(() => response({ items: [article], total: 1 }))
    api.listTranslations = vi.fn((id: number) => response({
      items: id === 501
        ? [
            { id: 11, articleId: 501, locale: 'en-US', status: 'published' },
            { id: 12, articleId: 501, locale: 'ru-RU', status: 'failed', errorCode: 'llm_timeout' },
          ]
        : [],
    }))
    api.getAIConfig = vi.fn(() => response({ defaultModel: 'gpt-4o-mini', providerKeyRef: 'default' }))
    api.createTranslationTask = vi.fn(() => response({ id: 'task-1' }))

    const wrapper = await mountContentCenter()
    const articleTab = wrapper.findAll('button').find((b) => b.text().trim() === '深度解析')
    expect(articleTab, 'article tab should exist').toBeTruthy()
    await articleTab!.trigger('click')
    await flushPromises()

    const listText = wrapper.text()
    expect(listText).toContain('EN·已发布')
    expect(listText).toContain('RU·失败')

    const editButton = wrapper.findAll('button').find((b) => b.text().trim() === '编辑')
    await editButton!.trigger('click')
    await flushPromises()

    const localeBoxes = wrapper.findAll('input[data-locale]')
    expect(localeBoxes.length).toBe(4)
    const locales = localeBoxes.map((box) => box.attributes('data-locale'))
    expect(locales).toEqual(['en-US', 'ru-RU', 'fr-FR', 'de-DE'])
    expect(localeBoxes[0].element as HTMLInputElement).toHaveProperty('checked', true)
    expect(localeBoxes[2].element as HTMLInputElement).toHaveProperty('checked', false)

    await localeBoxes[2].setValue(true)
    await localeBoxes[3].setValue(true)
    await flushPromises()

    const generateButton = wrapper.findAll('button').find((b) => b.text().includes('生成所选语言译文'))
    expect(generateButton, 'generate selected locales button should exist').toBeTruthy()
    expect(generateButton!.text()).toContain('4')
    await generateButton!.trigger('click')
    await flushPromises()

    expect(api.createTranslationTask).toHaveBeenCalledTimes(1)
    const [articleId, body] = api.createTranslationTask.mock.calls[0]
    expect(articleId).toBe(501)
    expect(body.locales).toEqual(['en-US', 'ru-RU', 'fr-FR', 'de-DE'])
    expect(body.model).toBe('gpt-4o-mini')
    wrapper.unmount()
  })

  it('自动化设置用语言勾选组保存目标语言数组', async () => {
    api.getAutomation = vi.fn(() => response({ targetLocales: ['en-US'], dedupeWindowHours: 24 }))
    api.updateAutomation = vi.fn(() => response({}))

    const wrapper = await mountContentCenter()
    const automationTab = wrapper.findAll('button').find((b) => b.text().trim() === '自动化设置')
    expect(automationTab, 'automation tab should exist').toBeTruthy()
    await automationTab!.trigger('click')
    await flushPromises()

    const group = wrapper.get('.locale-checkbox-group')
    const boxes = group.findAll('input[type="checkbox"]')
    expect(boxes.length).toBe(4)
    expect((boxes[0].element as HTMLInputElement).checked).toBe(true)
    expect((boxes[2].element as HTMLInputElement).checked).toBe(false)

    await boxes[2].setValue(true)
    await flushPromises()

    const saveButton = wrapper.findAll('button').find((b) => b.text().trim() === '保存自动化设置')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(api.updateAutomation).toHaveBeenCalledTimes(1)
    const body = api.updateAutomation.mock.calls[0][0]
    expect(body.targetLocales).toEqual(['en-US', 'fr-FR'])
    expect(body.targetLocalesText).toBe('en-US,fr-FR')
    wrapper.unmount()
  })

  it('新闻源创建提交真实抓取间隔与启用状态', async () => {
    api.createSource = vi.fn(() => response({ id: 9 }))

    const wrapper = await mountContentCenter()
    const sourcesTab = wrapper.findAll('button').find((b) => b.text().trim() === '新闻源')
    expect(sourcesTab, 'sources tab should exist').toBeTruthy()
    await sourcesTab!.trigger('click')
    await flushPromises()

    const nameInput = wrapper.findAll('input').find((i) => i.attributes('placeholder') === '来源名称（必填）')
    const urlInput = wrapper.findAll('input').find((i) => i.attributes('placeholder') === '公开 RSS/Atom/JSON URL（必填）')
    expect(nameInput, 'name input should exist').toBeTruthy()
    expect(urlInput, 'url input should exist').toBeTruthy()
    await nameInput!.setValue('验收科技源')
    await urlInput!.setValue('https://example.com/feed.xml')

    const intervalInput = wrapper.get('input.interval-input')
    await intervalInput.setValue('45')
    const enabledCheckbox = wrapper.get('.inline-check input[type="checkbox"]')
    expect((enabledCheckbox.element as HTMLInputElement).checked).toBe(true)
    await enabledCheckbox.setValue(false)

    const createButton = wrapper.findAll('button').find((b) => b.text().trim() === '新增新闻源')
    expect(createButton, 'create source button should exist').toBeTruthy()
    await createButton!.trigger('click')
    await flushPromises()

    expect(api.createSource).toHaveBeenCalledTimes(1)
    const payload = api.createSource.mock.calls[0][0]
    expect(payload.name).toBe('验收科技源')
    expect(payload.url).toBe('https://example.com/feed.xml')
    expect(payload.intervalMinutes).toBe(45)
    expect(payload.enabled).toBe(false)
    wrapper.unmount()
  })
  it('新闻编辑弹窗展示自动翻译任务状态与失败原因', async () => {
    api.listNewsTranslationTasks = vi.fn((id: number) => response({
      items: id === 1381
        ? [
            { id: 'nt-1', targetLocale: 'en-US', status: 'succeeded' },
            { id: 'nt-2', targetLocale: 'ru-RU', status: 'failed', errorCode: 'llm_rate_limited' },
          ]
        : [],
    }))

    const wrapper = await mountContentCenter()
    await openNewsEditor(wrapper)

    const text = wrapper.text()
    expect(text).toContain('译文任务')
    expect(text).toContain('EN已完成')
    expect(text).toContain('RU失败')
    expect(text).toContain('llm_rate_limited')
    expect(api.listNewsTranslationTasks).toHaveBeenCalledWith(1381)
    wrapper.unmount()
  })

  it('BI 分析：渲染 10 张图表、4 张今日 KPI，并以中文展示位置/来源代码', async () => {
    const wrapper = await mountContentCenter()
    const biTab = wrapper.findAll('button').find((b) => b.text().trim() === 'BI 分析')
    expect(biTab, 'BI tab should exist').toBeTruthy()
    await biTab!.trigger('click')
    await flushPromises()

    // 10 个维度图表卡片，标题全部为本地化文案
    const cards = wrapper.findAll('.chart-card')
    expect(cards).toHaveLength(10)
    const titles = cards.map((c) => c.find('h3').text())
    expect(titles).toEqual([
      '访客、浏览与阅读趋势',
      '文章阅读 TOP10',
      '新闻阅读 TOP10',
      '位置点击率',
      '按分类阅读分布',
      '按语言阅读分布',
      '按新闻源阅读分布',
      '来源渠道',
      '平均阅读时长',
      '发布趋势',
    ])

    // 4 张 KPI 卡
    expect(wrapper.findAll('.metrics .metric-card')).toHaveLength(4)
    const text = wrapper.text()
    expect(text).toContain('今日 · 访客数（UV）')
    expect(text).toContain('12')

    // 代码值本地化：不出现 home_hero / search 原始代码
    expect(text).toContain('首页精选位')
    expect(text).toContain('搜索引擎')
    expect(text).not.toContain('home_hero')
    // 图例/数据集标签同样本地化（R11-04）
    expect(text).toContain('访客数（UV）')
    expect(text).toContain('新闻源')

    // 四个分布维度按 dim 请求
    const dims = Array.from(new Set(api.analyticsDistribution.mock.calls.map((call) => call[0]))).sort()
    expect(dims).toEqual(['category', 'locale', 'referrer', 'source'])
    wrapper.unmount()
  })

  it('BI 分析：切换到近 7 天会按 7 天窗口重新拉取数据并回传语言/位置筛选', async () => {
    const wrapper = await mountContentCenter()
    const biTab = wrapper.findAll('button').find((b) => b.text().trim() === 'BI 分析')
    await biTab!.trigger('click')
    await flushPromises()

    const trendCallsBefore = api.analyticsTrends.mock.calls.length
    const rangeSelect = wrapper.findAllComponents(Select)[0]
    expect(rangeSelect, 'range select should exist').toBeTruthy()
    rangeSelect.vm.$emit('update:modelValue', '7')
    await flushPromises()

    expect(api.analyticsTrends.mock.calls.length).toBe(trendCallsBefore + 1)
    const params = api.analyticsTrends.mock.calls.at(-1)?.[0] as any
    const spanDays = (Date.parse(params.to) - Date.parse(params.from)) / 86400000
    expect(spanDays).toBe(6)

    // 语言与位置筛选联动
    const selects = wrapper.findAllComponents(Select)
    const localeSelect = selects.find((s) => s.attributes('data-test') === 'bi-locale')
    expect(localeSelect).toBeTruthy()
    localeSelect!.vm.$emit('update:modelValue', 'zh')
    await flushPromises()
    expect((api.analyticsTrends.mock.calls.at(-1)?.[0] as any).locale).toBe('zh')
    wrapper.unmount()
  })
})
