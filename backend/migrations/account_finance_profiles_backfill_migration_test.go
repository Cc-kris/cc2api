package migrations

import (
	"strings"
	"testing"
)

func TestAccountFinanceProfilesBackfillKeepsUnconfiguredMultiplierAccountsContractValid(t *testing.T) {
	content, err := FS.ReadFile("197_backfill_account_finance_profiles.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(content))
	for _, required := range []string{
		"case when a.upstream_cost_multiplier is null then null else 'multiplier' end",
		"case when a.upstream_cost_multiplier is null then null else change.id end",
		"a.upstream_cost_multiplier",
		"'unconfigured'",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("account finance profile backfill missing null-safe contract projection %q", required)
		}
	}
	if strings.Contains(sql, "change.id,a.upstream_cost_multiplier,'multiplier',a.upstream_cost_multiplier,change.id") {
		t.Fatal("backfill must not assign contract_type=multiplier when the account multiplier is NULL")
	}
}
