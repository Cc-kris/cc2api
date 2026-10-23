//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestContentCenterMigrationSchema(t *testing.T) {
	tx := testTx(t)

	for _, table := range []string{
		"content_news_sources",
		"content_news_items",
		"content_articles",
		"content_translations",
		"content_automation_settings",
		"content_translation_settings",
		"content_fetch_runs",
		"content_analytics_events",
		"content_analytics_daily",
		"content_slug_redirects",
		"content_audit_logs",
		"content_writing_tasks",
		"content_news_quota_reservations",
	} {
		requireTable(t, tx, table)
	}

	requireColumn(t, tx, "content_news_items", "source_canonical_url", "text", 0, false)
	requireColumn(t, tx, "content_news_items", "body_html", "text", 0, true)
	requireColumn(t, tx, "content_news_items", "body_text", "text", 0, true)
	requireColumn(t, tx, "content_news_items", "body_hash", "character varying", 64, true)
	requireColumn(t, tx, "content_news_sources", "mapping_json", "jsonb", 0, false)
	requireColumn(t, tx, "content_analytics_daily", "date", "date", 0, false)
	requireColumn(t, tx, "content_analytics_events", "payload_json", "jsonb", 0, false)
	requireColumn(t, tx, "content_writing_tasks", "lease_until", "timestamp with time zone", 0, true)
	requireColumn(t, tx, "content_writing_tasks", "version", "bigint", 0, false)
	requireColumn(t, tx, "content_news_quota_reservations", "reserved_count", "integer", 0, false)
	requireConstraintDefinitionContains(t, tx, "content_news_quota_reservations", "content_news_quota_reservations_quota_kind_check", "fetch", "publish")

	requireConstraintDefinitionContains(t, tx, "content_news_sources", "content_news_sources_url_check", "source_type", "btrim")
	requireConstraintDefinitionContains(t, tx, "content_news_sources", "content_news_sources_enabled_requirements_check", "terms_confirmed_at", "authorization_note", "robots_policy")
	requireConstraintDefinitionContains(t, tx, "content_news_items", "content_news_items_published_fields_check", "body_html", "body_text", "source_name", "published_at")
	requireConstraintDefinitionContains(t, tx, "content_fetch_runs", "content_fetch_runs_trigger_check", "cron", "manual")
	requireConstraintDefinitionContains(t, tx, "content_fetch_runs", "content_fetch_runs_status_check", "succeeded", "interrupted")
	requireConstraintDefinitionContains(t, tx, "content_analytics_events", "content_analytics_events_payload_check", "content_analytics_payload_is_allowed")
	requireConstraintDefinitionContains(t, tx, "content_writing_tasks", "content_writing_tasks_kind_check", "writing", "translation")
	requireConstraintDefinitionContains(t, tx, "content_writing_tasks", "content_writing_tasks_status_check", "queued", "cancelled")
	requireIndex(t, tx, "content_news_sources", "content_news_sources_category_idx")
	requireIndex(t, tx, "content_news_items", "content_news_items_deleted_at_idx")
	requireIndex(t, tx, "content_writing_tasks", "content_writing_tasks_claim_idx")

	for _, fk := range []struct {
		table     string
		column    string
		reference string
	}{
		{table: "content_news_sources", column: "created_by", reference: "users"},
		{table: "content_news_sources", column: "updated_by", reference: "users"},
		{table: "content_news_items", column: "created_by", reference: "users"},
		{table: "content_news_items", column: "updated_by", reference: "users"},
		{table: "content_articles", column: "author_id", reference: "users"},
		{table: "content_translations", column: "reviewed_by", reference: "users"},
		{table: "content_audit_logs", column: "actor_id", reference: "users"},
	} {
		requireForeignKeyOnDelete(t, tx, fk.table, fk.column, fk.reference, "SET NULL")
	}
	requireForeignKeyOnDelete(t, tx, "content_news_items", "source_id", "content_news_sources", "NO ACTION")
	requireForeignKeyOnDelete(t, tx, "content_fetch_runs", "source_id", "content_news_sources", "NO ACTION")

	requirePartialUniqueIndexDefinition(t, tx, "content_news_items", "content_news_items_locale_slug_uq", "locale", "slug")
	requirePartialUniqueIndexDefinition(t, tx, "content_news_items", "content_news_items_source_external_uq", "source_id", "external_id", "external_id IS NOT NULL", "deleted_at IS NULL")
	requirePartialUniqueIndexDefinition(t, tx, "content_articles", "content_articles_locale_slug_uq", "locale", "slug")
	requirePartialUniqueIndexDefinition(t, tx, "content_translations", "content_translations_identity_uq", "content_type", "source_version")

	for _, table := range []string{"content_automation_settings", "content_translation_settings"} {
		var count int
		require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table+" WHERE id = 1").Scan(&count))
		require.Equal(t, 1, count, "expected singleton row in %s", table)
	}

	var homeMode string
	require.NoError(t, tx.QueryRowContext(context.Background(), "SELECT value FROM settings WHERE key = 'home_mode'").Scan(&homeMode))
	require.Contains(t, []string{"structured", "legacy_custom"}, homeMode)

	// The runner applies every migration before tests and the runner's shared
	// idempotency test re-applies them; this assertion keeps the content center
	// migration covered by the same contract without mutating test data.
	require.NoError(t, ApplyMigrations(context.Background(), integrationDB))
}

