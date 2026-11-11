import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('@/api/admin', () => ({
  adminAPI: {
    usage: { searchUsers: vi.fn().mockResolvedValue([]), searchApiKeys: vi.fn().mockResolvedValue([]) },
    accounts: { list: vi.fn().mockResolvedValue({ items: [] }) },
    groups: { getAll: vi.fn().mockResolvedValue([]), list: vi.fn().mockResolvedValue({ items: [] }) },
    dashboard: { getModelStats: vi.fn().mockResolvedValue({ models: [] }) }
  }
}))

import UsageFilters from '../UsageFilters.vue'

const SelectStub = defineComponent({
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: `<select :data-testid="$attrs['data-testid']" @change="$emit('update:modelValue', JSON.parse($event.target.value)); $emit('change')">
    <option v-for="o in options" :key="String(o.value)" :value="JSON.stringify(o.value)">{{ o.label }}</option>
  </select>`
})

const mountFilters = (showAuditFilters: boolean, modelValue: Record<string, unknown> = {}) =>
  mount(UsageFilters, {
    props: { modelValue, exporting: false, startDate: '2026-11-01', endDate: '2026-11-05', showAuditFilters },
    global: { stubs: { Select: SelectStub, DateRangePicker: true, Icon: true } }
  })

describe('UsageFilters V03-R11 audit filters', () => {
  it('is hidden unless enabled (the cleanup dialog does not support it)', () => {
    const wrapper = mountFilters(false)
    expect(wrapper.find('[data-testid="usage-filter-upstream-request-id"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="usage-filter-upstream-model-mismatch"]').exists()).toBe(false)
  })

  it('applies the trimmed upstream request id on Enter and clears it when emptied', async () => {
    const model: Record<string, unknown> = {}
    const wrapper = mountFilters(true, model)
    const input = wrapper.get('[data-testid="usage-filter-upstream-request-id"]')

    await input.setValue('  req_abc  ')
    await input.trigger('keyup.enter')
    expect(model.upstream_request_id).toBe('req_abc')
    expect(wrapper.emitted('change')).toHaveLength(1)

    // 同值再次回车不重复查询
    await input.trigger('blur')
    expect(wrapper.emitted('change')).toHaveLength(1)

    await input.setValue('')
    await input.trigger('blur')
    expect(model.upstream_request_id).toBeUndefined()
    expect(wrapper.emitted('change')).toHaveLength(2)
  })

  it('offers all / mismatched / matched options for upstream_model_mismatch', () => {
    const wrapper = mountFilters(true)
    const options = wrapper.get('[data-testid="usage-filter-upstream-model-mismatch"]').findAll('option').map((o) => o.text())
    expect(options).toEqual([
      'admin.usage.upstreamModelMismatchAll',
      'admin.usage.upstreamModelMismatchOnly',
      'admin.usage.upstreamModelMismatchMatched'
    ])
  })

  it('shows the "all" option when upstream_model_mismatch is unset and writes back undefined', async () => {
    const model: Record<string, unknown> = {}
    const wrapper = mountFilters(true, model)
    const select = () => wrapper.findAllComponents(SelectStub).find((c) => c.attributes('data-testid') === 'usage-filter-upstream-model-mismatch')!
    // Select 按严格相等匹配选项；未设置时必须传 null 才能命中「全部」，否则显示「请选择」。
    expect(select().props('modelValue')).toBeNull()

    const el = select().get('select')
    await el.setValue(JSON.stringify(true))
    expect(model.upstream_model_mismatch).toBe(true)
    expect(select().props('modelValue')).toBe(true)

    await el.setValue(JSON.stringify(false))
    expect(model.upstream_model_mismatch).toBe(false)

    await el.setValue(JSON.stringify(null))
    expect(model.upstream_model_mismatch).toBeUndefined()
    expect('upstream_model_mismatch' in model && model.upstream_model_mismatch !== undefined).toBe(false)
    expect(select().props('modelValue')).toBeNull()
    expect(wrapper.emitted('change')).toHaveLength(3)
  })
})
