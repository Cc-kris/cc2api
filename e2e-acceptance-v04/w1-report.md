# W1 新闻编辑弹窗 — 真实 E2E 验收报告

- 执行时间：2026-10-28
- 被测环境：本地栈 `http://127.0.0.1:3100`（前端）+ admin 接口，真实登录、真实数据库
- 用例文件：`frontend/e2e/acceptance/w1-news-editor.spec.ts`（NE-01 ~ NE-19）
- 执行方式：`PLAYWRIGHT_EXTERNAL_SERVER=1 npx playwright test e2e/acceptance/w1-news-editor.spec.ts --project=chromium --workers=1`
- 数据构造：beforeAll 用真实登录态调 API 造 2 条 `ACC-W1` 新闻，afterAll 删除（`DELETE /admin/content/news/:id`）
- 原始测量：`e2e-acceptance-v04/w1-measurements.jsonl`（189 条 DOM/样式实测记录）
- 截图/视频/trace：`frontend/test-results/playwright/acceptance-w1-news-editor-*`

## 一、总览

**19 条用例：4 PASS / 15 FAIL。** 全部为真实浏览器点击 + CDP 取值，无 mock、无接口直塞伪造。

| 编号 | 验收点 | 结论 | 关键实测 |
|---|---|---|---|
| NE-01 | 弹窗多视口尺寸/圆角/居中/小屏不溢出 | **FAIL** | 圆角实测 `14px`，要求 `12px`；居中与小屏不溢出 PASS |
| NE-02 | 左右两栏布局与左栏字段顺序 | **FAIL** | 左栏实测宽 `828px`（要求 320±8）；两栏未并排（y 相差 210px，纵向堆叠） |
| NE-03 | 分类、状态使用统一 Select | **FAIL** | 弹窗无「状态」字段；分类是 `input + datalist`，非统一 Select |
| NE-04 | 标签词条式 TagInput | **FAIL** | 无 chip 元素，实测普通 input，placeholder「多个标签用英文逗号分隔」 |
| NE-05 | 同步语言多选（En/Ru/Fr/De） | **FAIL** | 弹窗内 checkbox 数 `0` |
| NE-06 | 译文面板不挤压编辑区 | **FAIL** | 1440×900：面板与正文编辑器相交 `75px`，编辑器可视高度仅 `15px`；1024×768 可视高度 `0px` |
| NE-07 | 编辑器占满剩余高度 + 3000 字内部滚动 | **FAIL** | 贴入 3000 字后外框高度变化 `112px`（应不变，内部滚动） |
| NE-08 | 工具栏 10 项 + 逐项插入标签 | **FAIL** | 实测仅 6 项：加粗/斜体/下划线/项目列表/编号列表/插入链接；缺 H2、H3、引用、插入图片 |
| NE-09 | 编辑区可聚焦/输入/撤销 | **PASS** | contenteditable=DIV，输入与撤销正常 |
| NE-10 | 粘贴 script/onclick/style 后不落库 | **FAIL** | `script`/`onclick` 被清，但 `style` 属性残留：`<p style="color:red">…</p>` |
| NE-11 | 插图：大小/格式校验 + 走上传接口 + base64 不内嵌 | **FAIL** | 编辑器无上传入口（file input 数 0，工具栏无「插入图片」）；保存后 `bodyHtml` 仍内嵌 `data:image/png;base64,…` |
| NE-12 | ≥10000 字正文往返逐字一致 | **PASS** | API bodyText 10518 字符 vs 归一化后 9759，无丢失 |
| NE-13 | 四种关闭方式（无修改）均可关闭 | **PASS** | ×/取消/遮罩/Esc 均可直接关闭 |
| NE-14 | 有修改时四种关闭方式都弹未保存确认 | **FAIL** | ×/取消/点遮罩/Esc 四种方式**都没有**弹「有未保存修改，确认离开？」 |
| NE-15 | 改标题保存：关弹窗 + 顶部已保存 + 列表刷新 | **PASS** | 保存成功、弹窗关闭、草稿状态保持不变 |
| NE-16 | 底部按钮顺序/尺寸/颜色 + 标题必填红星 | **FAIL** | 按钮顺序实测 `["保存新闻","取消"]`（保存左、取消右，要求相反）；标题无必填红星（`titleStar=false`），失焦空标题无提示 |
| NE-17 | 输入框 2px/8 圆角 + 深色对比度 ≥4.5 | **FAIL** | 浅色下 2px/8px/42px 通过、对比度 18.58 通过；但**深色模式下弹窗整块仍是白底**（`html.dark` 已生效，输入框/正文/工具栏 bg 仍 `rgb(255,255,255)`，亮度 1.0）→ 深色主题未覆盖弹窗 |
| NE-18 | Tab 焦点顺序且不逃出弹窗 | **FAIL** | Tab 22 次跑到弹窗外的后台侧栏（Dashboard/Ops/Users/…），且 Tab 序列未经过正文编辑区 |
| NE-19 | 保存后前台详情渲染 h2/li/引用/图片/链接（浅+深） | **FAIL** | 公开接口只返回 `bodyText`，无 `bodyHtml`；前台详情 h2/li/bq/a/img 全部缺失，只剩纯文本 |

