<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <CacheNavPills active="advanced" />
            <h1 class="mt-4 text-2xl font-semibold text-gray-900 dark:text-white">
              {{ t('admin.cacheManagement.advancedPage.title') }}
            </h1>
            <p class="mt-2 max-w-3xl text-sm text-gray-600 dark:text-gray-400">
              {{ t('admin.cacheManagement.advancedPage.description') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary" :disabled="loading || saving || statsLoading" @click="loadAll(true)">
              {{ t('admin.cacheManagement.advancedPage.actions.refresh') }}
            </button>
            <button
              type="button"
              class="btn btn-primary"
              :disabled="saving || loading || !canManage || validationErrors.length > 0 || !dirty"
              @click="saveConfig"
            >
              {{ saving ? t('admin.cacheManagement.advancedPage.actions.saving') : t('admin.cacheManagement.advancedPage.actions.save') }}
            </button>
          </div>
        </div>
      </div>

      <div v-if="!canViewPage" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200">
        {{ t('admin.cacheManagement.advancedPage.noPermission') }}
      </div>

      <template v-else>
        <div v-if="pageAlert" class="rounded-xl border px-4 py-3 text-sm" :class="pageAlert.className">
          {{ pageAlert.message }}
        </div>

        <div
          v-if="!canManage"
          class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/10 dark:text-amber-200"
        >
          {{ t('admin.cacheManagement.advancedPage.readonlyNotice') }}
        </div>

        <div
          v-if="validationErrors.length > 0"
          class="rounded-xl border border-red-200 bg-red-50 px-4 py-4 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200"
        >
          <p class="font-medium">{{ t('admin.cacheManagement.validationTitle') }}</p>
          <ul class="mt-2 list-disc space-y-1 pl-5">
            <li v-for="item in validationErrors" :key="item">{{ item }}</li>
          </ul>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
          <div v-for="card in summaryCards" :key="card.key" class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ card.label }}</p>
            <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ card.value }}</p>
            <p v-if="card.hint" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ card.hint }}</p>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
          <div class="space-y-6">
            <section class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.form.title') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.form.hint') }}</p>
              </div>
              <div class="space-y-6 px-6 py-5">
                <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                    <div class="flex items-center justify-between gap-4">
                      <div>
                        <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.fields.enabled.label') }}</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.enabled.hint') }}</p>
                      </div>
                      <Toggle v-model="form.advanced_cache_enabled" :disabled="!canManage" />
                    </div>
                  </div>
                  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                    <div class="flex items-center justify-between gap-4">
                      <div>
                        <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.fields.compressionEnabled.label') }}</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.compressionEnabled.hint') }}</p>
                      </div>
                      <Toggle v-model="form.compression_enabled" :disabled="!canManage" />
                    </div>
                  </div>
                  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                    <div class="flex items-center justify-between gap-4">
                      <div>
                        <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.fields.costSavingEnabled.label') }}</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.costSavingEnabled.hint') }}</p>
                      </div>
                      <Toggle v-model="form.cost_saving_enabled" :disabled="!canManage" />
                    </div>
                  </div>
                  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                    <div class="flex items-center justify-between gap-4">
                      <div>
                        <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.fields.promptCacheEnabled.label') }}</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.promptCacheEnabled.hint') }}</p>
                      </div>
                      <Toggle v-model="form.upstream_prompt_cache_enabled" :disabled="!canManage" />
                    </div>
                  </div>
                </div>

                <div class="grid grid-cols-1 gap-5 md:grid-cols-2">
                  <label class="block">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.fields.redisCapacity.label') }}</span>
                    <input v-model.number="form.redis_capacity_mb" type="number" min="64" :max="form.memory_safe_limit_mb" step="1" class="input" :disabled="!canManage" />
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.redisCapacity.hint', { value: form.memory_safe_limit_mb }) }}</p>
                  </label>
                  <label class="block">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.fields.memorySafeLimit.label') }}</span>
                    <input :value="`${form.memory_safe_limit_mb} MB`" type="text" class="input" disabled readonly />
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.memorySafeLimit.hint') }}</p>
                  </label>
                  <label class="block">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.fields.compressionThreshold.label') }}</span>
                    <input v-model.number="form.compression_threshold_kb" type="number" min="1" :max="responseLimitKB" step="1" class="input" :disabled="!canManage || !form.compression_enabled" />
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.compressionThreshold.hint', { value: responseLimitKB }) }}</p>
                  </label>
                  <label class="block">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.fields.responseLimit.label') }}</span>
                    <input :value="formatBytes(cacheConfig.max_response_bytes)" type="text" class="input" disabled readonly />
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.responseLimit.hint') }}</p>
                  </label>
                  <label class="block">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.fields.evictionPolicy.label') }}</span>
                    <select v-model="form.eviction_policy" class="input" :disabled="!canManage">
                      <option v-for="option in evictionPolicyOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
                    </select>
                  </label>
                  <label class="block">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.fields.hotWindow.label') }}</span>
                    <select v-model="form.hot_window" class="input" :disabled="!canManage">
                      <option v-for="option in hotWindowOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
                    </select>
                  </label>
                  <label class="block md:col-span-2">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.fields.hotThreshold.label') }}</span>
                    <input v-model.number="form.hot_threshold" type="number" min="1" step="1" class="input" :disabled="!canManage" />
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.hotThreshold.hint') }}</p>
                  </label>
                </div>

                <div class="space-y-4 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.fields.grayScope.title') }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.grayScope.hint') }}</p>
                  </div>

                  <div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
                    <div class="space-y-3">
                      <p class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.fields.grayScope.apiKeys') }}</p>
                      <input
                        v-model.trim="apiKeyKeyword"
                        type="text"
                        class="input"
                        :disabled="!canManage"
                        :placeholder="t('admin.cacheManagement.advancedPage.fields.grayScope.apiKeyPlaceholder')"
                      />
                      <div class="max-h-40 space-y-2 overflow-y-auto rounded-lg border border-dashed border-gray-200 p-3 dark:border-dark-700">
                        <button
                          v-for="item in apiKeyOptions"
                          :key="item.id"
                          type="button"
                          class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm transition hover:bg-gray-50 dark:hover:bg-dark-700"
                          :disabled="!canManage"
                          @click="toggleApiKey(item)"
                        >
                          <span>{{ apiKeyOptionLabel(item) }}</span>
                          <span class="text-xs text-gray-500 dark:text-gray-400">{{ selectedApiKeyIds.has(item.id) ? t('admin.cacheManagement.advancedPage.fields.grayScope.selected') : t('admin.cacheManagement.advancedPage.fields.grayScope.select') }}</span>
                        </button>
                        <p v-if="apiKeyKeyword && apiKeyOptions.length === 0" class="text-xs text-gray-500 dark:text-gray-400">
                          {{ t('admin.cacheManagement.advancedPage.fields.grayScope.apiKeyEmpty') }}
                        </p>
                      </div>
                      <div class="flex flex-wrap gap-2">
                        <span v-for="item in selectedApiKeys" :key="`selected-api-${item.id}`" class="inline-flex items-center gap-2 rounded-full bg-primary-50 px-3 py-1 text-xs text-primary-700 dark:bg-primary-900/20 dark:text-primary-200">
                          {{ apiKeyOptionLabel(item) }}
                          <button type="button" :disabled="!canManage" @click="removeApiKey(item.id)">×</button>
                        </span>
                      </div>
                    </div>

                    <div class="space-y-3">
                      <p class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.fields.grayScope.groups') }}</p>
                      <div class="max-h-52 space-y-2 overflow-y-auto rounded-lg border border-dashed border-gray-200 p-3 dark:border-dark-700">
                        <label v-for="group in groupOptions" :key="group.id" class="flex items-center gap-3 text-sm text-gray-700 dark:text-gray-300">
                          <input :checked="selectedGroupIds.has(group.id)" type="checkbox" class="h-4 w-4 rounded border-gray-300" :disabled="!canManage" @change="toggleGroup(group.id)" />
                          <span>{{ group.name }}</span>
                        </label>
                        <p v-if="groupOptions.length === 0" class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.grayScope.groupsEmpty') }}</p>
                      </div>
                    </div>

                    <div class="space-y-3">
                      <p class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.fields.grayScope.models') }}</p>
                      <div class="flex gap-2">
                        <input
                          v-model.trim="modelKeyword"
                          type="text"
                          class="input"
                          :disabled="!canManage"
                          :placeholder="t('admin.cacheManagement.advancedPage.fields.grayScope.modelsPlaceholder')"
                          @keydown.enter.prevent="addModel"
                        />
                        <button type="button" class="btn btn-secondary shrink-0" :disabled="!canManage || !modelKeyword" @click="addModel">
                          {{ t('admin.cacheManagement.advancedPage.fields.grayScope.addModel') }}
                        </button>
                      </div>
                      <div class="flex min-h-[96px] flex-wrap gap-2 rounded-lg border border-dashed border-gray-200 p-3 dark:border-dark-700">
                        <span v-for="model in form.gray_scope.models" :key="model" class="inline-flex items-center gap-2 rounded-full bg-primary-50 px-3 py-1 text-xs text-primary-700 dark:bg-primary-900/20 dark:text-primary-200">
                          {{ model }}
                          <button type="button" :disabled="!canManage" @click="removeModel(model)">×</button>
                        </span>
                        <p v-if="form.gray_scope.models.length === 0" class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.fields.grayScope.modelsEmpty') }}</p>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </section>
          </div>

          <div class="space-y-6">
            <section class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.filters.title') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.filters.hint') }}</p>
              </div>
              <div class="space-y-4 px-6 py-5">
                <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
                  <label class="block">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.filters.timeRange') }}</span>
                    <select v-model="statsFilters.time_range" class="input">
                      <option v-for="option in timeRangeOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
                    </select>
                  </label>
                  <label class="block">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.filters.hotspotLimit') }}</span>
                    <input v-model.number="statsFilters.hotspot_limit" type="number" min="1" max="100" step="1" class="input" />
                  </label>
                  <label class="block">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.filters.platform') }}</span>
                    <select v-model="statsFilters.platform" class="input">
                      <option value="">{{ t('admin.cacheManagement.advancedPage.filters.allPlatforms') }}</option>
                      <option v-for="option in platformOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
                    </select>
                  </label>
                  <label class="block">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.filters.group') }}</span>
                    <select v-model="statsFilters.group_id" class="input">
                      <option value="">{{ t('admin.cacheManagement.advancedPage.filters.allGroups') }}</option>
                      <option v-for="group in groupOptions" :key="group.id" :value="String(group.id)">{{ group.name }}</option>
                    </select>
                  </label>
                  <label class="block md:col-span-2">
                    <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.advancedPage.filters.model') }}</span>
                    <input v-model.trim="statsFilters.model" type="text" class="input" :placeholder="t('admin.cacheManagement.advancedPage.filters.modelPlaceholder')" />
                  </label>
                </div>
                <div v-if="statsLoadError" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200">
                  {{ statsLoadError }}
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <button type="button" class="btn btn-primary" :disabled="statsLoading" @click="loadStats(true)">
                    {{ statsLoading ? t('admin.cacheManagement.advancedPage.filters.loading') : t('admin.cacheManagement.advancedPage.filters.apply') }}
                  </button>
                  <button type="button" class="btn btn-secondary" :disabled="statsLoading" @click="resetStatsFilters">
                    {{ t('admin.cacheManagement.advancedPage.filters.reset') }}
                  </button>
                </div>
              </div>
            </section>

            <section class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.info.title') }}</h2>
              </div>
              <div class="space-y-3 px-6 py-5 text-sm text-gray-600 dark:text-gray-300">
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.info.grayScope') }}</p>
                  <p class="mt-1 font-medium text-gray-900 dark:text-white">{{ grayScopeSummary }}</p>
                </div>
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.info.responseLimit') }}</p>
                  <p class="mt-1 font-medium text-gray-900 dark:text-white">{{ formatBytes(cacheConfig.max_response_bytes) }}</p>
                </div>
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.advancedPage.info.updatedAt') }}</p>
                  <p class="mt-1 font-medium text-gray-900 dark:text-white">{{ statsUpdatedAt }}</p>
                </div>
              </div>
            </section>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
          <section class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.sections.capacity') }}</h2>
            </div>
            <div class="grid grid-cols-1 gap-4 px-6 py-5 md:grid-cols-2">
              <div v-for="item in capacityCards" :key="item.key" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</p>
                <p class="mt-2 text-lg font-semibold text-gray-900 dark:text-white">{{ item.value }}</p>
                <p v-if="item.hint" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ item.hint }}</p>
              </div>
            </div>
          </section>

          <section class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.sections.compression') }}</h2>
            </div>
            <div class="grid grid-cols-1 gap-4 px-6 py-5 md:grid-cols-2">
              <div v-for="item in compressionCards" :key="item.key" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</p>
                <p class="mt-2 text-lg font-semibold text-gray-900 dark:text-white">{{ item.value }}</p>
              </div>
            </div>
          </section>
        </div>

        <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,0.8fr)]">
          <section class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.sections.hotspots') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ hotspotSectionHint }}</p>
            </div>
            <div class="overflow-x-auto px-6 py-5">
              <div v-if="lastStats.hotspots.length === 0" class="rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
                {{ t('admin.cacheManagement.advancedPage.empty.hotspots') }}
              </div>
              <table v-else class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
                <thead class="bg-gray-50 dark:bg-dark-900/40">
                  <tr>
                    <th v-for="column in hotspotColumns" :key="column.key" class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ column.label }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="row in lastStats.hotspots" :key="`${row.rank}-${row.platform}-${row.model}-${row.api_key.id}`">
                    <td class="px-4 py-3 text-sm text-gray-900 dark:text-white">#{{ row.rank }}</td>
                    <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                      <div class="font-medium">{{ row.model || '--' }}</div>
                      <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ row.platform || '--' }}</div>
                    </td>
                    <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">{{ row.group?.name || t('admin.cacheManagement.advancedPage.hotspotFallbacks.noGroup') }}</td>
                    <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">{{ row.api_key?.display || t('admin.cacheManagement.advancedPage.hotspotFallbacks.unknownKey') }}</td>
                    <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">{{ formatInteger(row.hit_count) }}</td>
                    <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">{{ formatInteger(row.hit_tokens) }}</td>
                    <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">{{ formatDateTime(row.last_hit_at) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <section class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.cacheManagement.advancedPage.sections.savings') }}</h2>
            </div>
            <div class="space-y-4 px-6 py-5">
              <div v-for="item in savingsCards" :key="item.key" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</p>
                <p class="mt-2 text-lg font-semibold text-gray-900 dark:text-white">{{ item.value }}</p>
                <p v-if="item.hint" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ item.hint }}</p>
              </div>
            </div>
          </section>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import CacheNavPills from './CacheNavPills.vue'
