<template>
  <div class="relative" ref="containerRef">
    <button
      ref="triggerRef"
      type="button"
      :id="triggerId"
      @click="toggle"
      :disabled="disabled || loading"
      :aria-expanded="isOpen"
      :aria-haspopup="'listbox'"
      :aria-busy="loading ? 'true' : undefined"
      :role="isSearchable ? undefined : 'combobox'"
      :aria-label="ariaLabelledby ? undefined : triggerAccessibleLabel"
      :aria-labelledby="ariaLabelledby || undefined"
      :aria-controls="listboxId"
      :aria-activedescendant="!isSearchable ? activeDescendantId : undefined"
      :class="[
        'select-trigger',
        isOpen && 'select-trigger-open',
        error && 'select-trigger-error',
        disabled && 'select-trigger-disabled'
      ]"
      @keydown="onTriggerKeyDown"
    >
      <span class="select-value">
        <slot name="selected" :option="selectedOption" :options="selectedOptions">
          <template v-if="multiple && selectedOptions.length">
            <span class="select-multiple-summary">
              {{ selectedOptions.length }} selected
            </span>
          </template>
          <template v-else>{{ selectedLabel }}</template>
        </slot>
      </span>
      <span class="select-actions">
        <span v-if="loading" class="select-loading" role="status" aria-label="Loading"></span>
        <span v-else class="select-icon">
          <Icon
            name="chevronDown"
            size="md"
            :class="['transition-transform duration-200', isOpen && 'rotate-180']"
          />
        </span>
      </span>
    </button>
    <button
      v-if="clearable && hasValue && !disabled && !loading"
      type="button"
      class="select-clear select-clear-floating"
      aria-label="Clear selection"
      @click.stop="clearSelection"
    >
      <Icon name="x" size="sm" />
    </button>

    <!-- Teleport dropdown to body to escape stacking context -->
    <Teleport to="body">
      <Transition name="select-dropdown">
        <div
          v-if="isOpen"
          ref="dropdownRef"
          class="select-dropdown-portal"
          :class="[instanceId]"
          :style="dropdownStyle"
          @click.stop
          @mousedown.stop
          @keydown="onDropdownKeyDown"
        >
          <!-- Search input -->
          <div v-if="isSearchable" class="select-search">
            <Icon name="search" size="sm" class="text-gray-400" />
            <input
              :id="searchInputId"
              ref="searchInputRef"
              v-model="searchQuery"
              type="text"
              role="combobox"
              aria-autocomplete="list"
              :aria-label="searchPlaceholderText"
              :aria-expanded="isOpen"
              :aria-controls="listboxId"
              :aria-activedescendant="activeDescendantId"
              :placeholder="searchPlaceholderText"
              class="select-search-input"
              @click.stop
            />
          </div>

          <!-- Options list -->
          <div
            class="select-options"
            ref="optionsListRef"
            role="listbox"
            :id="listboxId"
            :aria-labelledby="isSearchable ? searchInputId : triggerId"
            :aria-multiselectable="multiple ? 'true' : undefined"
          >
            <div
              v-for="(option, index) in filteredOptions"
              :key="optionId(index)"
              :id="optionId(index)"
              :role="isGroupHeaderOption(option) ? 'presentation' : 'option'"
              :aria-selected="isGroupHeaderOption(option) ? undefined : isSelected(option)"
              :aria-disabled="isGroupHeaderOption(option) ? undefined : isOptionDisabled(option)"
              :aria-current="focusedIndex === index && !isGroupHeaderOption(option) ? 'true' : undefined"
              @click.stop="!isOptionDisabled(option) && !isGroupHeaderOption(option) && selectOption(option)"
              @mouseenter="handleOptionMouseEnter(option, index)"
              :class="[
                'select-option',
                isGroupHeaderOption(option) && 'select-option-group',
                isSelected(option) && 'select-option-selected',
                isOptionDisabled(option) && !isGroupHeaderOption(option) && 'select-option-disabled',
                focusedIndex === index && !isGroupHeaderOption(option) && 'select-option-focused'
              ]"
            >
              <slot name="option" :option="option" :selected="isSelected(option)">
                <Icon
                  v-if="option._creatable"
                  name="search"
                  size="sm"
                  class="flex-shrink-0 text-gray-400"
                />
                <span class="select-option-label" :class="option._creatable && 'italic text-gray-500 dark:text-dark-300'">{{ getOptionLabel(option) }}</span>
                <Icon
                  v-if="isSelected(option)"
                  name="check"
                  size="sm"
                  class="text-primary-500"
                  :stroke-width="2"
                />
              </slot>
            </div>

            <!-- Empty state -->
            <div v-if="loading" class="select-empty" role="status">
              Loading…
            </div>
            <div v-else-if="filteredOptions.length === 0" class="select-empty">
              {{ emptyTextDisplay }}
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

