//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration147CreatesOpsCacheMinuteStatsAndIsIdempotent(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	content, err := migrations.FS.ReadFile("147_ops_cache_minute_stats.sql")
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "first migration execution should succeed")
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "second migration execution should be idempotent")

	minuteAt := time.Date(2026, 6, 7, 11, 35, 0, 0, time.UTC)
	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_cache_minute_stats (
    minute_at, platform, model, group_id, api_key_id, cache_type,
    total_requests, candidate_requests, hit_requests, bypass_requests,
    store_success, store_skip, input_tokens, output_tokens, hit_tokens,
    candidate_tokens, all_request_tokens, bypass_reasons, store_skip_reasons,
    estimated_saved_amount
) VALUES (
    $1, 'openai', 'gpt-5.5', 3, 12, 'exact',
    100, 80, 30, 20,
    50, 10, 1000, 200, 600,
    900, 1200, '{"header_bypass":20}'::jsonb, '{"too_large":10}'::jsonb,
    1.23000000
)`, minuteAt)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_cache_minute_stats (minute_at, platform, model, cache_type)
VALUES ($1, 'gemini', 'gemini-2.5-pro', 'semantic')
`, minuteAt)
	require.NoError(t, err, "insert with defaults should succeed")

	var rowCount int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_cache_minute_stats").Scan(&rowCount))
	require.Equal(t, 2, rowCount, "existing rows must be preserved after migration")

	var hitTokens int64
	var bypassReasons string
	var storeSkipReasons string
	var estimatedSaved string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT hit_tokens, bypass_reasons::text, store_skip_reasons::text, estimated_saved_amount::text
FROM ops_cache_minute_stats
WHERE platform = 'openai' AND model = 'gpt-5.5'
`).Scan(&hitTokens, &bypassReasons, &storeSkipReasons, &estimatedSaved))
	require.Equal(t, int64(600), hitTokens)
	require.Contains(t, bypassReasons, "header_bypass")
	require.Contains(t, storeSkipReasons, "too_large")
	require.Equal(t, "1.23000000", estimatedSaved)

	var totalRequests int64
	var candidateRequests int64
	var defaultReasons string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT total_requests, candidate_requests, bypass_reasons::text
FROM ops_cache_minute_stats
WHERE platform = 'gemini'
`).Scan(&totalRequests, &candidateRequests, &defaultReasons))
	require.Equal(t, int64(0), totalRequests)
	require.Equal(t, int64(0), candidateRequests)
	require.Equal(t, "{}", defaultReasons)

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_cache_minute_stats (minute_at, platform, model, group_id, cache_type)
VALUES ($1, 'gemini', 'gemini-2.5-pro', 3, 'semantic')
`, minuteAt)
	require.NoError(t, err, "same minute/model/cache type with a concrete group should not collide with the global NULL bucket")

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_cache_minute_stats (minute_at, platform, model, api_key_id, cache_type)
VALUES ($1, 'gemini', 'gemini-2.5-pro', 12, 'semantic')
`, minuteAt)
	require.NoError(t, err, "same minute/model/cache type with a concrete API key should not collide with the global NULL bucket")

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

	expectConstraintError("duplicate_null_bucket", `
INSERT INTO ops_cache_minute_stats (minute_at, platform, model, cache_type)
VALUES ($1, 'gemini', 'gemini-2.5-pro', 'semantic')
`, minuteAt)

	expectConstraintError("invalid_cache_type", `
INSERT INTO ops_cache_minute_stats (minute_at, platform, model, cache_type)
VALUES ($1, 'openai', 'gpt-5.5', 'unknown')
`, minuteAt)

	expectConstraintError("negative_counter", `
INSERT INTO ops_cache_minute_stats (minute_at, platform, model, cache_type, hit_tokens)
VALUES ($1, 'openai', 'gpt-5.5', 'exact', -1)
`, minuteAt)

	expectConstraintError("negative_group_id", `
INSERT INTO ops_cache_minute_stats (minute_at, platform, model, group_id, cache_type)
VALUES ($1, 'openai', 'gpt-5.5', -1, 'exact')
`, minuteAt)
}
