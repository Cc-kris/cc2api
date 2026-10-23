<template>
  <div class="relative" ref="dropdownRef">
    <button
      @click="toggleDropdown"
      :disabled="switching"
      class="neo-icon-btn flex items-center gap-1.5 rounded-xl border-2 border-[#121316] bg-white px-3 py-1.5 font-mono text-xs font-extrabold text-[#121316] shadow-[2px_2px_0_#121316] transition-all hover:-translate-x-0.5 hover:-translate-y-0.5 hover:bg-[#CCFF00] hover:text-[#121316] hover:shadow-[3.5px_3.5px_0_#121316] dark:border-[#E2E8F0] dark:bg-[#1E2433] dark:text-[#F8FAFC] dark:hover:bg-[#2563EB] dark:hover:text-white"
      :title="currentLocale?.name"
      :aria-label="t('common.changeLanguage')"
    >
      <span>{{ currentLocale?.code.toUpperCase() }}</span>
      <Icon
        name="chevronDown"
        size="xs"
        class="transition-transform duration-200"
        :class="{ 'rotate-180': isOpen }"
      />
    </button>

    <transition name="dropdown">
      <div
        v-if="isOpen"
        class="dropdown neo-dropdown absolute right-0 z-50 mt-2 w-40 overflow-hidden rounded-xl border-2 border-[#121316] bg-white p-1.5 shadow-[5px_5px_0_#121316] dark:border-[#E2E8F0] dark:bg-[#181C26]"
      >
        <button
          v-for="locale in availableLocales"
          :key="locale.code"
          :disabled="switching"
          @click="selectLocale(locale.code)"
          class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm font-bold transition-all"
          :class="
            locale.code === currentLocaleCode
              ? 'border-2 border-[#121316] bg-[#CCFF00] font-black text-[#121316] shadow-[2px_2px_0_#121316] dark:border-[#60A5FA] dark:bg-[#2563EB] dark:text-white'
              : 'text-[#121316] hover:bg-[#EDF4FF] hover:text-[#121316] dark:text-[#F8FAFC] dark:hover:bg-[#262D40] dark:hover:text-white'
          "
        >
          <span>{{ locale.name }}</span>
          <Icon v-if="locale.code === currentLocaleCode" name="check" size="sm" class="ml-auto text-[#121316]" />
        </button>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { setLocale, availableLocales } from '@/i18n'

const { locale, t } = useI18n()

const isOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const switching = ref(false)

const currentLocaleCode = computed(() => locale.value)
const currentLocale = computed(() => availableLocales.find((l) => l.code === locale.value))

function toggleDropdown() {
  isOpen.value = !isOpen.value
}

async function selectLocale(code: string) {
  if (switching.value || code === currentLocaleCode.value) {
    isOpen.value = false
    return
  }
  switching.value = true
  try {
    await setLocale(code)
    isOpen.value = false
  } finally {
    switching.value = false
  }
}

function handleClickOutside(event: MouseEvent) {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    isOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.15s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}
</style>