// Instance ID for unique click-outside detection
const instanceId = `select-${Math.random().toString(36).substring(2, 9)}`

export interface SelectOption {
  value: string | number | boolean | null
  label: string
  disabled?: boolean
  [key: string]: unknown
}

interface Props {
  modelValue: string | number | boolean | null | undefined | Array<string | number | boolean | null>
  options: SelectOption[] | Array<Record<string, unknown>>
  placeholder?: string
  disabled?: boolean
  error?: boolean
  searchable?: boolean | 'auto'
  searchPlaceholder?: string
  emptyText?: string
  valueKey?: string
  labelKey?: string
  creatable?: boolean
  creatablePrefix?: string
  multiple?: boolean
  clearable?: boolean
  loading?: boolean
  ariaLabel?: string
  ariaLabelledby?: string
}

type SelectValue = string | number | boolean | null

interface Emits {
  (e: 'update:modelValue', value: SelectValue): void
  (e: 'change', value: SelectValue, option: SelectOption | null): void
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  error: false,
  searchable: 'auto',
  creatable: false,
  creatablePrefix: '',
  valueKey: 'value',
  labelKey: 'label'
})

const emit = defineEmits<Emits>()

const isOpen = ref(false)
const searchQuery = ref('')
const focusedIndex = ref(-1)
const containerRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const searchInputRef = ref<HTMLInputElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const optionsListRef = ref<HTMLElement | null>(null)
const dropdownPosition = ref<'bottom' | 'top'>('bottom')
const triggerRect = ref<DOMRect | null>(null)

// i18n placeholders
const placeholderText = computed(() => props.placeholder ?? t('common.selectOption'))
const searchPlaceholderText = computed(() => props.searchPlaceholder ?? t('common.searchPlaceholder'))
const emptyTextDisplay = computed(() => props.emptyText ?? t('common.noOptionsFound'))

const triggerAccessibleLabel = computed(() => props.ariaLabel ?? t('common.selectOption'))
const triggerId = `${instanceId}-trigger`
const listboxId = `${instanceId}-listbox`
const optionId = (index: number) => `${instanceId}-option-${index}`
const activeDescendantId = computed(() => {
  const option = filteredOptions.value[focusedIndex.value]
  return option && !isOptionDisabled(option) && !isGroupHeaderOption(option)
    ? optionId(focusedIndex.value)
    : undefined
})

const isSearchable = computed(() => {
  if (props.searchable === 'auto') return props.options.length > 5
  return props.searchable
})

// Computed style for teleported dropdown
const dropdownStyle = computed(() => {
  if (!triggerRect.value) return {}

  const rect = triggerRect.value
  const style: Record<string, string> = {
    position: 'fixed',
    left: `${rect.left}px`,
    minWidth: `${rect.width}px`,
    zIndex: '100000020'
  }

  if (dropdownPosition.value === 'top') {
    style.bottom = `${window.innerHeight - rect.top + 4}px`
  } else {
    style.top = `${rect.bottom + 4}px`
  }

  return style
})

const getOptionValue = (option: any): any => {
  if (typeof option === 'object' && option !== null) {
    return option[props.valueKey]
  }
  return option
}

const getOptionLabel = (option: any): string => {
  if (typeof option === 'object' && option !== null) {
    return String(option[props.labelKey] ?? '')
  }
  return String(option ?? '')
}

const isOptionDisabled = (option: any): boolean => {
  if (typeof option === 'object' && option !== null) {
    return !!option.disabled
  }
  return false
}

const isGroupHeaderOption = (option: any): boolean => {
  if (typeof option === 'object' && option !== null) {
    return option.kind === 'group'
  }
  return false
}

const selectedOption = computed(() => {
  if (props.multiple || Array.isArray(props.modelValue)) return null
  return props.options.find((opt) => getOptionValue(opt) === props.modelValue) || null
})

const selectedOptions = computed(() => {
  const values = Array.isArray(props.modelValue) ? props.modelValue : []
  return props.options.filter((option) => values.includes(getOptionValue(option)))
})

const hasValue = computed(() => Array.isArray(props.modelValue)
  ? props.modelValue.length > 0
  : props.modelValue !== null && props.modelValue !== undefined && props.modelValue !== '')

const selectedLabel = computed(() => {
  if (selectedOption.value) {
    return getOptionLabel(selectedOption.value)
  }
  // In creatable mode, show the raw value if no matching option
  if (!props.multiple && props.creatable && props.modelValue) {
    return String(props.modelValue)
  }
  return placeholderText.value
})

