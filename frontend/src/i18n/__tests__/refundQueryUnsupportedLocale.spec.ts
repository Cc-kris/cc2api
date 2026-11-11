import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'

import en from '../locales/en'
import zh from '../locales/zh'
import { extractI18nErrorMessage } from '@/utils/apiError'

// V03-R03：查询退款状态对不支持查询的通道（如易支付）返回 400 REFUND_QUERY_UNSUPPORTED，
// 管理员订单页按 payment.errors.<reason> 翻译；缺 key 时会把后端英文原文直接弹给中文界面。
const backendError = {
  status: 400,
  code: 400,
  message: 'this payment provider does not support refund status query; please verify manually',
  reason: 'REFUND_QUERY_UNSUPPORTED'
}

describe('REFUND_QUERY_UNSUPPORTED locale', () => {
  it.each([
    ['zh', zh, '该支付通道不支持查询退款状态'],
    ['en', en, 'does not support refund status queries']
  ] as const)('%s translates the reason instead of showing the raw backend message', (locale, messages, expected) => {
    const i18n = createI18n({ legacy: false, locale, messages: { [locale]: messages } })
    const msg = extractI18nErrorMessage(backendError, i18n.global.t as never, 'payment.errors', 'fallback')
    expect(msg).toContain(expected)
    expect(msg).not.toBe(backendError.message)
  })
})
