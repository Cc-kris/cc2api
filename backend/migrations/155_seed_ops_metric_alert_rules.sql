-- Seed metric-driven ops alert rules for health score and incident metrics.
-- Idempotent: each built-in rule is keyed by filters.default_rule_key.

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P0-健康分严重过低',
  '1 分钟综合健康分低于 50 时，触发 P0 事故级告警。',
  true, 'P0', 'health_score', '<', 50,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p0_health_score_critical_metric"}'::jsonb,
  'v2', '[]'::jsonb,
  'P0', 1, 0, 1, '{}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p0_health_score_critical_metric'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P1-健康分偏低',
  '1 分钟综合健康分低于 70 时，触发 P1 高优先级告警。',
  true, 'P1', 'health_score', '<', 70,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p1_health_score_risk_metric"}'::jsonb,
  'v2', '[]'::jsonb,
  'P1', 1, 0, 1, '{}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p1_health_score_risk_metric'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P0-事故失败率严重过高',
  '1 分钟事故失败率达到 20%，且最终失败不少于 5 次、样本不少于 50 次时，触发 P0。',
  true, 'P0', 'final_failure_rate', '>=', 20,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p0_final_failure_rate_high_metric"}'::jsonb,
  'v2', '[]'::jsonb,
  'P0', 5, 20, 50, '{}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p0_final_failure_rate_high_metric'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P1-事故失败率过高',
  '1 分钟事故失败率达到 10%，且最终失败不少于 3 次、样本不少于 50 次时，触发 P1。',
  true, 'P1', 'final_failure_rate', '>=', 10,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p1_final_failure_rate_medium_metric"}'::jsonb,
  'v2', '[]'::jsonb,
  'P1', 3, 10, 50, '{}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p1_final_failure_rate_medium_metric'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P0-事故失败数严重过高',
  '1 分钟最终失败数达到 20 次时，触发 P0。',
  true, 'P0', 'final_failures', '>=', 20,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p0_final_failures_high_metric"}'::jsonb,
  'v2', '[]'::jsonb,
  'P0', 20, 0, 1, '{}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p0_final_failures_high_metric'
);

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  '内置-P1-事故失败数过高',
  '1 分钟最终失败数达到 5 次时，触发 P1。',
  true, 'P1', 'final_failures', '>=', 5,
  1, 1, 10, true, '{"builtin":true,"default_rule_key":"p1_final_failures_medium_metric"}'::jsonb,
  'v2', '[]'::jsonb,
  'P1', 5, 0, 1, '{}'::jsonb, 'record_only', 0,
  true, '["in_app","email"]'::jsonb, 10, 'normal', NOW(), NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM ops_alert_rules WHERE filters->>'default_rule_key' = 'p1_final_failures_medium_metric'
);