import Toggle from '@/components/common/Toggle.vue'
import { adminAPI } from '@/api/admin'
import {
  defaultAdvancedCacheConfig,
  defaultCacheManagementConfig,
  type AdvancedCacheConfig,
  type AdvancedCacheStatsResponse,
  type CacheManagementConfig,
} from '@/api/admin/cache'
import type { AdminGroup } from '@/types'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatCurrency } from '@/utils/format'
import { formatApiKeyOptionLabel } from '@/utils/adminSensitiveDisplay'

interface ApiKeyOption {
  id: number
  name: string
}

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const loading = ref(true)
const statsLoading = ref(false)
const saving = ref(false)
const loadError = ref('')
const statsLoadError = ref('')
const groups = ref<AdminGroup[]>([])
const apiKeyOptions = ref<ApiKeyOption[]>([])
const apiKeyKeyword = ref('')
const modelKeyword = ref('')
const apiKeySearchSeq = ref(0)
const lastSavedSnapshot = ref('')

const cacheConfig = reactive<CacheManagementConfig>(defaultCacheManagementConfig())
const form = reactive<AdvancedCacheConfig>(defaultAdvancedCacheConfig())
const selectedApiKeys = ref<ApiKeyOption[]>([])
const lastStats = ref<AdvancedCacheStatsResponse>({
  capacity: {
    current_used_bytes: 0,
    capacity_limit_bytes: 0,
    capacity_usage_rate: 0,
    memory_safe_limit_bytes: 0,
    eviction_policy: 'LRU',
    recent_eviction_count: 0,
    last_evicted_at: null,
  },
  compression: {
    enabled: true,
    raw_response_bytes: 0,
    stored_response_bytes: 0,
    compression_saved_bytes: 0,
    compression_saved_rate: 0,
    compressed_entry_count: 0,
    compression_failed_count: 0,
    decompression_failed_count: 0,
  },
  hotspots: [],
  savings: {
    local_response_cache_saved_tokens: 0,
    local_response_cache_saved_amount: null,
    upstream_prompt_cache_read_tokens: 0,
    upstream_prompt_cache_write_tokens: 0,
    upstream_prompt_cache_saved_amount: null,
    total_estimated_saved_amount: null,
    price_missing: false,
    price_missing_models: [],
  },
  empty_states: {
    hotspots: true,
    prompt_cache: true,
    price: false,
  },
  fallback: {
    advanced_cache_fallback_active: false,
    fallback_reason: null,
    last_fallback_at: null,
  },
  updated_at: '',
})

