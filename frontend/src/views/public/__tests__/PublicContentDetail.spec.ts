import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import * as ts from 'typescript'

const componentPath = resolve(__dirname, '../PublicContentView.vue')
const component = readFileSync(componentPath, 'utf8')

/** 从 .vue 源码中截取指定具名函数的完整源码（含函数体花括号配对） */
function extractFunction(source: string, name: string): string {
  const start = source.indexOf(`function ${name}(`)
  if (start < 0) throw new Error(`function ${name} not found`)
  let depth = 0
  let i = source.indexOf('{', start)
  for (; i < source.length; i += 1) {
    const ch = source[i]
    if (ch === '{') depth += 1
    else if (ch === '}') {
      depth -= 1
      if (depth === 0) break
    }
  }
  return source.slice(start, i + 1)
}

// 真实执行组件里的排序实现（TS -> JS 后求值），而不是只做文本断言
const fnSource = extractFunction(component, 'buildRelatedItems')
const compiled = ts.transpileModule(`${fnSource}\nexport { buildRelatedItems };`, {
  compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.ESNext }
}).outputText.replace('export { buildRelatedItems };', '')
const buildRelatedItems = new Function(`${compiled}; return buildRelatedItems;`)() as (
  pool: any[],
  current: any,
  zhMode: boolean,
  limit?: number
) => any[]

const item = (id: number, extra: Record<string, any> = {}) => ({
  id,
  slug: `slug-${id}`,
  title: `标题 ${id}`,
  status: 'published',
  category: 'Tech',
  tags: [],
  publishedAt: `2026-01-0${(id % 9) + 1}T00:00:00Z`,
  ...extra
})

describe('详情页「更多推荐」排序（R9-09）', () => {
  it('候选充足时固定 6 条，且排除当前篇（id 与 slug 双保险）', () => {
    const current = item(1, { slug: 'current', tags: ['ai'] })
    const pool = [current, item(1, { slug: 'current' }), ...Array.from({ length: 9 }, (_, i) => item(i + 2))]
    const result = buildRelatedItems(pool, current, true)
    expect(result).toHaveLength(6)
    expect(result.some((r) => r.slug === 'current')).toBe(false)
    expect(result.some((r) => r.id === 1)).toBe(false)
  })

  it('先按共同标签数排序，其次按发布时间倒序', () => {
    const current = item(1, { tags: ['ai', 'llm'], category: 'Tech' })
    const pool = [
      current,
      item(2, { tags: ['ai', 'llm'], publishedAt: '2026-01-01T00:00:00Z' }),
      item(3, { tags: ['ai', 'llm'], publishedAt: '2026-02-01T00:00:00Z' }),
      item(4, { tags: ['ai'], publishedAt: '2026-09-01T00:00:00Z' }),
      item(5, { tags: [], publishedAt: '2026-10-01T00:00:00Z' }),
    ]
    const result = buildRelatedItems(pool, current, true)
    expect(result.slice(0, 2).map((r) => r.id)).toEqual([3, 2])
    expect(result[2].id).toBe(4)
    expect(result).toHaveLength(4)
  })

  it('标签命中的推荐优先于仅同分类/最新的推荐', () => {
    const current = item(1, { tags: ['ai'], category: 'Tech' })
    const pool = [
      current,
      item(2, { tags: [], category: 'Tech', publishedAt: '2026-08-01T00:00:00Z' }),
      item(3, { tags: [], category: 'Other', publishedAt: '2026-09-09T00:00:00Z' }),
      item(4, { tags: ['ai'], category: 'Other', publishedAt: '2025-01-01T00:00:00Z' }),
    ]
    const result = buildRelatedItems(pool, current, true)
    expect(result.map((r) => r.id)).toEqual([4, 2, 3])
  })

  it('共同标签不足 6 条时，按同分类补齐，再按最新补齐', () => {
    const current = item(1, { tags: ['ai'], category: 'Tech' })
    const pool = [
      current,
      item(2, { tags: ['ai'], category: 'Other' }),
      item(3, { tags: [], category: 'Tech', publishedAt: '2026-03-01T00:00:00Z' }),
      item(4, { tags: [], category: 'Other', publishedAt: '2026-05-01T00:00:00Z' }),
      item(5, { tags: [], category: 'Other', publishedAt: '2026-06-01T00:00:00Z' }),
    ]
    const result = buildRelatedItems(pool, current, true)
    expect(result.map((r) => r.id)).toEqual([2, 3, 5, 4])
  })

  it('全站候选仅 3 条时只显示 3 条', () => {
    const current = item(1)
    const result = buildRelatedItems([current, item(2), item(3), item(4)], current, false)
    expect(result).toHaveLength(3)
  })

  it('只推荐已发布内容', () => {
    const current = item(1)
    const pool = [current, item(2, { status: 'draft' }), item(3, { status: 'published' }), item(4, { status: '' })]
    const ids = buildRelatedItems(pool, current, true).map((r) => r.id).sort((a, b) => a - b)
    expect(ids).toEqual([3, 4])
  })

  it('无当前篇数据时返回空数组', () => {
    expect(buildRelatedItems([item(2)], null, true)).toEqual([])
  })
})

describe('封面与阅读信息渲染契约（R7 / R2-02）', () => {
  it('详情页顶部渲染封面并限制高度 400px、16:9 裁剪', () => {
    expect(component).toContain('data-testid="detail-cover"')
    expect(component).toContain(':src="detailCover"')
    expect(component).toMatch(/\.article-cover \{[\s\S]*?max-height: 400px;/)
    expect(component).toMatch(/\.article-cover-img,[\s\S]{0,200}?object-fit: cover;/)
  })

  it('列表卡片封面固定 240px 宽、16:9，移动端置顶', () => {
    expect(component).toContain('class="list-card-cover spotlight-cover"')
    expect(component).toContain('class="list-card-cover timeline-cover"')
    expect(component).toMatch(/\.list-card-cover \{[\s\S]*?flex: 0 0 240px;[\s\S]*?aspect-ratio: 16 \/ 9;/)
    expect(component).toMatch(/@media \(max-width: 900px\) \{[\s\S]*?\.list-card-cover \{[\s\S]*?width: 100%;/)
    expect(component).toMatch(/\.timeline-cover \{[^}]*order: -1;/)
  })

  it('无封面或图片加载失败时使用统一占位，不渲染破图', () => {
    expect(component).toContain('class="cover-placeholder"')
    expect(component).toContain('@error="markCoverBroken(item)"')
    expect(component).toContain('@error="markCoverBroken(featuredItem)"')
    expect(component).toContain('@error="markCoverBroken(detail)"')
    expect(component).toContain('function markCoverBroken(')
    expect(component).toContain('function contentCover(')
  })

  it('详情页保留阅读时间与上一/下一篇', () => {
    expect(component).toContain('article-read-time')
    expect(component).toContain('estimateReadingTime(')
    expect(component).toContain('detail-prev-next-grid')
  })

  it('标题 2 行截断、标签不越界', () => {
    const block = component.slice(component.indexOf('TASK-024 封面'))
    const clampAt = block.indexOf('-webkit-line-clamp: 2;')
    expect(clampAt).toBeGreaterThan(0)
    expect(block.slice(clampAt - 200, clampAt)).toContain('.spotlight-title,')
    expect(component).toContain('.tl-tags { display: flex; flex-wrap: wrap;')
    expect(component).toMatch(/\.tl-tags \{[^}]*overflow: hidden;/)
  })
})
