# Sub2API 运维监控、缓存管理与后台管理增强 API 方案

## 1. 文档信息

| 项目 | 内容 |
|---|---|
| 文档名称 | Sub2API 运维监控、缓存管理与后台管理增强 API 方案 |
| 文档版本 | v2026.06.07-03 |
| 创建日期 | 2026-06-07 |
| 最后更新日期 | 2026-06-07 |
| 适用 PRD 版本 | `docs/sub2api-ops-cache-admin-vnext-prd-20260605.md` v2026.06.07-08 |
| 适用对象 | 后端工程师、前端工程师、测试工程师、技术负责人 |
| 文档性质 | API 设计方案 |

## 2. 通用约定

### 2.1 鉴权与权限

所有接口走后台管理员鉴权。

```http
Authorization: Bearer <admin_token>
```

接口必须执行角色权限判断。无权限时返回：

```json
{
  "error": "当前账号无权限执行此操作"
}
```

### 2.2 通用响应

成功：

```json
{
  "data": {},
  "message": "success"
}
```

分页：

```json
{
  "data": {
    "items": [],
    "total": 0,
    "page": 1,
    "page_size": 20
  },
  "message": "success"
}
```

失败：

```json
{
  "error": "错误提示"
}
```

### 2.3 通用枚举

#### 错误大类

```text
client, platform, upstream, account_pool, rate_limit, permission, balance, config, slow_request, unknown
```

#### 错误结果

```text
final_failed, recovered, client_aborted, unknown
```

#### 错误子类

错误子类字段统一使用 `error_subcategory`，用于描述大类下的稳定细分原因，例如 `upstream_rate_limit`、`upstream_timeout`、`client_auth_error`。

#### 客户端错误细分

客户端错误细分字段统一使用 `client_error_subcategory`，仅当 `error_category=client` 时有值。

```text
client_auth_error, client_rate_limit_error, client_balance_error, client_parameter_error, client_model_error, client_path_error, client_context_error, client_disconnect_error, client_insufficient_evidence
```

#### 告警等级

```text
P0, P1, P2, observe
```

#### 告警事件状态

```text
firing, acknowledged, processing, recovered, closed, silenced
```

## 3. 运维事故总览 API

### 3.1 获取事故总览

```http
GET /api/v1/admin/ops/incidents/overview
```

查询参数：

| 参数 | 类型 | 必填 | 默认值 | 规则 |
|---|---|---|---|---|
| time_range | string | 否 | 1m | 1m/5m/30m/1h/6h/24h/custom |
| start_time | string | 否 | 空 | custom 时必填 |
| end_time | string | 否 | 空 | custom 时必填，必须晚于 start_time |
| platform | string | 否 | 全部 | openai/claude/gemini |
| model | string | 否 | 全部 | 模型名 |
| group_id | number | 否 | 全部 | 分组 ID |

响应：

```json
{
  "data": {
    "status": "normal",
    "health_risk_score": 92,
    "score_level": "normal",
    "score_reasons": ["最近 1 分钟最终失败数为 0"],
    "summary": "当前系统运行正常",
    "final_failures": 0,
    "final_failure_rate": 0.0,
    "recovered_fluctuations": 1,
    "total_requests": 1200,
    "affected_users": 0,
    "affected_api_keys": 0,
    "affected_models": [],
    "affected_accounts": [],
    "latest_ai_analysis": null,
    "quick_filters": [],
    "updated_at": "2026-06-07T12:00:00+08:00"
  }
}
```

## 4. 告警规则 API

### 4.1 获取告警规则列表

```http
GET /api/v1/admin/ops/alert-rules
```

查询参数：

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| page | number | 否 | 默认 1 |
| page_size | number | 否 | 默认 20，最大 100 |
| enabled | boolean | 否 | 启用状态 |
| severity | string | 否 | P0/P1/P2/observe |
| keyword | string | 否 | 规则名称关键词 |

