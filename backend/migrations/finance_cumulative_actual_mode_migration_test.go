package migrations

import (
	"strings"
	"testing"
)

func TestFinanceCumulativeActualMigrationAllowsSettlementReadySnapshots(t *testing.T) {
	content, err := FS.ReadFile("195_finance_cumulative_actual_mode.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(content))
	for _, required := range []string{
		"drop constraint if exists account_finance_counter_derivation_check",
		"add constraint account_finance_counter_derivation_check",
		"'settlement_ready'",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("cumulative actual migration missing derivation constraint clause %q", required)
		}
	}
}
