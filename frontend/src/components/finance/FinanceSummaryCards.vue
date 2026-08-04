<template>
  <div class="space-y-4" data-testid="finance-summary">
    <div class="rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm text-blue-900 dark:border-blue-900 dark:bg-blue-950/30 dark:text-blue-100">
      <p class="font-semibold">先看这三个关系</p>
      <p class="mt-1"><strong>客户充值</strong>是客户打进来的钱；<strong>客户本期计费</strong>是客户这段时间用服务被扣的钱；<strong>客户还剩余额</strong>是客户以后还能用的钱。充值减余额只能大致反映消费，还会受退款、赠送和历史余额影响。</p>
      <p class="mt-1"><strong>预计赚/亏</strong> = 已经能对上上游成本的客户计费 − 上游本期使用成本 − 支付手续费。还有成本没核准时，这个数会标记为“含估算”。</p>
    </div>

    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <article v-for="item in primaryMetrics" :key="item.key" class="card p-5">
        <div class="flex items-start justify-between gap-3">
          <div>
            <p class="text-sm text-gray-500 dark:text-gray-400">{{ item.label }}</p>
            <p class="mt-2 text-2xl font-semibold tabular-nums" :class="item.tone">{{ item.value }}</p>
          </div>
          <span class="rounded-full px-2 py-1 text-xs font-medium" :class="item.statusClass">{{ item.status }}</span>
        </div>
        <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ item.secondary }}</p>
      </article>
    </div>

    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <article v-for="item in secondaryMetrics" :key="item.key" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ item.label }}</p>
        <p class="mt-2 text-xl font-semibold tabular-nums" :class="item.tone">{{ item.value }}</p>
        <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ item.hint }}</p>
      </article>
    </div>

    <div class="card overflow-hidden">
      <div class="border-b border-gray-200 px-4 py-3 dark:border-dark-700">
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">钱在哪里</h2>
      </div>
      <div class="grid grid-cols-1 divide-y divide-gray-100 sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-4 dark:divide-dark-700">
        <div v-for="item in periodMetrics" :key="item.key" class="p-4">
          <p class="text-sm text-gray-500">{{ item.label }}</p>
          <p class="mt-2 text-xl font-semibold tabular-nums" :class="item.tone">{{ item.value }}</p>
          <p class="mt-2 text-xs text-gray-500">{{ item.hint }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { FinanceFunds, FinanceOverview, FinanceOverviewMetric } from "@/api/admin/finance";
import { financeTone, formatFinanceMoney, formatFinancePercent } from "./format";

const props = defineProps<{ overview: FinanceOverview; funds?: FinanceFunds | null }>();

function metricStatus(metric: FinanceOverviewMetric | null | undefined) {
  if (!metric || !metric.amount || metric.status === "unavailable") return { text: "待补数据", className: "bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300" };
  if (metric.status === "complete") return { text: "数据完整", className: "bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300" };
  return { text: "含估算", className: "bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300" };
}

function changeText(metric: FinanceOverviewMetric) {
  if (!metric.previous_amount) return "上一周期无法比较";
  const change = metric.change_rate === null ? "无法比较" : formatFinancePercent(metric.change_rate);
  return `上一周期 ${formatFinanceMoney(metric.previous_amount, metric.currency)} · 变化 ${change}`;
}

const primaryMetrics = computed(() => [
  {
    key: "revenue", label: "客户本期计费", value: formatFinanceMoney(props.overview.revenue.amount, props.overview.revenue.currency),
    tone: "text-gray-900 dark:text-white", secondary: `客户请求扣费 + 订阅按天扣费 · ${changeText(props.overview.revenue)}`,
    status: metricStatus(props.overview.revenue).text, statusClass: metricStatus(props.overview.revenue).className,
  },
  {
    key: "cost", label: "上游本期使用成本", value: formatFinanceMoney(props.overview.upstream_cost.amount, props.overview.upstream_cost.currency),
    tone: "text-gray-900 dark:text-white", secondary: props.overview.quality.estimated_count > 0 ? `上游实际消耗中有 ${props.overview.quality.estimated_count} 笔仍是估算值` : changeText(props.overview.upstream_cost),
    status: metricStatus(props.overview.upstream_cost).text, statusClass: metricStatus(props.overview.upstream_cost).className,
  },
  {
    key: "profit", label: "预计赚 / 亏", value: formatFinanceMoney(props.overview.profit.amount, props.overview.profit.currency),
    tone: financeTone(props.overview.profit.amount), secondary: props.overview.profit.status === "complete" ? changeText(props.overview.profit) : "部分上游成本还没核准，金额会继续校正",
    status: metricStatus(props.overview.profit).text, statusClass: metricStatus(props.overview.profit).className,
  },
  {
    key: "margin", label: "每 100 元剩下", value: formatFinancePercent(props.overview.margin_rate),
    tone: financeTone(props.overview.margin_rate), secondary: `预计赚/亏占可计算客户计费的比例 · 成本覆盖率 ${formatFinancePercent(props.overview.quality.cost_coverage_rate)}`,
    status: props.overview.quality.status === "complete" ? "数据完整" : "含估算",
    statusClass: props.overview.quality.status === "complete" ? "bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300" : "bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300",
  },
]);

const secondaryMetrics = computed(() => [
  { key: "loss", label: "本期倒贴 / 少收", value: formatFinanceMoney(props.overview.loss_amount), tone: "text-red-600 dark:text-red-400", hint: `${props.overview.loss_request_count} 条请求的上游成本高于客户收费` },
  { key: "estimated", label: "还没核准的上游成本", value: formatFinanceMoney(props.overview.pending_settlement_cost ?? props.overview.estimated_cost_risk), tone: "text-amber-600 dark:text-amber-400", hint: "现在只能按价格或账号倍率估算" },
  { key: "unpriced", label: "暂时算不出利润的消费", value: formatFinanceMoney(props.overview.unconfigured_revenue_exposure ?? props.overview.quality.unpriced_revenue), tone: "text-amber-600 dark:text-amber-400", hint: "客户已被计费，但上游成本资料不完整" },
  { key: "alerts", label: "系统发现的风险", value: String(props.overview.open_alert_count), tone: props.overview.open_alert_count > 0 ? "text-red-600 dark:text-red-400" : "text-emerald-600 dark:text-emerald-400", hint: "系统自动记录亏损、成本缺失和余额同步异常，不要求手工确认" },
]);

const periodMetrics = computed(() => [
  { key: "customer-payment", label: "客户本期充值（收到）", value: formatFinanceMoney(props.funds?.customer_cash.payment), tone: "text-gray-900 dark:text-white", hint: "客户本期打进来的钱，不等于利润" },
  { key: "customer-balance", label: "客户还剩余额（欠客户）", value: formatFinanceMoney(props.funds?.customer_balance), tone: "text-gray-900 dark:text-white", hint: "客户以后还能消费的钱，不是你的利润" },
  { key: "upstream-topup", label: "上游本期充值（付出去）", value: props.funds?.upstream_cash.topup_available ? formatFinanceMoney(props.funds.upstream_cash.topup) : "未录入", tone: props.funds?.upstream_cash.topup_available ? "text-gray-900 dark:text-white" : "text-amber-700 dark:text-amber-300", hint: props.funds?.upstream_cash.topup_available ? `${props.funds.upstream_cash.topup_event_count} 笔充值流水；未用掉的仍在上游` : "系统没有上游充值流水" },
  { key: "wallet", label: "上游可用余额（现在）", value: formatFinanceMoney(props.overview.wallet_cash_total), tone: "text-gray-900 dark:text-white", hint: props.funds?.stale_wallet_count ? `${props.funds.stale_wallet_count} 个过期钱包未计入；详情仍显示最后记录值` : "只汇总最近同步成功且不重复的余额" },
]);
</script>
