# Grok 官方代码对比审计与修复实施文档

## 文档信息

- 文档版本：1.2
- 审计日期：2026-07-24（America/New_York）
- 本地发布基线：`v0.2.110`，提交 `71919518f2f5602808724c6f8042de3af493afde`
- 本地当前基线：提交 `3aab5a23e`
- 官方 Grok 功能分支：`feat/grok-custom-base-url-and-headers`，提交 `221581400b1c4c16fb01bfa22e93b970b66a5a64`
- 官方最新主线：`main`，提交 `cb24522dd53f8f363d008e3afdc8e4baf9788cab`，对应官方版本 `v0.1.164`
- 官方功能分支快照：`/tmp/sub2api-official-grok.6Za1Sx`
- 官方最新主线快照：`/tmp/sub2api-official-main.dcu8tj`
- 审计对象：Grok 账号、OAuth、请求转发、模型同步、媒体、计费、调度缓存、代理检测、前端配置与测试

## 一、结论

本地 Grok 的核心实现并非“没有从官方迁移”，而是大部分核心文件已经逐字迁入；当前问题集中在三类位置：

1. 官方功能合并后继续修复的代码没有全部同步到本地。
2. Grok 接入本地已有网关、计费、调度和清理机制时，连接层存在遗漏。
3. 官方最新代码自身仍有媒体安全、异步任务归属和跨模型计费缺陷，本地迁移时一并继承。

审计确认 14 个需要处理的问题，其中 P0 1 个、P1 10 个、P2 3 个。独立的 Composite 平台不属于 Grok 模块补漏，本次不迁移。

## 二、对比方法与覆盖范围

官方 Grok 功能分支建立在较早的 `v0.1.156` 附近，仅对比该分支会漏掉官方在合并后的持续修复。因此本次采用双基线：

- 用功能分支确认 Grok 初始模块边界与设计意图。
- 用官方最新主线确认合并后的修复、测试和最终行为。
- 用本地 `v0.2.110` 确认已经发布的真实行为。
- 用当前工作区确认尚未发布的临时修复，避免重复开发。

统计结果：

- 官方最新主线中引用 Grok/xAI 的文件：202 个。
- 本地对应引用文件：141 个。
- 对官方 Grok/xAI 相关文件逐项比较：76 个字节完全一致，126 个存在差异，46 个官方路径在本地不存在。
- 46 个缺失路径中包含官方拆分文件、迁移编号差异、locale 拆分和 Composite 独立平台，不能直接等同于 46 个功能遗漏。
- `backend/internal/pkg/xai/*`、OAuth、配额、媒体服务、大部分 Grok 请求协议和测试均已迁入。

## 三、问题清单

### P0-01：Grok `count_tokens` 会把 Grok 凭据发送到 Anthropic

- 类型：官方后续修复遗漏；已发布安全缺陷
- 影响：`v0.2.110` 中 Grok 分组调用 `/v1/messages/count_tokens` 时进入 Anthropic 计数链路，读取 Grok 访问令牌后发送至 `https://api.anthropic.com/v1/messages/count_tokens`。
- 本地证据：发布版路由仅对 `PlatformOpenAI` 分流，其余平台进入 `Gateway.CountTokens`。
- 官方证据：官方提交 `1433fbc4` 新增本地估算；最新主线的 `backend/internal/server/routes/gateway.go` 将 Grok 分流到 `GrokCountTokens`，`backend/internal/service/openai_gateway_count_tokens.go` 在本地解析并估算，不选择账号、不读取凭据、不访问上游。
- 修复：迁入官方本地估算器与处理器，删除当前临时 404 兜底。
- 验证：合法 Anthropic 兼容请求返回整数；非法结构返回 400；测试断言不选择账号、不读取凭据、不发出 HTTP 请求。

### P1-01：自定义渠道 `per_second` 视频计费少乘时长

