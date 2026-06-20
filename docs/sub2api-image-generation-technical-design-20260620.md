# Sub2API 生图链路恢复技术实施方案

## 1. 文档信息

| 项目 | 内容 |
|---|---|
| 文档版本 | v1.0 |
| 日期 | 2026-06-20 |
| 代码分支 | main |
| 目标版本 | v0.2.48 |
| 发布边界 | 只提交 GitHub 并生成 release，不手动更新线上版本 |

## 2. 现状摘要

当前代码在 WS `/responses` 首帧识别到 `image_generation` 意图时，会进入 `tryDirectOpenAIWebSocketImageBridgeToHTTPImages`，直接转 Images API。该策略从 v0.2.39 引入，导致 WS 与 HTTP 流式桥接边界混淆。

历史数据证明：

1. 大西瓜账号成功生图均为 HTTP `/responses` 流式，`openai_ws_mode=false`。
2. 小慕账号支持 Images API 流式与非流式，不支持 `gpt-5.5 + image_generation tool` 的 Responses 流式。
3. WS 生图兼容复杂度高，当前需求要求 WS 生图直接返回不支持。

## 3. 设计目标

1. WS 生图不进入 Images API 桥接。
2. WS 生图返回明确错误：`生图不支持ws的方式`。
3. HTTP `/responses` 生图保留桥接能力。
4. Images API 直接生图不受影响。
5. 现有重复生图保护继续有效。
6. 测试覆盖主路径、异常路径、重复路径。

## 4. 方案对比

| 方案 | 内容 | 结论 |
|---|---|---|
| 继续兼容 WS 桥接 | WS 生图失败后转 Images API | 放弃，复杂且产生重复风险 |
| 完全删除桥接 | 所有 Responses 生图都走原生 | 放弃，会破坏小慕等 Images-only 账号 |
| HTTP 桥接保留，WS 生图拒绝 | HTTP 保留主流能力，WS 直接提示不支持 | 采用 |

## 5. 推荐方案

### 5.1 WS 生图拒绝

修改 `backend/internal/handler/openai_gateway_handler.go` 的 `ResponsesWebSocket`：

1. 首帧读取与基础校验后，判断是否为生图意图。
2. 后续 turn 进入上游前，再次判断是否为生图意图。
3. 如果是生图意图，调用新增的 WS 生图不支持响应函数。
4. 不再调用 `tryDirectOpenAIWebSocketImageBridgeToHTTPImages`。
5. 保留 `tryDirectOpenAIWebSocketImageBridgeToHTTPImages` 函数不再引用，避免扩大删除范围。

### 5.2 WS 错误响应形态

实现一个小函数：

```go
writeOpenAIWSImageGenerationUnsupported(ctx, conn, model)
```

输出事件优先采用 JSON 文本帧：

```json
{
  "type": "response.failed",
  "response": {
    "status": "failed",
    "model": "...",
    "error": {
      "code": "image_generation_ws_unsupported",
      "message": "生图不支持ws的方式"
    }
  }
}
```

随后以 policy violation 或 normal closure 关闭，关闭 reason 使用同一文案。

### 5.3 HTTP `/responses` 保持现状

不移除 `ForwardResponsesImageBridgeToImages`、`acquireResponsesImageBridgeIdempotency`、`waitAndReplayResponsesImageBridgeDuplicate`。

HTTP 请求仍可在 service 层按现有逻辑进入桥接。

### 5.4 重复保护保留

保留当前 payload 级去重：

1. 同一 image body 使用同一 key。
2. in-flight 请求等待并回放。
3. 成功后回放。
4. 上游失败标记 retryable。

本次不新增意图级去重，避免扩大变更面。原因：用户已明确 WS 不需要兼容，重复问题的主要触发链路来自 WS/桥接混用；先关闭 WS 桥接入口。

## 6. 关键修改文件与职责

| 文件 | 修改 |
|---|---|
| `backend/internal/handler/openai_gateway_handler.go` | WS 首帧或后续 turn 生图意图直接返回不支持，不进入桥接 |
| `backend/internal/handler/openai_gateway_handler_test.go` | 增加 WS 生图拒绝测试 |
| `backend/internal/service/openai_images_test.go` 或现有相关测试 | 保留/补充 HTTP 桥接重复保护测试 |
| `docs/sub2api-image-generation-prd-20260620.md` | PRD |
| `docs/sub2api-image-generation-technical-design-20260620.md` | 技术方案 |
| `docs/sub2api-image-generation-test-cases-20260620.md` | 测试用例 |

## 7. 实施顺序

1. 新增 WS 生图不支持响应函数。
2. 修改 `ResponsesWebSocket` 中生图意图分支。
3. 增加单元测试：WS 生图不触发桥接，返回不支持。
4. 跑相关后端测试。
5. Code review。
6. 全量关键测试。
7. 提交 GitHub。
8. 创建 release。

## 8. 风险与回滚点

| 风险 | 控制 |
|---|---|
| 影响普通 WS 文本请求 | 分支只在 image_generation 意图下触发 |
| 客户端无法读取错误事件 | 同时发送 `response.failed` 与 close reason |
| HTTP 生图被误伤 | 不修改 HTTP `Responses` handler 与 service 桥接逻辑 |
| 旧函数残留 | 保留未引用函数，避免大范围删除造成回归 |

回滚点：恢复 `ResponsesWebSocket` 中原有 `tryDirectOpenAIWebSocketImageBridgeToHTTPImages` 调用即可。

## 9. 验证方案

1. `cd backend && go test ./internal/handler ./internal/service`。
2. 指定测试：WS 生图拒绝。
3. 指定测试：HTTP Responses 图片桥接重复保护。
4. 静态检查：`git diff`，确认没有线上配置变更。
5. Code review 检查安全、边界、回归风险。

## 10. 发布方案

1. 提交到 GitHub。
2. 创建 tag 与 GitHub release。
3. 不 SSH 部署。
4. 不重启线上服务。
5. 不修改线上数据库。
