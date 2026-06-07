//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMigration150CreatesOpsSemanticCacheAuditsAndIsIdempotent(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	entryContent, err := migrations.FS.ReadFile("149_ops_semantic_cache_entries.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(entryContent))
	require.NoError(t, err, "semantic entries migration should succeed before audits migration")

	content, err := migrations.FS.ReadFile("150_ops_semantic_cache_audits.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "first migration execution should succeed")
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "second migration execution should be idempotent")

	expiresAt := time.Date(2026, 6, 7, 12, 30, 0, 0, time.UTC)
	occurredAt := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	var entryID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO ops_semantic_cache_entries (
    namespace, platform, model, api_key_id, user_id, group_id,
    system_fingerprint, rule_version, embedding_model, embedding_dimension,
    normalized_prompt_hash, response_cache_key, expires_at
) VALUES (
    'api:12:user:7:group:3:openai:gpt-5.5:sys-a:semantic-v1', 'openai', 'gpt-5.5', 12, 7, 3,
    'sys-a', 'semantic-v1', 'text-embedding-3-large', 3072,
    'prompt-hash-1', 'exact-cache-key-1', $1
) RETURNING id
`, expiresAt).Scan(&entryID))

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_semantic_cache_audits (
    request_id, semantic_entry_id, occurred_at, platform, model, api_key_id,
    similarity, decision, block_reason, review_status, feedback_type, feedback_note,
    operator_user_id, auto_close_reason, source_summary, target_summary
) VALUES (
    'req-1', $1, $2, 'openai', 'gpt-5.5', 12,
    0.985000, 'hit', NULL, 'reusable', 'manual_mark', '可复用',
    9, NULL, 'source summary', 'target summary'
)
`, entryID, occurredAt)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `
INSERT INTO ops_semantic_cache_audits (request_id, platform, model, decision)
VALUES ('req-2', 'gemini', 'gemini-2.5-pro', 'miss')
`)
	require.NoError(t, err, "audit without candidate entry should support miss decisions")

	var rowCount int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_semantic_cache_audits").Scan(&rowCount))
	require.Equal(t, 2, rowCount, "existing audit rows must be preserved after migration")

	var similarity string
	var reviewStatus string
	var feedbackType string
	var sourceSummary string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT similarity::text, review_status, feedback_type, source_summary
FROM ops_semantic_cache_audits
WHERE request_id = 'req-1'
`).Scan(&similarity, &reviewStatus, &feedbackType, &sourceSummary))
	require.Equal(t, "0.985000", similarity)
	require.Equal(t, "reusable", reviewStatus)
	require.Equal(t, "manual_mark", feedbackType)
	require.Equal(t, "source summary", sourceSummary)

	var semanticEntryIsNull bool
	var defaultSimilarity string
	var defaultReviewStatus string
	var defaultFeedbackType string
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT semantic_entry_id IS NULL, similarity::text, review_status, feedback_type
FROM ops_semantic_cache_audits
WHERE request_id = 'req-2'
`).Scan(&semanticEntryIsNull, &defaultSimilarity, &defaultReviewStatus, &defaultFeedbackType))
	require.True(t, semanticEntryIsNull)
	require.Equal(t, "0.000000", defaultSimilarity)
	require.Equal(t, "pending", defaultReviewStatus)
	require.Equal(t, "none", defaultFeedbackType)

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

	expectConstraintError("invalid_decision", `
INSERT INTO ops_semantic_cache_audits (request_id, platform, model, decision)
VALUES ('req-invalid-decision', 'openai', 'gpt-5.5', 'unknown')
`)

	expectConstraintError("invalid_review_status", `
INSERT INTO ops_semantic_cache_audits (request_id, platform, model, decision, review_status)
VALUES ('req-invalid-review', 'openai', 'gpt-5.5', 'hit', 'unknown')
`)

	expectConstraintError("invalid_feedback_type", `
INSERT INTO ops_semantic_cache_audits (request_id, platform, model, decision, feedback_type)
VALUES ('req-invalid-feedback', 'openai', 'gpt-5.5', 'hit', 'unknown')
`)

	expectConstraintError("similarity_too_high", `
INSERT INTO ops_semantic_cache_audits (request_id, platform, model, decision, similarity)
VALUES ('req-sim-high', 'openai', 'gpt-5.5', 'hit', 1.100000)
`)

	expectConstraintError("negative_api_key_id", `
INSERT INTO ops_semantic_cache_audits (request_id, platform, model, api_key_id, decision)
VALUES ('req-negative-api-key', 'openai', 'gpt-5.5', -1, 'hit')
`)

	expectConstraintError("empty_request_id", `
INSERT INTO ops_semantic_cache_audits (request_id, platform, model, decision)
VALUES ('   ', 'openai', 'gpt-5.5', 'hit')
`)
}
