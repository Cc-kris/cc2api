<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { opsAPI, type AlertEvent } from '@/api/admin/ops'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const alertEvents = ref<AlertEvent[]>([])
const MAX_VISIBLE_PER_COLUMN = 3

const p0Events = computed(() =>
  alertEvents.value
    .filter(e => String(e.severity || '').trim().toUpperCase() === 'P0')
    .sort((a, b) => new Date(b.fired_at || b.created_at).getTime() - new Date(a.fired_at || a.created_at).getTime())
)

const p1Events = computed(() =>
  alertEvents.value
    .filter(e => String(e.severity || '').trim().toUpperCase() === 'P1')
    .sort((a, b) => new Date(b.fired_at || b.created_at).getTime() - new Date(a.fired_at || a.created_at).getTime())
)

const p2PlusEvents = computed(() =>
  alertEvents.value
    .filter(e => {
      const sev = String(e.severity || '').trim().toUpperCase()
      return sev === 'P2' || sev === 'P3'
    })
    .sort((a, b) => new Date(b.fired_at || b.created_at).getTime() - new Date(a.fired_at || a.created_at).getTime())
)

function getSeverityColor(severity: string): string {
  const sev = String(severity || '').trim().toUpperCase()
  if (sev === 'P0') return '#dc2626' // red-600
  if (sev === 'P1') return '#f97316' // orange-500
  return '#3b82f6' // blue-500
}

function getAISummaryText(event: AlertEvent): { text: string, status: 'loading' | 'error' | 'ready' | 'none' } {
  if (!event.latest_ai_status) {
    return { text: t('admin.ops.alertEvents.noAISummary') || '暂无 AI 结论', status: 'none' }
  }

  const aiStatus = String(event.latest_ai_status || '').trim().toLowerCase()
  if (aiStatus === 'running') {
    return { text: t('admin.ops.alertEvents.aiAnalyzing') || 'AI 分析中…', status: 'loading' }
  }
  if (aiStatus === 'failed') {
    return { text: t('admin.ops.alertEvents.aiAnalysisFailed') || 'AI 分析失败', status: 'error' }
  }

  const summary = String(event.latest_ai_summary || '').trim()
  if (summary) {
    const truncated = summary.length > 80 ? summary.substring(0, 80) + '…' : summary
    return { text: truncated, status: 'ready' }
  }

  return { text: t('admin.ops.alertEvents.noAISummary') || '暂无 AI 结论', status: 'none' }
}

function getDurationText(event: AlertEvent): string {
  const firedAt = new Date(event.fired_at || event.created_at)
  if (Number.isNaN(firedAt.getTime())) return '-'

  const resolvedAtStr = event.resolved_at || null

  if (resolvedAtStr) {
    const resolvedAt = new Date(resolvedAtStr)
    if (!Number.isNaN(resolvedAt.getTime())) {
      const ms = resolvedAt.getTime() - firedAt.getTime()
      return formatDurationMs(ms)
    }
  }

  const now = Date.now()
  const ms = now - firedAt.getTime()
  return formatDurationMs(ms)
}

