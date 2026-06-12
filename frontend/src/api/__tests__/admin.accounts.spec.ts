import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    post,
  },
}))

import { batchTestActive } from '@/api/admin/accounts'

describe('admin accounts api', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('disables the client timeout for one-click batch account tests', async () => {
    post.mockResolvedValue({ data: { total: 0, passed: 0, failed: 0, results: [] } })

    await batchTestActive()

    expect(post).toHaveBeenCalledWith(
      '/admin/accounts/batch-test-active',
      undefined,
      { timeout: 0 }
    )
  })
})
