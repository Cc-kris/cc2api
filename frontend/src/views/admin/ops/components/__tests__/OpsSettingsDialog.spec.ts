import { describe, it, expect, beforeEach, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsSettingsDialog from '../OpsSettingsDialog.vue'

const mockGetAlertRuntimeSettings = vi.fn()
const mockGetEmailNotificationConfig = vi.fn()
const mockGetAdvancedSettings = vi.fn()
const mockGetMetricThresholds = vi.fn()
const mockGetAIAnalysisConfig = vi.fn()
const mockListAlertRules = vi.fn()
const mockCreateAlertRule = vi.fn()
const mockUpdateAlertRule = vi.fn()
const mockTestAIAnalysisConnection = vi.fn()
const mockRouterPush = vi.fn()
const mockUpdateAlertRuntimeSettings = vi.fn()
const mockUpdateEmailNotificationConfig = vi.fn()
const mockUpdateAdvancedSettings = vi.fn()
const mockUpdateMetricThresholds = vi.fn()
const mockShowError = vi.fn()
const mockShowSuccess = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getAlertRuntimeSettings: (...args: any[]) => mockGetAlertRuntimeSettings(...args),
    getEmailNotificationConfig: (...args: any[]) => mockGetEmailNotificationConfig(...args),
    getAdvancedSettings: (...args: any[]) => mockGetAdvancedSettings(...args),
    getMetricThresholds: (...args: any[]) => mockGetMetricThresholds(...args),
    getAIAnalysisConfig: (...args: any[]) => mockGetAIAnalysisConfig(...args),
    listAlertRules: (...args: any[]) => mockListAlertRules(...args),
    createAlertRule: (...args: any[]) => mockCreateAlertRule(...args),
    updateAlertRule: (...args: any[]) => mockUpdateAlertRule(...args),
    testAIAnalysisConnection: (...args: any[]) => mockTestAIAnalysisConnection(...args),
    updateAlertRuntimeSettings: (...args: any[]) => mockUpdateAlertRuntimeSettings(...args),
    updateEmailNotificationConfig: (...args: any[]) => mockUpdateEmailNotificationConfig(...args),
    updateAdvancedSettings: (...args: any[]) => mockUpdateAdvancedSettings(...args),
    updateMetricThresholds: (...args: any[]) => mockUpdateMetricThresholds(...args),
  },
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mockRouterPush,
  }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: mockShowError,
    showSuccess: mockShowSuccess,
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})
const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: '' },
    width: { type: String, default: '' },
  },
  emits: ['close'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const SelectStub = defineComponent({
  name: 'SelectControlStub',
  props: {
    modelValue: { type: [String, Number], default: '' },
    options: { type: Array, default: () => [] },
  },
  emits: ['update:modelValue'],
  methods: {
    optionValue(option: any) {
      return option.value
    },
    optionLabel(option: any) {
      return option.label
    },
    onChange(event: Event) {
      this.$emit('update:modelValue', (event.target as HTMLSelectElement).value)
    },
  },
  template: '<select :value="modelValue" @change="onChange"><option v-for="option in options" :key="String(optionValue(option))" :value="optionValue(option)">{{ optionLabel(option) }}</option></select>',
})

const ToggleStub = defineComponent({
  name: 'Toggle',
  props: {
    modelValue: { type: Boolean, default: false },
  },
  emits: ['update:modelValue'],
  methods: {
    onChange(event: Event) {
      this.$emit('update:modelValue', (event.target as HTMLInputElement).checked)
    },
  },
  template: '<input type="checkbox" :checked="modelValue" @change="onChange" />',
})

function runtimeSettings() {
  return {
    evaluation_interval_seconds: 60,
    distributed_lock: { enabled: true, key: 'ops:alert:evaluator:leader', ttl_seconds: 30 },
    silencing: { enabled: false, global_until_rfc3339: '', global_reason: '', entries: [] },
    thresholds: {},
  }
}

function emailConfig() {
  return {
    alert: {
      enabled: true,
      recipients: [],
      min_severity: '',
      rate_limit_per_hour: 0,
      batching_window_seconds: 0,
      include_resolved_alerts: false,
    },
    report: {
      enabled: false,
      recipients: [],
      daily_summary_enabled: false,
      daily_summary_schedule: '0 9 * * *',
      weekly_summary_enabled: false,
      weekly_summary_schedule: '0 9 * * 1',
      error_digest_enabled: false,
      error_digest_schedule: '0 9 * * *',
      error_digest_min_count: 10,
      account_health_enabled: false,
      account_health_schedule: '0 9 * * *',
      account_health_error_rate_threshold: 10,
    },
  }
}

function advancedSettings() {
  return {
    data_retention: {
      cleanup_enabled: false,
      cleanup_schedule: '0 3 * * *',
      error_log_retention_days: 30,
      minute_metrics_retention_days: 30,
      hourly_metrics_retention_days: 90,
    },
    aggregation: {
      aggregation_enabled: true,
      minute_aggregation_interval_seconds: 60,
      hourly_aggregation_interval_seconds: 3600,
    },
    ignore_count_tokens_errors: true,
    ignore_context_canceled: true,
    ignore_no_available_accounts: true,
    ignore_invalid_api_key_errors: true,
    ignore_insufficient_balance_errors: true,
    display_openai_token_stats: true,
    display_alert_events: true,
    auto_refresh_enabled: true,
    auto_refresh_interval_seconds: 30,
  }
}

function aiAnalysisConfig() {
  return {
    enabled: true,
    base_url: 'https://ai.example.com/v1',
    api_key_masked: 'sk-***test',
    model: 'gpt-5.5',
    interface_type: 'responses',
    timeout_seconds: 60,
    max_samples: 50,
    auto_dedup_minutes: 10,
    global_rate_limit_per_minute: 10,
    auto_levels: ['P0', 'P1'],
    manual_enabled: true,
  }
}

describe('OpsSettingsDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetAlertRuntimeSettings.mockResolvedValue(runtimeSettings())
    mockGetEmailNotificationConfig.mockResolvedValue(emailConfig())
    mockGetAdvancedSettings.mockResolvedValue(advancedSettings())
    mockGetMetricThresholds.mockResolvedValue({
      sla_percent_min: 99.5,
      ttft_p99_ms_max: 500,
      request_error_rate_percent_max: 5,
      upstream_error_rate_percent_max: 5,
    })
    mockGetAIAnalysisConfig.mockResolvedValue(aiAnalysisConfig())
    mockListAlertRules.mockResolvedValue([
      {
        id: 99,
        name: '健康分过低告警',
        enabled: true,
        metric_type: 'health_score',
        operator: '<',
        threshold: 60,
        window_minutes: 1,
        sustained_minutes: 1,
        severity: 'P1',
        cooldown_minutes: 15,
        notify_email: true,
        rule_version: 'v2',
        trigger_level: 'P1',
        notification_channels: ['in_app', 'email'],
        silence_minutes: 15,
        auto_ai_analysis: true,
      },
    ])
    mockCreateAlertRule.mockImplementation(async (rule) => ({ ...rule, id: 100 }))
    mockUpdateAlertRule.mockImplementation(async (_id, rule) => ({ ...rule, id: _id }))
    mockTestAIAnalysisConnection.mockResolvedValue({ success: true, message: 'ok' })
    mockRouterPush.mockResolvedValue(undefined)
    mockUpdateAlertRuntimeSettings.mockResolvedValue(runtimeSettings())
    mockUpdateEmailNotificationConfig.mockImplementation(async (config) => config)
    mockUpdateAdvancedSettings.mockResolvedValue(advancedSettings())
    mockUpdateMetricThresholds.mockResolvedValue(undefined)
  })

  it('保存时会把当前输入框里的预警邮箱加入收件人并保持开启', async () => {
    const wrapper = mount(OpsSettingsDialog, {
      props: { show: false },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          Toggle: ToggleStub,
        },
      },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()
    const alertEmailInput = wrapper.find('input[type="email"]')
    await alertEmailInput.setValue('Ops@Example.COM')
    const saveButton = wrapper.findAll('.btn-primary').at(-1)
    expect(saveButton).toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(mockUpdateEmailNotificationConfig).toHaveBeenCalledWith(
      expect.objectContaining({
        alert: expect.objectContaining({
          enabled: true,
          recipients: ['ops@example.com'],
        }),
      })
    )
  })

  it('在运维设置中展示 AI 分析配置入口并跳转现有配置页面', async () => {
    const wrapper = mount(OpsSettingsDialog, {
      props: { show: false },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          Toggle: ToggleStub,
        },
      },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(mockGetAIAnalysisConfig).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('admin.ops.settings.aiAnalysisConfig')
    expect(wrapper.text()).toContain('https://ai.example.com/v1')
    expect(wrapper.text()).toContain('responses / gpt-5.5')

    const openButton = wrapper.findAll('button').find((button) => button.text() === 'admin.ops.settings.aiAnalysisOpenConfig')
    expect(openButton).toBeTruthy()
    await openButton!.trigger('click')

    expect(mockRouterPush).toHaveBeenCalledWith({ name: 'AdminOpsAIAnalysis' })
  })

  it('在运维设置中保存新版健康分告警规则字段', async () => {
    const wrapper = mount(OpsSettingsDialog, {
      props: { show: false },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          Toggle: ToggleStub,
        },
      },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(mockListAlertRules).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('新版运维告警规则')
    expect(wrapper.text()).toContain('健康分过低告警')

    const thresholdInput = wrapper.findAll('input[type="number"]').find((input) => input.attributes('max') === '100' && input.attributes('step') === '1')
    expect(thresholdInput).toBeTruthy()
    await thresholdInput!.setValue('55')

    const alertEmailInput = wrapper.find('input[type="email"]')
    await alertEmailInput.setValue('ops@example.com')

    const saveButton = wrapper.findAll('.btn-primary').at(-1)
    await saveButton!.trigger('click')
    await flushPromises()

    expect(mockUpdateAlertRule).toHaveBeenCalledWith(
      99,
      expect.objectContaining({
        rule_version: 'v2',
        metric_type: 'health_score',
        operator: '<',
        threshold: 55,
        trigger_level: 'P1',
        notification_channels: ['in_app', 'email'],
        silence_minutes: 15,
        notify_email: true,
      })
    )
  })

})
