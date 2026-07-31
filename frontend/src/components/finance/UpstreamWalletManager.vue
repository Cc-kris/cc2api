<template>
  <section class="card overflow-hidden">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 p-4 dark:border-dark-700">
      <div>
        <h2 class="font-semibold text-gray-900 dark:text-white">财务结算钱包</h2>
        <p class="mt-1 text-xs text-gray-500">配置上游采购价格、现金余额或 Token 配额。凭据只保存配置状态，不会回显。</p>
      </div>
      <div class="flex items-center gap-2">
        <select v-model.number="selectedUpstreamId" class="input min-w-52">
          <option :value="0">选择上游</option>
          <option v-for="upstream in upstreams" :key="upstream.id" :value="upstream.id">{{ upstream.name }}</option>
        </select>
        <button class="btn btn-primary" :disabled="!selectedUpstreamId" @click="openCreate">新增钱包</button>
      </div>
    </div>

    <div v-if="!selectedUpstreamId" class="p-8 text-center text-sm text-gray-500">选择一个上游后管理结算钱包。</div>
    <div v-else-if="loading" class="p-8 text-center text-sm text-gray-500">加载钱包中...</div>
    <div v-else class="overflow-x-auto">
      <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
        <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-3">钱包</th><th class="px-4 py-3">类型</th><th class="px-4 py-3">余额范围</th><th class="px-4 py-3">定价同步</th><th class="px-4 py-3">余额/配额同步</th><th class="px-4 py-3">账号</th><th class="px-4 py-3 text-right">操作</th></tr></thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="wallet in wallets" :key="wallet.id">
            <td class="px-4 py-3"><div class="font-medium text-gray-900 dark:text-white">{{ wallet.name }}</div><div class="text-xs text-gray-500">{{ adapterLabel(wallet.adapter_type) }} · {{ wallet.currency }} · {{ wallet.credential_configured ? '凭据已配置' : '无凭据' }}</div></td>
            <td class="px-4 py-3">{{ wallet.balance_kind === 'wallet_cash' ? '现金钱包' : 'Token 配额' }}</td>
            <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ wallet.balance_scope_key || `独立钱包 ${wallet.id}` }}</td>
            <td class="px-4 py-3"><StatusBadge :status="wallet.pricing_sync_status" /><div v-if="wallet.pricing_sync_error" class="mt-1 max-w-56 truncate text-xs text-red-600" :title="wallet.pricing_sync_error">{{ wallet.pricing_sync_error }}</div></td>
            <td class="px-4 py-3"><StatusBadge :status="wallet.balance_kind === 'wallet_cash' ? wallet.balance_sync_status : wallet.quota_sync_status" /><div class="mt-1 text-xs text-gray-500">{{ formatDate(wallet.balance_kind === 'wallet_cash' ? wallet.last_balance_sync_at : wallet.last_quota_sync_at) }}</div></td>
            <td class="px-4 py-3">{{ wallet.assigned_account_count }}</td>
            <td class="px-4 py-3 text-right whitespace-nowrap">
              <template v-if="wallet.adapter_type !== 'manual'"><button class="text-primary-600 hover:underline" :disabled="busy.has(wallet.id)" @click="probeWallet(wallet)">探测</button><button class="ml-3 text-primary-600 hover:underline" :disabled="busy.has(wallet.id)" @click="syncWallet(wallet, 'pricing')">同步定价</button><button class="ml-3 text-primary-600 hover:underline" :disabled="busy.has(wallet.id)" @click="syncWallet(wallet, wallet.balance_kind === 'wallet_cash' ? 'balance' : 'quota')">同步{{ wallet.balance_kind === 'wallet_cash' ? '余额' : '配额' }}</button><template v-if="wallet.adapter_type === 'protocol'"><button class="ml-3 text-primary-600 hover:underline" :disabled="busy.has(wallet.id)" @click="syncWallet(wallet, 'account-usage')">同步账号倍率</button><button class="ml-3 text-primary-600 hover:underline" :disabled="busy.has(wallet.id)" @click="syncWallet(wallet, 'funding')">同步充值</button></template></template>
              <template v-else><button class="text-primary-600 hover:underline" @click="openPriceImport(wallet)">导入价格</button></template>
              <button class="ml-3 text-primary-600 hover:underline" @click="openDetails(wallet)">详情</button>
              <button class="ml-3 text-primary-600 hover:underline" @click="openFundEvent(wallet)">记录资金</button>
              <button class="ml-3 text-primary-600 hover:underline" @click="openAssignment(wallet)">账号归属</button>
              <button class="ml-3 text-primary-600 hover:underline" @click="openEdit(wallet)">编辑</button>
              <button class="ml-3 text-red-600 hover:underline" @click="remove(wallet)">删除</button>
            </td>
          </tr>
          <tr v-if="wallets.length === 0"><td colspan="7" class="px-4 py-8 text-center text-gray-500">当前上游尚未配置财务结算钱包。</td></tr>
        </tbody>
      </table>
    </div>

    <div v-if="editing" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <form class="w-full max-w-2xl rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800" @submit.prevent="save">
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ editing.id ? '编辑结算钱包' : '新增结算钱包' }}</h3>
        <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
          <label class="text-sm">名称 <span class="text-red-500">*</span><input v-model="editing.name" class="input mt-1 w-full" required /></label>
          <label class="text-sm">适配器 <span class="text-red-500">*</span><select v-model="editing.adapter_type" class="input mt-1 w-full"><option value="newapi">NewAPI</option><option value="legacy_openai_billing">Legacy Billing</option><option value="protocol">通用财务协议</option><option value="manual">手工维护</option></select></label>
          <label v-if="editing.adapter_type === 'protocol'" class="text-sm md:col-span-2">协议版本 <span class="text-red-500">*</span><select v-model.number="editing.protocol_version_id" class="input mt-1 w-full" required><option :value="undefined" disabled>请选择已发布协议</option><option v-for="protocol in protocols" :key="protocol.id" :value="protocol.current_version_id">{{ protocol.name }}（{{ protocol.code }}）</option></select></label>
          <label class="text-sm md:col-span-2">Base URL <span v-if="editing.adapter_type !== 'manual'" class="text-red-500">*</span><input v-model="editing.base_url" class="input mt-1 w-full" :required="editing.adapter_type !== 'manual'" /><span v-if="editing.adapter_type === 'manual'" class="mt-1 block text-xs text-gray-500">手工钱包可留空；不会执行上游探测或自动同步。</span></label>
          <label class="text-sm">凭据 <span v-if="!editing.id && editing.adapter_type !== 'manual' && editing.adapter_type !== 'protocol'" class="text-red-500">*</span><input v-model="editing.credential" type="password" autocomplete="new-password" class="input mt-1 w-full" :required="!editing.id && editing.adapter_type !== 'manual' && editing.adapter_type !== 'protocol'" /><span v-if="editing.adapter_type === 'newapi'" class="mt-1 block text-xs text-gray-500">填写 NewAPI 用户中心 Access Token，不是模型 API Key。</span><span v-else-if="editing.adapter_type === 'legacy_openai_billing'" class="mt-1 block text-xs text-gray-500">填写 Legacy Billing 的管理凭据，不是模型 API Key。</span><span v-else-if="editing.adapter_type === 'protocol'" class="mt-1 block text-xs text-gray-500">协议选择“钱包财务凭据”时填写；协议使用账号凭据时留空。</span><span v-else class="mt-1 block text-xs text-gray-500">手工钱包不需要凭据。</span><span v-if="editing.id" class="mt-1 block text-xs text-gray-500">留空保留原凭据；系统不会回显现有凭据。</span></label>
          <label class="text-sm">余额类型 <select v-model="editing.balance_kind" class="input mt-1 w-full"><option value="wallet_cash">现金余额</option><option value="token_quota">Token 配额</option></select></label>
          <label class="text-sm">币种 <span class="text-red-500">*</span><input v-model="editing.currency" maxlength="3" class="input mt-1 w-full uppercase" required /></label>
          <label class="text-sm">共享余额范围<input v-model="editing.balance_scope_key" class="input mt-1 w-full" placeholder="同一上游共享余额的钱包填写相同值" /></label>
          <label class="text-sm">NewAPI 定价分组<input v-model="editing.pricing_group" class="input mt-1 w-full" placeholder="例如 default" /></label>
          <label class="flex items-center gap-2 pt-6 text-sm"><input v-model="editing.enabled" type="checkbox" />启用钱包</label>
        </div>
        <div class="mt-6 flex justify-end gap-2"><button type="button" class="btn btn-secondary" @click="editing = null">取消</button><button class="btn btn-primary" :disabled="saving">保存</button></div>
      </form>
    </div>

    <div v-if="assigning" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <form class="max-h-[90vh] w-full max-w-2xl overflow-y-auto rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800" @submit.prevent="saveAssignment">
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">账号归属 · {{ assigning.name }}</h3>
        <div class="mt-4 grid max-h-72 grid-cols-1 gap-2 overflow-y-auto rounded border border-gray-200 p-3 md:grid-cols-2 dark:border-dark-700">
          <label v-for="account in accountOptions" :key="account.id" class="flex items-center gap-2 text-sm"><input v-model="assignmentAccountIds" type="checkbox" :value="account.id" />{{ account.name || `账号 ${account.id}` }}（{{ account.platform }}）</label>
          <p v-if="accountOptions.length === 0" class="text-sm text-gray-500">没有可分配账号。</p>
        </div>
        <label class="mt-4 block text-sm">生效时间 <span class="text-red-500">*</span><input v-model="assignmentEffectiveAt" type="datetime-local" class="input mt-1 w-full" required /></label>
        <label class="mt-4 block text-sm">变更原因 <span class="text-red-500">*</span><textarea v-model="assignmentReason" class="input mt-1 w-full" rows="2" required /></label>
        <div class="mt-6 flex justify-end gap-2"><button type="button" class="btn btn-secondary" @click="assigning = null">取消</button><button class="btn btn-primary" :disabled="saving">保存归属</button></div>
      </form>
    </div>

    <div v-if="details" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div class="max-h-[90vh] w-full max-w-5xl overflow-y-auto rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800">
        <div class="flex items-center justify-between"><div><h3 class="text-lg font-semibold text-gray-900 dark:text-white">钱包详情 · {{ details.name }}</h3><p class="text-xs text-gray-500">精确采购价版本与同步历史</p></div><button class="btn btn-secondary" @click="details = null">关闭</button></div>
        <h4 class="mb-2 mt-5 font-medium">价格版本</h4>
        <div class="overflow-x-auto rounded border border-gray-200 dark:border-dark-700"><table class="min-w-full text-sm"><thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-700"><tr><th class="px-3 py-2">模型</th><th class="px-3 py-2">计费</th><th class="px-3 py-2">价格</th><th class="px-3 py-2">来源</th><th class="px-3 py-2">生效时间</th></tr></thead><tbody><tr v-for="price in prices" :key="price.id" class="border-t border-gray-100 dark:border-dark-700"><td class="px-3 py-2">{{ price.model_pattern }}</td><td class="px-3 py-2">{{ price.billing_mode }} {{ price.service_tier }}</td><td class="max-w-md px-3 py-2 font-mono text-xs">{{ JSON.stringify(price.price_detail) }}</td><td class="px-3 py-2">{{ price.source }}</td><td class="px-3 py-2">{{ formatDate(price.effective_from) }}</td></tr><tr v-if="prices.length === 0"><td colspan="5" class="p-6 text-center text-gray-500">暂无价格版本</td></tr></tbody></table></div>
        <h4 class="mb-2 mt-5 font-medium">同步历史</h4>
        <div class="overflow-x-auto rounded border border-gray-200 dark:border-dark-700"><table class="min-w-full text-sm"><thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-700"><tr><th class="px-3 py-2">类型</th><th class="px-3 py-2">状态</th><th class="px-3 py-2">采集/跳过</th><th class="px-3 py-2">耗时</th><th class="px-3 py-2">时间</th><th class="px-3 py-2">错误</th></tr></thead><tbody><tr v-for="run in histories" :key="run.id" class="border-t border-gray-100 dark:border-dark-700"><td class="px-3 py-2">{{ run.sync_type }}</td><td class="px-3 py-2"><StatusBadge :status="run.status" /></td><td class="px-3 py-2">{{ run.collected_count }}/{{ run.skipped_count }}</td><td class="px-3 py-2">{{ run.duration_ms == null ? '-' : `${run.duration_ms} ms` }}</td><td class="px-3 py-2">{{ formatDate(run.started_at) }}</td><td class="max-w-xs px-3 py-2 text-xs text-red-600">{{ run.error_summary || '-' }}</td></tr><tr v-if="histories.length === 0"><td colspan="6" class="p-6 text-center text-gray-500">暂无同步历史</td></tr></tbody></table></div>
        <h4 class="mb-2 mt-5 font-medium">资金事件</h4>
        <div class="overflow-x-auto rounded border border-gray-200 dark:border-dark-700"><table class="min-w-full text-sm"><thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-700"><tr><th class="px-3 py-2">类型</th><th class="px-3 py-2">支付金额</th><th class="px-3 py-2">到账额度</th><th class="px-3 py-2">赠送收益</th><th class="px-3 py-2">状态</th><th class="px-3 py-2">发生时间</th></tr></thead><tbody><tr v-for="event in fundEvents" :key="event.id" class="border-t border-gray-100 dark:border-dark-700"><td class="px-3 py-2">{{ fundEventLabel(event.event_type) }}</td><td class="px-3 py-2">{{ event.original_amount }} {{ event.currency }}</td><td class="px-3 py-2">{{ event.total_credit_units || '-' }}</td><td class="px-3 py-2">{{ event.bonus_income_original == null ? '-' : `${event.bonus_income_original} ${event.currency}` }}</td><td class="px-3 py-2">{{ bonusStatusLabel(event.bonus_status) }}</td><td class="px-3 py-2">{{ formatDate(event.occurred_at) }}</td></tr><tr v-if="fundEvents.length === 0"><td colspan="6" class="p-6 text-center text-gray-500">暂无资金事件</td></tr></tbody></table></div>
      </div>
    </div>

    <div v-if="priceImportWallet" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"><form class="w-full max-w-2xl rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800" @submit.prevent="savePriceImport"><h3 class="text-lg font-semibold">导入手工采购价 · {{ priceImportWallet.name }}</h3><p class="mt-1 text-xs text-gray-500">输入 JSON 数组；每项含 model_pattern、billing_mode、price_detail、currency，可选 effective_at。</p><textarea v-model="priceImportJSON" class="input mt-4 h-48 w-full font-mono text-xs" required /><div class="mt-5 flex justify-end gap-2"><button type="button" class="btn btn-secondary" @click="priceImportWallet = null">取消</button><button class="btn btn-primary" :disabled="saving">导入</button></div></form></div>
    <div v-if="fundWallet" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"><form class="w-full max-w-xl rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800" @submit.prevent="saveFundEvent"><h3 class="text-lg font-semibold">记录钱包资金 · {{ fundWallet.name }}</h3><p class="mt-1 text-xs text-gray-500">充值赠送独立计入财务收益，不会改变账号的上游倍率或历史请求成本。</p><div class="mt-4 grid grid-cols-2 gap-3"><label>类型<select v-model="fundEvent.event_type" class="input mt-1 w-full"><option value="opening_balance">期初余额</option><option value="topup">充值</option><option value="refund">退款</option><option value="adjustment">调整</option></select></label><label>支付金额<input v-model="fundEvent.original_amount" class="input mt-1 w-full" required /></label><label>币种<input v-model="fundEvent.currency" class="input mt-1 w-full" required /></label><label>USD 汇率<input v-model="fundEvent.fx_rate_to_usd" class="input mt-1 w-full" required /></label><label>汇率来源<input v-model="fundEvent.fx_source" class="input mt-1 w-full" required maxlength="80" placeholder="例如银行结算单/手工录入" /></label><label>USD 金额<input v-model="fundEvent.usd_amount" class="input mt-1 w-full" required /></label><label>发生时间<input v-model="fundEvent.occurred_at" type="datetime-local" class="input mt-1 w-full" required /></label><label v-if="fundEvent.event_type === 'topup' || fundEvent.event_type === 'refund'" class="col-span-2">上游交易号<input v-model="fundEvent.reference_no" class="input mt-1 w-full" required maxlength="200" placeholder="用于阻止同一笔上游交易重复入账" /></label><template v-if="fundEvent.event_type === 'topup'"><label>基础到账额度<input v-model="fundEvent.base_credit_units" class="input mt-1 w-full" placeholder="例如 5000" /></label><label>赠送到账额度<input v-model="fundEvent.bonus_credit_units" class="input mt-1 w-full" placeholder="例如 500" /></label><p class="col-span-2 rounded bg-blue-50 p-3 text-xs text-blue-800 dark:bg-blue-900/20 dark:text-blue-200">{{ rechargePreview }}</p></template><label v-if="fundEvent.event_type === 'refund'" class="col-span-2">被冲正的充值事件 ID<input v-model.number="fundEvent.reversed_event_id" type="number" min="1" class="input mt-1 w-full" placeholder="仅全额退款时填写" /></label><label class="col-span-2">备注<input v-model="fundEvent.note" class="input mt-1 w-full" required /></label></div><div class="mt-5 flex justify-end gap-2"><button type="button" class="btn btn-secondary" @click="fundWallet = null">取消</button><button class="btn btn-primary" :disabled="saving">保存</button></div></form></div>
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref, watch } from 'vue'
import { adminAPI } from '@/api/admin'
import type { Upstream } from '@/api/admin/upstreams'
import type { UpstreamFinancePriceInput, UpstreamFinancePriceVersion, UpstreamFinanceProtocol, UpstreamFinanceSyncHistory, UpstreamFundEvent, UpstreamWallet, UpstreamWalletInput } from '@/api/admin/upstreamWallets'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{ upstreams: Upstream[] }>()
const appStore = useAppStore()
const selectedUpstreamId = ref(0)
const wallets = ref<UpstreamWallet[]>([])
const loading = ref(false)
const saving = ref(false)
const busy = reactive(new Set<number>())
const editing = ref<(UpstreamWalletInput & { id?: number }) | null>(null)
const assigning = ref<UpstreamWallet | null>(null)
const details = ref<UpstreamWallet | null>(null)
const prices = ref<UpstreamFinancePriceVersion[]>([])
const histories = ref<UpstreamFinanceSyncHistory[]>([])
const fundEvents = ref<UpstreamFundEvent[]>([])
const protocols = ref<UpstreamFinanceProtocol[]>([])
const accountOptions = ref<Array<{ id: number; name?: string; platform?: string }>>([])
const assignmentAccountIds = ref<number[]>([])
const assignmentEffectiveAt = ref('')
const assignmentReason = ref('')
const priceImportWallet = ref<UpstreamWallet | null>(null)
const priceImportJSON = ref('[]')
const fundWallet = ref<UpstreamWallet | null>(null)
const fundEventIdempotencyKey = ref('')
const fundEvent = reactive({ event_type: 'topup' as 'opening_balance' | 'topup' | 'refund' | 'adjustment', original_amount: '', currency: 'USD', fx_rate_to_usd: '1', fx_source: 'manual', usd_amount: '', base_credit_units: '', bonus_credit_units: '', reversed_event_id: undefined as number | undefined, reference_no: '', occurred_at: '', note: '' })
const selectedUpstream = computed(() => props.upstreams.find(item => item.id === selectedUpstreamId.value))
const rechargePreview = computed(() => {
  const paid = Number(fundEvent.original_amount); const base = Number(fundEvent.base_credit_units); const bonus = Number(fundEvent.bonus_credit_units || 0)
  if (!(paid > 0) || !(base > 0) || bonus < 0) return '填写基础到账额度后，系统会计算充值比例、实际到账比例与赠送收益。'
  const bonusIncome = bonus * paid / base
  return `基础充值比例 1:${(base / paid).toFixed(4)}；实际到账比例 1:${((base + bonus) / paid).toFixed(4)}；本次赠送收益 ${bonusIncome.toFixed(4)} ${fundEvent.currency || ''}。`
})

