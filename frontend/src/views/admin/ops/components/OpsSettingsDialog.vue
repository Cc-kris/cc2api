<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { opsAPI, type AlertRule, type OpsAIAnalysisConfig } from '@/api/admin/ops'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import type { OpsAlertRuntimeSettings, EmailNotificationConfig, AlertSeverity, OpsAdvancedSettings, OpsMetricThresholds } from '../types'

const { t } = useI18n()
const appStore = useAppStore()
const router = useRouter()

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  close: []
  saved: []
}>()

const loading = ref(false)
const saving = ref(false)

// 运行时设置
const runtimeSettings = ref<OpsAlertRuntimeSettings | null>(null)
// 邮件通知配置
const emailConfig = ref<EmailNotificationConfig | null>(null)
// 高级设置
const advancedSettings = ref<OpsAdvancedSettings | null>(null)
// AI 分析配置入口
const aiAnalysisConfig = ref<OpsAIAnalysisConfig | null>(null)
const aiAnalysisTesting = ref(false)

type AlertTriggerLevel = 'P0' | 'P1' | 'P2' | 'observe'
type NotificationChannel = 'in_app' | 'email' | 'none'

interface HealthScoreAlertDraft {
  id?: number
  enabled: boolean
  threshold: number
  trigger_level: AlertTriggerLevel
  notification_channels: NotificationChannel[]
  silence_minutes: number
  auto_ai_analysis: boolean
}

const alertRules = ref<AlertRule[]>([])
const healthScoreAlert = ref<HealthScoreAlertDraft>({
  enabled: true,
  threshold: 60,
  trigger_level: 'P1',
  notification_channels: ['in_app', 'email'],
  silence_minutes: 10,
  auto_ai_analysis: true
})

const triggerLevelOptions: Array<{ value: AlertTriggerLevel; label: string }> = [
  { value: 'P0', label: 'P0 紧急' },
  { value: 'P1', label: 'P1 高风险' },
  { value: 'P2', label: 'P2 关注' },
  { value: 'observe', label: '观察' }
]

const notificationChannelOptions: Array<{ value: NotificationChannel; label: string }> = [
  { value: 'in_app', label: '站内事件' },
  { value: 'email', label: '邮件' },
  { value: 'none', label: '不通知' }
]

// 指标阈值配置
const metricThresholds = ref<OpsMetricThresholds>({
  sla_percent_min: 99.5,
  ttft_p99_ms_max: 500,
  request_error_rate_percent_max: 5,
  upstream_error_rate_percent_max: 5
})

// 加载所有配置
async function loadAllSettings() {
  loading.value = true
  try {
    const [runtime, email, advanced, thresholds, aiConfig, rules] = await Promise.all([
      opsAPI.getAlertRuntimeSettings(),
      opsAPI.getEmailNotificationConfig(),
      opsAPI.getAdvancedSettings(),
      opsAPI.getMetricThresholds(),
      opsAPI.getAIAnalysisConfig(),
      opsAPI.listAlertRules()
    ])
    runtimeSettings.value = runtime
    emailConfig.value = email
    advancedSettings.value = advanced
    aiAnalysisConfig.value = aiConfig
    alertRules.value = Array.isArray(rules) ? rules : []
    syncHealthScoreAlertDraft()
    // 如果后端返回了阈值，使用后端的值；否则保持默认值
    if (thresholds && Object.keys(thresholds).length > 0) {
        metricThresholds.value = {
          sla_percent_min: thresholds.sla_percent_min ?? 99.5,
          ttft_p99_ms_max: thresholds.ttft_p99_ms_max ?? 500,
          request_error_rate_percent_max: thresholds.request_error_rate_percent_max ?? 5,
          upstream_error_rate_percent_max: thresholds.upstream_error_rate_percent_max ?? 5
        }
    }
  } catch (err: any) {
    console.error('[OpsSettingsDialog] Failed to load settings', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.settings.loadFailed'))
  } finally {
    loading.value = false
  }
}

// 监听弹窗打开
watch(() => props.show, (show) => {
  if (show) {
    loadAllSettings()
  }
})

// 邮件输入
const alertRecipientInput = ref('')
const reportRecipientInput = ref('')

// 严重级别选项
const severityOptions: Array<{ value: AlertSeverity | ''; label: string }> = [
  { value: '', label: t('admin.ops.email.minSeverityAll') },
  { value: 'critical', label: 'Critical（仅 P0）' },
  { value: 'warning', label: 'Warning（P0 + P1）' },
  { value: 'info', label: 'Info（P0 + P1 + P2+）' }
]

// 验证邮箱
function isValidEmailAddress(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)
}

