package migrations

import (
	"regexp"
	"strings"
	"testing"
)

// V03-R11：230 只加可空、无默认的列，不改写大表。
func TestUsageLogAuditFieldsMigration(t *testing.T) {
	content, err := FS.ReadFile("230_usage_log_audit_fields.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(content))
	for _, required := range []string{
		"add column if not exists upstream_response_model varchar(200)",
		"add column if not exists upstream_model_mismatch boolean",
		"add column if not exists session_id varchar(255)",
		"add column if not exists requested_reasoning_effort varchar(20)",
		"add column if not exists upstream_request_id varchar(128)",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"default", "not null", "update ", "batch_image_jobs", "create index", "concurrently"} {
		if strings.Contains(stripSQLComments(sql), forbidden) {
			t.Fatalf("230 must only add nullable columns without defaults, found %q", forbidden)
		}
	}
}

// 231、232：每个文件只有一条 CREATE INDEX CONCURRENTLY IF NOT EXISTS，没有事务控制语句。
func TestUsageLogAuditIndexMigrationsAreSingleConcurrentStatements(t *testing.T) {
	cases := map[string]string{
		"231_usage_log_upstream_request_id_index_notx.sql":     "idx_usage_logs_upstream_request_id",
		"232_usage_log_upstream_model_mismatch_index_notx.sql": "idx_usage_logs_upstream_model_mismatch_created_at",
	}
	txControl := regexp.MustCompile(`(?i)\b(begin|commit|rollback)\b`)
	for name, index := range cases {
		content, err := FS.ReadFile(name)
		if err != nil {
			if strings.HasPrefix(name, "231_") {
				t.Logf("%s not present yet (added with V03-T17)", name)
				continue
			}
			t.Fatalf("read %s: %v", name, err)
		}
		sql := strings.ToLower(stripSQLComments(string(content)))
		if n := strings.Count(sql, "create index concurrently if not exists"); n != 1 {
			t.Fatalf("%s must contain exactly one CREATE INDEX CONCURRENTLY IF NOT EXISTS, got %d", name, n)
		}
		if strings.Count(sql, ";") != 1 {
			t.Fatalf("%s must contain a single statement", name)
		}
		if txControl.MatchString(sql) {
			t.Fatalf("%s must not contain BEGIN/COMMIT/ROLLBACK", name)
		}
		if !strings.Contains(sql, index) {
			t.Fatalf("%s must create %s", name, index)
		}
	}
}

func stripSQLComments(sql string) string {
	lines := strings.Split(sql, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
