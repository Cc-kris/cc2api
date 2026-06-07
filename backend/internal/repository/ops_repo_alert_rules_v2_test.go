package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryCreateAlertRulePersistsCompoundFields(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	rule := &service.OpsAlertRule{
		Name:                       "上游账号集中失败",
		Description:                "上游账号集中失败规则",
		Enabled:                    true,
		Severity:                   "P1",
		MetricType:                 "compound_rule",
		Operator:                   ">=",
		Threshold:                  5,
		WindowMinutes:              1,
		SustainedMinutes:           1,
		CooldownMinutes:            10,
		NotifyEmail:                true,
		Filters:                    map[string]any{"platform": "openai"},
		RuleVersion:                "v2",
		ErrorCategories:            []string{"upstream", "permission"},
		TriggerLevel:               "P1",
		MinFinalFailures:           5,
		MinFailureRate:             10,
		MinSampleCount:             50,
		ImpactScope:                map[string]int{"affected_api_keys": 2},
		RecoveredFluctuationPolicy: "observe_only",
		MinRecoveredFluctuations:   10,
		AutoAIAnalysis:             true,
		NotificationChannels:       []string{"in_app", "email"},
		SilenceMinutes:             10,
		MigrationState:             "normal",
	}

	rows := sqlmock.NewRows([]string{
		"id", "name", "description", "enabled", "severity", "metric_type", "operator", "threshold",
		"window_minutes", "sustained_minutes", "cooldown_minutes", "notify_email", "filters",
		"rule_version", "error_categories", "trigger_level", "min_final_failures", "min_failure_rate",
		"min_sample_count", "impact_scope", "recovered_fluctuation_policy", "min_recovered_fluctuations",
		"auto_ai_analysis", "notification_channels", "silence_minutes", "migration_state", "last_triggered_at", "created_at", "updated_at",
	}).AddRow(
		int64(1), rule.Name, rule.Description, true, "P1", "compound_rule", ">=", 5.0,
		1, 1, 10, true, []byte(`{"platform":"openai"}`),
		"v2", []byte(`["upstream","permission"]`), "P1", 5, 10.0,
		50, []byte(`{"affected_api_keys":2}`), "observe_only", 10,
		true, []byte(`["in_app","email"]`), 10, "normal", nil, now, now,
	)

	mock.ExpectQuery(`(?s)INSERT INTO ops_alert_rules .*rule_version.*error_categories.*RETURNING`).
		WithArgs(
			rule.Name, rule.Description, true, "P1", "compound_rule", ">=", 5.0,
			1, 1, 10, true, sqlmock.AnyArg(), "v2", sqlmock.AnyArg(), "P1", 5, 10.0, 50,
			sqlmock.AnyArg(), "observe_only", 10, true, sqlmock.AnyArg(), 10, "normal",
		).
		WillReturnRows(rows)

	got, err := repo.CreateAlertRule(context.Background(), rule)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.ID)
	require.Equal(t, []string{"upstream", "permission"}, got.ErrorCategories)
	require.Equal(t, map[string]int{"affected_api_keys": 2}, got.ImpactScope)
	require.Equal(t, []string{"in_app", "email"}, got.NotificationChannels)
	require.Equal(t, "v2", got.RuleVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}
