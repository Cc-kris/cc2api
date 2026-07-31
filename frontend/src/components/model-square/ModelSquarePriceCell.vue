<template>
  <div v-if="price" class="min-w-[150px] leading-tight" :title="fullValue">
    <div class="font-semibold text-gray-900 dark:text-white">
      {{ primaryValue }}
      <span class="font-normal text-gray-500 dark:text-dark-300">/ {{ unitLabel }}</span>
    </div>
    <div class="mt-1 text-xs text-gray-500 dark:text-dark-300">
      {{ t('modelSquare.originalPrice') }} {{ originalValue }} / {{ unitLabel }}
    </div>
  </div>
  <span v-else class="text-gray-400" aria-hidden="true">—</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ModelSquareUnitPrice } from '@/api/modelSquare'

const props = defineProps<{ price: ModelSquareUnitPrice | null | undefined }>()
const { t } = useI18n()

function formatAmount(value: string) {
  const normalized = value.trim()
  const match = normalized.match(/^([+-]?)(\d+)(?:\.(\d+))?$/)
  if (!match) return `$${normalized}`
  const [, sign, rawInteger, rawFraction = ''] = match
  const integer = rawInteger.replace(/^0+(?=\d)/, '')
  const fraction = rawFraction.slice(0, 8).replace(/0+$/, '')
  if (/^0+$/.test(integer) && fraction === '') return t('modelSquare.free')
  return `$${sign}${integer}${fraction ? `.${fraction}` : ''}`
}

const primaryValue = computed(() => props.price ? formatAmount(props.price.multiplier_price) : '')
const originalValue = computed(() => props.price ? formatAmount(props.price.original) : '')
const unitLabel = computed(() => props.price ? t(`modelSquare.units.${props.price.unit}`) : '')
const fullValue = computed(() => props.price
  ? `${t('modelSquare.multiplierPrice')} ${props.price.multiplier_price}; ${t('modelSquare.originalPrice')} ${props.price.original}; ${unitLabel.value}`
  : '')
</script>
