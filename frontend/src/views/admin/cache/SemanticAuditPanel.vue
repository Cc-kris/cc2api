<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <div class="flex flex-wrap items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400">
              <component
                v-for="item in navItems"
                :key="item.key"
                :is="item.to ? 'router-link' : 'span'"
                v-bind="item.to ? { to: item.to } : {}"
                class="rounded-full border px-3 py-1 transition-colors"
                :class="item.active
                  ? 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-700/60 dark:bg-primary-900/10 dark:text-primary-200'
                  : item.to
                    ? 'border-gray-200 text-gray-500 hover:border-primary-200 hover:text-primary-600 dark:border-dark-700 dark:text-gray-400 dark:hover:border-primary-700/60 dark:hover:text-primary-200'
                    : 'border-gray-200 bg-gray-50 text-gray-400 dark:border-dark-700 dark:bg-dark-900/20 dark:text-gray-500'"
              >
                {{ item.label }}
              </component>
            </div>
            <h1 class="mt-4 text-2xl font-semibold text-gray-900 dark:text-white">语义缓存审计</h1>
            <p class="mt-2 max-w-3xl text-sm text-gray-600 dark:text-gray-400">
              查看语义候选的命中记录、审核状态和反馈结果；页面仅展示脱敏摘要，不展示完整原请求和完整命中响应。
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadAudits(true)">
              刷新
            </button>
          </div>
        </div>
      </div>

      <div
        v-if="!canView"
        class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/10 dark:text-amber-200"
      >
        当前账号无权限查看语义缓存审计。
      </div>

      <div
        v-else-if="!canManage"
        class="rounded-xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800 dark:border-blue-900/50 dark:bg-blue-900/10 dark:text-blue-200"
      >
        当前账号仅可查看语义缓存审计，不能执行审核或反馈。
      </div>

      <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">筛选条件</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">支持按时间、平台、模型、API Key、审核状态、决策和相似度范围筛选。</p>
        </div>
        <div class="space-y-4 px-6 py-5">
          <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">开始时间</span>
              <input v-model="filters.startTime" type="datetime-local" class="input" :disabled="loading || !canView" />
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">结束时间</span>
              <input v-model="filters.endTime" type="datetime-local" class="input" :disabled="loading || !canView" />
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">平台</span>
              <select v-model="filters.platform" class="input" :disabled="loading || !canView">
                <option value="">全部</option>
                <option v-for="item in platformOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
              </select>
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">审核状态</span>
              <select v-model="filters.reviewStatus" class="input" :disabled="loading || !canView">
                <option value="">全部</option>
                <option v-for="item in reviewOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
              </select>
            </label>
          </div>

          <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">模型</span>
              <input v-model.trim="filters.model" type="text" class="input" :disabled="loading || !canView" placeholder="例如 gpt-5.5" />
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">API Key ID</span>
              <input v-model.trim="filters.apiKeyId" type="number" min="1" step="1" class="input" :disabled="loading || !canView" placeholder="例如 123" />
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">决策</span>
              <select v-model="filters.decision" class="input" :disabled="loading || !canView">
                <option value="">全部</option>
                <option v-for="item in decisionOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
              </select>
            </label>
            <div class="grid grid-cols-2 gap-3">
              <label class="block">
                <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">最低相似度</span>
                <input v-model.trim="filters.minSimilarity" type="number" min="0" max="1" step="0.0001" class="input" :disabled="loading || !canView" placeholder="0.9500" />
              </label>
              <label class="block">
                <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">最高相似度</span>
                <input v-model.trim="filters.maxSimilarity" type="number" min="0" max="1" step="0.0001" class="input" :disabled="loading || !canView" placeholder="1.0000" />
              </label>
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-primary" :disabled="loading || !canView || Boolean(validationError)" @click="applyFilters">
              {{ t('common.search') }}
            </button>
            <button type="button" class="btn btn-secondary" :disabled="loading || !canView" @click="resetFilters">
              {{ t('common.reset') }}
            </button>
          </div>

          <p v-if="validationError" class="text-sm text-red-600 dark:text-red-300">{{ validationError }}</p>
        </div>
      </div>

      <div v-if="canView" class="grid grid-cols-1 gap-4 xl:grid-cols-4">
        <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-gray-400">当前页记录</p>
          <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ formatInteger(rows.length) }}</p>
        </div>
        <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-gray-400">总记录数</p>
          <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ formatInteger(pagination.total) }}</p>
        </div>
        <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-gray-400">待审核</p>
          <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ formatInteger(pendingCount) }}</p>
        </div>
        <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-gray-400">已反馈</p>
          <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ formatInteger(feedbackCount) }}</p>
        </div>
      </div>

      <div v-if="canView" class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">审计列表</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">审核模式、观察模式、灰度命中的真实记录统一在此查看。</p>
            </div>
            <div class="text-xs text-gray-500 dark:text-gray-400">分页大小：{{ pagination.page_size }}</div>
          </div>
        </div>

        <div class="px-6 py-5">
          <div v-if="loadError" class="mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200">
            <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <span>{{ loadError }}</span>
              <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadAudits(true)">
                重试
              </button>
            </div>
          </div>

          <div class="space-y-4">
            <div
              v-for="row in rows"
              :key="row.id"
              class="rounded-xl border border-gray-200 p-4 shadow-sm dark:border-dark-700"
            >
              <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
                <div class="space-y-3">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-200">#{{ row.id }}</span>
                    <span class="rounded-full px-2.5 py-1 text-xs font-medium" :class="decisionClass(row.decision)">{{ decisionLabel(row.decision) }}</span>
                    <span class="rounded-full px-2.5 py-1 text-xs font-medium" :class="reviewClass(row.review_status)">{{ reviewLabel(row.review_status) }}</span>
                    <span
                      v-if="row.feedback_type && row.feedback_type !== 'none'"
                      class="rounded-full bg-amber-50 px-2.5 py-1 text-xs font-medium text-amber-800 dark:bg-amber-900/10 dark:text-amber-200"
                    >
                      {{ feedbackLabel(row.feedback_type) }}
                    </span>
                  </div>

                  <div class="grid grid-cols-1 gap-3 text-sm text-gray-600 dark:text-gray-300 md:grid-cols-2 xl:grid-cols-4">
                    <div><span class="text-gray-500 dark:text-gray-400">请求 ID：</span><span class="font-medium text-gray-900 dark:text-white">{{ row.request_id || '--' }}</span></div>
                    <div><span class="text-gray-500 dark:text-gray-400">候选 ID：</span><span class="font-medium text-gray-900 dark:text-white">{{ row.semantic_entry_id ?? '--' }}</span></div>
                    <div><span class="text-gray-500 dark:text-gray-400">平台 / 模型：</span><span class="font-medium text-gray-900 dark:text-white">{{ row.platform }} / {{ row.model }}</span></div>
                    <div><span class="text-gray-500 dark:text-gray-400">API Key：</span><span class="font-medium text-gray-900 dark:text-white">{{ row.api_key || '--' }}</span></div>
                    <div><span class="text-gray-500 dark:text-gray-400">相似度：</span><span class="font-medium text-gray-900 dark:text-white">{{ formatSimilarity(row.similarity) }}</span></div>
                    <div><span class="text-gray-500 dark:text-gray-400">发生时间：</span><span class="font-medium text-gray-900 dark:text-white">{{ formatDateTimeValue(row.occurred_at) || '--' }}</span></div>
                    <div><span class="text-gray-500 dark:text-gray-400">操作人：</span><span class="font-medium text-gray-900 dark:text-white">{{ row.operator_user_id ? `#${row.operator_user_id}` : '--' }}</span></div>
                    <div><span class="text-gray-500 dark:text-gray-400">更新时间：</span><span class="font-medium text-gray-900 dark:text-white">{{ formatDateTimeValue(row.updated_at) || '--' }}</span></div>
                  </div>
                </div>

                <div class="flex flex-wrap items-center gap-2 xl:justify-end">
                  <button type="button" class="btn btn-secondary" :disabled="!canManage || submitting" @click="openReviewDialog(row)">
                    审核
                  </button>
                  <button type="button" class="btn btn-secondary" :disabled="!canManage || submitting" @click="openFeedbackDialog(row)">
                    反馈
                  </button>
                </div>
              </div>

              <div class="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-2">
                <div class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/30">
                  <div class="flex items-center justify-between gap-3">
                    <p class="text-sm font-medium text-gray-900 dark:text-white">请求摘要</p>
                    <span class="text-xs text-gray-500 dark:text-gray-400">已脱敏</span>
                  </div>
                  <p class="mt-2 whitespace-pre-wrap break-words text-sm text-gray-600 dark:text-gray-300">{{ row.source_summary || '--' }}</p>
                </div>
                <div class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/30">
                  <div class="flex items-center justify-between gap-3">
                    <p class="text-sm font-medium text-gray-900 dark:text-white">命中摘要</p>
                    <span class="text-xs text-gray-500 dark:text-gray-400">已脱敏</span>
                  </div>
                  <p class="mt-2 whitespace-pre-wrap break-words text-sm text-gray-600 dark:text-gray-300">{{ row.target_summary || '--' }}</p>
                </div>
              </div>

              <div v-if="row.block_reason || row.feedback_note || row.auto_close_reason" class="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-3">
                <div v-if="row.block_reason" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
                  <p class="text-sm font-medium text-gray-900 dark:text-white">阻断原因</p>
                  <p class="mt-2 whitespace-pre-wrap break-words text-sm text-gray-600 dark:text-gray-300">{{ row.block_reason }}</p>
                </div>
                <div v-if="row.feedback_note" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
                  <p class="text-sm font-medium text-gray-900 dark:text-white">反馈说明</p>
                  <p class="mt-2 whitespace-pre-wrap break-words text-sm text-gray-600 dark:text-gray-300">{{ row.feedback_note }}</p>
                </div>
                <div v-if="row.auto_close_reason" class="rounded-xl border border-red-200 bg-red-50 p-4 dark:border-red-900/60 dark:bg-red-900/10">
                  <p class="text-sm font-medium text-red-700 dark:text-red-200">自动关闭原因</p>
                  <p class="mt-2 whitespace-pre-wrap break-words text-sm text-red-700 dark:text-red-200">{{ row.auto_close_reason }}</p>
                </div>
              </div>
            </div>

            <div
              v-if="!loading && rows.length === 0"
              class="rounded-xl border border-dashed border-gray-300 px-4 py-10 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400"
            >
              当前筛选条件下没有语义审计记录。
            </div>
          </div>
        </div>

        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </div>
    </div>

    <BaseDialog :show="dialog.visible" width="narrow" :title="dialogTitle" @close="closeDialog">
      <div class="space-y-4">
        <div v-if="dialog.record" class="rounded-xl border border-gray-200 bg-gray-50 p-4 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-900/30 dark:text-gray-300">
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium text-gray-900 dark:text-white">#{{ dialog.record.id }}</span>
            <span>{{ dialog.record.request_id }}</span>
          </div>
          <p class="mt-2">相似度：{{ formatSimilarity(dialog.record.similarity) }} · 当前审核状态：{{ reviewLabel(dialog.record.review_status) }}</p>
        </div>

        <label class="block">
          <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ dialog.mode === 'review' ? '审核结果' : '反馈类型' }}</span>
          <select v-model="dialog.value" class="input">
            <option v-for="item in dialogOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
          </select>
        </label>

        <label class="block">
          <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ dialog.mode === 'review' ? '审核说明（可选）' : '反馈说明（必填）' }}</span>
          <textarea
            v-model.trim="dialog.note"
            rows="4"
            class="input min-h-[112px]"
            :placeholder="dialog.mode === 'review' ? '例如：语义一致，可复用' : '例如：语义不同，不能复用'"
          />
        </label>

        <p v-if="dialogError" class="text-sm text-red-600 dark:text-red-300">{{ dialogError }}</p>

        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="submitting" @click="closeDialog">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="submitting || !dialogSubmitReady" @click="submitDialog">
            {{ submitting ? '提交中...' : '确认提交' }}
          </button>
        </div>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import { adminAPI } from '@/api/admin'
