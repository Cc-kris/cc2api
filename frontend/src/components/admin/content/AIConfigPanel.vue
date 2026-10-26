<template>
  <section class="ai-config-panel" :aria-label="t('contentV04.ai.configuration')">
    <header><div><h3>{{ t("contentV04.ai.title") }} · {{ t("contentV04.ai.configuration") }}</h3><p>{{ t("contentV04.ai.configureFirst") }}</p></div><button type="button" class="button" @click="reset">新增配置</button></header>
    <p v-if="error" class="error-message" role="alert">{{ error }}</p><p v-if="notice" class="success-message" role="status">{{ notice }}</p>
    <form class="ai-config-form" @submit.prevent="save">
      <label>配置名称<input v-model.trim="form.label" maxlength="50" required autocomplete="off" /></label>
      <label>{{ t("contentV04.ai.provider") }}<Select v-model="form.provider" :options="providerOptions" value-key="id" label-key="name" :aria-label="t('contentV04.ai.provider')" /></label>
      <label>{{ t("contentV04.ai.baseUrl") }}<input v-model.trim="form.baseUrl" type="url" required placeholder="https://api.example.com/v1" /></label>
      <label>{{ t("contentV04.ai.apiKey") }}<input v-model="form.apiKey" type="password" autocomplete="new-password" :placeholder="editing?.apiKeyMasked || t('contentV04.ai.apiKey')" /></label>
      <label class="models-field">{{ t("contentV04.ai.models") }}<textarea v-model="form.modelsText" rows="3"  :placeholder="t('contentV04.ai.manualModel')" /></label>
      <label>{{ t("contentV04.ai.default") }}<Select v-if="modelNames.length" v-model="form.defaultModel" :options="modelOptions" :aria-label="t('contentV04.ai.default')" /><input v-else v-model.trim="form.defaultModel" required :placeholder="t('contentV04.ai.manualModel')" /></label>
      <div class="ai-config-actions"><button type="button" class="button" :disabled="busy" @click="fetchModels">{{ t("contentV04.ai.fetchModels") }}</button><button type="button" class="button" :disabled="busy" @click="testConnection">{{ t("contentV04.ai.testConnection") }}</button><button class="button primary" :disabled="busy">{{ editing ? t('contentV04.ai.save') : t('contentV04.ai.save') }}</button></div>
    </form>
    <h4>{{ t("contentV04.ai.configuration") }}</h4><p v-if="!configs.length" class="muted">{{ t("contentV04.ai.configureFirst") }}</p>
    <ul class="ai-config-list"><li v-for="c in configs" :key="c.id"><div><strong>{{ c.label }}</strong><span>{{ providerLabel(c.provider) }} · {{ c.baseUrl }} · {{ (c.models || []).length }} {{ t("contentV04.ai.models") }}</span><small>{{ c.isDefault ? t('contentV04.ai.default') : '未设为默认' }} · {{ c.apiKeyMasked || (c.hasApiKey ? t('contentV04.ai.maskedKey') : '未配置 API Key') }}</small></div><div class="ai-config-actions"><button type="button" class="button" :disabled="c.isDefault || busy" @click="setDefault(c)">{{ c.isDefault ? t('contentV04.ai.default') : t('contentV04.ai.setDefault') }}</button><button type="button" class="button" @click="edit(c)">{{ t("contentV04.ai.edit") }}</button><button type="button" class="button danger" :disabled="c.isDefault || busy" @click="remove(c)">{{ t("contentV04.ai.delete") }}</button></div></li></ul>
  </section>
