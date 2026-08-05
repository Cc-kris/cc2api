<template>
  <AppLayout>
    <div class="mx-auto flex h-[calc(100vh-7rem)] max-w-7xl flex-col gap-4 px-4 py-4 sm:px-6 lg:px-8">
      <div class="grid min-h-0 flex-1 grid-cols-1 gap-4 lg:grid-cols-[380px_minmax(0,1fr)]">
        <aside class="min-h-0 overflow-y-auto rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="space-y-5">
            <section class="space-y-2">
              <label class="form-label">{{ t('videoGeneration.apiKeyLabel') }} <span class="text-red-500">*</span></label>
              <Select
                v-model="form.apiKeyId"
                :options="apiKeyOptions"
                :disabled="loadingKeys"
                :placeholder="loadingKeys ? t('videoGeneration.loadingApiKeys') : t('videoGeneration.selectApiKey')"
              />
              <p v-if="!loadingKeys && apiKeyOptions.length === 0" class="text-xs text-amber-600 dark:text-amber-300">{{ t('videoGeneration.noApiKeys') }}</p>
            </section>

            <section class="space-y-2">
              <label class="form-label">{{ t('videoGeneration.modelLabel') }} <span class="text-red-500">*</span></label>
              <Select v-model="form.modelOption" :options="modelOptions" :placeholder="t('videoGeneration.selectModel')" />
            </section>

            <section class="grid grid-cols-2 gap-3">
              <div class="space-y-2">
                <label class="form-label">{{ t('videoGeneration.resolutionLabel') }} <span class="text-red-500">*</span></label>
                <Select v-model="form.resolution" :options="resolutionOptions" :placeholder="t('videoGeneration.selectResolution')" />
              </div>
              <div class="space-y-2">
                <label class="form-label">{{ t('videoGeneration.aspectRatioLabel') }} <span class="text-red-500">*</span></label>
                <Select v-model="form.aspectRatio" :options="aspectRatioOptions" />
              </div>
            </section>

            <section class="grid grid-cols-2 gap-3">
              <div class="space-y-2">
                <label class="form-label">{{ t('videoGeneration.durationLabel') }} <span class="text-red-500">*</span></label>
                <Select v-model="form.duration" :options="durationOptions" />
              </div>
              <div class="space-y-2">
                <label class="form-label">{{ t('videoGeneration.audioLabel') }}</label>
                <Select v-model="generateAudioValue" :options="generateAudioOptions" />
              </div>
            </section>

            <section class="space-y-2">
              <label class="form-label">{{ t('videoGeneration.referenceImages') }} <span class="text-xs font-normal text-gray-400">{{ t('videoGeneration.referenceImagesHint') }}</span></label>
              <input ref="imageInputRef" type="file" class="hidden" accept="image/*" multiple @change="onReferenceImagesChange" />
              <button type="button" class="upload-button" @click="imageInputRef?.click()">
                <Icon name="upload" size="sm" /> {{ t('videoGeneration.uploadReferenceImages') }}
              </button>
              <AssetList :assets="form.referenceImages" @remove="removeReferenceImage" />
            </section>

            <section class="space-y-2">
              <label class="form-label">{{ t('videoGeneration.frameReference') }} <span class="text-red-500">*</span></label>
              <Select v-model="form.frameMode" :options="frameModeOptions" />
              <div v-if="form.frameMode !== 'none'" class="grid grid-cols-2 gap-2">
                <button type="button" class="upload-button" @click="firstFrameInputRef?.click()">{{ t('videoGeneration.uploadFirstFrame') }}</button>
                <button v-if="form.frameMode === 'start_end'" type="button" class="upload-button" @click="lastFrameInputRef?.click()">{{ t('videoGeneration.uploadLastFrame') }}</button>
              </div>
              <input ref="firstFrameInputRef" type="file" class="hidden" accept="image/*" @change="onFirstFrameChange" />
              <input ref="lastFrameInputRef" type="file" class="hidden" accept="image/*" @change="onLastFrameChange" />
              <AssetList :assets="frameAssets" @remove="removeFrameAsset" />
            </section>

            <section class="space-y-2">
              <label class="form-label">{{ t('videoGeneration.referenceVideos') }} <span class="text-xs font-normal text-gray-400">{{ t('videoGeneration.referenceVideosHint') }}</span></label>
              <input ref="videoInputRef" type="file" class="hidden" accept="video/*" multiple @change="onReferenceVideosChange" />
              <button type="button" class="upload-button" @click="videoInputRef?.click()">
                <Icon name="upload" size="sm" /> {{ t('videoGeneration.uploadReferenceVideos') }}
              </button>
              <AssetList :assets="form.referenceVideos" @remove="removeReferenceVideo" />
            </section>

            <section class="space-y-2">
              <label class="form-label">{{ t('videoGeneration.referenceAudios') }} <span class="text-xs font-normal text-gray-400">{{ t('videoGeneration.referenceAudiosHint') }}</span></label>
              <input ref="audioInputRef" type="file" class="hidden" accept="audio/*,.mp3,.wav,.m4a,.aac,.flac,.ogg" multiple @change="onReferenceAudiosChange" />
              <button type="button" class="upload-button" @click="audioInputRef?.click()">
                <Icon name="upload" size="sm" /> {{ t('videoGeneration.uploadReferenceAudios') }}
              </button>
              <p v-if="form.referenceAudios.length > 0 && !hasImageReference" class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/30 dark:text-amber-200">{{ t('videoGeneration.audioNeedsImage') }}</p>
              <AssetList :assets="form.referenceAudios" @remove="removeReferenceAudio" />
            </section>
          </div>
        </aside>

        <main class="flex min-h-0 flex-col rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="relative border-b border-gray-200 px-5 py-4 dark:border-dark-700">
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="flex items-center gap-2 text-base font-semibold text-gray-900 dark:text-dark-50">
                  <VideoGenerationLogo class="h-9 w-9" /> {{ t('videoGeneration.dialogTitle') }}
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-300">{{ t('videoGeneration.dialogDescription') }}</p>
              </div>
              <div class="relative flex items-center gap-2">
                <button type="button" class="btn btn-secondary btn-sm" @click="startNewSession">
                  <Icon name="plus" size="sm" /> {{ t('videoGeneration.newConversation') }}
                </button>
                <button type="button" class="btn btn-secondary btn-sm" @click.stop="showHistoryDropdown = !showHistoryDropdown">
                  <Icon name="clock" size="sm" /> {{ t('videoGeneration.history') }}
                </button>
                <div v-if="showHistoryDropdown" class="absolute right-0 top-full z-20 mt-2 w-72 rounded-xl border border-gray-200 bg-white p-2 shadow-lg dark:border-dark-700 dark:bg-dark-800">
                  <div v-if="sessionHistoryRecords.length === 0" class="px-3 py-6 text-center text-sm text-gray-400 dark:text-dark-300">{{ t('videoGeneration.emptyHistory') }}</div>
                  <button
                    v-for="record in sessionHistoryRecords"
                    :key="record.id"
                    type="button"
                    class="w-full rounded-lg px-3 py-2 text-left transition hover:bg-gray-50 dark:hover:bg-dark-700"
                    @click="openHistoryRecord(record)"
                  >
                    <div class="flex items-center justify-between gap-3">
                      <span class="truncate text-sm font-medium text-gray-900 dark:text-dark-50">{{ record.summary }}</span>
                      <span class="shrink-0 text-xs text-gray-400 dark:text-dark-300">{{ t('videoGeneration.generationCount', { count: record.generationCount }) }}</span>
                    </div>
                    <div class="mt-1 text-xs text-gray-500 dark:text-dark-300">{{ t('videoGeneration.lastGenerated', { time: formatHistoryTime(record.updatedAt) }) }}</div>
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div ref="messagesRef" class="min-h-0 flex-1 space-y-4 overflow-y-auto p-5">
            <div v-if="messages.length === 0" class="flex h-full items-center justify-center text-center text-sm text-gray-400 dark:text-dark-300">
              {{ t('videoGeneration.emptyConversation') }}
            </div>

            <div v-for="message in messages" :key="message.id" :class="['flex', message.role === 'user' ? 'justify-end' : 'justify-start']">
              <div :class="['max-w-[78%] rounded-2xl px-4 py-3 text-sm', message.role === 'user' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-800 dark:bg-dark-700 dark:text-dark-50']">
                <div class="whitespace-pre-wrap">{{ message.content }}</div>
                <div v-if="message.status === 'generating'" class="mt-2 flex items-center gap-2 text-xs text-gray-500 dark:text-dark-300">
                  <Icon name="refresh" size="sm" class="animate-spin" /> {{ t('videoGeneration.polling') }}
                </div>
                <div v-if="message.status === 'completed' && message.taskId" class="mt-3 flex flex-wrap items-center gap-3">
                  <button type="button" class="btn btn-primary btn-sm" :disabled="downloadingTaskId === message.taskId" @click="handleDownload(message.taskId)">
                    <Icon name="download" size="sm" :class="downloadingTaskId === message.taskId ? 'animate-bounce' : ''" />
                    {{ t('videoGeneration.download') }}
                  </button>
                  <span class="text-xs text-amber-600 dark:text-amber-300">{{ t('videoGeneration.expiryWarning') }}</span>
                </div>
                <div v-if="message.status === 'failed' && message.error" class="mt-2 text-xs text-red-500">{{ message.error }}</div>
              </div>
            </div>
          </div>

          <div class="border-t border-gray-200 p-4 dark:border-dark-700">
            <div v-if="submitError" class="mb-3 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-600 dark:bg-red-900/30 dark:text-red-200">{{ submitError }}</div>
            <div class="flex items-end gap-3">
              <textarea
                v-model="prompt"
                rows="3"
                class="input min-h-[72px] flex-1 resize-none"
                :placeholder="t('videoGeneration.promptPlaceholder')"
                :disabled="generating"
                @keydown.enter.exact.prevent="submitVideoGeneration"
              ></textarea>
              <button type="button" class="btn btn-primary min-h-[44px]" :disabled="generating" @click="submitVideoGeneration">
                <Icon v-if="generating" name="refresh" size="sm" class="animate-spin" />
                <Icon v-else name="play" size="sm" />
                {{ t('videoGeneration.submit') }}
              </button>
            </div>
          </div>
        </main>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import keysAPI from '@/api/keys'
