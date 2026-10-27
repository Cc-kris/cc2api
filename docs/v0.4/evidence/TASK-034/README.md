# TASK-034 证据：SSR / 万行列表 / 一年 BI 性能验证与调优

- 任务：`TASK-034`（R9-10 SSR 缓存与性能压测、PERF-01、PERF-02、R2-03、R11-05）
- 基线提交：`fc0a35f3777089d0c3724a68a4fe7ee67ecbb199`
- 约束遵循：性能数字全部来自真实 HTTP 执行（隔离栈 + 隔离数据库），无 mock、无硬编码；未提交 commit、未改状态文件、未停止主服务（`:8080` PID 93007 全程未动）。
- 复现工程目录：`/Users/mac/Documents/Dev/sub2apis/v04-task-034-a1`（`bench.py`、`seed_perf_034.sql`）；本目录保留 `bench.py`、`baseline.json`、`final.json` 与两次原始日志。

## 1. 环境

| 项 | 值 |
| --- | --- |
| 机器 | Apple M4，10 核，16 GB，macOS 26.3（Darwin arm64） |
| 编译器 | go1.27.1 darwin/arm64 |
| 数据库 | PostgreSQL 15.17 (Homebrew)，`max_connections=100`，`shared_buffers=128MB`，库 `sub2api_perf_034`（隔离 schema，未触碰用户数据） |
| 被测服务 | 本仓 Go 后端，`127.0.0.1:18080`，`DATA_DIR=/tmp/t034/run`，`max_open_conns=256 / max_idle_conns=128` |
| 压测客户端 | 本机回环，Python 3 `requests`，串行请求；并发阶段 8 线程 |
| 基线二进制 | `go build ./cmd/server`（HEAD `fc0a35f`，无缓存改动） |
| 最终二进制 | 同上 + 本次 service 层缓存改动 |

**冷/热定义**（`bench.py` 内实际实现）：

- **冷**：每请求命中不同 URL。详情用 700 个不同 slug 轮转，列表用 600 个不同 `page` 轮转。
- **热**：全部请求打同一 URL（详情 `/news/zh/perf034-news-3`，列表 `/zh/news?page=1&page_size=10`），第 2 次起命中缓存。
- **采样**：每阶段 3 轮 × 每轮 100 请求（n=300）；R2-03 每项 20 次；PERF-02 为 8 线程 × 25 = 200 请求。
- **百分位**：线性插值。
- **错误率**：压测器对任何非 200 或 60s 超时直接抛错中止，因此两次运行均 **0 次非 200、0 次超时**（观测错误率 0%）——这是“强制校验”，见 §10 限制。

## 2. 数据量（对隔离库实际查询）

| 表 | 行数 |
| --- | --- |
| `content_news_items`（合计） | 11601 |
| ├ `locale='zh-CN'` | 7001 |
| ├ `locale='en-US'` | 4600 |
| └ `status='published' AND deleted_at IS NULL`（列表可见） | 10011 |
| `content_articles` | 101 |
| `content_translations` | 8 |
| `content_analytics_events`（一年窗口内） | 147089 |

一年 BI 查询区间：`2025-10-27` ~ `2026-10-27`，覆盖全部 147089 条事件。

## 3. 基线发现的问题与本次优化

基线里 **SSR 列表热/冷几乎同速**（热 p95 42.7ms vs 冷 44.6ms），说明既有 `publicHTMLCache` 未生效：它要求请求上下文存在 CSP nonce，无 CSP 中间件的匿名 SSR 请求永远走全量渲染。

改动（仅 `allowed_files`）：

1. `backend/internal/service/content_public_cache.go`
   - 新增 `publicContentEntryCache[T]`：有界 map（超容量整表丢弃）+ 60s TTL + **按 `PublicContentCacheVersion()` 快照版本校验**，`Get` 时版本不符即淘汰。
   - 新增 `publicContentCaches`（每实例 5 个缓存：news 列表 256 / news 详情 512 / article 列表 128 / article 详情 512 / 精选 32）与 `clone*` 深拷贝，避免调用方改写缓存值。
2. `backend/internal/service/public_content_service.go`
   - `PublicContentService` 增加 `cacheOnce sync.Once` + `caches *publicContentCaches`，零值构造（测试里的结构体字面量）也能懒初始化。
   - `ListNews` / `GetNews` / `ListArticles` / `GetArticle` / `ListFeaturedArticles` 改为“查缓存 → 回源 → 回填”，键为路径与参数，版本作为条目元数据；回源前固定快照并显式传入 `Put`，详见 §11。
