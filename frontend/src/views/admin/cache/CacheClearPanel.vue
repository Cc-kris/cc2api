<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <div class="flex flex-wrap items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400">
              <component
                v-for="item in navItems"
                :key="item.key"
                :is="item.to ? 'router-link' : 'span'"
                v-bind="item.to ? { to: item.to } : {}"
                class="rounded-full border px-3 py-1 transition-colors"
                :class="item.active
                  ? 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-700/60 dark:bg-primary-900/10 dark:text-primary-200'
                  : item.to
                    ? 'border-gray-200 text-gray-500 hover:border-primary-200 hover:text-primary-600 dark:border-dark-700 dark:text-gray-400 dark:hover:border-primary-700/60 dark:hover:text-primary-200'
                    : 'border-gray-200 bg-gray-50 text-gray-400 dark:border-dark-700 dark:bg-dark-900/20 dark:text-gray-500'"
              >
                {{ item.label }}
              </component>
            </div>
            <h1 class="mt-4 text-2xl font-semibold text-gray-900 dark:text-white">
              {{ t('admin.cacheManagement.clearPage.title') }}
            </h1>
            <p class="mt-2 max-w-3xl text-sm text-gray-600 dark:text-gray-400">
              {{ t('admin.cacheManagement.clearPage.description') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary" :disabled="submitting" @click="resetForm">
              {{ t('admin.cacheManagement.clearPage.reset') }}
            </button>
            <button
              type="button"
              class="btn btn-danger"
              :disabled="!canSubmit || submitting || !canManage"
              @click="openConfirmDialog"
            >
              {{ submitting ? t('admin.cacheManagement.clearPage.submitting') : t('admin.cacheManagement.clearPage.submit') }}
            </button>
          </div>
        </div>
      </div>

      <div
        v-if="!canManage"
        class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/10 dark:text-amber-200"
      >
        {{ t('admin.cacheManagement.readonlyNotice') }}
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

      <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
        <div class="space-y-6">
          <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.cacheManagement.clearPage.clearTypeTitle') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.cacheManagement.clearPage.clearTypeHint') }}
              </p>
            </div>
            <div class="grid grid-cols-1 gap-3 px-6 py-5 md:grid-cols-2 xl:grid-cols-3">
              <label
                v-for="option in clearTypeOptions"
                :key="option.value"
                class="cursor-pointer rounded-xl border p-4 transition-colors"
                :class="form.clearType === option.value
                  ? 'border-primary-300 bg-primary-50/70 dark:border-primary-700/60 dark:bg-primary-900/10'
                  : 'border-gray-200 hover:border-primary-200 dark:border-dark-700 dark:hover:border-primary-700/50'"
              >
                <div class="flex items-start gap-3">
                  <input
                    v-model="form.clearType"
                    type="radio"
                    name="cache-clear-type"
                    :value="option.value"
                    class="mt-1 h-4 w-4 border-gray-300 text-primary-600 focus:ring-primary-500"
                    :disabled="submitting || !canManage"
                  />
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">{{ option.label }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ option.hint }}</p>
                  </div>
                </div>
              </label>
            </div>
          </div>

          <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.cacheManagement.clearPage.scopeTitle') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.cacheManagement.clearPage.scopeHint') }}
              </p>
            </div>
            <div class="space-y-5 px-6 py-5">
              <div v-if="requiresPlatforms" class="space-y-3">
                <div class="flex items-center justify-between gap-3">
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.cacheManagement.clearPage.fields.platforms') }}</p>
                  <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('common.selectedCount', { count: form.platforms.length }) }}</span>
                </div>
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <label
                    v-for="platform in platformOptions"
                    :key="platform.value"
                    class="flex cursor-pointer items-center justify-between rounded-lg border border-gray-200 px-4 py-3 text-sm dark:border-dark-700"
                  >
                    <span class="font-medium text-gray-900 dark:text-white">{{ platform.label }}</span>
                    <input
                      type="checkbox"
                      :checked="form.platforms.includes(platform.value)"
                      :disabled="submitting || !canManage"
                      class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                      @change="togglePlatform(platform.value, ($event.target as HTMLInputElement).checked)"
                    />
                  </label>
                </div>
              </div>

              <div v-if="form.clearType === 'by_model'" class="space-y-3">
                <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.cacheManagement.clearPage.fields.models') }}</p>
                <div class="flex flex-col gap-3 rounded-xl border border-gray-200 p-4 dark:border-dark-700">
                  <div class="flex gap-2">
                    <input
                      v-model.trim="modelKeyword"
                      type="text"
                      class="input flex-1"
                      :disabled="submitting || !canManage"
                      :placeholder="t('admin.cacheManagement.clearPage.fields.modelsPlaceholder')"
                      @keydown.enter.prevent="addModel"
                    />
                    <button type="button" class="btn btn-secondary shrink-0" :disabled="!modelKeyword || submitting || !canManage" @click="addModel">
                      {{ t('common.add') }}
                    </button>
                  </div>
                  <div class="flex flex-wrap gap-2" v-if="form.models.length > 0">
                    <span
                      v-for="model in form.models"
                      :key="model"
                      class="inline-flex items-center gap-2 rounded-full bg-primary-50 px-3 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-200"
                    >
                      {{ model }}
                      <button type="button" class="text-primary-500 hover:text-primary-700 dark:hover:text-primary-100" :disabled="submitting || !canManage" @click="removeModel(model)">
                        ×
                      </button>
                    </span>
                  </div>
                  <p v-else class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.clearPage.fields.modelsEmpty') }}</p>
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.clearPage.fields.modelsHint') }}</p>
                </div>
              </div>

              <div v-if="form.clearType === 'by_group'" class="space-y-3">
                <GroupSelector v-model="form.groupIds" :groups="groups" :searchable="true" />
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.clearPage.fields.groupsHint') }}</p>
              </div>

              <div v-if="form.clearType === 'by_api_key'" class="space-y-3">
                <div class="flex items-center justify-between gap-3">
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.cacheManagement.clearPage.fields.apiKeys') }}</p>
                  <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('common.selectedCount', { count: form.apiKeys.length }) }}</span>
                </div>
                <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
                  <div class="flex gap-2">
                    <input
                      v-model.trim="apiKeyKeyword"
                      type="text"
                      class="input flex-1"
                      :disabled="submitting || !canManage"
                      :placeholder="t('admin.cacheManagement.clearPage.fields.apiKeysPlaceholder')"
                      @input="() => debounceApiKeySearch()"
                    />
                    <button type="button" class="btn btn-secondary shrink-0" :disabled="submitting || !canManage" @click="debounceApiKeySearch(true)">
                      {{ t('common.search') }}
                    </button>
                  </div>
                  <div v-if="apiKeyResults.length > 0" class="mt-3 space-y-2 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30">
                    <button
                      v-for="item in apiKeyResults"
                      :key="item.id"
                      type="button"
                      class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm transition-colors hover:bg-white dark:hover:bg-dark-800"
                      :disabled="submitting || !canManage"
                      @click="selectApiKey(item)"
                    >
                      <span class="font-medium text-gray-900 dark:text-white">{{ item.name || `#${item.id}` }}</span>
                      <span class="text-xs text-gray-500 dark:text-gray-400">#{{ item.id }}</span>
                    </button>
                  </div>
                  <p v-else-if="apiKeySearchTried" class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.clearPage.fields.apiKeysEmpty') }}</p>
                  <div class="mt-3 flex flex-wrap gap-2" v-if="form.apiKeys.length > 0">
                    <span
                      v-for="item in form.apiKeys"
                      :key="item.id"
                      class="inline-flex items-center gap-2 rounded-full bg-primary-50 px-3 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-200"
                    >
                      {{ item.name || `#${item.id}` }} · #{{ item.id }}
                      <button type="button" class="text-primary-500 hover:text-primary-700 dark:hover:text-primary-100" :disabled="submitting || !canManage" @click="removeApiKey(item.id)">
                        ×
                      </button>
                    </span>
                  </div>
                  <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.clearPage.fields.apiKeysHint') }}</p>
                </div>
              </div>

              <div v-if="form.clearType === 'by_time'" class="grid grid-cols-1 gap-4 md:grid-cols-2">
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.clearPage.fields.startTime') }}</span>
                  <input v-model="form.startTime" type="datetime-local" class="input" :disabled="submitting || !canManage" />
                </label>
                <label class="block">
                  <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.clearPage.fields.endTime') }}</span>
                  <input v-model="form.endTime" type="datetime-local" class="input" :disabled="submitting || !canManage" />
                </label>
              </div>

              <div
                v-if="form.clearType === 'all'"
                class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200"
              >
                {{ t('admin.cacheManagement.clearPage.allHint') }}
              </div>

              <div
                v-if="form.clearType === 'expired'"
                class="rounded-xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-700 dark:border-blue-900/60 dark:bg-blue-900/10 dark:text-blue-200"
              >
                {{ t('admin.cacheManagement.clearPage.expiredHint') }}
              </div>
            </div>
          </div>
        </div>

        <div class="space-y-6">
          <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.cacheManagement.clearPage.previewTitle') }}
              </h2>
            </div>
            <div class="space-y-4 px-6 py-5 text-sm text-gray-600 dark:text-gray-300">
              <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.clearPage.previewType') }}</p>
                <p class="mt-1 font-semibold text-gray-900 dark:text-white">{{ currentTypeLabel }}</p>
              </div>
              <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.clearPage.previewScope') }}</p>
                <ul class="mt-2 space-y-2">
                  <li v-for="item in scopeSummary" :key="item.label" class="flex items-start justify-between gap-3">
                    <span class="text-gray-500 dark:text-gray-400">{{ item.label }}</span>
                    <span class="text-right font-medium text-gray-900 dark:text-white">{{ item.value }}</span>
                  </li>
                </ul>
              </div>
              <div class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/10 dark:text-amber-200">
                {{ t('admin.cacheManagement.clearPage.previewHint') }}
              </div>
            </div>
          </div>

          <div
            v-if="lastResult"
            class="rounded-xl border bg-white shadow-sm dark:bg-dark-800"
            :class="lastResult.status === 'success'
              ? 'border-emerald-200 dark:border-emerald-900/50'
              : lastResult.status === 'partial_success'
                ? 'border-amber-200 dark:border-amber-900/50'
                : 'border-red-200 dark:border-red-900/50'"
          >
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.cacheManagement.clearPage.resultTitle') }}
              </h2>
            </div>
            <div class="space-y-4 px-6 py-5 text-sm text-gray-600 dark:text-gray-300">
              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.clearPage.resultMatched') }}</p>
                  <p class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ formatInteger(lastResult.matched_keys) }}</p>
                </div>
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.clearPage.resultDeleted') }}</p>
                  <p class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ formatInteger(lastResult.deleted_keys) }}</p>
                </div>
              </div>
              <div class="rounded-lg px-4 py-3 text-sm"
                :class="lastResult.status === 'success'
                  ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/10 dark:text-emerald-200'
                  : lastResult.status === 'partial_success'
                    ? 'bg-amber-50 text-amber-800 dark:bg-amber-900/10 dark:text-amber-200'
                    : 'bg-red-50 text-red-700 dark:bg-red-900/10 dark:text-red-200'"
              >
                {{ formatResultMessage(lastResult) }}
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.cacheManagement.clearPage.auditTitle') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.cacheManagement.clearPage.auditHint') }}
              </p>
            </div>
            <button type="button" class="btn btn-secondary" :disabled="auditLoading" @click="loadAudits(true)">
              {{ auditLoading ? t('admin.cacheManagement.clearPage.auditRefreshing') : t('admin.cacheManagement.clearPage.auditRefresh') }}
            </button>
          </div>
        </div>

        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-5">
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.clearPage.auditFields.operator') }}</span>
              <input v-model.trim="auditFilters.operatorUserId" type="number" min="1" step="1" class="input" :placeholder="t('admin.cacheManagement.clearPage.auditPlaceholders.operator')" />
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.clearPage.auditFields.clearType') }}</span>
              <select v-model="auditFilters.clearType" class="input">
                <option value="">{{ t('common.all') }}</option>
                <option v-for="option in clearTypeOptions" :key="`audit-${option.value}`" :value="option.value">{{ option.label }}</option>
              </select>
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.clearPage.auditFields.status') }}</span>
              <select v-model="auditFilters.status" class="input">
                <option value="">{{ t('common.all') }}</option>
                <option v-for="option in auditStatusOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
              </select>
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.clearPage.auditFields.startTime') }}</span>
              <input v-model="auditFilters.startTime" type="datetime-local" class="input" />
            </label>
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.clearPage.auditFields.endTime') }}</span>
              <input v-model="auditFilters.endTime" type="datetime-local" class="input" />
            </label>
          </div>

          <div class="mt-4 flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-primary" :disabled="auditLoading || Boolean(auditValidationError)" @click="applyAuditFilters">
              {{ t('common.search') }}
            </button>
            <button type="button" class="btn btn-secondary" :disabled="auditLoading" @click="resetAuditFilters">
              {{ t('common.reset') }}
            </button>
          </div>

          <p v-if="auditValidationError" class="mt-3 text-sm text-red-600 dark:text-red-300">
            {{ auditValidationError }}
          </p>
        </div>

        <div class="px-6 py-5">
          <div v-if="auditError" class="mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/10 dark:text-red-200">
            <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <span>{{ auditError }}</span>
              <button type="button" class="btn btn-secondary" :disabled="auditLoading" @click="loadAudits(true)">
                {{ t('admin.cacheManagement.retry') }}
              </button>
            </div>
          </div>

          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-900/40">
                <tr>
                  <th v-for="column in auditColumns" :key="column.key" class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
                    {{ column.label }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-if="auditLoading">
                  <td :colspan="auditColumns.length" class="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
                    {{ t('common.loading') }}
                  </td>
                </tr>
                <tr v-else-if="auditRows.length === 0">
                  <td :colspan="auditColumns.length" class="px-4 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.cacheManagement.clearPage.auditEmpty') }}
                  </td>
                </tr>
                <tr v-for="row in auditRows" :key="row.id" class="align-top">
                  <td class="px-4 py-3 text-sm text-gray-900 dark:text-white">#{{ row.id }}</td>
                  <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">{{ formatOperator(row.operator_user_id) }}</td>
                  <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">{{ formatDateTimeValue(row.created_at) || '--' }}</td>
                  <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">{{ formatClearType(row.clear_type) }}</td>
                  <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                    <div class="max-w-[320px] whitespace-pre-wrap break-words">{{ formatAuditScope(row) }}</div>
                  </td>
                  <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                    <div>{{ formatInteger(row.deleted_keys) }}</div>
                    <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.cacheManagement.clearPage.auditMatchedShort') }} {{ formatInteger(row.matched_keys) }}</div>
                  </td>
                  <td class="px-4 py-3 text-sm">
                    <span class="inline-flex rounded-full px-2.5 py-1 text-xs font-medium" :class="statusClass(row.status)">
                      {{ formatStatus(row.status) }}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-200">
                    <span v-if="row.error_message">{{ row.error_message }}</span>
                    <span v-else class="text-gray-400 dark:text-gray-500">--</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <Pagination
          v-if="auditPagination.total > 0"
          :page="auditPagination.page"
          :total="auditPagination.total"
          :page-size="auditPagination.page_size"
          @update:page="handleAuditPageChange"
          @update:pageSize="handleAuditPageSizeChange"
        />
      </div>
    </div>

    <BaseDialog :show="confirmDialogVisible" width="narrow" :title="t('admin.cacheManagement.clearPage.confirmTitle')" @close="closeConfirmDialog">
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('admin.cacheManagement.clearPage.confirmDescription') }}</p>
        <div class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/30">
          <ul class="space-y-2 text-sm text-gray-600 dark:text-gray-300">
            <li v-for="item in scopeSummary" :key="item.label" class="flex items-start justify-between gap-3">
              <span class="text-gray-500 dark:text-gray-400">{{ item.label }}</span>
              <span class="text-right font-medium text-gray-900 dark:text-white">{{ item.value }}</span>
            </li>
          </ul>
        </div>
        <label v-if="requiresConfirmText" class="block">
          <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.cacheManagement.clearPage.confirmInputLabel') }}</span>
          <input v-model.trim="confirmText" type="text" class="input" :placeholder="t('admin.cacheManagement.clearPage.confirmInputPlaceholder')" />
        </label>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="submitting" @click="closeConfirmDialog">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-danger" :disabled="!confirmReady || submitting" @click="submitClear">
            {{ submitting ? t('admin.cacheManagement.clearPage.submitting') : t('admin.cacheManagement.clearPage.confirmSubmit') }}
          </button>
        </div>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import Pagination from '@/components/common/Pagination.vue'
