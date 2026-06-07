# Sub2API 运维监控、缓存管理与后台管理增强技术实现方案

## 1. 文档信息

| 项目 | 内容 |
|---|---|
| 文档名称 | Sub2API 运维监控、缓存管理与后台管理增强技术实现方案 |
| 文档版本 | v2026.06.07-03 |
| 创建日期 | 2026-06-07 |
| 最后更新日期 | 2026-06-07 |
| 适用 PRD 版本 | `docs/sub2api-ops-cache-admin-vnext-prd-20260605.md` v2026.06.07-08 |
| 适用对象 | 后端工程师、前端工程师、测试工程师、运维人员、技术负责人 |
| 文档性质 | 技术实现方案 |

## 2. 实现目标

本方案用于落地 PRD 中 F01-F15 的研发实现，形成可拆分、可灰度、可回滚的后台能力升级。

| 目标 | 实现结果 |
|---|---|
| 运维告警降噪 | 告警从单一百分比触发改为 1 分钟窗口、最终失败数、失败率、样本量、影响范围的复合判断 |
| 快速定位问题 | 建立事故总览、统一错误列表、统一错误详情和统一错误分类字典 |
| AI 运维分析 | 支持 AI 配置、测试连接、自动分析、手动分析、报告落库、人工反馈 |
| 缓存产品化 | 缓存管理独立菜单，支持配置、统计、清理、原因分析、导出 |
| 多平台缓存 | OpenAI、Claude、Gemini 文本类 HTTP/SSE 精确响应缓存 |
| 高级缓存策略 | 容量、压缩、淘汰、热点、成本节省、上游 Prompt Cache 展示 |
| 语义相似缓存 | 默认关闭，观察、人工审核、灰度、正式启用、质量回滚全链路 |
| 后台管理增强 | 用户余额筛选、账号编辑移除上游预警金额、仪表盘入账与复购统计 |

## 3. 现有系统复用边界

| 能力 | 复用方式 | 不做事项 |
|---|---|---|
| 请求与错误日志 | 复用现有 `ops_error_logs`、请求详情、上游错误记录 | 不重做日志采集底座 |
| 告警规则 | 复用现有告警规则和告警事件能力，扩展字段和规则解释 | 不删除历史规则数据 |
| 缓存 V1 | 复用现有 OpenAI 本地响应缓存链路 | 不回退现有 OpenAI 缓存能力 |
| 设置配置 | 继续使用系统设置持久化能力 | 不引入独立配置中心 |
| 权限体系 | 复用后台角色鉴权，新增字段级能力判断 | 不新增面向普通用户的后台角色 |
| 账号调度 | 只记录、展示和归因，不修改主调度算法 | 不重做账号选择算法 |

## 4. 总体技术架构

### 4.1 后端分层

```text
Admin HTTP Handlers
├─ OpsIncidentHandler
├─ OpsAlertRuleHandler
├─ OpsUnifiedErrorHandler
├─ OpsAIAnalysisHandler
├─ CacheManagementHandler
├─ SemanticCacheHandler
├─ DashboardRevenueHandler
├─ AdminUserHandler
└─ AdminAccountHandler

Services
├─ OpsIncidentService
├─ OpsAlertRuleService
├─ OpsUnifiedErrorService
├─ OpsAIAnalysisService
├─ CacheConfigService
├─ LocalResponseCacheService
├─ CacheStatsService
├─ CacheClearService
├─ AdvancedCacheService
├─ SemanticCacheService
├─ DashboardRevenueService
├─ PermissionPolicyService
└─ RedactionService

Storage
├─ PostgreSQL
├─ Redis
└─ AI / Embedding Provider
```

### 4.2 主请求保护原则