function getPendingRecipientInput(target: 'alert' | 'report') {
  return (target === 'alert' ? alertRecipientInput.value : reportRecipientInput.value).trim()
}

function setPendingRecipientInput(target: 'alert' | 'report', value: string) {
  if (target === 'alert') alertRecipientInput.value = value
  else reportRecipientInput.value = value
}

function commitRecipientInput(target: 'alert' | 'report', showError = true): boolean {
  if (!emailConfig.value) return true
  const raw = getPendingRecipientInput(target)
  if (!raw) return true

  if (!isValidEmailAddress(raw)) {
    if (showError) appStore.showError(t('common.invalidEmail'))
    return false
  }

  const normalized = raw.toLowerCase()
  const list = target === 'alert' ? emailConfig.value.alert.recipients : emailConfig.value.report.recipients
  if (!list.includes(normalized)) {
    list.push(normalized)
  }
  setPendingRecipientInput(target, '')
  return true
}

// 添加收件人
function addRecipient(target: 'alert' | 'report') {
  commitRecipientInput(target)
}

// 移除收件人
function removeRecipient(target: 'alert' | 'report', email: string) {
  if (!emailConfig.value) return
  const list = target === 'alert' ? emailConfig.value.alert.recipients : emailConfig.value.report.recipients
  const idx = list.indexOf(email)
  if (idx >= 0) list.splice(idx, 1)
}

function normalizeNotificationChannels(value?: string[], notifyEmail?: boolean): NotificationChannel[] {
  const allowed = new Set<NotificationChannel>(['in_app', 'email', 'none'])
  const channels = Array.isArray(value)
    ? value.map(item => String(item || '').trim()).filter((item): item is NotificationChannel => allowed.has(item as NotificationChannel))
    : []
  if (notifyEmail && !channels.includes('email')) channels.push('email')
  if (channels.length === 0) channels.push('in_app')
  if (channels.includes('none')) return ['none']
  return Array.from(new Set(channels))
}

function syncHealthScoreAlertDraft() {
  const rule = alertRules.value.find(item => item.rule_version === 'v2' && item.metric_type === 'health_score')
    || alertRules.value.find(item => item.metric_type === 'health_score')
  if (!rule) {
    healthScoreAlert.value = {
      enabled: true,
      threshold: 60,
      trigger_level: 'P1',
      notification_channels: ['in_app', 'email'],
      silence_minutes: 10,
      auto_ai_analysis: true
    }
    return
  }
  healthScoreAlert.value = {
    id: rule.id,
    enabled: rule.enabled,
    threshold: Number(rule.threshold || 60),
    trigger_level: (rule.trigger_level === 'P0' || rule.trigger_level === 'P2' || rule.trigger_level === 'observe') ? rule.trigger_level : 'P1',
    notification_channels: normalizeNotificationChannels(rule.notification_channels, rule.notify_email),
    silence_minutes: Number(rule.silence_minutes ?? rule.cooldown_minutes ?? 10),
    auto_ai_analysis: rule.auto_ai_analysis !== false
  }
}

