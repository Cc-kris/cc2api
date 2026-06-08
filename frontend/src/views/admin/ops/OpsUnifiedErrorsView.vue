<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <section class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">统一错误列表</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              统一查看错误分类、影响范围、处理结果和 AI 分析状态。
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-3">
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300"
              :disabled="loading"
              @click="fetchErrors"
            >
              刷新
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300"
              @click="resetFilters"
            >
              重置筛选
            </button>
          </div>
        </div>

        <div v-if="errorMessage" class="mt-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
          {{ errorMessage }}
        </div>

        <div class="mt-5 space-y-4">
          <div class="grid grid-cols-1 gap-3 xl:grid-cols-6">
            <div class="xl:col-span-2">
              <label class="filter-label">时间范围</label>
              <select v-model="timeRange" class="input">
                <option v-for="option in timeRangeOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </div>
            <div>
              <label class="filter-label">平台</label>
              <select v-model="platform" class="input">
                <option value="">全部</option>
                <option value="openai">OpenAI</option>
                <option value="claude">Claude</option>
                <option value="gemini">Gemini</option>
                <option value="other">其他</option>
              </select>
            </div>
            <div>
              <label class="filter-label">分组</label>
              <select v-model="groupId" class="input">
                <option value="">全部</option>
                <option v-for="group in groups" :key="group.id" :value="String(group.id)">
                  {{ group.name }}
                </option>
              </select>
            </div>
            <div>
              <label class="filter-label">模型</label>
              <input v-model.trim="model" type="text" class="input" placeholder="输入模型名">
            </div>
            <div>
              <label class="filter-label">AI 分析</label>
              <select v-model="aiAnalysis" class="input">
                <option value="all">全部</option>
                <option value="analyzed">已分析</option>
                <option value="not_analyzed">未分析</option>
              </select>
            </div>
          </div>

          <div v-if="timeRange === 'custom'" class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div>
              <label class="filter-label">开始时间</label>
              <input v-model="customStartInput" type="datetime-local" class="input">
            </div>
            <div>
              <label class="filter-label">结束时间</label>
              <input v-model="customEndInput" type="datetime-local" class="input">
            </div>
          </div>

          <div class="grid grid-cols-1 gap-3 xl:grid-cols-5">
            <div>
              <label class="filter-label">错误大类</label>
              <select v-model="errorCategories" class="input input-multi" multiple>
                <option v-for="option in errorCategoryOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </div>
            <div>
              <label class="filter-label">错误结果</label>
              <select v-model="errorResults" class="input input-multi" multiple>
                <option v-for="option in errorResultOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </div>
            <div>
              <label class="filter-label">严重度</label>
              <select v-model="severities" class="input input-multi" multiple>
                <option v-for="option in severityOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </div>
            <div>
              <label class="filter-label">错误子类</label>
              <input v-model.trim="errorSubcategoriesInput" type="text" class="input" placeholder="逗号分隔">
            </div>
            <div>
              <label class="filter-label">客户端错误细分</label>
              <select v-model="clientErrorSubcategories" class="input input-multi" multiple>
                <option v-for="option in clientErrorSubcategoryOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-3 xl:grid-cols-6">
            <div>
              <label class="filter-label">状态码</label>
              <input v-model.trim="statusCode" type="text" class="input" placeholder="429,500-504">
            </div>
            <div>
              <label class="filter-label">用户 ID</label>
              <input v-model.trim="userId" type="text" class="input" placeholder="数字 ID">
            </div>
            <div>
              <label class="filter-label">API Key ID</label>
              <input v-model.trim="apiKeyId" type="text" class="input" placeholder="数字 ID">
            </div>
            <div>
              <label class="filter-label">上游账号 ID</label>
              <input v-model.trim="upstreamAccountId" type="text" class="input" placeholder="数字 ID">
            </div>
            <div>
              <label class="filter-label">请求 ID</label>
              <input v-model.trim="requestId" type="text" class="input" placeholder="最长 128 字符">
            </div>
            <div>
              <label class="filter-label">关键词</label>
              <input v-model.trim="keyword" type="text" class="input" placeholder="搜索脱敏摘要">
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-3">
            <button
              type="button"
              class="inline-flex items-center rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
              :disabled="loading"
              @click="applyFilters"
            >
              查询
            </button>
            <span class="text-xs text-gray-500 dark:text-gray-400">
              默认最近 30 分钟；时间跨度最大 7 天；多选列表按住 Command 或 Ctrl 可多选。
            </span>
          </div>
        </div>
      </section>

      <section class="rounded-3xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-dark-700">
          <div class="text-sm font-medium text-gray-700 dark:text-gray-200">
            共 {{ total }} 条
          </div>
          <div class="flex items-center gap-3">
            <label class="text-xs text-gray-500 dark:text-gray-400">每页</label>
            <select v-model="pageSize" class="input w-24" @change="handlePageSizeChange">
              <option value="20">20</option>
              <option value="50">50</option>
              <option value="100">100</option>
            </select>
          </div>
        </div>

        <div v-if="loading && !hasLoadedOnce" class="flex items-center justify-center py-20">
          <div class="h-10 w-10 animate-spin rounded-full border-b-2 border-primary-600"></div>
        </div>

        <div v-else class="min-h-0 min-w-0 overflow-auto">
          <table class="min-w-[1800px] border-separate border-spacing-0">
            <thead class="sticky top-0 z-10 bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="table-th">
                  <button type="button" class="sort-button" @click="toggleSort('occurred_at')">
                    发生时间
                    <span>{{ sortIndicator('occurred_at') }}</span>
                  </button>
                </th>
                <th class="table-th">错误分类</th>
                <th class="table-th">错误子类</th>
                <th class="table-th">客户端错误细分</th>
                <th class="table-th">错误摘要</th>
                <th class="table-th">
                  <button type="button" class="sort-button" @click="toggleSort('severity')">
                    严重度
                    <span>{{ sortIndicator('severity') }}</span>
                  </button>
                </th>
                <th class="table-th">错误结果</th>
                <th class="table-th">
                  <button type="button" class="sort-button" @click="toggleSort('status_code')">
                    状态码
                    <span>{{ sortIndicator('status_code') }}</span>
                  </button>
                </th>
                <th class="table-th">用户</th>
                <th class="table-th">API Key</th>
                <th class="table-th">分组</th>
                <th class="table-th">平台</th>
                <th class="table-th">模型</th>
                <th class="table-th">上游账号</th>
                <th class="table-th">
                  <button type="button" class="sort-button" @click="toggleSort('same_kind_count')">
                    同类数量
                    <span>{{ sortIndicator('same_kind_count') }}</span>
                  </button>
                </th>
                <th class="table-th">AI 状态</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-if="items.length === 0">
                <td colspan="16" class="py-16 text-center text-sm text-gray-400 dark:text-dark-500">
                  暂无符合条件的错误记录
                </td>
              </tr>
              <tr
                v-for="item in items"
                :key="item.id"
                class="transition hover:bg-gray-50 dark:hover:bg-dark-800/50"
              >
                <td class="table-td whitespace-nowrap font-mono text-xs">{{ formatDateTime(item.occurred_at) }}</td>
                <td class="table-td">
                  <span class="badge badge-neutral">{{ formatCategory(item.error_category) }}</span>
                </td>
                <td class="table-td">{{ item.error_subcategory || '未细分' }}</td>
                <td class="table-td">{{ item.client_error_subcategory || '--' }}</td>
                <td class="table-td">
                  <div class="max-w-[320px] truncate" :title="item.summary || '暂无摘要'">
                    {{ item.summary || '暂无摘要' }}
                  </div>
                </td>
                <td class="table-td">
                  <span :class="['badge', severityBadgeClass(item.severity)]">{{ item.severity || 'normal' }}</span>
                </td>
                <td class="table-td">
                  <span class="badge badge-result">{{ formatErrorResult(item.error_result) }}</span>
                </td>
                <td class="table-td">
                  <span class="badge badge-status">{{ item.status_code || '--' }}</span>
                </td>
                <td class="table-td">{{ formatEntity(item.user, '未知用户') }}</td>
                <td class="table-td">{{ formatEntity(item.api_key, '未知 Key') }}</td>
                <td class="table-td">{{ formatEntity(item.group, '未分组') }}</td>
                <td class="table-td">{{ item.platform || '未知平台' }}</td>
                <td class="table-td">{{ item.model || '未知模型' }}</td>
                <td class="table-td">{{ formatEntity(item.upstream_account, '未命中上游') }}</td>
                <td class="table-td">{{ item.same_kind_count || 1 }}</td>
                <td class="table-td">
                  <span class="badge badge-ai">{{ formatAIStatus(item.ai_analysis_status) }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="bg-gray-50/50 dark:bg-dark-800/50">
          <Pagination
            v-if="total > 0"
            :total="total"
            :page="page"
            :page-size="numericPageSize()"
            @update:page="handlePageChange"
            @update:pageSize="handlePageSizeUpdate"
          />
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import { adminAPI } from '@/api'
import { listUnifiedErrors, type OpsUnifiedEntityRef, type OpsUnifiedErrorItem, type OpsUnifiedErrorListQueryParams } from '@/api/admin/ops'
import { useAppStore } from '@/stores'
import { formatDateTime, formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'

type GroupOption = {
  id: number
  name: string
}

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const timeRangeOptions = [
  { value: '30m', label: '最近 30 分钟' },
  { value: '1h', label: '最近 1 小时' },
  { value: '6h', label: '最近 6 小时' },
  { value: '24h', label: '最近 24 小时' },
  { value: '7d', label: '最近 7 天' },
  { value: 'custom', label: '自定义' }
]

const errorCategoryOptions = [
  { value: 'client', label: '客户端' },
  { value: 'platform', label: '平台' },
  { value: 'upstream', label: '上游' },
  { value: 'account_pool', label: '账号池' },
  { value: 'rate_limit', label: '限流' },
  { value: 'permission', label: '权限' },
  { value: 'balance', label: '余额' },
  { value: 'config', label: '配置' },
  { value: 'slow_request', label: '慢请求' },
  { value: 'unknown', label: '未知' }
]

const errorResultOptions = [
  { value: 'final_failed', label: '最终失败' },
  { value: 'recovered', label: '已恢复' },
  { value: 'client_aborted', label: '客户端中断' },
  { value: 'unknown', label: '未知' }
]

const severityOptions = [
  { value: 'P0', label: 'P0' },
  { value: 'P1', label: 'P1' },
  { value: 'P2', label: 'P2' },
  { value: 'observe', label: '观察' },
  { value: 'normal', label: '普通' }
]

const clientErrorSubcategoryOptions = [
  { value: 'client_auth_error', label: '认证错误' },
  { value: 'client_rate_limit_error', label: '限流错误' },
  { value: 'client_balance_error', label: '余额错误' },
  { value: 'client_parameter_error', label: '参数错误' },
  { value: 'client_model_error', label: '模型错误' },
  { value: 'client_path_error', label: '路径错误' },
  { value: 'client_context_error', label: '上下文错误' },
  { value: 'client_disconnect_error', label: '客户端断开' },
  { value: 'client_insufficient_evidence', label: '证据不足' }
]

const loading = ref(false)
const hasLoadedOnce = ref(false)
const errorMessage = ref('')
const items = ref<OpsUnifiedErrorItem[]>([])
const total = ref(0)
const groups = ref<GroupOption[]>([])

const timeRange = ref('30m')
const customStartInput = ref('')
const customEndInput = ref('')
const errorCategories = ref<string[]>([])
const errorSubcategoriesInput = ref('')
const clientErrorSubcategories = ref<string[]>([])
const errorResults = ref<string[]>([])
const severities = ref<string[]>([])
const statusCode = ref('')
const userId = ref('')
const apiKeyId = ref('')
const groupId = ref('')
const platform = ref('')
const model = ref('')
const upstreamAccountId = ref('')
const requestId = ref('')
const keyword = ref('')
const aiAnalysis = ref<'all' | 'analyzed' | 'not_analyzed'>('all')
const sortBy = ref<'occurred_at' | 'status_code' | 'severity' | 'same_kind_count'>('occurred_at')
const sortOrder = ref<'asc' | 'desc'>('desc')
const page = ref(1)
const pageSize = ref('20')

const numericPageSize = () => Number.parseInt(pageSize.value, 10) || 20

function splitCSV(value: string): string[] {
  return value.split(',').map((item) => item.trim()).filter(Boolean)
}

function firstQueryValue(value: unknown): string {
  if (Array.isArray(value)) return String(value[0] ?? '')
  return typeof value === 'string' ? value : ''
}

function parsePositiveInt(value: string): number | null {
  if (!value.trim()) return null
  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
}

function initializeFromRoute() {
  timeRange.value = firstQueryValue(route.query.time_range) || '30m'
  customStartInput.value = formatDateTimeLocalInput(firstQueryValue(route.query.start_time))
  customEndInput.value = formatDateTimeLocalInput(firstQueryValue(route.query.end_time))
  errorCategories.value = splitCSV(firstQueryValue(route.query.error_categories))
  errorSubcategoriesInput.value = firstQueryValue(route.query.error_subcategories)
  clientErrorSubcategories.value = splitCSV(firstQueryValue(route.query.client_error_subcategories))
  errorResults.value = splitCSV(firstQueryValue(route.query.error_results))
  severities.value = splitCSV(firstQueryValue(route.query.severity))
  statusCode.value = firstQueryValue(route.query.status_code)
  userId.value = firstQueryValue(route.query.user_id)
  apiKeyId.value = firstQueryValue(route.query.api_key_id)
  groupId.value = firstQueryValue(route.query.group_id)
  platform.value = firstQueryValue(route.query.platform)
  model.value = firstQueryValue(route.query.model)
  upstreamAccountId.value = firstQueryValue(route.query.upstream_account_id)
  requestId.value = firstQueryValue(route.query.request_id)
  keyword.value = firstQueryValue(route.query.keyword)
  aiAnalysis.value = (firstQueryValue(route.query.ai_analysis) as 'all' | 'analyzed' | 'not_analyzed') || 'all'
  sortBy.value = (firstQueryValue(route.query.sort_by) as typeof sortBy.value) || 'occurred_at'
  sortOrder.value = (firstQueryValue(route.query.sort_order) as typeof sortOrder.value) || 'desc'
  page.value = Number.parseInt(firstQueryValue(route.query.page), 10) || 1
  pageSize.value = firstQueryValue(route.query.page_size) || '20'
}

function buildQueryObject(): Record<string, string> {
  const query: Record<string, string> = {
    page: String(page.value),
    page_size: pageSize.value,
    sort_by: sortBy.value,
    sort_order: sortOrder.value,
    ai_analysis: aiAnalysis.value
  }

  if (timeRange.value === 'custom') {
    const startIso = parseDateTimeLocalInput(customStartInput.value)
    const endIso = parseDateTimeLocalInput(customEndInput.value)
    if (startIso) query.start_time = startIso
    if (endIso) query.end_time = endIso
    if (!startIso || !endIso) query.time_range = '30m'
  } else {
    query.time_range = timeRange.value
  }

  if (errorCategories.value.length) query.error_categories = errorCategories.value.join(',')
  if (errorSubcategoriesInput.value.trim()) query.error_subcategories = splitCSV(errorSubcategoriesInput.value).join(',')
  if (clientErrorSubcategories.value.length) query.client_error_subcategories = clientErrorSubcategories.value.join(',')
  if (errorResults.value.length) query.error_results = errorResults.value.join(',')
  if (severities.value.length) query.severity = severities.value.join(',')
  if (statusCode.value.trim()) query.status_code = statusCode.value.trim()
  if (userId.value.trim()) query.user_id = userId.value.trim()
  if (apiKeyId.value.trim()) query.api_key_id = apiKeyId.value.trim()
  if (groupId.value.trim()) query.group_id = groupId.value.trim()
  if (platform.value.trim()) query.platform = platform.value.trim()
  if (model.value.trim()) query.model = model.value.trim()
  if (upstreamAccountId.value.trim()) query.upstream_account_id = upstreamAccountId.value.trim()
  if (requestId.value.trim()) query.request_id = requestId.value.trim()
  if (keyword.value.trim()) query.keyword = keyword.value.trim()

  return query
}

function buildApiParams(): OpsUnifiedErrorListQueryParams {
  const query = buildQueryObject()
  return {
    page: page.value,
    page_size: numericPageSize(),
    time_range: query.time_range,
    start_time: query.start_time,
    end_time: query.end_time,
    error_categories: query.error_categories,
    error_subcategories: query.error_subcategories,
    client_error_subcategories: query.client_error_subcategories,
    error_results: query.error_results,
    severity: query.severity,
    status_code: query.status_code,
    user_id: parsePositiveInt(userId.value),
    api_key_id: parsePositiveInt(apiKeyId.value),
    group_id: parsePositiveInt(groupId.value),
    platform: query.platform,
    model: query.model,
    upstream_account_id: parsePositiveInt(upstreamAccountId.value),
    request_id: query.request_id,
    keyword: query.keyword,
    ai_analysis: aiAnalysis.value,
    sort_by: sortBy.value,
    sort_order: sortOrder.value
  }
}

async function syncRouteQuery() {
  const nextQuery = buildQueryObject()
  if (JSON.stringify(route.query) === JSON.stringify(nextQuery)) return
  await router.replace({ path: '/admin/ops/errors', query: nextQuery })
}

async function fetchErrors() {
  loading.value = true
  errorMessage.value = ''
  try {
    await syncRouteQuery()
    const response = await listUnifiedErrors(buildApiParams())
    items.value = response.items || []
    total.value = response.total || 0
    hasLoadedOnce.value = true
  } catch (err: any) {
    console.error('[OpsUnifiedErrorsView] Failed to fetch unified errors', err)
    items.value = []
    total.value = 0
    errorMessage.value = err?.message || err?.response?.data?.detail || '统一错误列表加载失败'
    appStore.showError(errorMessage.value)
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  void fetchErrors()
}

function resetFilters() {
  timeRange.value = '30m'
  customStartInput.value = ''
  customEndInput.value = ''
  errorCategories.value = []
  errorSubcategoriesInput.value = ''
  clientErrorSubcategories.value = []
  errorResults.value = []
  severities.value = []
  statusCode.value = ''
  userId.value = ''
  apiKeyId.value = ''
  groupId.value = ''
  platform.value = ''
  model.value = ''
  upstreamAccountId.value = ''
  requestId.value = ''
  keyword.value = ''
  aiAnalysis.value = 'all'
  sortBy.value = 'occurred_at'
  sortOrder.value = 'desc'
  page.value = 1
  pageSize.value = '20'
  void fetchErrors()
}

function toggleSort(field: 'occurred_at' | 'status_code' | 'severity' | 'same_kind_count') {
  if (sortBy.value === field) {
    sortOrder.value = sortOrder.value === 'desc' ? 'asc' : 'desc'
  } else {
    sortBy.value = field
    sortOrder.value = field === 'occurred_at' ? 'desc' : 'asc'
  }
  page.value = 1
  void fetchErrors()
}

function sortIndicator(field: 'occurred_at' | 'status_code' | 'severity' | 'same_kind_count'): string {
  if (sortBy.value !== field) return '↕'
  return sortOrder.value === 'desc' ? '↓' : '↑'
}

function handlePageChange(nextPage: number) {
  page.value = nextPage
  void fetchErrors()
}

function handlePageSizeChange() {
  page.value = 1
  void fetchErrors()
}

function handlePageSizeUpdate(nextPageSize: number) {
  pageSize.value = String(nextPageSize)
  page.value = 1
  void fetchErrors()
}

function formatEntity(entity: OpsUnifiedEntityRef | null | undefined, fallback: string): string {
  if (!entity) return fallback
  return entity.display || entity.email || entity.name || `#${entity.id}`
}

function formatCategory(value: string): string {
  return errorCategoryOptions.find((item) => item.value === value)?.label || value || '未分类'
}

function formatErrorResult(value: string): string {
  return errorResultOptions.find((item) => item.value === value)?.label || value || '未知'
}

function formatAIStatus(value: string): string {
  switch (value) {
    case 'completed':
      return '已完成'
    case 'running':
      return '分析中'
    case 'failed':
      return '失败'
    case 'expired':
      return '已过期'
    case 'pending':
      return '待分析'
    default:
      return '未分析'
  }
}

function severityBadgeClass(value: string): string {
  switch (value) {
    case 'P0':
      return 'badge-p0'
    case 'P1':
      return 'badge-p1'
    case 'P2':
      return 'badge-p2'
    case 'observe':
      return 'badge-observe'
    default:
      return 'badge-normal'
  }
}

async function loadGroups() {
  try {
    const list = await adminAPI.groups.getAll()
    groups.value = list.map((group: any) => ({
      id: group.id,
      name: group.name
    }))
  } catch (err) {
    console.error('[OpsUnifiedErrorsView] Failed to load groups', err)
    groups.value = []
  }
}

onMounted(async () => {
  initializeFromRoute()
  await loadGroups()
  await fetchErrors()
})
</script>

<style scoped>
.input {
  @apply w-full rounded-xl border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-dark-600 dark:bg-dark-900 dark:text-white;
}

.input-multi {
  min-height: 112px;
}

.filter-label {
  @apply mb-1 block text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400;
}

.table-th {
  @apply border-b border-gray-200 px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-dark-400;
}

.table-td {
  @apply px-4 py-3 text-sm text-gray-700 dark:text-gray-200;
}

.sort-button {
  @apply inline-flex items-center gap-1 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 transition hover:text-blue-600 dark:text-dark-400 dark:hover:text-blue-300;
}

.badge {
  @apply inline-flex items-center rounded-full px-2 py-1 text-xs font-semibold;
}

.badge-neutral {
  @apply bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200;
}

.badge-result {
  @apply bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-200;
}

.badge-status {
  @apply bg-gray-100 font-mono text-gray-700 dark:bg-dark-700 dark:text-gray-200;
}

.badge-ai {
  @apply bg-purple-50 text-purple-700 dark:bg-purple-900/30 dark:text-purple-200;
}

.badge-p0 {
  @apply bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-200;
}

.badge-p1 {
  @apply bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-200;
}

.badge-p2 {
  @apply bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-200;
}

.badge-observe {
  @apply bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-200;
}

.badge-normal {
  @apply bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200;
}
</style>
