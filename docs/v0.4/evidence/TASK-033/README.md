# TASK-033 五语言 / 双主题 / 三视口 / 三浏览器兼容验收证据

测试资产：`frontend/e2e/content-public-v04.spec.ts`（SSR/SEO/路由，12 用例）、`frontend/e2e/content-browser-v04.spec.ts`（浏览器 UI，11 用例）、`frontend/playwright.config.ts`（基址 3100；`E2E_EDGE=1` 注册 msedge，支持 `PLAYWRIGHT_EDGE_EXECUTABLE_PATH` 指定真实 Edge 二进制）。

本轮（attempt 3）完成 D3/D4 修复，并 **实际执行** Chromium / WebKit / Edge 三引擎矩阵（全部通过）。

> 本文件按 attempt 递增追加记录：§1～§5 为 attempt 3，§6 为 attempt 4（原生 select → common/Select），**§7 为 attempt 5（Safari Escape 修复 + 六项目 192/192 重跑，最终结论以 §7 为准）**。

## 1. 环境与运行

被测栈（真实运行，非 mock）：

| 角色 | 地址 | 说明 |
|---|---|---|
| SSR 公开页 + API（Go） | `http://127.0.0.1:8080` | 进程 `task033-s` |
| SPA（Vite dev） | `http://127.0.0.1:3100` | 公开站富交互层 |
| 测试数据 | news `ccai-api-gateway-news`、article `ccai-api-practice`（en 译文已发布，de 无译文） | 对应 R9-03/R9-04 场景 |

引擎版本（均实测 `browser.version()` 与 UA）：

| 项目 | 引擎 | 版本 | UA 关键标识 |
|---|---|---|---|
| chromium / tablet-chrome / mobile-chrome | Chromium（Playwright build 1228, Chrome for Testing） | 149.0.7827.55 | `HeadlessChrome/149.0.0.0` |
| webkit-desktop / webkit-mobile | WebKit（Playwright build 2336） | 26.5 | `Safari/605.1.15 Version/26.5` |
| msedge | Microsoft Edge（本机安装，非 Playwright 打包） | 156.0.4314.50 | `Edg/156.0.0.0` |

> 说明：本机 `playwright install chromium`（v1234）从 `cdn.playwright.dev` 下载停在 0%，改用已缓存的 Playwright Chromium build 1228（同属 Chromium 内核，经 `PLAYWRIGHT_EXECUTABLE_PATH` 注入），矩阵实际执行完成。

复跑命令：

```bash
cd frontend
# Chromium 三项目（69 条）
PLAYWRIGHT_EXTERNAL_SERVER=1 \
PLAYWRIGHT_EXECUTABLE_PATH="$HOME/Library/Caches/ms-playwright/chromium-1228/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing" \
pnpm exec playwright test e2e/content-browser-v04.spec.ts e2e/content-public-v04.spec.ts \
  --project=chromium --project=tablet-chrome --project=mobile-chrome

# WebKit 两项目（46 条）
PLAYWRIGHT_EXTERNAL_SERVER=1 \
pnpm exec playwright test e2e/content-browser-v04.spec.ts e2e/content-public-v04.spec.ts \
  --project=webkit-desktop --project=webkit-mobile

# 真实 Edge（23 条）
PLAYWRIGHT_EXTERNAL_SERVER=1 E2E_EDGE=1 \
PLAYWRIGHT_EDGE_EXECUTABLE_PATH="/Users/mac/Documents/Dev/sub2apis/.v04-tools/Microsoft Edge.app/Contents/MacOS/Microsoft Edge" \
pnpm exec playwright test e2e/content-browser-v04.spec.ts e2e/content-public-v04.spec.ts --project=msedge
```

## 2. 结果矩阵

