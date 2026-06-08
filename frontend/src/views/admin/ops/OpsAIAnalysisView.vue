<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <section class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.aiAnalysis.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.aiAnalysis.description') }}
            </p>
          </div>

          <div class="flex flex-wrap items-center gap-3">
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:border-blue-300 hover:text-blue-600 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300"
              :disabled="loading || saving || testing"
              @click="loadConfig"
            >
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              {{ t('admin.ops.aiAnalysis.refresh') }}
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:border-blue-300 hover:text-blue-600 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:text-gray-200 dark:hover:border-blue-500 dark:hover:text-blue-300"
              :disabled="testButtonDisabled"
              @click="runConnectionTest"
            >
              <Icon name="bolt" size="sm" :class="testing ? 'animate-pulse' : ''" />
              {{ testing ? t('admin.ops.aiAnalysis.testing') : testButtonLabel }}
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:bg-blue-300 dark:disabled:bg-blue-800/60"
              :disabled="saveButtonDisabled"
              @click="saveConfig()"
            >
              <Icon name="check" size="sm" />
              {{ saving ? t('admin.ops.aiAnalysis.saving') : t('admin.ops.aiAnalysis.save') }}
            </button>
          </div>
        </div>

        <div v-if="!isEditable" class="mt-4 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300">
          {{ t('admin.ops.aiAnalysis.readOnly') }}
        </div>

        <div v-else-if="isDirty" class="mt-4 rounded-2xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-700 dark:border-blue-900/40 dark:bg-blue-900/20 dark:text-blue-300">
          {{ t('admin.ops.aiAnalysis.dirtyHint') }}
        </div>

        <div v-else class="mt-4 rounded-2xl border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-800/70 dark:text-gray-300">
          {{ t('admin.ops.aiAnalysis.noChanges') }}
        </div>

        <div v-if="loadError" class="mt-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
          {{ loadError }}
        </div>
      </section>

      <section class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
        <div class="space-y-6">
          <div class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <div class="mb-4 flex items-center justify-between gap-3">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.aiAnalysis.cards.basic') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.ops.aiAnalysis.fields.enabledHint') }}</p>
              </div>
            </div>

            <div v-if="validationErrors.length" class="mb-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 dark:border-red-900/40 dark:bg-red-900/20">
              <div class="text-sm font-semibold text-red-700 dark:text-red-300">{{ t('admin.ops.aiAnalysis.validation.title') }}</div>
              <ul class="mt-2 list-disc space-y-1 pl-5 text-sm text-red-700 dark:text-red-300">
                <li v-for="item in validationErrors" :key="item">{{ item }}</li>
              </ul>
            </div>

            <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div class="md:col-span-2 rounded-2xl border border-gray-200 p-4 dark:border-dark-700">
                <label class="flex items-start justify-between gap-4">
                  <div>
                    <div class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.ops.aiAnalysis.fields.enabled') }}</div>
                    <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.aiAnalysis.fields.enabledHint') }}</div>
                  </div>
                  <input v-model="form.enabled" type="checkbox" class="mt-1 h-5 w-5 rounded border-gray-300 text-blue-600 focus:ring-blue-500" :disabled="!isEditable">
                </label>
              </div>

              <div class="md:col-span-2">
                <label class="filter-label">{{ t('admin.ops.aiAnalysis.fields.baseUrl') }}</label>
                <input v-model.trim="form.base_url" type="url" class="input" :disabled="!isEditable" placeholder="https://example.com/v1">
              </div>

              <div class="md:col-span-2">
                <label class="filter-label">{{ t('admin.ops.aiAnalysis.fields.apiKey') }}</label>
                <input v-model.trim="form.api_key" type="password" class="input" :disabled="!isEditable" :placeholder="t('admin.ops.aiAnalysis.fields.apiKeyPlaceholder')">
                <p v-if="storedMaskedKey" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.ops.aiAnalysis.fields.apiKeyHint', { masked: storedMaskedKey }) }}
                </p>
              </div>

              <div>
                <label class="filter-label">{{ t('admin.ops.aiAnalysis.fields.model') }}</label>
                <input v-model.trim="form.model" type="text" class="input" :disabled="!isEditable" placeholder="gpt-5.5">
              </div>

              <div>
                <label class="filter-label">{{ t('admin.ops.aiAnalysis.fields.interfaceType') }}</label>
                <select v-model="form.interface_type" class="input" :disabled="!isEditable">
                  <option v-for="option in interfaceTypeOptions" :key="option.value" :value="option.value">
                    {{ option.label }}
                  </option>
                </select>
              </div>
            </div>
          </div>

          <div class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.aiAnalysis.cards.behavior') }}</h2>
            <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <label class="filter-label">{{ t('admin.ops.aiAnalysis.fields.timeoutSeconds') }}</label>
                <input v-model.number="form.timeout_seconds" type="number" min="5" max="300" step="1" class="input" :disabled="!isEditable">
              </div>
              <div>
                <label class="filter-label">{{ t('admin.ops.aiAnalysis.fields.maxSamples') }}</label>
                <input v-model.number="form.max_samples" type="number" min="1" max="500" step="1" class="input" :disabled="!isEditable">
              </div>
              <div>
                <label class="filter-label">{{ t('admin.ops.aiAnalysis.fields.autoDedupMinutes') }}</label>
                <input v-model.number="form.auto_dedup_minutes" type="number" min="1" max="1440" step="1" class="input" :disabled="!isEditable">
              </div>
              <div>
                <label class="filter-label">{{ t('admin.ops.aiAnalysis.fields.globalRateLimitPerMinute') }}</label>
                <input v-model.number="form.global_rate_limit_per_minute" type="number" min="1" max="1000" step="1" class="input" :disabled="!isEditable">
              </div>

              <div class="md:col-span-2">
                <label class="filter-label">{{ t('admin.ops.aiAnalysis.fields.autoLevels') }}</label>
                <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
                  <label v-for="option in autoLevelOptions" :key="option.value" class="flex items-center gap-2 rounded-2xl border border-gray-200 px-3 py-3 text-sm text-gray-700 dark:border-dark-700 dark:text-gray-200">
                    <input
                      :checked="form.auto_levels.includes(option.value)"
                      type="checkbox"
                      class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                      :disabled="!isEditable"
                      @change="toggleAutoLevel(option.value)"
                    >
                    <span>{{ option.label }}</span>
                  </label>
                </div>
              </div>

              <div class="md:col-span-2 rounded-2xl border border-gray-200 p-4 dark:border-dark-700">
                <label class="flex items-start justify-between gap-4">
                  <div>
                    <div class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.ops.aiAnalysis.fields.manualEnabled') }}</div>
                    <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.aiAnalysis.fields.manualEnabledHint') }}</div>
                  </div>
                  <input v-model="form.manual_enabled" type="checkbox" class="mt-1 h-5 w-5 rounded border-gray-300 text-blue-600 focus:ring-blue-500" :disabled="!isEditable">
                </label>
              </div>
            </div>
          </div>
        </div>

        <div class="space-y-6">
          <div class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.aiAnalysis.cards.testResult') }}</h2>
            <div v-if="testResult" class="mt-4 rounded-2xl border p-4" :class="testResultClasses">
              <div class="flex items-start justify-between gap-3">
                <div>
                  <div class="text-sm font-semibold">{{ resolvedConnectionStatusLabel }}</div>
                  <div class="mt-1 text-sm">{{ resolvedConnectionMessage }}</div>
                </div>
                <span class="rounded-full px-3 py-1 text-xs font-semibold" :class="testResultBadgeClasses">
                  {{ String(testResult.status).toUpperCase() }}
                </span>
              </div>

              <div class="mt-4 space-y-2 text-xs">
                <div>{{ t('admin.ops.aiAnalysis.resultMeta.endpoint', { baseUrl: testResult.base_url || '-' }) }}</div>
                <div>{{ t('admin.ops.aiAnalysis.resultMeta.model', { model: testResult.model || '-' }) }}</div>
                <div v-if="typeof testResult.http_status === 'number'">
                  {{ t('admin.ops.aiAnalysis.resultMeta.httpStatus', { status: String(testResult.http_status) }) }}
                </div>
                <div v-if="typeof testResult.duration_ms === 'number'">
                  {{ t('admin.ops.aiAnalysis.resultMeta.duration', { duration: String(testResult.duration_ms) }) }}
                </div>
              </div>
            </div>
            <div v-else class="mt-4 rounded-2xl border border-dashed border-gray-300 px-4 py-6 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
              {{ t('admin.ops.aiAnalysis.noTestResult') }}
            </div>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  opsAPI,
  type OpsAIAnalysisConfig,
  type OpsAIAnalysisConnectionStatus,
  type OpsAIAnalysisInterfaceType,
  type OpsAIAnalysisTestResponse,
  type UpdateOpsAIAnalysisConfigRequest,
} from '@/api/admin/ops'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'

