<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <section class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div class="space-y-2">
            <div class="flex flex-wrap items-center gap-2">
              <span :class="['inline-flex items-center rounded-full px-3 py-1 text-xs font-semibold', resultBadgeClass]">
                {{ resultLabel }}
              </span>
              <span :class="['inline-flex items-center rounded-full px-3 py-1 text-xs font-semibold', aiStatusBadgeClass]">
                {{ aiStatusLabel }}
              </span>
            </div>
            <div>
              <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
                {{ detail?.conclusion.title || t('admin.ops.unifiedErrorDetail.title') }}
              </h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.unifiedErrorDetail.description') }}
              </p>
            </div>
            <p class="text-sm text-gray-600 dark:text-gray-300">
              {{ detail?.conclusion.summary || t('admin.ops.unifiedErrorDetail.emptySummary') }}
            </p>
          </div>

          <div class="flex flex-wrap items-center gap-3">
            <button type="button" class="btn btn-secondary btn-sm" @click="goBack">
              {{ t('admin.ops.unifiedErrorDetail.back') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="!detail?.request_chain.request_id"
              @click="copyRequestId"
            >
              {{ t('admin.ops.unifiedErrorDetail.copyRequestId') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" @click="focusRawRecord">
              {{ t('admin.ops.unifiedErrorDetail.viewLogs') }}
            </button>
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="manualAIActionDisabled"
              :title="manualAIActionDisabledReason || undefined"
              @click="runManualAIAnalysis"
            >
              {{ t('admin.ops.unifiedErrorDetail.manualAnalysis') }}
            </button>
          </div>
        </div>

        <div v-if="manualAIActionDisabledReason" class="mt-3 text-xs text-gray-500 dark:text-gray-400">
          {{ manualAIActionDisabledReason }}
        </div>

        <div v-if="loading" class="mt-6 flex items-center justify-center py-16">
          <div class="flex flex-col items-center gap-3">
            <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
            <div class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</div>
          </div>
        </div>

        <div v-else-if="errorMessage" class="mt-6 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
          {{ errorMessage }}
        </div>
      </section>

      <template v-if="detail && !loading">
        <section class="grid grid-cols-1 gap-4 lg:grid-cols-4">
          <div class="summary-card">
            <div class="summary-card__label">{{ t('admin.ops.unifiedErrorDetail.impact.sameKindCount') }}</div>
            <div class="summary-card__value">{{ detail.impact_scope.same_kind_count }}</div>
          </div>
          <div class="summary-card">
            <div class="summary-card__label">{{ t('admin.ops.unifiedErrorDetail.impact.affectedUsers') }}</div>
            <div class="summary-card__value">{{ detail.impact_scope.affected_users }}</div>
          </div>
          <div class="summary-card">
            <div class="summary-card__label">{{ t('admin.ops.unifiedErrorDetail.impact.affectedApiKeys') }}</div>
            <div class="summary-card__value">{{ detail.impact_scope.affected_api_keys }}</div>
          </div>
          <div class="summary-card">
            <div class="summary-card__label">{{ t('admin.ops.unifiedErrorDetail.impact.affectedAccounts') }}</div>
            <div class="summary-card__value">{{ detail.impact_scope.affected_upstream_accounts }}</div>
          </div>
        </section>

        <section class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.35fr)_minmax(0,1fr)]">
          <div class="space-y-6">
            <article class="detail-card">
              <h2 class="detail-card__title">{{ t('admin.ops.unifiedErrorDetail.sections.requestChain') }}</h2>
              <div class="detail-grid">
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.requestId') }}</div>
                  <div class="detail-field__value font-mono">{{ fallback(detail.request_chain.request_id) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.clientRequestId') }}</div>
                  <div class="detail-field__value font-mono">{{ fallback(detail.request_chain.client_request_id) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.user') }}</div>
                  <div class="detail-field__value">{{ entityLabel(detail.request_chain.user, t('admin.ops.unifiedErrorDetail.unknownUser')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.apiKey') }}</div>
                  <div class="detail-field__value">{{ entityLabel(detail.request_chain.api_key, t('admin.ops.unifiedErrorDetail.unknownApiKey')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.group') }}</div>
                  <div class="detail-field__value">{{ entityLabel(detail.request_chain.group, t('admin.ops.unifiedErrorDetail.ungrouped')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.upstreamAccount') }}</div>
                  <div class="detail-field__value">{{ entityLabel(detail.request_chain.upstream_account, t('admin.ops.unifiedErrorDetail.hiddenUpstreamAccount')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.platform') }}</div>
                  <div class="detail-field__value">{{ fallback(detail.request_chain.platform, t('admin.ops.unifiedErrorDetail.unknown')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.model') }}</div>
                  <div class="detail-field__value font-mono">{{ fallback(detail.request_chain.model, t('admin.ops.unifiedErrorDetail.unknown')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.requestedModel') }}</div>
                  <div class="detail-field__value font-mono">{{ fallback(detail.request_chain.requested_model, t('admin.ops.unifiedErrorDetail.unknown')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.upstreamModel') }}</div>
                  <div class="detail-field__value font-mono">{{ fallback(detail.request_chain.upstream_model, t('admin.ops.unifiedErrorDetail.unknown')) }}</div>
                </div>
                <div class="detail-field detail-field--full">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.requestPath') }}</div>
                  <div class="detail-field__value font-mono break-all">{{ fallback(detail.request_chain.request_path) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.inboundEndpoint') }}</div>
                  <div class="detail-field__value font-mono break-all">{{ fallback(detail.request_chain.inbound_endpoint) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.upstreamEndpoint') }}</div>
                  <div class="detail-field__value font-mono break-all">{{ fallback(detail.request_chain.upstream_endpoint) }}</div>
                </div>
              </div>
            </article>

            <article class="detail-card">
              <h2 class="detail-card__title">{{ t('admin.ops.unifiedErrorDetail.sections.classification') }}</h2>
              <div class="detail-grid">
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.errorCategory') }}</div>
                  <div class="detail-field__value">{{ fallback(detail.classification.error_category, t('admin.ops.unifiedErrorDetail.unclassified')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.errorSubcategory') }}</div>
                  <div class="detail-field__value">{{ fallback(detail.classification.error_subcategory, t('admin.ops.unifiedErrorDetail.unclassified')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.clientSubcategory') }}</div>
                  <div class="detail-field__value">{{ fallback(detail.classification.client_error_subcategory, t('admin.ops.unifiedErrorDetail.unclassified')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.confidence') }}</div>
                  <div class="detail-field__value">{{ fallback(detail.classification.classification_confidence, t('admin.ops.unifiedErrorDetail.unknown')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.statusCode') }}</div>
                  <div class="detail-field__value">{{ numberFallback(detail.classification.status_code) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.clientStatusCode') }}</div>
                  <div class="detail-field__value">{{ numberFallback(detail.classification.client_status_code) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.errorOwner') }}</div>
                  <div class="detail-field__value">{{ fallback(detail.classification.error_owner, t('admin.ops.unifiedErrorDetail.unknown')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.errorSource') }}</div>
                  <div class="detail-field__value">{{ fallback(detail.classification.error_source, t('admin.ops.unifiedErrorDetail.unknown')) }}</div>
                </div>
                <div class="detail-field detail-field--full">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.classificationReason') }}</div>
                  <div class="detail-field__value whitespace-pre-wrap">{{ fallback(detail.classification.classification_reason, t('admin.ops.unifiedErrorDetail.noReason')) }}</div>
                </div>
                <div v-if="detail.classification.missing_evidence?.length" class="detail-field detail-field--full">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.missingEvidence') }}</div>
                  <ul class="mt-2 space-y-2 text-sm text-gray-700 dark:text-gray-200">
                    <li v-for="item in detail.classification.missing_evidence" :key="item" class="flex gap-2">
                      <span class="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-amber-500" />
                      <span>{{ item }}</span>
                    </li>
                  </ul>
                </div>
              </div>
            </article>

            <article class="detail-card">
              <h2 class="detail-card__title">{{ t('admin.ops.unifiedErrorDetail.sections.recovery') }}</h2>
              <div class="detail-grid">
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.finalResult') }}</div>
                  <div class="detail-field__value">{{ resultLabel }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.resolved') }}</div>
                  <div class="detail-field__value">{{ detail.recovery.resolved ? t('common.yes') : t('common.no') }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.recovered') }}</div>
                  <div class="detail-field__value">{{ detail.recovery.recovered ? t('common.yes') : t('common.no') }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.recoveryMethod') }}</div>
                  <div class="detail-field__value">{{ fallback(detail.recovery.recovery_method, t('admin.ops.unifiedErrorDetail.unconfirmed')) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.resolvedAt') }}</div>
                  <div class="detail-field__value">{{ timeFallback(detail.recovery.resolved_at) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.affectsUser') }}</div>
                  <div class="detail-field__value">{{ detail.conclusion.affects_user ? t('common.yes') : t('common.no') }}</div>
                </div>
              </div>

              <div class="mt-5">
                <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.recommendedActions') }}</div>
                <ul v-if="detail.conclusion.recommended_actions?.length" class="mt-3 space-y-2 text-sm text-gray-700 dark:text-gray-200">
                  <li v-for="item in detail.conclusion.recommended_actions" :key="item" class="flex gap-2">
                    <span class="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500" />
                    <span>{{ item }}</span>
                  </li>
                </ul>
                <div v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.unifiedErrorDetail.noRecommendations') }}
                </div>
              </div>
            </article>
          </div>

          <div class="space-y-6">
            <article class="detail-card">
              <h2 class="detail-card__title">{{ t('admin.ops.unifiedErrorDetail.sections.aiAnalysis') }}</h2>
              <div class="detail-grid">
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.aiStatus') }}</div>
                  <div class="detail-field__value">{{ aiStatusLabel }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.aiTaskId') }}</div>
                  <div class="detail-field__value">{{ numberFallback(detail.ai_analysis.task_id) }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.analysisTime') }}</div>
                  <div class="detail-field__value">{{ aiAnalysisTimeLabel }}</div>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.analysisRange') }}</div>
                  <div class="detail-field__value">{{ aiAnalysisRangeLabel }}</div>
                </div>
                <div class="detail-field detail-field--full">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.aiSummary') }}</div>
                  <div class="detail-field__value whitespace-pre-wrap">{{ aiAnalysisSummary }}</div>
                </div>
              </div>

              <div v-if="aiReportLoading" class="mt-4 rounded-2xl bg-gray-50 px-4 py-3 text-sm text-gray-600 dark:bg-dark-800/70 dark:text-gray-300">
                {{ t('admin.ops.unifiedErrorDetail.analysisLoading') }}
              </div>

              <div v-else-if="aiReportError" class="mt-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
                {{ aiReportError }}
              </div>

              <div v-else-if="aiReportStateMessage" :class="['mt-4 rounded-2xl px-4 py-3 text-sm', aiReportStateClass]">
                {{ aiReportStateMessage }}
              </div>

              <div v-if="aiTaskDetail?.report" class="mt-5 space-y-4">
                <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
                    <div class="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                      <span>{{ t('admin.ops.unifiedErrorDetail.fields.analysisConfidence') }}</span>
                      <span
                        v-if="aiAnalysisConfidenceBadgeLabel"
                        :class="['rounded-full px-2 py-0.5 text-[11px] font-semibold normal-case tracking-normal', aiAnalysisConfidenceBadgeClass]"
                      >
                        {{ aiAnalysisConfidenceBadgeLabel }}
                      </span>
                    </div>
                    <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">
                      {{ aiAnalysisConfidenceText }}
                    </div>
                    <div v-if="aiAnalysisConfidenceLevel === 'low'" class="mt-2 text-xs text-amber-700 dark:text-amber-300">
                      {{ t('admin.ops.unifiedErrorDetail.lowConfidenceHint') }}
                    </div>
                  </div>

                  <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
                    <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                      {{ t('admin.ops.unifiedErrorDetail.fields.analysisImpact') }}
                    </div>
                    <ul v-if="aiAnalysisImpactItems.length" class="mt-2 space-y-2 text-sm text-gray-800 dark:text-gray-100">
                      <li v-for="item in aiAnalysisImpactItems" :key="item.label" class="flex items-center justify-between gap-3">
                        <span>{{ item.label }}</span>
                        <span class="font-semibold">{{ item.value }}</span>
                      </li>
                    </ul>
                    <div v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">
                      {{ t('admin.ops.unifiedErrorDetail.noImpactScope') }}
                    </div>
                  </div>
                </div>

                <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
                  <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.unifiedErrorDetail.fields.analysisEvidence') }}
                  </div>
                  <ul v-if="aiAnalysisEvidenceItems.length" class="mt-2 space-y-2 text-sm text-gray-800 dark:text-gray-100">
                    <li v-for="item in aiAnalysisEvidenceItems" :key="item" class="flex gap-2">
                      <span class="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500" />
                      <span>{{ item }}</span>
                    </li>
                  </ul>
                  <div v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.unifiedErrorDetail.noEvidence') }}
                  </div>
                </div>

                <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
                  <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.unifiedErrorDetail.fields.analysisActions') }}
                  </div>
                  <ul v-if="aiAnalysisActions.length" class="mt-2 space-y-2 text-sm text-gray-800 dark:text-gray-100">
                    <li v-for="item in aiAnalysisActions" :key="item" class="flex gap-2">
                      <span class="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500" />
                      <span>{{ item }}</span>
                    </li>
                  </ul>
                  <div v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.ops.unifiedErrorDetail.noRecommendations') }}
                  </div>
                </div>
              </div>
            </article>

            <article ref="rawRecordSection" class="detail-card">
              <h2 class="detail-card__title">{{ t('admin.ops.unifiedErrorDetail.sections.rawRecord') }}</h2>
              <div class="space-y-4">
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.rawErrorBodyPreview') }}</div>
                  <pre class="detail-pre"><code>{{ prettyPayload(detail.raw_record.error_body_preview) }}</code></pre>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.rawUpstreamErrors') }}</div>
                  <pre class="detail-pre"><code>{{ prettyPayload(detail.raw_record.upstream_errors) }}</code></pre>
                </div>
                <div class="detail-field">
                  <div class="detail-field__label">{{ t('admin.ops.unifiedErrorDetail.fields.rawErrorLog') }}</div>
                  <pre class="detail-pre"><code>{{ prettyPayload(detail.raw_record.error_log) }}</code></pre>
                </div>
              </div>
            </article>

            <article class="detail-card">
              <div class="flex items-center justify-between gap-3">
                <h2 class="detail-card__title">{{ t('admin.ops.unifiedErrorDetail.sections.sameKindErrors') }}</h2>
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.unifiedErrorDetail.sameKindCountLabel', { count: detail.same_kind_errors.length }) }}
                </span>
              </div>

              <div v-if="detail.same_kind_errors.length === 0" class="mt-3 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.unifiedErrorDetail.noSameKindErrors') }}
              </div>

              <div v-else class="mt-4 space-y-3">
                <button
                  v-for="item in detail.same_kind_errors"
                  :key="item.id"
                  type="button"
                  class="w-full rounded-2xl border border-gray-200 p-4 text-left transition hover:border-blue-300 hover:bg-blue-50/40 dark:border-dark-700 dark:hover:border-blue-500 dark:hover:bg-blue-900/10"
                  @click="openSameKindError(item.id)"
                >
                  <div class="flex flex-wrap items-center justify-between gap-3">
                    <div class="flex flex-wrap items-center gap-2">
                      <span :class="['inline-flex items-center rounded-full px-2.5 py-1 text-[11px] font-semibold', sameKindResultClass(item.error_result)]">
                        {{ errorResultLabel(item.error_result) }}
                      </span>
                      <span class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-1 text-[11px] font-semibold text-gray-600 dark:bg-dark-800 dark:text-gray-300">
                        {{ item.severity || t('admin.ops.unifiedErrorDetail.unknown') }}
                      </span>
                    </div>
                    <span class="text-xs text-gray-500 dark:text-gray-400">{{ timeFallback(item.occurred_at) }}</span>
                  </div>
                  <div class="mt-3 text-sm font-medium text-gray-900 dark:text-white">
                    {{ item.summary || t('admin.ops.unifiedErrorDetail.emptySummary') }}
                  </div>
                  <div class="mt-2 grid grid-cols-1 gap-2 text-xs text-gray-500 dark:text-gray-400 md:grid-cols-2">
                    <div>{{ t('admin.ops.unifiedErrorDetail.fields.user') }}：{{ entityLabel(item.user, t('admin.ops.unifiedErrorDetail.unknownUser')) }}</div>
                    <div>{{ t('admin.ops.unifiedErrorDetail.fields.apiKey') }}：{{ entityLabel(item.api_key, t('admin.ops.unifiedErrorDetail.unknownApiKey')) }}</div>
                    <div>{{ t('admin.ops.unifiedErrorDetail.fields.group') }}：{{ entityLabel(item.group, t('admin.ops.unifiedErrorDetail.ungrouped')) }}</div>
                    <div>{{ t('admin.ops.unifiedErrorDetail.fields.upstreamAccount') }}：{{ entityLabel(item.upstream_account, t('admin.ops.unifiedErrorDetail.hiddenUpstreamAccount')) }}</div>
                  </div>
                </button>
              </div>
            </article>
          </div>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useClipboard } from '@/composables/useClipboard'
import {
  opsAPI,
  type OpsAIAnalysisEvidenceItem,
  type OpsAIAnalysisImpactScope,
  type OpsAIAnalysisTaskDetailResponse,
  type OpsUnifiedEntityRef,
  type OpsUnifiedErrorDetail
} from '@/api/admin/ops'
import { useAppStore, useAuthStore } from '@/stores'
import { formatDateTime } from '@/utils/format'
import { canManageManualAIAnalysis, fetchOpsAIAnalysisConfig, isManualAIAnalysisConfigured, type OpsAIAnalysisConfigSnapshot } from './utils/manualAIAnalysis'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const { copyToClipboard } = useClipboard()
const { t } = useI18n()

const loading = ref(false)
const errorMessage = ref('')
const detail = ref<OpsUnifiedErrorDetail | null>(null)
const rawRecordSection = ref<HTMLElement | null>(null)
const manualAIConfig = ref<OpsAIAnalysisConfigSnapshot | null>(null)
const manualAIConfigLoaded = ref(false)
const manualAIConfigLoadError = ref('')
const activeManualAITaskId = ref<number | null>(null)
const aiTaskDetail = ref<OpsAIAnalysisTaskDetailResponse | null>(null)
const aiReportLoading = ref(false)
const aiReportError = ref('')
let aiReportPollTimer: ReturnType<typeof setTimeout> | null = null

const detailId = computed(() => {
  const parsed = Number.parseInt(String(route.params.id || ''), 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
})

const resultLabel = computed(() => errorResultLabel(detail.value?.conclusion.error_result || 'unknown'))
const resultBadgeClass = computed(() => sameKindResultClass(detail.value?.conclusion.error_result || 'unknown'))
const currentViewerRole = computed(() => String((authStore.user as { role?: string } | null)?.role || '').trim().toLowerCase())
const canRunManualAIAnalysis = computed(() => canManageManualAIAnalysis(currentViewerRole.value))
const aiAnalysisSummary = computed(() => {
  const reportSummary = String(aiTaskDetail.value?.report?.summary || '').trim()
  if (reportSummary) return reportSummary
  return fallback(detail.value?.ai_analysis.summary, t('admin.ops.unifiedErrorDetail.notAnalyzed'))
})
const aiAnalysisTimeLabel = computed(() => {
  const task = aiTaskDetail.value?.task
  return timeFallback(task?.finished_at || task?.started_at || task?.created_at)
})
const aiAnalysisRangeLabel = computed(() => {
  const task = aiTaskDetail.value?.task
  if (!task?.time_start || !task?.time_end) return '—'
  return `${formatDateTime(task.time_start)} ~ ${formatDateTime(task.time_end)}`
})
const aiAnalysisEvidenceItems = computed(() => {
  const value = aiTaskDetail.value?.report?.evidence
  if (Array.isArray(value)) {
    return value
      .map((item) => {
        if (typeof item === 'string') return item.trim()
        const entry = item as OpsAIAnalysisEvidenceItem
        return [entry.text, entry.label, entry.value]
          .map((part) => String(part ?? '').trim())
          .find(Boolean) || ''
      })
      .filter(Boolean)
  }
  if (typeof value === 'string' && value.trim()) return [value.trim()]
  return []
})
const aiAnalysisActions = computed(() => {
  const value = aiTaskDetail.value?.report?.suggested_actions
  if (Array.isArray(value)) {
    return value.map((item) => String(item || '').trim()).filter(Boolean)
  }
  if (typeof value === 'string' && value.trim()) return [value.trim()]
  return []
})
const aiAnalysisImpactItems = computed(() => {
  const raw = aiTaskDetail.value?.report?.impact_scope
  if (!raw || typeof raw !== 'object') return []
  const impact = raw as OpsAIAnalysisImpactScope
  const fields = [
    { key: 'affected_users', label: t('admin.ops.unifiedErrorDetail.impact.affectedUsers') },
    { key: 'affected_api_keys', label: t('admin.ops.unifiedErrorDetail.impact.affectedApiKeys') },
    { key: 'affected_models', label: t('admin.ops.unifiedErrorDetail.impact.affectedModels') },
    { key: 'affected_upstream_accounts', label: t('admin.ops.unifiedErrorDetail.impact.affectedAccounts') }
  ] as const
  return fields
    .map(({ key, label }) => {
      const value = impact[key]
      return typeof value === 'number' && Number.isFinite(value)
        ? { label, value: String(value) }
        : null
    })
    .filter((item): item is { label: string, value: string } => Boolean(item))
})
const aiAnalysisConfidenceLevel = computed(() => String(aiTaskDetail.value?.report?.confidence || '').trim().toLowerCase())
const aiAnalysisConfidenceBadgeLabel = computed(() => {
  switch (aiAnalysisConfidenceLevel.value) {
    case 'high':
      return t('admin.ops.unifiedErrorDetail.confidence.high')
    case 'medium':
      return t('admin.ops.unifiedErrorDetail.confidence.medium')
    case 'low':
      return t('admin.ops.unifiedErrorDetail.confidence.low')
    default:
      return ''
  }
})
const aiAnalysisConfidenceBadgeClass = computed(() => {
  switch (aiAnalysisConfidenceLevel.value) {
    case 'high':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'medium':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'low':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
  }
})
const aiAnalysisConfidenceText = computed(() => aiAnalysisConfidenceBadgeLabel.value || t('admin.ops.unifiedErrorDetail.unknown'))
const aiReportStatus = computed(() => String(aiTaskDetail.value?.task.status || detail.value?.ai_analysis.status || '').trim().toLowerCase())
const aiStatusLabel = computed(() => aiStatusText(aiReportStatus.value))
const aiStatusBadgeClass = computed(() => aiStatusClass(aiReportStatus.value))
const aiReportStateClass = computed(() => {
  switch (aiReportStatus.value) {
    case 'failed':
      return 'border border-red-200 bg-red-50 text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300'
    case 'expired':
      return 'border border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300'
    default:
      return 'bg-gray-50 text-gray-600 dark:bg-dark-800/70 dark:text-gray-300'
  }
})
const aiReportStateMessage = computed(() => {
  if (aiReportLoading.value || aiReportError.value || !detail.value?.ai_analysis.task_id) return ''
  if (aiReportStatus.value === 'pending' || aiReportStatus.value === 'running') {
    return t('admin.ops.unifiedErrorDetail.analysisPending')
  }
  if (aiReportStatus.value === 'completed' && !aiTaskDetail.value?.report) {
    return t('admin.ops.unifiedErrorDetail.analysisReportGenerating')
  }
  if (aiReportStatus.value === 'failed') {
    return aiTaskDetail.value?.task.error_message || t('admin.ops.unifiedErrorDetail.analysisFailed')
  }
  if (aiReportStatus.value === 'expired') {
    return t('admin.ops.unifiedErrorDetail.analysisExpired')
  }
  return ''
})

const manualAIActionDisabledReason = computed(() => {
  const status = aiReportStatus.value
  if (!canRunManualAIAnalysis.value) return '当前账号无权限执行此操作'
  if (!detailId.value || !detail.value) return t('admin.ops.unifiedErrorDetail.aiDisabled.noDetail')
  if (activeManualAITaskId.value) return '分析任务处理中，请稍后查看'
  if (manualAIConfigLoadError.value) return manualAIConfigLoadError.value
  if (!manualAIConfigLoaded.value) return 'AI 配置加载完成后可发起 AI 分析。'
  if (!isManualAIAnalysisConfigured(manualAIConfig.value)) return '请先配置 AI 分析服务'
  if (status === 'pending' || status === 'running') return t('admin.ops.unifiedErrorDetail.aiDisabled.running')
  return ''
})

const manualAIActionDisabled = computed(() => manualAIActionDisabledReason.value !== '')

function fallback(value: string | null | undefined, empty = '—'): string {
  const text = String(value || '').trim()
  return text || empty
}

function numberFallback(value: number | null | undefined): string {
  return typeof value === 'number' && Number.isFinite(value) ? String(value) : '—'
}

function timeFallback(value: string | null | undefined): string {
  if (!value) return '—'
  return formatDateTime(value)
}

function entityLabel(entity: OpsUnifiedEntityRef | null | undefined, empty: string): string {
  if (!entity) return empty
  const label = [entity.display, entity.email, entity.name].map((item) => String(item || '').trim()).find(Boolean)
  if (label) return label
  return entity.id > 0 ? `#${entity.id}` : empty
}

function sameKindResultClass(result: string): string {
  const normalized = String(result || '').trim().toLowerCase()
  if (normalized === 'final_failed') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (normalized === 'recovered') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (normalized === 'client_aborted') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return 'bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-gray-300'
}

function aiStatusClass(status: string): string {
  const normalized = String(status || '').trim().toLowerCase()
  if (normalized === 'completed' || normalized === 'analyzed') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (normalized === 'pending' || normalized === 'running') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (normalized === 'expired') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (normalized === 'failed') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  return 'bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-gray-300'
}

function errorResultLabel(result: string): string {
  switch (String(result || '').trim().toLowerCase()) {
    case 'final_failed':
      return t('admin.ops.unifiedErrorDetail.results.finalFailed')
    case 'recovered':
      return t('admin.ops.unifiedErrorDetail.results.recovered')
    case 'client_aborted':
      return t('admin.ops.unifiedErrorDetail.results.clientAborted')
    default:
      return t('admin.ops.unifiedErrorDetail.results.unknown')
  }
}

function aiStatusText(status: string): string {
  switch (String(status || '').trim().toLowerCase()) {
    case 'completed':
    case 'analyzed':
      return t('admin.ops.unifiedErrorDetail.aiStatus.completed')
    case 'pending':
      return t('admin.ops.unifiedErrorDetail.aiStatus.pending')
    case 'running':
      return t('admin.ops.unifiedErrorDetail.aiStatus.running')
    case 'failed':
      return t('admin.ops.unifiedErrorDetail.aiStatus.failed')
    case 'expired':
      return t('admin.ops.unifiedErrorDetail.analysisExpired')
    default:
      return t('admin.ops.unifiedErrorDetail.aiStatus.notAnalyzed')
  }
}

function prettyPayload(value: unknown): string {
  if (value == null) return t('common.noData')
  if (typeof value === 'string') {
    const text = value.trim()
    if (!text) return t('common.noData')
    try {
      return JSON.stringify(JSON.parse(text), null, 2)
    } catch {
      return text
    }
  }
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

async function fetchDetail() {
  if (!detailId.value) {
    detail.value = null
    errorMessage.value = t('admin.ops.unifiedErrorDetail.invalidId')
    return
  }

  loading.value = true
  errorMessage.value = ''
  try {
    detail.value = await opsAPI.getUnifiedErrorDetail(detailId.value)
    const status = String(detail.value?.ai_analysis.status || '').trim().toLowerCase()
    if (status !== 'pending' && status !== 'running') {
      activeManualAITaskId.value = null
    }
  } catch (error: any) {
    detail.value = null
    activeManualAITaskId.value = null
    errorMessage.value = error?.message || t('admin.ops.failedToLoadErrorDetail')
  } finally {
    loading.value = false
  }
}

function stopAIReportPolling() {
  if (aiReportPollTimer) {
    clearTimeout(aiReportPollTimer)
    aiReportPollTimer = null
  }
}

async function fetchAIAnalysisTaskDetail(taskId: number, poll = false) {
  aiReportLoading.value = true
  aiReportError.value = ''
  try {
    const response = await opsAPI.getAIAnalysisTaskDetail(taskId)
    aiTaskDetail.value = response
    const status = String(response.task.status || '').trim().toLowerCase()
    const shouldContinuePolling =
      status === 'pending' ||
      status === 'running' ||
      (status === 'completed' && !response.report)
    if (poll && shouldContinuePolling) {
      stopAIReportPolling()
      aiReportPollTimer = setTimeout(() => {
        void fetchAIAnalysisTaskDetail(taskId, true)
      }, 5000)
    } else {
      if (activeManualAITaskId.value === taskId) {
        activeManualAITaskId.value = null
      }
      stopAIReportPolling()
    }
  } catch (error: any) {
    aiTaskDetail.value = null
    aiReportError.value = error?.message || t('admin.ops.unifiedErrorDetail.analysisLoadFailed')
    if (activeManualAITaskId.value === taskId) {
      activeManualAITaskId.value = null
    }
    stopAIReportPolling()
  } finally {
    aiReportLoading.value = false
  }
}

async function syncAIAnalysisReport(detailValue: OpsUnifiedErrorDetail | null) {
  stopAIReportPolling()
  aiTaskDetail.value = null
  aiReportError.value = ''
  if (!detailValue?.ai_analysis.task_id) return
  await fetchAIAnalysisTaskDetail(detailValue.ai_analysis.task_id, true)
}

function goBack() {
  if (window.history.length > 1) {
    router.back()
    return
  }
  router.push({ name: 'AdminOpsOverview' })
}

async function copyRequestId() {
  const requestId = detail.value?.request_chain.request_id
  if (!requestId) return
  await copyToClipboard(requestId, t('common.copied'))
}

function focusRawRecord() {
  if (!detail.value?.raw_record) {
    appStore.showWarning(t('admin.ops.unifiedErrorDetail.logNotFound'))
    return
  }
  nextTick(() => {
    rawRecordSection.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  })
}

async function runManualAIAnalysis() {
  if (!detail.value || !detailId.value || manualAIActionDisabled.value) return

  const occurredAtRaw = detail.value.raw_record.error_log?.created_at || new Date().toISOString()
  const occurredAt = new Date(occurredAtRaw)
  const start = new Date(occurredAt.getTime() - 5 * 60 * 1000)
  const end = new Date(occurredAt.getTime() + 5 * 60 * 1000)

  const filters: Record<string, any> = {
    error_categories: [detail.value.classification.error_category],
    error_subcategories: [detail.value.classification.error_subcategory],
    platform: detail.value.request_chain.platform,
    model: detail.value.request_chain.model,
    request_id: detail.value.request_chain.request_id
  }

  if (detail.value.classification.client_error_subcategory) {
    filters.client_error_subcategories = [detail.value.classification.client_error_subcategory]
  }

  try {
    const response = await opsAPI.createAIAnalysisTask({
      source_type: 'unified_errors',
      source_id: detailId.value,
      time_start: start.toISOString(),
      time_end: end.toISOString(),
      filters
    })
    activeManualAITaskId.value = response.task_id
    appStore.showSuccess(response.message || t('admin.ops.incidentOverview.analysisSubmitted'))
    await fetchDetail()
    await fetchAIAnalysisTaskDetail(response.task_id, true)
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.ops.incidentOverview.analysisCreateFailed'))
  }
}

async function loadManualAIAnalysisConfig() {
  manualAIConfigLoadError.value = ''
  try {
    manualAIConfig.value = await fetchOpsAIAnalysisConfig()
  } catch (err: any) {
    manualAIConfig.value = null
    manualAIConfigLoadError.value = err?.message || 'AI 配置加载失败，请稍后重试'
  } finally {
    manualAIConfigLoaded.value = true
  }
}

function openSameKindError(id: number) {
  if (!id || id === detailId.value) return
  router.push({ name: 'AdminOpsUnifiedErrorDetail', params: { id: String(id) } })
}

watch(
  () => detailId.value,
  () => {
    void fetchDetail()
    void loadManualAIAnalysisConfig()
  },
  { immediate: true }
)

watch(
  () => detail.value,
  (nextDetail) => {
    void syncAIAnalysisReport(nextDetail)
  }
)

onUnmounted(() => {
  stopAIReportPolling()
})
</script>

<style scoped>
.btn {
  @apply inline-flex items-center justify-center rounded-xl px-4 py-2 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-60;
}

.btn-sm {
  @apply px-3 py-2 text-sm;
}

.btn-secondary {
  @apply border border-gray-200 text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300;
}

.btn-primary {
  @apply bg-blue-600 text-white hover:bg-blue-700 disabled:bg-blue-300 dark:disabled:bg-blue-800/60;
}

.summary-card {
  @apply rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900;
}

.summary-card__label {
  @apply text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400;
}

.summary-card__value {
  @apply mt-2 text-3xl font-semibold text-gray-900 dark:text-white;
}

.detail-card {
  @apply rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900;
}

.detail-card__title {
  @apply text-lg font-semibold text-gray-900 dark:text-white;
}

.detail-grid {
  @apply mt-4 grid grid-cols-1 gap-4 md:grid-cols-2;
}

.detail-field {
  @apply min-w-0;
}

.detail-field--full {
  @apply md:col-span-2;
}

.detail-field__label {
  @apply text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400;
}

.detail-field__value {
  @apply mt-2 text-sm text-gray-900 dark:text-white break-words;
}

.detail-pre {
  @apply mt-2 max-h-72 overflow-auto rounded-2xl border border-gray-200 bg-gray-50 p-4 text-xs text-gray-800 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-100;
}
</style>
