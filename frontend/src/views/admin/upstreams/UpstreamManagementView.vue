<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">上游管理</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">按 Base URL 聚合同一上游下的多个账号，余额会按账号消耗实时扣减。</p>
        </div>
        <div class="flex gap-2">
          <button class="btn btn-secondary" :disabled="loading || syncing" @click="syncAccounts">一键添加</button>
          <button class="btn btn-primary" @click="openCreate">新建上游</button>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-4">
        <div class="card p-4"><div class="text-sm text-gray-500">上游数</div><div class="mt-2 text-2xl font-semibold">{{ items.length }}</div></div>
        <div class="card p-4"><div class="text-sm text-gray-500">总余额</div><div class="mt-2 text-2xl font-semibold">{{ money(totalCurrentBalance) }}</div></div>
        <div class="card p-4"><div class="text-sm text-gray-500">已消耗</div><div class="mt-2 text-2xl font-semibold">{{ money(totalConsumedBalance) }}</div></div>
        <div class="card p-4"><div class="text-sm text-gray-500">账号数</div><div class="mt-2 text-2xl font-semibold">{{ totalAccounts }}</div></div>
      </div>

      <div class="card overflow-hidden">
        <div v-if="loading" class="p-8 text-center text-gray-500">加载中...</div>
        <table v-else class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-800">
            <tr>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">名称</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">Base URL</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">平台计费</th>
              <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">余额</th>
              <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">已消耗</th>
              <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">账号数</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">告警</th>
              <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="item in items" :key="item.id" class="hover:bg-gray-50 dark:hover:bg-dark-800/60">
              <td class="px-4 py-3"><div class="font-medium text-gray-900 dark:text-white">{{ item.name }}</div><div class="text-xs text-gray-500">{{ item.notes }}</div></td>
              <td class="max-w-md truncate px-4 py-3 text-sm text-gray-600 dark:text-gray-300" :title="item.base_url">{{ item.base_url }}</td>
              <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                <div v-if="item.platform_rates?.length" class="flex max-w-sm flex-wrap gap-1">
                  <span v-for="rate in item.platform_rates" :key="`${item.id}-${rate.platform}`" class="rounded bg-gray-100 px-2 py-0.5 text-xs dark:bg-dark-700">
                    <template v-if="rate.billing_mode === 'image_per_use'">{{ rate.platform }} · 图片 {{ money(rate.image_unit_price) }}/次</template><template v-else>{{ rate.platform }} × {{ rate.rate_multiplier }}</template>
                  </span>
                </div>
                <span v-else class="text-gray-400">未配置平台计费，按 1 倍计算</span>
              </td>
              <td class="px-4 py-3 text-right text-sm font-medium" :class="balanceClass(item)">{{ money(item.current_balance) }}</td>
              <td class="px-4 py-3 text-right text-sm">{{ money(item.consumed_balance) }}</td>
              <td class="px-4 py-3 text-right text-sm">{{ item.account_count }}</td>
              <td class="px-4 py-3 text-sm"><span v-if="item.balance_alert_enabled">低于 {{ money(item.alert_balance || 0) }}</span><span v-else class="text-gray-400">未开启</span></td>
              <td class="px-4 py-3 text-right text-sm"><button class="text-primary-600 hover:underline" @click="openEdit(item)">编辑</button><button class="ml-3 text-red-600 hover:underline" @click="remove(item)">删除</button></td>
            </tr>
            <tr v-if="items.length === 0"><td colspan="8" class="px-4 py-8 text-center text-gray-500">暂无上游，点击“一键添加”从账号 Base URL 同步。</td></tr>
          </tbody>
        </table>
      </div>

      <div v-if="editing" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
        <form class="max-h-[90vh] w-full max-w-4xl overflow-y-auto rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800" @submit.prevent="save">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ editing.id ? '编辑上游' : '新建上游' }}</h2>
          <p class="mt-1 text-xs text-gray-500"><span class="text-red-500">*</span> 为必填项。</p>
          <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
            <label class="block text-sm">名称 <span class="text-red-500">*</span><input v-model="editing.name" class="input mt-1 w-full" required /></label>
            <label class="block text-sm">余额 <span class="text-red-500">*</span><input v-model.number="editing.initial_balance" type="number" min="0" step="0.0001" class="input mt-1 w-full" required /></label>
            <label class="block text-sm md:col-span-2">Base URL <span class="text-red-500">*</span><input v-model="editing.base_url" class="input mt-1 w-full" required /></label>
            <label class="block text-sm">告警余额 <span v-if="editing.balance_alert_enabled" class="text-red-500">*</span><input v-model.number="editing.alert_balance" type="number" min="0" step="0.0001" class="input mt-1 w-full" :required="editing.balance_alert_enabled" /></label>
            <label class="flex items-center gap-2 text-sm"><input v-model="editing.balance_alert_enabled" type="checkbox" />开启余额不足通知</label>
            <label class="block text-sm md:col-span-2">备注<textarea v-model="editing.notes" class="input mt-1 w-full" rows="3"></textarea></label>
          </div>

          <div class="mt-5 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="flex items-center justify-between gap-3">
              <div>
                <div class="font-medium text-gray-900 dark:text-white">按平台设置计费</div>
                <p class="mt-1 text-xs text-gray-500">平台类型从账号管理读取；每个平台只选择一种计费方式：Token 倍率或图片按次金额。</p>
              </div>
              <button type="button" class="btn btn-secondary" @click="addPlatformRate">添加平台计费</button>
            </div>
            <datalist id="upstream-platform-options">
              <option v-for="platform in platformOptions" :key="platform" :value="platform" />
            </datalist>
            <div class="mt-3 space-y-2">
              <div v-for="(rate, index) in editing.platform_rates" :key="index" class="grid grid-cols-1 gap-2 md:grid-cols-[1fr_160px_180px_auto]">
                <label class="text-xs text-gray-500">平台 <span class="text-red-500">*</span><input v-model="rate.platform" list="upstream-platform-options" class="input mt-1 w-full" placeholder="例如 anthropic / openai" required /></label>
                <label class="text-xs text-gray-500">计费方式 <span class="text-red-500">*</span><select v-model="rate.billing_mode" class="input mt-1 w-full" required><option value="token">Token 倍率</option><option value="image_per_use">图片按次</option></select></label>
                <label v-if="rate.billing_mode === 'image_per_use'" class="text-xs text-gray-500">图片按次金额 <span class="text-red-500">*</span><input v-model.number="rate.image_unit_price" type="number" min="0.0001" step="0.0001" class="input mt-1 w-full" placeholder="例如 0.08" required /></label>
                <label v-else class="text-xs text-gray-500">Token 倍率 <span class="text-red-500">*</span><input v-model.number="rate.rate_multiplier" type="number" min="0" step="0.0001" class="input mt-1 w-full" placeholder="倍率" required /></label>
                <div class="flex items-end"><button type="button" class="btn btn-secondary text-red-600" @click="removePlatformRate(index)">删除</button></div>
              </div>
              <div v-if="editing.platform_rates.length === 0" class="rounded bg-gray-50 p-3 text-sm text-gray-500 dark:bg-dark-700/50">暂无平台计费配置，未配置平台按 1 倍计算。</div>
            </div>
          </div>

          <div class="mt-6 flex justify-end gap-2"><button type="button" class="btn btn-secondary" @click="editing = null">取消</button><button class="btn btn-primary" :disabled="saving">保存</button></div>
        </form>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { adminAPI } from '@/api/admin'