import type {
  SemanticCacheAuditDecision,
  SemanticCacheAuditFeedbackRequest,
  SemanticCacheAuditFeedbackType,
  SemanticCacheAuditRecord,
  SemanticCacheAuditReviewRequest,
  SemanticCacheAuditReviewStatus
} from '@/api/admin/cache'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime as formatDateTimeValue } from '@/utils/format'

type AuditFiltersForm = {
  startTime: string
  endTime: string
  platform: string
  model: string
  apiKeyId: string
  reviewStatus: SemanticCacheAuditReviewStatus | ''
  decision: SemanticCacheAuditDecision | ''
  minSimilarity: string
  maxSimilarity: string
}

type DialogMode = 'review' | 'feedback' | null

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const loading = ref(false)
const loadError = ref('')
const rows = ref<SemanticCacheAuditRecord[]>([])
const submitting = ref(false)
const dialogError = ref('')

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
})

const filters = reactive<AuditFiltersForm>({
  startTime: '',
  endTime: '',
  platform: '',
  model: '',
  apiKeyId: '',
  reviewStatus: '',
  decision: '',
  minSimilarity: '',
  maxSimilarity: ''
})

const dialog = reactive<{
  visible: boolean
  mode: DialogMode
  record: SemanticCacheAuditRecord | null
  value: string
  note: string
}>({
  visible: false,
  mode: null,
  record: null,
  value: '',
  note: ''
})

