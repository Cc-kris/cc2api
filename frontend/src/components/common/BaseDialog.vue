<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="show"
        class="modal-overlay"
        :style="zIndexStyle"
        role="presentation"
        @click.self="handleOverlayClick"
      >
        <section
          ref="dialogRef"
          :class="['modal-content', `modal-width-${width}`]"
          :style="panelStyle"
          role="dialog"
          :aria-modal="discardConfirmationOpen ? undefined : 'true'"
          :aria-hidden="discardConfirmationOpen ? 'true' : undefined"
          :inert="discardConfirmationOpen"
          :aria-labelledby="dialogTitleId"
          :aria-describedby="describedBy"
          tabindex="-1"
          @click.stop
        >
          <header class="modal-header">
            <h2 :id="dialogTitleId" class="modal-title">{{ title }}</h2>
            <button
              type="button"
              class="modal-close-button"
              aria-label="Close dialog"
              @click="requestClose"
            >
              <Icon name="x" size="md" />
            </button>
          </header>

          <div class="modal-body">
            <slot />
          </div>

          <footer v-if="$slots.footer" class="modal-footer">
            <slot name="footer" />
          </footer>
        </section>

        <div
          v-if="discardConfirmationOpen"
          class="modal-discard-overlay"
          @click.self="cancelDiscard"
        >
          <section
            ref="confirmationRef"
            class="modal-content modal-discard-dialog"
            role="alertdialog"
            aria-modal="true"
            :aria-labelledby="discardTitleId"
            :aria-describedby="discardMessageId"
            tabindex="-1"
            @click.stop
          >
            <header class="modal-header">
              <h2 :id="discardTitleId" class="modal-title">{{ discardTitle }}</h2>
            </header>
            <div class="modal-body">
              <p :id="discardMessageId">{{ discardMessage }}</p>
            </div>
            <footer class="modal-footer modal-discard-actions">
              <button ref="keepEditingButton" type="button" class="btn btn-secondary" @click="cancelDiscard">
                {{ keepEditingText }}
              </button>
              <button type="button" class="btn btn-danger" @click="confirmDiscard">
                {{ discardText }}
              </button>
            </footer>
          </section>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, useId, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'

// Existing call sites keep their width names; each is mapped to the v0.4 modal scale.
type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full' | 'small' | 'medium' | 'large'

interface Props {
  show: boolean
  title: string
  width?: DialogWidth
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  zIndex?: number
  hasUnsavedChanges?: boolean
  describedBy?: string
  discardTitle?: string
  discardMessage?: string
  keepEditingText?: string
  discardText?: string
}

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  closeOnEscape: true,
  closeOnClickOutside: true,
  zIndex: 50,
  hasUnsavedChanges: false,
  discardTitle: 'Discard unsaved changes?',
  discardMessage: 'You have unsaved changes. Do you want to discard them?',
  keepEditingText: 'Keep editing',
  discardText: 'Discard changes'
})

const emit = defineEmits<{
  (event: 'close'): void
}>()

const dialogRef = ref<HTMLElement | null>(null)
const confirmationRef = ref<HTMLElement | null>(null)
const keepEditingButton = ref<HTMLButtonElement | null>(null)
const discardConfirmationOpen = ref(false)
const id = useId()
const dialogTitleId = `dialog-title-${id}`
const discardTitleId = `dialog-discard-title-${id}`
const discardMessageId = `dialog-discard-message-${id}`
let previousActiveElement: HTMLElement | null = null
let focusBeforeConfirmation: HTMLElement | null = null
let bodyLockHeld = false

const widthPixels: Record<DialogWidth, string> = {
  narrow: '400px',
  small: '400px',
  normal: '640px',
  medium: '640px',
  wide: '880px',
  large: '880px',
  'extra-wide': '880px',
  full: 'calc(100vw - 32px)'
}

const zIndexStyle = computed(() => ({ zIndex: props.zIndex }))
const panelStyle = computed(() => ({
  '--dialog-width': widthPixels[props.width],
  '--dialog-max-height': props.width === 'large' || props.width === 'wide' || props.width === 'extra-wide' || props.width === 'full'
    ? 'min(90vh, 900px)'
    : '85vh'
}))

const focusableSelector = [
  'a[href]',
  'area[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  'iframe',
  '[contenteditable="true"]',
  '[tabindex]:not([tabindex="-1"])'
].join(',')

const currentFocusScope = () => discardConfirmationOpen.value ? confirmationRef.value : dialogRef.value

