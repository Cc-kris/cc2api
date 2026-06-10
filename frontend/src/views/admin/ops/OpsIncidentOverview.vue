<template>
  <AppLayout>
    <div class="ov-page">

      <!-- ─── 操作栏 ─── -->
      <div class="ov-actionbar">
        <!-- 第一行：标题 + 右侧按钮组（始终同行，不折） -->
        <div class="flex min-h-[40px] items-center justify-between gap-2 px-1">
          <h1 class="shrink-0 text-sm font-bold text-gray-900 dark:text-white">运维监控</h1>
          <div class="flex shrink-0 items-center gap-1.5">
            <span class="hidden text-xs text-gray-400 dark:text-gray-500 sm:inline mr-1">
              <span class="inline-block h-2 w-2 rounded-full bg-emerald-400 mr-0.5 align-middle" />
              {{ t('admin.ops.incidentOverview.autoRefresh', { seconds: autoRefreshCountdown }) }}
            </span>
            <button type="button" class="ov-btn" :disabled="loading" @click="fetchOverview">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              刷新
            </button>
            <button
              type="button"
              class="ov-btn ov-btn--primary"
              :disabled="manualAIActionDisabled"
              :title="manualAIActionDisabledReason || undefined"
              @click="triggerManualAIAnalysis"
            >
              <Icon name="sparkles" size="sm" />
              <span class="hidden sm:inline">手动 AI 分析</span>
              <span class="sm:hidden">AI</span>
            </button>
            <button type="button" class="ov-btn hidden sm:inline-flex" @click="openErrorDetailsFromPreset({ title: '全部错误', impactPlatformSla: true })">
              <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h7"/></svg>
              查看错误列表
            </button>
            <button v-if="canManageOpsSettings" type="button" class="ov-btn" @click="showOpsSettingsDialog = true">
              <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
              <span class="hidden sm:inline">运维设置</span>
            </button>
          </div>
        </div>
        <!-- 第二行：筛选器（独立行，flex-wrap 安全） -->
        <div class="flex flex-wrap items-center gap-1.5 px-1 pb-2 pt-1">
          <label class="ov-filter-label">时间范围</label>
          <select v-model="timeRange" class="ov-select" @change="handleTimeRangeChange(timeRange)">
            <option v-for="opt in timeRangeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
          </select>
          <label class="ov-filter-label ml-1">平台</label>
          <select v-model="platform" class="ov-select w-24">
            <option value="">全部</option>
            <option value="openai">OpenAI</option>
            <option value="claude">Claude</option>
            <option value="gemini">Gemini</option>
          </select>
          <label class="ov-filter-label ml-1">分组</label>
          <select v-model="groupSelection" class="ov-select w-28">
            <option value="">全部分组</option>
            <option v-for="group in filteredGroups" :key="group.id" :value="String(group.id)">{{ group.name }}</option>
          </select>
          <input v-model="model" type="text" class="ov-select w-28" :placeholder="t('admin.ops.incidentOverview.modelPlaceholder')">
          <template v-if="timeRange === 'custom'">
            <input v-model="customTimeStartInput" type="datetime-local" class="ov-select w-40">
            <span class="text-xs text-gray-400">—</span>
            <input v-model="customTimeEndInput" type="datetime-local" class="ov-select w-40">
            <button type="button" class="ov-btn ov-btn--primary text-xs" @click="applyCustomTimeRange">{{ t('common.confirm') }}</button>
          </template>
        </div>
      </div>

      <!-- 错误提示 -->
      <div v-if="errorMessage" class="mt-3 rounded-xl border border-red-200 bg-red-50 px-4 py-2.5 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">{{ errorMessage }}</div>

      <!-- Loading skeleton（首次加载无数据时） -->
      <div v-if="!displayOverview && loading" class="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2 xl:grid-cols-3">
        <div v-for="i in 3" :key="i" class="h-48 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-800" />
      </div>

      <template v-if="!loading || displayOverview">

        <!-- ═══ 第一行：健康分 | 事故摘要 | 推荐操作 ═══ -->
        <div class="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2 xl:grid-cols-3">

          <!-- A · 健康分 -->
          <div class="ov-card flex gap-4">
            <!-- 圆形进度 -->
            <div class="flex shrink-0 flex-col items-center">
              <div class="ov-score-ring" :style="`--score: ${scoreNumeric ?? 0}`">
                <svg viewBox="0 0 80 80" class="ov-score-ring-svg">
                  <circle cx="40" cy="40" r="34" fill="none" stroke="#e5e7eb" stroke-width="7" class="dark:stroke-dark-700"/>
                  <circle
                    cx="40" cy="40" r="34" fill="none"
                    :stroke="scoreNumeric !== null && scoreNumeric >= 90 ? '#10b981' : scoreNumeric !== null && scoreNumeric >= 70 ? '#3b82f6' : scoreNumeric !== null && scoreNumeric >= 50 ? '#f59e0b' : '#ef4444'"
                    stroke-width="7"
                    stroke-linecap="round"
                    stroke-dasharray="213.6"
                    :stroke-dashoffset="scoreNumeric !== null ? 213.6 - (scoreNumeric / 100) * 213.6 : 213.6"
                    transform="rotate(-90 40 40)"
                    class="transition-all duration-700"
                  />
                </svg>
                <div class="ov-score-ring-inner">
                  <span :class="['text-2xl font-black leading-none', scoreColorClass]">{{ scoreValue }}</span>
                  <span class="text-[10px] text-gray-400">/100</span>
                </div>
              </div>
              <span :class="['mt-2 rounded-full px-2.5 py-0.5 text-[11px] font-semibold', scoreLevelBadgeClass]">{{ scoreLevelLabel }}</span>
            </div>
            <!-- 右侧内容 -->
            <div class="flex-1 min-w-0">
              <div class="mb-2 flex items-center gap-1.5">
                <span class="ov-section-title">健康分</span>
                <span v-if="showSmallSampleProtection" class="text-[10px] text-amber-500">小样本保护</span>
              </div>
              <p v-if="parsedScoreDeductions.length === 0" class="text-xs text-gray-500 dark:text-gray-400">当前无扣分，系统运行正常</p>
              <div v-else class="space-y-1">
                <div
                  v-for="item in parsedScoreDeductions.slice(0, 4)"
                  :key="item.label"
                  class="flex items-center justify-between gap-2"
                >
                  <span class="truncate text-xs text-gray-600 dark:text-gray-300">{{ item.label }}</span>
                  <span class="shrink-0 text-xs font-bold text-red-600 dark:text-red-400">−{{ item.points }}</span>
                </div>
                <div v-if="parsedScoreDeductions.length > 4" class="text-[10px] text-gray-400">还有 {{ parsedScoreDeductions.length - 4 }} 项...</div>
              </div>
              <button type="button" class="mt-2 text-xs text-blue-500 hover:text-blue-600 dark:text-blue-400 cursor-pointer" @click="showScoreReasonsDialog = true">查看扣分明细 →</button>
            </div>
          </div>

          <!-- B · 事故摘要 -->
          <div class="ov-card">
            <div class="mb-2 flex items-center justify-between">
              <span class="ov-section-title">事故摘要（{{ timeRangeOptions.find(o => o.value === timeRange)?.label || timeRange }}）</span>
              <div :class="['ov-status-badge', statusBadgeClass]">
                {{ statusIcon }} {{ statusLabel }}
              </div>
            </div>
            <!-- 6指标网格 -->
            <div class="grid grid-cols-3 gap-2">
              <div class="ov-metric-cell">
                <div class="ov-metric-label">最终失败</div>
                <div :class="['ov-metric-val', (displayOverview?.final_failures ?? 0) > 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white']">
                  {{ formatInteger(displayOverview?.final_failures ?? 0) }}
                </div>
              </div>
              <div class="ov-metric-cell">
                <div class="ov-metric-label">失败率</div>
                <div :class="['ov-metric-val', (displayOverview?.final_failure_rate ?? 0) > 0.05 ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white']">
                  {{ formatPercent(displayOverview?.final_failure_rate ?? 0) }}
                </div>
              </div>
              <div class="ov-metric-cell">
                <div class="ov-metric-label">已恢复波动</div>
                <div class="ov-metric-val text-amber-600 dark:text-amber-400">{{ formatInteger(displayOverview?.recovered_fluctuations ?? 0) }}</div>
              </div>
              <div class="ov-metric-cell">
                <div class="ov-metric-label">总请求数</div>
                <div class="ov-metric-val text-gray-900 dark:text-white">{{ formatInteger(displayOverview?.total_requests ?? 0) }}</div>
              </div>
              <div class="ov-metric-cell">
                <div class="ov-metric-label">影响用户数</div>
                <div class="ov-metric-val text-gray-900 dark:text-white">{{ formatInteger(displayOverview?.affected_users ?? 0) }}</div>
              </div>
              <div class="ov-metric-cell">
                <div class="ov-metric-label">影响 API Key</div>
                <div class="ov-metric-val text-gray-900 dark:text-white">{{ formatInteger(displayOverview?.affected_api_keys ?? 0) }}</div>
              </div>
            </div>
            <!-- AI 结论 -->
            <div v-if="currentSummary" class="mt-2.5 rounded-xl bg-blue-50 px-3 py-2 text-xs text-blue-700 leading-relaxed dark:bg-blue-900/15 dark:text-blue-300">
              {{ currentSummary }}
            </div>
          </div>

          <!-- C · 推荐操作 -->
          <div class="ov-card">
            <div class="mb-2 flex items-center justify-between">
              <span class="ov-section-title">{{ actionSectionLabel }}</span>
              <button
                type="button"
                class="ov-btn ov-btn--primary text-[11px] py-1 px-2"
                :disabled="manualAIActionDisabled"
                @click="triggerManualAIAnalysis"
              >
                <Icon name="sparkles" size="sm" />
                AI 分析
              </button>
            </div>
            <div v-if="recommendedActions.length" class="space-y-2">
              <div
                v-for="(action, index) in recommendedActions.slice(0, 4)"
                :key="action"
                :class="['ov-action-item', actionItemClass(index)]"
              >
                <div :class="['ov-action-num', actionNumClass(index)]">{{ index + 1 }}</div>
                <div class="flex-1 min-w-0">
                  <div class="text-xs font-semibold leading-snug text-gray-900 dark:text-gray-100">{{ action }}</div>
                </div>
              </div>
            </div>
            <div v-else class="rounded-xl bg-gray-50 px-3 py-4 text-center text-xs text-gray-500 dark:bg-dark-800/70 dark:text-gray-400">
              {{ t('admin.ops.incidentOverview.noRecommendedActions') }}
            </div>
            <button v-if="recommendedActions.length > 4" class="mt-2 text-xs text-blue-500 hover:text-blue-600 cursor-pointer" @click="triggerManualAIAnalysis">
              查看更多建议 →
            </button>
          </div>
        </div>

        <!-- ═══ 第二行：错误分类 | 依赖健康 | 影响范围 ═══ -->
        <div class="mt-3 grid grid-cols-1 gap-3 lg:grid-cols-2 xl:grid-cols-3">

          <!-- D · 错误分类分布（水平柱状图） -->
          <div class="ov-card">
            <div class="mb-3 flex items-center justify-between">
              <span class="ov-section-title">错误分类分布</span>
              <span class="text-xs text-gray-400 dark:text-gray-500">总错误数：{{ formatInteger(displayOverview?.total_requests ?? 0) }}</span>
            </div>
            <div v-if="errorCategoryHorizontalData.length === 0" class="rounded-xl bg-gray-50 px-3 py-6 text-center text-xs text-gray-500 dark:bg-dark-800/70 dark:text-gray-400">
              当前窗口无错误
            </div>
            <div v-else class="space-y-2.5">
              <button
                v-for="item in errorCategoryHorizontalData.slice(0, 6)"
                :key="item.key"
                type="button"
                class="w-full cursor-pointer group"
                @click="navigateToErrorCategory(item.key)"
              >
                <div class="flex items-center gap-2 mb-0.5">
                  <span class="inline-block h-2 w-2 shrink-0 rounded-full" :style="`background:${item.color}`" />
                  <span class="flex-1 text-left text-xs text-gray-700 dark:text-gray-200 truncate group-hover:text-blue-600 dark:group-hover:text-blue-400 transition-colors">{{ item.label }}</span>
                  <span class="shrink-0 text-xs font-semibold text-gray-900 dark:text-white">{{ formatInteger(item.count) }}</span>
                  <span class="shrink-0 w-10 text-right text-xs text-gray-400">{{ item.percent.toFixed(1) }}%</span>
                </div>
                <div class="ml-4 h-1.5 w-full rounded-full bg-gray-100 dark:bg-dark-700 overflow-hidden">
                  <div class="h-full rounded-full transition-all duration-500" :style="`width:${item.percent}%;background:${item.color}`" />
                </div>
              </button>
            </div>
            <div class="mt-3 border-t border-gray-100 pt-2.5 dark:border-dark-700">
              <button type="button" class="text-xs text-blue-500 hover:text-blue-600 cursor-pointer" @click="openErrorDetailsFromPreset({ title: '全部错误', impactPlatformSla: true })">
                查看错误列表（带筛选）→
              </button>
            </div>
          </div>

          <!-- E · 依赖健康 -->
          <div class="ov-card">
            <div class="mb-3 flex items-center justify-between">
              <span class="ov-section-title">依赖健康</span>
              <span v-if="displayOverview?.system_metrics" class="text-[10px] text-gray-400 dark:text-gray-500">
                {{ formatDateTime(displayOverview?.system_metrics.created_at) }}
              </span>
            </div>
            <div v-if="!displayOverview?.system_metrics" class="rounded-xl bg-gray-50 px-3 py-6 text-center text-xs text-gray-500 dark:bg-dark-800/70 dark:text-gray-400">
              暂无系统指标
            </div>
            <div v-else-if="infraCritical" class="rounded-xl border border-red-200 bg-red-50 p-3 dark:border-red-900/40 dark:bg-red-900/15">
              <div class="flex items-center gap-2 text-xs font-semibold text-red-700 dark:text-red-300">
                <span class="text-red-500">⚠</span>
                基础设施异常：
                <span v-if="displayOverview?.system_metrics.db_ok === false && displayOverview?.system_metrics.redis_ok === false">DB · Redis 不可用</span>
                <span v-else-if="displayOverview?.system_metrics.db_ok === false">DB 不可用</span>
                <span v-else>Redis 不可用</span>
              </div>
            </div>
            <div v-else class="grid grid-cols-2 gap-x-4 gap-y-2.5">
              <div class="ov-infra-row">
                <span class="ov-infra-name">CPU 使用率</span>
                <span :class="['ov-infra-status', infraTextClass(displayOverview?.system_metrics.cpu_usage_percent, 70, 90)]">
                  {{ formatOptionalPercent(displayOverview?.system_metrics.cpu_usage_percent) }}
                  <span class="ml-1 text-[10px]">{{ infraStatusTag(displayOverview?.system_metrics.cpu_usage_percent, 70, 90) }}</span>
                </span>
              </div>
              <div class="ov-infra-row">
                <span class="ov-infra-name">内存使用率</span>
                <span :class="['ov-infra-status', infraTextClass(displayOverview?.system_metrics.memory_usage_percent, 70, 90)]">
                  {{ formatOptionalPercent(displayOverview?.system_metrics.memory_usage_percent) }}
                  <span class="ml-1 text-[10px]">{{ infraStatusTag(displayOverview?.system_metrics.memory_usage_percent, 70, 90) }}</span>
                </span>
              </div>
              <div class="ov-infra-row">
                <span class="ov-infra-name">数据库状态</span>
                <span :class="['ov-infra-status font-semibold', displayOverview?.system_metrics.db_ok === true ? 'text-emerald-600 dark:text-emerald-400' : displayOverview?.system_metrics.db_ok === false ? 'text-red-600 dark:text-red-400' : 'text-gray-400']">
                  {{ formatBooleanStatus(displayOverview?.system_metrics.db_ok) }}
                </span>
              </div>
              <div class="ov-infra-row">
                <span class="ov-infra-name">Redis 状态</span>
                <span :class="['ov-infra-status font-semibold', displayOverview?.system_metrics.redis_ok === true ? 'text-emerald-600 dark:text-emerald-400' : displayOverview?.system_metrics.redis_ok === false ? 'text-red-600 dark:text-red-400' : 'text-gray-400']">
                  {{ formatBooleanStatus(displayOverview?.system_metrics.redis_ok) }}
                </span>
              </div>
              <div class="ov-infra-row">
                <span class="ov-infra-name">并发队列深度</span>
                <span :class="['ov-infra-status', infraTextClass(displayOverview?.system_metrics.concurrency_queue_depth, 100, 300)]">
                  {{ formatOptionalInteger(displayOverview?.system_metrics.concurrency_queue_depth) }}
                  <span class="ml-1 text-[10px]">{{ infraStatusTag(displayOverview?.system_metrics.concurrency_queue_depth, 100, 300) }}</span>
                </span>
              </div>
            </div>
            <div class="mt-3 border-t border-gray-100 pt-2.5 dark:border-dark-700">
              <div class="flex items-center gap-3 text-[10px] text-gray-400">
                <span class="flex items-center gap-1"><span class="h-2 w-2 rounded-full bg-emerald-400" />正常</span>
                <span class="flex items-center gap-1"><span class="h-2 w-2 rounded-full bg-amber-400" />关注</span>
                <span class="flex items-center gap-1"><span class="h-2 w-2 rounded-full bg-red-400" />异常</span>
              </div>
            </div>
          </div>

          <!-- F · 影响范围 -->
          <div class="ov-card">
            <div class="mb-3 flex items-center justify-between">
              <span class="ov-section-title">影响范围（{{ timeRangeOptions.find(o => o.value === timeRange)?.label || timeRange }}）</span>
            </div>
            <div class="grid grid-cols-3 gap-2">
              <!-- 影响用户数 -->
              <div class="ov-impact-cell">
                <svg class="ov-impact-icon text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"/></svg>
                <div class="ov-impact-val">{{ formatInteger(displayOverview?.affected_users ?? 0) }}</div>
                <div class="ov-impact-label">影响用户数</div>
              </div>
              <!-- 影响API Key -->
              <div class="ov-impact-cell">
                <svg class="ov-impact-icon text-indigo-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/></svg>
                <div class="ov-impact-val">{{ formatInteger(displayOverview?.affected_api_keys ?? 0) }}</div>
                <div class="ov-impact-label">影响 API Key</div>
              </div>
              <!-- 影响分组数 -->
              <div class="ov-impact-cell">
                <svg class="ov-impact-icon text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"/></svg>
                <div class="ov-impact-val">{{ displayOverview?.affected_models.length }}</div>
                <div class="ov-impact-label">影响模型数</div>
              </div>
              <!-- 影响账号 -->
              <div class="ov-impact-cell">
                <svg class="ov-impact-icon text-orange-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 15a4 4 0 004 4h9a5 5 0 10-.1-9.999 5.002 5.002 0 10-9.78 2.096A4.001 4.001 0 003 15z"/></svg>
                <div class="ov-impact-val">{{ formatInteger(displayOverview?.affected_accounts?.length ?? 0) }}</div>
                <div class="ov-impact-label">影响上游账号</div>
              </div>
              <!-- 最终失败 -->
              <div class="ov-impact-cell">
                <svg class="ov-impact-icon text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
                <div :class="['ov-impact-val', (displayOverview?.final_failures ?? 0) > 0 ? 'text-red-600 dark:text-red-400' : '']">{{ formatInteger(displayOverview?.final_failures ?? 0) }}</div>
                <div class="ov-impact-label">最终失败请求</div>
              </div>
              <!-- 已恢复波动 -->
              <div class="ov-impact-cell">
                <svg class="ov-impact-icon text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6"/></svg>
                <div class="ov-impact-val text-amber-600 dark:text-amber-400">{{ formatInteger(displayOverview?.recovered_fluctuations ?? 0) }}</div>
                <div class="ov-impact-label">已恢复波动</div>
              </div>
            </div>
          </div>
        </div>

        <!-- ═══ 第三行：三个趋势图 ═══ -->
        <div class="mt-3 grid grid-cols-1 gap-3 lg:grid-cols-2 xl:grid-cols-3">
          <!-- 错误趋势图（复用现有组件） -->
          <div style="height: 240px">
            <OpsErrorTrendChart
              :points="errorTrend?.points ?? []"
              :loading="loadingErrorTrend"
              :time-range="timeRange"
              @open-request-errors="openErrorDetailsFromPreset({ title: t('admin.ops.clientErrors'), category: 'client_error' }, 'request')"
              @open-upstream-errors="openErrorDetailsFromPreset({ title: t('admin.ops.upstreamErrors'), category: 'upstream_error' }, 'upstream')"
            />
          </div>
          <!-- 请求趋势图（占位，数据未接入时显示提示） -->
          <div class="ov-card flex flex-col" style="height: 240px">
            <div class="mb-2 flex items-center justify-between shrink-0">
              <span class="ov-section-title">请求趋势</span>
            </div>
            <div class="flex-1 flex items-center justify-center text-xs text-gray-400 dark:text-gray-500">
              <span>请前往运维仪表盘查看完整请求趋势</span>
            </div>
          </div>
          <!-- 延迟趋势图（占位） -->
          <div class="ov-card flex flex-col" style="height: 240px">
            <div class="mb-2 flex items-center justify-between shrink-0">
              <span class="ov-section-title">延迟趋势</span>
            </div>
            <div class="flex-1 flex items-center justify-center text-xs text-gray-400 dark:text-gray-500">
              <span>请前往运维仪表盘查看完整延迟趋势</span>
            </div>
          </div>
        </div>

        <!-- ═══ 第四行：告警时间线 | AI分析报告 ═══ -->
        <div class="mt-3 grid grid-cols-1 gap-3 lg:grid-cols-2 xl:grid-cols-[1fr_2fr]">

          <!-- G · 告警事件时间线 -->
          <OpsAlertEventsCard />

          <!-- H · AI分析报告 -->
          <div class="ov-card flex flex-col" style="min-height: 300px">
            <div class="mb-3 flex items-center justify-between shrink-0">
              <span class="ov-section-title">最近 AI 分析报告</span>
              <div class="flex items-center gap-2">
                <button
                  v-if="displayOverview?.latest_ai_analysis"
                  type="button"
                  class="ov-btn"
                  @click="openLatestAIAnalysis"
                >查看完整报告 →</button>
                <button
                  type="button"
                  class="ov-btn ov-btn--primary"
                  :disabled="manualAIActionDisabled"
                  :title="manualAIActionDisabledReason || undefined"
                  @click="triggerManualAIAnalysis"
                >
                  <Icon name="sparkles" size="sm" />
                  手动分析
                </button>
              </div>
            </div>

            <!-- 有报告 -->
            <template v-if="latestAnalysisState === 'ready' && displayOverview?.latest_ai_analysis">
              <!-- 元信息行 -->
              <div class="mb-3 flex flex-wrap items-center gap-2 shrink-0">
                <span :class="['rounded-full px-2.5 py-0.5 text-xs font-semibold', latestAnalysisStatusClass]">{{ latestAnalysisStatusLabel }}</span>
                <span class="text-xs font-semibold text-gray-800 dark:text-gray-100 truncate max-w-xs">
                  AI 分析报告
                </span>
                <span class="text-[11px] text-gray-400 dark:text-gray-500">
                  分析时间：{{ formatDateTime(displayOverview?.latest_ai_analysis.created_at) }}
                </span>
              </div>
              <!-- 5列横向内容 -->
              <div class="grid grid-cols-2 gap-3 flex-1 min-h-0 md:grid-cols-5">
                <div class="ov-report-col">
                  <div class="ov-report-col-title">摘要</div>
                  <p class="ov-report-col-body">{{ displayOverview?.latest_ai_analysis.summary || '报告已生成，点击查看完整内容。' }}</p>
                </div>
                <div class="ov-report-col">
                  <div class="ov-report-col-title">根因判断</div>
                  <p class="ov-report-col-body text-amber-700 dark:text-amber-300">点击"查看完整报告"获取根因分析</p>
                </div>
                <div class="ov-report-col">
                  <div class="ov-report-col-title">影响范围</div>
                  <p class="ov-report-col-body">点击"查看完整报告"获取影响范围</p>
                </div>
                <div class="ov-report-col">
                  <div class="ov-report-col-title">证据（部分）</div>
                  <p class="ov-report-col-body">点击"查看完整报告"获取证据列表</p>
                </div>
                <div class="ov-report-col">
                  <div class="ov-report-col-title">推荐操作</div>
                  <p class="ov-report-col-body">点击"查看完整报告"获取建议操作</p>
                </div>
              </div>
              <div class="mt-3 flex justify-end border-t border-gray-100 pt-2.5 shrink-0 dark:border-dark-700">
                <button type="button" class="text-xs text-blue-500 hover:text-blue-600 cursor-pointer dark:text-blue-400" @click="openLatestAIAnalysis">
                  查看全部分析报告 →
                </button>
              </div>
            </template>

            <!-- 分析中 -->
            <div v-else-if="latestAnalysisState === 'pending'" class="flex-1 flex items-center justify-center">
              <div class="rounded-xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-700 dark:border-blue-900/40 dark:bg-blue-900/20 dark:text-blue-300">
                AI 分析正在进行中，请稍候...
              </div>
            </div>

            <!-- 过期 -->
            <div v-else-if="latestAnalysisState === 'expired'" class="flex-1 flex items-center justify-center">
              <div class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300">
                {{ t('admin.ops.incidentOverview.analysisExpired') }}
              </div>
            </div>

            <!-- 无报告 -->
            <div v-else class="flex-1 flex flex-col items-center justify-center gap-3">
              <div class="text-sm text-gray-400 dark:text-gray-500 text-center">{{ t('admin.ops.incidentOverview.noAnalysis') }}</div>
              <div v-if="manualAIActionDisabledReason" class="text-xs text-gray-400 dark:text-gray-500 text-center">{{ manualAIActionDisabledReason }}</div>
              <button
                type="button"
                class="ov-btn ov-btn--primary"
                :disabled="manualAIActionDisabled"
                @click="triggerManualAIAnalysis"
              >
                <Icon name="sparkles" size="sm" />
                立即发起 AI 分析
              </button>
            </div>
          </div>
        </div>

      </template>

      <!-- Empty state -->
      <section v-else class="mt-4 rounded-2xl border border-dashed border-gray-200 bg-white p-10 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <EmptyState
          :title="t('admin.ops.incidentOverview.emptyTitle')"
          :description="t('admin.ops.incidentOverview.emptyDescription')"
          :action-text="t('common.refresh')"
          @action="fetchOverview"
        />
      </section>
    </div>

    <!-- ── Dialogs ── -->
    <BaseDialog :show="showScoreReasonsDialog" :title="t('admin.ops.incidentOverview.scoreReasonsTitle')" width="wide" @close="showScoreReasonsDialog = false">
      <div class="space-y-3">
        <div v-for="reason in scoreReasons" :key="reason" class="rounded-xl bg-gray-50 px-4 py-3 text-sm text-gray-700 dark:bg-dark-800 dark:text-gray-200">{{ reason }}</div>
      </div>
    </BaseDialog>

    <BaseDialog :show="showAlertEventsDialog" :title="t('admin.ops.alertEvents.title')" width="extra-wide" @close="showAlertEventsDialog = false">
      <OpsAlertEventsCard />
    </BaseDialog>

    <OpsSettingsDialog
      v-if="canManageOpsSettings"
      :show="showOpsSettingsDialog"
      @close="showOpsSettingsDialog = false"
      @saved="handleOpsSettingsSaved"
    />

    <BaseDialog :show="showAIReportDialog" :title="t('admin.ops.incidentOverview.analysisDialogTitle')" width="wide" @close="closeAIReportDialog">
      <div v-if="aiReportLoading" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.ops.incidentOverview.analysisLoading') }}
      </div>
      <div v-else-if="aiReportError" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
        {{ aiReportError }}
      </div>
      <div v-else-if="aiTaskDetail" class="space-y-4">
        <div class="flex flex-wrap items-center gap-2">
          <span :class="['rounded-full px-3 py-1 text-xs font-semibold', analysisTaskStatusClass(aiTaskDetail.task.status)]">
            {{ analysisTaskStatusLabel(aiTaskDetail.task.status) }}
          </span>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(aiTaskDetail.task.created_at) }}</span>
        </div>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.ops.incidentOverview.analysisTime') }}</div>
            <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">{{ analysisTaskTimeLabel }}</div>
          </div>
          <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.ops.incidentOverview.analysisRange') }}</div>
            <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">{{ analysisTaskRangeLabel }}</div>
          </div>
        </div>
        <div v-if="analysisTaskStateMessage" :class="analysisTaskStateClass">{{ analysisTaskStateMessage }}</div>
        <div v-if="aiTaskDetail.report" class="space-y-4">
          <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.ops.incidentOverview.analysisSummary') }}</div>
            <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">{{ aiTaskDetail.report.summary }}</div>
          </div>
          <div v-if="aiTaskDetail.report.root_cause" class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.ops.incidentOverview.analysisRootCause') }}</div>
            <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">{{ aiTaskDetail.report.root_cause }}</div>
          </div>
          <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800/70">
              <div class="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                <span>{{ t('admin.ops.incidentOverview.analysisConfidence') }}</span>
                <span v-if="analysisConfidenceBadgeLabel" :class="['rounded-full px-2 py-0.5 text-[11px] font-semibold normal-case tracking-normal', analysisConfidenceBadgeClass]">{{ analysisConfidenceBadgeLabel }}</span>
              </div>
              <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">{{ analysisConfidenceText }}</div>
            </div>
            <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800/70">
              <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.ops.incidentOverview.analysisImpact') }}</div>
              <ul v-if="analysisImpactItems.length" class="mt-2 space-y-2 text-sm text-gray-800 dark:text-gray-100">
                <li v-for="item in analysisImpactItems" :key="item.label" class="flex items-center justify-between gap-3">
                  <span>{{ item.label }}</span><span class="font-semibold">{{ item.value }}</span>
                </li>
              </ul>
              <div v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.ops.incidentOverview.noImpactScope') }}</div>
            </div>
          </div>
          <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.ops.incidentOverview.analysisEvidence') }}</div>
            <ul v-if="analysisEvidenceItems.length" class="mt-2 space-y-2 text-sm text-gray-800 dark:text-gray-100">
              <li v-for="item in analysisEvidenceItems" :key="item" class="flex gap-2"><span class="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500" /><span>{{ item }}</span></li>
            </ul>
          </div>
          <div v-if="analysisActions.length" class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800/70">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ t('admin.ops.incidentOverview.analysisActions') }}</div>
            <ul class="mt-2 space-y-2 text-sm text-gray-800 dark:text-gray-100">
              <li v-for="item in analysisActions" :key="item" class="flex gap-2"><span class="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500" /><span>{{ item }}</span></li>
            </ul>
          </div>
          <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">AI 反馈</div>
                <div class="mt-2 text-sm text-gray-800 dark:text-gray-100">当前状态：{{ currentFeedbackStatusLabel }}</div>
                <div v-if="aiTaskDetail.report.feedback_at" class="mt-1 text-xs text-gray-500 dark:text-gray-400">最近提交：{{ formatDateTime(aiTaskDetail.report.feedback_at) }}</div>
              </div>
              <span :class="['rounded-full px-3 py-1 text-xs font-semibold', aiTaskDetail.report.feedback_status && aiTaskDetail.report.feedback_status !== 'none' ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-gray-300']">{{ currentFeedbackStatusLabel }}</span>
            </div>
            <div v-if="canSubmitAIReportFeedback" class="mt-4 space-y-4">
              <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
                <button
                  v-for="option in feedbackOptions"
                  :key="option.value"
                  type="button"
                  :class="['rounded-xl border px-4 py-3 text-sm font-medium transition', feedbackForm.feedback_status === option.value ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300' : 'border-gray-200 text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-700 dark:text-gray-200']"
                  @click="feedbackForm.feedback_status = option.value"
                >{{ option.label }}</button>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-200">补充说明</label>
                <textarea v-model="feedbackForm.feedback_note" rows="4" maxlength="500" class="input min-h-[112px]" placeholder="补充判断依据、遗漏信息或错误原因" />
                <div class="mt-2 flex items-center justify-between text-xs">
                  <span class="text-gray-500 dark:text-gray-400">最多 500 字，可留空。</span>
                  <span :class="feedbackNoteLength > 500 ? 'text-red-600 dark:text-red-300' : 'text-gray-500 dark:text-gray-400'">{{ feedbackNoteLength }}/500</span>
                </div>
              </div>
              <div class="flex flex-wrap items-center justify-between gap-3">
                <div class="text-xs text-gray-500 dark:text-gray-400">提交后会覆盖该报告最近一次人工反馈。</div>
                <button type="button" class="inline-flex items-center rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-blue-300 dark:disabled:bg-blue-800/60" :disabled="feedbackSubmitDisabled" @click="submitAIReportFeedback">{{ feedbackSaving ? '提交中...' : '提交反馈' }}</button>
              </div>
            </div>
            <div v-else class="mt-4 rounded-xl border border-dashed border-gray-200 px-4 py-3 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">当前账号无权限反馈 AI 分析报告。</div>
          </div>
        </div>
        <div v-else class="rounded-xl bg-gray-50 p-4 text-sm text-gray-600 dark:bg-dark-800/70 dark:text-gray-300">{{ analysisTaskFallbackMessage }}</div>
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
  type OpsAIAnalysisEvidenceItem,
  type OpsAIAnalysisFeedbackStatus,
  type OpsAIAnalysisImpactScope,
  type OpsAIAnalysisTaskCreateRequest,
  type OpsAIAnalysisTaskDetailResponse,
  type OpsErrorTrendResponse,
  type OpsIncidentOverview,
  type OpsIncidentOverviewParams,
  type OpsIncidentOverviewTimeRange
} from '@/api/admin/ops'
import { useAppStore, useAuthStore } from '@/stores'
import { formatDateTime, formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'
import OpsAlertEventsCard from './components/OpsAlertEventsCard.vue'
import OpsErrorTrendChart from './components/OpsErrorTrendChart.vue'
import OpsSettingsDialog from './components/OpsSettingsDialog.vue'
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

const errorTrend = ref<OpsErrorTrendResponse | null>(null)
const loadingErrorTrend = ref(false)

const customTimeStartInput = ref('')
const customTimeEndInput = ref('')
const customTimeStartISO = ref<string | null>(null)
const customTimeEndISO = ref<string | null>(null)

const showScoreReasonsDialog = ref(false)
const showAlertEventsDialog = ref(false)
const showAIReportDialog = ref(false)
const showOpsSettingsDialog = ref(false)
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
let errorTrendRequestId = 0

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
const scoreReasons = computed(() => displayOverview?.value?.score_reasons ?? [t('admin.ops.incidentOverview.scoreReasonEmpty')])

type ParsedScoreDeduction = {
  label: string
  points: number
  reason: string
}

// Parse score_reasons strings into structured items.
// Backend may send strings like "失败率过高 (-15分): 原因说明" or plain explanatory text.
const parsedScoreDeductions = computed<ParsedScoreDeduction[]>(() => {
  const reasons = displayOverview?.value?.score_reasons ?? []
  return reasons
    .map(reason => {
      const str = String(reason || '').trim()
      if (!str) return null
      // Pattern: "Label (-N分): Reason" or "Label (-N分)"
      const match = str.match(/^(.+?)\s*\(-(\d+)\s*分\)(?:[:：]\s*(.+))?$/)
      if (match) {
        const points = Number.parseInt(match[2], 10)
        if (points === 0) return null  // 过滤零扣分项
        return { label: match[1].trim(), points, reason: match[3]?.trim() || '' }
      }
      // 不显示纯说明文字（无扣分点的行）
      return null
    })
    .filter((item): item is ParsedScoreDeduction => item !== null)
})


const recommendedActions = computed(() => {
  const actions = displayOverview?.value?.recommended_actions ?? []
  return actions.filter(a => String(a || '').trim())
})
const currentSummary = computed(() => displayOverview?.value?.summary || t('admin.ops.incidentOverview.noSummary'))

const infraCritical = computed(() => {
  const m = displayOverview?.value?.system_metrics
  return m !== null && m !== undefined && (m.db_ok === false || m.redis_ok === false)
})

const showSmallSampleProtection = computed(() => {
  const ov = displayOverview?.value
  if (!ov) return false
  return ov.final_failures <= 2 && ov.total_requests > 0
})

const statusLabel = computed(() => {
  const status = String(displayOverview?.value?.status || '').trim().toLowerCase()
  const key = `admin.ops.incidentOverview.status.${status || 'normal'}`
  const translated = t(key)
  return translated === key ? t('admin.ops.incidentOverview.status.normal') : translated
})

const scoreLevelLabel = computed(() => {
  const level = String(displayOverview?.value?.score_level || '').trim().toLowerCase()
  const key = `admin.ops.incidentOverview.scoreLevel.${level || 'normal'}`
  const translated = t(key)
  return translated === key ? t('admin.ops.incidentOverview.scoreLevel.normal') : translated
})

const scoreLevelBadgeClass = computed(() => {
  switch (String(displayOverview?.value?.score_level || '').trim().toLowerCase()) {
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
  const value = displayOverview?.value?.health_risk_score
  return typeof value === 'number' && Number.isFinite(value) ? String(value) : '--'
})

const latestAnalysisState = computed<'none' | 'ready' | 'pending' | 'expired'>(() => {
  const analysis = displayOverview?.value?.latest_ai_analysis
  if (!analysis) return 'none'
  const status = String(analysis.status || '').trim().toLowerCase()
  if (status === 'expired') return 'expired'
  if (status === 'pending' || status === 'running') return 'pending'
  return 'ready'
})

const latestAnalysisStatusLabel = computed(() => {
  const status = String(displayOverview?.value?.latest_ai_analysis?.status || '').trim().toLowerCase()
  if (!status) return t('admin.ops.incidentOverview.analysisStatus.completed')
  const key = `admin.ops.incidentOverview.analysisStatus.${status}`
  const translated = t(key)
  return translated === key ? status : translated
})

const latestAnalysisStatusClass = computed(() => analysisTaskStatusClass(displayOverview?.value?.latest_ai_analysis?.status || 'completed'))
const currentViewerRole = computed(() => String((authStore.user as { role?: string } | null)?.role || '').trim().toLowerCase())
const canRunManualAIAnalysis = computed(() => canManageManualAIAnalysis(currentViewerRole.value))
const canSubmitAIReportFeedback = computed(() => aiFeedbackAllowedRoles.has(currentViewerRole.value))
const canManageOpsSettings = computed(() => canManageManualAIAnalysis(currentViewerRole.value))

const manualAIActionDisabledReason = computed(() => {
  if (!canRunManualAIAnalysis.value) return '当前账号无权限执行此操作'
  if (activeAITaskId.value) return t('admin.ops.incidentOverview.analysisPending')
  if (manualAIConfigLoadError.value) return manualAIConfigLoadError.value
  if (!manualAIConfigLoaded.value) return 'AI 配置加载完成后可发起 AI 分析。'
  if (!isManualAIAnalysisConfigured(manualAIConfig.value)) return '请先配置 AI 分析服务'
  const current = displayOverview?.value
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
const analysisTaskTimeLabel = computed(() => {
  const task = aiTaskDetail.value?.task
  return timeFallback(task?.finished_at || task?.started_at || task?.created_at)
})
const analysisTaskRangeLabel = computed(() => {
  const task = aiTaskDetail.value?.task
  if (!task?.time_start || !task?.time_end) return '—'
  return `${formatDateTime(task.time_start)} ~ ${formatDateTime(task.time_end)}`
})
const analysisTaskStatus = computed(() => String(aiTaskDetail.value?.task.status || '').trim().toLowerCase())
const analysisTaskStateClass = computed(() => {
  switch (analysisTaskStatus.value) {
    case 'failed':
      return 'rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300'
    case 'expired':
      return 'rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300'
    default:
      return 'rounded-2xl bg-gray-50 px-4 py-3 text-sm text-gray-600 dark:bg-dark-800/70 dark:text-gray-300'
  }
})
const analysisTaskStateMessage = computed(() => {
  if (aiReportLoading.value || aiReportError.value || !aiTaskDetail.value?.task) return ''
  if (analysisTaskStatus.value === 'pending' || analysisTaskStatus.value === 'running') {
    return t('admin.ops.incidentOverview.analysisPending')
  }
  if (analysisTaskStatus.value === 'completed' && !aiTaskDetail.value?.report) {
    return t('admin.ops.incidentOverview.analysisReportGenerating')
  }
  if (analysisTaskStatus.value === 'failed') {
    return aiTaskDetail.value.task.error_message || t('admin.ops.incidentOverview.analysisFailed')
  }
  if (analysisTaskStatus.value === 'expired') {
    return t('admin.ops.incidentOverview.analysisStatus.expired')
  }
  return ''
})
const analysisTaskFallbackMessage = computed(() => {
  if (aiReportLoading.value) return t('admin.ops.incidentOverview.analysisLoading')
  if (aiReportError.value) return aiReportError.value
  return t('admin.ops.incidentOverview.analysisPending')
})
const analysisEvidenceItems = computed(() => {
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
const analysisImpactItems = computed(() => {
  const raw = aiTaskDetail.value?.report?.impact_scope
  if (!raw || typeof raw !== 'object') return []
  const impact = raw as OpsAIAnalysisImpactScope
  const fields = [
    { key: 'affected_users', label: t('admin.ops.incidentOverview.impact.affectedUsers') },
    { key: 'affected_api_keys', label: t('admin.ops.incidentOverview.impact.affectedApiKeys') },
    { key: 'affected_models', label: t('admin.ops.incidentOverview.impact.affectedModels') },
    { key: 'affected_upstream_accounts', label: t('admin.ops.incidentOverview.impact.affectedAccounts') }
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
const analysisConfidenceLevel = computed(() => String(aiTaskDetail.value?.report?.confidence || '').trim().toLowerCase())
const analysisConfidenceBadgeLabel = computed(() => {
  switch (analysisConfidenceLevel.value) {
    case 'high':
      return t('admin.ops.incidentOverview.confidence.high')
    case 'medium':
      return t('admin.ops.incidentOverview.confidence.medium')
    case 'low':
      return t('admin.ops.incidentOverview.confidence.low')
    default:
      return ''
  }
})
const analysisConfidenceBadgeClass = computed(() => {
  switch (analysisConfidenceLevel.value) {
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
const analysisConfidenceText = computed(() => analysisConfidenceBadgeLabel.value || t('admin.ops.unifiedErrorDetail.unknown'))

const feedbackNoteLength = computed(() => Array.from(feedbackForm.value.feedback_note.trim()).length)
const feedbackSubmitDisabled = computed(() => {
  if (!canSubmitAIReportFeedback.value) return true
  if (feedbackSaving.value || aiReportLoading.value) return true
  if (!aiTaskDetail.value?.report) return true
  return feedbackNoteLength.value > 500
})
const currentFeedbackStatusLabel = computed(() => feedbackStatusLabel(aiTaskDetail.value?.report?.feedback_status))

// 가로형 바 차트 데이터 (참조 이미지 스타일)
const errorCategoryHorizontalData = computed(() => {
  const counts = displayOverview?.value?.error_category_counts
  if (!counts || Object.keys(counts).length === 0) return []
  const colorHexMap: Record<string, string> = {
    client: '#60a5fa', platform: '#818cf8', upstream: '#fb923c',
    account_pool: '#fbbf24', rate_limit: '#facc15', permission: '#c084fc',
    balance: '#fb7185', config: '#9ca3af', slow_request: '#22d3ee', unknown: '#94a3b8'
  }
  const total = Object.values(counts).reduce((s, v) => s + (typeof v === 'number' ? v : 0), 0) || 1
  const labelMap: Record<string, string> = {
    client: '客户端错误', platform: '平台错误', upstream: '上游错误',
    account_pool: '账号池错误', rate_limit: '上游限流', permission: '权限错误',
    balance: '余额不足', config: '配置错误', slow_request: '慢请求', unknown: '未分类'
  }
  return Object.entries(counts)
    .filter(([, v]) => typeof v === 'number' && v > 0)
    .sort(([, a], [, b]) => (b as number) - (a as number))
    .map(([key, count]) => ({
      key,
      label: labelMap[key] || key,
      count: count as number,
      percent: ((count as number) / total) * 100,
      color: colorHexMap[key] || '#9ca3af'
    }))
})

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

function formatOptionalInteger(value: number | null | undefined): string {
  return typeof value === 'number' && Number.isFinite(value) ? formatInteger(value) : '--'
}

function formatOptionalPercent(value: number | null | undefined): string {
  return typeof value === 'number' && Number.isFinite(value) ? `${value.toFixed(1)}%` : '--'
}

function formatBooleanStatus(value: boolean | null | undefined): string {
  if (value === true) return '正常'
  if (value === false) return '异常'
  return '--'
}

async function handleOpsSettingsSaved() {
  showOpsSettingsDialog.value = false
  await Promise.all([loadManualAIAnalysisConfig(), fetchOverview()])
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
    const [data] = await Promise.all([
      opsAPI.getIncidentOverview(buildOverviewParams(), { signal: fetchController.signal }),
      fetchErrorTrend(fetchController.signal)
    ])
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

async function fetchErrorTrend(signal?: AbortSignal) {
  const requestId = ++errorTrendRequestId
  loadingErrorTrend.value = true
  try {
    const params: Record<string, any> = {
      platform: platform.value.trim() || undefined,
      group_id: groupId.value
    }
    if (timeRange.value === 'custom') {
      if (customTimeStartISO.value && customTimeEndISO.value) {
        params.start_time = customTimeStartISO.value
        params.end_time = customTimeEndISO.value
      }
    } else {
      const tr = timeRange.value as '1m' | '5m' | '30m' | '1h' | '6h' | '24h'
      // Map 1m to 5m for trend (1m has no trend data)
      params.time_range = tr === '1m' ? '5m' : tr
    }
    const data = await opsAPI.getErrorTrend(params, { signal })
    if (requestId !== errorTrendRequestId) return
    errorTrend.value = data
  } catch (err) {
    if (isCanceledRequest(err)) return
    if (requestId !== errorTrendRequestId) return
    console.error('[OpsIncidentOverview] Failed to load error trend', err)
    errorTrend.value = null
  } finally {
    if (requestId === errorTrendRequestId) {
      loadingErrorTrend.value = false
    }
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
  // 轮询更新时不显示 loading（避免内容区闪烁），只有初始加载才显示
  const isInitialLoad = !aiTaskDetail.value
  if (isInitialLoad) aiReportLoading.value = true
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
  const taskId = displayOverview?.value?.latest_ai_analysis?.id
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

function timeFallback(value: string | null | undefined): string {
  if (!value) return '—'
  return formatDateTime(value)
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
    ai_analysis: 'all',
    from_overview: '1'
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

function navigateToErrorCategory(categoryKey: string) {
  const preset: OpsErrorDetailsPreset = {
    title: `${categoryKey} 错误`,
    category: categoryKey,
    impactPlatformSla: true
  }
  openErrorDetailsFromPreset(preset)
}

function openErrorDetailsFromPreset(preset: OpsErrorDetailsPreset, type: 'request' | 'upstream' = 'request') {
  void router.push({
    path: '/admin/ops/errors',
    query: buildUnifiedErrorsQuery(preset, type)
  })
}


// ─── New layout helpers ───────────────────────────────────────────────────────

const scoreNumeric = computed(() => {
  const v = displayOverview?.value?.health_risk_score
  return typeof v === 'number' && Number.isFinite(v) ? v : null
})

const scoreColorClass = computed(() => {
  switch (String(displayOverview?.value?.score_level || '').trim().toLowerCase()) {
    case 'incident': return 'text-red-600 dark:text-red-400'
    case 'risk': return 'text-amber-600 dark:text-amber-400'
    case 'observing': return 'text-blue-600 dark:text-blue-400'
    default: return 'text-emerald-600 dark:text-emerald-400'
  }
})

const statusIcon = computed(() => {
  switch (String(displayOverview?.value?.status || '').trim().toLowerCase()) {
    case 'incident': return '⚡'
    case 'risk': return '⚠'
    case 'observing': return '👁'
    default: return '✓'
  }
})

const actionSectionLabel = computed(() => {
  const status = String(displayOverview?.value?.status || '').trim().toLowerCase()
  if (status === 'incident' || status === 'risk') return '需要立即处理'
  if (status === 'observing') return '需要关注'
  return '建议操作'
})

function actionItemClass(index: number): string {
  const status = String(displayOverview?.value?.status || '').trim().toLowerCase()
  if (status === 'incident') {
    if (index === 0) return 'ov-action-item--red'
    if (index === 1) return 'ov-action-item--amber'
    return 'ov-action-item--blue'
  }
  if (status === 'risk') {
    if (index === 0) return 'ov-action-item--amber'
    return 'ov-action-item--blue'
  }
  return 'ov-action-item--blue'
}

function actionNumClass(index: number): string {
  const status = String(displayOverview?.value?.status || '').trim().toLowerCase()
  if (status === 'incident') {
    if (index === 0) return 'ov-action-num--red'
    if (index === 1) return 'ov-action-num--amber'
    return 'ov-action-num--blue'
  }
  if (status === 'risk') {
    return index === 0 ? 'ov-action-num--amber' : 'ov-action-num--blue'
  }
  return 'ov-action-num--blue'
}

// ─────────────────────────────────────────────────────────────────────────────

// 新增：infra 文字颜色 helper
function infraTextClass(value: number | null | undefined, warnThreshold: number, errorThreshold: number): string {
  if (value === null || value === undefined) return 'text-gray-400 dark:text-gray-500'
  if (value >= errorThreshold) return 'text-red-600 dark:text-red-400'
  if (value >= warnThreshold) return 'text-amber-600 dark:text-amber-400'
  return 'text-emerald-600 dark:text-emerald-400'
}

function infraStatusTag(value: number | null | undefined, warnThreshold: number, errorThreshold: number): string {
  if (value === null || value === undefined) return ''
  if (value >= errorThreshold) return '异常'
  if (value >= warnThreshold) return '关注'
  return '正常'
}

// 新增：状态徽标 class
const statusBadgeClass = computed(() => {
  switch (String(displayOverview?.value?.status || '').trim().toLowerCase()) {
    case 'incident': return 'ov-status-badge--red'
    case 'risk': return 'ov-status-badge--amber'
    case 'observing': return 'ov-status-badge--blue'
    default: return 'ov-status-badge--green'
  }
})

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
/* ── Action bar ── */
.ov-actionbar {
  @apply sticky top-0 z-40 -mx-4 bg-white px-4 pb-0 border-b border-gray-200 dark:bg-dark-900 dark:border-dark-700 md:-mx-6 md:px-6;
}

/* ── Time chips ── */
.ov-chip {
  @apply rounded-full border border-gray-200 px-3 py-1 text-xs font-medium text-gray-600 transition hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-300 dark:hover:border-blue-500;
}
.ov-chip--active {
  @apply bg-blue-600 border-blue-600 text-white dark:bg-blue-500 dark:border-blue-500;
}

/* ── Buttons ── */
.ov-btn {
  @apply inline-flex items-center gap-1.5 rounded-xl border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-700 transition hover:border-blue-300 hover:text-blue-600 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300;
}
.ov-btn--primary {
  @apply bg-blue-600 border-blue-600 text-white hover:bg-blue-700 hover:border-blue-700 hover:text-white dark:bg-blue-500 dark:border-blue-500 dark:hover:bg-blue-600;
}

/* ── Inputs ── */
.ov-input-sm {
  @apply h-8 rounded-lg border border-gray-300 bg-white px-2.5 text-xs text-gray-900 outline-none transition focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-dark-600 dark:bg-dark-900 dark:text-white;
}

/* ── Card ── */
.ov-card {
  @apply rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900;
}

/* ── Section label ── */
.ov-section-label {
  @apply mb-3 flex items-center gap-1.5 text-xs font-bold uppercase tracking-wider text-gray-400 dark:text-gray-500;
}

/* ── Dot ── */
.ov-dot {
  @apply inline-block h-1.5 w-1.5 shrink-0 rounded-full;
}
.ov-dot--red { @apply bg-red-500; }
.ov-dot--amber { @apply bg-amber-500; }
.ov-dot--blue { @apply bg-blue-500; }
.ov-dot--green { @apply bg-emerald-500; }
.ov-dot--gray { @apply bg-gray-400; }

/* ── Badge ── */
.ov-badge {
  @apply inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold;
}

/* ── Hero score ── */
.ov-hero-score {
  @apply text-6xl font-black leading-none tracking-tight;
}

/* ── Score track bar ── */
.ov-score-track {
  @apply relative h-2 w-full max-w-[200px] overflow-visible rounded-full;
  background: linear-gradient(90deg, #ef4444 0 49%, #f59e0b 49% 69%, #60a5fa 69% 89%, #10b981 89%);
}
.ov-score-arrow {
  @apply absolute top-[-5px] h-[18px] w-0.5 -translate-x-1/2 rounded-sm bg-gray-900 dark:bg-gray-100;
}
.ov-score-track-labels {
  @apply mt-1 flex max-w-[200px] justify-between text-[10px] text-gray-400 dark:text-gray-500;
}

/* ── Status banner ── */
.ov-banner-top {
  @apply min-h-[52px];
}

/* ── Alert pills ── */
.ov-apill {
  @apply inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-bold;
}
.ov-apill--red { @apply bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300; }
.ov-apill--amber { @apply bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300; }
.ov-apill--blue { @apply bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300; }

/* ── Tags ── */
.ov-tag {
  @apply inline-flex cursor-pointer items-center rounded-full border border-gray-200 px-2 py-0.5 text-xs font-medium text-gray-600 transition hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-400;
}
.ov-tag--model {
  @apply border-red-200 bg-red-50 text-red-700 hover:border-red-300 hover:bg-red-100 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300;
}
.ov-tag--account {
  @apply border-amber-200 bg-amber-50 text-amber-700 hover:border-amber-300 hover:bg-amber-100 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300;
}

/* ── Stat rows ── */
.ov-stat-row {
  @apply flex min-h-[34px] items-center gap-0 border-b border-gray-100 px-5 last:border-b-0 dark:border-dark-800;
}
.ov-stat-label {
  @apply w-28 shrink-0 text-xs font-semibold text-gray-400 dark:text-gray-500;
}
.ov-stat-value {
  @apply flex flex-wrap items-center gap-2 text-sm font-bold text-gray-900 dark:text-gray-100;
}
.ov-stat-sub {
  @apply text-xs font-normal text-gray-400 dark:text-gray-500;
}

/* ── Action items ── */
.ov-action-item {
  @apply flex items-start gap-2.5 rounded-xl border p-3;
}
.ov-action-item--red { @apply border-red-200 bg-red-50 dark:border-red-800 dark:bg-red-900/20; }
.ov-action-item--amber { @apply border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-900/20; }
.ov-action-item--blue { @apply border-blue-200 bg-blue-50 dark:border-blue-800 dark:bg-blue-900/20; }

.ov-action-num {
  @apply flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[11px] font-bold text-white;
}
.ov-action-num--red { @apply bg-red-500; }
.ov-action-num--amber { @apply bg-amber-500; }
.ov-action-num--blue { @apply bg-blue-500; }

/* ── Infra cards ── */
.ov-infra-card {
  @apply rounded-2xl p-4 text-center;
}
.ov-infra-card--ok { @apply bg-emerald-50 dark:bg-emerald-900/20; }
.ov-infra-card--warn { @apply bg-amber-50 dark:bg-amber-900/20; }
.ov-infra-card--error { @apply bg-red-50 dark:bg-red-900/20; }
.ov-infra-card--neutral { @apply bg-gray-50 dark:bg-dark-800/70; }

.ov-infra-label {
  @apply mb-1.5 text-xs font-bold uppercase tracking-wide;
}
.ov-infra-card--ok .ov-infra-label { @apply text-emerald-700 dark:text-emerald-400; }
.ov-infra-card--warn .ov-infra-label { @apply text-amber-700 dark:text-amber-400; }
.ov-infra-card--error .ov-infra-label { @apply text-red-700 dark:text-red-400; }
.ov-infra-card--neutral .ov-infra-label { @apply text-gray-500 dark:text-gray-400; }

.ov-infra-val {
  @apply text-xl font-black;
}
.ov-infra-card--ok .ov-infra-val { @apply text-emerald-700 dark:text-emerald-400; }
.ov-infra-card--warn .ov-infra-val { @apply text-amber-700 dark:text-amber-400; }
.ov-infra-card--error .ov-infra-val { @apply text-red-700 dark:text-red-400; }
.ov-infra-card--neutral .ov-infra-val { @apply text-gray-600 dark:text-gray-300; }

.ov-infra-hint {
  @apply mt-1 text-[10px];
}
.ov-infra-card--ok .ov-infra-hint { @apply text-emerald-600 dark:text-emerald-500; }
.ov-infra-card--warn .ov-infra-hint { @apply text-amber-600 dark:text-amber-500; }
.ov-infra-card--error .ov-infra-hint { @apply text-red-600 dark:text-red-500; }
.ov-infra-card--neutral .ov-infra-hint { @apply text-gray-400 dark:text-gray-500; }

/* ── Page wrapper ── */
.ov-page {
  @apply pb-12;
}

/* ── Filter label ── */
.ov-filter-label {
  @apply text-xs font-medium text-gray-500 dark:text-gray-400;
}

/* ── Select ── */
.ov-select {
  @apply h-7 rounded-lg border border-gray-200 bg-white px-2 text-xs text-gray-900 outline-none transition focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-dark-600 dark:bg-dark-900 dark:text-white;
}

/* ── Section title ── */
.ov-section-title {
  @apply text-xs font-bold uppercase tracking-wide text-gray-500 dark:text-gray-400;
}

/* ── Metric cell ── */
.ov-metric-cell {
  @apply rounded-xl bg-gray-50 px-3 py-2.5 dark:bg-dark-800/60;
}
.ov-metric-label {
  @apply mb-1 text-[10px] font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500;
}
.ov-metric-val {
  @apply text-lg font-black leading-none;
}

/* ── Score ring ── */
.ov-score-ring {
  @apply relative flex h-20 w-20 items-center justify-center;
}
.ov-score-ring-svg {
  @apply absolute inset-0 h-full w-full;
}
.ov-score-ring-inner {
  @apply relative flex flex-col items-center;
}

/* ── Status badge ── */
.ov-status-badge {
  @apply inline-flex items-center rounded-full px-2.5 py-0.5 text-[11px] font-bold;
}
.ov-status-badge--green { @apply bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300; }
.ov-status-badge--blue { @apply bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300; }
.ov-status-badge--amber { @apply bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300; }
.ov-status-badge--red { @apply bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300; }

/* ── Infra row (new design) ── */
.ov-infra-row {
  @apply flex items-center justify-between gap-2;
}
.ov-infra-name {
  @apply text-xs text-gray-500 dark:text-gray-400 truncate;
}
.ov-infra-status {
  @apply shrink-0 text-xs font-semibold;
}

/* ── Impact cell ── */
.ov-impact-cell {
  @apply flex flex-col items-center rounded-xl bg-gray-50 px-2 py-3 dark:bg-dark-800/60;
}
.ov-impact-icon {
  @apply mb-1.5 h-5 w-5;
}
.ov-impact-val {
  @apply text-xl font-black leading-none text-gray-900 dark:text-white;
}
.ov-impact-label {
  @apply mt-1 text-[10px] font-medium text-gray-400 dark:text-gray-500 text-center;
}

/* ── Report columns ── */
.ov-report-col {
  @apply rounded-xl bg-gray-50 p-3 dark:bg-dark-800/60;
}
.ov-report-col-title {
  @apply mb-1.5 text-[10px] font-bold uppercase tracking-wide text-gray-400 dark:text-gray-500;
}
.ov-report-col-body {
  @apply text-xs text-gray-700 dark:text-gray-200 leading-relaxed;
}
</style>
