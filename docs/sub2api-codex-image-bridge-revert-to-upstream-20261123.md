# Codex 生图桥接恢复官方逻辑 — 技术方案（2026-11-23）

- 基线：线上版本 `v0.3.17` = commit `146e08aa8`（`origin/release/v0.3.17`，`backend/cmd/server/VERSION` = `0.3.17`）
- 目标分支：`fix/codex-image-bridge-upstream`（已从 `146e08aa8` 创建）
- 对照基准：`upstream/main`（`Wei-Shaw/sub2api`，VERSION = `0.2.24`）
- 目标：**删除 fork 自造的 Codex 生图 extension 机制，改为与官方 sub2api 完全一致的 hosted `image_generation` 工具注入机制**

---

## 一、结论与判定依据

fork 与官方在这一块是**两套互不相干的实现**，不是同一套逻辑的版本差：

| 维度 | 官方 sub2api (`upstream/main`) | 我们的 fork (`v0.3.17`) |
|---|---|---|
| 核心思路 | 服务端**注入** hosted `image_generation` 工具 + `tool_choice=auto` + bridge instructions，让上游原生出图 | 服务端**伪造 extension**：判定本回合是"文本编排"还是"图片执行"，把请求改写/续写到不同分组，本地拼装图片请求 |
| 开关来源 | account override → channel override → `gateway.codex_image_generation_bridge_enabled`（默认 false） | 仅 channel `features_config.codex_image_generation_bridge`（可为 bool 或 `{platform: bool, orchestrator_group_id: int}`） |
| 分组编排 | 无。分组只做权限门（`GroupAllowsImageGeneration`） | 有。`orchestrator_group_id` 指定"文本编排组"，图片请求另走 key 的图片组 |
| 代码规模 | 桥接本体 ~110 行（`codex_image_generation_bridge.go`） | 桥接本体 81 行 + `image_generation_intent.go` 内 33 个专属函数 + `codex_image_prompt_controls.go` 620 行 |

**因此本方案不是"改几行分支条件"，而是"删掉自造机制 + 移植官方机制"。**

前置决策依据（已由真机验证得出，见对话上下文）：Codex Desktop 第一发请求永远是文本模型，官方机制在 `bridge_enabled=false`（默认）时行为就是**纯透传**，不注入任何东西；生图能力由「分组 + 模型映射 + `GroupAllowsImageGeneration` 权限门」提供，不需要 extension 编排。

---

## 二、范围界定

### 2.1 要删除（fork 专有，官方无对应）

**A. 后端机制代码**

| 文件 | 删除内容 |
|---|---|
| `backend/internal/service/image_generation_intent.go` | 33 个 fork 专有函数（见 §2.3 清单）；保留 14 个与官方逐字相同的函数 |
| `backend/internal/service/openai_codex_transform.go` | 4 个 fork 专有函数：`NormalizeOpenAIWSImageGenerationChannelMapping`、`compactCodexCallID`、`normalizeOpenAIResponsesImageGenerationToolsForModel`、`normalizeOpenAIResponsesImageGenerationToolsInBody`（后两个若被官方同名函数替代则直接删） |
| `backend/internal/service/codex_image_generation_bridge.go` | 重写为官方版本（删 `CodexImageGenerationOrchestratorGroupID`） |
| `backend/internal/service/codex_image_prompt_controls.go` | 整文件删除（620 行，仅被 `openai_images.go:91` 调用） |

**B. 判定/调度调用点**（`backend/internal/handler/openai_gateway_handler.go`，行号为 `v0.3.17` 现状）

- `:314` `IsImageGenerationPermissionIntentForPlatform` → 改回官方 `IsImageGenerationIntentForPlatform`（或保留兼容壳，见 §3.2）
- `:326` `ClassifyCodexImageRequest` + `codexRoute`
- `:341-342` `IsCodexSystemBackgroundTurn` / `IsSuccessfulCodexImageGenerationExtensionContinuation`
- `:365` `codexDecision.IsImageExecution()`
- `:629` `PrepareCodexImageRouteRequest`
- `:1630` `AttachCodexTurnMetadata`
- `:1668-1706` WS 分支的 `ClassifyCodexImageRequest` / `IsImageExecution` / `TextBypass` / `IsCodexSystemBackgroundTurn`
- `:1731` `:1738` `:1741` `:1772` `:1839` `:1842` 路由/映射筛选里的 extension 判定

