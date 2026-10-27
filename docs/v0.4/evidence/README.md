# v0.4 验收证据索引

本目录汇总本次交付可复核的证据：TASK-031/033/034/035 有独立证据目录，其余任务（TASK-001～030、032）仅记录提交号与 worker 原始结果位置，未随本次落盘独立目录。最终结论见 `../test_execution.md` 与 `../acceptance_result.md`。

## 任务提交

| TASK | Commit | 证据目录 |
|---|---|---|
| TASK-001 | `916fed7c8988` | 见 `TASK-035/task-results.json` |
| TASK-002 | `b70548354ae2` | 见 `TASK-035/task-results.json` |
| TASK-003 | `8400282774ab` | 见 `TASK-035/task-results.json` |
| TASK-004 | `944472cb92b9` | 见 `TASK-035/task-results.json` |
| TASK-005 | `4f83c13ce368` | 见 `TASK-035/task-results.json` |
| TASK-006 | `b8e2ff5ee578` | 见 `TASK-035/task-results.json` |
| TASK-007 | `158aefb0e119` | 见 `TASK-035/task-results.json` |
| TASK-008 | `d62372f9d46f` | 见 `TASK-035/task-results.json` |
| TASK-009 | `e8ce6007ad32` | 见 `TASK-035/task-results.json` |
| TASK-010 | `8f82897dd700` | 见 `TASK-035/task-results.json` |
| TASK-011 | `e28d858eebde` | 见 `TASK-035/task-results.json` |
| TASK-012 | `fef277f63138` | 见 `TASK-035/task-results.json` |
| TASK-013 | `c845dc77cd65` | 见 `TASK-035/task-results.json` |
| TASK-014 | `82b991b3da54` | 见 `TASK-035/task-results.json` |
| TASK-015 | `abe897ce5f71` | 见 `TASK-035/task-results.json` |
| TASK-016 | `714e368d9a20` | 见 `TASK-035/task-results.json` |
| TASK-017 | `6c8a074ebaef` | 见 `TASK-035/task-results.json` |
| TASK-018 | `5f2e2dacb7d4` | 见 `TASK-035/task-results.json` |
| TASK-019 | `ccc930a09b52` | 见 `TASK-035/task-results.json` |
| TASK-020 | `6cf159170808` | 见 `TASK-035/task-results.json` |
| TASK-021 | `01371048c69d` | 见 `TASK-035/task-results.json` |
| TASK-022 | `5195065d5a04` | 见 `TASK-035/task-results.json` |
| TASK-023 | `78a3ef1d2842` | 见 `TASK-035/task-results.json` |
| TASK-024 | `16042f9562af` | 见 `TASK-035/task-results.json` |
| TASK-025 | `f43beff123c9` | 见 `TASK-035/task-results.json` |
| TASK-026 | `11f2d7ced9d6` | 见 `TASK-035/task-results.json` |
| TASK-027 | `92273cf285bc` | 见 `TASK-035/task-results.json` |
| TASK-028 | `b8affd9f2c96` | 见 `TASK-035/task-results.json` |
| TASK-029 | `9c006a7e939f` | 见 `TASK-035/task-results.json` |
| TASK-030 | `0ce18c4c4b3e` | 见 `TASK-035/task-results.json` |
| TASK-031 | `0dd4ca37a490` | [`TASK-031/`](TASK-031/) |
| TASK-032 | `19e60dc592f2` | 见 `TASK-035/task-results.json` |
| TASK-033 | `aaa0c652a1db` | [`TASK-033/`](TASK-033/) |
| TASK-034 | `a6b7eaeb17e6` | [`TASK-034/`](TASK-034/) |

## 最终横切证据

- `TASK-033/`：五语言、双主题、三视口、三浏览器与 R9-02 稳定性。
- `TASK-034/final.json`：最终真实 HTTP 性能矩阵；SSR 热 p95=2.2ms，年 BI p95=360.1ms，混合并发 45.5 RPS。
- `TASK-035/browser-matrix-final.log`：最终六项目 192 项浏览器矩阵。
- `TASK-035/ui-traces/`：22 个成功 UI trace。
- `TASK-035/make-test-frontend.log`：6 个关键前端文件、98 项测试。
- `TASK-035/backend-unit-ci-go1.26.6-jsonv2.log`：Go 1.26.6/jsonv2 CI 定向后端测试。
- `TASK-035/ci-jsonv2.md`：CI、Release、Security workflow 的 Go 实验环境修复。
- `TASK-035/task-results.json`：前 34 个任务的提交、原始 worker 结果来源与主会话验收状态。
- `TASK-035/traceability.json`：94 TC 与 17 AC 机器可读追踪。

## 结论边界

- 内容中心 94 TC/17 AC 与全仓所有历史测试不是同一口径；既有 15 项失败（Go 12：gateway 5、repository 6、SSL 1；前端 3：locale parity）单独披露。
- 性能测试使用隔离 PostgreSQL/Redis 与真实 HTTP，不是 mock。
- 上线清单不等于生产已发布；生产执行结果必须另行回填。