| 子能力 | 主请求保护规则 |
|---|---|
| 精确缓存读取 | Redis 读取失败时跳过缓存，继续请求上游 |
| 精确缓存写入 | 写入失败只记录原因，不影响响应返回 |
| 缓存统计 | 异步写入，失败可丢弃或重试，不阻塞主请求 |
| AI 分析 | 与主请求完全解耦，只消费日志和聚合数据 |
| 语义向量生成 | 精确缓存写入成功后异步执行，失败不影响精确缓存 |
| 语义缓存查找 | 服务异常、超时、维度不一致时绕过语义缓存 |
| 高级缓存策略 | 异常时回退到精确缓存基础能力 |

## 5. 数据模型方案

### 5.1 告警规则扩展

扩展现有告警规则存储，兼容旧规则并支持新复合规则。

| 字段 | 类型 | 说明 |
|---|---|---|
| rule_version | string | 规则版本，旧规则为 v1，新规则为 v2 |
| error_categories | json | 统一错误分类字典多选 |
| trigger_level | string | P0/P1/P2/观察 |
| min_final_failures | int | 最小最终失败数 |
| min_failure_rate | decimal | 最小失败率，0 表示不启用失败率条件 |
| min_sample_count | int | 最小样本量 |
| impact_scope | json | 影响用户数、API Key 数、分组数、模型数、上游账号数条件 |
| recovered_fluctuation_policy | string | 只记录、参与观察、参与告警 |
| min_recovered_fluctuations | int | 最小已恢复波动数 |
| auto_ai_analysis | bool | 是否自动触发 AI 分析 |
| notification_channels | json | 站内、邮件、无 |
| silence_minutes | int | 静默分钟数 |
| migration_state | string | normal/migrated/readonly_legacy |

### 5.2 告警事件扩展

| 字段 | 类型 | 说明 |
|---|---|---|
| event_key | string | 告警去重 Key |
| lifecycle_status | string | 触发中、已确认、处理中、已恢复、已关闭、已静默 |
| merged_count | int | 静默期内合并次数 |
| last_seen_at | datetime | 最近命中时间 |
| recovered_at | datetime | 恢复时间 |
| acknowledged_at | datetime | 确认时间 |
| acknowledged_by | bigint | 确认人 |
| closed_at | datetime | 关闭时间 |
| closed_by | bigint | 关闭人 |
| trigger_snapshot | json | 触发时指标快照 |
| score_snapshot | json | 健康风险分数快照 |
| ai_task_id | bigint | 关联 AI 分析任务 |

### 5.3 AI 分析任务与报告

#### AI 分析任务

| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint | 任务 ID |
| source_type | string | alert_event/unified_errors/manual_filter |
| source_id | bigint | 来源对象 ID |
| trigger_type | string | auto/manual |
| trigger_user_id | bigint | 手动触发用户 |
| time_start | datetime | 分析开始时间 |
| time_end | datetime | 分析结束时间 |
| filters | json | 筛选条件 |
| status | string | pending/running/completed/failed/expired |
| sample_count | int | 样本数量 |
| provider | string | 接口类型 |
| model | string | AI 模型 |
| error_message | text | 失败原因 |
| started_at | datetime | 开始时间 |
| finished_at | datetime | 完成时间 |

#### AI 分析报告

| 字段 | 类型 | 说明 |
|---|---|---|
| task_id | bigint | 任务 ID |
| summary | text | 问题结论 |
| root_cause | text | 根因判断 |
| impact_scope | json | 影响范围 |
| evidence | json | 证据摘要 |
| suggested_actions | json | 建议动作 |
| error_breakdown | json | 错误分布 |
| confidence | string | high/medium/low |
| feedback_status | string | none/useful/not_useful/wrong_category |
| feedback_note | text | 反馈说明 |
| feedback_user_id | bigint | 最近一次反馈人用户 ID |
| feedback_at | datetime | 最近一次反馈时间 |

### 5.4 缓存统计分钟聚合

