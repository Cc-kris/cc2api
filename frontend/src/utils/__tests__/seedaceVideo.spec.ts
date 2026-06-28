import { describe, expect, it } from 'vitest'

import {
  buildSeedaceVideoPayload,
  coerceSeedaceResolution,
  getSeedaceResolutionOptions,
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
