//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration148CreatesOpsCacheClearAuditsAndIsIdempotent(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	content, err := migrations.FS.ReadFile("148_ops_cache_clear_audits.sql")
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "first migration execution should succeed")
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "second migration execution should be idempotent")

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_cache_clear_audits (operator_user_id, clear_type, scope, matched_keys, deleted_keys, status, error_message)
VALUES (42, 'by_model', '{"platform":"openai","model":"gpt-5.5"}'::jsonb, 100, 90, 'partial_success', '10 keys failed')
`)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_cache_clear_audits (clear_type, status)
VALUES ('expired', 'success')
`)
	require.NoError(t, err, "insert with defaults should succeed")

	var rowCount int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_cache_clear_audits").Scan(&rowCount))
	require.Equal(t, 2, rowCount, "existing rows must be preserved after migration")

	var matchedKeys int64
	var deletedKeys int64
	var scope string
	var createdAt time.Time
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT matched_keys, deleted_keys, scope::text, created_at
FROM ops_cache_clear_audits
WHERE operator_user_id = 42
`).Scan(&matchedKeys, &deletedKeys, &scope, &createdAt))
	require.Equal(t, int64(100), matchedKeys)
	require.Equal(t, int64(90), deletedKeys)
	require.Contains(t, scope, "gpt-5.5")
	require.False(t, createdAt.IsZero())

	var defaultMatchedKeys int64
	var defaultDeletedKeys int64
	var defaultScope string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT matched_keys, deleted_keys, scope::text
FROM ops_cache_clear_audits
WHERE clear_type = 'expired'
`).Scan(&defaultMatchedKeys, &defaultDeletedKeys, &defaultScope))
	require.Equal(t, int64(0), defaultMatchedKeys)
	require.Equal(t, int64(0), defaultDeletedKeys)
	require.Equal(t, "{}", defaultScope)

	expectConstraintError := func(name string, query string) {
		t.Helper()
		_, err = tx.ExecContext(ctx, "SAVEPOINT "+name)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, query)
		require.Error(t, err)
		_, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name)
		require.NoError(t, rollbackErr)
		_, err = tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name)
		require.NoError(t, err)
	}

	expectConstraintError("invalid_clear_type", `
INSERT INTO ops_cache_clear_audits (clear_type, status)
VALUES ('namespace', 'success')
`)

	expectConstraintError("invalid_status", `
INSERT INTO ops_cache_clear_audits (clear_type, status)
VALUES ('all', 'unknown')
`)

	expectConstraintError("negative_deleted_keys", `
INSERT INTO ops_cache_clear_audits (clear_type, status, deleted_keys)
VALUES ('all', 'success', -1)
`)

	expectConstraintError("deleted_exceeds_matched", `
INSERT INTO ops_cache_clear_audits (clear_type, status, matched_keys, deleted_keys)
VALUES ('all', 'success', 1, 2)
`)
}
