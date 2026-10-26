import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
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

    const loadingWrapper = mount(Select, {
      props: { modelValue: null, options, loading: true }
    })
    const loadingTrigger = loadingWrapper.get('button')
    expect(loadingTrigger.attributes('disabled')).toBeDefined()
    expect(loadingTrigger.attributes('aria-busy')).toBe('true')
    expect(loadingTrigger.get('[role="status"]').attributes('aria-label')).toBe('Loading')
    loadingWrapper.unmount()
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
