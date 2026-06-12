import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { AdminUser } from '@/types'
import UsersView from '../UsersView.vue'

const {
  listUsers,
  listTags,
  batchAction,
  getAllGroups,
  getBatchUsersUsage,
  listEnabledDefinitions,
  getBatchUserAttributes
} = vi.hoisted(() => ({
  listUsers: vi.fn(),
  listTags: vi.fn(),
  batchAction: vi.fn(),
  getAllGroups: vi.fn(),
  getBatchUsersUsage: vi.fn(),
  listEnabledDefinitions: vi.fn(),
  getBatchUserAttributes: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      list: listUsers,
      listTags,
      batchAction,
      toggleStatus: vi.fn(),
      delete: vi.fn()
    },
    groups: {
      getAll: getAllGroups
    },
    dashboard: {
      getBatchUsersUsage
    },
    userAttributes: {
      listEnabledDefinitions,
      getBatchUserAttributes
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const createAdminUser = (overrides: Partial<AdminUser> = {}): AdminUser => ({
  id: 42,
  username: 'scoped-user',
  email: 'scoped@example.com',
  role: 'user',
  balance: 0,
  concurrency: 1,
  status: 'active',
  allowed_groups: [],
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-04-17T00:00:00Z',
  updated_at: '2026-04-17T00:00:00Z',
  notes: '',
  last_active_at: '2026-04-16T02:00:00Z',
  last_used_at: '2026-04-17T02:00:00Z',
  current_concurrency: 0,
  ...overrides
})

const DataTableStub = {
  props: ['columns', 'data'],
  emits: ['sort'],
  template: `
    <div>
      <div data-test="columns">{{ columns.map(col => col.key).join(',') }}</div>
      <button data-test="sort-last-used" @click="$emit('sort', 'last_used_at', 'desc')">sort</button>
      <div data-test="header-select">
        <slot name="header-select" />
      </div>
      <div v-for="row in data" :key="row.id">
        <div data-test="row-select">
          <slot name="cell-select" :row="row" />
        </div>
        <slot name="cell-last_used_at" :value="row.last_used_at" :row="row" />
      </div>
    </div>
  `
}

const BaseDialogStub = {
  props: ['show'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
}

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  methods: {
    handleChange(event: Event) {
      const value = (event.target as HTMLSelectElement).value
      const option = this.options.find((item: any) => String(item.value) === value) || null
      this.$emit('update:modelValue', value)
      this.$emit('change', value, option)
    }
  },
  template: `
    <select data-test="select" :value="modelValue" @change="handleChange">
      <option v-for="option in options" :key="String(option.value)" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
}

describe('admin UsersView', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  beforeEach(() => {
    localStorage.clear()

    listUsers.mockReset()
    listTags.mockReset()
    batchAction.mockReset()
    getAllGroups.mockReset()
    getBatchUsersUsage.mockReset()
    listEnabledDefinitions.mockReset()
    getBatchUserAttributes.mockReset()

    listUsers.mockResolvedValue({
      items: [createAdminUser()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listTags.mockResolvedValue([])
    batchAction.mockResolvedValue({ total: 0, success: 0, failed: 0 })
    getAllGroups.mockResolvedValue([])
    getBatchUsersUsage.mockResolvedValue({ stats: {} })
    listEnabledDefinitions.mockResolvedValue([])
    getBatchUserAttributes.mockResolvedValue({ values: {} })
  })

  it('shows active, used, and created activity columns in order and requests last_used_at sort', async () => {
    const wrapper = mount(UsersView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          EmptyState: true,
          GroupBadge: true,
          Select: true,
          UserAttributesConfigModal: true,
          UserConcurrencyCell: true,
          UserCreateModal: true,
          UserEditModal: true,
          UserApiKeysModal: true,
          UserAllowedGroupsModal: true,
          UserBalanceModal: true,
          UserBalanceHistoryModal: true,
          GroupReplaceModal: true,
          Icon: true,
          Teleport: true
        }
      }
    })

    await flushPromises()

    const columns = wrapper.get('[data-test="columns"]').text()
    const visibleColumns = columns.split(',')
    expect(visibleColumns.slice(-4, -1)).toEqual(['last_active_at', 'last_used_at', 'created_at'])
    expect(visibleColumns).not.toContain('last_login_at')

    await wrapper.get('[data-test="sort-last-used"]').trigger('click')
    await flushPromises()

    expect(listUsers).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({
        sort_by: 'last_used_at',
        sort_order: 'desc'
      }),
      expect.any(Object)
    )
  })

  it('passes balance between filters to the users API', async () => {
    localStorage.setItem('user-visible-filters', JSON.stringify(['balance']))
    localStorage.setItem('user-filter-values', JSON.stringify({
      balanceType: 'between',
      balanceMin: '10',
      balanceMax: '100'
    }))

    mount(UsersView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          EmptyState: true,
          GroupBadge: true,
          Select: true,
          UserAttributesConfigModal: true,
          UserConcurrencyCell: true,
          UserCreateModal: true,
          UserEditModal: true,
          UserApiKeysModal: true,
          UserAllowedGroupsModal: true,
          UserBalanceModal: true,
          UserBalanceHistoryModal: true,
          GroupReplaceModal: true,
          Icon: true,
          Teleport: true
        }
      }
    })

    await flushPromises()

    expect(listUsers).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({
        balance_filter_type: 'between',
        balance_min: '10',
        balance_max: '100'
      }),
      expect.any(Object)
    )
  })

  it('passes negative less-than balance filters to the users API', async () => {
    localStorage.setItem('user-visible-filters', JSON.stringify(['balance']))
    localStorage.setItem('user-filter-values', JSON.stringify({
      balanceType: 'lt',
      balanceMax: '-0.01'
    }))

    mount(UsersView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          EmptyState: true,
          GroupBadge: true,
          Select: true,
          UserAttributesConfigModal: true,
          UserConcurrencyCell: true,
          UserCreateModal: true,
          UserEditModal: true,
          UserApiKeysModal: true,
          UserAllowedGroupsModal: true,
          UserBalanceModal: true,
          UserBalanceHistoryModal: true,
          GroupReplaceModal: true,
          Icon: true,
          Teleport: true
        }
      }
    })

    await flushPromises()

    expect(listUsers).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({
        balance_filter_type: 'lt',
        balance_max: '-0.01'
      }),
      expect.any(Object)
    )
  })

  it('keeps applying persisted balance filter even when the filter control is hidden', async () => {
    localStorage.setItem('user-visible-filters', JSON.stringify([]))
    localStorage.setItem('user-filter-values', JSON.stringify({
      balanceType: 'between',
      balanceMin: '10',
      balanceMax: '100'
    }))

    mount(UsersView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          EmptyState: true,
          GroupBadge: true,
          Select: true,
          UserAttributesConfigModal: true,
          UserConcurrencyCell: true,
          UserCreateModal: true,
          UserEditModal: true,
          UserApiKeysModal: true,
          UserAllowedGroupsModal: true,
          UserBalanceModal: true,
          UserBalanceHistoryModal: true,
          GroupReplaceModal: true,
          Icon: true,
          Teleport: true
        }
      }
    })

    await flushPromises()

    expect(listUsers).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({
        balance_filter_type: 'between',
        balance_min: '10',
        balance_max: '100'
      }),
      expect.any(Object)
    )
  })

  it('applies balance greater-than-or-equal filter after selecting type and entering an amount', async () => {
    localStorage.setItem('user-visible-filters', JSON.stringify(['balance']))

    const wrapper = mount(UsersView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          EmptyState: true,
          GroupBadge: true,
          Select: SelectStub,
          UserAttributesConfigModal: true,
          UserConcurrencyCell: true,
          UserCreateModal: true,
          UserEditModal: true,
          UserApiKeysModal: true,
          UserAllowedGroupsModal: true,
          UserBalanceModal: true,
          UserBalanceHistoryModal: true,
          GroupReplaceModal: true,
          UserTagManagementModal: true,
          Icon: true,
          Teleport: true
        }
      }
    })

    await flushPromises()
    expect(listUsers).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-test="select"]').setValue('gte')
    await flushPromises()
    expect(listUsers).toHaveBeenCalledTimes(1)

    await wrapper.get('input[type="number"]').setValue('10')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(listUsers).toHaveBeenCalledTimes(2)
    expect(listUsers).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({
        balance_filter_type: 'gte',
        balance_min: '10',
        balance_max: undefined
      }),
      expect.any(Object)
    )
  })

  it('calls batch disable for selected users', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    batchAction.mockResolvedValue({ total: 2, success: 2, failed: 0 })
    listUsers.mockResolvedValue({
      items: [
        createAdminUser({ id: 42, email: 'first@example.com' }),
        createAdminUser({ id: 43, email: 'second@example.com' })
      ],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mount(UsersView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          BaseDialog: BaseDialogStub,
          Pagination: true,
          ConfirmDialog: true,
          EmptyState: true,
          GroupBadge: true,
          Select: true,
          UserAttributesConfigModal: true,
          UserConcurrencyCell: true,
          UserCreateModal: true,
          UserEditModal: true,
          UserApiKeysModal: true,
          UserAllowedGroupsModal: true,
          UserBalanceModal: true,
          UserBalanceHistoryModal: true,
          GroupReplaceModal: true,
          UserTagManagementModal: true,
          Icon: true,
          Teleport: true
        }
      }
    })

    await flushPromises()

    const rowCheckboxes = wrapper.findAll('[data-test="row-select"] input[type="checkbox"]')
    await rowCheckboxes[0].setValue(true)
    await rowCheckboxes[1].setValue(true)
    await flushPromises()

    expect(wrapper.find('[data-test="user-bulk-actions"]').exists()).toBe(true)
    await wrapper.get('[data-test="bulk-disable-users"]').trigger('click')
    await flushPromises()

    expect(confirmSpy).toHaveBeenCalled()
    expect(batchAction).toHaveBeenCalledWith({
      user_ids: [42, 43],
      action: 'disable'
    })
  })

  it('opens batch tag dialog and submits selected tags', async () => {
    listTags.mockResolvedValue([
      { id: 7, name: 'vip', created_at: '2026-04-17T00:00:00Z', updated_at: '2026-04-17T00:00:00Z' }
    ])
    batchAction.mockResolvedValue({ total: 1, success: 1, failed: 0 })

    const wrapper = mount(UsersView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          BaseDialog: BaseDialogStub,
          Pagination: true,
          ConfirmDialog: true,
          EmptyState: true,
          GroupBadge: true,
          Select: true,
          UserAttributesConfigModal: true,
          UserConcurrencyCell: true,
          UserCreateModal: true,
          UserEditModal: true,
          UserApiKeysModal: true,
          UserAllowedGroupsModal: true,
          UserBalanceModal: true,
          UserBalanceHistoryModal: true,
          GroupReplaceModal: true,
          UserTagManagementModal: true,
          Icon: true,
          Teleport: true
        }
      }
    })

    await flushPromises()

    await wrapper.get('[data-test="row-select"] input[type="checkbox"]').setValue(true)
    await flushPromises()
    await wrapper.get('[data-test="bulk-tag-users"]').trigger('click')
    await flushPromises()

    expect(listTags).toHaveBeenCalled()
    await wrapper.get('[data-test="bulk-tag-checkbox"]').setValue(true)
    await wrapper.get('[data-test="bulk-tag-submit"]').trigger('click')
    await flushPromises()

    expect(batchAction).toHaveBeenCalledWith({
      user_ids: [42],
      action: 'add_tags',
      tag_ids: [7]
    })
  })

  it('blocks invalid balance between filters before querying users', async () => {
    localStorage.setItem('user-visible-filters', JSON.stringify(['balance']))
    localStorage.setItem('user-filter-values', JSON.stringify({
      balanceType: 'between',
      balanceMin: '100',
      balanceMax: '10'
    }))

    mount(UsersView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          EmptyState: true,
          GroupBadge: true,
          Select: true,
          UserAttributesConfigModal: true,
          UserConcurrencyCell: true,
          UserCreateModal: true,
          UserEditModal: true,
          UserApiKeysModal: true,
          UserAllowedGroupsModal: true,
          UserBalanceModal: true,
          UserBalanceHistoryModal: true,
          GroupReplaceModal: true,
          Icon: true,
          Teleport: true
        }
      }
    })

    await flushPromises()

    expect(listUsers).not.toHaveBeenCalled()
  })

})
