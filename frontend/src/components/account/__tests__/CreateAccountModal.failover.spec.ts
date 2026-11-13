import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'

const { createMock } = vi.hoisted(() => ({ createMock: vi.fn().mockResolvedValue({ id: 1 }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: true }) }))
vi.mock('@/api/admin', () => ({ adminAPI: {
 accounts: { create: createMock, checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false }) },
 settings: { getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false }), getSettings: vi.fn().mockResolvedValue({}) },
 tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
} }))
vi.mock('@/api/admin/accounts', () => ({ getAntigravityDefaultModelMapping: vi.fn() }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
import CreateAccountModal from '../CreateAccountModal.vue'

const BaseDialog = defineComponent({ props: ['show'], template: '<div v-if="show"><slot/><slot name="footer"/></div>' })
function mountModal() {
 return mount(CreateAccountModal, { props: { show: true, proxies: [], groups: [] }, global: { stubs: {
 BaseDialog, Icon: true, PlatformIcon: true, Select: true, ProxySelector: true, GroupSelector: true,
 ModelWhitelistSelector: true, OAuthAuthorizationFlow: true, ProxyAdBanner: true
 } } })
}
async function apiKeyModal() {
 const wrapper = mountModal()
 await wrapper.get('[data-tour="account-form-name"]').setValue('Failover test')
 const keyButton = wrapper.findAll('button').find(b => b.text().includes('admin.accounts.apiKey'))
 expect(keyButton).toBeTruthy()
 await keyButton!.trigger('click')
 await wrapper.get('input[type="password"]').setValue('sk-test')
 return wrapper
}

describe('CreateAccountModal failover settings', () => {
 it.each(['', '[]'])('preserves missing versus explicit empty retry policy %s', async value => {
  createMock.mockClear()
  const wrapper = await apiKeyModal()
  ;(wrapper.vm as any).poolModeEnabled = true
  await flushPromises()
  await wrapper.get('input[placeholder="400, 401, 403, 429, 502, 503, 504, 529"]').setValue(value)
  await wrapper.get('form').trigger('submit.prevent')
  await flushPromises()
  expect(createMock).toHaveBeenCalledTimes(1)
  const credentials = createMock.mock.calls[0]?.[0]?.credentials
  if (value === '[]') expect(credentials.pool_mode_retry_status_codes).toEqual([])
  else expect(credentials).not.toHaveProperty('pool_mode_retry_status_codes')
 })
 it('creates model-scoped rules and blocks fractional retry codes before saving normalized integers', async () => {
  createMock.mockClear()
  const wrapper = await apiKeyModal()
  const vm = wrapper.vm as any
  vm.poolModeEnabled = true
  vm.tempUnschedEnabled = true
  await flushPromises()
  const add = wrapper.findAll('button').find(b => b.text().includes('admin.accounts.tempUnschedulable.addRule'))!
  await add.trigger('click')
  const scope = wrapper.get('[aria-label="admin.accounts.tempUnschedulable.scope"]')
  expect((scope.element as HTMLSelectElement).value).toBe('account')
  await scope.setValue('model')
  await wrapper.get('input[placeholder="admin.accounts.tempUnschedulable.errorCodePlaceholder"]').setValue('503')
  await wrapper.get('input[placeholder="admin.accounts.tempUnschedulable.durationPlaceholder"]').setValue('5')
  await wrapper.get('input[placeholder="admin.accounts.tempUnschedulable.keywordsPlaceholder"]').setValue('busy')
  const codes = wrapper.get('input[placeholder="400, 401, 403, 429, 502, 503, 504, 529"]')
  await codes.setValue('429.5')
  await wrapper.get('form').trigger('submit.prevent')
  expect(createMock).not.toHaveBeenCalled()
  await codes.setValue('503, 429, 503')
  await wrapper.get('form').trigger('submit.prevent')
  await flushPromises()
  expect(createMock).toHaveBeenCalledTimes(1)
  expect(createMock.mock.calls[0]?.[0]?.credentials).toMatchObject({
   pool_mode_retry_count: 2, pool_mode_retry_status_codes: [429, 503],
   temp_unschedulable_rules: [{ scope: 'model', error_code: 503, duration_minutes: 5, keywords: ['busy'] }]
  })
 })
})