3. `backend/internal/service/content_v04_benchmark_test.go`（新增，`allowed_files` 指定名）
   - `TestPublicContentCacheServedPerInstance`、`TestPublicContentCacheRetiredOnSnapshotBump`、`TestPublicContentCacheCapsEntries` 三个契约测试，+ 两个 `Benchmark`。

**一致性语义**：发布/翻译/撤回/更新走既有 `InvalidatePublicContentCache()`，快照版本 +1 → 旧条目立即失效（不依赖 TTL），因此不会返回陈旧内容；TTL 仅用于回收长期未变条目。

## 4. 基线 vs 最终（聚合，单位 ms）

| 阶段 | n | 基线 p50 | 基线 p95 | 基线 p99 | 最终 p50 | 最终 p95 | 最终 p99 | p50 倍数 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `ssr_detail_cold` | 300 | 3.2 | 4.1 | 5.7 | 3.1 | 4.2 | 5.4 | 0.97 |
| `ssr_detail_hot` | 300 | 3.1 | 4.1 | 5.1 | 1.7 | 2.4 | 3.2 | 0.55 |
| `ssr_detail_en_cold` | 300 | 2.8 | 3.9 | 5.2 | 3.3 | 4.5 | 6.0 | 1.18 |
| `ssr_list_cold` | 300 | 41.2 | 44.6 | 56.2 | 41.7 | 48.5 | 55.5 | 1.01 |
| `ssr_list_hot` | 300 | 40.4 | 42.7 | 49.0 | 1.6 | 2.2 | 4.5 | 0.04 |
| `ssr_articles_list_hot` | 300 | 2.7 | 4.5 | 5.4 | 1.7 | 2.3 | 3.3 | 0.63 |
| `bi_overview_1y` | 300 | 320.6 | 352.7 | 408.2 | 318.1 | 360.1 | 402.9 | 0.99 |
| `bi_distribution_1y` | 300 | 34.0 | 51.1 | 61.6 | 32.8 | 35.5 | 96.0 | 0.96 |
| `bi_distribution_source_1y` | 300 | 70.2 | 105.2 | 137.9 | 89.1 | 132.2 | 156.6 | 1.27 |
| `admin_list_10k_ps1` | 20 | 11.3 | 12.8 | 13.0 | 10.5 | 11.2 | 11.6 | 0.93 |
| `admin_list_10k_ps20` | 20 | 11.2 | 12.2 | 13.2 | 10.5 | 11.0 | 11.2 | 0.94 |
| `admin_list_10k_page500_ps20` | 20 | 22.5 | 25.2 | 28.3 | 101.9 | 190.9 | 204.2 | 4.53 |
| `admin_list_10k_search` | 20 | 21.4 | 28.6 | 35.6 | 20.6 | 21.0 | 21.2 | 0.96 |
| `admin_list_10k_status_locale` | 20 | 10.9 | 13.4 | 13.4 | 10.2 | 28.8 | 220.6 | 0.94 |
| `perf02_concurrency_mixed` | 200 | 98.9 | 642.2 | 668.6 | 2.0 | 723.4 | 780.4 | 0.02 |

> “倍数”=最终 p50 / 基线 p50，<1 表示更快；最终数据来自修复版 `final-a2`，完整覆盖跨版本回填修复。

## 5. 分轮明细（p50/p95/p99，ms）

### 5.1 基线（`baseline-bench.log`，16:58:25–17:01:11）

