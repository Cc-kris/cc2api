-- Ops cache/admin vNext: migrate legacy single-percent alert rules into v2 compound rules.
-- Legacy rows are preserved and marked as migrated/readonly; generated v2 rows are idempotent per legacy rule id.

INSERT INTO ops_alert_rules (
  name, description, enabled, severity, metric_type, operator, threshold,
  window_minutes, sustained_minutes, cooldown_minutes, notify_email, filters,
  rule_version, error_categories, trigger_level, min_final_failures, min_failure_rate,
  min_sample_count, impact_scope, recovered_fluctuation_policy, min_recovered_fluctuations,
  auto_ai_analysis, notification_channels, silence_minutes, migration_state, created_at, updated_at
)
SELECT
  CONCAT('迁移-', legacy.id, '-', LEFT(legacy.name, 100)),
  CONCAT('由旧单一百分比规则迁移：', COALESCE(NULLIF(legacy.description, ''), legacy.name)),
  legacy.enabled,
  CASE WHEN legacy.severity IN ('P0','P1','P2') THEN legacy.severity ELSE 'P2' END,
  'compound_rule',
  '>=',
  CASE
    WHEN legacy.severity = 'P0' THEN 5
    WHEN legacy.severity = 'P1' THEN 3
    ELSE 1
  END,
  1,
  1,
  GREATEST(COALESCE(legacy.cooldown_minutes, 10), 0),
  COALESCE(legacy.notify_email, true),
  jsonb_build_object(
    'builtin', false,
    'migrated_from_rule_id', legacy.id,
    'default_rule_key', CONCAT('migrated_legacy_percent_rule_', legacy.id),
    'legacy_metric_type', legacy.metric_type,
    'legacy_operator', legacy.operator,
    'legacy_threshold', legacy.threshold
  ),
  'v2',
  CASE
    WHEN legacy.metric_type = 'upstream_error_rate' THEN '["upstream","rate_limit","permission","balance"]'::jsonb
    ELSE '["client","platform","upstream","account_pool","rate_limit","permission","balance","config","slow_request","unknown"]'::jsonb
  END,
  CASE WHEN legacy.severity IN ('P0','P1','P2') THEN legacy.severity ELSE 'P2' END,
  CASE
    WHEN legacy.severity = 'P0' THEN 5
    WHEN legacy.severity = 'P1' THEN 3
    ELSE 1
  END,
  CASE
    WHEN legacy.metric_type = 'success_rate' THEN GREATEST(0, LEAST(100, 100 - legacy.threshold))
    ELSE GREATEST(0, LEAST(100, legacy.threshold))
  END,
  50,
  '{}'::jsonb,
  'record_only',
  0,
  CASE WHEN legacy.severity IN ('P0','P1') THEN true ELSE false END,
  CASE
    WHEN COALESCE(legacy.notify_email, true) AND legacy.severity IN ('P0','P1') THEN '["in_app","email"]'::jsonb
    ELSE '["in_app"]'::jsonb
  END,
  GREATEST(COALESCE(legacy.cooldown_minutes, 10), 0),
  'normal',
  NOW(),
  NOW()
FROM ops_alert_rules legacy
WHERE legacy.rule_version = 'v1'
  AND legacy.metric_type IN ('success_rate', 'error_rate', 'upstream_error_rate')
  AND legacy.threshold >= 0
  AND legacy.threshold <= 100
  AND NOT EXISTS (
    SELECT 1
    FROM ops_alert_rules migrated
    WHERE migrated.filters->>'default_rule_key' = CONCAT('migrated_legacy_percent_rule_', legacy.id)
  );

UPDATE ops_alert_rules legacy
SET migration_state = 'migrated',
    updated_at = NOW()
WHERE legacy.rule_version = 'v1'
  AND legacy.metric_type IN ('success_rate', 'error_rate', 'upstream_error_rate')
  AND legacy.migration_state <> 'migrated'
  AND EXISTS (
    SELECT 1
    FROM ops_alert_rules migrated
    WHERE migrated.filters->>'default_rule_key' = CONCAT('migrated_legacy_percent_rule_', legacy.id)
  );
