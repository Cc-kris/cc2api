# Sub2API 生图链路恢复测试用例

## 1. 文档信息

| 项目 | 内容 |
|---|---|
| 文档版本 | v1.0 |
| 日期 | 2026-06-20 |
| 测试范围 | OpenAI `/responses`、WS `/responses`、Images API、生图桥接去重 |
| 目标发布版本 | v0.2.48 |

## 2. 测试目标

1. WS 生图请求明确返回不支持。
2. HTTP 流式 `/responses` 生图不受影响。
3. Images API 生图不受影响。
4. 桥接重复请求仍被抑制。
5. 普通 WS 文本请求不受影响。

## 3. 测试用例

### TC-001 WS 生图请求返回不支持

| 项目 | 内容 |
|---|---|
| 前置条件 | WS `/responses` 请求首帧包含 `tools:[{type:"image_generation"}]`、`tool_choice:{type:"image_generation"}` 或 image-only model |
| 操作 | 建立 WS 并发送首帧 |
| 预期 | 收到 `response.failed`，错误码 `image_generation_ws_unsupported`，文案 `生图不支持ws的方式` |
| 预期 | 连接关闭 |
| 预期 | 不调用 Images API 桥接 |

### TC-002 WS 生图不产生 usage 图片记录

| 项目 | 内容 |
|---|---|
| 前置条件 | TC-001 已执行 |
| 操作 | 检查使用量记录 |
| 预期 | 不产生 `image_count > 0` 的记录 |

### TC-003 普通 WS 文本请求不受影响

| 项目 | 内容 |
|---|---|
| 前置条件 | WS 首帧不包含生图意图 |
| 操作 | 发送普通文本请求 |
| 预期 | 继续进入原有 WS passthrough 流程 |
| 预期 | 不返回 `生图不支持ws的方式` |

### TC-004 HTTP `/responses stream=true` 生图仍可处理

| 项目 | 内容 |
|---|---|
| 前置条件 | HTTP POST `/responses`，`stream=true`，包含生图意图 |
| 操作 | 发送请求 |
| 预期 | 不受 WS 拒绝逻辑影响 |
| 预期 | 继续走原有 Responses 或桥接处理 |

### TC-005 HTTP 桥接同 payload 重复请求不重复生成

| 项目 | 内容 |
|---|---|
| 前置条件 | 两个相同 HTTP `/responses` 生图请求并发进入 |
| 操作 | 并发执行 |
| 预期 | 只有一个 owner 调用上游 Images API |
| 预期 | 第二个等待并回放 |

### TC-006 HTTP 桥接成功后重复请求回放

| 项目 | 内容 |
|---|---|
| 前置条件 | 首次桥接已成功 |
| 操作 | TTL 内发送相同请求 |
| 预期 | 返回已保存结果，不再次调用上游 |

### TC-007 HTTP 桥接上游失败

| 项目 | 内容 |
|---|---|
| 前置条件 | 上游 Images API 返回 5xx 或网络错误 |
| 操作 | 发送生图请求 |
| 预期 | 返回现有错误形态 |
| 预期 | idempotency 标记为 retryable |

### TC-008 Images API 非流式生图不受影响

| 项目 | 内容 |
|---|---|
| 操作 | POST `/v1/images/generations`，`stream=false` |
| 预期 | 保持原有 JSON 图片响应 |

### TC-009 Images API 流式生图不受影响

| 项目 | 内容 |
|---|---|
| 操作 | POST `/v1/images/generations`，`stream=true` |
| 预期 | 保持原有 SSE 图片 chunk 响应 |

### TC-010 请求体不合法

| 项目 | 内容 |
|---|---|
| 操作 | WS 首帧发送非法 JSON |
| 预期 | 仍返回原有非法请求错误，不进入生图不支持分支 |

### TC-011 `previous_response_id` 的 WS 生图请求

| 项目 | 内容 |
|---|---|
| 操作 | WS 首帧包含 `previous_response_id` 与生图意图 |
| 预期 | 返回 `生图不支持ws的方式` |

### TC-012 WS 多步骤后续 turn 生图返回不支持

| 项目 | 内容 |
|---|---|
| 前置条件 | 第一轮 WS 普通文本请求已成功 |
| 操作 | 第二轮 WS turn 发送 `image_generation` 生图请求 |
| 预期 | 第二轮收到 `response.failed`，错误码 `image_generation_ws_unsupported`，文案 `生图不支持ws的方式` |
| 预期 | 第二轮不访问上游账号 |

### TC-013 多客户端并发 WS 生图

| 项目 | 内容 |
|---|---|
| 操作 | 多个 WS 生图请求并发进入 |
| 预期 | 全部快速返回不支持 |
| 预期 | 不产生上游 Images API 请求 |

## 4. 自动化测试命令

```bash
cd backend && go test ./internal/handler ./internal/service
```

## 5. 验收结论标准

1. 所有新增测试通过。
2. 相关历史测试通过。
3. Code review 无阻断问题。
4. GitHub release 创建成功。
5. 没有手动更新线上版本。
