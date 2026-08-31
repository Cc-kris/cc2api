<template>
  <div class="min-w-[150px] space-y-1" :title="title">
    <div class="flex items-center gap-2 text-xs">
      <span :class="labelClass">{{ formatDuration(firstTokenMs) }}</span>
      <span class="text-gray-400">/</span>
      <span :class="labelClass">{{ formatDuration(durationMs) }}</span>
    </div>
    <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700" aria-hidden="true">
      <div class="h-full rounded-full transition-[width]" :class="barClass" :style="{ width: `${health.width}%` }" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { formatDuration, latencyHealth } from '@/utils/duration'

const props = defineProps<{ firstTokenMs: number | null | undefined; durationMs: number | null | undefined; isError?: boolean }>()
const health = computed(() => latencyHealth(props.durationMs, props.isError))
const labelClass = computed(() => ({
  'text-gray-400 dark:text-gray-500': health.value.state === 'missing',
  'text-emerald-600 dark:text-emerald-400': health.value.state === 'good',
  'text-amber-600 dark:text-amber-400': health.value.state === 'slow',
  'text-red-600 dark:text-red-400': health.value.state === 'critical' || health.value.state === 'error',
}))
const barClass = computed(() => ({
  'bg-gray-300 dark:bg-gray-600': health.value.state === 'missing',
  'bg-emerald-500': health.value.state === 'good',
  'bg-amber-500': health.value.state === 'slow',
  'bg-red-500': health.value.state === 'critical' || health.value.state === 'error',
}))
const title = computed(() => `TTFT ${formatDuration(props.firstTokenMs)} · Total ${formatDuration(props.durationMs)}`)
</script>