const statsFilters = reactive({
  time_range: '1d',
  platform: '',
  model: '',
  group_id: '',
  hotspot_limit: 20,
})

const viewerRole = computed(() => String((authStore.user as { role?: string } | null)?.role || '').trim().toLowerCase())
const canManage = computed(() => viewerRole.value === '' || viewerRole.value === 'admin')
const canViewPage = computed(() => {
  const role = viewerRole.value
  return role === '' || role === 'admin' || role === 'ops' || role === 'operator' || role === 'operation' || role === 'operations' || role === 'yunying' || role === '运营'
})
const canViewAmount = computed(() => {
  const role = viewerRole.value
  return role === '' || role === 'admin' || role === 'business' || role === 'business_operator' || role === 'business-operator' || role === 'yunying' || role === '运营'
})

const selectedApiKeyIds = computed(() => new Set(form.gray_scope.api_key_ids))
const selectedGroupIds = computed(() => new Set(form.gray_scope.group_ids))
const groupOptions = computed(() => groups.value.filter((group) => group && typeof group.id === 'number'))
const responseLimitKB = computed(() => Math.max(1, Math.floor((cacheConfig.max_response_bytes || 0) / 1024)))
const grayScopeSummary = computed(() => {
  const apiKeys = selectedApiKeys.value.length
  const groupsCount = form.gray_scope.group_ids.length
  const modelsCount = form.gray_scope.models.length
  return t('admin.cacheManagement.advancedPage.info.grayScopeValue', { apiKeys, groups: groupsCount, models: modelsCount })
})
const statsUpdatedAt = computed(() => formatDateTime(lastStats.value.updated_at))