const filteredOptions = computed(() => {
  let opts = props.options as any[]
  if (isSearchable.value && searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    opts = opts.filter((opt) => {
      // Match label
      if (getOptionLabel(opt).toLowerCase().includes(query)) return true
      // Also match description if present
      if (opt.description && String(opt.description).toLowerCase().includes(query)) return true
      return false
    })
    // In creatable mode, always prepend a fuzzy search option
    if (props.creatable && searchQuery.value.trim()) {
      const trimmed = searchQuery.value.trim()
      const prefix = props.creatablePrefix || t('common.search')
      opts = [{ [props.valueKey]: trimmed, [props.labelKey]: `${prefix} "${trimmed}"`, _creatable: true }, ...opts]
    }
  }
  return opts
})

const isSelected = (option: any): boolean => {
  const value = getOptionValue(option)
  return Array.isArray(props.modelValue)
    ? props.modelValue.some((selected) => selected === value)
    : value === props.modelValue
}

const findNextEnabledIndex = (startIndex: number): number => {
  const opts = filteredOptions.value
  if (opts.length === 0) return -1
  for (let offset = 0; offset < opts.length; offset++) {
    const idx = (startIndex + offset) % opts.length
    if (!isOptionDisabled(opts[idx]) && !isGroupHeaderOption(opts[idx])) return idx
  }
  return -1
}

const findPrevEnabledIndex = (startIndex: number): number => {
  const opts = filteredOptions.value
  if (opts.length === 0) return -1
  for (let offset = 0; offset < opts.length; offset++) {
    const idx = (startIndex - offset + opts.length) % opts.length
    if (!isOptionDisabled(opts[idx]) && !isGroupHeaderOption(opts[idx])) return idx
  }
  return -1
}

const handleOptionMouseEnter = (option: any, index: number) => {
  if (isOptionDisabled(option) || isGroupHeaderOption(option)) return
  focusedIndex.value = index
}

// Update trigger rect periodically while open to follow scroll/resize
const updateTriggerRect = () => {
  if (containerRef.value) {
    triggerRect.value = containerRef.value.getBoundingClientRect()
  }
}

const calculateDropdownPosition = () => {
  if (!containerRef.value) return
  updateTriggerRect()

  nextTick(() => {
    if (!dropdownRef.value || !triggerRect.value) return
    const dropdownHeight = dropdownRef.value.offsetHeight || 240
    const spaceBelow = window.innerHeight - triggerRect.value.bottom
    const spaceAbove = triggerRect.value.top

    if (spaceBelow < dropdownHeight && spaceAbove > dropdownHeight) {
      dropdownPosition.value = 'top'
    } else {
      dropdownPosition.value = 'bottom'
    }
  })
}

const toggle = () => {
  if (props.disabled) return
  isOpen.value = !isOpen.value
}

watch(isOpen, (open) => {
  if (open) {
    calculateDropdownPosition()
    // Reset focused index to current selection or first item
    if (filteredOptions.value.length === 0) {
      focusedIndex.value = -1
    } else {
      const selectedIdx = filteredOptions.value.findIndex(isSelected)
      if (selectedIdx >= 0 && !isOptionDisabled(filteredOptions.value[selectedIdx]) && !isGroupHeaderOption(filteredOptions.value[selectedIdx])) {
        focusedIndex.value = selectedIdx
      } else {
        focusedIndex.value = findNextEnabledIndex(selectedIdx >= 0 ? selectedIdx + 1 : 0)
      }
    }

    if (isSearchable.value) {
      nextTick(() => searchInputRef.value?.focus())
    }
    // Add scroll listener to update position
    window.addEventListener('scroll', updateTriggerRect, { capture: true, passive: true })
    window.addEventListener('resize', calculateDropdownPosition)
  } else {
    searchQuery.value = ''
    focusedIndex.value = -1
    window.removeEventListener('scroll', updateTriggerRect, { capture: true })
    window.removeEventListener('resize', calculateDropdownPosition)
  }
})

const selectOption = (option: any) => {
  const value = getOptionValue(option) ?? null
  if (props.multiple) {
    const currentValues = Array.isArray(props.modelValue) ? [...props.modelValue] : []
    const selectedIndex = currentValues.findIndex((selected) => selected === value)
    if (selectedIndex >= 0) currentValues.splice(selectedIndex, 1)
    else currentValues.push(value)
    emit('update:modelValue', currentValues as unknown as SelectValue)
    emit('change', currentValues as unknown as SelectValue, option)
    return
  }
  emit('update:modelValue', value)
  emit('change', value, option)
  isOpen.value = false
  triggerRef.value?.focus()
}

