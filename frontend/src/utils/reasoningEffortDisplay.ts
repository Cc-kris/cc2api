// V03-R11：用量列表的推理强度显示。客户端请求值与实际转发值都有且不同时，分两行显示。
export interface ReasoningEffortRow {
  reasoning_effort?: string | null
  requested_reasoning_effort?: string | null
}

const normalize = (value: string | null | undefined) => (value ?? '').trim().toLowerCase().replace(/[-_\s]/g, '')

export function hasReasoningEffortMapping(row: ReasoningEffortRow): boolean {
  const requested = normalize(row.requested_reasoning_effort)
  const forwarded = normalize(row.reasoning_effort)
  return !!requested && !!forwarded && requested !== forwarded
}

// V03-R11：导出列「模型一致性」回答的是「是否一致」，与 upstream_model_mismatch 取反；未观测（null）留空。
export function upstreamModelConsistencyCell(
  mismatch: boolean | null | undefined,
  labels: { yes: string; no: string }
): string {
  if (mismatch == null) return ''
  return mismatch ? labels.no : labels.yes
}