- 类型：本地扩展引入的计费缺陷
- 影响：例如单价 0.10 美元/秒、10 秒、1 个视频，当前只计 0.10 美元，正确金额为 1.00 美元。
- 本地证据：`calculateOpenAIVideoCost` 的 `per_second` 分支把 `RequestCount` 设为视频数量，已解析的视频时长未参与计算；现有测试只覆盖 1 秒，掩盖了缺陷。
- 修复：`per_second` 的计费数量固定为“视频数量 × 归一化秒数”，继续复用现有统一计费器和分层价格。
- 验证：覆盖 1、8、15 秒，多视频、默认时长和分层价格。

### P1-02：Grok OAuth 在线模型同步缺失

- 类型：官方后续修复遗漏
- 影响：管理员刷新上游模型时无法按 Grok OAuth 协议请求模型列表，模型能力可能保持旧缓存或同步失败。
- 本地证据：`backend/internal/service/upstream_models.go` 仅处理 Antigravity、OpenAI、Gemini、Anthropic，没有 Grok 分支。
- 官方证据：官方最新主线实现 `buildGrokUpstreamModelsRequest`，使用 Grok token provider；仅对受信 CLI 主机添加身份头；应用账号请求头覆写；使用 Grok 模型 ID 解析规则。
- 修复：按本地 `AccountTestService` 构造方式迁入 Grok 请求构建与模型解析逻辑。
- 验证：覆盖 OAuth CLI 主机、官方 API 主机、自定义受信地址、头覆写、token 获取失败和模型解析。

### P1-03：CLI 代理 403 兼容回退缺失

- 类型：官方后续修复遗漏
- 影响：CLI 代理返回兼容性 `403 Access denied` 时，本地直接失败；这与“持续报错但偶尔有回复”及权限不足现象吻合。
- 官方证据：官方提交 `2946281a` 在 HTTP 传输边界增加一次性回退，仅对可重放、已认证、CLI 代理请求且错误属于兼容性拒绝时改发 `api.x.ai`；回退请求移除 CLI/user 身份头。
- 修复：迁入受限回退逻辑，不对额度、套餐、内容策略等业务 403 回退，不对不可重放请求回退。
- 验证：兼容性 403 回退成功；业务 403 不回退；非 CLI 主机不回退；不可重放请求不回退；回退请求不携带 CLI/user 身份头。

### P1-04：调度缓存丢失 Grok 媒体资格、计费快照和工具缓存开关

- 类型：官方后续修复遗漏 + 官方最新仍存在一项遗漏
- 影响：Redis 调度命中后，账号对象失去 `grok_media_eligible` 与 `grok_billing_snapshot`，导致媒体账号选择和免费/付费资格判断错误；同时失去 `grok_client_tool_cache_enabled`，导致后台关闭工具缓存的设置不生效。
- 本地证据：`filterSchedulerExtra` 只保留 OpenAI WebSocket 等字段；调度快照命中后直接返回裁剪账号，Grok 请求策略直接读取该账号的 `Extra`。
- 官方证据：官方提交 `eaf06917` 已保留前两个字段；官方最新仍未保留工具缓存开关。
- 修复：调度元数据保留三个 Grok 字段，不扩大到凭据或其他非调度字段。
- 验证：序列化、快照读取和策略层分别验证显式 `false` 不被改回默认 `true`。

### P1-05：Grok OAuth 清理任务未接入服务退出流程

- 类型：本地集成遗漏；当前工作区已修复
- 影响：OAuth 会话清理协程在服务退出时没有收到 `Stop()`，可能造成退出等待和资源残留。
- 官方证据：官方最新清理装配包含 Grok OAuth service。
- 修复：`provideCleanup` 注入并停止 Grok OAuth service，重新生成 Wire。
- 验证：Wire 编译测试与清理依赖测试通过。

### P1-06：Grok multipart 媒体审核只检查前 20 MiB，实际却转发完整文件

