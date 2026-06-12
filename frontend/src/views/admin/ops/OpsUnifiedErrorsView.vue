<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <section class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">统一错误列表</h1>
          </div>
          <div class="flex flex-wrap items-center gap-3">
            <button
              v-if="fromOverview"
              type="button"
              class="inline-flex items-center gap-2 rounded-xl border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300"
              @click="backToOverview"
            >
              返回运维总览
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300"
              :disabled="loading"
              @click="fetchErrors"
            >
              刷新
            </button>
            <button
              v-if="canRunManualAIAnalysis"
              type="button"
              class="inline-flex items-center gap-2 rounded-xl border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300"
              @click="resetFilters"
            >
              重置筛选
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-blue-300 dark:disabled:bg-blue-800/60"
              :disabled="manualAIActionDisabled"
              :title="manualAIActionDisabledReason || undefined"
              @click="runManualAIAnalysis"
            >
              {{ manualAIActionLoading ? '分析中...' : '手动 AI 分析' }}
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-blue-300 dark:disabled:bg-blue-800/60"
              :disabled="exportButtonDisabled"
              :title="exportButtonTitle || undefined"
              @click="exportErrors"
            >
              {{ exporting ? '导出中...' : '导出 CSV' }}
            </button>
          </div>
        </div>

        <div class="mt-3 space-y-1 text-xs text-gray-500 dark:text-gray-400">
          <div>{{ exportHint }}</div>
          <div v-if="manualAIActionDisabledReason">{{ manualAIActionDisabledReason }}</div>
        </div>

        <div v-if="errorMessage" class="mt-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
          {{ errorMessage }}
        </div>

        <div v-if="fromOverview && hasActiveFilters" class="mt-4 flex items-center justify-between rounded-2xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-700 dark:border-blue-900/40 dark:bg-blue-900/20 dark:text-blue-300">
          <div>当前筛选来自运维总览</div>
          <button type="button" class="ml-4 inline-flex rounded border border-blue-300 px-3 py-1 text-xs font-medium hover:bg-blue-100 dark:border-blue-700 dark:hover:bg-blue-900/40" @click="resetFilters">
            清空并重置
          </button>
        </div>

        <div class="mt-5 space-y-4">
          <!-- Row 1: 时间范围 + 关键词 + 状态码 -->
          <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
            <div>
              <label class="filter-label">时间范围</label>
              <select v-model="timeRange" class="input">
                <option v-for="option in timeRangeOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </div>
            <div>
              <label class="filter-label">关键词</label>
              <input v-model.trim="keyword" type="text" class="input" placeholder="搜索脱敏摘要">
            </div>
            <div>
              <label class="filter-label">状态码</label>
              <input v-model.trim="statusCode" type="text" class="input" placeholder="429,500-504">
            </div>
          </div>

          <!-- Row 2: 错误大类 Toggle 组 (全宽，flex wrap) -->
          <div>
            <label class="filter-label">错误大类</label>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="option in errorCategoryOptions"
                :key="option.value"
                type="button"
                :class="['toggle-button', errorCategories.includes(option.value) ? 'toggle-button-active' : 'toggle-button-inactive']"
                @click="toggleCategory(option.value)"
              >
                {{ option.label }}
              </button>
            </div>
          </div>

          <!-- Row 3: 错误结果 Toggle 组 + 平台 Toggle 组 (同行) -->
          <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div>
              <label class="filter-label">错误结果</label>
              <div class="flex flex-wrap gap-2">
                <button
                  v-for="option in errorResultOptions"
                  :key="option.value"
                  type="button"
                  :class="['toggle-button', errorResults.includes(option.value) ? 'toggle-button-active' : 'toggle-button-inactive']"
                  @click="toggleErrorResult(option.value)"
                >
                  {{ option.label }}
                </button>
              </div>
            </div>
            <div>
              <label class="filter-label">平台</label>
              <div class="flex flex-wrap gap-2">
                <button
                  type="button"
                  :class="['toggle-button', !platform ? 'toggle-button-active' : 'toggle-button-inactive']"
                  @click="platform = ''"
                >
                  全部
                </button>
                <button
                  v-for="value in ['openai', 'claude', 'gemini', 'other']"
                  :key="value"
                  type="button"
                  :class="['toggle-button', platform === value ? 'toggle-button-active' : 'toggle-button-inactive']"
                  @click="platform = value"
                >
                  {{ formatPlatformLabel(value) }}
                </button>
              </div>
            </div>
          </div>

          <!-- 分组（保留在"更多筛选"中） -->
          <div v-if="showMoreFilters" class="grid grid-cols-1 gap-3 xl:grid-cols-5">
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

          <div v-if="showMoreFilters" class="grid grid-cols-1 gap-3 xl:grid-cols-6">
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
            <button
              type="button"
              class="inline-flex items-center rounded-xl border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300"
              @click="showMoreFilters = !showMoreFilters"
            >
              {{ showMoreFilters ? '收起筛选项' : moreFiltersButtonLabel }}
            </button>
          </div>

          <div v-if="!showMoreFilters && advancedFilterSummaries.length" class="flex flex-wrap gap-2">
            <span
              v-for="summary in advancedFilterSummaries"
              :key="summary"
              class="rounded-full bg-blue-50 px-3 py-1 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-200"
            >
              {{ summary }}
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
          <!-- Batch operations bar -->
          <transition name="slide-down">
            <div v-if="selectedCount > 0" class="sticky top-0 z-20 border-b border-gray-200 bg-blue-50 px-5 py-3 dark:border-dark-700 dark:bg-blue-900/20">
              <div class="flex items-center justify-between">
                <div class="text-sm font-medium text-gray-700 dark:text-gray-200">
                  已选 {{ selectedCount }} 条 / {{ total }} 条
                </div>
                <div class="flex items-center gap-3">
                  <button
                    type="button"
                    class="inline-flex items-center gap-2 rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-blue-300 dark:disabled:bg-blue-800/60"
                    :disabled="batchAnalysisDisabled"
                    :title="batchAnalysisTitle"
                    @click="batchAnalyzeSelected"
                  >
                    {{ batchAnalyzing ? '分析中...' : '合并时段 AI 分析' }}
                  </button>
                  <button
                    type="button"
                    class="inline-flex items-center gap-2 rounded-xl border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300"
                    @click="clearSelection"
                  >
                    取消选择
                  </button>
                </div>
              </div>
            </div>
          </transition>

          <table class="min-w-[1300px] border-separate border-spacing-0">
            <thead class="sticky top-0 z-10 bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="table-th w-12">
                  <input
                    type="checkbox"
                    :checked="allSelected"
                    :indeterminate="indeterminate"
                    class="rounded border-gray-300"
                    @change="toggleAllSelection"
                  >
                </th>
                <th v-if="visibleColumns.has('occurred_at')" class="table-th">
                  <button type="button" class="sort-button" @click="toggleSort('occurred_at')">
                    发生时间
                    <span>{{ sortIndicator('occurred_at') }}</span>
                  </button>
                </th>
                <th v-if="visibleColumns.has('error_category')" class="table-th">错误分类</th>
                <th v-if="visibleColumns.has('error_subcategory')" class="table-th">错误子类</th>
                <th v-if="visibleColumns.has('severity')" class="table-th">严重度</th>
                <th v-if="visibleColumns.has('summary')" class="table-th">错误摘要</th>
                <th v-if="visibleColumns.has('error_result')" class="table-th">错误结果</th>
                <th v-if="visibleColumns.has('status_code')" class="table-th">
                  <button type="button" class="sort-button" @click="toggleSort('status_code')">
                    状态码
                    <span>{{ sortIndicator('status_code') }}</span>
                  </button>
                </th>
                <th v-if="visibleColumns.has('user')" class="table-th">用户</th>
                <th v-if="visibleColumns.has('group')" class="table-th">分组</th>
                <th v-if="visibleColumns.has('platform')" class="table-th">平台</th>
                <th v-if="visibleColumns.has('model')" class="table-th">模型</th>
                <th v-if="visibleColumns.has('upstream_account')" class="table-th">上游账号</th>
                <th v-if="visibleColumns.has('api_key')" class="table-th">API Key</th>
                <th v-if="visibleColumns.has('request_id')" class="table-th">请求 ID</th>
                <th v-if="visibleColumns.has('same_kind_count')" class="table-th">
                  <button type="button" class="sort-button" @click="toggleSort('same_kind_count')">
                    同类数量
                    <span>{{ sortIndicator('same_kind_count') }}</span>
                  </button>
                </th>
                <th v-if="visibleColumns.has('ai_analysis_status')" class="table-th">AI 状态</th>
                <th class="table-th w-12 relative">
                  <button
                    type="button"
                    class="inline-flex h-6 w-6 items-center justify-center rounded hover:bg-gray-200 dark:hover:bg-dark-700"
                    @click="columnPopoverOpen = !columnPopoverOpen"
                  >
                    ⚙
                  </button>
                  <!-- Column control popover -->
                  <div v-if="columnPopoverOpen" class="absolute right-0 top-full z-30 mt-1 rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-900">
                    <div class="p-3 space-y-2 max-h-96 overflow-y-auto">
                      <div v-for="col in allColumns" :key="col.key" class="flex items-center gap-2">
                        <input
                          type="checkbox"
                          :id="`col-${col.key}`"
                          :checked="visibleColumns.has(col.key)"
                          class="rounded border-gray-300"
                          @change="toggleColumnVisibility(col.key)"
                        >
                        <label :for="`col-${col.key}`" class="text-sm text-gray-700 dark:text-gray-200">
                          {{ col.label }}
                          <span v-if="col.lowFreq" class="text-xs text-gray-500 dark:text-gray-400">(低频)</span>
                        </label>
                      </div>
                    </div>
                  </div>
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-if="items.length === 0">
                <td :colspan="visibleColumns.size + 2" class="py-16 text-center text-sm text-gray-400 dark:text-dark-500">
                  暂无符合条件的错误记录
                </td>
              </tr>
              <tr
                v-for="item in items"
                :key="item.id"
                :class="['cursor-pointer transition hover:bg-gray-50 dark:hover:bg-dark-800/50', getSeverityBorderClass(item.severity)]"
              >
                <td class="table-td w-12">
                  <input
                    type="checkbox"
                    :checked="selectedIds.has(item.id)"
                    class="rounded border-gray-300"
                    @change="toggleRowSelection(item.id)"
                    @click.stop
                  >
                </td>
                <td v-if="visibleColumns.has('occurred_at')" class="table-td whitespace-nowrap font-mono text-xs" @click="openDetail(item.id)">{{ formatDateTime(item.occurred_at) }}</td>
                <td v-if="visibleColumns.has('error_category')" class="table-td" @click="openDetail(item.id)">
                  <span class="badge badge-neutral">{{ formatCategory(item.error_category) }}</span>
                </td>
                <td v-if="visibleColumns.has('error_subcategory')" class="table-td" @click="openDetail(item.id)">{{ formatSubcategory(item.error_subcategory) }}</td>
                <td v-if="visibleColumns.has('severity')" class="table-td" @click="openDetail(item.id)">
                  <span :class="getSeverityBadgeClass(item.severity)">{{ formatSeverity(item.severity) }}</span>
                </td>
                <td v-if="visibleColumns.has('summary')" class="table-td" @click="openDetail(item.id)">
                  <div class="max-w-[320px] truncate" :title="item.summary || '暂无摘要'">
                    {{ item.summary || '暂无摘要' }}
                  </div>
                </td>
                <td v-if="visibleColumns.has('error_result')" class="table-td" @click="openDetail(item.id)">
                  <span class="badge badge-result">{{ formatErrorResult(item.error_result) }}</span>
                </td>
                <td v-if="visibleColumns.has('status_code')" class="table-td" @click="openDetail(item.id)">
                  <span class="badge badge-status">{{ item.status_code || '--' }}</span>
                </td>
                <td v-if="visibleColumns.has('user')" class="table-td" @click="openDetail(item.id)">{{ formatEntity(item.user, '未知用户') }}</td>
                <td v-if="visibleColumns.has('group')" class="table-td" @click="openDetail(item.id)">{{ formatEntity(item.group, '未分组') }}</td>
                <td v-if="visibleColumns.has('platform')" class="table-td" @click="openDetail(item.id)">{{ item.platform || '未知平台' }}</td>
                <td v-if="visibleColumns.has('model')" class="table-td" @click="openDetail(item.id)">{{ item.model || '--' }}</td>
                <td v-if="visibleColumns.has('upstream_account')" class="table-td" @click="openDetail(item.id)">{{ formatEntity(item.upstream_account, '未命中上游') }}</td>
                <td v-if="visibleColumns.has('api_key')" class="table-td" @click="openDetail(item.id)">{{ formatEntity(item.api_key, '--') }}</td>
                <td v-if="visibleColumns.has('request_id')" class="table-td" @click="openDetail(item.id)">{{ '--' }}</td>
                <td v-if="visibleColumns.has('same_kind_count')" class="table-td" @click="openDetail(item.id)">{{ item.same_kind_count || 1 }}</td>
                <td v-if="visibleColumns.has('ai_analysis_status')" class="table-td" @click.stop>
                  <button
                    v-if="item.ai_analysis_status === 'not_analyzed'"
                    type="button"
                    class="badge badge-ai hover:opacity-80 cursor-pointer"
                    :disabled="rowsBeingAnalyzed.has(item.id)"
                    @click="triggerRowAIAnalysis(item)"
                  >
                    {{ rowsBeingAnalyzed.has(item.id) ? '分析中' : formatAIStatus(item.ai_analysis_status) }}
                  </button>
                  <button
                    v-else-if="item.ai_analysis_status === 'analyzed' || item.ai_analysis_status === 'completed'"
                    type="button"
                    class="badge badge-ai hover:opacity-80 cursor-pointer"
                    @click="showAIReport(item)"
                  >
                    {{ formatAIStatus(item.ai_analysis_status) }}
                  </button>
                  <span v-else class="badge badge-ai">{{ formatAIStatus(item.ai_analysis_status) }}</span>
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

    <!-- EL04: AI Report Drawer -->
    <transition name="slide-from-right">
      <div v-if="drawerOpen" class="fixed inset-0 z-40 overflow-hidden">
        <div class="absolute inset-0 bg-black/30 dark:bg-black/50" @click="drawerOpen = false"></div>
        <div class="absolute right-0 top-0 bottom-0 w-full max-w-md overflow-y-auto rounded-l-2xl border-l border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-900">
          <div class="flex items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">AI 分析报告</h2>
            <button
              type="button"
              class="inline-flex h-8 w-8 items-center justify-center rounded hover:bg-gray-100 dark:hover:bg-dark-800"
              @click="drawerOpen = false"
            >
              ✕
            </button>
          </div>

          <div v-if="drawerLoading" class="flex items-center justify-center py-12">
            <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-blue-600"></div>
          </div>

          <div v-else-if="drawerReport" class="space-y-4 p-5">
            <div>
              <h3 class="text-xs font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">分析结论</h3>
              <p class="mt-2 text-sm text-gray-700 dark:text-gray-200">{{ drawerReport.summary || '--' }}</p>
            </div>

            <div v-if="drawerReport.confidence" class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
              <div class="text-xs font-semibold text-gray-600 dark:text-gray-300">置信度</div>
              <div class="mt-1 text-sm font-medium text-blue-600 dark:text-blue-300">{{ drawerReport.confidence }}</div>
            </div>

            <div v-if="drawerReport.suggested_actions">
              <h3 class="text-xs font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">建议行动</h3>
              <ul class="mt-2 space-y-2">
                <li
                  v-for="(action, idx) in ((typeof drawerReport.suggested_actions === 'string' ? [drawerReport.suggested_actions] : drawerReport.suggested_actions) || []).slice(0, 3)"
                  :key="idx"
                  class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-200"
                >
                  <span class="mt-1 flex-shrink-0 h-1.5 w-1.5 rounded-full bg-blue-500"></span>
                  <span>{{ action }}</span>
                </li>
              </ul>
            </div>

            <div v-if="drawerReport.root_cause" class="rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20">
              <div class="text-xs font-semibold text-amber-700 dark:text-amber-300">根本原因</div>
              <div class="mt-1 text-sm text-amber-900 dark:text-amber-100">{{ drawerReport.root_cause }}</div>
            </div>
          </div>

          <div v-else class="flex items-center justify-center py-12">
            <div class="text-center">
              <p class="text-sm text-gray-500 dark:text-gray-400">暂无 AI 报告数据</p>
            </div>
          </div>
        </div>
      </div>
    </transition>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import { adminAPI } from '@/api'
