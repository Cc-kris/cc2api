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
})