**C. service 层调用点**

- `openai_gateway_service.go:2723`、`:3054`、`:3660` — `IsImageGenerationPermissionIntentMap` / `IsImageGenerationPermissionIntent` → 统一改官方 `IsImageGenerationIntent*`
- `openai_gateway_service.go:3067` — 保留（官方也有 `IsImageGenerationIntentMap` 计费判定），但需与官方新版语义对齐
- `openai_ws_forwarder.go:2872` — 同上
- `openai_gateway_responses_chat_fallback.go:34` — 同上
- `openai_images.go:85` `SetCodexImageResponseAdapterEnabled`、`:91` `ApplyCodexImagePromptControls`、`:606`、`:981`、`openai_images_responses.go:756` — 删除适配器开关与 prompt controls 调用

**D. 前端配置面**

- `frontend/src/views/admin/ChannelsView.vue:355-365`（"编排组"下拉）、`:700-701`、`:798-799`、`:1169-1178`、`:1247`、`:1263-1264`、`:1488-1489` — 删除 `codex_image_orchestrator_group_id` 相关全部逻辑，只保留官方语义的 `codex_image_generation_bridge` 布尔开关（按 platform 存 map）

### 2.2 要移植（官方有、fork 没有）

| 位置 | 内容 |
|---|---|
| `codex_image_generation_bridge.go` | `featureKeyCodexImageGenerationExplicitToolPolicy` 常量、`codexImageGenerationExplicitToolPolicyAllow/Strip`、`boolOverrideFromMap`、`stringOverrideFromMap`、`normalizeCodexImageGenerationExplicitToolPolicy`、`(*Account).CodexImageGenerationBridgeOverride()`、`(*Account).CodexImageGenerationExplicitToolPolicy()` |
| `openai_gateway_service.go` | 方法 `(*OpenAIGatewayService).isCodexImageGenerationBridgeEnabled(ctx, account, apiKey)`（官方 `:629`，三级优先级） |
| `openai_codex_transform.go` | `ensureOpenAIResponsesImageGenerationToolChoiceAuto`、`applyCodexImageGenerationBridgeInstructions`、`codexImageGenerationBridgeMarker` 常量、`hasCodexImageGenerationFunctionTool`、`stripOpenAIImageGenerationTools`（+ `FromInput` / `FromRawPayload`）、`stripCodexSparkImageGenerationTools`、`inputContainsImageGenerationTool`、`toolsContainImageGeneration`、`isOpenAIImageGenerationToolMap`、`isImageGenNamespaceToolMap`、`stripOpenAIImageGenerationToolList` |
| `image_generation_intent.go` | `openAIRequestBodyImageGenerationToolNeedsNormalization`、`openAIRequestBodyHasImageGenerationDeclaration`、`openAIBodyMayDeclareImageGeneration`、`IsImageGenerationIntentForPlatformValidated` 等 8 个官方函数 |
| `Forward()` 注入块 | 官方 `openai_gateway_forward.go:373-470` 整段语义：权限门 → explicit strip 策略 → 注入 `image_generation` 工具 → `tool_choice=auto` → `normalizeOpenAIResponsesImageGenerationTools` → compact 豁免 |

### 2.3 fork 专有函数清单（`image_generation_intent.go`，33 个，待删）