const getFocusableElements = (scope: HTMLElement) =>
  Array.from(scope.querySelectorAll<HTMLElement>(focusableSelector)).filter((element) => {
    return !element.hasAttribute('hidden') && element.getAttribute('aria-hidden') !== 'true'
  })

const focusInitialElement = async () => {
  await nextTick()
  const scope = currentFocusScope()
  if (!scope) return
  const target = scope.querySelector<HTMLElement>(
    '[autofocus], input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled])'
  ) ?? getFocusableElements(scope)[0] ?? scope
  target.focus()
}

const requestClose = () => {
  if (discardConfirmationOpen.value) return
  if (props.hasUnsavedChanges) {
    focusBeforeConfirmation = document.activeElement instanceof HTMLElement ? document.activeElement : null
    discardConfirmationOpen.value = true
    void focusInitialElement()
    return
  }
  emit('close')
}

const handleOverlayClick = () => {
  if (!props.closeOnClickOutside) return
  requestClose()
}

const cancelDiscard = () => {
  discardConfirmationOpen.value = false
  void nextTick(() => {
    const target = focusBeforeConfirmation
    if (target?.isConnected) target.focus()
    else void focusInitialElement()
    focusBeforeConfirmation = null
  })
}

const confirmDiscard = () => {
  discardConfirmationOpen.value = false
  emit('close')
}

const handleKeydown = (event: KeyboardEvent) => {
  if (!props.show) return
  const scope = currentFocusScope()
  if (!scope) return

  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopImmediatePropagation()
    if (discardConfirmationOpen.value) cancelDiscard()
    else if (props.closeOnEscape) requestClose()
    return
  }

  if (event.key !== 'Tab') return
  const focusable = getFocusableElements(scope)
  if (!focusable.length) {
    event.preventDefault()
    scope.focus()
    return
  }
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  const activeElement = document.activeElement
  if (!scope.contains(activeElement)) {
    event.preventDefault()
    ;(event.shiftKey ? last : first).focus()
  } else if (event.shiftKey && activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen) {
      previousActiveElement = document.activeElement instanceof HTMLElement ? document.activeElement : null
      if (!bodyLockHeld && typeof document !== 'undefined') {
        document.body.classList.add('modal-open')
        bodyLockHeld = true
      }
      await focusInitialElement()
    } else {
      discardConfirmationOpen.value = false
      if (bodyLockHeld && typeof document !== 'undefined') {
        document.body.classList.remove('modal-open')
        bodyLockHeld = false
      }
      const target = previousActiveElement
      previousActiveElement = null
      if (target?.isConnected) target.focus()
    }
  },
  { immediate: true, flush: 'post' }
)

onMounted(() => document.addEventListener('keydown', handleKeydown))
onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
  if (bodyLockHeld) document.body.classList.remove('modal-open')
  const target = previousActiveElement
  previousActiveElement = null
  bodyLockHeld = false
  if (target?.isConnected) target.focus()
})
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: auto;
  box-sizing: border-box;
  padding: 16px;
  background: rgb(15 23 42 / 55%);
}

.modal-content {
  display: flex;
  flex: 0 1 auto;
  flex-direction: column;
  width: min(var(--dialog-width, 640px), calc(100vw - 32px));
  max-width: calc(100vw - 32px);
  max-height: min(var(--dialog-max-height, 85vh), calc(100dvh - 32px));
  min-height: 0;
  overflow: hidden;
  box-sizing: border-box;
  margin: auto;
}

.modal-header,
.modal-footer {
  flex: 0 0 auto;
}

.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.modal-title {
  min-width: 0;
  margin: 0;
}

.modal-close-button {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  min-width: 40px;
  min-height: 40px;
  border: 0;
  border-radius: 12px;
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.modal-close-button:focus-visible,
.modal-discard-actions button:focus-visible {
  outline: 3px solid #2563eb;
  outline-offset: 2px;
}

.modal-body {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
  overscroll-behavior: contain;
}

.modal-discard-overlay {
  position: fixed;
  z-index: 1;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: auto;
  box-sizing: border-box;
  padding: 16px;
  background: rgb(15 23 42 / 48%);
}

.modal-discard-dialog {
  --dialog-width: 400px;
  --dialog-max-height: 85vh;
}

.modal-discard-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 12px;
}

@media (prefers-reduced-motion: reduce) {
  .modal-content {
    scroll-behavior: auto;
  }
}
</style>