type AutoLevel = 'P0' | 'P1' | 'P2' | 'observe'

type FormState = UpdateOpsAIAnalysisConfigRequest & {
  api_key: string
}

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const editableRoles = new Set(['admin'])

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const loadError = ref('')
const storedMaskedKey = ref('')
const originalConfigSignature = ref('')
const testResult = ref<OpsAIAnalysisTestResponse | null>(null)

const form = reactive<FormState>({
  enabled: false,
  base_url: '',
  api_key: '',
  model: '',
  interface_type: 'responses',
  timeout_seconds: 60,
  max_samples: 50,
  auto_dedup_minutes: 10,
  global_rate_limit_per_minute: 10,
  auto_levels: ['P0', 'P1'],
  manual_enabled: true,
})

const interfaceTypeOptions = computed<Array<{ value: OpsAIAnalysisInterfaceType; label: string }>>(() => [
  { value: 'openai_compatible', label: t('admin.ops.aiAnalysis.interfaceTypes.openai_compatible') },
  { value: 'responses', label: t('admin.ops.aiAnalysis.interfaceTypes.responses') },
  { value: 'anthropic_compatible', label: t('admin.ops.aiAnalysis.interfaceTypes.anthropic_compatible') },
  { value: 'gemini_compatible', label: t('admin.ops.aiAnalysis.interfaceTypes.gemini_compatible') },
])

