# v0.4 测试执行记录

- 执行日期：2026-10-27
- 验收分支：`feat/content-v04`
- 用例来源：`v0.4_test_cases_20261026_v3.3.md`，预期行为来自 PRD，而非按实现改写。
- 执行结论：94/94 条内容中心用例 PASS；17/17 组终验 PASS。
- 最终六项目浏览器矩阵：192/192 PASS（2.8m）；UI trace 22/22 PASS。

## CI 与质量门

| 门禁 | 结果 | 证据 |
|---|---|---|
| 前端 lint / typecheck / build | PASS | `evidence/TASK-035/lint.log`、`typecheck.log`、`build.log` |
| `make test-frontend`（6 文件 98 测试） | PASS | `evidence/TASK-035/make-test-frontend.log` |
| Go 1.26.6 + `GOEXPERIMENT=jsonv2` CI 定向 unit | PASS | `evidence/TASK-035/backend-unit-ci-go1.26.6-jsonv2.log` |
| 后端发布构建 | PASS | `evidence/TASK-035/backend-build.log` |
| `golangci-lint` | 未取得有效结论（工具链不兼容，不构成 PASS） | `evidence/TASK-035/golangci-lint.log`（3 项 typecheck 报错源于 golangci 与 Go 1.27.1 导出数据版本不兼容，非代码缺陷） |

> 全仓补充扫描（`evidence/TASK-035/go-test-all.log`、`vitest-all.log`）发现与本版本无关的既有失败共 15 项：
> Go 12 项 —— service 5 项 gateway（Anthropic/Gemini/OpenAI 缓存与取消流）、repository 6 项（用户邮箱/微信身份归一化，SQLite 缺 `hashtext` 函数导致 `SQL logic error`）、config 1 项默认 SSLMode；
> 前端 3 项 —— `userLocaleParity` 的 de/fr/ru 键与占位符断言（全仓 vitest 941/944）。
> 这些失败在 v0.4 基线提交前即存在（相关测试文件早于本次需求），不被冒充为全绿；内容中心定向测试与 CI 门禁单独记录。

## 任务 worker 结果口径

- `evidence/TASK-035/task-results.json` 逐任务记录 `commit`、`worker_result_file` 与主会话 `acceptance_status`（34 条）。
- 其中 3 条原始 worker 状态为 `FAILED`（TASK-008、TASK-017、TASK-026）：TASK-008 因当时仓库内 `content_news_admin_test.go` 语法错误阻塞（该文件今已不存在，非本任务修改），TASK-017/026 为测试命令无结果返回；三者均由主会话在后续门禁（`backend-unit-ci-go1.26.6-jsonv2.log`、`go-test-all.log`）中复验并记为 PASS，本记录不隐藏原始失败状态。
- 3 条（TASK-019、TASK-021、TASK-028）无独立 worker 结果文件，为主会话直接实现/验收，`worker_result` 为 null。

## 94 条用例逐项追踪

> 证据口径：本次交付仅 `evidence/TASK-031`、`TASK-033`、`TASK-034`、`TASK-035` 有独立目录；TASK-001～030、032 的单任务证据未随本次落盘，其提交号与 worker 原始结果记录在 `evidence/TASK-035/task-results.json`。下表仅 TASK-031/033/034/035 为可点击目录，其余编号为追溯标签。