| 引擎矩阵 | 用例数 | 结果 |
|---|---|---|
| Chromium（chromium / tablet-chrome / mobile-chrome） | 23 × 3 = 69 | **69 passed**（24.5s，EXIT=0） |
| WebKit（webkit-desktop / webkit-mobile） | 23 × 2 = 46 | **46 passed**（18.6s，EXIT=0） |
| Edge（msedge，真实 156） | 23 × 1 = 23 | **23 passed**（15.3s，EXIT=0） |
| 合计 | **138** | **138 passed / 0 failed** |

后端与类型：

| 检查 | 命令 | 结果 |
|---|---|---|
| Go handler 测试 | `go test ./internal/handler/...` | ok（handler 21.4s、admin 0.7s、dto 1.4s、quotaview 0.9s） |
| Go 路由测试 | `go test ./internal/server/...` | ok（routes 2.4s、middleware 0.5s） |
| 前端类型 | `pnpm exec vue-tsc --noEmit` | EXIT=0，无错误 |

23 用例覆盖：R7-01/02/03、R9-02～R9-09、R12-01/02/04/05/06、R13-01～03、R14-01/02、R15-01/02、COMP-01（五语言 zh/en/ru/fr/de × 双主题 × 三视口 1440/1024/390）。

## 3. 本轮修复项（D3 / D4）

| 编号 | 用例 | 根因（实测） | 修复 | 修复后证据 |
|---|---|---|---|---|
| D3 | R13-01 / R14-02 / COMP-01 | `.neo-sub-header-bar` 为三列 `grid: 1fr auto 1fr` + `padding: 14px 36px`，390px 下三列最小内容宽 `181.7 + 171.4 + 209.8 = 563px`，撑出 `scrollWidth=599`；越界元素 `.cta-wrap`(right=599,w=210)、`.lang-switcher`(right=532)、`.nav-home-btn`(right=453) | `PublicContentView.vue` 新增 `@media (max-width: 767px)`：页头改 `flex` 可换行（左侧品牌+面包屑一行、分段导航与 CTA 换行并 `margin-left:auto`），`≤420px` 再隐藏 `.seg-code`、收缩字号 | 390px 桌面 Chromium：`/news` docW **390**、`/articles` **390**、`/articles/zh/ccai-api-practice` **390**、`/news/zh/ccai-api-gateway-news` **391**（≤ winW+1），越界元素列表为空 |
| D3′ | R14-02（WebKit 独有回归） | 移动断点 `.more-related-grid { grid-template-columns: 1fr }` 的隐式 `min-width:auto` 被卡片内长 URL 的 min-content（WebKit 实测 **315.7px** > 容器 **232px**）撑破，390px 下 docW=392 | 改为 `minmax(0, 1fr)` + `.more-related-card{min-width:0}` + `.more-card-title/.more-card-excerpt{overflow-wrap:anywhere}` | WebKit 390px 详情页 docW **392 → 382**，越界元素为空；webkit 两项目 46/46 |
| D4 | R15-01/02 | 全局 `select:not([multiple])` 已有自绘箭头，但后台视图 scoped 规则 `.toolbar select[data-v-*] { background: #fff }`（简写）把 `background-image` 重置为 `initial`，且选择器优先级更高 | `style.css` 中箭头两条规则（浅色/暗色）的 `background-image` 加 `!important` 并注明原因 | `/admin/content` 首个 `<select>` 计算样式：`appearance:none`、`background-image:url("data:image/svg+xml,...")`、`background-size:12px 8px`、`background-position:calc(100% - 12px) 50%`、`padding-right:36px` |

用例健壮性调整（非放宽断言，**已被 attempt 4 取代**）：本小节记录的历史方案是对原生 `<select>` 使用 Playwright `selectOption()`。attempt 4 已将 R15 涉及的 7 个原生 `<select>` 全部迁移为 `common/Select`（自绘 listbox），`selectOption()` 方案随之作废并已从用例中删除。现行断言见 §6.2：真实鼠标点击 trigger → 选中项 → `toHaveAttribute('aria-expanded')` / trigger 文案 + 拦截 `/admin/content/articles?status=published` 请求；真实键盘 `Enter`/`ArrowDown`/`Enter`/`Space`/`Escape` 操作自绘 listbox。断言强度高于原 `selectOption()` 实现。

