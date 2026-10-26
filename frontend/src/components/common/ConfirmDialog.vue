<template>
  <BaseDialog
    :show="show"
    :title="title"
    width="narrow"
    :has-unsaved-changes="hasUnsavedChanges"
    :discard-title="discardTitle || title"
    :discard-message="discardMessage || message"
    :keep-editing-text="cancelText"
    :discard-text="discardText || confirmText"
    @close="handleCancel"
  >
    <div class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">{{ message }}</p>
      <slot />
    </div>
    <template #footer>
      <div class="flex flex-wrap justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="handleCancel">
          {{ cancelText }}
        </button>
        <button type="button" :class="['btn', danger ? 'btn-danger' : 'btn-primary']" @click="handleConfirm">
          {{ confirmText }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from './BaseDialog.vue'

const { t } = useI18n()

interface Props {
  show: boolean
  title: string
  message: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
  hasUnsavedChanges?: boolean
  discardTitle?: string
  discardMessage?: string
  discardText?: string
}

const props = withDefaults(defineProps<Props>(), {
  danger: false,
  hasUnsavedChanges: false
})
const emit = defineEmits<{ (event: 'confirm'): void; (event: 'cancel'): void }>()
const confirmText = computed(() => props.confirmText || t('common.confirm'))
const cancelText = computed(() => props.cancelText || t('common.cancel'))
const discardText = computed(() => props.discardText || t('common.discard', 'Discard changes'))
const discardTitle = computed(() => props.discardTitle || t('common.unsavedChanges', 'Unsaved changes'))
const discardMessage = computed(() => props.discardMessage || props.message)

const handleConfirm = () => emit('confirm')
const handleCancel = () => emit('cancel')
</script>
