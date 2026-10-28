# v0.4 E2E 验收报告 · W2 文章编辑（标签 / SEO / 精选 / 文章翻译）

- 执行日期：2026-10-28 22:20–22:35（本机）
- 用例来源：`docs/v0.4/v0.4_e2e_acceptance_cases_20261028.md` W2（AE-01 ~ AE-19）
- 被测对象：内容中心 → 文章管理（本地栈 :3100 → :8080 → PostgreSQL）
- 执行方式：真实登录真实接口，无 mock、无注入 token；`workers=1`

## 1. 结论

| 项 | 值 |
|---|---|
| 用例总数 | 19 |
| PASS | **6**（AE-04 / 06 / 07 / 13 / 18 / 19） |
| FAIL | **13**（AE-01 / 02 / 03 / 05 / 08 / 09 / 10 / 11 / 12 / 14 / 15 / 16 / 17） |
| BLOCKED | 0 |
| 通过率 | 31.6% |

核心结论：文章的**增删改落库链路是通的**（10,000 字往返逐字一致、保存后列表出现、精选刷新保持），
但**编辑弹窗的 UI 规范与 PRD/UI brief 大面积不符**（宽度 1230 vs 880、圆角 16 vs 12、左右栏颠倒、工具栏缺 5 项），
且存在 **3 个功能性缺口**：同步语言不创建翻译任务（AE-15）、保存后不关弹窗无「已保存」提示（AE-17）、
Esc 无法关闭 + 任何关闭方式都没有「未保存修改」确认（AE-16）。

## 2. 执行信息

- 命令（每个 worker 独立 `--output`）：
  ```bash
  cd frontend && PATH=/usr/local/bin:/opt/homebrew/bin:$PATH \
    PLAYWRIGHT_EXTERNAL_SERVER=1 \
    PLAYWRIGHT_EXECUTABLE_PATH='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' \
    E2E_REAL_OUT=<repo>/e2e-acceptance-v04 \
    npx playwright test e2e/acceptance/w2-article-editor.spec.ts \
    --project=chromium --workers=1 --reporter=list --output=test-results/acc-w2
  ```
- Spec：`frontend/e2e/acceptance/w2-article-editor.spec.ts`（1 用例 = 1 `test()`，标题以 ID 开头）
- 视图：默认 1440×900；AE-01 额外 1024×768、390×844
- 测试数据：`ACC-W2-<角色>-<stamp>`；跑完已清理（`DELETE FROM content_articles WHERE title LIKE 'ACC-W2-%'`，累计清理 41 行含历史残留，复查 0 行残留）
- 运行日志：`frontend/temp/acc_v04_w2/run2.log`（干净全量）、`failures.txt`（失败摘要）

> 说明：首轮全量在 AE-09 处被中断，原因是**另一路 W3 验收**（同一 E2E 账号）并发登录把本会话踢掉，
> 表现为 AE-06/AE-09 之类"登录超时"假失败。等 W3 跑完后清理数据重跑得到本报告的干净结果；
> 期间给 `login()` 加了重试包装，避免与并发账号互踢导致整轮报废。

## 3. 结果总表

