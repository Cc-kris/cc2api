<template>
  <div class="card overflow-hidden">
    <div
      class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 p-4 dark:border-dark-700"
    >
      <div>
        <h2 class="font-semibold text-gray-900 dark:text-white">利润分析</h2>
        <p class="mt-1 text-xs text-gray-500">
          按同一口径定位低毛利客户、分组、模型、渠道和上游账号。
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-3">
        <label
          class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300"
        >
          分析维度
          <select
            :value="dimension"
            class="input min-w-36"
            data-testid="finance-dimension"
            @change="
              $emit(
                'update:dimension',
                ($event.target as HTMLSelectElement).value,
              )
            "
          >
            <option value="user">客户</option>
            <option value="group">客户分组</option>
            <option value="requested_model">请求模型</option>
            <option value="upstream_model">上游模型</option>
            <option value="channel">渠道</option>
            <option value="upstream">上游站点</option>
            <option value="wallet">上游钱包</option>
            <option value="account">路由账号</option>
            <option value="billing_type">计费类型</option>
            <option value="business_type">业务类型</option>
          </select>
        </label>
        <button
          class="btn btn-secondary"
          :disabled="exporting"
          data-testid="finance-export-create"
          @click="$emit('export')"
        >
          {{ exporting ? "正在生成导出文件..." : "导出 CSV" }}
        </button>
        <button
          v-if="exportJob?.status === 'completed' && exportJob.download_url"
          class="btn btn-primary"
          data-testid="finance-export-download"
          @click="$emit('download')"
        >
          下载文件
        </button>
      </div>
    </div>
    <div
      v-if="exportJob || exportError"
      class="border-b border-gray-200 px-4 py-3 text-xs dark:border-dark-700"
    >
      <p v-if="exportError" class="text-red-600">{{ exportError }}</p>
      <p
        v-else-if="exportJob"
        class="text-gray-500"
        data-testid="finance-export-status"
      >
        导出任务 #{{ exportJob.id }}：{{ exportStatusLabel(exportJob.status)
        }}<span v-if="exportJob.status === 'running'"
          >（{{ Math.round(Number(exportJob.progress || 0) * 100) }}%）</span
        ><span v-if="exportJob.status === 'completed'"
          >，共 {{ exportJob.row_count ?? 0 }} 行</span
        >
      </p>
    </div>
    <div v-if="error" class="p-6 text-sm text-red-600">{{ error }}</div>
    <div v-else-if="loading" class="p-8 text-center text-sm text-gray-500">
      正在加载利润分析...
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
              <th class="px-4 py-3">维度</th>
              <th class="px-4 py-3 text-right">收入</th>
              <th class="px-4 py-3 text-right">已确认成本</th>
              <th class="px-4 py-3 text-right">输入</th>
              <th class="px-4 py-3 text-right">输出</th>
              <th class="px-4 py-3 text-right">缓存</th>
              <th class="px-4 py-3 text-right">Fast</th>
              <th class="px-4 py-3 text-right">图片</th>
              <th class="px-4 py-3 text-right">视频</th>
              <th class="px-4 py-3 text-right">其他</th>
              <th class="px-4 py-3 text-right">毛利</th>
              <th class="px-4 py-3 text-right">毛利率</th>
              <th class="px-4 py-3 text-right">亏损金额</th>
              <th class="px-4 py-3 text-right">请求数</th>
              <th class="px-4 py-3 text-right">待完善</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr
              v-for="item in items"
              :key="item.dimension_key"
              class="hover:bg-gray-50 dark:hover:bg-dark-800/60"
            >
              <td class="px-4 py-3 font-medium text-gray-900 dark:text-white">
                {{ item.dimension_name || item.dimension_key }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinanceMoney(item.revenue) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinanceMoney(item.upstream_cost) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinanceMoney(item.input_cost) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinanceMoney(item.output_cost) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinanceMoney(item.cache_cost) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinanceMoney(item.fast_cost) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinanceMoney(item.image_cost) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinanceMoney(item.video_cost) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinanceMoney(item.other_cost) }}
              </td>
              <td
                class="px-4 py-3 text-right font-medium tabular-nums"
                :class="financeTone(item.profit)"
              >
                {{ formatFinanceMoney(item.profit) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ formatFinancePercent(item.margin_rate) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums text-red-600">
                {{ formatFinanceMoney(item.loss_amount) }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ item.request_count }}
              </td>
              <td class="px-4 py-3 text-right tabular-nums">
                {{ item.estimated_count + item.missing_count }}
              </td>
            </tr>
            <tr v-if="items.length === 0">
              <td colspan="15" class="px-4 py-10 text-center text-gray-500">
                所选期间暂无利润分析数据
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
import type {
  FinanceBreakdownItem,
  FinanceExportJob,
} from "@/api/admin/finance";
import Pagination from "@/components/common/Pagination.vue";
import {
  financeTone,
  formatFinanceMoney,
  formatFinancePercent,
} from "./format";

defineProps<{
  items: FinanceBreakdownItem[];
  dimension: string;
  loading?: boolean;
  error?: string;
  page: number;
  pageSize: number;
  total: number;
  exporting?: boolean;
  exportJob?: FinanceExportJob | null;
  exportError?: string;
}>();
defineEmits<{
  "update:dimension": [value: string];
  "update:page": [value: number];
  "update:pageSize": [value: number];
  export: [];
  download: [];
}>();

function exportStatusLabel(status: FinanceExportJob["status"]) {
  return (
    {
      queued: "等待处理",
      running: "生成中",
      completed: "已完成",
      failed: "生成失败",
    } as const
  )[status];
}
</script>
