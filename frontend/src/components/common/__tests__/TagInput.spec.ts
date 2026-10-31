import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import TagInput from '../TagInput.vue'

const setup = (suggestions: string[] = []) => mount(TagInput, { props: { modelValue: ['AI'], suggestions, 'onUpdate:modelValue': (v: string[]) => { void v } } })
const emitted = (w: ReturnType<typeof setup>) => w.emitted('update:modelValue')?.at(-1)?.[0]

describe('TagInput', () => {
  it('does not commit IME composition, then commits compositionend + Enter', async () => {
    const w = setup(); const input = w.get('input')
    await input.trigger('compositionstart'); await input.setValue('中文'); await input.trigger('keydown', { key: 'Enter', isComposing: true, keyCode: 229 })
    expect(w.emitted('update:modelValue')).toBeUndefined()
    await input.trigger('compositionend'); await input.trigger('keydown', { key: 'Enter' })
    expect(emitted(w)).toEqual(['AI', '中文'])
  })
  it('commits Tab and blur, normalizes duplicates, backspace removes last', async () => {
    const w = setup(); const input = w.get('input')
    await input.setValue('  ai  '); await input.trigger('keydown', { key: 'Tab' }); expect(emitted(w)).toEqual(['AI'])
    await input.setValue(' Gateway '); vi.useFakeTimers(); await input.trigger('blur'); vi.runAllTimers(); vi.useRealTimers(); await nextTick(); expect(emitted(w)).toEqual(['AI', 'Gateway'])
    await input.trigger('keydown', { key: 'Backspace' }); expect(emitted(w)).toEqual([])
  })
  it('shows existing suggestions on focus and supports a single string model', async () => {
    const w = mount(TagInput, { props: { modelValue: '', suggestions: ['Gateway', 'LLM'] } })
    await w.get('input').trigger('focus')
    expect(w.findAll('[role=option]').map(x => x.text())).toEqual(['Gateway', 'LLM'])
    await w.get('input').setValue('new-tag')
    await w.get('input').trigger('keydown', { key: 'Enter' })
    expect(w.emitted('update:modelValue')?.at(-1)?.[0]).toBe('new-tag')
  })
  it('navigates suggestions, escapes and splits pasted delimiters', async () => {
    const w = setup(['Gateway', 'LLM']); const input = w.get('input')
    await input.setValue('g'); await input.trigger('keydown', { key: 'ArrowDown' }); await input.trigger('keydown', { key: 'Enter' }); expect(emitted(w)).toEqual(['AI', 'Gateway'])
    await input.trigger('keydown', { key: 'Escape' }); expect(w.get('[role=combobox]').attributes('aria-expanded')).toBe('false')
    await input.trigger('paste', { clipboardData: { getData: () => 'one, two\nthree；four' } }); expect(emitted(w)).toEqual(['AI', 'one', 'two', 'three', 'four'])
  })
})
