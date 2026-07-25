import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const REQUIRED_HEADER_OVERRIDE_KEYS = [
  'title',
  'hint',
  'info',
  'namePlaceholder',
  'valuePlaceholder',
  'addRow',
  'importJson',
  'importJsonApply',
  'importJsonCancel',
  'importJsonHint',
  'importJsonInvalid',
  'copyJson',
  'emptyValueHint',
  'invalidName',
  'blockedName',
  'duplicateName',
  'invalidValue',
  'tooManyEntries'
] as const

const REQUIRED_GROK_BASE_URL_KEYS = ['title', 'hint', 'placeholder', 'required', 'invalid'] as const

describe('Grok account locale keys', () => {
  it.each([
    ['zh', zh],
    ['en', en]
  ])('contains every account setting label in %s', (_name, locale) => {
    const { grokClientToolCache, headerOverride, grokCustomBaseUrl } = locale.admin.accounts

    expect(grokClientToolCache.title).toEqual(expect.any(String))
    expect(grokClientToolCache.hint).toEqual(expect.any(String))

    for (const key of REQUIRED_HEADER_OVERRIDE_KEYS) {
      expect(headerOverride[key]).toEqual(expect.any(String))
      expect(headerOverride[key].length).toBeGreaterThan(0)
    }

    for (const key of REQUIRED_GROK_BASE_URL_KEYS) {
      expect(grokCustomBaseUrl[key]).toEqual(expect.any(String))
      expect(grokCustomBaseUrl[key].length).toBeGreaterThan(0)
    }

    expect(grokCustomBaseUrl.presets.cli).toEqual(expect.any(String))
    expect(grokCustomBaseUrl.presets.official).toEqual(expect.any(String))
  })

})
