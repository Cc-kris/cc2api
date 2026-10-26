<template>
  <div
    role="navigation"
    aria-label="Pagination"
    class="neo-pagination flex items-center justify-between rounded-2xl border-2 border-[#121316] bg-white px-4 py-3 shadow-[4px_4px_0_#121316] dark:border-[#E2E8F0] dark:bg-[#181C26] sm:px-6"
  >
    <div class="flex flex-1 items-center justify-between sm:hidden">
      <!-- Mobile pagination -->
      <button
        @click="goToPage(currentPage - 1)"
        :disabled="totalPages === 0 || currentPage <= 1"
        class="relative inline-flex items-center rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600"
      >
        {{ t('pagination.previous') }}
      </button>
      <span class="text-sm text-gray-700 dark:text-gray-300">
        {{ t('pagination.pageOf', { page: currentPage, total: totalPages }) }}
      </span>
      <button
        @click="goToPage(currentPage + 1)"
        :disabled="totalPages === 0 || currentPage >= totalPages"
        class="relative ml-3 inline-flex items-center rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600"
      >
        {{ t('pagination.next') }}
      </button>
    </div>

    <div class="hidden sm:flex sm:flex-1 sm:items-center sm:justify-between">
      <!-- Desktop pagination info -->
      <div class="flex items-center space-x-4">
        <p class="text-sm text-gray-700 dark:text-gray-300">
          {{ t('pagination.showing') }}
          <span class="font-medium">{{ fromItem }}</span>
          {{ t('pagination.to') }}
          <span class="font-medium">{{ toItem }}</span>
          {{ t('pagination.of') }}
          <span class="font-medium">{{ normalizedTotal }}</span>
          {{ t('pagination.results') }}
        </p>

        <!-- Page size selector -->
        <div v-if="showPageSizeSelector" class="flex items-center space-x-2">
          <span class="text-sm text-gray-700 dark:text-gray-300"
            >{{ t('pagination.perPage') }}:</span
          >
          <div class="page-size-select w-20">
            <Select
              :model-value="pageSize"
              :options="pageSizeSelectOptions"
              @update:model-value="handlePageSizeChange"
            />
          </div>
        </div>

        <div v-if="showJump" class="flex items-center space-x-2">
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('pagination.jumpTo') }}</span>
          <input
            v-model="jumpPage"
            type="number"
            min="1"
            :max="totalPages"
            class="input w-20 text-sm"
            :placeholder="t('pagination.jumpPlaceholder')"
            @keyup.enter="submitJump"
          />
          <button type="button" class="btn btn-ghost btn-sm" @click="submitJump">
            {{ t('pagination.jumpAction') }}
          </button>
        </div>
      </div>

      <!-- Desktop pagination buttons -->
      <nav
        class="relative z-0 inline-flex items-center gap-1.5"
        aria-label="Pagination"
      >
        <!-- Previous button -->
        <button
          @click="goToPage(currentPage - 1)"
          :disabled="totalPages === 0 || currentPage <= 1"
          class="neo-page-btn relative inline-flex items-center rounded-lg border-2 border-[#121316] bg-white px-2.5 py-1.5 text-sm font-extrabold text-[#121316] shadow-[2px_2px_0_#121316] transition-all hover:-translate-x-0.5 hover:-translate-y-0.5 hover:bg-[#EDF4FF] disabled:cursor-not-allowed disabled:opacity-45 dark:border-[#E2E8F0] dark:bg-[#222838] dark:text-[#F8FAFC] dark:hover:bg-[#2563EB] dark:hover:text-white"
          :aria-label="t('pagination.previous')"
        >
          <Icon name="chevronLeft" size="md" />
        </button>

        <!-- Page numbers -->
        <button
          v-for="(pageNum, index) in visiblePages"
          :key="`${pageNum}-${index}`"
          @click="typeof pageNum === 'number' && goToPage(pageNum)"
          :disabled="typeof pageNum !== 'number'"
          :class="[
            'neo-page-btn relative inline-flex items-center rounded-lg border-2 border-[#121316] px-3.5 py-1.5 font-mono text-sm font-extrabold transition-all',
            pageNum === currentPage
              ? 'z-10 bg-[#CCFF00] text-[#121316] dark:bg-[#2563EB] dark:text-white dark:border-[#60A5FA] font-black shadow-[2.5px_2.5px_0_#121316]'
              : 'bg-white text-[#121316] shadow-[2px_2px_0_#121316] hover:-translate-x-0.5 hover:-translate-y-0.5 hover:bg-[#EDF4FF] dark:border-[#E2E8F0] dark:bg-[#222838] dark:text-[#F8FAFC] dark:hover:bg-[#2563EB] dark:hover:text-white',
            typeof pageNum !== 'number' && 'cursor-default'
          ]"
          :aria-label="
            typeof pageNum === 'number' ? t('pagination.goToPage', { page: pageNum }) : undefined
          "
          :aria-current="pageNum === currentPage ? 'page' : undefined"
        >
          {{ pageNum }}
        </button>

        <!-- Next button -->
        <button
          @click="goToPage(currentPage + 1)"
          :disabled="totalPages === 0 || currentPage >= totalPages"
          class="neo-page-btn relative inline-flex items-center rounded-lg border-2 border-[#121316] bg-white px-2.5 py-1.5 text-sm font-extrabold text-[#121316] shadow-[2px_2px_0_#121316] transition-all hover:-translate-x-0.5 hover:-translate-y-0.5 hover:bg-[#EDF4FF] disabled:cursor-not-allowed disabled:opacity-45 dark:border-[#E2E8F0] dark:bg-[#222838] dark:text-[#F8FAFC] dark:hover:bg-[#2563EB] dark:hover:text-white"
          :aria-label="t('pagination.next')"
        >
          <Icon name="chevronRight" size="md" />
        </button>
      </nav>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import Select from './Select.vue'
