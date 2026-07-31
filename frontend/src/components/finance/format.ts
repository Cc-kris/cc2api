export const FINANCE_UNAVAILABLE = '无法计算'

export function financeNumber(value: string | number | null | undefined): number | null {
  if (value === null || value === undefined || value === '') return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

export function formatFinanceMoney(value: string | number | null | undefined, currency = 'USD'): string {
  const parsed = financeNumber(value)
  if (parsed === null) return FINANCE_UNAVAILABLE
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency,
    minimumFractionDigits: Math.abs(parsed) > 0 && Math.abs(parsed) < 0.01 ? 6 : 2,
    maximumFractionDigits: Math.abs(parsed) > 0 && Math.abs(parsed) < 0.01 ? 6 : 2
  }).format(parsed)
}

export function formatFinancePercent(value: string | number | null | undefined): string {
  const parsed = financeNumber(value)
  if (parsed === null) return FINANCE_UNAVAILABLE
  return `${(parsed * 100).toFixed(2)}%`
}

export function formatFinanceDate(value: string | null | undefined): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString('zh-CN', { hour12: false })
}

export function financeTone(value: string | number | null | undefined): string {
  const parsed = financeNumber(value)
  if (parsed === null) return 'text-gray-500 dark:text-gray-400'
  if (parsed < 0) return 'text-red-700 dark:text-red-300'
  if (parsed > 0) return 'text-emerald-700 dark:text-emerald-300'
  return 'text-gray-900 dark:text-white'
}
