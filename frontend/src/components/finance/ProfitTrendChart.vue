<template>
  <div class="card p-5">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-2">
      <div>
        <h2 class="font-semibold text-gray-900 dark:text-white">经营趋势</h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">使用毛利与综合利润使用同一日期范围；综合利润包含充值赠送收益。</p>
      </div>
      <span class="text-xs text-gray-500">{{ items.length }} 个时间桶</span>
    </div>
    <div v-if="error" class="flex h-64 items-center justify-center text-sm text-red-600">{{ error }}</div>
    <div v-else-if="loading" class="flex h-64 items-center justify-center text-sm text-gray-500">正在加载经营趋势...</div>
    <div v-else-if="items.length" class="relative h-80">
      <Line :data="chartData" :options="chartOptions" />
    </div>
    <div v-else class="flex h-64 items-center justify-center text-sm text-gray-500">所选期间暂无趋势数据</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Line } from 'vue-chartjs'
import {
  CategoryScale,
  Chart as ChartJS,
  Legend,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip,
  type ChartOptions
} from 'chart.js'

import type { FinanceTrendItem } from '@/api/admin/finance'
import { financeNumber } from './format'

ChartJS.register(CategoryScale, Legend, LineElement, LinearScale, PointElement, Tooltip)

const props = defineProps<{ items: FinanceTrendItem[]; loading?: boolean; error?: string }>()

const chartData = computed(() => ({
  labels: props.items.map(item => new Date(item.bucket_start).toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' })),
  datasets: [
    { label: '经营收入', data: props.items.map(item => financeNumber(item.revenue)), borderColor: '#2563eb', backgroundColor: '#2563eb', tension: 0.28, spanGaps: true },
    { label: '已确认上游成本', data: props.items.map(item => financeNumber(item.upstream_cost)), borderColor: '#f59e0b', backgroundColor: '#f59e0b', tension: 0.28, spanGaps: true },
    { label: '已确认毛利', data: props.items.map(item => financeNumber(item.profit)), borderColor: '#059669', backgroundColor: '#059669', tension: 0.28, spanGaps: true }
    , { label: '综合利润', data: props.items.map(item => financeNumber(item.combined_profit ?? item.profit)), borderColor: '#7c3aed', backgroundColor: '#7c3aed', tension: 0.28, spanGaps: true }
  ]
}))

const chartOptions: ChartOptions<'line'> = {
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index', intersect: false },
  plugins: {
    legend: { position: 'bottom' },
    tooltip: { mode: 'index', intersect: false }
  },
  scales: {
    y: { ticks: { callback: value => `$${Number(value).toFixed(2)}` } }
  }
}
</script>
