package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration112UsesIdempotentAddColumn(t *testing.T) {
	content, err := FS.ReadFile("112_add_payment_order_provider_key_snapshot.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS provider_key VARCHAR(30)")
	require.NotContains(t, sql, "ADD COLUMN provider_key VARCHAR(30);")
}

func TestMigration118DoesNotForceOverwriteAuthSourceGrantDefaults(t *testing.T) {
	content, err := FS.ReadFile("118_wechat_dual_mode_and_auth_source_defaults.sql")
	require.NoError(t, err)

	sql := string(content)
	require.NotContains(t, sql, "UPDATE settings")
	require.NotContains(t, sql, "SET value = 'false'")
	require.True(t, strings.Contains(sql, "ON CONFLICT (key) DO NOTHING"))
	require.Contains(t, sql, "THEN ''")
}

func TestAuthIdentityReportTypeWideningRunsBeforeLongReportWritersAndStillReconcilesAt121(t *testing.T) {
	preflightContent, err := FS.ReadFile("108a_widen_auth_identity_migration_report_type.sql")
	require.NoError(t, err)

	preflightSQL := string(preflightContent)
	require.Contains(t, preflightSQL, "ALTER TABLE auth_identity_migration_reports")
	require.Contains(t, preflightSQL, "ALTER COLUMN report_type TYPE VARCHAR(80)")

	content, err := FS.ReadFile("109_auth_identity_compat_backfill.sql")
	require.NoError(t, err)

	sql := string(content)
	require.NotContains(t, sql, "ALTER TABLE auth_identity_migration_reports")

	followupContent, err := FS.ReadFile("121_auth_identity_migration_report_type_widen.sql")
	require.NoError(t, err)

	followupSQL := string(followupContent)
	require.Contains(t, followupSQL, "ALTER TABLE auth_identity_migration_reports")
	require.Contains(t, followupSQL, "ALTER COLUMN report_type TYPE VARCHAR(80)")
}

