import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { api, editorControl } = vi.hoisted(() => ({
  api: {
    listNews: vi.fn(),
    getNews: vi.fn(),
    updateNews: vi.fn(),
    showToast: vi.fn(),
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
  const ChartStub = component({
    setup: () => () => createElement('div'),
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

async function mountContentCenter() {
  const wrapper = mount(ContentCenterView)
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
})