function formatDurationMs(ms: number): string {
  const safe = Math.max(0, Math.floor(ms))
  const sec = Math.floor(safe / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr}h`
  const day = Math.floor(hr / 24)
  return `${day}d`
}

function getAISummaryClass(summaryStatus: 'loading' | 'error' | 'ready' | 'none'): string {
  switch (summaryStatus) {
    case 'ready':
      return 'text-gray-700 dark:text-gray-200'
    case 'loading':
      return 'text-blue-600 dark:text-blue-400'
    case 'error':
      return 'text-gray-500 dark:text-gray-400'
    case 'none':
      return 'text-gray-500 dark:text-gray-400'
    default:
      return 'text-gray-600 dark:text-gray-300'
  }
}

async function loadAlertEvents() {
  loading.value = true
  try {
    const data = await opsAPI.listAlertEvents({
      status: 'firing',
      limit: 50
    })
    alertEvents.value = data
  } catch (err: any) {
    console.error('[OpsAlertGroupsByPriority] Failed to load alert events', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertEvents.loadFailed'))
    alertEvents.value = []
  } finally {
    loading.value = false
  }
}

function renderColumn(events: AlertEvent[]) {
  const visible = events.slice(0, MAX_VISIBLE_PER_COLUMN)
  const hidden = Math.max(0, events.length - MAX_VISIBLE_PER_COLUMN)
  return { visible, hidden }
}

onMounted(() => {
  void loadAlertEvents()
})
</script>

<template>
  <div class="ov-alert-groups-wrapper">
    <div v-if="loading" class="flex items-center justify-center py-8 text-sm text-gray-500 dark:text-gray-400">
      <svg class="mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
      {{ t('admin.ops.alertEvents.loading') }}
    </div>

    <div v-else class="grid grid-cols-1 gap-3 md:grid-cols-3">
      <!-- P0 Column -->
      <div class="ov-alert-column">
        <div class="ov-alert-column-header">
          <div class="ov-alert-column-badge" :style="{ backgroundColor: getSeverityColor('P0') }">P0</div>
          <span class="text-xs font-bold text-gray-600 dark:text-gray-300">{{ p0Events.length }} 条告警</span>
        </div>
        <div v-if="p0Events.length === 0" class="ov-alert-column-empty">
          暂无 P0 触发中告警
        </div>
        <div v-else class="ov-alert-column-body">
          <div
            v-for="event in renderColumn(p0Events).visible"
            :key="event.id"
            class="ov-alert-card ov-alert-card--p0"
          >
            <div class="ov-alert-card-title">{{ event.title || '-' }}</div>
            <div class="ov-alert-card-reason">{{ event.description || '-' }}</div>
            <div class="ov-alert-card-duration">持续 {{ getDurationText(event) }}</div>
            <div :class="['ov-alert-card-ai', getAISummaryClass(getAISummaryText(event).status)]">
              {{ getAISummaryText(event).text }}
            </div>
            <div class="ov-alert-card-actions">
              <button type="button" class="ov-alert-card-btn">查看事件</button>
            </div>
          </div>
          <div v-if="renderColumn(p0Events).hidden > 0" class="ov-alert-card-more">
            还有 {{ renderColumn(p0Events).hidden }} 条 P0
          </div>
        </div>
      </div>

      <!-- P1 Column -->
      <div class="ov-alert-column">
        <div class="ov-alert-column-header">
          <div class="ov-alert-column-badge" :style="{ backgroundColor: getSeverityColor('P1') }">P1</div>
          <span class="text-xs font-bold text-gray-600 dark:text-gray-300">{{ p1Events.length }} 条告警</span>
        </div>
        <div v-if="p1Events.length === 0" class="ov-alert-column-empty">
          暂无 P1 触发中告警
        </div>
        <div v-else class="ov-alert-column-body">
          <div
            v-for="event in renderColumn(p1Events).visible"
            :key="event.id"
            class="ov-alert-card ov-alert-card--p1"
          >
            <div class="ov-alert-card-title">{{ event.title || '-' }}</div>
            <div class="ov-alert-card-reason">{{ event.description || '-' }}</div>
            <div class="ov-alert-card-duration">持续 {{ getDurationText(event) }}</div>
            <div :class="['ov-alert-card-ai', getAISummaryClass(getAISummaryText(event).status)]">
              {{ getAISummaryText(event).text }}
            </div>
            <div class="ov-alert-card-actions">
              <button type="button" class="ov-alert-card-btn">查看事件</button>
            </div>
          </div>
          <div v-if="renderColumn(p1Events).hidden > 0" class="ov-alert-card-more">
            还有 {{ renderColumn(p1Events).hidden }} 条 P1
          </div>
        </div>
      </div>

      <!-- P2+ Column -->
      <div class="ov-alert-column">
        <div class="ov-alert-column-header">
          <div class="ov-alert-column-badge" :style="{ backgroundColor: getSeverityColor('P2') }">P2+观察</div>
          <span class="text-xs font-bold text-gray-600 dark:text-gray-300">{{ p2PlusEvents.length }} 条告警</span>
        </div>
        <div v-if="p2PlusEvents.length === 0" class="ov-alert-column-empty">
          暂无 P2 及以下触发中告警
        </div>
        <div v-else class="ov-alert-column-body">
          <div
            v-for="event in renderColumn(p2PlusEvents).visible"
            :key="event.id"
            class="ov-alert-card ov-alert-card--p2"
          >
            <div class="ov-alert-card-title">{{ event.title || '-' }}</div>
            <div class="ov-alert-card-reason">{{ event.description || '-' }}</div>
            <div class="ov-alert-card-duration">持续 {{ getDurationText(event) }}</div>
            <div :class="['ov-alert-card-ai', getAISummaryClass(getAISummaryText(event).status)]">
              {{ getAISummaryText(event).text }}
            </div>
            <div class="ov-alert-card-actions">
              <button type="button" class="ov-alert-card-btn">查看事件</button>
            </div>
          </div>
          <div v-if="renderColumn(p2PlusEvents).hidden > 0" class="ov-alert-card-more">
            还有 {{ renderColumn(p2PlusEvents).hidden }} 条 P2+
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ov-alert-groups-wrapper {
  @apply w-full;
}

.ov-alert-column {
  @apply rounded-2xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900;
}

.ov-alert-column-header {
  @apply mb-3 flex items-center gap-2;
}

.ov-alert-column-badge {
  @apply inline-flex h-6 w-6 items-center justify-center rounded text-xs font-bold text-white;
}

.ov-alert-column-empty {
  @apply rounded-lg bg-gray-50 p-3 text-center text-xs text-gray-500 dark:bg-dark-800/70 dark:text-gray-400;
}

.ov-alert-column-body {
  @apply flex flex-col gap-2;
}

.ov-alert-card {
  @apply relative rounded-xl border p-3;
}

.ov-alert-card--p0 {
  @apply border-l-4 border-l-red-600 border-red-200 bg-red-50 dark:border-red-800 dark:bg-red-900/20;
}

.ov-alert-card--p1 {
  @apply border-l-4 border-l-orange-500 border-orange-200 bg-orange-50 dark:border-orange-800 dark:bg-orange-900/20;
}

.ov-alert-card--p2 {
  @apply border-l-4 border-l-blue-500 border-blue-200 bg-blue-50 dark:border-blue-800 dark:bg-blue-900/20;
}

.ov-alert-card-title {
  @apply truncate text-xs font-bold text-gray-900 dark:text-gray-100;
}

.ov-alert-card-reason {
  @apply mt-1 line-clamp-2 text-xs text-gray-600 dark:text-gray-300;
}

.ov-alert-card-duration {
  @apply mt-1.5 text-xs font-medium text-gray-600 dark:text-gray-400;
}

.ov-alert-card-ai {
  @apply mt-2 line-clamp-2 text-xs leading-relaxed;
}

.ov-alert-card-actions {
  @apply mt-2 flex gap-2;
}

.ov-alert-card-btn {
  @apply flex-1 rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-xs font-medium text-gray-700 transition hover:border-blue-400 hover:text-blue-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300;
}

.ov-alert-card-more {
  @apply rounded-lg border border-dashed border-gray-300 bg-gray-50 px-2 py-2 text-center text-xs text-gray-500 dark:border-dark-600 dark:bg-dark-800/50 dark:text-gray-400;
}
</style>
