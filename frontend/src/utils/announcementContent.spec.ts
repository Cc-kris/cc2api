import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
const editorTestControl = vi.hoisted(() => ({ editor: null as any }))

vi.mock('@wangeditor/editor-for-vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    Editor: defineComponent({
      emits: ['onCreated', 'onChange'],
      setup(_, { emit }) {
        return () => h('button', { onClick: () => emit('onCreated', editorTestControl.editor) }, 'create editor')
      },
    }),
    Toolbar: defineComponent({ setup: () => () => h('div') }),
  }
})

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
    const editor = {
      getHtml: vi.fn(() => '<p>initial</p>'),
      setHtml: vi.fn(),
      destroy: vi.fn(),
    }
    editorTestControl.editor = editor
    const wrapper = mount(AnnouncementRichTextEditor, { props: { modelValue: '<p>initial</p>' } })

    await wrapper.setProps({ modelValue: html })
    await wrapper.get('button').trigger('click')

    expect(editor.setHtml).toHaveBeenCalledTimes(1)
    expect(editor.setHtml).toHaveBeenCalledWith(html)
    wrapper.unmount()
    editorTestControl.editor = null
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
