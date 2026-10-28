# W5 公开站（Public Frontend）真实 E2E 验收报告

- 版本：v0.4 / W5
- 执行时间：2026-10-28（全量一轮 125.5s）
- 用例来源：`frontend/e2e/acceptance/w5-public.spec.ts`（PUB-01 ~ PUB-16，Chromium、`--workers=1`、真实 Chrome）
- 被测环境（双前台并存，均为真实运行进程，无 mock）：
  - A. Vite SPA `http://127.0.0.1:3100`：公开页 `/`、`/news`、`/articles`、`/articles/:locale/:slug` 由 `PublicContentView.vue` 渲染
  - B. Go SSR 宿主 `http://127.0.0.1:8080`：`/en/`、`/en/news`、`/zh/articles` 等前缀页 SSR 输出
- 截图（36 张）：`e2e-acceptance-v04/shots/w5/`
- 结论：**16 条用例 12 通过 / 4 失败**（PUB-04、PUB-06、PUB-11、PUB-14），失败项均定位到下表中的真实产品缺陷，非脚本口径问题。

---

## 一、总览

| # | 用例 | 结果 | 关键实测 |
|---|------|------|----------|
| PUB-01 | 语言前缀直访 200 / 无前缀 301 | ✅ | `/en/news` `/zh/articles` `/en/` `/ru/news` `/fr/articles` `/de/articles` 全 200；`/news`→301 `/zh/news`、`/articles`→301 `/zh/articles` |
| PUB-02 | `/articles/en/<slug>` SSR 含英文正文 | ✅ | status 200、`html lang=en-US`、正文汉字数 0 |
| PUB-03 | 无译文语言回落中文但界面文案跟随 URL | ✅ | ru/fr/de 均 200，nav 分别 `Новости/Actualités/Nachrichten`，正文汉字数 25（回落中文） |
| PUB-04 | 列表每页 20 条 + 分页控件 + page 边界 | ❌ | SSR `/zh/articles?page=999`→`{"status":200,"articles":18,"hasPager":false}`；SPA 每页 5 张卡 |
| PUB-05 | 英文列表总数 = 全部文章数 | ✅ | API total en/zh/ru = 93/78/78，五语种并集 93，英文接口 93 条（其中 6 条中文标题回落） |
| PUB-06 | 列表卡左封面 240px 16:9、详情 ≤400px | ❌ | 列表 5 行 `hasCover:false`（占位元素 10 个）；详情封面 `w=1050, h=400` |
| PUB-07 | 精选区：柠绿胶囊 + 1 大 9 小 + 倒序 | ✅ | pill `rgb(204,255,0)`/字 `rgb(18,19,22)`/`999px`；`largeCount:1 smallCount:9`；大卡 453.59 / 区块 1152 = 39.4% |
| PUB-08 | 精选卡对比度 ≥4.5 双主题 | ✅ | 10 张卡 fg `rgb(18,19,22)` / bg `rgb(255,255,255)` ratio 18.58（双主题一致） |
| PUB-09 | 精选卡悬停上浮 + 阴影加深 + 可点进入 | ✅ | `none → matrix(1,0,0,1,-2,-2)`；阴影 `4px → 7px`；点击后进 `/articles/en-US/task031-featured-2-1576587` |
| PUB-10 | 390×844 精选区布局 | ✅ | 大卡 342 / 视口 390、列数 1、横向溢出 0 |
| PUB-11 | 详情「更多深度解析」6 条 + 上下篇可点 | ❌ | SPA 正常（6 链接、不含当前、2 张上下篇均可点）；**SSR 源码不含「更多深度解析」「上一篇」** |
| PUB-12 | 语言切换器五项顺序 + 写 cookie + 跳前缀 URL | ✅ | 去重后顺序 `English/中文/Русский/Français/Deutsch`；切 ru → `/ru`、`galley_locale=ru`、`sub2api_locale=ru`、`html lang=ru`；再切 en → `/en`、cookie=en；三个页面路由均复现 |
| PUB-13 | 全站样式扫描（原生 select/单下边框/#10B981/36±1） | ✅ | 原生 select 0、单下边框输入框 0、`#10B981` 命中 0；按钮高度有 38/34/48 等偏差（见风险 R3） |
| PUB-14 | 正文对比度抽样（前 30 文本节点，双主题） | ❌ | light 首页 9 处违规（最低 2.95）、`/news` 2 处、`/articles` 1 处；dark 首页 11 处（最低 1.07） |
| PUB-15 | 键盘 Tab 覆盖导航/语言/卡片/分页且焦点可见 | ✅ | 前 40 次 Tab 命中 nav(1)/lang(4)/card(10)/pager(25)；无焦点样式计数 0/59 |
| PUB-16 | 详情停留 ≥10s 有埋点（referrer/时长/语言） | ✅ | 3 条事件 `page_view/page_leave/page_view`；`locale:"zh"`、`payload.duration_ms`、`referrerHost:"www.google.com"` |

---

## 二、缺陷明细（4 项，含复现与取证）

### D1（PUB-04，P1）SSR 列表页无分页且 `page` 参数不生效；SPA 每页仅 5 条
- 证据：
  - `GET http://127.0.0.1:8080/zh/articles?page=999` → 200，页面文章 **18 条**、无分页控件
  - `GET http://127.0.0.1:8080/zh/articles` → 20 条（第 1 页）
  - SPA `/articles` 第 1 页卡片 **5** 张，页码 `["1","2","3","4","5"]` 正常、当前页高亮正常（`rgb(255,255,255)` / bg `rgb(18,19,22)`）
  - 边界：SPA `page=999`→0 张、`page=0`→1、`page=abc`→1（SPA 侧兜底正确）
