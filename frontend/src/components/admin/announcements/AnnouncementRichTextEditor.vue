<template>
  <div class="rich-text-editor" data-test="news-rich-editor" :class="{ 'is-readonly': readOnly, 'is-fullscreen': fullscreen }">
    <div class="rich-text-toolbar-row">
      <div class="rich-text-toolbar" role="toolbar" aria-label="正文格式">
      <button
        v-for="tool in toolbar"
        :key="tool.key"
        type="button"
        class="rich-text-tool"
        :class="{ 'is-active': active[tool.key] }"
        :data-command="tool.key"
        :aria-label="tool.label"
        :title="tool.hint"
        :aria-pressed="active[tool.key] ? 'true' : 'false'"
        :disabled="readOnly || (tool.key === 'image' && uploading)"
        @mousedown.prevent
        @click="onToolClick(tool)"
      >
        <span class="rich-text-tool-icon" aria-hidden="true">{{ tool.icon }}</span>
      </button>
      <input
        ref="fileInputEl"
        class="rich-text-file-input"
        type="file"
        accept=".jpg,.jpeg,.png,.webp,.gif,image/jpeg,image/png,image/webp,image/gif"
        tabindex="-1"
        aria-label="选择要插入的图片"
        :disabled="readOnly || uploading"
        @change="onFileInputChange"
      />
    </div>
    <div class="rich-text-advanced" role="toolbar" aria-label="更多格式">
      <button
        v-for="tool in advancedTools"
        :key="'adv-' + tool.key"
        type="button"
        class="rich-text-adv-tool"
        :class="{ 'is-active': active[tool.key] }"
        :data-command="tool.key"
        :aria-label="tool.label"
        :title="tool.hint"
        :aria-pressed="active[tool.key] ? 'true' : 'false'"
        :disabled="readOnly"
        @mousedown.prevent
        @click="onAdvancedClick(tool)"
      >
        <span class="rich-text-tool-icon" aria-hidden="true">{{ tool.icon }}</span>
      </button>
      <span class="rich-text-adv-divider" aria-hidden="true"></span>
      <span class="rich-text-stats" data-test="rich-text-stats">{{ stats.chars }} 字 · {{ stats.words }} 词</span>
      <button
        type="button"
        class="rich-text-adv-tool rich-text-adv-fullscreen"
        :aria-label="fullscreen ? '退出全屏' : '全屏编辑'"
        :title="fullscreen ? '退出全屏（Esc）' : '全屏编辑（Ctrl+Shift+F）'"
        @mousedown.prevent
        @click="toggleFullscreen"
      >
        <span class="rich-text-tool-icon" aria-hidden="true">{{ fullscreen ? '⤡' : '⤢' }}</span>
      </button>
    </div>
    </div>
    <div ref="editorHostEl" class="rich-text-body"></div>
    <p v-if="errorMessage" class="rich-text-error" role="alert" data-test="rich-text-error">{{ errorMessage }}</p>
    <p v-else-if="uploading" class="rich-text-hint" data-test="rich-text-uploading">图片上传中，请稍候…</p>
    <p v-else class="rich-text-hint">支持加粗、斜体、H2/H3、列表、引用、链接与插图（JPG/PNG/WEBP/GIF，≤5MB）；粘贴内容会自动清洗。</p>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { Editor } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Underline from '@tiptap/extension-underline'
import Link from '@tiptap/extension-link'
import Image from '@tiptap/extension-image'
import Placeholder from '@tiptap/extension-placeholder'
import { normalizeAnnouncementEditorHtml } from '@/utils/announcementContent'
import {
  buildEditorImageHtml,
  containsInlineDataImage,
  dataUrlToEditorImageFile,
  extractInlineDataImages,
  sanitizeEditorHtml,
  sanitizeEditorHtmlIfNeeded,
  validateEditorImageFile,
} from '@/utils/announcementEditorRichText'
import { contentAdminAPI } from '@/api/admin/content'

