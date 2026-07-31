import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/api/client', () => ({ apiClient: { get } }))

import { listModelSquareGroups, listModelSquareModels } from '@/api/modelSquare'

describe('model square API', () => {
  beforeEach(() => get.mockReset())

  it('loads groups with the supplied abort signal', async () => {
    const controller = new AbortController()
    get.mockResolvedValueOnce({ data: { groups: [], catalog_updated_at: '2026-07-26T00:00:00Z' } })

    await listModelSquareGroups(controller.signal)

    expect(get).toHaveBeenCalledWith('/model-square/groups', { signal: controller.signal })
  })

  it('loads a group model page without leaking signal into query params', async () => {
    const controller = new AbortController()
    get.mockResolvedValueOnce({ data: { group_id: 12, items: [], next_cursor: null } })

    await listModelSquareModels(12, {
      q: 'gpt',
      cursor: 'signed',
      page_size: 100,
      catalog_updated_at: '2026-07-26T00:00:00Z',
      signal: controller.signal,
    })

    expect(get).toHaveBeenCalledWith('/model-square/groups/12/models', {
      params: {
        q: 'gpt',
        cursor: 'signed',
        page_size: 100,
        catalog_updated_at: '2026-07-26T00:00:00Z',
      },
      signal: controller.signal,
    })
  })
})
