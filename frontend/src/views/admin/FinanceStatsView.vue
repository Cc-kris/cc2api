<template>
  <AppLayout>
    <div class="space-y-6">
      <header
        class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between"
      >
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            财务管理
          </h1>
          <p class="mt-1 text-sm text-gray-500">
            用历史计费快照核算收入、上游成本和利润，并持续追踪亏损与资金风险。
          </p>
        </div>
        <p v-if="overview" class="text-xs text-gray-500">
          数据生成时间：{{ formatFinanceDate(overview.generated_at) }}
        </p>
      </header>

      <section class="card p-4" aria-label="财务筛选条件">
        <div
          class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-[minmax(160px,1fr)_minmax(160px,1fr)_minmax(180px,1fr)_150px_auto]"
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
          <label class="block text-sm text-gray-600 dark:text-gray-300"
            >时区<input
              v-model.trim="timezone"
              class="input mt-1 w-full"
              data-testid="finance-timezone"
          /></label>
          <label class="block text-sm text-gray-600 dark:text-gray-300"
            >统计粒度<select
              v-model="granularity"
              class="input mt-1 w-full"
              data-testid="finance-granularity"
            >
              <option value="hour">小时</option>
              <option value="day">天</option>
              <option value="week">周</option>
              <option value="month">月</option>
            </select></label
          >
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
          99%，报表只展示已覆盖范围毛利；未定价收入和缺失成本记录已单独列入风险。
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
          正在加载经营总览...
        </div>
        <template v-else-if="overview"
          ><FinanceSummaryCards :overview="overview" /><ProfitTrendChart
            :items="trend"
            :loading="loading.trend"
            :error="errors.trend"
        /></template>
        <div v-else class="card p-10 text-center text-sm text-gray-500">
          暂无经营总览数据
        </div>
      </section>

      <FinanceInitializationPanel v-else-if="activeTab === 'initialization'" @initialized="loadAll" />

      <FinanceAnalysisTable
        v-else-if="activeTab === 'profit'"
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
        v-else-if="activeTab === 'losses'"
        :items="losses"
        :loading="loading.losses"
        :error="errors.losses"
        :page="lossPagination.page"
        :page-size="lossPagination.page_size"
        :total="lossPagination.total"
        @open-alert="openLossAlert"
        @update:page="changeLossPage"
        @update:page-size="changeLossPageSize"
      />

      <section v-else-if="activeTab === 'funds'">
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
      </section>

      <FinanceSettlementPanel v-else-if="activeTab === 'settlements'" />

      <FinanceFXRatePanel v-else-if="activeTab === 'fx-rates'" />

      <section v-else-if="activeTab === 'quality'">
        <div v-if="errors.quality" class="card p-6 text-sm text-red-600">
          {{ errors.quality }}
        </div>
        <div
          v-else-if="loading.quality"
          class="card p-10 text-center text-sm text-gray-500"
        >
          正在加载数据质量...
        </div>
        <div v-else-if="dataQuality" class="space-y-4">
          <FinanceDataQualityPanel :data="dataQuality" /><FinanceBackfillPanel
            :start-date="startDate"
            :end-date="endDate"
          />
        </div>
        <div v-else class="card p-10 text-center text-sm text-gray-500">
          暂无数据质量结果
        </div>
      </section>

      <div v-else class="space-y-4">
        <FinanceAlertsPanel
          :items="alerts"
          :loading="loading.alerts"
          :error="errors.alerts"
          :updating-id="updatingAlertId"
          :page="alertPagination.page"
          :page-size="alertPagination.page_size"
          :total="alertPagination.total"
          @update="handleAlertUpdate"
          @update:page="changeAlertPage"
          @update:page-size="changeAlertPageSize"
        /><FinanceReconciliationPanel />
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import AppLayout from "@/components/layout/AppLayout.vue";
import FinanceAlertsPanel from "@/components/finance/FinanceAlertsPanel.vue";
import FinanceBackfillPanel from "@/components/finance/FinanceBackfillPanel.vue";
import FinanceAnalysisTable from "@/components/finance/FinanceAnalysisTable.vue";
import FinanceDataQualityPanel from "@/components/finance/FinanceDataQualityPanel.vue";
import FinanceFundsPanel from "@/components/finance/FinanceFundsPanel.vue";
import FinanceFXRatePanel from "@/components/finance/FinanceFXRatePanel.vue";
import FinanceInitializationPanel from "@/components/finance/FinanceInitializationPanel.vue";
import FinanceLossTable from "@/components/finance/FinanceLossTable.vue";
import FinanceReconciliationPanel from "@/components/finance/FinanceReconciliationPanel.vue";
import FinanceSummaryCards from "@/components/finance/FinanceSummaryCards.vue";
import FinanceSettlementPanel from "@/components/finance/FinanceSettlementPanel.vue";
import ProfitTrendChart from "@/components/finance/ProfitTrendChart.vue";
import { formatFinanceDate, financeNumber } from "@/components/finance/format";
import { adminAPI } from "@/api/admin";
import type {
  FinanceAlert,
  FinanceBreakdownDimension,
  FinanceBreakdownItem,
  FinanceDataQuality,
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
  | "funds"
  | "quality"
  | "alerts";
type FinanceTab =
  | "overview"
  | "profit"
  | "losses"
  | "funds"
  | "settlements"
  | "fx-rates"
  | "initialization"
  | "quality"
  | "alerts";

const tabs: Array<{ key: FinanceTab; label: string }> = [
  { key: "overview", label: "经营总览" },
  { key: "initialization", label: "财务初始化" },
  { key: "profit", label: "利润分析" },
  { key: "losses", label: "亏损追踪" },
  { key: "funds", label: "资金余额" },
  { key: "settlements", label: "成本结算" },
  { key: "fx-rates", label: "汇率管理" },
  { key: "quality", label: "数据质量" },
  { key: "alerts", label: "财务预警" },
];

function localDate(daysAgo = 0) {
  const date = new Date();
  date.setDate(date.getDate() - daysAgo);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

const startDate = ref(localDate(29));
const endDate = ref(localDate());
const timezone = ref(
  Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Shanghai",
);
const granularity = ref<FinanceGranularity>("day");
const activeTab = ref<FinanceTab>("overview");
const dimension = ref<FinanceBreakdownDimension>("user");
const refreshing = ref(false);
const updatingAlertId = ref<number | null>(null);
const exporting = ref(false);
const exportJob = ref<FinanceExportJob | null>(null);
const exportError = ref("");
let exportPollTimer: ReturnType<typeof setTimeout> | null = null;

const overview = ref<FinanceOverview | null>(null);
const trend = ref<FinanceTrendItem[]>([]);
const breakdown = ref<FinanceBreakdownItem[]>([]);
const losses = ref<FinanceLossItem[]>([]);
const funds = ref<FinanceFunds | null>(null);
const dataQuality = ref<FinanceDataQuality | null>(null);
const alerts = ref<FinanceAlert[]>([]);
const breakdownPagination = reactive({ page: 1, page_size: 50, total: 0 });
const lossPagination = reactive({ page: 1, page_size: 50, total: 0 });
const alertPagination = reactive({ page: 1, page_size: 50, total: 0 });

const loading = reactive<Record<FinanceSection, boolean>>({
  overview: false,
  trend: false,
  breakdown: false,
  losses: false,
  funds: false,
  quality: false,
  alerts: false,
});
const errors = reactive<Record<FinanceSection, string>>({
  overview: "",
  trend: "",
  breakdown: "",
  losses: "",
  funds: "",
  quality: "",
  alerts: "",
});
const dateError = computed(() =>
  !startDate.value || !endDate.value
    ? "请选择完整的开始日期和结束日期"
    : startDate.value > endDate.value
      ? "开始日期不能晚于结束日期"
      : "",
);
const coverageRate = computed(() =>
  financeNumber(
    overview.value?.quality.cost_coverage_rate ??
      dataQuality.value?.quality.cost_coverage_rate,
  ),
);
const coverageRisk = computed(
  () =>
    Boolean(overview.value || dataQuality.value) &&
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

function setLoading(value: boolean) {
  for (const section of Object.keys(loading) as FinanceSection[])
    loading[section] = value;
}
function clearResults() {
  overview.value = null;
  trend.value = [];
  breakdown.value = [];
  losses.value = [];
  funds.value = null;
  dataQuality.value = null;
  alerts.value = [];
  breakdownPagination.page = 1;
  breakdownPagination.total = 0;
  lossPagination.page = 1;
  lossPagination.total = 0;
  alertPagination.page = 1;
  alertPagination.total = 0;
  resetFinanceExport();
  for (const section of Object.keys(errors) as FinanceSection[])
    errors[section] = "";
}

function rejectMessage(reason: unknown, fallback: string) {
  return extractApiErrorMessage(reason, fallback);
}

async function loadAll() {
  if (dateError.value) return;
  refreshing.value = true;
  clearResults();
  setLoading(true);
  const params = filters();
  const results = await Promise.allSettled([
    adminAPI.finance.getOverview(params),
    adminAPI.finance.getTrend(params),
    adminAPI.finance.getBreakdown({
      ...params,
      dimension: dimension.value,
      sort_by: "profit",
      sort_order: "asc",
      page: breakdownPagination.page,
      page_size: breakdownPagination.page_size,
    }),
    adminAPI.finance.getLosses({
      ...params,
      sort_by: "profit",
      sort_order: "asc",
      page: lossPagination.page,
      page_size: lossPagination.page_size,
    }),
    adminAPI.finance.getFunds(params),
    adminAPI.finance.getDataQuality({ ...params, data_scope: "all" }),
    adminAPI.finance.getAlerts({
      ...params,
      page: alertPagination.page,
      page_size: alertPagination.page_size,
    }),
  ]);
  const sections: FinanceSection[] = [
    "overview",
    "trend",
    "breakdown",
    "losses",
    "funds",
    "quality",
    "alerts",
  ];
  const fallbacks = [
    "经营总览加载失败",
    "利润趋势加载失败",
    "利润分析加载失败",
    "亏损记录加载失败",
    "资金余额加载失败",
    "数据质量加载失败",
    "财务预警加载失败",
  ];
  results.forEach((result, index) => {
    const section = sections[index];
    loading[section] = false;
    if (result.status === "rejected") {
      errors[section] = rejectMessage(result.reason, fallbacks[index]);
      return;
    }
    if (section === "overview")
      overview.value = result.value as FinanceOverview;
    else if (section === "trend")
      trend.value = result.value as FinanceTrendItem[];
    else if (section === "breakdown") {
      const response = result.value as {
        items: FinanceBreakdownItem[] | null;
        total: number;
        page: number;
        page_size: number;
      };
      breakdown.value = response.items || [];
      Object.assign(breakdownPagination, {
        total: response.total,
        page: response.page,
        page_size: response.page_size,
      });
    } else if (section === "losses") {
      const response = result.value as {
        items: FinanceLossItem[] | null;
        total: number;
        page: number;
        page_size: number;
      };
      losses.value = response.items || [];
      Object.assign(lossPagination, {
        total: response.total,
        page: response.page,
        page_size: response.page_size,
      });
    } else if (section === "funds") funds.value = result.value as FinanceFunds;
    else if (section === "quality")
      dataQuality.value = result.value as FinanceDataQuality;
    else {
      const response = result.value as {
        items: FinanceAlert[] | null;
        total: number;
        page: number;
        page_size: number;
      };
      alerts.value = response.items || [];
      Object.assign(alertPagination, {
        total: response.total,
        page: response.page,
        page_size: response.page_size,
      });
    }
  });
  refreshing.value = false;
}

async function changeDimension(value: string) {
  dimension.value = value as FinanceBreakdownDimension;
  breakdownPagination.page = 1;
  resetFinanceExport();
  await loadBreakdown();
}

async function loadBreakdown() {
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
    breakdown.value = result.items || [];
    Object.assign(breakdownPagination, {
      total: result.total,
      page: result.page,
      page_size: result.page_size,
    });
  } catch (error) {
    errors.breakdown = rejectMessage(error, "利润分析加载失败");
  } finally {
    loading.breakdown = false;
  }
}

async function loadLosses() {
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
    losses.value = result.items || [];
    Object.assign(lossPagination, {
      total: result.total,
      page: result.page,
      page_size: result.page_size,
    });
  } catch (error) {
    errors.losses = rejectMessage(error, "亏损记录加载失败");
  } finally {
    loading.losses = false;
  }
}

async function loadAlerts() {
  loading.alerts = true;
  errors.alerts = "";
  alerts.value = [];
  try {
    const result = await adminAPI.finance.getAlerts({
      ...filters(),
      page: alertPagination.page,
      page_size: alertPagination.page_size,
    });
    alerts.value = result.items || [];
    Object.assign(alertPagination, {
      total: result.total,
      page: result.page,
      page_size: result.page_size,
    });
  } catch (error) {
    errors.alerts = rejectMessage(error, "财务预警加载失败");
  } finally {
    loading.alerts = false;
  }
}

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
function changeAlertPage(page: number) {
  alertPagination.page = page;
  void loadAlerts();
}
function changeAlertPageSize(pageSize: number) {
  alertPagination.page_size = pageSize;
  alertPagination.page = 1;
  void loadAlerts();
}

async function handleAlertUpdate(payload: {
  id: number;
  status: FinanceAlert["status"];
  note: string;
}) {
  updatingAlertId.value = payload.id;
  errors.alerts = "";
  try {
    await adminAPI.finance.updateAlert(payload.id, {
      status: payload.status,
      note: payload.note,
    });
    const [alertResult, overviewResult] = await Promise.all([
      adminAPI.finance.getAlerts({
        ...filters(),
        page: alertPagination.page,
        page_size: alertPagination.page_size,
      }),
      adminAPI.finance.getOverview(filters()),
    ]);
    alerts.value = alertResult.items || [];
    Object.assign(alertPagination, {
      total: alertResult.total,
      page: alertResult.page,
      page_size: alertResult.page_size,
    });
    overview.value = overviewResult;
  } catch (error) {
    errors.alerts = rejectMessage(error, "财务预警处理失败");
  } finally {
    updatingAlertId.value = null;
  }
}

function openLossAlert() {
  activeTab.value = "alerts";
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
