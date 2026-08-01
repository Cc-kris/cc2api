<template>
  <section class="card p-5" data-testid="finance-initialization-panel">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="font-semibold text-gray-900 dark:text-white">财务初始化</h2>
        <p class="mt-1 text-sm text-gray-500">系统自动识别上游、账号归属和财务详情；只需确认账号上游倍率与上游当前余额。</p>
      </div>
      <button class="btn btn-secondary" :disabled="loading || applying" @click="() => scan()">{{ loading ? '扫描中...' : '扫描现有账号和上游' }}</button>
    </div>

    <p v-if="error" class="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</p>
    <p v-if="result" class="mt-4 rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-200">初始化完成：{{ result.initialized_accounts }} 个账号、{{ result.initialized_upstreams }} 个上游，新增 {{ result.created_wallets }} 个财务钱包。</p>

    <template v-if="scanData">
      <div class="mt-5 rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-100">
        倍率按账号保存，和上游充值赠送比例无关。初始化后，新的使用记录会冻结当时的倍率；历史缺少证据的记录仍会保留为待确认，不会被当前倍率覆盖。
      </div>

      <section class="mt-5 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
        <header class="border-b border-gray-200 p-4 dark:border-dark-700"><h3 class="font-medium text-gray-900 dark:text-white">账号上游倍率</h3><p class="mt-1 text-xs text-gray-500">空白项必须填写；已有值可直接确认或改正。</p></header>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-3">账号</th><th class="px-4 py-3">上游</th><th class="px-4 py-3">平台</th><th class="px-4 py-3">上游倍率</th><th class="px-4 py-3">财务状态</th></tr></thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="account in accountDrafts" :key="account.account_id"><td class="px-4 py-3 font-medium text-gray-900 dark:text-white">{{ account.account_name }}</td><td class="px-4 py-3">{{ account.upstream_name || '未识别上游' }}</td><td class="px-4 py-3">{{ account.platform }}</td><td class="px-4 py-3"><input v-model.trim="account.upstream_cost_multiplier" class="input w-36" type="number" min="0" max="9999.9999" step="0.0001" inputmode="decimal" :aria-label="`${account.account_name} 上游倍率`" /></td><td class="px-4 py-3">{{ account.finance_profile_ready ? '已配置，将保留现有计费模式' : '将自动创建' }}</td></tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="mt-5 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
        <header class="border-b border-gray-200 p-4 dark:border-dark-700"><h3 class="font-medium text-gray-900 dark:text-white">上游当前余额</h3><p class="mt-1 text-xs text-gray-500">每个上游只填写一次。系统会自动建立财务余额记录并作为期初余额。</p></header>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-3">上游</th><th class="px-4 py-3">关联账号</th><th class="px-4 py-3">当前余额（按上游币种）</th><th class="px-4 py-3">财务余额记录</th></tr></thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="upstream in upstreamDrafts" :key="upstream.upstream_id"><td class="px-4 py-3"><div class="font-medium text-gray-900 dark:text-white">{{ upstream.upstream_name }}</div><div class="text-xs text-gray-500">{{ upstream.base_url }}</div></td><td class="px-4 py-3">{{ upstream.account_count }}</td><td class="px-4 py-3"><div class="mb-1 text-xs text-gray-500">{{ upstream.currency || 'USD' }}</div><input v-model.number="upstream.current_balance" class="input w-40" type="number" min="0" step="0.0001" :aria-label="`${upstream.upstream_name} 当前余额`" /></td><td class="px-4 py-3">{{ upstream.finance_wallet_set ? '已存在，将更新期初余额' : '将自动创建' }}</td></tr>
            </tbody>
          </table>
        </div>
      </section>

      <label class="mt-5 block text-sm text-gray-700 dark:text-gray-200">初始化说明<input v-model.trim="reason" class="input mt-1 w-full" maxlength="500" /></label>
      <div class="mt-5 flex justify-end"><button class="btn btn-primary" :disabled="applying || !canApply" @click="apply">{{ applying ? '正在初始化...' : '确认并初始化财务' }}</button></div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type { FinanceInitializationResult, FinanceInitializationScan } from '@/api/admin/finance'
import { extractApiErrorMessage } from '@/utils/apiError'

const emit = defineEmits<{ initialized: [] }>()
const scanData = ref<FinanceInitializationScan | null>(null)
const accountDrafts = ref<Array<FinanceInitializationScan['accounts'][number] & { upstream_cost_multiplier: string }>>([])
const upstreamDrafts = ref<Array<FinanceInitializationScan['upstreams'][number] & { current_balance: number }>>([])
const reason = ref('首次财务初始化')
const loading = ref(false)
const applying = ref(false)
const error = ref('')
const result = ref<FinanceInitializationResult | null>(null)

const canApply = computed(() => reason.value.length >= 5 && accountDrafts.value.every(item => String(item.upstream_cost_multiplier).trim() !== '' && Number.isFinite(Number(item.upstream_cost_multiplier)) && Number(item.upstream_cost_multiplier) >= 0) && upstreamDrafts.value.every(item => Number.isFinite(Number(item.current_balance)) && Number(item.current_balance) >= 0))

async function scan(clearResult = true) {
  loading.value = true
  error.value = ''
  if (clearResult) result.value = null
  try {
    // 账号同步是写操作，先通过 POST 同步，再调用只读扫描接口。
    await adminAPI.upstreams.syncFromAccounts()
    scanData.value = await adminAPI.finance.scanInitialization()
    accountDrafts.value = (scanData.value.accounts || []).map(item => ({ ...item, upstream_cost_multiplier: item.current_multiplier || '' }))
    upstreamDrafts.value = (scanData.value.upstreams || []).map(item => ({ ...item, current_balance: Number(item.current_balance || 0) }))
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, '财务初始化扫描失败')
  } finally {
    loading.value = false
  }
}

async function apply() {
  if (!canApply.value) return
  applying.value = true
  error.value = ''
  try {
    result.value = await adminAPI.finance.applyInitialization({
      accounts: accountDrafts.value.map(item => ({ account_id: item.account_id, upstream_cost_multiplier: String(item.upstream_cost_multiplier) })),
      upstreams: upstreamDrafts.value.map(item => ({ upstream_id: item.upstream_id, current_balance: Number(item.current_balance) })),
      reason: reason.value,
    })
    emit('initialized')
    await scan(false)
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, '财务初始化失败')
  } finally {
    applying.value = false
  }
}
</script>