- 类型：官方最新代码自身缺陷；本地完整继承
- 影响：文件前 20 MiB 为安全内容、尾部为违规内容时，审核结果与实际发送内容不一致，形成审核绕过。
- 证据：`parseGrokMediaMultipartRequest` 使用 `io.LimitReader(part, 20MiB)` 后静默截断；`ForwardGrokMedia` 转发原始完整 multipart body；系统请求体上限远高于 20 MiB。
- 修复：读取 `限制 + 1` 字节；超限明确返回 413；畸形 multipart 返回 400；解析失败和超限均不得发往 xAI。
- 验证：刚好等于限制可通过；超过 1 字节返回 413；尾部数据不得被静默丢弃；上游调用次数为 0。

### P1-07：异步视频任务归属只存缓存，不是持久事实

- 类型：官方最新代码自身缺陷；本地完整继承
- 影响：缓存过期、Redis 重启或绑定写入失败后，合法用户查询自己的视频任务会得到 404；上游创建成功后即使绑定失败，客户端仍收到任务 ID。
- 证据：创建后用 sticky-session cache/TTL 绑定 request ID 到账号；查询链路强制依赖缓存；本地 usage log 已有 `video_task_id`、账号、用户、API Key 和分组字段，但 Grok 视频未写入该任务字段。
- 修复：新增独立的 `grok_video_task_bindings` 持久归属表。Grok 视频创建响应先缓存在服务内，按“用户 + API Key + 分组 + task ID”写入归属成功后才交付客户端；Redis 仅作为加速层。持久写入失败时返回 502，不向客户端暴露无法继续查询的任务。
- 验证：缓存命中、缓存丢失后数据库恢复、跨用户、跨 API Key、跨分组、写入失败和重复查询全部覆盖。

### P1-08：Composer 视觉桥接把两个模型用量合并后按主模型计费

- 类型：官方最新代码自身缺陷；本地完整继承
- 影响：图片描述实际由 `grok-build-0.1` 完成，主回答由 `grok-composer-2.5-fast` 完成；当前把两段 token 合并后全部按 Composer 价格计费，导致成本和客户扣费失真。
- 证据：官方与本地 `backend/internal/service/openai_gateway_grok.go` 字节一致；`bridgeGrokComposerImageInputs` 汇总视觉模型 usage，`forwardAsRawChatCompletions` 将其加到主结果 usage，后续只有一个 `BillingModel`。
- 修复：在转发结果中保存辅助模型 usage；主请求与辅助请求分别计算成本并合并总金额；用量日志继续记录总 token 和合并后的分项成本。辅助模型缺价时保留主模型费用并记录明确告警，不再把整笔请求降为零费用。
- 验证：两个模型配置不同价格时分别计价；多图时辅助用量累加；无图时不产生辅助用量；辅助模型缺价时主模型费用不归零。

### P1-09：Grok OAuth 429 没有请求级后续尝试预算

- 类型：官方后续修复遗漏
- 影响：Grok OAuth 账号返回 429 后，本地沿用 OpenAI 的全局风暴判断，可能继续切换多个账号；在混合 OAuth/API Key 池中会放大无效重试并拖长客户等待。
- 官方证据：官方最新主线增加 `OpenAIOAuth429FailoverState`，首个 Grok OAuth 429 只允许再尝试一个不同账号；后续账号无论使用 OAuth 还是 API Key，只要失败就停止切换。
- 修复：迁入请求级状态并接入 Responses、Messages、Chat Completions、Images 和 Grok Media 五条调用链。
- 验证：首个 429 不立即停止；第二个账号任意失败停止；非 429 不启动该预算；OpenAI OAuth 风暴逻辑保持原行为。

### P1-10：管理员手动刷新 Grok OAuth 账号仍落入 Anthropic 刷新链路