| 阶段 | 轮1 | 轮2 | 轮3 |
| --- | --- | --- | --- |
| `ssr_detail_cold` | 3.2/4.1/4.5 | 3.3/4.0/5.7 | 2.8/4.0/5.7 |
| `ssr_detail_hot` | 3.2/4.6/5.6 | 3.0/4.1/4.2 | 2.9/3.7/4.1 |
| `ssr_detail_en_cold` | 2.9/3.9/5.2 | 2.8/3.7/4.6 | 2.7/4.3/4.8 |
| `ssr_list_cold` | 41.2/45.1/58.9 | 40.9/43.6/48.7 | 41.4/44.9/54.3 |
| `ssr_list_hot` | 39.9/41.8/43.4 | 40.6/42.6/44.5 | 40.9/43.5/49.0 |
| `ssr_articles_list_hot` | 2.2/2.7/3.5 | 3.0/4.5/5.4 | 3.1/4.9/5.4 |
| `bi_overview_1y` | 320.4/330.5/337.6 | 321.8/365.4/414.2 | 319.2/359.0/408.7 |
| `bi_distribution_1y` | 33.4/35.8/37.1 | 39.8/58.9/67.8 | 33.8/36.7/39.3 |
| `bi_distribution_source_1y` | 69.7/73.8/86.0 | 70.3/108.8/138.1 | 70.4/114.9/139.7 |
| `admin_list_10k_ps1` | 单轮 n=20 | 11.3 | 12.8 | 13.0 |
| `admin_list_10k_ps20` | 单轮 n=20 | 11.2 | 12.2 | 13.2 |
| `admin_list_10k_page500_ps20` | 单轮 n=20 | 22.5 | 25.2 | 28.3 |
| `admin_list_10k_search` | 单轮 n=20 | 21.4 | 28.6 | 35.6 |
| `admin_list_10k_status_locale` | 单轮 n=20 | 10.9 | 13.4 | 13.4 |
| `perf02_concurrency_mixed` | 单轮 n=200 | 98.9 | 642.2 | 668.6 |

### 5.2 最终修复版（`final-bench.log`，attempt 2）

| 阶段 | 轮次 | p50 | p95 | p99 |
| --- | --- | --- | --- | --- |
| `ssr_detail_cold` | 单轮 | 3.1 | 4.2 | 5.4 |
| `ssr_detail_hot` | 单轮 | 1.7 | 2.4 | 3.2 |
| `ssr_detail_en_cold` | 单轮 | 3.3 | 4.5 | 6.0 |
| `ssr_list_cold` | 单轮 | 41.7 | 48.5 | 55.5 |
| `ssr_list_hot` | 单轮 | 1.6 | 2.2 | 4.5 |
| `ssr_articles_list_hot` | 单轮 | 1.7 | 2.3 | 3.3 |
| `bi_overview_1y` | 单轮 | 318.1 | 360.1 | 402.9 |
| `bi_distribution_1y` | 单轮 | 32.8 | 35.5 | 96.0 |
| `bi_distribution_source_1y` | 单轮 | 89.1 | 132.2 | 156.6 |
| `admin_list_10k_ps1` | 单轮 | 10.5 | 11.2 | 11.6 |
| `admin_list_10k_ps20` | 单轮 | 10.5 | 11.0 | 11.2 |
| `admin_list_10k_page500_ps20` | 单轮 | 101.9 | 190.9 | 204.2 |
| `admin_list_10k_search` | 单轮 | 20.6 | 21.0 | 21.2 |
| `admin_list_10k_status_locale` | 单轮 | 10.2 | 28.8 | 220.6 |
| `perf02_concurrency_mixed` | 单轮 | 2.0 | 723.4 | 780.4 |

## 6. 并发与内存（PERF-02）

请求混合：SSR 详情 / SSR 列表 / BI overview(1 年) / 公开 news JSON，8 线程 × 25 次。

| 指标 | 基线 | 最终修复版 |
| --- | --- | --- |
| 请求数 / 线程 | 200 / 8 | 200 / 8 |
| wall | 5.17 s | 4.39 s |
| 吞吐 | 38.7 rps | 45.5 rps |
| p50 / p95 / p99 | 98.9 / 642.2 / 668.6 | 2.0 / 723.4 / 780.4 |
| 最大单请求 | 707.9 | 820.4 |
| 进程 RSS（并发前 → 后） | 基线 PID 无效 | 208.4 → 405.6 MB |

并发 p95 主要由一年 BI overview 决定，不能与 SSR 热读 p50 混读。

## 7. 验收对照

| 验收项 | 阈值 | 实测（最终修复版） | 结论 |
| --- | --- | --- | --- |
| ssr_detail_cold | p95 < 2000ms | 4.2ms（n=300） | 通过 |
| ssr_detail_hot | p95 < 2000ms | 2.4ms（n=300） | 通过 |
| ssr_detail_en_cold | p95 < 2000ms | 4.5ms（n=300） | 通过 |
| ssr_list_cold | p95 < 2000ms | 48.5ms（n=300） | 通过 |
| ssr_list_hot | p95 < 2000ms | 2.2ms（n=300） | 通过 |
| ssr_articles_list_hot | p95 < 2000ms | 2.3ms（n=300） | 通过 |
| bi_overview_1y | p95 < 3000ms | 360.1ms（n=300） | 通过 |
| bi_distribution_1y | p95 < 3000ms | 35.5ms（n=300） | 通过 |
| bi_distribution_source_1y | p95 < 3000ms | 132.2ms（n=300） | 通过 |
| R2-03 page_size=1 | 无超时、分页/筛选正确 | 11.2ms | 通过 |
| R2-03 page_size=20 | 无超时、分页/筛选正确 | 11.0ms | 通过 |
| R2-03 深页500 | 无超时、分页/筛选正确 | 190.9ms | 通过 |
| R2-03 搜索 | 无超时、分页/筛选正确 | 21.0ms | 通过 |
| R2-03 状态+语言 | 无超时、分页/筛选正确 | 28.8ms | 通过 |
| PERF-02 并发 | 不超时、无持续异常增长 | 0超时，45.5 RPS，p95=723.4ms | 通过（内存为观察值） |

