import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageTable from '../UsageTable.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.request_id" :data-row="row.request_id">
        <slot name="cell-model" :row="row" :value="row.model" />
        <slot name="cell-reasoning_effort" :row="row" />
        <slot name="cell-upstream_request_id" :row="row" />
        <slot name="cell-session_id" :row="row" />
      </div>
    </div>
  `
}

const mountTable = (rows: Record<string, unknown>[]) =>
  mount(UsageTable, {
    props: { data: rows, loading: false, columns: [] },
    global: { stubs: { DataTable: DataTableStub, EmptyState: true, Icon: true, Teleport: true } }
  })

describe('admin UsageTable V03-R11 audit cells', () => {
  it('flags upstream model mismatch and shows the upstream response model', () => {
    const wrapper = mountTable([
      { request_id: 'r1', model: 'claude-sonnet-4', upstream_response_model: 'claude-sonnet-4-5', upstream_model_mismatch: true },
      { request_id: 'r2', model: 'gpt-5.1', upstream_response_model: 'gpt-5.1', upstream_model_mismatch: false },
      { request_id: 'r3', model: 'gpt-5.1' }
    ])
    const r1 = wrapper.find('[data-row="r1"]')
    expect(r1.find('[data-testid="usage-upstream-model-mismatch-badge"]').exists()).toBe(true)
    expect(r1.text()).toContain('claude-sonnet-4-5')
    // 一致且与发送模型相同：不重复显示
    expect(wrapper.find('[data-row="r2"] [data-testid="usage-upstream-response-model"]').exists()).toBe(false)
    expect(wrapper.find('[data-row="r3"] [data-testid="usage-upstream-response-model"]').exists()).toBe(false)
  })

  it('shows requested reasoning effort first and the forwarded value under ↳', () => {
    const wrapper = mountTable([
      { request_id: 'r1', model: 'gpt-5.4', requested_reasoning_effort: 'max', reasoning_effort: 'xhigh' },
      { request_id: 'r2', model: 'gpt-5.4', requested_reasoning_effort: null, reasoning_effort: 'high' }
    ])
    const mapped = wrapper.find('[data-row="r1"]').text()
    expect(mapped).toMatch(/Max[\s\S]*↳[\s\S]*XHigh/)
    const plain = wrapper.find('[data-row="r2"]').text()
    expect(plain).toContain('High')
    expect(plain).not.toContain('↳')
  })

  it('renders upstream request id and session id cells', () => {
    const wrapper = mountTable([
      { request_id: 'r1', model: 'm', upstream_request_id: 'req_abc', session_id: 'sess-1' },
      { request_id: 'r2', model: 'm' }
    ])
    expect(wrapper.find('[data-row="r1"] [data-testid="usage-upstream-request-id"]').text()).toBe('req_abc')
    expect(wrapper.find('[data-row="r1"]').text()).toContain('sess-1')
    expect(wrapper.find('[data-row="r2"] [data-testid="usage-upstream-request-id"]').exists()).toBe(false)
  })
})