import { createSeedaceVideoTask, downloadSeedaceVideo, pollSeedaceVideoTask } from '@/api/seedaceVideo'
import { listSeedaceVideoHistory, saveSeedaceVideoHistory, type SeedaceVideoHistoryMessage, type SeedaceVideoHistoryRecord } from '@/api/seedaceVideoHistory'
import type { ApiKey, SelectOption } from '@/types'
import {
  SEEDACE_VIDEO_ASPECT_RATIO_OPTIONS,
  SEEDACE_VIDEO_DURATION_OPTIONS,
  SEEDACE_VIDEO_MODEL_OPTIONS,
  buildSeedaceVideoPayload,
  coerceSeedaceResolution,
  createSeedaceSessionSummary,
  extractSeedaceTaskId,
  isSeedaceVideoCompleted,
  getSeedaceResolutionOptions,
  hasSeedaceImageReference,
  isAllowedSeedaceAudioFile,
  normalizeSeedaceTaskStatus,
  validateSeedaceVideoForm,
  type SeedaceVideoFormState,
  type SeedaceVideoReferenceAsset,
  type SeedaceVideoResolution,
} from '@/utils/seedaceVideo'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatTime } from '@/utils/format'

type ChatMessage = SeedaceVideoHistoryMessage

interface SessionHistoryRecord extends Omit<SeedaceVideoHistoryRecord, 'updatedAt'> {
  updatedAt: number
}

