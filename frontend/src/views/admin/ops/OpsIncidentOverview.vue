<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <section class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
          <div class="space-y-2">
            <div class="flex items-center gap-2">
              <span :class="['inline-flex items-center rounded-full px-3 py-1 text-xs font-semibold', statusBadgeClass]">
                {{ statusLabel }}
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.incidentOverview.autoRefresh', { seconds: autoRefreshCountdown }) }}
              </span>
            </div>
            <div>
              <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
                {{ t('admin.ops.incidentOverview.title') }}
              </h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.incidentOverview.description') }}
              </p>
            </div>
            <p class="text-sm text-gray-600 dark:text-gray-300">
              {{ currentSummary }}
            </p>
          </div>

          <div class="flex flex-wrap items-center gap-3">
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300"
              :disabled="loading"
              @click="fetchOverview"
            >
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              {{ t('common.refresh') }}
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-blue-300 dark:disabled:bg-blue-800/60"
              :disabled="manualAIActionDisabled"
              :title="manualAIActionDisabledReason || undefined"
              @click="triggerManualAIAnalysis"
            >
              <Icon name="sparkles" size="sm" />
              {{ t('admin.ops.incidentOverview.manualAnalysis') }}
            </button>
          </div>
        </div>

        <div v-if="manualAIActionDisabledReason" class="mt-3 text-xs text-gray-500 dark:text-gray-400">
          {{ manualAIActionDisabledReason }}
        </div>

        <div v-if="errorMessage" class="mt-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
          {{ errorMessage }}
        </div>

        <div class="mt-5 grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,2.2fr)_minmax(0,1fr)]">
          <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/80">
            <div class="flex flex-wrap gap-2">
              <button
                v-for="option in timeRangeOptions"
                :key="option.value"
                type="button"
                :class="[
                  'rounded-xl px-3 py-2 text-sm font-medium transition',
                  timeRange === option.value
                    ? 'bg-blue-600 text-white shadow-sm'
                    : 'bg-white text-gray-600 hover:bg-gray-100 dark:bg-dark-900 dark:text-gray-300 dark:hover:bg-dark-700'
                ]"
                @click="handleTimeRangeChange(option.value)"
              >
                {{ option.label }}
              </button>
            </div>

            <div class="mt-4 grid grid-cols-1 gap-3 md:grid-cols-3">
              <div>
                <label class="mb-1 block text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.requestDetails.table.platform') }}
                </label>
                <select v-model="platform" class="input">
                  <option value="">{{ t('common.all') }}</option>
                  <option value="openai">OpenAI</option>
                  <option value="claude">Claude</option>
                  <option value="gemini">Gemini</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.requestDetails.table.model') }}
                </label>
                <input
                  v-model="model"
                  type="text"
                  class="input"
                  :placeholder="t('admin.ops.incidentOverview.modelPlaceholder')"
                >
              </div>
              <div>
                <label class="mb-1 block text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.requestDetails.table.group') }}
                </label>
                <select v-model="groupSelection" class="input">
                  <option value="">{{ t('common.all') }}</option>
                  <option v-for="group in filteredGroups" :key="group.id" :value="String(group.id)">
                    {{ group.name }}
                  </option>
                </select>
              </div>
            </div>

            <div v-if="timeRange === 'custom'" class="mt-4 grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
              <div>
                <label class="mb-1 block text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.customTimeRange.startTime') }}
                </label>
                <input v-model="customTimeStartInput" type="datetime-local" class="input">
              </div>
              <div>
                <label class="mb-1 block text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.customTimeRange.endTime') }}
                </label>
                <input v-model="customTimeEndInput" type="datetime-local" class="input">
              </div>
              <div class="flex items-end">
                <button
                  type="button"
                  class="inline-flex w-full items-center justify-center rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
                  @click="applyCustomTimeRange"
                >
                  {{ t('common.confirm') }}
                </button>
              </div>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div class="rounded-2xl bg-blue-50 p-4 dark:bg-blue-900/20">
              <div class="text-xs font-medium uppercase tracking-wide text-blue-700 dark:text-blue-300">
                {{ t('admin.ops.incidentOverview.lastUpdated') }}
              </div>
              <div class="mt-2 text-sm font-semibold text-blue-900 dark:text-blue-100">
                {{ formattedUpdatedAt }}
              </div>
            </div>
            <div class="rounded-2xl bg-amber-50 p-4 dark:bg-amber-900/20">
              <div class="text-xs font-medium uppercase tracking-wide text-amber-700 dark:text-amber-300">
                {{ t('admin.ops.incidentOverview.currentWindow') }}
              </div>
              <div class="mt-2 text-sm font-semibold text-amber-900 dark:text-amber-100">
                {{ currentWindowLabel }}
              </div>
            </div>
          </div>
        </div>
      </section>

      <div v-if="!displayOverview && loading" class="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div v-for="index in 3" :key="index" class="h-40 animate-pulse rounded-3xl bg-gray-100 dark:bg-dark-800" />
      </div>

      <template v-else-if="displayOverview">
        <section class="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <button
            type="button"
            class="rounded-3xl border border-gray-200 bg-white p-5 text-left shadow-sm transition hover:border-blue-300 hover:shadow-md dark:border-dark-700 dark:bg-dark-900 dark:hover:border-blue-500"
            @click="showAlertEventsDialog = true"
          >
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.incidentOverview.statusCard') }}
                </div>
                <div class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">
                  {{ statusLabel }}
                </div>
              </div>
              <span :class="['rounded-full px-3 py-1 text-xs font-semibold', statusBadgeClass]">
                {{ displayOverview.status.toUpperCase() }}
              </span>
            </div>
            <p class="mt-3 text-sm text-gray-600 dark:text-gray-300">
              {{ currentSummary }}
            </p>
            <div class="mt-4 text-xs font-medium text-blue-600 dark:text-blue-300">
              {{ t('admin.ops.incidentOverview.openAlertEvents') }}
            </div>
          </button>

          <button
            type="button"
            class="rounded-3xl border border-gray-200 bg-white p-5 text-left shadow-sm transition hover:border-blue-300 hover:shadow-md dark:border-dark-700 dark:bg-dark-900 dark:hover:border-blue-500"
            @click="showScoreReasonsDialog = true"
          >
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.incidentOverview.riskScore') }}
                </div>
                <div class="mt-2 text-3xl font-semibold text-gray-900 dark:text-white">
                  {{ scoreValue }}
                </div>
              </div>
              <span :class="['rounded-full px-3 py-1 text-xs font-semibold', scoreLevelBadgeClass]">
                {{ scoreLevelLabel }}
              </span>
            </div>
            <div class="mt-4 text-sm text-gray-600 dark:text-gray-300">
              {{ primaryScoreReason }}
            </div>
            <div class="mt-4 text-xs font-medium text-blue-600 dark:text-blue-300">
              {{ t('admin.ops.incidentOverview.viewScoreReasons') }}
            </div>
          </button>

          <div class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.incidentOverview.primarySummary') }}
                </div>
                <div class="mt-2 text-lg font-semibold text-gray-900 dark:text-white">
                  {{ currentSummary }}
                </div>
              </div>
              <button
                type="button"
                class="rounded-xl border border-gray-200 px-3 py-2 text-xs font-medium text-gray-600 hover:border-blue-300 hover:text-blue-600 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:text-gray-300 dark:hover:border-blue-500 dark:hover:text-blue-300"
                :disabled="!canOpenSummaryDetails"
                @click="openSummaryDetails"
              >
                {{ canOpenSummaryDetails ? t('admin.ops.incidentOverview.viewDetails') : t('admin.ops.incidentOverview.noDetails') }}
              </button>
            </div>

            <ul class="mt-4 space-y-2 text-sm text-gray-600 dark:text-gray-300">
              <li v-for="action in recommendedActions" :key="action" class="flex gap-2">
                <span class="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500" />
                <span>{{ action }}</span>
              </li>
            </ul>
          </div>
        </section>

        <section class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="flex items-center justify-between gap-3">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.ops.incidentOverview.impactTitle') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.incidentOverview.impactDescription') }}
              </p>
            </div>
          </div>

          <div class="mt-5 grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
            <button type="button" class="impact-card" @click="openErrorDetailsFromPreset({ title: t('admin.ops.incidentOverview.finalFailures'), impactPlatformSla: true })">
              <span class="impact-card__label">{{ t('admin.ops.incidentOverview.finalFailures') }}</span>
              <span class="impact-card__value">{{ formatInteger(displayOverview.final_failures) }}</span>
              <span class="impact-card__hint">{{ formatPercent(displayOverview.final_failure_rate) }}</span>
            </button>

            <button type="button" class="impact-card" @click="openErrorDetailsFromPreset({ title: t('admin.ops.incidentOverview.recoveredFluctuations'), view: 'all' })">
              <span class="impact-card__label">{{ t('admin.ops.incidentOverview.recoveredFluctuations') }}</span>
              <span class="impact-card__value">{{ formatInteger(displayOverview.recovered_fluctuations) }}</span>
              <span class="impact-card__hint">{{ t('admin.ops.incidentOverview.recoveredHint') }}</span>
            </button>

            <button type="button" class="impact-card" @click="openErrorDetailsFromPreset({ title: t('admin.ops.incidentOverview.affectedUsers'), view: 'all' })">
              <span class="impact-card__label">{{ t('admin.ops.incidentOverview.affectedUsers') }}</span>
              <span class="impact-card__value">{{ formatInteger(displayOverview.affected_users) }}</span>
              <span class="impact-card__hint">{{ t('admin.ops.incidentOverview.userImpactHint') }}</span>
            </button>

            <button type="button" class="impact-card" @click="openErrorDetailsFromPreset({ title: t('admin.ops.incidentOverview.affectedApiKeys'), view: 'all' })">
              <span class="impact-card__label">{{ t('admin.ops.incidentOverview.affectedApiKeys') }}</span>
              <span class="impact-card__value">{{ formatInteger(displayOverview.affected_api_keys) }}</span>
              <span class="impact-card__hint">{{ t('admin.ops.incidentOverview.apiKeyImpactHint') }}</span>
            </button>
          </div>

          <div class="mt-5 grid grid-cols-1 gap-4 xl:grid-cols-2">
            <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
              <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.incidentOverview.affectedModels') }}
              </div>
              <div v-if="displayOverview.affected_models.length" class="mt-3 flex flex-wrap gap-2">
                <button
                  v-for="item in displayOverview.affected_models"
                  :key="item"
                  type="button"
                  class="rounded-full border border-blue-200 bg-blue-50 px-3 py-1 text-sm text-blue-700 hover:border-blue-300 hover:bg-blue-100 dark:border-blue-800 dark:bg-blue-900/30 dark:text-blue-200 dark:hover:border-blue-600"
                  @click="applyModelFilter(item)"
                >
                  {{ item || t('admin.ops.incidentOverview.unknownModel') }}
                </button>
              </div>
              <div v-else class="mt-3 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.incidentOverview.none') }}
              </div>
            </div>

            <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
              <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.incidentOverview.affectedAccounts') }}
              </div>
              <div v-if="displayOverview.affected_accounts.length" class="mt-3 flex flex-wrap gap-2">
                <button
                  v-for="account in displayOverview.affected_accounts"
                  :key="account.id"
                  type="button"
                  class="rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1 text-sm text-emerald-700 hover:border-emerald-300 hover:bg-emerald-100 dark:border-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-200 dark:hover:border-emerald-600"
                  @click="openAccountDetails(account.id, account.name)"
                >
                  {{ account.name || t('admin.ops.incidentOverview.unknownAccount') }}
                </button>
              </div>
              <div v-else class="mt-3 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.incidentOverview.none') }}
              </div>
            </div>
          </div>
        </section>

        <section class="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
          <div class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <div class="flex items-start justify-between gap-3">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.ops.incidentOverview.latestAnalysisTitle') }}
                </h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.incidentOverview.latestAnalysisDescription') }}
                </p>
              </div>
              <button
                type="button"
                class="rounded-xl border border-gray-200 px-3 py-2 text-xs font-medium text-gray-600 hover:border-blue-300 hover:text-blue-600 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:text-gray-300 dark:hover:border-blue-500 dark:hover:text-blue-300"
                :disabled="!displayOverview.latest_ai_analysis"
                @click="openLatestAIAnalysis"
              >
                {{ displayOverview.latest_ai_analysis ? t('admin.ops.incidentOverview.openAnalysis') : t('admin.ops.incidentOverview.noAnalysis') }}
              </button>
            </div>

            <div v-if="latestAnalysisState === 'ready' && displayOverview.latest_ai_analysis" class="mt-4 rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
              <div class="flex items-center gap-2">
                <span :class="['rounded-full px-3 py-1 text-xs font-semibold', latestAnalysisStatusClass]">
                  {{ latestAnalysisStatusLabel }}
                </span>
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  {{ formatDateTime(displayOverview.latest_ai_analysis.created_at) }}
                </span>
              </div>
              <p class="mt-3 text-sm text-gray-700 dark:text-gray-200">
                {{ displayOverview.latest_ai_analysis.summary }}
              </p>
            </div>

            <div v-else-if="latestAnalysisState === 'expired'" class="mt-4 rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300">
              {{ t('admin.ops.incidentOverview.analysisExpired') }}
            </div>

            <div v-else class="mt-4 rounded-2xl border border-dashed border-gray-200 bg-gray-50 p-4 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800/70 dark:text-gray-400">
              {{ t('admin.ops.incidentOverview.noAnalysis') }}
            </div>
          </div>

          <div class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.ops.incidentOverview.quickFiltersTitle') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.incidentOverview.quickFiltersDescription') }}
            </p>

            <div v-if="displayOverview.quick_filters.length" class="mt-4 flex flex-wrap gap-2">
              <button
                v-for="filter in displayOverview.quick_filters"
                :key="`${filter.label}-${JSON.stringify(filter.params)}`"
                type="button"
                class="rounded-full border border-gray-200 px-3 py-2 text-sm text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300"
                @click="openQuickFilter(filter)"
              >
                {{ filter.label }}
              </button>
            </div>
            <div v-else class="mt-4 rounded-2xl border border-dashed border-gray-200 bg-gray-50 p-4 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800/70 dark:text-gray-400">
              {{ t('admin.ops.incidentOverview.noQuickFilters') }}
            </div>
          </div>
        </section>
      </template>

      <section v-else class="rounded-3xl border border-dashed border-gray-200 bg-white p-10 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <EmptyState
          :title="t('admin.ops.incidentOverview.emptyTitle')"
          :description="t('admin.ops.incidentOverview.emptyDescription')"
          :action-text="t('common.refresh')"
          @action="fetchOverview"
        />
      </section>
    </div>

    <BaseDialog :show="showScoreReasonsDialog" :title="t('admin.ops.incidentOverview.scoreReasonsTitle')" width="wide" @close="showScoreReasonsDialog = false">
      <div class="space-y-3">
        <div
          v-for="reason in scoreReasons"
          :key="reason"
          class="rounded-2xl bg-gray-50 px-4 py-3 text-sm text-gray-700 dark:bg-dark-800 dark:text-gray-200"
        >
          {{ reason }}
        </div>
      </div>
    </BaseDialog>

    <BaseDialog :show="showAlertEventsDialog" :title="t('admin.ops.alertEvents.title')" width="extra-wide" @close="showAlertEventsDialog = false">
      <OpsAlertEventsCard />
    </BaseDialog>

    <BaseDialog :show="showAIReportDialog" :title="t('admin.ops.incidentOverview.analysisDialogTitle')" width="wide" @close="closeAIReportDialog">
      <div v-if="aiReportLoading" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.incidentOverview.analysisLoading') }}
      </div>
      <div v-else-if="aiReportError" class="rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
        {{ aiReportError }}
      </div>
      <div v-else-if="aiTaskDetail" class="space-y-4">
        <div class="flex flex-wrap items-center gap-2">
          <span :class="['rounded-full px-3 py-1 text-xs font-semibold', analysisTaskStatusClass(aiTaskDetail.task.status)]">
            {{ analysisTaskStatusLabel(aiTaskDetail.task.status) }}
          </span>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ formatDateTime(aiTaskDetail.task.created_at) }}
          </span>
        </div>

        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.incidentOverview.analysisTime') }}
            </div>
            <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">
              {{ analysisTaskTimeLabel }}
            </div>
          </div>

          <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.incidentOverview.analysisRange') }}
            </div>
            <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">
              {{ analysisTaskRangeLabel }}
            </div>
          </div>
        </div>

        <div v-if="analysisTaskStateMessage" :class="analysisTaskStateClass">
          {{ analysisTaskStateMessage }}
        </div>

        <div v-if="aiTaskDetail.report" class="space-y-4">
          <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.incidentOverview.analysisSummary') }}
            </div>
            <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">
              {{ aiTaskDetail.report.summary }}
            </div>
          </div>

          <div v-if="aiTaskDetail.report.root_cause" class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.incidentOverview.analysisRootCause') }}
            </div>
            <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">
              {{ aiTaskDetail.report.root_cause }}
            </div>
          </div>

          <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
              <div class="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                <span>{{ t('admin.ops.incidentOverview.analysisConfidence') }}</span>
                <span
                  v-if="analysisConfidenceBadgeLabel"
                  :class="['rounded-full px-2 py-0.5 text-[11px] font-semibold normal-case tracking-normal', analysisConfidenceBadgeClass]"
                >
                  {{ analysisConfidenceBadgeLabel }}
                </span>
              </div>
              <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">
                {{ analysisConfidenceText }}
              </div>
              <div
                v-if="analysisConfidenceLevel === 'low'"
                class="mt-2 text-xs text-amber-700 dark:text-amber-300"
              >
                {{ t('admin.ops.incidentOverview.lowConfidenceHint') }}
              </div>
            </div>

            <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
              <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.incidentOverview.analysisImpact') }}
              </div>
              <ul v-if="analysisImpactItems.length" class="mt-2 space-y-2 text-sm text-gray-800 dark:text-gray-100">
                <li v-for="item in analysisImpactItems" :key="item.label" class="flex items-center justify-between gap-3">
                  <span>{{ item.label }}</span>
                  <span class="font-semibold">{{ item.value }}</span>
                </li>
              </ul>
              <div v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.incidentOverview.noImpactScope') }}
              </div>
            </div>
          </div>

          <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.incidentOverview.analysisEvidence') }}
            </div>
            <ul v-if="analysisEvidenceItems.length" class="mt-2 space-y-2 text-sm text-gray-800 dark:text-gray-100">
              <li v-for="item in analysisEvidenceItems" :key="item" class="flex gap-2">
                <span class="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500" />
                <span>{{ item }}</span>
              </li>
            </ul>
            <div v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.incidentOverview.noEvidence') }}
            </div>
          </div>

          <div v-if="analysisActions.length" class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.incidentOverview.analysisActions') }}
            </div>
            <ul class="mt-2 space-y-2 text-sm text-gray-800 dark:text-gray-100">
              <li v-for="item in analysisActions" :key="item" class="flex gap-2">
                <span class="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500" />
                <span>{{ item }}</span>
              </li>
            </ul>
          </div>

          <div class="rounded-2xl border border-gray-200 p-4 dark:border-dark-700">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  AI 反馈
                </div>
                <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">
                  当前状态：{{ currentFeedbackStatusLabel }}
                </div>
                <div v-if="aiTaskDetail.report.feedback_at" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  最近提交：{{ formatDateTime(aiTaskDetail.report.feedback_at) }}
                </div>
              </div>
              <span
                :class="[
                  'rounded-full px-3 py-1 text-xs font-semibold',
                  aiTaskDetail.report.feedback_status && aiTaskDetail.report.feedback_status !== 'none'
                    ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                    : 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-gray-300'
                ]"
              >
                {{ currentFeedbackStatusLabel }}
              </span>
            </div>

            <div v-if="canSubmitAIReportFeedback" class="mt-4 space-y-4">
              <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
                <button
                  v-for="option in feedbackOptions"
                  :key="option.value"
                  type="button"
                  :class="[
                    'rounded-2xl border px-4 py-3 text-sm font-medium transition',
                    feedbackForm.feedback_status === option.value
                      ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300'
                      : 'border-gray-200 text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-700 dark:text-gray-200'
                  ]"
                  @click="feedbackForm.feedback_status = option.value"
                >
                  {{ option.label }}
                </button>
              </div>

              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-200">补充说明</label>
                <textarea
                  v-model="feedbackForm.feedback_note"
                  rows="4"
                  maxlength="500"
                  class="input min-h-[112px]"
                  placeholder="补充判断依据、遗漏信息或错误原因"
                />
                <div class="mt-2 flex items-center justify-between text-xs">
                  <span class="text-gray-500 dark:text-gray-400">最多 500 字，可留空。</span>
                  <span :class="feedbackNoteLength > 500 ? 'text-red-600 dark:text-red-300' : 'text-gray-500 dark:text-gray-400'">
                    {{ feedbackNoteLength }}/500
                  </span>
                </div>
              </div>

              <div class="flex flex-wrap items-center justify-between gap-3">
                <div class="text-xs text-gray-500 dark:text-gray-400">
                  提交后会覆盖该报告最近一次人工反馈。
                </div>
                <button
                  type="button"
                  class="inline-flex items-center rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-blue-300 dark:disabled:bg-blue-800/60"
                  :disabled="feedbackSubmitDisabled"
                  @click="submitAIReportFeedback"
                >
                  {{ feedbackSaving ? '提交中...' : '提交反馈' }}
                </button>
              </div>
            </div>

            <div v-else class="mt-4 rounded-2xl border border-dashed border-gray-200 px-4 py-3 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
              当前账号无权限反馈 AI 分析报告。
            </div>
          </div>
        </div>

        <div v-else class="rounded-2xl bg-gray-50 p-4 text-sm text-gray-600 dark:bg-dark-800/70 dark:text-gray-300">
          {{ analysisTaskFallbackMessage }}
        </div>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useDebounceFn } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api'
