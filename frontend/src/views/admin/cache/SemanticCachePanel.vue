<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <div class="flex flex-wrap items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400">
              <component
                v-for="item in navItems"
                :key="item.key"
                :is="item.to ? 'router-link' : 'span'"
                v-bind="item.to ? { to: item.to } : {}"
                class="rounded-full border px-3 py-1 transition-colors"
                :class="item.active
                  ? 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-700/60 dark:bg-primary-900/10 dark:text-primary-200'
                  : item.to
                    ? 'border-gray-200 text-gray-500 hover:border-primary-200 hover:text-primary-600 dark:border-dark-700 dark:text-gray-400 dark:hover:border-primary-700/60 dark:hover:text-primary-200'
                    : 'border-gray-200 bg-gray-50 text-gray-400 dark:border-dark-700 dark:bg-dark-900/20 dark:text-gray-500'"
              >
                {{ item.label }}
              </component>
            </div>
            <h1 class="mt-4 text-2xl font-semibold text-gray-900 dark:text-white">语义缓存配置</h1>
            <p class="mt-2 max-w-3xl text-sm text-gray-600 dark:text-gray-400">
              配置语义模型服务、命中阶段、灰度范围和质量回滚阈值；保存后新请求生效，测试连接不会修改线上配置，审计记录请在语义审计页处理。
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary" :disabled="loading || saving || testing" @click="loadConfig(true)">
              刷新
            </button>
            <button type="button" class="btn btn-secondary" :disabled="saving || testing || !dirty" @click="resetToLoaded">
              撤销修改
            </button>
            <button type="button" class="btn btn-secondary" :disabled="!canManage || saving || testing || validationErrors.length > 0" @click="testConnection">
              {{ testing ? '测试中...' : '测试连接' }}
            </button>
            <button type="button" class="btn btn-primary" :disabled="!canManage || loading || saving || testing || validationErrors.length > 0 || !dirty" @click="saveConfig">
              {{ saving ? '保存中...' : '保存配置' }}
            </button>
          </div>
        </div>
      </div>

      <div
        v-if="!canManage"
        class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/10 dark:text-amber-200"
      >
        当前账号仅可查看语义缓存配置，不能保存修改或测试连接。
      </div>

      <div
        v-if="form.auto_closed"
        class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200"
      >
        语义缓存已自动关闭：{{ form.auto_close_reason || '达到质量回滚阈值' }}
        <span v-if="form.auto_closed_at">（{{ form.auto_closed_at }}）</span>
      </div>

      <div
        v-if="loadError"
        class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200"
      >
        {{ loadError }}
      </div>

      <div
        v-if="validationErrors.length > 0"
        class="rounded-xl border border-red-200 bg-red-50 px-4 py-4 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200"
      >
        <p class="font-medium">保存前请先修正以下问题：</p>
        <ul class="mt-2 list-disc space-y-1 pl-5">
          <li v-for="item in validationErrors" :key="item">{{ item }}</li>
        </ul>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <template v-else>
        <div class="grid grid-cols-1 gap-4 xl:grid-cols-4">
          <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs text-gray-500 dark:text-gray-400">当前状态</p>
            <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ form.enabled ? '已启用' : '已关闭' }}</p>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs text-gray-500 dark:text-gray-400">命中阶段</p>
            <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ currentStageLabel }}</p>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs text-gray-500 dark:text-gray-400">规则版本</p>
            <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ form.rule_version || '--' }}</p>
          </div>
          <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs text-gray-500 dark:text-gray-400">向量维度</p>
            <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ form.embedding_dimension ?? '暂未获取' }}</p>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,0.8fr)]">
          <div class="space-y-6">
            <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">开关与阶段</h2>
              </div>
              <div class="space-y-5 px-6 py-5">
                <div class="flex items-center justify-between gap-4">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">语义缓存开关</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">未完成模型配置时不能开启；关闭后仅保留配置，不参与请求复用。</p>
                  </div>
                  <Toggle v-model="form.enabled" :disabled="!canManage || (!canEnableSemantic && !form.enabled)" />
                </div>

                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">命中阶段</span>
                  <select v-model="form.stage" class="input" :disabled="!canManage">
                    <option v-for="item in stageOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
                  </select>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">observe 只观察，review 强制待审，gray 仅灰度 Key 可复用，active 正式启用，rollback 为回滚关闭。</p>
                </label>

                <div class="flex items-center justify-between gap-4 rounded-xl border border-gray-200 px-4 py-3 dark:border-dark-700">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">人工审核模式</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">开启后命中候选先进入审核，不直接返回语义缓存。</p>
                  </div>
                  <Toggle v-model="form.review_mode" :disabled="!canManage" />
                </div>
              </div>
            </div>

            <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">模型服务</h2>
              </div>
              <div class="grid grid-cols-1 gap-5 px-6 py-5 md:grid-cols-2">
                <label class="block md:col-span-2">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">语义模型服务地址</span>
                  <input v-model.trim="form.semantic_model_base_url" type="text" class="input" :disabled="!canManage" placeholder="https://example.com/v1" />
                </label>

                <label class="block md:col-span-2">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">语义模型 API Key</span>
                  <input v-model.trim="semanticApiKeyInput" type="password" class="input" :disabled="!canManage" placeholder="留空则沿用已保存的 Key" />
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">当前脱敏值：{{ form.semantic_api_key_masked || '未配置' }}</p>
                </label>

                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">语义模型名称</span>
                  <input v-model.trim="form.semantic_model_name" type="text" class="input" :disabled="!canManage" placeholder="text-embedding-3-large" />
                </label>

                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">命名空间</span>
                  <input v-model.trim="form.namespace" type="text" class="input" :disabled="!canManage" placeholder="default" />
                </label>

                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">向量维度</span>
                  <input :value="form.embedding_dimension ?? ''" type="text" class="input" disabled placeholder="测试连接成功后自动回填" />
                </label>

                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">规则版本</span>
                  <input :value="form.rule_version" type="text" class="input" disabled />
                </label>
              </div>
            </div>

            <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">匹配策略</h2>
              </div>
              <div class="grid grid-cols-1 gap-5 px-6 py-5 md:grid-cols-2">
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">相似度阈值</span>
                  <input v-model.number="form.similarity_threshold" type="number" min="0.9" max="1" step="0.0001" class="input" :disabled="!canManage" />
                </label>
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">最大复用时间（分钟）</span>
                  <input v-model.number="form.max_reuse_minutes" type="number" min="1" max="1440" step="1" class="input" :disabled="!canManage" />
                </label>
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">最大候选数</span>
                  <input v-model.number="form.max_candidates" type="number" min="1" max="200" step="1" class="input" :disabled="!canManage" />
                </label>
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">质量回滚阈值（%）</span>
                  <input v-model.number="form.quality_rollback_threshold_percent" type="number" min="0" max="100" step="0.01" class="input" :disabled="!canManage" />
                </label>
              </div>
            </div>

            <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">隔离范围</h2>
              </div>
              <div class="space-y-5 px-6 py-5">
                <div>
                  <div class="mb-3 flex items-center justify-between gap-3">
                    <p class="text-sm font-medium text-gray-900 dark:text-white">适用平台</p>
                    <span class="text-xs text-gray-500 dark:text-gray-400">{{ form.platforms.length }} 项</span>
                  </div>
                  <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
                    <label v-for="platform in platformOptions" :key="platform.value" class="flex cursor-pointer items-center justify-between rounded-lg border border-gray-200 px-4 py-3 text-sm dark:border-dark-700">
                      <span class="font-medium text-gray-900 dark:text-white">{{ platform.label }}</span>
                      <input
                        type="checkbox"
                        :checked="form.platforms.includes(platform.value)"
                        :disabled="!canManage"
                        class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                        @change="togglePlatform(platform.value, ($event.target as HTMLInputElement).checked)"
                      />
                    </label>
                  </div>
                </div>

                <div class="space-y-3">
                  <p class="text-sm font-medium text-gray-900 dark:text-white">适用模型白名单</p>
                  <textarea
                    v-model="modelAllowlistText"
                    rows="4"
                    class="input min-h-[112px]"
                    :disabled="!canManage"
                    placeholder="每行一个模型，支持通配符"
                  />
                </div>

                <div class="space-y-3">
                  <div class="flex items-center justify-between gap-3">
                    <p class="text-sm font-medium text-gray-900 dark:text-white">灰度 API Key</p>
                    <span class="text-xs text-gray-500 dark:text-gray-400">{{ selectedGrayApiKeys.length }} 项</span>
                  </div>
                  <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
                    <div class="flex gap-2">
                      <input
                        v-model.trim="apiKeyKeyword"
                        type="text"
                        class="input flex-1"
                        :disabled="!canManage || testingApiKeys"
                        placeholder="输入 API Key 名称搜索灰度范围"
                        @input="() => debounceApiKeySearch()"
                      />
                      <button type="button" class="btn btn-secondary shrink-0" :disabled="!canManage || testingApiKeys" @click="debounceApiKeySearch(true)">
                        搜索
                      </button>
                    </div>
                    <div v-if="apiKeyResults.length > 0" class="mt-3 space-y-2 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30">
                      <button
                        v-for="item in apiKeyResults"
                        :key="item.id"
                        type="button"
                        class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm transition-colors hover:bg-white dark:hover:bg-dark-800"
                        :disabled="!canManage"
                        @click="selectGrayApiKey(item)"
                      >
                        <span class="font-medium text-gray-900 dark:text-white">{{ formatApiKeyOptionLabel(item.name, item.id) }}</span>
                                              </button>
                    </div>
                    <div v-if="selectedGrayApiKeys.length > 0" class="mt-3 flex flex-wrap gap-2">
                      <span
                        v-for="item in selectedGrayApiKeys"
                        :key="item.id"
                        class="inline-flex items-center gap-2 rounded-full bg-primary-50 px-3 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-200"
                      >
                        {{ formatApiKeyOptionLabel(item.name, item.id) }}
                        <button type="button" :disabled="!canManage" @click="removeGrayApiKey(item.id)">×</button>
                      </span>
                    </div>
                    <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">灰度阶段下，仅所选 API Key 可返回语义缓存；未选择任何 Key 时不可保存为 gray。</p>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="space-y-6">
            <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">当前规则</h2>
              </div>
              <div class="space-y-4 px-6 py-5 text-sm text-gray-600 dark:text-gray-300">
                <div>
                  <p class="font-medium text-gray-900 dark:text-white">命中隔离</p>
                  <p class="mt-1">API Key + User + Group + Platform + Model + System 指纹 + 规则版本</p>
                </div>
                <div>
                  <p class="font-medium text-gray-900 dark:text-white">保存规则</p>
                  <p class="mt-1">语义模型 Key 留空时保留原密文；修改模型、维度、阈值或规则字段后将生成新的规则版本。</p>
                </div>
                <div class="rounded-lg border border-blue-100 bg-blue-50 px-4 py-3 text-xs text-blue-700 dark:border-blue-900/40 dark:bg-blue-900/10 dark:text-blue-200">
                  连接测试只验证当前提交配置的可达性和模型可用性，不会自动开启语义缓存。
                </div>
              </div>
            </div>

            <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">最近一次测试连接</h2>
              </div>
              <div class="space-y-4 px-6 py-5 text-sm text-gray-600 dark:text-gray-300">
                <template v-if="testResult">
                  <div class="rounded-lg px-4 py-3"
                    :class="testResult.success
                      ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/10 dark:text-emerald-200'
                      : 'bg-red-50 text-red-700 dark:bg-red-900/10 dark:text-red-200'"
                  >
                    {{ testResult.message }}
                  </div>
                  <ul class="space-y-2">
                    <li class="flex items-start justify-between gap-3"><span>状态</span><span class="font-medium text-gray-900 dark:text-white">{{ testResult.status }}</span></li>
                    <li class="flex items-start justify-between gap-3"><span>模型</span><span class="font-medium text-gray-900 dark:text-white">{{ testResult.model || '--' }}</span></li>
                    <li class="flex items-start justify-between gap-3"><span>向量维度</span><span class="font-medium text-gray-900 dark:text-white">{{ testResult.embedding_dimension ?? '--' }}</span></li>
                    <li class="flex items-start justify-between gap-3"><span>HTTP 状态</span><span class="font-medium text-gray-900 dark:text-white">{{ testResult.http_status ?? '--' }}</span></li>
                    <li class="flex items-start justify-between gap-3"><span>耗时</span><span class="font-medium text-gray-900 dark:text-white">{{ testResult.duration_ms }}ms</span></li>
                  </ul>
                </template>
                <p v-else class="text-xs text-gray-500 dark:text-gray-400">尚未执行连接测试。</p>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Toggle from '@/components/common/Toggle.vue'