## 4. 复现命令（缺陷级）

```bash
# D3：390px 桌面视口读 document.documentElement.scrollWidth（修复前 /news=599，修复后 =390）
# D4：/admin/content 读首个 select 的 appearance / background-image / background-size
curl -s http://127.0.0.1:8080/articles/de/ccai-api-practice | grep -o '>Nachrichten<\|>Artikel<\|>Dokumentation<'   # D1（已修，德语页头）
curl -s http://127.0.0.1:8080/articles/en/ccai-api-practice | grep -o 'hreflang="[^"]*"'                        # D2（已修，en+zh+x-default）
```

## 5. 局限与风险

1. Chromium 项目使用 Playwright build 1228（149）而非 1234（151）：`cdn.playwright.dev` 下载停滞，未能在本轮升级；引擎族一致，且三视口/双主题/五语言断言全绿。
2. 真实 Edge 通过 `PLAYWRIGHT_EDGE_EXECUTABLE_PATH` 注入（headless），UA 含 `Edg/156.0.0.0`；有头 Edge 的窗口级验证未做（headless 已覆盖布局/样式/脚本错误）。
3. `R14-01` 判 PASS（公开页计算样式已无 `rgb(16,185,129)`），但 Tailwind 产物仍含 `.from-emerald-500` 等旧绿工具类（后台模块引用），属后台启用后的潜在残留，非本任务范围。
## 6. 本轮（attempt 4）R15 退回修复：7 个原生 select → common/Select

### 6.1 迁移清单（`frontend/src/views/admin/ContentCenterView.vue`）

| # | 位置 | 原实现 | 迁移后 | data-test |
|---|---|---|---|---|
| 1 | 新闻动态工具栏 | `<select v-model="newsStatus" @change="loadNews">` | `common/Select` + `onNewsStatusSelect`（先赋值再 `loadNews`） | `news-status` |
| 2 | 新闻源表单 | `<select v-model="sourceForm.sourceType">` | `common/Select` + `sourceTypeOptions` | `news-source-type` |
| 3 | 自动化设置 | `<select v-model="automation.publishMode">` | `common/Select` + `publishModeOptions` | `automation-publish-mode` |
| 4 | 文章列表工具栏 | `<select v-model="articleStatus" @change="loadArticles">` | `common/Select` + `onArticleFilterSelect` | `article-status` |
| 5 | 文章编辑弹窗·模型 | `<select v-model="articleForm.model">` | `common/Select` + `articleModelSelectOptions`（沿用已加载模型列表，含占位项） | `article-model` |
| 6 | 文章编辑弹窗·分类 | `<select v-model="articleForm.category">`（`.select-with-create`） | `common/Select` + `articleCategorySelectOptions`（保留「新建分类」输入+按钮） | `article-category` |
| 7 | AI 创作·brief 语言 | `<select v-model="briefLanguage">` | `common/Select` + `briefLanguageOptions` | `brief-language` |

迁移后 `grep -c '<select' src/views/admin/ContentCenterView.vue` = **0**；`<Select` 使用数 = **11**（7 新增 + 4 既有）。

样式：新增 `.toolbar-select{flex:0 0 auto;width:140px}` 与 `.toolbar-select :deep(.select-trigger){height:38px;padding-block:0;font-size:13px}`，使自绘下拉与同排输入框等高、不被 flex 拉宽；`.select-with-create > .relative{flex:1;min-width:0}` 替代原先对原生 `select` 的 `flex:1`。

### 6.2 断言（R15-01/02，`frontend/e2e/content-browser-v04.spec.ts`）

