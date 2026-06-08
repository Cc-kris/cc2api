<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
              {{ t('admin.cacheManagement.title') }}
            </h1>
            <p class="mt-2 max-w-3xl text-sm text-gray-600 dark:text-gray-400">
              {{ t('admin.cacheManagement.description') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary" :disabled="loading || saving" @click="loadConfig(true)">
              {{ t('admin.cacheManagement.refresh') }}
            </button>
            <button type="button" class="btn btn-secondary" :disabled="saving || !dirty" @click="resetToLoaded">
              {{ t('admin.cacheManagement.resetChanges') }}
            </button>
            <button
              type="button"
              class="btn btn-primary"
              :disabled="saving || loading || !canManage || validationErrors.length > 0 || !dirty"
              @click="saveConfig"
            >
              {{ saving ? t('admin.cacheManagement.saving') : t('admin.cacheManagement.save') }}
            </button>
          </div>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-3 xl:grid-cols-5">
        <div
          v-for="section in sections"
          :key="section.key"
          class="rounded-xl border p-4 shadow-sm transition-colors"
          :class="section.active
            ? 'border-primary-200 bg-primary-50/70 dark:border-primary-700/60 dark:bg-primary-900/10'
            : 'border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800'"
        >
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ section.title }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ section.description }}</p>
            </div>
            <span
              class="inline-flex rounded-full px-2.5 py-1 text-xs font-medium"
              :class="section.active
                ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-200'
                : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300'"
            >
              {{ section.badge }}
            </span>
          </div>
        </div>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <template v-else>
        <div v-if="loadError" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <span>{{ loadError }}</span>
            <button type="button" class="btn btn-secondary" @click="loadConfig(true)">
              {{ t('admin.cacheManagement.retry') }}
            </button>
          </div>
        </div>

        <div
          v-if="!canManage"
          class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/10 dark:text-amber-200"
        >
          {{ t('admin.cacheManagement.readonlyNotice') }}
        </div>

        <div
          v-if="validationErrors.length > 0"
          class="rounded-xl border border-red-200 bg-red-50 px-4 py-4 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200"
        >
          <p class="font-medium">{{ t('admin.cacheManagement.validationTitle') }}</p>
          <ul class="mt-2 list-disc space-y-1 pl-5">
            <li v-for="item in validationErrors" :key="item">{{ item }}</li>
          </ul>
        </div>

        <div class="grid grid-cols-1 gap-4 xl:grid-cols-4">
          <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.summary.global') }}</p>
            <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">
              {{ form.global_enabled ? t('common.enabled') : t('common.disabled') }}
            </p>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.summary.ttl') }}</p>
            <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ form.ttl_seconds }}s</p>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.summary.requestLimit') }}</p>
            <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ formatBytes(form.max_request_bytes) }}</p>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.summary.responseLimit') }}</p>
            <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ formatBytes(form.max_response_bytes) }}</p>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,0.8fr)]">
          <div class="space-y-6">
            <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.cacheManagement.sections.switches') }}
                </h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.cacheManagement.switchesHint') }}
                </p>
              </div>
              <div class="space-y-4 px-6 py-5">
                <div class="flex items-center justify-between gap-4">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.cacheManagement.fields.globalEnabled.label') }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.fields.globalEnabled.hint') }}</p>
                  </div>
                  <Toggle v-model="form.global_enabled" :disabled="!canManage" />
                </div>
                <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
                  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                    <div class="flex items-center justify-between gap-4">
                      <div>
                        <p class="text-sm font-medium text-gray-900 dark:text-white">OpenAI</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.fields.platformHint') }}</p>
                      </div>
                      <Toggle v-model="form.platforms.openai.enabled" :disabled="!canManage" />
                    </div>
                  </div>
                  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                    <div class="flex items-center justify-between gap-4">
                      <div>
                        <p class="text-sm font-medium text-gray-900 dark:text-white">Claude</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.fields.platformHint') }}</p>
                      </div>
                      <Toggle v-model="form.platforms.claude.enabled" :disabled="!canManage" />
                    </div>
                  </div>
                  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                    <div class="flex items-center justify-between gap-4">
                      <div>
                        <p class="text-sm font-medium text-gray-900 dark:text-white">Gemini</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.fields.platformHint') }}</p>
                      </div>
                      <Toggle v-model="form.platforms.gemini.enabled" :disabled="!canManage" />
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.cacheManagement.sections.limits') }}
                </h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.cacheManagement.limitsHint') }}
                </p>
              </div>
              <div class="grid grid-cols-1 gap-5 px-6 py-5 md:grid-cols-2">
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.fields.ttl.label') }}</span>
                  <input v-model.number="form.ttl_seconds" type="number" min="60" max="86400" step="1" class="input" :disabled="!canManage" />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.fields.ttl.hint') }}</p>
                </label>
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.fields.temperature.label') }}</span>
                  <input v-model.number="form.max_temperature" type="number" min="0" max="2" step="0.1" class="input" :disabled="!canManage" />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.fields.temperature.hint') }}</p>
                </label>
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.fields.maxRequestBytes.label') }}</span>
                  <input v-model.number="form.max_request_bytes" type="number" min="1024" max="5242880" step="1024" class="input" :disabled="!canManage" />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.fields.maxRequestBytes.hint', { value: formatBytes(form.max_request_bytes) }) }}</p>
                </label>
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.fields.maxResponseBytes.label') }}</span>
                  <input v-model.number="form.max_response_bytes" type="number" min="1024" max="10485760" step="1024" class="input" :disabled="!canManage" />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.fields.maxResponseBytes.hint', { value: formatBytes(form.max_response_bytes) }) }}</p>
                </label>
              </div>
            </div>

            <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.cacheManagement.sections.models') }}
                </h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.cacheManagement.modelsHint') }}
                </p>
              </div>
              <div class="grid grid-cols-1 gap-5 px-6 py-5 lg:grid-cols-2">
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.fields.allowlist.label') }}</span>
                  <textarea
                    v-model.trim="allowlistText"
                    rows="8"
                    class="input min-h-[180px] font-mono text-sm"
                    :disabled="!canManage"
                    :placeholder="t('admin.cacheManagement.fields.allowlist.placeholder')"
                  />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.fields.allowlist.hint') }}</p>
                </label>
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.fields.blocklist.label') }}</span>
                  <textarea
                    v-model.trim="blocklistText"
                    rows="8"
                    class="input min-h-[180px] font-mono text-sm"
                    :disabled="!canManage"
                    :placeholder="t('admin.cacheManagement.fields.blocklist.placeholder')"
                  />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.fields.blocklist.hint') }}</p>
                </label>
              </div>
            </div>
          </div>

          <div class="space-y-6">
            <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.cacheManagement.sections.scope') }}
                </h2>
              </div>
              <div class="space-y-4 px-6 py-5 text-sm text-gray-600 dark:text-gray-300">
                <div>
                  <p class="font-medium text-gray-900 dark:text-white">{{ t('admin.cacheManagement.scopeTitle') }}</p>
                  <p class="mt-1">{{ t('admin.cacheManagement.scopeValue') }}</p>
                </div>
                <div>
                  <p class="font-medium text-gray-900 dark:text-white">{{ t('admin.cacheManagement.bypassHeaderTitle') }}</p>
                  <div class="mt-2 rounded-lg bg-gray-50 px-3 py-2 font-mono text-xs text-gray-700 dark:bg-dark-900/50 dark:text-gray-200">
                    {{ bypassHeaderValue }}
                  </div>
                </div>
                <div class="rounded-lg border border-blue-100 bg-blue-50 px-4 py-3 text-xs text-blue-700 dark:border-blue-900/40 dark:bg-blue-900/10 dark:text-blue-200">
                  {{ t('admin.cacheManagement.scopeHint') }}
                </div>
              </div>
            </div>

            <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.cacheManagement.sections.defaults') }}
                </h2>
              </div>
              <div class="space-y-4 px-6 py-5 text-sm text-gray-600 dark:text-gray-300">
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
                    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.defaults.ttl') }}</p>
                    <p class="mt-1 font-semibold text-gray-900 dark:text-white">600s</p>
                  </div>
                  <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
                    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.defaults.temperature') }}</p>
                    <p class="mt-1 font-semibold text-gray-900 dark:text-white">0.3</p>
                  </div>
                  <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
                    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.defaults.requestLimit') }}</p>
                    <p class="mt-1 font-semibold text-gray-900 dark:text-white">256 KB</p>
                  </div>
                  <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
                    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.defaults.responseLimit') }}</p>
                    <p class="mt-1 font-semibold text-gray-900 dark:text-white">512 KB</p>
                  </div>
                </div>
                <button type="button" class="btn btn-secondary w-full" :disabled="!canManage || saving" @click="resetToDefaults">
                  {{ t('admin.cacheManagement.resetDefaultValues') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, reactive, ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Toggle from '@/components/common/Toggle.vue'
import { adminAPI } from '@/api/admin'
import { defaultCacheManagementConfig, type CacheManagementConfig } from '@/api/admin/cache'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const allowlistText = ref('')
const blocklistText = ref('')
const lastSavedSnapshot = ref('')

const form = reactive<CacheManagementConfig>(defaultCacheManagementConfig())

const viewerRole = computed(() => String((authStore.user as { role?: string } | null)?.role || '').trim().toLowerCase())
const canManage = computed(() => viewerRole.value === '' || viewerRole.value === 'admin')
const bypassHeaderValue = computed(() => `${form.bypass_header.name}: ${form.bypass_header.value}`)

const sections = computed(() => [
  {
    key: 'config',
    title: t('admin.cacheManagement.sectionCards.configTitle'),
    description: t('admin.cacheManagement.sectionCards.configDescription'),
    badge: t('admin.cacheManagement.sectionCards.active'),
    active: true
  },
  {
    key: 'stats',
    title: t('admin.cacheManagement.sectionCards.statsTitle'),
    description: t('admin.cacheManagement.sectionCards.statsDescription'),
    badge: t('admin.cacheManagement.sectionCards.pending'),
    active: false
  },
  {
    key: 'maintenance',
    title: t('admin.cacheManagement.sectionCards.maintenanceTitle'),
    description: t('admin.cacheManagement.sectionCards.maintenanceDescription'),
    badge: t('admin.cacheManagement.sectionCards.pending'),
    active: false
  },
  {
    key: 'advanced',
    title: t('admin.cacheManagement.sectionCards.advancedTitle'),
    description: t('admin.cacheManagement.sectionCards.advancedDescription'),
    badge: t('admin.cacheManagement.sectionCards.pending'),
    active: false
  },
  {
    key: 'semantic',
    title: t('admin.cacheManagement.sectionCards.semanticTitle'),
    description: t('admin.cacheManagement.sectionCards.semanticDescription'),
    badge: t('admin.cacheManagement.sectionCards.pending'),
    active: false
  }
])

const validationErrors = computed(() => {
  const errors: string[] = []
  const payload = buildPayload()

  if (!Number.isFinite(payload.ttl_seconds) || payload.ttl_seconds < 60 || payload.ttl_seconds > 86400) {
    errors.push(t('admin.cacheManagement.validation.ttl'))
  }
  if (!Number.isFinite(payload.max_request_bytes) || payload.max_request_bytes < 1024 || payload.max_request_bytes > 5 * 1024 * 1024) {
    errors.push(t('admin.cacheManagement.validation.maxRequestBytes'))
  }
  if (!Number.isFinite(payload.max_response_bytes) || payload.max_response_bytes < 1024 || payload.max_response_bytes > 10 * 1024 * 1024) {
    errors.push(t('admin.cacheManagement.validation.maxResponseBytes'))
  }
  if (!Number.isFinite(payload.max_temperature) || payload.max_temperature < 0 || payload.max_temperature > 2) {
    errors.push(t('admin.cacheManagement.validation.maxTemperature'))
  }

  const allow = new Set(payload.model_allowlist.map((item) => item.toLowerCase()))
  const overlap = payload.model_blocklist.find((item) => allow.has(item.toLowerCase()))
  if (overlap) {
    errors.push(t('admin.cacheManagement.validation.modelOverlap', { model: overlap }))
  }

  return errors
})

const dirty = computed(() => serializeConfig(buildPayload()) !== lastSavedSnapshot.value)

function formatModelList(items: string[]): string {
  return items.join('\n')
}

function parseModelList(text: string): string[] {
  const seen = new Set<string>()
  return text
    .split(/[\n,]/g)
    .map((item) => item.trim())
    .filter((item) => {
      if (!item) return false
      const key = item.toLowerCase()
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
}

function serializeConfig(config: CacheManagementConfig): string {
  return JSON.stringify(config)
}

function cloneConfig(config: CacheManagementConfig): CacheManagementConfig {
  return JSON.parse(JSON.stringify(config)) as CacheManagementConfig
}

function applyConfig(config: CacheManagementConfig): void {
  const next = cloneConfig(config)
  form.global_enabled = next.global_enabled
  form.platforms.openai.enabled = next.platforms.openai.enabled
  form.platforms.claude.enabled = next.platforms.claude.enabled
  form.platforms.gemini.enabled = next.platforms.gemini.enabled
  form.ttl_seconds = next.ttl_seconds
  form.max_request_bytes = next.max_request_bytes
  form.max_response_bytes = next.max_response_bytes
  form.max_temperature = next.max_temperature
  form.model_allowlist = [...next.model_allowlist]
  form.model_blocklist = [...next.model_blocklist]
  form.bypass_header.name = next.bypass_header.name
  form.bypass_header.value = next.bypass_header.value
  allowlistText.value = formatModelList(next.model_allowlist)
  blocklistText.value = formatModelList(next.model_blocklist)
}

function buildPayload(): CacheManagementConfig {
  return {
    global_enabled: Boolean(form.global_enabled),
    platforms: {
      openai: { enabled: Boolean(form.platforms.openai.enabled) },
      claude: { enabled: Boolean(form.platforms.claude.enabled) },
      gemini: { enabled: Boolean(form.platforms.gemini.enabled) }
    },
    ttl_seconds: Number(form.ttl_seconds),
    max_request_bytes: Number(form.max_request_bytes),
    max_response_bytes: Number(form.max_response_bytes),
    max_temperature: Number(form.max_temperature),
    model_allowlist: parseModelList(allowlistText.value),
    model_blocklist: parseModelList(blocklistText.value),
    bypass_header: {
      name: form.bypass_header.name,
      value: form.bypass_header.value
    }
  }
}

function rememberSaved(config: CacheManagementConfig): void {
  lastSavedSnapshot.value = serializeConfig(config)
}

function resetToDefaults(): void {
  applyConfig(defaultCacheManagementConfig())
}

function resetToLoaded(): void {
  applyConfig(JSON.parse(lastSavedSnapshot.value) as CacheManagementConfig)
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B'
  }
  if (bytes >= 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(bytes % (1024 * 1024) === 0 ? 0 : 1)} MB`
  }
  if (bytes >= 1024) {
    return `${(bytes / 1024).toFixed(bytes % 1024 === 0 ? 0 : 1)} KB`
  }
  return `${bytes} B`
}

async function loadConfig(force = false): Promise<void> {
  if (!force && !loading.value && lastSavedSnapshot.value) {
    return
  }
  loading.value = true
  loadError.value = ''
  try {
    const { data } = await adminAPI.cache.getConfig()
    const merged = {
      ...defaultCacheManagementConfig(),
      ...cloneConfig(data || defaultCacheManagementConfig()),
      platforms: {
        ...defaultCacheManagementConfig().platforms,
        ...(data?.platforms || {})
      },
      bypass_header: {
        ...defaultCacheManagementConfig().bypass_header,
        ...(data?.bypass_header || {})
      },
      model_allowlist: Array.isArray(data?.model_allowlist) ? data.model_allowlist : [],
      model_blocklist: Array.isArray(data?.model_blocklist) ? data.model_blocklist : []
    } satisfies CacheManagementConfig
    applyConfig(merged)
    rememberSaved(buildPayload())
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, t('admin.cacheManagement.loadFailed'))
    appStore.showError(loadError.value)
  } finally {
    loading.value = false
  }
}

async function saveConfig(): Promise<void> {
  if (!canManage.value) {
    appStore.showError(t('admin.cacheManagement.readonlyNotice'))
    return
  }
  if (validationErrors.value.length > 0) {
    appStore.showError(validationErrors.value[0])
    return
  }

  saving.value = true
  try {
    const payload = buildPayload()
    const { data } = await adminAPI.cache.updateConfig(payload)
    applyConfig(data || payload)
    rememberSaved(buildPayload())
    appStore.showSuccess(t('admin.cacheManagement.saved'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.cacheManagement.saveFailed')))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadConfig(true)
})
</script>