| 字段 | 类型 | 说明 |
|---|---|---|
| minute_at | datetime | 分钟时间桶 |
| platform | string | OpenAI/Claude/Gemini |
| model | string | 模型 |
| group_id | bigint | 分组 ID |
| api_key_id | bigint | API Key ID |
| cache_type | string | exact/semantic |
| total_requests | bigint | 全部请求数 |
| candidate_requests | bigint | 候选请求数 |
| hit_requests | bigint | 命中请求数 |
| bypass_requests | bigint | 绕过请求数 |
| store_success | bigint | 写入成功数 |
| store_skip | bigint | 写入跳过数 |
| input_tokens | bigint | 输入 tokens |
| output_tokens | bigint | 输出 tokens |
| hit_tokens | bigint | 命中 tokens |
| candidate_tokens | bigint | 候选请求 tokens |
| all_request_tokens | bigint | 全部请求 tokens |
| bypass_reasons | json | 绕过原因分布 |
| store_skip_reasons | json | 写入跳过原因分布 |
| estimated_saved_amount | decimal | 预估节省金额 |

### 5.5 缓存清理审计

| 字段 | 类型 | 说明 |
|---|---|---|
| operator_user_id | bigint | 操作人 |
| clear_type | string | 全部、平台、模型、分组、API Key、时间、过期 |
| scope | json | 清理范围 |
| matched_keys | bigint | 匹配 Key 数 |
| deleted_keys | bigint | 删除 Key 数 |
| status | string | success/failed/partial_success |
| error_message | text | 失败原因 |
| created_at | datetime | 操作时间 |

### 5.6 语义缓存条目与审计

#### 语义缓存条目

| 字段 | 类型 | 说明 |
|---|---|---|
| namespace | string | API Key、用户、分组、平台、模型、system 指纹、规则版本组成 |
| platform | string | 平台 |
| model | string | 模型 |
| api_key_id | bigint | API Key ID |
| user_id | bigint | 用户 ID |
| group_id | bigint | 分组 ID |
| system_fingerprint | string | system/instructions 指纹 |
| rule_version | string | 语义规则版本 |
| embedding_model | string | 向量模型 |
| embedding_dimension | int | 向量维度 |
| embedding_ref | string/json | 向量存储引用或向量值 |
| normalized_prompt_hash | string | 标准化语义内容哈希 |
| response_cache_key | string | 对应精确缓存 Key |
| status | string | active/expired/deleted/invalidated |
| expires_at | datetime | 过期时间 |

#### 语义缓存审计

| 字段 | 类型 | 说明 |
|---|---|---|
| request_id | string | 请求 ID |
| semantic_entry_id | bigint | 命中候选 |
| similarity | decimal | 相似度 |
| decision | string | observe/hit/miss/blocked/rollback |
| block_reason | string | 阻断原因 |
| review_status | string | 未审核/可复用/不可复用/争议 |
| feedback_type | string | wrong_hit/complaint/none |
| feedback_note | text | 反馈说明 |
| operator_user_id | bigint | 审核人 |

## 6. 配置方案

| 配置 Key | 说明 | 存储要求 |
|---|---|---|
| ops_ai_analysis_config | AI 分析服务地址、API Key、模型、接口类型、超时、样本数、自动触发、同类去重间隔、全局频率限制 | API Key 加密保存，读取脱敏 |
| cache_management_config | OpenAI/Claude/Gemini 开关、TTL、大小、temperature、模型白名单/黑名单 | 修改后新请求生效 |
| advanced_cache_config | 高级缓存总开关、灰度范围、Redis 容量、压缩、阈值、淘汰策略、热点窗口、成本展示 | `advanced_cache_enabled=false` 时只保存配置和展示，不介入线上请求；灰度范围为空 |
| semantic_cache_config | 语义服务地址、API Key、模型、维度、阈值、审核模式、灰度 Key、回滚阈值 | API Key 加密，默认关闭 |

## 7. 核心服务设计

### 7.1 OpsIncidentService

| 能力 | 实现 |
|---|---|
| 事故总览 | 查询最近窗口聚合数据，返回状态、摘要、影响范围、最新 AI 报告 |
| 健康风险分数 | 按 PRD 10.1.9 计算，最终失败数小于等于 2 时最低 70 |
| 分数原因 | 返回扣分项、样本量、失败数、影响范围 |
| 快捷筛选 | 生成可传给统一错误列表的筛选参数 |