import { adminAPI } from '@/api/admin'
import type { SimpleApiKey } from '@/api/admin/usage'
import type { AdminGroup } from '@/types'
import type { CacheClearAuditFilter, CacheClearAuditRecord, CacheClearRequest, CacheClearResult, CacheClearType } from '@/api/admin/cache'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime as formatDateTimeValue } from '@/utils/format'

type ClearTypeForm = {
  clearType: CacheClearType
  platforms: string[]
  models: string[]
  groupIds: number[]
  apiKeys: SimpleApiKey[]
  startTime: string
  endTime: string
}

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const groups = ref<AdminGroup[]>([])
const submitting = ref(false)
const confirmDialogVisible = ref(false)
const confirmText = ref('')
const modelKeyword = ref('')
const apiKeyKeyword = ref('')
const apiKeyResults = ref<SimpleApiKey[]>([])
const apiKeySearchTried = ref(false)
const lastResult = ref<CacheClearResult | null>(null)
const auditLoading = ref(false)
const auditError = ref('')
const auditRows = ref<CacheClearAuditRecord[]>([])
let apiKeySearchTimeout: ReturnType<typeof setTimeout> | null = null

const form = reactive<ClearTypeForm>({
  clearType: 'expired',
  platforms: [],
  models: [],
  groupIds: [],
  apiKeys: [],
  startTime: '',
  endTime: ''
})

