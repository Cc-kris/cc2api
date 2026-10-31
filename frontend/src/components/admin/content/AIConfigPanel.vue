<template>
  <section class="ai-config-panel" :aria-label="t('contentV04.ai.configuration')">
    <header class="ai-config-header">
      <div><h3>{{ t("contentV04.ai.configuration") }}</h3><p>管理文章生成和翻译使用的模型服务。</p></div>
      <button type="button" class="button primary" @click="openCreate">新增配置</button>
    </header>
    <p v-if="pageError" class="ai-feedback error" role="alert">{{ pageError }}</p>
    <p v-if="pageNotice" class="ai-feedback success" role="status">{{ pageNotice }}</p>
    <div v-if="!configs.length" class="ai-config-empty">{{ t("contentV04.ai.configureFirst") }}</div>
    <ul v-else class="ai-config-list">
      <li v-for="c in configs" :key="c.id">
        <div class="ai-config-summary">
          <div class="ai-config-title"><strong>{{ c.label }}</strong><span v-if="c.isDefault" class="default-badge">{{ t('contentV04.ai.default') }}</span><span v-if="c.keyUnreadable" class="key-unreadable-badge" title="已存 API Key 无法用当前加密钥解开，请编辑后重新填写">Key 需重填</span></div>
          <span>{{ providerLabel(c.provider) }} · {{ c.defaultModel || c.models?.[0] || '—' }}</span>
          <small>{{ c.baseUrl }} · {{ keyDisplay(c) }}</small>
        </div>
        <div class="ai-config-row-actions">
          <button type="button" class="secondary-button" :disabled="c.isDefault || busy" :aria-pressed="!!c.isDefault" @click="setDefault(c)">{{ t('contentV04.ai.setDefault') }}</button>
          <button type="button" class="secondary-button" @click="edit(c)">{{ t("contentV04.ai.edit") }}</button>
          <button type="button" class="danger-button" :disabled="busy" :title="c.isDefault ? '删除后会自动把剩余第一条设为默认' : ''" @click="remove(c)">{{ t("contentV04.ai.delete") }}</button>
        </div>
      </li>
    </ul>

    <div v-if="dialogOpen" class="ai-config-modal" @mousedown.self="requestClose">
      <div
        class="ai-config-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="ai-config-dialog-title"
        @keydown.esc.stop.prevent="requestClose"
      >
        <header class="ai-dialog-header">
          <div><h3 id="ai-config-dialog-title">{{ editing ? '编辑模型配置' : '新增模型配置' }}</h3><p>先填写服务信息，再获取模型或手动输入。</p></div>
          <button type="button" class="ai-dialog-close" aria-label="关闭模型配置" :disabled="busy" @click="requestClose">×</button>
        </header>
        <form @submit.prevent="save">
          <div class="ai-config-body">
            <p v-if="error" class="ai-feedback error" role="alert">{{ error }}</p>
            <p v-if="notice" class="ai-feedback success" role="status">{{ notice }}</p>
            <div class="ai-config-form">
              <label class="ai-field">
                <span class="ai-field-label">配置名称<span class="ai-required" aria-hidden="true">*</span></span>
                <input v-model.trim="form.label" maxlength="50" required autocomplete="off" placeholder="例如：文章生成主模型" />
              </label>
              <div class="ai-field" data-field="provider">
                <label class="ai-field-label" id="ai-provider-label">{{ t("contentV04.ai.provider") }}<span class="ai-required" aria-hidden="true">*</span></label>
                <Select
                  v-model="form.provider"
                  :options="providerOptions"
                  :aria-labelledby="'ai-provider-label'"
                />
              </div>
              <label class="ai-field ai-field-wide">
                <span class="ai-field-label">{{ t("contentV04.ai.baseUrl") }}<span class="ai-required" aria-hidden="true">*</span></span>
                <input v-model.trim="form.baseUrl" type="url" required placeholder="https://api.example.com/v1" />
              </label>
              <label class="ai-field ai-field-wide">
                <span class="ai-field-label">{{ t("contentV04.ai.apiKey") }}<span v-if="!editing" class="ai-required" aria-hidden="true">*</span></span>
                <input v-model="form.apiKey" type="password" autocomplete="new-password" :placeholder="editing?.apiKeyMasked || (editing ? '留空则保留原密钥' : t('contentV04.ai.apiKey'))" />
              </label>
              <label class="ai-field ai-field-wide">
                <span class="ai-field-label">{{ t("contentV04.ai.models") }}<span class="ai-required" aria-hidden="true">*</span></span>
                <textarea v-model="form.modelsText" rows="4" placeholder="每行一个模型名称"></textarea>
              </label>
              <label class="ai-field ai-field-wide">
                <span class="ai-field-label">{{ t("contentV04.ai.default") }}<span class="ai-required" aria-hidden="true">*</span></span>
                <Select v-if="modelNames.length" v-model="form.defaultModel" :options="modelOptions" />
                <input v-else v-model.trim="form.defaultModel" required :placeholder="t('contentV04.ai.manualModel')" />
              </label>
              <label class="ai-field ai-field-wide ai-checkbox-field">
                <input v-model="form.isDefault" type="checkbox" />
                <span class="ai-field-label">{{ t('contentV04.ai.setDefault') }}</span>
              </label>
            </div>
            <div class="ai-config-tools">
              <button type="button" class="secondary-button" :disabled="busy" @click="fetchModels">{{ t("contentV04.ai.fetchModels") }}</button>
              <button type="button" class="secondary-button" :disabled="busy" @click="testConnection">{{ t("contentV04.ai.testConnection") }}</button>
            </div>
          </div>
          <footer class="ai-dialog-footer">
            <button type="button" class="secondary-button" :disabled="busy" @click="requestClose">取消</button>
            <button type="submit" class="button primary" :disabled="busy">{{ busy ? '处理中…' : t('contentV04.ai.save') }}</button>
          </footer>
        </form>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import contentAPI from '@/api/admin/content'
