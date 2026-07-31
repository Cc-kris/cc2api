package migrations

import (
	"strings"
	"testing"
)

func TestFinanceFXRateNoOverlapMigrationAddsDatabaseGuard(t *testing.T) {
	content, err := FS.ReadFile("202_finance_fx_rate_no_overlap.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(content))
	for _, required := range []string{
		"create or replace function finance_fx_rate_reject_overlap",
		"tstzrange",
		"raise exception",
		"create trigger finance_fx_rate_no_overlap",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("FX overlap migration missing %q", required)
		}
	}
}
