# Discovery Report（项目 v0.4）

- 时间：2026-10-26；范围：内容中心与前台内容站任务拆分前的只读代码核查。
- 基线：Git tag `v0.3.5` 已存在；本轮项目 v0.4、五文档 v3.3；未修改业务代码。
- 方法：读取五文档与 R1～R3 审核记录；枚举后端 handler/service/repository/routes/render、ent schema、迁移和前端内容/i18n/共享组件路径；核对 AI 配置、上传与 BI 现状。

## 1. 已核实现状

1. 后端为 Go/Gin/ent，前端为 Vue 3/Vite/wangEditor；内容 schema 集中于 `backend/ent/schema/content.go`，不是多个独立 content 表 schema 文件。
2. 内容管理入口为 `backend/internal/server/routes/content_admin.go` 与 `frontend/src/views/admin/ContentCenterView.vue`；公开入口为 `routes/public_content.go`、`render/content_renderer.go`、`PublicContentView.vue`。
3. 文章、新闻源、公开查询、BI、翻译任务与缓存已有独立 service/repository/test 文件，v0.4 按这些边界拆 TASK。
4. `data/content_ai_provider.json` 已有单配置结构，文件权限为 0600；本轮升级 version=2 多配置，不建 `content_ai_models` 表，不复用 payment 加解密函数。
5. 现有上传处理器使用 `ContentAllowedImageMIMEs` 与 `ContentMaxImageBytes`，与本地上传契约一致；真实存储与云端配置差异列入实现时验证，不在任务清单另造第二套规则。
6. 当前没有 `backend/migrations/219_content_v1.sql`，在新清单中明确标为新增；219 的 featured_at、来源快照、翻译约束及 BI 白名单须一次迁移完成并对账。
7. 五语 registry、localeDetection、公开缓存、SSR 路由及 BI handler 均存在；新增聚合函数按 `content_analytics.go` 等现有文件扩展，不另造支付聚合入口。
8. 共享组件与页面存在并发修改风险；除交付依赖外，调度器必须对 TASK 影响文件集合加全局互斥锁，任何交集均禁止并行。

## 2. 影响与风险

| 风险 | 影响 | 任务控制 |
|---|---|---|
| ent 生成物远大于手写 schema diff | LOC 估算与审查失真 | TASK-001 统一生成、完整提交；手写 LOC 与生成物分开审查 |
| ContentCenterView/PublicContentView/style.css 多任务共享 | 并行覆盖与回归 | 仅沿依赖链串行执行；每个切片独立提交 |
| 219 数据迁移不可逆误操作 | 精选顺序、来源、翻译重复 | 先 pg_dump；ID/顺序/约束/校验和对账；失败中止 |
| AI JSON 并发与 Key 回显 | 配置覆盖、凭据泄漏 | revision+If-Match；0600；请求/响应/日志测试 |
| SSR 多语言与缓存 | 错语言、闪烁、陈旧内容 | self-canonical/hreflang/noindex；版本化缓存与失效测试 |
| BI 心跳与 page_leave | 时长重复、聚合慢 | view 最大有效值；索引与一年数据性能验证 |

## 3. 任务清单结论

- 采用 35 个可独立测试/回滚的 vertical slice；每项手写代码预估 ≤200 LOC、影响文件 ≤5。
- REQ-001～016 覆盖 R1～R15 与质量门；TASK-001～034 对 94 条 TC 均有功能归属，TASK-035 做 17 组 AC 终验。
- 官方校验报告：`runtime/task_validation_report.json`；独立审计补充路径、DAG、TC 区间展开和 AC 覆盖。
- 本报告只支持任务拆分与门禁，不证明功能已实现。
