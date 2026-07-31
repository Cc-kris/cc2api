package migrations

import (
	"strings"
	"testing"
)

func TestFinanceSettlementProfileAndBillingTimeMigrationFreezesBothBoundaries(t *testing.T) {
	content, err := FS.ReadFile("200_finance_settlement_profile_and_billing_time.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(content))
	for _, required := range []string{
		"account_finance_counter_snapshots",
		"account_finance_profile_id",
		"usage_upstream_attempts",
		"billing_observed_at",
		"completed_at",
		"upstream_cost_settlement_intervals",
		"is distinct from current_snapshot.account_finance_profile_id",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing settlement boundary %q", required)
		}
	}
}
