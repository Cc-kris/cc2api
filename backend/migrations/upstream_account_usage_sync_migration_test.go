package migrations

import (
	"strings"
	"testing"
)

func TestUpstreamAccountUsageSyncMigrationPreservesFundingAndExcludesRechargeInputs(t *testing.T) {
	content, err := FS.ReadFile("189_upstream_account_usage_sync.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(content)
	for _, required := range []string{"'funding'", "'account_usage'", "DROP CONSTRAINT IF EXISTS upstream_finance_sync_runs_type_check"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"upstream_fund_events", "recharge_ratio", "bonus_credit_units"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("account usage sync migration must not read funding facts: %s", forbidden)
		}
	}
}