const autoLevelOptions = computed<Array<{ value: AutoLevel; label: string }>>(() => [
  { value: 'P0', label: t('admin.ops.aiAnalysis.autoLevels.P0') },
  { value: 'P1', label: t('admin.ops.aiAnalysis.autoLevels.P1') },
  { value: 'P2', label: t('admin.ops.aiAnalysis.autoLevels.P2') },
  { value: 'observe', label: t('admin.ops.aiAnalysis.autoLevels.observe') },
])

const currentViewerRole = computed(() => String((authStore.user as { role?: string } | null)?.role || '').trim().toLowerCase())
const isEditable = computed(() => editableRoles.has(currentViewerRole.value))

function normalizedPayload(): UpdateOpsAIAnalysisConfigRequest {
  return {
    enabled: Boolean(form.enabled),
    base_url: String(form.base_url || '').trim(),
    model: String(form.model || '').trim(),
    interface_type: form.interface_type,
    timeout_seconds: Number(form.timeout_seconds),
    max_samples: Number(form.max_samples),
    auto_dedup_minutes: Number(form.auto_dedup_minutes),
    global_rate_limit_per_minute: Number(form.global_rate_limit_per_minute),
    auto_levels: [...form.auto_levels].sort(),
    manual_enabled: Boolean(form.manual_enabled),
  }
}

const isDirty = computed(() => {
  return originalConfigSignature.value !== JSON.stringify(normalizedPayload()) || form.api_key.trim() !== ''
})

const validationErrors = computed(() => {
  const errors: string[] = []
  const baseUrl = form.base_url.trim()
  const model = form.model.trim()
  const apiKey = form.api_key.trim()
  const hasStoredKey = storedMaskedKey.value.trim() !== ''

  try {
    const parsed = new URL(baseUrl)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      errors.push(t('admin.ops.aiAnalysis.validation.baseUrl'))
    }
  } catch {
    errors.push(t('admin.ops.aiAnalysis.validation.baseUrl'))
  }

  if (!apiKey && !hasStoredKey) errors.push(t('admin.ops.aiAnalysis.validation.apiKey'))
  if (!model || model.length > 100) errors.push(t('admin.ops.aiAnalysis.validation.model'))
  if (!form.interface_type) errors.push(t('admin.ops.aiAnalysis.validation.interfaceType'))
  if (!Number.isInteger(form.timeout_seconds) || form.timeout_seconds < 5 || form.timeout_seconds > 300) {
    errors.push(t('admin.ops.aiAnalysis.validation.timeoutSeconds'))
  }
  if (!Number.isInteger(form.max_samples) || form.max_samples < 1 || form.max_samples > 500) {
    errors.push(t('admin.ops.aiAnalysis.validation.maxSamples'))
  }
  if (!Number.isInteger(form.auto_dedup_minutes) || form.auto_dedup_minutes < 1 || form.auto_dedup_minutes > 1440) {
    errors.push(t('admin.ops.aiAnalysis.validation.autoDedupMinutes'))
  }
  if (!Number.isInteger(form.global_rate_limit_per_minute) || form.global_rate_limit_per_minute < 1 || form.global_rate_limit_per_minute > 1000) {
    errors.push(t('admin.ops.aiAnalysis.validation.globalRateLimitPerMinute'))
  }

  return [...new Set(errors)]
})

const canTestWithCurrentForm = computed(() => {
  return form.base_url.trim() !== '' && form.model.trim() !== '' && (form.api_key.trim() !== '' || storedMaskedKey.value.trim() !== '')
})

