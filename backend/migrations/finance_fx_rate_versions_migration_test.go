package migrations

import (
	"strings"
	"testing"
)

func TestFinanceFXRateVersionsMigrationFreezesHistoricalEvidence(t *testing.T) {
	content, err := FS.ReadFile("199_finance_fx_rate_versions.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(content))
	for _, required := range []string{
		"finance_fx_rate_versions",
		"rate_to_usd decimal(20,10) not null",
		"fx_rate_version_id bigint references finance_fx_rate_versions(id) on delete restrict",
		"alter table upstream_fund_events",
		"alter table usage_finance_records",
		"alter table usage_finance_cost_segments",
		"alter table upstream_cost_settlement_intervals",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("FX migration missing %q", required)
		}
	}
}
