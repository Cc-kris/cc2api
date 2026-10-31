<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'

const props = withDefaults(defineProps<{
  modelValue: string[] | string
  suggestions?: string[]
  placeholder?: string
  /** 最多多少个词条 */
  max?: number
  /** 单个词条最多多少个「字」（中文=1 字，英文/数字/符号=半角 0.5 字）；0 表示不限制 */
  tagMaxWidth?: number
  /** 常驻规则说明，如「单篇最多 10 个标签，单个标签最长 20 字」 */
  limitHint?: string
}>(), { suggestions: () => [], placeholder: '输入标签后按 Enter', max: 20, tagMaxWidth: 0, limitHint: '' })
const emit = defineEmits<{ 'update:modelValue': [value: string[] | string] }>()
const draft = ref('')
const composing = ref(false)
const active = ref(-1)
const suggestionsOpen = ref(false)
const error = ref('')
const input = ref<HTMLInputElement>()
const tags = computed(() => Array.isArray(props.modelValue) ? props.modelValue : String(props.modelValue || '').split(/[,，]/).map(v => v.trim()).filter(Boolean))
const normalized = (value: string) => value.trim().replace(/\s+/g, ' ')
/** 视觉字宽：CJK / 全角按 1 字，其余按半角 0.5 字 */
function visualWidth(value: string): number {
  const fullWidth = /[\u1100-\u115F\u2E80-\u303E\u3041-\u33FF\u3400-\u4DBF\u4E00-\u9FFF\uA000-\uA4CF\uAC00-\uD7A3\uF900-\uFAFF\uFE30-\uFE4F\uFF00-\uFF60\uFFE0-\uFFE6]/
  let width = 0
  for (const ch of value) width += fullWidth.test(ch) ? 1 : 0.5
  return width
}
const shortLabel = (value: string) => (value.length > 10 ? `${value.slice(0, 10)}…` : value)
const filtered = computed(() => {
  const q = normalized(draft.value).toLocaleLowerCase()
  const existing = new Set(tags.value.map(v => v.toLocaleLowerCase()))
  return [...new Set(props.suggestions.map(normalized).filter(v => v && !existing.has(v.toLocaleLowerCase()) && (!q || v.toLocaleLowerCase().includes(q))))].slice(0, 8)
})
function update(values: string[]) {
  const result: string[] = []
  const seen = new Set<string>()
  for (const value of values.map(normalized)) {
    const key = value.toLocaleLowerCase()
    if (value && !seen.has(key)) { seen.add(key); result.push(value) }
  }
  const limited = result.slice(0, props.max)
  emit('update:modelValue', Array.isArray(props.modelValue) ? limited : limited.join(','))
}
function add(value = draft.value) {
  if (composing.value) return
  const incoming = value.split(/[,，、;；\n]+/).map(normalized).filter(Boolean)
  if (!incoming.length) { draft.value = ''; active.value = -1; suggestionsOpen.value = false; return }
  const current = tags.value.map(v => v.toLocaleLowerCase())
  const accepted: string[] = []
  let message = ''
  for (const item of incoming) {
    if (props.tagMaxWidth > 0 && visualWidth(item) > props.tagMaxWidth) {
      message = `单个标签最长 ${props.tagMaxWidth} 字（中文按 1 字、英文与符号按半角计），「${shortLabel(item)}」超出长度上限，未添加`
      continue
    }
    if (current.length + accepted.length >= props.max) {
      message = `最多 ${props.max} 个标签，已达上限，「${shortLabel(item)}」未添加`
      break
    }
    accepted.push(item)
  }
  error.value = message
  if (accepted.length) update([...tags.value, ...accepted])
  draft.value = ''
  active.value = -1
  suggestionsOpen.value = false
}
function remove(index: number) { error.value = ''; update(tags.value.filter((_, i) => i !== index)) }
function keydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown' && filtered.value.length) { event.preventDefault(); suggestionsOpen.value = true; active.value = (active.value + 1) % filtered.value.length; return }
  if (event.key === 'ArrowUp' && filtered.value.length) { event.preventDefault(); suggestionsOpen.value = true; active.value = (active.value - 1 + filtered.value.length) % filtered.value.length; return }
  // Esc 只在联想下拉打开时“吃掉”按键（关下拉），否则交还给外层弹窗关闭逻辑
  if (event.key === 'Escape') {
    if (suggestionsOpen.value || active.value >= 0) {
      event.preventDefault()
      event.stopPropagation()
      active.value = -1
      suggestionsOpen.value = false
      return
    }
    return
  }
  if (composing.value || event.isComposing || event.keyCode === 229) return
  if (event.key === 'Enter' || event.key === 'Tab') {
    if (normalized(draft.value) || active.value >= 0) event.preventDefault()
    add(active.value >= 0 ? filtered.value[active.value] : draft.value)
  } else if (event.key === 'Backspace' && !draft.value && tags.value.length) remove(tags.value.length - 1)
}
function onInput() { error.value = ''; suggestionsOpen.value = true }
function compositionstart() { composing.value = true }
function compositionend(event: CompositionEvent) { composing.value = false; draft.value = (event.target as HTMLInputElement).value }
function blur() { window.setTimeout(() => { add(); active.value = -1; suggestionsOpen.value = false }, 100) }
function focus() { suggestionsOpen.value = true; input.value?.focus() }
function paste(event: ClipboardEvent) { event.preventDefault(); add(event.clipboardData?.getData('text') || draft.value) }
function choose(value: string) { add(value); nextTick(focus) }
</script>
<template>
  <div class="tag-input-wrap">
    <div class="tag-input" @click="focus">
      <span v-for="(tag, index) in tags" :key="tag.toLocaleLowerCase()" class="tag-chip">{{ tag }}<button type="button" :aria-label="`删除标签 ${tag}`" @click.stop="remove(index)">×</button></span>
      <input ref="input" v-model="draft" @input="onInput" :placeholder="tags.length ? '' : placeholder" role="combobox" aria-autocomplete="list" :aria-expanded="suggestionsOpen && filtered.length > 0" @keydown="keydown" @compositionstart="compositionstart" @compositionend="compositionend" @focus="suggestionsOpen = true" @blur="blur" @paste="paste">
      <ul v-if="suggestionsOpen && filtered.length" class="tag-suggestions" role="listbox"><li v-for="(item, index) in filtered" :key="item" role="option" :aria-selected="index === active" :class="{ active: index === active }" @mousedown.prevent="choose(item)">{{ item }}</li></ul>
    </div>
    <p v-if="error" class="tag-input-error" role="alert">{{ error }}</p>
    <p v-else-if="limitHint" class="tag-input-hint">{{ limitHint }}</p>
  </div>
