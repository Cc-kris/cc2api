import DOMPurify from 'dompurify'

/**
 * 富文本编辑器（AnnouncementRichTextEditor）专用的清洗与图片校验工具。
 *
 * 与 utils/announcementContent.ts 的区别：这里的白名单严格按
 * v0.4 plan「富文本白名单」收口（p,h2,h3,strong,em,u,s,ul,ol,li,blockquote,
 * a[href,target,rel],img[src,alt],br,code,pre），**不允许 style 属性、
 * 不允许 on* 属性、不允许 data: URI**，用于粘贴/拖入内容与保存前的收口。
 * announcementContent.ts 的 sanitize 仍服务于历史公告渲染，行为保持不变。
 */

export const EDITOR_ALLOWED_TAGS = [
  'p', 'h2', 'h3', 'strong', 'em', 'u', 's',
  'ul', 'ol', 'li', 'blockquote', 'a', 'img', 'br', 'code', 'pre',
] as const

export const EDITOR_ALLOWED_ATTR = ['href', 'target', 'rel', 'src', 'alt'] as const

/** 单图上限 5MB（与后端 /admin/content/upload 的 5MiB 对齐） */
export const EDITOR_IMAGE_MAX_BYTES = 5 * 1024 * 1024
export const EDITOR_IMAGE_MIME_TYPES = ['image/jpeg', 'image/png', 'image/webp', 'image/gif'] as const
export const EDITOR_IMAGE_EXTENSIONS = ['jpg', 'jpeg', 'png', 'webp', 'gif'] as const

/** 命中这些特征就值得再走一次严格清洗（避免每次按键都 DOMPurify 全文） */
const SUSPICIOUS_HTML = /<script|<style|<iframe|<object|<embed|\son[a-z]+\s*=|style\s*=|data:|\bjavascript:/i

/**
 * href/src 白名单：显式协议只允许 http(s)；无协议的站内相对路径（/a/b、a/b、#x、?q）允许。
 * data:/javascript:/vbscript:/mailto:/file: 等一律拒绝。
 */
export function isSafeEditorUrl(value: string | null | undefined): boolean {
  const url = String(value ?? '').trim()
  if (!url) return false
  // 显式协议（含 data:、javascript:）
  if (/^[a-z][a-z0-9+.-]*:/i.test(url)) return /^https?:\/\//i.test(url)
  // 协议相对 //host/path 属于外部地址但没写协议，按白名单口径拒绝
  if (url.startsWith('//')) return false
  return true
}

function scrubEditorUrls(html: string): string {
  if (!html) return ''
  if (typeof DOMParser === 'undefined') return html
  const doc = new DOMParser().parseFromString(`<body><div id="rich-text-root">${html}</div></body>`, 'text/html')
  const root = doc.getElementById('rich-text-root')
  if (!root) return html

  root.querySelectorAll('img').forEach((img) => {
    const src = img.getAttribute('src') || ''
    if (!isSafeEditorUrl(src)) {
      img.remove()
      return
    }
    img.removeAttribute('style')
    img.removeAttribute('srcset')
  })

  root.querySelectorAll('a').forEach((anchor) => {
    const href = anchor.getAttribute('href') || ''
    if (!isSafeEditorUrl(href)) {
      const children = Array.from(anchor.childNodes)
      anchor.replaceWith(...children)
      return
    }
    anchor.setAttribute('target', '_blank')
    anchor.setAttribute('rel', 'noopener noreferrer')
  })

  return root.innerHTML
}

/**
 * 按 v0.4 富文本白名单清洗 HTML。
 * - 去掉 script/style/iframe 等标签与 on*、style 属性
 * - href/src 只允许 http(s) 与站内相对路径，data: URI 的图片整张丢弃
 */
export function sanitizeEditorHtml(html: string | null | undefined): string {
  const dirty = String(html ?? '')
  if (!dirty.trim()) return ''
  const clean = DOMPurify.sanitize(dirty, {
    ALLOWED_TAGS: [...EDITOR_ALLOWED_TAGS],
    ALLOWED_ATTR: [...EDITOR_ALLOWED_ATTR],
    ALLOW_DATA_ATTR: false,
    KEEP_CONTENT: true,
  })
  return scrubEditorUrls(clean).trim()
}

/** 只有命中可疑特征才走完整清洗，供 input 事件里低成本收口 */
export function sanitizeEditorHtmlIfNeeded(html: string | null | undefined): string {
  const raw = String(html ?? '')
  return SUSPICIOUS_HTML.test(raw) ? sanitizeEditorHtml(raw) : raw
}