`AttachCodexTurnMetadata`、`ClassifyCodexImageRequest`、`CodexImageResponseAdapterEnabled`、`ExtractUpstreamErrorMessageForBridge`、`HasCodexImageGenerationExtensionTool`、`IsCodexImageGenerationExtensionContinuation`、`IsCodexImageGenerationExtensionTurn`、`IsCodexSystemBackgroundTurn`、`IsImageExecution`、`IsImageGenerationPermissionIntent`、`IsImageGenerationPermissionIntentForPlatform`、`IsImageGenerationPermissionIntentMap`、`IsSuccessfulCodexImageGenerationExtensionContinuation`、`PrepareCodexImageGenerationExtensionDispatch`、`PrepareCodexImageRouteRequest`、`PrepareCodexSystemBackgroundTextDispatch`、`ResolveOpenAIResponsesImageBillingConfigDetailedForBridge`、`SetCodexImageResponseAdapterEnabled`、`ShouldBypassCodexSystemBackgroundImageMapping`、`ShouldUseCodexImageGenerationExtension`、`UsesOrchestratorGroup`、`classifyCodexImageGenerationExtensionContinuation`、`classifyCodexRequestRole`、`cloneRequestMapForImageIntent`、`codexAdditionalTools`、`codexImageToolChoiceSelected`、`codexJSONToolsContainImageGenerationExtension`、`codexRequestHasConversationImages`、`codexTurnMetadata`、`hasExplicitOpenAIImageGenerationIntent`、`isCodexImageGenerationFailureOutput`、`stripOpenAIImageGenerationToolDeclarations`、`stripOpenAIImageGenerationToolDeclarationsFromBody`

---

## 三、实施任务分解

> 铁律：每一步结束必须 `cd backend && go build ./...` 通过才进下一步；每步一个独立 commit，保证可单独 revert。

### T1 — 移植官方桥接基座（不改调用点）

1. 用官方版本重写 `codex_image_generation_bridge.go`，**保留** fork 的 `boolOverridePtr`、`platformBoolOverride`、`Channel.CodexImageGenerationBridgeOverride(platform)`。
2. 在 `openai_gateway_service.go` 添加 `isCodexImageGenerationBridgeEnabled`（照官方 `:629`），开关链：`account.CodexImageGenerationBridgeOverride()` → `channel.CodexImageGenerationBridgeOverride(platform)` → `s.cfg.Gateway.CodexImageGenerationBridgeEnabled`。
3. 校验：`grep -rn "CodexImageGenerationOrchestratorGroupID" backend/` 此时仍存在，编译必须通过。

**验收**：`go build ./...` 通过；`grep -c "func (a \*Account) CodexImageGenerationBridgeOverride" backend/internal/service/codex_image_generation_bridge.go` = 1。

### T2 — 移植官方 helper 函数族

1. 从 `openai_codex_transform.go`（上游 `:718-1150`）移植 §2.2 列出的 transform 函数与 `codexImageGenerationBridgeMarker` 常量。
2. 从 `image_generation_intent.go`（上游 `:311+`）移植 8 个官方 intent 函数。
3. 处理同名冲突：fork 已有 `ensureOpenAIResponsesImageGenerationTool`（`:815`）、`normalizeOpenAIResponsesImageGenerationTools`（`:697`）、`hasOpenAIImageGenerationTool`（`:642`）、`isCodexSparkModel`（`:638`）——**逐个比对正文，取官方实现**，不要直接覆盖整个文件。

**验收**：`go build ./...` 通过；此步结束前不删除任何调用点，故运行时行为不变。

### T3 — 切换调用点到官方语义

按文件顺序改，每改一处立即编译：

1. `openai_gateway_service.go`：`:2723`、`:3054`、`:3660` 的 `IsImageGenerationPermissionIntent*` → `IsImageGenerationIntent*`；`:3067` 与官方新版 `imageIntent` 计算对齐。
2. `openai_ws_forwarder.go:2872`、`openai_gateway_responses_chat_fallback.go:34` 同步替换。
3. `openai_gateway_handler.go`：`:314`、`:365`、`:1668` 替换为官方判定。
4. 在 `Forward()`（签名 `:2516`，body 改写区 `:2734-3086`）插入官方注入块（官方 `openai_gateway_forward.go:373-470` 语义）：
   - **注意**：fork 的 `Forward()` 没有官方的 `ensureReqBody()` / `markDecodedModified()` 闭包，只有 `bodyModified` 布尔 + `markPatchSet` / `markPatchDelete` / `disablePatch`。且 fork 在 `:2702` 一开始就 `getOpenAIRequestBodyMap(c, body)` 解出 `reqBody`（map），不是官方"按需解码"的形态。
   - 因此注入块要**改写成 map 直接操作**：`ensureOpenAIResponsesImageGenerationTool(reqBody)` 等函数返回 bool，直接 `if ... { bodyModified = true }`，末尾沿用既有 `:3086 if bodyModified { json.Marshal(reqBody) }` 回写。
   - 计算 `codexImageGenerationBridgeEnabled`（含 `isCodexCLI`、Responses Lite 头、`imageGenerationAllowed`、explicit policy ≠ strip）
   - explicit policy == strip → `stripOpenAIImageGenerationTools`
   - `imageIntent && !imageGenerationAllowed` → 403（官方 `:398-402`）
   - `imageGenerationAllowed && !isCompactRequest` → 注入工具 / `tool_choice=auto` / normalize