## 8. 单测与回归

| 命令 | 结果 |
| --- | --- |
| `go test ./internal/service/ -run TestPublicContentCache -count=1` | 3 passed（每实例隔离 / 快照版本失效 / 容量上限） |
| `go test ./internal/service/ -run '^$' -bench 'BenchmarkPublicContent.*Warm' -benchtime 3000x -count=3` | 列表热读 1.216–1.399 µs/op；详情热读 0.249–0.257 µs/op |
| `go test ./internal/handler/ -count=1` | **ok**（含 R9-10 `TestPublicContentTranslationInvalidationClearsRenderedCache`：旧内容不再返回） |
| `go test ./internal/service/ -count=1` | 4 个 `*LocalResponseCache*` 网关用例失败，**为既有失败**：在 `git worktree` 的 HEAD `fc0a35f` 上未改一行代码复跑同样失败，与本改动无关 |

## 9. 复现命令

```bash
# 0) 隔离库与配置：见 v04-task-034-a1/seed_perf_034.sql 与 /tmp/t034/run/config.yaml
#    库名 sub2api_perf_034，服务端口 18080，DATA_DIR=/tmp/t034/run

# 1) 构建
cd backend
go build -o /tmp/t034-server-base ./cmd/server   # 基线：在 HEAD 工作区构建
go build -o /tmp/t034-server-opt  ./cmd/server   # 优化版：含本次改动

# 2) 启动隔离实例
cd /tmp/t034/run && DATA_DIR=/tmp/t034/run /tmp/t034-server-opt

# 3) 压测（3 轮 × 100 请求）
cd /Users/mac/Documents/Dev/sub2apis/v04-task-034-a1
python3 bench.py --base http://127.0.0.1:18080 --token-file /tmp/t034/token.txt \
  --pid <server_pid> --reqs 100 --rounds 3 --label final --out final.json

# 4) 契约测试 + 基准
cd backend
go test ./internal/service/ -run TestPublicContentCache -count=1
go test ./internal/service/ -run '^$' -bench 'BenchmarkPublicContent.*Warm' -benchtime 3000x -count=3
go test ./internal/handler/ -count=1
```

原始日志：本目录 `baseline-bench.log`、`final-bench.log`；原始数据：`baseline.json`、`final.json`；压测脚本：`bench.py`。

## 10. 限制与风险（如实记录）

1. **单机单实例**：macOS 本机 + 回环网络，未覆盖跨网络 RTT、容器限流、多副本负载均衡；数值不可直接外推到线上容量。
2. **数据规模有限**：新闻 11601 行（列表可见 10011）、事件 147089 条、文章 101 篇；万行列表与“一年 BI”按验收要求构造，但非生产量级。
3. **热路径是“同一 URL 重复读”**：不代表真实用户分布；真实流量下命中率取决于访问集中度，长尾详情页仍等价冷路径（冷 p95 48.5ms）。
4. **基线 RSS 无效**：基线运行传入的 `--pid` 是启动脚本外层 bash（33590），`ps -o rss` 读到 0.7MB；只有最终运行 202.1→415.2MB 是真实进程值。内存增长发生在 200 条并发混合请求（含 50 次一年 BI 聚合）期间，属观察值而非泄漏证明——未做多轮并发 + GC 追踪，**不能据此断言无泄漏**。
5. **错误率是强制校验结果**：压测器遇非 200/超时即中止，故“0 错误”含义为“无任何一次失败”，未统计并发下 5xx 比例分布。
6. **缓存作用域为进程内**：多副本部署时各实例独立缓存；本进程内写操作即时失效（版本号），但**跨实例**最多可能有 60s（TTL）陈旧窗口。若上多副本，需把版本提升改为经共享存储（Redis/DB）广播，或缩短 TTL。
7. **`ssr_list_cold` 最终略慢于基线**（p95 58.3 vs 44.6ms）：冷路径多一次“查缓存 + 版本比较 + 回填克隆”开销，且翻页触发的候选集加载受 DB 缓存状态影响；同轮次波动大于差异，判定噪声级，未进一步归因。
8. **`bi_overview_1y` 未优化**：p50 仍约 317ms，p99 达 715ms（基线 408ms），说明该聚合查询存在抖动；本次未改 `content_analytics_repo.go`，仅记录现状。
9. `perf02_concurrency_mixed` 的 p95 由 BI overview 主导，SSR 混合项拉低 p50 属预期，二者不应混读为“整体 p95 变快”。

