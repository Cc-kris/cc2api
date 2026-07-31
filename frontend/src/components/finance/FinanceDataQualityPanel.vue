<template>
  <div class="space-y-4">
    <div v-if="coverageRisk" class="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200" data-testid="quality-risk">
      成本覆盖率低于 99%，当前只展示已覆盖范围毛利，不能作为全站净利润。
    </div>
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-6">
      <article v-for="item in metrics" :key="item.label" class="card p-4">
        <p class="text-sm text-gray-500">{{ item.label }}</p>
        <p class="mt-2 text-xl font-semibold tabular-nums" :class="item.tone">{{ item.value }}</p>
        <p class="mt-2 text-xs text-gray-500">{{ item.hint }}</p>
      </article>
    </div>
    <section class="card overflow-hidden">
      <div class="border-b border-gray-200 p-4 dark:border-dark-700">
        <h2 class="font-semibold text-gray-900 dark:text-white">数据质量问题</h2>
        <p class="mt-1 text-xs text-gray-500">缺少钱包、价格、倍率、用量或计费支持的记录不进入已确认利润。</p>
      </div>
      <div class="overflow-x-auto" role="region" aria-label="数据质量问题明细" tabindex="0">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-3">用量日志</th><th class="px-4 py-3">问题类型</th><th class="px-4 py-3">关联对象</th><th class="px-4 py-3 text-right">受影响收入</th><th class="px-4 py-3">首次发现</th><th class="px-4 py-3">可重算</th></tr></thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="item in issues" :key="`${item.usage_log_id}-${item.issue_type}`"><td class="px-4 py-3">#{{ item.usage_log_id }}</td><td class="px-4 py-3 font-medium text-gray-900 dark:text-white">{{ issueLabel(item.issue_type) }}</td><td class="px-4 py-3">{{ item.related_type }}{{ item.related_id ? ` #${item.related_id}` : '' }}</td><td class="px-4 py-3 text-right tabular-nums">{{ formatFinanceMoney(item.exposed_revenue) }}</td><td class="px-4 py-3 text-gray-500">{{ formatFinanceDate(item.first_detected_at) }}</td><td class="px-4 py-3">{{ item.recalculable ? '是' : '否' }}</td></tr>
            <tr v-if="issues.length === 0"><td colspan="6" class="px-4 py-10 text-center text-gray-500">所选期间未发现数据质量问题</td></tr>
          </tbody>
        </table>
      </div>
    </section>
    <section class="card overflow-hidden" data-testid="promotion-reconciliation-panel">
      <div class="border-b border-gray-200 p-4 dark:border-dark-700">
        <h2 class="font-semibold text-gray-900 dark:text-white">历史优惠额度待核对</h2>
        <p class="mt-1 text-xs text-gray-500">切换财务口径前产生的优惠额度无法从旧记录还原剩余值。确认后系统会更正可用优惠余额并写入操作审计。</p>
      </div>
      <p v-if="promotionError" class="m-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ promotionError }}</p>
      <div class="overflow-x-auto" role="region" aria-label="历史优惠额度待核对明细" tabindex="0">
        <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-800"><tr><th class="px-4 py-3">客户</th><th class="px-4 py-3 text-right">历史发放</th><th class="px-4 py-3 text-right">系统当前余额</th><th class="px-4 py-3">确认剩余额度</th><th class="px-4 py-3">核对说明</th><th class="px-4 py-3">操作</th></tr></thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr v-for="item in promotionItems" :key="item.user_id">
              <td class="px-4 py-3"><p class="font-medium text-gray-900 dark:text-white">{{ item.username || item.user_email }}</p><p class="text-xs text-gray-500">{{ item.user_email }} · #{{ item.user_id }}</p></td>
              <td class="px-4 py-3 text-right tabular-nums">{{ formatFinanceMoney(item.detected_historical_bonus) }}</td>
              <td class="px-4 py-3 text-right tabular-nums">{{ formatFinanceMoney(item.current_remaining_amount) }}</td>
              <td class="px-4 py-3"><input v-model="promotionDrafts[item.user_id].amount" class="input w-36" type="number" min="0" step="0.0000000001" aria-label="确认剩余额度" placeholder="输入核对值" /></td>
              <td class="px-4 py-3"><input v-model="promotionDrafts[item.user_id].note" class="input min-w-56" maxlength="2000" aria-label="核对说明" placeholder="填写核对依据" /></td>
              <td class="px-4 py-3"><button class="btn btn-primary whitespace-nowrap" :disabled="resolvingUserID === item.user_id || !hasPromotionAmount(item.user_id) || !promotionDrafts[item.user_id].note.trim()" @click="resolvePromotion(item.user_id)">{{ resolvingUserID === item.user_id ? '处理中' : '确认并入账' }}</button></td>
            </tr>
            <tr v-if="!promotionLoading && promotionItems.length === 0"><td colspan="6" class="px-4 py-10 text-center text-gray-500">没有待核对的历史优惠额度</td></tr>
            <tr v-if="promotionLoading"><td colspan="6" class="px-4 py-10 text-center text-gray-500">正在加载待核对记录</td></tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type { FinanceDataQuality, PromotionCreditReconciliation } from '@/api/admin/finance'
