package migrations

import (
	"strings"
	"testing"
)

func TestContentV04MigrationDefinesSchemaContract(t *testing.T) {
	sql, err := FS.ReadFile("219_content_v1.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(sql)
	for _, fragment := range []string{
		"ALTER TABLE content_articles",
		"ADD COLUMN IF NOT EXISTS featured_at TIMESTAMPTZ NULL",
		"ADD COLUMN IF NOT EXISTS seo_keywords JSONB NULL",
		"content_articles_featured_idx",
		"ALTER COLUMN interval_minutes DROP NOT NULL",
		"ALTER COLUMN interval_minutes DROP DEFAULT",
		"CHECK (interval_minutes IS NULL OR interval_minutes >= 15)",
		"'page_leave'",
		"'heartbeat'",
		"'duration_ms'",
		"'view_id'",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestContentV04MigrationIsIdempotentAndDoesNotClaimBusinessBackfill(t *testing.T) {
	sql, err := FS.ReadFile("219_content_v1.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(sql)
	for _, fragment := range []string{
		"IF NOT EXISTS",
		"DROP CONSTRAINT IF EXISTS",
		"CREATE OR REPLACE FUNCTION content_analytics_payload_is_allowed",
		"post-write",
		"clear existing featured_at values",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("migration missing safety marker %q", fragment)
		}
	}
}
