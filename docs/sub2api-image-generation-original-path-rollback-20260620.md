# sub2api 生图链路回退到原装行为方案

## 文档信息

- 版本：v0.2.49
- 日期：2026-06-20
- 范围：OpenAI `/responses` 生图、Codex 流式生图、WebSocket 生图、直接 `/images/*` 生图
- 结论：移除 `/responses` 到 `/images/generations` 的自动桥接和兜底桥接，恢复桥接前的原装上游 `/responses` 行为；WebSocket 生图继续明确拒绝；直接 Images API 保持可用。

## 一、问题结论

线上重复生图不是 WebSocket 路径触发，而是 HTTP `/responses` 路径触发。重复请求进入系统后，被本系统的 Codex 生图桥接逻辑自动转成 `/images/generations` 请求。多轮上下文会导致请求体持续变化，因此基于完整请求体的幂等保护无法阻止同一用户意图被多次转生图。

本次修复不继续加幂等补丁，不继续兼容桥接行为，而是删除桥接运行路径，恢复桥接前逻辑：客户端请求 `/responses` 就转发 `/responses`，系统不再替客户端自动改造成 Images API。

## 二、整改记录定位

| 节点 | 版本 | 行为 |
|---|---:|---|
| 桥接前 | v0.2.37 | `/responses` 生图按原请求转发上游，不转 `/images/generations` |
| 首次加入生图兜底 | v0.2.38 | 开始增加 `/responses` 生图失败后转 Images API 的兜底 |
| 直接桥接 | v0.2.39 | 开始将 Codex `/responses` 生图直接转 `/images/generations` |
| 多次幂等修复 | v0.2.40 - v0.2.47 | 围绕桥接后的重复、重放、多轮编辑继续修补 |
| WS 拒绝 | v0.2.48 | WebSocket 生图返回“不支持 ws 的方式” |
| 本次回退 | v0.2.49 | 删除 HTTP/WS 生图桥接运行代码，恢复 `/responses` 原装转发 |

## 三、线上验证依据

1. 线上日志显示 v0.2.48 的 WebSocket 生图拒绝逻辑已经生效，重复生图不是 WS 桥接造成。
2. 重复生图记录来自 HTTP `/responses`，上游路径为 `/v1/responses` 入口，随后系统日志出现“Responses image bridge direct to /images/generations”。
3. 账号 `0.08/张-生图专用-大西瓜` 在桥接前已经存在成功生图记录，说明不依赖本系统桥接也能通过原装 `/responses` 链路完成生图。
4. 桥接前节点为 v0.2.37；从 v0.2.38 开始出现生图兜底，从 v0.2.39 开始出现直接桥接。

## 四、目标行为

| 场景 | v0.2.49 行为 | 计费与记录 |
|---|---|---|
| HTTP `/responses` 普通文本 | 原样转发 `/v1/responses` | 按原响应记录 |
| HTTP `/responses` 显式 image_generation 工具 | 原样转发 `/v1/responses`，只做字段规范化，不自动转 Images API | 上游返回图片时按响应统计 |
| HTTP `/responses` 模型为 `gpt-image-*` | 原样转发 `/v1/responses`，不自动转 Images API | 上游返回图片时按响应统计 |
| Codex 流式 `/responses` 生图 | 原样流式转发上游 `/v1/responses` | 不生成第二条 Images API 请求 |
| WebSocket 生图 | 返回“不支持 ws 的方式” | 不触发上游生图 |
| 直接 `/images/generations` | 保持原直接 Images API 能力 | 按 Images API 记录 |
| 直接 `/images/edits` | 保持原直接 Images API 能力 | 按 Images API 记录 |
| 生图权限关闭 | 继续拒绝 image_generation 意图 | 不触发上游生图 |

## 五、具体改造

### 1. 删除 HTTP `/responses` 自动桥接

删除以下运行行为：

- 自动向 Codex 文本请求注入 `image_generation` 工具。
- 自动追加让模型使用 `image_generation` 的桥接说明。
- 将 `/responses` 生图请求直接转成 `/images/generations`。
- `/responses` 上游失败后兜底转 `/images/generations`。
- 桥接专用幂等、重放、冲突响应。

保留以下行为：

- OpenAI Responses 请求体中 `image_generation` 工具字段的官方字段名规范化。
- 生图权限检查。
- 原始上游 `/responses` 响应里的图片数量和尺寸统计。

### 2. 删除 WebSocket 生图桥接直连

删除 WebSocket 收到生图请求后转 HTTP Images API 的直连桥接函数。WebSocket 生图入口保留 v0.2.48 的拒绝策略，向客户端返回明确错误，不再发起上游生图。

### 3. 删除旧桥接测试和补充回归测试

删除直接验证桥接成功的旧测试，新增或调整回归测试，确保：

- 打开桥接配置也不会把 `/responses` 改成 `/images/generations`。
- API Key passthrough 的 `gpt-image-2` 请求仍发送到 `/v1/responses`。
- WebSocket 生图不进入桥接。
- 直接 Images API 测试继续通过。

## 六、与 2026-06-19 桥接方案的差异

| 项目 | 2026-06-19 桥接方案 | v0.2.49 回退方案 |
|---|---|---|
| 主思路 | 兼容 Codex 生图，把 `/responses` 转 Images API | 恢复原装 `/responses`，不再转 Images API |
| 重复处理 | 通过桥接幂等、重放、冲突保护 | 删除重复来源，不产生桥接副请求 |
| 多轮上下文 | 需要解析历史上下文和上次图片 | 不解析，不改写，交给上游处理 |
| WS 生图 | 曾尝试桥接到 HTTP Images | 明确不支持 WS 生图 |
| 风险 | 请求体变化时仍可能误判重复或重复生成 | 系统不再创造额外生图请求 |
| 保留能力 | 直接 Images API 可用 | 直接 Images API 可用 |

## 七、测试用例

### 正向用例

1. HTTP `/responses` 普通文本请求仍正常返回。
2. HTTP `/responses` 携带 `image_generation` 工具时仍发送到 `/v1/responses`。
3. HTTP `/responses` 使用 `gpt-image-2` 时仍发送到 `/v1/responses`。
4. 直接 `/images/generations` 成功。
5. 直接 `/images/edits` 成功。
6. OAuth Images API 流式和非流式成功。

### 异常用例

1. 生图权限关闭时，请求被拒绝且不触发上游。
2. WebSocket 生图返回“不支持 ws 的方式”，不触发上游。
3. 上游 Images API 返回错误时，错误透传和计费不误记。
4. 流式图片响应中断时，保留原有异常处理。
5. 客户端断开后，直接 Images API 仍按原有逻辑排空上游以完成必要记录。

### 多步骤用例

1. 多轮 `/responses` 请求中包含历史图片结果时，不被本系统解析为 Images API 编辑请求。
2. 连续两次 Codex 流式 `/responses` 生图请求，只产生两次原装 `/responses` 上游请求，不产生额外 `/images/generations` 请求。
3. 同一请求重试时，不再返回桥接专用 `image_generation_idempotency_conflict`。

## 八、验收标准

1. 代码检索不到旧桥接运行入口：`ForwardResponsesImageBridgeToImages`、`tryFallbackResponsesImageGenerationToImagesAPI`、`Responses image bridge direct`、`image_generation_idempotency_conflict`。
2. `go test -count=1 ./internal/handler ./internal/service` 通过。
3. `go test -count=1 ./...` 通过或仅存在与本次无关的既有测试环境问题；若失败必须记录失败原因。
4. GitHub Release v0.2.49 创建成功，发布后停止，不手动更新线上版本。