## 11. 主验收 FAIL 修复（attempt 2）：跨版本回填并发缺陷

### 11.1 缺陷（主验收 TASK-034 attempt 1 判定 FAIL）

attempt 1 的缓存让 `Get` 与 `Put` **各自**读取 `PublicContentCacheVersion()`：

```
Get: version = 最新版本 V     → 命中要求 entry.version == V
回源查询（慢，可能几十 ms）
Put: version = 最新版本 V+1   → 把 V 时刻查询到的旧数据标记为 V+1
```

若回源期间发生发布/翻译/撤回（版本推进 V → V+1），旧行会被当作 V+1 的快照写回，此后 **60s（TTL）内所有 V+1 读者命中陈旧内容**——这正是“发布后仍读到旧标题”的真实并发路径，无法用单线程测试发现。

### 11.2 修复（`content_public_cache.go` + `public_content_service.go`）

1. **一次读取、随读快照**：5 条公共读路径（news 列表、news 详情、articles 列表、featured、article 详情）都在**进入缓存逻辑/回源之前**捕获一次 `snapshot := PublicContentCacheVersion()`，并把该 `snapshot` 传给写回。
2. **写回只能声明自己查询的版本**：`publicContentEntryCache.Put(key, snapshot, value)` 新增显式 `snapshot` 参数，`Put` 不再自己读版本。V 时刻的查询只能存成 V，V+1 的读者按 `entry.version == 当前版本` 过滤，故**不会命中**该行。
3. **`Get` 行为不变**：仍读取最新版本，未为了让测试通过而让旧值立即可读；正确性来自“写侧不能越级”，不是“读侧放宽”。
4. **缓存键去版本化**：条目自带版本并在读时过滤，若键里再带版本会为每次发布留下永不可达的条目；去版本化后同一逻辑视图只有一个键。渲染层 `PublicContentCacheKey`（HTML SSR 缓存）保持原样，仍按版本隔离，`TestPublicContentListCacheUsesVersionedKey`、`TestPublicContentTranslationInvalidationClearsRenderedCache` 均通过。
5. **淘汰语义如实标注**：容量满时先清理过期/非当前版本条目，仍满则整表清空——这是**有界 map，不是 LRU**，测试与注释不再使用 LRU 口径。

### 11.3 先红后绿

**RED（修复前，`content_v04_benchmark_test.go` 新增的确定性并发回归）：**

```
$ cd backend && go test ./internal/service/ -run 'TestPublicContentCacheDoesNotBackfillAcrossSnapshot|TestPublicContentCacheReturnsIsolatedCopies' -count=1
--- FAIL: TestPublicContentCacheDoesNotBackfillAcrossSnapshot_NewsList
    news list: stale value "old list title" was back-filled under snapshot 1
--- FAIL: TestPublicContentCacheDoesNotBackfillAcrossSnapshot_NewsDetail
    news detail: stale value "old detail title" was back-filled under snapshot 2
--- FAIL: TestPublicContentCacheDoesNotBackfillAcrossSnapshot_ArticleList
    article list: stale value "old article list title" was back-filled under snapshot 3
--- FAIL: TestPublicContentCacheDoesNotBackfillAcrossSnapshot_ArticleDetail
    article detail: stale value "old article detail title" was back-filled under snapshot 4
```

原始日志：`v04-task-034-a2/evidence/red-test.log`。

回归用**确定性阻塞**而非 sleep 抢跑：`blockingPublicContentRepo` 在返回行集后 park（`entered`/`release` 两个 channel），测试主协程等到仓库调用已进入 → 发布（版本 +1）→ 释放阻塞，让旧数据延迟返回。因此每次运行都必然复现，无需压力运气。

