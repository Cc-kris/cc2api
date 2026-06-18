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
              <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">默认倍率</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">模型倍率</th>
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
              <td class="px-4 py-3 text-right text-sm">{{ item.rate_multiplier }}</td>
              <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                <div v-if="item.model_rates?.length" class="flex max-w-xs flex-wrap gap-1">
                  <span v-for="rate in item.model_rates" :key="`${item.id}-${rate.model}`" class="rounded bg-gray-100 px-2 py-0.5 text-xs dark:bg-dark-700">{{ rate.model }} × {{ rate.rate_multiplier }}</span>
                </div>
                <span v-else class="text-gray-400">使用默认倍率</span>
              </td>
              <td class="px-4 py-3 text-right text-sm font-medium" :class="balanceClass(item)">{{ money(item.current_balance) }}</td>
              <td class="px-4 py-3 text-right text-sm">{{ money(item.consumed_balance) }}</td>
              <td class="px-4 py-3 text-right text-sm">{{ item.account_count }}</td>
              <td class="px-4 py-3 text-sm"><span v-if="item.balance_alert_enabled">低于 {{ money(item.alert_balance || 0) }}</span><span v-else class="text-gray-400">未开启</span></td>
              <td class="px-4 py-3 text-right text-sm"><button class="text-primary-600 hover:underline" @click="openEdit(item)">编辑</button><button class="ml-3 text-red-600 hover:underline" @click="remove(item)">删除</button></td>
            </tr>
            <tr v-if="items.length === 0"><td colspan="9" class="px-4 py-8 text-center text-gray-500">暂无上游，点击“一键添加”从账号 Base URL 同步。</td></tr>
          </tbody>
        </table>
      </div>

      <div v-if="editing" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
        <form class="max-h-[90vh] w-full max-w-3xl overflow-y-auto rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800" @submit.prevent="save">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ editing.id ? '编辑上游' : '新建上游' }}</h2>
          <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
            <label class="block text-sm">名称<input v-model="editing.name" class="input mt-1 w-full" required /></label>
            <label class="block text-sm">默认倍率<input v-model.number="editing.rate_multiplier" type="number" min="0" step="0.0001" class="input mt-1 w-full" required /></label>
            <label class="block text-sm md:col-span-2">Base URL<input v-model="editing.base_url" class="input mt-1 w-full" required /></label>
            <label class="block text-sm">余额<input v-model.number="editing.initial_balance" type="number" min="0" step="0.0001" class="input mt-1 w-full" /></label>
            <label class="block text-sm">告警余额<input v-model.number="editing.alert_balance" type="number" min="0" step="0.0001" class="input mt-1 w-full" /></label>
            <label class="flex items-center gap-2 text-sm"><input v-model="editing.balance_alert_enabled" type="checkbox" />开启余额不足通知</label>
            <label class="block text-sm md:col-span-2">备注<textarea v-model="editing.notes" class="input mt-1 w-full" rows="3"></textarea></label>
          </div>

          <div class="mt-5 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="flex items-center justify-between gap-3">
              <div>
                <div class="font-medium text-gray-900 dark:text-white">按模型设置倍率</div>
                <p class="mt-1 text-xs text-gray-500">命中模型时使用这里的倍率；未命中时使用上面的默认倍率。</p>
              </div>
              <button type="button" class="btn btn-secondary" @click="addModelRate">添加模型倍率</button>
            </div>
            <div class="mt-3 space-y-2">
              <div v-for="(rate, index) in editing.model_rates" :key="index" class="grid grid-cols-1 gap-2 md:grid-cols-[1fr_160px_auto]">
                <input v-model="rate.model" class="input w-full" placeholder="模型，例如 claude-sonnet-4-5" />
                <input v-model.number="rate.rate_multiplier" type="number" min="0" step="0.0001" class="input w-full" placeholder="倍率" />
                <button type="button" class="btn btn-secondary text-red-600" @click="removeModelRate(index)">删除</button>
              </div>
              <div v-if="editing.model_rates.length === 0" class="rounded bg-gray-50 p-3 text-sm text-gray-500 dark:bg-dark-700/50">暂无模型专属倍率。</div>
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
import type { Upstream, UpstreamInput } from '@/api/admin/upstreams'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const items = ref<Upstream[]>([])
const loading = ref(false)
const saving = ref(false)
const syncing = ref(false)
const editing = ref<(UpstreamInput & { id?: number }) | null>(null)

const totalCurrentBalance = computed(() => items.value.reduce((sum, item) => sum + Number(item.current_balance || 0), 0))
const totalConsumedBalance = computed(() => items.value.reduce((sum, item) => sum + Number(item.consumed_balance || 0), 0))
const totalAccounts = computed(() => items.value.reduce((sum, item) => sum + Number(item.account_count || 0), 0))

function money(value: number) { return Number(value || 0).toFixed(4) }
function balanceClass(item: Upstream) { return item.balance_alert_enabled && item.alert_balance != null && item.current_balance <= item.alert_balance ? 'text-red-600' : 'text-gray-900 dark:text-white' }
function toInput(item?: Upstream): UpstreamInput & { id?: number } { return item ? { id: item.id, base_url: item.base_url, name: item.name, rate_multiplier: item.rate_multiplier, model_rates: (item.model_rates || []).map(rate => ({ id: rate.id, model: rate.model, rate_multiplier: rate.rate_multiplier })), initial_balance: item.initial_balance, balance_alert_enabled: item.balance_alert_enabled, alert_balance: item.alert_balance ?? null, notes: item.notes || '' } : { base_url: '', name: '', rate_multiplier: 1, model_rates: [], initial_balance: 0, balance_alert_enabled: false, alert_balance: null, notes: '' } }
function openCreate() { editing.value = toInput() }
function openEdit(item: Upstream) { editing.value = toInput(item) }
function addModelRate() { editing.value?.model_rates.push({ model: '', rate_multiplier: 1 }) }
function removeModelRate(index: number) { editing.value?.model_rates.splice(index, 1) }
async function load() { loading.value = true; try { items.value = await adminAPI.upstreams.list() } finally { loading.value = false } }
async function save() { if (!editing.value) return; saving.value = true; try { const { id, ...payload } = editing.value; payload.model_rates = payload.model_rates.filter(rate => rate.model.trim()); if (id) await adminAPI.upstreams.update(id, payload); else await adminAPI.upstreams.create(payload); editing.value = null; await load(); appStore.showSuccess('保存成功') } catch (e: unknown) { appStore.showError(e instanceof Error ? e.message : '保存失败') } finally { saving.value = false } }
async function syncAccounts() { syncing.value = true; try { const res = await adminAPI.upstreams.syncFromAccounts(); await load(); appStore.showSuccess(`已添加 ${res.created} 个上游`) } catch (e: unknown) { appStore.showError(e instanceof Error ? e.message : '同步失败') } finally { syncing.value = false } }
async function remove(item: Upstream) { if (!window.confirm(`确认删除上游 ${item.name}？`)) return; await adminAPI.upstreams.deleteUpstream(item.id); await load() }
onMounted(load)
</script>
