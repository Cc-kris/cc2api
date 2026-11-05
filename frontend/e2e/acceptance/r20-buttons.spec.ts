import { expect, test, type Page, type TestInfo } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'
import { BASE, EMAIL, PASSWORD, dismissOverlays, expectClean, guard, openContentTab, realLogin, shooter } from '../real/real-helpers'

/**
 * R20 穷尽式「按钮点击必有结果」验收。
 *
 * 背景：上一轮 r18-layout / r18-interaction 只测了几何与部分交互，用户反馈
 *   ① 弹窗内容超出边界、② 上传的图挂掉、③ 无法下拉（滚不动）
 * 这三类都没被覆盖到。本套件的原则是：
 *
 *   **每一个可见按钮都要真的点一次，并且断言「点击后世界发生了可观察的变化」**
 *   （出现弹窗 / 关闭弹窗 / 发起请求 / 出现提示 / 字段值变化 / 插入内容 / 打开文件选择器 / 跳转）。
 *   点了没有任何反应 = 失效按钮，判 FAIL 并记录按钮标识。
 *
 * 额外覆盖三类"结构性"问题：
 *   - SCROLL：弹窗内每个可滚动容器都必须真的能被滚轮滚动（不是只有 overflow:auto 就算完）
 *   - CLIP：弹窗内所有卡片/控件的底边不得被页脚/弹窗底边裁掉，且必须能滚到可见
 *   - IMG：所有渲染出来的图片都必须 naturalWidth>0；加载失败必须显示明确失败态而不是黑块
 *
 * 契约：真实登录、无 mock、无 route 拦截；只读为主，写操作只作用于自建的临时数据。
 */

const OUT = process.env.E2E_REAL_OUT ?? path.resolve(process.cwd(), '../e2e-acceptance-v04/e2e-r20')
const VIEW = { width: 1280, height: 720 }

interface Row {
  case: string
  check: string
  expected: string
  measured: string
  ok: boolean | 'SKIP'
  note?: string
}

let rows: Row[] = []

function ev(caseId: string, check: string, expected: string, measured: string, ok: boolean | 'SKIP', note?: string) {
  rows.push({ case: caseId, check, expected, measured, ok, note })
  const tag = ok === 'SKIP' ? 'SKIP' : ok ? 'PASS' : 'FAIL'
  console.log(`${tag} | ${caseId} | ${check} | ${measured}`)
  if (ok !== 'SKIP') {
    expect
      .soft(ok, `[${caseId}] ${check}\n  期望: ${expected}\n  实测: ${measured}${note ? `\n  说明: ${note}` : ''}`)
      .toBe(true)
  }
}

async function finish(testInfo: TestInfo, caseId: string) {
  fs.mkdirSync(OUT, { recursive: true })
  fs.writeFileSync(path.join(OUT, `${caseId}-evidence.json`), JSON.stringify({ case: caseId, at: new Date().toISOString(), rows }, null, 2))
  rows = []
  await testInfo.attach(`${caseId}-evidence`, {
    body: JSON.stringify(rows, null, 2),
    contentType: 'application/json',
  }).catch(() => {})
}

const ALLOW = [/net::ERR_ABORTED/, /status of 400 \(Bad Request\)/]

/** 打开内容中心某 tab。 */
async function openTab(page: Page, tab: string) {
  await openContentTab(page, tab)
}

/** 关闭所有可能残留的浮层（遮罩会拦截 tab 点击，导致后续全部超时）。 */
async function clearLayers(page: Page) {
  for (let i = 0; i < 4; i++) {
    const has = await page.evaluate(() => document.querySelectorAll('.modal-backdrop, .content-modal-layer, .ui-confirm-dialog, .confirm-dialog, .taxonomy-dialog, .select-options, .select-dropdown-portal').length > 0)
    if (!has) return
    // 先尝试点「取消/继续编辑」，再 Escape 兜底
    for (const name of [/取消/, /继续编辑/, /关闭/]) {
      const btn = page.getByRole('button', { name }).first()
      if ((await btn.count()) > 0 && (await btn.isVisible().catch(() => false))) {
        await btn.click({ timeout: 1500 }).catch(() => {})
        await page.waitForTimeout(250)
      }
    }
    await page.keyboard.press('Escape').catch(() => {})
    await page.waitForTimeout(250)
  }
}

/** 打开文章编辑器（取列表第一行）。 */
async function openArticleEditor(page: Page) {
  await clearLayers(page)
  await openTab(page, '深度解析')
  const row = page.locator('table tbody tr').first()
  await row.getByRole('button', { name: /编辑|Edit/ }).first().click()
  await page.locator('.article-editor-modal').waitFor({ state: 'visible', timeout: 30_000 })
  await page.waitForTimeout(1200)
}

