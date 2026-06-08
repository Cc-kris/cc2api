<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import { adminAPI } from '@/api'
import { opsAPI } from '@/api/admin/ops'
import type { AlertRule, MetricType, Operator } from '../types'
import type { OpsSeverity } from '@/api/admin/ops'
import { formatDateTime } from '../utils/opsFormatters'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const rules = ref<AlertRule[]>([])

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

onMounted(() => {
  load()
  loadGroups()
})

const showEditor = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const draft = ref<AlertRule | null>(null)

const keywordFilter = ref('')
const severityFilter = ref('')
const enabledFilter = ref<'all' | 'enabled' | 'disabled'>('all')

type SortKey = 'updated_at' | 'name' | 'trigger_level' | 'min_final_failures' | 'min_failure_rate' | 'min_sample_count'
const sortKey = ref<SortKey>('updated_at')
const sortDirection = ref<'asc' | 'desc'>('desc')

type MetricGroup = 'system' | 'group' | 'account'

interface MetricDefinition {
  type: MetricType
  group: MetricGroup
  label: string
  description: string
  recommendedOperator: Operator
  recommendedThreshold: number
  unit?: string
}

const groupMetricTypes = new Set<MetricType>([
  'group_available_accounts',
  'group_available_ratio',
  'group_rate_limit_ratio'
])

const severityRank: Record<string, number> = {
  P0: 0,
  P1: 1,
  P2: 2,
  P3: 3,
  OBSERVE: 4
}

function parsePositiveInt(value: unknown): number | null {
  if (value == null) return null
  if (typeof value === 'boolean') return null
  const n = typeof value === 'number' ? value : Number.parseInt(String(value), 10)
  return Number.isFinite(n) && n > 0 ? n : null
}

function normalizeLevel(value?: string | null): string {
  return String(value || '').trim().toUpperCase()
}

function getRuleLevel(rule: AlertRule): string {
  const level = normalizeLevel(rule.trigger_level || rule.severity)
  return level || 'P2'
}

function isCompoundRule(rule: AlertRule): boolean {
  return rule.rule_version === 'v2' || rule.metric_type === 'compound_rule'
}

function getNotificationChannels(rule: AlertRule): string[] {
  if (Array.isArray(rule.notification_channels) && rule.notification_channels.length > 0) {
    return rule.notification_channels
  }
  return rule.notify_email ? ['in_app', 'email'] : ['in_app']
}

function formatPercent(value?: number | null): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  return `${value % 1 === 0 ? value.toFixed(0) : value.toFixed(2).replace(/\.0+$/, '').replace(/(\.\d*?)0+$/, '$1')}%`
}

function formatImpactScopeSummary(rule: AlertRule): string {
  const impact = rule.impact_scope || {}
  const items = [
    { key: 'affected_users', label: t('admin.ops.alertRules.scope.affectedUsers') },
    { key: 'affected_api_keys', label: t('admin.ops.alertRules.scope.affectedApiKeys') },
    { key: 'affected_models', label: t('admin.ops.alertRules.scope.affectedModels') },
    { key: 'affected_upstream_accounts', label: t('admin.ops.alertRules.scope.affectedUpstreamAccounts') }
  ].flatMap(({ key, label }) => {
    const value = impact[key]
    return typeof value === 'number' && value > 0 ? [`${label} ≥ ${value}`] : []
  })
  return items.length > 0 ? items.join(' · ') : t('admin.ops.alertRules.values.notLimited')
}

function formatCategories(rule: AlertRule): string[] {
  if (Array.isArray(rule.error_categories) && rule.error_categories.length > 0) {
    return rule.error_categories
  }
  return []
}

function formatRuleCondition(rule: AlertRule): string {
  if (isCompoundRule(rule)) {
    return [
      `${t('admin.ops.alertRules.table.minFinalFailures')} ≥ ${rule.min_final_failures || 0}`,
      `${t('admin.ops.alertRules.table.minFailureRate')} ≥ ${formatPercent(rule.min_failure_rate)}`,
      `${t('admin.ops.alertRules.table.minSampleCount')} ≥ ${rule.min_sample_count || 0}`
    ].join(' · ')
  }
  return `${rule.metric_type} ${rule.operator} ${rule.threshold}`
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

const severityOptions = computed(() => {
  const sev: OpsSeverity[] = ['P0', 'P1', 'P2', 'P3']
  return sev.map((s) => ({ value: s, label: s }))
})

const filterSeverityOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.ops.alertRules.filters.allSeverities') },
  ...severityOptions.value
])

