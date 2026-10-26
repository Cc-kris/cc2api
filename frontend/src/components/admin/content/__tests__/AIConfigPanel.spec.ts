import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { api } = vi.hoisted(() => ({ api: {
  listAIConfigs: vi.fn(), mutateAIConfig: vi.fn(), testAIConfig: vi.fn(), fetchAIModels: vi.fn(),
} }))
vi.mock('@/api/admin/content', () => ({ default: api }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'contentV04.ai.title': 'AI 创作',
      'contentV04.ai.configuration': '模型配置',
      'contentV04.ai.configureFirst': '请先配置模型',
      'contentV04.ai.provider': '厂商',
      'contentV04.ai.baseUrl': 'Base URL',
      'contentV04.ai.apiKey': 'API Key',
      'contentV04.ai.models': '模型',
      'contentV04.ai.manualModel': '手动输入模型名',
      'contentV04.ai.default': '默认',
      'contentV04.ai.fetchModels': '获取模型',
      'contentV04.ai.testConnection': '测试连通',
      'contentV04.ai.save': '保存配置',
      'contentV04.ai.maskedKey': 'Key 已掩码',
      'contentV04.ai.setDefault': '设为默认',
      'contentV04.ai.edit': '编辑配置',
      'contentV04.ai.delete': '删除配置',
      'contentV04.ai.deleteConfirm': '确认删除此配置？',
    } as Record<string, string>)[key] || key,
  }),
}))
import AIConfigPanel from '../AIConfigPanel.vue'

const saved = { id: 'cfg-default', label: 'Primary', provider: 'openai', baseUrl: 'https://api.example.com/v1', defaultModel: 'gpt-test', models: ['gpt-test'], isDefault: true, hasApiKey: true, apiKeyMasked: 'sk-****1234' }
const reply = (configs = [saved], revision = 'rev-1') => ({ data: { data: { configs, revision } }, headers: {} })
beforeEach(() => { vi.clearAllMocks(); api.listAIConfigs.mockResolvedValue(reply()); api.mutateAIConfig.mockResolvedValue(reply()); api.testAIConfig.mockResolvedValue({ data: { data: { ok: true, latencyMs: 12 } } }); api.fetchAIModels.mockResolvedValue({ data: { data: { models: ['model-a', 'model-b'] } } }) })

describe('AIConfigPanel', () => {
  it('loads existing configs without exposing the API key and preserves the masked key on edit', async () => {
    const wrapper = mount(AIConfigPanel); await flushPromises()
    expect(wrapper.text()).toContain('sk-****1234')
    expect(wrapper.text()).not.toContain('secret-key')
    expect(wrapper.get('button:disabled').text()).toContain('默认')
    await wrapper.get('button').trigger('click')
    await wrapper.get('ul button:nth-child(2)').trigger('click')
    await flushPromises()
    expect((wrapper.get('input[type="password"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.text()).toContain('API Key')
  })

  it('validates duplicate names and saves a new default with the current revision', async () => {
    api.listAIConfigs.mockResolvedValueOnce(reply()).mockResolvedValueOnce(reply([saved]))
    const wrapper = mount(AIConfigPanel); await flushPromises()
    await wrapper.get('button').trigger('click')
    const inputs = wrapper.findAll('form input')
    await inputs[0].setValue('Primary')
    await inputs[1].setValue('https://new.example/v1')
    await inputs[2].setValue('new-secret')
    await inputs[3].setValue('model-new')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.text()).toContain('配置名称不能重复')
    await inputs[0].setValue('Secondary')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.mutateAIConfig).toHaveBeenCalledWith('save', expect.objectContaining({ label: 'Secondary', apiKey: 'new-secret', defaultModel: 'model-new', isDefault: false }), 'rev-1')
  })

  it('fetches models, tests the connection, and requires confirmation before deleting an unused config', async () => {
    const secondary = { ...saved, id: 'cfg-2', label: 'Secondary', isDefault: false }
    api.listAIConfigs.mockResolvedValue(reply([saved, secondary]))
    const wrapper = mount(AIConfigPanel); await flushPromises()
    await wrapper.get('button').trigger('click')
    const buttons = () => wrapper.findAll('button')
    await buttons().find(b => b.text().includes('新增配置'))!.trigger('click')
    await buttons().find(b => b.text().includes('获取模型'))!.trigger('click'); await flushPromises()
    expect(api.fetchAIModels).toHaveBeenCalled()
    expect(wrapper.text()).toContain('已获取 2 个模型')
    await buttons().find(b => b.text().includes('测试连通'))!.trigger('click'); await flushPromises()
    expect(api.testAIConfig).toHaveBeenCalled()
    expect(wrapper.text()).toContain('连接成功')
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await wrapper.get('ul li:nth-child(2) button:last-child').trigger('click')
    expect(confirm).toHaveBeenCalledOnce()
    expect(api.mutateAIConfig).not.toHaveBeenCalled()
    confirm.mockRestore()
  })
})
