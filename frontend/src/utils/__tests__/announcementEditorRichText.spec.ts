import { describe, expect, it } from 'vitest'

import {
  EDITOR_IMAGE_MAX_BYTES,
  buildEditorImageHtml,
  containsInlineDataImage,
  dataUrlToEditorImageFile,
  extractInlineDataImages,
  isSafeEditorUrl,
  sanitizeEditorHtml,
  sanitizeEditorHtmlIfNeeded,
  validateEditorImageFile,
} from '../announcementEditorRichText'

describe('sanitizeEditorHtml（NE-10 粘贴清洗）', () => {
  it('去掉 script/style/iframe 标签与 on* / style 属性', () => {
    const dirty = [
      '<h2 onclick="alert(1)">标题</h2>',
      '<script>alert(1)</script>',
      '<style>.x{color:red}</style>',
      '<iframe src="https://evil.test"></iframe>',
      '<p style="color:red;font-size:20px">段落<span onmouseover="hack()">文本</span></p>',
    ].join('')
    const clean = sanitizeEditorHtml(dirty)

    expect(clean).not.toMatch(/<script/i)
    expect(clean).not.toMatch(/<style/i)
    expect(clean).not.toMatch(/<iframe/i)
    expect(clean).not.toMatch(/\son[a-z]+=/i)
    expect(clean).not.toMatch(/style=/i)
    expect(clean).toContain('<h2>标题</h2>')
    expect(clean).toContain('段落')
    expect(clean).toContain('文本')
  })

  it('保留白名单标签：strong/em/h3/ol/ul/li/blockquote/a/img', () => {
    const html =
      '<p><strong>粗</strong><em>斜</em></p><h3>小标题</h3>' +
      '<ol><li>一</li></ol><ul><li>二</li></ul><blockquote>引用</blockquote>' +
      '<p><a href="/news/1">内链</a></p><p><img src="https://cdn.test/a.png" alt="图"></p>'
    const clean = sanitizeEditorHtml(html)

    for (const tag of ['<strong>', '<em>', '<h3>', '<ol>', '<ul>', '<li>', '<blockquote>', '<a ', '<img']) {
      expect(clean, `白名单标签 ${tag} 丢失`).toContain(tag)
    }
    expect(clean).toContain('href="/news/1"')
    expect(clean).toContain('src="https://cdn.test/a.png"')
  })

  it('拒绝 javascript: 链接与 data: 图片', () => {
    const clean = sanitizeEditorHtml(
      '<p><a href="javascript:alert(1)">点我</a></p><p><img src="data:image/png;base64,iVBORw0KGgo="></p><p>正文</p>',
    )

    expect(clean).not.toMatch(/javascript:/i)
    expect(clean).not.toMatch(/data:image/i)
    expect(containsInlineDataImage(clean)).toBe(false)
    expect(clean).toContain('点我')
    expect(clean).toContain('正文')
  })

  it('外链补 target/rel，站内相对路径不加协议判断', () => {
    const clean = sanitizeEditorHtml('<p><a href="https://example.com/x">外链</a></p>')
    expect(clean).toContain('target="_blank"')
    expect(clean).toContain('rel="noopener noreferrer"')
    expect(isSafeEditorUrl('/uploads/a.png')).toBe(true)
    expect(isSafeEditorUrl('https://x.test/a.png')).toBe(true)
    expect(isSafeEditorUrl('//x.test/a.png')).toBe(false)
    expect(isSafeEditorUrl('javascript:alert(1)')).toBe(false)
  })

  it('sanitizeEditorHtmlIfNeeded 只在命中可疑特征时才改变内容', () => {
    const plain = '<p>普通正文</p>'
    expect(sanitizeEditorHtmlIfNeeded(plain)).toBe(plain)
    const dirty = '<p style="color:red">红</p>'
    expect(sanitizeEditorHtmlIfNeeded(dirty)).toBe('<p>红</p>')
  })
})

describe('validateEditorImageFile（NE-11 前端预检）', () => {
  it('拦住超过 5MB 的图片', () => {
    const result = validateEditorImageFile({
      name: 'big.png',
      type: 'image/png',
      size: EDITOR_IMAGE_MAX_BYTES + 1,
    })
    expect(result.ok).toBe(false)
    expect(result.ok === false && result.message).toContain('5MB')
  })

  it('允许正好 5MB 的图片', () => {
    expect(validateEditorImageFile({ name: 'ok.png', type: 'image/png', size: EDITOR_IMAGE_MAX_BYTES })).toEqual({ ok: true })
  })

  it('拦住 .bmp / image/bmp', () => {
    const result = validateEditorImageFile({ name: 'pic.bmp', type: 'image/bmp', size: 1024 })
    expect(result.ok).toBe(false)
    expect(result.ok === false && result.message).toContain('JPG')
  })

  it('拦住 svg 与无扩展名文件', () => {
    expect(validateEditorImageFile({ name: 'pic.svg', type: 'image/svg+xml', size: 100 }).ok).toBe(false)
    expect(validateEditorImageFile({ name: 'blob', type: '', size: 100 }).ok).toBe(false)
  })

  it('MIME 缺失时回退扩展名判断', () => {
    expect(validateEditorImageFile({ name: 'a.WEBP', type: '', size: 100 })).toEqual({ ok: true })
    expect(validateEditorImageFile({ name: 'a.gif', type: 'application/octet-stream', size: 100 })).toEqual({ ok: true })
  })

  it('空文件被拒', () => {
    expect(validateEditorImageFile({ name: 'a.png', type: 'image/png', size: 0 }).ok).toBe(false)
  })
})

describe('buildEditorImageHtml', () => {
  it('生成服务器 URL 的 img，不使用 data:', () => {
    const html = buildEditorImageHtml('/uploads/2026/10/a.png', '配图')
    expect(html).toBe('<p><img src="/uploads/2026/10/a.png" alt="配图"></p>')
    expect(containsInlineDataImage(html)).toBe(false)
  })

  it('data: 与 javascript: 地址直接拒绝（返回空串）', () => {
    expect(buildEditorImageHtml('data:image/png;base64,AAAA')).toBe('')
    expect(buildEditorImageHtml('javascript:alert(1)')).toBe('')
  })

  it('转义引号防止属性注入', () => {
    const html = buildEditorImageHtml('/u/a.png', '"><script>x</script>')
    expect(html).not.toContain('<script>')
    expect(html).toContain('&quot;')
  })
})

describe('粘贴内联图片转上传（NE-11 base64 不内嵌）', () => {
  it('extractInlineDataImages 只取 data: 图片并保留 alt', () => {
    const html =
      '<p><img src="data:image/png;base64,AAAA" alt="截图"></p><p><img src="https://cdn.test/b.png" alt="x"></p>'
    const items = extractInlineDataImages(html)
    expect(items).toHaveLength(1)
    expect(items[0].src.startsWith('data:image/png;base64,')).toBe(true)
    expect(items[0].alt).toBe('截图')
  })

  it('dataUrlToEditorImageFile 生成同类型 File，bmp 返回 null', () => {
    const file = dataUrlToEditorImageFile('data:image/png;base64,iVBORw0KGgo=', 'pasted')
    expect(file).toBeInstanceOf(File)
    expect(file?.type).toBe('image/png')
    expect(file?.name.endsWith('.png')).toBe(true)
    expect(dataUrlToEditorImageFile('data:image/bmp;base64,Qk0AAAAA', 'pasted')).toBeNull()
  })
})
