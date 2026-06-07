<template>
  <div class="space-y-6">
    <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
      <div class="flex items-start justify-between gap-4">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
            {{ t('admin.cacheManagement.title') }}
          </h1>
          <p class="mt-2 max-w-3xl text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.cacheManagement.description') }}
          </p>
        </div>
      </div>
    </div>

    <div class="rounded-xl border border-dashed border-gray-300 bg-gray-50 p-6 dark:border-dark-600 dark:bg-dark-800/50">
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">
        {{ t('admin.cacheManagement.shellTitle') }}
      </h2>
      <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
        {{ t('admin.cacheManagement.shellDescription') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>
