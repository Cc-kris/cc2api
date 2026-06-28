export type SeedaceVideoModelOption =
  | 'domestic-seedance-2.0'
  | 'international-seedance-2.0'
  | 'international-seedance-2.0-fast'

export type SeedaceVideoResolution = '480p' | '720p' | '1080p'
export type SeedaceVideoAspectRatio = '16:9' | '9:16' | '1:1' | '4:3' | '3:4' | '21:9'
export type SeedaceVideoDuration = 10 | 15
export type SeedaceVideoFrameMode = 'none' | 'start_frame' | 'start_end'

export interface SeedaceVideoReferenceAsset {
  name: string
  dataUrl: string
  mimeType: string
  durationSeconds?: number
}

export interface SeedaceVideoFormState {
  apiKeyId: number | null
  modelOption: SeedaceVideoModelOption | ''
  resolution: SeedaceVideoResolution | ''
  aspectRatio: SeedaceVideoAspectRatio
  duration: SeedaceVideoDuration
  referenceImages: SeedaceVideoReferenceAsset[]
  referenceVideos: SeedaceVideoReferenceAsset[]
  referenceAudios: SeedaceVideoReferenceAsset[]
  frameMode: SeedaceVideoFrameMode
  firstFrame: SeedaceVideoReferenceAsset | null
  lastFrame: SeedaceVideoReferenceAsset | null
  generateAudio: boolean
}

interface ModelOptionConfig {
  value: SeedaceVideoModelOption
  label: string
  scope: 'domestic' | 'international'
  family: string
  resolutions: SeedaceVideoResolution[]
  defaultResolution: SeedaceVideoResolution
}

export const SEEDACE_VIDEO_MODEL_OPTIONS: ModelOptionConfig[] = [
  {
    value: 'domestic-seedance-2.0',
    label: '国内-Seedance 2.0',
    scope: 'domestic',
    family: 'seedance-2.0',
    resolutions: ['720p', '1080p'],
    defaultResolution: '720p',
  },
  {
    value: 'international-seedance-2.0',
    label: '国际-Seedance 2.0',
    scope: 'international',
    family: 'seedance-2-0',
    resolutions: ['480p', '720p', '1080p'],
    defaultResolution: '720p',
  },
  {
    value: 'international-seedance-2.0-fast',
    label: '国际-Seedance 2.0 Fast',
    scope: 'international',
    family: 'seedance-2-0-fast',
    resolutions: ['480p', '720p'],
    defaultResolution: '720p',
  },
]

export const SEEDACE_VIDEO_ASPECT_RATIO_OPTIONS: SeedaceVideoAspectRatio[] = ['16:9', '9:16', '1:1', '4:3', '3:4', '21:9']
export const SEEDACE_VIDEO_DURATION_OPTIONS: SeedaceVideoDuration[] = [10, 15]

export function getSeedaceModelOption(value: SeedaceVideoModelOption | ''): ModelOptionConfig | null {
  return SEEDACE_VIDEO_MODEL_OPTIONS.find((option) => option.value === value) ?? null
}

export function getSeedaceResolutionOptions(value: SeedaceVideoModelOption | ''): SeedaceVideoResolution[] {
  return getSeedaceModelOption(value)?.resolutions ?? []
}

export function coerceSeedaceResolution(
  modelOption: SeedaceVideoModelOption | '',
  resolution: SeedaceVideoResolution | ''
): SeedaceVideoResolution | '' {
  const option = getSeedaceModelOption(modelOption)
  if (!option) return ''
  return option.resolutions.includes(resolution as SeedaceVideoResolution) ? (resolution as SeedaceVideoResolution) : option.defaultResolution
}

export function hasSeedaceReferenceAssets(form: Pick<SeedaceVideoFormState, 'referenceImages' | 'referenceVideos' | 'referenceAudios' | 'firstFrame' | 'lastFrame'>): boolean {
  return (
    form.referenceImages.length > 0 ||
    form.referenceVideos.length > 0 ||
    form.referenceAudios.length > 0 ||
    form.firstFrame !== null ||
    form.lastFrame !== null
  )
}

export function hasSeedaceImageReference(form: Pick<SeedaceVideoFormState, 'referenceImages' | 'firstFrame' | 'lastFrame'>): boolean {
  return form.referenceImages.length > 0 || form.firstFrame !== null || form.lastFrame !== null
}

export function resolveSeedaceUpstreamModel(form: SeedaceVideoFormState): string {
  const option = getSeedaceModelOption(form.modelOption)
  if (!option) {
    throw new Error('请选择模型')
  }
  if (!form.resolution || !option.resolutions.includes(form.resolution)) {
    throw new Error('当前模型不支持该分辨率')
  }
  if (option.scope === 'domestic') {
    return `${option.family}-${form.resolution.replace('p', '')}`
  }
  const refSuffix = hasSeedaceReferenceAssets(form) ? '-ref' : ''
  return `dreamina-${option.family}-${form.resolution}${refSuffix}`
}

export function buildSeedaceVideoPayload(form: SeedaceVideoFormState, prompt: string): Record<string, unknown> {
  const option = getSeedaceModelOption(form.modelOption)
  if (!option) {
    throw new Error('请选择模型')
  }

  const payload: Record<string, unknown> = {
    model: resolveSeedaceUpstreamModel(form),
    prompt: prompt.trim(),
    aspect_ratio: form.aspectRatio,
  }

  if (option.scope === 'domestic') {
    payload.seconds = String(form.duration)
    attachDomesticReferences(payload, form)
  } else {
    payload.duration = form.duration
    attachInternationalReferences(payload, form)
  }

  if (!form.generateAudio) {
    payload.generate_audio = false
  }

  return payload
}