type ToolKey =
  | 'bold'
  | 'italic'
  | 'h2'
  | 'h3'
  | 'ol'
  | 'ul'
  | 'quote'
  | 'link'
  | 'image'
  | 'clear'
  | 'underline'
  | 'strike'
  | 'inlineCode'
  | 'codeBlock'
  | 'undo'
  | 'redo'

interface ToolDef {
  key: ToolKey
  /** aria-label / 用例判定用的中文名 */
  label: string
  /** 中文 title（含快捷键提示） */
  hint: string
  icon: string
}

/**
 * 工具栏顺序按 PRD R2 与用例（NE-08 / AE-03）：
 * 加粗、斜体、H2、H3、有序列表、无序列表、引用、链接、插入图片、清除格式（共 10 项）。
 *
 * 命名口径：用例正文用「有序列表 / 无序列表」，AE-03 的逐项验证又用
 * `button[aria-label="项目列表"]`、`button[aria-label="编号列表"]` 精确选择；
 * 两者只能靠 aria-label 同时包含两个叫法来兼容 → 「有序列表（编号列表）」「无序列表（项目列表）」。
 */
const toolbar: ToolDef[] = [
  { key: 'bold', label: '加粗', hint: '加粗（Ctrl+B）', icon: 'B' },
  { key: 'italic', label: '斜体', hint: '斜体（Ctrl+I）', icon: 'I' },
  { key: 'h2', label: 'H2', hint: '二级标题（H2）', icon: 'H2' },
  { key: 'h3', label: 'H3', hint: '三级标题（H3）', icon: 'H3' },
  { key: 'ol', label: '有序列表（编号列表）', hint: '有序列表（编号）', icon: '1.' },
  { key: 'ul', label: '无序列表（项目列表）', hint: '无序列表（项目符号）', icon: '•' },
  { key: 'quote', label: '引用', hint: '引用块', icon: '❝' },
  { key: 'link', label: '链接', hint: '插入链接（Ctrl+K）', icon: '🔗' },
  { key: 'image', label: '插入图片', hint: '插入图片（≤5MB，JPG/PNG/WEBP/GIF）', icon: '🖼' },
  { key: 'clear', label: '清除格式', hint: '清除格式', icon: 'Tx' },
]

const props = withDefaults(
  defineProps<{ modelValue: string; placeholder?: string; readOnly?: boolean; sessionKey?: string | number }>(),
  { placeholder: '', readOnly: false, sessionKey: '' },
)
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const editorHostEl = ref<HTMLElement | null>(null)
const fileInputEl = ref<HTMLInputElement | null>(null)
const errorMessage = ref('')
const uploading = ref(false)
const fullscreen = ref(false)
const stats = reactive({ chars: 0, words: 0 })

/**
 * 高级工具行：白名单（p/h2/h3/strong/em/u/s/ul/ol/li/blockquote/a/img/br/code/pre）之外的
 * 结构一律不提供，避免插进去被 sync() 清掉。
 */
const advancedTools: ToolDef[] = [
  { key: 'underline', label: '下划线', hint: '下划线（Ctrl+U）', icon: 'U' },
  { key: 'strike', label: '删除线', hint: '删除线（Ctrl+Shift+S）', icon: 'S' },
  { key: 'inlineCode', label: '行内代码', hint: '行内代码', icon: '</>' },
  { key: 'codeBlock', label: '代码块', hint: '代码块', icon: '{ }' },
  { key: 'undo', label: '撤销', hint: '撤销（Ctrl+Z）', icon: '↶' },
  { key: 'redo', label: '重做', hint: '重做（Ctrl+Shift+Z）', icon: '↷' },
]

const active = reactive<Record<string, boolean>>({
  bold: false,
  italic: false,
  h2: false,
  h3: false,
  ol: false,
  ul: false,
  quote: false,
  link: false,
  image: false,
  clear: false,
  underline: false,
  strike: false,
  inlineCode: false,
  codeBlock: false,
  undo: false,
  redo: false,
})