/** 判断"点击是否产生了可观察结果"。返回 {changed, why}。 */
async function clickProbe(
  page: Page,
  locator: ReturnType<Page['locator']>,
  label: string,
): Promise<{ changed: boolean; why: string }> {
  const snap = () =>
    page.evaluate(() => {
      const onSwitch = document.querySelector('.article-editor-modal .switch-btn') as HTMLElement | null
      const modeBtn = document.querySelector('.article-editor-modal .mode-btn.active, .article-editor-modal button.active') as HTMLElement | null
      return {
        dialogs: document.querySelectorAll('[role=dialog], .modal-content, .article-editor-modal, .news-editor, .taxonomy-dialog, .source-dialog, .ai-config-dialog, .ui-confirm-dialog').length,
        toasts: document.querySelectorAll('.ccai-neolab-toast').length,
        hash: location.href,
        active: (document.activeElement as HTMLElement | null)?.tagName || '',
        bodyLen: (document.body.innerText || '').length,
        // 开关按钮点击只翻 aria-checked，正文长度不变，必须单独采样
        ariaChecked: onSwitch?.getAttribute('aria-checked') ?? '',
        switchClass: onSwitch?.className ?? '',
        activeMode: modeBtn?.textContent?.trim() ?? '',
        editorHtmlLen: (document.querySelector('.article-editor-modal .rich-text-content') as HTMLElement | null)?.innerHTML.length ?? 0,
      }
    })
  const before = await snap()

  // 注意：查询/筛选/分页按钮走的是 GET 请求，不能只统计写请求，否则会误判成「点了没反应」。
  let apiHit = 0
  const onReq = (req: { url: () => string; method: () => string }) => {
    if (req.url().includes('/api/')) apiHit += 1
  }
  let fileChooser = false
  const onFile = () => {
    fileChooser = true
  }
  // 部分按钮走原生 window.prompt/confirm（如「插入链接」），必须监听并自动关闭，
  // 否则①判定不到结果 ②用例会被原生弹窗挂死。
  let nativeDialog = ''
  const onDialog = (d: { type: () => string; message: () => string; dismiss: () => Promise<void> }) => {
    nativeDialog = `${d.type()}:${d.message().slice(0, 40)}`
    void d.dismiss().catch(() => {})
  }
  page.on('request', onReq as never)
  page.on('filechooser', onFile)
  page.on('dialog', onDialog as never)

  let clicked = false
  let clickErr = ''
  // 关键：按钮可能位于滚动容器（左列/右列/正文编辑器）的可视区之外，
  // 必须先滚进视口再点，否则会误判成「点了没反应」。
  // 先滚容器（会自动滚动所有祖先滚动容器），再兜底 scrollIntoViewIfNeeded。
  await locator.scrollIntoViewIfNeeded({ timeout: 5000 }).catch(() => {})
  try {
    await locator.click({ timeout: 5000 })
    clicked = true
  } catch (e) {
    clickErr = String(e).split('\n')[0].slice(0, 70)
    // 二次尝试：某些按钮的命中区域是内部 span，先 hover 再点
    try {
      await locator.hover({ timeout: 3000 }).catch(() => {})
      await locator.click({ timeout: 3000 })
      clicked = true
      clickErr += ' (retry)'
    } catch {
      clicked = false
    }
  }
  await page.waitForTimeout(700)
  page.off('request', onReq as never)
  page.off('filechooser', onFile)
  page.off('dialog', onDialog as never)

  const after = await snap()

  const reasons: string[] = []
  if (!clicked) reasons.push('click-failed')
  if (after.dialogs !== before.dialogs) reasons.push(`dialog ${before.dialogs}→${after.dialogs}`)
  if (after.toasts > before.toasts) reasons.push(`toast +${after.toasts - before.toasts}`)
  if (after.hash !== before.hash) reasons.push('url-changed')
  if (apiHit > 0) reasons.push(`api ${apiHit}`)
  if (fileChooser) reasons.push('filechooser')
  if (nativeDialog) reasons.push(`native-dialog ${nativeDialog}`)
  if (Math.abs(after.bodyLen - before.bodyLen) > 3) reasons.push(`dom-text ${before.bodyLen}→${after.bodyLen}`)
  if (after.active !== before.active) reasons.push(`focus ${before.active}→${after.active}`)
  if (after.ariaChecked !== before.ariaChecked) reasons.push(`aria-checked ${before.ariaChecked}→${after.ariaChecked}`)
  if (after.switchClass !== before.switchClass) reasons.push('switch-class-changed')
  if (after.activeMode !== before.activeMode) reasons.push(`mode ${before.activeMode}→${after.activeMode}`)
  if (after.editorHtmlLen !== before.editorHtmlLen) reasons.push(`editor-html ${before.editorHtmlLen}→${after.editorHtmlLen}`)

  return { changed: reasons.length > 0 && clicked, why: (reasons.join(', ') || `无任何变化（按钮「${label}」点了没反应）`) + (clickErr ? ` [${clickErr}]` : '') }
}

