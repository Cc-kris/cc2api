<template>
  <section class="rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="account-finance-profile">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="font-medium text-gray-900 dark:text-gray-100">上游成本配置</h3>
        <p class="mt-1 text-xs text-gray-500">配置按版本生效；已产生的请求继续使用当时冻结的倍率与成本证据。</p>
      </div>
      <span :class="readinessClass" class="rounded-full px-2.5 py-1 text-xs font-medium">{{ readinessLabel }}</span>
    </div>

    <div v-if="readiness?.issues?.length" class="mt-3 rounded-md bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950/30 dark:text-amber-100">
      <p v-for="issue in readiness.issues" :key="issue">{{ issue }}</p>
    </div>

    <div class="mt-4 grid gap-4 md:grid-cols-2">
      <label class="input-label">成本模式
        <select v-model="form.cost_mode" class="input mt-1 w-full">
          <option value="contract_multiplier">账号/合同倍率</option>
          <option value="manual">采购价格目录</option>
          <option value="request_charge">上游响应实际扣费</option>
          <option value="cumulative_list_and_actual">累计原价与实扣</option>
          <option value="cumulative_actual">仅累计实际扣费</option>
        </select>
      </label>
      <label class="input-label">余额单位性质
        <select v-model="form.balance_unit_semantics" class="input mt-1 w-full">
          <option value="none">无余额结算</option>
          <option value="fiat_currency">法定货币</option>
          <option value="platform_credit">平台积分/额度</option>
        </select>
      </label>
      <label v-if="isCumulative" class="input-label">上游钱包 ID
        <input v-model.number="form.wallet_id" type="number" min="1" class="input mt-1 w-full" />
      </label>
      <label v-if="isCumulative || form.cost_mode === 'request_charge'" class="input-label">协议版本 ID
        <input v-model.number="form.protocol_version_id" type="number" min="1" class="input mt-1 w-full" />
      </label>
      <label v-if="isCumulative" class="input-label">累计计数器归属
        <select v-model="form.counter_scope" class="input mt-1 w-full">
          <option value="account">账号独立</option>
          <option value="wallet">钱包共享（仅观测）</option>
          <option value="organization">组织共享（仅观测）</option>
        </select>
      </label>
      <label v-if="form.cost_mode === 'contract_multiplier'" class="input-label">合同倍率（留空使用账号上游倍率）
        <input v-model="form.contract_multiplier" type="number" min="0" step="0.0001" class="input mt-1 w-full" placeholder="留空时使用上方账号上游倍率" />
      </label>
      <label class="input-label">接口来源
        <select v-model="form.endpoint_source" class="input mt-1 w-full">
          <option value="account_base_url">账号 Base URL</option>
          <option value="wallet_base_url">钱包 Base URL</option>
        </select>
      </label>
      <label class="input-label">安全 Base URL 快照
        <input v-model="form.endpoint_base_url_snapshot" class="input mt-1 w-full" placeholder="https://upstream.example.com" />
      </label>
      <label class="input-label">生效时间
        <input v-model="effectiveFromLocal" type="datetime-local" class="input mt-1 w-full" />
      </label>
      <label class="input-label md:col-span-2">变更原因
        <textarea v-model="form.reason" minlength="5" maxlength="500" rows="2" class="input mt-1 w-full" placeholder="说明本次财务配置变更原因"></textarea>
      </label>
    </div>

    <div class="mt-4 flex justify-end">
      <button type="button" class="btn btn-secondary" :disabled="loading || saving" @click="save">
        {{ saving ? '保存中…' : '保存上游成本配置' }}
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type { AccountFinanceProfileInput, AccountFinanceReadiness } from '@/api/admin/accounts'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ accountId: number }>()
const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const readiness = ref<AccountFinanceReadiness | null>(null)
const effectiveFromLocal = ref(toLocalInput(new Date()))
const form = reactive<AccountFinanceProfileInput>({
  cost_mode: 'contract_multiplier', endpoint_source: 'account_base_url', endpoint_base_url_snapshot: '',
  credential_source: 'account_api_key', counter_scope: 'account', balance_unit_semantics: 'none',
  expected_version: 0, effective_from: new Date().toISOString(), reason: '',
})
const isCumulative = computed(() => form.cost_mode === 'cumulative_list_and_actual' || form.cost_mode === 'cumulative_actual')
const readinessLabel = computed(() => ({ ready_exact: '真实成本就绪', ready_priced: '采购价格就绪', ready_contract: '合同成本就绪', pending_settlement: '等待结算', sync_error: '同步异常', unconfigured: '未配置' }[readiness.value?.status || 'unconfigured']))
const readinessClass = computed(() => readiness.value?.status?.startsWith('ready_') ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-200' : readiness.value?.status === 'sync_error' ? 'bg-red-100 text-red-800 dark:bg-red-950/40 dark:text-red-200' : 'bg-amber-100 text-amber-800 dark:bg-amber-950/40 dark:text-amber-200')

function toLocalInput(value: Date) {
  const offset = value.getTimezoneOffset() * 60_000
  return new Date(value.getTime() - offset).toISOString().slice(0, 16)
}

async function load() {
  loading.value = true
  try {
    readiness.value = await adminAPI.accounts.getFinanceReadiness(props.accountId)
    const profile = readiness.value.profile
    if (profile) {
      Object.assign(form, {
        wallet_id: profile.wallet_id, protocol_version_id: profile.protocol_version_id, cost_mode: profile.cost_mode,
        pricing_group: profile.pricing_group, endpoint_source: profile.endpoint_source,
        endpoint_base_url_snapshot: profile.endpoint_base_url_snapshot, credential_source: profile.credential_source,
        counter_scope: profile.counter_scope, counter_scope_key: profile.counter_scope_key,
        balance_unit_semantics: profile.balance_unit_semantics, contract_type: profile.contract_type,
        contract_multiplier: profile.contract_multiplier, expected_version: profile.version, reason: '',
      })
      effectiveFromLocal.value = toLocalInput(new Date())
    }
  } catch (error: any) {
    appStore.showError(error?.message || '加载账号财务配置失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (form.reason.trim().length < 5) {
    appStore.showError('变更原因至少填写 5 个字符')
    return
  }
  if (isCumulative.value && (!form.wallet_id || !form.protocol_version_id)) {
    appStore.showError('累计结算模式必须填写钱包和协议版本')
    return
  }
  if (form.cost_mode === 'request_charge' && !form.protocol_version_id) {
    appStore.showError('响应扣费模式必须填写协议版本')
    return
  }
  saving.value = true
  try {
    const contractMultiplier = form.contract_multiplier?.trim() || undefined
    const payload: AccountFinanceProfileInput = {
      ...form,
      effective_from: new Date(effectiveFromLocal.value).toISOString(),
      contract_multiplier: contractMultiplier,
      contract_type: form.cost_mode === 'contract_multiplier' && contractMultiplier ? 'multiplier' : undefined,
    }
    const profile = await adminAPI.accounts.updateFinanceProfile(props.accountId, payload)
    form.expected_version = profile.version
    form.reason = ''
    appStore.showSuccess('上游成本配置已保存并生成新版本')
    await load()
  } catch (error: any) {
    appStore.showError(error?.message || '保存账号财务配置失败')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
