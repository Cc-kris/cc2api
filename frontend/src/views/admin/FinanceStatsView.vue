<template>
  <AppLayout>
    <div class="space-y-6">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">财务统计</h1>
        <p class="mt-1 text-sm text-gray-500">统计用户充值、用户消耗、上游成本与已消耗利润。</p>
      </div>
      <div class="card p-4">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_180px_auto]">
          <label class="block text-sm text-gray-600 dark:text-gray-300">开始日期<input v-model="startDate" type="date" class="input mt-1 w-full" /></label>
          <label class="block text-sm text-gray-600 dark:text-gray-300">结束日期<input v-model="endDate" type="date" class="input mt-1 w-full" /></label>
          <label class="block text-sm text-gray-600 dark:text-gray-300">粒度<select v-model="granularity" class="input mt-1 w-full"><option value="hour">小时</option><option value="day">天</option><option value="month">月</option></select></label>
          <div class="flex items-end"><button class="btn btn-secondary w-full md:w-auto" @click="load">刷新</button></div>
        </div>
      </div>
      <div class="grid grid-cols-1 gap-4 md:grid-cols-3 xl:grid-cols-6"><Stat title="用户充值总金额" :value="money(summary.user_recharge_total)" /><Stat title="上游充值总金额" :value="money(summary.upstream_recharge_total)" /><Stat title="用户消耗金额" :value="money(summary.user_consumed_amount)" /><Stat title="上游消耗金额" :value="money(summary.upstream_consumed_amount)" /><Stat title="已消耗利润" :value="money(summary.consumed_profit)" /><Stat title="利润率" :value="`${money(summary.consumed_profit_rate)}%`" /></div>
      <div class="card p-4"><h2 class="mb-4 font-semibold">上游成本 / 用户消耗金额 / 已消耗利润曲线图</h2><div v-if="lineData" class="relative h-80"><Line :data="lineData" :options="chartOptions" /></div><div v-else class="flex h-64 items-center justify-center text-sm text-gray-500">暂无数据</div></div>
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref } from 'vue'
import { Line } from 'vue-chartjs'
import { CategoryScale, Chart as ChartJS, Legend, LineElement, LinearScale, PointElement, Tooltip, type ChartOptions } from 'chart.js'
import AppLayout from '@/components/layout/AppLayout.vue'
import { adminAPI } from '@/api/admin'
import type { FinanceStatsResponse, FinanceStatsSummary } from '@/api/admin/upstreams'
ChartJS.register(CategoryScale, Legend, LineElement, LinearScale, PointElement, Tooltip)
const today = new Date().toISOString().slice(0, 10)
const startDate = ref(new Date(Date.now() - 29 * 86400000).toISOString().slice(0, 10))
const endDate = ref(today)
const granularity = ref<'hour' | 'day' | 'month'>('day')
const stats = ref<FinanceStatsResponse | null>(null)
const summary = computed<FinanceStatsSummary>(() => stats.value?.summary || { user_recharge_total: 0, upstream_recharge_total: 0, user_consumed_amount: 0, upstream_consumed_amount: 0, consumed_profit: 0, consumed_profit_rate: 0 })
const chartOptions: ChartOptions<'line'> = {
  responsive: true,
  maintainAspectRatio: false,
  animation: false,
  resizeDelay: 200,
  interaction: { mode: 'index', intersect: false },
  plugins: {
    tooltip: {
      mode: 'index',
      intersect: false,
      callbacks: {
        label: context => `${context.dataset.label || ''}: ${money(Number(context.parsed.y || 0))}`
      }
    }
  },
  scales: { y: { beginAtZero: true } }
}
const lineData = computed(() => {
  const trend = Array.isArray(stats.value?.trend) ? stats.value.trend : []
  if (!trend.length) return null
  return {
    labels: trend.map(p => formatBucket(p.bucket)),
    datasets: [
      { label: '上游成本', borderColor: '#dc2626', backgroundColor: 'rgba(220,38,38,.12)', data: trend.map(p => safeNumber(p.upstream_cost)), tension: .3 },
      { label: '用户消耗金额', borderColor: '#2563eb', backgroundColor: 'rgba(37,99,235,.12)', data: trend.map(p => safeNumber(p.user_consumed_amount)), tension: .3 },
      { label: '已消耗利润', borderColor: '#16a34a', backgroundColor: 'rgba(22,163,74,.12)', data: trend.map(p => safeNumber(p.profit)), tension: .3 },
    ]
  }
})
async function load() { stats.value = await adminAPI.upstreams.getFinanceStats({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value }) }
function safeNumber(v: unknown) {
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}
function money(v: number) { return safeNumber(v).toFixed(4) }
function formatBucket(v: string) {
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return String(v || '')
  if (granularity.value === 'month') return d.toLocaleDateString([], { year: 'numeric', month: '2-digit' })
  return d.toLocaleDateString()
}
const Stat = defineComponent({ props: { title: String, value: [String, Number] }, setup: p => () => h('div', { class: 'card p-4' }, [h('div', { class: 'text-sm text-gray-500' }, p.title), h('div', { class: 'mt-2 text-xl font-semibold text-gray-900 dark:text-white' }, String(p.value ?? ''))]) })
onMounted(load)
</script>