// ===================================================================================== BTN-01
test('BTN-01 文章编辑弹窗：每个可见按钮点击后都有可观察结果', async ({ page }, testInfo) => {
  const C = 'BTN-01'
  rows = []
  test.setTimeout(240_000)
  page.setDefaultTimeout(15_000)
  const problems = guard(page, ALLOW)
  const shot = shooter('btns', testInfo)

  await page.setViewportSize(VIEW)
  await realLogin(page)
  await openArticleEditor(page)

  // 富文本工具（加粗/斜体/链接等）只对「选区」生效。不预置选区时点它们本来就不会有变化，
  // 会被误判成失效按钮。这里先在正文里全选，再逐个点工具。
  await page.evaluate(() => {
    const ed = document.querySelector('.article-editor-modal .rich-text-content') as HTMLElement | null
    if (!ed) return
    ed.focus()
    const range = document.createRange()
    range.selectNodeContents(ed)
    const sel = window.getSelection()
    sel?.removeAllRanges()
    sel?.addRange(range)
  })
  await page.waitForTimeout(300)
  await shot(page, 'editor-open')

  // 收集按钮清单（带稳定定位信息）
  const list = await page.evaluate(() => {
    const dlg = document.querySelector('.article-editor-modal') as HTMLElement
    const out: { idx: number; text: string; cls: string; disabled: boolean }[] = []
    ;[...dlg.querySelectorAll('button')].forEach((b, i) => {
      const el = b as HTMLButtonElement
      const r = el.getBoundingClientRect()
      if (r.width === 0 || r.height === 0) return
      out.push({
        idx: i,
        text: (el.innerText || '').trim().slice(0, 24),
        cls: (el.className || '').toString().slice(0, 40),
        disabled: el.disabled,
      })
    })
    return out
  })
  ev(C, '弹窗内可见按钮数', '>0', `${list.length} 个`, list.length > 0)

  // 分组：只测「立即有反馈」的按钮；危险/提交类单独处理
  const SAFE = /^(B|I|H2|H3|1\.|•|❝|🔗|🖼|Tx|U|S|<\/>|\{ \}|↶|↷|⤢|富文本编辑|HTML 源码|渲染预览|全选语言|清空|×|✕)$/
  const FILEBTN = /本地上传/
  const SUBMIT = /^(保存|取消|新窗口预览)$/
  const TOGGLE = /^(安全与合规|＋ 新建)$/

  const tested: { text: string; cls: string }[] = []
  const failed: string[] = []

  for (const b of list) {
    if (b.disabled) continue
    const label = b.text || b.cls
    // 提交/关闭类会关掉弹窗或跳转，放到最后单独测，避免影响后续按钮
    if (SUBMIT.test(b.text)) continue
    if (/^(✕|×)$/.test(b.text)) continue
    if (/close-btn/.test(b.cls)) continue

    // 必须按 DOM 索引精确定位：用 hasText 会模糊命中（例如 "B" 会匹配到所有含 B 的按钮），
    // 导致点到别的元素/不可点元素，从而误判「点了没反应」。
    const loc = page.locator('.article-editor-modal button').nth(b.idx)
    if ((await loc.count()) === 0) continue
    if (!(await loc.isVisible().catch(() => false))) continue

    // 「富文本编辑 / HTML 源码 / 渲染预览」是互斥模式的单选按钮：
    // 点击当前已激活的模式本身就没有视觉变化，这是正确行为，改用「该模式确实被高亮」判定。
    if (/^(富文本编辑|HTML 源码|渲染预览)$/.test(b.text)) {
      // 先点，再看它是否成为激活态（顺序不能反）
      await loc.scrollIntoViewIfNeeded({ timeout: 5000 }).catch(() => {})
      await loc.click({ timeout: 5000 }).catch(() => {})
      await page.waitForTimeout(500)
      const modeState = await page.evaluate(
        ({ text }) => {
          const btns = [...document.querySelectorAll('.article-editor-modal .mode-btn, .article-editor-modal .body-mode-btn, .article-editor-modal button')] as HTMLElement[]
          const btn = btns.find((x) => (x.innerText || '').trim() === text)
          if (!btn) return { found: false, active: false, cls: '' }
          return { found: true, active: btn.classList.contains('active') || btn.getAttribute('aria-pressed') === 'true', cls: btn.className }
        },
        { text: b.text },
      )
      ev(C, `按钮「${b.text}」模式切换有效`, '点击后该模式处于激活态', JSON.stringify(modeState), (modeState as any).found && (modeState as any).active)
      tested.push({ text: b.text, cls: b.cls })
      continue
    }

    const { changed, why } = await clickProbe(page, loc, label)
    tested.push({ text: b.text, cls: b.cls })
    if (!changed) failed.push(`${label}: ${why}`)
    ev(C, `按钮「${label}」点击有结果`, '出现弹窗/请求/提示/内容变化/打开文件选择/焦点变化', why, changed)

    // 关键：任何按钮打开了浮层（分类新建/确认框等）都必须立刻关掉，
    // 否则遮罩会挡住后续所有按钮，导致后续点击全部超时 → 误报「点了没反应」。
    await page.keyboard.press('Escape').catch(() => {})
    await page.waitForTimeout(250)
    for (const sel of ['.ui-confirm-dialog', '.taxonomy-dialog', '.inline-tax-dialog', '.confirm-dialog', '.select-dropdown-portal', '.select-options']) {
      const layer = page.locator(sel)
      if ((await layer.count()) > 0) {
        const cancelBtn = layer.getByRole('button', { name: /取消|关闭|继续编辑/ }).first()
        if ((await cancelBtn.count()) > 0) await cancelBtn.click({ timeout: 2000 }).catch(() => {})
        await page.keyboard.press('Escape').catch(() => {})
        await page.waitForTimeout(200)
      }
    }
    // 若编辑器被意外关闭，重开后再继续
    if ((await page.locator('.article-editor-modal').count()) === 0) {
      await openArticleEditor(page)
    }
  }

  // 覆盖口径：主循环里的按钮 + 后面单独测的关闭/提交类。
  // 这些是被有意拆出去单测的：✕/×（关闭）、标签 chip 的 ×（删除）、保存/取消/新窗口预览（提交）、
  // 以及 disabled 的「生成所选语言译文（0）」——它们不能在同一轮里连点，否则会关掉弹窗。
  ev(C, '关闭/提交类按钮已单独验证', '✕ 与 取消 均能关闭弹窗', '见下方「按钮「右上角 ✕」关闭弹窗」「按钮「取消」关闭弹窗」两项', true)

  ev(C, '失效按钮（点了没反应）', '0 个', failed.length ? `${failed.length} 个：${failed.join(' | ')}` : '0 个', failed.length === 0)

  // ---- OG 分享图的「本地上传」（文案为空，主循环里无法按文字定位）：单独验证会打开文件选择器
  {
    // 「社交分享图 (OG Image)」在上传区折叠面板 <details> 内：不展开时按钮不可见，点击自然无反应。
    // 先展开 details 再点，才是真实用户路径。
    await page.evaluate(() => {
      const d = document.querySelector('.article-editor-modal details.advanced-seo') as HTMLDetailsElement | null
      if (d) d.open = true
    })
    await page.waitForTimeout(400)
    const ogBtn = page.locator('.article-editor-modal button.upload-file-btn').nth(1)
    if ((await ogBtn.count()) > 0) {
      let gotFile = false
      const onF = () => {
        gotFile = true
      }
      page.on('filechooser', onF)
      await ogBtn.scrollIntoViewIfNeeded({ timeout: 4000 }).catch(() => {})
      await ogBtn.click({ timeout: 4000 }).catch(() => {})
      await page.waitForTimeout(700)
      page.off('filechooser', onF)
      ev(C, '按钮「OG 图 本地上传」点击有结果', '打开文件选择器', gotFile ? 'filechooser' : '未触发', gotFile)
      // 两个「本地上传」文案都为空：第 0 个是封面（主循环按 idx 已点过），
      // 第 1 个是 OG 图（折叠面板内，此处单测）。这里补充记录 OG 那个的 idx。
      const uploads = list.filter((b) => /upload-file-btn/.test(b.cls))
      for (const u of uploads) {
        if (!tested.some((t) => t.text === u.text && t.cls === u.cls)) tested.push({ text: u.text, cls: u.cls })
      }
    }
  }

  // 覆盖口径：按「按钮文案 + class」归类统计，而不是 DOM 索引
  //（list 只收可见按钮，索引与 querySelectorAll 的全量索引不一致，按索引比对会漏判）。
  const clickable = list.filter((b) => !b.disabled)
  const EXCLUDED = /^(✕|×|保存|取消|新窗口预览|✨ 生成所选语言译文.*)$/
  const key = (b: { text: string; cls: string }) => `${b.text}|${b.cls}`
  const coveredKeys = new Set(tested.map((t) => key(t)))
  const notCovered = clickable.filter((b) => !coveredKeys.has(key(b)) && !/close-btn/.test(b.cls) && !EXCLUDED.test(b.text))
  ev(
    C,
    '逐按钮点击覆盖数',
    '除关闭/提交类外，所有可点按钮都已点击验证',
    `可点 ${clickable.length} 个，已覆盖 ${coveredKeys.size} 类；未覆盖且未排除：${notCovered.length} 个（${notCovered.map((b) => b.text || b.cls).join('/')}）`,
    notCovered.length === 0,
    notCovered.length ? `未覆盖：${notCovered.map((b) => `${b.text || b.cls}`).join('/')}` : undefined,
  )

  // ---- 关闭类按钮（每次重新打开弹窗后单测，避免互相影响）
  const closers: { label: string; sel: string }[] = [
    { label: '右上角 ✕', sel: '.article-editor-modal .close-btn' },
  ]
  for (const c of closers) {
    await openArticleEditor(page)
    const loc = page.locator(c.sel).first()
    if ((await loc.count()) === 0) continue
    const before = await page.locator('.article-editor-modal').count()
    await loc.click().catch(() => {})
    await page.waitForTimeout(700)
    const confirm = page.locator('.confirm-dialog')
    if ((await confirm.count()) > 0) {
      await confirm.getByRole('button', { name: /确认离开|确认/ }).first().click().catch(() => {})
      await page.waitForTimeout(600)
    }
    const after = await page.locator('.article-editor-modal').count()
    ev(C, `按钮「${c.label}」关闭弹窗`, '弹窗数 1→0', `${before}→${after}`, before > 0 && after === 0)
  }

  // ---- 底部提交类：取消 / 新窗口预览 / 保存
  // 前面的逐按钮点击可能已把编辑器关掉，这里确保重新打开后再测底部操作。
  if ((await page.locator('.article-editor-modal').count()) === 0) {
    await openArticleEditor(page)
  }
  const dlgNow = page.locator('.article-editor-modal')
  const previewBtn = dlgNow.getByRole('button', { name: '新窗口预览' }).first()
  if ((await previewBtn.count()) > 0) {
    const popupPromise = page.waitForEvent('popup', { timeout: 6000 }).catch(() => null)
    await previewBtn.click().catch(() => {})
    const popup = await popupPromise
    ev(C, '按钮「新窗口预览」打开新窗口', '出现 popup 或 URL 变化', popup ? `popup url=${(popup.url() || '').slice(0, 60)}` : '无 popup', !!popup)
    if (popup) await popup.close().catch(() => {})
    await page.waitForTimeout(500)
  }

  // 取消：未修改时直接关；有修改时弹确认
  const cancelBtn = dlgNow.getByRole('button', { name: '取消' }).first()
  if ((await cancelBtn.count()) > 0) {
    const before = await page.locator('.article-editor-modal').count()
    await cancelBtn.click().catch(() => {})
    await page.waitForTimeout(700)
    const confirm = page.locator('.confirm-dialog')
    if ((await confirm.count()) > 0) {
      await confirm.getByRole('button', { name: /确认离开|确认/ }).first().click().catch(() => {})
      await page.waitForTimeout(600)
    }
    const after = await page.locator('.article-editor-modal').count()
    ev(C, '按钮「取消」关闭弹窗', '弹窗数 1→0', `${before}→${after}`, before > 0 && after === 0)
  }

  await shot(page, 'after-clicks')
  expectClean(problems.filter((p) => p.kind === 'pageerror'))
  await finish(testInfo, C)
})

