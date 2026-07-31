import axios from 'axios'
import { computed, onBeforeUnmount, ref } from 'vue'
import modelSquareAPI, {
  type ModelSquareGroup,
  type ModelSquareModel,
} from '@/api/modelSquare'
import { extractApiErrorCode } from '@/utils/apiError'

export function useModelSquare() {
  const groups = ref<ModelSquareGroup[]>([])
  const items = ref<ModelSquareModel[]>([])
  const groupsLoading = ref(false)
  const modelsLoading = ref(false)
  const loadingMore = ref(false)
  const groupsError = ref<unknown>(null)
  const modelsError = ref<unknown>(null)
  const nextCursor = ref<string | null>(null)
  const catalogUpdatedAt = ref<string | null>(null)

  let groupsController: AbortController | null = null
  let modelsController: AbortController | null = null

  const hasMore = computed(() => nextCursor.value !== null)

  async function loadGroups() {
    groupsController?.abort()
    groupsController = new AbortController()
    groupsLoading.value = true
    groupsError.value = null
    try {
      const result = await modelSquareAPI.listGroups(groupsController.signal)
      groups.value = result.groups
      catalogUpdatedAt.value = result.catalog_updated_at
      return result
    } catch (error) {
      if (!axios.isCancel(error)) groupsError.value = error
      throw error
    } finally {
      groupsLoading.value = false
    }
  }

  function resetModels() {
    modelsController?.abort()
    modelsController = null
    items.value = []
    nextCursor.value = null
    modelsError.value = null
  }

  async function loadModels(groupId: number, search: string, append = false) {
    if (append && (!nextCursor.value || loadingMore.value)) return
    if (!append) {
      modelsController?.abort()
      items.value = []
      nextCursor.value = null
      modelsLoading.value = true
    } else {
      loadingMore.value = true
    }
    const controller = new AbortController()
    modelsController = controller
    modelsError.value = null
    try {
      const result = await modelSquareAPI.listModels(groupId, {
        q: search || undefined,
        cursor: append ? nextCursor.value ?? undefined : undefined,
        page_size: 100,
        catalog_updated_at: append ? catalogUpdatedAt.value ?? undefined : undefined,
        signal: controller.signal,
      })
      if (controller.signal.aborted) return
      items.value = append ? [...items.value, ...result.items] : result.items
      nextCursor.value = result.next_cursor
      catalogUpdatedAt.value = result.catalog_updated_at
      return result
    } catch (error) {
      if (axios.isCancel(error)) return
      if (extractApiErrorCode(error) === 'CATALOG_CHANGED') throw error
      modelsError.value = error
      throw error
    } finally {
      if (!controller.signal.aborted) {
        modelsLoading.value = false
        loadingMore.value = false
      }
    }
  }

  function abortAll() {
    groupsController?.abort()
    modelsController?.abort()
  }

  onBeforeUnmount(abortAll)

  return {
    groups,
    items,
    groupsLoading,
    modelsLoading,
    loadingMore,
    groupsError,
    modelsError,
    nextCursor,
    catalogUpdatedAt,
    hasMore,
    loadGroups,
    loadModels,
    resetModels,
    abortAll,
  }
}
