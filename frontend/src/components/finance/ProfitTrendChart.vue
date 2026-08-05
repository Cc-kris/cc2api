<template>
  <div class="card p-5">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-2">
      <div>
        <h2 class="font-semibold text-gray-900 dark:text-white">利润趋势</h2>
      </div>
      <span class="text-xs text-gray-500">{{ items.length }} 个时间段</span>
    </div>
    <div v-if="error" class="flex h-64 items-center justify-center text-sm text-red-600">{{ error }}</div>
    <div v-else-if="loading" class="flex h-64 items-center justify-center text-sm text-gray-500">正在加载利润趋势...</div>
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
  labels: props.items.map(item => new Date(item.bucket_start).toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit', timeZone: 'Asia/Shanghai' })),
  datasets: [
    { label: '客户消费', data: props.items.map(item => financeNumber(item.revenue)), borderColor: '#2563eb', backgroundColor: '#2563eb', tension: 0.28, spanGaps: true },
    { label: '上游成本', data: props.items.map(item => financeNumber(item.upstream_cost)), borderColor: '#f59e0b', backgroundColor: '#f59e0b', tension: 0.28, spanGaps: true },
    { label: '利润', data: props.items.map(item => financeNumber(item.profit)), borderColor: '#059669', backgroundColor: '#059669', tension: 0.28, spanGaps: true }
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
