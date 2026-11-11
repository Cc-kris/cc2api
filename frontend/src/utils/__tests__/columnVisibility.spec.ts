import { describe, expect, it } from 'vitest'

import { resolveHiddenColumns, serializeHiddenColumns } from '@/utils/columnVisibility'

const COLUMNS = ['api_key', 'model', 'reasoning_effort', 'request_id', 'upstream_request_id', 'session_id', 'user_agent']
const OPTIONS = {
  columns: COLUMNS,
  defaultHidden: ['reasoning_effort', 'user_agent', 'upstream_request_id', 'session_id'],
  legacyNewColumns: ['upstream_request_id', 'session_id']
}

const sorted = (set: Set<string>) => [...set].sort()

describe('resolveHiddenColumns', () => {
  it('uses defaults when nothing is saved', () => {
    expect(sorted(resolveHiddenColumns(null, OPTIONS))).toEqual(
      ['reasoning_effort', 'session_id', 'upstream_request_id', 'user_agent']
    )
  })

  it('hides newly added default-hidden columns for an old array save', () => {
    // 旧存档：用户打开了 reasoning_effort、隐藏了 model；保存时还没有新列。
    const raw = JSON.stringify(['model', 'user_agent'])
    const hidden = resolveHiddenColumns(raw, OPTIONS)
    expect(sorted(hidden)).toEqual(['model', 'session_id', 'upstream_request_id', 'user_agent'])
    // 用户原来打开的旧列不受影响。
    expect(hidden.has('reasoning_effort')).toBe(false)
  })

  it('keeps new columns open once the user opened them in a new-format save', () => {
    const afterUserOpened = serializeHiddenColumns(['reasoning_effort', 'user_agent', 'session_id'], COLUMNS)
    const hidden = resolveHiddenColumns(afterUserOpened, OPTIONS)
    expect(hidden.has('upstream_request_id')).toBe(false)
    expect(sorted(hidden)).toEqual(['reasoning_effort', 'session_id', 'user_agent'])
  })

  it('applies defaults to columns added after a new-format save', () => {
    const raw = serializeHiddenColumns(['model'], ['api_key', 'model', 'reasoning_effort'])
    const hidden = resolveHiddenColumns(raw, { columns: COLUMNS, defaultHidden: OPTIONS.defaultHidden })
    // reasoning_effort 保存时已存在且未隐藏，保持打开；其余默认隐藏列是新列。
    expect(sorted(hidden)).toEqual(['model', 'session_id', 'upstream_request_id', 'user_agent'])
  })

  it('falls back to defaults for corrupt or unknown data', () => {
    for (const raw of ['not json', '{"v":2}', '42', 'null', '[1,2]']) {
      expect(sorted(resolveHiddenColumns(raw, OPTIONS)), raw).toEqual(
        ['reasoning_effort', 'session_id', 'upstream_request_id', 'user_agent']
      )
    }
  })

  it('serializes hidden columns together with the known column list', () => {
    expect(JSON.parse(serializeHiddenColumns(new Set(['model']), ['api_key', 'model']))).toEqual({
      v: 2,
      hidden: ['model'],
      known: ['api_key', 'model']
    })
  })
})
