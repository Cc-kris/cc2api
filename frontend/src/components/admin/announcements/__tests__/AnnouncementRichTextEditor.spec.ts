import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { uploadContentImage } = vi.hoisted(() => ({ uploadContentImage: vi.fn() }))

vi.mock('@/api/admin/content', () => ({
  contentAdminAPI: { uploadContentImage },
  default: { uploadContentImage },
}))

import AnnouncementRichTextEditor from '../AnnouncementRichTextEditor.vue'

function mountEditor(props: Record<string, unknown> = {}) {
  return mount(AnnouncementRichTextEditor, {
    props: { modelValue: '<p>初始正文</p>', ...props },
  })
}

function paste(wrapper: ReturnType<typeof mountEditor>, payload: { html?: string; text?: string; files?: File[] }) {
  const editor = wrapper.get('.rich-text-content').element as HTMLElement
  const event = new Event('paste', { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'clipboardData', {
    value: {
      files: payload.files ?? [],
      getData: (type: string) => {
        if (type === 'text/html') return payload.html ?? ''
        if (type === 'text/plain') return payload.text ?? ''
        return ''
      },
    },
  })
  editor.dispatchEvent(event)
  return event
}

function emittedHtml(wrapper: ReturnType<typeof mountEditor>): string {
  const events = wrapper.emitted('update:modelValue') as unknown as string[][] | undefined
  return events?.length ? String(events[events.length - 1][0]) : ''
}

function makeFile(name: string, type: string, size: number): File {
  const file = new File([new ArrayBuffer(0)], name, { type })
  Object.defineProperty(file, 'size', { value: size })
  return file
}

beforeEach(() => {
  uploadContentImage.mockReset()
})

describe('AnnouncementRichTextEditor 工具栏（NE-08 / AE-03）', () => {
  it('工具栏为 PRD R2 的 10 项（无下划线），名字与顺序符合用例', () => {
    const wrapper = mountEditor()
    const buttons = wrapper.findAll('.rich-text-toolbar button')
    expect(buttons).toHaveLength(10)

    const labels = buttons.map((b) => b.attributes('aria-label'))
    expect(labels).toEqual([
      '加粗',
      '斜体',
      'H2',
      'H3',
      '有序列表（编号列表）',
      '无序列表（项目列表）',
      '引用',
      '链接',
      '插入图片',
      '清除格式',
    ])
    // 用例口径：NE-08 用「有序/无序列表」全匹配，AE-03 用 button[aria-label="项目列表"] 精确选择
    expect(labels.some((l) => String(l).includes('有序列表'))).toBe(true)
    expect(labels.some((l) => String(l).includes('无序列表'))).toBe(true)
    expect(labels.some((l) => String(l).includes('项目列表'))).toBe(true)
    expect(labels.some((l) => String(l).includes('编号列表'))).toBe(true)
  })

  it('每个按钮都有中文 title、aria-pressed 与可用键盘触发（原生 button + 不劫持 mousedown）', async () => {
    const wrapper = mountEditor()
    const buttons = wrapper.findAll('.rich-text-toolbar button')
    for (const button of buttons) {
      expect(button.attributes('title'), `${button.attributes('aria-label')} 缺 title`).toBeTruthy()
      expect(button.attributes('aria-pressed')).toBe('false')
      expect(button.element.tagName).toBe('BUTTON')
      expect(button.attributes('type')).toBe('button')
    }
    // 聚焦 + 回车不应抛错（jsdom 无 execCommand 时也要安全降级）
    await buttons[0].trigger('keydown', { key: 'Enter' })
    await buttons[0].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeFalsy()
  })

  it('编辑区存在可见的 file input，供「插入图片」使用', () => {
    const wrapper = mountEditor()
    const inputs = wrapper.findAll('[data-test="news-rich-editor"] input[type="file"]')
    expect(inputs).toHaveLength(1)
    expect(inputs[0].attributes('accept')).toContain('image/png')
    expect(inputs[0].attributes('accept')).toContain('.webp')
  })

  it('readOnly 时按钮与编辑区都禁用', () => {
    const wrapper = mountEditor({ readOnly: true })
    expect(wrapper.get('.rich-text-content').attributes('contenteditable')).toBe('false')
    for (const button of wrapper.findAll('.rich-text-toolbar button')) expect(button.attributes('disabled')).toBeDefined()
  })

  it('编辑区随容器伸缩：不写死高度，内容区内部滚动', () => {
    const wrapper = mountEditor()
    const style = wrapper.get('.rich-text-editor').attributes('style') || ''
    expect(style).not.toMatch(/height\s*:/)
    expect(wrapper.get('.rich-text-editor').classes()).toContain('rich-text-editor')
  })
})

describe('AnnouncementRichTextEditor 粘贴清洗（NE-10）', () => {
  it('粘贴含 script/on*/style 的 HTML 后被清洗', async () => {
    const wrapper = mountEditor()
    paste(wrapper, {
      html: '<h2 onclick="steal()">标题</h2><script>alert(1)</script><style>.a{}</style><p style="color:red">正文</p>',
    })
    await flushPromises()

    const html = emittedHtml(wrapper)
    expect(html).not.toMatch(/<script/i)
    expect(html).not.toMatch(/<style/i)
    expect(html).not.toMatch(/\son[a-z]+=/i)
    expect(html).not.toMatch(/style=/i)
    expect(html).toContain('正文')
    expect(wrapper.get('.rich-text-content').element.innerHTML).not.toMatch(/style=/i)
  })

  it('粘贴 base64 图片时转成 File 走上传，正文不出现 data:image', async () => {
    uploadContentImage.mockResolvedValue({ data: { data: { url: '/uploads/pasted-b64.png' } } })
    const wrapper = mountEditor()
    paste(wrapper, {
      html: '<p>前文</p><p><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUg=="></p><p>后文</p>',
    })
    await flushPromises()

    expect(uploadContentImage).toHaveBeenCalledTimes(1)
    const uploaded = uploadContentImage.mock.calls[0][0] as File
    expect(uploaded.type).toBe('image/png')
    const html = emittedHtml(wrapper)
    expect(html).not.toMatch(/data:image/i)
    expect(html).toContain('/uploads/pasted-b64.png')
    expect(wrapper.get('.rich-text-content').element.innerHTML).not.toMatch(/data:image/i)
  })

  it('粘贴不支持的 base64 图片（bmp）时给可见中文提示且不写进正文', async () => {
    const wrapper = mountEditor()
    paste(wrapper, { html: '<p><img src="data:image/bmp;base64,Qk0AAAAA"></p>' })
    await flushPromises()

    expect(uploadContentImage).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="rich-text-error"]').text()).toContain('无法识别')
    expect(emittedHtml(wrapper)).not.toMatch(/data:image/i)
  })

  it('粘贴 iframe 与 javascript: 链接被移除', async () => {
    const wrapper = mountEditor()
    paste(wrapper, { html: '<iframe src="https://evil.test"></iframe><p><a href="javascript:alert(1)">点我</a></p>' })
    await flushPromises()

    const html = emittedHtml(wrapper)
    expect(html).not.toMatch(/<iframe/i)
    expect(html).not.toMatch(/javascript:/i)
    expect(html).toContain('点我')
  })
})

