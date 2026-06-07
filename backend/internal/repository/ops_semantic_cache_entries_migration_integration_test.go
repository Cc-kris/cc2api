//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration149CreatesOpsSemanticCacheEntriesAndIsIdempotent(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	content, err := migrations.FS.ReadFile("149_ops_semantic_cache_entries.sql")
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "first migration execution should succeed")
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "second migration execution should be idempotent")

	expiresAt := time.Date(2026, 6, 7, 12, 30, 0, 0, time.UTC)
	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_semantic_cache_entries (
    namespace, platform, model, api_key_id, user_id, group_id,
    system_fingerprint, rule_version, embedding_model, embedding_dimension,
    embedding_ref, normalized_prompt_hash, response_cache_key, status, expires_at
) VALUES (
    'api:12:user:7:group:3:openai:gpt-5.5:sys-a:semantic-v1', 'openai', 'gpt-5.5', 12, 7, 3,
    'sys-a', 'semantic-v1', 'text-embedding-3-large', 3072,
    '{"provider":"pgvector","id":"vec-1"}'::jsonb, 'prompt-hash-1', 'exact-cache-key-1', 'active', $1
)`, expiresAt)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_semantic_cache_entries (
    namespace, platform, model, system_fingerprint, rule_version,
    embedding_model, embedding_dimension, normalized_prompt_hash, response_cache_key, expires_at
) VALUES (
    'api:13:user:8:group:4:gemini:gemini-2.5-pro:sys-b:semantic-v1', 'gemini', 'gemini-2.5-pro', 'sys-b', 'semantic-v1',
    'text-embedding-004', 768, 'prompt-hash-1', 'exact-cache-key-2', $1
)`, expiresAt)
	require.NoError(t, err, "same prompt hash under a different namespace should be allowed")

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_semantic_cache_entries (
    namespace, platform, model, system_fingerprint, rule_version,
    embedding_model, embedding_dimension, normalized_prompt_hash, response_cache_key, status, expires_at
) VALUES (
    'api:12:user:7:group:3:openai:gpt-5.5:sys-a:semantic-v1', 'openai', 'gpt-5.5', 'sys-a', 'semantic-v1',
    'text-embedding-3-large', 3072, 'prompt-hash-1', 'exact-cache-key-expired', 'expired', $1
)`, expiresAt)
	require.NoError(t, err, "historical non-active duplicate should be allowed")

	var rowCount int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_semantic_cache_entries").Scan(&rowCount))
	require.Equal(t, 3, rowCount, "existing rows must be preserved after migration")

	var embeddingDimension int
	var embeddingRef string
	var status string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT embedding_dimension, embedding_ref::text, status
FROM ops_semantic_cache_entries
WHERE response_cache_key = 'exact-cache-key-1'
`).Scan(&embeddingDimension, &embeddingRef, &status))
	require.Equal(t, 3072, embeddingDimension)
	require.Contains(t, embeddingRef, "vec-1")
	require.Equal(t, "active", status)

	var defaultEmbeddingRef string
	var defaultStatus string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT embedding_ref::text, status
FROM ops_semantic_cache_entries
WHERE response_cache_key = 'exact-cache-key-2'
`).Scan(&defaultEmbeddingRef, &defaultStatus))
	require.Equal(t, "{}", defaultEmbeddingRef)
	require.Equal(t, "active", defaultStatus)

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

	expectConstraintError("duplicate_active_entry", `
INSERT INTO ops_semantic_cache_entries (
    namespace, platform, model, system_fingerprint, rule_version,
    embedding_model, embedding_dimension, normalized_prompt_hash, response_cache_key, expires_at
) VALUES (
    'api:12:user:7:group:3:openai:gpt-5.5:sys-a:semantic-v1', 'openai', 'gpt-5.5', 'sys-a', 'semantic-v1',
    'text-embedding-3-large', 3072, 'prompt-hash-1', 'exact-cache-key-duplicate', $1
)`, expiresAt)

	expectConstraintError("invalid_status", `
INSERT INTO ops_semantic_cache_entries (
    namespace, platform, model, system_fingerprint, rule_version,
    embedding_model, embedding_dimension, normalized_prompt_hash, response_cache_key, status, expires_at
) VALUES (
    'ns-invalid-status', 'openai', 'gpt-5.5', 'sys-a', 'semantic-v1',
    'text-embedding-3-large', 3072, 'prompt-hash-invalid-status', 'cache-invalid-status', 'unknown', $1
)`, expiresAt)

	expectConstraintError("negative_dimension", `
INSERT INTO ops_semantic_cache_entries (
    namespace, platform, model, system_fingerprint, rule_version,
    embedding_model, embedding_dimension, normalized_prompt_hash, response_cache_key, expires_at
) VALUES (
    'ns-negative-dimension', 'openai', 'gpt-5.5', 'sys-a', 'semantic-v1',
    'text-embedding-3-large', -1, 'prompt-hash-negative-dimension', 'cache-negative-dimension', $1
)`, expiresAt)

	expectConstraintError("negative_api_key_id", `
INSERT INTO ops_semantic_cache_entries (
    namespace, platform, model, api_key_id, system_fingerprint, rule_version,
    embedding_model, embedding_dimension, normalized_prompt_hash, response_cache_key, expires_at
) VALUES (
    'ns-negative-api-key', 'openai', 'gpt-5.5', -1, 'sys-a', 'semantic-v1',
    'text-embedding-3-large', 3072, 'prompt-hash-negative-api-key', 'cache-negative-api-key', $1
)`, expiresAt)

	expectConstraintError("empty_namespace", `
INSERT INTO ops_semantic_cache_entries (
    namespace, platform, model, system_fingerprint, rule_version,
    embedding_model, embedding_dimension, normalized_prompt_hash, response_cache_key, expires_at
) VALUES (
    '   ', 'openai', 'gpt-5.5', 'sys-a', 'semantic-v1',
    'text-embedding-3-large', 3072, 'prompt-hash-empty-namespace', 'cache-empty-namespace', $1
)`, expiresAt)
}
