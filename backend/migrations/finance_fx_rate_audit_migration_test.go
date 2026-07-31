package migrations

import (
	"strings"
	"testing"
)

func TestFinanceFXRateAuditMigrationAddsTraceabilityAndIdempotency(t *testing.T) {
	content, err := FS.ReadFile("203_finance_fx_rate_audit.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(content))
	for _, required := range []string{"operator_id", "change_reason", "idempotency_key", "unique index"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("FX audit migration missing %q", required)
		}
	}
}