const { t } = useI18n()

const VideoGenerationLogo = defineComponent({
  name: 'VideoGenerationLogo',
  setup(_, { attrs }) {
    return () => h('svg', {
      ...attrs,
      viewBox: '0 0 64 64',
      fill: 'none',
      xmlns: 'http://www.w3.org/2000/svg',
      'aria-hidden': 'true'
    }, [
      h('rect', { x: '8', y: '16', width: '38', height: '30', rx: '10', fill: '#7C3AED' }),
      h('rect', { x: '12', y: '20', width: '30', height: '22', rx: '7', fill: '#A78BFA' }),
      h('circle', { cx: '22', cy: '31', r: '5', fill: '#FFFFFF' }),
      h('circle', { cx: '22', cy: '31', r: '2.2', fill: '#1F2937' }),
      h('circle', { cx: '34', cy: '31', r: '5', fill: '#FFFFFF' }),
      h('circle', { cx: '34', cy: '31', r: '2.2', fill: '#1F2937' }),
      h('path', { d: 'M23 39c3.2 2.4 7.8 2.4 11 0', stroke: '#FFFFFF', 'stroke-width': '3', 'stroke-linecap': 'round' }),
      h('path', { d: 'M46 25l10-6c1.8-1.1 4 .2 4 2.3v19.4c0 2.1-2.2 3.4-4 2.3l-10-6V25z', fill: '#F59E0B' }),
      h('circle', { cx: '18', cy: '13', r: '5', fill: '#22C55E' }),
      h('circle', { cx: '36', cy: '11', r: '4', fill: '#38BDF8' }),
      h('path', { d: 'M18 16v4M36 15v5', stroke: '#4C1D95', 'stroke-width': '3', 'stroke-linecap': 'round' }),
      h('path', { d: 'M11 48h33', stroke: '#F97316', 'stroke-width': '4', 'stroke-linecap': 'round' })
    ])
  }
})