import { exportUnifiedErrors, listUnifiedErrors, opsAPI, type OpsUnifiedEntityRef, type OpsUnifiedErrorItem, type OpsUnifiedErrorListQueryParams } from '@/api/admin/ops'
import { useAppStore, useAuthStore } from '@/stores'
import { formatDateTime } from '@/utils/format'
import { canManageManualAIAnalysis, fetchOpsAIAnalysisConfig, isManualAIAnalysisConfigured, type OpsAIAnalysisConfigSnapshot } from './utils/manualAIAnalysis'

type GroupOption = {
  id: number
  name: string
}

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()

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
  { value: 'client_auth_error', label: '凭证/访问控制错误' },
  { value: 'client_rate_limit_error', label: '限流错误' },
  { value: 'client_balance_error', label: '余额或额度错误' },
  { value: 'client_group_error', label: '分组不可用' },
  { value: 'client_subscription_error', label: '订阅错误' },
  { value: 'client_parameter_error', label: '参数错误' },
  { value: 'client_model_error', label: '模型错误' },
  { value: 'client_path_error', label: '路径错误' },
  { value: 'client_context_error', label: '上下文错误' },
  { value: 'client_disconnect_error', label: '客户端断开/上传中断' },
  { value: 'client_insufficient_evidence', label: '证据不足' }
]