const viewerRole = computed(() => String((authStore.user as { role?: string } | null)?.role || '').trim().toLowerCase())
const canManage = computed(() => viewerRole.value === '' || viewerRole.value === 'admin')
const requiresPlatforms = computed(() => form.clearType === 'by_platform' || form.clearType === 'by_model')
const requiresConfirmText = computed(() => form.clearType === 'all')
const auditPagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
})
const auditFilters = reactive({
  operatorUserId: '',
  clearType: '',
  status: '',
  startTime: '',
  endTime: ''
})

const navItems = computed(() => [
  { key: 'home', to: '/admin/settings/cache', label: t('admin.cacheManagement.nav.home'), active: false },
  { key: 'config', to: null, label: t('admin.cacheManagement.nav.config'), active: false },
  { key: 'stats', to: null, label: t('admin.cacheManagement.nav.stats'), active: false },
  { key: 'clear', to: '/admin/settings/cache/clear', label: t('admin.cacheManagement.nav.clear'), active: true },
  { key: 'advanced', to: null, label: t('admin.cacheManagement.nav.advanced'), active: false },
  { key: 'semantic', to: null, label: t('admin.cacheManagement.nav.semantic'), active: false }
])

const clearTypeOptions = computed(() => [
  { value: 'expired', label: t('admin.cacheManagement.clearPage.types.expired'), hint: t('admin.cacheManagement.clearPage.typeHints.expired') },
  { value: 'by_platform', label: t('admin.cacheManagement.clearPage.types.byPlatform'), hint: t('admin.cacheManagement.clearPage.typeHints.byPlatform') },
  { value: 'by_model', label: t('admin.cacheManagement.clearPage.types.byModel'), hint: t('admin.cacheManagement.clearPage.typeHints.byModel') },
  { value: 'by_group', label: t('admin.cacheManagement.clearPage.types.byGroup'), hint: t('admin.cacheManagement.clearPage.typeHints.byGroup') },
  { value: 'by_api_key', label: t('admin.cacheManagement.clearPage.types.byApiKey'), hint: t('admin.cacheManagement.clearPage.typeHints.byApiKey') },
  { value: 'by_time', label: t('admin.cacheManagement.clearPage.types.byTime'), hint: t('admin.cacheManagement.clearPage.typeHints.byTime') },
  { value: 'all', label: t('admin.cacheManagement.clearPage.types.all'), hint: t('admin.cacheManagement.clearPage.typeHints.all') }
])
const auditStatusOptions = computed(() => [
  { value: 'success', label: t('admin.cacheManagement.clearPage.auditStatuses.success') },
  { value: 'partial_success', label: t('admin.cacheManagement.clearPage.auditStatuses.partialSuccess') },
  { value: 'failed', label: t('admin.cacheManagement.clearPage.auditStatuses.failed') }
])

