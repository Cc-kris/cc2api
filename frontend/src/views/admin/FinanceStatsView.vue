<template>
  <AppLayout>
    <div class="space-y-6">
      <header
        class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between"
      >
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            经营与财务
          </h1>
        </div>
        <p v-if="overview" class="text-xs text-gray-500">
          数据生成时间：{{ formatFinanceDate(overview.generated_at) }}
        </p>
      </header>

      <section class="card p-4" aria-label="财务筛选条件">
        <div
          class="grid grid-cols-1 gap-3 sm:grid-cols-[minmax(160px,1fr)_minmax(160px,1fr)_auto]"
        >
          <label class="block text-sm text-gray-600 dark:text-gray-300"
            >开始日期<input
              v-model="startDate"
              type="date"
              class="input mt-1 w-full"
              data-testid="finance-start-date"
          /></label>
          <label class="block text-sm text-gray-600 dark:text-gray-300"
            >结束日期<input
              v-model="endDate"
              type="date"
              class="input mt-1 w-full"
              data-testid="finance-end-date"
          /></label>
          <div class="flex items-end">
            <button
              class="btn btn-primary w-full"
              :disabled="refreshing"
              data-testid="finance-refresh"
              @click="loadAll"
            >
              {{ refreshing ? "刷新中..." : "刷新报表" }}
            </button>
          </div>
        </div>
        <p v-if="dateError" class="mt-3 text-sm text-red-600">
          {{ dateError }}
        </p>
      </section>

      <div
        v-if="coverageRisk"
        class="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200"
        data-testid="finance-coverage-risk"
      >
        <p class="font-semibold">当前财务数据不能代表全站净利润</p>
        <p class="mt-1">
          成本覆盖率低于
          99%，当前利润只能按已有成本估算。系统会自动标记缺失数据，不要求你逐条确认。
        </p>
      </div>

      <nav
        class="flex gap-2 overflow-x-auto border-b border-gray-200 pb-px dark:border-dark-700"
        aria-label="财务管理板块"
      >
        <button
          v-for="tab in tabs"
          :key="tab.key"
          class="whitespace-nowrap border-b-2 px-4 py-3 text-sm font-medium"
          :class="
            activeTab === tab.key
              ? 'border-primary-700 text-primary-700 dark:border-primary-300 dark:text-primary-300'
              : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
          "
          @click="activeTab = tab.key"
        >
          {{ tab.label }}
        </button>
      </nav>

      <section
        v-if="activeTab === 'overview'"
        class="space-y-4"
        data-testid="finance-overview-tab"
      >
        <div v-if="errors.overview" class="card p-6 text-sm text-red-600">
          {{ errors.overview }}
        </div>
        <div
          v-else-if="loading.overview"
          class="card p-10 text-center text-sm text-gray-500"
        >
          正在加载财务总览...
        </div>
        <template v-else-if="overview"
          ><FinanceSummaryCards :overview="overview" :funds="funds" /><ProfitTrendChart
            :items="trend"
            :loading="loading.trend"
            :error="errors.trend"
        /></template>
        <div v-else class="card p-10 text-center text-sm text-gray-500">
          暂无财务总览数据
        </div>
      </section>

      <section v-else-if="activeTab === 'profit'" class="space-y-6">
        <FinanceAnalysisTable
          :items="breakdown"
          :dimension="dimension"
          :loading="loading.breakdown"
          :error="errors.breakdown"
          :page="breakdownPagination.page"
          :page-size="breakdownPagination.page_size"
          :total="breakdownPagination.total"
          :exporting="exporting"
          :export-job="exportJob"
          :export-error="exportError"
          @update:dimension="changeDimension"
          @update:page="changeBreakdownPage"
          @update:page-size="changeBreakdownPageSize"
          @export="startFinanceExport"
          @download="downloadFinanceExport"
        />
        <FinanceLossTable
          :items="losses"
          :loading="loading.losses"
          :error="errors.losses"
          :page="lossPagination.page"
          :page-size="lossPagination.page_size"
          :total="lossPagination.total"
          @update:page="changeLossPage"
          @update:page-size="changeLossPageSize"
        />
      </section>

      <section v-else-if="activeTab === 'funds'" class="space-y-6">
        <div v-if="errors.funds" class="card p-6 text-sm text-red-600">
          {{ errors.funds }}
        </div>
        <div
          v-else-if="loading.funds"
          class="card p-10 text-center text-sm text-gray-500"
        >
          正在加载资金余额...
        </div>
        <FinanceFundsPanel v-else-if="funds" :funds="funds" />
        <div v-else class="card p-10 text-center text-sm text-gray-500">
          暂无资金余额数据
        </div>
        <AccountFinanceSettings @changed="handleFinanceDataChanged" />
      </section>

    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import AppLayout from "@/components/layout/AppLayout.vue";
