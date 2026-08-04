<template>
  <div class="card overflow-hidden">
    <div class="border-b border-gray-200 p-4 dark:border-dark-700">
      <h2 class="font-semibold text-gray-900 dark:text-white">亏损追踪</h2>
      <p class="mt-1 text-xs text-gray-500">
        每一行都说明客户少收了多少钱、发生在哪个账号，以及这笔成本是已确认还是估算。
      </p>
    </div>
    <div v-if="error" class="p-6 text-sm text-red-600">{{ error }}</div>
    <div v-else-if="loading" class="p-8 text-center text-sm text-gray-500">
      正在加载亏损记录...
    </div>
    <div v-else>
      <div class="overflow-x-auto">
        <table
          class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700"
        >
          <thead
            class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800"
          >
            <tr>
              <th class="px-4 py-3">发生时间</th>
              <th class="px-4 py-3">客户 / 分组</th>
              <th class="px-4 py-3">模型</th>
              <th class="px-4 py-3">上游 / 账号</th>
              <th class="px-4 py-3 text-right">收入</th>
              <th class="px-4 py-3 text-right">成本</th>
              <th class="px-4 py-3 text-right">亏损</th>
              <th class="px-4 py-3">原因</th>
                <th class="px-4 py-3">系统记录状态</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr
              v-for="item in items"
              :key="item.usage_log_id"
              class="align-top hover:bg-gray-50 dark:hover:bg-dark-800/60"
            >
              <td
                class="whitespace-nowrap px-4 py-3 text-gray-600 dark:text-gray-300"
              >
                {{ formatFinanceDate(item.usage_created_at) }}
              </td>
              <td class="px-4 py-3">
                <p class="font-medium text-gray-900 dark:text-white">
                  {{ item.user_name || `用户 #${item.user_id}` }}
                </p>
                <p class="mt-1 text-xs text-gray-500">
                  {{ item.group_name || "未分组" }}
                </p>
              </td>
              <td class="px-4 py-3">
                <p class="font-medium text-gray-900 dark:text-white">
                  {{ item.requested_model || "-" }}
                </p>
                <p class="mt-1 text-xs text-gray-500">
                  实际：{{ item.upstream_model || "-" }}
                </p>
              </td>
              <td class="px-4 py-3">
                <p class="font-medium text-gray-900 dark:text-white">
                  {{ item.upstream_name || "-" }}
                </p>
                <p class="mt-1 text-xs text-gray-500">
                  {{ item.account_name || "-" }}
                </p>
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinanceMoney(item.revenue) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinanceMoney(item.upstream_cost) }}
              </td>
              <td
                class="px-4 py-3 text-right font-semibold tabular-nums text-red-600 dark:text-red-400"
              >
                {{ formatFinanceMoney(item.loss_amount) }}
              </td>
              <td class="px-4 py-3">
                <p class="text-gray-900 dark:text-white">
                  {{ lossReason(item.loss_reason) }}
                </p>
                <p class="mt-1 max-w-64 text-xs text-gray-500">
                  客户收 {{ formatFinanceMoney(item.revenue) }}，上游花 {{ formatFinanceMoney(item.upstream_cost) }}，少收 {{ formatFinanceMoney(item.loss_amount) }}。
                </p>
                <p class="mt-1 text-xs" :class="item.cost_status === 'exact' ? 'text-emerald-600' : 'text-amber-600'">
                  {{ item.cost_status === 'exact' ? '已确认成本' : '估算成本，金额可能变化' }}
                </p>
                <p class="mt-1 text-xs text-gray-500">
                  {{ item.request_id || `日志 #${item.usage_log_id}` }}
                </p>
              </td>
              <td class="px-4 py-3">
                <p class="font-medium text-gray-900 dark:text-white">
                  {{ statusLabel(item.status) }}
                </p>
                <p v-if="item.assignee_id" class="mt-1 text-xs text-gray-500">
                  负责人 #{{ item.assignee_id }}
                </p>
                <p
                  v-if="item.handled_note"
                  class="mt-1 max-w-56 text-xs text-gray-500"
                >
                  {{ item.handled_note }}
                </p>
                <p v-if="item.alert_id" class="mt-1 text-xs text-gray-500">系统已记录该问题</p>
                <p v-else class="mt-1 text-xs text-amber-600">系统尚未生成关联记录</p>
              </td>
            </tr>
            <tr v-if="items.length === 0">
              <td colspan="9" class="px-4 py-10 text-center text-gray-500">
                所选期间没有已确认亏损记录
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <Pagination
        v-if="total > 0"
        :page="page"
        :total="total"
        :page-size="pageSize"
        @update:page="$emit('update:page', $event)"
        @update:page-size="$emit('update:pageSize', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { FinanceLossItem } from "@/api/admin/finance";
import Pagination from "@/components/common/Pagination.vue";
import { formatFinanceDate, formatFinanceMoney } from "./format";

defineProps<{
  items: FinanceLossItem[];
  loading?: boolean;
  error?: string;
  page: number;
  pageSize: number;
  total: number;
}>();
defineEmits<{
  "update:page": [value: number];
  "update:pageSize": [value: number];
}>();

const reasonLabels: Record<string, string> = {
  sales_multiplier_too_low: "销售倍率不足以覆盖成本",
  upstream_multiplier_increased: "上游倍率上升导致成本增加",
  channel_price_mismatch: "渠道价格与销售价格不匹配",
  fast_cost_not_covered: "Fast 成本未被销售价格覆盖",
  cache_cost_not_covered: "缓存成本未被销售价格覆盖",
  multi_attempt_cost: "多次上游尝试产生额外成本",
  other: "其他已确认亏损",
};

function lossReason(reason: string) {
  return reasonLabels[reason] || reason || "待核实";
}

function statusLabel(status: FinanceLossItem["status"]) {
  return (
    (
      {
        open: "系统已发现",
        acknowledged: "系统已记录",
        resolved: "已恢复",
        ignored: "不再统计",
        untracked: "等待系统记录",
      } as const
    )[status] || status
  );
}
</script>
