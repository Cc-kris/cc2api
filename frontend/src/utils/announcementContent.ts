import { marked } from 'marked'
import DOMPurify from 'dompurify'

export const ANNOUNCEMENT_RICH_TEXT_MARKER = '<!-- sub2api:announcement-content html -->'

marked.setOptions({
  breaks: true,
  gfm: true,
})

export function isRichAnnouncementContent(content: string | null | undefined): boolean {
  return String(content || '').trimStart().startsWith(ANNOUNCEMENT_RICH_TEXT_MARKER)
}

export function stripRichAnnouncementMarker(content: string | null | undefined): string {
  const raw = String(content || '')
  return isRichAnnouncementContent(raw) ? raw.trimStart().slice(ANNOUNCEMENT_RICH_TEXT_MARKER.length).trimStart() : raw
}

export function wrapRichAnnouncementContent(html: string): string {
  return `${ANNOUNCEMENT_RICH_TEXT_MARKER}\n${html.trim()}`
}

export function renderAnnouncementContent(content: string | null | undefined): string {
  const raw = String(content || '')
  if (!raw.trim()) return ''
  const html = isRichAnnouncementContent(raw) ? stripRichAnnouncementMarker(raw) : (marked.parse(raw) as string)
  return sanitizeAnnouncementHtml(html)
}

export function sanitizeAnnouncementHtml(html: string): string {
  return DOMPurify.sanitize(html, {
    ADD_TAGS: ['video', 'source'],
    ADD_ATTR: ['controls', 'poster', 'preload', 'playsinline', 'src', 'type', 'width', 'height', 'target', 'rel'],
  })
}

export function announcementEditorInitialHtml(content: string | null | undefined): string {
  const raw = String(content || '')
  if (!raw.trim()) return '<p><br></p>'
  if (isRichAnnouncementContent(raw)) return sanitizeAnnouncementHtml(stripRichAnnouncementMarker(raw))
  return sanitizeAnnouncementHtml(marked.parse(raw) as string)
}

export function normalizeAnnouncementEditorHtml(html: string): string {
  return sanitizeAnnouncementHtml(html).trim()
}