let lastEmitted = ''
/** 本地是否有尚未被外部 modelValue 回声确认的编辑（防止过期服务端值冲掉用户输入） */
let dirty = false
let listeners: Array<() => void> = []

function initialHtml(): string {
  return normalizeAnnouncementEditorHtml(props.modelValue) || '<p></p>'
}

const editor = shallowRef<Editor | null>(null)

/**
 * TipTap（ProseMirror）实例。
 * 不用 @tiptap/vue-3 的 useEditor：它在 onMounted 才建实例、EditorContent 还要再等一个 tick，
 * 编辑区 DOM 不是同步存在的（单测/首屏都会短暂空白）。这里自己持有宿主 div 并在 onMounted 同步建实例，
 * ProseMirror 的 contenteditable 元素本身带 class="rich-text-content"（readOnly 时原生 contenteditable="false"）。
 */
function buildExtensions() {
  return [
    StarterKit.configure({
      heading: { levels: [2, 3] },
    }),
    Underline,
    Link.configure({
      openOnClick: false,
      autolink: true,
      HTMLAttributes: { target: '_blank', rel: 'noopener noreferrer' },
    }),
    Image.configure({ inline: false, allowBase64: false }),
    Placeholder.configure({ placeholder: () => props.placeholder || '请输入正文…' }),
  ]
}

function editorAttributes() {
  return {
    class: 'rich-text-content',
    role: 'textbox',
    'aria-multiline': 'true',
    'aria-label': props.placeholder || '正文编辑',
    'aria-readonly': props.readOnly ? 'true' : 'false',
    'data-placeholder': props.placeholder || '',
  }
}

function createEditorInstance(contentHtml: string) {
  destroyEditorInstance()
  const host = editorHostEl.value
  if (!host) return
  const instance = new Editor({
    element: host,
    content: contentHtml,
    editable: !props.readOnly,
    extensions: buildExtensions(),
    editorProps: { attributes: editorAttributes() },
    onUpdate: () => {
      errorMessage.value = ''
      sync()
    },
    onSelectionUpdate: () => refreshActiveState(),
    onTransaction: () => refreshActiveState(),
  })
  editor.value = instance
  attachDomListeners(instance.view.dom as HTMLElement)
  refreshStats()
  refreshActiveState()
}

function destroyEditorInstance() {
  detachDomListeners()
  editor.value?.destroy()
  editor.value = null
}

/**
 * 编辑区 DOM 被外部直接改写（绕过 ProseMirror 事务）时由 ProseMirror 自己的 DOMObserver 读回，
 * 不需要额外重建实例——早期这里按 DOM 文本重建，会在正常输入（input 事件先于 state 更新）
 * 时误判为「DOM 与 state 不一致」而销毁重建，导致只留下第一个字符、加粗等标记丢失。
 */

function currentHtml(): string {
  return editor.value ? editor.value.getHTML() : ''
}

function refreshActiveState() {
  const instance = editor.value
  if (!instance) return
  const isActive = (name: string, attrs?: Record<string, unknown>) => {
    try {
      return instance.isActive(name, attrs)
    } catch {
      return false
    }
  }
  active.bold = isActive('bold')
  active.italic = isActive('italic')
  active.underline = isActive('underline')
  active.strike = isActive('strike')
  active.ol = isActive('orderedList')
  active.ul = isActive('bulletList')
  active.quote = isActive('blockquote')
  active.h2 = isActive('heading', { level: 2 })
  active.h3 = isActive('heading', { level: 3 })
  active.link = isActive('link')
  active.inlineCode = isActive('code')
  active.codeBlock = isActive('codeBlock')
  active.undo = false
  active.redo = false
}

/** 字数统计：中文字符按字计，英文按空格切词 */
function refreshStats() {
  const instance = editor.value
  if (!instance) return
  const text = (instance.getText() || '').replace(/\u00a0/g, ' ')
  stats.chars = text.replace(/\s/g, '').length
  stats.words = text.trim() ? text.trim().split(/\s+/).length : 0
}

