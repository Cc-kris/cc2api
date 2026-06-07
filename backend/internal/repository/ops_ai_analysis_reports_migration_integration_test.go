//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration146CreatesOpsAIAnalysisReportsAndIsIdempotent(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	content, err := migrations.FS.ReadFile("146_ops_ai_analysis_reports.sql")
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "first migration execution should succeed")
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "second migration execution should be idempotent")

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_ai_analysis_reports (task_id, summary, root_cause, impact_scope, evidence, suggested_actions, error_breakdown, confidence, feedback_status, feedback_note)
VALUES (
    101,
    'upstream rate limit spike',
    'provider account throttled',
    '{"groups":[1]}'::jsonb,
    '[{"type":"log","count":12}]'::jsonb,
    '["switch account","notify customer"]'::jsonb,
    '{"upstream_rate_limit":12}'::jsonb,
    'high',
    'useful',
    'confirmed by ops'
)`)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, "INSERT INTO ops_ai_analysis_reports (task_id) VALUES (102)")
	require.NoError(t, err, "insert with defaults should succeed")

	var rowCount int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_ai_analysis_reports").Scan(&rowCount))
	require.Equal(t, 2, rowCount, "existing rows must be preserved after migration")

	var summary string
	var confidence string
	var feedbackStatus string
	var evidence string
	var createdAt time.Time
	var updatedAt time.Time
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT summary, confidence, feedback_status, evidence::text, created_at, updated_at
FROM ops_ai_analysis_reports
WHERE task_id = 102`).Scan(&summary, &confidence, &feedbackStatus, &evidence, &createdAt, &updatedAt))
	require.Equal(t, "", summary)
	require.Equal(t, "medium", confidence)
	require.Equal(t, "none", feedbackStatus)
	require.Equal(t, "[]", evidence)
	require.False(t, createdAt.IsZero())
	require.False(t, updatedAt.IsZero())

	expectConstraintError := func(name string, query string, args ...any) {
		t.Helper()
		_, err = tx.ExecContext(ctx, "SAVEPOINT "+name)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, query, args...)
		require.Error(t, err)
		_, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name)
		require.NoError(t, rollbackErr)
		_, err = tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name)
		require.NoError(t, err)
	}

	expectConstraintError("invalid_confidence", `
INSERT INTO ops_ai_analysis_reports (task_id, confidence)
VALUES (201, 'certain')
`)

	expectConstraintError("invalid_feedback_status", `
INSERT INTO ops_ai_analysis_reports (task_id, feedback_status)
VALUES (202, 'bad')
`)
}
