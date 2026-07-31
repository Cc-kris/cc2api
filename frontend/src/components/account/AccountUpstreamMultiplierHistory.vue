<template>
  <section class="rounded-lg border border-gray-200 dark:border-dark-600">
    <button
      type="button"
      class="flex w-full items-center justify-between px-4 py-3 text-left text-sm font-medium text-gray-900 dark:text-gray-100"
      :aria-expanded="expanded"
      @click="expanded = !expanded"
    >
      <span>{{ t('admin.accounts.upstreamMultiplierHistory') }}</span>
      <span aria-hidden="true">{{ expanded ? '−' : '+' }}</span>
    </button>

    <div v-if="expanded" class="border-t border-gray-200 p-4 dark:border-dark-600">
      <p v-if="loading" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
      <p v-else-if="error" class="text-sm text-red-600">{{ error }}</p>
      <p v-else-if="recentItems.length === 0" class="text-sm text-gray-500">
        {{ t('admin.accounts.upstreamMultiplierHistoryEmpty') }}
      </p>
      <div v-else class="overflow-x-auto">
        <table class="min-w-full text-left text-xs">
          <thead class="text-gray-500 dark:text-gray-400">
            <tr>
              <th class="pb-2 pr-4 font-medium">{{ t('admin.accounts.upstreamMultiplierEffectiveAt') }}</th>
              <th class="pb-2 pr-4 font-medium">{{ t('admin.accounts.upstreamMultiplierOldValue') }}</th>
              <th class="pb-2 pr-4 font-medium">{{ t('admin.accounts.upstreamMultiplierNewValue') }}</th>
              <th class="pb-2 pr-4 font-medium">{{ t('admin.accounts.upstreamMultiplierOperator') }}</th>
              <th class="pb-2 font-medium">{{ t('admin.accounts.upstreamMultiplierReason') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
            <tr v-for="item in recentItems" :key="item.id">
              <td class="py-2 pr-4 whitespace-nowrap">{{ formatDateTime(item.effective_at) }}</td>
              <td class="py-2 pr-4 font-mono">{{ item.old_multiplier ?? '—' }}</td>
              <td class="py-2 pr-4 font-mono">{{ item.new_multiplier }}</td>
              <td class="py-2 pr-4">{{ item.operator_name ?? '—' }}</td>
              <td class="py-2">{{ item.reason || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <button
        v-if="total > recentItems.length"
        type="button"
        class="mt-3 text-sm font-medium text-primary-600 hover:text-primary-700"
        @click="openAll"
      >
        {{ t('admin.accounts.upstreamMultiplierViewAll') }}
      </button>
    </div>
  </section>

  <BaseDialog
    :show="showAll"
    :title="t('admin.accounts.upstreamMultiplierHistory')"
    width="wide"
    @close="showAll = false"
  >
    <p v-if="allLoading" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
    <p v-else-if="error" class="text-sm text-red-600">{{ error }}</p>
    <div v-else class="overflow-x-auto">
      <table class="min-w-full text-left text-sm">
        <thead class="text-gray-500 dark:text-gray-400">
          <tr>
            <th class="pb-3 pr-4 font-medium">{{ t('admin.accounts.upstreamMultiplierEffectiveAt') }}</th>
            <th class="pb-3 pr-4 font-medium">{{ t('admin.accounts.upstreamMultiplierOldValue') }}</th>
            <th class="pb-3 pr-4 font-medium">{{ t('admin.accounts.upstreamMultiplierNewValue') }}</th>
            <th class="pb-3 pr-4 font-medium">{{ t('admin.accounts.upstreamMultiplierOperator') }}</th>
            <th class="pb-3 font-medium">{{ t('admin.accounts.upstreamMultiplierReason') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
          <tr v-for="item in allItems" :key="item.id">
            <td class="py-3 pr-4 whitespace-nowrap">{{ formatDateTime(item.effective_at) }}</td>
            <td class="py-3 pr-4 font-mono">{{ item.old_multiplier ?? '—' }}</td>
            <td class="py-3 pr-4 font-mono">{{ item.new_multiplier }}</td>
            <td class="py-3 pr-4">{{ item.operator_name ?? '—' }}</td>
            <td class="py-3">{{ item.reason || '—' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <template #footer>
      <div class="flex items-center justify-between gap-3">
        <span class="text-sm text-gray-500">{{ page }} / {{ Math.max(1, pages) }}</span>
        <div class="flex gap-2">
          <button type="button" class="btn btn-secondary" :disabled="page <= 1 || allLoading" @click="loadPage(page - 1)">
            {{ t('admin.accounts.upstreamMultiplierPreviousPage') }}
          </button>
          <button type="button" class="btn btn-secondary" :disabled="page >= pages || allLoading" @click="loadPage(page + 1)">
            {{ t('admin.accounts.upstreamMultiplierNextPage') }}
          </button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AccountUpstreamMultiplierChange } from '@/types'
import { formatDateTime } from '@/utils/format'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ accountId: number }>()
const { t } = useI18n()

const expanded = ref(false)
const loading = ref(false)
const allLoading = ref(false)
const error = ref('')
const recentItems = ref<AccountUpstreamMultiplierChange[]>([])
const allItems = ref<AccountUpstreamMultiplierChange[]>([])
const total = ref(0)
const page = ref(1)
const pages = ref(1)
const showAll = ref(false)

const loadRecent = async () => {
  loading.value = true
  error.value = ''
  try {
    const result = await adminAPI.accounts.listUpstreamMultiplierChanges(props.accountId, 1, 5)
    recentItems.value = result.items
    total.value = result.total
  } catch {
    error.value = t('admin.accounts.upstreamMultiplierHistoryLoadFailed')
  } finally {
    loading.value = false
  }
}

const loadPage = async (nextPage: number) => {
  allLoading.value = true
  error.value = ''
  try {
    const result = await adminAPI.accounts.listUpstreamMultiplierChanges(props.accountId, nextPage, 20)
    allItems.value = result.items
    page.value = result.page
    pages.value = result.pages
  } catch {
    error.value = t('admin.accounts.upstreamMultiplierHistoryLoadFailed')
  } finally {
    allLoading.value = false
  }
}

const openAll = async () => {
  showAll.value = true
  await loadPage(1)
}

watch(
  () => [props.accountId, expanded.value] as const,
  ([, isExpanded]) => {
    if (isExpanded) void loadRecent()
  }
)
</script>