- 类型：本地集成遗漏；CI 回归暴露
- 影响：管理员对 Grok OAuth 账号执行手动刷新时，`refreshSingleAccount` 没有 Grok 分支，最终调用 Anthropic OAuth 服务；服务未配置时直接崩溃，配置存在时会使用错误协议。
- 证据：`AccountHandler` 已持有 Grok OAuth service，但刷新分支只识别 OpenAI、Gemini 和 Antigravity，其余平台统一进入 Anthropic；原 Grok 测试的构造参数也已落后于当前构造函数，导致 `unit` 标签 CI 无法编译。
- 修复：在 Anthropic 兜底前加入显式 Grok 分支，通过 Grok OAuth service 刷新；保留账号自定义 `base_url` 和其他非 token 凭据；测试改用 `SetGrokServices` 注入。
- 验证：断言只调用 Grok OAuth service、新 token 写入、自定义 base URL 保留、管理员与服务契约测试可编译并通过。

### P2-01：代理质量检测没有 Grok

- 类型：官方后续修复遗漏
- 影响：管理员无法用系统代理质量检测验证到 xAI 的连通性。
- 官方证据：官方提交 `b8c640d2` 增加 `https://api.x.ai/v1/models`，GET，401 视为网络可达。
- 修复：迁入 Grok 检测目标、后端验证和前端显示。
- 验证：目标 URL、方法、允许状态码和界面文案测试通过。

### P2-02：Grok 媒体 handler 未完全吸收官方最新模型维度上报

- 类型：官方后续改进遗漏与本地架构差异
- 影响：媒体模型维度的成功或失败状态如果只按账号上报，会污染同一账号其他模型的调度判断。
- 修复：继续使用本地已经存在的 `ReportOpenAIAccountScheduleModelResult`，确保 Grok Media 成功、失败与切换均携带归一化后的实际模型；429 的请求级预算由 P1-09 单独处理。
- 验证：媒体模型映射、成功清除瞬态失败、失败按实际模型上报的测试通过。

### P2-03：Grok 账号配置文案缺失

- 类型：前端迁移遗漏；当前工作区已修复
- 影响：页面直接显示 `admin.accounts.grokCustomBaseUrl.title` 等键名，用户无法理解开关含义。
- 缺失组：`headerOverride`、`grokCustomBaseUrl`、`grokClientToolCache`。
- 修复：补齐中英文文案，并按官方最新含义明确“仅已识别 Free OAuth 账号生效，可能改变客户端自动工具选择”。
- 验证：中英文键集合完整、页面不显示原始 i18n key、类型检查通过。

## 四、明确不纳入本次开发的内容

### Composite 平台

官方最新主线包含 Composite 平台及其 Grok 路由修复，本地不存在该平台的整体账户、调度、配置、计费和前端架构。它是独立产品功能，不是 Grok 模块的漏拷文件。本次不迁入 Composite 代码，也不为其增加兼容层。

### Grok 媒体分组自动启用策略

本地 migration 169 与官方 migration 158 行为一致：为 Grok 分组启用媒体生成。这是官方明确的迁移策略，不属于遗漏或代码缺陷，本次保持不变。

### 全局审核服务的 fail-open 策略

本次修复 multipart 审核内容与转发内容不一致的问题，不改变全站既有的审核服务异常策略，避免把 Grok 补漏扩大为全局业务规则调整。

### 全站无账号语义诊断框架

官方最新主线的 `404 model_not_found` / `503 temporarily unavailable` 区分依赖全站新增的 `ModelAvailabilityDiagnoser`、数据库诊断查询以及所有平台 handler 的统一改造。本地尚无该共享框架；仅复制 Grok 调用点会产生错误归类。本次保留现有 503 行为，不引入半套兼容层，该项不计为 Grok 模块交付缺口。

## 五、实施顺序

1. 安全边界：本地 `count_tokens`、multipart 严格解析。
2. 真实可用性：Grok OAuth 模型同步、CLI 403 受限回退、调度缓存字段。
3. 计费正确性：`per_second` 视频、Composer 辅助模型分项计费。
4. 任务可恢复性：视频任务持久归属与缓存恢复。
5. 运维与界面：OAuth 清理、代理质量检测、媒体模型维度上报、i18n。
6. 每组完成后运行定向测试；全部完成后执行独立 Code Review、全量后端测试、前端测试、类型检查和生产构建。