/** 收口：命中可疑特征（style、on* 事件、script、data:）时清洗后再对外抛出，保证保存值干净 */
function sync() {
  const html = currentHtml()
  if (!html) return
  const cleaned = sanitizeEditorHtmlIfNeeded(html)
  // 只要本地产生了内容变化就置脏：外部的（可能是过期的）modelValue 回声不能冲掉用户正在写的内容
  dirty = true
  lastEmitted = cleaned
  refreshStats()
  emit('update:modelValue', cleaned)
}

function setContent(value: string | null | undefined) {
  const instance = editor.value
  const html = normalizeAnnouncementEditorHtml(value ?? '') || '<p></p>'
  if (!instance) return
  // 第二参数 false：不触发 update 事件，避免外部回灌又抛一次 update:modelValue
  instance.commands.setContent(html, false)
  lastEmitted = instance.getHTML()
  dirty = false
  refreshStats()
  refreshActiveState()
}

/** 在当前选区插入已清洗的 HTML（走编辑器事务，保留撤销栈） */
function insertHtml(html: string) {
  const instance = editor.value
  const safe = sanitizeEditorHtml(html)
  if (!instance || !safe) return
  instance.chain().focus().insertContent(safe).run()
}

function promptForLink() {
  const instance = editor.value
  if (!instance || props.readOnly) return
  let url = ''
  if (typeof window !== 'undefined' && typeof window.prompt === 'function') {
    url = window.prompt('请输入链接地址（https:// 开头，或以 / 开头的站内路径）', 'https://') || ''
  }
  const value = url.trim()
  if (!value) return
  if (!/^https?:\/\//i.test(value) && !value.startsWith('/')) {
    errorMessage.value = '链接只支持 http(s) 地址或以 / 开头的站内路径'
    return
  }
  errorMessage.value = ''
  const chain = instance.chain().focus().extendMarkRange('link')
  if (instance.state.selection.empty) {
    chain.insertContent(`<a href="${value}" target="_blank" rel="noopener noreferrer">${value}</a>`).run()
  } else {
    chain.setLink({ href: value, target: '_blank', rel: 'noopener noreferrer' }).run()
  }
  sync()
}

function insertImage(url: string, alt: string) {
  const html = buildEditorImageHtml(url, alt)
  if (!html) {
    errorMessage.value = '图片地址不合法，已取消插入'
    return
  }
  errorMessage.value = ''
  insertHtml(html)
  sync()
}

async function uploadImage(file: File): Promise<string> {
  const response = await contentAdminAPI.uploadContentImage(file)
  const body: any = (response as any)?.data ?? response
  const payload = body?.data ?? body
  const url = String(payload?.url || payload?.path || '').trim()
  if (!url) throw new Error('上传响应缺少图片地址')
  if (containsInlineDataImage(url)) throw new Error('不接受内联 base64 图片')
  return url
}

async function handleImageFiles(files: File[]) {
  if (props.readOnly || !files.length) return
  for (const file of files) {
    const check = validateEditorImageFile(file)
    if (!check.ok) {
      errorMessage.value = check.message
      continue
    }
    errorMessage.value = ''
    uploading.value = true
    try {
      const url = await uploadImage(file)
      insertImage(url, file.name.replace(/\.[^.]+$/, ''))
    } catch (error) {
      const message = (error as Error)?.message || ''
      errorMessage.value = message.includes('5MB') || message.includes('格式') || message.includes('base64')
        ? message
        : '图片上传失败，请稍后重试'
    } finally {
      uploading.value = false
    }
  }
}

function onFileInputChange(event: Event) {
  const input = event.target as HTMLInputElement | null
  const files = Array.from(input?.files ?? []) as File[]
  if (input) input.value = ''
  if (!files.length) return
  void handleImageFiles(files)
}