const evictionPolicyOptions = [
  { value: 'LRU', label: 'LRU' },
  { value: 'LFU', label: 'LFU' },
  { value: 'W-TinyLFU', label: 'W-TinyLFU' },
]

const hotWindowOptions = computed(() => [
  { value: '15m', label: t('admin.cacheManagement.advancedPage.hotWindows.15m') },
  { value: '1h', label: t('admin.cacheManagement.advancedPage.hotWindows.1h') },
  { value: '6h', label: t('admin.cacheManagement.advancedPage.hotWindows.6h') },
  { value: '24h', label: t('admin.cacheManagement.advancedPage.hotWindows.24h') },
])

const timeRangeOptions = computed(() => [
  { value: '1h', label: t('admin.cacheManagement.advancedPage.timeRanges.1h') },
  { value: '6h', label: t('admin.cacheManagement.advancedPage.timeRanges.6h') },
  { value: '1d', label: t('admin.cacheManagement.advancedPage.timeRanges.1d') },
  { value: '7d', label: t('admin.cacheManagement.advancedPage.timeRanges.7d') },
  { value: '31d', label: t('admin.cacheManagement.advancedPage.timeRanges.31d') },
])

const platformOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'claude', label: 'Claude' },
  { value: 'gemini', label: 'Gemini' },
]