const AssetList = defineComponent({
  name: 'SeedaceAssetList',
  props: {
    assets: { type: Array<SeedaceVideoReferenceAsset>, required: true },
  },
  emits: ['remove'],
  setup(props, { emit }) {
    return () =>
      h('div', { class: 'space-y-1' },
        (props.assets as SeedaceVideoReferenceAsset[]).map((asset, index) =>
          h('div', { class: 'flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-100' }, [
            h('span', { class: 'truncate' }, asset.durationSeconds ? `${asset.name}（${asset.durationSeconds.toFixed(1)}s）` : asset.name),
            h('button', { type: 'button', class: 'ml-2 text-gray-400 hover:text-red-500', onClick: () => emit('remove', index) }, t('videoGeneration.remove')),
          ]),
        ),
      )
  },
})

const apiKeys = ref<ApiKey[]>([])
const loadingKeys = ref(false)
const messages = ref<ChatMessage[]>([])
const prompt = ref('')
const submitError = ref('')
const generating = ref(false)
const downloadingTaskId = ref('')
const messagesRef = ref<HTMLElement | null>(null)
const abortController = ref<AbortController | null>(null)
const showHistoryDropdown = ref(false)
const sessionId = ref<string>(crypto.randomUUID())
const sessionSummary = ref('')
const sessionGenerationCount = ref(0)
const sessionUpdatedAt = ref(0)
const sessionHistoryRecords = ref<SessionHistoryRecord[]>([])

const imageInputRef = ref<HTMLInputElement | null>(null)
const videoInputRef = ref<HTMLInputElement | null>(null)
const audioInputRef = ref<HTMLInputElement | null>(null)
const firstFrameInputRef = ref<HTMLInputElement | null>(null)
const lastFrameInputRef = ref<HTMLInputElement | null>(null)

const form = reactive<SeedaceVideoFormState>({
  apiKeyId: null,
  modelOption: 'domestic-seedance-2.0',
  resolution: '720p',
  aspectRatio: '16:9',
  duration: 10,
  referenceImages: [],
  referenceVideos: [],
  referenceAudios: [],
  frameMode: 'none',
  firstFrame: null,
  lastFrame: null,
  generateAudio: true,
})

const modelOptions = computed<SelectOption[]>(() => SEEDACE_VIDEO_MODEL_OPTIONS.map((option) => ({
  value: option.value,
  label: t(`videoGeneration.models.${option.value === 'domestic-seedance-2.0' ? 'domestic' : option.value === 'international-seedance-2.0' ? 'international' : 'internationalFast'}`),
})))
const aspectRatioOptions = SEEDACE_VIDEO_ASPECT_RATIO_OPTIONS.map((value) => ({ value, label: value }))
const durationOptions = SEEDACE_VIDEO_DURATION_OPTIONS.map((value) => ({ value, label: `${value}s` }))
const frameModeOptions = computed<SelectOption[]>(() => [
  { value: 'none', label: t('videoGeneration.frameModes.none') },
  { value: 'start_frame', label: t('videoGeneration.frameModes.startFrame') },
  { value: 'start_end', label: t('videoGeneration.frameModes.startEnd') },
])
const generateAudioOptions = computed<SelectOption[]>(() => [
  { value: 'yes', label: t('videoGeneration.audioOptions.yes') },
  { value: 'no', label: t('videoGeneration.audioOptions.no') },
])