const clearSelection = () => {
  const value: SelectValue = props.multiple ? ([] as unknown as SelectValue) : null
  emit('update:modelValue', value)
  emit('change', value, null)
}

// Keyboards
const onTriggerKeyDown = (event: KeyboardEvent) => {
  if (isSearchable.value) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp' || event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      if (!isOpen.value) isOpen.value = true
    }
    return
  }

  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      if (!isOpen.value) isOpen.value = true
      else {
        focusedIndex.value = findNextEnabledIndex(focusedIndex.value + 1)
        if (focusedIndex.value >= 0) scrollToFocused()
      }
      break
    case 'ArrowUp':
      event.preventDefault()
      if (!isOpen.value) isOpen.value = true
      else {
        focusedIndex.value = findPrevEnabledIndex(focusedIndex.value - 1)
        if (focusedIndex.value >= 0) scrollToFocused()
      }
      break
    case 'Enter':
    case ' ': {
      event.preventDefault()
      if (!isOpen.value) {
        isOpen.value = true
      } else if (focusedIndex.value >= 0 && focusedIndex.value < filteredOptions.value.length) {
        const option = filteredOptions.value[focusedIndex.value]
        if (!isOptionDisabled(option) && !isGroupHeaderOption(option)) selectOption(option)
      }
      break
    }
    case 'Escape':
      if (isOpen.value) {
        event.preventDefault()
        isOpen.value = false
        triggerRef.value?.focus()
      }
      break
    case 'Tab':
      if (isOpen.value) isOpen.value = false
      break
  }
}

const onDropdownKeyDown = (event: KeyboardEvent) => {
  if (!isSearchable.value) return
  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      focusedIndex.value = findNextEnabledIndex(focusedIndex.value + 1)
      if (focusedIndex.value >= 0) scrollToFocused()
      break
    case 'ArrowUp':
      event.preventDefault()
      focusedIndex.value = findPrevEnabledIndex(focusedIndex.value - 1)
      if (focusedIndex.value >= 0) scrollToFocused()
      break
    case 'Enter':
      event.preventDefault()
      if (focusedIndex.value >= 0 && focusedIndex.value < filteredOptions.value.length) {
        const opt = filteredOptions.value[focusedIndex.value]
        if (!isOptionDisabled(opt) && !isGroupHeaderOption(opt)) selectOption(opt)
      }
      break
    case 'Escape':
      event.preventDefault()
      isOpen.value = false
      triggerRef.value?.focus()
      break
    case 'Tab':
      isOpen.value = false
      break
  }
}

const scrollToFocused = () => {
  nextTick(() => {
    const list = optionsListRef.value
    if (!list) return
    const focusedEl = list.children[focusedIndex.value] as HTMLElement
    if (!focusedEl) return

    if (focusedEl.offsetTop < list.scrollTop) {
      list.scrollTop = focusedEl.offsetTop
    } else if (focusedEl.offsetTop + focusedEl.offsetHeight > list.scrollTop + list.offsetHeight) {
      list.scrollTop = focusedEl.offsetTop + focusedEl.offsetHeight - list.offsetHeight
    }
  })
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  // Check if click is inside THIS specific instance's dropdown or trigger
  const isInDropdown = !!target.closest(`.${instanceId}`)
  const isInTrigger = containerRef.value?.contains(target)

  if (!isInDropdown && !isInTrigger && isOpen.value) {
    isOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  window.removeEventListener('scroll', updateTriggerRect, { capture: true })
  window.removeEventListener('resize', calculateDropdownPosition)
})
</script>

<style scoped>
.select-trigger {
  @apply relative w-full h-9 min-h-9 flex items-center justify-between gap-2 rounded-lg;
  @apply px-3.5 py-2.5 rounded-xl text-sm font-bold;
  background-color: #FFFFFF;
  border: 2px solid #121316;
  color: #121316;
  box-shadow: 2.5px 2.5px 0px #121316;
  @apply transition-all duration-150;
  @apply focus:outline-none;
  @apply cursor-pointer;
}
:global(.dark) .select-trigger {
  background-color: #181C26;
  border-color: #E2E8F0;
  color: #F8FAFC;
  box-shadow: 2.5px 2.5px 0px #000000;
}

.select-trigger:hover:not(:disabled) {
  transform: translate(-1px, -1px);
  box-shadow: 3.5px 3.5px 0px #121316;
  background-color: #F8FAFF;
}
:global(.dark) .select-trigger:hover:not(:disabled) {
  background-color: #242B3D;
  box-shadow: 3.5px 3.5px 0px #000000;
}