const validationErrors = computed(() => {
  const errors: string[] = []
  if (!Number.isFinite(form.redis_capacity_mb) || form.redis_capacity_mb < 64 || form.redis_capacity_mb > form.memory_safe_limit_mb) {
    errors.push(t('admin.cacheManagement.advancedPage.validation.redisCapacity', { value: form.memory_safe_limit_mb }))
  }
  if (!Number.isFinite(form.compression_threshold_kb) || form.compression_threshold_kb < 1 || form.compression_threshold_kb > responseLimitKB.value) {
    errors.push(t('admin.cacheManagement.advancedPage.validation.compressionThreshold', { value: responseLimitKB.value }))
  }
  if (!evictionPolicyOptions.some((option) => option.value === form.eviction_policy)) {
    errors.push(t('admin.cacheManagement.advancedPage.validation.evictionPolicy'))
  }
  if (!hotWindowOptions.value.some((option) => option.value === form.hot_window)) {
    errors.push(t('admin.cacheManagement.advancedPage.validation.hotWindow'))
  }
  if (!Number.isFinite(form.hot_threshold) || form.hot_threshold < 1) {
    errors.push(t('admin.cacheManagement.advancedPage.validation.hotThreshold'))
  }
  return errors
})

const dirty = computed(() => JSON.stringify(buildPayload()) !== lastSavedSnapshot.value)

const pageAlert = computed(() => {
  if (loadError.value) {
    return {
      className: 'border-red-200 bg-red-50 text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200',
      message: loadError.value,
    }
  }
  if (!form.advanced_cache_enabled) {
    return {
      className: 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/10 dark:text-amber-200',
      message: t('admin.cacheManagement.advancedPage.alerts.disabled'),
    }
  }
  if (lastStats.value.fallback.advanced_cache_fallback_active) {
    return {
      className: 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/10 dark:text-amber-200',
      message: t('admin.cacheManagement.advancedPage.alerts.fallback', {
        reason: formatFallbackReason(lastStats.value.fallback.fallback_reason),
      }),
    }
  }
  return null
})

const summaryCards = computed(() => [
  {
    key: 'mode',
    label: t('admin.cacheManagement.advancedPage.summary.mode'),
    value: form.advanced_cache_enabled ? t('common.enabled') : t('common.disabled'),
    hint: form.gray_scope.api_key_ids.length + form.gray_scope.group_ids.length + form.gray_scope.models.length === 0
      ? t('admin.cacheManagement.advancedPage.summary.grayScopeEmpty')
      : grayScopeSummary.value,
  },
  {
    key: 'capacity',
    label: t('admin.cacheManagement.advancedPage.summary.capacity'),
    value: `${form.redis_capacity_mb} MB`,
    hint: t('admin.cacheManagement.advancedPage.summary.safeLimit', { value: form.memory_safe_limit_mb }),
  },
  {
    key: 'compression',
    label: t('admin.cacheManagement.advancedPage.summary.compression'),
    value: form.compression_enabled ? t('common.enabled') : t('common.disabled'),
    hint: t('admin.cacheManagement.advancedPage.summary.compressionThreshold', { value: form.compression_threshold_kb }),
  },
  {
    key: 'updated',
    label: t('admin.cacheManagement.advancedPage.summary.updatedAt'),
    value: statsUpdatedAt.value,
  },
])

const capacityCards = computed(() => [
  {
    key: 'used',
    label: t('admin.cacheManagement.advancedPage.capacity.currentUsed'),
    value: formatBytes(lastStats.value.capacity.current_used_bytes),
  },
  {
    key: 'limit',
    label: t('admin.cacheManagement.advancedPage.capacity.limit'),
    value: formatBytes(lastStats.value.capacity.capacity_limit_bytes || form.redis_capacity_mb * 1024 * 1024),
  },
  {
    key: 'usageRate',
    label: t('admin.cacheManagement.advancedPage.capacity.usageRate'),
    value: formatPercent(lastStats.value.capacity.capacity_usage_rate),
    hint: t('admin.cacheManagement.advancedPage.capacity.safeLimit', { value: formatBytes(lastStats.value.capacity.memory_safe_limit_bytes) }),
  },
  {
    key: 'policy',
    label: t('admin.cacheManagement.advancedPage.capacity.policy'),
    value: lastStats.value.capacity.eviction_policy || form.eviction_policy || 'LRU',
  },
  {
    key: 'evictions',
    label: t('admin.cacheManagement.advancedPage.capacity.evictions'),
    value: formatInteger(lastStats.value.capacity.recent_eviction_count),
  },
  {
    key: 'evictedAt',
    label: t('admin.cacheManagement.advancedPage.capacity.lastEvictedAt'),
    value: formatDateTime(lastStats.value.capacity.last_evicted_at),
  },
])