</template>
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import contentAPI from '@/api/admin/content'
import Select from '@/components/common/Select.vue'
const { t } = useI18n()
const emit = defineEmits<{ (e: 'configs-updated', configs: any[]): void }>()
const providers = [{id:'openai',name:'OpenAI'},{id:'anthropic',name:'Anthropic'},{id:'deepseek',name:'DeepSeek'},{id:'zhipu',name:'智谱'},{id:'moonshot',name:'月之暗面'},{id:'aliyun',name:'阿里通义'},{id:'custom',name:'自定义'}]
const configs = ref<any[]>([]), revision = ref(''), error = ref(''), notice = ref(''), busy = ref(false), editing = ref<any>(null)
const form = reactive({ label:'', provider:'openai', baseUrl:'https://api.openai.com/v1', apiKey:'', modelsText:'', defaultModel:'' })
const modelNames = computed(() => form.modelsText.split(/[\n,]+/).map(v => v.trim()).filter(Boolean))
const providerOptions = providers.map(p => ({ id: p.id, name: p.name }))
const modelOptions = computed(() => modelNames.value.map(name => ({ value: name, label: name })))
const payload = (response: any) => response?.data?.data ?? response?.data ?? response ?? {}
async function load() { try { const r=await contentAPI.listAIConfigs(); const d=payload(r); configs.value=d.configs||[]; revision.value=d.revision||r.headers?.etag?.replaceAll('"','')||''; emit('configs-updated',configs.value) } catch { error.value='配置加载失败，请刷新重试' } }
function providerLabel(id:string) { return providers.find(p => p.id === id)?.name || id }
function reset() { editing.value=null; Object.assign(form,{label:'',provider:'openai',baseUrl:'https://api.openai.com/v1',apiKey:'',modelsText:'',defaultModel:''}); error.value=''; notice.value='' }
function edit(c:any) { editing.value=c; Object.assign(form,{label:c.label,provider:c.provider,baseUrl:c.baseUrl,apiKey:'',modelsText:(c.models||[]).join('\n'),defaultModel:c.defaultModel||c.models?.[0]||''}); error.value=''; notice.value='' }
async function mutate(action:'save'|'default'|'delete',body:any) { const r=await contentAPI.mutateAIConfig(action,body,revision.value); const d=payload(r); if(d.revision) revision.value=d.revision; await load() }
async function save() { error.value=''; if(!form.label.trim()||[...form.label.trim()].length>50){error.value='配置名称必填，且最多 50 个字符';return} if(configs.value.some(c=>c.id!==editing.value?.id&&c.label.trim().toLocaleLowerCase()===form.label.trim().toLocaleLowerCase())){error.value='配置名称不能重复';return} if(!form.defaultModel.trim()){error.value='默认模型不能为空';return} if(!editing.value && !form.apiKey.trim()){error.value='新配置必须填写 API Key';return} busy.value=true; try { await mutate('save',{id:editing.value?.id,label:form.label.trim(),provider:form.provider,baseUrl:form.baseUrl.trim(),apiKey:form.apiKey,models:modelNames.value,defaultModel:form.defaultModel,isDefault:editing.value?.isDefault||!configs.value.length}); reset(); notice.value='配置已保存' } catch(e:any){error.value=e?.response?.data?.message||e?.response?.data?.error||'配置保存失败，请刷新后重试'} finally{busy.value=false} }
async function setDefault(c:any){busy.value=true;error.value='';try{await mutate('default',{id:c.id});notice.value='默认配置已更新'}catch(e:any){error.value=e?.response?.data?.message||e?.response?.data?.error||'默认配置更新失败'}finally{busy.value=false}}
async function remove(c:any){if(c.isDefault||!window.confirm(`${t('contentV04.ai.deleteConfirm')} (${c.label})`))return;busy.value=true;error.value='';try{await mutate('delete',{id:c.id});notice.value='配置已删除'}catch(e:any){error.value=e?.response?.data?.message||e?.response?.data?.error||'配置正在使用，无法删除'}finally{busy.value=false}}
async function testConnection(){busy.value=true;error.value='';try{const d=payload(await contentAPI.testAIConfig({baseUrl:form.baseUrl,apiKey:form.apiKey}));notice.value=d.ok?`连接成功（${d.latencyMs??0} ms）`:`连接失败：${d.error||'请求失败'}`}catch{error.value='连接测试失败，请检查 Base URL 和 Key'}finally{busy.value=false}}
async function fetchModels(){busy.value=true;error.value='';try{const d=payload(await contentAPI.fetchAIModels({baseUrl:form.baseUrl,apiKey:form.apiKey}));const list=d.models||d.data?.models||[];if(!list.length){notice.value='未获取到模型，可手动输入模型名';return}form.modelsText=list.join('\n');if(!form.defaultModel)form.defaultModel=list[0];notice.value=`已获取 ${list.length} 个模型`}catch{error.value='获取模型失败；可以手动输入模型名'}finally{busy.value=false}}
onMounted(load)
</script>