const enabledOptions = computed<SelectOption[]>(() => [
  { value: 'all', label: t('admin.ops.alertRules.filters.allStatuses') },
  { value: 'enabled', label: t('common.enabled') },
  { value: 'disabled', label: t('common.disabled') }
])

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
      rule.metric_type,
      rule.trigger_level,
      rule.severity,
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

const hasActiveFilters = computed(() => {
  return Boolean(keywordFilter.value.trim()) || Boolean(severityFilter.value) || enabledFilter.value !== 'all'
})

function resetFilters() {
  keywordFilter.value = ''
  severityFilter.value = ''
  enabledFilter.value = 'all'
  sortKey.value = 'updated_at'
  sortDirection.value = 'desc'
}

const groupOptionsBase = ref<SelectOption[]>([])

async function loadGroups() {
  try {
    const list = await adminAPI.groups.getAll()
    groupOptionsBase.value = list.map((g) => ({ value: g.id, label: g.name }))
  } catch (err) {
    console.error('[OpsAlertRulesCard] Failed to load groups', err)
    groupOptionsBase.value = []
  }
}

const isGroupMetricSelected = computed(() => {
  const metricType = draft.value?.metric_type
  return metricType ? groupMetricTypes.has(metricType) : false
})

const draftGroupId = computed<number | null>({
  get() {
    return parsePositiveInt(draft.value?.filters?.group_id)
  },
  set(value) {
    if (!draft.value) return
    if (value == null) {
      if (!draft.value.filters) return
      delete draft.value.filters.group_id
      if (Object.keys(draft.value.filters).length === 0) {
        delete draft.value.filters
      }
      return
    }
    if (!draft.value.filters) draft.value.filters = {}
    draft.value.filters.group_id = value
  }
})

const groupOptions = computed<SelectOption[]>(() => {
  if (isGroupMetricSelected.value) return groupOptionsBase.value
  return [{ value: null, label: t('admin.ops.alertRules.form.allGroups') }, ...groupOptionsBase.value]
})

const metricDefinitions = computed(() => {
  return [
    {
      type: 'success_rate',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.successRate'),
      description: t('admin.ops.alertRules.metricDescriptions.successRate'),
      recommendedOperator: '<',
      recommendedThreshold: 99,
      unit: '%'
    },
    {
      type: 'error_rate',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.errorRate'),
      description: t('admin.ops.alertRules.metricDescriptions.errorRate'),
      recommendedOperator: '>',
      recommendedThreshold: 1,
      unit: '%'
    },
    {
      type: 'upstream_error_rate',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.upstreamErrorRate'),
      description: t('admin.ops.alertRules.metricDescriptions.upstreamErrorRate'),
      recommendedOperator: '>',
      recommendedThreshold: 1,
      unit: '%'
    },
    {
      type: 'cpu_usage_percent',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.cpu'),
      description: t('admin.ops.alertRules.metricDescriptions.cpu'),
      recommendedOperator: '>',
      recommendedThreshold: 80,
      unit: '%'
    },
    {
      type: 'memory_usage_percent',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.memory'),
      description: t('admin.ops.alertRules.metricDescriptions.memory'),
      recommendedOperator: '>',
      recommendedThreshold: 80,
      unit: '%'
    },
    {
      type: 'concurrency_queue_depth',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.queueDepth'),
      description: t('admin.ops.alertRules.metricDescriptions.queueDepth'),
      recommendedOperator: '>',
      recommendedThreshold: 10
    },
    {
      type: 'group_available_accounts',
      group: 'group',
      label: t('admin.ops.alertRules.metrics.groupAvailableAccounts'),
      description: t('admin.ops.alertRules.metricDescriptions.groupAvailableAccounts'),
      recommendedOperator: '<',
      recommendedThreshold: 1
    },
    {
      type: 'group_available_ratio',
      group: 'group',
      label: t('admin.ops.alertRules.metrics.groupAvailableRatio'),
      description: t('admin.ops.alertRules.metricDescriptions.groupAvailableRatio'),
      recommendedOperator: '<',
      recommendedThreshold: 50,
      unit: '%'
    },
    {
      type: 'group_rate_limit_ratio',
      group: 'group',
      label: t('admin.ops.alertRules.metrics.groupRateLimitRatio'),
      description: t('admin.ops.alertRules.metricDescriptions.groupRateLimitRatio'),
      recommendedOperator: '>',
      recommendedThreshold: 10,
      unit: '%'
    },
    {
      type: 'account_rate_limited_count',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.accountRateLimitedCount'),
      description: t('admin.ops.alertRules.metricDescriptions.accountRateLimitedCount'),
      recommendedOperator: '>',
      recommendedThreshold: 0
    },
    {
      type: 'account_error_count',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.accountErrorCount'),
      description: t('admin.ops.alertRules.metricDescriptions.accountErrorCount'),
      recommendedOperator: '>',
      recommendedThreshold: 0
    },
    {
      type: 'account_error_ratio',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.accountErrorRatio'),
      description: t('admin.ops.alertRules.metricDescriptions.accountErrorRatio'),
      recommendedOperator: '>',
      recommendedThreshold: 5,
      unit: '%'
    },
    {
      type: 'overload_account_count',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.overloadAccountCount'),
      description: t('admin.ops.alertRules.metricDescriptions.overloadAccountCount'),
      recommendedOperator: '>',
      recommendedThreshold: 0
    }
  ] satisfies MetricDefinition[]
})