5. **验收**：`grep -rn "IsImageGenerationPermissionIntent\|ClassifyCodexImageRequest\|PrepareCodexImageRouteRequest" backend/internal/handler/openai_gateway_handler.go` 只剩 T4 要删的位置。

### T4 — 删除 fork 专有机制

1. 删 `codex_image_prompt_controls.go`（整文件）与 `openai_images.go:91` 调用。
2. 删 `CodexImageResponseAdapterEnabled` / `SetCodexImageResponseAdapterEnabled` 及 `openai_images.go:85/606/981`、`openai_images_responses.go:756` 的适配器分支。
3. 删 `image_generation_intent.go` 33 个专有函数（§2.3）。
4. 删 `openai_codex_transform.go` 4 个专有函数；确认 `image_generation_intent.go:101` 对 `NormalizeOpenAIWSImageGenerationChannelMapping` 的调用一并移除或改为官方路径。
5. 删 `codex_image_generation_bridge.go` 的 `CodexImageGenerationOrchestratorGroupID` 及 `encoding/json`/`strconv` import。
6. 清理 `channel_service.go` 的 `validateCodexImageGenerationBridge`、`IsCodexImageGenerationBridgeGroup`、`ResolveCodexImageGenerationRoute`、`ResolveCodexModelsManifestGroupID`（逐个确认是否仅桥接使用，非桥接用途的保留）。

**验收**：`go build ./...` 通过；`grep -rn "orchestrator_group_id" backend/` = 0。

### T5 — 清理测试与前端

1. 后端：删除/重写引用被删符号的测试。**已核实存在**的受影响文件（`backend/internal/service/`）：`image_generation_intent_test.go`、`openai_codex_transform_test.go`、`openai_image_generation_controls_test.go`。注意**没有** `codex_image_generation_bridge_test.go`。
2. 前端 `ChannelsView.vue`：按 §2.1 D 删除编排组字段；开关按官方 `{platform: bool}` 结构保存。
3. 前端 `EditAccountModal.vue:4402-4403`：改为官方三态（`codexImageToolMode`：enabled / strip / default），落 `extra.codex_image_generation_bridge` 与 `extra.codex_image_generation_explicit_tool_policy`。
4. `pnpm -C frontend run build` 必须成功（vite 直写 `backend/internal/web/dist`）。

**验收**：`go build ./...` + `go test ./internal/service/... -run 'Image|Codex'` + 前端 build 三者全绿。

### T6 — 存量配置排查（上线前置条件）

线上库 `channels.features_config` 里可能已有：
- `{"codex_image_generation_bridge": true}`（bool）→ 恢复官方语义后 = **注入 hosted 工具**，行为变化
- `{"codex_image_generation_bridge": {"openai": true, "orchestrator_group_id": N}}` → 官方 `platformBoolOverride` 读 `raw[platform]`，`true` 仍会生效；`orchestrator_group_id` 变成死配置

**动作**：上线前执行只读排查 SQL，把每个渠道的实际值列出来，逐条确认是否要改：

```sql
SELECT id, name, platform, features_config->'codex_image_generation_bridge' AS bridge_cfg
FROM channels
WHERE features_config ? 'codex_image_generation_bridge';
```

判定规则：**除明确要启用宿主注入的渠道外，一律置为 false 或删除该键**（恢复"纯透传"= 我们真机验证过的目标行为）。

---

## 四、验证方案（E2E，真机 Codex）