import { adminAPI } from '@/api/admin'
import { defaultSemanticCacheConfig, type SemanticCacheConfig, type SemanticCacheConnectionTestResult, type SemanticCacheStage } from '@/api/admin/cache'
import type { SimpleApiKey } from '@/api/admin/usage'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatApiKeyOptionLabel } from '@/utils/adminSensitiveDisplay'

const appStore = useAppStore()
const authStore = useAuthStore()

const loading = ref(true)
const saving = ref(false)
const testing = ref(false)
const testingApiKeys = ref(false)
const loadError = ref('')
const semanticApiKeyInput = ref('')
const modelAllowlistText = ref('')
const apiKeyKeyword = ref('')
const apiKeyResults = ref<SimpleApiKey[]>([])
const selectedGrayApiKeys = ref<SimpleApiKey[]>([])
const testResult = ref<SemanticCacheConnectionTestResult | null>(null)
const lastSavedSnapshot = ref('')
let apiKeySearchTimeout: ReturnType<typeof setTimeout> | null = null

const form = reactive<SemanticCacheConfig>(defaultSemanticCacheConfig())

const viewerRole = computed(() => String((authStore.user as { role?: string } | null)?.role || '').trim().toLowerCase())
const canManage = computed(() => viewerRole.value === '' || viewerRole.value === 'admin')
const canEnableSemantic = computed(() => hasRequiredConnectionFields.value)
const currentStageLabel = computed(() => stageOptions.find((item) => item.value === form.stage)?.label ?? form.stage)
const hasRequiredConnectionFields = computed(() => Boolean(form.semantic_model_base_url.trim() && (semanticApiKeyInput.value.trim() || form.semantic_api_key_masked) && form.semantic_model_name.trim()))

