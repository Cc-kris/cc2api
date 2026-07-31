package migrations

import (
	"strings"
	"testing"
)

func TestRequestFinanceVersionEvidenceMigrationDoesNotFabricateHistory(t *testing.T) {
	content, err := FS.ReadFile("198_request_finance_version_evidence.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(content))
	for _, required := range []string{
		"upstream_cost_multiplier_change_id",
		"current_finance_profile_id",
		"usage_logs",
		"usage_upstream_attempts",
		"usage_finance_records",
		"usage_finance_cost_segments",
		"account_finance_profile_id",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("request finance evidence migration missing %q", required)
		}
	}
	if strings.Contains(sql, "update usage_logs") || strings.Contains(sql, "update usage_upstream_attempts") {
		t.Fatal("historical request evidence must remain unknown instead of being backfilled from current account state")
	}
}
