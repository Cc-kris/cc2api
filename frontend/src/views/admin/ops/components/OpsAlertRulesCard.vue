<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import { adminAPI } from '@/api'
import { opsAPI } from '@/api/admin/ops'
import type { AlertRule, EmailNotificationConfig } from '../types'
import type { OpsSeverity } from '@/api/admin/ops'
import { formatDateTime } from '../utils/opsFormatters'

const { t } = useI18n()
const appStore = useAppStore()

const ALL_ERROR_CATEGORIES = [
  'client',
  'platform',
  'upstream',
  'account_pool',
  'rate_limit',
  'permission',
  'balance',
  'config',
  'slow_request',
  'unknown'
] as const

type AlertTriggerLevel = 'P0' | 'P1' | 'P2' | 'observe'
type ImpactScopeKey = 'affected_users' | 'affected_api_keys' | 'affected_groups' | 'affected_models' | 'affected_upstream_accounts'
type RecoveredPolicy = 'record_only' | 'observe_only' | 'alert'
type NotificationChannel = 'in_app' | 'email' | 'none'
type AlertRuleMetricType = 'compound_rule' | 'health_score' | 'final_failure_rate' | 'final_failures'
type SortKey = 'updated_at' | 'name' | 'trigger_level' | 'min_final_failures' | 'min_failure_rate' | 'min_sample_count'

const DEFAULT_RULE_BY_LEVEL: Record<AlertTriggerLevel, {
  min_final_failures: number
  min_failure_rate: number
  min_sample_count: number
  auto_ai_analysis: boolean
  notification_channels: NotificationChannel[]
}> = {
  P0: {
    min_final_failures: 20,
    min_failure_rate: 20,
    min_sample_count: 50,
    auto_ai_analysis: true,
    notification_channels: ['in_app', 'email']
  },
  P1: {
    min_final_failures: 5,
    min_failure_rate: 10,
    min_sample_count: 50,
    auto_ai_analysis: true,
    notification_channels: ['in_app', 'email']
  },
  P2: {
    min_final_failures: 1,
    min_failure_rate: 0,
    min_sample_count: 50,
    auto_ai_analysis: false,
    notification_channels: ['in_app']
  },
  observe: {
    min_final_failures: 1,
    min_failure_rate: 0,
    min_sample_count: 50,
    auto_ai_analysis: false,
    notification_channels: ['in_app']
  }
}

const severityRank: Record<string, number> = {
  P0: 0,
  P1: 1,
  P2: 2,
  observe: 3
}

const alertMetricTypes: AlertRuleMetricType[] = ['compound_rule', 'health_score', 'final_failure_rate', 'final_failures']

function normalizeAlertMetricType(value?: string | null): AlertRuleMetricType {
  const raw = String(value || '').trim()
  return alertMetricTypes.includes(raw as AlertRuleMetricType) ? (raw as AlertRuleMetricType) : 'compound_rule'
}

function defaultHealthScoreThreshold(level: AlertTriggerLevel): number {
  if (level === 'P0') return 50
  if (level === 'P1') return 70
  return 90
}

function metricOperator(metricType: AlertRuleMetricType) {
  return metricType === 'health_score' ? '<' : '>='
}

const loading = ref(false)
const rules = ref<AlertRule[]>([])
const emailConfig = ref<EmailNotificationConfig | null>(null)
const showEditor = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const draft = ref<AlertRule | null>(null)
const showDeleteConfirm = ref(false)
const pendingDelete = ref<AlertRule | null>(null)
const keywordFilter = ref('')
const severityFilter = ref<'' | AlertTriggerLevel>('')
const enabledFilter = ref<'all' | 'enabled' | 'disabled'>('all')
const sortKey = ref<SortKey>('updated_at')
const sortDirection = ref<'asc' | 'desc'>('desc')
const groupOptionsBase = ref<SelectOption[]>([])
const syncingDraft = ref(false)

function parsePositiveInt(value: unknown): number | null {
  if (value == null || value === '') return null
  if (typeof value === 'boolean') return null
  const n = typeof value === 'number' ? value : Number.parseInt(String(value), 10)
  return Number.isInteger(n) && n > 0 ? n : null
}

function parseNonNegativeInt(value: unknown): number | null {
  if (value == null || value === '') return null
  if (typeof value === 'boolean') return null
  const n = typeof value === 'number' ? value : Number.parseInt(String(value), 10)
  return Number.isInteger(n) && n >= 0 ? n : null
}

function hasAtMostTwoDecimals(value: number): boolean {
  return Math.round(value * 100) === value * 100
}

function normalizeTriggerLevel(value?: string | null): AlertTriggerLevel {
  const raw = String(value || '').trim()
  if (!raw) return 'P2'
  if (raw === '观察' || /^p3$/i.test(raw) || /^observe$/i.test(raw)) return 'observe'
  const upper = raw.toUpperCase()
  if (upper === 'P0' || upper === 'P1' || upper === 'P2') return upper
  return 'P2'
}

function normalizeNotificationChannels(
  channels?: string[] | null,
  notifyEmail?: boolean,
  fallback: NotificationChannel[] = ['in_app']
): NotificationChannel[] {
  const normalized = Array.isArray(channels)
    ? channels
        .map((item) => String(item || '').trim())
        .filter((item): item is NotificationChannel => item === 'in_app' || item === 'email' || item === 'none')
    : []

  const deduped = Array.from(new Set(normalized))
  if (deduped.includes('none')) return ['none']
  if (deduped.length > 0) return deduped
  if (notifyEmail) return ['in_app', 'email']
  return [...fallback]
}

function sanitizeImpactScope(scope?: Record<string, number> | null): Record<string, number> {
  const next: Record<string, number> = {}
  if (!scope || typeof scope !== 'object') return next
  ;(['affected_users', 'affected_api_keys', 'affected_groups', 'affected_models', 'affected_upstream_accounts'] as ImpactScopeKey[]).forEach((key) => {
    const value = parsePositiveInt(scope[key])
    if (value != null) next[key] = value
  })
  return next
}

