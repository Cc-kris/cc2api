import { describe, expect, it } from 'vitest'
import { upstreamModelConsistencyCell } from '../reasoningEffortDisplay'

const labels = { yes: '是', no: '否' }

describe('upstreamModelConsistencyCell', () => {
  it('answers "is the model consistent" (inverse of upstream_model_mismatch)', () => {
    expect(upstreamModelConsistencyCell(true, labels)).toBe('否')
    expect(upstreamModelConsistencyCell(false, labels)).toBe('是')
  })

  it('leaves unobserved rows empty', () => {
    expect(upstreamModelConsistencyCell(null, labels)).toBe('')
    expect(upstreamModelConsistencyCell(undefined, labels)).toBe('')
  })
})
