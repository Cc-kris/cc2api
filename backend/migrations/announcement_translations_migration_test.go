package migrations

import (
	"strings"
	"testing"
)

func TestAnnouncementTranslationsMigration(t *testing.T) {
	content, err := FS.ReadFile("204_announcement_translations.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(content))
	for _, required := range []string{"source_locale", "source_version", "translations", "status", "updated_at", "jsonb_build_object"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("announcement translation migration missing %q", required)
		}
	}
}