function getRuleLevel(rule: AlertRule): AlertTriggerLevel {
  return normalizeTriggerLevel(rule.trigger_level || rule.severity)
}

function getLevelDefaults(level: AlertTriggerLevel) {
  return DEFAULT_RULE_BY_LEVEL[level]
}

function normalizeDraft(source?: AlertRule | null): AlertRule {
  const level = normalizeTriggerLevel(source?.trigger_level || source?.severity)
  const metricType = normalizeAlertMetricType(source?.metric_type)
  const defaults = getLevelDefaults(level)
  const notificationChannels = normalizeNotificationChannels(source?.notification_channels, source?.notify_email, defaults.notification_channels)
  const minFinalFailures = parsePositiveInt(source?.min_final_failures) ?? defaults.min_final_failures
  const minSampleCount = parsePositiveInt(source?.min_sample_count) ?? defaults.min_sample_count
  const minRecovered = parsePositiveInt(source?.min_recovered_fluctuations) ?? 10
  const defaultThreshold = metricType === 'health_score' ? defaultHealthScoreThreshold(level) : minFinalFailures

  return {
    ...(source ? JSON.parse(JSON.stringify(source)) : {}),
    name: source?.name ?? '',
    description: source?.description ?? '',
    enabled: source?.enabled ?? true,
    metric_type: metricType,
    operator: metricOperator(metricType),
    threshold: typeof source?.threshold === 'number' && Number.isFinite(source.threshold) && source.threshold > 0 ? source.threshold : defaultThreshold,
    window_minutes: 1,
    sustained_minutes: parsePositiveInt(source?.sustained_minutes) ?? 1,
    severity: level === 'observe' ? 'P3' : (level as OpsSeverity),
    cooldown_minutes: parseNonNegativeInt(source?.cooldown_minutes) ?? (parseNonNegativeInt(source?.silence_minutes) ?? 10),
    notify_email: notificationChannels.includes('email'),
    filters: source?.filters ? JSON.parse(JSON.stringify(source.filters)) : undefined,
    rule_version: 'v2',
    error_categories: Array.isArray(source?.error_categories) && source!.error_categories!.length > 0
      ? Array.from(new Set(source!.error_categories!.map((item) => String(item).trim()).filter(Boolean)))
      : (metricType === 'compound_rule' ? [...ALL_ERROR_CATEGORIES] : []),
    trigger_level: level,
    min_final_failures: minFinalFailures,
    min_failure_rate: typeof source?.min_failure_rate === 'number' && Number.isFinite(source.min_failure_rate)
      ? source.min_failure_rate
      : defaults.min_failure_rate,
    min_sample_count: minSampleCount,
    impact_scope: sanitizeImpactScope(source?.impact_scope),
    recovered_fluctuation_policy: (source?.recovered_fluctuation_policy as RecoveredPolicy) || 'record_only',
    min_recovered_fluctuations: minRecovered,
    auto_ai_analysis: typeof source?.auto_ai_analysis === 'boolean' ? source.auto_ai_analysis : defaults.auto_ai_analysis,
    notification_channels: notificationChannels,
    silence_minutes: parseNonNegativeInt(source?.silence_minutes) ?? (parseNonNegativeInt(source?.cooldown_minutes) ?? 10),
    migration_state: source?.migration_state ?? '',
    last_triggered_at: source?.last_triggered_at ?? null,
    created_at: source?.created_at,
    updated_at: source?.updated_at,
    id: source?.id
  }
}

function buildPayload(rule: AlertRule): AlertRule {
  const level = normalizeTriggerLevel(rule.trigger_level)
  const metricType = normalizeAlertMetricType(rule.metric_type)
  const defaults = getLevelDefaults(level)
  const notificationChannels = normalizeNotificationChannels(rule.notification_channels, rule.notify_email, defaults.notification_channels)
  const impactScope = sanitizeImpactScope(rule.impact_scope)
  const minFinalFailures = parsePositiveInt(rule.min_final_failures) ?? defaults.min_final_failures
  const minFailureRate = typeof rule.min_failure_rate === 'number' && Number.isFinite(rule.min_failure_rate) ? rule.min_failure_rate : defaults.min_failure_rate
  const minSampleCount = parsePositiveInt(rule.min_sample_count) ?? defaults.min_sample_count
  const threshold = metricType === 'health_score'
    ? (typeof rule.threshold === 'number' && Number.isFinite(rule.threshold) ? rule.threshold : defaultHealthScoreThreshold(level))
    : metricType === 'final_failure_rate'
      ? minFailureRate
      : minFinalFailures
  const minRecovered = level && rule.recovered_fluctuation_policy !== 'record_only'
    ? (parsePositiveInt(rule.min_recovered_fluctuations) ?? 0)
    : 0

  return {
    ...rule,
    metric_type: metricType,
    operator: metricOperator(metricType),
    threshold,
    window_minutes: 1,
    sustained_minutes: 1,
    severity: level === 'observe' ? 'P3' : (level as OpsSeverity),
    cooldown_minutes: parseNonNegativeInt(rule.silence_minutes) ?? 10,
    notify_email: notificationChannels.includes('email'),
    rule_version: 'v2',
    error_categories: metricType === 'compound_rule' ? Array.from(new Set((rule.error_categories || []).map((item) => String(item).trim()).filter(Boolean))) : [],
    trigger_level: level,
    min_final_failures: minFinalFailures,
    min_failure_rate: minFailureRate,
    min_sample_count: minSampleCount,
    impact_scope: impactScope,
    recovered_fluctuation_policy: (rule.recovered_fluctuation_policy as RecoveredPolicy) || 'record_only',
    min_recovered_fluctuations: minRecovered,
    auto_ai_analysis: Boolean(rule.auto_ai_analysis),
    notification_channels: notificationChannels,
    silence_minutes: parseNonNegativeInt(rule.silence_minutes) ?? 10,
    description: String(rule.description || '').trim()
  }
}