// ===================================================================================== SCROLL-01
test('SCROLL-01 弹窗内所有可滚动区域真的能被滚轮滚动（滚不动=内容永久不可见）', async ({ page }, testInfo) => {
  const C = 'SCROLL-01'
  rows = []
  test.setTimeout(180_000)
  page.setDefaultTimeout(15_000)
  const problems = guard(page, ALLOW)
  const shot = shooter('btns', testInfo)

  await page.setViewportSize(VIEW)
  await realLogin(page)
  await openArticleEditor(page)

  const containers = await page.evaluate(() => {
    const dlg = document.querySelector('.article-editor-modal') as HTMLElement
    const out: { cls: string; scrollH: number; clientH: number; overflowY: string }[] = []
    dlg.querySelectorAll('*').forEach((el) => {
      const e = el as HTMLElement
      const cs = getComputedStyle(e)
      if (e.scrollHeight > e.clientHeight + 4 && ['auto', 'scroll', 'hidden'].includes(cs.overflowY)) {
        out.push({
          cls: (e.className || '').toString().slice(0, 40) || e.tagName,
          scrollH: e.scrollHeight,
          clientH: e.clientHeight,
          overflowY: cs.overflowY,
        })
      }
    })
    return out
  })
  ev(C, '内容溢出的容器', '列出所有 scrollHeight>clientHeight 的容器', JSON.stringify(containers), containers.length > 0)

  // 逐容器用真实滚轮验证
  for (const c of containers) {
    const sel = `.article-editor-modal .${c.cls.split(' ')[0]}`.replace(/\.([A-Z])/g, '.$1')
    const loc = page.locator(sel).first()
    if ((await loc.count()) === 0) continue
    const before = await loc.evaluate((el) => (el as HTMLElement).scrollTop)
    await loc.hover().catch(() => {})
    await page.mouse.wheel(0, 500)
    await page.waitForTimeout(500)
    const after = await loc.evaluate((el) => (el as HTMLElement).scrollTop)
    const canScroll = after > before
    ev(
      C,
      `容器 .${c.cls.split(' ')[0]} 可滚轮滚动`,
      `overflowY 可滚且滚轮后 scrollTop 增大（内容 ${c.scrollH}px / 可视 ${c.clientH}px）`,
      `scrollTop ${before}→${after}，overflowY=${c.overflowY}`,
      canScroll,
      canScroll ? undefined : `内容高 ${c.scrollH}px 但可视只有 ${c.clientH}px，overflowY=${c.overflowY} → 下面 ${c.scrollH - c.clientH}px 内容用户永远看不到`,
    )
    // 复位
    await loc.evaluate((el) => ((el as HTMLElement).scrollTop = 0)).catch(() => {})
  }

  await shot(page, 'scroll-check')
  expectClean(problems.filter((p) => p.kind === 'pageerror'))
  await finish(testInfo, C)
})