const navItems = computed(() => [
  { key: 'home', to: '/admin/settings/cache', label: '管理首页', active: false },
  { key: 'stats', to: '/admin/settings/cache/stats', label: '缓存统计', active: false },
  { key: 'semantic', to: '/admin/settings/cache/semantic', label: '语义配置', active: false },
  { key: 'audit', to: '/admin/settings/cache/semantic-audits', label: '语义审计', active: true }
])

const viewerRole = computed(() => normalizeRole(String((authStore.user as { role?: string } | null)?.role || '')))
const canView = computed(() => ['owner', 'ops', 'business', 'support'].includes(viewerRole.value))
const canManage = computed(() => ['owner', 'ops'].includes(viewerRole.value))

const platformOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'claude', label: 'Claude' },
  { value: 'gemini', label: 'Gemini' }
]

const reviewOptions: Array<{ value: SemanticCacheAuditReviewStatus; label: string }> = [
  { value: 'pending', label: '未审核' },
  { value: 'reusable', label: '可复用' },
  { value: 'not_reusable', label: '不可复用' },
  { value: 'disputed', label: '争议' }
]

const decisionOptions: Array<{ value: SemanticCacheAuditDecision; label: string }> = [
  { value: 'observe', label: '观察' },
  { value: 'hit', label: '命中' },
  { value: 'miss', label: '未命中' },
  { value: 'blocked', label: '已阻断' },
  { value: 'rollback', label: '已回滚' }
]

