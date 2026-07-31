import { describe, expect, it, vi } from 'vitest'
import { resolveModelSquareEnabled, type ModelSquarePublicSettingsStore } from '../modelSquareGuard'
import type { PublicSettings } from '@/types'

function publicSettings(enabled: boolean): PublicSettings {
  return {
    model_square_enabled: enabled,
  } as PublicSettings
}

describe('resolveModelSquareEnabled', () => {
  it('waits for public settings before evaluating the route switch', async () => {
    let completeRequest: ((settings: PublicSettings) => void) | undefined
    const store: ModelSquarePublicSettingsStore = {
      publicSettingsLoaded: false,
      cachedPublicSettings: null,
      fetchPublicSettings: vi.fn(() => new Promise<PublicSettings>((resolve) => {
        completeRequest = (settings) => {
          store.cachedPublicSettings = settings
          store.publicSettingsLoaded = true
          resolve(settings)
        }
      })),
    }

    let settled = false
    const resultPromise = resolveModelSquareEnabled(store).then((result) => {
      settled = true
      return result
    })
    await Promise.resolve()
    expect(settled).toBe(false)

    completeRequest?.(publicSettings(true))
    await expect(resultPromise).resolves.toBe(true)
    expect(store.fetchPublicSettings).toHaveBeenCalledTimes(1)
  })

  it('fails closed when settings cannot be loaded', async () => {
    const store: ModelSquarePublicSettingsStore = {
      publicSettingsLoaded: false,
      cachedPublicSettings: null,
      fetchPublicSettings: vi.fn().mockResolvedValue(null),
    }

    await expect(resolveModelSquareEnabled(store)).resolves.toBe(false)
  })
})