const StatusBadge = defineComponent({ props: { status: { type: String, required: true } }, setup(p) { return () => h('span', { class: ['rounded-full px-2 py-1 text-xs', ['success', 'completed'].includes(p.status) ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : ['failed', 'partial'].includes(p.status) ? 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'] }, statusLabel(p.status)) } })

function statusLabel(status: string) { return ({ idle: '未同步', queued: '排队中', running: '同步中', success: '成功', completed: '成功', failed: '失败', partial: '部分成功', unsupported: '不支持' } as Record<string, string>)[status] || status || '未知' }
function fundEventLabel(eventType: string) { return ({ opening_balance: '期初余额', topup: '充值', refund: '退款', adjustment: '调整' } as Record<string, string>)[eventType] || eventType }
function bonusStatusLabel(status: string) { return ({ not_applicable: '不适用', confirmed: '已确认', unresolved: '待确认', reversed: '已冲正' } as Record<string, string>)[status] || status }
function adapterLabel(adapter: string) { return adapter === 'newapi' ? 'NewAPI' : adapter === 'legacy_openai_billing' ? 'Legacy Billing（Token 配额）' : adapter === 'protocol' ? '通用财务协议' : '手工维护' }
function formatDate(value: string | null) { return value ? new Date(value).toLocaleString('zh-CN') : '未同步' }
function errorMessage(error: unknown, fallback: string) { return extractApiErrorMessage(error, fallback) }
function emptyWallet(): UpstreamWalletInput & { id?: number } { return { name: '', adapter_type: 'newapi', base_url: '', credential: '', currency: 'USD', balance_kind: 'wallet_cash', balance_scope_key: '', pricing_group: '', protocol_version_id: undefined, enabled: true } }

async function loadWallets() {
  if (!selectedUpstreamId.value) { wallets.value = []; return }
  loading.value = true
  try { wallets.value = await adminAPI.upstreamWallets.list(selectedUpstreamId.value) }
  catch (error) { appStore.showError(errorMessage(error, '钱包加载失败')) }
  finally { loading.value = false }
}
function openCreate() { editing.value = emptyWallet() }
function openEdit(wallet: UpstreamWallet) { editing.value = { id: wallet.id, name: wallet.name, adapter_type: wallet.adapter_type, base_url: wallet.base_url || '', credential: '', currency: wallet.currency, balance_kind: wallet.balance_kind, balance_scope_key: wallet.balance_scope_key || '', pricing_group: wallet.pricing_group || '', protocol_version_id: wallet.protocol_version_id, enabled: wallet.enabled } }

async function loadProtocols() { try { protocols.value = await adminAPI.upstreamWallets.listPublishedProtocols() } catch (error) { appStore.showError(errorMessage(error, '财务协议加载失败')) } }
async function save() {
  if (!editing.value) return
  saving.value = true
  try {
    const { id, ...raw } = editing.value
    const payload: UpstreamWalletInput = { ...raw, currency: raw.currency.trim().toUpperCase(), balance_scope_key: raw.balance_scope_key?.trim(), pricing_group: raw.pricing_group?.trim() }
    if (id && !payload.credential) delete payload.credential
    if (id) await adminAPI.upstreamWallets.update(id, payload)
    else await adminAPI.upstreamWallets.create(selectedUpstreamId.value, payload)
    editing.value = null
    await loadWallets()
    appStore.showSuccess('钱包已保存')
  } catch (error) { appStore.showError(errorMessage(error, '钱包保存失败')) }
  finally { saving.value = false }
}
async function withBusy(wallet: UpstreamWallet, action: () => Promise<void>) { busy.add(wallet.id); try { await action() } finally { busy.delete(wallet.id) } }
async function probeWallet(wallet: UpstreamWallet) { await withBusy(wallet, async () => { try { const result = await adminAPI.upstreamWallets.probe(wallet.id); appStore.showSuccess(result.reachable ? `探测成功，耗时 ${result.latency_ms} ms` : `探测未通过：${result.error_summary || '上游不可达'}`); await loadWallets() } catch (error) { appStore.showError(errorMessage(error, '能力探测失败')) } }) }
async function syncWallet(wallet: UpstreamWallet, type: 'pricing' | 'balance' | 'quota' | 'funding' | 'account-usage') { await withBusy(wallet, async () => { try { const result = await adminAPI.upstreamWallets.sync(wallet.id, type); appStore.showSuccess(result.created ? '同步任务已创建' : '已有相同同步任务，未重复创建'); await loadWallets() } catch (error) { appStore.showError(errorMessage(error, '同步任务创建失败')) } }) }
async function remove(wallet: UpstreamWallet) { if (!window.confirm(`确认删除钱包 ${wallet.name}？历史财务记录会保留。`)) return; try { await adminAPI.upstreamWallets.deleteWallet(wallet.id); await loadWallets(); appStore.showSuccess('钱包已删除') } catch (error) { appStore.showError(errorMessage(error, '钱包删除失败')) } }
function normalizedURL(value: unknown) { return String(value || '').trim().toLowerCase().replace(/\/+$/, '') }
function accountURL(item: any) {
  const explicit = item.credentials?.base_url || (item.extra?.custom_base_url_enabled ? item.extra?.custom_base_url : '')
  if (explicit) return normalizedURL(explicit)
  if (item.platform === 'openai') return 'https://api.openai.com'
  if (item.platform === 'gemini') return 'https://generativelanguage.googleapis.com'
  if (item.platform === 'antigravity' && ['api_key', 'apikey'].includes(item.type)) return 'https://api.anthropic.com/antigravity'
  if (['api_key', 'apikey'].includes(item.type)) return 'https://api.anthropic.com'
  return ''
}
async function openAssignment(wallet: UpstreamWallet) { assigning.value = wallet; assignmentAccountIds.value = []; assignmentReason.value = ''; const now = new Date(); now.setMinutes(now.getMinutes() - now.getTimezoneOffset()); assignmentEffectiveAt.value = now.toISOString().slice(0, 16); try { const response = await adminAPI.accounts.list(1, 1000); const target = normalizedURL(selectedUpstream.value?.normalized_base_url || selectedUpstream.value?.base_url); accountOptions.value = (response.items || []).filter(item => accountURL(item) === target).map(item => ({ id: item.id, name: item.name, platform: item.platform })) } catch (error) { appStore.showError(errorMessage(error, '账号列表加载失败')) } }
async function saveAssignment() { if (!assigning.value) return; saving.value = true; try { await adminAPI.upstreamWallets.assignAccounts(assigning.value.id, assignmentAccountIds.value, new Date(assignmentEffectiveAt.value).toISOString(), assignmentReason.value.trim()); assigning.value = null; await loadWallets(); appStore.showSuccess('账号归属已保存') } catch (error) { appStore.showError(errorMessage(error, '账号归属保存失败')) } finally { saving.value = false } }
async function openDetails(wallet: UpstreamWallet) { details.value = wallet; prices.value = []; histories.value = []; fundEvents.value = []; try { const [priceResult, historyResult, fundResult] = await Promise.all([adminAPI.upstreamWallets.listPrices(wallet.id), adminAPI.upstreamWallets.listSyncHistory(wallet.id), adminAPI.upstreamWallets.listFundEvents(wallet.id)]); prices.value = priceResult.items || []; histories.value = historyResult.items || []; fundEvents.value = fundResult.items || [] } catch (error) { appStore.showError(errorMessage(error, '钱包详情加载失败')) } }
function openPriceImport(wallet: UpstreamWallet) { priceImportWallet.value = wallet; priceImportJSON.value = '[]' }
async function savePriceImport() { if (!priceImportWallet.value) return; saving.value = true; try { const prices = JSON.parse(priceImportJSON.value) as UpstreamFinancePriceInput[]; if (!Array.isArray(prices) || prices.length === 0) throw new Error('价格 JSON 必须是非空数组'); const result = await adminAPI.upstreamWallets.importPrices(priceImportWallet.value.id, prices); priceImportWallet.value = null; appStore.showSuccess(`已导入 ${result.created_count} 条价格`) } catch (error) { appStore.showError(errorMessage(error, '价格导入失败')) } finally { saving.value = false } }
function newIdempotencyKey() { return globalThis.crypto?.randomUUID?.() || `fund-${Date.now()}-${Math.random().toString(16).slice(2)}` }
function openFundEvent(wallet: UpstreamWallet) { fundWallet.value = wallet; fundEventIdempotencyKey.value = newIdempotencyKey(); const now = new Date(); now.setMinutes(now.getMinutes() - now.getTimezoneOffset()); Object.assign(fundEvent, { event_type: 'topup', original_amount: '', currency: wallet.currency, fx_rate_to_usd: '1', fx_source: 'manual', usd_amount: '', base_credit_units: '', bonus_credit_units: '', reversed_event_id: undefined, reference_no: '', occurred_at: now.toISOString().slice(0, 16), note: '' }) }
async function saveFundEvent() { if (!fundWallet.value) return; saving.value = true; try { const observedAt = new Date(fundEvent.occurred_at).toISOString(); await adminAPI.upstreamWallets.createFundEvent(fundWallet.value.id, { ...fundEvent, occurred_at: observedAt, fx_observed_at: observedAt }, fundEventIdempotencyKey.value); fundWallet.value = null; fundEventIdempotencyKey.value = ''; appStore.showSuccess('资金事件已记录') } catch (error) { appStore.showError(errorMessage(error, '资金事件保存失败')) } finally { saving.value = false } }

watch(selectedUpstreamId, loadWallets)
watch(() => props.upstreams, values => { if (!selectedUpstreamId.value && values.length) selectedUpstreamId.value = values[0].id }, { immediate: true })
onMounted(() => { void loadWallets(); void loadProtocols() })
</script>
