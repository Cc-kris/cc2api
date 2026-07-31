<template>
  <div class="space-y-4" data-testid="finance-summary">
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
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">当前与历史利润</h2>
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
import { computed } from 'vue'

import type { FinanceOverview, FinanceOverviewMetric } from '@/api/admin/finance'
import { financeTone, formatFinanceMoney, formatFinancePercent } from './format'

const props = defineProps<{ overview: FinanceOverview }>()

const rechargeBonusMetric = computed<FinanceOverviewMetric>(() => props.overview.recharge_bonus_income || {
  amount: '0', currency: 'USD', previous_amount: '0', change_rate: null, status: 'complete'
})
const combinedProfitMetric = computed<FinanceOverviewMetric>(() => props.overview.combined_profit || props.overview.profit)
const historicalCombinedProfitMetric = computed<FinanceOverviewMetric>(() => props.overview.historical_combined_profit || props.overview.historical_profit)

function metricStatus(metric: FinanceOverviewMetric | null | undefined) {
  if (!metric || metric.amount === null) return { text: '待完善', className: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300' }
  if (metric.status === 'complete') return { text: '已确认', className: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' }
  return { text: '部分数据', className: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300' }
}

function changeText(metric: FinanceOverviewMetric) {
  if (metric.previous_amount === null) return '上一周期：无法计算'
  const change = metric.change_rate === null ? '无法计算' : formatFinancePercent(metric.change_rate)
  return `上一周期 ${formatFinanceMoney(metric.previous_amount, metric.currency)} · 变化 ${change}`
}

const primaryMetrics = computed(() => [
  {
    key: 'revenue', label: '经营收入', value: formatFinanceMoney(props.overview.revenue.amount, props.overview.revenue.currency),
    tone: 'text-gray-900 dark:text-white', secondary: changeText(props.overview.revenue),
    status: metricStatus(props.overview.revenue).text, statusClass: metricStatus(props.overview.revenue).className
  },
  {
    key: 'cost', label: '账面上游成本', value: formatFinanceMoney(props.overview.upstream_cost.amount, props.overview.upstream_cost.currency),
    tone: 'text-gray-900 dark:text-white', secondary: changeText(props.overview.upstream_cost),
    status: metricStatus(props.overview.upstream_cost).text, statusClass: metricStatus(props.overview.upstream_cost).className
  },
  {
    key: 'profit', label: '账面毛利', value: formatFinanceMoney(props.overview.profit.amount, props.overview.profit.currency),
    tone: financeTone(props.overview.profit.amount), secondary: changeText(props.overview.profit),
    status: metricStatus(props.overview.profit).text, statusClass: metricStatus(props.overview.profit).className
  },
  {
    key: 'combined-profit', label: '综合利润', value: formatFinanceMoney(combinedProfitMetric.value.amount, combinedProfitMetric.value.currency),
    tone: financeTone(combinedProfitMetric.value.amount), secondary: `含充值赠送收益 ${formatFinanceMoney(rechargeBonusMetric.value.amount, rechargeBonusMetric.value.currency)}`,
    status: metricStatus(combinedProfitMetric.value).text, statusClass: metricStatus(combinedProfitMetric.value).className
  },
  {
    key: 'margin', label: '账面毛利率', value: formatFinancePercent(props.overview.margin_rate),
    tone: financeTone(props.overview.margin_rate), secondary: `成本覆盖率 ${formatFinancePercent(props.overview.quality.cost_coverage_rate)}`,
    status: props.overview.quality.status === 'complete' ? '数据完整' : '数据不完整',
    statusClass: props.overview.quality.status === 'complete'
      ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
      : 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  }
])

const secondaryMetrics = computed(() => [
  { key: 'loss', label: '所选期间账面亏损', value: formatFinanceMoney(props.overview.loss_amount), tone: 'text-red-600 dark:text-red-400', hint: `${props.overview.loss_request_count} 条成本已覆盖的亏损请求` },
  { key: 'settled-profit', label: '已结算利润', value: formatFinanceMoney(props.overview.settled_profit?.amount, props.overview.settled_profit?.currency), tone: financeTone(props.overview.settled_profit?.amount), hint: `真实成本结算覆盖率 ${formatFinancePercent(props.overview.settlement_coverage_rate)}` },
  { key: 'estimated', label: '待结算成本', value: formatFinanceMoney(props.overview.pending_settlement_cost ?? props.overview.estimated_cost_risk), tone: 'text-amber-600 dark:text-amber-400', hint: '已按合同或账号倍率入账，等待真实上游成本闭合' },
  { key: 'unconfirmed-exact', label: '待闭合真实成本', value: formatFinanceMoney(props.overview.unconfirmed_exact_cost), tone: 'text-amber-600 dark:text-amber-400', hint: '已取得真实成本，但同一请求仍有未确认成本段' },
  { key: 'unpriced', label: '未配置收入暴露', value: formatFinanceMoney(props.overview.unconfigured_revenue_exposure ?? props.overview.quality.unpriced_revenue), tone: 'text-amber-600 dark:text-amber-400', hint: '成本未配置，不进入账面利润和已结算利润' },
  { key: 'wallet', label: '上游钱包余额', value: formatFinanceMoney(props.overview.wallet_cash_total), tone: 'text-gray-900 dark:text-white', hint: `${props.overview.token_quota_wallet_count} 个 Token 配额钱包（不计入现金）` },
  { key: 'alerts', label: '待处理财务预警', value: String(props.overview.open_alert_count), tone: props.overview.open_alert_count > 0 ? 'text-red-600 dark:text-red-400' : 'text-emerald-600 dark:text-emerald-400', hint: '包含亏损、成本缺失与同步异常' }
])

const periodMetrics = computed(() => [
  { key: 'today', label: '今日账面毛利', value: formatFinanceMoney(props.overview.today_profit?.amount, props.overview.today_profit?.currency), tone: financeTone(props.overview.today_profit?.amount), hint: '按当前筛选时区计算' },
  { key: 'month', label: '本月账面毛利', value: formatFinanceMoney(props.overview.month_profit?.amount, props.overview.month_profit?.currency), tone: financeTone(props.overview.month_profit?.amount), hint: '本月截至当前时间' },
  { key: 'history', label: '历史累计账面毛利（含充值赠送）', value: formatFinanceMoney(historicalCombinedProfitMetric.value.amount, historicalCombinedProfitMetric.value.currency), tone: financeTone(historicalCombinedProfitMetric.value.amount), hint: '使用历史计费快照与充值赠送收益累计' },
  { key: 'history-loss', label: '历史累计账面亏损', value: formatFinanceMoney(props.overview.historical_loss_amount), tone: 'text-red-600 dark:text-red-400', hint: '包含真实结算、合同计价和倍率暂估，并在风险卡片中分栏' }
])
</script>