const saveButtonDisabled = computed(() => !isEditable.value || loading.value || saving.value || testing.value)
const testButtonDisabled = computed(() => !isEditable.value || loading.value || saving.value || testing.value || !canTestWithCurrentForm.value)
const testButtonLabel = computed(() => testResult.value ? t('admin.ops.aiAnalysis.testActionRetry') : t('admin.ops.aiAnalysis.testAction'))

const resolvedConnectionStatusLabel = computed(() => {
  const status = (testResult.value?.status || 'failed') as OpsAIAnalysisConnectionStatus
  return t(`admin.ops.aiAnalysis.connectionStatus.${status}`)
})

const resolvedConnectionMessage = computed(() => {
  const status = (testResult.value?.status || 'failed') as OpsAIAnalysisConnectionStatus
  const fallback = testResult.value?.message || t('admin.ops.aiAnalysis.testFailed')
  const translated = t(`admin.ops.aiAnalysis.connectionStatus.${status}`)
  return translated === `admin.ops.aiAnalysis.connectionStatus.${status}` ? fallback : translated
})

const testResultClasses = computed(() => {
  if (testResult.value?.success) {
    return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/40 dark:bg-emerald-900/20 dark:text-emerald-300'
  }
  return 'border-red-200 bg-red-50 text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300'
})

const testResultBadgeClasses = computed(() => {
  if (testResult.value?.success) {
    return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-200'
  }
  return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-200'
})

function applyConfig(config: OpsAIAnalysisConfig) {
  form.enabled = config.enabled
  form.base_url = config.base_url || ''
  form.api_key = ''
  form.model = config.model || ''
  form.interface_type = config.interface_type || 'responses'
  form.timeout_seconds = config.timeout_seconds ?? 60
  form.max_samples = config.max_samples ?? 50
  form.auto_dedup_minutes = config.auto_dedup_minutes ?? 10
  form.global_rate_limit_per_minute = config.global_rate_limit_per_minute ?? 10
  form.auto_levels = Array.isArray(config.auto_levels) ? [...config.auto_levels].sort() : ['P0', 'P1']
  form.manual_enabled = config.manual_enabled ?? true
  storedMaskedKey.value = config.api_key_masked || ''
  originalConfigSignature.value = JSON.stringify(normalizedPayload())
}

function toggleAutoLevel(level: AutoLevel) {
  if (!isEditable.value) return
  if (form.auto_levels.includes(level)) {
    form.auto_levels = form.auto_levels.filter((item) => item !== level)
    return
  }
  form.auto_levels = [...form.auto_levels, level].sort()
}

async function loadConfig() {
  loading.value = true
  loadError.value = ''
  try {
    const config = await opsAPI.getAIAnalysisConfig()
    applyConfig(config)
  } catch (err: unknown) {
    loadError.value = extractApiErrorMessage(err, t('admin.ops.aiAnalysis.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function saveConfig(options: { silentSuccess?: boolean } = {}): Promise<boolean> {
  if (!isEditable.value) return false
  if (validationErrors.value.length > 0) {
    appStore.showError(validationErrors.value[0])
    return false
  }

  saving.value = true
  try {
    const payload: UpdateOpsAIAnalysisConfigRequest = normalizedPayload()
    if (form.api_key.trim()) payload.api_key = form.api_key.trim()
    const config = await opsAPI.updateAIAnalysisConfig(payload)
    applyConfig(config)
    if (!options.silentSuccess) appStore.showSuccess(t('admin.ops.aiAnalysis.saveSuccess'))
    return true
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.ops.aiAnalysis.saveFailed')))
    return false
  } finally {
    saving.value = false
  }
}

async function runConnectionTest() {
  if (!isEditable.value) return
  if (validationErrors.value.length > 0) {
    appStore.showError(validationErrors.value[0])
    return
  }

  if (isDirty.value) {
    const saved = await saveConfig({ silentSuccess: true })
    if (!saved) return
  }

  testing.value = true
  try {
    const result = await opsAPI.testAIAnalysisConnection()
    testResult.value = result
    const message = resolvedStatusMessage(result)
    if (result.success) {
      appStore.showSuccess(message)
    } else {
      appStore.showError(message)
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.ops.aiAnalysis.testFailed')))
  } finally {
    testing.value = false
  }
}

function resolvedStatusMessage(result: OpsAIAnalysisTestResponse): string {
  const key = `admin.ops.aiAnalysis.connectionStatus.${result.status}`
  const translated = t(key)
  if (translated !== key) return translated
  return result.message || t('admin.ops.aiAnalysis.testFailed')
}

onMounted(() => {
  void loadConfig()
})
</script>