/** 工具按钮 → TipTap 命令（不再依赖 document.execCommand） */
function runCommand(key: ToolKey) {
  const instance = editor.value
  if (!instance || props.readOnly) return
  const chain = instance.chain().focus()
  switch (key) {
    case 'bold':
      chain.toggleBold().run()
      break
    case 'italic':
      chain.toggleItalic().run()
      break
    case 'underline':
      chain.toggleUnderline().run()
      break
    case 'strike':
      chain.toggleStrike().run()
      break
    case 'h2':
      chain.toggleHeading({ level: 2 }).run()
      break
    case 'h3':
      chain.toggleHeading({ level: 3 }).run()
      break
    case 'ol':
      chain.toggleOrderedList().run()
      break
    case 'ul':
      chain.toggleBulletList().run()
      break
    case 'quote':
      chain.toggleBlockquote().run()
      break
    case 'inlineCode':
      chain.toggleCode().run()
      break
    case 'codeBlock':
      chain.toggleCodeBlock().run()
      break
    case 'undo':
      chain.undo().run()
      break
    case 'redo':
      chain.redo().run()
      break
    default:
      return
  }
  refreshActiveState()
}

function clearFormatting() {
  const instance = editor.value
  if (!instance || props.readOnly) return
  instance.chain().focus().unsetAllMarks().clearNodes().run()
  refreshActiveState()
  sync()
}

function onToolClick(tool: ToolDef) {
  if (props.readOnly) return
  if (tool.key === 'link') return promptForLink()
  if (tool.key === 'image') {
    if (uploading.value) return
    errorMessage.value = ''
    fileInputEl.value?.click()
    return
  }
  if (tool.key === 'clear') return clearFormatting()
  runCommand(tool.key)
}

function onAdvancedClick(tool: ToolDef) {
  if (props.readOnly) return
  errorMessage.value = ''
  if (tool.key === 'inlineCode') {
    const instance = editor.value
    if (instance && instance.state.selection.empty) {
      errorMessage.value = '请先选中要设为代码的文字'
      return
    }
  }
  runCommand(tool.key)
}

function toggleFullscreen() {
  fullscreen.value = !fullscreen.value
  if (fullscreen.value) {
    // 全屏时把焦点交回正文，避免用户还要再点一次
    window.setTimeout(() => editor.value?.commands.focus(), 0)
  }
}

/**
 * 粘贴/拖入内容里带的内联 base64 图片：转成 File 后走和文件选择框同一套「预检 → 上传 → 插入 URL」，
 * 上传失败或格式/体积不合格都给可见中文提示，任何情况下都不把 data: 写进正文。
 */
function handleInlineDataImages(html: string) {
  const inlineImages = extractInlineDataImages(html)
  if (!inlineImages.length) return
  const files: File[] = []
  for (let index = 0; index < inlineImages.length; index += 1) {
    const item = inlineImages[index]
    const file = dataUrlToEditorImageFile(item.src, item.alt || `pasted-image-${index + 1}`)
    if (!file) {
      errorMessage.value = '粘贴的图片无法识别，请用「插入图片」上传 JPG/PNG/WEBP/GIF（≤5MB）'
      continue
    }
    files.push(file)
  }
  if (files.length) void handleImageFiles(files)
}

function onPaste(event: ClipboardEvent) {
  if (props.readOnly) return
  const data = event.clipboardData
  if (!data) return
  const files = Array.from(data.files ?? []).filter((file) => file.type.startsWith('image/')) as File[]
  if (files.length) {
    event.preventDefault()
    void handleImageFiles(files)
    return
  }
  const html = data.getData('text/html')
  if (!html) return // 纯文本粘贴交给编辑器（纯文本天然安全）
  event.preventDefault()
  const cleaned = sanitizeEditorHtml(html)
  if (cleaned) {
    insertHtml(cleaned)
    sync()
  } else {
    const text = data.getData('text/plain')
    if (text) insertHtml(`<p>${text.replace(/</g, '&lt;').replace(/>/g, '&gt;')}</p>`)
    sync()
  }
  // 剪贴板里的 base64 图片（Word/网页截图常见）转 File 上传，不内嵌
  handleInlineDataImages(html)
}

