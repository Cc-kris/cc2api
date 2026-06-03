# Sub2API 本地精确 Response Cache 技术实现方案

版本：2026-06-03-01
适用仓库：Cc-kris/sub2apis

## 1. 现状

当前项目已有 Redis repository 层，包含 gateway sticky session、API Key auth cache、billing cache 等实现；OpenAI 网关由 `OpenAIGatewayHandler` 负责读取请求、鉴权、并发控制、账号选择、调用 `OpenAIGatewayService` 转发并记录用量。

第一版缓存不进入底层上游转发 service，而是在 handler 层接入，并通过 ResponseWriter 捕获非流式 JSON 与流式 SSE 原文，原因：

1. handler 层能拿到 API Key、用户、分组、原始请求体、接口类型和 stream 标记。
2. handler 层可以在账号选择前命中缓存，真正避免占用上游账号和并发槽位。
3. 对现有 `Forward` / `ForwardAsChatCompletions` / `ForwardAsAnthropic` 改动最小。

## 2. 新增模块

### 2.1 backend/internal/service/local_response_cache.go

职责：

- 定义缓存配置 `LocalResponseCacheConfig`。
- 定义缓存条目 `LocalResponseCacheEntry`。
- 构建稳定缓存 key。
- 判断请求是否可缓存。
- 标准化 JSON 请求体。
- 提供命中/写入所需的服务方法。

### 2.2 backend/internal/repository/gateway_cache.go

职责：

- 复用现有 Gateway Cache Redis 实现存储缓存条目。
- key 前缀：`local_response_cache:v1:`。
- value 使用 JSON，后续可扩展 gzip。
- Redis 错误上抛给 service，handler 层只记录并降级。

### 2.3 backend/internal/handler/openai_response_cache.go

职责：

- 在 OpenAI 网关 handler 中封装缓存读写逻辑。
- 命中后写回响应 body 和 header；流式命中时回放已缓存 SSE body。
- 未命中时包装 `gin.ResponseWriter`，捕获非流式 JSON 或流式 SSE 响应体，成功结束后写入 Redis。
- 设置 `X-Sub2API-Cache` header。

## 3. 接入点

### 3.1 `/v1/responses`

接入在：

- 请求 JSON 校验后
- billing eligibility 通过后
- 用户并发槽位获取前或后

第一版接在 billing eligibility 后、账号选择前。命中缓存时不占账号槽位，不请求上游。

### 3.2 `/v1/chat/completions`

接入同上，处理 OpenAI 平台 handler `backend/internal/handler/openai_chat_completions.go` 中的非流式 JSON 与流式 SSE 请求。

### 3.3 `/v1/messages`

接入 OpenAI 平台 messages handler，处理 `stream=false` 与 `stream=true`。

## 4. 缓存 key

格式：

```text
sha256(api_key_id + group_id + endpoint + platform + model + normalized_body)
```

Redis key：

```text
local_response_cache:v1:{hash}
```

不把明文 prompt 放入 key。

## 5. 跳过规则

`ShouldBypassLocalResponseCache` 返回跳过原因：

- disabled
- no_api_key
- no_group
- request_too_large
- invalid_json
- temperature_too_high
- tools_or_functions
- sensitive_content
- explicit_bypass

## 6. 响应捕获

使用 `gin.ResponseWriter` 包装器捕获：

- status code
- response body
- content type

只缓存：

- HTTP 200
- JSON 或 text/event-stream
- body 长度大于 0 且不超过配置上限
- 流式响应必须完整结束，不能缓存中断响应

## 7. 降级策略

Redis Get/Set 出错：

- 不返回错误给用户
- 日志记录 warn/debug
- 继续原始上游链路

## 8. 配置策略

第一版先用安全默认配置：

- 默认关闭
- 可通过系统设置 → 通用设置启用
- 默认 TTL 10 分钟
- 最大响应 512KB
- 最大请求 256KB
- temperature 阈值 0.3

后续再接入完整后台 UI。

## 9. 测试

单元测试：

1. 同一 API Key + Group + 请求体生成相同 key。
2. 不同 Group 生成不同 key。
3. stream=true 首次捕获完整 SSE，后续命中直接回放 SSE 原文。
4. tools/function_call 跳过。
5. temperature 超阈值跳过。
6. 敏感字段跳过。
7. Redis repository set/get 正常。
8. handler 命中时返回缓存 body 和 `X-Sub2API-Cache: hit`。
9. 流式请求首次捕获 SSE，二次命中回放 SSE。
10. 系统设置通用设置可以打开/关闭缓存。

回归测试：

- `go test ./internal/service -run LocalResponseCache`
- `go test ./internal/repository -run LocalResponseCache`
- `go test ./internal/handler -run LocalResponseCache`
- `go test ./... -count=1` 视耗时执行。

## 10. 系统设置 UI 接入

后端在 `SystemSettings` / admin setting DTO 中新增 `local_response_cache_enabled`。前端系统设置通用设置增加一个开关，保存时随现有通用设置提交。默认值为 false。

## 11. 流式实现细节

流式缓存不解析事件内容，不重组 chunk，只缓存已经成功写给客户端的 SSE 原文。ResponseWriter 包装器需要同时实现 `http.Flusher`，确保原始流式刷新行为不被破坏。写入缓存必须在 handler 完成上游转发且未出现错误后进行；如果中途 failover、panic、客户端断开、下游写入错误、未看到终止事件或响应超过大小限制，直接丢弃捕获内容。终止事件包含 `[DONE]`、`response.completed` / `response.done`、`message_stop` 等已完成标记。