.select-trigger-open {
  border-color: #2563EB !important;
  box-shadow: 3.5px 3.5px 0px #2563EB !important;
}

.select-trigger-error {
  border-color: #FF3B30 !important;
  box-shadow: 3px 3px 0px #FF3B30 !important;
}

.select-trigger-disabled {
  @apply opacity-55 cursor-not-allowed;
  background-color: #F1F5F9;
}
:global(.dark) .select-trigger-disabled {
  background-color: #0F172A;
}

.select-value {
  @apply truncate text-left flex-1 font-bold;
  color: inherit;
}

.select-actions {
  @apply flex flex-shrink-0 items-center gap-1;
}

.select-clear {
  @apply inline-flex h-5 w-5 items-center justify-center rounded-full cursor-pointer;
  color: #5E6577;
}
.select-clear:hover,
.select-clear:focus-visible {
  color: #121316;
  background-color: #E2E8F0;
  outline: 2px solid #2563EB;
  outline-offset: 1px;
}
:global(.dark) .select-clear {
  color: #CBD5E1;
}
:global(.dark) .select-clear:hover,
:global(.dark) .select-clear:focus-visible {
  color: #F8FAFC;
  background-color: #334155;
}

.select-clear-floating {
  position: absolute;
  right: 2.25rem;
  top: 50%;
  transform: translateY(-50%);
  z-index: 1;
}

.select-icon {
  @apply flex-shrink-0 flex items-center;
  color: #121316;
}
:global(.dark) .select-icon {
  color: #F8FAFC;
}

.select-dropdown-portal {
  @apply w-full rounded-xl overflow-hidden;
  background-color: #FFFFFF;
  border: 2.5px solid #121316;
  box-shadow: 6px 6px 0px #121316;
}
:global(.dark) .select-dropdown-portal {
  background-color: #181C26;
  border-color: #E2E8F0;
  box-shadow: 6px 6px 0px #000000;
}

.select-search {
  @apply flex items-center gap-2 px-3 py-2.5;
  border-bottom: 2px solid #121316;
  background-color: #F4F7FF;
}
:global(.dark) .select-search {
  border-bottom-color: #475569;
  background-color: #111520;
}

.select-search-input {
  @apply w-full bg-transparent text-sm font-bold;
  color: #121316;
  @apply placeholder-gray-500;
  @apply focus:outline-none;
}
:global(.dark) .select-search-input {
  color: #F8FAFC;
}

.select-options {
  @apply max-h-60 overflow-y-auto p-1.5;
}

.select-group-header {
  @apply px-3 py-1.5 text-[11px] font-black tracking-wider uppercase;
  font-family: 'JetBrains Mono', monospace;
  color: #5E6577;
}
:global(.dark) .select-group-header {
  color: #94A3B8;
}

.select-option {
  @apply flex items-center justify-between gap-2 px-3 py-2.5 cursor-pointer rounded-lg my-0.5;
  @apply text-sm font-bold;
  color: #121316;
  border: 1.5px solid transparent;
  @apply transition-all duration-100;
}
:global(.dark) .select-option {
  color: #F8FAFC;
}

.select-option:hover:not(.select-option-disabled):not(.select-option-selected),
.select-option-focused:not(.select-option-disabled):not(.select-option-selected) {
  background-color: #EDF4FF;
  color: #121316;
  border-color: #121316;
}
:global(.dark) .select-option:hover:not(.select-option-disabled):not(.select-option-selected),
:global(.dark) .select-option-focused:not(.select-option-disabled):not(.select-option-selected) {
  background-color: #262E42;
  color: #FFFFFF;
  border-color: #94A3B8;
}

/* Selected options use the product's primary blue with high-contrast white text in both themes. */
.select-option-selected {
  background-color: #2563EB !important;
  color: #FFFFFF !important;
  border: 2px solid #1D4ED8 !important;
  box-shadow: 2px 2px 0px #121316 !important;
  font-weight: 900 !important;
}
.select-option-selected * {
  color: #FFFFFF !important;
}

.select-option-disabled {
  @apply opacity-45 cursor-not-allowed;
}

.select-option-label {
  @apply truncate;
}

.select-empty {
  @apply px-4 py-8 text-center text-sm font-bold;
  color: #5E6577;
}
</style>

<style scoped>
:global(.dark) .select-option-selected {
  background-color: #2563EB !important;
  color: #FFFFFF !important;
  border-color: #1D4ED8 !important;
}
:global(.dark) .select-option-selected * {
  color: #FFFFFF !important;
}
</style>