## 二、缺陷清单（按严重度）

### P0 — 功能不可用 / 数据安全

1. **NE-11 富文本无插图能力**：编辑器工具栏没有「插入图片」，没有 file input，用户无法插图；同时 base64 图片未被清洗，直接入库内嵌（体积膨胀 + 可被塞入任意 data URI）。
2. **NE-19 富文本结构在落地后被抹平**：公开详情接口只给 `bodyText`，`hasBodyHtml=false`。后台精心排版的标题层级、列表、引用、图片、链接在前台全部丢失 → 富文本编辑器的价值归零。
3. **NE-10 XSS 清洗不彻底**：`style` 属性未被剥离（`<p style="color:red">`），可被用于视觉欺骗/部分 CSS 注入；`script`/`onclick` 已正确清除。
4. **NE-14 无未保存离开保护**：四种关闭方式全都不弹确认，用户误操作即丢失全部编辑内容（这是编辑器最基本的保护）。
5. **NE-06 译文面板压塌编辑区**：1440×900 下与正文相交 75px、编辑器可视高度只剩 15px；1024×768 下可视高度 0px —— 等于在常见分辨率下**无法编辑正文**。

### P1 — 布局与交互不符合设计

6. **NE-02 两栏布局未实现**：左栏（配置栏）实测 828px 宽，且两栏纵向堆叠（y 差 210px），完全不是「左 320 配置栏 + 右自适应编辑区」。
7. **NE-08 工具栏缺项**：只有 6 项，缺 H2/H3/引用/插入图片。
8. **NE-16 底部按钮顺序与设计相反**：实测「保存新闻」在左、「取消」在右；且标题没有必填红星、空标题失焦无校验提示。
9. **NE-03 / NE-04 / NE-05 表单控件未按设计实现**：缺「状态」字段；分类是 `input+datalist` 而非统一 Select；标签是逗号分隔文本框而非词条 chip；同步语言多选（4 个语言 checkbox）完全缺失。
10. **NE-07 高度自适应失效**：贴入 3000 字后弹窗外框长高 112px 而非内部滚动。
11. **NE-18 焦点管理**：Tab 会跑到弹窗外 22 个后台元素，且焦点顺序没经过正文编辑区 → 键盘用户无法用 Tab 进入编辑区。

### P2 — 视觉细节

12. **NE-01 弹窗圆角 14px ≠ 设计 12px**（1440 与 1024 都是 14px）。
13. **NE-17 深色主题未覆盖弹窗**：全局 `html.dark` 已加、页面 body 背景变 `rgb(15,18,24)`，但新闻编辑弹窗内输入框/正文/工具栏仍是纯白 `rgb(255,255,255)`。深色模式下这个弹窗等于「白板」。（浅色态下 2px 边框、8px 圆角、42px 高度、18.58:1 对比度全部达标。）

## 三、已达标项（PASS 明细）

- **NE-09**：编辑区真实可聚焦、可输入、Ctrl/⌘+Z 可撤销，不是只读空壳。
- **NE-12**：≥10000 字长文保存往返，字符级无丢失（10518 → 归一化 9759，差异来自 HTML 标签/空白归一化）。
- **NE-13**：无修改时 ×/取消/点遮罩/Esc 四种关闭方式都能直接关掉。
- **NE-15**：改标题后保存 → 弹窗关闭、顶部出现「已保存」、列表刷新、草稿状态未被误改。

## 四、复现与证据

```bash
cd frontend
PLAYWRIGHT_EXTERNAL_SERVER=1 npx playwright test e2e/acceptance/w1-news-editor.spec.ts \
  --project=chromium --workers=1 --reporter=line
```

- 测量数据：`e2e-acceptance-v04/w1-measurements.jsonl`（每条用例的 DOM 矩形、computed style、对比度计算均落盘，可直接核对上表数字）
- 失败截图：`frontend/test-results/playwright/acceptance-w1-news-editor-*/test-failed-*.png`
- 失败 trace：同名目录下 `trace.zip`（`npx playwright show-trace <path>`）

## 五、验收方法与限制说明

- 全部用例走真实 UI：真实登录 → 内容中心 tab → 点「编辑」打开弹窗，断言值来自 `getComputedStyle` / `getBoundingClientRect`，不读源码常量。
- **主题开关**：后台的主题切换在左侧边栏底部（`button.sidebar-link`，文案「深色模式/浅色模式」，见 `AppSidebar.vue:146`）。弹窗打开时遮罩会挡住该按钮，因此深色对比度测量采用「先关弹窗 → 点侧栏开关 → 重开弹窗」的真实操作路径，未直接改 class。
- **NE-19 深色段例外**：前台详情页本身没有主题开关（匿名访客跟随系统），该段用 `document.documentElement.classList.add('dark')` 模拟系统深色，已在用例内注明。这一条不影响其结论——失败原因是接口只给 `bodyText`，与主题无关。
- 每轮全量跑之前会清理残留弹窗（`afterEach` 点「取消」收尾），避免上一条用例留下的弹窗遮罩导致下一条超时误判。
