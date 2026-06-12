<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.tags.manageTitle')"
    width="normal"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="flex gap-2">
        <input
          v-model="newTagName"
          type="text"
          class="input"
          :placeholder="t('admin.users.tags.namePlaceholder')"
          maxlength="50"
          @keyup.enter="handleCreate"
        />
        <button type="button" class="btn btn-primary whitespace-nowrap" :disabled="saving || !newTagName.trim()" @click="handleCreate">
          {{ t('admin.users.tags.add') }}
        </button>
      </div>

      <div v-if="loading" class="rounded-lg bg-gray-50 px-4 py-6 text-sm text-gray-500 dark:bg-dark-800 dark:text-dark-400">
        {{ t('common.loading') }}
      </div>

      <div v-else-if="tags.length === 0" class="rounded-lg border border-dashed border-gray-300 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">
        {{ t('admin.users.tags.empty') }}
      </div>

      <div v-else class="max-h-80 divide-y divide-gray-100 overflow-y-auto rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-700">
        <div v-for="tag in tags" :key="tag.id" class="flex items-center justify-between gap-3 px-4 py-3">
          <span class="inline-flex min-w-0 items-center rounded-full bg-primary-50 px-2.5 py-1 text-sm font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-200">
            <span class="truncate">{{ tag.name }}</span>
          </span>
          <button type="button" class="btn btn-secondary px-3 py-1 text-xs text-red-600 hover:text-red-700 dark:text-red-400" :disabled="deletingId === tag.id" @click="handleDelete(tag.id)">
            {{ deletingId === tag.id ? t('common.deleting') : t('common.delete') }}
          </button>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { UserTag } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'changed'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const tags = ref<UserTag[]>([])
const loading = ref(false)
const saving = ref(false)
const deletingId = ref<number | null>(null)
const newTagName = ref('')

async function loadTags() {
  loading.value = true
  try {
    tags.value = await adminAPI.users.listTags()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.tags.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function handleCreate() {
  const name = newTagName.value.trim()
  if (!name) return
  saving.value = true
  try {
    await adminAPI.users.createTag(name)
    newTagName.value = ''
    await loadTags()
    emit('changed')
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.tags.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function handleDelete(id: number) {
  if (!window.confirm(t('common.confirm'))) return
  deletingId.value = id
  try {
    await adminAPI.users.deleteTag(id)
    await loadTags()
    emit('changed')
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.tags.deleteFailed'))
  } finally {
    deletingId.value = null
  }
}

watch(() => props.show, (show) => {
  if (show) {
    newTagName.value = ''
    void loadTags()
  }
})
</script>
