import { describe, expect, it } from 'vitest'
import {
  applyUpstreamRequestIdHeader,
  isValidUpstreamRequestIdHeader,
  readUpstreamRequestIdHeader,
  UPSTREAM_REQUEST_ID_HEADER_EXTRA_KEY
} from '../upstreamRequestIdHeader'
import { hasReasoningEffortMapping } from '../reasoningEffortDisplay'

describe('upstreamRequestIdHeader', () => {
  it('validates like the backend: token chars, at most 64 bytes, empty allowed', () => {
    expect(isValidUpstreamRequestIdHeader('')).toBe(true)
    expect(isValidUpstreamRequestIdHeader('   ')).toBe(true)
    expect(isValidUpstreamRequestIdHeader('X-Oneapi-Request-Id')).toBe(true)
    expect(isValidUpstreamRequestIdHeader('x'.repeat(64))).toBe(true)
    expect(isValidUpstreamRequestIdHeader('x'.repeat(65))).toBe(false)
    expect(isValidUpstreamRequestIdHeader('X Request Id')).toBe(false)
    expect(isValidUpstreamRequestIdHeader('X-Request-Id:')).toBe(false)
    expect(isValidUpstreamRequestIdHeader('X-Request\nId')).toBe(false)
    expect(isValidUpstreamRequestIdHeader('请求头')).toBe(false)
  })

  it('reads, writes and clears the extra key without touching other keys', () => {
    expect(readUpstreamRequestIdHeader(undefined)).toBe('')
    expect(readUpstreamRequestIdHeader({ [UPSTREAM_REQUEST_ID_HEADER_EXTRA_KEY]: ' request-id ' })).toBe('request-id')

    const base = { base_rpm: 10 }
    expect(applyUpstreamRequestIdHeader(base, ' X-Oneapi-Request-Id ')).toEqual({ base_rpm: 10, upstream_request_id_header: 'X-Oneapi-Request-Id' })
    expect(base).toEqual({ base_rpm: 10 })

    const withHeader = { base_rpm: 10, upstream_request_id_header: 'request-id' }
    expect(applyUpstreamRequestIdHeader(withHeader, '')).toEqual({ base_rpm: 10 })
    expect(applyUpstreamRequestIdHeader(undefined, '')).toBeUndefined()
    expect(applyUpstreamRequestIdHeader(undefined, 'request-id')).toEqual({ upstream_request_id_header: 'request-id' })
  })
})

describe('hasReasoningEffortMapping', () => {
  it('only splits rows when requested and forwarded effort differ', () => {
    expect(hasReasoningEffortMapping({ requested_reasoning_effort: 'max', reasoning_effort: 'xhigh' })).toBe(true)
    expect(hasReasoningEffortMapping({ requested_reasoning_effort: 'high', reasoning_effort: 'HIGH' })).toBe(false)
    expect(hasReasoningEffortMapping({ requested_reasoning_effort: 'x-high', reasoning_effort: 'xhigh' })).toBe(false)
    expect(hasReasoningEffortMapping({ requested_reasoning_effort: null, reasoning_effort: 'high' })).toBe(false)
    expect(hasReasoningEffortMapping({ requested_reasoning_effort: 'max', reasoning_effort: null })).toBe(false)
  })
})
