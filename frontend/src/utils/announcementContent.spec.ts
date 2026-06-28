import { describe, expect, it } from 'vitest'
import {
  ANNOUNCEMENT_RICH_TEXT_MARKER,
  announcementEditorInitialHtml,
  isRichAnnouncementContent,
  renderAnnouncementContent,
  wrapRichAnnouncementContent,
} from './announcementContent'

describe('announcementContent', () => {
  it('renders historical markdown announcements', () => {
    const html = renderAnnouncementContent('## 标题\n\n![图](https://example.com/a.png)\n\n| A | B |\n| - | - |\n| 1 | 2 |')
    expect(html).toContain('<h2>标题</h2>')
    expect(html).toContain('<img')
    expect(html).toContain('<table>')
  })

  it('renders rich html announcements without markdown conversion', () => {
    const rich = wrapRichAnnouncementContent('<p><strong>正文</strong></p><video controls src="https://example.com/a.mp4"></video>')
    const html = renderAnnouncementContent(rich)
    expect(isRichAnnouncementContent(rich)).toBe(true)
    expect(html).toContain('<strong>正文</strong>')
    expect(html).toContain('<video')
  })

  it('converts historical markdown to editor html when editing', () => {
    expect(announcementEditorInitialHtml('**旧公告**')).toContain('<strong>旧公告</strong>')
  })

  it('removes unsafe scripts from rich content', () => {
    const html = renderAnnouncementContent(`${ANNOUNCEMENT_RICH_TEXT_MARKER}\n<p>ok</p><script>alert(1)</script>`)
    expect(html).toContain('<p>ok</p>')
    expect(html).not.toContain('<script>')
  })
})