function onDrop(event: DragEvent) {
  if (props.readOnly) return
  const data = event.dataTransfer
  if (!data) return
  const files = Array.from(data.files ?? []).filter((file) => file.type.startsWith('image/')) as File[]
  if (files.length) {
    event.preventDefault()
    void handleImageFiles(files)
    return
  }
  const html = data.getData('text/html')
  if (!html) return
  event.preventDefault()
  const cleaned = sanitizeEditorHtml(html)
  if (cleaned) {
    insertHtml(cleaned)
    sync()
  }
  handleInlineDataImages(html)
}

function onDragOver(event: DragEvent) {
  if (!props.readOnly) event.preventDefault()
}

function attachDomListeners(el: HTMLElement) {
  const onInput = () => {
    // 内容同步完全交给 ProseMirror 的 onUpdate（它会先更新 state 再 emit）
    errorMessage.value = ''
  }
  const onSelectionChange = () => refreshActiveState()
  // capture 阶段拦截：先于 ProseMirror 自带的粘贴/拖放处理，避免同一份内容被插入两次
  el.addEventListener('input', onInput)
  el.addEventListener('paste', onPaste as EventListener, true)
  el.addEventListener('drop', onDrop as EventListener, true)
  el.addEventListener('dragover', onDragOver as EventListener, true)
  const onKeyUp = () => refreshActiveState()
  el.addEventListener('keyup', onKeyUp)
  el.addEventListener('mouseup', onKeyUp)
  const onKeydown = (event: KeyboardEvent) => {
    if (props.readOnly) return
    if (event.key === 'Escape' && fullscreen.value) {
      fullscreen.value = false
      return
    }
    if (!(event.ctrlKey || event.metaKey)) return
    if (event.shiftKey && event.key.toLowerCase() === 'f') {
      event.preventDefault()
      toggleFullscreen()
    }
  }
  el.addEventListener('keydown', onKeydown)
  document.addEventListener('selectionchange', onSelectionChange)

  listeners = [
    () => el.removeEventListener('keydown', onKeydown),
    () => el.removeEventListener('input', onInput),
    () => el.removeEventListener('paste', onPaste as EventListener, true),
    () => el.removeEventListener('drop', onDrop as EventListener, true),
    () => el.removeEventListener('dragover', onDragOver as EventListener, true),
    () => el.removeEventListener('keyup', onKeyUp),
    () => el.removeEventListener('mouseup', onKeyUp),
    () => document.removeEventListener('selectionchange', onSelectionChange),
  ]
}

function detachDomListeners() {
  listeners.forEach((off) => off())
  listeners = []
}

onMounted(() => {
  createEditorInstance(initialHtml())
})

onBeforeUnmount(() => {
  destroyEditorInstance()
})

watch(
  () => props.modelValue,
  (value) => {
    const instance = editor.value
    if (!instance) return
    const next = value ?? ''
    // 自己抛出去的值的回声：只是确认本地内容，清掉脏标记
    if (next === lastEmitted) {
      dirty = false
      return
    }
    if (next === instance.getHTML()) return
    // 本地有未回灌的编辑时，不让外部（可能是过期的服务端值）冲掉用户正在写的内容
    if (dirty) return
    setContent(next)
  },
)

watch(
  () => props.readOnly,
  (value) => {
    editor.value?.setEditable(!value)
    const dom = editor.value?.view.dom as HTMLElement | undefined
    dom?.setAttribute('aria-readonly', value ? 'true' : 'false')
  },
)

// 会话切换（内容中心切换新闻/文章）时强制用外部值重灌，避免残留上一篇正文
watch(
  () => props.sessionKey,
  () => {
    setContent(props.modelValue)
  },
)
</script>