const navItems = computed(() => [
  { key: 'home', to: '/admin/settings/cache', label: '管理首页', active: false },
  { key: 'stats', to: '/admin/settings/cache/stats', label: '缓存统计', active: false },
  { key: 'semantic', to: '/admin/settings/cache/semantic', label: '语义配置', active: true },
  { key: 'audit', to: '/admin/settings/cache/semantic-audits', label: '语义审计', active: false }
])

const stageOptions: Array<{ value: SemanticCacheStage; label: string }> = [
  { value: 'observe', label: 'Observe 观察模式' },
  { value: 'review', label: 'Review 审核模式' },
  { value: 'gray', label: 'Gray 灰度模式' },
  { value: 'active', label: 'Active 正式启用' },
  { value: 'rollback', label: 'Rollback 回滚关闭' }
]

const platformOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'claude', label: 'Claude' },
  { value: 'gemini', label: 'Gemini' }
]

const validationErrors = computed(() => {
  const errors: string[] = []
  if (form.semantic_model_base_url.trim()) {
    try {
      const url = new URL(form.semantic_model_base_url.trim())
      if (!['http:', 'https:'].includes(url.protocol)) {
        errors.push('语义模型服务地址必须是 http 或 https。')
      }
    } catch {
      errors.push('语义模型服务地址格式不正确。')
    }
  }
  if (form.similarity_threshold < 0.9 || form.similarity_threshold > 1) {
    errors.push('相似度阈值必须在 0.90 到 1.00 之间。')
  }
  if (!hasMaxDecimals(form.similarity_threshold, 4)) {
    errors.push('相似度阈值最多保留 4 位小数。')
  }
  if (!Number.isFinite(form.max_reuse_minutes) || form.max_reuse_minutes < 1 || form.max_reuse_minutes > 1440) {
    errors.push('最大复用时间必须在 1 到 1440 分钟之间。')
  }
  if (!Number.isFinite(form.max_candidates) || form.max_candidates < 1 || form.max_candidates > 200) {
    errors.push('最大候选数必须在 1 到 200 之间。')
  }
  if (!Number.isFinite(form.quality_rollback_threshold_percent) || form.quality_rollback_threshold_percent < 0 || form.quality_rollback_threshold_percent > 100) {
    errors.push('质量回滚阈值必须在 0 到 100 之间。')
  }
  if (!hasMaxDecimals(form.quality_rollback_threshold_percent, 2)) {
    errors.push('质量回滚阈值最多保留 2 位小数。')
  }
  if (form.enabled && !hasRequiredConnectionFields.value) {
    errors.push('启用语义缓存前，请先完成服务地址、API Key 和模型名称配置。')
  }
  if (form.stage === 'gray' && form.gray_api_key_ids.length === 0) {
    errors.push('灰度模式必须至少选择一个 API Key。')
  }
  if (form.stage === 'rollback' && form.enabled) {
    errors.push('回滚阶段不能保持启用状态。')
  }
  return errors
})