const reviewActionOptions: Array<{ value: SemanticCacheAuditReviewRequest['review_status']; label: string }> = [
  { value: 'reusable', label: '可复用' },
  { value: 'not_reusable', label: '不可复用' },
  { value: 'disputed', label: '争议' }
]

const feedbackOptions: Array<{ value: SemanticCacheAuditFeedbackRequest['feedback_type']; label: string }> = [
  { value: 'wrong_hit', label: '误命中' },
  { value: 'complaint', label: '用户投诉' },
  { value: 'manual_mark', label: '人工标记' }
]

const pendingCount = computed(() => rows.value.filter((row) => row.review_status === 'pending').length)
const feedbackCount = computed(() => rows.value.filter((row) => row.feedback_type && row.feedback_type !== 'none').length)

const validationError = computed(() => {
  if (!filters.startTime && !filters.endTime && !filters.minSimilarity && !filters.maxSimilarity && !filters.apiKeyId) {
    return ''
  }

  if ((filters.startTime && !filters.endTime) || (!filters.startTime && filters.endTime)) {
    return '开始时间和结束时间必须同时填写。'
  }
  if (filters.startTime && filters.endTime) {
    const start = new Date(filters.startTime)
    const end = new Date(filters.endTime)
    if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) {
      return '时间格式不正确。'
    }
    if (start.getTime() > end.getTime()) {
      return '开始时间不能晚于结束时间。'
    }
    if (end.getTime() - start.getTime() > 31 * 24 * 60 * 60 * 1000) {
      return '时间范围不能超过 31 天。'
    }
  }

  if (filters.apiKeyId) {
    const apiKeyId = Number(filters.apiKeyId)
    if (!Number.isInteger(apiKeyId) || apiKeyId <= 0) {
      return 'API Key ID 必须是正整数。'
    }
  }

  const minSimilarity = parseOptionalNumber(filters.minSimilarity)
  const maxSimilarity = parseOptionalNumber(filters.maxSimilarity)
  if (minSimilarity !== undefined && (minSimilarity < 0 || minSimilarity > 1)) {
    return '最低相似度必须在 0 到 1 之间。'
  }
  if (maxSimilarity !== undefined && (maxSimilarity < 0 || maxSimilarity > 1)) {
    return '最高相似度必须在 0 到 1 之间。'
  }
  if (minSimilarity !== undefined && maxSimilarity !== undefined && minSimilarity > maxSimilarity) {
    return '最低相似度不能大于最高相似度。'
  }
  return ''
})