const platformOptions = computed(() => [
  { value: 'openai', label: 'OpenAI' },
  { value: 'claude', label: 'Claude' },
  { value: 'gemini', label: 'Gemini' }
])

const currentTypeLabel = computed(() => {
  return clearTypeOptions.value.find((item) => item.value === form.clearType)?.label ?? form.clearType
})

const scopeSummary = computed(() => {
  const summary: Array<{ label: string; value: string }> = [
    { label: t('admin.cacheManagement.clearPage.previewType'), value: currentTypeLabel.value }
  ]

  if (requiresPlatforms.value) {
    summary.push({
      label: t('admin.cacheManagement.clearPage.fields.platforms'),
      value: form.platforms.length > 0 ? form.platforms.join(', ') : t('common.notConfigured')
    })
  }
  if (form.clearType === 'by_model') {
    summary.push({
      label: t('admin.cacheManagement.clearPage.fields.models'),
      value: form.models.length > 0 ? form.models.join(', ') : t('common.notConfigured')
    })
  }
  if (form.clearType === 'by_group') {
    const names = groups.value
      .filter((group) => form.groupIds.includes(group.id))
      .map((group) => group.name)
    summary.push({
      label: t('admin.cacheManagement.clearPage.fields.groups'),
      value: names.length > 0 ? names.join(', ') : t('common.notConfigured')
    })
  }
  if (form.clearType === 'by_api_key') {
    summary.push({
      label: t('admin.cacheManagement.clearPage.fields.apiKeys'),
      value: form.apiKeys.length > 0
        ? form.apiKeys.map((item) => `${item.name || `#${item.id}`} · #${item.id}`).join(', ')
        : t('common.notConfigured')
    })
  }
  if (form.clearType === 'by_time') {
    summary.push(
      { label: t('admin.cacheManagement.clearPage.fields.startTime'), value: form.startTime || t('common.notConfigured') },
      { label: t('admin.cacheManagement.clearPage.fields.endTime'), value: form.endTime || t('common.notConfigured') }
    )
  }

  return summary
})

const validationErrors = computed(() => {
  const errors: string[] = []

  switch (form.clearType) {
    case 'by_platform':
      if (form.platforms.length === 0) {
        errors.push(t('admin.cacheManagement.clearPage.validation.platforms'))
      }
      break
    case 'by_model':
      if (form.platforms.length === 0) {
        errors.push(t('admin.cacheManagement.clearPage.validation.platforms'))
      }
      if (form.models.length === 0) {
        errors.push(t('admin.cacheManagement.clearPage.validation.models'))
      }
      break
    case 'by_group':
      if (form.groupIds.length === 0) {
        errors.push(t('admin.cacheManagement.clearPage.validation.groups'))
      }
      break
    case 'by_api_key':
      if (form.apiKeys.length === 0) {
        errors.push(t('admin.cacheManagement.clearPage.validation.apiKeys'))
      }
      break
    case 'by_time': {
      if (!form.startTime || !form.endTime) {
        errors.push(t('admin.cacheManagement.clearPage.validation.timeRequired'))
        break
      }
      const start = new Date(form.startTime)
      const end = new Date(form.endTime)
      if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) {
        errors.push(t('admin.cacheManagement.clearPage.validation.timeInvalid'))
        break
      }
      if (start.getTime() > end.getTime()) {
        errors.push(t('admin.cacheManagement.clearPage.validation.timeOrder'))
      }
      if (end.getTime() - start.getTime() > 31 * 24 * 60 * 60 * 1000) {
        errors.push(t('admin.cacheManagement.clearPage.validation.timeRange'))
      }
      break
    }
  }

  return errors
})