### 4.2 创建告警规则

```http
POST /api/v1/admin/ops/alert-rules
```

请求：

```json
{
  "name": "上游账号集中失败",
  "enabled": true,
  "time_window": "1m",
  "error_categories": ["upstream", "permission"],
  "trigger_level": "P1",
  "min_final_failures": 5,
  "min_failure_rate": 10.0,
  "min_sample_count": 50,
  "impact_scope": {
    "affected_users": 2,
    "affected_api_keys": 2,
    "affected_models": 1,
    "affected_upstream_accounts": 1
  },
  "recovered_fluctuation_policy": "observe_only",
  "min_recovered_fluctuations": 10,
  "auto_ai_analysis": true,
  "notification_channels": ["in_app", "email"],
  "silence_minutes": 10,
  "description": "上游账号集中失败规则"
}
```

校验失败示例：

```json
{
  "error": "最小最终失败数不能大于最小样本量"
}
```

### 4.3 更新告警规则

```http
PUT /api/v1/admin/ops/alert-rules/{id}
```

请求同创建接口。

### 4.4 删除告警规则

```http
DELETE /api/v1/admin/ops/alert-rules/{id}
```

### 4.5 获取告警事件列表

```http
GET /api/v1/admin/ops/alert-events
```

查询参数：`status`、`severity`、`time_range`、`platform`、`model`、`group_id`、`page`、`page_size`。

### 4.6 更新告警事件状态

```http
PUT /api/v1/admin/ops/alert-events/{id}/status
```

请求：

```json
{
  "status": "acknowledged",
  "note": "已确认，处理中"
}
```

## 5. 统一错误中心 API

### 5.1 获取统一错误列表

```http
GET /api/v1/admin/ops/unified-errors
```

查询参数：

| 参数 | 类型 | 必填 | 默认值 | 规则 |
|---|---|---|---|---|
| page | number | 否 | 1 | >= 1 |
| page_size | number | 否 | 20 | 20/50/100 |
| start_time | string | 否 | 最近 30 分钟 | 最大跨度 7 天 |
| end_time | string | 否 | 当前时间 | 不得早于 start_time |
| error_categories | string | 否 | 全部 | 逗号分隔错误大类 |
| error_subcategories | string | 否 | 全部 | 逗号分隔错误子类 |
| client_error_subcategories | string | 否 | 全部 | 逗号分隔客户端错误细分，仅对 client 分类生效 |
| error_results | string | 否 | 全部 | final_failed/recovered/client_aborted/unknown |
| severity | string | 否 | 全部 | P0/P1/P2/observe/normal |
| status_code | string | 否 | 全部 | 支持单值、逗号、区间 |
| user_id | number | 否 | 全部 | 用户 ID |
| api_key_id | number | 否 | 全部 | API Key ID |
| group_id | number | 否 | 全部 | 分组 ID |
| platform | string | 否 | 全部 | openai/claude/gemini/other |
| model | string | 否 | 全部 | 模型 |
| upstream_account_id | number | 否 | 全部 | 上游账号 ID |
| request_id | string | 否 | 空 | 最多 128 字 |
| keyword | string | 否 | 空 | 2-100 字，匹配脱敏摘要 |
| ai_analysis | string | 否 | all | all/analyzed/not_analyzed |
| sort_by | string | 否 | occurred_at | occurred_at/status_code/severity/same_kind_count |
| sort_order | string | 否 | desc | asc/desc |

响应：

```json
{
  "data": {
    "items": [
      {
        "id": 1,
        "occurred_at": "2026-06-07T12:00:00+08:00",
        "error_category": "upstream",
        "error_subcategory": "upstream_rate_limit",
        "client_error_subcategory": null,
        "error_result": "final_failed",
        "severity": "P1",
        "status_code": 429,
        "user": { "id": 6, "email": "k***@example.com" },
        "api_key": { "id": 12, "name": "prod-key", "display": "prod-key #12 ****abcd" },
        "group": { "id": 3, "name": "VIP" },
        "platform": "openai",
        "model": "gpt-5.5",
        "upstream_account": { "id": 88, "name": "Op***01" },
        "summary": "上游返回 429 rate limit",
        "same_kind_count": 8,
        "ai_analysis_status": "completed"
      }
    ],
    "total": 1,
    "page": 1,
    "page_size": 20
  }
}
```

