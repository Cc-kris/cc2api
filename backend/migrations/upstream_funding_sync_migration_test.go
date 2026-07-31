package migrations

import (
	"strings"
	"testing"
)

func TestUpstreamFundingSyncMigrationOnlyExtendsSyncJobType(t *testing.T) {
	content, err := FS.ReadFile("188_upstream_funding_sync.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(content)
	for _, required := range []string{
		"upstream_finance_sync_runs_sync_type_check",
		"'funding'",
		"only appends upstream_fund_events",
		"never changes account",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"UPDATE accounts", "upstream_cost_multiplier", "recharge_ratio"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("funding sync migration must not couple recharge to multiplier: %s", forbidden)
		}
	}
}
