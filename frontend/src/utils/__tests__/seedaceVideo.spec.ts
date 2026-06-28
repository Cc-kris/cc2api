import { describe, expect, it } from 'vitest'

import {
  buildSeedaceVideoPayload,
  coerceSeedaceResolution,
  createSeedaceSessionSummary,
  extractSeedaceVideoUrl,
  getSeedaceResolutionOptions,
  isSeedaceVideoCompleted,
  normalizeSeedaceTaskStatus,
  resolveSeedaceUpstreamModel,
  validateSeedaceVideoForm,
  type SeedaceVideoFormState,
} from '../seedaceVideo'

const asset = { name: 'ref.png', mimeType: 'image/png', dataUrl: 'data:image/png;base64,aaa' }

function createForm(overrides: Partial<SeedaceVideoFormState> = {}): SeedaceVideoFormState {
  return {
    apiKeyId: 1,
    modelOption: 'domestic-seedance-2.0',
    resolution: '720p',
    aspectRatio: '16:9',
    duration: 10,
    referenceImages: [],
    referenceVideos: [],
    referenceAudios: [],
    frameMode: 'none',
    firstFrame: null,
    lastFrame: null,
    generateAudio: true,
    ...overrides,
  }
}

describe('seedaceVideo model mapping', () => {
  it('uses source-prefixed UI model option and domestic resolution to build domestic upstream model', () => {
    const form = createForm({ modelOption: 'domestic-seedance-2.0', resolution: '1080p' })

    expect(resolveSeedaceUpstreamModel(form)).toBe('seedance-2.0-1080')
  })

  it('builds international standard model without exposing dreamina in UI options', () => {
    const form = createForm({ modelOption: 'international-seedance-2.0', resolution: '720p' })

    expect(resolveSeedaceUpstreamModel(form)).toBe('dreamina-seedance-2-0-720p')
  })

  it('adds ref suffix for international models only when references exist', () => {
    const form = createForm({
      modelOption: 'international-seedance-2.0-fast',
      resolution: '480p',
      referenceImages: [asset],
    })

    expect(resolveSeedaceUpstreamModel(form)).toBe('dreamina-seedance-2-0-fast-480p-ref')
  })

  it('returns dynamic resolution options and coerces unsupported resolution', () => {
    expect(getSeedaceResolutionOptions('domestic-seedance-2.0')).toEqual(['720p', '1080p'])
    expect(getSeedaceResolutionOptions('international-seedance-2.0-fast')).toEqual(['480p', '720p'])
    expect(coerceSeedaceResolution('international-seedance-2.0-fast', '1080p')).toBe('720p')
  })
})

describe('seedaceVideo task response helpers', () => {
  it('treats documented completed and SUCCESS statuses as completed even before extracting a URL', () => {
    expect(isSeedaceVideoCompleted({ status: 'completed' })).toBe(true)
    expect(isSeedaceVideoCompleted({ data: { status: 'SUCCESS' } })).toBe(true)
    expect(normalizeSeedaceTaskStatus({ data: { status: 'in_progress' } })).toBe('running')
  })

  it('extracts video URLs from nested fallback fields and arrays', () => {
    expect(extractSeedaceVideoUrl({ data: { data: { result_url: 'https://cdn.example.com/a.mp4' } } })).toBe('https://cdn.example.com/a.mp4')
    expect(extractSeedaceVideoUrl({ output: [{ download_url: 'https://cdn.example.com/b.mp4' }] })).toBe('https://cdn.example.com/b.mp4')
  })

  it('builds a single 10-character session summary from the latest input', () => {
    expect(createSeedaceSessionSummary('  生成一个雨夜城市霓虹灯慢镜头  ')).toBe('生成一个雨夜城市霓虹')
    expect(createSeedaceSessionSummary('!!!')).toBe('未命名会话')
  })
})

describe('seedaceVideo payload and validation', () => {
  it('uses seconds string for domestic models', () => {
    const payload = buildSeedaceVideoPayload(createForm({ duration: 15 }), '一只猫在奔跑')

    expect(payload).toMatchObject({
      model: 'seedance-2.0-720',
      prompt: '一只猫在奔跑',
      seconds: '15',
      aspect_ratio: '16:9',
    })
    expect(payload).not.toHaveProperty('duration')
  })

  it('uses numeric duration and first/last frame fields for international models', () => {
    const payload = buildSeedaceVideoPayload(
      createForm({
        modelOption: 'international-seedance-2.0',
        frameMode: 'start_end',
        firstFrame: asset,
        lastFrame: { ...asset, name: 'last.png' },
        generateAudio: false,
      }),
      '日落海面',
    )

    expect(payload).toMatchObject({
      model: 'dreamina-seedance-2-0-720p-ref',
      duration: 10,
      first_frame: asset.dataUrl,
      last_frame: asset.dataUrl,
      generate_audio: false,
    })
    expect(payload).not.toHaveProperty('seconds')
  })

  it('blocks audio without image reference before sending request', () => {
    const errors = validateSeedaceVideoForm(
      createForm({ referenceAudios: [{ name: 'a.mp3', mimeType: 'audio/mpeg', dataUrl: 'data:audio/mpeg;base64,aaa' }] }),
      '生成视频',
    )

    expect(errors).toContain('上传参考音频时，需要至少上传 1 张参考图')
  })
})
