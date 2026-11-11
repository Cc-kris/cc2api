<script setup lang="ts">
// 移植自上游 de27905e8 UpstreamRequestIdHeaderField.vue；本地留空时默认读 x-request-id。
import { computed, useSlots } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import { DEFAULT_UPSTREAM_REQUEST_ID_HEADER, isValidUpstreamRequestIdHeader } from '@/utils/upstreamRequestIdHeader'

const props = defineProps<{
  platform?: string
  type?: string
  /** 批量编辑未勾选时禁用输入。 */
  disabled?: boolean
  /** 输入框 id，供批量编辑的勾选框 aria-controls 关联。 */
  inputId?: string
  labelId?: string
  /** 标签关联的控件 id；批量编辑关联到勾选框，与其他批量字段一致。 */
  labelFor?: string
  /** 覆盖默认提示（批量编辑说明不能清空）。 */
  hint?: string
}>()
const model = defineModel<string>({ default: '' })
const { t } = useI18n()
const slots = useSlots()

interface HeaderExample {
  label: string
  header: string
  note?: string
}

// 各平台官方 API 在响应头中声明请求 ID 的头名。
const OFFICIAL_REQUEST_ID_HEADERS: Record<string, { platform: string; header: string }> = {
  openai: { platform: 'OpenAI', header: 'x-request-id' },
  anthropic: { platform: 'Anthropic', header: 'request-id' },
  gemini: { platform: 'Gemini', header: 'x-goog-request-id' },
  antigravity: { platform: 'Antigravity', header: 'x-goog-request-id' },
  grok: { platform: 'Grok', header: 'xai-request-id' }
}

// 只有 API Key 接入可能指向第三方中继；OAuth 等直连官方。
const examples = computed<HeaderExample[]>(() => {
  const items: HeaderExample[] = []
  if (props.type === 'apikey') {
    items.push({
      label: 'sub2api',
      header: 'X-Client-Request-ID',
      note: t('admin.accounts.upstreamRequestIdHeaderHelp.sub2apiNote')
    })
    items.push({ label: 'new-api / one-api', header: 'X-Oneapi-Request-Id' })
  }
  const official = props.platform ? OFFICIAL_REQUEST_ID_HEADERS[props.platform] : undefined
  if (official) {
    items.push({
      label: t('admin.accounts.upstreamRequestIdHeaderHelp.official', { platform: official.platform }),
      header: official.header
    })
  }
  return items
})

const invalid = computed(() => !isValidUpstreamRequestIdHeader(model.value))
</script>

<template>
  <div>
    <div class="flex items-center" :class="slots['label-action'] && 'mb-3'">
      <label :id="labelId" class="input-label" :class="slots['label-action'] && 'mb-0'" :for="labelFor ?? inputId">{{ t('admin.accounts.upstreamRequestIdHeader') }}</label>
      <HelpTooltip width-class="w-80">
        <p>{{ t('admin.accounts.upstreamRequestIdHeaderHelp.intro', { header: DEFAULT_UPSTREAM_REQUEST_ID_HEADER }) }}</p>
        <template v-if="examples.length">
          <p class="mt-2 font-medium">{{ t('admin.accounts.upstreamRequestIdHeaderHelp.examplesTitle') }}</p>
          <ul class="mt-1 space-y-1.5">
            <li v-for="item in examples" :key="item.label">
              <div class="flex items-center justify-between gap-3">
                <span class="text-gray-300">{{ item.label }}</span>
                <code class="select-all rounded bg-white/10 px-1.5 py-0.5 font-mono text-[11px] text-white">{{ item.header }}</code>
              </div>
              <p v-if="item.note" class="mt-0.5 text-gray-400">{{ item.note }}</p>
            </li>
          </ul>
        </template>
      </HelpTooltip>
      <div v-if="slots['label-action']" class="ml-auto flex items-center">
        <slot name="label-action" />
      </div>
    </div>
    <input
      :id="inputId"
      v-model="model"
      data-testid="upstream-request-id-header"
      type="text"
      maxlength="64"
      autocomplete="off"
      spellcheck="false"
      :disabled="disabled"
      class="input font-mono"
      :class="[
        { 'border-red-500 focus:border-red-500 focus:ring-red-500': invalid },
        disabled && 'cursor-not-allowed opacity-50'
      ]"
      :placeholder="DEFAULT_UPSTREAM_REQUEST_ID_HEADER"
    />
    <p v-if="invalid" class="mt-1 text-xs text-red-600 dark:text-red-400" data-testid="upstream-request-id-header-error">
      {{ t('admin.accounts.upstreamRequestIdHeaderInvalid') }}
    </p>
    <p v-else class="input-hint">{{ hint || t('admin.accounts.upstreamRequestIdHeaderHint', { header: DEFAULT_UPSTREAM_REQUEST_ID_HEADER }) }}</p>
  </div>
</template>