### 5.2 获取统一错误详情

```http
GET /api/v1/admin/ops/unified-errors/{id}
```

响应包含：`conclusion`、`request_chain`、`classification`、`impact_scope`、`recovery`、`ai_analysis`、`raw_record`、`same_kind_errors`。

`classification` 必须包含：

```json
{
  "error_category": "client",
  "error_subcategory": "client_parameter_error",
  "client_error_subcategory": "client_parameter_error",
  "classification_confidence": "high",
  "classification_reason": "请求参数缺少必填字段 model"
}
```


### 5.3 导出统一错误列表

```http
GET /api/v1/admin/ops/unified-errors/export
```

规则：最长 7 天，最多 100000 行，导出执行同页面脱敏规则。导出字段必须包含 `error_category`、`error_subcategory`、`client_error_subcategory`。

## 6. AI 运维分析 API

### 6.1 获取 AI 配置

```http
GET /api/v1/admin/ops/ai-analysis/config
```

响应：

```json
{
  "data": {
    "enabled": true,
    "base_url": "https://example.com/v1",
    "api_key_masked": "****abcd",
    "model": "gpt-5.5",
    "interface_type": "responses",
    "timeout_seconds": 60,
    "max_samples": 50,
    "auto_dedup_minutes": 10,
    "global_rate_limit_per_minute": 10,
    "auto_levels": ["P0", "P1"],
    "manual_enabled": true
  }
}
```

### 6.2 更新 AI 配置

```http
PUT /api/v1/admin/ops/ai-analysis/config
```

字段校验按 PRD 第 18 章执行。API Key 为空时保留原密文；传入新值时覆盖保存。

### 6.3 测试 AI 连接

```http
POST /api/v1/admin/ops/ai-analysis/test
```

规则：读取已保存的 AI 分析配置，验证服务地址、API Key 和模型是否可用；不创建 AI 分析任务，不写入 AI 分析报告。

响应：

```json
{
  "data": {
    "success": true,
    "status": "success",
    "message": "AI 分析服务连接成功",
    "interface_type": "responses",
    "base_url": "https://example.com/v1",
    "model": "gpt-5.5",
    "duration_ms": 320,
    "http_status": 200
  }
}
```

`status` 枚举：

| status | 说明 |
|---|---|
| success | 连接成功，服务地址、秘钥、模型可用 |
| config_error | 配置缺失、秘钥不可用或接口类型错误 |
| auth_failed | 上游返回 401/403，表示认证失败 |
| network_failed | 域名、端口、TLS 或网络连接失败 |
| timeout | 测试连接超时 |
| failed | 上游返回非 2xx 且非 401/403 的异常状态 |

失败时 HTTP 状态仍返回 200，页面根据 `success=false`、`status` 和 `message` 展示失败原因。运维监控未启用时按通用错误返回。

### 6.4 创建 AI 分析任务

```http
POST /api/v1/admin/ops/ai-analysis/tasks
```

请求：

```json
{
  "source_type": "unified_errors",
  "source_id": null,
  "time_start": "2026-06-07T11:30:00+08:00",
  "time_end": "2026-06-07T12:00:00+08:00",
  "filters": {
    "error_categories": ["upstream"],
    "error_subcategories": ["upstream_rate_limit"],
    "client_error_subcategories": [],
    "platform": "openai"
  }
}
```

字段规则：