- 视图内 `select` 元素计数 **0**（新闻动态、新闻源、抓取任务、自动化设置、分类与标签、AI 创作、BI 分析逐一切换断言）。
- **真实鼠标**：点击 `news-status` trigger → 选中「已发布」→ trigger 文案更新 + 拦截到 `/admin/content/articles?status=published` 请求（`expect.poll`）。
- **真实键盘**：`Enter` 打开 → `ArrowDown` 循环至「已发布」（不依赖组件初始高亮索引）→ `Enter` 确认；再次 `Space` 打开校验 `aria-selected` → `Escape` 关闭。
- 文章编辑弹窗：`article-model` / `article-category` 均为自绘下拉且可选中。
- **布局不变量**：工具栏 trigger 与 input 高度差 ≤6px、trigger 宽度 ≤200px、`documentElement.scrollWidth - clientWidth ≤ 0`。

### 6.3 单测（`frontend/src/views/admin/__tests__/ContentCenterView.spec.ts`）

新增 `R15 内容中心下拉统一为 common/Select（AC-14）` 2 条：挂载后断言 `wrapper.findAll('select')` 为 0、`common/Select` 承载 6 个筛选/表单下拉；并驱动 `Select` 的 `update:modelValue` + `change` 双 emit，验证父层 `loadNews/loadArticles` 重新拉取。
`AIConfigPanel` 挂载时会自行调用 `listAIConfigs` 并回填（空结果会盖掉注入的默认模型），故在 spec 内 mock 该接口返回默认模型配置，使 brief 面板真实渲染。

### 6.4 验证记录（本轮实测）

| 项 | 命令 | 结果 |
|---|---|---|
| 单测（内容中心） | `pnpm vitest run src/views/admin/__tests__/ContentCenterView.spec.ts` | **12 passed** |
| 前端类型 | `pnpm exec vue-tsc --noEmit` | EXIT=0，无错误 |
| E2E 契约矩阵 | `playwright test e2e/content-admin-v04 e2e/content-public-v04 e2e/content-browser-v04 --project=chromium --project=tablet-chrome --project=mobile-chrome --project=webkit-desktop --project=webkit-mobile --workers=1` | **160 passed（2.3m）** |
| 全量单测 | `pnpm vitest run` | 940 passed / 3 failed（`src/i18n/__tests__/userLocaleParity.spec.ts`，de/fr/ru 与 en 键集不一致，属既有问题：本轮未改动任何 i18n 文件，`git status` 无 i18n 变更） |

### 6.5 attempt 4 遗留（已在 attempt 5 关闭，见 §7）

- ~~WebKit(Safari) 鼠标展开后 `Escape` 不生效~~ → **已在 attempt 5 修复**：`common/Select.vue` 打开态改为在 `document` 捕获阶段监听 `keydown`；E2E 中的人为 `focus()` 已删除，改为真实鼠标展开后直接按 Esc。详见 §7。
- ~~本机未安装 Microsoft Edge~~ → **该结论有误**：真实 Edge 已通过项目私有二进制 `.v04-tools/Microsoft Edge.app/Contents/MacOS/Microsoft Edge`（v156，UA `Edg/156.0.0.0`）运行，主会话六项目契约矩阵 **192/192 passed**（日志 `/tmp/v04-033-main-matrix.log`）。


## 7. 本轮（attempt 5）Safari Escape 缺陷修复 + 六项目重跑

### 7.1 缺陷与根因（实测，非推断）

- 现象：WebKit(Safari) 下「真实鼠标点开 `common/Select` → 直接按 Esc」下拉不关闭；Chromium 正常。
- 根因：Safari 对 `<button>` 的鼠标点击**不赋予焦点**（`document.activeElement` 仍为 `body`），而 `Select.vue` 的 Escape 关闭逻辑原先只绑在 trigger / dropdown 自身的 `keydown` 上，事件根本到不了组件。
- attempt 4 曾在 E2E 里用 `newsStatus.focus()` 绕过，属**人为掩盖**，本轮已删除。

### 7.2 修复（`frontend/src/components/common/Select.vue`）

打开态（`watch(isOpen)` 为 true）在 `document` 上注册**捕获阶段** `keydown`，关闭态移除，`onUnmounted` 兜底清理：