import Select from '@/components/common/Select.vue'

const i18n = useI18n()
const t = i18n.t
const emit = defineEmits<{ 'configs-updated': [configs: any[]] }>()

/** PRD R10 预设厂商清单；「自定义」按 OpenAI 兼容协议对接。 */
const providers = [
  { id: 'openai', name: 'OpenAI', baseUrl: 'https://api.openai.com/v1' },
  { id: 'anthropic', name: 'Anthropic', baseUrl: 'https://api.anthropic.com/v1' },
  { id: 'deepseek', name: 'DeepSeek', baseUrl: 'https://api.deepseek.com/v1' },
  { id: 'zhipu', name: '智谱', baseUrl: 'https://open.bigmodel.cn/api/paas/v4' },
  { id: 'moonshot', name: '月之暗面', baseUrl: 'https://api.moonshot.cn/v1' },
  { id: 'qwen', name: '阿里通义', baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1' },
  { id: 'custom', name: '自定义（Custom / OpenAI 兼容）', baseUrl: '' },
]
/** 老配置里存过的厂商 id，只做展示兜底，不再出现在下拉里。 */
const legacyProviders: Record<string, string> = { openrouter: 'OpenRouter', ollama: 'Ollama' }

const configs = ref<any[]>([])
const revision = ref('')
const busy = ref(false)
const error = ref('')
const notice = ref('')
const pageError = ref('')
const pageNotice = ref('')
const editing = ref<any>(null)
const dialogOpen = ref(false)
/** 弹窗打开那一刻的表单快照，用来判断是否有未保存修改。 */
const pristine = ref('')

const form = reactive({ label: '', provider: 'openai', baseUrl: 'https://api.openai.com/v1', apiKey: '', modelsText: '', defaultModel: '', isDefault: false })

/** Select 组件默认读 options 的 value/label 两个键，这里保持 {value,label} 结构即可。 */
const providerOptions = computed(() => {
  const base = providers.map((p) => ({ value: p.id, label: p.name }))
  const currentId = editing.value?.provider
  if (currentId && !providers.some((p) => p.id === currentId)) {
    base.push({ value: currentId, label: legacyProviders[currentId] || currentId })
  }
  return base
})
const modelNames = computed(() => [...new Set(form.modelsText.split(/[\n,]/).map((x) => x.trim()).filter(Boolean))])
const modelOptions = computed(() => modelNames.value.map((x) => ({ value: x, label: x })))

// 填了模型列表后，默认模型必须落在列表内：为空或已失效时自动取第一个，避免保存时报“默认模型不能为空”。
watch(modelNames, (list) => {
  if (!list.length) return
  if (!form.defaultModel || !list.includes(form.defaultModel)) form.defaultModel = list[0]
})
const snapshot = () => JSON.stringify({ l: form.label, p: form.provider, b: form.baseUrl, k: form.apiKey, m: form.modelsText, d: form.defaultModel, def: form.isDefault })
const isDirty = computed(() => dialogOpen.value && snapshot() !== pristine.value)

const payload = (response: any) => response?.data?.data ?? response?.data ?? response ?? {}

async function load() {
  try {
    const r = await contentAPI.listAIConfigs()
    const d = payload(r)
    configs.value = d.configs || []
    revision.value = d.revision || r.headers?.etag?.replaceAll('"', '') || ''
    emit('configs-updated', configs.value)
  } catch {
    pageError.value = '配置加载失败，请刷新重试'
  }
}
function providerLabel(id: string) {
  return providers.find((p) => p.id === id)?.name || legacyProviders[id] || id
}
/** 列表只展示掩码：优先后端给的 apiKeyMasked；后端没给时才用固定掩码占位。 */
function keyDisplay(c: any) {
  if (c?.apiKeyMasked) return c.apiKeyMasked
  if (c?.hasApiKey) return 'sk-****'
  return '—'
}
function resetForm() {
  Object.assign(form, { label: '', provider: 'openai', baseUrl: 'https://api.openai.com/v1', apiKey: '', modelsText: '', defaultModel: '', isDefault: false })
  error.value = ''
  notice.value = ''
}
async function openDialog() {
  dialogOpen.value = true
  await nextTick()
  pristine.value = snapshot()
  document.querySelector<HTMLInputElement>('.ai-config-dialog input')?.focus()
}
function openCreate() {
  editing.value = null
  resetForm()
  void openDialog()
}
function edit(c: any) {
  editing.value = c
  Object.assign(form, {
    label: c.label,
    provider: c.provider,
    baseUrl: c.baseUrl,
    apiKey: '',
    modelsText: (c.models || []).join('\n'),
    defaultModel: c.defaultModel || c.models?.[0] || '',
    isDefault: !!c.isDefault,
  })
  error.value = ''
  notice.value = ''
  void openDialog()
}

const DISCARD_MESSAGES: Record<string, string> = {
  en: 'Discard unsaved changes?',
  zh: '有未保存的修改，确定放弃吗？',
  ru: 'Отменить несохранённые изменения?',
  fr: 'Abandonner les modifications non enregistrées ?',
  de: 'Nicht gespeicherte Änderungen verwerfen?',
}
function discardMessage() {
  const holder = i18n as unknown as { locale?: { value?: unknown } }
  const raw = String(holder.locale?.value ?? 'en').slice(0, 2).toLowerCase()
  return DISCARD_MESSAGES[raw] || DISCARD_MESSAGES.en
}
/** ×、取消、Esc、点遮罩统一走这里：有未保存内容先二次确认。 */
function requestClose() {
  if (busy.value) return
  if (isDirty.value && !window.confirm(discardMessage())) return
  closeDialog()
}
function closeDialog() {
  dialogOpen.value = false
  editing.value = null
  pristine.value = ''
  resetForm()
}

async function mutate(action: 'save' | 'default' | 'delete', body: any) {
  const send = async () => payload(await contentAPI.mutateAIConfig(action, body, revision.value))
  let d: any
  try {
    d = await send()
  } catch (e: any) {
    const status = e?.response?.status
    const reason = String(e?.response?.data?.reason || '')
    const message = String(e?.response?.data?.message || '')
    // revision 过期（面板停留过久 / 多端同时修改）时自动重取最新 revision 再试一次，
    // 否则用户看到的永远是"保存失败"。
    if (status === 409 && /revision/i.test(reason + message)) {
      await load()
      d = await send()
    } else {
      throw e
    }
  }
  if (d.revision) revision.value = d.revision
  await load()
}
/** 把后端 reason 翻成人话，避免只看到一句"失败"。 */
function describeError(e: any, fallback: string) {
  const data = e?.response?.data || {}
  const raw = String(data.message || data.error || '')
  if (/revision/i.test(String(data.reason || '') + raw)) return '配置已被其他操作更新，请重试'
  if (/model_in_use/.test(raw)) return '该配置正被进行中的写作/翻译任务使用，请等任务结束再删除'
  if (/duplicate/i.test(raw)) return '配置名称已存在，请换一个名称'
  if (/encryption_not_configured/.test(raw)) return '服务端未配置加密密钥（TOTP_ENCRYPTION_KEY），无法保存 API Key'
  return raw || fallback
}
async function save() {
  error.value = ''
  notice.value = ''
  if (!form.label.trim() || [...form.label.trim()].length > 50) {
    error.value = '配置名称必填，且最多 50 个字符'
    return
  }
  if (configs.value.some((c) => c.id !== editing.value?.id && c.label.trim().toLocaleLowerCase() === form.label.trim().toLocaleLowerCase())) {
    error.value = '配置名称不能重复'
    return
  }
  if (!form.defaultModel.trim()) {
    error.value = '默认模型不能为空'
    return
  }
  if (!editing.value && !form.apiKey.trim()) {
    error.value = '新配置必须填写 API Key'
    return
  }
  busy.value = true
  try {
    await mutate('save', {
      id: editing.value?.id,
      label: form.label.trim(),
      provider: form.provider,
      baseUrl: form.baseUrl.trim(),
      apiKey: form.apiKey,
      models: modelNames.value,
      defaultModel: form.defaultModel,
      // 勾选框决定是否设为默认；新建第一条配置时也视为默认。
      isDefault: form.isDefault || !configs.value.length,
    })
    busy.value = false
    pristine.value = snapshot()
    closeDialog()
    pageNotice.value = '模型配置已保存'
  } catch (e: any) {
    error.value = describeError(e, '配置保存失败，请刷新后重试')
  } finally {
    busy.value = false
  }
}
async function setDefault(c: any) {
  busy.value = true
  pageError.value = ''
  try {
    await mutate('default', { id: c.id })
    pageNotice.value = '默认配置已更新'
  } catch (e: any) {
    pageError.value = describeError(e, '默认配置更新失败')
  } finally {
    busy.value = false
  }
}
async function remove(c: any) {
  if (!window.confirm(`${t('contentV04.ai.deleteConfirm')} (${c.label})`)) return
  busy.value = true
  pageError.value = ''
  try {
    await mutate('delete', { id: c.id })
    pageNotice.value = '配置已删除'
  } catch (e: any) {
    pageError.value = describeError(e, '配置正在使用，无法删除')
  } finally {
    busy.value = false
  }
}
async function testConnection() {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const d = payload(await contentAPI.testAIConfig({ baseUrl: form.baseUrl, apiKey: form.apiKey }))
    if (d.ok) notice.value = `连接成功（${d.latencyMs ?? 0} ms）`
    else error.value = `连接失败：${d.error || '请求失败'}`
  } catch {
    error.value = '连接测试失败，请检查 Base URL 和 Key'
  } finally {
    busy.value = false
  }
}
async function fetchModels() {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const d = payload(await contentAPI.fetchAIModels({ baseUrl: form.baseUrl, apiKey: form.apiKey }))
    const list = d.models || d.data?.models || []
    if (!list.length) {
      notice.value = '未获取到模型，可手动输入模型名'
      return
    }
    form.modelsText = list.join('\n')
    if (!form.defaultModel) form.defaultModel = list[0]
    notice.value = `已获取 ${list.length} 个模型`
  } catch {
    error.value = '获取模型失败；可以手动输入模型名'
  } finally {
    busy.value = false
  }
}
onMounted(load)
</script>

