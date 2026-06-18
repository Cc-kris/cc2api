<template>
  <AppLayout>
    <div class="space-y-6">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">上游消费统计</h1>
        <p class="mt-1 text-sm text-gray-500">按时间范围和粒度查看上游消耗与 Token 趋势。</p>
      </div>
      <div class="card p-4">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_180px_auto]">
          <label class="block text-sm text-gray-600 dark:text-gray-300">开始日期<input v-model="startDate" type="date" class="input mt-1 w-full" /></label>
          <label class="block text-sm text-gray-600 dark:text-gray-300">结束日期<input v-model="endDate" type="date" class="input mt-1 w-full" /></label>
          <label class="block text-sm text-gray-600 dark:text-gray-300">粒度<select v-model="granularity" class="input mt-1 w-full"><option value="hour">小时</option><option value="day">天</option><option value="month">月</option></select></label>
          <div class="flex items-end"><button class="btn btn-secondary w-full md:w-auto" @click="load">刷新</button></div>
        </div>
      </div>
      <div class="grid grid-cols-1 gap-4 md:grid-cols-4"><Stat title="上游数" :value="stats?.summary.upstream_count || 0" /><Stat title="上游总余额" :value="money(stats?.summary.total_current_balance || 0)" /><Stat title="已消耗余额" :value="money(stats?.summary.total_consumed_balance || 0)" /><Stat title="Token 总量" :value="number(stats?.summary.total_tokens || 0)" /></div>
      <div class="grid grid-cols-1 gap-6 xl:grid-cols-2"><div class="card p-4"><h2 class="mb-4 font-semibold">上游消耗柱形图</h2><div v-if="barData" class="relative h-72"><Bar :data="barData" :options="barOptions" /></div><Empty v-else /></div><div class="card p-4"><h2 class="mb-4 font-semibold">Token 消耗曲线图</h2><div v-if="lineData" class="relative h-72"><Line :data="lineData" :options="lineOptions" /></div><Empty v-else /></div></div>
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref } from 'vue'
import { Bar, Line } from 'vue-chartjs'
import { BarElement, CategoryScale, Chart as ChartJS, Legend, LineElement, LinearScale, PointElement, Tooltip, type ChartOptions } from 'chart.js'
import AppLayout from '@/components/layout/AppLayout.vue'
import { adminAPI } from '@/api/admin'
import type { UpstreamStatsResponse } from '@/api/admin/upstreams'
ChartJS.register(BarElement, CategoryScale, Legend, LineElement, LinearScale, PointElement, Tooltip)
const today = new Date().toISOString().slice(0, 10)
const startDate = ref(new Date(Date.now() - 29 * 86400000).toISOString().slice(0, 10))
const endDate = ref(today)
const granularity = ref<'hour' | 'day' | 'month'>('day')
const stats = ref<UpstreamStatsResponse | null>(null)
const barOptions: ChartOptions<'bar'> = { responsive: true, maintainAspectRatio: false, animation: false, resizeDelay: 200 }
const lineOptions: ChartOptions<'line'> = { responsive: true, maintainAspectRatio: false, animation: false, resizeDelay: 200 }
const barData = computed(() => stats.value?.cost_bars?.length ? { labels: stats.value.cost_bars.map(p => p.upstream_name || '未命名'), datasets: [{ label: '消耗余额', backgroundColor: '#2563eb', data: stats.value.cost_bars.map(p => p.consumed_balance) }] } : null)
const lineData = computed(() => stats.value?.token_trend?.length ? { labels: stats.value.token_trend.map(p => formatBucket(p.bucket)), datasets: [{ label: 'Token', borderColor: '#16a34a', backgroundColor: 'rgba(22,163,74,.15)', data: stats.value.token_trend.map(p => p.total_tokens), tension: .3 }] } : null)
async function load() { stats.value = await adminAPI.upstreams.getStats({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value }) }
function money(v: number) { return Number(v || 0).toFixed(4) }
function number(v: number) { return Number(v || 0).toLocaleString() }
function formatBucket(v: string) { return new Date(v).toLocaleString() }
const Stat = defineComponent({ props: { title: String, value: [String, Number] }, setup: p => () => h('div', { class: 'card p-4' }, [h('div', { class: 'text-sm text-gray-500' }, p.title), h('div', { class: 'mt-2 text-2xl font-semibold text-gray-900 dark:text-white' }, String(p.value ?? ''))]) })
const Empty = defineComponent({ setup: () => () => h('div', { class: 'flex h-64 items-center justify-center text-sm text-gray-500' }, '暂无数据') })
onMounted(load)
</script>
