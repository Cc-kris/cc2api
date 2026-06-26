<template>
  <AppLayout>
    <main class="min-h-screen bg-gray-50 px-4 py-6 dark:bg-dark-950 sm:px-6 lg:px-8">
      <div class="mx-auto max-w-5xl space-y-6">
        <section class="rounded-lg border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
            <div>
              <p class="text-sm font-medium text-primary-600 dark:text-primary-400">CCAI Video API</p>
              <h1 class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">Seedance 视频调用说明</h1>
              <p class="mt-3 max-w-3xl text-sm leading-6 text-gray-600 dark:text-dark-300">
                Seedance 视频接口用于文生视频、图生视频、首尾帧、参考视频和参考音频生成。接口为异步流程：先提交任务，拿到任务 ID 后轮询查询结果。
              </p>
            </div>
            <div class="rounded-lg border border-primary-100 bg-primary-50 px-4 py-3 text-sm text-primary-800 dark:border-primary-900/50 dark:bg-primary-900/20 dark:text-primary-200">
              <div class="font-semibold">Base URL</div>
              <code class="mt-1 block break-all font-mono text-xs">https://cc-ai.xyz/v1</code>
            </div>
          </div>
        </section>

        <section class="guide-section">
          <h2>接入前准备</h2>
          <div class="grid gap-4 md:grid-cols-3">
            <div class="guide-card">
              <h3>1. 创建专用 API 密钥</h3>
              <p>在 API 密钥页面创建可调用 Seedance 视频模型的密钥。请求时使用 <code>Authorization: Bearer sk-...</code>。</p>
            </div>
            <div class="guide-card">
              <h3>2. 使用视频接口</h3>
              <p>视频生成固定走 <code>/v1/video/generations</code>，不要使用聊天接口。</p>
            </div>
            <div class="guide-card">
              <h3>3. 轮询获取结果</h3>
              <p>创建任务只返回任务状态，最终视频链接需要用任务 ID 继续查询。</p>
            </div>
          </div>
        </section>

        <section class="guide-section">
          <h2>模型与能力</h2>
          <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
            <table class="w-full border-collapse text-left text-sm">
              <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-dark-400">
                <tr>
                  <th class="px-4 py-3">模型</th>
                  <th class="px-4 py-3">分辨率</th>
                  <th class="px-4 py-3">适用方式</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
                <tr v-for="model in models" :key="model.name">
                  <td class="px-4 py-3 font-mono text-xs text-gray-900 dark:text-white">{{ model.name }}</td>
                  <td class="px-4 py-3 text-gray-700 dark:text-dark-200">{{ model.resolution }}</td>
                  <td class="px-4 py-3 text-gray-700 dark:text-dark-200">{{ model.usage }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p class="mt-3 text-sm leading-6 text-gray-600 dark:text-dark-300">
            <code>-ref</code> 模型用于带参考素材的生成方式，例如图生视频、首尾帧、参考视频和参考音频。具体可用模型以 CCAI 后台展示为准。
          </p>
        </section>

        <section class="guide-section">
          <h2>提交任务</h2>
          <p class="guide-copy">所有生成方式都使用同一个创建接口。</p>
          <CodeBlock :code="createEndpoint" />
          <div class="grid gap-4 md:grid-cols-2">
            <div class="guide-card">
              <h3>常用字段</h3>
              <ul class="guide-list">
                <li><code>model</code>：模型名称。</li>
                <li><code>prompt</code>：生成要求。</li>
                <li><code>duration</code>：视频时长，单位秒。</li>
                <li><code>aspect_ratio</code>：画面比例，例如 <code>16:9</code>。</li>
                <li><code>generate_audio</code>：是否生成声音。</li>
              </ul>
            </div>
            <div class="guide-card">
              <h3>结果字段</h3>
              <ul class="guide-list">
                <li><code>id</code> 或 <code>task_id</code>：用于后续查询。</li>
                <li><code>status</code>：任务状态。</li>
                <li><code>video_url</code>、<code>url</code> 或 <code>result_url</code>：生成完成后的视频链接。</li>
              </ul>
            </div>
          </div>
        </section>

        <section class="guide-section">
          <h2>调用示例</h2>
          <div class="space-y-5">
            <ExampleBlock
              title="文生视频"
              :code="textToVideoExample"
            />
            <ExampleBlock
              title="图生视频"
              :code="imageToVideoExample"
            />
            <ExampleBlock
              title="首尾帧过渡"
              :code="frameTransitionExample"
            />
            <ExampleBlock
              title="参考视频生成"
              :code="referenceVideoExample"
            />
            <ExampleBlock
              title="参考音频生成"
              :code="referenceAudioExample"
            />
          </div>
        </section>

        <section class="guide-section">
          <h2>轮询任务</h2>
          <p class="guide-copy">任务创建成功后，使用任务 ID 查询生成进度。高峰期视频任务会排队，轮询间隔保持在 5 到 10 秒。</p>
          <CodeBlock :code="pollExample" />
        </section>

        <section class="guide-section">
          <h2>注意事项</h2>
          <div class="grid gap-4 md:grid-cols-2">
            <div class="guide-card">
              <h3>素材要求</h3>
              <p>外部图片、视频和音频链接必须能被公网访问。参考视频建议总时长不超过 15 秒，参考素材数量按模型限制控制。</p>
            </div>
            <div class="guide-card">
              <h3>视频链接保存</h3>
              <p>生成结果返回的是视频访问链接。业务上需要长期保存时，应在拿到链接后下载并转存到自己的存储。</p>
            </div>
            <div class="guide-card">
              <h3>计费方式</h3>
              <p>视频按生成秒数计费。实际扣费以 CCAI 后台用量记录和账户余额为准。</p>
            </div>
            <div class="guide-card">
              <h3>常见错误</h3>
              <p>401 表示密钥错误；403 多为余额或权限不足；模型不存在通常是密钥分组不支持该模型或模型名写错。</p>
            </div>
          </div>
        </section>
      </div>
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { defineComponent, h } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'

const CodeBlock = defineComponent({
  name: 'CodeBlock',
  props: {
    code: {
      type: String,
      required: true,
    },
  },
  setup(props) {
    return () => h('pre', { class: 'guide-code' }, h('code', props.code))
  },
})

const ExampleBlock = defineComponent({
  name: 'ExampleBlock',
  props: {
    title: {
      type: String,
      required: true,
    },
    code: {
      type: String,
      required: true,
    },
  },
  setup(props) {
    return () => h('div', { class: 'guide-example' }, [
      h('h3', props.title),
      h(CodeBlock, { code: props.code }),
    ])
  },
})

const createEndpoint = `curl https://cc-ai.xyz/v1/video/generations \\
  -H "Authorization: Bearer sk-你的CCAI密钥" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "dreamina-seedance-2-0-720p",
    "prompt": "一只猫在阳光下穿过花园，镜头缓慢推进",
    "duration": 5
  }'`

const textToVideoExample = `curl https://cc-ai.xyz/v1/video/generations \\
  -H "Authorization: Bearer sk-你的CCAI密钥" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "dreamina-seedance-2-0-720p",
    "prompt": "清晨的城市街道，行人和车辆自然移动，电影感运镜",
    "duration": 5,
    "aspect_ratio": "16:9",
    "generate_audio": true
  }'`

const imageToVideoExample = `curl https://cc-ai.xyz/v1/video/generations \\
  -H "Authorization: Bearer sk-你的CCAI密钥" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "镜头缓缓推进，画面中的人物转身看向远处",
    "duration": 5,
    "image": "https://your-domain.example/photo.jpg"
  }'`

const frameTransitionExample = `curl https://cc-ai.xyz/v1/video/generations \\
  -H "Authorization: Bearer sk-你的CCAI密钥" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "从第一张画面自然过渡到第二张画面，保持主体一致",
    "duration": 5,
    "first_frame": "https://your-domain.example/first.jpg",
    "last_frame": "https://your-domain.example/last.jpg"
  }'`

const referenceVideoExample = `curl https://cc-ai.xyz/v1/video/generations \\
  -H "Authorization: Bearer sk-你的CCAI密钥" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "参考 @Video1 的镜头节奏，生成一段户外运动视频",
    "duration": 5,
    "reference_videos": [
      "https://your-domain.example/camera-reference.mp4"
    ]
  }'`

const referenceAudioExample = `curl https://cc-ai.xyz/v1/video/generations \\
  -H "Authorization: Bearer sk-你的CCAI密钥" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "@Image1 中的人物跟随 @Audio1 的节奏跳舞",
    "duration": 5,
    "image": "https://your-domain.example/person.jpg",
    "audio_url": "https://your-domain.example/music.mp3"
  }'`

const pollExample = `curl https://cc-ai.xyz/v1/video/generations/task_xxx \\
  -H "Authorization: Bearer sk-你的CCAI密钥"`

const models = [
  { name: 'dreamina-seedance-2-0-480p', resolution: '480P', usage: '文生视频' },
  { name: 'dreamina-seedance-2-0-480p-ref', resolution: '480P', usage: '图生、首尾帧、参考视频、参考音频' },
  { name: 'dreamina-seedance-2-0-720p', resolution: '720P', usage: '文生视频' },
  { name: 'dreamina-seedance-2-0-720p-ref', resolution: '720P', usage: '图生、首尾帧、参考视频、参考音频' },
  { name: 'dreamina-seedance-2-0-1080p', resolution: '1080P', usage: '文生视频' },
  { name: 'dreamina-seedance-2-0-1080p-ref', resolution: '1080P', usage: '图生、首尾帧、参考视频、参考音频' },
  { name: 'dreamina-seedance-2-0-fast-480p', resolution: '480P', usage: '快速文生视频' },
  { name: 'dreamina-seedance-2-0-fast-480p-ref', resolution: '480P', usage: '快速图生和参考素材生成' },
]
</script>

<style scoped>
.guide-section {
  @apply rounded-lg border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900;
}

.guide-section h2 {
  @apply mb-4 text-lg font-semibold text-gray-900 dark:text-white;
}

.guide-card {
  @apply rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800;
}

.guide-card h3,
.guide-example h3 {
  @apply mb-2 text-sm font-semibold text-gray-900 dark:text-white;
}

.guide-card p,
.guide-copy,
.guide-list {
  @apply text-sm leading-6 text-gray-600 dark:text-dark-300;
}

.guide-list {
  @apply list-disc space-y-1 pl-5;
}

.guide-card code,
.guide-list code,
.guide-section p code {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs text-gray-900 dark:bg-dark-700 dark:text-dark-100;
}

.guide-example {
  @apply space-y-2;
}

.guide-code {
  @apply overflow-x-auto rounded-lg border border-gray-200 bg-gray-950 p-4 text-xs leading-6 text-gray-100 dark:border-dark-700;
}

.guide-code code {
  @apply font-mono;
}
</style>
