import { describe, it, expect, beforeEach, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsSettingsDialog from '../OpsSettingsDialog.vue'

const mockGetAlertRuntimeSettings = vi.fn()
const mockGetEmailNotificationConfig = vi.fn()
const mockGetAdvancedSettings = vi.fn()
const mockGetMetricThresholds = vi.fn()
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
    updateAlertRuntimeSettings: (...args: any[]) => mockUpdateAlertRuntimeSettings(...args),
    updateEmailNotificationConfig: (...args: any[]) => mockUpdateEmailNotificationConfig(...args),
    updateAdvancedSettings: (...args: any[]) => mockUpdateAdvancedSettings(...args),
    updateMetricThresholds: (...args: any[]) => mockUpdateMetricThresholds(...args),
  },
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
    await wrapper.find('.btn-primary').trigger('click')
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
})