import {
  opsAPI,
  type OpsAIAnalysisFeedbackStatus,
  type OpsAIAnalysisTaskCreateRequest,
  type OpsAIAnalysisTaskDetailResponse,
  type OpsIncidentOverview,
  type OpsIncidentOverviewParams,
  type OpsIncidentOverviewTimeRange,
  type OpsIncidentQuickFilter
} from '@/api/admin/ops'
import { useAppStore, useAuthStore } from '@/stores'
import { formatDateTime, formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'
import OpsAlertEventsCard from './components/OpsAlertEventsCard.vue'
import { canManageManualAIAnalysis, fetchOpsAIAnalysisConfig, isManualAIAnalysisConfigured, type OpsAIAnalysisConfigSnapshot } from './utils/manualAIAnalysis'

const router = useRouter()

type AdminGroupOption = {
  id: number
  name: string
  platform?: string
}

type OpsErrorDetailsPreset = {
  title?: string
  category?: string
  impactPlatformSla?: boolean
  phase?: string
  owner?: string
  view?: 'errors' | 'excluded' | 'all'
  statusCodes?: string
  clientFailed?: boolean
  model?: string
  upstreamAccountId?: number
}

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const aiFeedbackAllowedRoles = new Set([
  'admin',
  'ops',
  'operation',
  'operator',
  'operations',
  'customer_service',
  'customer-service',
  'customerservice',
  'support',
  'service',
  'cs'
])
const feedbackOptions: Array<{ value: Exclude<OpsAIAnalysisFeedbackStatus, 'none'>, label: string }> = [
  { value: 'useful', label: '有用' },
  { value: 'not_useful', label: '无用' },
  { value: 'wrong_category', label: '错误归因' }
]

const timeRange = ref<OpsIncidentOverviewTimeRange>('1m')
const platform = ref('')
const model = ref('')
const groupId = ref<number | null>(null)
const groups = ref<AdminGroupOption[]>([])

const loading = ref(false)
const hasLoadedOnce = ref(false)
const errorMessage = ref('')
const overview = ref<OpsIncidentOverview | null>(null)
const lastSuccessfulOverview = ref<OpsIncidentOverview | null>(null)

const customTimeStartInput = ref('')
const customTimeEndInput = ref('')
const customTimeStartISO = ref<string | null>(null)
const customTimeEndISO = ref<string | null>(null)

const showScoreReasonsDialog = ref(false)
const showAlertEventsDialog = ref(false)
const showAIReportDialog = ref(false)
const aiReportLoading = ref(false)
const aiReportError = ref('')
const aiTaskDetail = ref<OpsAIAnalysisTaskDetailResponse | null>(null)
const activeAITaskId = ref<number | null>(null)
const manualAIConfig = ref<OpsAIAnalysisConfigSnapshot | null>(null)
const manualAIConfigLoaded = ref(false)
const manualAIConfigLoadError = ref('')
const feedbackSaving = ref(false)
const feedbackForm = ref<{
  feedback_status: Exclude<OpsAIAnalysisFeedbackStatus, 'none'>
  feedback_note: string
}>({
  feedback_status: 'useful',
  feedback_note: ''
})


const autoRefreshCountdown = ref(30)
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null
let aiReportPollTimer: ReturnType<typeof setTimeout> | null = null
let fetchController: AbortController | null = null

const timeRangeOptions = computed(() => [
  { value: '1m' as OpsIncidentOverviewTimeRange, label: t('admin.ops.incidentOverview.timeRanges.1m') },
  { value: '5m' as OpsIncidentOverviewTimeRange, label: t('admin.ops.incidentOverview.timeRanges.5m') },
  { value: '30m' as OpsIncidentOverviewTimeRange, label: t('admin.ops.incidentOverview.timeRanges.30m') },
  { value: '1h' as OpsIncidentOverviewTimeRange, label: t('admin.ops.incidentOverview.timeRanges.1h') },
  { value: '6h' as OpsIncidentOverviewTimeRange, label: t('admin.ops.incidentOverview.timeRanges.6h') },
  { value: '24h' as OpsIncidentOverviewTimeRange, label: t('admin.ops.incidentOverview.timeRanges.24h') },
  { value: 'custom' as OpsIncidentOverviewTimeRange, label: t('admin.ops.timeRange.custom') }
])

const filteredGroups = computed(() => {
  const currentPlatform = platform.value.trim().toLowerCase()
  if (!currentPlatform) return groups.value
  return groups.value.filter((group) => {
    const groupPlatform = String(group.platform || '').trim().toLowerCase()
    return !groupPlatform || groupPlatform === currentPlatform
  })
})

const groupSelection = computed({
  get: () => (typeof groupId.value === 'number' && groupId.value > 0 ? String(groupId.value) : ''),
  set: (value: string) => {
    const nextValue = Number.parseInt(String(value || '').trim(), 10)
    groupId.value = Number.isFinite(nextValue) && nextValue > 0 ? nextValue : null
  }
})

const displayOverview = computed(() => overview.value ?? lastSuccessfulOverview.value)
const scoreReasons = computed(() => displayOverview.value?.score_reasons ?? [t('admin.ops.incidentOverview.scoreReasonEmpty')])
const primaryScoreReason = computed(() => scoreReasons.value[0] || t('admin.ops.incidentOverview.scoreReasonEmpty'))
const recommendedActions = computed(() => {
  const actions = displayOverview.value?.recommended_actions ?? []
  return actions.length ? actions : [t('admin.ops.incidentOverview.noRecommendedActions')]
})
const currentSummary = computed(() => displayOverview.value?.summary || t('admin.ops.incidentOverview.noSummary'))

const statusLabel = computed(() => {
  const status = String(displayOverview.value?.status || '').trim().toLowerCase()
  const key = `admin.ops.incidentOverview.status.${status || 'normal'}`
  const translated = t(key)
  return translated === key ? t('admin.ops.incidentOverview.status.normal') : translated
})

const statusBadgeClass = computed(() => {
  switch (String(displayOverview.value?.status || '').trim().toLowerCase()) {
    case 'incident':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'risk':
      return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'
    case 'observing':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    default:
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  }
})

const scoreLevelLabel = computed(() => {
  const level = String(displayOverview.value?.score_level || '').trim().toLowerCase()
  const key = `admin.ops.incidentOverview.scoreLevel.${level || 'normal'}`
  const translated = t(key)
  return translated === key ? t('admin.ops.incidentOverview.scoreLevel.normal') : translated
})

const scoreLevelBadgeClass = computed(() => {
  switch (String(displayOverview.value?.score_level || '').trim().toLowerCase()) {
    case 'incident':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'risk':
      return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'
    case 'observing':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    default:
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  }
})

const scoreValue = computed(() => {
  const value = displayOverview.value?.health_risk_score
  return typeof value === 'number' && Number.isFinite(value) ? String(value) : '--'
})

const formattedUpdatedAt = computed(() => formatDateTime(displayOverview.value?.updated_at) || '--')

const currentWindowLabel = computed(() => {
  if (timeRange.value !== 'custom') {
    return t(`admin.ops.incidentOverview.timeRanges.${timeRange.value}`)
  }
  if (customTimeStartISO.value && customTimeEndISO.value) {
    return `${formatDateTime(customTimeStartISO.value)} ~ ${formatDateTime(customTimeEndISO.value)}`
  }
  return t('admin.ops.timeRange.custom')
})

const latestAnalysisState = computed<'none' | 'ready' | 'expired'>(() => {
  const analysis = displayOverview.value?.latest_ai_analysis
  if (!analysis) return 'none'
  if (String(analysis.status || '').trim().toLowerCase() === 'expired') return 'expired'
  return 'ready'
})

const latestAnalysisStatusLabel = computed(() => {
  const status = String(displayOverview.value?.latest_ai_analysis?.status || '').trim().toLowerCase()
  if (!status) return t('admin.ops.incidentOverview.analysisStatus.completed')
  const key = `admin.ops.incidentOverview.analysisStatus.${status}`
  const translated = t(key)
  return translated === key ? status : translated
})

const latestAnalysisStatusClass = computed(() => analysisTaskStatusClass(displayOverview.value?.latest_ai_analysis?.status || 'completed'))
const currentViewerRole = computed(() => String((authStore.user as { role?: string } | null)?.role || '').trim().toLowerCase())
const canRunManualAIAnalysis = computed(() => canManageManualAIAnalysis(currentViewerRole.value))
const canSubmitAIReportFeedback = computed(() => aiFeedbackAllowedRoles.has(currentViewerRole.value))

const canOpenSummaryDetails = computed(() => {
  return Boolean(displayOverview.value?.quick_filters?.length)
})

const manualAIActionDisabledReason = computed(() => {
  if (!canRunManualAIAnalysis.value) return '当前账号无权限执行此操作'
  if (activeAITaskId.value) return t('admin.ops.incidentOverview.analysisPending')
  if (manualAIConfigLoadError.value) return manualAIConfigLoadError.value
  if (!manualAIConfigLoaded.value) return 'AI 配置加载完成后可发起 AI 分析。'
  if (!isManualAIAnalysisConfigured(manualAIConfig.value)) return '请先配置 AI 分析服务'
  const current = displayOverview.value
  if (!current) return t('admin.ops.incidentOverview.analysisDisabled.loading')
  if (selectedWindowMs.value > 24 * 60 * 60 * 1000) {
    return t('admin.ops.incidentOverview.analysisDisabled.timeTooLarge')
  }
  if (current.final_failures <= 0 && current.recovered_fluctuations <= 0) {
    return t('admin.ops.incidentOverview.analysisDisabled.noErrors')
  }
  return ''
})

const manualAIActionDisabled = computed(() => manualAIActionDisabledReason.value !== '')

const selectedWindowMs = computed(() => {
  if (timeRange.value === 'custom') {
    if (!customTimeStartISO.value || !customTimeEndISO.value) return 0
    return Math.max(0, new Date(customTimeEndISO.value).getTime() - new Date(customTimeStartISO.value).getTime())
  }
  return incidentWindowMs(timeRange.value)
})

const analysisActions = computed(() => {
  const value = aiTaskDetail.value?.report?.suggested_actions
  if (Array.isArray(value)) {
    return value.map((item) => String(item || '').trim()).filter(Boolean)
  }
  if (typeof value === 'string' && value.trim()) return [value.trim()]
  return []
})

const feedbackNoteLength = computed(() => Array.from(feedbackForm.value.feedback_note.trim()).length)
const feedbackSubmitDisabled = computed(() => {
  if (!canSubmitAIReportFeedback.value) return true
  if (feedbackSaving.value || aiReportLoading.value) return true
  if (!aiTaskDetail.value?.report) return true
  return feedbackNoteLength.value > 500
})
const currentFeedbackStatusLabel = computed(() => feedbackStatusLabel(aiTaskDetail.value?.report?.feedback_status))

const debouncedFetchOverview = useDebounceFn(() => {
  void fetchOverview()
}, 300)

watch([platform, model, groupId], () => {
  debouncedFetchOverview()
})

watch(
  () => platform.value,
  (nextPlatform) => {
    const currentGroup = groups.value.find((group) => group.id === groupId.value)
    if (!currentGroup) return
    const groupPlatform = String(currentGroup.platform || '').trim().toLowerCase()
    if (nextPlatform && groupPlatform && groupPlatform !== String(nextPlatform).trim().toLowerCase()) {
      groupId.value = null
    }
  }
)

function incidentWindowMs(value: OpsIncidentOverviewTimeRange): number {
  switch (value) {
    case '1m':
      return 60 * 1000
    case '5m':
      return 5 * 60 * 1000
    case '30m':
      return 30 * 60 * 1000
    case '1h':
      return 60 * 60 * 1000
    case '6h':
      return 6 * 60 * 60 * 1000
    case '24h':
      return 24 * 60 * 60 * 1000
    default:
      return 0
  }
}

function formatInteger(value: number): string {
  return new Intl.NumberFormat().format(Number.isFinite(value) ? value : 0)
}

function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return '--'
  return `${(value * 100).toFixed(value >= 0.1 ? 1 : 2)}%`
}