import { getConfiguredTablePageSizeOptions, normalizeTablePageSize } from '@/utils/tablePreferences'
import { setPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()

interface Props {
  total: number
  page: number
  pageSize: number
  pageSizeOptions?: number[]
  showPageSizeSelector?: boolean
  showJump?: boolean
}

interface Emits {
  (e: 'update:page', page: number): void
  (e: 'update:pageSize', pageSize: number): void
}

const props = withDefaults(defineProps<Props>(), {
  pageSizeOptions: () => getConfiguredTablePageSizeOptions(),
  showPageSizeSelector: true,
  showJump: false
})

const emit = defineEmits<Emits>()

const totalPages = computed(() => {
  const pageSize = Number.isFinite(props.pageSize) && props.pageSize > 0 ? props.pageSize : 1
  const total = Number.isFinite(props.total) && props.total > 0 ? props.total : 0
  return Math.ceil(total / pageSize)
})
const currentPage = computed(() => {
  if (totalPages.value === 0) return 0
  const requestedPage = Number.isFinite(props.page) ? Math.floor(props.page) : 1
  return Math.min(Math.max(requestedPage, 1), totalPages.value)
})

const fromItem = computed(() => {
  if (totalPages.value === 0) return 0
  return (currentPage.value - 1) * normalizedPageSize.value + 1
})

const toItem = computed(() => {
  if (totalPages.value === 0) return 0
  return Math.min(currentPage.value * normalizedPageSize.value, normalizedTotal.value)
})

const normalizedPageSize = computed(() =>
  Number.isFinite(props.pageSize) && props.pageSize > 0 ? props.pageSize : 1
)
const normalizedTotal = computed(() =>
  Number.isFinite(props.total) && props.total > 0 ? props.total : 0
)

const pageSizeSelectOptions = computed(() => {
  const options = Array.from(
    new Set([
      ...getConfiguredTablePageSizeOptions(),
      normalizeTablePageSize(props.pageSize)
    ])
  ).sort((a, b) => a - b)

  return options.map((size) => ({
    value: size,
    label: String(size)
  }))
})

const jumpPage = ref('')

const visiblePages = computed(() => {
  const pages: (number | string)[] = []
  const maxVisible = 7
  const total = totalPages.value

  if (total <= maxVisible) {
    // Show all pages if total is small
    for (let i = 1; i <= total; i++) {
      pages.push(i)
    }
  } else {
    // Always show first page
    pages.push(1)

    const start = Math.max(2, currentPage.value - 2)
    const end = Math.min(total - 1, currentPage.value + 2)

    // Add ellipsis before if needed
    if (start > 2) {
      pages.push('...')
    }

    // Add middle pages
    for (let i = start; i <= end; i++) {
      pages.push(i)
    }

    // Add ellipsis after if needed
    if (end < total - 1) {
      pages.push('...')
    }

    // Always show last page
    pages.push(total)
  }

  return pages
})

const goToPage = (newPage: number) => {
  if (newPage >= 1 && newPage <= totalPages.value && newPage !== currentPage.value) {
    emit('update:page', newPage)
  }
}

const handlePageSizeChange = (value: string | number | boolean | null) => {
  if (value === null || typeof value === 'boolean') return
  const newPageSize = normalizeTablePageSize(typeof value === 'string' ? parseInt(value, 10) : value)
  setPersistedPageSize(newPageSize)
  emit('update:pageSize', newPageSize)
}

const submitJump = () => {
  if (totalPages.value === 0) {
    jumpPage.value = ''
    return
  }
  const value = jumpPage.value.trim()
  if (!value) return
  const pageNum = Number.parseInt(value, 10)
  if (Number.isNaN(pageNum)) return
  const nextPage = Math.min(Math.max(pageNum, 1), totalPages.value)
  jumpPage.value = ''
  goToPage(nextPage)
}
</script>

<style scoped>
.page-size-select :deep(.select-trigger) {
  @apply px-3 py-1.5 text-sm;
}
</style>
