-- Ops cache/admin vNext: seed PRD 10.1.3 v2 compound default alert rules.
-- Idempotent: each built-in rule is keyed by filters.default_rule_key.



INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P0-服务不可用',
  '健康检查失败、DB 不可用或 Redis 关键依赖不可用导致主请求失败时，触发 P0。',
  true, 'P0', 'compound_rule', '>=', 1,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p0_service_unavailable"}'::jsonb,
  'v2', '["platform","config"]'::jsonb,
  'P0', 1, 0, 1, '{}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p0_service_unavailable'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P0-最终失败数量高',
  '1 分钟内最终失败请求数达到 20 次，触发 P0 事故级告警。',
  true, 'P0', 'compound_rule', '>=', 20,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p0_final_failures_high"}'::jsonb,
  'v2', '["client","platform","upstream","account_pool","rate_limit","permission","balance","config","slow_request","unknown"]'::jsonb,
  'P0', 20, 0, 50, '{}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p0_final_failures_high'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P0-最终失败率高',
  '1 分钟内最终失败率达到 20%，且最终失败请求数不少于 5 次，触发 P0 事故级告警。',
  true, 'P0', 'compound_rule', '>=', 5,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p0_final_failure_rate_high"}'::jsonb,
  'v2', '["client","platform","upstream","account_pool","rate_limit","permission","balance","config","slow_request","unknown"]'::jsonb,
  'P0', 5, 20, 50, '{}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p0_final_failure_rate_high'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P0-集中影响多用户',
  '同一错误分类 1 分钟内影响用户数达到 3 个，且最终失败不少于 5 次，触发 P0。',
  true, 'P0', 'compound_rule', '>=', 5,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p0_affected_users_concentrated"}'::jsonb,
  'v2', '["client","platform","upstream","account_pool","rate_limit","permission","balance","config","slow_request","unknown"]'::jsonb,
  'P0', 5, 0, 50, '{"affected_users":3}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p0_affected_users_concentrated'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P0-集中影响APIKey',
  '同一错误分类 1 分钟内影响 API Key 数达到 3 个，且最终失败不少于 5 次，触发 P0。',
  true, 'P0', 'compound_rule', '>=', 5,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p0_affected_api_keys_concentrated"}'::jsonb,
  'v2', '["client","platform","upstream","account_pool","rate_limit","permission","balance","config","slow_request","unknown"]'::jsonb,
  'P0', 5, 0, 50, '{"affected_api_keys":3}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p0_affected_api_keys_concentrated'
);



INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P0-同一模型大面积失败',
  '同一模型 1 分钟内最终失败请求数达到 10 次且失败率达到 20%，触发 P0。',
  true, 'P0', 'compound_rule', '>=', 10,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p0_model_wide_failure"}'::jsonb,
  'v2', '["platform","upstream","account_pool","rate_limit","permission","balance","config"]'::jsonb,
  'P0', 10, 20, 50, '{"affected_models":1}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p0_model_wide_failure'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P0-关键分组账号池不可用',
  '任一启用中的核心分组可用账号数为 0，触发 P0。',
  true, 'P0', 'compound_rule', '>=', 1,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p0_core_group_unavailable"}'::jsonb,
  'v2', '["account_pool","platform","config"]'::jsonb,
  'P0', 1, 0, 1, '{"affected_groups":1}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p0_core_group_unavailable'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P1-最终失败数量中等',
  '1 分钟内最终失败请求数达到 5 次，触发 P1 高优先级告警。',
  true, 'P1', 'compound_rule', '>=', 5,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p1_final_failures_medium"}'::jsonb,
  'v2', '["client","platform","upstream","account_pool","rate_limit","permission","balance","config","slow_request","unknown"]'::jsonb,
  'P1', 5, 0, 50, '{}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p1_final_failures_medium'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P1-最终失败率中等',
  '1 分钟内最终失败率达到 10%，且最终失败请求数不少于 3 次，触发 P1。',
  true, 'P1', 'compound_rule', '>=', 3,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p1_final_failure_rate_medium"}'::jsonb,
  'v2', '["client","platform","upstream","account_pool","rate_limit","permission","balance","config","slow_request","unknown"]'::jsonb,
  'P1', 3, 10, 50, '{}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p1_final_failure_rate_medium'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P1-上游账号集中失败',
  '同一上游账号 1 分钟内最终失败数达到 3 次，触发 P1。',
  true, 'P1', 'compound_rule', '>=', 3,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p1_upstream_account_concentrated"}'::jsonb,
  'v2', '["upstream","permission","balance","rate_limit"]'::jsonb,
  'P1', 3, 0, 20, '{"affected_upstream_accounts":1}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p1_upstream_account_concentrated'
);



INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P1-同一模型权限额度错误集中',
  '同一模型 1 分钟内权限、额度、订阅类错误数达到 3 次，触发 P1。',
  true, 'P1', 'compound_rule', '>=', 3,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p1_model_permission_quota_concentrated"}'::jsonb,
  'v2', '["permission","balance","rate_limit","upstream"]'::jsonb,
  'P1', 3, 0, 20, '{"affected_models":1}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p1_model_permission_quota_concentrated'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P2-少量最终失败观察',
  '1 分钟内出现 1～2 条最终失败时进入 P2 观察，不发送强提醒。',
  true, 'P2', 'compound_rule', '>=', 1,
  1, 1, 10, false, '{"builtin":true,"default_rule_key":"p2_few_final_failures_observe"}'::jsonb,
  'v2', '["client","platform","upstream","account_pool","rate_limit","permission","balance","config","slow_request","unknown"]'::jsonb,
  'P2', 1, 0, 1, '{}'::jsonb, 'observe_only', 1,
  false, '["in_app"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p2_few_final_failures_observe'
);


INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P1-同一用户严重受影响',
  '同一用户 1 分钟内最终失败数达到 3 次，触发 P1。',
  true, 'P1', 'compound_rule', '>=', 3,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p1_single_user_severely_affected"}'::jsonb,
  'v2', '["client","platform","upstream","rate_limit","permission","balance","config"]'::jsonb,
  'P1', 3, 0, 20, '{"affected_users":1}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p1_single_user_severely_affected'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P2-已恢复上游波动观察',
  '上游中途失败但最终请求成功时进入 P2 观察，不发送强提醒。',
  true, 'P2', 'compound_rule', '>=', 1,
  1, 1, 10, false, '{"builtin":true,"default_rule_key":"p2_recovered_upstream_fluctuation"}'::jsonb,
  'v2', '["upstream","rate_limit","permission","balance"]'::jsonb,
  'P2', 1, 0, 1, '{}'::jsonb, 'observe_only', 1,
  false, '["in_app"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p2_recovered_upstream_fluctuation'
);


INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P2-单一用户或Key波动观察',
  '仅影响 1 个用户或 1 个 API Key 且最终失败数较低时，进入 P2 观察。',
  true, 'P2', 'compound_rule', '>=', 1,
  1, 1, 10, false, '{"builtin":true,"default_rule_key":"p2_single_user_or_key_fluctuation"}'::jsonb,
  'v2', '["client","platform","upstream","rate_limit","permission","balance","config"]'::jsonb,
  'P2', 1, 0, 1, '{"affected_users":1,"affected_api_keys":1}'::jsonb, 'observe_only', 1,
  false, '["in_app"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p2_single_user_or_key_fluctuation'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P2-慢请求波动观察',
  '低样本下 P99 延迟高但无最终失败时，进入 P2 观察。',
  true, 'P2', 'compound_rule', '>=', 1,
  1, 1, 10, false, '{"builtin":true,"default_rule_key":"p2_slow_request_fluctuation"}'::jsonb,
  'v2', '["slow_request"]'::jsonb,
  'P2', 1, 0, 1, '{}'::jsonb, 'observe_only', 1,
  false, '["in_app"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p2_slow_request_fluctuation'
);