const apiKeyOptions = computed<SelectOption[]>(() => apiKeys.value.map((key) => ({ value: key.id, label: `${key.name}（${key.group?.name || 'seedace'}）` })))
const selectedApiKey = computed(() => apiKeys.value.find((key) => key.id === form.apiKeyId) ?? null)
const resolutionOptions = computed<SelectOption[]>(() => getSeedaceResolutionOptions(form.modelOption).map((value) => ({ value, label: value.toUpperCase() })))
const hasImageReference = computed(() => hasSeedaceImageReference(form))
const frameAssets = computed<SeedaceVideoReferenceAsset[]>(() => [form.firstFrame, form.lastFrame].filter((asset): asset is SeedaceVideoReferenceAsset => asset !== null))
const generateAudioValue = computed({
  get: () => (form.generateAudio ? 'yes' : 'no'),
  set: (value: string | number | boolean | null) => {
    form.generateAudio = value !== 'no'
  },
})
watch(
  () => form.modelOption,
  () => {
    form.resolution = coerceSeedaceResolution(form.modelOption, form.resolution) as SeedaceVideoResolution
  },
)

watch(
  () => form.frameMode,
  (mode) => {
    if (mode === 'none') {
      form.firstFrame = null
      form.lastFrame = null
    } else if (mode === 'start_frame') {
      form.lastFrame = null
    }
  },
)

async function loadInitialData() {
  await Promise.all([loadAPIKeys(), loadHistoryRecords()])
}

async function loadHistoryRecords() {
  try {
    const records = await listSeedaceVideoHistory()
    sessionHistoryRecords.value = records.map((record) => ({ ...record, updatedAt: Date.parse(record.updatedAt) || Date.now() }))
  } catch (err: unknown) {
    submitError.value = extractApiErrorMessage(err, t('videoGeneration.errors.historyLoadFailed'))
  }
}

async function loadAPIKeys() {
  loadingKeys.value = true
  try {
    const result = await keysAPI.list(1, 100, { status: 'active' })
    apiKeys.value = result.items.filter((key) => key.status === 'active' && key.group?.platform === 'seedace')
    if (!form.apiKeyId && apiKeys.value.length > 0) form.apiKeyId = apiKeys.value[0].id
  } catch (err: unknown) {
    submitError.value = extractApiErrorMessage(err, t('videoGeneration.errors.apiKeysLoadFailed'))
  } finally {
    loadingKeys.value = false
  }
}

async function submitVideoGeneration() {
  submitError.value = ''
  const errors = validateSeedaceVideoForm(form, prompt.value)
  if (errors.length > 0) {
    submitError.value = t(`videoGeneration.errors.${errors[0]}`)
    return
  }
  if (!selectedApiKey.value) {
    submitError.value = t('videoGeneration.errors.apiKeyRequired')
    return
  }

  const userContent = prompt.value.trim()
  updateSessionHistory(userContent)
  const assistantMessage: ChatMessage = {
    id: crypto.randomUUID(),
    role: 'assistant',
    content: t('videoGeneration.generating'),
    status: 'generating',
  }
  messages.value.push({ id: crypto.randomUUID(), role: 'user', content: userContent }, assistantMessage)
  upsertCurrentSessionHistoryRecord()
  prompt.value = ''
  generating.value = true
  abortController.value?.abort()
  abortController.value = new AbortController()
  await scrollMessagesToBottom()

  try {
    const payload = buildSeedaceVideoPayload(form, userContent)
    const createResult = await createSeedaceVideoTask(selectedApiKey.value.key, payload, abortController.value.signal)
    const taskId = extractSeedaceTaskId(createResult)
    if (!taskId) throw new Error('upstreamTaskMissing')
    assistantMessage.taskId = taskId
    await pollUntilCompleted(selectedApiKey.value.key, taskId, assistantMessage)
  } catch (err: unknown) {
    assistantMessage.status = 'failed'
    assistantMessage.content = t('videoGeneration.generationFailed')
    const errorKey = err instanceof Error ? err.message : ''
    assistantMessage.error = errorKey && t(`videoGeneration.errors.${errorKey}`) !== `videoGeneration.errors.${errorKey}`
      ? t(`videoGeneration.errors.${errorKey}`)
      : extractApiErrorMessage(err, t('videoGeneration.generationFailed'))
  } finally {
    generating.value = false
    upsertCurrentSessionHistoryRecord()
    await scrollMessagesToBottom()
  }
}