```ts
const closeOnEscape = () => {
  isOpen.value = false
  triggerRef.value?.focus({ preventScroll: true })   // 无障碍：焦点回到 trigger
}

const handleDocumentKeyDown = (event: KeyboardEvent) => {
  if (!isOpen.value || event.key !== 'Escape') return
  event.preventDefault()
  event.stopPropagation()   // 下拉消费该次 Esc，避免连带关闭外层 BaseDialog
  closeOnEscape()
}
```

选择「捕获阶段 + stopPropagation」的原因：`BaseDialog.vue` 在 `document` 上以**冒泡**阶段监听 `keydown` 并调用 `stopImmediatePropagation()`；捕获阶段可先于它执行，既保证下拉关闭，又能阻止外层对话框同时关闭。`DateRangePicker.vue` / `ProxySelector.vue` 已有 `document keydown` 先例，此处保持同一模式。

### 7.3 先红后绿（同一份 E2E，未改断言强度）

E2E（`frontend/e2e/content-browser-v04.spec.ts`，R15-01/02）删除人为 `focus()` 后：

| 阶段 | 命令 | 结果 |
|---|---|---|
| 红（组件未修） | `playwright test e2e/content-browser-v04.spec.ts --project=webkit-desktop -g "R15-01/02" --workers=1` | **1 failed** — `Escape 关闭下拉（无需事先聚焦）`：`aria-expanded` Expected `"false"` / Received `"true"`（日志 `/tmp/v04-033-a5-red.log`） |
| 绿（修复后，WebKit） | 同上 | **1 passed (3.6s)**（`/tmp/v04-033-a5-green-webkit.log`） |
| 绿（修复后，Chromium） | `--project=chromium` | **1 passed (3.6s)**（`/tmp/v04-033-a5-green-chromium.log`） |

新增断言（真实鼠标路径，不加 focus）：`aria-expanded=false`、dropdown 卸载（`toHaveCount(0)`）、已选值不被清空、二次 Esc 不重开、URL 仍为 `/admin/content`。

### 7.4 单测回归（`frontend/src/components/common/__tests__/formControls.spec.ts`）

新增 1 条 `Escape closes an open Select without relying on focus inside the component (Safari mouse click)`：

- 点击 trigger 打开后主动 `blur()`，断言 `document.activeElement === body`（复刻 Safari 无焦点场景）；
- 在 `body` 上派发 `Escape` → `aria-expanded=false`、不 emit `update:modelValue`（不清空已选值）、焦点回到 trigger；
- 注册一个外层冒泡 `keydown` 探针，断言**未被调用**（Esc 被下拉消费）；
- 关闭态再按 Esc → 探针**被调用 1 次**（不吞掉页面其它键盘监听）。
- 用例用 `try/finally` + `unmount()` 清理，避免挂载残留污染后续用例。

结果：`vitest run src/components/common/__tests__/formControls.spec.ts` → **9 passed**。

### 7.5 全量矩阵重跑（修复后，六项目）

```bash
cd frontend
PLAYWRIGHT_EXTERNAL_SERVER=1 \
PLAYWRIGHT_EXECUTABLE_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
E2E_EDGE=1 \
PLAYWRIGHT_EDGE_EXECUTABLE_PATH="/Users/mac/Documents/Dev/sub2apis/.v04-tools/Microsoft Edge.app/Contents/MacOS/Microsoft Edge" \
pnpm exec playwright test e2e/content-admin-v04.spec.ts e2e/content-public-v04.spec.ts e2e/content-browser-v04.spec.ts \
  --project=chromium --project=tablet-chrome --project=mobile-chrome \
  --project=webkit-desktop --project=webkit-mobile --project=msedge --workers=1
```

| 检查 | 结果 |
|---|---|
| 六项目契约矩阵（9 + 12 + 11 = 32 用例 × 6 引擎） | **192 passed (2.7m)**，日志 `/tmp/v04-033-a5-matrix.log`；主会话首跑同一矩阵 192 passed（`/tmp/v04-033-main-matrix.log`） |
| `vitest run src/views/admin/__tests__/ContentCenterView.spec.ts` | **12 passed** |
| `vue-tsc --noEmit` | **EXIT=0**，无类型错误 |

