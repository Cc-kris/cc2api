import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelSquarePriceCell from '../ModelSquarePriceCell.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'modelSquare.free': '免费',
      'modelSquare.originalPrice': '原价',
      'modelSquare.multiplierPrice': '倍率价',
      'modelSquare.units.per_1m_tokens': '1M Token',
    }[key] ?? key),
  }),
}))

describe('ModelSquarePriceCell', () => {
  it('renders multiplier price as primary and original price as secondary', () => {
    const wrapper = mount(ModelSquarePriceCell, {
      props: {
        price: { original: '2.50000000', multiplier_price: '2.75000000', unit: 'per_1m_tokens' },
      },
    })

    expect(wrapper.text()).toContain('$2.75 / 1M Token')
    expect(wrapper.text()).toContain('原价 $2.5 / 1M Token')
    expect(wrapper.attributes('title')).toContain('2.75000000')
  })

  it('preserves low eight-decimal prices without converting through JavaScript Number', () => {
    const wrapper = mount(ModelSquarePriceCell, {
      props: {
        price: { original: '0.00000001', multiplier_price: '0.00000008', unit: 'per_1m_tokens' },
      },
    })

    expect(wrapper.text()).toContain('$0.00000008 / 1M Token')
    expect(wrapper.text()).toContain('原价 $0.00000001 / 1M Token')
    expect(wrapper.text()).not.toContain('免费')
  })

  it('shows an explicit zero price as free instead of hiding it', () => {
    const wrapper = mount(ModelSquarePriceCell, {
      props: {
        price: { original: '0.00000000', multiplier_price: '0.00000000', unit: 'per_1m_tokens' },
      },
    })

    expect(wrapper.text()).toContain('免费')
    expect(wrapper.text()).not.toContain('—')
  })
})