| ID | 类型 | 结果 | 实测值 / 违规明细 | 关键截图 |
|---|---|---|---|---|
| AE-01 | UI | **FAIL** | 1440 弹窗宽 **1230.52**（期望 880±4）；圆角 **16px**（期望 12）；**正文编辑器在左栏**（bodyInLeft=true）；1024 宽 **953.34**（期望 880±4）；390 无横向溢出 ✔ | `AE-01-01-1440-dialog.png`、`AE-01-02-1440-columns.png`、`AE-01-03-1024-dialog.png`、`AE-01-04-390-dialog.png` |
| AE-02 | UI/U | **FAIL** | 编辑器底边 786.3 / 底部按钮区 791.8 → gap 5.5px ✔；3000 字后**编辑器外框 +120.0px**（应不变）、**弹窗 +16.6px**（应不变长）；内部滚动 sh=1871 > ch=432 ✔；译文卡 y=1592.6 与编辑器不相交 ✔ | `AE-02-01-editor-layout.png`、`AE-02-02-editor-3000chars.png` |
| AE-03 | F | **FAIL** | 工具栏仅 **6** 项：加粗/斜体/下划线/项目列表/编号列表/插入链接；缺 **H2、H3、引用、插入图片、清除格式**（「项目/编号列表」≈用例的有序/无序列表，仅命名差异）；已有 6 项点击均生效 | `AE-03-01-toolbar.png`、`AE-03-02-toolbar-applied.png` |
| AE-04 | F | **PASS** | 10,000+ 字往返：payload 10532 字 / 纯文本 9284 字 / 重开后 10532 字，`equal=true` 逐字一致 | `AE-04-01-filled-10k.png`、`AE-04-02-roundtrip-source.png` |
| AE-05 | F | **FAIL** | 「，」「,」生效，「**、**」**不分隔**（得到 `Beta、Gamma` 一个 chip）；`  AI  ` 去重生效 ✔（未新增）；存在独立「**＋ 新建标签**」按钮 1 个（应只剩 TagInput 一种入口） | `AE-05-01-separators.png`、`AE-05-02-no-standalone-button.png` |
| AE-06 | F/U | **PASS** | 组字中 Enter：chip 0 → compositionend 后 Enter 生成「组字中」 | `AE-06-01-composing-enter.png`、`AE-06-02-after-composition.png` |
| AE-07 | F/U | **PASS** | 打开弹窗联想 0 条 → 输入 1 字 **8** 条 → ↓+Enter 选中 `CCAI` → Esc 关闭（1→0） | `AE-07-01..04` |
| AE-08 | F | **FAIL** | 第 11 个标签**仍生成**（chips=11）；21 字标签**仍生成**（chips=12）；有提示节点但无拦截 | `AE-08-01-eleven-tags.png`、`AE-08-02-long-tag.png` |
| AE-09 | F | **FAIL** | 标签库列表查不到新标签（`[]`）。DB 交叉验证：文章 246 `tags=["ACC-W2-NEWTAG-…"]`，即**文章侧已写入**，**标签库聚合未更新** | `AE-09-01-tag-typed.png`、`AE-09-02-taxonomy-list.png` |
| AE-10 | F/U | **FAIL** | 正文为空时按钮 `disabled=false`，点击**无请求/无提示/无字段变化**；生成中**无加载态**（仍可点）；desc 123 字 ✔、kw 8 个 ✔；**SEO 标题被改写**：`"" → "ACC-W2 SEO 校验标题 / CCAI"`（应不变），slug 未变 ✔ | `AE-10-01..03` |
| AE-11 | U | **FAIL** | 已有 SEO 描述时**无「已有内容，是否覆盖？」确认**，直接覆盖原描述（`unchanged=false`） | `AE-11-01-overwrite-confirm.png` |
| AE-12 | UI | **FAIL** | 摘要初始高 **84.1px**、SEO 描述 **59.0px**（均 < 96px）；50 行后摘要 84.75px 内部滚动 ✔；SEO 描述 `maxlength=300` 被截断，未真正测到 400px 上限 | `AE-12-01..04` |
| AE-13 | F/U | **PASS** | 草稿行精选开关 `disabled=true`，`title="仅已发布文章可设为精选"` | `AE-13-01-draft-featured.png` |
| AE-14 | F/UI | **FAIL** | 精选开关是**原生 checkbox**（`INPUT type=checkbox`、`appearance=auto`、16×16、无 `role=switch`、无滑块 knob）→ 非滑块；刷新后状态保持 ✔；开态未见 `#2563EB` 背景 | `AE-14-01..04` |
| AE-15 | F | **FAIL** | 勾选英+法保存后，列表译文列仍为「**未翻译**」，无 EN/FR 状态；DB 侧近 3 小时 **0 条** `content_translations` → **未创建任何翻译任务** | `AE-15-01..03` |
| AE-16 | U | **FAIL** | ×、取消、点遮罩：**直接关闭、无「有未保存修改，确认离开？」**；**Esc 完全不关闭**（modalOpen=1）。双重违约 | `AE-16-01..04` |
| AE-17 | F/U | **FAIL** | 保存后**弹窗仍打开**（modalCount=1）、**无「已保存」提示**（toast=[]）；列表出现新文章 ✔、状态仍 draft ✔ | `AE-17-01..03` |
| AE-18 | UI | **PASS**（复核后） | 列表工具栏「文章状态」= 统一 Select（svg 箭头，无原生 select/datalist）；弹窗分类 = `.select-trigger[aria-label="文章分类"]`，7 个选项，可选中「架构与网关」 | `AE-18-01-toolbar-status.png`、`AE-18-02-selects.png`、`AE-18-03-category-open.png` |
| AE-19 | U | **PASS**（复核后） | 14 次 Tab 全程在弹窗内；无焦点无可见指示项 **0**；顺序 = 左栏摘要→编辑器(tab/工具条/正文)→右栏分类→＋新建→标签输入，与视觉一致 | `AE-19-01-tab-sequence.png` |

截图目录：`frontend/e2e-acceptance-v04/shots/w2/`（48 张）；复核新增：`e2e-acceptance-v04/AE-18-01-toolbar-status.png` 等。

## 4. 缺陷清单（按严重度）

### P0 · 功能缺口 / 数据风险