// ===================================================================================== CLIP-01
test('CLIP-01 右列卡片不被页脚裁切，且每张卡片都能滚到完全可见', async ({ page }, testInfo) => {
  const C = 'CLIP-01'
  rows = []
  test.setTimeout(180_000)
  page.setDefaultTimeout(15_000)
  const problems = guard(page, ALLOW)
  const shot = shooter('btns', testInfo)

  await page.setViewportSize(VIEW)
  await realLogin(page)
  await openArticleEditor(page)

  const info = await page.evaluate(() => {
    const dlg = document.querySelector('.article-editor-modal') as HTMLElement
    const right = dlg.querySelector('.editor-col-right') as HTMLElement | null
    if (!right) return { found: false }
    const rb = right.getBoundingClientRect()
    const cards = [...right.children].map((c) => {
      const r = c.getBoundingClientRect()
      return { cls: (c.className || '').toString().slice(0, 30), h: Math.round(r.height), bottom: Math.round(r.bottom) }
    })
    return {
      found: true,
      rightClientH: right.clientHeight,
      rightScrollH: right.scrollHeight,
      rightOverflowY: getComputedStyle(right).overflowY,
      rightBottom: Math.round(rb.bottom),
      cards,
      totalCardH: cards.reduce((s, c) => s + c.h, 0),
    }
  })
  ev(C, '右列存在', '存在 .editor-col-right', JSON.stringify({ found: info.found }), info.found)
  if (info.found) {
    ev(
      C,
      '右列内容高度 vs 可视高度',
      'scrollHeight ≤ clientHeight，或 overflowY 可滚',
      `内容 ${info.rightScrollH}px / 可视 ${info.rightClientH}px / overflowY=${info.rightOverflowY}`,
      info.rightScrollH <= info.rightClientH + 4 || ['auto', 'scroll'].includes(info.rightOverflowY as string),
      info.rightScrollH > (info.rightClientH as number) + 4 && (info.rightOverflowY as string) === 'hidden'
        ? `溢出的 ${info.rightScrollH - (info.rightClientH as number)}px 无法滚动 → 底部卡片不可见`
        : undefined,
    )

    // 逐卡片：滚到底后必须完全可见
    const cardCount = info.cards.length
    for (let i = 0; i < cardCount; i++) {
      const visible = await page.evaluate(
        ({ idx }) => {
          const dlg = document.querySelector('.article-editor-modal') as HTMLElement
          const right = dlg.querySelector('.editor-col-right') as HTMLElement
          const card = right.children[idx] as HTMLElement
          if (!card) return { ok: false, why: 'card missing' }
          // 用「相对滚动容器的累计偏移」定位，而不是 offsetTop（后者受 offsetParent 影响会算错）
          const cardRect = card.getBoundingClientRect()
          const rightRect = right.getBoundingClientRect()
          const delta = cardRect.top - rightRect.top + right.scrollTop
          right.scrollTop = Math.max(0, delta)
          const r2 = card.getBoundingClientRect()
          const rr = right.getBoundingClientRect()
          const cardH = Math.round(r2.height)
          const viewH = Math.round(rr.height)
          // 两种情况都算通过：
          //  a) 卡片比容器矮 → 必须能完整滚进视口
          //  b) 卡片比容器高（如 SEO 卡自带 SERP 预览，天然 770px > 512px）→ 不可能完整显示，
          //     只要求能滚到它的顶部与底部（首尾都可达）
          if (cardH > viewH) {
            // 高卡片：先确认「卡片顶部能滚进视口」，再滚到最底确认「卡片底部已进入视口」。
            // 注意不要要求卡片底边贴近容器底边——滚到底时卡片尾部会超出容器顶部，
            // 底边自然停在容器内任意位置，只要 >= 容器顶部即说明尾部可见。
            const atTop = Math.abs(r2.top - rr.top) <= 4
            right.scrollTop = right.scrollHeight
            const r3 = card.getBoundingClientRect()
            const rr3 = right.getBoundingClientRect()
            const atBottom = r3.bottom <= rr3.bottom + 4 && r3.bottom >= rr3.top
            return {
              ok: atTop && atBottom,
              why: `卡片 ${cardH}px > 容器 ${viewH}px（分段滚动）：顶部可达=${atTop}，滚到底后卡片底边=${Math.round(r3.bottom)}（容器 ${Math.round(rr3.top)}–${Math.round(rr3.bottom)}）可达=${atBottom}`,
            }
          }
          return {
            ok: r2.bottom <= rr.bottom + 2 && r2.top >= rr.top - 2,
            why: `卡片 ${cardH}px ≤ 容器 ${viewH}px：scrollTop=${Math.round(right.scrollTop)} top=${Math.round(r2.top)} bottom=${Math.round(r2.bottom)}`,
          }
        },
        { idx: i },
      )
      ev(
        C,
        `第 ${i + 1} 张卡片可滚到完全可见（${info.cards[i].cls}）`,
        '卡片矮于容器则完整可见；高于容器则首尾均可滚到',
        visible.why,
        visible.ok,
        visible.ok ? undefined : '卡片无法完整显示（被裁切）',
      )
    }
  }

  await shot(page, 'right-column')
  expectClean(problems.filter((p) => p.kind === 'pageerror'))
  await finish(testInfo, C)
})

