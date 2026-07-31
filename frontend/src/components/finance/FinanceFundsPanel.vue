<template>
  <div class="space-y-4">
    <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
      <article v-for="item in cashMetrics" :key="item.label" class="card p-5">
        <p class="text-sm text-gray-500">{{ item.label }}</p>
        <p class="mt-2 text-xl font-semibold tabular-nums" :class="item.tone">{{ item.value }}</p>
        <p class="mt-2 text-xs text-gray-500">{{ item.hint }}</p>
      </article>
    </div>

    <div v-if="funds.stale_wallet_count || funds.failed_sync_count" class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-200">
      余额数据存在风险：{{ funds.stale_wallet_count }} 个钱包数据过期，{{ funds.failed_sync_count }} 个钱包同步失败。
    </div>

    <section class="card overflow-hidden">
      <div class="border-b border-gray-200 p-4 dark:border-dark-700">
        <h2 class="font-semibold text-gray-900 dark:text-white">上游现金钱包</h2>
        <p class="mt-1 text-xs text-gray-500">现金余额可用于资金风险判断，不与 Token 配额相加。</p>
      </div>
      <div class="overflow-x-auto" role="region" aria-label="上游现金钱包明细" tabindex="0">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-3">钱包</th><th class="px-4 py-3">余额范围</th><th class="px-4 py-3 text-right">余额</th><th class="px-4 py-3 text-right">日均成本</th><th class="px-4 py-3 text-right">可用天数</th><th class="px-4 py-3">同步状态</th><th class="px-4 py-3">采集时间</th></tr></thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="item in cashWallets" :key="item.wallet_id">
              <td class="px-4 py-3 font-medium text-gray-900 dark:text-white">{{ item.wallet_name }}</td>
              <td class="px-4 py-3"><div class="flex flex-wrap gap-1"><span v-if="item.balance_scope_key" class="rounded bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ item.balance_scope_key }}</span><span v-if="item.included_in_total === false" class="rounded bg-blue-50 px-2 py-1 text-xs text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">共享余额，不重复汇总</span></div></td>
              <td class="px-4 py-3 text-right tabular-nums">{{ formatFinanceMoney(item.balance, item.currency) }}</td>
              <td class="px-4 py-3 text-right tabular-nums">{{ formatFinanceMoney(item.daily_cost, item.currency) }}</td>
              <td class="px-4 py-3 text-right tabular-nums">{{ item.available_days ?? '无法计算' }}</td>
              <td class="px-4 py-3"><span :class="syncClass(item.stale ? 'stale' : item.sync_status)" class="rounded-full px-2 py-1 text-xs">{{ syncLabel(item.stale ? 'stale' : item.sync_status) }}</span></td>
              <td class="px-4 py-3 text-gray-500">{{ formatFinanceDate(item.collected_at) }}</td>
            </tr>
            <tr v-if="cashWallets.length === 0"><td colspan="7" class="px-4 py-10 text-center text-gray-500">暂无现金钱包余额数据</td></tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="card overflow-hidden">
      <div class="border-b border-gray-200 p-4 dark:border-dark-700">
        <h2 class="font-semibold text-gray-900 dark:text-white">Token 配额</h2>
        <p class="mt-1 text-xs text-gray-500">配额仅反映可用调用额度，不作为现金资产。</p>
      </div>
      <div class="overflow-x-auto" role="region" aria-label="Token 配额明细" tabindex="0">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-3">钱包</th><th class="px-4 py-3 text-right">总配额</th><th class="px-4 py-3 text-right">已使用</th><th class="px-4 py-3 text-right">剩余额度</th><th class="px-4 py-3">同步状态</th><th class="px-4 py-3">采集时间</th></tr></thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="item in quotaWallets" :key="item.wallet_id"><td class="px-4 py-3 font-medium text-gray-900 dark:text-white">{{ item.wallet_name }}</td><td class="px-4 py-3 text-right tabular-nums">{{ formatQuota(item.total_quota, item.currency) }}</td><td class="px-4 py-3 text-right tabular-nums">{{ formatQuota(item.used_quota, item.currency) }}</td><td class="px-4 py-3 text-right tabular-nums">{{ formatQuota(item.remaining_quota, item.currency) }}</td><td class="px-4 py-3"><span :class="syncClass(item.sync_status)" class="rounded-full px-2 py-1 text-xs">{{ syncLabel(item.sync_status) }}</span></td><td class="px-4 py-3 text-gray-500">{{ formatFinanceDate(item.collected_at) }}</td></tr>
            <tr v-if="quotaWallets.length === 0"><td colspan="6" class="px-4 py-10 text-center text-gray-500">暂无 Token 配额数据</td></tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { FinanceFunds } from '@/api/admin/finance'
import { financeTone, formatFinanceDate, formatFinanceMoney } from './format'

const props = defineProps<{ funds: FinanceFunds }>()
const cashWallets = computed(() => props.funds.wallet_cash || [])
const quotaWallets = computed(() => props.funds.token_quota || [])
const cashMetrics = computed(() => [
  { label: '客户净现金流', value: formatFinanceMoney(props.funds.customer_cash.net_cash), tone: financeTone(props.funds.customer_cash.net_cash), hint: '充值 - 退款 - 支付手续费' },
  { label: '上游净现金流', value: formatFinanceMoney(props.funds.upstream_cash.net_cash), tone: financeTone(props.funds.upstream_cash.net_cash), hint: '充值、退款与资金调整' },
  { label: '客户充值', value: formatFinanceMoney(props.funds.customer_cash.payment), tone: 'text-gray-900 dark:text-white', hint: `退款 ${formatFinanceMoney(props.funds.customer_cash.refund)}` },
  { label: '上游充值', value: formatFinanceMoney(props.funds.upstream_cash.topup), tone: 'text-gray-900 dark:text-white', hint: `退款 ${formatFinanceMoney(props.funds.upstream_cash.refund)}` }
  , { label: '充值赠送收益', value: formatFinanceMoney(props.funds.upstream_cash.recharge_bonus_income), tone: financeTone(props.funds.upstream_cash.recharge_bonus_income), hint: '独立核算，不改变上游倍率或请求成本' }
])

function formatQuota(value: string | null, currency: string) {
  if (value === null || !Number.isFinite(Number(value))) return '无法计算'
  return `${Number(value).toLocaleString('zh-CN', { maximumFractionDigits: 4 })} ${currency || 'Token'}`
}
function syncLabel(status: string) { return status === 'success' ? '正常' : status === 'stale' ? '已过期' : status === 'failed' ? '失败' : status || '未知' }
function syncClass(status: string) { return status === 'success' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300' }
</script>