func TestMigration119DefersPaymentIndexRolloutToOnlineFollowup(t *testing.T) {
	content, err := FS.ReadFile("119_enforce_payment_orders_out_trade_no_unique.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "120_enforce_payment_orders_out_trade_no_unique_notx.sql")
	require.Contains(t, sql, "NULL;")
	require.NotContains(t, sql, "CREATE UNIQUE INDEX")
	require.NotContains(t, sql, "DROP INDEX")

	followupContent, err := FS.ReadFile("120_enforce_payment_orders_out_trade_no_unique_notx.sql")
	require.NoError(t, err)

	followupSQL := string(followupContent)
	require.Contains(t, followupSQL, "explicit duplicate out_trade_no precheck")
	require.Contains(t, followupSQL, "stale invalid paymentorder_out_trade_no_unique index")
	require.Contains(t, followupSQL, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS paymentorder_out_trade_no_unique")
	require.NotContains(t, followupSQL, "DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no_unique")
	require.Contains(t, followupSQL, "DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no")
	require.Contains(t, followupSQL, "WHERE out_trade_no <> ''")

	alignmentContent, err := FS.ReadFile("120a_align_payment_orders_out_trade_no_index_name.sql")
	require.NoError(t, err)

	alignmentSQL := string(alignmentContent)
	require.Contains(t, alignmentSQL, "paymentorder_out_trade_no_unique")
	require.Contains(t, alignmentSQL, "RENAME TO paymentorder_out_trade_no")
}

func TestMigration110SeedsAuthSourceSignupGrantsDisabledByDefault(t *testing.T) {
	content, err := FS.ReadFile("110_pending_auth_and_provider_default_grants.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "('auth_source_default_email_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_linuxdo_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_oidc_grant_on_signup', 'false')")
	require.Contains(t, sql, "('auth_source_default_wechat_grant_on_signup', 'false')")
	require.NotContains(t, sql, "('auth_source_default_email_grant_on_signup', 'true')")
}

func TestMigration122ScrubsPendingOAuthCompletionTokensAtRest(t *testing.T) {
	content, err := FS.ReadFile("122_pending_auth_completion_token_cleanup.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "UPDATE pending_auth_sessions")
	require.Contains(t, sql, "completion_response")
	require.Contains(t, sql, "access_token")
	require.Contains(t, sql, "refresh_token")
	require.Contains(t, sql, "expires_in")
	require.Contains(t, sql, "token_type")
}

func TestMigration123BackfillsLegacyAuthSourceGrantDefaultsSafely(t *testing.T) {
	content, err := FS.ReadFile("123_fix_legacy_auth_source_grant_on_signup_defaults.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "110_pending_auth_and_provider_default_grants.sql")
	require.Contains(t, sql, "schema_migrations")
	require.Contains(t, sql, "updated_at")
	require.Contains(t, sql, "'_grant_on_signup'")
	require.Contains(t, sql, "value = 'false'")
	require.Contains(t, sql, "auth_identity_migration_reports")
}

func TestMigration124BackfillsLegacyOIDCSecurityFlagsSafely(t *testing.T) {
	content, err := FS.ReadFile("124_backfill_legacy_oidc_security_flags.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "oidc_connect_use_pkce")
	require.Contains(t, sql, "oidc_connect_validate_id_token")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
	require.Contains(t, sql, "oidc_connect_enabled")
	require.Contains(t, sql, "'false'")
}

func TestMigration134AddsAffiliateLedgerAuditFieldsWithoutJSONCast(t *testing.T) {
	content, err := FS.ReadFile("134_affiliate_ledger_audit_snapshots.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS source_order_id BIGINT")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS balance_after DECIMAL(20,8)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS aff_quota_after DECIMAL(20,8)")
	require.Contains(t, sql, "substring(")
	require.Contains(t, sql, `"rebateAmount"`)
	require.Contains(t, sql, "COUNT(*) OVER (PARTITION BY ra.order_id) AS order_match_count")
	require.Contains(t, sql, "COUNT(*) OVER (PARTITION BY ual.id) AS ledger_match_count")
	require.NotContains(t, sql, "detail::jsonb")
}

func TestMigration135AllowsGitHubAndGoogleAuthProviders(t *testing.T) {
	content, err := FS.ReadFile("135_allow_email_oauth_provider_types.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "users_signup_source_check")
	require.Contains(t, sql, "auth_identities_provider_type_check")
	require.Contains(t, sql, "auth_identity_channels_provider_type_check")
	require.Contains(t, sql, "pending_auth_sessions_provider_type_check")
	require.Contains(t, sql, "'github'")
	require.Contains(t, sql, "'google'")
}

func TestMigration143ExtendsOpsAlertRulesWithoutDroppingLegacyRows(t *testing.T) {
	content, err := FS.ReadFile("143_ops_alert_rule_compound_conditions.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ALTER TABLE ops_alert_rules")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS rule_version VARCHAR(16)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS error_categories JSONB")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS min_final_failures INT")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS min_failure_rate DECIMAL(5,2)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS min_sample_count INT")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS impact_scope JSONB")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS recovered_fluctuation_policy VARCHAR(32)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS min_recovered_fluctuations INT")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS auto_ai_analysis BOOLEAN")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS notification_channels JSONB")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS silence_minutes INT")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS migration_state VARCHAR(32)")
	require.Contains(t, sql, "WHERE rule_version IS NULL")
	require.Contains(t, sql, "WHERE migration_state IS NULL")
	require.Contains(t, sql, "SET migration_state = 'readonly_legacy'")
	require.Contains(t, sql, "SET min_recovered_fluctuations = 0")
	require.Contains(t, sql, "SET DEFAULT 'v2'")
	require.Contains(t, sql, "ALTER COLUMN rule_version SET NOT NULL")
	require.Contains(t, sql, "ALTER COLUMN min_recovered_fluctuations SET DEFAULT 0")
	require.Contains(t, sql, "ALTER COLUMN min_recovered_fluctuations SET NOT NULL")
	require.Contains(t, sql, "ALTER COLUMN notification_channels SET DEFAULT")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_alert_rules_rule_version")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_alert_rules_trigger_level")
	require.NotContains(t, sql, "ADD COLUMN rule_version")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
	require.NotContains(t, sql, "DELETE FROM ops_alert_rules")
	require.NotContains(t, sql, "TRUNCATE")
	require.NotContains(t, sql, "DELETE FROM ops_alert_rules")
}

func TestMigration143BackfillsOnlyNullCompoundAlertRuleFields(t *testing.T) {
	content, err := FS.ReadFile("143_ops_alert_rule_compound_conditions.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "UPDATE ops_alert_rules\nSET rule_version = 'v1'\nWHERE rule_version IS NULL")
	require.Contains(t, sql, "UPDATE ops_alert_rules\nSET migration_state = 'readonly_legacy'\nWHERE migration_state IS NULL")
	require.Contains(t, sql, "UPDATE ops_alert_rules\nSET notification_channels = CASE")
	require.Contains(t, sql, "WHERE notification_channels IS NULL")
	require.NotContains(t, sql, "UPDATE ops_alert_rules\nSET name")
	require.NotContains(t, sql, "UPDATE ops_alert_rules\nSET metric_type")
	require.NotContains(t, sql, "UPDATE ops_alert_rules\nSET threshold")
}

func TestMigration143UsesIdempotentColumnAdds(t *testing.T) {
	content, err := FS.ReadFile("143_ops_alert_rule_compound_conditions.sql")
	require.NoError(t, err)

	sql := string(content)
	for _, column := range []string{
		"rule_version",
		"error_categories",
		"trigger_level",
		"min_final_failures",
		"min_failure_rate",
		"min_sample_count",
		"impact_scope",
		"recovered_fluctuation_policy",
		"min_recovered_fluctuations",
		"auto_ai_analysis",
		"notification_channels",
		"silence_minutes",
		"migration_state",
	} {
		require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS "+column, "column %s must be added idempotently", column)
		require.NotContains(t, sql, "ADD COLUMN "+column, "column %s must not be added non-idempotently", column)
	}
}

func TestMigration144ExtendsOpsAlertEventsLifecycleFields(t *testing.T) {
	content, err := FS.ReadFile("144_ops_alert_event_lifecycle_fields.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ALTER TABLE ops_alert_events")
	for _, column := range []string{
		"event_key",
		"lifecycle_status",
		"merged_count",
		"last_seen_at",
		"recovered_at",
		"acknowledged_at",
		"acknowledged_by",
		"acknowledged_note",
		"processing_at",
		"processing_by",
		"processing_note",
		"processing_action",
		"closed_at",
		"closed_by",
		"closed_reason",
		"trigger_snapshot",
		"score_snapshot",
		"ai_task_id",
	} {
		require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS "+column, "column %s must be added idempotently", column)
		require.NotContains(t, sql, "ADD COLUMN "+column, "column %s must not be added non-idempotently", column)
	}
	require.Contains(t, sql, "SET lifecycle_status = CASE")
	require.Contains(t, sql, "WHEN status = 'resolved' THEN 'recovered'")
	require.Contains(t, sql, "WHEN status = 'manual_resolved' THEN 'closed'")
	require.Contains(t, sql, "WHERE lifecycle_status IS NULL")
	require.Contains(t, sql, "SET merged_count = 0")
	require.Contains(t, sql, "WHERE merged_count IS NULL")
	require.Contains(t, sql, "SET last_seen_at = fired_at")
	require.Contains(t, sql, "WHERE last_seen_at IS NULL")
	require.Contains(t, sql, "SET recovered_at = resolved_at")
	require.Contains(t, sql, "WHERE recovered_at IS NULL")
	require.Contains(t, sql, "AND resolved_at IS NOT NULL")
	require.Contains(t, sql, "SET closed_at = resolved_at")
	require.Contains(t, sql, "WHERE closed_at IS NULL")
	require.Contains(t, sql, "status = 'manual_resolved'")
	require.Contains(t, sql, "AND resolved_at IS NOT NULL")
	require.Contains(t, sql, "ALTER COLUMN lifecycle_status SET NOT NULL")
	require.Contains(t, sql, "ALTER COLUMN merged_count SET NOT NULL")
	require.Contains(t, sql, "ALTER COLUMN last_seen_at SET NOT NULL")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_alert_events_event_key_status")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_alert_events_lifecycle_last_seen")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_alert_events_ai_task_id")
	require.NotContains(t, sql, "UPDATE ops_alert_events\nSET title")
	require.NotContains(t, sql, "UPDATE ops_alert_events\nSET description")
	require.NotContains(t, sql, "UPDATE ops_alert_events\nSET metric_value")
	require.NotContains(t, sql, "UPDATE ops_alert_events\nSET threshold_value")
	require.NotContains(t, sql, "UPDATE ops_alert_events\nSET dimensions")
	require.NotContains(t, sql, "UPDATE ops_alert_events\nSET email_sent")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
	require.NotContains(t, sql, "DELETE FROM ops_alert_events")
	require.NotContains(t, sql, "TRUNCATE")
}

func TestMigration145CreatesOpsAIAnalysisTasks(t *testing.T) {
	content, err := FS.ReadFile("145_ops_ai_analysis_tasks.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS ops_ai_analysis_tasks")
	for _, column := range []string{
		"id BIGSERIAL PRIMARY KEY",
		"source_type VARCHAR(32) NOT NULL",
		"source_id BIGINT",
		"trigger_type VARCHAR(16) NOT NULL",
		"trigger_user_id BIGINT",
		"time_start TIMESTAMPTZ NOT NULL",
		"time_end TIMESTAMPTZ NOT NULL",
		"filters JSONB NOT NULL DEFAULT '{}'::jsonb",
		"status VARCHAR(16) NOT NULL DEFAULT 'pending'",
		"sample_count INT NOT NULL DEFAULT 0",
		"provider VARCHAR(32)",
		"model VARCHAR(100)",
		"error_message TEXT",
		"started_at TIMESTAMPTZ",
		"finished_at TIMESTAMPTZ",
	} {
		require.Contains(t, sql, column)
	}
	require.Contains(t, sql, "CHECK (source_type IN ('alert_event', 'unified_errors', 'manual_filter'))")
	require.Contains(t, sql, "CHECK (trigger_type IN ('auto', 'manual'))")
	require.Contains(t, sql, "CHECK (status IN ('pending', 'running', 'completed', 'failed', 'expired'))")
	require.Contains(t, sql, "CHECK (sample_count >= 0)")
	require.Contains(t, sql, "CHECK (time_end >= time_start)")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_status_created_at")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_source")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_time_range")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_tasks_trigger_user_id")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
	require.NotContains(t, sql, "DELETE FROM ops_ai_analysis_tasks")
	require.NotContains(t, sql, "TRUNCATE")
}

