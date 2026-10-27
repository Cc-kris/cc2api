import { readFileSync } from 'node:fs'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { flushPromises } from '@vue/test-utils'
import Input from '@/components/common/Input.vue'
import TextArea from '@/components/common/TextArea.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const options = [
  { value: 'one', label: 'One' },
  { value: 'two', label: 'Two' }
]

afterEach(() => {
  document.body.querySelectorAll('.select-dropdown-portal').forEach((element) => element.remove())
})

describe('form controls', () => {
  it('Input emits string values and exposes the required visual states', async () => {
    const wrapper = mount(Input, {
      props: { modelValue: null, error: 'Required', id: 'name' }
    })
    const input = wrapper.get('input')

    expect(input.element.value).toBe('')
    expect(input.classes()).toContain('h-9')
    expect(input.classes()).toContain('rounded-lg')
    expect(input.attributes('aria-invalid')).toBe('true')

    await input.setValue('Alice')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['Alice'])
  })

  it('TextArea auto-grows within the 96px to 400px bounds', async () => {
    const wrapper = mount(TextArea, { props: { modelValue: '' } })
    const textarea = wrapper.get('textarea').element as HTMLTextAreaElement

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 180 })
    await wrapper.get('textarea').trigger('input')
    expect(textarea.style.height).toBe('180px')
    expect(textarea.style.overflowY).toBe('hidden')

    Object.defineProperty(textarea, 'scrollHeight', { configurable: true, value: 520 })
    await wrapper.get('textarea').trigger('input')
    expect(textarea.style.height).toBe('400px')
    expect(textarea.style.overflowY).toBe('auto')
  })

  it('Select supports multiple selection, clearable and loading states', async () => {
    const wrapper = mount(Select, {
      props: { modelValue: [], options, multiple: true, clearable: true }
    })
    const trigger = wrapper.get('button')

    expect(trigger.attributes('aria-haspopup')).toBe('listbox')
    await trigger.trigger('click')
    await nextTick()

    const dropdown = document.body.querySelector('[role="listbox"]')
    expect(dropdown).not.toBeNull()
    expect(dropdown?.getAttribute('aria-multiselectable')).toBe('true')
    const option = dropdown?.querySelector('[role="option"]') as HTMLElement
    option.click()
    await nextTick()
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['one']])
    expect(document.body.querySelector('[role="listbox"]')).not.toBeNull()

    await wrapper.setProps({ modelValue: ['one'] })
    const clear = wrapper.get('[aria-label="Clear selection"]')
    await clear.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[]])
    wrapper.unmount()

    const loadingWrapper = mount(Select, {
      props: { modelValue: null, options, loading: true }
    })
    const loadingTrigger = loadingWrapper.get('button')
    expect(loadingTrigger.attributes('disabled')).toBeDefined()
    expect(loadingTrigger.attributes('aria-busy')).toBe('true')
    expect(loadingTrigger.get('[role="status"]').attributes('aria-label')).toBe('Loading')
    loadingWrapper.unmount()
  })

  it('non-searchable Select keeps trigger focus and exposes an accessible active option while navigating', async () => {
    const adversarialOptions = [
      { kind: 'group', label: 'Group heading' },
      { value: 'disabled', label: 'Unavailable', disabled: true },
      { value: 'first', label: 'First' },
      { value: 'last', label: 'Last' }
    ]
    const wrapper = mount(Select, {
      attachTo: document.body,
      props: {
        modelValue: null,
        options: adversarialOptions,
        searchable: false,
        ariaLabel: 'Choose a value'
      }
    })
    const trigger = wrapper.get('button')

    expect(trigger.attributes('role')).toBe('combobox')
    expect(trigger.attributes('aria-label')).toBe('Choose a value')
    expect(trigger.attributes('aria-controls')).toBeTruthy()
    trigger.element.focus()

    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await nextTick()
    const listbox = document.body.querySelector('[role="listbox"]') as HTMLElement
    expect(listbox).not.toBeNull()
    expect(listbox.getAttribute('aria-labelledby')).toBe(trigger.attributes('id'))
    expect(trigger.element).toBe(document.activeElement)

    const options = [...listbox.querySelectorAll('[role="option"]')]
    expect(options.map((option) => option.textContent?.trim())).toEqual(['Unavailable', 'First', 'Last'])
    expect(trigger.attributes('aria-activedescendant')).toBe(options[1].id)
    expect(listbox.querySelector('[role="presentation"]')?.textContent?.trim()).toBe('Group heading')

    await trigger.trigger('keydown', { key: 'ArrowDown' })
    expect(trigger.attributes('aria-activedescendant')).toBe(options[2].id)
    await trigger.trigger('keydown', { key: 'Enter' })
    await nextTick()
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['last'])
    expect(trigger.attributes('aria-expanded')).toBe('false')
    await flushPromises()
    wrapper.unmount()
  })

  it('Select uses its localized fallback name and skips disabled/group entries for its active descendant', async () => {
    const wrapper = mount(Select, {
      props: {
        modelValue: null,
        options: [
          { kind: 'group', label: 'Group heading' },
          { value: 'disabled', label: 'Unavailable', disabled: true },
          { value: 'available', label: 'Available' }
        ],
        searchable: false
      }
    })
    const trigger = wrapper.get('button')
    expect(trigger.attributes('aria-label')).toBe('common.selectOption')
    await wrapper.setProps({ ariaLabelledby: 'external-label' })
    expect(trigger.attributes('aria-labelledby')).toBe('external-label')
    expect(trigger.attributes('aria-label')).toBeUndefined()

    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await nextTick()
    const available = document.body.querySelector('[role="option"]:last-child') as HTMLElement
    expect(trigger.attributes('aria-activedescendant')).toBe(available.id)
    expect(document.body.querySelector('[role="option"][aria-disabled="true"]')).not.toBeNull()
    wrapper.unmount()
  })

  it('Escape closes an open Select without relying on focus inside the component (Safari mouse click)', async () => {
    const wrapper = mount(Select, {
      attachTo: document.body,
      props: { modelValue: 'one', options, ariaLabel: 'Pick a value' }
    })
    const trigger = wrapper.get('button')
    const outerHandler = vi.fn()

    try {
      // Safari：鼠标点击 <button> 不会让它获得焦点，焦点仍留在 body 上
      await trigger.trigger('click')
      await nextTick()
      expect(trigger.attributes('aria-expanded')).toBe('true')
      expect(document.body.querySelector('[role="listbox"]')).not.toBeNull()

      ;(document.activeElement as HTMLElement | null)?.blur()
      expect(document.activeElement).toBe(document.body)

      // 外层（如 BaseDialog）的冒泡监听不应被连带触发
      document.addEventListener('keydown', outerHandler)
      document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
      await nextTick()

      expect(trigger.attributes('aria-expanded'), '无焦点时 Escape 也能关闭').toBe('false')
      expect(wrapper.emitted('update:modelValue'), 'Escape 不改变已选值').toBeUndefined()
      expect(document.activeElement, '关闭后焦点回到 trigger').toBe(trigger.element)
      expect(outerHandler, 'Escape 被下拉消费，不外泄给外层').not.toHaveBeenCalled()

      // 已关闭时再次 Escape 不应做任何事，也不拦截页面的其它键盘监听
      outerHandler.mockClear()
      document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
      await nextTick()
      expect(outerHandler, '关闭态不再消费 Escape').toHaveBeenCalledTimes(1)
    } finally {
      document.removeEventListener('keydown', outerHandler)
      wrapper.unmount()
      await flushPromises()
    }
  })

  it('searchable Select keeps its combobox outside the labeled listbox', async () => {
    const wrapper = mount(Select, {
      attachTo: document.body,
      props: {
        modelValue: null,
        options: [{ value: 'one', label: 'One' }],
        searchable: true,
        searchPlaceholder: 'Filter options'
      }
    })
    await wrapper.get('button').trigger('click')
    await nextTick()

    const searchInput = document.body.querySelector('[role="combobox"]') as HTMLInputElement
    const listbox = document.body.querySelector('[role="listbox"]') as HTMLElement
    expect(searchInput).not.toBeNull()
    expect(listbox).not.toBeNull()
    expect(listbox.contains(searchInput)).toBe(false)
    expect(searchInput.getAttribute('aria-label')).toBe('Filter options')
    expect(searchInput.getAttribute('aria-controls')).toBe(listbox.id)
    expect(listbox.getAttribute('aria-labelledby')).toBe(searchInput.id)
    expect(searchInput.getAttribute('aria-activedescendant')).toBe(listbox.querySelector('[role="option"]')?.id)

    wrapper.unmount()
  })

  it('Select selected option uses the primary blue and high contrast foreground', () => {
    const selectSource = readFileSync('src/components/common/Select.vue', 'utf8')
    const selectedStyle = selectSource.match(/\.select-option-selected \{([\s\S]*?)\n\}/)?.[1]
    expect(selectedStyle).toContain('background-color: #2563EB')
    expect(selectedStyle).toContain('color: #FFFFFF')
    expect(selectSource).toContain(':global(.dark) .select-option-selected')
    expect(selectSource).not.toContain('#CCFF00')
  })

  it('Toggle emits only when enabled and exposes switch semantics', async () => {
    const wrapper = mount(Toggle, { props: { modelValue: false, ariaLabel: 'Enable feature' } })
    const toggle = wrapper.get('[role="switch"]')

    expect(toggle.attributes('aria-checked')).toBe('false')
    expect(toggle.attributes('aria-label')).toBe('Enable feature')
    await toggle.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[true]])

    await wrapper.setProps({ disabled: true })
    await toggle.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toHaveLength(1)
    expect(toggle.attributes('aria-disabled')).toBe('true')
  })
})
