package migrations

import (
	"strings"
	"testing"
)

func TestContentWritingTasksMigrationDefinesDurableQueueContract(t *testing.T) {
	sql, err := FS.ReadFile("216_content_writing_tasks.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(sql)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS content_writing_tasks",
		"task_id VARCHAR(96) PRIMARY KEY",
		"lease_until TIMESTAMPTZ",
		"version BIGINT NOT NULL DEFAULT 1",
		"content_writing_tasks_claim_idx",
		"FOREIGN KEY (article_id)",
		"FOREIGN KEY (translation_id)",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