const errorSubcategoryLabels: Record<string, string> = {
  client_auth_error: '凭证/访问控制错误',
  client_rate_limit_error: '限流错误',
  client_balance_error: '余额或额度错误',
  client_group_error: '分组不可用',
  client_subscription_error: '订阅错误',
  client_parameter_error: '参数错误',
  client_model_error: '模型或渠道错误',
  client_path_error: '路径或方法错误',
  client_context_error: '上下文超限',
  client_disconnect_error: '客户端断开/上传中断',
  client_insufficient_evidence: '证据不足',
  account_pool_empty: '账号池无可用账号',
  upstream_rate_limit: '上游限流',
  upstream_permission_error: '上游权限错误',
  upstream_balance_error: '上游余额或额度不足',
  upstream_timeout: '上游超时',
  upstream_unavailable: '上游不可用',
  upstream_error: '上游错误',
  config_model_mapping_error: '配置或模型映射错误',
  slow_response: '慢请求',
  platform_dependency_error: '平台依赖错误',
  platform_internal_error: '平台内部错误',
  unknown_insufficient_evidence: '证据不足'
}

const loading = ref(false)
const exporting = ref(false)
const manualAIActionLoading = ref(false)
const hasLoadedOnce = ref(false)
const errorMessage = ref('')
const items = ref<OpsUnifiedErrorItem[]>([])
const total = ref(0)
const groups = ref<GroupOption[]>([])
const manualAIConfig = ref<OpsAIAnalysisConfigSnapshot | null>(null)
const manualAIConfigLoaded = ref(false)
const manualAIConfigLoadError = ref('')
const activeManualAITaskId = ref<number | null>(null)
let manualAIPollTimer: ReturnType<typeof setTimeout> | null = null

