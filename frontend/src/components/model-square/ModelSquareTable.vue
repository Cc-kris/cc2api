<template>
  <div class="h-full min-h-[360px]">
    <div
      ref="desktopScrollRef"
      data-testid="model-square-desktop-list"
      class="hidden h-full overflow-auto md:block"
      @scroll.passive="handleScroll"
    >
      <div role="table" :aria-label="t('modelSquare.modelTable')" class="min-w-max text-sm">
        <div
          role="row"
          class="sticky top-0 z-20 grid border-b border-gray-200 bg-gray-50 text-xs font-semibold text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300"
          :style="gridStyle"
        >
          <div
            v-for="column in columns"
            :key="column.key"
            role="columnheader"
            class="px-4 py-3"
            :class="column.key === 'name' ? 'sticky left-0 z-30 bg-gray-50 dark:bg-dark-800' : ''"
          >
            {{ column.label }}
          </div>
        </div>

        <div v-if="items.length" class="relative" :style="{ height: `${virtualizer.getTotalSize()}px` }">
          <div
            v-for="virtualRow in virtualizer.getVirtualItems()"
            :key="items[virtualRow.index].name"
            :ref="measureRow"
            :data-index="virtualRow.index"
            role="row"
            class="absolute left-0 top-0 grid w-full border-b border-gray-100 bg-white text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200"
            :style="{ ...gridStyle, transform: `translateY(${virtualRow.start}px)` }"
          >
            <template v-for="column in columns" :key="column.key">
              <div
                v-if="column.key === 'name'"
                role="cell"
                class="sticky left-0 z-10 flex min-w-0 items-start gap-2 bg-white px-4 py-4 dark:bg-dark-800"
              >
                <span class="min-w-0 break-all font-medium text-gray-900 dark:text-white">{{ items[virtualRow.index].name }}</span>
                <button type="button" class="shrink-0 text-gray-400 hover:text-primary-600" :aria-label="t('modelSquare.copyModel')" @click="copyModel(items[virtualRow.index].name)">
                  <Icon name="copy" size="xs" />
                </button>
              </div>
              <div v-else-if="column.key === 'billing_mode'" role="cell" class="px-4 py-4">
                <div>{{ t(`modelSquare.billingModes.${items[virtualRow.index].billing_mode}`) }}</div>
                <div class="mt-1 text-xs text-gray-500">{{ t(`modelSquare.pricingSources.${pricingSource(items[virtualRow.index])}`) }}</div>
              </div>
              <div v-else-if="column.key === 'cache_write'" role="cell" class="space-y-3 px-4 py-4">
                <div v-if="items[virtualRow.index].prices.cache_write_5m">
                  <div class="mb-1 text-xs font-medium text-gray-500">{{ t('modelSquare.columns.cacheWrite5m') }}</div>
                  <ModelSquarePriceCell :price="items[virtualRow.index].prices.cache_write_5m" />
                </div>
                <div v-if="items[virtualRow.index].prices.cache_write_1h">
                  <div class="mb-1 text-xs font-medium text-gray-500">{{ t('modelSquare.columns.cacheWrite1h') }}</div>
                  <ModelSquarePriceCell :price="items[virtualRow.index].prices.cache_write_1h" />
                </div>
                <span v-if="!items[virtualRow.index].prices.cache_write_5m && !items[virtualRow.index].prices.cache_write_1h" class="text-gray-400" aria-hidden="true">-</span>
              </div>
              <div v-else-if="column.key === 'fast'" role="cell" class="px-4 py-4">
                <details v-if="items[virtualRow.index].fast_prices" class="group">
                  <summary class="cursor-pointer font-medium text-primary-700 marker:hidden dark:text-primary-300">
                    {{ t('modelSquare.viewFast') }}
                    <Icon name="chevronDown" size="xs" class="ml-1 inline transition-transform group-open:rotate-180" />
                  </summary>
                  <div class="mt-3 space-y-3 rounded-lg bg-gray-50 p-3 dark:bg-dark-700">
                    <div v-for="entry in fastEntries(items[virtualRow.index])" :key="entry.key">
                      <div class="mb-1 text-xs font-medium text-gray-500">{{ entry.label }}</div>
                      <ModelSquarePriceCell :price="entry.price" />
                    </div>
                  </div>
                </details>
                <span v-else class="text-gray-400" aria-hidden="true">-</span>
              </div>
              <div v-else-if="column.key === 'tiers'" role="cell" class="px-4 py-4">
                <details v-if="items[virtualRow.index].tiers.length" class="group">
                  <summary class="cursor-pointer font-medium text-primary-700 marker:hidden dark:text-primary-300">
                    {{ t('modelSquare.viewTiers', { count: items[virtualRow.index].tiers.length }) }}
                    <Icon name="chevronDown" size="xs" class="ml-1 inline transition-transform group-open:rotate-180" />
                  </summary>
                  <div class="mt-3 space-y-3 rounded-lg bg-gray-50 p-3 dark:bg-dark-700">
                    <TierPrices v-for="tier in items[virtualRow.index].tiers" :key="`${tier.sort_order}-${tier.tier_label || tier.min_tokens}`" :tier="tier" />
                  </div>
                </details>
                <span v-else class="text-gray-400" aria-hidden="true">-</span>
              </div>
              <div v-else role="cell" class="px-4 py-4">
                <ModelSquarePriceCell :price="priceFor(items[virtualRow.index], column.key)" />
              </div>
            </template>
          </div>
        </div>

        <LoadingRows :loading="loading" :loading-more="loadingMore" />
        <div v-if="!loading && !loadingMore && items.length && !hasMore" class="p-4 text-center text-xs text-gray-500">
          {{ t('modelSquare.allLoaded') }}
        </div>
      </div>
    </div>

    <div
      ref="mobileScrollRef"
      data-testid="model-square-mobile-list"
      class="h-full overflow-x-hidden overflow-y-auto px-3 py-2 md:hidden"
      @scroll.passive="handleScroll"
    >
      <details
        v-for="item in items"
        :key="item.name"
        class="group border-b border-gray-100 bg-white dark:border-dark-700 dark:bg-dark-800"
      >
        <summary class="flex min-w-0 cursor-pointer list-none items-center justify-between gap-3 py-4 marker:hidden">
          <span class="min-w-0">
            <span class="block break-all text-sm font-semibold text-gray-900 dark:text-white">{{ item.name }}</span>
            <span class="mt-1 block text-xs text-gray-500 dark:text-dark-300">
              {{ t(`modelSquare.billingModes.${item.billing_mode}`) }} · {{ t(`modelSquare.pricingSources.${pricingSource(item)}`) }}
            </span>
          </span>
          <span class="flex shrink-0 items-center gap-2">
            <span v-if="primaryPrice(item)" class="text-right text-sm font-semibold text-gray-900 dark:text-white">
              {{ compactPrice(primaryPrice(item)) }}
            </span>
            <Icon name="chevronDown" size="sm" class="text-gray-400 transition-transform group-open:rotate-180" />
          </span>
        </summary>

        <div class="space-y-5 pb-5">
          <div class="grid min-w-0 gap-4">
            <div v-for="entry in regularEntries(item)" :key="entry.key" class="min-w-0">
              <div class="mb-1 text-xs font-medium text-gray-500">{{ entry.label }}</div>
              <ModelSquarePriceCell :price="entry.price" />
            </div>
          </div>

          <div v-if="item.fast_prices" class="min-w-0 border-t border-gray-100 pt-4 dark:border-dark-700">
            <div class="mb-3 text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('modelSquare.columns.fast') }}</div>
            <div class="grid min-w-0 gap-4">
              <div v-for="entry in fastEntries(item)" :key="entry.key" class="min-w-0">
                <div class="mb-1 text-xs font-medium text-gray-500">{{ entry.label }}</div>
                <ModelSquarePriceCell :price="entry.price" />
              </div>
            </div>
          </div>

          <div v-if="item.tiers.length" class="min-w-0 space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700">
            <div class="text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('modelSquare.columns.tiers') }}</div>
            <TierPrices v-for="tier in item.tiers" :key="`${tier.sort_order}-${tier.tier_label || tier.min_tokens}`" :tier="tier" />
          </div>

          <button type="button" class="btn btn-ghost px-0 text-xs" @click="copyModel(item.name)">
            <Icon name="copy" size="xs" />
            <span>{{ t('modelSquare.copyModel') }}</span>
          </button>
        </div>
      </details>

      <LoadingRows :loading="loading" :loading-more="loadingMore" />
      <div v-if="!loading && !loadingMore && items.length && !hasMore" class="p-4 text-center text-xs text-gray-500">
        {{ t('modelSquare.allLoaded') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref, type ComponentPublicInstance, type PropType, type VNodeRef } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { useI18n } from 'vue-i18n'
import type { ModelSquareModel, ModelSquareTierPrice, ModelSquareUnitPrice } from '@/api/modelSquare'
import Icon from '@/components/icons/Icon.vue'
import ModelSquarePriceCell from './ModelSquarePriceCell.vue'

const props = defineProps<{
  items: ModelSquareModel[]
  loading: boolean
  loadingMore: boolean
  hasMore: boolean
}>()
const emit = defineEmits<{ loadMore: []; copied: [name: string] }>()
const { t } = useI18n()
const desktopScrollRef = ref<HTMLElement | null>(null)
const mobileScrollRef = ref<HTMLElement | null>(null)

const columns = computed(() => [
  { key: 'name', label: t('modelSquare.columns.name'), width: 220 },
  { key: 'billing_mode', label: t('modelSquare.columns.billingMode'), width: 96 },
  { key: 'input', label: t('modelSquare.columns.input'), width: 190 },
  { key: 'output', label: t('modelSquare.columns.output'), width: 190 },
  { key: 'cache_read', label: t('modelSquare.columns.cacheRead'), width: 190 },
  { key: 'cache_write', label: t('modelSquare.columns.cacheWrite'), width: 220 },
  { key: 'image_output', label: t('modelSquare.columns.imageOutput'), width: 190 },
  { key: 'per_request', label: t('modelSquare.columns.perRequest'), width: 190 },
  { key: 'per_second', label: t('modelSquare.columns.perSecond'), width: 190 },
  { key: 'fast', label: t('modelSquare.columns.fast'), width: 210 },
  { key: 'tiers', label: t('modelSquare.columns.tiers'), width: 220 },
])

const gridStyle = computed(() => ({ gridTemplateColumns: columns.value.map(column => `${column.width}px`).join(' ') }))
const pricingSource = (item: ModelSquareModel) => item.pricing_source || 'system'
const virtualizer = useVirtualizer(computed(() => ({
  count: props.items.length,
  getScrollElement: () => desktopScrollRef.value,
  estimateSize: () => 84,
  overscan: 6,
})))

const measureRow: VNodeRef = (element) => {
  const node = element instanceof Element ? element : (element as ComponentPublicInstance | null)?.$el
  if (node instanceof Element) virtualizer.value.measureElement(node)
}

const LoadingRows = defineComponent({
  props: {
    loading: { type: Boolean, required: true },
    loadingMore: { type: Boolean, required: true },
  },
  setup(componentProps) {
    return () => {
      const count = componentProps.loading ? 5 : componentProps.loadingMore ? 3 : 0
      if (!count) return null
      return h('div', { class: 'space-y-2 p-4', 'aria-live': 'polite' }, Array.from({ length: count }, (_, index) => h('div', {
        key: index,
        class: componentProps.loading ? 'h-16 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-700' : 'h-12 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-700',
      })))
    }
  },
})

const TierPrices = defineComponent({
  props: { tier: { type: Object as PropType<ModelSquareTierPrice>, required: true } },
  setup(componentProps) {
    return () => h('div', { class: 'min-w-0 border-b border-gray-200 pb-3 last:border-0 last:pb-0 dark:border-dark-600' }, [
      h('div', { class: 'mb-2 font-medium' }, tierLabel(componentProps.tier)),
      h('div', { class: 'grid min-w-0 gap-3' }, tierEntries(componentProps.tier).map(entry => h('div', { key: entry.key, class: 'min-w-0' }, [
        h('div', { class: 'mb-1 text-xs text-gray-500' }, entry.label),
        h(ModelSquarePriceCell, { price: entry.price }),
      ]))),
    ])
  },
})

function priceFor(item: ModelSquareModel, key: string): ModelSquareUnitPrice | null | undefined {
  return item.prices[key as keyof typeof item.prices]
}

function regularEntries(item: ModelSquareModel) {
  return [
    { key: 'input', label: t('modelSquare.columns.input'), price: item.prices.input },
    { key: 'output', label: t('modelSquare.columns.output'), price: item.prices.output },
    { key: 'cache_read', label: t('modelSquare.columns.cacheRead'), price: item.prices.cache_read },
    { key: 'cache_write_5m', label: t('modelSquare.columns.cacheWrite5m'), price: item.prices.cache_write_5m },
    { key: 'cache_write_1h', label: t('modelSquare.columns.cacheWrite1h'), price: item.prices.cache_write_1h },
    { key: 'image_output', label: t('modelSquare.columns.imageOutput'), price: item.prices.image_output },
    { key: 'per_request', label: t('modelSquare.columns.perRequest'), price: item.prices.per_request },
    { key: 'per_second', label: t('modelSquare.columns.perSecond'), price: item.prices.per_second },
  ].filter((entry): entry is { key: string; label: string; price: ModelSquareUnitPrice } => entry.price != null)
}

function fastEntries(item: ModelSquareModel) {
  if (!item.fast_prices) return []
  return [
    { key: 'input', label: t('modelSquare.columns.input'), price: item.fast_prices.input },
    { key: 'output', label: t('modelSquare.columns.output'), price: item.fast_prices.output },
    { key: 'cache_read', label: t('modelSquare.columns.cacheRead'), price: item.fast_prices.cache_read },
    { key: 'cache_write_5m', label: t('modelSquare.columns.cacheWrite5m'), price: item.fast_prices.cache_write_5m },
    { key: 'cache_write_1h', label: t('modelSquare.columns.cacheWrite1h'), price: item.fast_prices.cache_write_1h },
  ].filter((entry): entry is { key: string; label: string; price: ModelSquareUnitPrice } => entry.price != null)
}

function tierEntries(tier: ModelSquareTierPrice) {
  return [
    { key: 'input', label: t('modelSquare.columns.input'), price: tier.input },
    { key: 'output', label: t('modelSquare.columns.output'), price: tier.output },
    { key: 'cache_read', label: t('modelSquare.columns.cacheRead'), price: tier.cache_read },
    { key: 'cache_write', label: t('modelSquare.columns.cacheWrite'), price: tier.cache_write },
    { key: 'per_request', label: t('modelSquare.columns.perRequest'), price: tier.per_request },
  ].filter((entry): entry is { key: string; label: string; price: ModelSquareUnitPrice } => entry.price != null)
}

function tierLabel(tier: ModelSquareTierPrice) {
  if (tier.tier_label) return tier.tier_label
  const maximum = tier.max_tokens == null ? 'unlimited' : tier.max_tokens.toLocaleString()
  return `${tier.min_tokens.toLocaleString()} - ${maximum} ${t('modelSquare.tokens')}`
}

function primaryPrice(item: ModelSquareModel): ModelSquareUnitPrice | null {
  return item.prices.per_request
    ?? item.prices.per_second
    ?? item.prices.image_output
    ?? item.prices.input
    ?? item.prices.output
    ?? null
}

function compactPrice(price: ModelSquareUnitPrice | null) {
  if (!price) return ''
  if (Number(price.multiplier_price) === 0) return t('modelSquare.free')
  return `$${Number(price.multiplier_price).toFixed(6)}`
}

async function copyModel(name: string) {
  await navigator.clipboard.writeText(name)
  emit('copied', name)
}

function handleScroll(event: Event) {
  const element = event.currentTarget as HTMLElement | null
  if (!element || !props.hasMore || props.loadingMore) return
  if (element.scrollHeight - element.scrollTop - element.clientHeight < 300) emit('loadMore')
}
</script>
