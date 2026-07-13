import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

const { updateAccountMock, checkMixedChannelRiskMock, fetchUpstreamModelsMock } = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  fetchUpstreamModelsMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: true
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock,
      fetchUpstreamModels: fetchUpstreamModelsMock
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
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

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const ModelWhitelistSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: {
    modelValue: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: `
    <div>
      <button
        type="button"
        data-testid="rewrite-to-snapshot"
        @click="$emit('update:modelValue', ['gpt-5.2-2025-12-11'])"
      >
        rewrite
      </button>
      <span data-testid="model-whitelist-value">
        {{ Array.isArray(modelValue) ? modelValue.join(',') : '' }}
      </span>
    </div>
  `
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: `
    <select
      v-bind="$attrs"
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
})

function buildAccount() {
  return {
    id: 1,
    name: 'OpenAI Key',
    notes: '',
    platform: 'openai',
    type: 'apikey',
    credentials: {
      api_key: 'sk-test',
      base_url: 'https://api.openai.com',
      model_mapping: {
        'gpt-5.2': 'gpt-5.2'
      }
    },
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false
  } as any
}

function mountModal(account = buildAccount()) {
  return mount(EditAccountModal, {
    props: {
      show: true,
      account,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: ModelWhitelistSelectorStub
      }
    }
  })
}

describe('EditAccountModal', () => {
  it('reopening the same account rehydrates the OpenAI whitelist from props', async () => {
    const account = buildAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('[data-testid="rewrite-to-snapshot"]').trigger('click')
    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2-2025-12-11')

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
      'gpt-5.2': 'gpt-5.2'
    })
  })

  it('submits OpenAI compact mode and compact-only model mapping', async () => {
    const account = buildAccount()
    account.extra = {
      openai_compact_mode: 'force_on'
    }
    account.credentials = {
      ...account.credentials,
      compact_model_mapping: {
        'gpt-5.4': 'gpt-5.4-openai-compact'
      }
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.openai_compact_mode).toBe('force_on')
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.compact_model_mapping).toEqual({
      'gpt-5.4': 'gpt-5.4-openai-compact'
    })
  })

  it('removes deprecated account-level Codex image generation bridge overrides', async () => {
    const account = buildAccount()
    account.extra = {
      codex_image_generation_bridge: false,
      codex_image_generation_bridge_enabled: true
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    expect(wrapper.find('[data-testid^="codex-image-bridge-"]').exists()).toBe(false)
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('codex_image_generation_bridge')
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('codex_image_generation_bridge_enabled')
  })
  it('removes upstream prepaid and deprecated upstream warning fields when saving', async () => {
    const account = buildAccount()
    account.credentials = {
      ...account.credentials,
      pool_mode: false
    }
    account.extra = {
      ...(account.extra || {}),
      upstream_prepaid_amount: 120,
      upstream_warning_amount: 10,
      upstream_notify_enabled: true
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('[data-testid="pool-mode-toggle"]').trigger('click')
    expect(wrapper.find('[data-testid="upstream-prepaid-amount"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="upstream-warning-amount"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="upstream-notify-enabled"]').exists()).toBe(false)
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.credentials?.pool_mode).toBe(true)
    expect(payload?.extra).not.toHaveProperty('upstream_prepaid_amount')
    expect(payload?.extra).not.toHaveProperty('upstream_warning_amount')
    expect(payload?.extra).not.toHaveProperty('upstream_notify_enabled')
  })

  it('fetches upstream models and saves identity model mappings', async () => {
    const account = buildAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    fetchUpstreamModelsMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)
    fetchUpstreamModelsMock.mockResolvedValue([
      { id: 'z-model', type: 'model', display_name: 'z-model', created_at: '' },
      { id: 'a-model', type: 'model', display_name: 'a-model', created_at: '' },
      { id: 'z-model', type: 'model', display_name: 'z-model', created_at: '' }
    ])

    const wrapper = mountModal(account)

    await wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.modelMapping'))!.trigger('click')
    const fetchButton = wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.fetchUpstreamModels'))
    expect(fetchButton).toBeTruthy()
    await fetchButton!.trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(fetchUpstreamModelsMock).toHaveBeenCalledWith(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
      'z-model': 'z-model',
      'a-model': 'a-model'
    })
  })


  it('fetches upstream models and saves mappings for Anthropic setup token accounts', async () => {
    const account = buildAccount()
    account.id = 2
    account.name = 'Anthropic Setup Token'
    account.platform = 'anthropic'
    account.type = 'setup-token'
    account.credentials = {
      access_token: 'claude-token',
      model_mapping: {
        'claude-old': 'claude-new'
      }
    }
    account.extra = {}
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    fetchUpstreamModelsMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)
    fetchUpstreamModelsMock.mockResolvedValue([
      { id: 'claude-opus-4-1', type: 'model', display_name: 'claude-opus-4-1', created_at: '' },
      { id: 'claude-sonnet-4-5', type: 'model', display_name: 'claude-sonnet-4-5', created_at: '' }
    ])

    const wrapper = mountModal(account)

    await wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.modelMapping'))!.trigger('click')
    const fetchButton = wrapper.findAll('button').find((button) => button.text().includes('admin.accounts.fetchUpstreamModels'))
    expect(fetchButton).toBeTruthy()
    await fetchButton!.trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(fetchUpstreamModelsMock).toHaveBeenCalledWith(2)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
      'claude-opus-4-1': 'claude-opus-4-1',
      'claude-sonnet-4-5': 'claude-sonnet-4-5'
    })
  })

  it('does not fetch upstream models when OpenAI passthrough is enabled', async () => {
    const account = buildAccount()
    account.extra = { openai_passthrough: true }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    fetchUpstreamModelsMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    const text = wrapper.text()
    expect(text).toContain('admin.accounts.openai.modelRestrictionDisabledByPassthrough')
    expect(text).not.toContain('admin.accounts.fetchUpstreamModels')
    expect(fetchUpstreamModelsMock).not.toHaveBeenCalled()
  })

})
