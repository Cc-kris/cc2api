# Grok 官方代码对比审计与修复实施文档

## 文档信息

- 文档版本：1.6
- 审计日期：2026-07-24（America/New_York）
- 本地发布基线：`v0.2.110`，提交 `71919518f2f5602808724c6f8042de3af493afde`
- 本地当前基线：`v0.2.112`，提交 `4843bb048df8d9810dc0c2585708345bcce68845`
- 官方 Grok 功能分支：`feat/grok-custom-base-url-and-headers`，提交 `221581400b1c4c16fb01bfa22e93b970b66a5a64`
- 官方最新主线：`main`，提交 `cb24522dd53f8f363d008e3afdc8e4baf9788cab`，对应官方版本 `v0.1.164`
- 二次审计官方主线：`main`，提交 `37ed639d1e696daf1e3266aae3c172e837a53842`
- 官方功能分支快照：`/tmp/sub2api-official-grok.6Za1Sx`
- 官方最新主线快照：`/tmp/sub2api-official-main.dcu8tj`
- 审计对象：Grok 账号、OAuth、请求转发、模型同步、媒体、计费、调度缓存、代理检测、前端配置与测试

## 一、结论

本地 Grok 的核心实现并非“没有从官方迁移”，而是大部分核心文件已经逐字迁入；当前问题集中在三类位置：

1. 官方功能合并后继续修复的代码没有全部同步到本地。
2. Grok 接入本地已有网关、计费、调度和清理机制时，连接层存在遗漏。
3. 官方最新代码自身仍有媒体安全、异步任务归属和跨模型计费缺陷，本地迁移时一并继承。

首轮审计确认 18 个需要处理的问题，其中 P0 1 个、P1 13 个、P2 4 个。2026-07-25 二次审计进一步确认：首轮迁移覆盖了核心请求链，但遗漏了账号模型目录、分组、渠道和定价等管理面接入点，导致原生 Grok 无法从后台完整配置并投入 Codex 使用。独立的 Composite 平台不属于 Grok 模块补漏，本次不迁移。

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

### P1-11：通用账号创建、批量创建和数据导入没有触发 Grok 主动探测

- 类型：官方调用点迁移遗漏
- 影响：通过通用账号页面、批量创建或数据导入产生的 Grok 账号不会执行主动探测；只有 Grok 专用 OAuth/SSO 入口会探测，导致同类账号因创建入口不同而产生能力状态差异。
- 证据：`scheduleGrokImportProbe` 在本地只被 Grok OAuth handler 调用；官方最新代码还在 `AccountHandler.Create`、`BatchCreate` 和 `importData` 三处调用。
- 修复：补齐三个创建链路的调度调用；幂等重放仍不重复调度，探测失败不影响创建结果。
- 验证：分别覆盖单个创建、批量创建和数据导入，断言每个新 Grok 账号仅探测一次。

### P1-12：Messages 转 Grok Responses 缺少加密推理清理与单次重试

- 类型：官方调用点迁移遗漏
- 影响：Claude/Codex 多轮会话携带旧账号生成的 `thinking.signature` 时，Grok 返回无法解密的 400；本地直接进入错误/切换账号流程，新账号继续收到相同签名，形成连续失败。
- 证据：本地已迁入 `requestHasGrokEncryptedReasoning`、`stripAnthropicThinkingSignatures` 和重试状态函数，但均没有生产调用；官方在 `ForwardAsAnthropic` 的请求发送和错误处理两层接入。
- 修复：发送层在首个 400 后移除 Responses `encrypted_content` 并重试一次；仍失败时从原始 Messages 请求移除 `thinking.signature`，并用请求级标记禁止无限重试。
- 验证：模拟首次解密失败、第二次成功，断言仅发送两次、缓存身份保持不变、第二次请求不含旧加密内容。

### P1-13：Grok `/responses/compact` 响应转换函数未接入实际返回链路

- 类型：官方调用点迁移遗漏
- 影响：请求 Compact 接口时，上游返回普通 reasoning/message 数组，本地未转换成 Codex 需要的单个 `compaction` 输出；函数虽然存在且有单元测试，但真实请求永远不会调用。
- 证据：`convertGrokResponseToOpenAICompact` 只在测试中出现；官方在非流式响应处理、提取 usage 前按 Grok + Compact 路径调用。
- 修复：在非流式响应链路接入转换，并迁入按物理行识别 SSE 的判断，避免 JSON 文本里的 `data:` 被误认为 SSE 而绕过 Compact 转换。
- 验证：端到端调用非流式响应处理，断言输出为 `compaction`、加密状态和摘要保留、usage 正常提取，摘要中包含 `data:` 仍按 JSON 处理。

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

