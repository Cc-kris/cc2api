import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import GroupsView from '../GroupsView.vue'

// V03-T15：「允许生图」关闭时表单里的按张计费同步置为 false，提交的请求也是 false。
const { update, create } = vi.hoisted(() => {
  vi.stubGlobal('localStorage', { getItem: vi.fn(() => null), setItem: vi.fn(), removeItem: vi.fn() })
  return { update: vi.fn(), create: vi.fn() }
})

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 }),
      getAll: vi.fn().mockResolvedValue([]),
      getUsageSummary: vi.fn().mockResolvedValue([]),
      getCapacitySummary: vi.fn().mockResolvedValue([]),
      update,
      create,
    },
    accounts: { list: vi.fn().mockResolvedValue({ items: [], total: 0 }), getById: vi.fn() },
  },
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn(), showWarning: vi.fn() }),
}))
vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({ isCurrentStep: () => false, nextStep: vi.fn() }),
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }
})

const openaiGroup = {
  id: 7, name: '生图组', description: '', platform: 'openai', rate_multiplier: 1, is_exclusive: false,
  status: 'active', subscription_type: 'standard', allow_image_generation: true, image_count_billing: true,
  image_rate_independent: false, image_rate_multiplier: 1, supported_model_scopes: [], model_routing: {},
}

const mountView = async () => {
  const wrapper = mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot /></div>' },
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        DataTable: true, Pagination: true, ConfirmDialog: true, EmptyState: true, Select: true,
        PlatformIcon: true, Icon: true, GroupRateMultipliersModal: true, GroupRPMOverridesModal: true,
        GroupCapacityBadge: true, VueDraggable: true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

const checkboxFor = (wrapper: Awaited<ReturnType<typeof mountView>>, formId: string, labelKey: string) => {
  const label = wrapper.find(`#${formId}`).findAll('label').find((l) => l.text().trim().startsWith(labelKey))
  expect(label, labelKey).toBeTruthy()
  return label!.find('input[type="checkbox"]')
}

describe('GroupsView image count billing', () => {
  beforeEach(() => {
    update.mockReset().mockResolvedValue({})
    create.mockReset().mockResolvedValue({})
  })

  it('sends image_count_billing false after image generation is turned off in the edit form', async () => {
    const wrapper = await mountView()
    await (wrapper.vm as any).handleEdit({ ...openaiGroup })
    await flushPromises()

    const billing = wrapper.find('#edit-image-count-billing-input')
    expect((billing.element as HTMLInputElement).checked).toBe(true)

    await checkboxFor(wrapper, 'edit-group-form', 'admin.groups.imagePricing.allowImageGeneration').setValue(false)
    expect((wrapper.vm as any).editForm.image_count_billing).toBe(false)

    // 重新允许生图，按张计费保持关闭，需要管理员重新勾选。
    await checkboxFor(wrapper, 'edit-group-form', 'admin.groups.imagePricing.allowImageGeneration').setValue(true)
    // 勾选框随 v-if 重新创建，需要重新查找。
    const billingAgain = wrapper.find('#edit-image-count-billing-input')
    expect((billingAgain.element as HTMLInputElement).checked).toBe(false)
    await checkboxFor(wrapper, 'edit-group-form', 'admin.groups.imagePricing.allowImageGeneration').setValue(false)

    await wrapper.find('#edit-group-form').trigger('submit.prevent')
    await flushPromises()

    expect(update).toHaveBeenCalledTimes(1)
    expect(update.mock.calls[0][0]).toBe(7)
    expect(update.mock.calls[0][1]).toMatchObject({ allow_image_generation: false, image_count_billing: false })
  })

  it('keeps image_count_billing true when image generation stays on', async () => {
    const wrapper = await mountView()
    await (wrapper.vm as any).handleEdit({ ...openaiGroup })
    await flushPromises()

    await wrapper.find('#edit-group-form').trigger('submit.prevent')
    await flushPromises()

    expect(update.mock.calls[0][1]).toMatchObject({ allow_image_generation: true, image_count_billing: true })
  })

  it('sends false for a stored inconsistent group (billing on, image generation off)', async () => {
    const wrapper = await mountView()
    await (wrapper.vm as any).handleEdit({ ...openaiGroup, allow_image_generation: false })
    await flushPromises()

    await wrapper.find('#edit-group-form').trigger('submit.prevent')
    await flushPromises()

    expect(update.mock.calls[0][1]).toMatchObject({ allow_image_generation: false, image_count_billing: false })
  })

  it('sends false from the create form after image generation is turned off', async () => {
    const wrapper = await mountView()
    const vm = wrapper.vm as any
    vm.showCreateModal = true
    vm.createForm.name = '新组'
    vm.createForm.platform = 'openai'
    vm.createForm.allow_image_generation = true
    await flushPromises()
    vm.createForm.image_count_billing = true
    await flushPromises()

    vm.createForm.allow_image_generation = false
    await flushPromises()
    expect(vm.createForm.image_count_billing).toBe(false)

    await vm.handleCreateGroup()
    await flushPromises()
    expect(create.mock.calls[0][0]).toMatchObject({ allow_image_generation: false, image_count_billing: false })
  })
})