<style scoped>
.ai-config-panel {
  max-width: 100%;
  color: #121316;
}
.ai-config-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 20px;
}
.ai-config-header h3 {
  margin: 0 0 6px;
  font-size: 18px;
}
.ai-config-header p,
.ai-config-dialog header p {
  margin: 0;
  color: #5e6577;
  line-height: 1.5;
}
.ai-config-empty {
  padding: 32px;
  border: 2px dashed #9aa1b1;
  border-radius: 12px;
  text-align: center;
  color: #5e6577;
}
.ai-config-list {
  display: grid;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.ai-config-list li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px;
  border: 2px solid #121316;
  border-radius: 12px;
  background: #fff;
}
.ai-config-summary {
  display: grid;
  gap: 6px;
  min-width: 0;
}
.ai-config-summary > span,
.ai-config-summary small {
  overflow-wrap: anywhere;
  color: #5e6577;
}
.ai-config-title {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ai-checkbox-field {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ai-checkbox-field input[type='checkbox'] {
  width: 16px;
  height: 16px;
  margin: 0;
}
.default-badge {
  padding: 2px 7px;
  border: 1px solid #121316;
  border-radius: 5px;
  background: #ccff00;
  font-size: 11px;
  font-weight: 800;
}
.key-unreadable-badge {
  padding: 2px 7px;
  border: 1px solid #121316;
  border-radius: 5px;
  background: #ffb4a8;
  font-size: 11px;
  font-weight: 800;
}
.ai-config-row-actions,
.ai-config-tools {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  flex-shrink: 0;
}
.ai-feedback {
  margin: 0 0 16px;
  padding: 10px 12px;
  border: 1.5px solid;
  border-radius: 8px;
  font-size: 13px;
  line-height: 1.5;
}
.ai-feedback.error {
  border-color: #b42318;
  background: #fff1f0;
  color: #b42318;
}
.ai-feedback.success {
  border-color: #1f6e43;
  background: #eefbf3;
  color: #1f6e43;
}
.ai-config-modal {
  position: fixed;
  inset: 0;
  z-index: 1400;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: rgba(18, 19, 22, 0.55);
}
/* 中弹窗：宽 640px、圆角 12px、页头/页脚固定、仅内容区滚动（UI 规范 1.3 / 3.6） */
.ai-config-dialog {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  width: min(640px, 100%);
  max-height: calc(100dvh - 32px);
  overflow: hidden;
  border: 2px solid #121316;
  border-radius: 12px;
  background: #fff;
  box-shadow: 6px 6px 0 #121316;
  color: #121316;
}
.ai-dialog-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 20px 22px 0;
}
.ai-config-dialog h3 {
  margin: 0 0 6px;
  font-size: 20px;
}
.ai-dialog-close {
  flex: 0 0 36px;
  width: 36px;
  height: 36px;
  border: 2px solid #121316;
  border-radius: 8px;
  background: #fff;
  font-size: 24px;
  line-height: 1;
  cursor: pointer;
}
.ai-config-dialog form {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}
.ai-config-body {
  overflow-y: auto;
  padding: 18px 22px 0;
}
.ai-config-form {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}
.ai-field {
  display: grid;
  gap: 7px;
  min-width: 0;
  font-size: 13px;
  font-weight: 700;
}
.ai-field-label {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}
.ai-required {
  color: #b42318;
  font-weight: 800;
}
.ai-config-form input,
.ai-config-form textarea {
  box-sizing: border-box;
  width: 100%;
  min-height: 42px;
  padding: 9px 11px;
  border: 1.5px solid #9aa1b1;
  border-radius: 8px;
  background: #fff;
  color: #121316;
  font: inherit;
  font-weight: 400;
  line-height: 1.5;
}
.ai-config-form textarea {
  resize: vertical;
  min-height: 96px;
}
.ai-field-wide {
  grid-column: 1 / -1;
}
.ai-config-tools {
  margin: 18px 0 0;
}
.ai-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 16px 22px 20px;
  border-top: 1.5px solid #d5d9e2;
  background: #fff;
}
.danger-button:disabled,
.secondary-button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
@media (max-width: 640px) {
  .ai-config-list li,
  .ai-config-header {
    align-items: stretch;
    flex-direction: column;
  }
  .ai-config-form {
    grid-template-columns: 1fr;
  }
  .ai-config-dialog {
    max-height: calc(100dvh - 16px);
  }
  .ai-dialog-header,
  .ai-config-body,
  .ai-dialog-footer {
    padding-left: 16px;
    padding-right: 16px;
  }
  .ai-field-wide {
    grid-column: auto;
  }
  .ai-config-row-actions > * {
    flex: 1;
  }
}
</style>