const dirty = computed(() => JSON.stringify(buildPayload()) !== lastSavedSnapshot.value)

function applyConfig(config: SemanticCacheConfig): void {
  const next = JSON.parse(JSON.stringify(config)) as SemanticCacheConfig
  Object.assign(form, next)
  semanticApiKeyInput.value = ''
  modelAllowlistText.value = next.model_allowlist.join('\n')
}

function buildPayload(): SemanticCacheConfig {
  return {
    ...JSON.parse(JSON.stringify(form)),
    semantic_api_key: semanticApiKeyInput.value.trim() || undefined,
    model_allowlist: parseModelList(modelAllowlistText.value),
    gray_api_key_ids: selectedGrayApiKeys.value.map((item) => item.id)
  }
}

function parseModelList(text: string): string[] {
  const seen = new Set<string>()
  return text
    .split(/[\n,]/g)
    .map((item) => item.trim())
    .filter((item) => {
      if (!item) return false
      const lower = item.toLowerCase()
      if (seen.has(lower)) return false
      seen.add(lower)
      return true
    })
}

function rememberSaved(config: SemanticCacheConfig): void {
  lastSavedSnapshot.value = JSON.stringify(config)
}

function hasMaxDecimals(value: number, decimals: number): boolean {
  const factor = 10 ** decimals
  return Math.abs(value * factor - Math.round(value * factor)) < 1e-9
}

