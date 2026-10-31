import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import AnnouncementRichTextEditor from '@/components/admin/announcements/AnnouncementRichTextEditor.vue'
import {
  ANNOUNCEMENT_RICH_TEXT_MARKER,
  announcementEditorInitialHtml,
  isRichAnnouncementContent,
  normalizeAnnouncementEditorHtml,
  renderAnnouncementContent,
  wrapRichAnnouncementContent,
} from './announcementContent'

describe('announcementContent', () => {
  it('replays the latest long body when the editor is created after async content arrives', async () => {
    const body = 'x'.repeat(10_000)
    const html = `<p>${body}</p>`
    const wrapper = mount(AnnouncementRichTextEditor, { props: { modelValue: '<p>initial</p>' } })

    // 异步拿到正文后外部值回灌：长正文必须完整进 contenteditable（N5 不被截断）
    await wrapper.setProps({ modelValue: html })

    expect(wrapper.get('.rich-text-content').element.innerHTML).toBe(html)
    wrapper.unmount()
  })

  it('keeps local edits when modelValue changes, then accepts content on a new editor session', async () => {
    const wrapper = mount(AnnouncementRichTextEditor, {
      props: { modelValue: '<p>saved</p>', sessionKey: 'news-1' },
    })
    const editor = wrapper.get('.rich-text-content').element as HTMLElement
    editor.innerHTML = '<p>local edit</p>'
    await wrapper.get('.rich-text-content').trigger('input')

    await wrapper.setProps({ modelValue: '<p>stale server value</p>' })
    expect(editor.innerHTML).toBe('<p>local edit</p>')

    await wrapper.setProps({ modelValue: '<p>next news</p>', sessionKey: 'news-2' })
    expect(editor.innerHTML).toContain('next news')
    wrapper.unmount()
  })

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

  it('preserves a 10,000-character rich body through editor initialization and save normalization', () => {
    const body = 'x'.repeat(10_000)
    const html = `<p>${body}</p>`
    const stored = wrapRichAnnouncementContent(html)

    expect(announcementEditorInitialHtml(stored)).toBe(html)
    expect(normalizeAnnouncementEditorHtml(html)).toBe(html)
    expect(wrapRichAnnouncementContent(announcementEditorInitialHtml(stored))).toBe(stored)
  })

  it('removes unsafe scripts from rich content', () => {
    const html = renderAnnouncementContent(`${ANNOUNCEMENT_RICH_TEXT_MARKER}\n<p>ok</p><script>alert(1)</script>`)
    expect(html).toContain('<p>ok</p>')
    expect(html).not.toContain('<script>')
  })
})