function attachDomesticReferences(payload: Record<string, unknown>, form: SeedaceVideoFormState): void {
  if (form.frameMode === 'start_frame') {
    if (form.firstFrame) {
      payload.image_url = form.firstFrame.dataUrl
      payload.video_config = { reference_mode: 'start_frame' }
    }
  } else if (form.frameMode === 'start_end') {
    if (form.firstFrame && form.lastFrame) {
      payload.reference_image_urls = [form.firstFrame.dataUrl, form.lastFrame.dataUrl]
      payload.video_config = { reference_mode: 'start_end' }
    }
  } else if (form.referenceImages.length === 1) {
    payload.image_url = form.referenceImages[0].dataUrl
  } else if (form.referenceImages.length > 1) {
    payload.reference_image_urls = form.referenceImages.map((asset) => asset.dataUrl)
  }

  if (form.referenceVideos.length > 0) {
    payload.reference_videos = form.referenceVideos.map((asset) => asset.dataUrl)
  }
  attachAudioReferences(payload, form)
}

function attachInternationalReferences(payload: Record<string, unknown>, form: SeedaceVideoFormState): void {
  if (form.frameMode === 'start_frame') {
    if (form.firstFrame) {
      payload.first_frame = form.firstFrame.dataUrl
    }
  } else if (form.frameMode === 'start_end') {
    if (form.firstFrame) payload.first_frame = form.firstFrame.dataUrl
    if (form.lastFrame) payload.last_frame = form.lastFrame.dataUrl
  } else if (form.referenceImages.length === 1) {
    payload.image = form.referenceImages[0].dataUrl
  } else if (form.referenceImages.length > 1) {
    payload.images = form.referenceImages.map((asset) => asset.dataUrl)
  }

  if (form.referenceVideos.length > 0) {
    payload.reference_videos = form.referenceVideos.map((asset) => asset.dataUrl)
  }
  attachAudioReferences(payload, form)
}

function attachAudioReferences(payload: Record<string, unknown>, form: SeedaceVideoFormState): void {
  if (form.referenceAudios.length === 1) {
    payload.audio_url = form.referenceAudios[0].dataUrl
  } else if (form.referenceAudios.length > 1) {
    payload.reference_audios = form.referenceAudios.map((asset) => asset.dataUrl)
  }
}

export function validateSeedaceVideoForm(form: SeedaceVideoFormState, prompt: string): string[] {
  const errors: string[] = []
  const option = getSeedaceModelOption(form.modelOption)

  if (!form.apiKeyId) errors.push('请选择 API Key')
  if (!option) errors.push('请选择模型')
  if (!form.resolution) errors.push('请选择分辨率')
  if (option && form.resolution && !option.resolutions.includes(form.resolution)) errors.push('当前模型不支持该分辨率')
  if (!form.aspectRatio) errors.push('请选择视频比例')
  if (!SEEDACE_VIDEO_DURATION_OPTIONS.includes(form.duration)) errors.push('请选择视频时间')
  if (!prompt.trim()) errors.push('请输入视频生成内容')
  if (form.referenceImages.length > 9) errors.push('参考图不能超过 9 张')
  if (form.referenceVideos.length > 3) errors.push('参考视频不能超过 3 个')
  if (form.referenceAudios.length > 3) errors.push('参考音频不能超过 3 个')
  if (form.referenceVideos.some((asset) => (asset.durationSeconds ?? 0) >= 15)) errors.push('参考视频必须小于 15 秒')
  if (form.referenceAudios.length > 0 && !hasSeedaceImageReference(form)) errors.push('上传参考音频时，需要至少上传 1 张参考图')
  if (form.frameMode === 'start_frame' && !form.firstFrame) errors.push('首帧参考模式需要上传首帧图')
  if (form.frameMode === 'start_end') {
    if (!form.firstFrame) errors.push('首尾帧参考模式需要上传首帧图')
    if (!form.lastFrame) errors.push('首尾帧参考模式需要上传尾帧图')
  }

  return errors
}

export function extractSeedaceTaskId(payload: unknown): string {
  return firstStringByKeys(payload, ['task_id', 'id'])
}

export function extractSeedaceVideoUrl(payload: unknown): string {
  return firstStringByKeys(payload, ['video_url', 'result_url', 'url', 'download_url'])
}

export function normalizeSeedaceTaskStatus(payload: unknown): 'pending' | 'running' | 'success' | 'failed' | 'unknown' {
  const status = firstStringByKeys(payload, ['status', 'state']).toLowerCase()
  if (['success', 'succeeded', 'completed', 'complete', 'done'].includes(status)) return 'success'
  if (['failed', 'fail', 'error', 'cancelled', 'canceled'].includes(status)) return 'failed'
  if (['running', 'processing', 'generating', 'in_progress'].includes(status)) return 'running'
  if (['pending', 'queued', 'created'].includes(status)) return 'pending'
  return 'unknown'
}

function firstStringByKeys(value: unknown, keys: string[]): string {
  if (!value || typeof value !== 'object') return ''
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = firstStringByKeys(item, keys)
      if (found) return found
    }
    return ''
  }
  const record = value as Record<string, unknown>
  for (const key of keys) {
    const raw = record[key]
    if (typeof raw === 'string' && raw.trim()) return raw.trim()
  }
  for (const child of Object.values(record)) {
    const found = firstStringByKeys(child, keys)
    if (found) return found
  }
  return ''
}

export function isAllowedSeedaceAudioFile(file: File): boolean {
  const type = file.type.toLowerCase()
  const name = file.name.toLowerCase()
  return type.startsWith('audio/') || /\.(mp3|wav|m4a|aac|flac|ogg)$/i.test(name)
}