function toggleHealthScoreChannel(channel: NotificationChannel, checked: boolean) {
  const current = new Set(healthScoreAlert.value.notification_channels)
  if (channel === 'none') {
    healthScoreAlert.value.notification_channels = checked ? ['none'] : ['in_app']
    return
  }
  current.delete('none')
  if (checked) current.add(channel)
  else current.delete(channel)
  healthScoreAlert.value.notification_channels = current.size ? Array.from(current) : ['none']
}

function severityFromTriggerLevel(level: AlertTriggerLevel) {
  return level === 'observe' ? 'P3' : level
}

function buildHealthScoreAlertPayload(): AlertRule {
  const channels = normalizeNotificationChannels(healthScoreAlert.value.notification_channels)
  return {
    id: healthScoreAlert.value.id,
    name: '健康分过低告警',
    description: '健康分低于配置阈值时触发运维告警。',
    enabled: healthScoreAlert.value.enabled,
    metric_type: 'health_score',
    operator: '<',
    threshold: Number(healthScoreAlert.value.threshold || 0),
    window_minutes: 1,
    sustained_minutes: 1,
    severity: severityFromTriggerLevel(healthScoreAlert.value.trigger_level),
    cooldown_minutes: Number(healthScoreAlert.value.silence_minutes || 0),
    notify_email: channels.includes('email'),
    filters: { default_rule_key: 'settings_health_score_low' },
    rule_version: 'v2',
    error_categories: [],
    trigger_level: healthScoreAlert.value.trigger_level,
    min_final_failures: 1,
    min_failure_rate: 0,
    min_sample_count: 1,
    impact_scope: {},
    recovered_fluctuation_policy: 'record_only',
    min_recovered_fluctuations: 0,
    auto_ai_analysis: healthScoreAlert.value.auto_ai_analysis,
    notification_channels: channels,
    silence_minutes: Number(healthScoreAlert.value.silence_minutes || 0),
    migration_state: 'normal'
  }
}

async function saveHealthScoreAlertRule() {
  const payload = buildHealthScoreAlertPayload()
  if (healthScoreAlert.value.id) {
    await opsAPI.updateAlertRule(healthScoreAlert.value.id, payload)
  } else {
    const created = await opsAPI.createAlertRule(payload)
    healthScoreAlert.value.id = created.id
  }
}

