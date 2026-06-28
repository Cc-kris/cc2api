import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { downloadSeedaceVideo, SeedaceVideoAPIError } from './seedaceVideo'

describe('downloadSeedaceVideo', () => {
  let submitSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    localStorage.clear()
    submitSpy = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(function submit(this: HTMLFormElement) {
      this.dataset.submitted = 'true'
    })
    vi.useFakeTimers()
  })

  afterEach(() => {
    submitSpy.mockRestore()
    vi.useRealTimers()
    document.body.innerHTML = ''
    localStorage.clear()
  })

  it('uses a direct form post so the browser can start the download immediately', () => {
    localStorage.setItem('auth_token', 'jwt-token')
    const fetchSpy = vi.spyOn(globalThis, 'fetch')

    downloadSeedaceVideo('task abc/1', 12)

    expect(submitSpy).toHaveBeenCalledTimes(1)
    const submittedForm = submitSpy.mock.instances[0] as HTMLFormElement
    expect(submittedForm.method).toBe('post')
    expect(submittedForm.action).toBe(`${window.location.origin}/api/v1/user/video-generations/task%20abc%2F1/download`)
    expect(submittedForm.target).toMatch(/^seedace-video-download-/)
    expect((submittedForm.elements.namedItem('api_key_id') as HTMLInputElement).value).toBe('12')
    expect((submittedForm.elements.namedItem('auth_token') as HTMLInputElement).value).toBe('jwt-token')
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('throws a clear error when the user token is missing', () => {
    expect(() => downloadSeedaceVideo('task-1', 1)).toThrow(SeedaceVideoAPIError)
    expect(submitSpy).not.toHaveBeenCalled()
  })
})