const compressionCards = computed(() => [
  {
    key: 'raw',
    label: t('admin.cacheManagement.advancedPage.compression.raw'),
    value: formatBytes(lastStats.value.compression.raw_response_bytes),
  },
  {
    key: 'stored',
    label: t('admin.cacheManagement.advancedPage.compression.stored'),
    value: formatBytes(lastStats.value.compression.stored_response_bytes),
  },
  {
    key: 'savedBytes',
    label: t('admin.cacheManagement.advancedPage.compression.savedBytes'),
    value: formatBytes(lastStats.value.compression.compression_saved_bytes),
  },
  {
    key: 'savedRate',
    label: t('admin.cacheManagement.advancedPage.compression.savedRate'),
    value: formatPercent(lastStats.value.compression.compression_saved_rate),
  },
  {
    key: 'failed',
    label: t('admin.cacheManagement.advancedPage.compression.failed'),
    value: formatInteger(lastStats.value.compression.compression_failed_count),
  },
  {
    key: 'decompressFailed',
    label: t('admin.cacheManagement.advancedPage.compression.decompressFailed'),
    value: formatInteger(lastStats.value.compression.decompression_failed_count),
  },
])

const hotspotColumns = computed(() => [
  { key: 'rank', label: t('admin.cacheManagement.advancedPage.hotspots.rank') },
  { key: 'model', label: t('admin.cacheManagement.advancedPage.hotspots.model') },
  { key: 'group', label: t('admin.cacheManagement.advancedPage.hotspots.group') },
  { key: 'apiKey', label: t('admin.cacheManagement.advancedPage.hotspots.apiKey') },
  { key: 'hits', label: t('admin.cacheManagement.advancedPage.hotspots.hits') },
  { key: 'tokens', label: t('admin.cacheManagement.advancedPage.hotspots.tokens') },
  { key: 'lastHitAt', label: t('admin.cacheManagement.advancedPage.hotspots.lastHitAt') },
])

const hotspotSectionHint = computed(() => {
  if (statsLoadError.value && lastStats.value.hotspots.length > 0) {
    return t('admin.cacheManagement.advancedPage.hotspots.failedWithFallback')
  }
  return t('admin.cacheManagement.advancedPage.hotspots.hint')
})

const savingsCards = computed(() => {
  const amountVisible = canViewAmount.value
  return [
    {
      key: 'localTokens',
      label: t('admin.cacheManagement.advancedPage.savings.localTokens'),
      value: formatInteger(lastStats.value.savings.local_response_cache_saved_tokens),
    },
    {
      key: 'localAmount',
      label: t('admin.cacheManagement.advancedPage.savings.localAmount'),
      value: amountVisible ? formatAmount(lastStats.value.savings.local_response_cache_saved_amount) : t('admin.cacheManagement.advancedPage.hiddenAmount'),
    },
    {
      key: 'promptRead',
      label: t('admin.cacheManagement.advancedPage.savings.promptRead'),
      value: lastStats.value.empty_states.prompt_cache
        ? t('admin.cacheManagement.advancedPage.empty.promptCache')
        : formatInteger(lastStats.value.savings.upstream_prompt_cache_read_tokens),
    },
    {
      key: 'promptWrite',
      label: t('admin.cacheManagement.advancedPage.savings.promptWrite'),
      value: lastStats.value.empty_states.prompt_cache
        ? t('admin.cacheManagement.advancedPage.empty.promptCache')
        : formatInteger(lastStats.value.savings.upstream_prompt_cache_write_tokens),
    },
    {
      key: 'promptAmount',
      label: t('admin.cacheManagement.advancedPage.savings.promptAmount'),
      value: amountVisible ? formatAmount(lastStats.value.savings.upstream_prompt_cache_saved_amount) : t('admin.cacheManagement.advancedPage.hiddenAmount'),
    },
    {
      key: 'totalAmount',
      label: t('admin.cacheManagement.advancedPage.savings.totalAmount'),
      value: amountVisible ? formatAmount(lastStats.value.savings.total_estimated_saved_amount) : t('admin.cacheManagement.advancedPage.hiddenAmount'),
      hint: lastStats.value.savings.price_missing
        ? t('admin.cacheManagement.advancedPage.savings.priceMissingModels', { models: formatMissingModels(lastStats.value.savings.price_missing_models) })
        : t('admin.cacheManagement.advancedPage.savings.priceComplete'),
    },
  ]
})

watch(apiKeyKeyword, async (value) => {
  const query = value.trim()
  if (query.length < 2) {
    apiKeyOptions.value = selectedApiKeys.value.slice()
    return
  }

  const seq = ++apiKeySearchSeq.value
  try {
    const rows = await adminAPI.usage.searchApiKeys(undefined, query)
    if (seq !== apiKeySearchSeq.value) return
    apiKeyOptions.value = rows.map((item) => ({ id: item.id, name: item.name }))
  } catch {
    if (seq !== apiKeySearchSeq.value) return
    apiKeyOptions.value = selectedApiKeys.value.slice()
  }
})

