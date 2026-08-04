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
                <div v-for="group in billingPriceGroups(items[virtualRow.index])" :key="group.key" class="mt-3">
                  <div class="mb-1 text-xs font-medium text-gray-500">{{ group.label }}</div>
                  <div class="space-y-3">
                    <div
                      v-for="entry in group.entries"
                      :key="entry.key"
                      :data-testid="entry.isTier ? 'model-square-tier-price' : undefined"
                    >
                      <div v-if="entry.label" class="mb-1 text-xs text-gray-500">{{ entry.label }}</div>
                      <ModelSquarePriceCell :price="entry.price" />
                    </div>
                  </div>
                </div>
              </div>
              <div
                v-else-if="isPriceColumn(column.key)"
                role="cell"
                :data-testid="`model-square-desktop-${column.key}-cell`"
                class="px-4 py-4"
              >
                <div v-if="priceEntriesFor(items[virtualRow.index], column.key).length" class="space-y-4">
                  <div
                    v-for="entry in priceEntriesFor(items[virtualRow.index], column.key)"
                    :key="entry.key"
                    :data-testid="entry.isTier ? 'model-square-tier-price' : undefined"
                  >
                    <div v-if="entry.label" class="mb-1 text-xs font-medium text-gray-500">{{ entry.label }}</div>
                    <ModelSquarePriceCell :price="entry.price" />
                  </div>
                </div>
                <span v-else class="text-gray-400" aria-hidden="true">—</span>
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
            <span v-for="entry in billingEntries(item)" :key="entry.key" class="mt-1 block text-xs text-gray-500 dark:text-dark-300">
              {{ entry.label }}: {{ compactPrice(entry.price) }}
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
          <div v-if="billingPriceGroups(item).length" class="min-w-0">
            <div class="mb-2 text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('modelSquare.columns.billingMode') }}</div>
            <div class="grid min-w-0 gap-4">
              <div v-for="group in billingPriceGroups(item)" :key="group.key" class="min-w-0">
                <div class="mb-1 text-xs font-medium text-gray-500">{{ group.label }}</div>
                <div class="space-y-3">
                  <div
                    v-for="entry in group.entries"
                    :key="entry.key"
                    :data-testid="entry.isTier ? 'model-square-tier-price' : undefined"
                  >
                    <div v-if="entry.label" class="mb-1 text-xs text-gray-500">{{ entry.label }}</div>
                    <ModelSquarePriceCell :price="entry.price" />
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div class="grid min-w-0 gap-4">
            <div
              v-for="column in mobilePriceColumns(item)"
              :key="column.key"
              :data-testid="`model-square-mobile-${column.key}-prices`"
              class="min-w-0"
            >
              <div class="mb-1 text-xs font-medium text-gray-500">{{ column.label }}</div>
              <div class="space-y-3">
                <div
                  v-for="entry in column.entries"
                  :key="entry.key"
                  :data-testid="entry.isTier ? 'model-square-tier-price' : undefined"
                  class="min-w-0"
                >
                  <div v-if="entry.label" class="mb-1 text-xs text-gray-500">{{ entry.label }}</div>
                  <ModelSquarePriceCell :price="entry.price" />
                </div>
              </div>
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
import { computed, defineComponent, h, ref, type ComponentPublicInstance, type VNodeRef } from 'vue'
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
type PriceColumnKey = 'input' | 'output' | 'cache_read' | 'cache_write'
type TierPriceKey = PriceColumnKey | 'per_request'
type PriceDisplayEntry = {
  key: string
  label: string | null
  price: ModelSquareUnitPrice
  isTier: boolean
}
const priceColumnKeys: PriceColumnKey[] = ['input', 'output', 'cache_read', 'cache_write']

