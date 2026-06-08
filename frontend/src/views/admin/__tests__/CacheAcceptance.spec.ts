import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

import router from '@/router'
import { useAuthStore } from '@/stores/auth'
import { canAccessAdminPath, canExportCacheStats } from '@/utils/adminAccess'
import { formatUpstreamAccountForRole, formatUserEmailForRole, redactSensitivePreview } from '@/utils/adminSensitiveDisplay'
import CacheManagementView from '../CacheManagementView.vue'
import CacheStatsView from '../CacheStatsView.vue'
import CacheClearPanel from '../cache/CacheClearPanel.vue'
import AdvancedCachePanel from '../cache/AdvancedCachePanel.vue'
import SemanticCachePanel from '../cache/SemanticCachePanel.vue'

const {
  getConfig,
  updateConfig,
  exportStats,
  getStats,
  getAdvancedConfig,
  updateAdvancedConfig,
  getAdvancedStats,
  getSemanticConfig,
  updateSemanticConfig,
  testSemanticConfig,
  clearLocalResponseCache,
  listClearAudits,
  getAllGroups,
  searchApiKeys,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  exportStats: vi.fn(),
  getStats: vi.fn(),
  getAdvancedConfig: vi.fn(),
  updateAdvancedConfig: vi.fn(),
  getAdvancedStats: vi.fn(),
  getSemanticConfig: vi.fn(),
  updateSemanticConfig: vi.fn(),
  testSemanticConfig: vi.fn(),
  clearLocalResponseCache: vi.fn(),
  listClearAudits: vi.fn(),
  getAllGroups: vi.fn(),
  searchApiKeys: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    cache: {
      getConfig,
      updateConfig,
      exportStats,
      getStats,
      getAdvancedConfig,
      updateAdvancedConfig,
      getAdvancedStats,
      getSemanticConfig,
      updateSemanticConfig,
      testSemanticConfig,
      clearLocalResponseCache,
      listClearAudits
    },
    groups: {
      getAll: getAllGroups
    },
    usage: {
      searchApiKeys
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        return `${key} ${JSON.stringify(params)}`
      }
    })
  }
})

vi.mock('@/utils/format', () => ({
  formatCurrency: (value: number) => `¥${Number(value).toFixed(2)}`,
  formatDateTime: (value?: string | null) => value || ''
}))

const AppLayoutStub = { template: '<div><slot /></div>' }
const RouterLinkStub = { props: ['to'], template: '<a :href="typeof to === \'string\' ? to : to?.path"><slot /></a>' }
const ToggleStub = {
  props: ['modelValue', 'disabled'],
  emits: ['update:modelValue'],
  template: '<button type="button" :disabled="disabled" @click="$emit(\'update:modelValue\', !modelValue)"><slot />{{ modelValue ? \'on\' : \'off\' }}</button>'
}
const BaseDialogStub = { props: ['show', 'title'], emits: ['close'], template: '<div v-if="show"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>' }
const GroupSelectorStub = { props: ['modelValue', 'groups'], emits: ['update:modelValue'], template: '<div data-test="group-selector"></div>' }
const PaginationStub = { template: '<div data-test="pagination"></div>' }

function setViewerRole(role: string) {
  const authStore = useAuthStore()
  authStore.user = {
    id: 1,
    username: `${role || 'admin'}-viewer`,
    email: `${role || 'admin'}@example.com`,
    role: role === 'admin' || role === '' ? 'admin' : role as 'admin',
    balance: 0,
    concurrency: 1,
    status: 'active',
    allowed_groups: [],
    balance_notify_enabled: false,
    balance_notify_threshold: null,
    balance_notify_extra_emails: [],
    created_at: '2026-06-08T00:00:00Z',
    updated_at: '2026-06-08T00:00:00Z'
  }
}

function defaultCacheConfig() {
  return {
    global_enabled: true,
    platforms: {
      openai: { enabled: true },
      claude: { enabled: true },
      gemini: { enabled: true }
    },
    ttl_seconds: 600,
    max_request_bytes: 262144,
    max_response_bytes: 524288,
    max_temperature: 0.3,
    model_allowlist: ['gpt-5.5'],
    model_blocklist: [],
    bypass_header: { name: 'X-Sub2API-Cache-Control', value: 'bypass' }
  }
}


function defaultAdvancedConfig() {
  return {
    advanced_cache_enabled: false,
    gray_scope: { api_key_ids: [], group_ids: [], models: [] },
    redis_capacity_mb: 512,
    memory_safe_limit_mb: 2048,
    compression_enabled: true,
    compression_threshold_kb: 64,
    eviction_policy: 'LRU',
    hot_window: '1h',
    hot_threshold: 5,
    cost_saving_enabled: true,
    upstream_prompt_cache_enabled: true
  }
}

