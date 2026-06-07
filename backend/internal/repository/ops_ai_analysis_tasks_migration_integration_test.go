//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration145CreatesOpsAIAnalysisTasksAndIsIdempotent(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	content, err := migrations.FS.ReadFile("145_ops_ai_analysis_tasks.sql")
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "first migration execution should succeed")
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "second migration execution should be idempotent")

	timeStart := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	timeEnd := time.Date(2026, 6, 7, 10, 1, 0, 0, time.UTC)
	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_ai_analysis_tasks (id, source_type, source_id, trigger_type, trigger_user_id, time_start, time_end, filters)
VALUES (1, 'unified_errors', NULL, 'manual', 42, $1, $2, '{"platform":"openai"}'::jsonb)
`, timeStart, timeEnd)
	require.NoError(t, err)

	var rowCount int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_ai_analysis_tasks").Scan(&rowCount))
	require.Equal(t, 1, rowCount, "existing rows must be preserved after migration")

	var status string
	var sampleCount int
	var filters string
	var createdAt time.Time
	var updatedAt time.Time
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT status, sample_count, filters::text, created_at, updated_at
FROM ops_ai_analysis_tasks
WHERE id = 1`).Scan(&status, &sampleCount, &filters, &createdAt, &updatedAt))
	require.Equal(t, "pending", status)
	require.Equal(t, 0, sampleCount)
	require.Contains(t, filters, "openai")
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

	expectConstraintError("invalid_source_type", `
INSERT INTO ops_ai_analysis_tasks (source_type, trigger_type, time_start, time_end, filters, status)
VALUES ('unknown', 'manual', $1, $2, '{}'::jsonb, 'pending')
`, timeStart, timeEnd)

	expectConstraintError("invalid_time_range", `
INSERT INTO ops_ai_analysis_tasks (source_type, trigger_type, time_start, time_end, filters, status)
VALUES ('unified_errors', 'manual', $2, $1, '{}'::jsonb, 'pending')
`, timeStart, timeEnd)
}
