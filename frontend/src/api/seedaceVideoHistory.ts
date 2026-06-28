import { apiClient } from './client'

export interface SeedaceVideoHistoryMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  status?: 'generating' | 'completed' | 'failed'
  taskId?: string
  error?: string
}

export interface SeedaceVideoHistoryRecord {
  id: string
  summary: string
  generationCount: number
  updatedAt: string
  messages: SeedaceVideoHistoryMessage[]
}

export async function listSeedaceVideoHistory(): Promise<SeedaceVideoHistoryRecord[]> {
  const { data } = await apiClient.get<{ items: SeedaceVideoHistoryRecord[] }>('/user/video-generations/history')
  return data.items || []
}

export async function saveSeedaceVideoHistory(record: SeedaceVideoHistoryRecord): Promise<SeedaceVideoHistoryRecord> {
  const { data } = await apiClient.put<SeedaceVideoHistoryRecord>(`/user/video-generations/history/${encodeURIComponent(record.id)}`, {
    summary: record.summary,
    generationCount: record.generationCount,
    messages: record.messages,
  })
  return data
}
