package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestFinanceSettlementMigrationKeepsRechargeOutsideRequestCost(t *testing.T) {
	payload, err := os.ReadFile("190_finance_settlement_intervals.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(payload))
	for _, required := range []string{"upstream_cost_settlement_intervals", "usage_cost_settlement_allocations", "previous_snapshot_id", "current_snapshot_id", "allocation_rate", "invalidated_at"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("settlement migration missing %s", required)
		}
	}
	for _, forbidden := range []string{"upstream_fund_events", "base_recharge_ratio", "bonus_credit_units"} {
		if strings.Contains(sql, "references "+forbidden) || strings.Contains(sql, "join "+forbidden) {
			t.Fatalf("settlement migration must not couple request cost to recharge fact %s", forbidden)
		}
	}
}