import AccountFinanceSettings from "@/components/finance/AccountFinanceSettings.vue";
import FinanceAnalysisTable from "@/components/finance/FinanceAnalysisTable.vue";
import FinanceFundsPanel from "@/components/finance/FinanceFundsPanel.vue";
import FinanceLossTable from "@/components/finance/FinanceLossTable.vue";
import FinanceSummaryCards from "@/components/finance/FinanceSummaryCards.vue";
import ProfitTrendChart from "@/components/finance/ProfitTrendChart.vue";
import { formatFinanceDate, financeNumber } from "@/components/finance/format";
import { adminAPI } from "@/api/admin";
import type {
  FinanceBreakdownDimension,
  FinanceBreakdownItem,
  FinanceFilterParams,
  FinanceExportJob,
  FinanceFunds,
  FinanceGranularity,
  FinanceLossItem,
  FinanceOverview,
  FinanceTrendItem,
} from "@/api/admin/finance";
import { extractApiErrorMessage } from "@/utils/apiError";

type FinanceSection =
  | "overview"
  | "trend"
  | "breakdown"
  | "losses"
  | "funds";
type FinanceTab = "overview" | "profit" | "funds";

const tabs: Array<{ key: FinanceTab; label: string }> = [
  { key: "overview", label: "财务总览" },
  { key: "profit", label: "盈利分析" },
  { key: "funds", label: "上游账号" },
];