describe('AnnouncementRichTextEditor 插入图片（NE-11）', () => {
  it('超过 5MB 的图片被拦，显示中文错误且不调用上传接口', async () => {
    const wrapper = mountEditor()
    const input = wrapper.get('input[type="file"]')
    const file = makeFile('big.png', 'image/png', 6 * 1024 * 1024)
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })

    await input.trigger('change')
    await flushPromises()

    expect(uploadContentImage).not.toHaveBeenCalled()
    const error = wrapper.get('[data-test="rich-text-error"]')
    expect(error.text()).toContain('5MB')
  })

  it('bmp 图片被拦，提示格式不支持', async () => {
    const wrapper = mountEditor()
    const input = wrapper.get('input[type="file"]')
    const file = makeFile('pic.bmp', 'image/bmp', 1024)
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })

    await input.trigger('change')
    await flushPromises()

    expect(uploadContentImage).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="rich-text-error"]').text()).toContain('仅支持 JPG')
  })

  it('合格 png 走上传接口，正文插入服务器 URL（非 base64）', async () => {
    uploadContentImage.mockResolvedValue({ data: { code: 0, data: { url: '/uploads/2026/10/ok.png' } } })
    const wrapper = mountEditor()
    const input = wrapper.get('input[type="file"]')
    const file = makeFile('ok.png', 'image/png', 2048)
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })

    await input.trigger('change')
    await flushPromises()

    expect(uploadContentImage).toHaveBeenCalledTimes(1)
    expect(uploadContentImage.mock.calls[0][0].name).toBe('ok.png')
    const html = emittedHtml(wrapper)
    expect(html).toContain('src="/uploads/2026/10/ok.png"')
    expect(html).not.toMatch(/data:/i)
    expect(wrapper.find('[data-test="rich-text-error"]').exists()).toBe(false)
  })

  it('上传接口返回 data: 地址时拒绝插入并提示', async () => {
    uploadContentImage.mockResolvedValue({ data: { data: { url: 'data:image/png;base64,AAAA' } } })
    const wrapper = mountEditor()
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [makeFile('x.png', 'image/png', 1024)], configurable: true })

    await input.trigger('change')
    await flushPromises()

    expect(wrapper.get('[data-test="rich-text-error"]').text()).toContain('base64')
    expect(wrapper.get('.rich-text-content').element.innerHTML).not.toMatch(/data:image/i)
  })

  it('上传失败给出中文错误提示且正文不变', async () => {
    uploadContentImage.mockRejectedValue(new Error('boom'))
    const wrapper = mountEditor()
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [makeFile('x.png', 'image/png', 1024)], configurable: true })

    await input.trigger('change')
    await flushPromises()

    expect(wrapper.get('[data-test="rich-text-error"]').text()).toContain('图片上传失败')
    expect(wrapper.get('.rich-text-content').element.innerHTML).toContain('初始正文')
  })

  it('粘贴图片文件走同一套上传流程', async () => {
    uploadContentImage.mockResolvedValue({ data: { data: { url: '/uploads/pasted.png' } } })
    const wrapper = mountEditor()
    paste(wrapper, { files: [makeFile('pasted.png', 'image/png', 4096)] })
    await flushPromises()

    expect(uploadContentImage).toHaveBeenCalledTimes(1)
    expect(emittedHtml(wrapper)).toContain('/uploads/pasted.png')
  })
})

