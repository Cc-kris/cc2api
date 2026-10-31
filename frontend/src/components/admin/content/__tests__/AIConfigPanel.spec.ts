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
import Select from '@/components/common/Select.vue'
import panelRaw from '../AIConfigPanel.vue?raw'

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

  const openCreate = async (wrapper: ReturnType<typeof mount>) => {
    await wrapper.findAll('button').find((b) => b.text().includes('新增配置'))!.trigger('click')
    await flushPromises()
  }
  const fillCreate = async (wrapper: ReturnType<typeof mount>, values: { label: string; baseUrl: string; apiKey: string; model: string }) => {
    const inputs = wrapper.findAll('form input')
    await inputs[0].setValue(values.label)
    await inputs[1].setValue(values.baseUrl)
    await inputs[2].setValue(values.apiKey)
    await inputs[3].setValue(values.model)
  }

  // AI-02：厂商下拉必须按 PRD R10 列全 7 项，且选项标签能渲染出文字
  it('AI-02 厂商下拉按 PRD R10 提供 7 项且标签非空', async () => {
    const wrapper = mount(AIConfigPanel); await flushPromises()
    await openCreate(wrapper)
    const select = wrapper.findComponent(Select)
    const options = select.props('options') as Array<{ value: string; label: string }>
    expect(options.map((o) => o.value)).toEqual(['openai', 'anthropic', 'deepseek', 'zhipu', 'moonshot', 'qwen', 'custom'])
    expect(options.every((o) => typeof o.label === 'string' && o.label.trim().length > 0)).toBe(true)
    expect(options.map((o) => o.label).join('|')).toContain('智谱')
    expect(options.map((o) => o.label).join('|')).toContain('月之暗面')
    expect(options.map((o) => o.label).join('|')).toContain('阿里通义')
    // 触发器要能显示当前厂商（旧实现 value-key="id" 导致这里空白）
    expect(select.find('.select-trigger').text()).toContain('OpenAI')
    expect(select.find('.select-trigger').text().trim().length).toBeGreaterThan(0)
  })

  // AI-02：中弹窗宽 640px（±4）、圆角 12px、必填项带红星
  it('AI-02 弹窗规格 640px/12px 与必填红星', async () => {
    expect(panelRaw).toMatch(/\.ai-config-dialog\s*\{[^}]*width:\s*min\(640px,\s*100%\)/)
    expect(panelRaw).toMatch(/\.ai-config-dialog\s*\{[^}]*border-radius:\s*12px/)
    const wrapper = mount(AIConfigPanel); await flushPromises()
    await openCreate(wrapper)
    // 新增时：配置名称 / 厂商 / Base URL / API Key / 模型 / 默认模型 = 6 个必填红星
    expect(wrapper.findAll('.ai-required').length).toBe(6)
    expect(wrapper.get('.ai-required').text()).toBe('*')
  })

  // AI-10：有未保存内容时 × / 取消 / Esc 都要先二次确认，确认丢才关闭并清空
  it('AI-10 脏数据关闭需二次确认（×/取消/Esc），确认后关闭且重开为空', async () => {
    const wrapper = mount(AIConfigPanel); await flushPromises()
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)

    // 未改动时点 × 不打扰用户
    await openCreate(wrapper)
    await wrapper.get('.ai-dialog-close').trigger('click')
    expect(confirm).not.toHaveBeenCalled()
    expect(wrapper.find('.ai-config-dialog').exists()).toBe(false)

    await openCreate(wrapper)
    await wrapper.get('.ai-config-dialog input').setValue('dirty-name')

    await wrapper.get('.ai-dialog-close').trigger('click')
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(wrapper.find('.ai-config-dialog').exists()).toBe(true)
    expect((wrapper.get('.ai-config-dialog input').element as HTMLInputElement).value).toBe('dirty-name')

    await wrapper.get('.ai-dialog-footer button').trigger('click')
    expect(confirm).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.ai-config-dialog').exists()).toBe(true)

    await wrapper.get('.ai-config-dialog').trigger('keydown', { key: 'Escape' })
    expect(confirm).toHaveBeenCalledTimes(3)
    expect(wrapper.find('.ai-config-dialog').exists()).toBe(true)

    // 遮罩（overlay 自身）同样走确认
    await wrapper.get('.ai-config-modal').trigger('mousedown')
    expect(confirm).toHaveBeenCalledTimes(4)
    expect(wrapper.find('.ai-config-dialog').exists()).toBe(true)

    confirm.mockReturnValue(true)
    await wrapper.get('.ai-dialog-close').trigger('click')
    expect(wrapper.find('.ai-config-dialog').exists()).toBe(false)
    await openCreate(wrapper)
    expect((wrapper.get('.ai-config-dialog input').element as HTMLInputElement).value).toBe('')
    confirm.mockRestore()
  })

  // AI-04：保存带 API Key 的配置后列表只渲染后端给的掩码
  it('AI-04 保存带 Key 的配置后列表只显示掩码', async () => {
    const created = { ...saved, id: 'cfg-ak', label: 'E2E-AK', isDefault: false, apiKeyMasked: 'sk-ab****yz' }
    const wrapper = mount(AIConfigPanel); await flushPromises()
    api.mutateAIConfig.mockResolvedValue(reply([saved, created]))
    api.listAIConfigs.mockResolvedValue(reply([saved, created]))
    await openCreate(wrapper)
    await fillCreate(wrapper, { label: 'E2E-AK', baseUrl: 'https://api.example.com/v1', apiKey: 'plain-secret-123', model: 'gpt-new' })
    await wrapper.get('form').trigger('submit'); await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('sk-ab****yz')
    expect(text).not.toContain('plain-secret-123')
    expect(text).toMatch(/\*{4}/)
  })

  // AI-04 兜底：后端即使把明文塞进列表，前端也只渲染掩码
  it('AI-04 后端未给掩码时不回显明文 Key', async () => {
    api.listAIConfigs.mockResolvedValue(reply([{ ...saved, apiKeyMasked: undefined, apiKey: 'leaked-plain-key' }]))
    const wrapper = mount(AIConfigPanel); await flushPromises()
    expect(wrapper.text()).not.toContain('leaked-plain-key')
    expect(wrapper.text()).toContain('****')
  })

  // AI-05 回归：拦截器把错误压成扁平对象后，409 自动重试必须仍然生效
  // （旧代码读 e.response.data → 恒为 undefined → 重试永不触发，用户只看到"保存失败，请刷新后重试"）
  it('AI-05 revision 冲突（扁平 409）自动刷新 revision 并重试成功', async () => {
    const created = { ...saved, id: 'cfg-9', label: 'Retry-OK', isDefault: false, models: ['gpt-new'] }
    api.mutateAIConfig
      .mockRejectedValueOnce({ status: 409, code: 409, reason: 'config_revision_conflict', message: 'AI config revision conflict' })
      .mockResolvedValueOnce(reply([saved, created], 'rev-2'))
    let listCalls = 0
    api.listAIConfigs.mockImplementation(async () => {
      listCalls += 1
      // 前两次（首屏加载 + 打开弹窗刷新）返回旧 revision，409 之后的重取才拿到新 revision
      return reply([saved], listCalls <= 2 ? 'rev-1' : 'rev-2')
    })
    const wrapper = mount(AIConfigPanel); await flushPromises()
    await openCreate(wrapper)
    await fillCreate(wrapper, { label: 'Retry-OK', baseUrl: 'https://api.example.com/v1', apiKey: 'secret-retry', model: 'gpt-new' })
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.mutateAIConfig).toHaveBeenCalledTimes(2)
    // 第一次用 rev-1，重试前已重新拉取列表拿到 rev-2
    expect(api.mutateAIConfig.mock.calls[0][2]).toBe('rev-1')
    expect(api.mutateAIConfig.mock.calls[1][2]).toBe('rev-2')
    expect(wrapper.text()).not.toContain('配置保存失败')
  })
})
