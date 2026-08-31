import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageLatencyHealth from '../UsageLatencyHealth.vue'

describe('UsageLatencyHealth', () => {
  it('renders formatted latency and good health for fast requests', () => {
    const wrapper = mount(UsageLatencyHealth, {
      props: { firstTokenMs: 1200, durationMs: 2400 },
    })

    expect(wrapper.text()).toContain('1.2s')
    expect(wrapper.text()).toContain('2.4s')
    expect(wrapper.find('.bg-emerald-500').exists()).toBe(true)
    expect(wrapper.find('.h-full').attributes('style')).toContain('width: 8%')
    expect(wrapper.attributes('title')).toBe('TTFT 1.2s · Total 2.4s')
  })

  it('marks failed and missing requests distinctly', () => {
    const failed = mount(UsageLatencyHealth, {
      props: { firstTokenMs: null, durationMs: 6000, isError: true },
    })
    expect(failed.text()).toContain('-')
    expect(failed.find('.bg-red-500').exists()).toBe(true)

    const missing = mount(UsageLatencyHealth, {
      props: { firstTokenMs: undefined, durationMs: undefined },
    })
    expect(missing.text()).toBe('-/-')
    expect(missing.find('.bg-gray-300').exists()).toBe(true)
    expect(missing.find('.h-full').attributes('style')).toContain('width: 0%')
  })
})
