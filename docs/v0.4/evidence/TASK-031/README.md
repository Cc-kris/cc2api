# TASK-031 证据：内容管理核心链路 E2E + DEFECT-031-01 修复

- 任务：完成内容管理核心链路端到端场景及真实数据取证
- 基线提交：`0ce18c4c4b3e5b5375a049af06e7caf3241f2151`
- 隔离栈：Go `:8080`（真实 PostgreSQL）+ Vite dev `:3100`（复用系统 Chrome 启动，`PLAYWRIGHT_EXECUTABLE_PATH`）

## 1. 真实缺陷 DEFECT-031-01（已修复，回归用例现为正常 PASS）

**PRD v3.3 L121/L168**：抓取源「间隔（分钟）」留空 = 继承全局（默认 15 分钟）；源有独立间隔时必须显示该固定值。

**缺陷现象**（修复前 `ContentCenterView.vue::sourceScheduleText`）：

| 场景 | 期望 | 修复前实际 |
| --- | --- | --- |
| 源 `intervalMinutes = NULL`（继承全局 15） | 跟随全局（15 分钟） | 每 15 分钟抓取一次（把全局 `fetchCron` 当成源自身间隔） |
| 源 `intervalMinutes = 45` | 每 45 分钟抓取一次 | 有 cron 时优先显示 cron 文案，与源真实调度不一致 |

**修复**：

- `sourceScheduleText()`：只认源自身 `intervalMinutes`（`?? 0`，不再回退 `scheduleMinutes` / `scheduleLabel` / `fetchCron`）；`NULL` → `跟随全局（N 分钟）`。
- 新增 `globalFetchIntervalMinutes()`：解析 `automation.fetchCron`（`0 */15 * * * *` → 15，5/6 段兼容，异常回退 15）。
- 删除误导性 `cronToScheduleText()`。
- 创建源表单：`intervalMinutes` 默认 `null`，留空提交 `null`（不再用 15 覆盖），占位符显示「跟随全局（N 分钟）」；详情面板间隔输入框同样带该占位符。

## 2. 负向对照（证明回归用例真能拦住缺陷，而非"记缺陷后放行"）

临时把 `ContentCenterView.vue` 换回 baseline（`git show HEAD:...`）后执行：

| 用例 | baseline（缺陷代码） | 修复后 |
| --- | --- | --- |
| `ContentCenterView.spec.ts` 单测 | **1 failed**（实际文本 `每 15 分钟抓取一次`） | 10 passed |
| E2E `R1-02 UI 回归` | **1 failed**（DOM 未出现「跟随全局」） | passed |

恢复后文件 `md5` 与修复版一致（`80055248d7b3ded4066a8e1aa9d2c8e9`），单测复跑 10 passed。

## 3. E2E 场景与取证点

`frontend/e2e/content-admin-v04.spec.ts`（9 条，真实调用隔离栈 API + 真实浏览器渲染）：

| 用例 | 取证点 |
| --- | --- |
| R1-01 标签删除二次确认 | 取消后 taxonomy 仍有该标签；确认后标签消失且 taxonomy 同步 |
| R1-02/R1-03 抓取间隔边界 | API 落库 `intervalMinutes`：空→`null`，`0/14/1441` 拒绝，`15/1440` 通过 |
| R1-02 UI 回归 | 源码列表行 DOM 文本：继承行含「跟随全局（15 分钟）」，独立行含「每 45 分钟抓取一次」且不含「跟随全局」 |
| R1-04 万字正文 | 保存后详情回显 + 重新 GET 长度一致、无截断 |
| R2-01 响应式 | 1440/1024/390 三视口 `scrollWidth <= clientWidth`，编辑弹窗宽度不超视口 |
| R5-01 输入法组字 | 组字中 Enter 不生成词条，`compositionend` 后 Enter 才生成 |
| R6-01/R6-05 精选 | `featured_at` 写入且严格倒序；**公共** `/api/v1/public/articles/featured` ≤10 条且新设置在前（后台列表不做 10 条断言——10 条是前台展示上限） |
| R10-06/R10-12 加密/默认模型缺失 | 返回明确业务错误，响应体不回显明文 Key |
| R9-01 翻译任务 | 缺模型被拒；指定模型后任务为 `queued`（非假成功） |

## 4. 复现命令

```bash
cd frontend
# 单测 + 类型
npx vitest run src/views/admin/__tests__/ContentCenterView.spec.ts   # 10 passed
npx vue-tsc --noEmit                                                 # 无错误

# E2E（隔离栈已在 :3100 运行；本机缺 Playwright 浏览器二进制，复用系统 Chrome）
PLAYWRIGHT_EXECUTABLE_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
PLAYWRIGHT_EXTERNAL_SERVER=1 E2E_BASE_URL=http://127.0.0.1:3100 S2A_BASE_URL=http://127.0.0.1:3100 \
npx playwright test e2e/content-admin-v04.spec.ts --project=chromium --workers=1 --reporter=line
```

结果：`9 passed (6.1s)`，`EXIT=0`。

## 5. 测试稳健性修正（属测试选择器/造数问题，非产品缺陷）

- `openAdmin()` 预置 `admin_guide_2_admin_v4_interactive` / `admin_guide_guest_user_v4_interactive = true`：driver.js 新手引导浮层会遮挡弹窗按钮（每个全新 context 都重新触发），此处模拟「已看过引导」的老用户，避免用 `force: true` 掩盖遮挡。
- `newArticle()` 改用后端真实字段 `locale` / `excerpt`（此前误传 `language` / `summary` 被忽略，文章落 `en-US`，不进中文公共精选）。
- R6 的「≤10」断言改到公共接口（`MaxPublicFeaturedArticles = 10` 的唯一事实源）。