const canSubmit = computed(() => canManage.value && validationErrors.value.length === 0)
const confirmReady = computed(() => !requiresConfirmText.value || confirmText.value === '确认清理')
const auditColumns = computed(() => [
  { key: 'id', label: t('admin.cacheManagement.clearPage.auditColumns.id') },
  { key: 'operator', label: t('admin.cacheManagement.clearPage.auditColumns.operator') },
  { key: 'createdAt', label: t('admin.cacheManagement.clearPage.auditColumns.createdAt') },
  { key: 'clearType', label: t('admin.cacheManagement.clearPage.auditColumns.clearType') },
  { key: 'scope', label: t('admin.cacheManagement.clearPage.auditColumns.scope') },
  { key: 'result', label: t('admin.cacheManagement.clearPage.auditColumns.result') },
  { key: 'status', label: t('admin.cacheManagement.clearPage.auditColumns.status') },
  { key: 'error', label: t('admin.cacheManagement.clearPage.auditColumns.error') }
])
const auditValidationError = computed(() => {
  if (!auditFilters.startTime && !auditFilters.endTime) return ''
  if (!auditFilters.startTime || !auditFilters.endTime) {
    return t('admin.cacheManagement.clearPage.auditValidation.timeRequired')
  }
  const start = new Date(auditFilters.startTime)
  const end = new Date(auditFilters.endTime)
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) {
    return t('admin.cacheManagement.clearPage.auditValidation.timeInvalid')
  }
  if (start.getTime() > end.getTime()) {
    return t('admin.cacheManagement.clearPage.auditValidation.timeOrder')
  }
  if (end.getTime() - start.getTime() > 31 * 24 * 60 * 60 * 1000) {
    return t('admin.cacheManagement.clearPage.auditValidation.timeRange')
  }
  return ''
})