func TestMigration146CreatesOpsAIAnalysisReports(t *testing.T) {
	content, err := FS.ReadFile("146_ops_ai_analysis_reports.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS ops_ai_analysis_reports")
	for _, column := range []string{
		"task_id BIGINT PRIMARY KEY",
		"summary TEXT NOT NULL DEFAULT ''",
		"root_cause TEXT",
		"impact_scope JSONB NOT NULL DEFAULT '{}'::jsonb",
		"evidence JSONB NOT NULL DEFAULT '[]'::jsonb",
		"suggested_actions JSONB NOT NULL DEFAULT '[]'::jsonb",
		"error_breakdown JSONB NOT NULL DEFAULT '{}'::jsonb",
		"confidence VARCHAR(16) NOT NULL DEFAULT 'medium'",
		"feedback_status VARCHAR(32) NOT NULL DEFAULT 'none'",
		"feedback_note TEXT",
	} {
		require.Contains(t, sql, column)
	}
	require.Contains(t, sql, "CHECK (confidence IN ('high', 'medium', 'low'))")
	require.Contains(t, sql, "CHECK (feedback_status IN ('none', 'useful', 'not_useful', 'wrong_category'))")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_reports_confidence")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_ai_analysis_reports_feedback_status")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
	require.NotContains(t, sql, "DELETE FROM ops_ai_analysis_reports")
	require.NotContains(t, sql, "TRUNCATE")
}

