<template>
  <div class="space-y-5">
    <section class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
      <h3 class="font-medium">计费语义与能力</h3>
      <div class="mt-3 grid gap-4 md:grid-cols-2">
        <label>成本模式
          <select v-model="draft.cost_mode" class="input mt-1 w-full">
            <option value="manual">人工维护</option>
            <option value="contract_multiplier">合同倍率</option>
            <option value="request_charge">上游响应实际扣费</option>
            <option value="cumulative_list_and_actual">累计原价与实扣</option>
			<option value="cumulative_actual">仅累计实际扣费</option>
          </select>
        </label>
        <label>上游单位性质
          <select v-model="draft.unit_semantics" class="input mt-1 w-full">
            <option value="none">无成本单位</option>
            <option value="fiat_currency">法定货币</option>
            <option value="platform_credit">平台积分</option>
          </select>
        </label>
		<label v-if="isCumulativeMode">累计计数器归属
          <select v-model="draft.counter_scope" class="input mt-1 w-full" required>
            <option value="">请选择</option>
            <option value="account">账号独立计数器</option>
            <option value="wallet">钱包共享计数器</option>
            <option value="organization">组织共享计数器</option>
          </select>
          <span class="mt-1 block text-xs text-gray-500">共享计数器在完成钱包级分摊配置前只保存观测，不会按账号重复结算。</span>
        </label>
      </div>
      <fieldset class="mt-4">
        <legend class="text-sm text-gray-600 dark:text-gray-300">协议能力</legend>
        <div class="mt-2 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          <label v-for="option in capabilityOptions" :key="option.value" class="flex items-center gap-2 rounded border border-gray-200 px-3 py-2 text-sm dark:border-dark-700">
            <input v-model="draft.capabilities" type="checkbox" :value="option.value" />{{ option.label }}
          </label>
        </div>
      </fieldset>
    </section>

    <section class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
      <h3 class="font-medium">认证方式</h3>
      <p class="mt-1 text-xs text-gray-500">这里只声明凭据从哪里读取，不填写或保存真实密钥。</p>
      <div class="mt-3 grid gap-4 md:grid-cols-3">
        <label>认证类型
          <select v-model="draft.authentication.type" class="input mt-1 w-full">
            <option value="none">无需认证</option><option value="bearer">Bearer</option><option value="api_key_header">API Key 请求头</option>
          </select>
        </label>
        <label>凭据来源
          <select v-model="draft.authentication.credential_source" class="input mt-1 w-full" :disabled="draft.authentication.type === 'none'">
            <option value="">不使用</option><option value="wallet_finance_credential">钱包财务凭据</option><option value="account_api_key">账号 API Key</option><option value="account_access_token">账号 Access Token</option><option value="account_token">账号 Token</option><option value="account_setup_token">账号 Setup Token</option>
          </select>
        </label>
        <label v-if="draft.authentication.type === 'api_key_header'">请求头名
          <input v-model="draft.authentication.header_name" class="input mt-1 w-full" placeholder="X-Finance-Key" />
        </label>
      </div>
    </section>

    <section class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
      <div class="flex items-center justify-between gap-3"><div><h3 class="font-medium">自动识别规则</h3><p class="mt-1 text-xs text-gray-500">按顺序探测，响应满足条件后绑定该协议版本。</p></div><button type="button" class="btn btn-secondary" @click="addRecognition">新增规则</button></div>
      <div v-if="draft.recognition.length" class="mt-3 space-y-3">
        <div v-for="(rule, index) in draft.recognition" :key="index" class="grid gap-3 rounded bg-gray-50 p-3 md:grid-cols-6 dark:bg-dark-900">
          <select v-model="rule.method" class="input"><option>GET</option></select>
          <input v-model="rule.path" class="input md:col-span-2" placeholder="/api/status" />
          <input v-model="rule.match.path" class="input md:col-span-2" placeholder="$.data.version" />
          <button type="button" class="text-sm text-red-600" @click="draft.recognition.splice(index, 1)">删除</button>
          <input v-model="rule.platform" class="input" placeholder="平台（可选）" />
          <input v-model="rule.account_type" class="input" placeholder="账号类型（可选）" />
          <input v-model.number="rule.match.status" type="number" class="input" placeholder="HTTP 状态" />
          <input v-model="rule.match.equals" class="input md:col-span-2" placeholder="等于值（可选）" />
        </div>
      </div>
      <p v-else class="mt-3 text-sm text-gray-500">未配置自动识别规则，钱包需手工选择协议。</p>
    </section>

    <section class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
      <h3 class="font-medium">接口与字段映射</h3>
      <p class="mt-1 text-xs text-gray-500">每项能力使用独立接口；映射目标是系统字段，来源为 JSONPath。</p>
      <div v-if="draft.capabilities.length" class="mt-3 space-y-4">
        <article v-for="capability in draft.capabilities" :key="capability" class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900">
          <div class="flex flex-wrap items-center justify-between gap-3"><h4 class="font-medium">{{ capabilityLabel(capability) }}</h4><button type="button" class="text-sm text-primary-700 dark:text-primary-400" @click="addMapping(capability)">新增字段映射</button></div>
          <div class="mt-3 grid gap-3 md:grid-cols-4">
            <label>请求方式<select v-model="operation(capability).method" class="input mt-1 w-full"><option>GET</option><option>POST</option></select></label>
            <label class="md:col-span-2">接口路径<input v-model="operation(capability).path" class="input mt-1 w-full" placeholder="/api/finance/usage" /></label>
            <label>证据类型<input v-model="operation(capability).evidence_type" class="input mt-1 w-full" placeholder="cumulative" /></label>
            <label>SSE 完成事件<input v-model="operation(capability).sse_event" class="input mt-1 w-full" placeholder="response.completed" /></label>
            <label>分页方式<select v-model="operation(capability).pagination.type" class="input mt-1 w-full"><option value="">不分页</option><option value="page">页码</option><option value="cursor">游标</option></select></label>
            <label v-if="operation(capability).pagination.type === 'page'">页码参数<input v-model="operation(capability).pagination.page_parameter" class="input mt-1 w-full" placeholder="page" /></label>
            <label v-if="operation(capability).pagination.type === 'cursor'">游标参数<input v-model="operation(capability).pagination.cursor_parameter" class="input mt-1 w-full" placeholder="cursor" /></label>
            <label v-if="operation(capability).pagination.type === 'cursor'">响应游标路径<input v-model="operation(capability).pagination.cursor_path" class="input mt-1 w-full" placeholder="$.next_cursor" /></label>
            <label v-if="operation(capability).pagination.type">最大页数<input v-model.number="operation(capability).pagination.max_pages" type="number" min="1" max="100" class="input mt-1 w-full" /></label>
            <label class="md:col-span-4">受限请求体 JSON<textarea v-model="operation(capability).body_json" class="input mt-1 h-24 w-full font-mono text-xs" placeholder='{"range":"{{period}}"}' /></label>
          </div>
          <div class="mt-3 space-y-2">
            <div v-for="(mapping, index) in operation(capability).mapping_rows" :key="index" class="flex flex-wrap gap-2">
              <input v-model="mapping.field" class="input min-w-40 flex-1" placeholder="系统字段，如 actual_cost" />
              <input v-model="mapping.path" class="input min-w-48 flex-[2]" placeholder="JSONPath，如 $.data.actual_cost" />
              <button type="button" class="px-2 text-sm text-red-600" @click="operation(capability).mapping_rows.splice(index, 1)">删除</button>
            </div>
          </div>
        </article>
      </div>
      <p v-else class="mt-3 text-sm text-gray-500">先选择至少一项协议能力。</p>
    </section>

    <section class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
      <h3 class="font-medium">响应脱敏路径</h3>
      <p class="mt-1 text-xs text-gray-500">系统还会递归清除常见敏感字段；这里补充该上游的专有字段。</p>
      <textarea v-model="redactText" class="input mt-3 h-24 w-full font-mono text-xs" placeholder="$.data.secret&#10;$.token" />
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'