function buildPayload(): AdvancedCacheConfig {
  return {
    advanced_cache_enabled: Boolean(form.advanced_cache_enabled),
    gray_scope: {
      api_key_ids: dedupeNumberList(form.gray_scope.api_key_ids),
      group_ids: dedupeNumberList(form.gray_scope.group_ids),
      models: dedupeStringList(form.gray_scope.models),
    },
    redis_capacity_mb: Math.round(Number(form.redis_capacity_mb) || 0),
    memory_safe_limit_mb: Math.round(Number(form.memory_safe_limit_mb) || 0),
    compression_enabled: Boolean(form.compression_enabled),
    compression_threshold_kb: Math.round(Number(form.compression_threshold_kb) || 0),
    eviction_policy: String(form.eviction_policy || '').trim(),
    hot_window: String(form.hot_window || '').trim(),
    hot_threshold: Math.round(Number(form.hot_threshold) || 0),
    cost_saving_enabled: Boolean(form.cost_saving_enabled),
    upstream_prompt_cache_enabled: Boolean(form.upstream_prompt_cache_enabled),
  }
}

function cloneAdvancedConfig(config: AdvancedCacheConfig): AdvancedCacheConfig {
  return JSON.parse(JSON.stringify(config)) as AdvancedCacheConfig
}

function rememberSaved(config: AdvancedCacheConfig): void {
  lastSavedSnapshot.value = JSON.stringify(config)
}

function applyConfig(config: AdvancedCacheConfig): void {
  const next = cloneAdvancedConfig(config)
  form.advanced_cache_enabled = next.advanced_cache_enabled
  form.gray_scope.api_key_ids = [...next.gray_scope.api_key_ids]
  form.gray_scope.group_ids = [...next.gray_scope.group_ids]
  form.gray_scope.models = [...next.gray_scope.models]
  form.redis_capacity_mb = next.redis_capacity_mb
  form.memory_safe_limit_mb = next.memory_safe_limit_mb
  form.compression_enabled = next.compression_enabled
  form.compression_threshold_kb = next.compression_threshold_kb
  form.eviction_policy = next.eviction_policy
  form.hot_window = next.hot_window
  form.hot_threshold = next.hot_threshold
  form.cost_saving_enabled = next.cost_saving_enabled
  form.upstream_prompt_cache_enabled = next.upstream_prompt_cache_enabled
}

function hydrateSelectedApiKeys(searchRows: ApiKeyOption[]): void {
  const merged = new Map<number, ApiKeyOption>()
  for (const item of selectedApiKeys.value) merged.set(item.id, item)
  for (const item of searchRows) merged.set(item.id, item)
  selectedApiKeys.value = form.gray_scope.api_key_ids.map((id) => merged.get(id) || { id, name: `#${id}` })
  if (apiKeyKeyword.value.trim().length < 2) {
    apiKeyOptions.value = selectedApiKeys.value.slice()
  }
}

function dedupeNumberList(items: number[]): number[] {
  const seen = new Set<number>()
  return items
    .map((item) => Number(item))
    .filter((item) => Number.isFinite(item) && item > 0)
    .filter((item) => {
      if (seen.has(item)) return false
      seen.add(item)
      return true
    })
}

function dedupeStringList(items: string[]): string[] {
  const seen = new Set<string>()
  return items
    .map((item) => String(item || '').trim())
    .filter(Boolean)
    .filter((item) => {
      const lower = item.toLowerCase()
      if (seen.has(lower)) return false
      seen.add(lower)
      return true
    })
}

function addModel(): void {
  const value = modelKeyword.value.trim()
  if (!value) return
  form.gray_scope.models = dedupeStringList([...form.gray_scope.models, value])
  modelKeyword.value = ''
}

function removeModel(model: string): void {
  form.gray_scope.models = form.gray_scope.models.filter((item) => item !== model)
}

function toggleGroup(groupId: number): void {
  const next = new Set(form.gray_scope.group_ids)
  if (next.has(groupId)) next.delete(groupId)
  else next.add(groupId)
  form.gray_scope.group_ids = Array.from(next)
}

function toggleApiKey(item: ApiKeyOption): void {
  const next = new Set(form.gray_scope.api_key_ids)
  if (next.has(item.id)) next.delete(item.id)
  else next.add(item.id)
  form.gray_scope.api_key_ids = Array.from(next)
  hydrateSelectedApiKeys([item])
}

function removeApiKey(id: number): void {
  form.gray_scope.api_key_ids = form.gray_scope.api_key_ids.filter((item) => item !== id)
  selectedApiKeys.value = selectedApiKeys.value.filter((item) => item.id !== id)
  apiKeyOptions.value = apiKeyOptions.value.filter((item) => item.id !== id)
}

function apiKeyOptionLabel(item: ApiKeyOption): string {
  return formatApiKeyOptionLabel(item.name, item.id)
}

function formatInteger(value: number | string | null | undefined): string {
  const numeric = Number(value ?? 0)
  if (!Number.isFinite(numeric)) return '--'
  return Math.round(numeric).toLocaleString()
}

function formatPercent(value: number | string | null | undefined): string {
  const numeric = Number(value ?? 0)
  if (!Number.isFinite(numeric)) return '0.00%'
  return `${numeric.toFixed(2)}%`
}

