import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

// 精选开关闪回回归：后台列表刷新（查询/切换视图/保存文章触发的 loadArticles）在精选切换之前发出、
// 在切换之后才返回时，旧响应里的 tags 仍带「精选」，会覆盖乐观更新的本地状态，让开关自己翻回去。
const { api } = vi.hoisted(() => ({
  api: {
    listNews: vi.fn(),
    listArticles: vi.fn(),
    mutateTaxonomy: vi.fn(),
    listTranslations: vi.fn(),
    showToast: vi.fn(),
  },
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
    props: { data: { type: Object, default: () => ({}) }, options: { type: Object, default: () => ({}) } },
    setup() {
      return () => createElement('div', { class: 'chart-stub' })
    },
  })
  return { Line: ChartStub, Bar: ChartStub, Doughnut: ChartStub }
})

import { createI18n } from 'vue-i18n'
import contentV04Messages from '@/i18n/locales/contentV04'
import ContentCenterView from '../ContentCenterView.vue'

/** 已发布且当前被标为精选的文章（tags 里带「精选」，与后端 isArticleFeatured 判定一致） */
function featuredRow() {
  return {
    id: 601,
    title: '精选闪回验收文章',
    slug: 'featured-race-601',
    locale: 'zh-CN',
    status: 'published',
    tags: ['精选'],
    readCount: 12,
    publishedAt: '2026-10-01T00:00:00Z',
  }
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

/** 挂载并切到「深度解析」页签，等首屏文章列表渲染完成 */
async function mountArticleList() {
  const wrapper = mount(ContentCenterView, {
    global: { plugins: [createTestI18n()] },
  })
  await flushPromises()
  const articleTab = wrapper.findAll('button').find((b) => b.text().trim() === '深度解析')
  expect(articleTab, '深度解析页签应存在').toBeTruthy()
  await articleTab!.trigger('click')
  await flushPromises()
  return wrapper
}

function featuredSwitch(wrapper: ReturnType<typeof mount>) {
  const el = wrapper.get('.featured-toggle')
  return el
}

/** 点工具栏「查询」触发一次后台列表刷新 */
async function clickQuery(wrapper: ReturnType<typeof mount>) {
  const queryButton = wrapper.findAll('button').find((b) => b.text().trim() === '查询')
  expect(queryButton, '文章工具栏应有「查询」按钮').toBeTruthy()
  await queryButton!.trigger('click')
  await flushPromises()
}

beforeEach(() => {
  api.listNews.mockReset().mockImplementation(async () => ({ data: { items: [], total: 0 } }))
  api.listTranslations.mockReset().mockImplementation(async () => ({ data: { items: [] } }))
  api.showToast.mockReset()
  api.listArticles.mockReset()
  api.mutateTaxonomy.mockReset()
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
    configurable: true,
    value: vi.fn(),
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('ContentCenterView 精选开关并发回归（BUG：旧列表响应把开关翻回去）', () => {
  it('精选切换在途时返回的旧列表响应不得把开关翻回「开」', async () => {
    // 首屏/切页签的列表请求都立即返回精选文章；进入「挂起」模式后的请求由用例手动放行，
    // 用切换精选之前读到的旧数据返回，复现「旧响应晚到」的时序
    let deferList = false
    let releaseStaleList: (() => void) | null = null
    api.listArticles.mockImplementation(() => {
      if (!deferList) return Promise.resolve({ data: { items: [featuredRow()], total: 1 } })
      return new Promise((resolve) => {
        releaseStaleList = () => resolve({ data: { items: [featuredRow()], total: 1 } })
      })
    })
    // 精选写入也挂起：模拟「写请求还没返回，后台列表响应先到」的真实时序
    let releaseMutation: (() => void) | null = null
    api.mutateTaxonomy.mockImplementation(() => new Promise<void>((resolve) => {
      releaseMutation = () => resolve()
    }))

    const wrapper = await mountArticleList()
    expect(featuredSwitch(wrapper).classes()).toContain('on')

    // 1) 后台刷新发出（响应被挂起）
    deferList = true
    const listCallsBeforeQuery = api.listArticles.mock.calls.length
    await clickQuery(wrapper)
    expect(api.listArticles.mock.calls.length).toBe(listCallsBeforeQuery + 1)
    expect(releaseStaleList, '后台列表请求应处于在途状态').toBeTruthy()

    // 2) 用户取消精选：本地立即变「关」
    await featuredSwitch(wrapper).trigger('click')
    await flushPromises()
    expect(api.mutateTaxonomy).toHaveBeenCalledTimes(1)
    expect(api.mutateTaxonomy.mock.calls[0][0]).toMatchObject({
      scope: 'featured_article',
      action: 'unset',
      name: '601',
    })
    expect(featuredSwitch(wrapper).classes()).not.toContain('on')

    // 3) 旧列表响应（仍带「精选」）此时到达：开关必须保持「关」，不能闪回
    releaseStaleList!()
    await flushPromises()
    expect(featuredSwitch(wrapper).classes(), '旧列表响应不得把精选开关翻回开').not.toContain('on')

    // 4) 写请求随后成功返回：状态依旧稳定
    releaseMutation!()
    await flushPromises()
    expect(featuredSwitch(wrapper).classes()).not.toContain('on')
    expect(wrapper.text()).not.toContain('精选状态更新失败')
    wrapper.unmount()
  })

  it('列表响应在精选写入成功之后才返回（读取早于提交）同样不得翻回', async () => {
    let deferList = false
    let releaseStaleList: (() => void) | null = null
    api.listArticles.mockImplementation(() => {
      if (!deferList) return Promise.resolve({ data: { items: [featuredRow()], total: 1 } })
      return new Promise((resolve) => {
        releaseStaleList = () => resolve({ data: { items: [featuredRow()], total: 1 } })
      })
    })
    api.mutateTaxonomy.mockImplementation(async () => ({ data: { ok: true } }))

    const wrapper = await mountArticleList()
    expect(featuredSwitch(wrapper).classes()).toContain('on')

    deferList = true
    await clickQuery(wrapper) // 列表请求在读旧数据，响应被挂起
    expect(releaseStaleList, '后台列表请求应处于在途状态').toBeTruthy()
    await featuredSwitch(wrapper).trigger('click')
    await flushPromises()
    expect(api.mutateTaxonomy).toHaveBeenCalledTimes(1)
    expect(featuredSwitch(wrapper).classes()).not.toContain('on')

    releaseStaleList!()
    await flushPromises()
    expect(featuredSwitch(wrapper).classes(), '写入已完成后到达的旧响应同样不得翻回').not.toContain('on')
    wrapper.unmount()
  })

  it('精选写入失败仍回滚到原状态并提示错误', async () => {
    api.listArticles.mockImplementation(async () => ({ data: { items: [featuredRow()], total: 1 } }))
    api.mutateTaxonomy.mockImplementation(async () => {
      throw { message: 'boom', code: 'internal_error' }
    })

    const wrapper = await mountArticleList()
    expect(featuredSwitch(wrapper).classes()).toContain('on')

    await featuredSwitch(wrapper).trigger('click')
    await flushPromises()

    expect(featuredSwitch(wrapper).classes(), '失败后应回滚为原状态（精选开）').toContain('on')
    expect(wrapper.text()).toContain('精选状态更新失败')
    wrapper.unmount()
  })

  it('非并发场景下切换不闪回、也不额外触发列表刷新', async () => {
    api.listArticles.mockImplementation(async () => ({ data: { items: [featuredRow()], total: 1 } }))
    api.mutateTaxonomy.mockImplementation(async () => ({ data: { ok: true } }))

    const wrapper = await mountArticleList()
    const listCallsAfterMount = api.listArticles.mock.calls.length

    await featuredSwitch(wrapper).trigger('click')
    await flushPromises()
    expect(featuredSwitch(wrapper).classes()).not.toContain('on')
    expect(api.listArticles.mock.calls.length).toBe(listCallsAfterMount)

    await featuredSwitch(wrapper).trigger('click')
    await flushPromises()
    expect(featuredSwitch(wrapper).classes()).toContain('on')
    wrapper.unmount()
  })
})