const selectedMetricDefinition = computed(() => {
  const metricType = draft.value?.metric_type
  if (!metricType) return null
  return metricDefinitions.value.find((m) => m.type === metricType) ?? null
})

const metricOptions = computed(() => {
  const buildGroup = (group: MetricGroup): SelectOption[] => {
    const items = metricDefinitions.value.filter((m) => m.group === group)
    if (items.length === 0) return []
    const headerValue = `__group__${group}`
    return [
      {
        value: headerValue,
        label: t(`admin.ops.alertRules.metricGroups.${group}`),
        disabled: true,
        kind: 'group'
      },
      ...items.map((m) => ({ value: m.type, label: m.label }))
    ]
  }

  return [...buildGroup('system'), ...buildGroup('group'), ...buildGroup('account')]
})

const operatorOptions = computed(() => {
  const ops: Operator[] = ['>', '>=', '<', '<=', '==', '!=']
  return ops.map((o) => ({ value: o, label: o }))
})

function newRuleDraft(): AlertRule {
  return {
    name: '',
    description: '',
    enabled: true,
    metric_type: 'error_rate',
    operator: '>',
    threshold: 1,
    window_minutes: 1,
    sustained_minutes: 2,
    severity: 'P1',
    cooldown_minutes: 10,
    notify_email: true
  }
}

function openCreate() {
  editingId.value = null
  draft.value = newRuleDraft()
  showEditor.value = true
}

function openEdit(rule: AlertRule) {
  editingId.value = rule.id ?? null
  draft.value = JSON.parse(JSON.stringify(rule))
  showEditor.value = true
}

const editorValidation = computed(() => {
  const errors: string[] = []
  const r = draft.value
  if (!r) return { valid: true, errors }
  if (!r.name || !r.name.trim()) errors.push(t('admin.ops.alertRules.validation.nameRequired'))
  if (!r.metric_type) errors.push(t('admin.ops.alertRules.validation.metricRequired'))
  if (groupMetricTypes.has(r.metric_type) && !parsePositiveInt(r.filters?.group_id)) {
    errors.push(t('admin.ops.alertRules.validation.groupIdRequired'))
  }
  if (!r.operator) errors.push(t('admin.ops.alertRules.validation.operatorRequired'))
  if (!(typeof r.threshold === 'number' && Number.isFinite(r.threshold))) {
    errors.push(t('admin.ops.alertRules.validation.thresholdRequired'))
  }
  if (!(typeof r.window_minutes === 'number' && Number.isFinite(r.window_minutes) && [1, 5, 60].includes(r.window_minutes))) {
    errors.push(t('admin.ops.alertRules.validation.windowRange'))
  }
  if (!(typeof r.sustained_minutes === 'number' && Number.isFinite(r.sustained_minutes) && r.sustained_minutes >= 1 && r.sustained_minutes <= 1440)) {
    errors.push(t('admin.ops.alertRules.validation.sustainedRange'))
  }
  if (!(typeof r.cooldown_minutes === 'number' && Number.isFinite(r.cooldown_minutes) && r.cooldown_minutes >= 0 && r.cooldown_minutes <= 1440)) {
    errors.push(t('admin.ops.alertRules.validation.cooldownRange'))
  }
  return { valid: errors.length === 0, errors }
})

async function save() {
  if (!draft.value) return
  if (!editorValidation.value.valid) {
    appStore.showError(editorValidation.value.errors[0] || t('admin.ops.alertRules.validation.invalid'))
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await opsAPI.updateAlertRule(editingId.value, draft.value)
    } else {
      await opsAPI.createAlertRule(draft.value)
    }
    showEditor.value = false
    draft.value = null
    editingId.value = null
    await load()
    appStore.showSuccess(t('admin.ops.alertRules.saveSuccess'))
  } catch (err: any) {
    console.error('[OpsAlertRulesCard] Failed to save rule', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertRules.saveFailed'))
  } finally {
    saving.value = false
  }
}