const columns = computed(() => [
  { key: 'name', label: t('modelSquare.columns.name'), width: 220 },
  { key: 'billing_mode', label: t('modelSquare.columns.billingMode'), width: 220 },
  { key: 'input', label: t('modelSquare.columns.input'), width: 190 },
  { key: 'output', label: t('modelSquare.columns.output'), width: 190 },
  { key: 'cache_read', label: t('modelSquare.columns.cacheRead'), width: 190 },
  { key: 'cache_write', label: t('modelSquare.columns.cacheWrite'), width: 220 },
  { key: 'fast', label: t('modelSquare.columns.fast'), width: 210 },
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

function isPriceColumn(key: string): key is PriceColumnKey {
  return priceColumnKeys.includes(key as PriceColumnKey)
}

function singlePriceEntry(key: string, price: ModelSquareUnitPrice | null | undefined, label: string | null = null): PriceDisplayEntry[] {
  return price ? [{ key, label, price, isTier: false }] : []
}

function tierPriceEntriesFor(item: ModelSquareModel, key: TierPriceKey): PriceDisplayEntry[] {
  return item.tiers.flatMap((tier) => {
    const price = tier[key]
    if (!price) return []
    return [{
      key: `${key}-${tier.min_tokens}-${tier.max_tokens ?? 'open'}-${tier.sort_order}`,
      label: tierTitle(tier),
      price,
      isTier: true,
    }]
  })
}

function priceEntriesFor(item: ModelSquareModel, key: string): PriceDisplayEntry[] {
  if (!isPriceColumn(key)) return []
  const tierEntries = tierPriceEntriesFor(item, key)
  if (tierEntries.length) return tierEntries

  if (key === 'cache_write') {
    return [
      ...singlePriceEntry('cache_write_5m', item.prices.cache_write_5m, t('modelSquare.columns.cacheWrite5m')),
      ...singlePriceEntry('cache_write_1h', item.prices.cache_write_1h, t('modelSquare.columns.cacheWrite1h')),
    ]
  }

  return singlePriceEntry(key, item.prices[key])
}

function mobilePriceColumns(item: ModelSquareModel) {
  const labels: Record<PriceColumnKey, string> = {
    input: t('modelSquare.columns.input'),
    output: t('modelSquare.columns.output'),
    cache_read: t('modelSquare.columns.cacheRead'),
    cache_write: t('modelSquare.columns.cacheWrite'),
  }
  return priceColumnKeys
    .map(key => ({ key, label: labels[key], entries: priceEntriesFor(item, key) }))
    .filter(column => column.entries.length > 0)
}

function billingEntries(item: ModelSquareModel) {
  return [
    { key: 'image_output', label: t('modelSquare.columns.imageOutput'), price: item.prices.image_output },
    { key: 'per_request', label: t('modelSquare.columns.perRequest'), price: item.prices.per_request },
    { key: 'per_second', label: t('modelSquare.columns.perSecond'), price: item.prices.per_second },
  ].filter((entry): entry is { key: string; label: string; price: ModelSquareUnitPrice } => entry.price != null)
}

function billingPriceGroups(item: ModelSquareModel) {
  const tierPerRequest = tierPriceEntriesFor(item, 'per_request')
  return [
    {
      key: 'image_output',
      label: t('modelSquare.columns.imageOutput'),
      entries: singlePriceEntry('image_output', item.prices.image_output),
    },
    {
      key: 'per_request',
      label: t('modelSquare.columns.perRequest'),
      entries: tierPerRequest.length
        ? tierPerRequest
        : singlePriceEntry('per_request', item.prices.per_request),
    },
    {
      key: 'per_second',
      label: t('modelSquare.columns.perSecond'),
      entries: singlePriceEntry('per_second', item.prices.per_second),
    },
  ].filter(group => group.entries.length > 0)
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

function formatTokenCount(value: number) {
  return new Intl.NumberFormat().format(value)
}

function tierTitle(tier: ModelSquareTierPrice) {
  const min = formatTokenCount(tier.min_tokens)
  const range = tier.max_tokens == null
    ? `${t('modelSquare.tokens')} ≥ ${min}`
    : `${min} ≤ ${t('modelSquare.tokens')} < ${formatTokenCount(tier.max_tokens)}`
  return tier.tier_label ? `${tier.tier_label} · ${range}` : range
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
