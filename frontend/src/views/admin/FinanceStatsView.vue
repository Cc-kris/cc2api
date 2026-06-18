<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3"><div><h1 class="text-2xl font-bold text-gray-900 dark:text-white">财务统计</h1><p class="mt-1 text-sm text-gray-500">统计用户充值、用户消耗、上游成本与已消耗利润。</p></div><div class="flex flex-wrap items-center gap-2"><input v-model="startDate" type="date" class="input" /><input v-model="endDate" type="date" class="input" /><select v-model="granularity" class="input"><option value="hour">小时</option><option value="day">天</option><option value="month">月</option></select><button class="btn btn-secondary" @click="load">刷新</button></div></div>
      <div class="grid grid-cols-1 gap-4 md:grid-cols-3 xl:grid-cols-6"><Stat title="用户充值总金额" :value="money(summary.user_recharge_total)" /><Stat title="上游充值总金额" :value="money(summary.upstream_recharge_total)" /><Stat title="用户消耗金额" :value="money(summary.user_consumed_amount)" /><Stat title="上游消耗金额" :value="money(summary.upstream_consumed_amount)" /><Stat title="已消耗利润" :value="money(summary.consumed_profit)" /><Stat title="利润率" :value="`${money(summary.consumed_profit_rate)}%`" /></div>
      <div class="card p-4"><h2 class="mb-4 font-semibold">利润 / 上游成本 / 用户充值曲线图</h2><Line v-if="lineData" :data="lineData" :options="chartOptions" /><div v-else class="flex h-64 items-center justify-center text-sm text-gray-500">暂无数据</div></div>
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref } from 'vue'
import { Line } from 'vue-chartjs'
import { CategoryScale, Chart as ChartJS, Legend, LineElement, LinearScale, PointElement, Tooltip } from 'chart.js'
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
const chartOptions = { responsive: true, maintainAspectRatio: false }
const lineData = computed(() => stats.value?.trend?.length ? { labels: stats.value.trend.map(p => new Date(p.bucket).toLocaleString()), datasets: [{ label: '已消耗利润', borderColor: '#16a34a', data: stats.value.trend.map(p => p.profit), tension: .3 }, { label: '上游成本', borderColor: '#dc2626', data: stats.value.trend.map(p => p.upstream_cost), tension: .3 }, { label: '用户充值', borderColor: '#2563eb', data: stats.value.trend.map(p => p.user_recharge), tension: .3 }] } : null)
async function load() { stats.value = await adminAPI.upstreams.getFinanceStats({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value }) }
function money(v: number) { return Number(v || 0).toFixed(4) }
const Stat = defineComponent({ props: { title: String, value: [String, Number] }, setup: p => () => h('div', { class: 'card p-4' }, [h('div', { class: 'text-sm text-gray-500' }, p.title), h('div', { class: 'mt-2 text-xl font-semibold text-gray-900 dark:text-white' }, String(p.value ?? ''))]) })
onMounted(load)
</script>
<style scoped>.card :deep(canvas){min-height:20rem}</style>