const dialogTitle = computed(() => {
  if (dialog.mode === 'review') return '审核语义候选'
  if (dialog.mode === 'feedback') return '提交误命中反馈'
  return '操作语义审计'
})

const dialogOptions = computed(() => {
  return dialog.mode === 'review' ? reviewActionOptions : feedbackOptions
})

const dialogSubmitReady = computed(() => {
  if (!dialog.visible || !dialog.mode || !dialog.record || !dialog.value) return false
  if (dialog.mode === 'feedback') {
    return dialog.note.trim().length > 0 && dialog.note.trim().length <= 500
  }
  return dialog.note.trim().length <= 500
})

function normalizeRole(role: string): string {
  const value = role.trim().toLowerCase()
  if (!value || value === 'admin') return 'owner'
  if (['ops', 'operation', 'operator', 'operations'].includes(value)) return 'ops'
  if (['business', 'business_operator', 'business-operator', '运营', 'yunying'].includes(value)) return 'business'
  if (['customer_service', 'customer-service', 'customerservice', 'support', 'service', 'cs'].includes(value)) return 'support'
  return value
}

function parseOptionalNumber(value: string): number | undefined {
  if (!value.trim()) return undefined
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : Number.NaN
}

function toISOString(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
}

function buildParams() {
  return {
    page: pagination.page,
    page_size: pagination.page_size,
    start_time: toISOString(filters.startTime),
    end_time: toISOString(filters.endTime),
    platform: filters.platform || undefined,
    model: filters.model.trim() || undefined,
    api_key_id: filters.apiKeyId ? Number(filters.apiKeyId) : undefined,
    review_status: filters.reviewStatus || undefined,
    decision: filters.decision || undefined,
    min_similarity: filters.minSimilarity ? Number(filters.minSimilarity) : undefined,
    max_similarity: filters.maxSimilarity ? Number(filters.maxSimilarity) : undefined
  }
}

function formatInteger(value: number): string {
  return new Intl.NumberFormat('zh-CN').format(value || 0)
}

function formatSimilarity(value: number): string {
  if (!Number.isFinite(value)) return '--'
  return value.toFixed(4)
}

function decisionLabel(value: SemanticCacheAuditDecision): string {
  return decisionOptions.find((item) => item.value === value)?.label ?? value
}

function reviewLabel(value: SemanticCacheAuditReviewStatus): string {
  return reviewOptions.find((item) => item.value === value)?.label ?? value
}

function feedbackLabel(value?: SemanticCacheAuditFeedbackType): string {
  return feedbackOptions.find((item) => item.value === value)?.label ?? value || '--'
}

function decisionClass(value: SemanticCacheAuditDecision): string {
  switch (value) {
    case 'hit':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/10 dark:text-emerald-200'
    case 'blocked':
    case 'rollback':
      return 'bg-red-50 text-red-700 dark:bg-red-900/10 dark:text-red-200'
    case 'miss':
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
    default:
      return 'bg-blue-50 text-blue-700 dark:bg-blue-900/10 dark:text-blue-200'
  }
}

