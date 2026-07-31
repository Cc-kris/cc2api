<template>
  <section class="card overflow-hidden">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 p-4 dark:border-dark-700"><div><h2 class="font-semibold text-gray-900 dark:text-white">上游账单对账</h2><p class="mt-1 text-xs text-gray-500">导入上游账单后，系统按钱包和账期与已确认采购成本比较。</p></div><button class="btn btn-secondary" @click="showImport = !showImport">{{ showImport ? '取消导入' : '导入账单' }}</button></div>
    <form v-if="showImport" class="grid grid-cols-1 gap-3 border-b border-gray-200 bg-gray-50 p-4 md:grid-cols-3 dark:border-dark-700 dark:bg-dark-800/50" @submit.prevent="importBill">
      <label class="text-sm">钱包 ID <input v-model.number="walletId" type="number" min="1" class="input mt-1 w-full" required /></label>
      <label class="text-sm">账期开始 <input v-model="periodStart" type="datetime-local" class="input mt-1 w-full" required /></label>
      <label class="text-sm">账期结束 <input v-model="periodEnd" type="datetime-local" class="input mt-1 w-full" required /></label>
      <label class="text-sm">币种 <input v-model="currency" maxlength="3" class="input mt-1 w-full uppercase" required /></label>
      <label class="text-sm">上游账单编号 <input v-model="sourceReference" class="input mt-1 w-full" /></label>
      <label class="text-sm">CSV 文件 <input type="file" accept=".csv,text/csv" class="mt-1 block w-full text-sm" required @change="setFile" /></label>
      <div class="md:col-span-3 flex justify-end"><button class="btn btn-primary" :disabled="working">{{ working ? '导入中...' : '校验并导入' }}</button></div>
    </form>
    <p v-if="error" class="p-4 text-sm text-red-700 dark:text-red-300">{{ error }}</p>
    <div class="overflow-x-auto" role="region" aria-label="上游账单对账结果" tabindex="0">
      <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700"><thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-3">钱包/账期</th><th class="px-4 py-3 text-right">上游账单</th><th class="px-4 py-3 text-right">系统成本</th><th class="px-4 py-3 text-right">差额</th><th class="px-4 py-3">状态</th><th class="px-4 py-3">处理</th></tr></thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700"><tr v-for="item in items" :key="item.id"><td class="px-4 py-3"><div class="font-medium">{{ item.wallet_name }}</div><div class="text-xs text-gray-500">{{ formatFinanceDate(item.period_start) }} 至 {{ formatFinanceDate(item.period_end) }}</div></td><td class="px-4 py-3 text-right tabular-nums">{{ formatFinanceMoney(item.upstream_bill_amount, item.currency) }}</td><td class="px-4 py-3 text-right tabular-nums">{{ formatFinanceMoney(item.system_cost_amount, item.currency) }}</td><td class="px-4 py-3 text-right font-medium tabular-nums" :class="differenceTone(item.difference_amount)">{{ formatFinanceMoney(item.difference_amount, item.currency) }}<div class="text-xs">{{ item.difference_rate == null ? '无法计算' : formatFinancePercent(item.difference_rate) }}</div></td><td class="px-4 py-3">{{ statusLabel(item.status) }}</td><td class="px-4 py-3"><div class="flex min-w-72 gap-2"><input v-model="notes[item.id]" class="input min-w-40" placeholder="处理说明" /><select v-model="statuses[item.id]" class="input"><option value="confirmed">确认</option><option value="ignored">忽略</option><option value="pending">待处理</option></select><button class="btn btn-secondary" :disabled="working || !notes[item.id]?.trim()" @click="update(item)">保存</button></div></td></tr><tr v-if="!loading && items.length === 0"><td colspan="6" class="p-8 text-center text-gray-500">暂无上游账单对账记录</td></tr></tbody></table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type { FinanceReconciliation } from '@/api/admin/finance'
import { extractApiErrorMessage } from '@/utils/apiError'
import { financeTone, formatFinanceDate, formatFinanceMoney, formatFinancePercent } from './format'

const items = ref<FinanceReconciliation[]>([])
const loading = ref(false)
const working = ref(false)
const error = ref('')
const showImport = ref(false)
const walletId = ref<number | null>(null)
const periodStart = ref('')
const periodEnd = ref('')
const currency = ref('USD')
const sourceReference = ref('')
const file = ref<File | null>(null)
const statuses = reactive<Record<number, 'confirmed' | 'ignored' | 'pending'>>({})
const notes = reactive<Record<number, string>>({})
function statusLabel(value: string) { return ({ matched: '已匹配', difference: '存在差额', confirmed: '已确认', ignored: '已忽略', pending: '待处理' } as Record<string, string>)[value] || value }
function differenceTone(value: string) { const parsed = Number(value); return financeTone(Number.isFinite(parsed) && parsed !== 0 ? -Math.abs(parsed) : parsed) }
function setFile(event: Event) { file.value = (event.target as HTMLInputElement).files?.[0] || null }
async function load() { loading.value = true; error.value = ''; try { const result = await adminAPI.finance.getReconciliations({ page: 1, page_size: 100 }); items.value = result.items || []; for (const item of items.value) statuses[item.id] = ['confirmed', 'ignored', 'pending'].includes(item.status) ? item.status as 'confirmed' | 'ignored' | 'pending' : 'pending' } catch (caught) { error.value = extractApiErrorMessage(caught, '对账记录加载失败') } finally { loading.value = false } }
async function importBill() { if (!file.value || !walletId.value) return; working.value = true; error.value = ''; try { const form = new FormData(); form.set('wallet_id', String(walletId.value)); form.set('period_start', new Date(periodStart.value).toISOString()); form.set('period_end', new Date(periodEnd.value).toISOString()); form.set('currency', currency.value.trim().toUpperCase()); form.set('source_reference', sourceReference.value.trim()); form.set('file', file.value); await adminAPI.finance.importReconciliation(form); showImport.value = false; await load() } catch (caught) { error.value = extractApiErrorMessage(caught, '账单导入失败') } finally { working.value = false } }
async function update(item: FinanceReconciliation) { working.value = true; error.value = ''; try { await adminAPI.finance.updateReconciliation(item.id, { status: statuses[item.id], note: notes[item.id].trim() }); notes[item.id] = ''; await load() } catch (caught) { error.value = extractApiErrorMessage(caught, '对账状态保存失败') } finally { working.value = false } }
onMounted(load)
</script>
