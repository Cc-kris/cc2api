# 架构蓝图

> 用途：研发执行前的轻量架构参考；基线为线上 v0.3.5 与本轮项目 v0.4 文档。
> 来源：2026-10-26 对仓库目录、Go/Vue 依赖、内容服务/路由/迁移及 v0.4 五文档的只读核查。

## 1. 系统架构

### 1.1 技术栈

- 后端：Go 1.25.1、Gin v1.11.0、ent v0.14.5。
- 前端：Vue 3.5.30、Vite 7.3.1、wangEditor。
- 数据：PostgreSQL；运行环境提供 Redis。
- 本地研发：按项目记忆使用 FlyEnv 管理 PostgreSQL/Redis；生产发布另走项目既有发布流程。

### 1.2 目录结构

```text
sub2apis/
├── backend/
│   ├── ent/schema/content.go       # 内容领域 ent schema 单一聚合文件
│   ├── internal/handler/           # admin/public HTTP 边界
│   ├── internal/server/routes/     # 管理、公开内容、SSR 路由
│   ├── internal/service/           # 文章、新闻、任务、翻译、BI、AI 配置
│   ├── internal/repository/        # 内容持久化与公开查询
│   ├── internal/render/            # 公开页 SSR
│   └── migrations/                 # 顺序 SQL 迁移
├── frontend/src/
│   ├── api/admin/content.ts        # 内容管理 API 客户端
│   ├── views/admin/ContentCenterView.vue
│   ├── views/public/PublicContentView.vue
│   ├── views/admin/FinanceStatsView.vue
│   ├── components/common/          # 共享 UI 基元
│   ├── i18n/                       # 语言注册、检测与文案
│   └── style.css                   # 全局主题与组件 token
├── docs/v0.4/                      # 本轮研发门禁文档
└── runtime/                        # 本地任务校验报告，不是业务运行状态
```

## 2. 核心模块

| 模块 | 职责 | 关键代码路径 |
|---|---|---|
| 内容管理 | 文章、新闻、分类、写作、翻译、AI 配置 | backend/internal/service/content_*.go；frontend/src/views/admin/ContentCenterView.vue |
| 公开内容 | 列表/详情、locale、推荐、SSR | backend/internal/service/public_content_service.go；backend/internal/render/content_renderer.go；frontend/src/views/public/PublicContentView.vue |
| 内容 BI | 事件采集、聚合、dashboard | backend/internal/handler/content_analytics_handler.go；frontend/src/views/admin/FinanceStatsView.vue |
| 内容迁移 | 字段、约束、历史数据对账 | backend/ent/schema/content.go；backend/migrations/ |
| 共享前端基元 | 表单、弹窗、标签、分页、状态、编辑器 | frontend/src/components/common/；frontend/src/style.css |
| i18n | 五语注册、检测、cookie 与本地存储 | frontend/src/i18n/registry.ts；frontend/src/i18n/localeDetection.ts |

## 3. 数据流

### 3.1 核心流程

```text
管理操作 → Gin admin route → handler → content service → ent/repository → PostgreSQL
公开请求 → public route → service/cache → ent/repository → SSR renderer → HTML
浏览器事件 → content_analytics_handler → 事件表 → 聚合查询 → FinanceStatsView
AI 配置 → data/content_ai_provider.json → 默认配置解析 → 写作/SEO/翻译调用方
```

### 3.2 状态管理

- 内容发布状态、新闻源启停、翻译任务状态以服务端持久化状态为准。
- 精选唯一真源为文章 `featured_at`，按 ID 逆序映射公开顺序；旧数组只允许一次性迁移输入。
- AI 多配置使用 version/revision 乐观并发，Key 只在新增/更新时输入，不明文回显。
- 前端 cookie 与 localStorage 同步语言偏好；服务端 URL locale 决定 SSR 正文语言。

## 4. 关键约束

### 4.1 技术约束

- 现有 ent schema 集中于 `backend/ent/schema/content.go`，生成树由统一任务管理，禁止多任务并行改 schema。
- 管理后台与公开页分别存在共享大组件，相关 TASK 必须串行，禁止无协调并行写入。
- 219 迁移需先备份并做 ID/顺序/校验和对账；生成文件不计手写 LOC，但须完整提交。
- 业务复用现有 middleware、route group、error/response 与现有内容存储，不复制支付领域配置或加密接口。

### 4.2 业务规则

- 五语为 en/zh/ru/fr/de；无译文正文回落原文，不跳转，使用 self-canonical 与 noindex,follow。
- 公开文章/新闻列表每页 20；首页按 PRD 数量展示，不把分页上限改成 6。
- R10 不建数据库表；默认配置被文章生成、SEO、翻译共同使用。
- BI 图例、坐标轴、tooltip、筛选与位置名按界面五语显示，时间聚合取 view 最大有效值。

## 5. 已知问题

| 问题 | 影响范围 | 计划解决版本 |
|---|---|---|
| 内容管理/公开页面较集中，并行修改易冲突 | 多个 v0.4 TASK | v0.4 通过依赖图与全局文件互斥锁控制，后续迭代再拆分 |
| ent schema 集中且生成物多 | 迁移与代码审查 | v0.4 统一由 TASK-001 持有并提交生成物 |
| 旧任务清单含已废止设计 | 编码误入 | 已由 v0.4 v1.0 清单替代，旧清单保持 STALE |
| 当前蓝图由本次轻量逆向建立，未覆盖账户/网关全部细节 | 非内容领域扩展 | 后续涉及对应领域时补充 |

## 更新记录

- 2026-10-26 v0.4：首次基于当前代码与五文档 v3.3 建立轻量蓝图，用于任务清单架构门禁。