function formatBytes(bytes: number | null | undefined): string {
  const numeric = Number(bytes ?? 0)
  if (!Number.isFinite(numeric) || numeric <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = numeric
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toFixed(value >= 100 || index === 0 ? 0 : value >= 10 ? 1 : 2)} ${units[index]}`
}

function formatDateTime(value?: string | null): string {
  if (!value) return '--'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return '--'
  return parsed.toLocaleString()
}

function formatAmount(value?: string | null): string {
  if (!canViewAmount.value) return t('admin.cacheManagement.advancedPage.hiddenAmount')
  if (!value || lastStats.value.savings.price_missing) {
    return t('admin.cacheManagement.advancedPage.priceMissing')
  }
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) {
    return t('admin.cacheManagement.advancedPage.priceMissing')
  }
  return formatCurrency(numeric)
}

function formatMissingModels(models: string[]): string {
  if (!models || models.length === 0) {
    return t('admin.cacheManagement.advancedPage.savings.priceComplete')
  }
  return models.join('、')
}

function formatFallbackReason(reason?: string | null): string {
  if (!reason) return t('common.unknown')
  if (reason === 'advanced_cache_disabled') {
    return t('admin.cacheManagement.advancedPage.fallbackReasons.disabled')
  }
  return reason
}

function buildStatsQuery() {
  const query: Record<string, string | number> = {
    time_range: statsFilters.time_range,
    hotspot_limit: Math.min(100, Math.max(1, Math.round(Number(statsFilters.hotspot_limit) || 20))),
  }
  if (statsFilters.platform) query.platform = statsFilters.platform
  if (statsFilters.model) query.model = statsFilters.model
  if (statsFilters.group_id) query.group_id = Number(statsFilters.group_id)
  return query
}

async function loadGroups(): Promise<void> {
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch {
    groups.value = []
  }
}

async function loadConfig(forceToast = false): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const [{ data: configData }, { data: baseCacheData }] = await Promise.all([
      adminAPI.cache.getAdvancedConfig(),
      adminAPI.cache.getConfig(),
    ])
    const merged = {
      ...defaultAdvancedCacheConfig(),
      ...cloneAdvancedConfig(configData || defaultAdvancedCacheConfig()),
      gray_scope: {
        ...defaultAdvancedCacheConfig().gray_scope,
        ...(configData?.gray_scope || {}),
      },
    } satisfies AdvancedCacheConfig
    applyConfig(merged)
    rememberSaved(buildPayload())
    Object.assign(cacheConfig, {
      ...defaultCacheManagementConfig(),
      ...(baseCacheData || {}),
      platforms: {
        ...defaultCacheManagementConfig().platforms,
        ...(baseCacheData?.platforms || {}),
      },
      bypass_header: {
        ...defaultCacheManagementConfig().bypass_header,
        ...(baseCacheData?.bypass_header || {}),
      },
      model_allowlist: Array.isArray(baseCacheData?.model_allowlist) ? baseCacheData.model_allowlist : [],
      model_blocklist: Array.isArray(baseCacheData?.model_blocklist) ? baseCacheData.model_blocklist : [],
    })
    hydrateSelectedApiKeys([])
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, t('admin.cacheManagement.advancedPage.loadFailed'))
    if (forceToast) appStore.showError(loadError.value)
  } finally {
    loading.value = false
  }
}

async function loadStats(forceToast = false): Promise<void> {
  statsLoading.value = true
  statsLoadError.value = ''
  try {
    const { data } = await adminAPI.cache.getAdvancedStats(buildStatsQuery())
    lastStats.value = data || lastStats.value
  } catch (error) {
    statsLoadError.value = extractApiErrorMessage(error, t('admin.cacheManagement.advancedPage.statsLoadFailed'))
    if (forceToast) appStore.showError(statsLoadError.value)
  } finally {
    statsLoading.value = false
  }
}

async function loadAll(forceToast = false): Promise<void> {
  await Promise.all([loadConfig(forceToast), loadStats(forceToast), loadGroups()])
}

async function saveConfig(): Promise<void> {
  if (!canManage.value) {
    appStore.showError(t('admin.cacheManagement.advancedPage.readonlyNotice'))
    return
  }
  if (validationErrors.value.length > 0) {
    appStore.showError(validationErrors.value[0])
    return
  }

  saving.value = true
  try {
    const payload = buildPayload()
    const { data } = await adminAPI.cache.updateAdvancedConfig(payload)
    const merged = {
      ...payload,
      ...(data || {}),
      gray_scope: {
        ...payload.gray_scope,
        ...(data?.gray_scope || {}),
      },
    } satisfies AdvancedCacheConfig
    applyConfig(merged)
    rememberSaved(buildPayload())
    appStore.showSuccess(t('admin.cacheManagement.advancedPage.saved'))
    await loadStats(false)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.cacheManagement.advancedPage.saveFailed')))
  } finally {
    saving.value = false
  }
}

function resetStatsFilters(): void {
  statsFilters.time_range = '1d'
  statsFilters.platform = ''
  statsFilters.model = ''
  statsFilters.group_id = ''
  statsFilters.hotspot_limit = 20
  loadStats(false)
}

onMounted(async () => {
  await loadAll(false)
})
</script>
