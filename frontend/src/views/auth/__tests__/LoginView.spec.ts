import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LoginView from '@/views/auth/LoginView.vue'
import type { PublicSettings } from '@/types'

const {
  routerPushMock,
  loginMock,
  login2FAMock,
  showWarningMock,
  showErrorMock,
  showSuccessMock,
  getPublicSettingsMock,
  basePublicSettings,
} = vi.hoisted(() => ({
  routerPushMock: vi.fn(),
  loginMock: vi.fn(),
  login2FAMock: vi.fn(),
  showWarningMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn(),
  getPublicSettingsMock: vi.fn(),
  basePublicSettings: {
    registration_enabled: true,
    email_verify_enabled: false,
    force_email_on_third_party_signup: false,
    registration_email_suffix_whitelist: [],
    promo_code_enabled: false,
    password_reset_enabled: false,
    invitation_code_enabled: false,
    turnstile_enabled: false,
    turnstile_site_key: '',
    site_name: 'Sub2API',
    site_logo: '',
    site_subtitle: '',
    api_base_url: '',
    contact_info: '',
    doc_url: '',
    home_content: '',
    hide_ccs_import_button: false,
    payment_enabled: false,
    risk_control_enabled: false,
    table_default_page_size: 20,
    table_page_size_options: [10, 20, 50],
    custom_menu_items: [],
    custom_endpoints: [],
    linuxdo_oauth_enabled: false,
    dingtalk_oauth_enabled: false,
    wechat_oauth_enabled: false,
    oidc_oauth_enabled: false,
    oidc_oauth_provider_name: 'OIDC',
    github_oauth_enabled: false,
    google_oauth_enabled: false,
    backend_mode_enabled: false,
    version: 'test',
    balance_low_notify_enabled: false,
    account_quota_notify_enabled: false,
    balance_low_notify_threshold: 0,
    channel_monitor_enabled: false,
    channel_monitor_public_enabled: false,
    channel_monitor_default_interval_seconds: 60,
    available_channels_enabled: false,
    affiliate_enabled: false,
  } satisfies PublicSettings,
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: (...args: any[]) => routerPushMock(...args),
    currentRoute: { value: { query: {} } },
  }),
  RouterLink: {
    name: 'RouterLink',
    props: ['to'],
    template: '<a><slot /></a>',
  },
  'router-link': {
    name: 'router-link',
    props: ['to'],
    template: '<a><slot /></a>',
  },
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      locale: { value: 'zh' },
      setLocaleMessage: vi.fn(),
    },
  }),
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    login: (...args: any[]) => loginMock(...args),
    login2FA: (...args: any[]) => login2FAMock(...args),
  }),
  useAppStore: () => ({
    showWarning: (...args: any[]) => showWarningMock(...args),
    showError: (...args: any[]) => showErrorMock(...args),
    showSuccess: (...args: any[]) => showSuccessMock(...args),
  }),
}))

vi.mock('@/api/auth', () => {
  return {
    getPublicSettings: (...args: any[]) => getPublicSettingsMock(...args),
    isTotp2FARequired: (response: any) => response?.requires_2fa === true,
    isWeChatWebOAuthEnabled: (settings: any) => settings?.wechat_oauth_enabled === true,
  }
})

vi.mock('@/utils/apiError', () => ({
  extractI18nErrorMessage: (error: any, _t: any, _scope: string, fallback: string) =>
    error?.message || fallback,
}))

vi.mock('@/utils/oauthAffiliate', () => ({
  clearAllAffiliateReferralCodes: vi.fn(),
}))

const publicSettings = (overrides: Partial<PublicSettings> = {}): PublicSettings => ({
  ...basePublicSettings,
  ...overrides,
})

