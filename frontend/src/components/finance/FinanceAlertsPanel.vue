<template>
  <div class="card overflow-hidden">
    <div class="border-b border-gray-200 p-4 dark:border-dark-700">
      <h2 class="font-semibold text-gray-900 dark:text-white">财务预警</h2>
      <p class="mt-1 text-xs text-gray-500">
        集中处理持续亏损、成本缺失和余额同步异常。
      </p>
    </div>
    <div v-if="error" class="p-6 text-sm text-red-600">{{ error }}</div>
    <div v-else-if="loading" class="p-8 text-center text-sm text-gray-500">
      正在加载财务预警...
    </div>
    <div v-else>
      <div class="divide-y divide-gray-100 dark:divide-dark-700">
        <article v-for="item in items" :key="item.id" class="p-5">
          <div
            class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between"
          >
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span
                  class="rounded-full px-2 py-1 text-xs font-medium"
                  :class="severityClass(item.severity)"
                  >{{ severityLabel(item.severity) }}</span
                ><span
                  class="rounded-full bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                  >{{ statusLabel(item.status) }}</span
                >
                <h3 class="font-semibold text-gray-900 dark:text-white">
                  {{ item.title }}
                </h3>
              </div>
              <p class="mt-2 text-sm text-gray-600 dark:text-gray-300">
                {{ item.description }}
              </p>
              <p class="mt-2 text-xs text-gray-500">
                影响金额 {{ formatFinanceMoney(item.impact_amount) }} ·
                {{ item.request_count }} 条请求 · 最近发生
                {{ formatFinanceDate(item.last_occurred_at) }}
              </p>
            </div>
            <div
              class="grid w-full gap-2 sm:grid-cols-[140px_minmax(200px,1fr)_auto] xl:w-[560px]"
            >
              <select
                v-model="drafts[item.id].status"
                class="input"
                :disabled="updatingId === item.id"
              >
                <option value="open">待处理</option>
                <option value="acknowledged">已确认</option>
                <option value="resolved">已解决</option>
                <option value="ignored">已忽略</option>
              </select>
              <input
                v-model.trim="drafts[item.id].note"
                class="input"
                placeholder="填写处理说明"
                :disabled="updatingId === item.id"
              />
              <button
                class="btn btn-primary"
                :disabled="updatingId === item.id || !drafts[item.id].note"
                @click="submit(item.id)"
              >
                {{ updatingId === item.id ? "保存中..." : "保存处理" }}
              </button>
            </div>
          </div>
        </article>
        <div
          v-if="items.length === 0"
          class="p-10 text-center text-sm text-gray-500"
        >
          所选期间暂无财务预警
        </div>
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
import { reactive, watch } from "vue";
import type { FinanceAlert } from "@/api/admin/finance";
import Pagination from "@/components/common/Pagination.vue";
import { formatFinanceDate, formatFinanceMoney } from "./format";

const props = defineProps<{
  items: FinanceAlert[];
  loading?: boolean;
  error?: string;
  updatingId?: number | null;
  page: number;
  pageSize: number;
  total: number;
}>();
const emit = defineEmits<{
  update: [
    payload: { id: number; status: FinanceAlert["status"]; note: string },
  ];
  "update:page": [value: number];
  "update:pageSize": [value: number];
}>();
const drafts = reactive<
  Record<number, { status: FinanceAlert["status"]; note: string }>
>({});
watch(
  () => props.items,
  (items) => {
    for (const item of items)
      drafts[item.id] = { status: item.status, note: item.handled_note || "" };
  },
  { immediate: true },
);
function submit(id: number) {
  const draft = drafts[id];
  if (draft?.note)
    emit("update", { id, status: draft.status, note: draft.note });
}
function severityLabel(value: FinanceAlert["severity"]) {
  return value === "critical" ? "严重" : value === "warning" ? "警告" : "提示";
}
function severityClass(value: FinanceAlert["severity"]) {
  return value === "critical"
    ? "bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300"
    : value === "warning"
      ? "bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
      : "bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300";
}
function statusLabel(value: FinanceAlert["status"]) {
  return (
    (
      {
        open: "待处理",
        acknowledged: "已确认",
        resolved: "已解决",
        ignored: "已忽略",
      } as Record<string, string>
    )[value] || value
  );
}
</script>
