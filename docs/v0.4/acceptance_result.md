# v0.4 验收结果

- 节点评分：**2/2**（内容中心验收域证据完整，真实浏览器路径已执行）。
- 执行时间：2026-10-27；执行人：主会话验收 + 各任务 worker。
- 环境：macOS arm64；Go 1.26.6/jsonv2；Node 20 项目依赖；Chrome/Edge/WebKit；桌面/平板/移动；浅色/深色。
- 总结：**94/94 TC PASS，17/17 AC PASS**。

## 口径说明

原 `v0.4_acceptance_20261026_v3.3.md` 含 AC-01～AC-15；任务清单额外引用 AC-16/17。为保持 35 任务追踪完整，本报告将二者作为任务清单的跨域汇总终验，并明确标注来源，不反向修改 PRD 或原验收文档。

| AC | 来源/REQ | 实际结果 | 对应 TC | 证据 |
|---|---|---|---|---|
| AC-01 | 验收矩阵 / R1 + 第一章 | PASS：三个存量 bug 修复；统一组件、双主题、弹窗和状态反馈完整 | R1-01, R1-02, R1-03, R1-04, R1-05, R1-06, R1-07 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-02 | 验收矩阵 / R2 | PASS：数据/记录/说明/组件四类布局统一；长列表可控；三视口无溢出 | R2-01, R2-02, R2-03, R2-04, R2-05 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-03 | 验收矩阵 / R3、R7、R9 | PASS：新闻源抓取、自动翻译、封面继承、失败恢复全链路通过 | R3-01, R3-02, R3-03, R3-04, R3-05, R3-06, R3-07, R3-08, R7-01, R7-02, R7-03, R9-01, R9-02 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-04 | 验收矩阵 / R4 | PASS：定时任务统一 Toggle/Input，状态刷新保持，对比度达标 | R4-01, R4-02 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-05 | 验收矩阵 / R5 | PASS：标签唯一输入方式；SEO 默认模型、覆盖确认、长度/数量边界与异常完整 | R5-01, R5-02, R5-03, R5-04, R5-05, R5-06, R5-07, R5-08 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-06 | 验收矩阵 / R6、R13 | PASS：精选迁移零差、唯一真源、1 大+9 小、0/不足 10 和双主题正确 | R6-01, R6-02, R6-03, R6-04, R6-05, R6-06, R13-01, R13-02, R13-03 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-07 | 验收矩阵 / R7 | PASS：新闻/文章、抓取/翻译/手动/上传来源封面全显示，失败统一占位 | R7-01, R7-02, R7-03, R7-04, R7-05, R7-06 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-08 | 验收矩阵 / R8 | PASS：默认 20 条、真实 total、空/越界/非法参数与 URL 行为一致 | R8-01, R8-02, R8-03, R8-04, R8-05, R8-06 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-09 | 验收矩阵 / R9 | PASS：文章/新闻翻译状态机、SSR、locale、canonical/hreflang/sitemap 全通过 | R9-01, R9-02, R9-03, R9-04, R9-05, R9-06, R9-07, R9-08, R9-09, R9-10 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-10 | 验收矩阵 / R10 | PASS：多配置 CRUD/默认/测试/加密/并发/删除保护及三处模型消费通过 | R10-01, R10-02, R10-03, R10-04, R10-05, R10-06, R10-07, R10-08, R10-09, R10-10, R10-11, R10-12, R10-13 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-11 | 验收矩阵 / R11 | PASS：十维度、五语、筛选、事件去重、均值与聚合性能通过 | R11-01, R11-02, R11-03, R11-04, R11-05, R11-06 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-12 | 验收矩阵 / R12 | PASS：URL>cookie>localStorage>浏览器；无前缀/长码 301 与中间件路由一致 | R12-01, R12-02, R12-03, R12-04, R12-05, R12-06 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-13 | 验收矩阵 / R14 | PASS：旧绿 0 残留，四类板块双主题与响应式走查通过 | R14-01, R14-02 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-14 | 验收矩阵 / R15 | PASS：无原生下拉、无下划线输入框，键盘可操作 | R15-01, R15-02 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-15 | 验收矩阵 / 性能/安全/兼容约束 | PASS：SSR p95<2s、BI p95<3s；权限/浏览器/三视门通过 | PERF-01, PERF-02, SEC-01, COMP-01 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-16 | 任务清单汇总 / REQ-016 | PASS：任务清单补充跨域终验：权限与敏感信息、性能及浏览器兼容矩阵全部通过 | SEC-01, COMP-01, PERF-01, PERF-02 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |
| AC-17 | 任务清单汇总 / REQ-015 | PASS：任务清单补充视觉终验：基础控件、neo-lab 主题、双主题与响应式全部通过 | R1-05, R1-06, R3-01, R4-01, R14-01, R14-02, R15-01, R15-02 | `test_execution.md`；任务追溯见 `evidence/TASK-035/task-results.json` |

## 关键物理证据

- 浏览器：`evidence/TASK-035/browser-matrix-final.log`（192/192 = 32 用例 × 6 项目）；22 份 `evidence/TASK-035/ui-traces/**/trace.zip`（11 个用例 × chromium/mobile-chrome，日志 22 passed）。
- 性能：SSR 热 p95=2.2ms；一年 BI p95=360.1ms；混合并发 45.5 RPS，见 `evidence/TASK-034/final.json`。
- 安全：TASK-032 权限、Key 掩码、上传、公开字段与越权测试；本版本未引入明文密钥。
- CI：三条 Go workflow 统一 `GOEXPERIMENT=jsonv2`，见 `evidence/TASK-035/ci-jsonv2.md`。

## 已知非本版本风险

1. 全仓既有失败共 15 项（Go 12 + 前端 3），均在 v0.4 基线提交前存在，需独立基线修复；不影响本次 94 条内容中心用例的结论，但不能宣称全仓测试全绿。
   - Go 12 项：service 5 项 gateway（Anthropic/Gemini/OpenAI 缓存与取消流）、repository 6 项（用户邮箱/微信身份归一化，SQLite 缺 `hashtext` 导致 `SQL logic error`）、config 1 项默认 SSLMode（断言 `prefer`，实现默认 `disable`）。
   - 前端 3 项：`userLocaleParity` 的 de/fr/ru 键与占位符断言失败（全仓 vitest 941/944；`make test-frontend` 的 98/98 为内容中心定向口径）。
2. 进程内公开内容缓存在多副本下最长可能有 60 秒陈旧窗口；发布/回滚需滚动重启或等待失效。
3. 未在本轮向生产部署，也未对生产数据执行回滚；发布清单必须由发布执行人逐项勾选。