const showDeleteConfirm = ref(false)
const pendingDelete = ref<AlertRule | null>(null)

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
                    {{ formatDateTime(row.updated_at || row.created_at) }}
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
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                {{ row.window_minutes || 1 }}m
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
                {{ getRuleLevel(row) }}
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                {{ isCompoundRule(row) ? row.min_final_failures : '--' }}
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                {{ isCompoundRule(row) ? formatPercent(row.min_failure_rate) : '--' }}
              </td>
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200">
                {{ isCompoundRule(row) ? row.min_sample_count : '--' }}
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
      @close="showEditor = false"
    >
      <div class="space-y-4">
        <div v-if="!editorValidation.valid" class="rounded-xl bg-red-50 p-4 text-xs text-red-700 dark:bg-red-900/30 dark:text-red-300">
          <div class="font-bold">{{ t('admin.ops.alertRules.validation.title') }}</div>
          <ul class="mt-1 list-disc pl-5">
            <li v-for="e in editorValidation.errors" :key="e">{{ e }}</li>
          </ul>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div class="md:col-span-2">
            <label class="input-label">{{ t('admin.ops.alertRules.form.name') }}</label>
            <input v-model="draft!.name" class="input" type="text" />
          </div>

          <div class="md:col-span-2">
            <label class="input-label">{{ t('admin.ops.alertRules.form.description') }}</label>
            <input v-model="draft!.description" class="input" type="text" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.metric') }}</label>
            <Select v-model="draft!.metric_type" :options="metricOptions" />
            <div v-if="selectedMetricDefinition" class="mt-1 space-y-0.5 text-xs text-gray-500 dark:text-gray-400">
              <p>{{ selectedMetricDefinition.description }}</p>
              <p>
                {{
                  t('admin.ops.alertRules.hints.recommended', {
                    operator: selectedMetricDefinition.recommendedOperator,
                    threshold: selectedMetricDefinition.recommendedThreshold,
                    unit: selectedMetricDefinition.unit || ''
                  })
                }}
              </p>
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.operator') }}</label>
            <Select v-model="draft!.operator" :options="operatorOptions" />
          </div>

          <div class="md:col-span-2">
            <label class="input-label">
              {{ t('admin.ops.alertRules.form.groupId') }}
              <span v-if="isGroupMetricSelected" class="ml-1 text-red-500">*</span>
            </label>
            <Select
              v-model="draftGroupId"
              :options="groupOptions"
              searchable
              :placeholder="t('admin.ops.alertRules.form.groupPlaceholder')"
              :error="isGroupMetricSelected && !draftGroupId"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ isGroupMetricSelected ? t('admin.ops.alertRules.hints.groupRequired') : t('admin.ops.alertRules.hints.groupOptional') }}
            </p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.threshold') }}</label>
            <input v-model.number="draft!.threshold" class="input" type="number" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.severity') }}</label>
            <Select v-model="draft!.severity" :options="severityOptions" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.window') }}</label>
            <input :value="`${draft!.window_minutes}m`" class="input cursor-not-allowed bg-gray-50 dark:bg-dark-700/50" type="text" disabled />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.alertRules.values.fixedOneMinuteWindow') }}</p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.sustained') }}</label>
            <input v-model.number="draft!.sustained_minutes" class="input" type="number" min="1" max="1440" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.cooldown') }}</label>
            <input v-model.number="draft!.cooldown_minutes" class="input" type="number" min="0" max="1440" />
          </div>

          <div class="flex items-center justify-between rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-800/50 md:col-span-2">
            <span class="text-xs font-bold text-gray-700 dark:text-gray-200">{{ t('admin.ops.alertRules.form.enabled') }}</span>
            <input v-model="draft!.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          </div>

          <div class="flex items-center justify-between rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-800/50 md:col-span-2">
            <span class="text-xs font-bold text-gray-700 dark:text-gray-200">{{ t('admin.ops.alertRules.form.notifyEmail') }}</span>
            <input v-model="draft!.notify_email" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex items-center justify-end gap-2">
          <button class="btn btn-secondary" :disabled="saving" @click="showEditor = false">
            {{ t('common.cancel') }}
          </button>
          <button class="btn btn-primary" :disabled="saving" @click="save">
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