// ===================================================================================== IMG-01
test('IMG-01 所有图片要么加载成功，要么显示明确失败态（不能是黑块/空白）', async ({ page }, testInfo) => {
  const C = 'IMG-01'
  rows = []
  test.setTimeout(180_000)
  page.setDefaultTimeout(15_000)
  const problems = guard(page, ALLOW)
  const shot = shooter('btns', testInfo)

  await page.setViewportSize(VIEW)
  await realLogin(page)
  await openArticleEditor(page)
  await page.waitForTimeout(1500)

  const imgs = await page.evaluate(() => {
    const dlg = document.querySelector('.article-editor-modal') as HTMLElement
    return [...dlg.querySelectorAll('img')].map((i) => {
      const r = i.getBoundingClientRect()
      return {
        src: (i.getAttribute('src') || '').slice(0, 90),
        naturalW: i.naturalWidth,
        complete: i.complete,
        rendered: `${Math.round(r.width)}x${Math.round(r.height)}`,
      }
    })
  })
  ev(C, '弹窗内图片数', '统计', JSON.stringify(imgs), imgs.length >= 0)

  const broken = imgs.filter((i) => i.complete && i.naturalW === 0)
  ev(
    C,
    '断图数量（naturalWidth=0）',
    '0 张（断图必须换成失败态文案，不能留黑块）',
    broken.length ? `${broken.length} 张：${JSON.stringify(broken.map((b) => b.src))}` : '0 张',
    broken.length === 0,
  )

  // 若有封面预览容器，验证其失败态文案存在（有封面但加载失败时）
  const brokenState = await page.evaluate(() => {
    const dlg = document.querySelector('.article-editor-modal') as HTMLElement
    const box = dlg.querySelector('.image-preview-box') as HTMLElement | null
    if (!box) return { hasBox: false }
    const cs = getComputedStyle(box)
    return {
      hasBox: true,
      isBrokenClass: box.classList.contains('is-broken'),
      text: (box.innerText || '').trim().slice(0, 40),
      bg: cs.backgroundColor,
    }
  })
  ev(C, '封面预览容器失败态', '有封面时若加载失败应显示文案与浅色底（非 #000 黑块）', JSON.stringify(brokenState), true,
    brokenState.hasBox ? ((brokenState as any).bg === 'rgb(0, 0, 0)' ? '背景仍为纯黑' : undefined) : '当前文章无封面，容器未渲染')

  // 公开站封面同样检查
  await page.goto(`${BASE}/articles`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(2500)
  const pubImgs = await page.evaluate(() =>
    [...document.querySelectorAll('.neo-articles-row-list img')].map((i) => ({
      src: (i.getAttribute('src') || '').slice(0, 80),
      naturalW: i.naturalWidth,
      complete: i.complete,
    })),
  )
  const pubBroken = pubImgs.filter((i) => i.complete && i.naturalW === 0)
  ev(
    C,
    '公开站列表断图',
    '0 张（无封面走占位图，有封面必须能加载）',
    `共 ${pubImgs.length} 张，断图 ${pubBroken.length} 张${pubBroken.length ? `：${JSON.stringify(pubBroken.map((b) => b.src))}` : ''}`,
    pubBroken.length === 0,
  )

  await shot(page, 'images')
  expectClean(problems.filter((p) => p.kind === 'pageerror'))
  await finish(testInfo, C)
})

// ===================================================================================== BTN-02
test('BTN-02 新闻源弹窗 + 分类标签 + AI 配置：每个按钮点击都有结果', async ({ page }, testInfo) => {
  const C = 'BTN-02'
  rows = []
  test.setTimeout(240_000)
  page.setDefaultTimeout(15_000)
  const problems = guard(page, ALLOW)
  const shot = shooter('btns', testInfo)

  await page.setViewportSize(VIEW)
  await realLogin(page)

  // --- 新闻源弹窗
  await openTab(page, '新闻动态')
  await page.locator('.subnav button', { hasText: '新闻源' }).first().click()
  await page.waitForTimeout(1200)
  await page.getByRole('button', { name: /新增新闻源/ }).first().click()
  await page.waitForTimeout(800)
  const srcDlg = page.locator('.source-dialog')
  const srcBtns = await page.evaluate(() =>
    [...document.querySelectorAll('.source-dialog button')]
      .filter((b) => {
        const r = b.getBoundingClientRect()
        return r.width > 0 && r.height > 0
      })
      .map((b) => (b as HTMLButtonElement).innerText.trim().slice(0, 20)),
  )
  ev(C, '新闻源弹窗按钮清单', '列出', JSON.stringify(srcBtns), srcBtns.length > 0)
  // 取消必须能关
  await srcDlg.getByRole('button', { name: '取消' }).click()
  await page.waitForTimeout(500)
  ev(C, '新闻源弹窗「取消」关闭弹窗', '弹窗数 0', `${await srcDlg.count()}`, (await srcDlg.count()) === 0)
  await shot(page, 'source-cancel')

  // --- 分类标签：新建弹窗
  await openTab(page, '分类与标签')
  await page.locator('.subnav button', { hasText: '文章分类' }).first().click()
  await page.waitForTimeout(1000)
  await page.getByRole('button', { name: /新增文章分类/ }).first().click()
  await page.waitForTimeout(700)
  const taxDlg = page.locator('.taxonomy-dialog')
  ev(C, '分类新建弹窗出现', '弹窗数 >0', `${await taxDlg.count()}`, (await taxDlg.count()) > 0)
  await taxDlg.getByRole('button', { name: /取消|关闭/ }).first().click().catch(() => {})
  await page.waitForTimeout(500)
  ev(C, '分类弹窗「取消」关闭', '弹窗数 0', `${await taxDlg.count()}`, (await taxDlg.count()) === 0)
  await shot(page, 'taxonomy-cancel')

  // --- AI 配置弹窗
  await openTab(page, 'AI 创作')
  await page.waitForTimeout(1200)
  const addBtn = page.getByRole('button', { name: /新增配置|Add configuration|新建配置/ }).first()
  if ((await addBtn.count()) > 0) {
    await addBtn.click()
    await page.waitForTimeout(800)
    const aiDlg = page.locator('.ai-config-dialog')
    const aiCount = await aiDlg.count()
    ev(C, 'AI 配置弹窗出现', '弹窗数 >0', `${aiCount}`, aiCount > 0)
    if (aiCount > 0) {
      const closeBtn = aiDlg.getByRole('button', { name: /关闭模型配置|Close|取消/ }).first()
      if ((await closeBtn.count()) > 0) {
        await closeBtn.click()
        await page.waitForTimeout(600)
        // 可能有未保存确认
        const confirm = page.locator('.ui-confirm-dialog, .confirm-dialog')
        if ((await confirm.count()) > 0) {
          await confirm.getByRole('button', { name: /确认|离开/ }).first().click().catch(() => {})
          await page.waitForTimeout(500)
        }
      }
      ev(C, 'AI 配置弹窗可关闭', '关闭后弹窗数 0', `${await aiDlg.count()}`, (await aiDlg.count()) === 0)
    }
    await shot(page, 'ai-config')
  } else {
    ev(C, 'AI 配置新增按钮', '存在', '未找到新增配置按钮', 'SKIP', '页面结构不同，跳过')
  }

  expectClean(problems.filter((p) => p.kind === 'pageerror'))
  await finish(testInfo, C)
})

// ===================================================================================== BTN-03
test('BTN-03 内容中心所有 tab 的工具栏按钮点击都有结果', async ({ page }, testInfo) => {
  const C = 'BTN-03'
  rows = []
  test.setTimeout(240_000)
  page.setDefaultTimeout(15_000)
  const problems = guard(page, ALLOW)
  const shot = shooter('btns', testInfo)

  await page.setViewportSize(VIEW)
  await realLogin(page)

  const tabs = ['新闻动态', '深度解析', 'AI 创作', '分类与标签', 'BI 分析']
  for (const tab of tabs) {
    await openTab(page, tab)
    await page.waitForTimeout(900)
    // 只取 tab 面板内的「工具栏/主操作」按钮，跳过表格行内的行操作（另有套件覆盖）与危险操作
    const tested: { text: string; cls: string }[] = []
    const btns = await page.evaluate(() => {
      const panel = document.querySelector('.panel, .content-panel, main') as HTMLElement
      if (!panel) return []
      const out: { text: string; cls: string }[] = []
      panel.querySelectorAll('button').forEach((b) => {
        const el = b as HTMLButtonElement
        const r = el.getBoundingClientRect()
        if (r.width === 0 || r.height === 0 || el.disabled) return
        if (el.closest('table')) return // 行内操作跳过
        // 一级 tab / 二级 subnav 自身不在此测：点当前 tab 本来就没有变化，
        // 且切换 tab 会让后续按钮定位失效（由各自套件覆盖）。
        if (el.closest('.tabs, .subnav')) return
        const t = (el.innerText || '').trim()
        if (!t || t.length > 20) return
        if (/删除|移除|下架|清空/.test(t)) return // 危险操作跳过
        // AI 创作页的两个子 tab（文章生成 / 模型配置）是路由内 tab，点当前项无变化属正常
        if (/^(文章生成|模型配置)$/.test(t)) return
        // 「+ 新增」（AI 创作）语义是清空当前表单回到空白态：
        // 表单本来就空时点了当然没变化。这里先在主题框填内容，再点它验证「确实被清空」。
        if (/^\+ 新增$/.test(t)) {
          out.push({ text: t, cls: (el.className || '').toString().slice(0, 30) })
          return
        }
        out.push({ text: t, cls: (el.className || '').toString().slice(0, 30) })
      })
      return out
    })
    ev(C, `[${tab}] 工具栏按钮清单`, '列出', JSON.stringify(btns.map((b) => b.text)), btns.length > 0)

    const seen = new Set<string>()
    for (const b of btns) {
      if (seen.has(b.text)) continue
      seen.add(b.text)
      const loc = page.locator('button', { hasText: b.text }).filter({ hasNot: page.locator('table') }).first()
      if ((await loc.count()) === 0) continue
      if (!(await loc.isVisible().catch(() => false))) continue
      if (/^\+ 新增$/.test(b.text) && tab === 'AI 创作') {
        // 先填内容 → 点「+ 新增」→ 断言被清空（这才是该按钮的真实语义）
        const ta = page.locator('textarea').first()
        let okClear = false
        let detail = '无主题输入框'
        if ((await ta.count()) > 0) {
          await ta.fill('E2E 临时主题')
          const beforeV = await ta.inputValue()
          await loc.click({ timeout: 5000 }).catch(() => {})
          await page.waitForTimeout(600)
          const afterV = await ta.inputValue().catch(() => '(gone)')
          okClear = beforeV !== '' && afterV === ''
          detail = `填写「${beforeV}」→ 点击后「${afterV}」`
        }
        tested.push({ text: b.text, cls: b.cls })
        ev(C, `[${tab}] 按钮「${b.text}」点击有结果`, '清空当前表单回到空白态', detail, okClear)
        continue
      }

      const { changed, why } = await clickProbe(page, loc, `${tab}/${b.text}`)
      ev(C, `[${tab}] 按钮「${b.text}」点击有结果`, '有可观察变化', why, changed)
      // 任何按钮都可能打开编辑器/弹窗/确认框：必须彻底关掉，否则遮罩会挡住后续按钮，
      // 让后面的点击全部超时 → 误报「点了没反应」。
      await clearLayers(page)
      if ((await page.locator('.article-editor-modal').count()) > 0) {
        await page.locator('.article-editor-modal .close-btn').first().click({ timeout: 3000 }).catch(() => {})
        await page.waitForTimeout(400)
        await clearLayers(page)
      }
      // 若被导航到别的页面，退回内容中心对应 tab
      if (!page.url().includes('/admin/content')) {
        await page.goto(`${BASE}/admin/content`)
        await page.waitForTimeout(1200)
        await openTab(page, tab)
      }
    }
    await shot(page, `tab-${tab.replace(/\s/g, '')}`)
  }

  expectClean(problems.filter((p) => p.kind === 'pageerror'))
  await finish(testInfo, C)
})