describe('AnnouncementRichTextEditor 对外接口', () => {
  it('外部 modelValue 变化时重灌内容', async () => {
    const wrapper = mountEditor()
    await wrapper.setProps({ modelValue: '<p>新的正文</p>' })
    expect(wrapper.get('.rich-text-content').element.innerHTML).toContain('新的正文')
  })

  it('sessionKey 变化时用外部值重灌（内容中心切换新闻/文章）', async () => {
    const wrapper = mountEditor({ sessionKey: 'news:1' })
    ;(wrapper.get('.rich-text-content').element as HTMLElement).innerHTML = '<p>上一会话残留</p>'
    await wrapper.setProps({ sessionKey: 'news:2', modelValue: '<p>新会话正文</p>' })

    expect(wrapper.get('.rich-text-content').element.innerHTML).toContain('新会话正文')
    expect(wrapper.get('.rich-text-content').element.innerHTML).not.toContain('上一会话残留')
  })

  it('输入时抛出 update:modelValue，且值已收口（无 style 属性）', async () => {
    const wrapper = mountEditor()
    const editor = wrapper.get('.rich-text-content').element as HTMLElement
    editor.innerHTML = '<p style="color:red">带样式</p>'
    await wrapper.get('.rich-text-content').trigger('input')

    const html = emittedHtml(wrapper)
    expect(html).toContain('带样式')
    expect(html).not.toMatch(/style=/i)
  })
})
