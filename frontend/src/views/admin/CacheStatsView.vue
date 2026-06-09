<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <CacheNavPills active="stats" />
            <h1 class="mt-4 text-2xl font-semibold text-gray-900 dark:text-white">
              {{ t('admin.cacheStats.title') }}
            </h1>
            <p class="mt-2 max-w-3xl text-sm text-gray-600 dark:text-gray-400">
              {{ t('admin.cacheStats.description') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadStats(true)">
              {{ t('admin.cacheStats.actions.refresh') }}
            </button>
          </div>
        </div>
      </div>

      <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('admin.cacheStats.filters.title') }}
          </h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.cacheStats.filters.hint') }}
          </p>
        </div>
        <div class="space-y-5 px-6 py-5">
          <div class="grid grid-cols-1 gap-4 xl:grid-cols-5">
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheStats.filters.timeRange') }}</span>
              <select v-model="filters.time_range" class="input">
                <option v-for="option in timeRangeOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
              </select>
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheStats.filters.platform') }}</span>
              <select v-model="filters.platform" class="input">
                <option value="">{{ t('admin.cacheStats.filters.allPlatforms') }}</option>
                <option v-for="option in platformOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
              </select>
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheStats.filters.model') }}</span>
              <input
                v-model.trim="filters.model"
                class="input"
                type="text"
                :list="modelSuggestions.length > 0 ? 'cache-stats-models' : undefined"
                :placeholder="t('admin.cacheStats.filters.modelPlaceholder')"
              />
              <datalist id="cache-stats-models">
                <option v-for="model in modelSuggestions" :key="model" :value="model" />
              </datalist>
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheStats.filters.group') }}</span>
              <select v-model="filters.group_id" class="input">
                <option value="">{{ t('admin.cacheStats.filters.allGroups') }}</option>
                <option v-for="group in groupOptions" :key="group.id" :value="String(group.id)">{{ group.name }}</option>
              </select>
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheStats.filters.apiKey') }}</span>
              <input
                v-model.trim="apiKeySearchText"
                class="input"
                type="text"
                :list="apiKeyOptions.length > 0 ? 'cache-stats-api-keys' : undefined"
                :placeholder="t('admin.cacheStats.filters.apiKeyPlaceholder')"
                @blur="syncSelectedApiKey"
                @keydown.enter.prevent="syncSelectedApiKey"
              />
              <datalist id="cache-stats-api-keys">
                <option v-for="item in apiKeyOptions" :key="item.id" :value="apiKeyOptionLabel(item)">{{ apiKeyOptionLabel(item) }}</option>
              </datalist>
            </label>
          </div>

          <div v-if="filters.time_range === 'custom'" class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
            <label class="block xl:col-span-2">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheStats.filters.startTime') }}</span>
              <input v-model="filters.start_time" class="input" type="datetime-local" />
            </label>
            <label class="block xl:col-span-2">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheStats.filters.endTime') }}</span>
              <input v-model="filters.end_time" class="input" type="datetime-local" />
            </label>
          </div>

          <div v-if="filterValidationError" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200">
            {{ filterValidationError }}
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-primary" :disabled="loading || Boolean(filterValidationError)" @click="loadStats(true)">
              {{ loading ? t('admin.cacheStats.actions.loading') : t('admin.cacheStats.actions.applyFilters') }}
            </button>
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="resetFilters">
              {{ t('admin.cacheStats.actions.resetFilters') }}
            </button>
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.cacheStats.filters.apiKeyHint') }}
            </p>
          </div>
        </div>
      </div>

      <div v-if="loadError" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <span>{{ loadError }}</span>
          <button type="button" class="btn btn-secondary" @click="loadStats(true)">
            {{ t('admin.cacheStats.actions.retry') }}
          </button>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-5">
        <div v-for="card in summaryCards" :key="card.key" class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ card.label }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ card.value }}</p>
          <p v-if="card.hint" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ card.hint }}</p>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <section class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.cacheStats.sections.bypassReasons') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.cacheStats.sections.reasonHint') }}</p>
          </div>
          <div class="px-6 py-5">
            <div v-if="bypassReasonRows.length === 0" class="rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
              {{ t('admin.cacheStats.emptyBypassReasons') }}
            </div>
            <div v-else class="space-y-4">
              <div v-for="row in bypassReasonRows" :key="`bypass-${row.reason}`" class="space-y-2">
                <div class="flex items-center justify-between gap-3 text-sm">
                  <span class="truncate font-medium text-gray-900 dark:text-white">{{ formatCacheReason(row.reason) }}</span>
                  <span class="shrink-0 text-gray-500 dark:text-gray-400">{{ formatInteger(row.count) }} · {{ formatPercent(row.percent) }}</span>
                </div>
                <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                  <div class="h-full rounded-full bg-amber-500" :style="{ width: `${Math.min(row.percent, 100)}%` }"></div>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.cacheStats.sections.storeSkipReasons') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.cacheStats.sections.reasonHint') }}</p>
          </div>
          <div class="px-6 py-5">
            <div v-if="storeSkipReasonRows.length === 0" class="rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
              {{ t('admin.cacheStats.emptyStoreSkipReasons') }}
            </div>
            <div v-else class="space-y-4">
              <div v-for="row in storeSkipReasonRows" :key="`skip-${row.reason}`" class="space-y-2">
                <div class="flex items-center justify-between gap-3 text-sm">
                  <span class="truncate font-medium text-gray-900 dark:text-white">{{ formatCacheReason(row.reason) }}</span>
                  <span class="shrink-0 text-gray-500 dark:text-gray-400">{{ formatInteger(row.count) }} · {{ formatPercent(row.percent) }}</span>
                </div>
                <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                  <div class="h-full rounded-full bg-sky-500" :style="{ width: `${Math.min(row.percent, 100)}%` }"></div>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>

      <section class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.cacheStats.sections.modelTable') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.cacheStats.sections.modelTableHint') }}</p>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-900/40">
              <tr>
                <th v-for="column in tableColumns" :key="column.key" class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ column.label }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-if="sortedRows.length === 0">
                <td :colspan="tableColumns.length" class="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.cacheStats.emptyModelRows') }}
                </td>
              </tr>
              <tr v-for="row in sortedRows" :key="`${row.platform}-${row.model}`" class="align-top">
                <td class="px-4 py-3 text-sm text-gray-900 dark:text-white">
                  <div class="font-medium">{{ row.platform || '--' }}</div>
                  <div class="mt-1 break-all text-xs text-gray-500 dark:text-gray-400">{{ row.model || '--' }}</div>
                </td>
                <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                  <div>{{ formatInteger(row.total_requests) }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheStats.table.candidateShort') }} {{ formatInteger(row.candidate_requests) }}</div>
                </td>
                <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                  <div>{{ formatInteger(row.hit_requests) }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheStats.table.missShort') }} {{ formatInteger(row.miss_requests) }}</div>
                </td>
                <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                  <div>{{ formatInteger(row.bypass_requests) }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ formatCacheReason(row.top_bypass_reason) }}</div>
                </td>
                <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                  <div>{{ formatInteger(row.store_success) }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheStats.table.skipShort') }} {{ formatInteger(row.store_skip) }}</div>
                </td>
                <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                  <div>{{ formatInteger(row.hit_tokens) }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheStats.table.inputShort') }} {{ formatInteger(row.input_tokens) }}</div>
                </td>
                <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                  <div>{{ formatPercent(row.request_hit_rate) }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheStats.table.tokensRateShort') }} {{ formatPercent(row.tokens_hit_rate) }}</div>
                </td>
                <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                  {{ formatSavedAmount(row.estimated_saved_amount) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import CacheNavPills from './cache/CacheNavPills.vue'
import { adminAPI } from '@/api/admin'
import type { CacheStatsResponse } from '@/api/admin/cache'
import type { AdminGroup } from '@/types'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatCurrency } from '@/utils/format'
import { formatApiKeyOptionLabel } from '@/utils/adminSensitiveDisplay'

interface ApiKeySearchOption {
  id: number
  name: string
}

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const loading = ref(false)
const loadError = ref('')
const groups = ref<AdminGroup[]>([])
const apiKeyOptions = ref<ApiKeySearchOption[]>([])
const apiKeySearchText = ref('')
const apiKeySearchSeq = ref(0)
const lastLoadedResponse = ref<CacheStatsResponse | null>(null)

const filters = reactive({
  time_range: '1d',
  start_time: '',
  end_time: '',
  platform: '',
  model: '',
  group_id: '',
  api_key_id: ''
})

const viewerRole = computed(() => String((authStore.user as { role?: string } | null)?.role || '').trim().toLowerCase())
const canViewSavedAmount = computed(() => {
  const role = viewerRole.value
  return role === '' || role === 'admin' || role === 'business' || role === 'business_operator' || role === 'business-operator' || role === 'yunying' || role === '运营'
})

const timeRangeOptions = computed(() => [
  { value: '1h', label: t('admin.cacheStats.timeRanges.1h') },
  { value: '6h', label: t('admin.cacheStats.timeRanges.6h') },
  { value: '1d', label: t('admin.cacheStats.timeRanges.1d') },
  { value: '7d', label: t('admin.cacheStats.timeRanges.7d') },
  { value: '30d', label: t('admin.cacheStats.timeRanges.30d') },
  { value: 'custom', label: t('admin.cacheStats.timeRanges.custom') }
])

const platformOptions = computed(() => [
  { value: 'openai', label: 'OpenAI' },
  { value: 'claude', label: 'Claude' },
  { value: 'gemini', label: 'Gemini' }
])

const groupOptions = computed(() => groups.value.filter((group) => group && typeof group.id === 'number'))
const summary = computed(() => lastLoadedResponse.value?.summary ?? {
  total_requests: 0,
  candidate_requests: 0,
  hit_requests: 0,
  miss_requests: 0,
  bypass_requests: 0,
  store_success: 0,
  store_skip: 0,
  request_hit_rate: 0,
  input_tokens: 0,
  output_tokens: 0,
  hit_tokens: 0,
  candidate_tokens: 0,
  tokens_hit_rate: 0,
  overall_tokens_coverage: 0,
  estimated_saved_amount: '0'
})
const modelRows = computed(() => lastLoadedResponse.value?.model_rows ?? [])
const bypassReasonRows = computed(() => lastLoadedResponse.value?.bypass_reasons ?? [])
const storeSkipReasonRows = computed(() => lastLoadedResponse.value?.store_skip_reasons ?? [])
const modelSuggestions = computed(() => Array.from(new Set(modelRows.value.map((row) => row.model).filter(Boolean))).sort())

const filterValidationError = computed(() => {
  if (filters.time_range !== 'custom') return ''
  if (!filters.start_time || !filters.end_time) {
    return t('admin.cacheStats.validation.customRangeRequired')
  }
  const start = new Date(filters.start_time)
  const end = new Date(filters.end_time)
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) {
    return t('admin.cacheStats.validation.invalidDate')
  }
  if (start.getTime() > end.getTime()) {
    return t('admin.cacheStats.validation.invalidRange')
  }
  const diffMs = end.getTime() - start.getTime()
  if (diffMs > 31 * 24 * 60 * 60 * 1000) {
    return t('admin.cacheStats.validation.maxRange')
  }
  return ''
})

const summaryCards = computed(() => [
  {
    key: 'total',
    label: t('admin.cacheStats.cards.totalRequests'),
    value: formatInteger(summary.value.total_requests)
  },
  {
    key: 'candidate',
    label: t('admin.cacheStats.cards.candidateRequests'),
    value: formatInteger(summary.value.candidate_requests)
  },
  {
    key: 'hits',
    label: t('admin.cacheStats.cards.hitRequests'),
    value: formatInteger(summary.value.hit_requests)
  },
  {
    key: 'input',
    label: t('admin.cacheStats.cards.inputTokens'),
    value: formatInteger(summary.value.input_tokens)
  },
  {
    key: 'output',
    label: t('admin.cacheStats.cards.outputTokens'),
    value: formatInteger(summary.value.output_tokens)
  },
  {
    key: 'hitTokens',
    label: t('admin.cacheStats.cards.hitTokens'),
    value: formatInteger(summary.value.hit_tokens)
  },
  {
    key: 'requestHitRate',
    label: t('admin.cacheStats.cards.requestHitRate'),
    value: formatPercent(summary.value.request_hit_rate)
  },
  {
    key: 'tokensHitRate',
    label: t('admin.cacheStats.cards.tokensHitRate'),
    value: formatPercent(summary.value.tokens_hit_rate),
    hint: t('admin.cacheStats.cards.coverageHint', { value: formatPercent(summary.value.overall_tokens_coverage) })
  },
  {
    key: 'saved',
    label: t('admin.cacheStats.cards.estimatedSaved'),
    value: formatSavedAmount(summary.value.estimated_saved_amount)
  }
])

const sortedRows = computed(() => {
  return [...modelRows.value].sort((a, b) => {
    if (b.hit_tokens !== a.hit_tokens) return b.hit_tokens - a.hit_tokens
    if (a.platform !== b.platform) return a.platform.localeCompare(b.platform)
    return a.model.localeCompare(b.model)
  })
})

const tableColumns = computed(() => [
  { key: 'model', label: t('admin.cacheStats.table.model') },
  { key: 'requests', label: t('admin.cacheStats.table.requests') },
  { key: 'hits', label: t('admin.cacheStats.table.hits') },
  { key: 'bypass', label: t('admin.cacheStats.table.bypass') },
  { key: 'store', label: t('admin.cacheStats.table.store') },
  { key: 'tokens', label: t('admin.cacheStats.table.tokens') },
  { key: 'rates', label: t('admin.cacheStats.table.rates') },
  { key: 'saved', label: t('admin.cacheStats.table.saved') }
])

watch(apiKeySearchText, async (value) => {
  const query = value.trim()
  if (query.length < 2) {
    apiKeyOptions.value = []
    if (!query) filters.api_key_id = ''
    return
  }

  const seq = ++apiKeySearchSeq.value
  try {
    const rows = await adminAPI.usage.searchApiKeys(undefined, query)
    if (seq !== apiKeySearchSeq.value) return
    apiKeyOptions.value = rows.map((item) => ({ id: item.id, name: item.name }))
  } catch {
    if (seq !== apiKeySearchSeq.value) return
    apiKeyOptions.value = []
  }
})

function apiKeyOptionLabel(item: ApiKeySearchOption): string {
  return formatApiKeyOptionLabel(item.name, item.id)
}

function syncSelectedApiKey(): void {
  const raw = apiKeySearchText.value.trim()
  if (!raw) {
    filters.api_key_id = ''
    return
  }

  const matched = apiKeyOptions.value.find((item) => apiKeyOptionLabel(item) === raw)
  if (matched) {
    filters.api_key_id = String(matched.id)
    apiKeySearchText.value = apiKeyOptionLabel(matched)
    return
  }

  const directId = raw.match(/#?(\d+)$/)
  if (directId) {
    filters.api_key_id = directId[1]
    return
  }

  filters.api_key_id = ''
}

function formatInteger(value: number | string | null | undefined): string {
  const numeric = Number(value ?? 0)
  if (!Number.isFinite(numeric)) return '--'
  return Math.round(numeric).toLocaleString()
}

function formatPercent(value: number | string | null | undefined): string {
  const numeric = Number(value ?? 0)
  if (!Number.isFinite(numeric)) return '--'
  return `${numeric.toFixed(2)}%`
}

function formatSavedAmount(value: string | number | null | undefined): string {
  if (!canViewSavedAmount.value) {
    return t('admin.cacheStats.hiddenAmount')
  }
  const numeric = Number(value ?? 0)
  if (!Number.isFinite(numeric)) {
    return t('admin.cacheStats.priceMissing')
  }
  return formatCurrency(numeric)
}

function formatCacheReason(value: string | null | undefined): string {
  const normalized = String(value || '').trim()
  if (!normalized) return '--'
  const map: Record<string, string> = {
    disabled: '缓存未开启',
    explicit_bypass: '请求明确跳过缓存',
    no_api_key: '缺少 API Key',
    no_group: '缺少分组',
    request_too_large: '请求过大',
    body_too_large: '请求体过大',
    response_too_large: '响应过大',
    forward_error: '转发请求失败',
    write_error: '写入缓存失败',
    status_not_ok: '响应状态不适合缓存',
    empty_body: '响应内容为空',
    content_type: '响应类型不适合缓存',
    stream_incomplete: '流式响应未完整结束',
    store_failed: '缓存存储失败',
    unsafe_content: '内容安全检查未通过',
    invalid_json: '请求格式不是有效 JSON',
    tools_or_functions: '包含工具调用或函数调用',
    sensitive_content: '包含敏感内容',
    temperature_too_high: '温度参数过高',
    model_not_allowed: '模型未加入缓存范围',
    platform_disabled: '该平台未启用缓存',
    cache_miss: '未命中缓存',
    no_cacheable_content: '无可缓存内容',
    streaming_unsupported: '当前流式响应不支持缓存'
  }
  return map[normalized] || normalized
}

function buildQuery() {
  syncSelectedApiKey()

  const query: Record<string, string | number> = {}
  if (filters.time_range === 'custom') {
    if (filters.start_time) {
      query.start_time = new Date(filters.start_time).toISOString()
    }
    if (filters.end_time) {
      query.end_time = new Date(filters.end_time).toISOString()
    }
  } else {
    query.time_range = filters.time_range
  }
  if (filters.platform) query.platform = filters.platform
  if (filters.model) query.model = filters.model
  if (filters.group_id) query.group_id = Number(filters.group_id)
  if (filters.api_key_id) query.api_key_id = Number(filters.api_key_id)
  return query
}

async function loadGroups(): Promise<void> {
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch {
    groups.value = []
  }
}

async function loadStats(forceToast = false): Promise<void> {
  if (filterValidationError.value) {
    if (forceToast) {
      appStore.showError(filterValidationError.value)
    }
    return
  }

  loading.value = true
  loadError.value = ''
  try {
    const { data } = await adminAPI.cache.getStats(buildQuery())
    lastLoadedResponse.value = data
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, t('admin.cacheStats.loadFailed'))
    if (forceToast) {
      appStore.showError(loadError.value)
    }
  } finally {
    loading.value = false
  }
}

function resetFilters(): void {
  filters.time_range = '1d'
  filters.start_time = ''
  filters.end_time = ''
  filters.platform = ''
  filters.model = ''
  filters.group_id = ''
  filters.api_key_id = ''
  apiKeySearchText.value = ''
  apiKeyOptions.value = []
  loadStats(false)
}

onMounted(async () => {
  await loadGroups()
  await loadStats(false)
})
</script>