function defaultAdvancedStats() {
  return {
    capacity: { current_used_bytes: 0, capacity_limit_bytes: 0, capacity_usage_rate: 0, memory_safe_limit_bytes: 0, eviction_policy: 'LRU', recent_eviction_count: 0, last_evicted_at: null },
    compression: { enabled: true, raw_response_bytes: 0, stored_response_bytes: 0, compression_saved_bytes: 0, compression_saved_rate: 0, compressed_entry_count: 0, compression_failed_count: 0, decompression_failed_count: 0 },
    hotspots: [],
    savings: { local_response_cache_saved_tokens: 0, local_response_cache_saved_amount: null, upstream_prompt_cache_read_tokens: 0, upstream_prompt_cache_write_tokens: 0, upstream_prompt_cache_saved_amount: null, total_estimated_saved_amount: null, price_missing: false, price_missing_models: [] },
    empty_states: { hotspots: true, prompt_cache: true, price: false },
    fallback: { advanced_cache_fallback_active: false, fallback_reason: null, last_fallback_at: null },
    updated_at: '2026-06-08T00:00:00Z'
  }
}

function defaultSemanticConfig() {
  return {
    enabled: false,
    stage: 'observe' as const,
    platforms: [],
    model_allowlist: [],
    semantic_model_base_url: '',
    semantic_api_key_masked: '',
    semantic_model_name: '',
    namespace: 'default',
    embedding_dimension: null,
    rule_version: 'v1',
    similarity_threshold: 0.98,
    max_reuse_minutes: 10,
    max_candidates: 20,
    gray_api_key_ids: [],
    review_mode: true,
    quality_rollback_threshold_percent: 1,
    auto_closed: false,
    auto_close_reason: null,
    auto_closed_at: null
  }
}

function mountCacheManagement(role = 'admin') {
  setViewerRole(role)
  return mount(CacheManagementView, {
    global: {
      plugins: [router],
      stubs: { AppLayout: AppLayoutStub, RouterLink: RouterLinkStub, Toggle: ToggleStub }
    }
  })
}

function mountCacheStats(role = 'admin') {
  setViewerRole(role)
  return mount(CacheStatsView, {
    global: {
      stubs: { AppLayout: AppLayoutStub, RouterLink: RouterLinkStub }
    }
  })
}


function mountAdvancedCache(role = 'admin') {
  setViewerRole(role)
  return mount(AdvancedCachePanel, {
    global: {
      stubs: { AppLayout: AppLayoutStub, RouterLink: RouterLinkStub, Toggle: ToggleStub, GroupSelector: GroupSelectorStub }
    }
  })
}

function mountSemanticCache(role = 'admin') {
  setViewerRole(role)
  return mount(SemanticCachePanel, {
    global: {
      stubs: { AppLayout: AppLayoutStub, RouterLink: RouterLinkStub, Toggle: ToggleStub }
    }
  })
}

function mountCacheClear(role = 'admin') {
  setViewerRole(role)
  return mount(CacheClearPanel, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        BaseDialog: BaseDialogStub,
        GroupSelector: GroupSelectorStub,
        Pagination: PaginationStub,
        RouterLink: RouterLinkStub
      }
    }
  })
}

