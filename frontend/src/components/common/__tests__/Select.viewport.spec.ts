import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import Select from '../Select.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('Select viewport positioning', () => {
  let wrapper: VueWrapper | undefined
  let triggerLeft: number
  let triggerWidth: number
  let panelWidth: number
  let onResize: ResizeObserverCallback | undefined
  const disconnect = vi.fn()

  const settle = async () => {
    await nextTick()
    await nextTick()
    await nextTick()
  }
  const panel = () => document.body.querySelector<HTMLElement>('[role="listbox"]')!
  const open = async () => {
    wrapper = mount(Select, {
      attachTo: document.body,
      props: {
        modelValue: '',
        options: [
          { value: '', label: '全部隐私模式' },
          { value: 'off', label: 'Training disabled — long privacy option' }
        ]
      },
      global: { stubs: { Icon: true } }
    })
    await wrapper.get('button').trigger('click')
    await settle()
  }

  beforeEach(() => {
    triggerLeft = 188
    triggerWidth = 160
    panelWidth = 262
    vi.stubGlobal('innerWidth', 390)
    vi.stubGlobal('innerHeight', 844)
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: ResizeObserverCallback) { onResize = callback }
      observe() {}
      disconnect = disconnect
    })
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      const dropdown = this.getAttribute('role') === 'listbox'
      return {
        x: dropdown ? 0 : triggerLeft, y: 100,
        left: dropdown ? 0 : triggerLeft, top: 100,
        width: dropdown ? Math.min(panelWidth, window.innerWidth - 16) : triggerWidth,
        height: 40, right: triggerLeft + triggerWidth, bottom: 140,
        toJSON: () => ({})
      } as DOMRect
    })
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    document.body.innerHTML = ''
    onResize = undefined
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    disconnect.mockClear()
  })

  it('shifts a right-edge dropdown inside the viewport without reducing its readable width', async () => {
    await open()
    expect(panel().style.left).toBe('120px')
    expect(panel().style.maxWidth).toBe('374px')
    expect(panel().textContent).toContain('Training disabled — long privacy option')
  })

  it('caps both minimum and maximum width when even the trigger is wider than the viewport', async () => {
    triggerWidth = 600
    panelWidth = 600
    await open()
    expect(panel().style.left).toBe('8px')
    expect(panel().style.minWidth).toBe('374px')
    expect(panel().style.maxWidth).toBe('374px')
  })

  it('keeps desktop alignment and recalculates after viewport and label size changes', async () => {
    vi.stubGlobal('innerWidth', 1280)
    await open()
    expect(panel().style.left).toBe('188px')
    vi.stubGlobal('innerWidth', 390)
    window.dispatchEvent(new Event('resize'))
    await settle()
    expect(panel().style.left).toBe('120px')
    panelWidth = 328
    onResize?.([], {} as ResizeObserver)
    await settle()
    expect(panel().style.left).toBe('54px')
  })

  it('preserves keyboard selection and releases its observer when closed', async () => {
    await open()
    expect(document.activeElement).toBe(panel())
    panel().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    panel().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await settle()
    expect(wrapper!.emitted('update:modelValue')?.[0]).toEqual(['off'])
    expect(disconnect).toHaveBeenCalledOnce()
    expect(wrapper!.get('button').attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(wrapper!.get('button').element)
  })
})
