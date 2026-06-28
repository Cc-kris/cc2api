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

export function downloadSeedaceVideo(taskId: string, apiKeyId: number): void {
  const token = localStorage.getItem('auth_token')
  if (!token) throw new SeedaceVideoAPIError(401, '请先登录后再下载视频', {})

  const iframeName = `seedace-video-download-${crypto.randomUUID()}`
  const iframe = document.createElement('iframe')
  iframe.name = iframeName
  iframe.className = 'hidden'
  iframe.setAttribute('aria-hidden', 'true')

  const form = document.createElement('form')
  form.method = 'POST'
  form.action = `/api/v1/user/video-generations/${encodeURIComponent(taskId)}/download`
  form.target = iframeName
  form.className = 'hidden'

  appendHiddenInput(form, 'api_key_id', String(apiKeyId))
  appendHiddenInput(form, 'auth_token', token)

  document.body.appendChild(iframe)
  document.body.appendChild(form)
  form.submit()
  document.body.removeChild(form)
  setTimeout(() => {
    iframe.remove()
  }, 60_000)
}

function appendHiddenInput(form: HTMLFormElement, name: string, value: string) {
  const input = document.createElement('input')
  input.type = 'hidden'
  input.name = name
  input.value = value
  form.appendChild(input)
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
