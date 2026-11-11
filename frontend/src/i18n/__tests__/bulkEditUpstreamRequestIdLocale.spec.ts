import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

// V03-R11：批量编辑弹窗的上游请求 ID 头字段文案（de/fr/ru 不含 admin 作用域，回落到英文）。
const KEYS = ['upstreamRequestIdHeaderHint', 'upstreamRequestIdHeaderRequired'] as const

describe('bulk edit upstream request ID header locale', () => {
  it.each([
    ['zh', zh],
    ['en', en]
  ] as const)('%s has the bulk edit upstream request ID header messages', (_locale, messages) => {
    const bulkEdit = (messages.admin.accounts as Record<string, unknown>).bulkEdit as Record<string, string>
    for (const key of KEYS) {
      expect(bulkEdit[key], key).toBeTruthy()
    }
    // 已有的单个账号文案仍复用于标签、校验和帮助提示。
    expect(messages.admin.accounts.upstreamRequestIdHeader).toBeTruthy()
    expect(messages.admin.accounts.upstreamRequestIdHeaderInvalid).toBeTruthy()
  })
})