function buildOverviewParams(): OpsIncidentOverviewParams {
  const params: OpsIncidentOverviewParams = {
    time_range: timeRange.value,
    platform: platform.value.trim() || undefined,
    model: model.value.trim() || undefined,
    group_id: groupId.value
  }

  if (timeRange.value === 'custom') {
    params.start_time = customTimeStartISO.value || undefined
    params.end_time = customTimeEndISO.value || undefined
  }

  return params
}

function getCurrentRangeBounds(): { start: string, end: string } {
  if (timeRange.value === 'custom' && customTimeStartISO.value && customTimeEndISO.value) {
    return {
      start: customTimeStartISO.value,
      end: customTimeEndISO.value
    }
  }

  const endTime = new Date()
  const startTime = new Date(endTime.getTime() - incidentWindowMs(timeRange.value))
  return {
    start: startTime.toISOString(),
    end: endTime.toISOString()
  }
}

function abortFetch() {
  if (fetchController) {
    fetchController.abort()
    fetchController = null
  }
}

function isCanceledRequest(err: unknown): boolean {
  return Boolean(
    err &&
    typeof err === 'object' &&
    'code' in err &&
    (err as Record<string, unknown>).code === 'ERR_CANCELED'
  )
}

async function fetchOverview() {
  if (timeRange.value === 'custom' && (!customTimeStartISO.value || !customTimeEndISO.value)) {
    return
  }

  abortFetch()
  fetchController = new AbortController()
  loading.value = true

  try {
    const data = await opsAPI.getIncidentOverview(buildOverviewParams(), { signal: fetchController.signal })
    overview.value = data
    lastSuccessfulOverview.value = data
    errorMessage.value = ''
    hasLoadedOnce.value = true
    autoRefreshCountdown.value = 30
  } catch (err: any) {
    if (isCanceledRequest(err)) return
    console.error('[OpsIncidentOverview] Failed to load incident overview', err)
    overview.value = lastSuccessfulOverview.value
    errorMessage.value = err?.message || t('admin.ops.incidentOverview.loadFailed')
    if (!hasLoadedOnce.value) {
      appStore.showError(errorMessage.value)
    }
  } finally {
    loading.value = false
  }
}

