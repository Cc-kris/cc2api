export type LatencyHealth = 'missing' | 'good' | 'slow' | 'critical' | 'error'

function trimDecimal(value: number): string {
  return value.toFixed(2).replace(/\.00$/, '').replace(/(\.\d)0$/, '$1')
}

export function formatDuration(ms: number | null | undefined): string {
  if (ms == null || !Number.isFinite(ms) || ms < 0) return '-'
  if (ms < 60_000) return `${trimDecimal(ms / 1000)}s`
  const totalSeconds = Math.floor(ms / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours > 0) return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`
  return seconds > 0 ? `${minutes}m ${seconds}s` : `${minutes}m`
}

export function latencyHealth(durationMs: number | null | undefined, isError = false): { state: LatencyHealth; width: number } {
  const width = durationMs == null ? 0 : Math.min(Math.max(durationMs / 30_000 * 100, 0), 100)
  if (isError) return { state: 'error', width }
  if (durationMs == null || !Number.isFinite(durationMs) || durationMs < 0) return { state: 'missing', width: 0 }
  const state: LatencyHealth = durationMs <= 5_000 ? 'good' : durationMs <= 30_000 ? 'slow' : 'critical'
  return { state, width }
}
