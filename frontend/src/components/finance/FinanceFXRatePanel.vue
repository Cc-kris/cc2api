<template>
  <section class="space-y-4" data-testid="finance-fx-rate-panel">
    <div class="card p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="font-semibold text-gray-900 dark:text-white">汇率版本</h2>
          <p class="mt-1 text-xs text-gray-500">非美元采购价按生效时间选取这里的汇率，并把版本冻结到历史成本记录。</p>
        </div>
        <button type="button" class="btn btn-secondary" @click="showForm = !showForm">{{ showForm ? '取消录入' : '录入汇率' }}</button>
      </div>
      <form v-if="showForm" class="mt-4 grid grid-cols-1 gap-3 border-t border-gray-200 pt-4 md:grid-cols-3 dark:border-dark-700" @submit.prevent="submit">
        <label class="text-sm">币种<input v-model="form.currency" maxlength="3" class="input mt-1 w-full uppercase" required /></label>
        <label class="text-sm">兑美元汇率<input v-model="form.rate_to_usd" inputmode="decimal" class="input mt-1 w-full" placeholder="例如 0.138" required /></label>
        <label class="text-sm">来源<input v-model="form.source" maxlength="80" class="input mt-1 w-full" placeholder="manual_admin" /></label>
        <label class="text-sm">观测时间<input v-model="form.observed_at" type="datetime-local" class="input mt-1 w-full" required /></label>
        <label class="text-sm">生效时间<input v-model="form.effective_from" type="datetime-local" class="input mt-1 w-full" required /></label>
        <label class="text-sm md:col-span-2">变更原因<textarea v-model="form.change_reason" minlength="5" maxlength="500" class="input mt-1 w-full" rows="2" required placeholder="填写本次汇率变更的业务原因（5-500字）" /></label>
        <div class="flex items-end justify-end"><button class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存版本' }}</button></div>
      </form>
    </div>

    <p v-if="error" class="card p-4 text-sm text-red-700 dark:text-red-300">{{ error }}</p>
    <div class="card overflow-x-auto" role="region" aria-label="汇率版本列表" tabindex="0">
      <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
        <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-3">币种</th><th class="px-4 py-3 text-right">兑美元汇率</th><th class="px-4 py-3">来源</th><th class="px-4 py-3">变更原因</th><th class="px-4 py-3">观测时间</th><th class="px-4 py-3">生效时间</th></tr></thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="item in items" :key="item.id"><td class="px-4 py-3 font-medium">{{ item.currency }}</td><td class="px-4 py-3 text-right tabular-nums">{{ item.rate_to_usd }}</td><td class="px-4 py-3">{{ item.source }}</td><td class="max-w-xs px-4 py-3">{{ item.change_reason || '历史版本未记录' }}</td><td class="px-4 py-3">{{ formatFinanceDate(item.observed_at) }}</td><td class="px-4 py-3">{{ formatFinanceDate(item.effective_from) }}</td></tr>
          <tr v-if="!loading && items.length === 0"><td colspan="6" class="p-8 text-center text-gray-500">暂无汇率版本</td></tr>
        </tbody>
      </table>
      <p v-if="loading" class="p-6 text-center text-sm text-gray-500">正在加载汇率版本...</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type { FinanceFXRateVersion } from '@/api/admin/finance'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatFinanceDate } from './format'

const items = ref<FinanceFXRateVersion[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const showForm = ref(false)
const nowLocal = () => {
  const date = new Date(Date.now() - new Date().getTimezoneOffset() * 60_000)
  return date.toISOString().slice(0, 16)
}
const form = reactive({ currency: 'CNY', rate_to_usd: '', source: 'manual_admin', observed_at: nowLocal(), effective_from: nowLocal(), change_reason: '' })

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await adminAPI.finance.getFXRates({ page: 1, page_size: 100 })
    items.value = result.items || []
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, '汇率版本加载失败')
  } finally {
    loading.value = false
  }
}

async function submit() {
  saving.value = true
  error.value = ''
  try {
    await adminAPI.finance.createFXRate({
      currency: form.currency.trim().toUpperCase(),
      rate_to_usd: form.rate_to_usd.trim(),
      source: form.source.trim() || 'manual_admin',
      observed_at: new Date(form.observed_at).toISOString(),
      effective_from: new Date(form.effective_from).toISOString(),
      change_reason: form.change_reason.trim(),
    })
    form.rate_to_usd = ''
    form.change_reason = ''
    showForm.value = false
    await load()
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, '汇率版本保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