<style scoped>
.rich-text-editor {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  overflow: hidden;
  border: 1.5px solid #d5d9e2;
  border-radius: 10px;
  background: #fff;
}
/* 主工具栏 + 更多格式合并为同一行：垂直空间宝贵，两行工具栏会挤掉正文高度 */
.rich-text-toolbar-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 6px;
  border-bottom: 1px solid #d5d9e2;
  background: #f8fafc;
  flex: 0 0 auto;
  min-width: 0;
}
.rich-text-toolbar {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 6px;
  padding: 0;
  border-bottom: none;
  background: transparent;
  flex: 0 0 auto;
}
.rich-text-tool {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 34px;
  min-height: 30px;
  padding: 3px 8px;
  border: 1px solid #d5d9e2;
  border-radius: 6px;
  background: #fff;
  color: #1f2937;
  font-size: 13px;
  line-height: 1;
  cursor: pointer;
}
.rich-text-tool:hover:not(:disabled) { border-color: #93c5fd; background: #eff6ff; }
.rich-text-tool:focus-visible { outline: 2px solid #2563eb; outline-offset: 2px; }
.rich-text-tool.is-active { border-color: #2563eb; background: #dbeafe; color: #1d4ed8; }
.rich-text-tool:disabled { opacity: 0.45; cursor: not-allowed; }
.rich-text-tool-icon { font-weight: 600; }
.rich-text-advanced {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  padding: 0;
  border-bottom: none;
  background: transparent;
  flex: 1 1 auto;
  min-width: 0;
  overflow-x: auto;
}
.rich-text-adv-tool {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 30px;
  min-height: 26px;
  padding: 2px 7px;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  color: #475569;
  font-size: 12px;
  line-height: 1;
  cursor: pointer;
}
.rich-text-adv-tool:hover:not(:disabled) { border-color: #cbd5e1; background: #fff; color: #0f172a; }
.rich-text-adv-tool:focus-visible { outline: 2px solid #2563eb; outline-offset: 1px; }
.rich-text-adv-tool.is-active { border-color: #2563eb; background: #dbeafe; color: #1d4ed8; }
.rich-text-adv-tool:disabled { opacity: 0.45; cursor: not-allowed; }
.rich-text-adv-divider { width: 1px; height: 16px; margin: 0 2px; background: #d5d9e2; }
.rich-text-stats { margin-left: auto; padding-right: 4px; color: #475569; font-size: 12px; font-variant-numeric: tabular-nums; white-space: nowrap; }
@media (max-width: 640px) {
  .rich-text-toolbar-row { flex-wrap: wrap; gap: 4px; }
  .rich-text-toolbar { flex-wrap: wrap; gap: 4px; max-width: 100%; }
  .rich-text-advanced { flex-wrap: wrap; justify-content: flex-start; gap: 4px; width: 100%; }
}
.rich-text-editor.is-fullscreen {
  position: fixed;
  inset: 12px;
  z-index: 60;
  border-radius: 12px;
  box-shadow: 0 24px 64px rgba(15, 23, 42, 0.28);
}
.rich-text-editor.is-fullscreen :deep(.rich-text-content) { min-height: 0; }
.rich-text-file-input { display: none; }
.rich-text-body {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  overflow: hidden;
}
.rich-text-body :deep(.rich-text-content) {
  flex: 1 1 auto;
  min-height: 240px;
  overflow-y: auto;
  padding: 12px;
  outline: none;
  background: #fff;
  line-height: 1.65;
}
.rich-text-body :deep(.rich-text-content:focus) { box-shadow: inset 0 0 0 2px #93c5fd; }
.rich-text-body :deep(.rich-text-content img) { max-width: 100%; height: auto; }
.rich-text-body :deep(.rich-text-content blockquote) {
  margin: 8px 0;
  padding: 4px 12px;
  border-left: 3px solid #cbd5e1;
  color: #475569;
}
.rich-text-body :deep(.rich-text-content p.is-editor-empty:first-child::before) {
  content: attr(data-placeholder);
  float: left;
  height: 0;
  color: #9ca3af;
  pointer-events: none;
}
.rich-text-hint { margin: 6px 12px; color: #64748b; font-size: 12px; line-height: 1.6; }
.rich-text-error { margin: 6px 12px; color: #dc2626; font-size: 12px; line-height: 1.6; }
.is-readonly :deep(.rich-text-content) { background: #f8fafc; }
</style>
