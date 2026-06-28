export interface SeedaceVideoAPIErrorPayload {
  message?: string
  error?: string | { message?: string; type?: string }
}

export class SeedaceVideoAPIError extends Error {
  status: number
  payload: unknown

  constructor(status: number, message: string, payload: unknown) {
    super(message)
    this.name = 'SeedaceVideoAPIError'
    this.status = status
    this.payload = payload
  }
}

export async function createSeedaceVideoTask(
  apiKey: string,
  payload: Record<string, unknown>,
  signal?: AbortSignal,
): Promise<unknown> {
  return requestSeedaceVideo('/v1/video/generations', apiKey, {
    method: 'POST',
    body: JSON.stringify(payload),
    signal,
  })
}

export async function pollSeedaceVideoTask(
  apiKey: string,
  taskId: string,
  signal?: AbortSignal,
): Promise<unknown> {
  return requestSeedaceVideo(`/v1/video/generations/${encodeURIComponent(taskId)}`, apiKey, {
    method: 'GET',
    signal,
  })
}

export async function downloadSeedaceVideo(taskId: string, apiKeyId: number): Promise<void> {
  const token = localStorage.getItem('auth_token')
  const response = await fetch(
    `/api/v1/user/video-generations/${encodeURIComponent(taskId)}/download?api_key_id=${encodeURIComponent(String(apiKeyId))}`,
    {
      method: 'GET',
      headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      credentials: 'include',
    },
  )
  if (!response.ok) {
    const payload = await readJSON(response)
    throw new SeedaceVideoAPIError(response.status, extractErrorMessage(payload) || '视频下载失败', payload)
  }

  const blob = await response.blob()
  const objectURL = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = objectURL
  link.download = `seedance-video-${taskId}.mp4`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  setTimeout(() => URL.revokeObjectURL(objectURL), 1000)
}

async function requestSeedaceVideo(
  url: string,
  apiKey: string,
  init: RequestInit,
): Promise<unknown> {
  const response = await fetch(url, {
    ...init,
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'Content-Type': 'application/json',
      ...(init.headers || {}),
    },
  })
  const payload = await readJSON(response)
  if (!response.ok) {
    throw new SeedaceVideoAPIError(response.status, extractErrorMessage(payload) || '视频生成接口请求失败', payload)
  }
  return payload
}

async function readJSON(response: Response): Promise<unknown> {
  const text = await response.text()
  if (!text) return {}
  try {
    return JSON.parse(text)
  } catch {
    return { message: text }
  }
}

function extractErrorMessage(payload: unknown): string {
  if (!payload || typeof payload !== 'object') return ''
  const record = payload as SeedaceVideoAPIErrorPayload
  if (typeof record.message === 'string') return record.message
  if (typeof record.error === 'string') return record.error
  if (record.error && typeof record.error === 'object' && typeof record.error.message === 'string') return record.error.message
  return ''
}
