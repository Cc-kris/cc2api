import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    post,
  },
}))

import { importAccounts } from '@/api/admin/channelMonitor'

describe('admin channel monitor API', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('calls the import accounts endpoint', async () => {
    post.mockResolvedValueOnce({
      data: {
        total_accounts: 3,
        created: 2,
        skipped_duplicate: 1,
        skipped_unsupported: 0,
      },
    })

    const result = await importAccounts()

    expect(post).toHaveBeenCalledWith('/admin/channel-monitors/import-accounts')
    expect(result.created).toBe(2)
  })
})