引擎：Chromium（本机 Google Chrome 二进制注入）× 3 项目、WebKit（Playwright build 2336，v26.5）× 2 项目、真实 Edge v156（`.v04-tools` 私有二进制，UA `Edg/156.0.0.0`）× 1 项目。

### 7.6 已知无关波动（不影响结论）

- `R13-02 精选徽章柠绿底深色字` 在 `--workers=2` 并行跑时曾出现 1 次失败（`dark=false 柠绿徽章存在` 取到 `null`）；单用例 `--repeat-each=3 --workers=1` **3/3 通过**，随后 `--workers=1` 的 192 矩阵全绿。属公开页数据加载与断言之间的并行竞态，与 `Select.vue` 改动无因果关系（该用例不涉及下拉组件，仅读公开页徽章计算样式），本任务未改动其代码。

## 8. 本轮（attempt 6）R9-02 测试数据依赖修复

### 8.1 失败现象与根因

主验收矩阵（189 passed / 3 failed）与 chromium 重跑中，`content-public-v04.spec.ts` 的 `R9-02 仅匹配当前 source_version 的译文对外发布` 稳定失败：`已发布译文数量 Expected >0 Received 0`（证据：`docs/v0.4/evidence/TASK-035/browser-matrix.log`、`r9-02-isolated.log`）。

根因是**测试数据依赖**，不是产品行为：旧用例只扫描后台文章列表第 1 页前 100 条，并对前 3 条 `status=published` 的译文断言；当样本集里没有已发布译文（或译文全部落后于当前文章版本）时 `checked` 恒为 0，断言必然失败。种子数据/发布顺序一变，结果就跟着漂移。

### 8.2 修复方式：自包含夹具（不再依赖现有 100 篇）

新用例通过管理员 API 自建独立数据，slug 唯一（`e2e-r9-02-<ts>-<rand>`），并发跑不串扰，`try/finally` 结束即删：

1. `POST /api/v1/admin/content/articles`（draft）→ `publish`：版本 +1（publish 走 `AddVersion(1)`）
2. `POST .../{id}/translations`（`sourceVersion` = 当前版本，`status=translation_draft`）→ `POST .../translations/{tid}/publish`
3. **当前版本分支**断言：`/articles/en/<slug>` 渲染英文正文、`index, follow`、self-canonical 指 `.../articles/en/<slug>`
4. `unpublish` → `publish`（各 +1 版本）：译文仍是 `published` 但 `source_version` 落后
5. **旧版本分支**断言：SSR 回落中文正文、`noindex, follow`、canonical 指 `.../articles/zh/<slug>`，且**不含**英文正文
6. `finally`：DELETE 译文 + DELETE 文章（软删）

关键点：第 4 步而不是"造一条落后版本的译文"——实测 `POST translations` 用非当前 `sourceVersion` 会被拒绝（见 §8.4），旧版本译文只能由"发布译文 → 文章版本继续推进"这一真实路径产生，正好复刻产品语义。

断言强度（避免"checked>0 就算过"）：两个分支都是**精确取值断言**（canonical 具体 URL、robots 具体值、正文语种命中/不命中），不是"存在即通过"；并且用 DB/API 侧证 `status=published && sourceVersion < article.version`，把"是版本门禁生效"和"译文根本没发布"区分开。

### 8.3 实测记录（本机 external server：SSR :8080 / admin :3100）

```bash
cd frontend
# 单用例 × 三引擎 × 重复 2/3 次（稳定性）
E2E_EDGE=1 pnpm exec playwright test e2e/content-public-v04.spec.ts -g "R9-02" \
  --project=chromium --project=webkit-desktop --project=msedge --repeat-each=2 --reporter=line
# 全文件 chromium 回归
E2E_EDGE=1 pnpm exec playwright test e2e/content-public-v04.spec.ts --project=chromium --reporter=line
```