// 验证
const validation = computed(() => {
  const errors: string[] = []

  // 验证运行时设置
  if (runtimeSettings.value) {
    const evalSeconds = runtimeSettings.value.evaluation_interval_seconds
    if (!Number.isFinite(evalSeconds) || evalSeconds < 1 || evalSeconds > 86400) {
      errors.push(t('admin.ops.runtime.validation.evalIntervalRange'))
    }
  }

  if (emailConfig.value) {
    const pendingAlert = getPendingRecipientInput('alert')
    const pendingReport = getPendingRecipientInput('report')

    if (pendingAlert && !isValidEmailAddress(pendingAlert)) {
      errors.push(t('common.invalidEmail'))
    }
    if (pendingReport && !isValidEmailAddress(pendingReport)) {
      errors.push(t('common.invalidEmail'))
    }
    if (emailConfig.value.alert.enabled && emailConfig.value.alert.recipients.length === 0 && !pendingAlert) {
      errors.push(t('admin.ops.email.validation.alertRecipientsRequired'))
    }
    if (emailConfig.value.report.enabled && emailConfig.value.report.recipients.length === 0 && !pendingReport) {
      errors.push(t('admin.ops.email.validation.reportRecipientsRequired'))
    }
  }

  if (healthScoreAlert.value) {
    const threshold = Number(healthScoreAlert.value.threshold)
    if (!Number.isFinite(threshold) || threshold < 0 || threshold > 100) {
      errors.push('健康分告警阈值必须在 0～100 之间')
    }
    const silenceMinutes = Number(healthScoreAlert.value.silence_minutes)
    if (!Number.isInteger(silenceMinutes) || silenceMinutes < 0 || silenceMinutes > 1440) {
      errors.push('告警静默时间必须是 0～1440 的整数分钟')
    }
    if (healthScoreAlert.value.enabled && healthScoreAlert.value.notification_channels.includes('email') && emailConfig.value?.alert.enabled && emailConfig.value.alert.recipients.length === 0 && !getPendingRecipientInput('alert')) {
      errors.push('健康分邮件告警需要填写预警收件人')
    }
  }

  // 验证高级设置
  if (advancedSettings.value) {
    const { error_log_retention_days, minute_metrics_retention_days, hourly_metrics_retention_days } = advancedSettings.value.data_retention
    if (error_log_retention_days < 0 || error_log_retention_days > 365) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }
    if (minute_metrics_retention_days < 0 || minute_metrics_retention_days > 365) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }
    if (hourly_metrics_retention_days < 0 || hourly_metrics_retention_days > 365) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }
  }

  // 验证指标阈值
  if (metricThresholds.value.sla_percent_min != null && (metricThresholds.value.sla_percent_min < 0 || metricThresholds.value.sla_percent_min > 100)) {
    errors.push(t('admin.ops.settings.validation.slaMinPercentRange'))
  }
  if (metricThresholds.value.ttft_p99_ms_max != null && metricThresholds.value.ttft_p99_ms_max < 0) {
    errors.push(t('admin.ops.settings.validation.ttftP99MaxRange'))
  }
  if (metricThresholds.value.request_error_rate_percent_max != null && (metricThresholds.value.request_error_rate_percent_max < 0 || metricThresholds.value.request_error_rate_percent_max > 100)) {
    errors.push(t('admin.ops.settings.validation.requestErrorRateMaxRange'))
  }
  if (metricThresholds.value.upstream_error_rate_percent_max != null && (metricThresholds.value.upstream_error_rate_percent_max < 0 || metricThresholds.value.upstream_error_rate_percent_max > 100)) {
    errors.push(t('admin.ops.settings.validation.upstreamErrorRateMaxRange'))
  }

  return { valid: errors.length === 0, errors }
})

// 保存所有配置

const aiAnalysisStatusText = computed(() => {
  if (!aiAnalysisConfig.value) return t('admin.ops.settings.aiAnalysisConfigNotLoaded')
  if (!aiAnalysisConfig.value.enabled) return t('admin.ops.settings.aiAnalysisConfigDisabled')
  return t('admin.ops.settings.aiAnalysisConfigEnabled')
})

const aiAnalysisModelText = computed(() => {
  if (!aiAnalysisConfig.value) return '--'
  const parts = [aiAnalysisConfig.value.interface_type, aiAnalysisConfig.value.model].filter(Boolean)
  return parts.length ? parts.join(' / ') : '--'
})

const aiAnalysisAutoLevelsText = computed(() => {
  const levels = aiAnalysisConfig.value?.auto_levels || []
  return levels.length ? levels.join(', ') : t('admin.ops.settings.aiAnalysisNoAutoLevels')
})

async function openAIAnalysisConfig() {
  emit('close')
  await router.push({ name: 'AdminOpsAIAnalysis' })
}

async function testAIAnalysisConfig() {
  if (aiAnalysisTesting.value) return
  aiAnalysisTesting.value = true
  try {
    const result = await opsAPI.testAIAnalysisConnection()
    if (result.success) {
      appStore.showSuccess(result.message || t('admin.ops.settings.aiAnalysisTestSuccess'))
    } else {
      appStore.showError(result.message || t('admin.ops.settings.aiAnalysisTestFailed'))
    }
  } catch (err: any) {
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || t('admin.ops.settings.aiAnalysisTestFailed'))
  } finally {
    aiAnalysisTesting.value = false
  }
}

