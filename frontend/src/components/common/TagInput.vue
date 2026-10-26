<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'

const props = withDefaults(defineProps<{ modelValue: string[] | string; suggestions?: string[]; placeholder?: string; max?: number }>(), { suggestions: () => [], placeholder: '输入标签后按 Enter', max: 20 })
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()
const draft = ref('')
const composing = ref(false)
const active = ref(-1)
const suggestionsOpen = ref(true)
const input = ref<HTMLInputElement>()
const tags = computed(() => Array.isArray(props.modelValue) ? props.modelValue : String(props.modelValue || '').split(/[,，]/).map(v => v.trim()).filter(Boolean))
const normalized = (value: string) => value.trim().replace(/\s+/g, ' ')
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
  emit('update:modelValue', result.slice(0, props.max))
}
function add(value = draft.value) {
  if (composing.value) return
  update([...tags.value, ...value.split(/[,，\n;；]+/)])
  draft.value = ''
  active.value = -1
  suggestionsOpen.value = true
}
function remove(index: number) { update(tags.value.filter((_, i) => i !== index)) }
function keydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown' && filtered.value.length) { event.preventDefault(); suggestionsOpen.value = true; active.value = (active.value + 1) % filtered.value.length; return }
  if (event.key === 'ArrowUp' && filtered.value.length) { event.preventDefault(); suggestionsOpen.value = true; active.value = (active.value - 1 + filtered.value.length) % filtered.value.length; return }
  if (event.key === 'Escape') { active.value = -1; suggestionsOpen.value = false; return }
  if (composing.value || event.isComposing || event.keyCode === 229) return
  if (event.key === 'Enter' || event.key === 'Tab') {
    if (normalized(draft.value) || active.value >= 0) event.preventDefault()
    add(active.value >= 0 ? filtered.value[active.value] : draft.value)
  } else if (event.key === 'Backspace' && !draft.value && tags.value.length) remove(tags.value.length - 1)
}
function compositionstart() { composing.value = true }
function compositionend(event: CompositionEvent) { composing.value = false; draft.value = (event.target as HTMLInputElement).value }
function blur() { window.setTimeout(() => { add(); active.value = -1; suggestionsOpen.value = false }, 100) }
function focus() { suggestionsOpen.value = true; input.value?.focus() }
function paste(event: ClipboardEvent) { event.preventDefault(); add(event.clipboardData?.getData('text') || draft.value) }
function choose(value: string) { add(value); nextTick(focus) }
</script>
<template>
  <div class="tag-input" @click="focus">
    <span v-for="(tag, index) in tags" :key="tag.toLocaleLowerCase()" class="tag-chip">{{ tag }}<button type="button" :aria-label="`删除标签 ${tag}`" @click.stop="remove(index)">×</button></span>
    <input ref="input" v-model="draft" :placeholder="tags.length ? '' : placeholder" role="combobox" aria-autocomplete="list" :aria-expanded="suggestionsOpen && filtered.length > 0" @keydown="keydown" @compositionstart="compositionstart" @compositionend="compositionend" @focus="suggestionsOpen = true" @blur="blur" @paste="paste">
    <ul v-if="suggestionsOpen && filtered.length" class="tag-suggestions" role="listbox"><li v-for="(item, index) in filtered" :key="item" role="option" :aria-selected="index === active" :class="{ active: index === active }" @mousedown.prevent="choose(item)">{{ item }}</li></ul>
  </div>
</template>
<style scoped>
.tag-input{position:relative;display:flex;flex-wrap:wrap;gap:6px;min-height:44px;padding:6px 8px;border:2px solid #121316;border-radius:10px;background:#fff}.tag-input:focus-within{box-shadow:2px 2px 0 #ccff00}.tag-input input{flex:1;min-width:120px;border:0;outline:0;padding:5px;font:inherit}.tag-chip{display:inline-flex;align-items:center;gap:4px;padding:4px 7px;border:1.5px solid #121316;border-radius:6px;background:#edf4ff;font-size:12px;font-weight:700}.tag-chip button{border:0;background:transparent;cursor:pointer;font-size:16px}.tag-suggestions{position:absolute;z-index:20;top:calc(100% + 4px);left:0;right:0;margin:0;padding:5px;border:2px solid #121316;border-radius:9px;background:#fff;list-style:none;box-shadow:3px 3px 0 #121316}.tag-suggestions li{padding:7px;cursor:pointer}.tag-suggestions li.active,.tag-suggestions li:hover{background:#ccff00}
</style>