| 字段 | 必填 | 规则 |
|---|---|---|
| source_type | 否 | `unified_errors` 或 `manual_filter`，为空按 `manual_filter` 处理 |
| source_id | 否 | 来源对象 ID；手动筛选任务可为空 |
| time_start | 是 | RFC3339 时间 |
| time_end | 是 | RFC3339 时间，必须晚于 `time_start` |
| filters | 否 | 当前统一错误列表筛选快照；未传按空筛选处理 |

`time_end - time_start` 最大 24 小时。`filters` 支持当前统一错误列表的错误分类、错误子类、客户端错误子类、处理结果、严重级别、状态码、用户、API Key、分组、上游账号、平台、模型、请求 ID、关键词等筛选字段。服务端会将筛选条件规范化后保存为任务快照，不保存 API 秘钥等敏感信息。

成功响应：

```http
HTTP/1.1 202 Accepted
```

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "task_id": 123,
    "status": "pending",
    "sample_count": 0,
    "matched_error_count": 18,
    "message": "AI 分析任务已提交"
  }
}
```

说明：

| 字段 | 说明 |
|---|---|
| task_id | 创建成功的 AI 分析任务 ID |
| status | 初始固定为 `pending` |
| sample_count | Worker 实际采样后更新；创建时为 0 |
| matched_error_count | 当前筛选条件命中的错误总数，用于页面提交反馈 |
| message | 前端成功提示文案 |

错误响应：

| HTTP 状态 | code | 场景 | 前端提示 |
|---|---|---|---|
| 400 | OPS_AI_ANALYSIS_NOT_CONFIGURED | AI 未启用、手动分析未启用、地址/模型/API Key 缺失 | 请先配置 AI 分析服务 |
| 400 | OPS_AI_ANALYSIS_INVALID_TIME / OPS_AI_ANALYSIS_INVALID_TIME_RANGE | 时间缺失、格式错误或结束时间不晚于开始时间 | 请选择有效时间范围 |
| 400 | OPS_AI_ANALYSIS_TIME_RANGE_TOO_LARGE | 时间范围超过 24 小时 | 手动分析时间范围最长 24 小时 |
| 400 | OPS_AI_ANALYSIS_INVALID_FILTER | 筛选字段类型或枚举非法 | 筛选条件不合法 |
| 400 | OPS_AI_ANALYSIS_NO_ERRORS | 当前筛选条件没有错误 | 当前范围暂无可分析错误 |
| 409 | OPS_AI_ANALYSIS_TASK_DUPLICATE | 同一筛选范围已有 pending/running 任务 | 分析任务处理中，请稍后查看 |
| 429 | OPS_AI_ANALYSIS_QUEUE_BUSY | pending/running 任务达到并发上限 3 个 | AI 分析队列繁忙，请稍后重试 |

并发控制：创建任务时在数据库事务内加锁，先检查同一筛选范围是否已有进行中任务，再检查队列上限，最后写入任务，避免重复提交和并发突破队列限制。

Worker 状态流转：后台 Worker 以 `FOR UPDATE SKIP LOCKED` 原子领取最早 `pending` 任务并置为 `running`；执行成功后置为 `completed` 并写入 `sample_count`、`finished_at`；执行失败后置为 `failed` 并写入脱敏后的 `error_message`、`finished_at`。服务停止或上下文取消时保持 `running`，不误标失败。

权限：平台所有者、管理员、运维角色可调用；其它后台角色返回 403。

### 6.5 获取 AI 分析任务和报告

```http
GET /api/v1/admin/ops/ai-analysis/tasks/{id}
```

报告响应中的 `root_cause`、`error_attribution`、`evidence_summary` 必须输出到可落地分类；当样本属于客户端请求错误时，`error_attribution.client_error_subcategory` 必须填充上述客户端错误细分枚举，证据不足时填 `client_insufficient_evidence`。

### 6.6 提交 AI 报告反馈

```http
POST /api/v1/admin/ops/ai-analysis/tasks/{id}/feedback
```

请求：

```json
{
  "feedback_status": "useful",
  "feedback_note": "判断准确"
}
```

## 7. 缓存管理 API

### 7.1 获取缓存配置

```http
GET /api/v1/admin/cache/config
```

### 7.2 更新缓存配置

```http
PUT /api/v1/admin/cache/config
```

请求：

```json
{
  "global_enabled": true,
  "platforms": {
    "openai": { "enabled": true },
    "claude": { "enabled": false },
    "gemini": { "enabled": false }
  },
  "ttl_seconds": 600,
  "max_request_bytes": 262144,
  "max_response_bytes": 524288,
  "max_temperature": 0.3,
  "model_allowlist": [],
  "model_blocklist": [],
  "bypass_header": {
    "name": "X-Sub2API-Cache-Control",
    "value": "bypass"
  }
}
```

固定绕过 Header：客户端请求带 `X-Sub2API-Cache-Control: bypass` 时，本次请求绕过缓存且不参与缓存 Key。

### 7.3 获取缓存统计

```http
GET /api/v1/admin/cache/stats
```

查询参数：`time_range`、`start_time`、`end_time`、`platform`、`model`、`api_key_id`、`group_id`。

响应：

```json
{
  "data": {
    "summary": {
      "total_requests": 2000,
      "candidate_requests": 1000,
      "hit_requests": 350,
      "request_hit_rate": 35.0,
      "input_tokens": 1200000,
      "output_tokens": 300000,
      "hit_tokens": 480000,
      "candidate_tokens": 1500000,
      "tokens_hit_rate": 32.0,
      "overall_tokens_coverage": 24.0,
      "estimated_saved_amount": "12.340000"
    },
    "model_rows": [],
    "bypass_reasons": [],
    "store_skip_reasons": []
  }
}
```

### 7.4 导出缓存统计

```http
GET /api/v1/admin/cache/stats/export
```

### 7.5 清理缓存

```http
POST /api/v1/admin/cache/clear
```

请求：

```json
{
  "clear_type": "by_model",
  "scope": {
    "platforms": ["openai"],
    "models": ["gpt-5.5"],
    "group_ids": [3],
    "api_key_ids": [12],
    "start_time": "2026-06-07T00:00:00+08:00",
    "end_time": "2026-06-07T23:59:59+08:00"
  },
  "confirm_text": "确认清理"
}
```

### 7.6 获取清理审计

```http
GET /api/v1/admin/cache/clear-audits
```

## 8. 高级缓存策略 API

### 8.1 获取高级缓存配置

```http
GET /api/v1/admin/cache/advanced-config
```

响应：

```json
{
  "data": {
    "advanced_cache_enabled": false,
    "gray_scope": {
      "api_key_ids": [],
      "group_ids": [],
      "models": []
    },
    "redis_capacity_mb": 512,
    "memory_safe_limit_mb": 2048,
    "compression_enabled": true,
    "compression_threshold_kb": 64,
    "eviction_policy": "LRU",
    "hot_window": "1h",
    "hot_threshold": 5,
    "cost_saving_enabled": true,
    "upstream_prompt_cache_enabled": true
  }
}
```

### 8.2 更新高级缓存配置

```http
PUT /api/v1/admin/cache/advanced-config
```

请求字段与 8.1 响应字段一致。`advanced_cache_enabled=false` 时，容量、压缩、淘汰、热点、成本节省等配置只保存和展示，不影响线上请求，线上请求回退到精确缓存逻辑；高级缓存灰度范围为空时不得对任何正式请求生效。

### 8.3 获取高级缓存统计

```http
GET /api/v1/admin/cache/advanced-stats
```

查询参数：

| 参数 | 类型 | 必填 | 默认值 | 规则 |
|---|---|---|---|---|
| time_range | string | 否 | today | today/1h/6h/24h/7d/31d/custom |
| start_time | string | 否 | 空 | custom 时必填 |
| end_time | string | 否 | 空 | custom 时必填；最长 31 天 |
| platform | string | 否 | 全部 | openai/claude/gemini |
| model | string | 否 | 全部 | 模型名 |
| api_key_id | number | 否 | 全部 | API Key ID |
| group_id | number | 否 | 全部 | 分组 ID |
| hotspot_limit | number | 否 | 20 | 1～100 |

响应：

```json
{
  "data": {
    "capacity": {
      "current_used_bytes": 268435456,
      "capacity_limit_bytes": 536870912,
      "capacity_usage_rate": 50.0,
      "memory_safe_limit_bytes": 2147483648,
      "eviction_policy": "LRU",
      "recent_eviction_count": 12,
      "last_evicted_at": "2026-06-07T12:00:00+08:00"
    },
    "compression": {
      "enabled": true,
      "raw_response_bytes": 1073741824,
      "stored_response_bytes": 429496729,
      "compression_saved_bytes": 644245095,
      "compression_saved_rate": 60.0,
      "compressed_entry_count": 2000,
      "compression_failed_count": 0,
      "decompression_failed_count": 0
    },
    "hotspots": [
      {
        "rank": 1,
        "platform": "openai",
        "model": "gpt-5.5",
        "group": { "id": 3, "name": "VIP" },
        "api_key": { "id": 12, "display": "prod-key #12 ****abcd" },
        "hit_count": 168,
        "hit_tokens": 560000,
        "last_hit_at": "2026-06-07T12:00:00+08:00"
      }
    ],
    "savings": {
      "local_response_cache_saved_tokens": 480000,
      "local_response_cache_saved_amount": "12.340000",
      "upstream_prompt_cache_read_tokens": 120000,
      "upstream_prompt_cache_write_tokens": 60000,
      "upstream_prompt_cache_saved_amount": "1.230000",
      "total_estimated_saved_amount": "13.570000",
      "price_missing": false,
      "price_missing_models": []
    },
    "empty_states": {
      "hotspots": false,
      "prompt_cache": false,
      "price": false
    },
    "fallback": {
      "advanced_cache_fallback_active": false,
      "fallback_reason": null,
      "last_fallback_at": null
    },
    "updated_at": "2026-06-07T12:00:00+08:00"
  }
}
```

字段规则：

1. `capacity_usage_rate = current_used_bytes / capacity_limit_bytes * 100`，分母为 0 时返回 0。
2. `compression_saved_rate = 1 - stored_response_bytes / raw_response_bytes`，分母为 0 时返回 0。
3. `hotspots` 不返回完整请求体、完整响应体、完整 API Key。
4. 本地响应缓存节省和上游 Prompt Cache 节省必须分字段返回，前端不得合并为单一 tokens 指标。
5. `price_missing=true` 时金额字段返回 `null` 或空字符串，前端展示价格缺失提示，不展示错误金额。
6. 高级缓存关闭或异常回退时，统计接口仍返回最近可用统计，并通过 `fallback` 字段展示回退状态。

## 9. 语义相似缓存 API

### 9.1 获取语义缓存配置

```http
GET /api/v1/admin/cache/semantic-config
```

响应：

```json
{
  "data": {
    "enabled": false,
    "stage": "observe",
    "platforms": [],
    "model_allowlist": [],
    "semantic_model_base_url": "",
    "semantic_api_key_masked": "",
    "semantic_model_name": "",
    "embedding_dimension": null,
    "rule_version": "v1",
    "similarity_threshold": 0.98,
    "max_reuse_minutes": 10,
    "max_candidates": 20,
    "gray_api_key_ids": [],
    "review_mode": true,
    "quality_rollback_threshold_percent": 1.0,
    "auto_closed": false,
    "auto_close_reason": null,
    "auto_closed_at": null
  }
}
```

### 9.2 更新语义缓存配置

```http
PUT /api/v1/admin/cache/semantic-config
```

请求：

```json
{
  "enabled": false,
  "stage": "observe",
  "platforms": ["openai"],
  "model_allowlist": ["gpt-5.5"],
  "semantic_model_base_url": "https://example.com/v1",
  "semantic_api_key": "sk-xxx",
  "semantic_model_name": "text-embedding-3-large",
  "similarity_threshold": 0.98,
  "max_reuse_minutes": 10,
  "max_candidates": 20,
  "gray_api_key_ids": [12],
  "review_mode": true,
  "quality_rollback_threshold_percent": 1.0
}
```

规则：`semantic_api_key` 为空时保留原密文；模型、向量维度、阈值、规则字段变更后生成新的 `rule_version`。`stage` 枚举为 `observe`、`review`、`gray`、`active`、`rollback`。

### 9.3 测试语义模型连接

```http
POST /api/v1/admin/cache/semantic-config/test
```

响应必须返回连接状态、模型可用性、向量维度、错误原因。

### 9.4 获取语义命中审计

```http
GET /api/v1/admin/cache/semantic-audits
```

查询参数：`page`、`page_size`、`start_time`、`end_time`、`platform`、`model`、`api_key_id`、`review_status`、`decision`、`min_similarity`、`max_similarity`。

列表字段：`id`、`request_id`、`semantic_entry_id`、`occurred_at`、`platform`、`model`、`api_key`、`similarity`、`decision`、`block_reason`、`review_status`、`feedback_type`、`auto_close_reason`、`source_summary`、`target_summary`。

### 9.5 审核语义候选

```http
POST /api/v1/admin/cache/semantic-audits/{id}/review
```

请求：

```json
{
  "review_status": "reusable",
  "note": "可复用"
}
```

`review_status` 枚举：`reusable`、`not_reusable`、`disputed`。

### 9.6 提交语义误命中反馈

```http
POST /api/v1/admin/cache/semantic-audits/{id}/feedback
```

请求：

```json
{
  "feedback_type": "wrong_hit",
  "note": "语义不同，不能复用"
}
```

`feedback_type` 枚举：`wrong_hit`、`complaint`、`manual_mark`。达到 `quality_rollback_threshold_percent` 后自动关闭语义缓存并记录 `auto_close_reason`，精确缓存继续可用。

## 10. 用户管理 API 增强

```http
GET /api/v1/admin/users
```

新增查询参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| balance_filter_type | string | none/gt/gte/lt/lte/between/eq |
| balance_min | string | 最小余额 |
| balance_max | string | 最大余额 |

## 11. 账号管理 API 调整

账号创建、编辑接口不再返回和提交以下字段：

```text
upstream_warning_amount
upstream_warning_email_enabled
upstream_warning_email_recipients
```

旧客户端提交时后端忽略，不写入、不校验、不触发邮件。

## 12. 仪表盘充值与复购 API

### 12.1 获取充值与入账概览

```http
GET /api/v1/admin/dashboard/revenue-overview
```

响应：

```json
{
  "data": {
    "total_credit_amount": "10000.00",
    "used_amount": "3200.00",
    "unused_amount": "6800.00",
    "non_admin_user_count": 200,
    "credited_user_count": 120,
    "is_estimated": false,
    "updated_at": "2026-06-07T12:00:00+08:00"
  }
}
```

### 12.2 获取复购分布

```http
GET /api/v1/admin/dashboard/repurchase-distribution
```

响应：

```json
{
  "data": {
    "buckets": [
      { "bucket": "zero", "label": "零购", "user_count": 100, "ratio": 50.0 },
      { "bucket": "one", "label": "一购", "user_count": 60, "ratio": 30.0 },
      { "bucket": "two", "label": "二购", "user_count": 20, "ratio": 10.0 },
      { "bucket": "three", "label": "三购", "user_count": 10, "ratio": 5.0 },
      { "bucket": "three_plus", "label": "三购以上", "user_count": 10, "ratio": 5.0 }
    ],
    "updated_at": "2026-06-07T12:00:00+08:00"
  }
}
```

