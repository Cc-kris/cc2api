import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn().mockResolvedValue(true)
  })
}))

import UseKeyModal from '../UseKeyModal.vue'

describe('UseKeyModal', () => {
  it('renders Codex config as a single config.toml with inline bearer token', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://cc-ai.xyz/v1',
        platform: 'openai'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const codeBlocks = wrapper.findAll('pre code')
    expect(codeBlocks).toHaveLength(1)
    expect(wrapper.text()).toContain('~/.codex/config.toml')
    expect(wrapper.text()).not.toContain('auth.json')

    const configText = codeBlocks[0].text()
    expect(configText).toContain('model_provider = "ccai"')
    expect(configText).toContain('model = "gpt-5.5"')
    expect(configText).toContain('experimental_bearer_token = "sk-test"')
    expect(configText).toContain('base_url = "https://cc-ai.xyz"')
    expect(configText).toContain('supports_websockets = true')
    expect(configText).toContain('requires_openai_auth = false')
    expect(configText).toContain('http_headers = { "x-openai-actor-authorization" = "ccai" }')
    expect(configText).not.toContain('OPENAI_API_KEY')
  })

  it('renders Codex WebSocket tab with the same single config.toml style', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-ws-test',
        baseUrl: 'https://other.example/v1',
        platform: 'openai'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const codexWsTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.codexCliWs')
    )
    expect(codexWsTab).toBeDefined()
    await codexWsTab!.trigger('click')
    await nextTick()

    const codeBlocks = wrapper.findAll('pre code')
    expect(codeBlocks).toHaveLength(1)
    expect(wrapper.text()).toContain('~/.codex/config.toml')
    expect(wrapper.text()).not.toContain('auth.json')

    const configText = codeBlocks[0].text()
    expect(configText).toContain('model_provider = "ccai"')
    expect(configText).toContain('experimental_bearer_token = "sk-ws-test"')
    expect(configText).toContain('base_url = "https://cc-ai.xyz"')
    expect(configText).toContain('supports_websockets = true')
    expect(configText).toContain('http_headers = { "x-openai-actor-authorization" = "ccai" }')
    expect(configText).not.toContain('OPENAI_API_KEY')
  })

  it('keeps OpenAI Claude Code tab on Claude settings instead of Codex config', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-claude-client',
        baseUrl: 'https://cc-ai.xyz/v1',
        platform: 'openai',
        allowMessagesDispatch: true
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const claudeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.claudeCode')
    )
    expect(claudeTab).toBeDefined()
    await claudeTab!.trigger('click')
    await nextTick()

    const codeBlocks = wrapper.findAll('pre code')
    expect(codeBlocks).toHaveLength(2)
    expect(wrapper.text()).toContain('~/.claude/settings.json')
    expect(wrapper.text()).not.toContain('~/.codex/config.toml')
    expect(wrapper.text()).not.toContain('auth.json')

    const configText = codeBlocks.map((block) => block.text()).join('\n')
    expect(configText).toContain('ANTHROPIC_BASE_URL="https://cc-ai.xyz"')
    expect(configText).toContain('ANTHROPIC_AUTH_TOKEN="sk-claude-client"')
    expect(configText).not.toContain('experimental_bearer_token')
    expect(configText).not.toContain('OPENAI_API_KEY')
  })

  it('keeps OpenAI OpenCode tab on opencode.json instead of Codex config', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-opencode-client',
        baseUrl: 'https://cc-ai.xyz/v1',
        platform: 'openai',
        allowMessagesDispatch: true
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const opencodeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.opencode')
    )
    expect(opencodeTab).toBeDefined()
    await opencodeTab!.trigger('click')
    await nextTick()

    const codeBlocks = wrapper.findAll('pre code')
    expect(codeBlocks).toHaveLength(1)
    expect(wrapper.text()).toContain('opencode.json')
    expect(wrapper.text()).not.toContain('~/.codex/config.toml')
    expect(wrapper.text()).not.toContain('auth.json')

    const configText = codeBlocks[0].text()
    expect(configText).toContain('"ccai"')
    expect(configText).toContain('"baseURL": "https://cc-ai.xyz/v1"')
    expect(configText).toContain('"apiKey": "sk-opencode-client"')
    expect(configText).toContain('"model": "ccai/gpt-5.5"')
    expect(configText).not.toContain('experimental_bearer_token')
    expect(configText).not.toContain('OPENAI_API_KEY')
  })

  it('renders OpenAI Windows config path with Windows separators', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-windows',
        baseUrl: 'https://example.com/v1',
        platform: 'openai'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const windowsTab = wrapper.findAll('button').find((button) =>
      button.text().includes('Windows')
    )
    expect(windowsTab).toBeDefined()
    await windowsTab!.trigger('click')
    await nextTick()

    expect(wrapper.text()).toContain('%userprofile%\\.codex\\config.toml')
    expect(wrapper.text()).not.toContain('%userprofile%\\.codex/config.toml')
  })

  it('keeps OpenAI config guidance free of legacy auth file and env-token wording', () => {
    const guidance = [
      zh.keys.useKeyModal.openai.description,
      zh.keys.useKeyModal.openai.configTomlHint,
      zh.keys.useKeyModal.openai.note,
      zh.keys.useKeyModal.openai.noteWindows,
      en.keys.useKeyModal.openai.description,
      en.keys.useKeyModal.openai.configTomlHint,
      en.keys.useKeyModal.openai.note,
      en.keys.useKeyModal.openai.noteWindows
    ].join('\n')

    expect(guidance).toContain('config.toml')
    expect(guidance).not.toContain('auth.json')
    expect(guidance).not.toContain('OPENAI_API_KEY')
    expect(guidance).not.toContain('ANTHROPIC_AUTH_TOKEN')
  })

})