function resetForm(): void {
  form.clearType = 'expired'
  form.platforms = []
  form.models = []
  form.groupIds = []
  form.apiKeys = []
  form.startTime = ''
  form.endTime = ''
  modelKeyword.value = ''
  apiKeyKeyword.value = ''
  apiKeyResults.value = []
  apiKeySearchTried.value = false
  confirmText.value = ''
}

function togglePlatform(platform: string, checked: boolean): void {
  if (checked) {
    if (!form.platforms.includes(platform)) {
      form.platforms = [...form.platforms, platform]
    }
    return
  }
  form.platforms = form.platforms.filter((item) => item !== platform)
}

function addModel(): void {
  const value = modelKeyword.value.trim()
  if (!value) return
  const exists = form.models.some((item) => item.toLowerCase() === value.toLowerCase())
  if (!exists) {
    form.models = [...form.models, value]
  }
  modelKeyword.value = ''
}

function removeModel(model: string): void {
  form.models = form.models.filter((item) => item !== model)
}

function selectApiKey(item: SimpleApiKey): void {
  const exists = form.apiKeys.some((current) => current.id === item.id)
  if (!exists) {
    form.apiKeys = [...form.apiKeys, item]
  }
  apiKeyKeyword.value = ''
  apiKeyResults.value = []
}

