import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const componentPath = resolve(__dirname, '../PublicContentView.vue')
const component = readFileSync(componentPath, 'utf8')
const stylePath = resolve(__dirname, '../../../style.css')
const globalStyle = readFileSync(stylePath, 'utf8')

describe('PublicContentView featured section contract', () => {
  it('renders a standalone featured section with one hero and at most nine secondary cards', () => {
    expect(component).toContain('data-testid="featured-section"')
    expect(component).toContain('v-if="showFeaturedSection"')
    expect(component).toContain('featuredItems[0]')
    expect(component).toContain('featuredItems.slice(1, 10)')
    expect(component).toContain('.slice(0, 10)')
    expect(component).toContain(':to="localePath(\'articles\')" class="public-featured-all-link"')
  })

  it('uses featured time descending with an ID descending deterministic tie-breaker', () => {
    expect(component).toContain('featuredSortTime(b) - featuredSortTime(a)')
    expect(component).toContain('Number(b?.id || 0) - Number(a?.id || 0)')
    expect(component).toContain('/public/articles/featured')
  })

  it('provides cover placeholders, two-line title clamping and light/dark contrast tokens', () => {
    expect(component).toContain('public-featured-cover-placeholder')
    expect(component).toContain('-webkit-line-clamp: 2;')
    expect(component).toContain('--featured-surface: #FFFFFF;')
    expect(component).toContain('--featured-ink: #121316;')
    expect(component).toContain('--featured-surface: #1E2026;')
    expect(component).toContain('--featured-ink: #E8EAED;')
    expect(component).toContain('color: #121316;')
    expect(component).toContain(':global(.dark) .public-featured-section')
  })

  it('defines three-column desktop, two-column tablet and one-column mobile layouts', () => {
    expect(component).toContain('grid-template-columns: repeat(3, minmax(0, 1fr));')
    expect(component).toContain('@media (max-width: 1100px)')
    expect(component).toContain('grid-template-columns: repeat(2, minmax(0, 1fr));')
    expect(component).toContain('@media (max-width: 767px)')
    expect(component).toContain('grid-template-columns: 1fr;')
    expect(globalStyle).toBeDefined()
  })
})