async function pollUntilCompleted(apiKey: string, taskId: string, message: ChatMessage) {
  for (let attempt = 0; attempt < 120; attempt += 1) {
    if (attempt > 0) await sleep(8000)
    const result = await pollSeedaceVideoTask(apiKey, taskId, abortController.value?.signal)
    const status = normalizeSeedaceTaskStatus(result)
    if (status === 'failed') {
      message.status = 'failed'
      message.content = t('videoGeneration.generationFailed')
      message.error = t('videoGeneration.errors.upstreamFailed')
      return
    }
    if (status === 'success' && !isSeedaceVideoCompleted(result)) {
      continue
    }
    if (isSeedaceVideoCompleted(result)) {
      message.status = 'completed'
      message.content = t('videoGeneration.generated')
      return
    }
  }
  message.status = 'failed'
  message.content = t('videoGeneration.generationFailed')
  message.error = t('videoGeneration.errors.pollingTimeout')
}

function updateSessionHistory(userContent: string) {
  sessionSummary.value = createSeedaceSessionSummary(userContent)
  sessionGenerationCount.value += 1
  sessionUpdatedAt.value = Date.now()
}

function upsertCurrentSessionHistoryRecord() {
  if (sessionGenerationCount.value === 0 || messages.value.length === 0) return
  const record: SessionHistoryRecord = {
    id: sessionId.value,
    summary: sessionSummary.value || t('videoGeneration.unnamedSession'),
    generationCount: sessionGenerationCount.value,
    updatedAt: sessionUpdatedAt.value,
    messages: cloneMessages(messages.value),
  }
  const index = sessionHistoryRecords.value.findIndex((item) => item.id === record.id)
  if (index >= 0) sessionHistoryRecords.value[index] = record
  else sessionHistoryRecords.value.unshift(record)
  void persistCurrentSessionHistoryRecord(record)
}

async function persistCurrentSessionHistoryRecord(record: SessionHistoryRecord) {
  try {
    const saved = await saveSeedaceVideoHistory({ ...record, updatedAt: new Date(record.updatedAt || Date.now()).toISOString() })
    const savedRecord: SessionHistoryRecord = { ...saved, updatedAt: Date.parse(saved.updatedAt) || record.updatedAt || Date.now() }
    const index = sessionHistoryRecords.value.findIndex((item) => item.id === savedRecord.id)
    if (index >= 0) sessionHistoryRecords.value[index] = savedRecord
  } catch (err: unknown) {
    submitError.value = extractApiErrorMessage(err, t('videoGeneration.errors.historySaveFailed'))
  }
}

function startNewSession() {
  upsertCurrentSessionHistoryRecord()
  abortController.value?.abort()
  abortController.value = null
  messages.value = []
  prompt.value = ''
  submitError.value = ''
  generating.value = false
  downloadingTaskId.value = ''
  showHistoryDropdown.value = false
  sessionId.value = crypto.randomUUID()
  sessionSummary.value = ''
  sessionGenerationCount.value = 0
  sessionUpdatedAt.value = 0
  void scrollMessagesToBottom()
}

function openHistoryRecord(record: SessionHistoryRecord) {
  upsertCurrentSessionHistoryRecord()
  sessionId.value = record.id
  sessionSummary.value = record.summary
  sessionGenerationCount.value = record.generationCount
  sessionUpdatedAt.value = record.updatedAt
  messages.value = cloneMessages(record.messages)
  showHistoryDropdown.value = false
  void scrollMessagesToBottom()
}

function cloneMessages(source: ChatMessage[]): ChatMessage[] {
  return source.map((message) => ({ ...message }))
}

function formatHistoryTime(timestamp: number) {
  if (!timestamp) return '-'
  return formatTime(new Date(timestamp))
}