async function load() {
  loading.value = true
  try {
    rules.value = await opsAPI.listAlertRules()
  } catch (err: any) {
    console.error('[OpsAlertRulesCard] Failed to load rules', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertRules.loadFailed'))
    rules.value = []
  } finally {
    loading.value = false
  }
}

async function loadGroups() {
  try {
    const list = await adminAPI.groups.getAll()
    groupOptionsBase.value = list.map((g) => ({ value: g.id, label: g.name }))
  } catch (err) {
    console.error('[OpsAlertRulesCard] Failed to load groups', err)
    groupOptionsBase.value = []
  }
}

async function loadEmailConfig() {
  try {
    emailConfig.value = await opsAPI.getEmailNotificationConfig()
  } catch (err) {
    console.error('[OpsAlertRulesCard] Failed to load email config', err)
    emailConfig.value = null
  }
}

onMounted(() => {
  load()
  loadGroups()
  loadEmailConfig()
})

const emailRecipients = computed(() => emailConfig.value?.alert?.recipients?.filter((item) => String(item || '').trim()) ?? [])
const hasEmailRecipients = computed(() => emailRecipients.value.length > 0)
const isReadOnlyLegacy = computed(() => draft.value?.migration_state === 'readonly_legacy')

const severityOptions = computed<SelectOption[]>(() => [
  { value: 'P0', label: t('admin.ops.alertRules.triggerLevels.P0') },
  { value: 'P1', label: t('admin.ops.alertRules.triggerLevels.P1') },
  { value: 'P2', label: t('admin.ops.alertRules.triggerLevels.P2') },
  { value: 'observe', label: t('admin.ops.alertRules.triggerLevels.observe') }
])

const metricTypeOptions = computed<SelectOption[]>(() => alertMetricTypes.map((value) => ({
  value,
  label: t(`admin.ops.alertRules.metrics.${value}`)
})))

const filterSeverityOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.ops.alertRules.filters.allSeverities') },
  ...severityOptions.value
])

const enabledOptions = computed<SelectOption[]>(() => [
  { value: 'all', label: t('admin.ops.alertRules.filters.allStatuses') },
  { value: 'enabled', label: t('common.enabled') },
  { value: 'disabled', label: t('common.disabled') }
])

const groupOptions = computed<SelectOption[]>(() => [{ value: null, label: t('admin.ops.alertRules.form.allGroups') }, ...groupOptionsBase.value])

const errorCategoryOptions = computed(() =>
  ALL_ERROR_CATEGORIES.map((value) => ({
    value,
    label: t(`admin.ops.alertRules.categories.${value}`)
  }))
)

const recoveredPolicyOptions = computed<SelectOption[]>(() => [
  { value: 'record_only', label: t('admin.ops.alertRules.recoveredPolicies.record_only') },
  { value: 'observe_only', label: t('admin.ops.alertRules.recoveredPolicies.observe_only') },
  { value: 'alert', label: t('admin.ops.alertRules.recoveredPolicies.alert') }
])

const notificationChannelOptions = computed(() => [
  { value: 'in_app', label: t('admin.ops.alertRules.channels.in_app') },
  { value: 'email', label: t('admin.ops.alertRules.channels.email') },
  { value: 'none', label: t('admin.ops.alertRules.channels.none') }
] satisfies Array<{ value: NotificationChannel; label: string }>)

const impactScopeFields = computed(() => [
  { key: 'affected_users' as const, label: t('admin.ops.alertRules.scope.affectedUsers') },
  { key: 'affected_api_keys' as const, label: t('admin.ops.alertRules.scope.affectedApiKeys') },
  { key: 'affected_groups' as const, label: t('admin.ops.alertRules.scope.affectedGroups') },
  { key: 'affected_models' as const, label: t('admin.ops.alertRules.scope.affectedModels') },
  { key: 'affected_upstream_accounts' as const, label: t('admin.ops.alertRules.scope.affectedUpstreamAccounts') }
])

function formatTriggerLevel(level: string): string {
  const normalized = normalizeTriggerLevel(level)
  return t(`admin.ops.alertRules.triggerLevels.${normalized}`)
}

function formatPercent(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  return `${value % 1 === 0 ? value.toFixed(0) : value.toFixed(2).replace(/\.0+$/, '').replace(/(\.\d*?)0+$/, '$1')}%`
}

function formatCategories(rule: AlertRule): string[] {
  if (normalizeAlertMetricType(rule.metric_type) !== 'compound_rule') return []
  const categories = Array.isArray(rule.error_categories) ? rule.error_categories : []
  return categories.map((category) => t(`admin.ops.alertRules.categories.${category}`))
}

function formatMetricType(rule: AlertRule): string {
  return t(`admin.ops.alertRules.metrics.${normalizeAlertMetricType(rule.metric_type)}`)
}

function getNotificationChannels(rule: AlertRule): NotificationChannel[] {
  return normalizeNotificationChannels(rule.notification_channels, rule.notify_email)
}

function formatImpactScopeSummary(rule: AlertRule): string {
  const impact = sanitizeImpactScope(rule.impact_scope)
  const items = impactScopeFields.value.flatMap(({ key, label }) => {
    const value = impact[key]
    return typeof value === 'number' && value > 0 ? [`${label} ≥ ${value}`] : []
  })
  return items.length > 0 ? items.join(' · ') : t('admin.ops.alertRules.values.notLimited')
}