describe('LoginView 登录协议交互', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    window.sessionStorage.clear()
    document.body.innerHTML = ''
  })

  function mountLoginView() {
    return mount(LoginView, {
      global: {
        stubs: {
          AuthLayout: { template: '<main><slot /><slot name="footer" /></main>' },
          Icon: { template: '<span />' },
          TurnstileWidget: { template: '<div />' },
          TotpLoginModal: { template: '<div />' },
          RouterLink: { template: '<a><slot /></a>' },
          'router-link': { template: '<a><slot /></a>' },
          EmailOAuthButtons: { template: '<div />' },
          LinuxDoOAuthSection: { template: '<div />' },
          DingTalkOAuthSection: { template: '<div />' },
          WechatOAuthSection: { template: '<div />' },
          OidcOAuthSection: { template: '<div />' },
        },
      },
    })
  }

  it('checkbox 模式未勾选登录协议时仍允许填写邮箱和密码，但提交登录时提示勾选协议且不发起登录', async () => {
    getPublicSettingsMock.mockResolvedValue(publicSettings({
      login_agreement_enabled: true,
      login_agreement_mode: 'checkbox',
      login_agreement_updated_at: '2026-06-04',
      login_agreement_revision: 'rev-20260604',
      login_agreement_documents: [{ id: 'terms', title: '用户协议', content_md: 'terms' }],
    }))

    const wrapper = mountLoginView()
    await flushPromises()

    const emailInput = wrapper.get('#email')
    const passwordInput = wrapper.get('#password')
    expect(emailInput.attributes('disabled')).toBeUndefined()
    expect(passwordInput.attributes('disabled')).toBeUndefined()

    await emailInput.setValue('test@example.com')
    await passwordInput.setValue('password123')
    expect((emailInput.element as HTMLInputElement).value).toBe('test@example.com')
    expect((passwordInput.element as HTMLInputElement).value).toBe('password123')

    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(showWarningMock).toHaveBeenCalledWith('请先阅读并同意最新条款后再登录。')
    expect(loginMock).not.toHaveBeenCalled()
    expect(routerPushMock).not.toHaveBeenCalled()
  })

  it('modal 模式未同意协议时不在加载后遮挡输入，点击登录时再提示并打开协议弹窗', async () => {
    getPublicSettingsMock.mockResolvedValue(publicSettings({
      login_agreement_enabled: true,
      login_agreement_mode: 'modal',
      login_agreement_updated_at: '2026-06-04',
      login_agreement_revision: 'rev-modal-20260604',
      login_agreement_documents: [{ id: 'terms', title: '用户协议', content_md: 'terms' }],
    }))

    const wrapper = mountLoginView()
    await flushPromises()

    expect(document.body.textContent || '').not.toContain('条款更新通知')
    await wrapper.get('#email').setValue('modal@example.com')
    await wrapper.get('#password').setValue('password123')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(showWarningMock).toHaveBeenCalledWith('请先阅读并同意最新条款后再登录。')
    expect(document.body.textContent || '').toContain('条款更新通知')
    expect(loginMock).not.toHaveBeenCalled()
  })

  it('勾选登录协议后继续走正常登录流程', async () => {
    getPublicSettingsMock.mockResolvedValue(publicSettings({
      login_agreement_enabled: true,
      login_agreement_mode: 'checkbox',
      login_agreement_updated_at: '2026-06-04',
      login_agreement_revision: 'rev-accepted-20260604',
      login_agreement_documents: [{ id: 'terms', title: '用户协议', content_md: 'terms' }],
    }))
    loginMock.mockResolvedValue({
      access_token: 'token',
      token_type: 'Bearer',
      user: { id: 1, username: 'test', email: 'test@example.com', role: 'user', balance: 0, concurrency: 5, status: 'active', allowed_groups: null, created_at: '', updated_at: '' },
    })

    const wrapper = mountLoginView()
    await flushPromises()

    await wrapper.get('#email').setValue('test@example.com')
    await wrapper.get('#password').setValue('password123')
    await wrapper.get('#login-agreement-consent').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(loginMock).toHaveBeenCalledWith({
      email: 'test@example.com',
      password: 'password123',
      turnstile_token: undefined,
    })
    expect(showSuccessMock).toHaveBeenCalledWith('auth.loginSuccess')
    expect(routerPushMock).toHaveBeenCalledWith('/dashboard')
  })
})
