import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, del } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), del: vi.fn() }))
vi.mock('@/api/client', () => ({ default: { get, post, put, delete: del } }))

import api from '@/api/admin/upstreamFinanceProtocols'

describe('upstream finance protocol API', () => {
  beforeEach(() => { get.mockReset(); post.mockReset(); put.mockReset(); del.mockReset() })

  it('lists published protocols and immutable versions', async () => {
    get.mockResolvedValueOnce({ data: { items: [{ id: 1 }], total: 1 } }).mockResolvedValueOnce({ data: [{ id: 11, version: 1 }] })
    expect((await api.list('published')).items).toHaveLength(1)
    expect(await api.versions(1)).toEqual([{ id: 11, version: 1 }])
    expect(get).toHaveBeenNthCalledWith(1, '/admin/upstream-finance-protocols', { params: { status: 'published', page: 1, page_size: 100 } })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/upstream-finance-protocols/1/versions')
  })

  it('does not persist a one-time test credential through draft update', async () => {
    post.mockResolvedValueOnce({ data: { facts: { balance: '10' } } })
    await api.testProtocol(1, 'https://vendor.example', 'one-time-secret', 'balance')
    expect(post).toHaveBeenCalledWith('/admin/upstream-finance-protocols/1/test', { base_url: 'https://vendor.example', credential: 'one-time-secret', operation: 'balance' })
    expect(put).not.toHaveBeenCalled()
  })

  it('copies an immutable version into a new draft protocol', async () => {
    const payload = { code: 'vendor_copy', name: 'Vendor Copy', protocol_type: 'http_json' as const, config: { cost_mode: 'manual' } }
    post.mockResolvedValueOnce({ data: { id: 2, ...payload, status: 'draft' } })
    await api.copy(1, payload)
    expect(post).toHaveBeenCalledWith('/admin/upstream-finance-protocols/1/copy', payload)
  })
})
