import apiClient from '../client'

export type FinanceProtocolStatus = 'draft' | 'published' | 'disabled'

export interface FinanceProtocolVersion {
  id: number
  protocol_id: number
  version: number
  config: Record<string, unknown>
  validation_status: string
  published_at?: string
}

export interface FinanceProtocol {
  id: number
  code: string
  name: string
  protocol_type: 'builtin' | 'http_json' | 'plugin'
  status: FinanceProtocolStatus
  current_version_id?: number
  current_version?: FinanceProtocolVersion
  updated_at: string
}

export interface FinanceProtocolInput {
  code: string
  name: string
  protocol_type: 'http_json' | 'plugin'
  config: Record<string, unknown>
}

interface Page<T> { items: T[]; total: number; page: number; page_size: number }

export async function list(status = ''): Promise<Page<FinanceProtocol>> {
  const { data } = await apiClient.get<Page<FinanceProtocol>>('/admin/upstream-finance-protocols', { params: { status, page: 1, page_size: 100 } })
  return data
}
export async function create(payload: FinanceProtocolInput): Promise<FinanceProtocol> {
  const { data } = await apiClient.post<FinanceProtocol>('/admin/upstream-finance-protocols', payload)
  return data
}
export async function copy(id: number, payload: FinanceProtocolInput): Promise<FinanceProtocol> {
  const { data } = await apiClient.post<FinanceProtocol>(`/admin/upstream-finance-protocols/${id}/copy`, payload)
  return data
}
export async function updateDraft(id: number, payload: Pick<FinanceProtocolInput, 'name' | 'config'>): Promise<FinanceProtocolVersion> {
  const { data } = await apiClient.put<FinanceProtocolVersion>(`/admin/upstream-finance-protocols/${id}/draft`, payload)
  return data
}
export async function versions(id: number): Promise<FinanceProtocolVersion[]> {
  const { data } = await apiClient.get<FinanceProtocolVersion[]>(`/admin/upstream-finance-protocols/${id}/versions`)
  return Array.isArray(data) ? data : []
}
export async function testProtocol(id: number, baseUrl: string, credential: string, operation: string): Promise<Record<string, unknown>> {
  const { data } = await apiClient.post<Record<string, unknown>>(`/admin/upstream-finance-protocols/${id}/test`, { base_url: baseUrl, credential, operation })
  return data
}
export async function publish(id: number, versionId: number): Promise<void> { await apiClient.post(`/admin/upstream-finance-protocols/${id}/publish`, { version_id: versionId }) }
export async function disable(id: number): Promise<void> { await apiClient.post(`/admin/upstream-finance-protocols/${id}/disable`) }
export async function deleteDraft(id: number): Promise<void> { await apiClient.delete(`/admin/upstream-finance-protocols/${id}/draft`) }

export default { list, create, copy, updateDraft, versions, testProtocol, publish, disable, deleteDraft }