| 编号 | 场景 | 期望 |
|---|---|---|
| TC-E2E-01 | 纯文本组，codex 普通对话 | 正常回复，无注入（抓包确认 body 无 `image_generation` 工具，因 bridge=false） |
| TC-E2E-02 | 生图组（key 属图片组 + 模型映射），codex 说"画一个红色正方形" | codex 客户端本地发起图片请求，产出图片；服务端 usage_logs 记录图片计费 |
| TC-E2E-03 | 生图组，codex 纯聊天不发图 | 正常文本回复，不被 403 拦截 |
| TC-E2E-04 | 纯文本组（无图片权限），codex 强行画图 | 403 `permission_error`（`GroupAllowsImageGeneration` 门生效） |
| TC-E2E-05 | 渠道 `bridge=true` + account `bridge=true`，Lite 头请求 | 注入被 Lite 头抑制（官方 `!isOpenAIResponsesLiteHeader` 条件） |
| TC-E2E-06 | account `explicit_tool_policy=strip` | 客户端自带的 `image_generation` 工具被剥离，请求可正常转发 |
| TC-E2E-07 | `/responses/compact` 请求 | 不注入工具、不改 `tool_choice`（compact 豁免） |
| TC-E2E-08 | WS（`previous_response_id` 续写）生图 | 与 HTTP 路径行为一致 |

辅助验证：`backend/internal/service` 单测覆盖 T2 移植的每个 helper（正/反用例各 ≥1）。

---

## 五、风险与回滚

| 风险 | 等级 | 处置 |
|---|---|---|
| 存量 `features_config` 里 `bridge=true` 的渠道在升级后突然开始注入 hosted 工具，上游若为第三方中转会 400 | **高** | T6 上线前必做；发布说明标注行为变更 |
| 33 个函数删除牵连面广，遗漏调用导致编译失败 | 中 | T4 用 `go build` + `go vet` 逐个暴露，不靠肉眼 |
| fork 现有 `image_generation_intent.go` 有 13 个同名函数与官方实现不同（`IsImageGenerationIntent` 44L vs 7L、`GroupAllowsImageGeneration` 7L vs 24L 等） | **高** | 这些是**计费与权限**相关，必须逐个比对官方实现后决定取谁；不能整文件覆盖 |
| 前端一次性删编排组会让已保存的旧配置在 UI 上无法回显 | 低 | 旧值留在 DB，UI 不读即可；如需彻底清理另起迁移 |
| 生图计费口径变化（`resolveOpenAIResponsesImageBillingConfigDetailed` 同名但 39L vs 4L） | 中 | T3 前先比对这两份实现，确认计费字段不丢 |

**回滚**：每个 Tn 独立 commit，任意步骤出问题 `git revert` 该 commit 即可回到上一可用状态；分支基线与线上 `146e08aa8` 完全一致，回退成本为零。

---

## 六、执行顺序与门禁

```
T1 基座  →  T2 helper  →  T3 调用点切换  →  T4 删除  →  T5 测试+前端  →  T6 存量排查  →  E2E
   ↑go build    ↑go build      ↑go build        ↑go build      ↑build+test+build    ↑SQL只读
```

- **准入**：T1/T2 不删任何调用点，纯增量，可随时合并。
- **危险点**：T3→T4 之间是中间态（新旧两套判定并存），**不可发布**，必须连续完成。
- **发布门禁**：T5 全绿 + T6 结论明确 + TC-E2E-01~04 真机通过。

---

## 七、已知不确定项（实施时须现场核实，不得臆断）

1. `image_generation_intent.go` 13 个同名函数的差异性质：哪些是 fork 为配合 extension 机制改的（应回官方），哪些是 fork 为其他功能（如 Grok/WS）改的（应保留）。**逐个 diff 后落表**。
2. `resolveOpenAIResponsesImageBillingConfigDetailed` / `...FromBody` 两份实现差异（39L vs 4L）背后的计费字段差异是否与 v0.3.13 的图片计费工作相关——若相关则**保留 fork 版本**。
3. `openai_gateway_service.go` 巨型文件里 `Forward()` 的实际结构是否能容纳官方注入块（`ensureReqBody` / `markDecodedModified` 闭包是否存在）——T3 步骤 4 前必须确认这两个闭包可用，否则需先补。
4. WS 路径（`openai_ws_forwarder.go`）在官方是否也有等价注入块；若无，WS 路径保持"仅权限门"，与本方案正文一致即可。