describe('cache management product acceptance', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.scrollTo = vi.fn()
    getConfig.mockReset()
    updateConfig.mockReset()
    exportStats.mockReset()
    getStats.mockReset()
    getAdvancedConfig.mockReset()
    updateAdvancedConfig.mockReset()
    getAdvancedStats.mockReset()
    getSemanticConfig.mockReset()
    updateSemanticConfig.mockReset()
    testSemanticConfig.mockReset()
    clearLocalResponseCache.mockReset()
    listClearAudits.mockReset()
    getAllGroups.mockReset()
    searchApiKeys.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    getConfig.mockResolvedValue({ data: defaultCacheConfig() })
    updateConfig.mockImplementation(async (payload) => ({ data: payload }))
    getAdvancedConfig.mockResolvedValue({ data: defaultAdvancedConfig() })
    updateAdvancedConfig.mockImplementation(async (payload) => ({ data: payload }))
    getAdvancedStats.mockResolvedValue({ data: defaultAdvancedStats() })
    getSemanticConfig.mockResolvedValue({ data: defaultSemanticConfig() })
    updateSemanticConfig.mockImplementation(async (payload) => ({ data: payload }))
    testSemanticConfig.mockResolvedValue({ data: { success: true, status: 'success', message: 'ok', semantic_model_base_url: 'https://embed.example.com', model: 'embed', duration_ms: 12 } })
    exportStats.mockResolvedValue({ data: new Blob(['platform,model']) })
    getAllGroups.mockResolvedValue([{ id: 1, name: '默认分组' }])
    searchApiKeys.mockResolvedValue([{ id: 101, name: 'prod-key' }])
    getStats.mockResolvedValue({
      data: {
        summary: {
          total_requests: 30,
          candidate_requests: 30,
          hit_requests: 27,
          miss_requests: 3,
          bypass_requests: 0,
          store_success: 3,
          store_skip: 0,
          request_hit_rate: 90,
          input_tokens: 3000,
          output_tokens: 1500,
          hit_tokens: 4050,
          candidate_tokens: 4500,
          tokens_hit_rate: 90,
          overall_tokens_coverage: 90,
          estimated_saved_amount: '12.5'
        },
        model_rows: [
          { platform: 'openai', model: 'gpt-5.5', total_requests: 10, candidate_requests: 10, hit_requests: 9, miss_requests: 1, bypass_requests: 0, store_success: 1, store_skip: 0, input_tokens: 1000, output_tokens: 500, hit_tokens: 1350, candidate_tokens: 1500, all_request_tokens: 1500, request_hit_rate: 90, tokens_hit_rate: 90, estimated_saved_amount: '4.0' },
          { platform: 'claude', model: 'claude-sonnet-4-5', total_requests: 10, candidate_requests: 10, hit_requests: 9, miss_requests: 1, bypass_requests: 0, store_success: 1, store_skip: 0, input_tokens: 1000, output_tokens: 500, hit_tokens: 1350, candidate_tokens: 1500, all_request_tokens: 1500, request_hit_rate: 90, tokens_hit_rate: 90, estimated_saved_amount: '4.0' },
          { platform: 'gemini', model: 'gemini-2.5-pro', total_requests: 10, candidate_requests: 10, hit_requests: 9, miss_requests: 1, bypass_requests: 0, store_success: 1, store_skip: 0, input_tokens: 1000, output_tokens: 500, hit_tokens: 1350, candidate_tokens: 1500, all_request_tokens: 1500, request_hit_rate: 90, tokens_hit_rate: 90, estimated_saved_amount: '4.5' }
        ],
        bypass_reasons: [],
        store_skip_reasons: []
      }
    })
    clearLocalResponseCache.mockResolvedValue({ data: { clear_type: 'expired', scope: {}, matched_keys: 3, deleted_keys: 3, status: 'success' } })
    listClearAudits.mockResolvedValue({ data: { items: [], total: 0, page: 1, page_size: 20, pages: 0 } })
  })

  it('validates config fields, blocks invalid save, and persists valid three-platform config', async () => {
    const wrapper = mountCacheManagement('admin')
    await flushPromises()

    expect(wrapper.text()).toContain('OpenAI')
    expect(wrapper.text()).toContain('Claude')
    expect(wrapper.text()).toContain('Gemini')
    expect(wrapper.text()).toContain('X-Sub2API-Cache-Control: bypass')

    const ttl = wrapper.find('input[type="number"]')
    await ttl.setValue('30')
    await flushPromises()
    const saveButton = wrapper.find('button.btn-primary')
    expect(wrapper.text()).toContain('admin.cacheManagement.validation.ttl')
    expect(saveButton.attributes('disabled')).toBeDefined()
    expect(updateConfig).not.toHaveBeenCalled()

    await ttl.setValue('900')
    await wrapper.find('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      ttl_seconds: 900,
      platforms: {
        openai: { enabled: true },
        claude: { enabled: true },
        gemini: { enabled: true }
      }
    }))
    expect(showSuccess).toHaveBeenCalledWith('admin.cacheManagement.saved')
  })

  it('keeps cache config readonly for ops role and hides export from ops role', async () => {
    const wrapper = mountCacheManagement('ops')
    await flushPromises()

    expect(wrapper.text()).toContain('admin.cacheManagement.readonlyNotice')
    const saveButton = wrapper.find('button.btn-primary')
    expect(saveButton.attributes('disabled')).toBeDefined()

    const exportButton = wrapper.findAll('button').find((button) => button.text() === 'admin.cacheManagement.export')
    expect(exportButton?.attributes('disabled')).toBeDefined()
  })

  it('shows cache stats model summary, validates custom time range, and masks saved amount for support role', async () => {
    const wrapper = mountCacheStats('support')
    await flushPromises()

    expect(wrapper.text()).toContain('90.00%')
    expect(wrapper.text()).toContain('gpt-5.5')
    expect(wrapper.text()).toContain('claude-sonnet-4-5')
    expect(wrapper.text()).toContain('gemini-2.5-pro')
    expect(wrapper.text()).toContain('admin.cacheStats.hiddenAmount')

    const selects = wrapper.findAll('select')
    await selects[0].setValue('custom')
    const applyButton = wrapper.find('button.btn-primary')
    expect(wrapper.text()).toContain('admin.cacheStats.validation.customRangeRequired')
    expect(applyButton.attributes('disabled')).toBeDefined()
  })



  it('shows load and save failures without overwriting current config', async () => {
    getConfig.mockRejectedValueOnce(new Error('load exploded'))
    const loadWrapper = mountCacheManagement('admin')
    await flushPromises()
    expect(loadWrapper.text()).toContain('load exploded')
    expect(showError).toHaveBeenCalledWith('load exploded')

    updateConfig.mockRejectedValueOnce(new Error('save exploded'))
    const wrapper = mountCacheManagement('admin')
    await flushPromises()
    const ttl = wrapper.find('input[type="number"]')
    await ttl.setValue('901')
    await wrapper.find('button.btn-primary').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('save exploded')
  })

  it('validates cache stats empty state, max time span, and export failure', async () => {
    getStats.mockResolvedValueOnce({ data: { summary: { total_requests: 0, candidate_requests: 0, hit_requests: 0, miss_requests: 0, bypass_requests: 0, store_success: 0, store_skip: 0, request_hit_rate: 0, input_tokens: 0, output_tokens: 0, hit_tokens: 0, candidate_tokens: 0, tokens_hit_rate: 0, overall_tokens_coverage: 0, estimated_saved_amount: '0' }, model_rows: [], bypass_reasons: [], store_skip_reasons: [] } })
    const wrapper = mountCacheStats('admin')
    await flushPromises()
    expect(wrapper.text()).toContain('admin.cacheStats.emptyModelRows')

    const selects = wrapper.findAll('select')
    await selects[0].setValue('custom')
    const dates = wrapper.findAll('input[type="datetime-local"]')
    await dates[0].setValue('2026-01-01T00:00')
    await dates[1].setValue('2026-02-15T00:00')
    expect(wrapper.text()).toContain('admin.cacheStats.validation.maxRange')

    exportStats.mockRejectedValueOnce(new Error('export exploded'))
    const configWrapper = mountCacheManagement('admin')
    await flushPromises()
    const exportButton = configWrapper.findAll('button').find((button) => button.text() === 'admin.cacheManagement.export')
    await exportButton?.trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('export exploded')
  })

  it('prevents duplicate cache clear submissions while request is in progress', async () => {
    let release!: () => void
    clearLocalResponseCache.mockReturnValueOnce(new Promise((resolve) => {
      release = () => resolve({ data: { clear_type: 'expired', scope: {}, matched_keys: 1, deleted_keys: 1, status: 'success' } })
    }))
    const wrapper = mountCacheClear('admin')
    await flushPromises()

    await wrapper.find('button.btn-danger').trigger('click')
    await flushPromises()
    const confirmButton = wrapper.findAll('button').find((button) => button.text() === 'admin.cacheManagement.clearPage.confirmSubmit')
    await confirmButton?.trigger('click')
    await confirmButton?.trigger('click')
    expect(clearLocalResponseCache).toHaveBeenCalledTimes(1)
    release()
    await flushPromises()
  })

  it('validates advanced and semantic cache boundary fields and default disabled state', async () => {
    const advanced = mountAdvancedCache('admin')
    await flushPromises()
    expect(advanced.text()).toContain('admin.cacheManagement.advancedPage.alerts.disabled')
    const advancedNumbers = advanced.findAll('input[type="number"]')
    await advancedNumbers[0].setValue('32')
    await flushPromises()
    expect(advanced.text()).toContain('admin.cacheManagement.advancedPage.validation.redisCapacity')

    const semantic = mountSemanticCache('admin')
    await flushPromises()
    expect(semantic.text()).toContain('Observe 观察模式')
    const semanticNumbers = semantic.findAll('input[type="number"]')
    await semanticNumbers[0].setValue('0.5')
    await flushPromises()
    expect(semantic.text()).toContain('相似度阈值必须在 0.90 到 1.00 之间。')
  })

  it('submits cache clear only after valid scope and confirmation', async () => {
    const wrapper = mountCacheClear('admin')
    await flushPromises()

    const submitButton = wrapper.find('button.btn-danger')
    expect(submitButton.attributes('disabled')).toBeUndefined()

    const radios = wrapper.findAll('input[type="radio"]')
    const byModel = radios.find((item) => item.attributes('value') === 'by_model')
    await byModel?.setValue(true)
    expect(wrapper.text()).toContain('admin.cacheManagement.clearPage.validation.platforms')

    const platformCheckbox = wrapper.findAll('input[type="checkbox"]').find((item) => item.element instanceof HTMLInputElement && item.element.value === 'on')
    await platformCheckbox?.setValue(true)
    const modelInput = wrapper.find('input[type="text"]')
    await modelInput.setValue('gpt-5.5')
    await modelInput.trigger('keydown.enter')
    await wrapper.find('button.btn-danger').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('admin.cacheManagement.clearPage.confirmTitle')
    const confirmInput = wrapper.findAll('input').find((item) => item.attributes('placeholder') === '确认清理')
    await confirmInput?.setValue('确认清理')
    const confirmButton = wrapper.findAll('button').find((button) => button.text() === 'admin.cacheManagement.clearPage.confirmSubmit')
    await confirmButton?.trigger('click')
    await flushPromises()

    expect(clearLocalResponseCache).toHaveBeenCalledWith(expect.objectContaining({
      clear_type: 'by_model',
      scope: expect.objectContaining({
        platforms: ['openai'],
        models: ['gpt-5.5']
      }),
    }))
  })
})