function reviewClass(value: SemanticCacheAuditReviewStatus): string {
  switch (value) {
    case 'reusable':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/10 dark:text-emerald-200'
    case 'not_reusable':
      return 'bg-red-50 text-red-700 dark:bg-red-900/10 dark:text-red-200'
    case 'disputed':
      return 'bg-amber-50 text-amber-800 dark:bg-amber-900/10 dark:text-amber-200'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
  }
}

function resetFilters(): void {
  filters.startTime = ''
  filters.endTime = ''
  filters.platform = ''
  filters.model = ''
  filters.apiKeyId = ''
  filters.reviewStatus = ''
  filters.decision = ''
  filters.minSimilarity = ''
  filters.maxSimilarity = ''
  loadError.value = ''
  pagination.page = 1
  void loadAudits(true)
}

function applyFilters(): void {
  if (validationError.value) {
    loadError.value = validationError.value
    return
  }
  void loadAudits(true)
}

function openReviewDialog(row: SemanticCacheAuditRecord): void {
  dialog.visible = true
  dialog.mode = 'review'
  dialog.record = row
  dialog.value = row.review_status === 'pending' ? 'reusable' : row.review_status
  dialog.note = ''
  dialogError.value = ''
}

function openFeedbackDialog(row: SemanticCacheAuditRecord): void {
  dialog.visible = true
  dialog.mode = 'feedback'
  dialog.record = row
  dialog.value = row.feedback_type && row.feedback_type !== 'none' ? row.feedback_type : 'wrong_hit'
  dialog.note = ''
  dialogError.value = ''
}

function closeDialog(): void {
  if (submitting.value) return
  dialog.visible = false
  dialog.mode = null
  dialog.record = null
  dialog.value = ''
  dialog.note = ''
  dialogError.value = ''
}

async function submitDialog(): Promise<void> {
  if (!dialog.record || !dialog.mode || !dialogSubmitReady.value) return

  if (dialog.note.trim().length > 500) {
    dialogError.value = '说明不能超过 500 个字符。'
    return
  }

  submitting.value = true
  dialogError.value = ''
  try {
    if (dialog.mode === 'review') {
      await adminAPI.cache.reviewSemanticAudit(dialog.record.id, {
        review_status: dialog.value as SemanticCacheAuditReviewRequest['review_status'],
        note: dialog.note.trim() || undefined
      })
      appStore.showSuccess('语义候选审核已提交')
    } else {
      if (!dialog.note.trim()) {
        dialogError.value = '反馈说明不能为空。'
        return
      }
      await adminAPI.cache.feedbackSemanticAudit(dialog.record.id, {
        feedback_type: dialog.value as SemanticCacheAuditFeedbackRequest['feedback_type'],
        note: dialog.note.trim()
      })
      appStore.showSuccess('语义误命中反馈已提交')
    }
    closeDialog()
    await loadAudits(false)
  } catch (error) {
    dialogError.value = extractApiErrorMessage(error, dialog.mode === 'review' ? '语义候选审核失败' : '语义误命中反馈失败')
    appStore.showError(dialogError.value)
  } finally {
    submitting.value = false
  }
}

async function loadAudits(resetPage = false): Promise<void> {
  if (!canView.value) {
    rows.value = []
    pagination.total = 0
    return
  }
  if (validationError.value) {
    loadError.value = validationError.value
    return
  }
  if (resetPage) {
    pagination.page = 1
  }
  loading.value = true
  loadError.value = ''
  try {
    const { data } = await adminAPI.cache.listSemanticAudits(buildParams())
    rows.value = Array.isArray(data.items) ? data.items : []
    pagination.total = Number(data.total || 0)
    pagination.page = Number(data.page || pagination.page || 1)
    pagination.page_size = Number(data.page_size || pagination.page_size || 20)
    pagination.pages = 'pages' in data && typeof data.pages === 'number'
      ? data.pages
      : (pagination.total > 0 ? Math.ceil(pagination.total / pagination.page_size) : 0)
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, '语义缓存审计加载失败')
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number): void {
  pagination.page = page
  void loadAudits()
}

function handlePageSizeChange(pageSize: number): void {
  pagination.page = 1
  pagination.page_size = pageSize
  void loadAudits()
}

onMounted(() => {
  void loadAudits(true)
})
</script>
