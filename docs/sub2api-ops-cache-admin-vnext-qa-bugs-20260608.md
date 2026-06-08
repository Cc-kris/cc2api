# sub2api 运维监控与缓存管理 vNext QA Bug 记录

- 版本：v1.0
- 日期：2026-06-08
- 范围：`docs/sub2api-ops-cache-admin-vnext-development-task-list-20260607.md` 中 QA-001 至 QA-022
- 测试策略：按现有自动化测试用例执行；失败记录到本文档；修复后进入下一轮，直到测试无失败。

## 第 1 轮测试

### 执行命令

| 编号 | 命令 | 结果 | 日志 |
|---|---|---|---|
| R1-BE-ALL | `cd backend && go test ./... -count=1 -timeout=10m` | 失败 | `/tmp/sub2api-qa-round1/backend-go-test.log` |
| R1-FE-ALL | `pnpm --dir frontend test:run` | 失败 | `/tmp/sub2api-qa-round1/frontend-vitest.log` |
| R1-BE-SERVICE | `cd backend && go test ./internal/service -count=1 -timeout=3m -v` | 失败 | `/tmp/sub2api-qa-round1/backend-service-v.log` |

### Bug 列表

| Bug ID | 来源 | 失败用例 | 现象 | 初步根因 | 状态 |
|---|---|---|---|---|---|
| BUG-R1-001 | 后端 | `TestBuildAIAnalysisContextSamplesAreRedacted` | AI 分析上下文样本摘要仍包含 `Bearer` 字样 | `BuildAIAnalysisContext` 对错误摘要只替换了密钥值，未移除敏感鉴权关键词 | 已修复，待第二轮全量验证 |
| BUG-R1-002 | 前端 | `OpsRequestDetailsModal.spec.ts`、`OpsErrorDetailsModal.spec.ts` | Vitest 报 `No "default" export is defined on the "@/api/admin/ops" mock` | 组件或聚合 API 依赖 `@/api/admin/ops` 的默认导出，但测试 mock 只提供了具名 `opsAPI` | 已修复，待第二轮全量验证 |

### 非失败但有噪声

以下日志来自测试用例主动模拟异常或组件 stub 不完整，不作为本轮 bug：

- Vue `router-link` stub 警告。
- Pinia/auth、subscription、table loader 等错误日志模拟。
- OpenAI WS 重试、UsageCleanup、UsageRecordWorkerPool panic 等测试内预期日志。

## 第 1 轮修复记录

| Bug ID | 修复内容 | 定向验证 |
|---|---|---|
| BUG-R1-001 | 后端 AI 分析上下文统一脱敏增强：移除 `Authorization Bearer` 鉴权语义残留，样本保留脱敏后的用户邮箱和上游账号名用于分析。 | `cd backend && go test ./internal/service -count=1 -run 'TestBuildAIAnalysisContextSamplesAreRedacted|TestRedactAIContextTextRedactsEmailAndToken' -v` 通过 |
| BUG-R1-002 | 前端失败套件补齐 `@/api/admin/ops` mock 的默认导出，并为 `OpsRequestDetailsModal` 测试初始化 Pinia 管理员身份。 | `pnpm --dir frontend exec vitest run src/views/admin/ops/components/__tests__/OpsRequestDetailsModal.spec.ts src/views/admin/ops/components/__tests__/OpsErrorDetailsModal.spec.ts` 通过 |

## 第 2 轮测试

### 执行命令

| 编号 | 命令 | 结果 | 日志 |
|---|---|---|---|
| R2-BE-ALL | `cd backend && go test ./... -count=1 -timeout=10m` | 通过 | `/tmp/sub2api-qa-round2/backend-go-test.log` |
| R2-FE-ALL | `pnpm --dir frontend test:run` | 失败 | `/tmp/sub2api-qa-round2/frontend-vitest.log` |

### Bug 列表

| Bug ID | 来源 | 失败用例 | 现象 | 初步根因 | 状态 |
|---|---|---|---|---|---|
| BUG-R2-001 | 前端 | `KeyUsageView.spec.ts` 测试结束后的未处理异步任务 | 全部 675 条断言通过，但 Vitest 捕获未处理异常 `requestAnimationFrame is not defined`，整体退出码为 1 | 测试环境只 mock 了 `requestIdleCallback`，未 mock jsdom 下缺失的 `requestAnimationFrame/cancelAnimationFrame` | 已修复，待第 3 轮全量验证 |

## 第 2 轮修复记录

| Bug ID | 修复内容 | 定向验证 |
|---|---|---|
| BUG-R2-001 | 前端 Vitest 全局测试环境补齐 `requestAnimationFrame` 与 `cancelAnimationFrame` mock。 | `pnpm --dir frontend exec vitest run src/views/__tests__/KeyUsageView.spec.ts` 通过 |

## 第 3 轮测试

### 执行命令

| 编号 | 命令 | 结果 | 日志 |
|---|---|---|---|
| R3-BE-ALL | `cd backend && go test ./... -count=1 -timeout=10m` | 通过 | `/tmp/sub2api-qa-round3/backend-go-test.log` |
| R3-FE-ALL | `pnpm --dir frontend test:run` | 通过 | `/tmp/sub2api-qa-round3/frontend-vitest.log` |

### 结论

第 3 轮未发现新的失败用例或未处理错误。

## 最终结论

- 第 1 轮发现 2 个 bug，并已修复。
- 第 2 轮发现 1 个测试环境 bug，并已修复。
- 第 3 轮后端全量 Go 测试和前端全量 Vitest 均通过。
- 产品验收补证后，最终补证轮在当前最终工作区重新执行后端全量 Go 测试和前端全量 Vitest，均通过。
- 本轮 QA 循环结束。


## 最终补证轮测试

### 执行背景

独立产品验收经理 + Code Reviewer 复核指出：第 3 轮日志生成时间早于后续产品验收补测，不能作为当前最终工作区的最终通过证据。因此在新增产品验收测试和脱敏修正全部落地后，重新执行全量测试。

### 执行命令

| 编号 | 命令 | 结果 | 日志 |
|---|---|---|---|
| FINAL-BE-ALL | `cd backend && go test ./... -count=1 -timeout=10m` | 通过 | `/tmp/sub2api-qa-final-20260608/backend-go-test-all.log` |
| FINAL-FE-ALL | `pnpm --dir frontend test:run` | 通过 | `/tmp/sub2api-qa-final-20260608/frontend-test-run.log` |

### 结果摘要

- 后端全量 Go 测试执行时间：2026-06-08 21:31:14 CST，结果通过。
- 前端全量 Vitest 执行时间：2026-06-08 21:32:58 CST，112 个测试文件、686 个测试全部通过。
- 最终补证轮未发现新的失败用例或未处理错误。
