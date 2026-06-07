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

	timeStart := time.Date(2026, 6, 7, 11, 0, 0, 0, time.UTC)
	timeEnd := timeStart.Add(5 * time.Minute)
	for _, status := range []string{"pending", "running", "completed", "failed", "expired"} {
		_, err = tx.ExecContext(ctx, `
INSERT INTO ops_ai_analysis_tasks (
    source_type, source_id, trigger_type, trigger_user_id,
    time_start, time_end, filters, status, sample_count,
    provider, model, error_message, started_at, finished_at, expires_at
) VALUES (
    'manual_filter', NULL, 'manual', NULL,
    $1, $2, '{"platform":"openai"}'::jsonb, $3, 3,
    'openai_compatible', 'gpt-5.4', NULL, NULL, NULL, $4
)`, timeStart, timeEnd, status, timeEnd.Add(time.Hour))
		require.NoError(t, err, "insert status %s", status)
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_ai_analysis_tasks (source_type, time_start, time_end)
VALUES ('manual_filter', $1, $2)`, timeStart, timeEnd)
	require.NoError(t, err, "insert with defaults should succeed")

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_ai_analysis_tasks (
    source_type, source_id, trigger_type, time_start, time_end, status
) VALUES (
    'alert_event', 101, 'auto', $1, $2, 'pending'
)`, timeStart, timeEnd)
	require.NoError(t, err, "insert auto alert task should succeed")

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_ai_analysis_tasks (
    source_type, trigger_type, time_start, time_end, status, error_message, finished_at
) VALUES (
    'unified_errors', 'manual', $1, $2, 'failed', 'provider timeout', $2
)`, timeStart, timeEnd)
	require.NoError(t, err, "insert failed task should succeed")

	var rowCount int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_ai_analysis_tasks WHERE time_start = $1", timeStart).Scan(&rowCount))
	require.Equal(t, 8, rowCount)

	var defaultTriggerType string
	var defaultStatus string
	var defaultSampleCount int
	var defaultFilters string
	var defaultCreatedAt time.Time
	var defaultUpdatedAt time.Time
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT trigger_type, status, sample_count, filters::text, created_at, updated_at
FROM ops_ai_analysis_tasks
WHERE source_type = 'manual_filter' AND provider IS NULL
ORDER BY id DESC
LIMIT 1`).Scan(&defaultTriggerType, &defaultStatus, &defaultSampleCount, &defaultFilters, &defaultCreatedAt, &defaultUpdatedAt))
	require.Equal(t, "manual", defaultTriggerType)
	require.Equal(t, "pending", defaultStatus)
	require.Equal(t, 0, defaultSampleCount)
	require.Equal(t, "{}", defaultFilters)
	require.False(t, defaultCreatedAt.IsZero())
	require.False(t, defaultUpdatedAt.IsZero())

	var autoTaskCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM ops_ai_analysis_tasks
WHERE source_type = 'alert_event'
  AND source_id = 101
  AND trigger_type = 'auto'
  AND status = 'pending'`).Scan(&autoTaskCount))
	require.Equal(t, 1, autoTaskCount)

	var failedTaskCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM ops_ai_analysis_tasks
WHERE source_type = 'unified_errors'
  AND status = 'failed'
  AND error_message = 'provider timeout'
  AND finished_at = $1`).Scan(&failedTaskCount), timeEnd)
	require.Equal(t, 1, failedTaskCount)

	for _, indexName := range []string{
		"idx_ops_ai_analysis_tasks_status_created",
		"idx_ops_ai_analysis_tasks_source",
		"idx_ops_ai_analysis_tasks_trigger_user",
		"idx_ops_ai_analysis_tasks_time_range",
	} {
		var exists bool
		require.NoError(t, tx.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM pg_indexes
    WHERE schemaname LIKE 'pg_temp_%'
      AND tablename = 'ops_ai_analysis_tasks'
      AND indexname = $1
)`, indexName).Scan(&exists))
		require.True(t, exists, "expected index %s to exist", indexName)
	}

	var triggerUserIndexDef string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT indexdef
FROM pg_indexes
WHERE schemaname LIKE 'pg_temp_%'
  AND tablename = 'ops_ai_analysis_tasks'
  AND indexname = 'idx_ops_ai_analysis_tasks_trigger_user'`).Scan(&triggerUserIndexDef))
	require.Contains(t, triggerUserIndexDef, "trigger_user_id IS NOT NULL")
}