type MappingRow = { field: string; path: string }
type EditableOperation = {
  method: string; path: string; evidence_type: string; sse_event: string; body_json: string
  pagination: { type: string; page_parameter: string; cursor_path: string; cursor_parameter: string; max_pages: number }
  mapping_rows: MappingRow[]
}
type EditableConfig = {
  capabilities: string[]; cost_mode: string; unit_semantics: string; counter_scope: string
  authentication: { type: string; credential_source: string; header_name: string }
  recognition: Array<{ method: string; path: string; platform: string; account_type: string; match: { path: string; status?: number; equals?: string } }>
  operations: Record<string, EditableOperation>; redact_paths: string[]
}

const props = defineProps<{ modelValue: Record<string, unknown> }>()
const emit = defineEmits<{ 'update:modelValue': [value: Record<string, unknown>] }>()
const capabilityOptions = [
  { value: 'pricing', label: '模型定价' }, { value: 'account_usage', label: '账号累计费用' },
  { value: 'balance', label: '现金余额' },
  { value: 'funding_transactions', label: '充值交易' }, { value: 'quota', label: '额度/积分余额' },
]
const capabilityLabels = Object.fromEntries(capabilityOptions.map(item => [item.value, item.label]))
let applyingExternal = false
const draft = reactive<EditableConfig>(normalizeConfig(props.modelValue))
const redactText = ref(draft.redact_paths.join('\n'))
const isCumulativeMode = computed(() => draft.cost_mode === 'cumulative_list_and_actual' || draft.cost_mode === 'cumulative_actual')

