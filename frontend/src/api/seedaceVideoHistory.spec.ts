import { beforeEach, describe, expect, it, vi } from 'vitest'

const getMock = vi.fn()
const putMock = vi.fn()

vi.mock('./client', () => ({
  apiClient: {
    get: getMock,
    put: putMock,
  },
}))

describe('seedaceVideoHistory api', () => {
  beforeEach(() => {
    getMock.mockReset()
    putMock.mockReset()
  })

  it('lists database-backed history records', async () => {
    getMock.mockResolvedValue({ data: { items: [{ id: 's1', summary: '摘要', generationCount: 1, updatedAt: '2026-06-28T00:00:00Z', messages: [] }] } })
    const { listSeedaceVideoHistory } = await import('./seedaceVideoHistory')

    const result = await listSeedaceVideoHistory()

    expect(getMock).toHaveBeenCalledWith('/user/video-generations/history')
    expect(result[0].id).toBe('s1')
  })

  it('saves a session history record by session id', async () => {
    putMock.mockResolvedValue({ data: { id: 'session 1', summary: '摘要', generationCount: 1, updatedAt: '2026-06-28T00:00:00Z', messages: [] } })
    const { saveSeedaceVideoHistory } = await import('./seedaceVideoHistory')

    await saveSeedaceVideoHistory({ id: 'session 1', summary: '摘要', generationCount: 1, updatedAt: '', messages: [] })

    expect(putMock).toHaveBeenCalledWith('/user/video-generations/history/session%201', {
      summary: '摘要',
      generationCount: 1,
      messages: [],
    })
  })
})
