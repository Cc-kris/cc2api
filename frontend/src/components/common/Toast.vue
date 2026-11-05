<template>
  <Teleport to="body">
    <div
      class="pointer-events-none fixed right-6 top-6 z-[9999] space-y-3.5"
      aria-live="polite"
      aria-atomic="true"
    >
      <TransitionGroup
        enter-active-class="transition duration-250 ease-out"
        enter-from-class="opacity-0 translate-x-10 scale-95"
        enter-to-class="opacity-100 translate-x-0 scale-100"
        leave-active-class="transition duration-150 ease-in"
        leave-from-class="opacity-100 translate-x-0 scale-100"
        leave-to-class="opacity-0 translate-x-10 scale-95"
      >
        <div
          v-for="toast in toasts"
          :key="toast.id"
          :class="[
            'ccai-neolab-toast pointer-events-auto min-w-[340px] max-w-md overflow-hidden rounded-2xl',
            getToastSurfaceClass(toast.type)
          ]"
        >
          <div class="p-4">
            <div class="flex items-start gap-3.5">
              <!-- Neo-Lab Stamp Badge -->
              <div
                :class="[
                  'mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border-2 border-[#121316] shadow-[2px_2px_0_#121316]',
                  getBadgeBgClass(toast.type)
                ]"
              >
                <Icon
                  :name="getToastIconName(toast.type)"
                  size="md"
                  class="text-[#121316]"
                  aria-hidden="true"
                />
              </div>

              <!-- Content -->
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <span
                    :class="[
                      'inline-block rounded-md border border-[#121316] px-1.5 py-0.5 font-mono text-[10px] font-black uppercase tracking-wider',
                      getTagClass(toast.type)
                    ]"
                  >
                    {{ getTypeLabel(toast.type) }}
                  </span>
                  <p v-if="toast.title" class="text-sm font-black text-[#121316]">
                    {{ toast.title }}
                  </p>
                </div>
                <p class="mt-1.5 text-sm font-bold leading-snug text-[#121316]">
                  {{ toast.message }}
                </p>
              </div>

              <!-- Close button -->
              <button
                @click="removeToast(toast.id)"
                class="-m-1 flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-lg border-2 border-[#121316] bg-white text-[#121316] shadow-[2px_2px_0_#121316] transition-transform hover:-translate-y-0.5"
                aria-label="Close notification"
              >
                <Icon name="x" size="sm" />
              </button>
            </div>
          </div>

          <!-- Progress bar -->
          <div v-if="toast.duration" class="h-1.5 border-t-2 border-[#121316] bg-white/70">
            <div
              :class="['h-full toast-progress', getProgressBarColor(toast.type)]"
              :style="{ animationDuration: `${toast.duration}ms` }"
            ></div>
          </div>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const toasts = computed(() => appStore.toasts)

const getToastIconName = (type: string): 'checkCircle' | 'xCircle' | 'exclamationTriangle' | 'infoCircle' => {
  switch (type) {
    case 'success':
      return 'checkCircle'
    case 'error':
      return 'xCircle'
    case 'warning':
      return 'exclamationTriangle'
    case 'info':
    default:
      return 'infoCircle'
  }
}

const getTypeLabel = (type: string): string => {
  switch (type) {
    case 'success':
      return '200 OK'
    case 'error':
      return 'ALERT'
    case 'warning':
      return 'WARN'
    default:
      return 'INFO'
  }
}

const getToastSurfaceClass = (type: string): string => {
  switch (type) {
    case 'error':
      return 'bg-[#FFF0EE]'
    case 'success':
      return 'bg-[#F0FDF4]'
    case 'warning':
      return 'bg-[#FEFCE8]'
    default:
      return 'bg-[#EFF6FF]'
  }
}

const getBadgeBgClass = (type: string): string => {
  switch (type) {
    case 'error':
      return 'bg-[#FF3B30] text-white'
    case 'success':
      return 'bg-[#16A34A] text-white'
    case 'warning':
      return 'bg-[#FF9500]'
    default:
      return 'bg-[#60A5FA]'
  }
}

const getTagClass = (type: string): string => {
  switch (type) {
    case 'error':
      return 'bg-[#FF3B30] text-white'
    case 'success':
      return 'bg-[#16A34A] text-white'
    case 'warning':
      return 'bg-[#FF9500] text-white'
    default:
      return 'bg-[#DBEAFE] text-[#1D4ED8]'
  }
}

const getProgressBarColor = (type: string): string => {
  switch (type) {
    case 'success':
      return 'bg-[#16A34A]'
    case 'error':
      return 'bg-[#FF3B30]'
    case 'warning':
      return 'bg-[#FF9500]'
    default:
      return 'bg-[#2563EB]'
  }
}

const removeToast = (id: string) => {
  appStore.hideToast(id)
}
</script>

<style scoped>
.ccai-neolab-toast {
  border: 2.5px solid #121316;
  box-shadow: 6px 6px 0px #121316;
}

.toast-progress {
  width: 100%;
  animation-name: toast-progress-shrink;
  animation-timing-function: linear;
  animation-fill-mode: forwards;
}

@keyframes toast-progress-shrink {
  from {
    width: 100%;
  }
  to {
    width: 0%;
  }
}
</style>
