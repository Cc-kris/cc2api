import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})


describe('AppSidebar cache management navigation', () => {
  it('keeps system settings as a single top-level entry for platform owners', () => {
    expect(componentSource).toContain("path: '/admin/settings'")
    expect(componentSource).toContain("label: t('nav.settings')")
    expect(componentSource).not.toContain("label: t('nav.generalSettings')")
  })
})

describe('AppSidebar custom menu navigation', () => {
  it('routes custom menu links through the custom page shell', () => {
    expect(componentSource).toContain('function resolveCustomMenuPath')
    expect(componentSource).toContain('return `/custom/${item.id}`')
    expect(componentSource.match(/path: resolveCustomMenuPath/g)?.length).toBe(3)
    expect(componentSource).not.toContain('nativeLink')
    expect(componentSource).not.toContain('isNativeCustomMenuLink')
    expect(componentSource).not.toContain('isNativeCustomMenuURL')
  })
})