async function saveAllSettings() {
  if (!validation.value.valid) {
    appStore.showError(validation.value.errors[0])
    return
  }

  if (!commitRecipientInput('alert') || !commitRecipientInput('report')) {
    return
  }

  if (!validation.value.valid) {
    appStore.showError(validation.value.errors[0])
    return
  }

  saving.value = true
  try {
    await Promise.all([
      runtimeSettings.value ? opsAPI.updateAlertRuntimeSettings(runtimeSettings.value) : Promise.resolve(),
      emailConfig.value ? opsAPI.updateEmailNotificationConfig(emailConfig.value) : Promise.resolve(),
      advancedSettings.value ? opsAPI.updateAdvancedSettings(advancedSettings.value) : Promise.resolve(),
      opsAPI.updateMetricThresholds(metricThresholds.value),
      saveHealthScoreAlertRule()
    ])
    appStore.showSuccess(t('admin.ops.settings.saveSuccess'))
    emit('saved')
    emit('close')
  } catch (err: any) {
    console.error('[OpsSettingsDialog] Failed to save settings', err)
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || t('admin.ops.settings.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <BaseDialog :show="show" :title="t('admin.ops.settings.title')" width="extra-wide" @close="emit('close')">
    <div v-if="loading" class="py-10 text-center text-sm text-gray-500">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="runtimeSettings && emailConfig && advancedSettings && aiAnalysisConfig" class="space-y-6">
      <!-- 验证错误 -->
      <div v-if="!validation.valid" class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
        <div class="font-bold">{{ t('admin.ops.settings.validation.title') }}</div>
        <ul class="mt-1 list-disc space-y-1 pl-4">
          <li v-for="msg in validation.errors" :key="msg">{{ msg }}</li>
        </ul>
      </div>

      <!-- 数据采集频率 -->
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.settings.dataCollection') }}</h4>
        <div>
          <label class="input-label">{{ t('admin.ops.settings.evaluationInterval') }}</label>
          <input
            v-model.number="runtimeSettings.evaluation_interval_seconds"
            type="number"
            min="1"
            max="86400"
            class="input"
          />
          <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.evaluationIntervalHint') }}</p>
        </div>
      </div>

      <!-- AI 分析配置入口 -->
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <div class="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div>
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.settings.aiAnalysisConfig') }}</h4>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.settings.aiAnalysisConfigHint') }}</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              class="btn btn-secondary whitespace-nowrap"
              type="button"
              :disabled="aiAnalysisTesting"
              @click="testAIAnalysisConfig"
            >
              {{ aiAnalysisTesting ? t('admin.ops.settings.aiAnalysisTesting') : t('admin.ops.settings.aiAnalysisTest') }}
            </button>
            <button class="btn btn-primary whitespace-nowrap" type="button" @click="openAIAnalysisConfig">
              {{ t('admin.ops.settings.aiAnalysisOpenConfig') }}
            </button>
          </div>
        </div>

        <div class="mt-4 grid grid-cols-1 gap-3 text-sm md:grid-cols-2">
          <div class="rounded-xl border border-gray-200 bg-white px-3 py-2 dark:border-dark-600 dark:bg-dark-800/70">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.settings.aiAnalysisStatus') }}</div>
            <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ aiAnalysisStatusText }}</div>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white px-3 py-2 dark:border-dark-600 dark:bg-dark-800/70">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.settings.aiAnalysisModel') }}</div>
            <div class="mt-1 font-mono text-xs text-gray-900 dark:text-white">{{ aiAnalysisModelText }}</div>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white px-3 py-2 dark:border-dark-600 dark:bg-dark-800/70">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.settings.aiAnalysisEndpoint') }}</div>
            <div class="mt-1 truncate font-mono text-xs text-gray-900 dark:text-white" :title="aiAnalysisConfig.base_url || '--'">{{ aiAnalysisConfig.base_url || '--' }}</div>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white px-3 py-2 dark:border-dark-600 dark:bg-dark-800/70">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.settings.aiAnalysisAutoLevels') }}</div>
            <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ aiAnalysisAutoLevelsText }}</div>
          </div>
        </div>
      </div>

      <!-- 预警配置 -->
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.settings.alertConfig') }}</h4>

        <div class="mb-3 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-800/40 dark:bg-amber-900/20 dark:text-amber-300">
          邮件通知依赖系统 SMTP 配置（系统设置 → 邮件），未配置 SMTP 时告警邮件无法发出。同时告警规则的通知方式需包含"邮件"才会触发推送。
        </div>

        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <label class="font-medium text-gray-900 dark:text-white">{{ t('admin.ops.settings.enableAlert') }}</label>
            </div>
            <Toggle v-model="emailConfig.alert.enabled" />
          </div>

          <div v-if="emailConfig.alert.enabled">
            <label class="input-label">{{ t('admin.ops.settings.alertRecipients') }}</label>
            <div class="flex gap-2">
              <input
                v-model="alertRecipientInput"
                type="email"
                class="input"
                :placeholder="t('admin.ops.settings.emailPlaceholder')"
                @keydown.enter.prevent="addRecipient('alert')"
              />
              <button class="btn btn-secondary whitespace-nowrap" type="button" @click="addRecipient('alert')">
                {{ t('common.add') }}
              </button>
            </div>
            <div class="mt-2 flex flex-wrap gap-2">
              <span
                v-for="email in emailConfig.alert.recipients"
                :key="email"
                class="inline-flex items-center gap-2 rounded-full bg-blue-100 px-3 py-1 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
              >
                {{ email }}
                <button type="button" class="text-blue-700/80 hover:text-blue-900" @click="removeRecipient('alert', email)">×</button>
              </span>
            </div>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.settings.recipientsHint') }}
            </p>
          </div>

          <div v-if="emailConfig.alert.enabled">
            <label class="input-label">{{ t('admin.ops.settings.minSeverity') }}</label>
            <Select v-model="emailConfig.alert.min_severity" :options="severityOptions" />
          </div>
        </div>
      </div>

      <!-- 评估报告配置 -->
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.settings.reportConfig') }}</h4>

        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <label class="font-medium text-gray-900 dark:text-white">{{ t('admin.ops.settings.enableReport') }}</label>
            </div>
            <Toggle v-model="emailConfig.report.enabled" />
          </div>

          <div v-if="emailConfig.report.enabled">
            <label class="input-label">{{ t('admin.ops.settings.reportRecipients') }}</label>
            <div class="flex gap-2">
              <input
                v-model="reportRecipientInput"
                type="email"
                class="input"
                :placeholder="t('admin.ops.settings.emailPlaceholder')"
                @keydown.enter.prevent="addRecipient('report')"
              />
              <button class="btn btn-secondary whitespace-nowrap" type="button" @click="addRecipient('report')">
                {{ t('common.add') }}
              </button>
            </div>
            <div class="mt-2 flex flex-wrap gap-2">
              <span
                v-for="email in emailConfig.report.recipients"
                :key="email"
                class="inline-flex items-center gap-2 rounded-full bg-blue-100 px-3 py-1 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
              >
                {{ email }}
                <button type="button" class="text-blue-700/80 hover:text-blue-900" @click="removeRecipient('report', email)">×</button>
              </span>
            </div>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.settings.recipientsHint') }}
            </p>
          </div>

          <div v-if="emailConfig.report.enabled" class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.dailySummary') }}</label>
              <Toggle v-model="emailConfig.report.daily_summary_enabled" />
            </div>
            <div v-if="emailConfig.report.daily_summary_enabled">
              <input v-model="emailConfig.report.daily_summary_schedule" type="text" class="input" placeholder="0 9 * * *" />
            </div>
            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.weeklySummary') }}</label>
              <Toggle v-model="emailConfig.report.weekly_summary_enabled" />
            </div>
            <div v-if="emailConfig.report.weekly_summary_enabled">
              <input v-model="emailConfig.report.weekly_summary_schedule" type="text" class="input" placeholder="0 9 * * 1" />
            </div>
          </div>
        </div>
      </div>

      <!-- 新版告警规则配置 -->
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <div class="flex flex-col gap-2 md:flex-row md:items-start md:justify-between">
          <div>
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">新版运维告警规则</h4>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">这里保存的是实际会触发告警和邮件的新规则字段，不再使用旧版指标阈值作为告警依据。</p>
          </div>
          <span class="rounded-full bg-blue-100 px-2 py-1 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">rule_version: v2</span>
        </div>

        <div class="mt-4 rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800/70">
          <div class="flex items-center justify-between gap-3">
            <div>
              <div class="font-medium text-gray-900 dark:text-white">健康分过低告警</div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">健康分低于阈值时触发；若通知方式包含邮件，会发送到上方预警收件人。</p>
            </div>
            <Toggle v-model="healthScoreAlert.enabled" />
          </div>

          <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-4">
            <label class="block text-sm text-gray-700 dark:text-gray-300">
              健康分低于 <span class="text-red-500">*</span>
              <input v-model.number="healthScoreAlert.threshold" type="number" min="0" max="100" step="1" class="input mt-1 w-full" />
            </label>
            <label class="block text-sm text-gray-700 dark:text-gray-300">
              触发级别 <span class="text-red-500">*</span>
              <Select v-model="healthScoreAlert.trigger_level" class="mt-1" :options="triggerLevelOptions" />
            </label>
            <label class="block text-sm text-gray-700 dark:text-gray-300">
              静默分钟 <span class="text-red-500">*</span>
              <input v-model.number="healthScoreAlert.silence_minutes" type="number" min="0" max="1440" step="1" class="input mt-1 w-full" />
            </label>
            <div class="flex items-end justify-between gap-3 rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-600">
              <span class="text-sm text-gray-700 dark:text-gray-300">自动 AI 分析</span>
              <Toggle v-model="healthScoreAlert.auto_ai_analysis" />
            </div>
          </div>

          <div class="mt-4">
            <div class="text-sm font-medium text-gray-700 dark:text-gray-300">通知方式 <span class="text-red-500">*</span></div>
            <div class="mt-2 flex flex-wrap gap-2">
              <label v-for="option in notificationChannelOptions" :key="option.value" class="inline-flex items-center gap-2 rounded-full border border-gray-200 px-3 py-1 text-xs dark:border-dark-600">
                <input
                  type="checkbox"
                  :checked="healthScoreAlert.notification_channels.includes(option.value)"
                  @change="toggleHealthScoreChannel(option.value, ($event.target as HTMLInputElement).checked)"
                />
                {{ option.label }}
              </label>
            </div>
          </div>
        </div>
      </div>

      <!-- 看板显示阈值 -->
      <details class="rounded-2xl bg-gray-50 dark:bg-dark-700/50">
        <summary class="cursor-pointer p-4 text-sm font-semibold text-gray-900 dark:text-white">
          看板显示阈值（不触发邮件）
        </summary>
        <div class="space-y-4 px-4 pb-4">
          <p class="text-xs text-gray-500 dark:text-gray-400">这些阈值只影响运维看板颜色和提示，不参与新版告警规则触发。</p>
          <div>
            <label class="input-label">{{ t('admin.ops.settings.slaMinPercent') }}</label>
            <input v-model.number="metricThresholds.sla_percent_min" type="number" min="0" max="100" step="0.1" class="input" />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.slaMinPercentHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.ops.settings.ttftP99MaxMs') }}</label>
            <input v-model.number="metricThresholds.ttft_p99_ms_max" type="number" min="0" step="50" class="input" />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.ttftP99MaxMsHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.ops.settings.requestErrorRateMaxPercent') }}</label>
            <input v-model.number="metricThresholds.request_error_rate_percent_max" type="number" min="0" max="100" step="0.1" class="input" />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.requestErrorRateMaxPercentHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.ops.settings.upstreamErrorRateMaxPercent') }}</label>
            <input v-model.number="metricThresholds.upstream_error_rate_percent_max" type="number" min="0" max="100" step="0.1" class="input" />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.upstreamErrorRateMaxPercentHint') }}</p>
          </div>
        </div>
      </details>

      <!-- 高级设置 -->
      <details class="rounded-2xl bg-gray-50 dark:bg-dark-700/50">
        <summary class="cursor-pointer p-4 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.ops.settings.advancedSettings') }}
        </summary>
        <div class="space-y-4 px-4 pb-4">
          <!-- 数据保留策略 -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.dataRetention') }}</h5>

            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.enableCleanup') }}</label>
              <Toggle v-model="advancedSettings.data_retention.cleanup_enabled" />
            </div>

            <div v-if="advancedSettings.data_retention.cleanup_enabled">
              <label class="input-label">{{ t('admin.ops.settings.cleanupSchedule') }}</label>
              <input
                v-model="advancedSettings.data_retention.cleanup_schedule"
                type="text"
                class="input"
                placeholder="0 2 * * *"
              />
              <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.cleanupScheduleHint') }}</p>
            </div>

            <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
              <div>
                <label class="input-label">{{ t('admin.ops.settings.errorLogRetentionDays') }}</label>
                <input
                  v-model.number="advancedSettings.data_retention.error_log_retention_days"
                  type="number"
                  min="0"
                  max="365"
                  class="input"
                />
              </div>
              <div>
                <label class="input-label">{{ t('admin.ops.settings.minuteMetricsRetentionDays') }}</label>
                <input
                  v-model.number="advancedSettings.data_retention.minute_metrics_retention_days"
                  type="number"
                  min="0"
                  max="365"
                  class="input"
                />
              </div>
              <div>
                <label class="input-label">{{ t('admin.ops.settings.hourlyMetricsRetentionDays') }}</label>
                <input
                  v-model.number="advancedSettings.data_retention.hourly_metrics_retention_days"
                  type="number"
                  min="0"
                  max="365"
                  class="input"
                />
              </div>
            </div>
            <p class="text-xs text-gray-500">{{ t('admin.ops.settings.retentionDaysHint') }}</p>
          </div>

          <!-- 预聚合任务 -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.aggregation') }}</h5>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.enableAggregation') }}</label>
                <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.aggregationHint') }}</p>
              </div>
              <Toggle v-model="advancedSettings.aggregation.aggregation_enabled" />
            </div>
          </div>

          <!-- Error Filtering -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.errorFiltering') }}</h5>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.ignoreCountTokensErrors') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreCountTokensErrorsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_count_tokens_errors" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.ignoreContextCanceled') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreContextCanceledHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_context_canceled" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.ignoreNoAvailableAccounts') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreNoAvailableAccountsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_no_available_accounts" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.ignoreInvalidApiKeyErrors') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreInvalidApiKeyErrorsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_invalid_api_key_errors" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.ignoreInsufficientBalanceErrors') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreInsufficientBalanceErrorsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_insufficient_balance_errors" />
            </div>
          </div>

          <!-- Auto Refresh -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.autoRefresh') }}</h5>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.enableAutoRefresh') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.enableAutoRefreshHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.auto_refresh_enabled" />
            </div>

            <div v-if="advancedSettings.auto_refresh_enabled">
              <label class="input-label">{{ t('admin.ops.settings.refreshInterval') }}</label>
              <Select
                v-model="advancedSettings.auto_refresh_interval_seconds"
                :options="[
                  { value: 15, label: t('admin.ops.settings.refreshInterval15s') },
                  { value: 30, label: t('admin.ops.settings.refreshInterval30s') },
                  { value: 60, label: t('admin.ops.settings.refreshInterval60s') }
                ]"
              />
            </div>
          </div>

          <!-- Dashboard Cards -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.dashboardCards') }}</h5>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.displayAlertEvents') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.displayAlertEventsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.display_alert_events" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.displayOpenAITokenStats') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.displayOpenAITokenStatsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.display_openai_token_stats" />
            </div>
          </div>
        </div>
      </details>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving || !validation.valid" @click="saveAllSettings">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>
