package migrations

import (
	"strings"
	"testing"
)

func TestAccountFinanceCounterSnapshotsMigrationPreservesMultiplierBoundaries(t *testing.T) {
	content, err := FS.ReadFile("187_account_finance_counter_snapshots.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(content)
	for _, required := range []string{
		"account_finance_counter_snapshots",
		"unit_semantics IN ('fiat_currency','platform_credit')",
		"idx_account_finance_counter_snapshot_idempotency",
		"multiplier_change_id BIGINT REFERENCES account_upstream_multiplier_changes",
		"platform_credit rows are evidence only",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"upstream_fund_events", "recharge_ratio", "bonus_credit"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("multiplier snapshot migration must not depend on recharge facts: %s", forbidden)
		}
	}
}
