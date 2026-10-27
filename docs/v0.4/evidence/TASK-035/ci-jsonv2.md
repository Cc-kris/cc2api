# TASK-035 · CI Go 1.26.6 + `GOEXPERIMENT=jsonv2` 环境修复

- 基线提交：`a6b7eaeb17e636c999b6b3193c0b887e7118a425`
- 变更范围：仅 3 个 workflow 的顶层 `env`（未改 go.mod、未改产品代码/测试）
- 原因：`backend/go.mod` 声明 `go 1.26.6`，代码 import `encoding/json/v2`、`encoding/json/jsontext`；
  这两个包在 Go 1.26 中仍受实验开关控制，必须先启用 `GOEXPERIMENT=jsonv2`，否则导入直接编译失败。

## 改动

| 文件 | 改动 |
| --- | --- |
| `.github/workflows/backend-ci.yml` | 顶层 `env: GOEXPERIMENT: "jsonv2"`（jobs: test / frontend / golangci-lint） |
| `.github/workflows/security-scan.yml` | 顶层 `env: GOEXPERIMENT: "jsonv2"`（jobs: backend-security / frontend-security） |
| `.github/workflows/release.yml` | 既有顶层 `env` 块内追加 `GOEXPERIMENT: "jsonv2"`（GoReleaser 构建继承） |

workflow 级 `env` 会下发给所有 job/step，因此 `actions/setup-go`、`make test-unit`、
`make test-integration`、`govulncheck`、`golangci-lint-action`、GoReleaser 内部 `go build` 均继承该开关。

## 验证（本机 go1.26.6 darwin/arm64，GOTOOLCHAIN=local）

| # | 命令 | 结果 |
| --- | --- | --- |
| 1 | `git diff --check` | exit 0（无空白/冲突问题） |
| 2 | `python3 -c "yaml.safe_load_all(...)"` ×3 | 3 个 workflow 均可解析；逐 job 展开 env 均含 `GOEXPERIMENT=jsonv2` |
| 3 | `go build ./...`（不带 jsonv2） | exit 1：`imports encoding/json/jsontext: build constraints exclude all Go files`（复现原故障） |
| 4 | `GOEXPERIMENT=jsonv2 go build ./...` | exit 0，8.0s |
| 5 | `GOEXPERIMENT=jsonv2 go test ./internal/server/...` | exit 0（middleware / routes 通过） |
| 6 | `GOEXPERIMENT=jsonv2 make test-unit` | 42 个包 ok；3 个包 FAIL（29 用例），**日志中 0 处 `jsontext`/`build constraints`/`setup failed`** |
| 7 | `GOEXPERIMENT=jsonv2 make test-integration` | 38 个包 ok；20 个用例 FAIL（同上，无编译类错误） |
| 8 | `git status --porcelain backend/go.mod backend/go.sum` | 空（依赖文件未被改动） |

日志：`make-test-unit.log` / `make-test-integration.log`（exit 2）、`ci-jsonv2-make-test-unit.log`（本机复跑）。

## 仍然失败但与本次改动无关的用例（基线提交即存在，未改产品代码）

- `internal/config` · `TestLoadDefaultDatabaseSSLMode`：`Database.SSLMode="disable", want "prefer"`。
  `setDefaults()` 里 `database.sslmode` 默认就是 `"disable"`（config.go:1706），测试期望 `"prefer"`，
  属于代码默认值与测试断言的既有矛盾；在 `/tmp`（无本地 config.yaml）复跑同样失败，可排除本地配置干扰。
- `internal/service`（Grok/缓存类）：如 `TestForwardAsAnthropicForGrokUsesXAIResponses` 期望型号 `grok-4.5`、
  实际 `grok-4.6`，是型号/路由期望值与当前实现不一致，与 JSON 实验开关无关（在 `/tmp` 与 backend 目录结果相同）。
- `internal/repository` 集成用例：本地 dev 数据库存在历史数据（如 `TestAccountRepoSuite/TestList` 期望 2 条、实得 10 条），
  为共享库状态问题，CI 干净库上不复现。

## 风险

1. 上述既有失败用例若在干净 CI 上不复现即无影响；若复现，属既有技术债，需另开任务修复（本次禁止改测试/产品代码）。
2. 未在真实 GitHub Actions 上跑通（本地无法触发），环境等价性由 #2–#7 的本机验证保证。
3. 本次未 commit（按要求）。