| TC | 状态 | 交付任务 | 证据 |
|---|---|---|---|
| R1-01 | PASS | TASK-002, TASK-031 | TASK-002<br>[TASK-031](evidence/TASK-031/) |
| R1-02 | PASS | TASK-003, TASK-031 | TASK-003<br>[TASK-031](evidence/TASK-031/) |
| R1-03 | PASS | TASK-003, TASK-031 | TASK-003<br>[TASK-031](evidence/TASK-031/) |
| R1-04 | PASS | TASK-004, TASK-031 | TASK-004<br>[TASK-031](evidence/TASK-031/) |
| R1-05 | PASS | TASK-005, TASK-007, TASK-030, TASK-031 | TASK-005<br>TASK-007<br>TASK-030<br>[TASK-031](evidence/TASK-031/) |
| R1-06 | PASS | TASK-005, TASK-006, TASK-007, TASK-031 | TASK-005<br>TASK-006<br>TASK-007<br>[TASK-031](evidence/TASK-031/) |
| R1-07 | PASS | TASK-006, TASK-007, TASK-031 | TASK-006<br>TASK-007<br>[TASK-031](evidence/TASK-031/) |
| R2-01 | PASS | TASK-006, TASK-007, TASK-013, TASK-020, TASK-031 | TASK-006<br>TASK-007<br>TASK-013<br>TASK-020<br>[TASK-031](evidence/TASK-031/) |
| R2-02 | PASS | TASK-007, TASK-024, TASK-031 | TASK-007<br>TASK-024<br>[TASK-031](evidence/TASK-031/) |
| R2-03 | PASS | TASK-007, TASK-021, TASK-031, TASK-034 | TASK-007<br>TASK-021<br>[TASK-031](evidence/TASK-031/)<br>[TASK-034](evidence/TASK-034/) |
| R2-04 | PASS | TASK-007, TASK-030, TASK-031 | TASK-007<br>TASK-030<br>[TASK-031](evidence/TASK-031/) |
| R2-05 | PASS | TASK-006, TASK-007, TASK-031 | TASK-006<br>TASK-007<br>[TASK-031](evidence/TASK-031/) |
| R3-01 | PASS | TASK-005, TASK-031 | TASK-005<br>[TASK-031](evidence/TASK-031/) |
| R3-02 | PASS | TASK-003, TASK-031 | TASK-003<br>[TASK-031](evidence/TASK-031/) |
| R3-03 | PASS | TASK-019, TASK-020, TASK-031 | TASK-019<br>TASK-020<br>[TASK-031](evidence/TASK-031/) |
| R3-04 | PASS | TASK-019, TASK-020, TASK-031 | TASK-019<br>TASK-020<br>[TASK-031](evidence/TASK-031/) |
| R3-05 | PASS | TASK-019, TASK-031 | TASK-019<br>[TASK-031](evidence/TASK-031/) |
| R3-06 | PASS | TASK-017, TASK-018, TASK-019, TASK-020, TASK-031 | TASK-017<br>TASK-018<br>TASK-019<br>TASK-020<br>[TASK-031](evidence/TASK-031/) |
| R3-07 | PASS | TASK-019, TASK-031 | TASK-019<br>[TASK-031](evidence/TASK-031/) |
| R3-08 | PASS | TASK-019, TASK-020, TASK-031 | TASK-019<br>TASK-020<br>[TASK-031](evidence/TASK-031/) |
| R4-01 | PASS | TASK-005, TASK-030 | TASK-005<br>TASK-030 |
| R4-02 | PASS | TASK-007 | TASK-007 |
| R5-01 | PASS | TASK-013, TASK-031 | TASK-013<br>[TASK-031](evidence/TASK-031/) |
| R5-02 | PASS | TASK-013, TASK-031 | TASK-013<br>[TASK-031](evidence/TASK-031/) |
| R5-03 | PASS | TASK-013, TASK-031 | TASK-013<br>[TASK-031](evidence/TASK-031/) |
| R5-04 | PASS | TASK-012, TASK-013, TASK-031 | TASK-012<br>TASK-013<br>[TASK-031](evidence/TASK-031/) |
| R5-05 | PASS | TASK-012, TASK-013, TASK-031 | TASK-012<br>TASK-013<br>[TASK-031](evidence/TASK-031/) |
| R5-06 | PASS | TASK-012, TASK-013, TASK-031 | TASK-012<br>TASK-013<br>[TASK-031](evidence/TASK-031/) |
| R5-07 | PASS | TASK-012, TASK-013, TASK-031 | TASK-012<br>TASK-013<br>[TASK-031](evidence/TASK-031/) |
| R5-08 | PASS | TASK-001, TASK-002, TASK-013, TASK-031 | TASK-001<br>TASK-002<br>TASK-013<br>[TASK-031](evidence/TASK-031/) |
| R6-01 | PASS | TASK-014, TASK-015, TASK-031 | TASK-014<br>TASK-015<br>[TASK-031](evidence/TASK-031/) |
| R6-02 | PASS | TASK-001, TASK-014, TASK-031 | TASK-001<br>TASK-014<br>[TASK-031](evidence/TASK-031/) |
| R6-03 | PASS | TASK-001, TASK-014, TASK-031 | TASK-001<br>TASK-014<br>[TASK-031](evidence/TASK-031/) |
| R6-04 | PASS | TASK-001, TASK-014, TASK-031 | TASK-001<br>TASK-014<br>[TASK-031](evidence/TASK-031/) |
| R6-05 | PASS | TASK-001, TASK-002, TASK-014, TASK-031 | TASK-001<br>TASK-002<br>TASK-014<br>[TASK-031](evidence/TASK-031/) |
| R6-06 | PASS | TASK-024 | TASK-024 |
| R7-01 | PASS | TASK-019, TASK-024, TASK-033 | TASK-019<br>TASK-024<br>[TASK-033](evidence/TASK-033/) |
| R7-02 | PASS | TASK-024, TASK-033 | TASK-024<br>[TASK-033](evidence/TASK-033/) |
| R7-03 | PASS | TASK-021, TASK-024, TASK-033 | TASK-021<br>TASK-024<br>[TASK-033](evidence/TASK-033/) |
| R7-04 | PASS | TASK-024 | TASK-024 |
| R7-05 | PASS | TASK-024 | TASK-024 |
| R7-06 | PASS | TASK-024 | TASK-024 |
| R8-01 | PASS | TASK-021, TASK-023, TASK-024 | TASK-021<br>TASK-023<br>TASK-024 |
| R8-02 | PASS | TASK-021, TASK-024 | TASK-021<br>TASK-024 |
| R8-03 | PASS | TASK-021, TASK-024 | TASK-021<br>TASK-024 |
| R8-04 | PASS | TASK-021, TASK-023, TASK-024 | TASK-021<br>TASK-023<br>TASK-024 |
| R8-05 | PASS | TASK-024 | TASK-024 |
| R8-06 | PASS | TASK-024 | TASK-024 |
| R9-01 | PASS | TASK-017, TASK-018, TASK-019, TASK-020, TASK-031 | TASK-017<br>TASK-018<br>TASK-019<br>TASK-020<br>[TASK-031](evidence/TASK-031/) |
| R9-02 | PASS | TASK-021, TASK-022, TASK-033 | TASK-021<br>TASK-022<br>[TASK-033](evidence/TASK-033/) |
| R9-03 | PASS | TASK-021, TASK-022, TASK-033 | TASK-021<br>TASK-022<br>[TASK-033](evidence/TASK-033/) |
| R9-04 | PASS | TASK-021, TASK-022, TASK-033 | TASK-021<br>TASK-022<br>[TASK-033](evidence/TASK-033/) |
| R9-05 | PASS | TASK-022, TASK-033 | TASK-022<br>[TASK-033](evidence/TASK-033/) |
| R9-06 | PASS | TASK-022, TASK-033 | TASK-022<br>[TASK-033](evidence/TASK-033/) |
| R9-07 | PASS | TASK-022, TASK-033 | TASK-022<br>[TASK-033](evidence/TASK-033/) |
| R9-08 | PASS | TASK-024, TASK-033 | TASK-024<br>[TASK-033](evidence/TASK-033/) |
| R9-09 | PASS | TASK-024, TASK-033 | TASK-024<br>[TASK-033](evidence/TASK-033/) |
| R9-10 | PASS | TASK-023 | TASK-023 |
| R10-01 | PASS | TASK-008, TASK-009, TASK-011, TASK-031 | TASK-008<br>TASK-009<br>TASK-011<br>[TASK-031](evidence/TASK-031/) |
| R10-02 | PASS | TASK-009, TASK-011, TASK-031 | TASK-009<br>TASK-011<br>[TASK-031](evidence/TASK-031/) |
| R10-03 | PASS | TASK-009, TASK-011, TASK-031 | TASK-009<br>TASK-011<br>[TASK-031](evidence/TASK-031/) |
| R10-04 | PASS | TASK-009, TASK-011, TASK-031 | TASK-009<br>TASK-011<br>[TASK-031](evidence/TASK-031/) |
| R10-05 | PASS | TASK-008, TASK-009, TASK-011, TASK-031, TASK-032 | TASK-008<br>TASK-009<br>TASK-011<br>[TASK-031](evidence/TASK-031/)<br>TASK-032 |
| R10-06 | PASS | TASK-008, TASK-031, TASK-032 | TASK-008<br>[TASK-031](evidence/TASK-031/)<br>TASK-032 |
| R10-07 | PASS | TASK-008, TASK-031 | TASK-008<br>[TASK-031](evidence/TASK-031/) |
| R10-08 | PASS | TASK-009, TASK-011, TASK-031 | TASK-009<br>TASK-011<br>[TASK-031](evidence/TASK-031/) |
| R10-09 | PASS | TASK-009, TASK-011, TASK-031 | TASK-009<br>TASK-011<br>[TASK-031](evidence/TASK-031/) |
| R10-10 | PASS | TASK-009, TASK-011, TASK-031 | TASK-009<br>TASK-011<br>[TASK-031](evidence/TASK-031/) |
| R10-11 | PASS | TASK-012, TASK-013, TASK-017, TASK-018, TASK-031 | TASK-012<br>TASK-013<br>TASK-017<br>TASK-018<br>[TASK-031](evidence/TASK-031/) |
| R10-12 | PASS | TASK-010, TASK-011, TASK-012, TASK-013, TASK-017, TASK-018, TASK-031 | TASK-010<br>TASK-011<br>TASK-012<br>TASK-013<br>TASK-017<br>TASK-018<br>[TASK-031](evidence/TASK-031/) |
| R10-13 | PASS | TASK-008, TASK-031 | TASK-008<br>[TASK-031](evidence/TASK-031/) |
| R11-01 | PASS | TASK-027, TASK-028, TASK-029 | TASK-027<br>TASK-028<br>TASK-029 |
| R11-02 | PASS | TASK-001, TASK-026, TASK-027, TASK-028, TASK-029 | TASK-001<br>TASK-026<br>TASK-027<br>TASK-028<br>TASK-029 |
| R11-03 | PASS | TASK-026, TASK-027, TASK-028, TASK-029 | TASK-026<br>TASK-027<br>TASK-028<br>TASK-029 |
| R11-04 | PASS | TASK-010, TASK-029 | TASK-010<br>TASK-029 |
| R11-05 | PASS | TASK-027, TASK-029, TASK-034 | TASK-027<br>TASK-029<br>[TASK-034](evidence/TASK-034/) |
| R11-06 | PASS | TASK-026, TASK-028 | TASK-026<br>TASK-028 |
| R12-01 | PASS | TASK-023, TASK-025, TASK-033 | TASK-023<br>TASK-025<br>[TASK-033](evidence/TASK-033/) |
| R12-02 | PASS | TASK-023, TASK-025, TASK-033 | TASK-023<br>TASK-025<br>[TASK-033](evidence/TASK-033/) |
| R12-03 | PASS | TASK-025, TASK-033 | TASK-025<br>[TASK-033](evidence/TASK-033/) |
| R12-04 | PASS | TASK-010, TASK-025, TASK-033 | TASK-010<br>TASK-025<br>[TASK-033](evidence/TASK-033/) |
| R12-05 | PASS | TASK-021, TASK-022, TASK-023, TASK-033 | TASK-021<br>TASK-022<br>TASK-023<br>[TASK-033](evidence/TASK-033/) |
| R12-06 | PASS | TASK-023, TASK-033 | TASK-023<br>[TASK-033](evidence/TASK-033/) |
| R13-01 | PASS | TASK-016, TASK-033 | TASK-016<br>[TASK-033](evidence/TASK-033/) |
| R13-02 | PASS | TASK-016, TASK-033 | TASK-016<br>[TASK-033](evidence/TASK-033/) |
| R13-03 | PASS | TASK-015, TASK-016, TASK-033 | TASK-015<br>TASK-016<br>[TASK-033](evidence/TASK-033/) |
| R14-01 | PASS | TASK-030, TASK-033 | TASK-030<br>[TASK-033](evidence/TASK-033/) |
| R14-02 | PASS | TASK-010, TASK-029, TASK-030, TASK-033 | TASK-010<br>TASK-029<br>TASK-030<br>[TASK-033](evidence/TASK-033/) |
| R15-01 | PASS | TASK-005, TASK-030, TASK-033 | TASK-005<br>TASK-030<br>[TASK-033](evidence/TASK-033/) |
| R15-02 | PASS | TASK-005, TASK-030, TASK-033 | TASK-005<br>TASK-030<br>[TASK-033](evidence/TASK-033/) |
| PERF-01 | PASS | TASK-034 | [TASK-034](evidence/TASK-034/) |
| PERF-02 | PASS | TASK-034 | [TASK-034](evidence/TASK-034/) |
| SEC-01 | PASS | TASK-032 | TASK-032 |
| COMP-01 | PASS | TASK-033 | [TASK-033](evidence/TASK-033/) |

## 复测说明

- R9-02 初轮因扫描可变“最新 100 篇”列表而假失败；033-a6 改为自包含当前/旧 `source_version` 夹具，三浏览器重复 6/6、9/9 通过后，035 再跑完整矩阵。
- UI trace 位于 `evidence/TASK-035/ui-traces/`；性能原始数据位于 `evidence/TASK-034/final.json`。
- 本记录不表示已向生产发布；上线与回滚步骤见 `release_rollback_checklist.md`。
