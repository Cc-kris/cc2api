import { describe, expect, it, vi } from 'vitest'

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

import { buildModelMappingObject, getModelsByPlatform, getPresetMappingsByPlatform, splitModelMappingObject } from '../useModelWhitelist'

describe('useModelWhitelist', () => {
  it('openai 模型列表包含 GPT-5.4 官方快照', () => {
    const models = getModelsByPlatform('openai')

    expect(models).toContain('gpt-5.4')
    expect(models).toContain('gpt-5.4-mini')
    expect(models).toContain('gpt-5.4-2026-03-05')
    expect(models).toContain('codex-auto-review')
  })

  it('openai 模型列表不再暴露已下线的 ChatGPT 登录 Codex 模型', () => {
    const models = getModelsByPlatform('openai')

    expect(models).not.toContain('gpt-5')
    expect(models).not.toContain('gpt-5.1')
    expect(models).not.toContain('gpt-5.1-codex')
    expect(models).not.toContain('gpt-5.1-codex-max')
    expect(models).not.toContain('gpt-5.1-codex-mini')
    expect(models).not.toContain('gpt-5.2-codex')
  })

  it('antigravity 模型列表包含图片模型兼容项', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models).toContain('gemini-3-pro-image')
  })

  it('Grok 平台使用 xAI 模型目录而不是 Claude 默认目录', () => {
    const models = getModelsByPlatform('grok')

    expect(models).toContain('grok-4.6')
    expect(models).toContain('grok-4.5')
    expect(models).toContain('grok-build-0.1')
    expect(models).toContain('grok-imagine-video-1.5')
    expect(models.some(model => model.startsWith('claude-'))).toBe(false)
  })

  it('Grok 4.7 进入模型列表和预设，grok-latest 仍指向 4.6', () => {
    expect(getModelsByPlatform('grok')).toContain('grok-4.7')
    expect(getModelsByPlatform('grok')).toContain('grok-4.7-latest')
    const presets = getPresetMappingsByPlatform('grok')
    expect(presets).toContainEqual(expect.objectContaining({ from: 'grok-4.7', to: 'grok-4.7' }))
    expect(presets).toContainEqual(expect.objectContaining({ from: 'grok-4.7-latest', to: 'grok-4.7' }))
    expect(presets).toContainEqual(expect.objectContaining({ from: 'grok-latest', to: 'grok-4.6' }))
  })

  it('Grok 平台使用 Grok 预设映射', () => {
    const presets = getPresetMappingsByPlatform('grok')

    expect(presets).toContainEqual(expect.objectContaining({ from: 'grok-4.6', to: 'grok-4.6' }))
    expect(presets).toContainEqual(expect.objectContaining({ from: 'grok-latest', to: 'grok-4.6' }))
    expect(presets).toContainEqual(expect.objectContaining({ from: 'grok-imagine', to: 'grok-imagine-image-quality' }))
    expect(presets.some(preset => preset.from.startsWith('claude-'))).toBe(false)
  })

  it('gemini 模型列表包含原生生图模型', () => {
    const models = getModelsByPlatform('gemini')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.0-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
  })

  it('antigravity 模型列表会把新的 Gemini 图片模型排在前面', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash-lite'))
  })

  it('Claude 模型列表包含新发布的 Claude 模型', () => {
    const claude = getModelsByPlatform('claude')
    const ag = getModelsByPlatform('antigravity')
    for (const model of ['claude-fable-5-1', 'claude-fable-5', 'claude-opus-4-8', 'claude-opus-5', 'claude-opus-5-5', 'claude-sonnet-5', 'claude-sonnet-5-5', 'claude-haiku-5-5']) {
      expect(claude).toContain(model)
    }
    expect(ag).toContain('claude-fable-5-1')
    expect(ag).toContain('claude-fable-5')
    expect(ag).toContain('claude-opus-4-8')
    // Antigravity 上 5.5 只有档位 ID，简称走默认映射
    expect(ag).not.toContain('claude-opus-5-5')
    expect(ag).not.toContain('claude-sonnet-5-5')
    for (const tier of ['high', 'medium', 'low']) {
      expect(ag).toContain(`claude-opus-5-5-${tier}`)
      expect(ag).toContain(`claude-sonnet-5-5-${tier}`)
    }
  })

  it('Claude 新模型预设使用各平台的官方模型 ID', () => {
    expect(getPresetMappingsByPlatform('claude')).toEqual(expect.arrayContaining([
      expect.objectContaining({ label: 'Sonnet 5.5', from: 'claude-sonnet-5-5', to: 'claude-sonnet-5-5' }),
      expect.objectContaining({ label: 'Opus 5.5', from: 'claude-opus-5-5', to: 'claude-opus-5-5' }),
      expect.objectContaining({ label: 'Fable 5.1', from: 'claude-fable-5-1', to: 'claude-fable-5-1' }),
      expect.objectContaining({ label: 'Haiku 5.5', from: 'claude-haiku-5-5', to: 'claude-haiku-5-5' })
    ]))
    expect(getPresetMappingsByPlatform('bedrock')).toEqual(expect.arrayContaining([
      expect.objectContaining({ label: 'Sonnet 5.5', from: 'claude-sonnet-5-5', to: 'global.anthropic.claude-sonnet-5-5' }),
      expect.objectContaining({ label: 'Opus 5', from: 'claude-opus-5', to: 'us.anthropic.claude-opus-5-v1' }),
      expect.objectContaining({ label: 'Fable 5', from: 'claude-fable-5', to: 'anthropic.claude-fable-5' })
    ]))
    // 上游没有 opus-5-5、haiku-5-5 的 Bedrock ID
    const bedrockFroms = getPresetMappingsByPlatform('bedrock').map(p => p.from)
    expect(bedrockFroms).not.toContain('claude-opus-5-5')
    expect(bedrockFroms).not.toContain('claude-haiku-5-5')
    expect(getPresetMappingsByPlatform('antigravity')).toEqual(expect.arrayContaining([
      expect.objectContaining({ from: 'claude-opus-5-5', to: 'claude-opus-5-5-high' }),
      expect.objectContaining({ from: 'claude-sonnet-5-5', to: 'claude-sonnet-5-5-high' })
    ]))
  })

  it('Claude 默认白名单保存后允许 Haiku 5.5', () => {
    const mapping = buildModelMappingObject('whitelist', getModelsByPlatform('anthropic'), [])
    expect(mapping?.['claude-haiku-5-5']).toBe('claude-haiku-5-5')
  })

  it('Gemini 3.6/3.7/3.8 Flash 和 4 Argon 在 gemini 与 antigravity 平台使用对应真实模型 ID', () => {
    const gemini = getModelsByPlatform('gemini')
    for (const model of ['gemini-3.6-flash', 'gemini-3.7-flash', 'gemini-3.8-flash', 'gemini-4-argon']) {
      expect(gemini).toContain(model)
    }
    const ag = getModelsByPlatform('antigravity')
    for (const base of ['gemini-3.6-flash', 'gemini-3.7-flash', 'gemini-3.8-flash']) {
      for (const tier of ['', '-high', '-low', '-medium', '-tiered']) {
        expect(ag).toContain(base + tier)
      }
    }
    expect(ag).not.toContain('gemini-4-argon')
    for (const tier of ['high', 'medium', 'low']) {
      expect(ag).toContain(`gemini-4-argon-${tier}`)
    }
    expect(getPresetMappingsByPlatform('antigravity')).toEqual(expect.arrayContaining([
      expect.objectContaining({ from: 'gemini-4-argon', to: 'gemini-4-argon-high' }),
      expect.objectContaining({ from: 'gemini-4-argon-high', to: 'gemini-4-argon-high' }),
      expect.objectContaining({ from: 'gemini-4-argon-medium', to: 'gemini-4-argon-medium' }),
      expect.objectContaining({ from: 'gemini-4-argon-low', to: 'gemini-4-argon-low' }),
      expect.objectContaining({ from: 'gemini-3.8-flash', to: 'gemini-3.8-flash' })
    ]))
    expect(getPresetMappingsByPlatform('gemini')).toEqual(expect.arrayContaining([
      expect.objectContaining({ from: 'gemini-4-argon', to: 'gemini-4-argon' }),
      expect.objectContaining({ from: 'gemini-3.8-flash', to: 'gemini-3.8-flash' })
    ]))
  })

  it('whitelist 模式会忽略通配符条目', () => {
    const mapping = buildModelMappingObject('whitelist', ['claude-*', 'gemini-3.1-flash-image'], [])
    expect(mapping).toEqual({
      'gemini-3.1-flash-image': 'gemini-3.1-flash-image'
    })
  })

  it('whitelist 模式会保留 GPT-5.4 官方快照的精确映射', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-2026-03-05'], [])

    expect(mapping).toEqual({
      'gpt-5.4-2026-03-05': 'gpt-5.4-2026-03-05'
    })
  })

  it('whitelist keeps GPT-5.4 mini exact mappings', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-mini'], [])

    expect(mapping).toEqual({
      'gpt-5.4-mini': 'gpt-5.4-mini'
    })
  })

  it('combined 模式会同时保留白名单身份映射和模型映射', () => {
    const mapping = buildModelMappingObject(
      'combined',
      ['gpt-5.4', 'claude-*'],
      [
        { from: 'gpt-latest', to: 'gpt-5.4' },
        { from: 'gpt-5.4', to: 'gpt-5.4-mini' }
      ]
    )

    expect(mapping).toEqual({
      'gpt-5.4': 'gpt-5.4-mini',
      'gpt-latest': 'gpt-5.4'
    })
  })

  it('splitModelMappingObject 会把身份映射还原成白名单，其余保留为映射', () => {
    const parsed = splitModelMappingObject({
      'gpt-5.4': 'gpt-5.4',
      'gpt-latest': 'gpt-5.4',
      ' ': 'gpt-empty',
      broken: 123
    })

    expect(parsed).toEqual({
      allowedModels: ['gpt-5.4'],
      modelMappings: [{ from: 'gpt-latest', to: 'gpt-5.4' }]
    })
  })
})
