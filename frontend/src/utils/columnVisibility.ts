// 表格列显示设置的读写。
//
// 存档格式 v2：{ v: 2, hidden: string[], known: string[] }。known 记录保存时已有的全部可切换列，
// 读取时不在 known 里的列视为用户没见过的新列，按 defaultHidden 处理；用户手动打开过的列
// 在 known 里且不在 hidden 里，保持打开。
//
// 旧格式是纯数组（只有 hidden），无法区分「用户打开了」和「保存时还没有这一列」。
// legacyNewColumns 列出旧格式之后才加入的列，读旧存档时把它们当作没见过的新列。

export const COLUMN_VISIBILITY_VERSION = 2

export interface ColumnVisibilityOptions {
  /** 当前全部可切换的列 key。 */
  columns: readonly string[]
  /** 默认隐藏的列。 */
  defaultHidden: readonly string[]
  /** 旧数组格式存档里不可能出现、需要按默认值处理的列。 */
  legacyNewColumns?: readonly string[]
}

interface StoredColumnVisibility {
  v: number
  hidden: string[]
  known: string[]
}

const isStringArray = (value: unknown): value is string[] =>
  Array.isArray(value) && value.every((item) => typeof item === 'string')

const defaults = (options: ColumnVisibilityOptions): Set<string> => new Set(options.defaultHidden)

/** 解析存档，返回应隐藏的列。raw 为 null 或无法解析时返回默认值。 */
export function resolveHiddenColumns(raw: string | null, options: ColumnVisibilityOptions): Set<string> {
  if (raw === null) return defaults(options)
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return defaults(options)
  }

  let hidden: string[]
  let known: Set<string>
  if (isStringArray(parsed)) {
    hidden = parsed
    const legacyNew = new Set(options.legacyNewColumns ?? [])
    known = new Set(options.columns.filter((key) => !legacyNew.has(key)))
  } else if (
    parsed &&
    typeof parsed === 'object' &&
    isStringArray((parsed as StoredColumnVisibility).hidden) &&
    isStringArray((parsed as StoredColumnVisibility).known)
  ) {
    hidden = (parsed as StoredColumnVisibility).hidden
    known = new Set((parsed as StoredColumnVisibility).known)
  } else {
    return defaults(options)
  }

  const result = new Set(hidden)
  for (const key of options.defaultHidden) {
    if (!known.has(key)) result.add(key)
  }
  return result
}

/** 生成存档字符串，known 写入当前全部可切换列。 */
export function serializeHiddenColumns(hidden: Iterable<string>, columns: readonly string[]): string {
  const stored: StoredColumnVisibility = {
    v: COLUMN_VISIBILITY_VERSION,
    hidden: [...hidden],
    known: [...columns]
  }
  return JSON.stringify(stored)
}