function handleDownload(taskId: string) {
  if (!form.apiKeyId) {
    submitError.value = t('videoGeneration.errors.apiKeyRequired')
    return
  }
  downloadingTaskId.value = taskId
  submitError.value = ''
  try {
    downloadSeedaceVideo(taskId, form.apiKeyId)
  } catch (err: unknown) {
    submitError.value = extractApiErrorMessage(err, t('videoGeneration.errors.downloadFailed'))
  } finally {
    setTimeout(() => {
      if (downloadingTaskId.value === taskId) downloadingTaskId.value = ''
    }, 300)
  }
}

async function onReferenceImagesChange(event: Event) {
  const input = event.target as HTMLInputElement
  await appendImageFiles(input.files, form.referenceImages, 9, t('videoGeneration.errors.tooManyImages'))
  input.value = ''
}

async function onReferenceVideosChange(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  if (form.referenceVideos.length + files.length > 3) {
    submitError.value = t('videoGeneration.errors.tooManyVideos')
    input.value = ''
    return
  }
  for (const file of files) {
    const duration = await readVideoDuration(file)
    if (duration >= 15) {
      submitError.value = t('videoGeneration.errors.videoTooLong')
      continue
    }
    form.referenceVideos.push({ name: file.name, mimeType: file.type, dataUrl: await readFileAsDataURL(file), durationSeconds: duration })
  }
  input.value = ''
}

async function onReferenceAudiosChange(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  if (form.referenceAudios.length + files.length > 3) {
    submitError.value = t('videoGeneration.errors.tooManyAudios')
    input.value = ''
    return
  }
  for (const file of files) {
    if (!isAllowedSeedaceAudioFile(file)) {
      submitError.value = t('videoGeneration.errors.audioTypeInvalid')
      continue
    }
    form.referenceAudios.push({ name: file.name, mimeType: file.type, dataUrl: await readFileAsDataURL(file) })
  }
  if (!hasImageReference.value) submitError.value = t('videoGeneration.errors.audioImageRequired')
  input.value = ''
}

async function onFirstFrameChange(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (file) form.firstFrame = { name: file.name, mimeType: file.type, dataUrl: await readFileAsDataURL(file) }
  ;(event.target as HTMLInputElement).value = ''
}

async function onLastFrameChange(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (file) form.lastFrame = { name: file.name, mimeType: file.type, dataUrl: await readFileAsDataURL(file) }
  ;(event.target as HTMLInputElement).value = ''
}

async function appendImageFiles(files: FileList | null, target: SeedaceVideoReferenceAsset[], limit: number, limitMessage: string) {
  const list = Array.from(files || [])
  if (target.length + list.length > limit) {
    submitError.value = limitMessage
    return
  }
  for (const file of list) {
    if (!file.type.startsWith('image/')) {
      submitError.value = t('videoGeneration.errors.imageTypeInvalid')
      continue
    }
    target.push({ name: file.name, mimeType: file.type, dataUrl: await readFileAsDataURL(file) })
  }
}

function removeReferenceImage(index: number) {
  form.referenceImages.splice(index, 1)
}
function removeReferenceVideo(index: number) {
  form.referenceVideos.splice(index, 1)
}
function removeReferenceAudio(index: number) {
  form.referenceAudios.splice(index, 1)
}
function removeFrameAsset(index: number) {
  if (index === 0) form.firstFrame = null
  else form.lastFrame = null
}

function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(new Error(t('videoGeneration.errors.fileReadFailed')))
    reader.readAsDataURL(file)
  })
}

function readVideoDuration(file: File): Promise<number> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file)
    const video = document.createElement('video')
    video.preload = 'metadata'
    video.onloadedmetadata = () => {
      const duration = video.duration || 0
      URL.revokeObjectURL(url)
      resolve(duration)
    }
    video.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error(t('videoGeneration.errors.videoReadFailed')))
    }
    video.src = url
  })
}

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

async function scrollMessagesToBottom() {
  await nextTick()
  if (messagesRef.value) messagesRef.value.scrollTop = messagesRef.value.scrollHeight
}

onMounted(loadInitialData)
onBeforeUnmount(() => abortController.value?.abort())
</script>

<style scoped>
.form-label {
  @apply text-sm font-medium text-gray-700 dark:text-dark-100;
}
.upload-button {
  @apply inline-flex w-full items-center justify-center gap-2 rounded-lg border border-dashed border-gray-300 px-3 py-2 text-sm text-gray-600 transition hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-dark-200 dark:hover:border-primary-500;
}
</style>
