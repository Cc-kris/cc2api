import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ list: vi.fn(), versions: vi.fn(), create: vi.fn(), copy: vi.fn(), updateDraft: vi.fn(), testProtocol: vi.fn(), publish: vi.fn(), disable: vi.fn(), deleteDraft: vi.fn(), success: vi.fn(), error: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { upstreamFinanceProtocols: mocks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.success, showError: mocks.error }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))

import View from '../UpstreamFinanceProtocolsView.vue'

describe('UpstreamFinanceProtocolsView', () => {
  it('shows published protocols and loads the immutable version for editing', async () => {
    mocks.list.mockResolvedValue({ items: [{ id: 7, code: 'vendor_x', name: 'Vendor X', protocol_type: 'http_json', status: 'published', current_version_id: 71 }] })
    mocks.versions.mockResolvedValue([{ id: 71, protocol_id: 7, version: 1, config: { cost_mode: 'manual' }, validation_status: 'valid' }])
    const wrapper = mount(View)
    await flushPromises()
    expect(wrapper.text()).toContain('Vendor X')
    await wrapper.findAll('button').find(button => button.text() === '查看/编辑')!.trigger('click')
    await flushPromises()
    expect(mocks.versions).toHaveBeenCalledWith(7)
    expect(wrapper.findAll('textarea').some(item => item.element.value.includes('cost_mode'))).toBe(true)
    expect(wrapper.text()).toContain('仅随本次测试发送，不保存、不回显')
    expect(wrapper.text()).toContain('计费语义与能力')
    expect(wrapper.text()).toContain('接口与字段映射')
    expect(wrapper.text()).toContain('复制为草稿')
  })

  it('copies a published immutable version through the copy endpoint', async () => {
    mocks.list.mockResolvedValue({ items: [{ id: 7, code: 'vendor_x', name: 'Vendor X', protocol_type: 'http_json', status: 'published', current_version_id: 71 }] })
    mocks.versions.mockResolvedValue([{ id: 71, protocol_id: 7, version: 1, config: { capabilities: [], authentication: { type: 'none' }, operations: {}, cost_mode: 'manual' }, validation_status: 'valid' }])
    mocks.copy.mockResolvedValue({ id: 8, code: 'vendor_x_copy', name: 'Vendor X 副本', protocol_type: 'http_json', status: 'draft', current_version: { id: 81 } })
    const wrapper = mount(View)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '复制为草稿')!.trigger('click')
    await flushPromises()
    expect((wrapper.get('input[pattern]').element as HTMLInputElement).value).toBe('vendor_x_copy')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.copy).toHaveBeenCalledWith(7, expect.objectContaining({ code: 'vendor_x_copy' }))
  })
})