function removeApiKey(id: number): void {
  form.apiKeys = form.apiKeys.filter((item) => item.id !== id)
}

function debounceApiKeySearch(immediate = false): void {
  apiKeySearchTried.value = true
  if (apiKeySearchTimeout) clearTimeout(apiKeySearchTimeout)
  const run = async () => {
    try {
      apiKeyResults.value = await adminAPI.usage.searchApiKeys(undefined, apiKeyKeyword.value || '')
    } catch {
      apiKeyResults.value = []
    }
  }
  if (immediate) {
    void run()
    return
  }
  apiKeySearchTimeout = setTimeout(() => {
    void run()
  }, 300)
}

function toISOString(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
}

function buildPayload(): CacheClearRequest {
  return {
    clear_type: form.clearType,
    scope: {
      platforms: form.clearType === 'by_platform' || form.clearType === 'by_model' ? [...form.platforms] : undefined,
      models: form.clearType === 'by_model' ? [...form.models] : undefined,
      group_ids: form.clearType === 'by_group' ? [...form.groupIds] : undefined,
      api_key_ids: form.clearType === 'by_api_key' ? form.apiKeys.map((item) => item.id) : undefined,
      start_time: form.clearType === 'by_time' ? toISOString(form.startTime) : undefined,
      end_time: form.clearType === 'by_time' ? toISOString(form.endTime) : undefined
    },
    confirm_text: requiresConfirmText.value ? confirmText.value : undefined
  }
}

watch(
  () => form.clearType,
  (nextType) => {
    confirmText.value = ''
    apiKeyResults.value = []
    apiKeySearchTried.value = false
    apiKeyKeyword.value = ''
    modelKeyword.value = ''

    if (nextType !== 'by_platform' && nextType !== 'by_model') {
      form.platforms = []
    }
    if (nextType !== 'by_model') {
      form.models = []
    }
    if (nextType !== 'by_group') {
      form.groupIds = []
    }
    if (nextType !== 'by_api_key') {
      form.apiKeys = []
    }
    if (nextType !== 'by_time') {
      form.startTime = ''
      form.endTime = ''
    }
  }
)

function openConfirmDialog(): void {
  if (!canSubmit.value) {
    appStore.showError(validationErrors.value[0] || t('admin.cacheManagement.clearPage.validation.default'))
    return
  }
  confirmDialogVisible.value = true
}

function closeConfirmDialog(): void {
  if (submitting.value) return
  confirmDialogVisible.value = false
  confirmText.value = ''
}

function formatInteger(value: number): string {
  return new Intl.NumberFormat('zh-CN').format(value || 0)
}

function formatResultMessage(result: CacheClearResult): string {
  if (result.status === 'success') {
    return t('admin.cacheManagement.clearPage.resultSuccess', {
      deleted: formatInteger(result.deleted_keys),
      matched: formatInteger(result.matched_keys)
    })
  }
  if (result.status === 'partial_success') {
    return t('admin.cacheManagement.clearPage.resultPartial', {
      deleted: formatInteger(result.deleted_keys),
      matched: formatInteger(result.matched_keys),
      reason: result.error_message || '-'
    })
  }
  return t('admin.cacheManagement.clearPage.resultFailed', {
    reason: result.error_message || '-'
  })
}

function buildAuditParams(): CacheClearAuditFilter {
  const params: CacheClearAuditFilter = {
    page: auditPagination.page,
    page_size: auditPagination.page_size
  }

  const operatorId = Number(auditFilters.operatorUserId)
  if (Number.isFinite(operatorId) && operatorId > 0) {
    params.operator_user_id = operatorId
  }
  if (auditFilters.clearType) {
    params.clear_type = auditFilters.clearType as CacheClearType
  }
  if (auditFilters.status) {
    params.status = auditFilters.status as CacheClearResult['status']
  }
  if (auditFilters.startTime) {
    params.start_time = toISOString(auditFilters.startTime)
  }
  if (auditFilters.endTime) {
    params.end_time = toISOString(auditFilters.endTime)
  }

  return params
}

function formatClearType(type: CacheClearType): string {
  return clearTypeOptions.value.find((item) => item.value === type)?.label ?? type
}