// EL04: AI status column interactions
const rowsBeingAnalyzed = ref<Set<number>>(new Set())
const drawerOpen = ref(false)
const drawerLoading = ref(false)
const drawerErrorId = ref<number | null>(null)
const drawerTaskId = ref<number | null>(null)
const drawerReport = ref<any>(null)

// EL05: Multi-select + batch operations
const selectedIds = ref<Set<number>>(new Set())
const batchAnalyzing = ref(false)

// EL06: Column control
const visibleColumns = ref<Set<string>>(new Set([
  'occurred_at',
  'error_category',
  'error_subcategory',
  'severity',
  'summary',
  'error_result',
  'status_code',
  'user',
  'group',
  'platform',
  'model',
  'same_kind_count',
  'ai_analysis_status'
]))
const allColumns = [
  { key: 'occurred_at', label: '发生时间', lowFreq: false },
  { key: 'error_category', label: '错误分类', lowFreq: false },
  { key: 'error_subcategory', label: '错误子类', lowFreq: false },
  { key: 'severity', label: '严重度', lowFreq: false },
  { key: 'summary', label: '错误摘要', lowFreq: false },
  { key: 'error_result', label: '错误结果', lowFreq: false },
  { key: 'status_code', label: '状态码', lowFreq: false },
  { key: 'user', label: '用户', lowFreq: false },
  { key: 'group', label: '分组', lowFreq: false },
  { key: 'platform', label: '平台', lowFreq: false },
  { key: 'model', label: '模型', lowFreq: false },
  { key: 'upstream_account', label: '上游账号', lowFreq: true },
  { key: 'api_key', label: 'API Key', lowFreq: true },
  { key: 'request_id', label: '请求 ID', lowFreq: true },
  { key: 'same_kind_count', label: '同类数量', lowFreq: false },
  { key: 'ai_analysis_status', label: 'AI 状态', lowFreq: false }
]
const columnPopoverOpen = ref(false)

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
const showMoreFilters = ref(false)
const sortBy = ref<'occurred_at' | 'status_code' | 'severity' | 'same_kind_count'>('occurred_at')
const sortOrder = ref<'asc' | 'desc'>('desc')
const page = ref(1)
const pageSize = ref('20')