function normalizeOperation(value: unknown): EditableOperation {
  const source = (value && typeof value === 'object' ? value : {}) as Record<string, any>
  const mapping = (source.mapping && typeof source.mapping === 'object' ? source.mapping : {}) as Record<string, unknown>
  const pagination = (source.pagination && typeof source.pagination === 'object' ? source.pagination : {}) as Record<string, any>
  return {
    method: String(source.method || 'GET').toUpperCase(), path: String(source.path || ''), evidence_type: String(source.evidence_type || ''),
    sse_event: String(source.sse_event || ''), body_json: source.body === undefined ? '' : JSON.stringify(source.body, null, 2),
    pagination: { type: String(pagination.type || ''), page_parameter: String(pagination.page_parameter || ''), cursor_path: String(pagination.cursor_path || ''), cursor_parameter: String(pagination.cursor_parameter || ''), max_pages: Number(pagination.max_pages || 10) },
    mapping_rows: Object.entries(mapping).map(([field, path]) => ({ field, path: String(path ?? '') })),
  }
}
function normalizeConfig(value: Record<string, unknown>): EditableConfig {
  const auth = (value.authentication && typeof value.authentication === 'object' ? value.authentication : {}) as Record<string, any>
  const sourceOperations = (value.operations && typeof value.operations === 'object' ? value.operations : {}) as Record<string, unknown>
  const operations: Record<string, EditableOperation> = {}
  for (const [key, operationValue] of Object.entries(sourceOperations)) operations[key] = normalizeOperation(operationValue)
  return {
    capabilities: Array.isArray(value.capabilities) ? value.capabilities.map(String) : [], cost_mode: String(value.cost_mode || 'manual'), unit_semantics: String(value.unit_semantics || 'none'), counter_scope: String(value.counter_scope || ''),
    authentication: { type: String(auth.type || 'none'), credential_source: String(auth.credential_source || ''), header_name: String(auth.header_name || '') },
    recognition: Array.isArray(value.recognition) ? value.recognition.map((raw: any) => ({ method: String(raw?.method || 'GET'), path: String(raw?.path || ''), platform: String(raw?.platform || ''), account_type: String(raw?.account_type || ''), match: { path: String(raw?.match?.path || ''), status: raw?.match?.status ? Number(raw.match.status) : undefined, equals: raw?.match?.equals === undefined ? undefined : String(raw.match.equals) } })) : [],
    operations, redact_paths: Array.isArray(value.redact_paths) ? value.redact_paths.map(String) : [],
  }
}
function replaceDraft(value: Record<string, unknown>) {
  applyingExternal = true
  const next = normalizeConfig(value)
  Object.assign(draft, next)
  draft.operations = next.operations
  draft.recognition = next.recognition
  draft.capabilities = next.capabilities
  redactText.value = next.redact_paths.join('\n')
  queueMicrotask(() => { applyingExternal = false })
}
function operation(capability: string) {
  if (!draft.operations[capability]) draft.operations[capability] = normalizeOperation(undefined)
  return draft.operations[capability]
}
function capabilityLabel(value: string) { return capabilityLabels[value] || value }
function addRecognition() { draft.recognition.push({ method: 'GET', path: '/', platform: '', account_type: '', match: { path: '$', status: 200 } }) }
function addMapping(capability: string) { operation(capability).mapping_rows.push({ field: '', path: '' }) }
function serializeConfig(): Record<string, unknown> {
  const operations: Record<string, unknown> = {}
  for (const capability of draft.capabilities) {
    const source = operation(capability)
    const mapping = Object.fromEntries(source.mapping_rows.filter(row => row.field.trim() && row.path.trim()).map(row => [row.field.trim(), row.path.trim()]))
    const item: Record<string, unknown> = { method: source.method, path: source.path, mapping }
    if (source.evidence_type.trim()) item.evidence_type = source.evidence_type.trim()
    if (source.sse_event.trim()) item.sse_event = source.sse_event.trim()
    if (source.body_json.trim()) { try { item.body = JSON.parse(source.body_json) } catch { item.body = source.body_json } }
    if (source.pagination.type) item.pagination = { type: source.pagination.type, page_parameter: source.pagination.page_parameter || undefined, cursor_path: source.pagination.cursor_path || undefined, cursor_parameter: source.pagination.cursor_parameter || undefined, max_pages: source.pagination.max_pages }
    operations[capability] = item
  }
  const authentication: Record<string, unknown> = { type: draft.authentication.type, credential_source: draft.authentication.type === 'none' ? '' : draft.authentication.credential_source }
  if (draft.authentication.header_name.trim()) authentication.header_name = draft.authentication.header_name.trim()
  return {
    capabilities: [...draft.capabilities], recognition: draft.recognition.map(rule => ({ method: rule.method, path: rule.path, match: { path: rule.match.path, status: rule.match.status || undefined, equals: rule.match.equals || undefined }, platform: rule.platform || undefined, account_type: rule.account_type || undefined })),
    authentication, operations, cost_mode: draft.cost_mode, unit_semantics: draft.unit_semantics,
		counter_scope: isCumulativeMode.value ? draft.counter_scope : undefined,
    redact_paths: redactText.value.split(/\r?\n/).map(item => item.trim()).filter(Boolean),
  }
}

watch(() => props.modelValue, value => replaceDraft(value), { deep: true })
watch([draft, redactText], () => { if (!applyingExternal) emit('update:modelValue', serializeConfig()) }, { deep: true })
</script>