function formatStatus(status: CacheClearResult['status']): string {
  return auditStatusOptions.value.find((item) => item.value === status)?.label ?? status
}

function formatOperator(operatorUserId?: number | null): string {
  if (typeof operatorUserId === 'number' && operatorUserId > 0) {
    return `#${operatorUserId}`
  }
  return '--'
}

function formatAuditScope(row: CacheClearAuditRecord): string {
  const lines: string[] = []
  if (row.scope.platforms?.length) {
    lines.push(`${t('admin.cacheManagement.clearPage.fields.platforms')}: ${row.scope.platforms.join(', ')}`)
  }
  if (row.scope.models?.length) {
    lines.push(`${t('admin.cacheManagement.clearPage.fields.models')}: ${row.scope.models.join(', ')}`)
  }
  if (row.scope.group_ids?.length) {
    lines.push(`${t('admin.cacheManagement.clearPage.fields.groups')}: ${row.scope.group_ids.join(', ')}`)
  }
  if (row.scope.api_key_ids?.length) {
    lines.push(`${t('admin.cacheManagement.clearPage.fields.apiKeys')}: ${row.scope.api_key_ids.join(', ')}`)
  }
  if (row.scope.start_time || row.scope.end_time) {
    lines.push(`${t('admin.cacheManagement.clearPage.auditFields.timeRange')}: ${formatDateTimeValue(row.scope.start_time) || '--'} ~ ${formatDateTimeValue(row.scope.end_time) || '--'}`)
  }
  return lines.length > 0 ? lines.join('\n') : t('admin.cacheManagement.clearPage.auditScopeAll')
}

function statusClass(status: CacheClearResult['status']): string {
  if (status === 'success') {
    return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/10 dark:text-emerald-200'
  }
  if (status === 'partial_success') {
    return 'bg-amber-50 text-amber-800 dark:bg-amber-900/10 dark:text-amber-200'
  }
  return 'bg-red-50 text-red-700 dark:bg-red-900/10 dark:text-red-200'
}

async function loadGroups(): Promise<void> {
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.cacheManagement.clearPage.groupsLoadFailed')))
  }
}

async function submitClear(): Promise<void> {
  if (!canSubmit.value || !confirmReady.value) {
    return
  }

  submitting.value = true
  try {
    const { data } = await adminAPI.cache.clearLocalResponseCache(buildPayload())
    lastResult.value = data
    appStore.showSuccess(
      data.status === 'success'
        ? t('admin.cacheManagement.clearPage.submitSuccess')
        : t('admin.cacheManagement.clearPage.submitPartial')
    )
    await loadAudits(true)
    confirmDialogVisible.value = false
    confirmText.value = ''
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.cacheManagement.clearPage.submitFailed')))
  } finally {
    submitting.value = false
  }
}

async function loadAudits(resetPage = false): Promise<void> {
  if (auditValidationError.value) {
    auditError.value = auditValidationError.value
    return
  }

  if (resetPage) {
    auditPagination.page = 1
  }

  auditLoading.value = true
  auditError.value = ''
  try {
    const { data } = await adminAPI.cache.listClearAudits(buildAuditParams())
    auditRows.value = Array.isArray(data.items) ? data.items : []
    auditPagination.total = Number(data.total || 0)
    auditPagination.page = Number(data.page || auditPagination.page || 1)
    auditPagination.page_size = Number(data.page_size || auditPagination.page_size || 20)
    auditPagination.pages = 'pages' in data && typeof data.pages === 'number'
      ? data.pages
      : (auditPagination.total > 0 ? Math.ceil(auditPagination.total / auditPagination.page_size) : 0)
  } catch (error) {
    auditError.value = extractApiErrorMessage(error, t('admin.cacheManagement.clearPage.auditLoadFailed'))
  } finally {
    auditLoading.value = false
  }
}

function applyAuditFilters(): void {
  if (auditValidationError.value) {
    auditError.value = auditValidationError.value
    return
  }
  void loadAudits(true)
}

function resetAuditFilters(): void {
  auditFilters.operatorUserId = ''
  auditFilters.clearType = ''
  auditFilters.status = ''
  auditFilters.startTime = ''
  auditFilters.endTime = ''
  auditError.value = ''
  auditPagination.page = 1
  void loadAudits(true)
}

function handleAuditPageChange(page: number): void {
  auditPagination.page = page
  void loadAudits()
}

function handleAuditPageSizeChange(pageSize: number): void {
  auditPagination.page = 1
  auditPagination.page_size = pageSize
  void loadAudits()
}

onMounted(() => {
  void loadGroups()
  void loadAudits()
})
</script>