import type { Upstream, UpstreamBillingMode, UpstreamInput } from '@/api/admin/upstreams'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const items = ref<Upstream[]>([])
const loading = ref(false)
const saving = ref(false)
const syncing = ref(false)
const editing = ref<(UpstreamInput & { id?: number }) | null>(null)
const fallbackPlatforms = ['anthropic', 'openai', 'gemini', 'azure-openai', 'bedrock', 'vertex', 'xai', 'deepseek', 'openrouter', 'siliconflow']
const accountPlatforms = ref<string[]>([])
const platformOptions = computed(() => accountPlatforms.value.length ? accountPlatforms.value : fallbackPlatforms)

const totalCurrentBalance = computed(() => items.value.reduce((sum, item) => sum + Number(item.current_balance || 0), 0))
const totalConsumedBalance = computed(() => items.value.reduce((sum, item) => sum + Number(item.consumed_balance || 0), 0))
const totalAccounts = computed(() => items.value.reduce((sum, item) => sum + Number(item.account_count || 0), 0))

function money(value: number) { return Number(value || 0).toFixed(4) }
function balanceClass(item: Upstream) { return item.balance_alert_enabled && item.alert_balance != null && item.current_balance <= item.alert_balance ? 'text-red-600' : 'text-gray-900 dark:text-white' }
function errorMessage(e: unknown, fallback: string) { return e && typeof e === 'object' && 'message' in e ? String((e as { message?: unknown }).message || fallback) : fallback }
function normalizeBillingMode(value: unknown): UpstreamBillingMode { return value === 'image_per_use' ? 'image_per_use' : 'token' }
function toInput(item?: Upstream): UpstreamInput & { id?: number } { return item ? { id: item.id, base_url: item.base_url, name: item.name, rate_multiplier: 1, platform_rates: (item.platform_rates || []).map(rate => ({ id: rate.id, platform: rate.platform, billing_mode: normalizeBillingMode(rate.billing_mode), rate_multiplier: rate.rate_multiplier || 1, image_unit_price: rate.image_unit_price || 0 })), initial_balance: item.initial_balance, balance_alert_enabled: item.balance_alert_enabled, alert_balance: item.alert_balance ?? null, notes: item.notes || '' } : { base_url: '', name: '', rate_multiplier: 1, platform_rates: [], initial_balance: 0, balance_alert_enabled: false, alert_balance: null, notes: '' } }
function openCreate() { editing.value = toInput() }
function openEdit(item: Upstream) { editing.value = toInput(item) }
function addPlatformRate() {
  if (!editing.value) return
  const used = new Set(editing.value.platform_rates.map(rate => rate.platform.trim().toLowerCase()).filter(Boolean))
  const platform = platformOptions.value.find(value => !used.has(value)) || ''
  editing.value.platform_rates.push({ platform, billing_mode: 'token', rate_multiplier: 1, image_unit_price: 0 })
}
function removePlatformRate(index: number) { editing.value?.platform_rates.splice(index, 1) }
async function loadPlatforms() {
  try {
    const response = await adminAPI.accounts.list(1, 1000, { lite: 'true' })
    const rows = Array.isArray(response?.items) ? response.items : []
    accountPlatforms.value = Array.from(new Set(rows.map(row => String(row.platform || '').trim().toLowerCase()).filter(Boolean))).sort()
  } catch {
    accountPlatforms.value = []
  }
}
async function load() { loading.value = true; try { const [upstreams] = await Promise.all([adminAPI.upstreams.list(), loadPlatforms()]); items.value = upstreams } finally { loading.value = false } }
async function save() {
  if (!editing.value) return
  if (editing.value.balance_alert_enabled && (editing.value.alert_balance == null || Number.isNaN(Number(editing.value.alert_balance)))) {
    appStore.showError('开启余额不足通知时，告警余额必填')
    return
  }
  saving.value = true
  try {
    const { id, ...payload } = editing.value
    payload.platform_rates = payload.platform_rates
      .map(rate => ({ ...rate, platform: rate.platform.trim().toLowerCase(), billing_mode: normalizeBillingMode(rate.billing_mode), rate_multiplier: rate.billing_mode === 'image_per_use' ? 1 : Number(rate.rate_multiplier || 1), image_unit_price: rate.billing_mode === 'image_per_use' ? Number(rate.image_unit_price || 0) : 0 }))
      .filter(rate => rate.platform)
    if (id) await adminAPI.upstreams.update(id, payload)
    else await adminAPI.upstreams.create(payload)
    editing.value = null
    await load()
    appStore.showSuccess('保存成功')
  } catch (e: unknown) {
    appStore.showError(errorMessage(e, '保存失败'))
  } finally {
    saving.value = false
  }
}
async function syncAccounts() { syncing.value = true; try { const res = await adminAPI.upstreams.syncFromAccounts(); await load(); appStore.showSuccess(`已添加 ${res.created} 个上游`) } catch (e: unknown) { appStore.showError(errorMessage(e, '同步失败')) } finally { syncing.value = false } }
async function remove(item: Upstream) {
  if (!window.confirm(`确认删除上游 ${item.name}？`)) return
  try {
    await adminAPI.upstreams.deleteUpstream(item.id)
    await load()
    appStore.showSuccess('删除成功')
  } catch (e: unknown) {
    appStore.showError(errorMessage(e, '删除失败'))
  }
}
onMounted(load)
</script>