/** HTML 里是否还残留内联 base64 图片 */
export function containsInlineDataImage(html: string | null | undefined): boolean {
  return /data:image\//i.test(String(html ?? ''))
}

export type EditorImageCheck = { ok: true } | { ok: false; message: string }

/**
 * 前端预检：≤5MB，且必须是 jpg/png/webp/gif。
 * 以 MIME 为主，MIME 缺失（部分系统粘贴/拖入给空 type）时回退到扩展名；
 * MIME 明确是别的 image/*（bmp、svg、tiff…）一律拒绝。
 */
export function validateEditorImageFile(file: { name?: string; size?: number; type?: string } | null | undefined): EditorImageCheck {
  if (!file) return { ok: false, message: '请选择要插入的图片文件' }
  const size = Number(file.size ?? 0)
  if (size <= 0) return { ok: false, message: '图片文件为空，请重新选择' }
  if (size > EDITOR_IMAGE_MAX_BYTES) return { ok: false, message: '图片不能超过 5MB，请压缩后再上传' }

  const mime = String(file.type ?? '').toLowerCase()
  const ext = (/\.([a-z0-9]+)\s*$/i.exec(String(file.name ?? ''))?.[1] || '').toLowerCase()
  const mimeOk = (EDITOR_IMAGE_MIME_TYPES as readonly string[]).includes(mime)
  const extOk = (EDITOR_IMAGE_EXTENSIONS as readonly string[]).includes(ext)

  if (mime.startsWith('image/')) {
    if (!mimeOk) return { ok: false, message: '仅支持 JPG、PNG、WEBP、GIF 格式的图片' }
    return { ok: true }
  }
  if (!extOk) return { ok: false, message: '仅支持 JPG、PNG、WEBP、GIF 格式的图片' }
  return { ok: true }
}

/** 生成插入正文的图片 HTML（src 来自上传接口返回值，永不使用 data:） */
export function buildEditorImageHtml(url: string, alt = ''): string {
  const src = String(url ?? '').trim()
  if (!isSafeEditorUrl(src)) return ''
  const escape = (v: string) =>
    v.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
  return `<p><img src="${escape(src)}" alt="${escape(String(alt ?? ''))}"></p>`
}

/** 从粘贴/拖入的 HTML 里找出内联 base64 图片（按出现顺序，去重） */
export function extractInlineDataImages(html: string | null | undefined): Array<{ src: string; alt: string }> {
  const raw = String(html ?? '')
  if (!containsInlineDataImage(raw)) return []
  const found: Array<{ src: string; alt: string }> = []
  const seen = new Set<string>()
  const imgTag = /<img\b[^>]*>/gi
  let tag: RegExpExecArray | null
  while ((tag = imgTag.exec(raw))) {
    const attrs = tag[0]
    const src = /src\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))/i.exec(attrs)
    const value = (src?.[2] ?? src?.[3] ?? src?.[4] ?? '').trim()
    if (!/^data:image\//i.test(value) || seen.has(value)) continue
    const alt = /alt\s*=\s*("([^"]*)"|'([^']*)')/i.exec(attrs)
    seen.add(value)
    found.push({ src: value, alt: (alt?.[2] ?? alt?.[3] ?? '').trim() })
  }
  return found
}

const DATA_URL_EXT: Record<string, string> = {
  'image/jpeg': 'jpg',
  'image/jpg': 'jpg',
  'image/png': 'png',
  'image/webp': 'webp',
  'image/gif': 'gif',
}

/**
 * 把 data:image/...;base64 转成 File，好走和文件选择框同一套「预检 → 上传 → 插入 URL」流程。
 * 解析不了（非 base64、MIME 不在白名单、缺少 atob）返回 null，由调用方给中文错误提示。
 */
export function dataUrlToEditorImageFile(
  dataUrl: string,
  baseName = 'pasted-image',
): File | null {
  const match = /^data:(image\/[a-z0-9.+-]+);base64,([a-z0-9+/=\s]+)$/i.exec(String(dataUrl ?? '').trim())
  if (!match) return null
  const mime = match[1].toLowerCase()
  const ext = DATA_URL_EXT[mime]
  if (!ext) return null
  const binary = typeof atob === 'function' ? atob(match[2].replace(/\s+/g, '')) : ''
  if (!binary) return null
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i)
  const name = `${String(baseName || 'pasted-image').replace(/\.[^.]+$/, '')}.${ext}`
  return new File([bytes], name, { type: mime })
}
