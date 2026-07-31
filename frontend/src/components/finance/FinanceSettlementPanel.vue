<template>
  <section class="space-y-4" data-testid="finance-settlement-panel">
    <div class="card p-4">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h2 class="font-semibold text-gray-900 dark:text-white">上游成本结算</h2>
          <p class="mt-1 text-xs text-gray-500">
            核对累计上游扣费形成的结算区间、请求分摊和历史修订。充值比例不参与这里的成本计算。
          </p>
        </div>
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-[160px_160px_auto]">
          <label class="text-xs text-gray-600 dark:text-gray-300">
            状态
            <select v-model="status" class="input mt-1 w-full" @change="reloadFromFirstPage">
              <option value="">全部</option>
              <option value="pending">待结算</option>
              <option value="settled">已结算</option>
              <option value="needs_review">待处理</option>
              <option value="failed">失败</option>
            </select>
          </label>
          <label class="text-xs text-gray-600 dark:text-gray-300">
            账号 ID
            <input v-model.number="accountId" type="number" min="1" class="input mt-1 w-full" placeholder="全部账号" />
          </label>
          <button class="btn btn-secondary" :disabled="loading" @click="reloadFromFirstPage">
            {{ loading ? "加载中..." : "查询" }}
          </button>
        </div>
      </div>
    </div>

    <p v-if="error" class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
    </p>

    <div class="card overflow-hidden">
      <div class="overflow-x-auto" role="region" aria-label="上游成本结算区间" tabindex="0">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800">
            <tr>
              <th class="px-4 py-3">账号 / 区间</th>
              <th class="px-4 py-3 text-right">上游原价</th>
              <th class="px-4 py-3 text-right">实际扣费</th>
              <th class="px-4 py-3 text-right">观测倍率</th>
              <th class="px-4 py-3 text-right">分摊差额（USD）</th>
              <th class="px-4 py-3">状态</th>
              <th class="px-4 py-3 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="item in items" :key="item.id">
              <td class="px-4 py-3">
                <div class="font-medium text-gray-900 dark:text-white">账号 #{{ item.account_id || item.owner_id }}</div>
                <div class="mt-1 whitespace-nowrap text-xs text-gray-500">
                  {{ formatFinanceDate(item.period_start) }} 至 {{ formatFinanceDate(item.period_end) }}
                </div>
                <div class="mt-1 text-xs text-gray-400">修订 v{{ item.current_revision }} · {{ item.request_count }} 个请求</div>
              </td>
              <td class="px-4 py-3 text-right tabular-nums">{{ item.list_cost_delta ? money(item.list_cost_delta, item.currency) : "—" }}</td>
              <td class="px-4 py-3 text-right font-medium tabular-nums">{{ money(item.actual_cost_delta, item.currency) }}</td>
              <td class="px-4 py-3 text-right tabular-nums">{{ item.observed_multiplier ? `${item.observed_multiplier}x` : "—" }}</td>
              <td class="px-4 py-3 text-right tabular-nums" :class="differenceClass(item.difference_amount)">
                {{ item.difference_amount == null ? "—" : money(item.difference_amount, "USD") }}
              </td>
              <td class="px-4 py-3">
                <span class="rounded-full px-2 py-1 text-xs font-medium" :class="statusClass(item.status)">{{ statusLabel(item.status) }}</span>
                <p v-if="item.error_summary" class="mt-2 max-w-xs text-xs text-red-600 dark:text-red-300">{{ item.error_summary }}</p>
              </td>
              <td class="px-4 py-3 text-right">
                <button class="btn btn-secondary" @click="openDetail(item.id)">查看分摊</button>
              </td>
            </tr>
            <tr v-if="!loading && items.length === 0">
              <td colspan="7" class="p-10 text-center text-gray-500">暂无结算区间</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="flex items-center justify-between border-t border-gray-200 p-4 text-sm dark:border-dark-700">
        <span class="text-gray-500">共 {{ total }} 条</span>
        <div class="flex gap-2">
          <button class="btn btn-secondary" :disabled="page <= 1 || loading" @click="changePage(page - 1)">上一页</button>
          <span class="px-2 py-2 text-gray-600 dark:text-gray-300">第 {{ page }} 页</span>
          <button class="btn btn-secondary" :disabled="page * pageSize >= total || loading" @click="changePage(page + 1)">下一页</button>
        </div>
      </div>
    </div>

    <section v-if="detail" class="card overflow-hidden" data-testid="finance-settlement-detail">
      <div class="flex flex-col gap-3 border-b border-gray-200 p-4 lg:flex-row lg:items-start lg:justify-between dark:border-dark-700">
        <div>
          <h3 class="font-semibold text-gray-900 dark:text-white">区间 #{{ detail.interval.id }} 分摊明细</h3>
          <p class="mt-1 text-xs text-gray-500">
            当前修订 v{{ detail.interval.current_revision }}，有效分摊合计
            {{ money(detail.interval.allocated_cost_total, "USD") }}，差额
            {{ money(detail.interval.difference_amount, "USD") }}。分摊金额统一为 USD。
          </p>
          <p v-if="detail.interval.fx_rate_to_usd" class="mt-1 text-xs text-gray-500" data-testid="finance-settlement-fx-evidence">
            历史汇率：1 {{ detail.interval.currency }} = {{ detail.interval.fx_rate_to_usd }} USD
            <template v-if="detail.interval.fx_rate_version_id"> · 版本 #{{ detail.interval.fx_rate_version_id }}</template>
            <template v-if="detail.interval.fx_source"> · {{ detail.interval.fx_source }}</template>
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button
            v-if="detail.interval.status !== 'settled' && detail.interval.unit_semantics === 'fiat_currency'"
            class="btn btn-secondary"
            :disabled="working"
            @click="retry"
          >
            重新结算
          </button>
          <button class="btn btn-secondary" @click="detail = null">关闭</button>
        </div>
      </div>

      <form v-if="detail.interval.status === 'settled'" class="flex flex-col gap-3 border-b border-gray-200 bg-gray-50 p-4 sm:flex-row sm:items-end dark:border-dark-700 dark:bg-dark-800/50" @submit.prevent="reallocate">
        <label class="flex-1 text-sm text-gray-700 dark:text-gray-200">
          重新分摊原因
          <input v-model.trim="reason" class="input mt-1 w-full" minlength="5" maxlength="500" required placeholder="说明重新分摊的业务原因" />
        </label>
        <button class="btn btn-primary" :disabled="working || reason.length < 5">
          {{ working ? "处理中..." : "创建新修订" }}
        </button>
      </form>

      <div class="overflow-x-auto" role="region" aria-label="请求成本分摊明细" tabindex="0">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800">
            <tr><th class="px-4 py-3">请求 / 尝试</th><th class="px-4 py-3">修订</th><th class="px-4 py-3 text-right">标准成本权重</th><th class="px-4 py-3 text-right">分摊比例</th><th class="px-4 py-3 text-right">实际成本</th><th class="px-4 py-3">状态</th></tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="allocation in detail.allocations || []" :key="allocation.id" :class="allocation.invalidated_at ? 'opacity-50' : ''">
              <td class="px-4 py-3"><div class="font-medium">{{ allocation.request_id || `#${allocation.usage_log_id}` }}</div><div class="text-xs text-gray-500">使用 #{{ allocation.usage_log_id }} · 尝试 {{ allocation.attempt_no }}</div></td>
              <td class="px-4 py-3">v{{ allocation.revision }}</td>
              <td class="px-4 py-3 text-right tabular-nums">{{ allocation.standard_cost_weight }}</td>
              <td class="px-4 py-3 text-right tabular-nums">{{ formatRate(allocation.allocation_rate) }}</td>
              <td class="px-4 py-3 text-right font-medium tabular-nums">{{ money(allocation.allocated_cost, "USD") }}</td>
              <td class="px-4 py-3">{{ allocation.invalidated_at ? "历史失效" : "当前有效" }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { adminAPI } from "@/api/admin";
import type {
  FinanceSettlementDetail,
  FinanceSettlementInterval,
  FinanceSettlementStatus,
} from "@/api/admin/finance";
import { extractApiErrorMessage } from "@/utils/apiError";
import { formatFinanceDate, formatFinanceMoney } from "./format";

const items = ref<FinanceSettlementInterval[]>([]);
const detail = ref<FinanceSettlementDetail | null>(null);
const status = ref<FinanceSettlementStatus | "">("");
const accountId = ref<number | null>(null);
const page = ref(1);
const pageSize = 20;
const total = ref(0);
const loading = ref(false);
const working = ref(false);
const error = ref("");
const reason = ref("");

function money(value?: string | null, currency?: string) {
  return value == null ? "—" : formatFinanceMoney(value, currency || "USD");
}
function formatRate(value: string) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? `${(parsed * 100).toFixed(4)}%` : value;
}
function statusLabel(value: FinanceSettlementStatus) {
  return ({ pending: "待结算", settled: "已结算", needs_review: "待处理", failed: "失败" } as const)[value];
}
function statusClass(value: FinanceSettlementStatus) {
  return value === "settled"
    ? "bg-emerald-100 text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300"
    : value === "pending"
      ? "bg-blue-100 text-blue-800 dark:bg-blue-950/40 dark:text-blue-300"
      : "bg-red-100 text-red-800 dark:bg-red-950/40 dark:text-red-300";
}
function differenceClass(value?: string) {
  return value != null && Number(value) !== 0 ? "text-red-700 dark:text-red-300" : "text-emerald-700 dark:text-emerald-300";
}
async function load() {
  loading.value = true;
  error.value = "";
  try {
    const result = await adminAPI.finance.getSettlements({
      status: status.value || undefined,
      account_id: accountId.value || undefined,
      page: page.value,
      page_size: pageSize,
    });
    items.value = result.items || [];
    total.value = result.total;
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, "结算区间加载失败");
  } finally {
    loading.value = false;
  }
}
function reloadFromFirstPage() {
  page.value = 1;
  void load();
}
function changePage(next: number) {
  page.value = next;
  void load();
}
async function openDetail(id: number) {
  error.value = "";
  try {
    detail.value = await adminAPI.finance.getSettlement(id);
    reason.value = "";
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, "结算详情加载失败");
  }
}
async function retry() {
  if (!detail.value) return;
  working.value = true;
  error.value = "";
  try {
    detail.value = await adminAPI.finance.retrySettlement(detail.value.interval.id);
    await load();
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, "重新结算失败");
  } finally {
    working.value = false;
  }
}
async function reallocate() {
  if (!detail.value || reason.value.length < 5) return;
  working.value = true;
  error.value = "";
  try {
    detail.value = await adminAPI.finance.reallocateSettlement(detail.value.interval.id, {
      expected_revision: detail.value.interval.current_revision,
      reason: reason.value,
    });
    reason.value = "";
    await load();
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, "重新分摊失败");
  } finally {
    working.value = false;
  }
}

onMounted(load);
</script>