const numericPageSize = () => Number.parseInt(pageSize.value, 10) || 20

const exportAllowedRoles = new Set(['admin', 'ops', 'operation', 'operator'])

const currentViewerRole = computed(() => String((authStore.user as { role?: string } | null)?.role || '').trim().toLowerCase())
const canExport = computed(() => exportAllowedRoles.has(currentViewerRole.value))
const canRunManualAIAnalysis = computed(() => canManageManualAIAnalysis(currentViewerRole.value))
const exportButtonDisabled = computed(() => exporting.value || !canExport.value)
const exportButtonTitle = computed(() => {
  if (exporting.value) return '正在导出错误列表'
  if (!canExport.value) return '当前账号无权限执行此操作'
  return '导出当前筛选条件下的错误列表 CSV'
})

// EL05: Batch selection
const allSelected = computed(() => items.value.length > 0 && items.value.length === selectedIds.value.size)
const indeterminate = computed(() => selectedIds.value.size > 0 && !allSelected.value)
const selectedCount = computed(() => selectedIds.value.size)
const batchAnalysisDisabled = computed(() => selectedCount.value === 0 || selectedCount.value > 50 || batchAnalyzing.value)
const batchAnalysisTitle = computed(() => {
  if (selectedCount.value === 0) return '请先选择至少一条错误'
  if (selectedCount.value > 50) return '最多只能选择 50 条错误'
  return `合并分析已选择的 ${selectedCount.value} 条错误`
})

const fromOverview = computed(() => firstQueryValue(route.query.from_overview) === '1')
const hasActiveFilters = computed(() => {
  return (
    errorCategories.value.length > 0 ||
    errorResults.value.length > 0 ||
    platform.value.trim() !== '' ||
    statusCode.value.trim() !== '' ||
    keyword.value.trim() !== '' ||
    timeRange.value !== '30m' ||
    groupId.value.trim() !== ''
  )
})
const advancedFilterCount = computed(() => {
  let count = 0
  if (model.value.trim()) count++
  if (aiAnalysis.value !== 'all') count++
  if (errorResults.value.length) count++
  if (severities.value.length) count++
  if (errorSubcategoriesInput.value.trim()) count++
  if (clientErrorSubcategories.value.length) count++
  if (userId.value.trim()) count++
  if (apiKeyId.value.trim()) count++
  if (upstreamAccountId.value.trim()) count++
  if (requestId.value.trim()) count++
  return count
})
const advancedFilterSummaries = computed(() => {
  const summaries: string[] = []
  const modelValue = model.value.trim()
  const errorSubcategoryValue = errorSubcategoriesInput.value.trim()
  const userIdValue = userId.value.trim()
  const apiKeyIdValue = apiKeyId.value.trim()
  const upstreamAccountIdValue = upstreamAccountId.value.trim()
  const requestIdValue = requestId.value.trim()

  if (modelValue) summaries.push(`模型：${modelValue}`)
  if (aiAnalysis.value !== 'all') summaries.push(`AI 分析：${aiAnalysis.value === 'analyzed' ? '已分析' : '未分析'}`)
  if (errorResults.value.length) summaries.push(`错误结果：${formatOptionValues(errorResultOptions, errorResults.value)}`)
  if (severities.value.length) summaries.push(`严重度：${formatOptionValues(severityOptions, severities.value)}`)
  if (errorSubcategoryValue) summaries.push(`错误子类：${errorSubcategoryValue}`)
  if (clientErrorSubcategories.value.length) summaries.push(`客户端错误细分：${formatOptionValues(clientErrorSubcategoryOptions, clientErrorSubcategories.value)}`)
  if (userIdValue) summaries.push(`用户 ID：${userIdValue}`)
  if (apiKeyIdValue) summaries.push(`API Key ID：${apiKeyIdValue}`)
  if (upstreamAccountIdValue) summaries.push(`上游账号 ID：${upstreamAccountIdValue}`)
  if (requestIdValue) summaries.push(`请求 ID：${requestIdValue}`)
  return summaries
})
const moreFiltersButtonLabel = computed(() => advancedFilterCount.value > 0 ? `更多筛选项（${advancedFilterCount.value}）` : '更多筛选项')
const exportHint = computed(() => {
  if (!canExport.value) return '当前账号无权限导出错误列表。'
  return '仅平台所有者、运维可导出；导出范围最长 7 天，最多 100000 行。'
})

const selectedWindowMs = computed(() => {
  if (timeRange.value === 'custom') {
    const startIso = buildDateTimeQuery(customStartInput.value)
    const endIso = buildDateTimeQuery(customEndInput.value)
    if (!startIso || !endIso) return 0
    return Math.max(0, new Date(endIso).getTime() - new Date(startIso).getTime())
  }
  switch (timeRange.value) {
    case '30m':
      return 30 * 60 * 1000
    case '1h':
      return 60 * 60 * 1000
    case '6h':
      return 6 * 60 * 60 * 1000
    case '24h':
      return 24 * 60 * 60 * 1000
    case '7d':
      return 7 * 24 * 60 * 60 * 1000
    default:
      return 0
  }
})