## 六、验收标准

- Grok `count_tokens` 全程不读取或发送任何上游凭据。
- CLI 403 只在官方定义的兼容错误上回退一次，业务权限错误保持原样。
- Redis 调度命中与数据库回退产生一致的 Grok 媒体和工具缓存策略。
- 所有视频计费均包含实际秒数，Composer 两种模型分别计价，辅助模型缺价不再清零主模型费用。
- multipart 超限或解析失败绝不转发上游。
- 视频任务在缓存清空后仍可由原请求方查询，其他请求方无法查询。
- Grok 代理检测、账号配置文案和退出清理正常。
- 新增回归测试覆盖上述边界；全量 Go 测试、前端测试、类型检查、构建和差异检查通过。
- 两个独立审查通道均无阻断项；审查发现写回本文档并完成修复。

## 七、开发与验证记录

### 已实施

- 安全：Grok `count_tokens` 改为本地估算；multipart 文件严格执行 20 MiB 单文件上限。
- 可用性：迁入 OAuth/API Key 模型同步、受限 CLI 403 回退、调度缓存字段与 Grok OAuth 退出清理；修复管理员手动刷新误入 Anthropic 链路。
- 计费：修复视频按秒计费；Composer 主模型与视觉辅助模型分开计价；辅助模型缺价不再清零整单。
- 异步任务：新增 migration 171 和持久归属读写；视频创建响应在持久归属成功前不提交给客户端。
- 重试：迁入 Grok OAuth 请求级 429 后续尝试预算，并接入五条请求链。
- 运维与前端：增加 Grok 代理质量目标，补齐中英文配置文案与键集合测试。

### 第一轮独立审查

- Code Reviewer：发现视频响应早于持久化、403 匹配过宽、辅助模型缺价导致整单归零三项阻断。
- Architect：发现 Grok OAuth 429 仍沿用 OpenAI 风暴逻辑、视频持久化边界不完整、辅助计费失败传播不合理三项阻断。
- 处理结果：上述阻断已全部进入本版本修复和回归测试。

### 修复后复审

- 主 Agent 按最终差异重新检查了视频任务归属、CLI 403 回退、OAuth 429 切换、跨模型计费和本地 token 估算五条高风险链路。
- 复审追加发现：图片生成响应如果携带 ID，原条件会误写入视频任务归属表。现已把绑定条件收窄到视频创建、编辑和续写端点。
- GitHub `unit` 标签 CI 追加发现：Grok 管理员手动刷新未接入 Grok OAuth service、两个测试构造器落后、服务契约桩缺少新增持久绑定接口、Header 测试使用非规范键。上述本次责任项均已修复。
- 最终结论：第一轮阻断项均已关闭；修复后复审未发现新的发布阻断项。

### 验证记录

- 后端全量：`go test ./...` 通过，包含 service、repository、handler、routes、migrations 和 Wire 装配测试。
- 后端静态检查：受影响包 `go vet` 通过；`go mod tidy -diff` 与 `git diff --check` 通过。
- 带 `unit` 标签的本次 Grok 定向测试通过，覆盖持久绑定隔离与冲突、视频响应缓冲、403 业务拒绝不回退、OAuth 429 一次后续尝试、跨模型计费、缺价边界、上游模型同步和本地 token 估算。
- 全仓 `unit` 标签已完成编译校验；执行测试时仍有未修改的 `oauth_refresh_api_test.go` 数据基线失败，对应生产代码和测试文件与当前 HEAD 无差异，不属于本次 Grok 变更的回归。
- 前端：126 个测试文件、754 个测试用例全部通过；ESLint、Vue/TypeScript 类型检查和生产构建通过。
- 构建仅保留既有的动态/静态混合导入和大 chunk 警告，不影响本次功能与发布。