### 7.2 OpsAlertRuleService

| 能力 | 实现 |
|---|---|
| 默认规则 | 初始化 PRD 10.1.3 的 P0/P1/P2 默认规则 |
| 自定义规则 | 支持字段级校验和联动校验 |
| 历史迁移 | 旧百分比规则迁移为复合规则，旧字段只读展示 |
| 事件合并 | 按 event_key 合并静默期内同类事件 |
| 生命周期 | 支持确认、处理、恢复、关闭、静默 |

### 7.3 OpsUnifiedErrorService

| 能力 | 实现 |
|---|---|
| 统一错误分类 | 共用 PRD 18.1.1 错误分类字典 |
| 列表筛选 | 时间、错误结果、错误大类、状态码、用户、API Key、分组、模型、上游账号、关键词、AI 状态 |
| 详情聚合 | 请求链路、错误详情、原始记录、同类问题、AI 报告 |
| 导出 | 按当前筛选导出，执行字段级权限和脱敏 |

### 7.4 OpsAIAnalysisService

| 能力 | 实现 |
|---|---|
| 配置管理 | 支持 OpenAI 兼容、Responses、Anthropic 兼容、Gemini 兼容 |
| 测试连接 | 验证服务地址、API Key、模型可用性 |
| 自动分析 | P0/P1 默认触发，遵守同类去重间隔和全局频率限制 |
| 手动分析 | 对当前筛选时间段和错误集合创建任务 |
| Worker | 后台轮询 pending 任务，使用 `FOR UPDATE SKIP LOCKED` 原子领取并置为 running；成功置 completed，失败置 failed，服务停止不误标失败 |
| 脱敏 | AI 输入只包含脱敏摘要和必要上下文；样本仅包含错误分类、状态、平台、模型、实体 ID、脱敏摘要和同类数量，不传完整邮箱、API Key、Token、代理地址、请求正文或响应正文 |
| 反馈 | 保存有用、无用、错误归因、补充说明 |

### 7.5 LocalResponseCacheService

| 能力 | 实现 |
|---|---|
| 精确缓存 Key | `cache:v2:{platform}:{api_key_id}:{group_id}:{endpoint}:{model}:{request_hash}` |
| 请求归一化 | 平台、接口、模型、system、messages/contents、tools、response_format、temperature、stream 参与 hash |
| OpenAI | 覆盖 Responses、Chat Completions 文本类请求 |
| Claude | 覆盖 Messages 文本类 HTTP/SSE 请求 |
| Gemini | 覆盖 generateContent、streamGenerateContent 文本类请求 |
| 完整流判断 | 流式未完整结束不写入缓存 |
| 统计 | 候选、命中、绕过、写入、tokens、原因分布异步记录 |

### 7.6 AdvancedCacheService

| 能力 | 实现 |
|---|---|
| 容量控制 | Redis 容量上限、安全上限校验、超过后按淘汰策略处理 |
| 压缩存储 | 响应体超过 64KB 且压缩开关开启时压缩保存 |
| 淘汰策略 | LRU/LFU/W-TinyLFU |
| 热点识别 | 15 分钟、1 小时、6 小时、24 小时窗口统计热点 |
| 成本展示 | 根据模型价格展示本地响应缓存节省、上游 Prompt Cache 节省和总预估节省，价格缺失时不展示错误金额 |
| Prompt Cache | 与本地响应缓存分开展示上游 cache read/write tokens |
| 高级统计 | 输出容量使用率、压缩前后大小、压缩节省率、淘汰次数、热点 Top N、回退状态 |

### 7.7 SemanticCacheService

| 能力 | 实现 |
|---|---|
| 启用阶段 | 默认关闭，观察、人工审核、灰度、正式启用 |
| 向量生成 | 精确缓存写入成功后异步生成 |
| 隔离范围 | API Key、用户、分组、平台、模型、system 指纹、语义规则版本 |
| 查找流程 | 精确未命中后执行语义查找，达到阈值后根据阶段决定是否返回 |
| 审计 | 记录候选、相似度、阻断原因、命中结果、审核状态 |
| 反馈 | 支持误命中、投诉、人工标记 |
| 自动回滚 | 近 24 小时投诉率或错误反馈率达到阈值即关闭 |