function formatRuleCondition(rule: AlertRule): string {
  const metricType = normalizeAlertMetricType(rule.metric_type)
  if (metricType === 'health_score') {
    return `${t('admin.ops.alertRules.metrics.health_score')} < ${rule.threshold || 0}`
  }
  if (metricType === 'final_failures') {
    return `${t('admin.ops.alertRules.metrics.final_failures')} ≥ ${rule.min_final_failures || rule.threshold || 0}`
  }
  if (metricType === 'final_failure_rate') {
    return [
      `${t('admin.ops.alertRules.metrics.final_failure_rate')} ≥ ${formatPercent(rule.min_failure_rate || rule.threshold)}`,
      `${t('admin.ops.alertRules.table.minFinalFailures')} ≥ ${rule.min_final_failures || 0}`,
      `${t('admin.ops.alertRules.table.minSampleCount')} ≥ ${rule.min_sample_count || 0}`
    ].join(' · ')
  }
  return [
    `${t('admin.ops.alertRules.table.minFinalFailures')} ≥ ${rule.min_final_failures || 0}`,
    `${t('admin.ops.alertRules.table.minFailureRate')} ≥ ${formatPercent(rule.min_failure_rate)}`,
    `${t('admin.ops.alertRules.table.minSampleCount')} ≥ ${rule.min_sample_count || 0}`
  ].join(' · ')
}

function compareRuleValues(a: AlertRule, b: AlertRule, key: SortKey): number {
  switch (key) {
    case 'name':
      return a.name.localeCompare(b.name, 'zh-Hans-CN')
    case 'trigger_level':
      return (severityRank[getRuleLevel(a)] ?? 99) - (severityRank[getRuleLevel(b)] ?? 99)
    case 'min_final_failures':
      return (a.min_final_failures || 0) - (b.min_final_failures || 0)
    case 'min_failure_rate':
      return (a.min_failure_rate || 0) - (b.min_failure_rate || 0)
    case 'min_sample_count':
      return (a.min_sample_count || 0) - (b.min_sample_count || 0)
    case 'updated_at':
    default:
      return new Date(a.updated_at || a.created_at || 0).getTime() - new Date(b.updated_at || b.created_at || 0).getTime()
  }
}

function toggleSort(nextKey: SortKey) {
  if (sortKey.value === nextKey) {
    sortDirection.value = sortDirection.value === 'asc' ? 'desc' : 'asc'
    return
  }
  sortKey.value = nextKey
  sortDirection.value = nextKey === 'name' ? 'asc' : 'desc'
}