- 判定：SSR 与 SPA **两套分页口径不一致**（SSR 20/页无控件、越界回落到全量；SPA 5/页），且需求口径为 20/页 + 分页控件。SSR 越界返回 18 条属参数未生效，SEO 与收录会产生重复内容。
- 复现：`curl -s 'http://127.0.0.1:8080/zh/articles?page=999' | grep -c '<article'`；截图 `PUB-04-01/02/03`.

### D2（PUB-06，P1）列表卡片完全没有封面图；详情封面宽度超限
- 证据（light/dark 一致）：列表 5 行样本 `hasCover:false` ×5，全站无封面行 9 行 + 10 个占位元素；详情页封面 `{"has":true,"w":1050,"h":400}`（内容区 1152 宽，等于整行铺开）
- 判定：① 列表卡左侧 240×135（16:9）封面槽位未渲染真实图（只有占位）；② 详情封面宽度 1050 > 400 上限。
- 复现：打开 `/articles` 双主题截图 `PUB-06-01-list-light.png` / `PUB-06-03-list-dark.png`；详情 `PUB-06-02/04`.

### D3（PUB-11，P2）详情页 SSR 不输出「更多深度解析」与「上一篇/下一篇」
- 证据：`GET http://127.0.0.1:8080/zh/articles/acc-w2-pub-mvs73a0w` 源码 `含更多深度解析=false`、`含上一篇=false`；同一 URL 走 SPA 后 `SECTION.detail-more-section` 存在、6 条相关推荐、不含当前文章、上下篇 2 张卡均可点
- 判定：内链仅客户端渲染，爬虫拿不到站内互链（SEO 内链结构缺失）。SPA 侧本身符合需求。
- 复现：`curl -s 'http://127.0.0.1:8080/zh/articles/acc-w2-pub-mvs73a0w' | grep -c 更多深度解析`；截图 `PUB-11-01-detail-related.png`.

### D4（PUB-14，P1）正文/徽标对比度不达 WCAG AA 4.5
- light 首页 9 处：白字压红底 3.55（`rgb(255,255,255)` on `rgb(255,59,48)`，含 🤯/Direct 徽标）、红字压浅粉底 2.95（`rgb(255,59,48)` on `rgb(255,228,225)`，标题与 429 限流/账单难控等标签）
- light `/news` 2 处：序号 `[02]` 3.08（`rgb(140,147,164)` on 白）、`BREAKING` 3.55；`/articles` 与详情页序号 `[01]` 各 1 处 3.08
- dark 首页 11 处，其中 **1.07**：`rgb(248,250,252)` 文字压在 `rgb(245,242,235)`（浅色底）——深色主题下该区块底色未切换，几乎不可读
- 判定：红色系标签/徽标与浅色序号为系统性配色问题；dark 主题存在「浅底浅字」区块，是主题切换漏项。
- 复现：截图 `PUB-14-01..08`。

---

## 三、风险与观察（未计入失败）

- **R1 语言切换器 DOM 双份**：`.lang-menu` 在桌面 + 移动两处同时挂载，切换器选项在 DOM 中重复出现（`English/English/中文/中文/...`）。视觉无碍，但读屏与自动化会重复播报；建议 `v-if` 按断点只渲染一套。
- **R2 切换语言存在整页跳转**：`/news` 切 Русский 会被 `window.location.assign('/ru/news')` 整页加载（SPA 未注册前缀列表路由），在 3100 端口该 URL 落 404 页；SSR 宿主（8080）该 URL 200。生产环境若前置 SSR 则正常，纯 SPA 部署会出现 404，需明确部署形态。
- **R3 控件高度口径**：`lang-switch-btn`/`pager-btn`/`pager-num` 实测 38px（要求 36±1），`sw-btn` 34px、`big-trigger-btn` 48px、`action-pill-btn` 35px。PUB-13 用例按「无原生 select / 无单下边框 / 无 #10B981」判定通过，高度偏差建议单独拉平。
- **R4 内容数据**：全部精选与多数文章标题仍是 `TASK031-featured-*` 合成数据，pub 语言切换后的“英文正文”仅 6 条走中文回落；PUB-02/03/05 的语言链路结论基于这些数据成立，但不代表真实译文质量。
- **R5 SSR 极简**：8080 宿主页无 CSS、无精选区、无语言切换器（PUB-01/04/11 部分结论由此得出），与 SPA 视觉不一致，需确认是否仅为过渡实现。

---

## 四、如何复跑

```bash
cd /Users/mac/Documents/Dev/sub2apis/frontend
export PATH=/usr/local/bin:/opt/homebrew/bin:$PATH
export PLAYWRIGHT_EXECUTABLE_PATH='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
export PLAYWRIGHT_EXTERNAL_SERVER=1
export E2E_REAL_OUT=/Users/mac/Documents/Dev/sub2apis/e2e-acceptance-v04/shots
# 全量
npx playwright test e2e/acceptance/w5-public.spec.ts --project=chromium --workers=1 --trace=off --reporter=line
# 单条
npx playwright test e2e/acceptance/w5-public.spec.ts --grep "PUB-14" --workers=1
```

前置：3100（Vite SPA）与 8080（Go SSR）两个真实进程需在跑。
