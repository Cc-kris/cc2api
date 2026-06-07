-- Ops cache/admin vNext: extend alert rules for compound condition evaluation.
-- This migration is intentionally idempotent and preserves existing v1 rule rows.

ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS rule_version VARCHAR(16),
    ADD COLUMN IF NOT EXISTS error_categories JSONB,
    ADD COLUMN IF NOT EXISTS trigger_level VARCHAR(16),
    ADD COLUMN IF NOT EXISTS min_final_failures INT,
    ADD COLUMN IF NOT EXISTS min_failure_rate DECIMAL(5,2),
    ADD COLUMN IF NOT EXISTS min_sample_count INT,
    ADD COLUMN IF NOT EXISTS impact_scope JSONB,
    ADD COLUMN IF NOT EXISTS recovered_fluctuation_policy VARCHAR(32),
    ADD COLUMN IF NOT EXISTS min_recovered_fluctuations INT,
    ADD COLUMN IF NOT EXISTS auto_ai_analysis BOOLEAN,
    ADD COLUMN IF NOT EXISTS notification_channels JSONB,
    ADD COLUMN IF NOT EXISTS silence_minutes INT,
    ADD COLUMN IF NOT EXISTS migration_state VARCHAR(32);

-- Existing rows are legacy rule definitions until BE-017 migrates them to v2 compound rules.
UPDATE ops_alert_rules
SET rule_version = 'v1'
WHERE rule_version IS NULL;

UPDATE ops_alert_rules
SET trigger_level = CASE
    WHEN severity IN ('P0', 'P1', 'P2') THEN severity
    WHEN severity = 'P3' THEN 'observe'
    ELSE 'P2'
END
WHERE trigger_level IS NULL;

UPDATE ops_alert_rules
SET error_categories = '[]'::jsonb
WHERE error_categories IS NULL;

UPDATE ops_alert_rules
SET min_final_failures = 1
WHERE min_final_failures IS NULL;

UPDATE ops_alert_rules
SET min_failure_rate = 0
WHERE min_failure_rate IS NULL;

UPDATE ops_alert_rules
SET min_sample_count = GREATEST(COALESCE(window_minutes, 1), 1)
WHERE min_sample_count IS NULL;

UPDATE ops_alert_rules
SET impact_scope = '{}'::jsonb
WHERE impact_scope IS NULL;

UPDATE ops_alert_rules
SET recovered_fluctuation_policy = 'record_only'
WHERE recovered_fluctuation_policy IS NULL;

UPDATE ops_alert_rules
SET auto_ai_analysis = false
WHERE auto_ai_analysis IS NULL;

UPDATE ops_alert_rules
SET notification_channels = CASE
    WHEN notify_email THEN '["in_app", "email"]'::jsonb
    ELSE '["in_app"]'::jsonb
END
WHERE notification_channels IS NULL;

UPDATE ops_alert_rules
SET silence_minutes = GREATEST(COALESCE(cooldown_minutes, 10), 0)
WHERE silence_minutes IS NULL;

UPDATE ops_alert_rules
SET migration_state = 'readonly_legacy'
WHERE migration_state IS NULL;

ALTER TABLE ops_alert_rules
    ALTER COLUMN rule_version SET DEFAULT 'v2',
    ALTER COLUMN rule_version SET NOT NULL,
    ALTER COLUMN error_categories SET DEFAULT '[]'::jsonb,
    ALTER COLUMN error_categories SET NOT NULL,
    ALTER COLUMN trigger_level SET DEFAULT 'P2',
    ALTER COLUMN trigger_level SET NOT NULL,
    ALTER COLUMN min_final_failures SET DEFAULT 1,
    ALTER COLUMN min_final_failures SET NOT NULL,
    ALTER COLUMN min_failure_rate SET DEFAULT 0,
    ALTER COLUMN min_failure_rate SET NOT NULL,
    ALTER COLUMN min_sample_count SET DEFAULT 1,
    ALTER COLUMN min_sample_count SET NOT NULL,
    ALTER COLUMN impact_scope SET DEFAULT '{}'::jsonb,
    ALTER COLUMN impact_scope SET NOT NULL,
    ALTER COLUMN recovered_fluctuation_policy SET DEFAULT 'record_only',
    ALTER COLUMN recovered_fluctuation_policy SET NOT NULL,
    ALTER COLUMN auto_ai_analysis SET DEFAULT false,
    ALTER COLUMN auto_ai_analysis SET NOT NULL,
    ALTER COLUMN notification_channels SET DEFAULT '["in_app"]'::jsonb,
    ALTER COLUMN notification_channels SET NOT NULL,
    ALTER COLUMN silence_minutes SET DEFAULT 10,
    ALTER COLUMN silence_minutes SET NOT NULL,
    ALTER COLUMN migration_state SET DEFAULT 'normal',
    ALTER COLUMN migration_state SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ops_alert_rules_rule_version
    ON ops_alert_rules (rule_version);

CREATE INDEX IF NOT EXISTS idx_ops_alert_rules_trigger_level
    ON ops_alert_rules (trigger_level);

COMMENT ON COLUMN ops_alert_rules.rule_version IS 'Alert rule schema version: v1 legacy, v2 compound rule.';
COMMENT ON COLUMN ops_alert_rules.error_categories IS 'Selected unified error categories for compound alert matching.';
COMMENT ON COLUMN ops_alert_rules.trigger_level IS 'Compound alert trigger level: P0/P1/P2/observe.';
COMMENT ON COLUMN ops_alert_rules.min_final_failures IS 'Minimum final failed requests required for alert firing.';
COMMENT ON COLUMN ops_alert_rules.min_failure_rate IS 'Minimum final failure rate percentage; 0 disables rate condition.';
COMMENT ON COLUMN ops_alert_rules.min_sample_count IS 'Minimum effective request sample count for rate-based alerts.';
COMMENT ON COLUMN ops_alert_rules.impact_scope IS 'Compound impact scope thresholds: users, api keys, groups, models, upstream accounts.';
COMMENT ON COLUMN ops_alert_rules.recovered_fluctuation_policy IS 'Recovered fluctuation policy: record_only/observe_only/alert.';
COMMENT ON COLUMN ops_alert_rules.min_recovered_fluctuations IS 'Minimum recovered fluctuation count when fluctuation policy participates.';
COMMENT ON COLUMN ops_alert_rules.auto_ai_analysis IS 'Whether this alert rule auto-creates AI analysis tasks.';
COMMENT ON COLUMN ops_alert_rules.notification_channels IS 'Notification channels: in_app, email, none.';
COMMENT ON COLUMN ops_alert_rules.silence_minutes IS 'Silence window in minutes for same alert key merging.';
COMMENT ON COLUMN ops_alert_rules.migration_state IS 'Rule migration state: normal/migrated/readonly_legacy.';