### 7.8 DashboardRevenueService

| 能力 | 实现 |
|---|---|
| 累计入账金额 | 统计非超管用户有效余额增加金额 |
| 已使用金额 | 优先消费流水；历史不足时估算并标记 |
| 未使用金额 | 非超管用户当前有效未使用余额 |
| 复购分布 | 零购、一购、二购、三购、三购以上 |
| 排除规则 | 排除超管自身账号金额；超管给客户加款计入客户有效入账 |

## 8. 后台权限与脱敏

### 8.1 权限策略

| 页面/能力 | 平台所有者 | 运维 | 运营 | 客服 |
|---|---|---|---|---|
| 告警规则编辑 | 可编辑 | 可编辑 | 不可见 | 不可见 |
| 事故总览 | 全量 | 全量 | 脱敏业务影响 | 脱敏摘要 |
| 错误列表/详情 | 全量 | 全量 | 脱敏 | 脱敏 |
| AI 配置 | 可编辑 | 只读脱敏 | 不可见 | 不可见 |
| AI 报告 | 全量 | 全量 | 脱敏 | 脱敏 |
| 缓存配置/清理 | 可编辑 | 只读审计 | 不可见 | 不可见 |
| 缓存统计 | 可导出 | 可见不可导出 | 可导出 | 不可见 |
| 充值与入账概览 | 汇总 | 不可见 | 汇总 | 不可见 |
| 语义缓存 | 可编辑配置、审核、反馈、关闭 | 可查看审计、执行审核和反馈；不可编辑配置、不可关闭总开关 | 不可见 | 不可见 |

### 8.2 脱敏服务

统一提供：

```text
RedactEmail
RedactAPIKey
RedactToken
RedactProxyURL
RedactRequestBody
RedactResponseBody
RedactAIContext
RedactUpstreamAccountName
```

脱敏必须覆盖页面、详情、导出、复制、AI 上下文。

## 9. 迁移与兼容

| 迁移项 | 规则 |
|---|---|
| 旧告警规则 | 迁移为复合规则，旧单一百分比规则不单独触发强告警 |
| 旧错误入口 | 保留跳转到统一错误中心并带入筛选 |
| 缓存通用设置 | 迁移到缓存管理菜单，原配置值保留 |
| OpenAI 缓存 V1 | 保留兼容读取周期，新写入使用 v2 Key |
| 账号上游预警字段 | 历史字段保留，页面不展示，提交忽略，不再触发邮件 |

## 10. 上线与回滚

| 阶段 | 上线内容 | 回滚方式 |
|---|---|---|
| 阶段 1 | 运维告警、事故总览、错误中心、AI 基础、用户/账号/仪表盘基础 | 关闭新告警规则，旧入口保留 |
| 阶段 2 | OpenAI 缓存产品化 | 关闭缓存总开关或 OpenAI 平台开关 |
| 阶段 3 | Claude/Gemini 精确缓存 | 关闭对应平台开关 |
| 阶段 4 | 高级缓存策略 | 关闭高级策略，回退精确缓存 |
| 阶段 5 | 语义相似缓存 | 关闭语义缓存开关，保留精确缓存 |

## 11. 验证要求

| 类型 | 必须验证 |
|---|---|
| 后端单元测试 | 告警规则、健康分数、错误分类、缓存 Key、缓存统计、语义回滚 |
| 后端接口测试 | API 参数校验、权限、脱敏、错误提示 |
| 前端测试 | 表单默认值、联动校验、按钮状态、空态、失败态 |
| 集成测试 | Redis 不可用、AI 超时、语义服务失败、旧入口跳转 |
| 数据验证 | 缓存命中率、tokens 命中率、累计入账、复购分布 |