describe('admin scoped access and legacy ops entry acceptance', () => {
  it('keeps scoped role matrix explicit for cache, export, semantic, and unified errors', () => {
    expect(canAccessAdminPath('/admin/settings/cache', 'admin')).toBe(true)
    expect(canAccessAdminPath('/admin/settings/cache', 'ops')).toBe(true)
    expect(canAccessAdminPath('/admin/settings/cache', 'business')).toBe(false)
    expect(canAccessAdminPath('/admin/settings/cache/stats', 'business')).toBe(true)
    expect(canExportCacheStats('business')).toBe(true)
    expect(canExportCacheStats('ops')).toBe(false)
    expect(canAccessAdminPath('/admin/settings/cache/semantic', 'ops')).toBe(true)
    expect(canAccessAdminPath('/admin/settings/cache/semantic', 'business')).toBe(false)
    expect(canAccessAdminPath('/admin/ops/errors', 'support')).toBe(true)
  })



  it('masks sensitive previews and role-scoped identities for page/detail display', () => {
    const raw = 'Authorization: Bearer real-token user=alice@example.com password=abc secret=def'
    const redacted = redactSensitivePreview(raw)
    expect(redacted).not.toContain('real-token')
    expect(redacted).not.toContain('alice@example.com')
    expect(redacted).not.toContain('abc')
    expect(redacted).not.toContain('def')
    expect(redacted).toContain('Authorization: Bearer ***')

    expect(formatUserEmailForRole('customer@example.com', 'support')).toBe('c***@example.com')
    expect(formatUserEmailForRole('customer@example.com', 'ops')).toBe('customer@example.com')
    expect(formatUpstreamAccountForRole('production-upstream-account', 'business')).not.toBe('production-upstream-account')
    expect(formatUpstreamAccountForRole('production-upstream-account', 'ops')).toBe('production-upstream-account')
  })

  it('keeps old ops entry alive and preserves legacy category filters in unified error center', () => {
    const oldOpsEntry = router.resolve('/admin/ops')
    const legacyUpstream = router.resolve('/admin/ops/errors?error_categories=upstream&time_range=1h')
    const legacyClient = router.resolve('/admin/ops/errors?error_categories=client&client_error_subcategory=invalid_request')
    const legacyRequest = router.resolve('/admin/ops/errors?error_results=final_failed&sort_by=occurred_at&sort_order=desc')

    expect(oldOpsEntry.name).toBe('AdminOpsOverview')
    expect(legacyUpstream.name).toBe('AdminOpsUnifiedErrors')
    expect(legacyUpstream.query.error_categories).toBe('upstream')
    expect(legacyClient.name).toBe('AdminOpsUnifiedErrors')
    expect(legacyClient.query.error_categories).toBe('client')
    expect(legacyClient.query.client_error_subcategory).toBe('invalid_request')
    expect(legacyRequest.name).toBe('AdminOpsUnifiedErrors')
    expect(legacyRequest.query.error_results).toBe('final_failed')
  })
})
