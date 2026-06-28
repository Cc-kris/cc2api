<template>
  <div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
    <Toolbar
      class="border-b border-gray-200 dark:border-dark-700"
      :editor="editorRef"
      :default-config="toolbarConfig"
      mode="default"
    />
    <Editor
      v-model="htmlValue"
      class="min-h-[320px] bg-white dark:bg-dark-800"
      :default-config="editorConfig"
      mode="default"
      @on-created="handleCreated"
      @on-change="handleChange"
    />
  </div>
  <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">
    支持文字样式、链接、图片、视频链接、表格、列表等。历史 Markdown 公告会自动转换后编辑。
  </p>
</template>

<script setup lang="ts">
import '@wangeditor/editor/dist/css/style.css'
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { Editor, Toolbar } from '@wangeditor/editor-for-vue'
import type { IDomEditor, IEditorConfig, IToolbarConfig } from '@wangeditor/editor'
import { normalizeAnnouncementEditorHtml } from '@/utils/announcementContent'

const props = defineProps<{
  modelValue: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const editorRef = shallowRef<IDomEditor | null>(null)
const htmlValue = ref(props.modelValue || '<p><br></p>')

watch(
  () => props.modelValue,
  (next) => {
    if (next !== htmlValue.value) {
      htmlValue.value = next || '<p><br></p>'
    }
  }
)

const toolbarConfig: Partial<IToolbarConfig> = {
  excludeKeys: ['fullScreen'],
}

const editorConfig = computed<Partial<IEditorConfig>>(() => ({
  placeholder: '请输入公告内容，可插入图片、视频链接和表格',
  MENU_CONF: {
    uploadImage: {
      maxFileSize: 2 * 1024 * 1024,
      allowedFileTypes: ['image/*'],
      customUpload(file: File, insertFn: (url: string, alt?: string, href?: string) => void) {
        readFileAsDataURL(file)
          .then((url) => insertFn(url, file.name, ''))
          .catch(() => window.alert('图片读取失败'))
      },
    },
    uploadVideo: {
      maxFileSize: 15 * 1024 * 1024,
      allowedFileTypes: ['video/*'],
      customUpload(file: File, insertFn: (url: string, poster?: string) => void) {
        readFileAsDataURL(file)
          .then((url) => insertFn(url, ''))
          .catch(() => window.alert('视频读取失败'))
      },
    },
    insertVideo: {
      checkVideo(src: string) {
        if (!/^https?:\/\//i.test(src) && !src.startsWith('data:video/')) return '请输入 http(s) 视频链接'
        return true
      },
    },
  },
}))

function handleCreated(editor: IDomEditor) {
  editorRef.value = editor
}

function handleChange() {
  emit('update:modelValue', normalizeAnnouncementEditorHtml(htmlValue.value))
}

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(new Error('file read failed'))
    reader.readAsDataURL(file)
  })
}

onBeforeUnmount(() => {
  editorRef.value?.destroy()
  editorRef.value = null
})
</script>

<style scoped>
:deep(.w-e-text-container) {
  min-height: 320px !important;
}

:deep(.w-e-text-placeholder) {
  color: #9ca3af;
}

:deep(.w-e-bar) {
  background: transparent;
}

:deep(.w-e-text-container [data-slate-editor]) {
  min-height: 320px;
}
</style>
