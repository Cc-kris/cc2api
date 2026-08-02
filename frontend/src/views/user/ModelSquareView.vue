<template>
  <AppLayout>
    <div class="flex h-[calc(100vh-64px-4rem)] min-h-[560px] flex-col gap-4">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p class="text-sm text-gray-500 dark:text-dark-300">{{ t('modelSquare.description') }}</p>
        <div class="flex items-center justify-between gap-3 sm:justify-end">
          <span v-if="catalogUpdatedAt" class="text-xs text-gray-500">
            {{ t('modelSquare.updatedAt', { value: formatDateTime(catalogUpdatedAt) }) }}
          </span>
          <button type="button" class="btn btn-secondary" :disabled="groupsLoading || modelsLoading" @click="refreshAll">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': groupsLoading || modelsLoading }" />
            <span>{{ t('common.refresh') }}</span>
          </button>
        </div>
      </div>

      <div v-if="groupsLoading && !groups.length" class="grid flex-1 gap-4 md:grid-cols-[260px_minmax(0,1fr)]">
        <div class="animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-800" />
        <div class="animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-800" />
      </div>

      <div v-else-if="groupsError" class="card flex flex-1 items-center justify-center p-8">
        <EmptyState :title="t('modelSquare.loadFailed')" :description="errorMessage(groupsError)" :action-text="t('common.retry')" :action-icon="false" @action="refreshAll" />
      </div>

      <div v-else-if="!groups.length" class="card flex flex-1 items-center justify-center p-8">
        <EmptyState :title="t('modelSquare.noGroups')" :description="t('modelSquare.noGroupsDescription')" :action-text="t('nav.dashboard')" action-to="/dashboard" :action-icon="false" />
      </div>

      <div v-else class="min-h-0 flex-1 overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800 md:grid md:grid-cols-[minmax(220px,260px)_minmax(0,1fr)]">
        <ModelSquareGroupList :groups="groups" :selected-id="selectedGroupId" @select="selectGroup" />

        <section class="flex min-h-0 min-w-0 flex-col">
          <div class="border-b border-gray-200 p-4 dark:border-dark-700">
            <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
              <div v-if="selectedGroup" class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ selectedGroup.name }}</h2>
                  <span class="text-xs text-gray-500">{{ selectedGroup.platform }}</span>
                </div>
                <p class="mt-1 text-xs text-gray-500">
                  {{ selectedGroup.has_custom_multiplier
                    ? t('modelSquare.myMultiplier', { value: formatFixedMultiplier(selectedGroup.effective_multiplier) })
                    : t('modelSquare.defaultMultiplier', { value: formatFixedMultiplier(selectedGroup.effective_multiplier) }) }}
                </p>
              </div>
              <div class="relative w-full xl:w-80">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input v-model="searchQuery" type="search" class="input pl-9 pr-9" :placeholder="t('modelSquare.searchModels')" />
                <button v-if="searchQuery" type="button" class="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-gray-400 hover:text-gray-700" :aria-label="t('modelSquare.clearSearch')" @click="searchQuery = ''">
                  <Icon name="x" size="sm" />
                </button>
              </div>
            </div>
          </div>

          <div v-if="modelsError && !items.length" class="flex flex-1 items-center justify-center p-8">
            <EmptyState :title="t('modelSquare.loadFailed')" :description="errorMessage(modelsError)" :action-text="t('common.retry')" :action-icon="false" @action="loadFirstPage" />
          </div>
          <div v-else-if="!modelsLoading && !items.length" class="flex flex-1 items-center justify-center p-8">
            <EmptyState
              :title="searchQuery ? t('modelSquare.noSearchResults') : t('modelSquare.noModels')"
              :description="searchQuery ? t('modelSquare.noSearchResultsDescription') : t('modelSquare.noModelsDescription')"
              :action-text="searchQuery ? t('modelSquare.clearSearch') : undefined"
              :action-icon="false"
              @action="searchQuery = ''"
            />
          </div>
          <ModelSquareTable
            v-else
            :items="items"
            :loading="modelsLoading"
            :loading-more="loadingMore"
            :has-more="hasMore"
            @load-more="loadNextPage"
            @copied="appStore.showSuccess(t('modelSquare.modelCopied'))"
          />
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useDebounceFn } from '@vueuse/core'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelSquareGroupList from '@/components/model-square/ModelSquareGroupList.vue'
import ModelSquareTable from '@/components/model-square/ModelSquareTable.vue'
import { useModelSquare } from '@/composables/useModelSquare'
import { useAppStore } from '@/stores/app'
import { extractApiErrorCode, extractApiErrorMessage } from '@/utils/apiError'
import { formatFixedMultiplier } from '@/utils/formatters'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const selectedGroupId = ref<number | null>(null)
const searchQuery = ref('')
const appliedSearch = ref('')

const {
  groups, items, groupsLoading, modelsLoading, loadingMore, groupsError, modelsError,
  catalogUpdatedAt, hasMore, loadGroups, loadModels, resetModels,
} = useModelSquare()

const selectedGroup = computed(() => groups.value.find(group => group.id === selectedGroupId.value) ?? null)

function routeGroupId() {
  const value = Number(route.query.group_id)
  return Number.isInteger(value) && value > 0 ? value : null
}

async function initialize() {
  try {
    const result = await loadGroups()
    const requested = routeGroupId()
    const selected = result.groups.some(group => group.id === requested) ? requested : result.groups[0]?.id ?? null
    selectedGroupId.value = selected
    if (selected == null) return
    if (requested !== selected) {
      await router.replace({ query: { ...route.query, group_id: String(selected) } })
    }
    await loadFirstPage()
  } catch {
    // Reactive error state renders the failure. Request cancellations are intentional.
  }
}

async function refreshAll() {
  resetModels()
  await initialize()
}

async function selectGroup(id: number) {
  if (id === selectedGroupId.value) return
  await router.replace({ query: { ...route.query, group_id: String(id) } })
}

async function loadFirstPage() {
  if (!selectedGroupId.value) return
  try {
    await loadModels(selectedGroupId.value, appliedSearch.value, false)
  } catch (error) {
    if (extractApiErrorCode(error) === 'CATALOG_CHANGED') {
      appStore.showInfo(t('modelSquare.catalogChanged'))
      resetModels()
      await loadModels(selectedGroupId.value, appliedSearch.value, false)
    }
  }
}

async function loadNextPage() {
  if (!selectedGroupId.value) return
  try {
    await loadModels(selectedGroupId.value, appliedSearch.value, true)
  } catch (error) {
    if (extractApiErrorCode(error) === 'CATALOG_CHANGED') {
      appStore.showInfo(t('modelSquare.catalogChanged'))
      resetModels()
      await loadModels(selectedGroupId.value, appliedSearch.value, false)
    }
  }
}

const applySearch = useDebounceFn(async () => {
  appliedSearch.value = searchQuery.value.trim()
  resetModels()
  await loadFirstPage()
}, 300)

watch(searchQuery, () => applySearch())
watch(() => route.query.group_id, async () => {
  if (!groups.value.length) return
  const requested = routeGroupId()
  if (!requested || !groups.value.some(group => group.id === requested) || requested === selectedGroupId.value) return
  selectedGroupId.value = requested
  searchQuery.value = ''
  appliedSearch.value = ''
  resetModels()
  await loadFirstPage()
})

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

function errorMessage(error: unknown) {
  return extractApiErrorMessage(error, t('common.error'))
}

onMounted(initialize)
</script>