async function loadGroups() {
  try {
    const list = await adminAPI.groups.getAll()
    groups.value = list.map((group: any) => ({
      id: group.id,
      name: group.name,
      platform: group.platform
    }))
  } catch (err) {
    console.error('[OpsIncidentOverview] Failed to load groups', err)
    groups.value = []
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

function startAutoRefresh() {
  stopAutoRefresh()
  autoRefreshCountdown.value = 30
  autoRefreshTimer = setInterval(() => {
    if (loading.value) return
    if (autoRefreshCountdown.value <= 1) {
      autoRefreshCountdown.value = 30
      void fetchOverview()
      return
    }
    autoRefreshCountdown.value -= 1
  }, 1000)
}

function stopAutoRefresh() {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

function closeAIReportDialog() {
  showAIReportDialog.value = false
  aiReportError.value = ''
  resetFeedbackForm()
  stopAIReportPolling()
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
    const detail = await opsAPI.getAIAnalysisTaskDetail(taskId)
    aiTaskDetail.value = detail
    syncFeedbackForm(detail)
    const status = String(detail.task.status || '').trim().toLowerCase()
    const shouldContinuePolling =
      status === 'pending' ||
      status === 'running' ||
      (status === 'completed' && !detail.report)
    if (poll && shouldContinuePolling) {
      stopAIReportPolling()
      aiReportPollTimer = setTimeout(() => {
        void fetchAIAnalysisTaskDetail(taskId, true)
      }, 5000)
    } else {
      if (activeAITaskId.value === taskId) {
        activeAITaskId.value = null
      }
      stopAIReportPolling()
      if (poll) void fetchOverview()
    }
  } catch (err: any) {
    console.error('[OpsIncidentOverview] Failed to load AI analysis task detail', err)
    aiReportError.value = err?.message || t('admin.ops.incidentOverview.analysisLoadFailed')
    if (activeAITaskId.value === taskId) activeAITaskId.value = null
    stopAIReportPolling()
  } finally {
    aiReportLoading.value = false
  }
}

async function openLatestAIAnalysis() {
  const taskId = displayOverview.value?.latest_ai_analysis?.id
  if (!taskId) return
  aiTaskDetail.value = null
  resetFeedbackForm()
  showAIReportDialog.value = true
  await fetchAIAnalysisTaskDetail(taskId, true)
}

function feedbackStatusLabel(status?: string | null): string {
  switch (String(status || '').trim().toLowerCase()) {
    case 'useful':
      return '有用'
    case 'not_useful':
      return '无用'
    case 'wrong_category':
      return '错误归因'
    default:
      return '未反馈'
  }
}

function syncFeedbackForm(detail: OpsAIAnalysisTaskDetailResponse | null) {
  const status = String(detail?.report?.feedback_status || '').trim().toLowerCase()
  if (status === 'useful' || status === 'not_useful' || status === 'wrong_category') {
    feedbackForm.value.feedback_status = status
  } else {
    feedbackForm.value.feedback_status = 'useful'
  }
  feedbackForm.value.feedback_note = String(detail?.report?.feedback_note || '')
}

function resetFeedbackForm() {
  feedbackForm.value.feedback_status = 'useful'
  feedbackForm.value.feedback_note = ''
  feedbackSaving.value = false
}

async function submitAIReportFeedback() {
  const taskId = aiTaskDetail.value?.task?.id
  if (!taskId || feedbackSubmitDisabled.value) return

  feedbackSaving.value = true
  try {
    const result = await opsAPI.updateAIAnalysisReportFeedback(taskId, {
      feedback_status: feedbackForm.value.feedback_status,
      feedback_note: feedbackForm.value.feedback_note.trim()
    })
    if (aiTaskDetail.value?.report) {
      aiTaskDetail.value = {
        ...aiTaskDetail.value,
        report: {
          ...aiTaskDetail.value.report,
          feedback_status: result.feedback_status,
          feedback_note: result.feedback_note || '',
          feedback_user_id: result.feedback_user_id,
          feedback_at: result.feedback_at,
          updated_at: result.feedback_at || aiTaskDetail.value.report.updated_at
        }
      }
    }
    syncFeedbackForm(aiTaskDetail.value)
    appStore.showSuccess('AI 反馈已提交')
  } catch (err: any) {
    appStore.showError(err?.message || 'AI 反馈提交失败')
  } finally {
    feedbackSaving.value = false
  }
}

function feedbackStatusLabel(status?: string | null): string {
  switch (String(status || '').trim().toLowerCase()) {
    case 'useful':
      return '有用'
    case 'not_useful':
      return '无用'
    case 'wrong_category':
      return '错误归因'
    default:
      return '未反馈'
  }
}

function syncFeedbackForm(detail: OpsAIAnalysisTaskDetailResponse | null) {
  const status = String(detail?.report?.feedback_status || '').trim().toLowerCase()
  if (status === 'useful' || status === 'not_useful' || status === 'wrong_category') {
    feedbackForm.value.feedback_status = status
  } else {
    feedbackForm.value.feedback_status = 'useful'
  }
  feedbackForm.value.feedback_note = String(detail?.report?.feedback_note || '')
}

function resetFeedbackForm() {
  feedbackForm.value.feedback_status = 'useful'
  feedbackForm.value.feedback_note = ''
  feedbackSaving.value = false
}

async function submitAIReportFeedback() {
  const taskId = aiTaskDetail.value?.task?.id
  if (!taskId || feedbackSubmitDisabled.value) return

  feedbackSaving.value = true
  try {
    const result = await opsAPI.updateAIAnalysisReportFeedback(taskId, {
      feedback_status: feedbackForm.value.feedback_status,
      feedback_note: feedbackForm.value.feedback_note.trim()
    })
    if (aiTaskDetail.value?.report) {
      aiTaskDetail.value = {
        ...aiTaskDetail.value,
        report: {
          ...aiTaskDetail.value.report,
          feedback_status: result.feedback_status,
          feedback_note: result.feedback_note || '',
          feedback_user_id: result.feedback_user_id,
          feedback_at: result.feedback_at,
          updated_at: result.feedback_at || aiTaskDetail.value.report.updated_at
        }
      }
    }
    syncFeedbackForm(aiTaskDetail.value)
    appStore.showSuccess('AI 反馈已提交')
  } catch (err: any) {
    appStore.showError(err?.message || 'AI 反馈提交失败')
  } finally {
    feedbackSaving.value = false
  }
}

async function triggerManualAIAnalysis() {
  if (manualAIActionDisabled.value) return

  const currentRange = getCurrentRangeBounds()
  const filters: Record<string, any> = {}
  if (platform.value.trim()) filters.platform = platform.value.trim()
  if (model.value.trim()) filters.model = model.value.trim()
  if (typeof groupId.value === 'number' && groupId.value > 0) filters.group_id = groupId.value

  const payload: OpsAIAnalysisTaskCreateRequest = {
    source_type: 'manual_filter',
    time_start: currentRange.start,
    time_end: currentRange.end,
    filters
  }

  try {
    const result = await opsAPI.createAIAnalysisTask(payload)
    activeAITaskId.value = result.task_id
    appStore.showSuccess(result.message || t('admin.ops.incidentOverview.analysisSubmitted'))
    showAIReportDialog.value = true
    aiTaskDetail.value = null
    await fetchAIAnalysisTaskDetail(result.task_id, true)
  } catch (err: any) {
    console.error('[OpsIncidentOverview] Failed to create manual AI analysis task', err)
    appStore.showError(err?.message || t('admin.ops.incidentOverview.analysisCreateFailed'))
  }
}

function analysisTaskStatusLabel(status: string): string {
  const normalized = String(status || '').trim().toLowerCase()
  const key = `admin.ops.incidentOverview.analysisStatus.${normalized || 'pending'}`
  const translated = t(key)
  return translated === key ? normalized || '-' : translated
}

function analysisTaskStatusClass(status: string): string {
  switch (String(status || '').trim().toLowerCase()) {
    case 'completed':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'failed':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'running':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'expired':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
  }
}

function handleTimeRangeChange(nextValue: OpsIncidentOverviewTimeRange) {
  if (nextValue === 'custom') {
    const now = Math.floor(Date.now() / 1000)
    customTimeStartInput.value = formatDateTimeLocalInput(now - 60)
    customTimeEndInput.value = formatDateTimeLocalInput(now)
    timeRange.value = 'custom'
    applyCustomTimeRange()
    return
  }

  timeRange.value = nextValue
  customTimeStartISO.value = null
  customTimeEndISO.value = null
  void fetchOverview()
}

function applyCustomTimeRange() {
  const start = parseDateTimeLocalInput(customTimeStartInput.value)
  const end = parseDateTimeLocalInput(customTimeEndInput.value)
  if (!start || !end || end <= start) {
    appStore.showWarning(t('admin.ops.incidentOverview.invalidCustomRange'))
    return
  }
  customTimeStartISO.value = new Date(start * 1000).toISOString()
  customTimeEndISO.value = new Date(end * 1000).toISOString()
  void fetchOverview()
}

function applyModelFilter(value: string) {
  model.value = value
  appStore.showInfo(t('admin.ops.incidentOverview.filterApplied', { field: t('admin.ops.requestDetails.table.model'), value }))
}

function openAccountDetails(accountId: number, accountName: string) {
  openErrorDetailsFromPreset({
    title: t('admin.ops.incidentOverview.accountDetailsTitle', { name: accountName || t('admin.ops.incidentOverview.unknownAccount') }),
    upstreamAccountId: accountId
  }, 'upstream')
}

function openQuickFilter(filter: OpsIncidentQuickFilter) {
  const params = filter.params || {}
  if (params.model) {
    applyModelFilter(params.model)
    return
  }

  const accountId = Number.parseInt(String(params.upstream_account_id || params.account_id || ''), 10)
  if (Number.isFinite(accountId) && accountId > 0) {
    openAccountDetails(accountId, filter.label.replace(/^上游账号：/, ''))
    return
  }

  const preset: OpsErrorDetailsPreset = {
    title: filter.label,
    category: params.category,
    impactPlatformSla: params.impact_platform_sla === 'true' || params.impact_platform_sla === '1',
    model: params.model
  }

  openErrorDetailsFromPreset(preset, params.category === 'upstream_error' ? 'upstream' : 'request')
}

function openSummaryDetails() {
  const firstFilter = displayOverview.value?.quick_filters?.[0]
  if (firstFilter) {
    openQuickFilter(firstFilter)
    return
  }
  showAlertEventsDialog.value = true
}

function mapLegacyCategoryToUnifiedCategory(category?: string): string | null {
  switch (String(category || '').trim()) {
    case 'upstream_error':
      return 'upstream'
    case 'platform_error':
      return 'platform'
    case 'client_error':
      return 'client'
    default:
      return null
  }
}

function buildUnifiedErrorsQuery(preset: OpsErrorDetailsPreset, type: 'request' | 'upstream'): Record<string, string> {
  const query: Record<string, string> = {
    page: '1',
    page_size: '20',
    sort_by: 'occurred_at',
    sort_order: 'desc',
    ai_analysis: 'all'
  }

  if (customTimeStartISO.value && customTimeEndISO.value) {
    query.start_time = customTimeStartISO.value
    query.end_time = customTimeEndISO.value
  } else {
    query.time_range = timeRange.value === 'custom' ? '30m' : timeRange.value
  }

  if (platform.value.trim()) query.platform = platform.value.trim()
  if (groupId.value) query.group_id = String(groupId.value)
  if (model.value.trim()) query.model = model.value.trim()
  if (preset.model) query.model = preset.model
  if (preset.statusCodes) query.status_code = preset.statusCodes
  if (typeof preset.upstreamAccountId === 'number' && preset.upstreamAccountId > 0) {
    query.upstream_account_id = String(preset.upstreamAccountId)
  }

  const mappedCategory = mapLegacyCategoryToUnifiedCategory(preset.category)
  if (mappedCategory) {
    query.error_categories = mappedCategory
  } else if (type === 'upstream') {
    query.error_categories = 'upstream'
  } else if (preset.clientFailed) {
    query.error_categories = 'client'
  }

  if (preset.impactPlatformSla) {
    query.error_results = 'final_failed'
  }

  if (preset.title === t('admin.ops.incidentOverview.recoveredFluctuations')) {
    query.error_results = 'recovered'
  }

  return query
}

function openErrorDetailsFromPreset(preset: OpsErrorDetailsPreset, type: 'request' | 'upstream' = 'request') {
  void router.push({
    path: '/admin/ops/errors',
    query: buildUnifiedErrorsQuery(preset, type)
  })
}


onMounted(async () => {
  await Promise.all([loadGroups(), loadManualAIAnalysisConfig()])
  await fetchOverview()
  startAutoRefresh()
})

onUnmounted(() => {
  stopAutoRefresh()
  stopAIReportPolling()
  abortFetch()
})
</script>

<style scoped>
.input {
  @apply w-full rounded-xl border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-dark-600 dark:bg-dark-900 dark:text-white;
}

.impact-card {
  @apply rounded-2xl bg-gray-50 p-4 text-left transition hover:bg-gray-100 dark:bg-dark-800/70 dark:hover:bg-dark-800;
}

.impact-card__label {
  @apply text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400;
}

.impact-card__value {
  @apply mt-2 block text-2xl font-semibold text-gray-900 dark:text-white;
}

.impact-card__hint {
  @apply mt-2 block text-xs text-gray-500 dark:text-gray-400;
}
</style>
