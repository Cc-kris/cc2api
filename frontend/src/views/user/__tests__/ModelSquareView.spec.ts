import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { listGroups, listModels, replace, showInfo, showSuccess } = vi.hoisted(() => ({
  listGroups: vi.fn(),
  listModels: vi.fn(),
  replace: vi.fn(),
  showInfo: vi.fn(),
  showSuccess: vi.fn(),
}))

const route = { query: {} as Record<string, string> }

vi.mock('@/api/modelSquare', () => ({
  default: { listGroups, listModels },
}))

vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ replace }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showInfo, showSuccess }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh-CN' } }),
  }
})

import ModelSquareView from '../ModelSquareView.vue'

const groupResult = {
  groups: [{
    id: 10,
    name: 'OpenAI',
    platform: 'openai',
    subscription_type: 'standard',
    default_multiplier: '1.2000',
    effective_multiplier: '1.1000',
    has_custom_multiplier: true,
    model_count: 1,
  }],
  catalog_updated_at: '2026-07-26T00:00:00Z',
}

const modelResult = {
  group_id: 10,
  group_name: 'OpenAI',
  effective_multiplier: '1.1000',
  items: [],
  next_cursor: null,
  catalog_updated_at: '2026-07-26T00:00:00Z',
}

describe('ModelSquareView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    route.query = {}
    listGroups.mockReset().mockResolvedValue(groupResult)
    listModels.mockReset().mockResolvedValue(modelResult)
    replace.mockReset().mockImplementation(async ({ query }: { query: Record<string, string> }) => { route.query = query })
    showInfo.mockReset()
    showSuccess.mockReset()
  })

  it('selects the first group, synchronizes URL, and loads its first 100 models', async () => {
    mount(ModelSquareView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          ModelSquareGroupList: true,
          ModelSquareTable: true,
          EmptyState: true,
          Icon: true,
        },
      },
    })
    await flushPromises()

    expect(replace).toHaveBeenCalledWith({ query: { group_id: '10' } })
    expect(listModels).toHaveBeenCalledWith(10, expect.objectContaining({ page_size: 100 }))
  })

  it('debounces model search and aborts the previous model request', async () => {
    route.query = { group_id: '10' }
    const wrapper = mount(ModelSquareView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          ModelSquareGroupList: true,
          ModelSquareTable: true,
          EmptyState: true,
          Icon: true,
        },
      },
    })
    await flushPromises()
    listModels.mockClear()

    await wrapper.get('input[type="search"]').setValue('gpt-5.5')
    await vi.advanceTimersByTimeAsync(299)
    expect(listModels).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    await flushPromises()

    expect(listModels).toHaveBeenCalledWith(10, expect.objectContaining({ q: 'gpt-5.5', page_size: 100 }))
  })
})
