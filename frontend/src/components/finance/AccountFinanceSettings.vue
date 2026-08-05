<template>
  <section class="card overflow-hidden" data-testid="account-finance-settings">
    <header
      class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-200 p-4 dark:border-dark-700"
    >
      <div>
        <h2 class="font-semibold text-gray-900 dark:text-white">
          上游账号采购倍率
        </h2>
        <p class="mt-1 text-xs text-gray-500">
          倍率按账号保存。已有数据会直接显示，只在上游采购价格发生变化时修改。
        </p>
      </div>
      <button class="btn btn-secondary btn-sm" :disabled="loading" @click="load">
        {{ loading ? "刷新中..." : "刷新账号" }}
      </button>
    </header>

    <p v-if="error" class="m-4 rounded-lg bg-red-50 p-3 text-sm text-red-700">
      {{ error }}
    </p>

    <div v-else class="overflow-x-auto" role="region" aria-label="上游账号采购倍率" tabindex="0">
      <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
        <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800">
          <tr>
            <th class="px-4 py-3">上游账号</th>
            <th class="px-4 py-3">平台</th>
            <th class="px-4 py-3">采购倍率</th>
            <th class="px-4 py-3 text-right">操作</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="account in accounts" :key="account.id">
            <td class="px-4 py-3 font-medium text-gray-900 dark:text-white">
              {{ account.name }}
            </td>
            <td class="px-4 py-3 text-gray-600 dark:text-gray-300">
              {{ account.platform }}
            </td>
            <td class="px-4 py-3">
              <template v-if="editingID === account.id">
                <input
                  v-model.trim="editingMultiplier"
                  class="input w-36"
                  type="number"
                  min="0"
                  max="9999.9999"
                  step="0.0001"
                  inputmode="decimal"
                  :aria-label="`${account.name} 采购倍率`"
                  @keyup.enter="save(account)"
                  @keyup.esc="cancelEdit"
                />
                <p v-if="validationError" class="mt-1 text-xs text-red-600">
                  {{ validationError }}
                </p>
              </template>
              <span v-else-if="account.upstream_cost_multiplier" class="tabular-nums">
                {{ normalizeMultiplier(account.upstream_cost_multiplier) }}
              </span>
              <span v-else class="text-amber-700 dark:text-amber-300">未填写</span>
            </td>
            <td class="whitespace-nowrap px-4 py-3 text-right">
              <template v-if="editingID === account.id">
                <button
                  class="text-primary-700 hover:underline disabled:opacity-50 dark:text-primary-300"
                  :disabled="saving"
                  @click="save(account)"
                >
                  {{ saving ? "保存中..." : "保存" }}
                </button>
                <button class="ml-3 text-gray-600 hover:underline" :disabled="saving" @click="cancelEdit">
                  取消
                </button>
              </template>
              <button v-else class="text-primary-700 hover:underline dark:text-primary-300" @click="startEdit(account)">
                {{ account.upstream_cost_multiplier ? "修改" : "填写" }}
              </button>
            </td>
          </tr>
          <tr v-if="loading">
            <td colspan="4" class="px-4 py-10 text-center text-gray-500">正在加载账号...</td>
          </tr>
          <tr v-else-if="accounts.length === 0">
            <td colspan="4" class="px-4 py-10 text-center text-gray-500">暂无上游账号</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { adminAPI } from "@/api/admin";
import type { Account } from "@/types";
import { useAppStore } from "@/stores/app";
import { extractApiErrorMessage } from "@/utils/apiError";

const emit = defineEmits<{ changed: [] }>();
const appStore = useAppStore();
const accounts = ref<Account[]>([]);
const loading = ref(false);
const saving = ref(false);
const error = ref("");
const validationError = ref("");
const editingID = ref<number | null>(null);
const editingMultiplier = ref("");

function normalizeMultiplier(value: string | null | undefined) {
  if (value == null || String(value).trim() === "") return "";
  const number = Number(value);
  return Number.isFinite(number) ? number.toFixed(4) : "";
}

function validateMultiplier(value: unknown) {
  const trimmed = String(value ?? "").trim();
  if (!/^\d+(?:\.\d{1,4})?$/.test(trimmed)) return "请输入最多 4 位小数的非负数字";
  const number = Number(trimmed);
  if (!Number.isFinite(number) || number < 0 || number > 9999.9999) return "倍率必须在 0 到 9999.9999 之间";
  return "";
}

function startEdit(account: Account) {
  editingID.value = account.id;
  editingMultiplier.value = normalizeMultiplier(account.upstream_cost_multiplier) || "1.0000";
  validationError.value = "";
}

function cancelEdit() {
  editingID.value = null;
  editingMultiplier.value = "";
  validationError.value = "";
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const response = await adminAPI.accounts.list(1, 1000, { lite: "true", sort_by: "name", sort_order: "asc" });
    accounts.value = response.items || [];
  } catch (caught) {
    error.value = extractApiErrorMessage(caught, "上游账号财务资料加载失败");
  } finally {
    loading.value = false;
  }
}

async function save(account: Account) {
  validationError.value = validateMultiplier(editingMultiplier.value);
  if (validationError.value || saving.value) return;
  saving.value = true;
  try {
    const multiplier = Number(editingMultiplier.value).toFixed(4);
    const updated = await adminAPI.accounts.update(account.id, {
      upstream_cost_multiplier: multiplier,
      upstream_cost_multiplier_change_reason: "经营与财务修改上游采购倍率",
    });
    const index = accounts.value.findIndex((item) => item.id === account.id);
    if (index >= 0) accounts.value[index] = { ...accounts.value[index], ...updated, upstream_cost_multiplier: multiplier };
    cancelEdit();
    emit("changed");
    appStore.showSuccess("采购倍率已保存");
  } catch (caught) {
    appStore.showError(extractApiErrorMessage(caught, "采购倍率保存失败"));
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>