const manualAIActionDisabledReason = computed(() => {
  if (!canRunManualAIAnalysis.value) return '当前账号无权限执行此操作'
  if (manualAIActionLoading.value || activeManualAITaskId.value) return '分析任务处理中，请稍后查看'
  if (manualAIConfigLoadError.value) return manualAIConfigLoadError.value
  if (!manualAIConfigLoaded.value) return 'AI 配置加载完成后可发起分析'
  if (!isManualAIAnalysisConfigured(manualAIConfig.value)) return '请先配置 AI 分析服务'
  if (selectedWindowMs.value > 24 * 60 * 60 * 1000) return '手动 AI 分析时间范围不能超过 24 小时'
  if (total.value <= 0) return '当前范围暂无可分析错误'
  return ''
})

const manualAIActionDisabled = computed(() => manualAIActionDisabledReason.value !== '')

function splitCSV(value: string): string[] {
  return value.split(',').map((item) => item.trim()).filter(Boolean)
}

function firstQueryValue(value: unknown): string {
  if (Array.isArray(value)) return String(value[0] ?? '')
  return typeof value === 'string' ? value : ''
}

function formatOptionValues(options: Array<{ value: string; label: string }>, values: string[]): string {
  const optionLabelMap = new Map(options.map((option) => [option.value, option.label]))
  return values.map((value) => optionLabelMap.get(value) || value).join('、')
}

function toggleCategory(value: string) {
  const index = errorCategories.value.indexOf(value)
  if (index >= 0) {
    errorCategories.value.splice(index, 1)
  } else {
    errorCategories.value.push(value)
  }
}

function toggleErrorResult(value: string) {
  const index = errorResults.value.indexOf(value)
  if (index >= 0) {
    errorResults.value.splice(index, 1)
  } else {
    errorResults.value.push(value)
  }
}

function formatPlatformLabel(value: string): string {
  const map: Record<string, string> = {
    openai: 'OpenAI',
    claude: 'Claude',
    gemini: 'Gemini',
    other: '其他'
  }
  return map[value] || value
}

function parsePositiveInt(value: string): number | null {
  if (!value.trim()) return null
  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
}