function togglePlatform(platform: string, checked: boolean): void {
  const next = new Set(form.platforms)
  if (checked) next.add(platform)
  else next.delete(platform)
  form.platforms = Array.from(next)
}

function resetToLoaded(): void {
  applyConfig(JSON.parse(lastSavedSnapshot.value) as SemanticCacheConfig)
  hydrateSelectedApiKeys()
}

function selectGrayApiKey(item: SimpleApiKey): void {
  if (selectedGrayApiKeys.value.some((current) => current.id === item.id)) return
  selectedGrayApiKeys.value = [...selectedGrayApiKeys.value, item]
  form.gray_api_key_ids = selectedGrayApiKeys.value.map((current) => current.id)
}

function removeGrayApiKey(id: number): void {
  selectedGrayApiKeys.value = selectedGrayApiKeys.value.filter((item) => item.id !== id)
  form.gray_api_key_ids = selectedGrayApiKeys.value.map((item) => item.id)
}

async function hydrateSelectedApiKeys(): Promise<void> {
  if (form.gray_api_key_ids.length === 0) {
    selectedGrayApiKeys.value = []
    return
  }
  const resolved: SimpleApiKey[] = []
  for (const id of form.gray_api_key_ids) {
    if (resolved.some((item) => item.id === id)) continue
    resolved.push({ id, name: '', user_id: 0 })
  }
  selectedGrayApiKeys.value = resolved
}

function debounceApiKeySearch(immediate = false): void {
  if (apiKeySearchTimeout) clearTimeout(apiKeySearchTimeout)
  if (!apiKeyKeyword.value.trim()) {
    apiKeyResults.value = []
    return
  }
  const run = async () => {
    testingApiKeys.value = true
    try {
      apiKeyResults.value = await adminAPI.usage.searchApiKeys(undefined, apiKeyKeyword.value.trim())
    } catch (error) {
      appStore.showError(extractApiErrorMessage(error, 'API Key 搜索失败'))
    } finally {
      testingApiKeys.value = false
    }
  }
  if (immediate) {
    void run()
    return
  }
  apiKeySearchTimeout = setTimeout(() => void run(), 250)
}

async function loadConfig(force = false): Promise<void> {
  if (!force && !loading.value && lastSavedSnapshot.value) return
  loading.value = true
  loadError.value = ''
  try {
    const { data: config } = await adminAPI.cache.getSemanticConfig()
    applyConfig(config)
    await hydrateSelectedApiKeys()
    rememberSaved(buildPayload())
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, '语义缓存配置加载失败')
  } finally {
    loading.value = false
  }
}

async function testConnection(): Promise<void> {
  testing.value = true
  try {
    const { data: result } = await adminAPI.cache.testSemanticConfig(buildPayload())
    testResult.value = result
    if (typeof result.embedding_dimension === 'number' && Number.isFinite(result.embedding_dimension)) {
      form.embedding_dimension = result.embedding_dimension
    }
    if (result.success) {
      appStore.showSuccess(result.message)
    } else {
      appStore.showError(result.message)
    }
  } catch (error) {
    const message = extractApiErrorMessage(error, '语义模型连接测试失败')
    testResult.value = {
      success: false,
      status: 'failed',
      message,
      semantic_model_base_url: form.semantic_model_base_url,
      model: form.semantic_model_name,
      duration_ms: 0
    }
    appStore.showError(message)
  } finally {
    testing.value = false
  }
}

async function saveConfig(): Promise<void> {
  saving.value = true
  try {
    const { data: saved } = await adminAPI.cache.updateSemanticConfig(buildPayload())
    applyConfig(saved)
    await hydrateSelectedApiKeys()
    rememberSaved(buildPayload())
    appStore.showSuccess('语义缓存配置已保存')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '语义缓存配置保存失败'))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void loadConfig(true)
})
</script>
