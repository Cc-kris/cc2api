<template>
  <div class="hidden h-full min-h-0 flex-col border-r border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800 md:flex">
    <div class="border-b border-gray-200 px-4 py-3 text-sm font-semibold text-gray-900 dark:border-dark-700 dark:text-white">
      {{ t('modelSquare.publicGroups') }}
    </div>
    <div
      ref="listRef"
      role="listbox"
      :aria-label="t('modelSquare.publicGroups')"
      class="min-h-0 flex-1 overflow-y-auto py-2"
      tabindex="0"
      @keydown.down.prevent="moveSelection(1)"
      @keydown.up.prevent="moveSelection(-1)"
      @keydown.enter.prevent="activateFocused"
    >
      <button
        v-for="(group, index) in groups"
        :key="group.id"
        type="button"
        role="option"
        :aria-selected="group.id === selectedId"
        :aria-current="group.id === selectedId ? 'true' : undefined"
        class="group-row w-full border-l-2 px-4 py-3 text-left transition-colors"
        :class="group.id === selectedId
          ? 'border-primary-500 bg-primary-50 text-primary-900 dark:bg-primary-950/30 dark:text-primary-100'
          : 'border-transparent text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-dark-700'"
        @focus="focusedIndex = index"
        @click="select(group.id)"
      >
        <div class="flex items-start justify-between gap-2">
          <span class="truncate text-sm font-medium">{{ group.name }}</span>
          <span class="shrink-0 text-sm font-semibold">{{ group.effective_multiplier }}×</span>
        </div>
        <div class="mt-1 flex items-center gap-2 text-xs text-gray-500 dark:text-dark-300">
          <span>{{ group.platform }}</span>
          <span>·</span>
          <span>{{ t('modelSquare.modelCount', { count: group.model_count }) }}</span>
          <span v-if="group.has_custom_multiplier" class="rounded bg-primary-100 px-1.5 py-0.5 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">
            {{ t('modelSquare.custom') }}
          </span>
        </div>
        <div v-if="group.has_custom_multiplier" class="mt-1 text-xs text-gray-600 dark:text-dark-300">
          {{ t('modelSquare.defaultMultiplier', { value: group.default_multiplier }) }}
        </div>
      </button>
    </div>
  </div>

  <div class="md:hidden">
    <button type="button" class="btn btn-secondary w-full justify-between" @click="drawerOpen = true">
      <span>{{ selectedGroup?.name || t('modelSquare.selectGroup') }}</span>
      <Icon name="chevronDown" size="sm" />
    </button>
    <Teleport to="body">
      <div v-if="drawerOpen" class="fixed inset-0 z-50 flex items-end bg-black/40" @click.self="drawerOpen = false">
        <section class="max-h-[80vh] w-full rounded-t-2xl bg-white p-4 shadow-xl dark:bg-dark-800" role="dialog" aria-modal="true" :aria-label="t('modelSquare.selectGroup')">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('modelSquare.selectGroup') }}</h2>
            <button type="button" class="btn btn-ghost p-2" :aria-label="t('common.close')" @click="drawerOpen = false">
              <Icon name="x" size="md" />
            </button>
          </div>
          <SearchInput v-model="groupSearch" :placeholder="t('modelSquare.searchGroups')" :debounce-ms="0" />
          <div class="mt-3 max-h-[55vh] overflow-y-auto">
            <button
              v-for="group in filteredGroups"
              :key="group.id"
              type="button"
              class="flex w-full items-center justify-between border-b border-gray-100 px-2 py-3 text-left dark:border-dark-700"
              @click="select(group.id)"
            >
              <span>
                <span class="block font-medium text-gray-900 dark:text-white">{{ group.name }}</span>
                <span class="text-xs text-gray-500">{{ group.platform }} · {{ t('modelSquare.modelCount', { count: group.model_count }) }}</span>
              </span>
              <span class="font-semibold text-gray-900 dark:text-white">{{ group.effective_multiplier }}×</span>
            </button>
          </div>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ModelSquareGroup } from '@/api/modelSquare'
import Icon from '@/components/icons/Icon.vue'
import SearchInput from '@/components/common/SearchInput.vue'

const props = defineProps<{ groups: ModelSquareGroup[]; selectedId: number | null }>()
const emit = defineEmits<{ select: [id: number] }>()
const { t } = useI18n()
const drawerOpen = ref(false)
const groupSearch = ref('')
const focusedIndex = ref(0)
const listRef = ref<HTMLElement | null>(null)

const selectedGroup = computed(() => props.groups.find(group => group.id === props.selectedId))
const filteredGroups = computed(() => {
  const query = groupSearch.value.trim().toLocaleLowerCase()
  return query ? props.groups.filter(group => `${group.name} ${group.platform}`.toLocaleLowerCase().includes(query)) : props.groups
})

watch(() => props.selectedId, id => {
  const index = props.groups.findIndex(group => group.id === id)
  if (index >= 0) focusedIndex.value = index
}, { immediate: true })

function select(id: number) {
  drawerOpen.value = false
  groupSearch.value = ''
  emit('select', id)
}

function moveSelection(delta: number) {
  if (!props.groups.length) return
  focusedIndex.value = (focusedIndex.value + delta + props.groups.length) % props.groups.length
  const buttons = listRef.value?.querySelectorAll<HTMLButtonElement>('[role="option"]')
  buttons?.[focusedIndex.value]?.focus()
}

function activateFocused() {
  const group = props.groups[focusedIndex.value]
  if (group) select(group.id)
}
</script>
