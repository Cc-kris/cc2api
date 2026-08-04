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

  it('renders each tier inside its corresponding desktop and mobile price column', () => {
    const tiered: ModelSquareModel = {
      ...item,
      name: 'minimax-m3',
      prices: {
        input: null,
        output: null,
        cache_read: null,
        cache_write_5m: null,
        cache_write_1h: null,
      },
      tiers: [
        {
          min_tokens: 0,
          max_tokens: 512000,
          sort_order: 0,
          input: { ...inputPrice, original: '2.1', multiplier_price: '1.68' },
          output: { ...inputPrice, original: '8.4', multiplier_price: '6.72' },
          cache_read: { ...inputPrice, original: '0.42', multiplier_price: '0.336', unit: 'per_1m_cache_tokens' },
          cache_write: { ...inputPrice, original: '2.6', multiplier_price: '2.08', unit: 'per_1m_cache_tokens' },
          per_request: null,
        },
        {
          min_tokens: 512000,
          max_tokens: null,
          sort_order: 1,
          input: { ...inputPrice, original: '4.2', multiplier_price: '3.36' },
          output: { ...inputPrice, original: '16.8', multiplier_price: '13.44' },
          cache_read: { ...inputPrice, original: '0.84', multiplier_price: '0.672', unit: 'per_1m_cache_tokens' },
          cache_write: { ...inputPrice, original: '2.625', multiplier_price: '2.1', unit: 'per_1m_cache_tokens' },
          per_request: null,
        },
      ],
    }
    const wrapper = mount(ModelSquareTable, {
      props: { items: [tiered], loading: false, loadingMore: false, hasMore: false },
      global: { stubs: { Icon: true } },
    })

    const desktop = wrapper.get('[data-testid="model-square-desktop-list"]')
    expect(desktop.find('[data-testid="model-square-desktop-tier-cell"]').exists()).toBe(false)
    expect(desktop.text()).not.toContain('modelSquare.viewTiers')

    const desktopInput = desktop.get('[data-testid="model-square-desktop-input-cell"]')
    expect(desktopInput.findAll('[data-testid="model-square-tier-price"]')).toHaveLength(2)
    expect(desktopInput.text()).toContain('0 ≤ modelSquare.tokens < 512,000')
    expect(desktopInput.text()).toContain('modelSquare.tokens ≥ 512,000')
    expect(desktopInput.text()).toContain('$1.68')
    expect(desktopInput.text()).toContain('$3.36')
    expect(desktopInput.text()).not.toContain('$6.72')

    const desktopOutput = desktop.get('[data-testid="model-square-desktop-output-cell"]')
    expect(desktopOutput.findAll('[data-testid="model-square-tier-price"]')).toHaveLength(2)
    expect(desktopOutput.text()).toContain('$6.72')
    expect(desktopOutput.text()).toContain('$13.44')

    const desktopCacheRead = desktop.get('[data-testid="model-square-desktop-cache_read-cell"]')
    expect(desktopCacheRead.findAll('[data-testid="model-square-tier-price"]')).toHaveLength(2)
    expect(desktopCacheRead.text()).toContain('$0.336')
    expect(desktopCacheRead.text()).toContain('$0.672')

    const desktopCacheWrite = desktop.get('[data-testid="model-square-desktop-cache_write-cell"]')
    expect(desktopCacheWrite.findAll('[data-testid="model-square-tier-price"]')).toHaveLength(2)
    expect(desktopCacheWrite.text()).toContain('$2.08')
    expect(desktopCacheWrite.text()).toContain('$2.1')

    const mobile = wrapper.get('[data-testid="model-square-mobile-list"]')
    expect(mobile.find('[data-testid="model-square-mobile-tier-list"]').exists()).toBe(false)
    expect(mobile.text()).not.toContain('modelSquare.columns.tiers')
    expect(mobile.get('[data-testid="model-square-mobile-input-prices"]').findAll('[data-testid="model-square-tier-price"]')).toHaveLength(2)
    expect(mobile.get('[data-testid="model-square-mobile-output-prices"]').findAll('[data-testid="model-square-tier-price"]')).toHaveLength(2)
    expect(mobile.get('[data-testid="model-square-mobile-cache_read-prices"]').findAll('[data-testid="model-square-tier-price"]')).toHaveLength(2)
    expect(mobile.get('[data-testid="model-square-mobile-cache_write-prices"]').findAll('[data-testid="model-square-tier-price"]')).toHaveLength(2)
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
