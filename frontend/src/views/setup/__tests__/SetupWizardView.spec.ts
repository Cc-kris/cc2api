import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import SetupWizardView from '@/views/setup/SetupWizardView.vue'

const { testDatabaseMock, testRedisMock, installMock } = vi.hoisted(() => ({
  testDatabaseMock: vi.fn(),
  testRedisMock: vi.fn(),
  installMock: vi.fn()
}))

vi.mock('@/api/setup', () => ({
  testDatabase: testDatabaseMock,
  testRedis: testRedisMock,
  install: installMock
}))

function mountWizard(locale: 'en' | 'zh') {
  const i18n = createI18n({
    legacy: false,
    locale,
    fallbackLocale: 'en',
    messages: { en, zh }
  })
  return mount(SetupWizardView, { global: { plugins: [i18n] } })
}

async function goToRedisStep(wrapper: ReturnType<typeof mountWizard>) {
  const testDbButton = wrapper.findAll('button').find((b) => b.classes('btn-secondary'))
  expect(testDbButton).toBeTruthy()
  await testDbButton!.trigger('click')
  await flushPromises()
  await wrapper.find('button.btn-primary').trigger('click')
  await flushPromises()
}

function redisUsernameInput(wrapper: ReturnType<typeof mountWizard>) {
  return wrapper.findAll('input[type="text"]').find((input) => {
    const label = input.element.parentElement?.querySelector('label')
    return label?.textContent?.includes('Username') || label?.textContent?.includes('用户名')
  })
}

describe('SetupWizardView Redis username', () => {
  beforeEach(() => {
    testDatabaseMock.mockReset().mockResolvedValue({})
    testRedisMock.mockReset().mockResolvedValue({})
    installMock.mockReset().mockResolvedValue({})
  })

  it('renders the optional Redis ACL username field with English copy', async () => {
    const wrapper = mountWizard('en')
    await goToRedisStep(wrapper)

    expect(wrapper.text()).toContain('Username (optional)')
    const input = redisUsernameInput(wrapper)
    expect(input).toBeTruthy()
    expect(input!.attributes('placeholder')).toBe('Leave empty for default user')
  })

  it('renders Chinese copy for the Redis username field', async () => {
    const wrapper = mountWizard('zh')
    await goToRedisStep(wrapper)

    expect(wrapper.text()).toContain('用户名（可选）')
    const input = redisUsernameInput(wrapper)
    expect(input).toBeTruthy()
    expect(input!.attributes('placeholder')).toBe('默认用户留空')
  })

  it('sends the username with the Redis connection test', async () => {
    const wrapper = mountWizard('en')
    await goToRedisStep(wrapper)

    await redisUsernameInput(wrapper)!.setValue('app-user')
    const testRedisButton = wrapper.findAll('button').find((b) => b.text().includes('Test Connection'))
    expect(testRedisButton).toBeTruthy()
    await testRedisButton!.trigger('click')
    await flushPromises()

    expect(testRedisMock).toHaveBeenCalledTimes(1)
    expect(testRedisMock.mock.calls[0][0]).toMatchObject({ host: 'localhost', port: 6379, username: 'app-user' })
  })

  it('defaults the username to empty so the default Redis user is used', async () => {
    const wrapper = mountWizard('en')
    await goToRedisStep(wrapper)

    const testRedisButton = wrapper.findAll('button').find((b) => b.text().includes('Test Connection'))
    await testRedisButton!.trigger('click')
    await flushPromises()

    expect(testRedisMock.mock.calls[0][0]).toMatchObject({ username: '' })
  })
})
