<template>
  <div class="ccai-auth-shell relative flex min-h-screen items-center justify-center overflow-hidden p-4 bg-[#F5F2EB] dark:bg-dark-950">
    <div class="absolute right-6 top-6 z-20 flex items-center gap-3">
      <router-link
        to="/"
        class="inline-flex h-10 items-center px-3.5 rounded-xl bg-white text-xs font-black text-[#121316] border-2 border-[#121316] shadow-[3px_3px_0_#121316] hover:-translate-y-0.5 transition-transform"
      >
        ← CCAI Home
      </router-link>
      <LocaleSwitcher />
    </div>

    <!-- Neo-Lab Blueprint Grid Background -->
    <div class="pointer-events-none fixed inset-0 ccai-blueprint-grid"></div>

    <!-- Content Container -->
    <div class="relative z-10 w-full max-w-md">
      <!-- Logo/Brand -->
      <div class="mb-7 text-center">
        <router-link
          to="/"
          class="mb-4 inline-flex h-14 px-5 items-center justify-center rounded-2xl bg-[#2563EB] text-white font-mono text-2xl font-black tracking-tight border-[2.5px] border-[#121316] shadow-[5px_5px_0_#121316] transition-transform hover:-rotate-3"
        >
          CCAI
        </router-link>
        <h1 class="mb-1.5 text-3xl font-black tracking-tight text-[#121316] dark:text-white">
          CCAI
        </h1>
        <p class="text-sm font-semibold text-[#5E6577] dark:text-dark-400">
          {{ siteSubtitle }}
        </p>
      </div>

      <!-- Card Container -->
      <div class="card-glass rounded-2xl p-8">
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="mt-6 text-center text-sm font-semibold">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="mt-8 text-center font-mono text-xs font-bold text-[#5E6577] dark:text-dark-500">
        &copy; {{ currentYear }} CCAI. {{ t('common.allRightsReserved') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

const appStore = useAppStore()
const { t } = useI18n()

const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || t('auth.defaultSiteSubtitle'))

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.text-gradient {
  @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent;
}
</style>
