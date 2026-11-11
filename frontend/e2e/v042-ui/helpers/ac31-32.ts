import type { Page, TestInfo } from '@playwright/test'

/**
 * AC31/AC32 私有 helper：只被 specs/ac31-32.spec.ts 使用。
 * 不改动共享 helpers / mock / 产品代码。
 */

export type Ac31Locale = 'zh-CN' | 'en-US'

export function localeFor(testInfo: TestInfo): Ac31Locale {
  return testInfo.project.name === 'mobile-en' ? 'en-US' : 'zh-CN'
}

export interface Ac31Labels {
  columnSettings: RegExp
  accountSearch: string
  editButton: string
  cancelButton: RegExp
  model: string
  group: string
  upstreamRequestId: string
  sessionId: string
  upstreamRequestIdPlaceholder: string
  mismatchOnly: string
  mismatchMatched: string
  mismatchBadge: string
  emptyTable: string
  exportButton: string
  /** 导出列头：上游响应模型 */
  upstreamResponseModelHeader: string
  /** 导出列头：会话 ID */
  sessionIdHeader: string
  /** 导出列头：模型一致性 */
  mismatchHeader: string
  /** 导出列头：请求推理强度（映射前） */
  requestedEffortHeader: string
  yes: string
  no: string
  upstreamRequestIdHeaderInvalid: string
}

/** 使用产品 i18n 的实际文案定位界面元素。 */
export function ac31Labels(locale: Ac31Locale): Ac31Labels {
  if (locale === 'en-US') {
    return {
      columnSettings: /Column Settings/i,
      accountSearch: 'Search accounts...',
      editButton: 'Edit',
      cancelButton: /^Cancel$/,
      model: 'Model',
      group: 'Group',
      upstreamRequestId: 'Upstream Request ID',
      sessionId: 'Session ID',
      upstreamRequestIdPlaceholder: 'Enter upstream request ID, press Enter',
      mismatchOnly: 'Mismatched only',
      mismatchMatched: 'Matched only',
      mismatchBadge: 'Mismatch',
      emptyTable: 'No usage records found. Try adjusting your filters.',
      exportButton: 'Export Excel',
      upstreamResponseModelHeader: 'Upstream Response Model',
      sessionIdHeader: 'Session ID',
      mismatchHeader: 'Model Consistency',
      requestedEffortHeader: 'Requested Effort',
      yes: 'Yes',
      no: 'No',
      upstreamRequestIdHeaderInvalid:
        'Must be a valid HTTP header field name of at most 64 characters, without spaces, colons or line breaks',
    }
  }
  return {
    columnSettings: /列设置/,
    accountSearch: '搜索账号...',
    editButton: '编辑',
    cancelButton: /^取消$/,
    model: '模型',
    group: '分组',
    upstreamRequestId: '上游请求 ID',
    sessionId: '会话 ID',
    upstreamRequestIdPlaceholder: '输入上游请求 ID，回车查询',
    mismatchOnly: '仅模型不一致',
    mismatchMatched: '仅模型一致',
    mismatchBadge: '不一致',
    emptyTable: '未找到使用记录，请尝试调整筛选条件。',
    exportButton: '导出 Excel',
    upstreamResponseModelHeader: '上游响应模型',
    sessionIdHeader: '会话 ID',
    mismatchHeader: '模型一致性',
    requestedEffortHeader: '请求推理强度',
    yes: '是',
    no: '否',
    upstreamRequestIdHeaderInvalid:
      '头名必须是合法的 HTTP 头字段名，不超过 64 个字符，不能含空格、冒号或换行',
  }
}

/** 打开 UsageFilters 里的 Select 并点选指定文案的选项。 */
export async function selectUsageFilter(page: Page, testId: string, optionLabel: string) {
  const root = page.getByTestId(testId)
  await root.locator('button').first().click()
  await page.getByRole('option', { name: optionLabel, exact: true }).click()
}

/**
 * 把原始字符串直接送进 Vue 的 v-model。
 *
 * `input[type=text]` 在浏览器层会执行 value sanitization：maxlength 不约束脚本赋值，
 * 但换行会被静默剥掉。AC32 需要验证组件层的校验分支（65 字符 / 含换行），
 * 因此这里覆写元素的 value 访问器并派发 input 事件，等价于粘贴/输入法把原始值
 * 交给组件；组件随后照常展示错误并阻止提交。关闭对话框后 DOM 销毁，覆写不残留。
 */
export async function setInputValueRaw(page: Page, selector: string, raw: string) {
  await page.evaluate(
    ({ selector, raw }) => {
      const el = document.querySelector(selector) as HTMLInputElement | null
      if (!el) throw new Error('input not found: ' + selector)
      let current = ''
      Object.defineProperty(el, 'value', {
        configurable: true,
        get: () => current,
        set: (next: string) => {
          current = String(next)
        },
      })
      current = raw
      el.dispatchEvent(new Event('input', { bubbles: true }))
    },
    { selector, raw },
  )
}

/** 记录账号写请求，用于断言非法输入没有触发保存。 */
export function trackAccountWrites(page: Page): string[] {
  const writes: string[] = []
  page.on('request', (req) => {
    const pathname = new URL(req.url()).pathname
    if (req.method() === 'PUT' && /^\/api\/v1\/admin\/accounts\/\d+$/.test(pathname)) {
      writes.push('PUT ' + pathname)
    }
    if (req.method() === 'POST' && pathname === '/api/v1/admin/accounts/bulk-update') {
      writes.push('POST ' + pathname)
    }
  })
  return writes
}
