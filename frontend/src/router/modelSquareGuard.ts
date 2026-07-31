import type { PublicSettings } from '@/types'

export interface ModelSquarePublicSettingsStore {
  publicSettingsLoaded: boolean
  cachedPublicSettings: PublicSettings | null
  fetchPublicSettings: () => Promise<PublicSettings | null>
}

export async function resolveModelSquareEnabled(store: ModelSquarePublicSettingsStore): Promise<boolean> {
  if (!store.publicSettingsLoaded) {
    await store.fetchPublicSettings()
  }
  return store.cachedPublicSettings?.model_square_enabled === true
}