func TestContentCenterMigrationBehavior(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	requireSQLRejected(t, tx, `
INSERT INTO content_news_sources (name, source_type, url)
VALUES ('invalid url', 'rss', 'javascript:alert(1)')
`)

	var sourceID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO content_news_sources (name, source_type, url)
VALUES ('manual test source', 'manual', NULL)
RETURNING id
`).Scan(&sourceID))

	requireSQLRejected(t, tx, `
INSERT INTO content_news_items (
    source_id, source_canonical_url, slug, title, status, published_at
) VALUES ($1, 'https://example.com/news', 'missing-body', 'Missing body', 'published', NOW())
`, sourceID)
	requireSQLRejected(t, tx, `
INSERT INTO content_news_items (
    source_id, source_canonical_url, slug, title, body_source_type
) VALUES ($1, 'https://example.com/news', 'bad-source-type', 'Bad source type', 'scraped_anyhow')
`, sourceID)

	requireSQLRejected(t, tx, `
INSERT INTO content_analytics_events (
    event_id, event_name, page_path, session_hash, event_at, payload_json
) VALUES ('invalid-payload-key', 'page_view', '/', 'session', NOW(), '{"api_key":"secret"}'::jsonb)
`)
	requireSQLRejected(t, tx, `
INSERT INTO content_writing_tasks (task_id, kind, topic, model, status, word_count)
VALUES ('bad-writing-status', 'writing', 'topic', 'model', 'unknown', 1)
`)
	requireSQLRejected(t, tx, `
INSERT INTO content_writing_tasks (task_id, kind, topic, model, status, word_count)
VALUES ('bad-writing-count', 'writing', 'topic', 'model', 'queued', -1)
`)

	var taskVersion int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO content_writing_tasks (task_id, kind, topic, model, status, version)
VALUES ('durable-writing-task', 'writing', 'topic', 'model', 'queued', 1)
RETURNING version
`).Scan(&taskVersion))
	require.Equal(t, int64(1), taskVersion)
	require.NoError(t, tx.QueryRowContext(ctx, `
UPDATE content_writing_tasks
SET status = 'running', lease_until = NOW() + INTERVAL '5 minutes', version = version + 1
WHERE task_id = 'durable-writing-task' AND version = 1
RETURNING version
`).Scan(&taskVersion))
	require.Equal(t, int64(2), taskVersion)

	requireSQLRejected(t, tx, `
INSERT INTO content_analytics_events (
    event_id, event_name, page_path, session_hash, event_at, payload_json
) VALUES ('invalid-payload-nested', 'content_read', '/articles/test', 'session', NOW(), '{"section":{"prompt":"secret"}}'::jsonb)
`)
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO content_analytics_events (
    event_id, event_name, page_path, session_hash, event_at, payload_json
) VALUES ('valid-payload', 'content_read', '/articles/test', 'session', NOW(), '{"read_seconds":12,"section":"intro"}'::jsonb)
RETURNING id
`).Scan(new(int64)))

	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO content_articles (locale, slug, title, deleted_at)
VALUES ('zh-CN', 'reserved-after-delete', 'Old article', NOW())
RETURNING id
`).Scan(new(int64)))
	requireSQLRejected(t, tx, `
INSERT INTO content_articles (locale, slug, title)
VALUES ('zh-CN', 'reserved-after-delete', 'Replacement article')
`)

	_, err := tx.ExecContext(ctx, "DELETE FROM settings WHERE key IN ('home_mode', 'home_content')")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "INSERT INTO settings (key, value) VALUES ('home_content', '<section>legacy</section>')")
	require.NoError(t, err)
	homeModeMigration, err := migrations.FS.ReadFile("214_home_mode.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(homeModeMigration))
	require.NoError(t, err)
	var homeMode string
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'home_mode'").Scan(&homeMode))
	require.Equal(t, "legacy_custom", homeMode)
	var legacyContent string
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'home_content'").Scan(&legacyContent))
	require.Equal(t, "<section>legacy</section>", legacyContent)

	var localDate time.Time
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT ('2026-09-08 16:30:00+00'::timestamptz AT TIME ZONE 'Asia/Shanghai')::date
`).Scan(&localDate))
	require.Equal(t, "2026-09-09", localDate.Format("2006-01-02"))
}

func requireSQLRejected(t *testing.T, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, query string, args ...any) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, execSQL(tx, ctx, "SAVEPOINT content_constraint_probe"))
	_, err := tx.ExecContext(ctx, query, args...)
	require.Error(t, err)
	require.NoError(t, execSQL(tx, ctx, "ROLLBACK TO SAVEPOINT content_constraint_probe"))
	require.NoError(t, execSQL(tx, ctx, "RELEASE SAVEPOINT content_constraint_probe"))
}

func execSQL(tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, ctx context.Context, query string) error {
	_, err := tx.ExecContext(ctx, query)
	return err
}
