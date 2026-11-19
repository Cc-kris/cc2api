import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'), 'utf8')

describe('CreateAccountModal Gemini help dialog sizing', () => {
  it('uses BaseDialog’s supported width prop instead of forwarding an extraneous max-width attribute', () => {
    const openingTag = source.match(/<!-- Gemini Help Dialog -->\s*<BaseDialog([\s\S]*?)>/)?.[1]

    expect(openingTag).toBeDefined()
    expect(openingTag).toContain('width="wide"')
    expect(openingTag).not.toContain('max-width=')
  })
})
