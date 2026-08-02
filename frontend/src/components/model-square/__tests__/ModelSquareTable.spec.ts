import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelSquareTable from '../ModelSquareTable.vue'
import type { ModelSquareModel } from '@/api/modelSquare'

vi.mock('@tanstack/vue-virtual', async () => {
  const { computed } = await import('vue')
  return {
    useVirtualizer: (options: any) => computed(() => ({
      getVirtualItems: () => Array.from({ length: options.value.count }, (_, index) => ({ index, start: index * 84 })),
      getTotalSize: () => options.value.count * 84,
      measureElement: () => undefined,
    })),
  }
})

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: { count?: number }) => params?.count != null ? `${key}:${params.count}` : key }),
}))

const inputPrice = { original: '2.50000000', multiplier_price: '2.75000000', unit: 'per_1m_tokens' as const }
const item: ModelSquareModel = {
  name: 'gpt-5.5',
  billing_mode: 'token',
  prices: {
    input: inputPrice,
    output: null,
    cache_read: null,
    cache_write_5m: null,
    cache_write_1h: null,
  },
  fast_prices: null,
  tiers: [],
}

describe('ModelSquareTable', () => {
  it('renders stable desktop columns and keeps the model-name column sticky', () => {
    const wrapper = mount(ModelSquareTable, {
      props: { items: [item], loading: false, loadingMore: false, hasMore: false },
      global: { stubs: { Icon: true, ModelSquarePriceCell: true } },
    })

    expect(wrapper.text()).toContain('modelSquare.columns.input')
    expect(wrapper.text()).toContain('modelSquare.columns.output')
    expect(wrapper.text()).toContain('modelSquare.columns.cacheRead')
    expect(wrapper.get('[role="columnheader"]').classes()).toContain('sticky')
  })

  it('renders a mobile vertical disclosure list without horizontal scrolling', () => {
    const wrapper = mount(ModelSquareTable, {
      props: { items: [item], loading: false, loadingMore: false, hasMore: false },
      global: { stubs: { Icon: true, ModelSquarePriceCell: true } },
    })

    const mobile = wrapper.get('[data-testid="model-square-mobile-list"]')
    expect(mobile.classes()).toContain('overflow-x-hidden')
    expect(mobile.find('details').exists()).toBe(true)
    expect(mobile.text()).toContain('gpt-5.5')
    expect(mobile.text()).toContain('modelSquare.columns.input')
  })

  it('renders Fast details and keeps image or per-request prices in the billing column', () => {
    const enriched: ModelSquareModel = {
      ...item,
      billing_mode: 'image',
      prices: { ...item.prices, image_output: inputPrice },
      fast_prices: { input: inputPrice, output: null, cache_read: null, cache_write_5m: null, cache_write_1h: null },
      tiers: [],
    }
    const wrapper = mount(ModelSquareTable, {
      props: { items: [enriched], loading: false, loadingMore: false, hasMore: false },
      global: { stubs: { Icon: true, ModelSquarePriceCell: true } },
    })

    expect(wrapper.text()).toContain('modelSquare.viewFast')
    expect(wrapper.text()).toContain('modelSquare.columns.imageOutput')
    expect(wrapper.text()).not.toContain('modelSquare.viewTiers')
  })

  it('requests the next page when scrolling within 300px of the bottom', async () => {
    const wrapper = mount(ModelSquareTable, {
      props: { items: [item], loading: false, loadingMore: false, hasMore: true },
      global: { stubs: { Icon: true, ModelSquarePriceCell: true } },
    })
    const scroller = wrapper.get('[data-testid="model-square-desktop-list"]')
    Object.defineProperties(scroller.element, {
      scrollHeight: { value: 1000 },
      scrollTop: { value: 550, writable: true },
      clientHeight: { value: 200 },
    })

    await scroller.trigger('scroll')

    expect(wrapper.emitted('loadMore')).toHaveLength(1)
  })
})