const filteredRules = computed(() => {
  const keyword = keywordFilter.value.trim().toLowerCase()
  return rules.value.filter((rule) => {
    if (severityFilter.value && getRuleLevel(rule) !== severityFilter.value) return false
    if (enabledFilter.value === 'enabled' && !rule.enabled) return false
    if (enabledFilter.value === 'disabled' && rule.enabled) return false
    if (!keyword) return true

    const haystack = [
      rule.name,
      rule.description,
      rule.trigger_level,
      rule.severity,
      formatMetricType(rule),
      ...(rule.error_categories || [])
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
    return haystack.includes(keyword)
  })
})

const displayedRules = computed(() => {
  const sorted = [...filteredRules.value].sort((a, b) => compareRuleValues(a, b, sortKey.value))
  return sortDirection.value === 'asc' ? sorted : sorted.reverse()
})

const hasActiveFilters = computed(() => Boolean(keywordFilter.value.trim()) || Boolean(severityFilter.value) || enabledFilter.value !== 'all')

function resetFilters() {
  keywordFilter.value = ''
  severityFilter.value = ''
  enabledFilter.value = 'all'
  sortKey.value = 'updated_at'
  sortDirection.value = 'desc'
}

function applyLevelDefaults(level: AlertTriggerLevel) {
  if (!draft.value) return
  const defaults = getLevelDefaults(level)
  draft.value.trigger_level = level
  draft.value.severity = level === 'observe' ? 'P3' : (level as OpsSeverity)
  draft.value.min_final_failures = defaults.min_final_failures
  draft.value.min_failure_rate = defaults.min_failure_rate
  draft.value.min_sample_count = defaults.min_sample_count
  draft.value.threshold = normalizeAlertMetricType(draft.value.metric_type) === 'health_score' ? defaultHealthScoreThreshold(level) : defaults.min_final_failures
  draft.value.auto_ai_analysis = defaults.auto_ai_analysis
  draft.value.notification_channels = [...defaults.notification_channels]
  draft.value.notify_email = defaults.notification_channels.includes('email')
}

watch(
  () => draft.value?.trigger_level,
  (next, prev) => {
    if (!draft.value || syncingDraft.value || !next || !prev || next === prev) return
    applyLevelDefaults(normalizeTriggerLevel(next))
  }
)

watch(
  () => draft.value?.metric_type,
  (next, prev) => {
    if (!draft.value || syncingDraft.value || !next || next === prev) return
    const metricType = normalizeAlertMetricType(next)
    draft.value.operator = metricOperator(metricType)
    draft.value.error_categories = metricType === 'compound_rule' ? [...ALL_ERROR_CATEGORIES] : []
    draft.value.threshold = metricType === 'health_score'
      ? defaultHealthScoreThreshold(normalizeTriggerLevel(draft.value.trigger_level))
      : (parsePositiveInt(draft.value.min_final_failures) ?? getLevelDefaults(normalizeTriggerLevel(draft.value.trigger_level)).min_final_failures)
  }
)

const draftGroupId = computed<number | null>({
  get() {
    return parsePositiveInt(draft.value?.filters?.group_id)
  },
  set(value) {
    if (!draft.value) return
    if (value == null) {
      if (!draft.value.filters) return
      delete draft.value.filters.group_id
      if (Object.keys(draft.value.filters).length === 0) delete draft.value.filters
      return
    }
    if (!draft.value.filters) draft.value.filters = {}
    draft.value.filters.group_id = value
  }
})

function openCreate() {
  editingId.value = null
  syncingDraft.value = true
  draft.value = normalizeDraft(null)
  showEditor.value = true
  syncingDraft.value = false
}

function openEdit(rule: AlertRule) {
  editingId.value = rule.id ?? null
  syncingDraft.value = true
  draft.value = normalizeDraft(rule)
  showEditor.value = true
  syncingDraft.value = false
}

function closeEditor() {
  showEditor.value = false
  draft.value = null
  editingId.value = null
}

function toggleNotificationChannel(channel: NotificationChannel, checked: boolean) {
  if (!draft.value) return
  const current = new Set(normalizeNotificationChannels(draft.value.notification_channels, draft.value.notify_email))
  if (checked) {
    if (channel === 'none') {
      draft.value.notification_channels = ['none']
      draft.value.notify_email = false
      return
    }
    current.delete('none')
    current.add(channel)
  } else {
    current.delete(channel)
  }
  draft.value.notification_channels = Array.from(current)
  draft.value.notify_email = draft.value.notification_channels.includes('email')
}

function onImpactScopeInput(key: ImpactScopeKey, rawValue: string) {
  if (!draft.value) return
  const parsed = parsePositiveInt(rawValue)
  const next = { ...sanitizeImpactScope(draft.value.impact_scope) }
  if (parsed == null) {
    delete next[key]
  } else {
    next[key] = parsed
  }
  draft.value.impact_scope = next
}

const editorValidation = computed(() => {
  const errors: string[] = []
  const rule = draft.value
  if (!rule) return { valid: true, errors }

  const name = String(rule.name || '').trim()
  if (!name) {
    errors.push(t('admin.ops.alertRules.validation.nameRequired'))
  } else if (name.length < 2 || name.length > 50) {
    errors.push(t('admin.ops.alertRules.validation.nameLength'))
  } else {
    const duplicated = rules.value.some((item) => item.id !== editingId.value && item.name.trim() === name)
    if (duplicated) errors.push(t('admin.ops.alertRules.validation.nameDuplicate'))
  }

  const metricType = normalizeAlertMetricType(rule.metric_type)
  if (metricType === 'compound_rule' && (!Array.isArray(rule.error_categories) || rule.error_categories.length === 0)) {
    errors.push(t('admin.ops.alertRules.validation.categoriesRequired'))
  }

  const minFinalFailures = parsePositiveInt(rule.min_final_failures)
  const minFailureRate = typeof rule.min_failure_rate === 'number' && Number.isFinite(rule.min_failure_rate) ? rule.min_failure_rate : null
  const minSampleCount = parsePositiveInt(rule.min_sample_count)

  if (metricType === 'health_score') {
    const threshold = typeof rule.threshold === 'number' && Number.isFinite(rule.threshold) ? rule.threshold : null
    if (threshold == null || threshold <= 0 || threshold > 100) {
      errors.push(t('admin.ops.alertRules.validation.healthScoreThresholdRange'))
    }
  } else {
    if (minFinalFailures == null || minFinalFailures < 1 || minFinalFailures > 100000) {
      errors.push(t('admin.ops.alertRules.validation.minFinalFailuresRange'))
    }
    if ((metricType === 'compound_rule' || metricType === 'final_failure_rate') && (minFailureRate == null || minFailureRate < 0 || minFailureRate > 100 || !hasAtMostTwoDecimals(minFailureRate))) {
      errors.push(t('admin.ops.alertRules.validation.minFailureRateRange'))
    }
    if ((metricType === 'compound_rule' || metricType === 'final_failure_rate') && (minSampleCount == null || minSampleCount < 1 || minSampleCount > 1000000)) {
      errors.push(t('admin.ops.alertRules.validation.minSampleCountRange'))
    }
    if ((metricType === 'compound_rule' || metricType === 'final_failure_rate') && minFailureRate != null && minFailureRate > 0) {
      if (minFinalFailures == null) errors.push(t('admin.ops.alertRules.validation.minFinalFailuresRequiredForRate'))
      if (minSampleCount == null) errors.push(t('admin.ops.alertRules.validation.minSampleCountRequiredForRate'))
      if (minFinalFailures != null && minSampleCount != null && minFinalFailures > minSampleCount) {
        errors.push(t('admin.ops.alertRules.validation.minFinalFailuresGtSample'))
      }
    }
  }

  for (const value of Object.values(sanitizeImpactScope(rule.impact_scope))) {
    if (value < 1 || value > 100000) {
      errors.push(t('admin.ops.alertRules.validation.impactScopeRange'))
      break
    }
  }

  const recoveredPolicy = (rule.recovered_fluctuation_policy as RecoveredPolicy) || 'record_only'
  if (recoveredPolicy !== 'record_only') {
    const minRecovered = parsePositiveInt(rule.min_recovered_fluctuations)
    if (minRecovered == null || minRecovered < 1 || minRecovered > 100000) {
      errors.push(t('admin.ops.alertRules.validation.minRecoveredRequired'))
    }
  }

  const channels = normalizeNotificationChannels(rule.notification_channels, rule.notify_email)
  if (channels.includes('none') && channels.length > 1) {
    errors.push(t('admin.ops.alertRules.validation.notificationNoneExclusive'))
  }
  if (channels.includes('email') && !hasEmailRecipients.value) {
    errors.push(t('admin.ops.alertRules.validation.emailRecipientsRequired'))
  }

  const silenceMinutes = parseNonNegativeInt(rule.silence_minutes)
  if (silenceMinutes == null || silenceMinutes < 0 || silenceMinutes > 1440) {
    errors.push(t('admin.ops.alertRules.validation.silenceMinutesRange'))
  }

  if (String(rule.description || '').trim().length > 500) {
    errors.push(t('admin.ops.alertRules.validation.descriptionLength'))
  }

  return { valid: errors.length === 0, errors }
})

async function save() {
  if (!draft.value || isReadOnlyLegacy.value) return
  if (!editorValidation.value.valid) {
    appStore.showError(editorValidation.value.errors[0] || t('admin.ops.alertRules.validation.invalid'))
    return
  }

  const payload = buildPayload(draft.value)
  saving.value = true
  try {
    if (editingId.value) {
      await opsAPI.updateAlertRule(editingId.value, payload)
    } else {
      await opsAPI.createAlertRule(payload)
    }
    closeEditor()
    await load()
    appStore.showSuccess(t('admin.ops.alertRules.saveSuccess'))
  } catch (err: any) {
    console.error('[OpsAlertRulesCard] Failed to save rule', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertRules.saveFailed'))
  } finally {
    saving.value = false
  }
}

function requestDelete(rule: AlertRule) {
  pendingDelete.value = rule
  showDeleteConfirm.value = true
}

async function confirmDelete() {
  if (!pendingDelete.value?.id) return
  try {
    await opsAPI.deleteAlertRule(pendingDelete.value.id)
    showDeleteConfirm.value = false
    pendingDelete.value = null
    await load()
    appStore.showSuccess(t('admin.ops.alertRules.deleteSuccess'))
  } catch (err: any) {
    console.error('[OpsAlertRulesCard] Failed to delete rule', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertRules.deleteFailed'))
  }
}

function cancelDelete() {
  showDeleteConfirm.value = false
  pendingDelete.value = null
}
</script>

<template>
  <div class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
    <div class="mb-4 flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
      <div>
        <h3 class="text-sm font-bold text-gray-900 dark:text-white">{{ t('admin.ops.alertRules.title') }}</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.alertRules.description') }}</p>
      </div>

      <div class="flex flex-wrap items-center gap-2 lg:justify-end">
        <button class="btn btn-sm btn-primary" :disabled="loading" @click="openCreate">
          {{ t('admin.ops.alertRules.create') }}
        </button>
        <button
          class="flex items-center gap-1.5 rounded-lg bg-gray-100 px-3 py-1.5 text-xs font-bold text-gray-700 transition-colors hover:bg-gray-200 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
          :disabled="loading"
          @click="load"
        >
          <svg class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          {{ t('common.refresh') }}
        </button>
      </div>
    </div>

    <div class="mb-4 grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,1.5fr)_220px_220px_auto]">
      <div class="relative">
        <input
          v-model="keywordFilter"
          type="text"
          :placeholder="t('admin.ops.alertRules.filters.keywordPlaceholder')"
          class="input pr-10"
        />
        <button
          v-if="keywordFilter"
          type="button"
          class="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-gray-400 transition hover:text-gray-600 dark:hover:text-gray-200"
          @click="keywordFilter = ''"
        >
          ×
        </button>
      </div>
      <Select v-model="severityFilter" :options="filterSeverityOptions" />
      <Select v-model="enabledFilter" :options="enabledOptions" />
      <button class="btn btn-sm btn-secondary xl:justify-self-end" :disabled="!hasActiveFilters" @click="resetFilters">
        {{ t('admin.ops.alertRules.filters.reset') }}
      </button>
    </div>

    <div v-if="loading" class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.ops.alertRules.loading') }}
    </div>

    <div v-else-if="rules.length === 0" class="rounded-xl border border-dashed border-gray-200 p-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
      {{ t('admin.ops.alertRules.empty') }}
    </div>

    <div v-else-if="displayedRules.length === 0" class="rounded-xl border border-dashed border-gray-200 p-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
      {{ t('admin.ops.alertRules.emptyFiltered') }}
    </div>

    <div v-else class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
      <div class="max-h-[560px] overflow-auto">
        <table class="min-w-[1560px] divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="sticky top-0 z-10 bg-gray-50 dark:bg-dark-900">
            <tr>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                <button class="inline-flex items-center gap-1 hover:text-gray-700 dark:hover:text-gray-200" @click="toggleSort('name')">
                  {{ t('admin.ops.alertRules.table.name') }}
                  <span v-if="sortKey === 'name'">{{ sortDirection === 'asc' ? '↑' : '↓' }}</span>
                </button>
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertRules.table.status') }}
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertRules.table.window') }}
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertRules.table.metric') }}
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertRules.table.categories') }}
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                <button class="inline-flex items-center gap-1 hover:text-gray-700 dark:hover:text-gray-200" @click="toggleSort('trigger_level')">
                  {{ t('admin.ops.alertRules.table.triggerLevel') }}
                  <span v-if="sortKey === 'trigger_level'">{{ sortDirection === 'asc' ? '↑' : '↓' }}</span>
                </button>
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                <button class="inline-flex items-center gap-1 hover:text-gray-700 dark:hover:text-gray-200" @click="toggleSort('min_final_failures')">
                  {{ t('admin.ops.alertRules.table.minFinalFailures') }}
                  <span v-if="sortKey === 'min_final_failures'">{{ sortDirection === 'asc' ? '↑' : '↓' }}</span>
                </button>
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                <button class="inline-flex items-center gap-1 hover:text-gray-700 dark:hover:text-gray-200" @click="toggleSort('min_failure_rate')">
                  {{ t('admin.ops.alertRules.table.minFailureRate') }}
                  <span v-if="sortKey === 'min_failure_rate'">{{ sortDirection === 'asc' ? '↑' : '↓' }}</span>
                </button>
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                <button class="inline-flex items-center gap-1 hover:text-gray-700 dark:hover:text-gray-200" @click="toggleSort('min_sample_count')">
                  {{ t('admin.ops.alertRules.table.minSampleCount') }}
                  <span v-if="sortKey === 'min_sample_count'">{{ sortDirection === 'asc' ? '↑' : '↓' }}</span>
                </button>
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertRules.table.impactScope') }}
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertRules.table.autoAIAnalysis') }}
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertRules.table.notificationChannels') }}
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertRules.table.silenceMinutes') }}
              </th>
              <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertRules.table.condition') }}
              </th>
              <th class="px-4 py-3 text-right text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.alertRules.table.actions') }}
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
            <tr v-for="row in displayedRules" :key="row.id" class="align-top hover:bg-gray-50 dark:hover:bg-dark-700/50">
              <td class="px-4 py-3">
                <button class="text-left" @click="openEdit(row)">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="text-xs font-bold text-primary-700 hover:underline dark:text-primary-300">{{ row.name }}</span>
                    <span
                      v-if="row.migration_state === 'readonly_legacy'"
                      class="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-bold text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
                    >
                      {{ t('admin.ops.alertRules.values.migrated') }}
                    </span>
                  </div>
                  <div v-if="row.description" class="mt-1 line-clamp-2 text-[11px] text-gray-500 dark:text-gray-400">
                    {{ row.description }}
                  </div>
                  <div class="mt-1 text-[10px] text-gray-400">
                    {{ row.updated_at || row.created_at ? formatDateTime((row.updated_at || row.created_at) as string) : '--' }}
                  </div>
                </button>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs">
                <span
                  class="inline-flex rounded-full px-2 py-1 text-[11px] font-bold"
                  :class="row.enabled ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'"
                >
                  {{ row.enabled ? t('common.enabled') : t('common.disabled') }}
                </span>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200">1m</td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                {{ formatMetricType(row) }}
              </td>
              <td class="px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                <div v-if="formatCategories(row).length > 0" class="flex max-w-[220px] flex-wrap gap-1">
                  <span
                    v-for="category in formatCategories(row)"
                    :key="category"
                    class="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                  >
                    {{ category }}
                  </span>
                </div>
                <span v-else class="text-gray-400">{{ t('admin.ops.alertRules.values.allCategories') }}</span>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs font-bold text-gray-700 dark:text-gray-200">
                {{ formatTriggerLevel(row.trigger_level || row.severity) }}
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                {{ row.min_final_failures ?? '--' }}
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                {{ formatPercent(row.min_failure_rate) }}
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                {{ row.min_sample_count ?? '--' }}
              </td>
              <td class="px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                <div class="max-w-[220px] truncate" :title="formatImpactScopeSummary(row)">
                  {{ formatImpactScopeSummary(row) }}
                </div>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                {{ row.auto_ai_analysis ? t('common.yes') : t('common.no') }}
              </td>
              <td class="px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                <div class="flex max-w-[200px] flex-wrap gap-1">
                  <span
                    v-for="channel in getNotificationChannels(row)"
                    :key="channel"
                    class="rounded-full bg-sky-100 px-2 py-0.5 text-[11px] text-sky-700 dark:bg-sky-900/30 dark:text-sky-300"
                  >
                    {{ t(`admin.ops.alertRules.channels.${channel}`) }}
                  </span>
                </div>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                {{ row.silence_minutes ?? row.cooldown_minutes ?? 0 }}m
              </td>
              <td class="px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                <div class="max-w-[260px] truncate" :title="formatRuleCondition(row)">
                  {{ formatRuleCondition(row) }}
                </div>
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-right text-xs">
                <button class="btn btn-sm btn-secondary" @click="openEdit(row)">{{ t('common.edit') }}</button>
                <button class="ml-2 btn btn-sm btn-danger" @click="requestDelete(row)">{{ t('common.delete') }}</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <BaseDialog
      :show="showEditor"
      :title="editingId ? t('admin.ops.alertRules.editTitle') : t('admin.ops.alertRules.createTitle')"
      width="wide"
      @close="closeEditor"
    >
      <div v-if="draft" class="space-y-5">
        <div v-if="isReadOnlyLegacy" class="rounded-xl bg-amber-50 p-4 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
          {{ t('admin.ops.alertRules.hints.readonlyLegacy') }}
        </div>

        <div v-if="!editorValidation.valid" class="rounded-xl bg-red-50 p-4 text-xs text-red-700 dark:bg-red-900/30 dark:text-red-300">
          <div class="font-bold">{{ t('admin.ops.alertRules.validation.title') }}</div>
          <ul class="mt-1 list-disc pl-5">
            <li v-for="e in editorValidation.errors" :key="e">{{ e }}</li>
          </ul>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.name') }}</label>
            <input v-model="draft.name" class="input" type="text" :disabled="isReadOnlyLegacy" maxlength="50" />
          </div>

          <div class="flex items-center justify-between rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-800/50">
            <span class="text-xs font-bold text-gray-700 dark:text-gray-200">{{ t('admin.ops.alertRules.form.enabled') }}</span>
            <input v-model="draft.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" :disabled="isReadOnlyLegacy" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.timeWindow') }}</label>
            <input value="1m" class="input cursor-not-allowed bg-gray-50 dark:bg-dark-700/50" type="text" disabled />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.alertRules.values.fixedOneMinuteWindow') }}</p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.triggerLevel') }}</label>
            <Select v-model="draft.trigger_level" :options="severityOptions" :disabled="isReadOnlyLegacy" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.metricType') }}</label>
            <Select v-model="draft.metric_type" :options="metricTypeOptions" :disabled="isReadOnlyLegacy" />
          </div>

          <div v-if="normalizeAlertMetricType(draft.metric_type) === 'compound_rule'" class="md:col-span-2">
            <label class="input-label">{{ t('admin.ops.alertRules.form.errorCategories') }}</label>
            <div class="rounded-2xl border border-gray-200 p-3 dark:border-dark-700">
              <div class="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-3">
                <label
                  v-for="option in errorCategoryOptions"
                  :key="option.value"
                  class="flex items-center gap-2 rounded-lg px-2 py-1 text-sm text-gray-700 dark:text-gray-200"
                >
                  <input
                    v-model="draft.error_categories"
                    type="checkbox"
                    :value="option.value"
                    class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                    :disabled="isReadOnlyLegacy"
                  />
                  <span>{{ option.label }}</span>
                </label>
              </div>
            </div>
          </div>

          <div v-if="normalizeAlertMetricType(draft.metric_type) === 'health_score'">
            <label class="input-label">{{ t('admin.ops.alertRules.form.healthScoreThreshold') }}</label>
            <input v-model.number="draft.threshold" class="input" type="number" min="1" max="100" step="1" :disabled="isReadOnlyLegacy" />
          </div>

          <div v-if="normalizeAlertMetricType(draft.metric_type) !== 'health_score'">
            <label class="input-label">{{ t('admin.ops.alertRules.form.minFinalFailures') }}</label>
            <input v-model.number="draft.min_final_failures" class="input" type="number" min="1" max="100000" step="1" :disabled="isReadOnlyLegacy" />
          </div>

          <div v-if="normalizeAlertMetricType(draft.metric_type) !== 'health_score'">
            <label class="input-label">{{ t('admin.ops.alertRules.form.minFailureRate') }}</label>
            <input v-model.number="draft.min_failure_rate" class="input" type="number" min="0" max="100" step="0.01" :disabled="isReadOnlyLegacy || normalizeAlertMetricType(draft.metric_type) === 'final_failures'" />
          </div>

          <div v-if="normalizeAlertMetricType(draft.metric_type) !== 'health_score'">
            <label class="input-label">{{ t('admin.ops.alertRules.form.minSampleCount') }}</label>
            <input v-model.number="draft.min_sample_count" class="input" type="number" min="1" max="1000000" step="1" :disabled="isReadOnlyLegacy || normalizeAlertMetricType(draft.metric_type) === 'final_failures'" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.silenceMinutes') }}</label>
            <input v-model.number="draft.silence_minutes" class="input" type="number" min="0" max="1440" step="1" :disabled="isReadOnlyLegacy" />
          </div>

          <div class="md:col-span-2">
            <label class="input-label">{{ t('admin.ops.alertRules.form.impactScope') }}</label>
            <div class="grid grid-cols-1 gap-3 rounded-2xl border border-gray-200 p-3 dark:border-dark-700 md:grid-cols-2 xl:grid-cols-3">
              <div v-for="field in impactScopeFields" :key="field.key">
                <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-300">{{ field.label }}</label>
                <input
                  class="input"
                  type="number"
                  min="1"
                  max="100000"
                  step="1"
                  :value="draft.impact_scope?.[field.key] ?? ''"
                  :disabled="isReadOnlyLegacy"
                  @input="onImpactScopeInput(field.key, ($event.target as HTMLInputElement).value)"
                />
              </div>
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.recoveredPolicy') }}</label>
            <Select v-model="draft.recovered_fluctuation_policy" :options="recoveredPolicyOptions" :disabled="isReadOnlyLegacy" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.minRecoveredFluctuations') }}</label>
            <input
              v-model.number="draft.min_recovered_fluctuations"
              class="input"
              type="number"
              min="1"
              max="100000"
              step="1"
              :disabled="isReadOnlyLegacy || draft.recovered_fluctuation_policy === 'record_only'"
            />
          </div>

          <div class="flex items-center justify-between rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-800/50">
            <div>
              <span class="text-xs font-bold text-gray-700 dark:text-gray-200">{{ t('admin.ops.alertRules.form.autoAIAnalysis') }}</span>
            </div>
            <input v-model="draft.auto_ai_analysis" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" :disabled="isReadOnlyLegacy" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.groupId') }}</label>
            <Select v-model="draftGroupId" :options="groupOptions" searchable :placeholder="t('admin.ops.alertRules.form.groupPlaceholder')" :disabled="isReadOnlyLegacy" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.alertRules.hints.groupOptional') }}</p>
          </div>

          <div class="md:col-span-2">
            <label class="input-label">{{ t('admin.ops.alertRules.form.notificationChannels') }}</label>
            <div class="rounded-2xl border border-gray-200 p-3 dark:border-dark-700">
              <div class="grid grid-cols-1 gap-2 md:grid-cols-3">
                <label
                  v-for="option in notificationChannelOptions"
                  :key="option.value"
                  class="flex items-center gap-2 rounded-lg px-2 py-1 text-sm text-gray-700 dark:text-gray-200"
                >
                  <input
                    :checked="draft.notification_channels?.includes(option.value)"
                    type="checkbox"
                    class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                    :disabled="isReadOnlyLegacy"
                    @change="toggleNotificationChannel(option.value, ($event.target as HTMLInputElement).checked)"
                  />
                  <span>{{ option.label }}</span>
                </label>
              </div>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                {{ hasEmailRecipients ? t('admin.ops.alertRules.hints.emailRecipientsReady') : t('admin.ops.alertRules.hints.emailRecipientsMissing') }}
              </p>
            </div>
          </div>

          <div class="md:col-span-2">
            <label class="input-label">{{ t('admin.ops.alertRules.form.description') }}</label>
            <textarea v-model="draft.description" class="input min-h-[112px]" rows="4" maxlength="500" :disabled="isReadOnlyLegacy" />
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex items-center justify-end gap-2">
          <button class="btn btn-secondary" :disabled="saving" @click="closeEditor">
            {{ t(isReadOnlyLegacy ? 'common.close' : 'common.cancel') }}
          </button>
          <button v-if="!isReadOnlyLegacy" class="btn btn-primary" :disabled="saving" @click="save">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.ops.alertRules.deleteConfirmTitle')"
      :message="t('admin.ops.alertRules.deleteConfirmMessage')"
      :confirmText="t('common.delete')"
      :cancelText="t('common.cancel')"
      @confirm="confirmDelete"
      @cancel="cancelDelete"
    />
  </div>
</template>
