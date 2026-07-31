<template>
  <section class="card p-5">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div><h2 class="font-semibold text-gray-900 dark:text-white">历史财务回算</h2><p class="mt-1 text-xs text-gray-500">只使用请求发生时的价格版本、倍率快照和上游尝试；预览不会写入账本。</p></div>
      <button class="btn btn-secondary" :disabled="working || !reason.trim()" @click="preview">{{ working ? '处理中...' : '预览回算' }}</button>
    </div>
    <label class="mt-4 block text-sm">回算原因 <span class="text-red-500">*</span><input v-model="reason" class="input mt-1 w-full" placeholder="例如：补齐历史缺失价格后的首次回算" /></label>
    <p v-if="error" class="mt-3 text-sm text-red-700 dark:text-red-300">{{ error }}</p>
    <div v-if="previewResult" class="mt-4 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
      <div class="grid grid-cols-2 gap-3 text-sm md:grid-cols-4">
        <div><span class="text-gray-500">预计记录</span><strong class="ml-2">{{ previewResult.estimated_records }}</strong></div>
        <div><span class="text-gray-500">可精确修复</span><strong class="ml-2 text-emerald-700 dark:text-emerald-300">{{ previewResult.exact_repairable }}</strong></div>
        <div><span class="text-gray-500">只能估算</span><strong class="ml-2">{{ previewResult.estimated_only }}</strong></div>
        <div><span class="text-gray-500">无法修复</span><strong class="ml-2 text-red-700 dark:text-red-300">{{ previewResult.unrepairable }}</strong></div>
      </div>
      <div v-if="previewResult.blockers?.length" class="mt-3 rounded bg-red-50 p-3 text-sm text-red-800 dark:bg-red-950/30 dark:text-red-200"><p class="font-medium">存在阻断项，不能启动回算</p><ul class="mt-1 list-disc pl-5"><li v-for="item in previewResult.blockers" :key="item">{{ item }}</li></ul></div>
      <div class="mt-4 flex justify-end"><button class="btn btn-primary" :disabled="working || Boolean(previewResult.blockers?.length)" @click="run">启动回算</button></div>
    </div>
    <div v-if="job" class="mt-4 rounded-lg border border-blue-200 bg-blue-50 p-4 text-sm dark:border-blue-900 dark:bg-blue-950/30">
      <div class="flex flex-wrap items-center justify-between gap-2"><div><strong>任务 #{{ job.job_id }}</strong><span class="ml-2">{{ job.status }}</span><span class="ml-2">进度 {{ formatProgress(job.progress) }}</span></div><div class="flex gap-2"><button v-if="job.status === 'running' || job.status === 'queued'" class="btn btn-secondary" @click="pause">暂停</button><button v-if="job.status === 'paused'" class="btn btn-secondary" @click="resume">恢复</button><button v-if="!terminal" class="btn btn-secondary" @click="refreshJob">刷新状态</button></div></div>
      <p class="mt-2 text-xs text-gray-600 dark:text-gray-300">已处理 {{ job.processed_count }}，成功 {{ job.success_count }}，失败 {{ job.failed_count }}</p>
      <p v-if="job.error_summary" class="mt-2 text-xs text-red-700 dark:text-red-300">{{ job.error_summary }}</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type { FinanceBackfillJob, FinanceBackfillPreview, FinanceBackfillRequest } from '@/api/admin/finance'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{ startDate: string; endDate: string }>()
const reason = ref('')
const working = ref(false)
const error = ref('')
const previewResult = ref<FinanceBackfillPreview | null>(null)
const job = ref<FinanceBackfillJob | null>(null)
const terminal = computed(() => job.value ? ['completed', 'failed', 'cancelled'].includes(job.value.status) : false)
function payload(): FinanceBackfillRequest { return { start_date: props.startDate, end_date: props.endDate, scope: { cost_status: ['missing_profile', 'missing_price', 'missing_multiplier', 'missing_usage', 'estimated'], account_ids: [], wallet_ids: [] }, pricing_policy: 'historical_only', dry_run_sample_size: 1000, reason: reason.value.trim() } }
function formatProgress(value: string) { const parsed = Number(value); return Number.isFinite(parsed) ? `${(parsed * 100).toFixed(2)}%` : '无法计算' }
async function execute(action: () => Promise<void>) { working.value = true; error.value = ''; try { await action() } catch (caught) { error.value = extractApiErrorMessage(caught, '历史回算操作失败') } finally { working.value = false } }
async function preview() { await execute(async () => { previewResult.value = await adminAPI.finance.previewBackfill(payload()); job.value = null }) }
async function run() { if (!previewResult.value) return; await execute(async () => { job.value = await adminAPI.finance.runBackfill({ ...payload(), preview_token: previewResult.value!.preview_token }) }) }
async function refreshJob() { if (!job.value) return; await execute(async () => { job.value = await adminAPI.finance.getBackfill(job.value!.job_id) }) }
async function pause() { if (!job.value) return; await execute(async () => { job.value = await adminAPI.finance.pauseBackfill(job.value!.job_id) }) }
async function resume() { if (!job.value) return; await execute(async () => { job.value = await adminAPI.finance.resumeBackfill(job.value!.job_id) }) }
</script>
