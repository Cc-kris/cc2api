<template>
  <div class="flex flex-wrap items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400">
    <RouterLink
      v-for="item in items"
      :key="item.key"
      :to="item.to"
      class="rounded-full border px-3 py-1 transition-colors"
      :class="item.key === active
        ? 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-700/60 dark:bg-primary-900/10 dark:text-primary-200'
        : 'border-gray-200 text-gray-500 hover:border-primary-200 hover:text-primary-600 dark:border-dark-700 dark:text-gray-400 dark:hover:border-primary-700/60 dark:hover:text-primary-200'"
    >
      {{ item.label }}
    </RouterLink>
  </div>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'

type CacheNavKey = 'home' | 'stats' | 'clear' | 'advanced' | 'semantic' | 'semanticAudit'

defineProps<{
  active: CacheNavKey
}>()

const items: Array<{ key: CacheNavKey; to: string; label: string }> = [
  { key: 'home', to: '/admin/settings/cache', label: '缓存配置' },
  { key: 'stats', to: '/admin/settings/cache/stats', label: '缓存统计' },
  { key: 'clear', to: '/admin/settings/cache/clear', label: '缓存清理' },
  { key: 'advanced', to: '/admin/settings/cache/advanced', label: '高级策略' },
  { key: 'semantic', to: '/admin/settings/cache/semantic', label: '语义缓存' },
  { key: 'semanticAudit', to: '/admin/settings/cache/semantic-audits', label: '语义审计' }
]
</script>