func TestMigration147CreatesOpsCacheMinuteStats(t *testing.T) {
	content, err := FS.ReadFile("147_ops_cache_minute_stats.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS ops_cache_minute_stats")
	for _, column := range []string{
		"minute_at TIMESTAMPTZ NOT NULL",
		"platform VARCHAR(32) NOT NULL",
		"model VARCHAR(128) NOT NULL",
		"group_id BIGINT",
		"api_key_id BIGINT",
		"cache_type VARCHAR(16) NOT NULL",
		"total_requests BIGINT NOT NULL DEFAULT 0",
		"candidate_requests BIGINT NOT NULL DEFAULT 0",
		"hit_requests BIGINT NOT NULL DEFAULT 0",
		"bypass_requests BIGINT NOT NULL DEFAULT 0",
		"store_success BIGINT NOT NULL DEFAULT 0",
		"store_skip BIGINT NOT NULL DEFAULT 0",
		"input_tokens BIGINT NOT NULL DEFAULT 0",
		"output_tokens BIGINT NOT NULL DEFAULT 0",
		"hit_tokens BIGINT NOT NULL DEFAULT 0",
		"candidate_tokens BIGINT NOT NULL DEFAULT 0",
		"all_request_tokens BIGINT NOT NULL DEFAULT 0",
		"bypass_reasons JSONB NOT NULL DEFAULT '{}'::jsonb",
		"store_skip_reasons JSONB NOT NULL DEFAULT '{}'::jsonb",
		"estimated_saved_amount DECIMAL(20,8) NOT NULL DEFAULT 0",
	} {
		require.Contains(t, sql, column)
	}
	require.Contains(t, sql, "CHECK (cache_type IN ('exact', 'semantic'))")
	require.Contains(t, sql, "total_requests >= 0")
	require.Contains(t, sql, "group_id IS NULL OR group_id >= 0")
	require.Contains(t, sql, "api_key_id IS NULL OR api_key_id >= 0")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS idx_ops_cache_minute_stats_unique_bucket")
	require.Contains(t, sql, "COALESCE(group_id, -1)")
	require.Contains(t, sql, "COALESCE(api_key_id, -1)")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_cache_minute_stats_time")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_cache_minute_stats_platform_model_time")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_cache_minute_stats_group_time")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_ops_cache_minute_stats_api_key_time")
	require.NotContains(t, sql, "DROP TABLE")
	require.NotContains(t, sql, "DROP COLUMN")
	require.NotContains(t, sql, "DELETE FROM ops_cache_minute_stats")
	require.NotContains(t, sql, "TRUNCATE")
}