1. **AE-15 同步语言保存后不创建翻译任务**
   - 复现：新建文章 → 勾选 English + Français → 保存 → 列表译文列
   - 实测：列显示「未翻译」，无 EN/FR 徽标；`content_translations` 3 小时内 0 条新增
   - 影响：R9 文章翻译链路在后台入口处断掉，用户以为已排期翻译
2. **AE-16 Esc 无法关闭 + 四种关闭方式都无「未保存修改」确认**
   - 实测：×/取消/遮罩直接关闭（确认框 0 个）；Esc 后弹窗仍在
   - 影响：误点 × / 点遮罩即丢失编辑内容；键盘用户无法用 Esc 退出
3. **AE-17 保存后不关弹窗、无「已保存」提示**
   - 实测：`modalCount=1`、`toasts=[]`；但列表已出现新行且为 draft（写入成功）
   - 影响：保存成功无反馈，用户会重复点击保存产生重复文章

### P1 · UI 规范 / 交互规范偏差

4. **AE-01 弹窗尺寸与栏位**：1440 下 1230.52px（应 880±4）、圆角 16px（应 12）、**正文编辑器在左栏**（应右栏）；1024 下 953.34px
5. **AE-02 编辑器不固定高度**：贴 3000 字后外框 +120px、弹窗 +16.6px（应内部滚动、外框不变）
6. **AE-03 富文本工具栏只有 6 项**：缺 H2 / H3 / 引用 / 插入图片 / 清除格式（5 项）
7. **AE-10 SEO 按钮**：空正文不禁用；生成中无加载态；**改写 SEO 标题**（应只写描述/关键词）
8. **AE-11 无覆盖确认**：已有 SEO 描述被直接覆盖
9. **AE-14 精选开关是原生 checkbox**，非滑块、无 `role=switch`、开态非 `#2563EB`

### P2 · 细节 / 上限 / 一致性

10. **AE-05**：`、` 不作为分隔符；保留独立「＋ 新建标签」按钮（应只有 TagInput）
11. **AE-08**：单篇标签无 10 个上限、标签名无 21 字上限
12. **AE-09**：新标签写进文章，但未聚合进标签库
13. **AE-12**：摘要 84px / SEO 描述 59px，未达初始高 ≥96px

## 5. 口径修正与复核记录（避免误判）

初版有两例断言**超出用例原文**，复核后改判 PASS，已在同一 spec 内修正并单跑验证：

| 用例 | 初版断言（过严） | 用例原文 | 修正后 |
|---|---|---|---|
| AE-18 | 要求 Select 圆角 8px、高 36px；并要求「状态」下拉在文章弹窗内 | 「分类下拉、状态下拉 → 统一 Select 组件，没有原生箭头和 datalist」；§1 口径的 8px 圆角只对**按钮/单行输入框**生效 | 改测：列表工具栏「文章状态」Select + 弹窗分类 Select 均为统一组件、无原生 select/datalist → **PASS** |
| AE-19 | 要求焦点 y 全局单调不减 | 「Tab 顺序与视觉顺序一致」 | 两栏布局下改为「同列内 y 不回跳（明显向右换列除外）」→ 实际顺序与视觉一致 → **PASS** |

其余 13 个 FAIL 均已逐条对照用例原文与 §1 量化口径，未放宽阈值，且 AE-09/AE-15 另用 DB 交叉验证过（文章侧写入 vs 标签库/翻译表缺记录）。

## 6. 覆盖盲区与风险

- **AE-15 重试分支未触发**：本次没有任何翻译任务进入失败态，故「失败项有重试按钮」未验证（不记 FAIL，但属未覆盖）。
- **AE-12 的 400px 上限未真正压测**：SEO 描述有 `maxlength=300`，50 行文本被浏览器截断，无法产生溢出。
- **AE-10 真实模型链路**：本次只验证了「生成成功」路径的字段结果，模型未配置时的提示文案未专门验证（默认模型已配置）。
- **AE-03 列表命名**：实现在用「项目列表/编号列表」，用例写「有序/无序列表」，功能等价，按命名差异记录不单独计缺陷。
- **并发风险**：同一 E2E 账号的并发运行会互踢登录（本次已导致一轮中断）；建议验收时段串行或按 worker 分配独立账号。

## 7. 复现入口

- 单例：`npx playwright test e2e/acceptance/w2-article-editor.spec.ts -g "AE-15"`（其它同理）
- 全量：见 §2 命令
- 数据清理：`psql ... -c "delete from content_articles where title like 'ACC-W2-%';"`
- 证据：`frontend/e2e-acceptance-v04/shots/w2/`、失败摘要 `frontend/temp/acc_v04_w2/failures.txt`、完整日志 `frontend/temp/acc_v04_w2/run2.log`