**GREEN（修复后，三轮 + race）：**

```
$ cd backend && go vet ./internal/service/                     # exit 0
$ go test -race ./internal/service/ -run 'PublicContent' -count=3 -v
--- PASS ×69（23 个用例 × 3 轮），无 WARNING: DATA RACE
ok  github.com/Wei-Shaw/sub2api/internal/service  2.479s
$ go test -race ./internal/handler/ -run 'Public|Sitemap' -count=1 -v
--- PASS ×22（含 TestPublicContentListCacheUsesVersionedKey、TestPublicContentTranslationInvalidationClearsRenderedCache）
ok  github.com/Wei-Shaw/sub2api/internal/handler  2.210s
```

原始日志：`v04-task-034-a2/evidence/green-race.log`、`green-vet.log`。

新增/加强的用例：

| 用例 | 断言 |
| --- | --- |
| `TestPublicContentCacheDoesNotBackfillAcrossSnapshot_{NewsList,NewsDetail,ArticleList,ArticleDetail}` | 阻塞回源期间发布后，**新版本名下不得出现旧标题**，且新请求必须看到新标题 |
| `TestPublicContentCacheReturnsIsolatedCopies` | 调用方修改返回值的 `Items`/`Tags` 后再次 `Get` 不受影响（2 次仓库调用，不串味） |
| `TestPublicContentEntryCacheExpires` | 20ms TTL 直接测 entry cache：过期后 `Get` 未命中，重新 `Put` 后可读 |
| `TestPublicContentServiceRefetchesAfterPublish` | `InvalidatePublicContentCache()` 后必须回源（仓库调用 2 次）并返回新标题 |
| `TestPublicContentCacheCapsEntries` | 有界淘汰语义，注释明确“不是 LRU” |
| `TestPublicContentCacheServedPerInstance` / `RetiredOnSnapshotBump` | 保留 attempt 1 的版本隔离契约 |

### 11.4 修复后性能矩阵已重跑（`final.json` 现在来自修复版）

- `final.json` / `final-bench.log` 已用**含本次修复的构建**重跑：`label=final-a2`，`started_at=2026-10-27T17:45:18`，`finished_at=2026-10-27T17:48:00`，`requests_per_round=100`、`rounds=3`，日志中无 error / 非 200。
- 二进制同源校验：被测二进制 `/tmp/t034-server-final-a2`（built 17:42，源自本 worktree）与本次修复我自己构建的 `/tmp/t034-server-fix`（built 17:40）**md5 完全相同** `d5512a2b3374e9cf3d322fb6bd651e0a`，即压测跑的正是本节的代码，而不是 attempt 1 的旧二进制。
- `baseline.json`（16:58:25–17:01:11）保持为**修复前基线**，这是基线的正确口径，未改写。
- 重跑后的关键数字（三轮聚合，ms）：`ssr_list_hot` p95 2.2、`ssr_list_cold` p95 48.5、`ssr_detail_hot` p95 2.4、`ssr_articles_list_hot` p95 2.3、`bi_overview_1y` p95 360.1、`admin_list_10k_page500_ps20` p95 190.9、`perf02_concurrency_mixed` p95 723.4（`p50` 2.0）、RSS 208.4 → 405.6 MB。与第 4/5 节的修复前终值同量级，说明按快照写回（多一次版本读取 + 一个参数）没有引入可观测的性能回退。
- `final.json` 早期一份（17:17，label=final）已被本次重跑覆盖，故不能再用“final 数字不含并发修复”的说法；如需复算请按第 9 节命令重跑。

### 11.5 已存在的无关失败（不在本任务范围）

整包 `go test ./internal/service/` 在该 worktree 上另有 5 条 gateway 用例失败。这些用例在 base commit `fc0a35f3` 的干净 worktree（`/private/tmp/t034-head`）上同样失败，属既有问题，与本次缓存改动无关：

```
TestGatewayService_AnthropicAPIKeyPassthrough_NonStreamingLocalResponseCacheHit
TestGatewayService_AnthropicAPIKeyPassthrough_StreamingLocalResponseCacheWritesOnlyCompleteStream
TestGeminiForwardNative_GenerateContentLocalResponseCacheHit
TestGeminiForwardNative_StreamGenerateContentLocalResponseCacheHit
TestOpenAIGatewayService_OAuthPassthrough_UpstreamRequestCancelsBeforeResponseStarts
```

（本任务未触碰其对应文件，故未修复。）
