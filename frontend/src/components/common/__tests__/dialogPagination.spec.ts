import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'
import ConfirmDialog from '../ConfirmDialog.vue'
import Pagination from '../Pagination.vue'
import StatusBadge from '../StatusBadge.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, values?: Record<string, unknown>) => {
      if (key === 'pagination.pageOf') return `Page ${values?.page} of ${values?.total}`
      return key
    }
  })
}))

vi.mock('@/components/icons/Icon.vue', () => ({
  default: { props: ['name', 'size'], template: '<span aria-hidden="true" />' }
}))

vi.mock('@/components/common/Select.vue', () => ({
  default: { props: ['modelValue', 'options'], template: '<select />' }
}))

afterEach(() => {
  document.body.innerHTML = ''
  document.body.classList.remove('modal-open')
  vi.restoreAllMocks()
})

describe('BaseDialog', () => {
  it('applies the responsive modal size scale and fixed-body layout', async () => {
    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Edit', width: 'wide' },
      slots: { default: '<input aria-label="Name" />', footer: '<button>Save</button>' }
    })
    await nextTick()
    const dialog = document.body.querySelector('[role="dialog"]') as HTMLElement
    expect(dialog.style.getPropertyValue('--dialog-width')).toBe('880px')
    expect(dialog.style.getPropertyValue('--dialog-max-height')).toBe('min(90vh, 900px)')
    expect(dialog.querySelector('.modal-body')).toBeTruthy()
    wrapper.unmount()
  })

  it('does not close when the overlay is clicked if outside-click closing is disabled', async () => {
    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Edit', closeOnClickOutside: false }
    })
    await nextTick()

    document.body.querySelector<HTMLElement>('.modal-overlay')?.click()
    await nextTick()

    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })

  it('traps keyboard focus and asks before discarding unsaved changes', async () => {
    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Edit', hasUnsavedChanges: true },
      slots: { default: '<button>First</button><button>Second</button>' }
    })
    await nextTick()
    const buttons = Array.from(document.body.querySelectorAll('button')) as HTMLButtonElement[]
    buttons[buttons.length - 1].focus()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }))
    expect(document.activeElement).toBe(buttons[0])

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(document.body.querySelector('[role="alertdialog"]')).toBeTruthy()
    expect(wrapper.emitted('close')).toBeUndefined()

    const keepEditing = document.body.querySelector('.modal-discard-actions button') as HTMLButtonElement
    keepEditing.click()
    await nextTick()
    expect(document.body.querySelector('[role="alertdialog"]')).toBeNull()

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    const discard = Array.from(document.body.querySelectorAll('.modal-discard-actions button'))[1] as HTMLButtonElement
    discard.click()
    await nextTick()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})

describe('ConfirmDialog', () => {
  it('emits confirm and cancel actions', async () => {
    const wrapper = mount(ConfirmDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Confirm', message: 'Continue?' }
    })
    await document.body.querySelector<HTMLButtonElement>('.btn-primary')?.click()
    await nextTick()
    await document.body.querySelector<HTMLButtonElement>('.btn-secondary')?.click()
    await nextTick()
    expect(wrapper.emitted('confirm')).toHaveLength(1)
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    wrapper.unmount()
  })
})

describe('Pagination', () => {
  it('renders a zero range and disables navigation for empty data', async () => {
    const wrapper = mount(Pagination, {
      props: { page: 9, pageSize: 20, total: 0, showPageSizeSelector: false }
    })
    expect(wrapper.text()).toContain('0')
    expect(wrapper.text()).toContain('Page 0 of 0')
    expect(wrapper.findAll('button').filter((button) => button.attributes('disabled') !== undefined).length).toBeGreaterThanOrEqual(2)
    await wrapper.find('input').exists() && flushPromises()
    wrapper.unmount()
  })

  it('clamps an out-of-range current page and emits bounded navigation', async () => {
    const wrapper = mount(Pagination, {
      props: { page: 99, pageSize: 10, total: 25, showPageSizeSelector: false }
    })
    expect(wrapper.text()).toContain('Page 3 of 3')
    const nextButtons = wrapper.findAll('button').filter((button) => button.attributes('aria-label') === 'pagination.next')
    expect(nextButtons.every((button) => button.attributes('disabled') !== undefined)).toBe(true)
    const previousButtons = wrapper.findAll('button').filter((button) => button.attributes('aria-label') === 'pagination.previous')
    await previousButtons[previousButtons.length - 1].trigger('click')
    expect(wrapper.emitted('update:page')?.[0]).toEqual([2])
  })
})

describe('StatusBadge', () => {
  it('exposes textual status to assistive technology while retaining the label', () => {
    const wrapper = mount(StatusBadge, { props: { status: 'error', label: 'Failed to load' } })
    expect(wrapper.attributes('role')).toBe('status')
    expect(wrapper.attributes('aria-live')).toBe('polite')
    expect(wrapper.attributes('aria-label')).toBe('Failed to load')
    expect(wrapper.text()).toContain('Failed to load')
  })
})