</template>
<style scoped>
.tag-input-wrap{display:flex;flex-direction:column;gap:4px}
.tag-input{position:relative;display:flex;flex-wrap:wrap;gap:6px;min-height:44px;padding:6px 8px;border:2px solid #121316;border-radius:10px;background:#fff}.tag-input:focus-within{box-shadow:2px 2px 0 #ccff00}.tag-input input{flex:1;min-width:120px;border:0;outline:0;padding:5px;font:inherit}.tag-chip{display:inline-flex;align-items:center;gap:4px;padding:4px 7px;border:1.5px solid #121316;border-radius:6px;background:#edf4ff;font-size:12px;font-weight:700}.tag-chip button{border:0;background:transparent;cursor:pointer;font-size:16px}.tag-suggestions{position:absolute;z-index:20;top:calc(100% + 4px);left:0;right:0;margin:0;padding:5px;border:2px solid #121316;border-radius:9px;background:#fff;list-style:none;box-shadow:3px 3px 0 #121316}.tag-suggestions li{padding:7px;cursor:pointer}.tag-suggestions li.active,.tag-suggestions li:hover{background:#ccff00}
.tag-input-hint{margin:0;font-size:11.5px;line-height:1.5;color:#6b7280}
.tag-input-error{margin:0;font-size:11.5px;line-height:1.5;font-weight:600;color:#b42318}
</style>