function localDate(daysAgo = 0) {
  const date = new Date();
  date.setDate(date.getDate() - daysAgo);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

const startDate = ref(localDate(29));
const endDate = ref(localDate());
const timezone = ref("Asia/Shanghai");
const granularity = ref<FinanceGranularity>("day");
const activeTab = ref<FinanceTab>("overview");
const dimension = ref<FinanceBreakdownDimension>("user");
const refreshing = ref(false);
const exporting = ref(false);
const exportJob = ref<FinanceExportJob | null>(null);
const exportError = ref("");
let exportPollTimer: ReturnType<typeof setTimeout> | null = null;
let refreshGeneration = 0;
let breakdownRequestID = 0;
let lossRequestID = 0;

const overview = ref<FinanceOverview | null>(null);
const trend = ref<FinanceTrendItem[]>([]);
const breakdown = ref<FinanceBreakdownItem[]>([]);
const losses = ref<FinanceLossItem[]>([]);
const funds = ref<FinanceFunds | null>(null);
const breakdownPagination = reactive({ page: 1, page_size: 50, total: 0 });
const lossPagination = reactive({ page: 1, page_size: 50, total: 0 });

const loading = reactive<Record<FinanceSection, boolean>>({
  overview: false,
  trend: false,
  breakdown: false,
  losses: false,
  funds: false,
});
const errors = reactive<Record<FinanceSection, string>>({
  overview: "",
  trend: "",
  breakdown: "",
  losses: "",
  funds: "",
});
const dateError = computed(() =>
  !startDate.value || !endDate.value
    ? "请选择完整的开始日期和结束日期"
    : startDate.value > endDate.value
      ? "开始日期不能晚于结束日期"
      : "",
);
const coverageRate = computed(() =>
  financeNumber(overview.value?.quality.cost_coverage_rate),
);
const coverageRisk = computed(
  () =>
    Boolean(overview.value) &&
    (coverageRate.value === null || coverageRate.value < 0.99),
);

function filters(): FinanceFilterParams {
  return {
    start_date: startDate.value,
    end_date: endDate.value,
    timezone: timezone.value || "Asia/Shanghai",
    granularity: granularity.value,
    data_scope: "all",
  };
}

function clearResults() {
  overview.value = null;
  trend.value = [];
  breakdown.value = [];
  losses.value = [];
  funds.value = null;
  breakdownPagination.page = 1;
  breakdownPagination.total = 0;
  lossPagination.page = 1;
  lossPagination.total = 0;
  resetFinanceExport();
  for (const section of Object.keys(errors) as FinanceSection[])
    errors[section] = "";
  for (const section of Object.keys(loading) as FinanceSection[])
    loading[section] = false;
}

function rejectMessage(reason: unknown, fallback: string) {
  return extractApiErrorMessage(reason, fallback);
}

async function loadAll() {
  if (dateError.value) return;
  const generation = ++refreshGeneration;
  refreshing.value = true;
  clearResults();
  // 资金数据独立加载：财务总览超时时，上游余额和充值记录仍然可用。
  void loadFunds(generation);
  loading.overview = true;
  try {
    const result = await adminAPI.finance.getOverview(filters());
    if (generation === refreshGeneration) overview.value = result;
  } catch (error) {
    if (generation === refreshGeneration) errors.overview = rejectMessage(error, "财务总览加载失败");
  } finally {
    if (generation === refreshGeneration) {
      loading.overview = false;
      refreshing.value = false;
    }
  }
  if (generation !== refreshGeneration || !overview.value) return;
  // Trend and cash are useful on the first screen, but must not block the
  // conclusion. The expensive drill-down reports load when their tab opens.
  void loadTrend(generation);
  if (activeTab.value === "profit") {
    void loadBreakdown(generation);
    void loadLosses(generation);
  }
}

async function loadTrend(generation = refreshGeneration) {
  loading.trend = true;
  errors.trend = "";
  try {
    const result = await adminAPI.finance.getTrend(filters());
    if (generation === refreshGeneration) trend.value = result;
  } catch (error) {
    if (generation === refreshGeneration) errors.trend = rejectMessage(error, "利润趋势加载失败");
  } finally {
    if (generation === refreshGeneration) loading.trend = false;
  }
}

async function loadFunds(generation = refreshGeneration) {
  loading.funds = true;
  errors.funds = "";
  try {
    const result = await adminAPI.finance.getFunds(filters());
    if (generation === refreshGeneration) funds.value = result;
  } catch (error) {
    if (generation === refreshGeneration) errors.funds = rejectMessage(error, "资金余额加载失败");
  } finally {
    if (generation === refreshGeneration) loading.funds = false;
  }
}

async function changeDimension(value: string) {
  dimension.value = value as FinanceBreakdownDimension;
  breakdownPagination.page = 1;
  resetFinanceExport();
  await loadBreakdown();
}

async function loadBreakdown(generation = refreshGeneration) {
  const requestID = ++breakdownRequestID;
  loading.breakdown = true;
  errors.breakdown = "";
  breakdown.value = [];
  try {
    const result = await adminAPI.finance.getBreakdown({
      ...filters(),
      dimension: dimension.value,
      sort_by: "profit",
      sort_order: "asc",
      page: breakdownPagination.page,
      page_size: breakdownPagination.page_size,
    });
    if (generation === refreshGeneration && requestID === breakdownRequestID) {
      breakdown.value = result.items || [];
      Object.assign(breakdownPagination, { total: result.total, page: result.page, page_size: result.page_size });
    }
  } catch (error) {
    if (generation === refreshGeneration && requestID === breakdownRequestID) errors.breakdown = rejectMessage(error, "利润分析加载失败");
  } finally {
    if (generation === refreshGeneration && requestID === breakdownRequestID) loading.breakdown = false;
  }
}

async function loadLosses(generation = refreshGeneration) {
  const requestID = ++lossRequestID;
  loading.losses = true;
  errors.losses = "";
  losses.value = [];
  try {
    const result = await adminAPI.finance.getLosses({
      ...filters(),
      sort_by: "profit",
      sort_order: "asc",
      page: lossPagination.page,
      page_size: lossPagination.page_size,
    });
    if (generation === refreshGeneration && requestID === lossRequestID) {
      losses.value = result.items || [];
      Object.assign(lossPagination, { total: result.total, page: result.page, page_size: result.page_size });
    }
  } catch (error) {
    if (generation === refreshGeneration && requestID === lossRequestID) errors.losses = rejectMessage(error, "亏损记录加载失败");
  } finally {
    if (generation === refreshGeneration && requestID === lossRequestID) loading.losses = false;
  }
}

watch(activeTab, (tab) => {
  if (tab === "profit") {
    if (!loading.breakdown && breakdownPagination.total === 0) void loadBreakdown();
    if (!loading.losses && lossPagination.total === 0) void loadLosses();
  }
});

function changeBreakdownPage(page: number) {
  breakdownPagination.page = page;
  void loadBreakdown();
}
function changeBreakdownPageSize(pageSize: number) {
  breakdownPagination.page_size = pageSize;
  breakdownPagination.page = 1;
  void loadBreakdown();
}
function changeLossPage(page: number) {
  lossPagination.page = page;
  void loadLosses();
}
function changeLossPageSize(pageSize: number) {
  lossPagination.page_size = pageSize;
  lossPagination.page = 1;
  void loadLosses();
}
function handleFinanceDataChanged() {
  void loadAll();
}
function resetFinanceExport() {
  if (exportPollTimer) clearTimeout(exportPollTimer);
  exportPollTimer = null;
  exporting.value = false;
  exportJob.value = null;
  exportError.value = "";
}

async function startFinanceExport() {
  if (dateError.value || exporting.value) return;
  resetFinanceExport();
  exporting.value = true;
  try {
    exportJob.value = await adminAPI.finance.createExport({
      report: "breakdown",
      format: "csv",
      filters: {
        start_date: startDate.value,
        end_date: endDate.value,
        dimension: dimension.value,
        data_scope: "all",
        sort_by: "profit",
        sort_order: "asc",
      },
      timezone: timezone.value || "Asia/Shanghai",
    });
    scheduleFinanceExportPoll();
  } catch (error) {
    exporting.value = false;
    exportError.value = rejectMessage(error, "财务报表导出任务创建失败");
  }
}

function scheduleFinanceExportPoll() {
  if (
    !exportJob.value ||
    !["queued", "running"].includes(exportJob.value.status)
  ) {
    exporting.value = false;
    return;
  }
  exportPollTimer = setTimeout(() => void pollFinanceExport(), 1000);
}

async function pollFinanceExport() {
  if (!exportJob.value) return;
  try {
    exportJob.value = await adminAPI.finance.getExport(exportJob.value.id);
    if (exportJob.value.status === "failed") {
      exporting.value = false;
      exportError.value = exportJob.value.error_summary || "财务报表导出失败";
      return;
    }
    scheduleFinanceExportPoll();
  } catch (error) {
    exporting.value = false;
    exportError.value = rejectMessage(error, "财务报表导出状态查询失败");
  }
}

async function downloadFinanceExport() {
  if (!exportJob.value?.download_url) return;
  try {
    const blob = await adminAPI.finance.downloadExport(
      exportJob.value.download_url,
    );
    const objectURL = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = objectURL;
    link.download = `finance-breakdown-${startDate.value}-${endDate.value}.csv`;
    link.click();
    URL.revokeObjectURL(objectURL);
    exportJob.value = { ...exportJob.value, download_url: undefined };
  } catch (error) {
    exportError.value = rejectMessage(
      error,
      "财务报表下载失败，请重新生成下载地址",
    );
  }
}

onMounted(loadAll);
onBeforeUnmount(() => {
  if (exportPollTimer) clearTimeout(exportPollTimer);
});
</script>
