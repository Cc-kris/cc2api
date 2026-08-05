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
      有 {{ funds.stale_wallet_count + funds.failed_sync_count }} 条上游余额无法确认，已从当前余额中排除。
    </div>

    <section class="card overflow-hidden">
      <div class="border-b border-gray-200 p-4 dark:border-dark-700">
        <h2 class="font-semibold text-gray-900 dark:text-white">上游当前余额</h2>
        <p class="mt-1 text-xs text-gray-500">只显示最近同步成功的余额；过期或失败的数据不列出，也不计入上游当前余额。</p>
      </div>
      <div class="overflow-x-auto" role="region" aria-label="上游现金钱包明细" tabindex="0">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-3">上游账户</th><th class="px-4 py-3 text-right">当前余额</th><th class="px-4 py-3 text-right">日均成本</th><th class="px-4 py-3 text-right">预计可用天数</th><th class="px-4 py-3">更新时间</th></tr></thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="item in cashWallets" :key="item.wallet_id">
              <td class="px-4 py-3 font-medium text-gray-900 dark:text-white">{{ item.wallet_name }}</td>
              <td class="px-4 py-3 text-right tabular-nums">{{ formatFinanceMoney(item.balance, item.currency) }}</td>
              <td class="px-4 py-3 text-right tabular-nums">{{ formatFinanceMoney(item.daily_cost, item.currency) }}</td>
              <td class="px-4 py-3 text-right tabular-nums">{{ item.available_days ?? '无法计算' }}</td>
              <td class="px-4 py-3 text-gray-500">{{ formatFinanceDate(item.collected_at) }}</td>
            </tr>
            <tr v-if="cashWallets.length === 0"><td colspan="5" class="px-4 py-10 text-center text-gray-500">暂无可确认的上游余额</td></tr>
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
const cashWallets = computed(() => (props.funds.wallet_cash || []).filter(item => item.sync_status === 'success' && !item.stale && item.included_in_total !== false))
const cashMetrics = computed(() => [
  { label: '客户本期充值', value: formatFinanceMoney(props.funds.customer_cash.payment), tone: 'text-gray-900 dark:text-white', hint: `本期收到；退款 ${formatFinanceMoney(props.funds.customer_cash.refund)}` },
  { label: '客户当前余额', value: formatFinanceMoney(props.funds.customer_balance), tone: 'text-gray-900 dark:text-white', hint: '客户尚未使用的余额' },
  { label: '上游本期充值', value: props.funds.upstream_cash.topup_available ? formatFinanceMoney(props.funds.upstream_cash.topup) : '未录入', tone: props.funds.upstream_cash.topup_available ? 'text-gray-900 dark:text-white' : 'text-amber-700 dark:text-amber-300', hint: props.funds.upstream_cash.topup_available ? `${props.funds.upstream_cash.topup_event_count} 笔充值记录` : '本期没有上游充值记录' },
  { label: '上游本期净付出', value: props.funds.upstream_cash.net_cash_available ? formatFinanceMoney(props.funds.upstream_cash.net_cash) : '未录入', tone: props.funds.upstream_cash.net_cash_available ? financeTone(props.funds.upstream_cash.net_cash) : 'text-amber-700 dark:text-amber-300', hint: props.funds.upstream_cash.net_cash_available ? '充值、退款和调整合计' : '本期没有上游资金流水' }
])

</script>