function formatDateTimeInput(value: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day}T${hours}:${minutes}`
}

function buildDateTimeQuery(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
}

function initializeFromRoute() {
  timeRange.value = firstQueryValue(route.query.time_range) || '30m'
  customStartInput.value = formatDateTimeInput(firstQueryValue(route.query.start_time))
  customEndInput.value = formatDateTimeInput(firstQueryValue(route.query.end_time))
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
    const startIso = buildDateTimeQuery(customStartInput.value)
    const endIso = buildDateTimeQuery(customEndInput.value)
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
  if (fromOverview.value) query.from_overview = '1'

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

function buildExportFilename(): string {
  const now = new Date()
  const pad2 = (value: number) => String(value).padStart(2, '0')
  return `ops-unified-errors-${now.getFullYear()}${pad2(now.getMonth() + 1)}${pad2(now.getDate())}${pad2(now.getHours())}${pad2(now.getMinutes())}${pad2(now.getSeconds())}.csv`
}

async function extractExportErrorMessage(err: any): Promise<string> {
  const blob = err?.response?.data
  if (blob instanceof Blob) {
    try {
      const text = await blob.text()
      if (text) {
        try {
          const parsed = JSON.parse(text)
          if (typeof parsed?.message === 'string' && parsed.message.trim()) return parsed.message.trim()
          if (typeof parsed?.detail === 'string' && parsed.detail.trim()) return parsed.detail.trim()
          if (typeof parsed?.error === 'string' && parsed.error.trim()) return parsed.error.trim()
        } catch {
          if (text.trim()) return text.trim()
        }
      }
    } catch {
      // ignore blob parsing failure and fall back below
    }
  }

  return err?.message || err?.response?.data?.detail || '错误列表导出失败，请稍后重试'
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

async function exportErrors() {
  if (exporting.value) return
  if (!canExport.value) {
    appStore.showError('当前账号无权限执行此操作')
    return
  }

  exporting.value = true
  try {
    await syncRouteQuery()
    const blob = await exportUnifiedErrors(buildApiParams())
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = buildExportFilename()
    link.click()
    URL.revokeObjectURL(url)
    appStore.showSuccess('错误列表导出成功')
  } catch (err: any) {
    const message = await extractExportErrorMessage(err)
    appStore.showError(message)
  } finally {
    exporting.value = false
  }
}

function getCurrentRangeBounds(): { start: string, end: string } | null {
  if (timeRange.value === 'custom') {
    const start = buildDateTimeQuery(customStartInput.value)
    const end = buildDateTimeQuery(customEndInput.value)
    if (!start || !end) return null
    return { start, end }
  }

  const endTime = new Date()
  const startTime = new Date(endTime.getTime() - selectedWindowMs.value)
  return {
    start: startTime.toISOString(),
    end: endTime.toISOString()
  }
}

function buildManualAIAnalysisFilters(): Record<string, any> {
  const filters: Record<string, any> = {}
  if (errorCategories.value.length) filters.error_categories = [...errorCategories.value]
  const errorSubcategories = splitCSV(errorSubcategoriesInput.value)
  if (errorSubcategories.length) filters.error_subcategories = errorSubcategories
  if (clientErrorSubcategories.value.length) filters.client_error_subcategories = [...clientErrorSubcategories.value]
  if (errorResults.value.length) filters.error_results = [...errorResults.value]
  if (severities.value.length) filters.severity = [...severities.value]

  const statusCodes = statusCode.value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
    .flatMap((segment) => {
      const rangeMatch = segment.match(/^(\d{3})-(\d{3})$/)
      if (rangeMatch) {
        const start = Number.parseInt(rangeMatch[1], 10)
        const end = Number.parseInt(rangeMatch[2], 10)
        if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return []
        return Array.from({ length: end - start + 1 }, (_, index) => start + index)
      }
      const code = Number.parseInt(segment, 10)
      return Number.isFinite(code) ? [code] : []
    })
  if (statusCodes.length) filters.status_code = Array.from(new Set(statusCodes)).sort((a, b) => a - b)

  const parsedUserId = parsePositiveInt(userId.value)
  if (parsedUserId) filters.user_id = parsedUserId
  const parsedApiKeyId = parsePositiveInt(apiKeyId.value)
  if (parsedApiKeyId) filters.api_key_id = parsedApiKeyId
  const parsedGroupId = parsePositiveInt(groupId.value)
  if (parsedGroupId) filters.group_id = parsedGroupId
  if (platform.value.trim()) filters.platform = platform.value.trim()
  if (model.value.trim()) filters.model = model.value.trim()
  const parsedUpstreamAccountId = parsePositiveInt(upstreamAccountId.value)
  if (parsedUpstreamAccountId) filters.upstream_account_id = parsedUpstreamAccountId
  if (requestId.value.trim()) filters.request_id = requestId.value.trim()
  if (keyword.value.trim()) filters.keyword = keyword.value.trim()
  return filters
}

async function runManualAIAnalysis() {
  if (manualAIActionDisabled.value || manualAIActionLoading.value) return
  const currentRange = getCurrentRangeBounds()
  if (!currentRange) {
    appStore.showError('请选择完整的开始和结束时间')
    return
  }

  manualAIActionLoading.value = true
  try {
    const result = await opsAPI.createAIAnalysisTask({
      source_type: 'manual_filter',
      time_start: currentRange.start,
      time_end: currentRange.end,
      filters: buildManualAIAnalysisFilters()
    })
    activeManualAITaskId.value = result.task_id
    appStore.showSuccess(result.message || 'AI 分析任务已提交')
    await fetchErrors()
    startManualAITaskPolling(result.task_id)
  } catch (err: any) {
    appStore.showError(err?.message || '创建 AI 分析任务失败')
  } finally {
    manualAIActionLoading.value = false
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
  showMoreFilters.value = false
  sortBy.value = 'occurred_at'
  sortOrder.value = 'desc'
  page.value = 1
  pageSize.value = '20'
  void fetchErrors()
}

function backToOverview() {
  void router.push({ name: 'AdminOpsOverview' })
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

function formatSubcategory(value: string | null | undefined): string {
  if (!value) return '未细分'
  return errorSubcategoryLabels[value] || value
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

function getSeverityBadgeClass(severity: string | undefined): string {
  switch (severity) {
    case 'P0':
      return 'badge badge-p0'
    case 'P1':
      return 'badge badge-p1'
    case 'P2':
      return 'badge badge-p2'
    case 'observe':
      return 'badge badge-observe'
    case 'normal':
      return 'badge badge-normal'
    default:
      return 'badge badge-normal'
  }
}

function getSeverityBorderClass(severity: string | undefined): string {
  switch (severity) {
    case 'P0':
      return 'border-l-4 border-l-red-500'
    case 'P1':
      return 'border-l-4 border-l-orange-500'
    case 'P2':
      return 'border-l-4 border-l-blue-500'
    default:
      return 'border-l-4 border-l-transparent'
  }
}

function formatSeverity(value: string | undefined): string {
  switch (value) {
    case 'P0': return 'P0'
    case 'P1': return 'P1'
    case 'P2': return 'P2'
    case 'observe': return '观察'
    case 'normal': return '普通'
    default: return value || '--'
  }
}

function openDetail(id: number) {
  if (!id) return
  void router.push({ name: 'AdminOpsUnifiedErrorDetail', params: { id: String(id) } })
}

// EL04: Single row AI analysis trigger
async function triggerRowAIAnalysis(item: OpsUnifiedErrorItem) {
  if (rowsBeingAnalyzed.value.has(item.id)) return

  const rangeStart = new Date(new Date(item.occurred_at).getTime() - 15 * 60 * 1000)
  const rangeEnd = new Date(new Date(item.occurred_at).getTime() + 15 * 60 * 1000)

  rowsBeingAnalyzed.value.add(item.id)
  try {
    await opsAPI.createAIAnalysisTask({
      source_type: 'manual_filter',
      time_start: rangeStart.toISOString(),
      time_end: rangeEnd.toISOString(),
      filters: {
        error_categories: [item.error_category]
      }
    })
    appStore.showSuccess('AI 分析任务已提交')
    await fetchErrors()
  } catch (err: any) {
    appStore.showError(err?.message || 'AI 分析任务提交失败')
  } finally {
    rowsBeingAnalyzed.value.delete(item.id)
  }
}

// EL04: Show AI report drawer
async function showAIReport(item: OpsUnifiedErrorItem) {
  if (!item.id) return
  drawerErrorId.value = item.id
  drawerOpen.value = true
  drawerLoading.value = true
  drawerReport.value = null
  drawerTaskId.value = null

  try {
    const detail = await opsAPI.getUnifiedErrorDetail(item.id)
    if (detail.ai_analysis?.task_id) {
      drawerTaskId.value = detail.ai_analysis.task_id
      const taskDetail = await opsAPI.getAIAnalysisTaskDetail(detail.ai_analysis.task_id)
      drawerReport.value = taskDetail.report || null
    }
  } catch (err: any) {
    appStore.showError(err?.message || '加载 AI 报告失败')
    drawerOpen.value = false
  } finally {
    drawerLoading.value = false
  }
}

// EL05: Toggle row selection
function toggleRowSelection(id: number) {
  if (selectedIds.value.has(id)) {
    selectedIds.value.delete(id)
  } else {
    selectedIds.value.add(id)
  }
}

// EL05: Toggle all selections
function toggleAllSelection() {
  if (allSelected.value) {
    selectedIds.value.clear()
  } else {
    selectedIds.value.clear()
    items.value.forEach(item => selectedIds.value.add(item.id))
  }
}

// EL05: Clear selections
function clearSelection() {
  selectedIds.value.clear()
}

// EL05: Batch AI analysis
async function batchAnalyzeSelected() {
  if (selectedIds.value.size === 0 || selectedIds.value.size > 50 || batchAnalyzing.value) return

  const selectedItems = items.value.filter(item => selectedIds.value.has(item.id))
  if (selectedItems.length === 0) return

  const occurredTimes = selectedItems.map(item => new Date(item.occurred_at).getTime())
  const startTime = new Date(Math.min(...occurredTimes))
  const endTime = new Date(Math.max(...occurredTimes))

  batchAnalyzing.value = true
  try {
    await opsAPI.createAIAnalysisTask({
      source_type: 'manual_filter',
      time_start: startTime.toISOString(),
      time_end: endTime.toISOString(),
      filters: {}
    })
    appStore.showSuccess(`已为 ${selectedItems.length} 条错误提交合并分析`)
    clearSelection()
    await fetchErrors()
  } catch (err: any) {
    appStore.showError(err?.message || '批量分析提交失败')
  } finally {
    batchAnalyzing.value = false
  }
}

// EL06: Toggle column visibility
function toggleColumnVisibility(key: string) {
  if (visibleColumns.value.has(key)) {
    visibleColumns.value.delete(key)
  } else {
    visibleColumns.value.add(key)
  }
  saveColumnSettings()
}

// EL06: Save column settings to localStorage
function saveColumnSettings() {
  const cols = Array.from(visibleColumns.value)
  localStorage.setItem('ops_errors_visible_cols', JSON.stringify(cols))
}

// EL06: Load column settings from localStorage
function loadColumnSettings() {
  try {
    const saved = localStorage.getItem('ops_errors_visible_cols')
    if (saved) {
      const cols = JSON.parse(saved) as string[]
      visibleColumns.value = new Set(cols)
    }
  } catch {
    // Fall back to defaults if localStorage is corrupted
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

function stopManualAITaskPolling() {
  if (manualAIPollTimer) {
    clearTimeout(manualAIPollTimer)
    manualAIPollTimer = null
  }
}

async function pollManualAITask(taskId: number) {
  try {
    const detail = await opsAPI.getAIAnalysisTaskDetail(taskId)
    const status = String(detail.task.status || '').trim().toLowerCase()
    const shouldContinuePolling =
      status === 'pending' ||
      status === 'running' ||
      (status === 'completed' && !detail.report)
    if (shouldContinuePolling) {
      manualAIPollTimer = setTimeout(() => {
        void pollManualAITask(taskId)
      }, 5000)
      return
    }
  } catch {
    // keep polling best-effort; backend errors should not break the page state
    manualAIPollTimer = setTimeout(() => {
      void pollManualAITask(taskId)
    }, 5000)
    return
  }

  activeManualAITaskId.value = null
  stopManualAITaskPolling()
  await fetchErrors()
}

function startManualAITaskPolling(taskId: number) {
  stopManualAITaskPolling()
  void pollManualAITask(taskId)
}

onMounted(async () => {
  initializeFromRoute()
  loadColumnSettings()
  await Promise.all([loadGroups(), loadManualAIAnalysisConfig()])
  await fetchErrors()
})

onUnmounted(() => {
  stopManualAITaskPolling()
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

.toggle-button {
  @apply inline-flex items-center rounded-lg border px-3 py-2 text-sm font-medium transition;
}

.toggle-button-active {
  @apply border-blue-600 bg-blue-600 text-white;
}

.toggle-button-inactive {
  @apply border-gray-300 bg-white text-gray-700 hover:border-blue-400 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-200 dark:hover:border-blue-500;
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

/* Transitions */
.slide-down-enter-active,
.slide-down-leave-active {
  transition: all 0.2s ease;
}

.slide-down-enter-from {
  transform: translateY(-100%);
  opacity: 0;
}

.slide-down-leave-to {
  transform: translateY(-100%);
  opacity: 0;
}

.slide-from-right-enter-active,
.slide-from-right-leave-active {
  transition: all 0.3s ease;
}

.slide-from-right-enter-from,
.slide-from-right-leave-to {
  transform: translateX(100%);
}

</style>