### P2-04：迁移后的 Grok 可达性没有纳入静态检查验收

- 类型：迁移与验收流程缺陷
- 影响：函数文件已存在但调用点遗漏，普通 Go 编译与函数级单测仍能通过；直到 GitHub `golangci-lint` 才以未使用函数暴露不完整迁移。
- 证据：主动探测、Messages 加密恢复和 Compact 转换均出现“实现存在、生产不可达”；同时存在大写错误文本和可简化状态分支等静态检查问题。
- 修复：以官方生产调用点为清单进行反向可达性核对；接通必要链路，删除本地架构已替代的冗余故障切换 helper，并修复本次 Grok 静态检查项。
- 验证：GitHub lint 日志不再出现本次涉及的 Grok 未使用函数、错误文本或状态分支问题。

## 三-A、2026-07-25 二次审计：原生 Grok 配置链路断点

### P0-02：Grok 账号模型目录错误回落到 Claude

- 现象：新建账号选择 Grok 后，模型选择器展示 Claude；管理员账号测试接口对 Grok OAuth 也返回 Claude 默认模型。
- 根因：前端平台值使用 `grok`，模型目录只识别 `xai`；后端管理员模型接口没有 Grok 分支，最终进入 Anthropic 默认分支。
- 官方对照：官方当前主线同时识别 `grok`/`xai`，并使用 `xai.DefaultModels()` 返回 Grok 模型。
- 修复：同步官方 Grok 模型目录、别名和预设映射；管理员模型接口增加 Grok 映射与默认目录分支。

### P0-03：Grok 分组和渠道无法从后台建立

- 现象：分组平台下拉和渠道平台列表均没有 Grok，渠道定价页面因此没有 Grok 入口。
- 根因：类型定义已包含 `grok`，页面实际选项数组和渠道平台顺序未同步。
- 影响：Grok 账号即使创建成功，也无法通过正常后台流程绑定原生 Grok 分组、渠道映射和渠道定价。
- 修复：分组创建/筛选、渠道平台段和跨页面配置验收统一加入 Grok，同时保留本地 Seedace 扩展。

### P0-04：Grok API Key 默认地址被写成 Anthropic

- 现象：账号弹窗切换到 Grok API Key 时，如果管理员不手动点击地址预设，提交值仍是 `https://api.anthropic.com`。
- 根因：平台切换监听和输入框占位没有 Grok 分支；提交时非空的错误地址覆盖了 Grok 正确默认值。
- 修复：平台切换、占位和提交三处统一使用 `https://api.x.ai/v1`。

### P0-05：OpenAI 兼容 Grok 账号被 Codex WebSocket 筛选器排除

- 现象：Codex 桌面端使用 WebSocket 请求 OpenAI 分组时，已探测为仅支持 Chat Completions 的 Grok 兼容账号仍被要求使用 Responses WebSocket，导致重连、上游失败或无可用账号。
- 根因：WebSocket 协议判定器没有读取 `openai_responses_supported=false`；首轮账号调度又只允许 WebSocket 上游，使现有 Responses 转 Chat Completions 的 HTTP 兼容逻辑无法到达。
- 修复：协议判定器将不支持 Responses 的 OpenAI API Key 账号强制定为 HTTP；Codex WebSocket 首轮允许选择 HTTP 账号，选中后直接进入 Responses 转 Chat Completions 链路；携带 `previous_response_id` 的续轮仍保持 WebSocket 会话约束。
- 验证：Codex 客户端 WebSocket 请求经 OpenAI 渠道映射为 `grok-4.5` 后，上游 HTTP Chat Completions 命中 1 次、上游 WebSocket 命中 0 次，完整结果与用量记录正常返回。

### P1-14：Grok 上游模型同步入口被前端隐藏

- 现象：后端已经支持 Grok `/models` 同步，但模型选择组件没有把 Grok 列为可同步平台；编辑账号切到模型映射模式后，旧同步按钮又会进入不支持 Grok 的后端分支。
- 修复：前端同步能力列表加入 Grok；旧模型映射入口复用同一套 Grok 上游模型能力，API Key 与 OAuth 均不再回落到 OpenAI/Anthropic 实现。
- 验证：覆盖 Grok API Key 的请求地址、鉴权头、模型去重排序和前端同步入口。

### P1-15：公共 `/v1/models` 对原生 Grok 回落到 Claude