import { financeNumber, formatFinanceDate, formatFinanceMoney, formatFinancePercent } from './format'

const props = defineProps<{ data: FinanceDataQuality }>()
const promotionItems = ref<PromotionCreditReconciliation[]>([])
const promotionTotal = ref(0)
const promotionLoading = ref(false)
const resolvingUserID = ref<number | null>(null)
const promotionError = ref('')
const promotionDrafts = reactive<Record<number, { amount: string | number; note: string }>>({})
const issues = computed(() => props.data.items || [])
const coverageRisk = computed(() => {
  const rate = financeNumber(props.data.quality.cost_coverage_rate)
  return rate === null || rate < 0.99
})
const metrics = computed(() => [
  { label: '成本覆盖率', value: formatFinancePercent(props.data.quality.cost_coverage_rate), tone: coverageRisk.value ? 'text-red-600 dark:text-red-400' : 'text-emerald-600 dark:text-emerald-400', hint: '低于 99% 标记财务风险' },
  { label: '精确成本记录', value: String(props.data.quality.exact_count), tone: 'text-gray-900 dark:text-white', hint: '进入已确认毛利' },
  { label: '估算成本记录', value: String(props.data.quality.estimated_count), tone: 'text-amber-800 dark:text-amber-300', hint: '单独披露，不混入确认值' },
  { label: '缺少成本信息', value: String((props.data.quality.missing_profile_count ?? 0) + props.data.quality.missing_price_count + props.data.quality.missing_multiplier_count + props.data.quality.missing_usage_count + (props.data.quality.unsupported_usage_count ?? 0)), tone: 'text-red-600 dark:text-red-400', hint: '钱包、价格、倍率、用量或计费支持缺失' },
  { label: '未定价收入', value: formatFinanceMoney(props.data.quality.unpriced_revenue), tone: 'text-amber-800 dark:text-amber-300', hint: '利润暂时无法计算' },
  { label: '历史优惠待核对', value: String(promotionTotal.value), tone: promotionTotal.value > 0 ? 'text-amber-800 dark:text-amber-300' : 'text-emerald-600 dark:text-emerald-400', hint: '确认后更正优惠余额并留审计' }
])
const labels: Record<string, string> = { missing_profile: '缺少结算钱包', missing_price: '缺少上游价格', missing_multiplier: '缺少历史上游倍率', missing_usage: '缺少计费用量', unsupported_usage: '不支持的用量类型' }
function issueLabel(type: string) { return labels[type] || type || '未知问题' }
function hasPromotionAmount(userID: number) { return String(promotionDrafts[userID]?.amount ?? '').trim() !== '' }

async function loadPromotionReconciliations() {
  promotionLoading.value = true
  promotionError.value = ''
  try {
    const result = await adminAPI.finance.getPromotionCreditReconciliations({ status: 'requires_reconciliation', page: 1, page_size: 100 })
    promotionItems.value = result.items || []
    promotionTotal.value = result.total
    for (const item of promotionItems.value) {
      promotionDrafts[item.user_id] ||= { amount: '', note: '' }
    }
  } catch (error) {
    promotionError.value = error instanceof Error ? error.message : '历史优惠额度加载失败'
  } finally {
    promotionLoading.value = false
  }
}

async function resolvePromotion(userID: number) {
  const draft = promotionDrafts[userID]
  if (!draft) return
  resolvingUserID.value = userID
  promotionError.value = ''
  try {
    await adminAPI.finance.resolvePromotionCreditReconciliation(userID, { confirmed_remaining_amount: String(draft.amount), note: draft.note.trim() })
    await loadPromotionReconciliations()
  } catch (error) {
    promotionError.value = error instanceof Error ? error.message : '历史优惠额度确认失败'
  } finally {
    resolvingUserID.value = null
  }
}

onMounted(loadPromotionReconciliations)
</script>