| 检查 | 结果 | 证据 |
|---|---|---|
| R9-02 × chromium / webkit-desktop / msedge（repeat-each=2） | **6 passed (873ms)**，5 workers 并发 | `docs/v0.4/evidence/TASK-033/r9-02-matrix.log` |
| R9-02 × 同上三引擎（repeat-each=3） | **9 passed (1.1s)** | `docs/v0.4/evidence/TASK-033/r9-02-stability-x3.log` |
| `content-public-v04.spec.ts` 全文件 chromium（12 用例） | **12 passed (474ms)** | `docs/v0.4/evidence/TASK-033/full-file-chromium.log` |
| 夹具数据清理核对（Postgres 直查） | `content_articles/content_translations` 中 `slug like 'e2e-r9-02-%'` 且 `deleted_at is null` 的记录 **0 条** | 见下 |

清理核对输出：

```
articles leftover(total incl deleted): 12        # 均为软删（status=deleted, deleted_at 非空）
live leftovers: 0
live translation leftovers: 0
live e2e-% articles: []                           # 全库无活跃 e2e-* 夹具
```

### 8.4 夹具链路实测证据（API 契约坑）

`docs/v0.4/evidence/TASK-033/r9-02-fixture-evidence.log`（一次性脚本实跑，非推断）：

```
create article 201 id=164 version=1
publish article 200 version=2
create translation 201 id=25 sourceVersion=2 status=translation_draft
publish translation 200
[CURRENT] GET /articles/en/e2e-r9-02-evidence-1793097174 200
          canonical=['https://ccai.xyz/articles/en/e2e-r9-02-evidence-1793097174']
          robots=['index, follow'] en_body=True zh_body=False
unpublish article 200 version=3
republish article 200 version=4
[DB] translation status=published sourceVersion=2 article version=4
[STALE]   GET /articles/en/e2e-r9-02-evidence-1793097174 200
          canonical=['https://ccai.xyz/articles/zh/e2e-r9-02-evidence-1793097174']
          robots=['noindex, follow'] en_body=False zh_body=True
cleanup article 204 (soft delete)
```

按此实测确认的契约（踩坑记录，供后续复用）：

- 译文 `status` 枚举是 `translation_draft` / `translation_review` / `published` / `unpublished` / `deleted`，**没有** `draft`；写 `draft` 会被判 `invalid content article input`。
- 直接以 `published` 创建译文会撞 DB 约束（`published_at` 必填）→ 必须"先建 draft 再 `POST /translations/{id}/publish`"。
- `publish` / `unpublish` 每次都 `AddVersion(1)`，因此"unpublish→publish"是最省事的版本推进手段（2 次调用 +2 版本）。
- `POST translations` 传非当前 `source_version` 会被 `validateTranslationSource` 拒绝，且错误被映射成 **500 `internal error`**（日志：`admin/content_article_handler.go:338 [ERROR] ... Error: invalid content article input`）。这是**error→HTTP 映射的既有小缺陷**（无效入参应 4xx），不在本任务 allowed_files（后端）内，此处仅记录，不扩大改动范围；对 R9-02 无影响（用例只用当前版本建译文）。

### 8.5 局限与风险

- 用例依赖管理员账号 `S2A_E2E_EMAIL/S2A_E2E_PASSWORD`（默认 `e2e-task031@sub2api.local`）。登录失败会**硬失败**而不是 skip（避免再次用 skip 掩盖数据/权限问题）；如换环境需同步注入 env。
- 夹具在共享库上每次留下一条**软删**文章行（`status=deleted`），与产品自身删除语义一致；如需彻底清空需单独清理作业。
- 用例依赖 `publish/unpublish` 各 +1 版本的实现细节（当前 `SetArticleStatus` 行为）。若产品改为"版本只在正文变更时递增"，第 4 步需改为编辑正文触发版本推进。
- 本次未重跑六项目全量矩阵（handoff 只要求该用例三引擎 + 全文件 chromium），全量矩阵请以主会话 TASK-035 结论为准。