- 现象：原生 Grok 分组没有显式账号模型映射时，公共模型接口返回 Claude 默认清单。
- 根因：公共模型接口只有 OpenAI、Gemini 两个平台分支，其余统一回落 Claude。
- 修复：按官方实现使用 xAI 默认目录，并为 Grok 4.5/Build 模型返回 Codex 可识别的推理档位元数据。

### P1-16：渠道定价同步后端拒绝 Grok

- 现象：即使绕过前端直接请求 Grok 定价模型同步，后端也返回不支持平台。
- 根因：LiteLLM provider 映射缺少 `grok -> xai`。
- 修复：加入 xAI provider 映射；渠道计价继续保持分组平台与定价平台严格一致，原生 Grok 必须使用 `platform=grok` 的价格项。

### P1-17：简单模式不会建立 Grok 默认分组

- 现象：简单模式启动只确保 Anthropic、OpenAI、Gemini、Antigravity 默认分组。
- 修复：同步官方 Grok 默认分组和默认媒体开关逻辑。

### 线上证据与现有数据边界

- 线上版本为 `v0.2.112`。数据库当前没有 `platform=grok` 的账号。
- 现有 Grok 分组 34、账号 120/126、渠道 10 和渠道价格全部标记为 `openai`；渠道映射为 `openai:* -> grok-4.5`。
- 2026-07-25 本次复核时 API Key 284 实际绑定的是分组 7，不是 Grok 分组 34；其当前用量也来自普通 GPT 账号。这个密钥在重新绑定分组 34 前不能作为 Grok 线上验收样本。
- API Key 284 在 2026-07-25 12:33 至 12:36（UTC+8）的 Codex 请求先发生 WebSocket 代理失败，HTTP 降级后收到上游 524；该时间窗没有成功用量记录。
- 当前线上链路属于 OpenAI 兼容格式的 Grok 上游，使用通用 Responses 转 Chat Completions 桥接，不触发只对 `platform=grok` 生效的 Grok OAuth 和原生媒体路由。
- 本次代码修复是在保留 OpenAI 兼容 Grok 链路的基础上，补齐并行的原生 Grok 配置能力。现有 `platform=openai` 数据不应迁移成 `platform=grok`，除非上游连接方式本身发生变化。

### 两条并行链路的最终边界

| 配置场景 | 账号平台 | 分组平台 | 渠道映射与定价平台 | 客户调用协议 | 上游处理方式 |
| --- | --- | --- | --- | --- | --- |
| OpenAI 兼容格式的 Grok 上游 | `openai` | `openai` | `openai` | OpenAI `/responses`、`/chat/completions` | 保持 OpenAI 兼容转发，按账号或渠道映射把客户模型改写为 `grok-*` |
| 原生 Grok/xAI 账号 | `grok` | `grok` | `grok` | 同样使用 OpenAI `/responses`、`/chat/completions` | 进入 Grok 专用认证、协议桥接、媒体和错误处理链路 |

平台字段表示本站连接上游时使用的适配器和定价命名空间，不表示客户请求必须使用哪一种协议。Codex 无论使用哪条链路，面向本站发送的都是 OpenAI 兼容请求。

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
5. 调用可达性：通用创建入口主动探测、Messages 加密恢复、Compact 响应转换。
6. 运维与界面：OAuth 清理、代理质量检测、媒体模型维度上报、i18n。
7. 每组完成后运行定向测试；全部完成后执行独立 Code Review、全量后端测试、前端测试、类型检查和生产构建。

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
- 原生配置链路：补齐 Grok 账号模型目录、API Key 默认地址、分组、渠道、渠道定价、公共模型清单、白名单同步、模型映射同步和简单模式默认分组。
- OpenAI 兼容 Grok：修复 Codex WebSocket 首轮对 Chat Completions-only 账号的调度与 HTTP 转换，不改变现有 OpenAI 账号、分组、渠道和定价归属。

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
- 全仓 `unit` 标签已完成编译校验；此前记录的 `oauth_refresh_api_test.go` 数据基线失败已不再出现，当前后端全量测试为通过状态。
- 前端：127 个测试文件、759 个测试用例全部通过；ESLint、Vue/TypeScript 类型检查和生产构建通过。
- 双链路回归：OpenAI 类型账号从兼容上游读取 `grok-*` 模型并转换为 Codex 清单通过；OpenAI 分组映射到 `grok-4.5` 的 HTTP/WS 请求通过；OpenAI 与原生 Grok 渠道定价双向隔离测试通过。
- 构建仅保留既有的动态/静态混合导入和大 chunk 警告，不影响本次功能与发布。
