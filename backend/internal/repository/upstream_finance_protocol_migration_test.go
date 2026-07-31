package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestUpstreamFinanceProtocolMigrationContainsImmutableVersionConstraints(t *testing.T) {
	payload, err := migrations.FS.ReadFile("179_upstream_finance_protocols.sql")
	require.NoError(t, err)
	sql := string(payload)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS upstream_finance_protocols",
		"CREATE TABLE IF NOT EXISTS upstream_finance_protocol_versions",
		"UNIQUE (protocol_id, version)",
		"CHECK (protocol_type IN ('builtin','http_json','plugin'))",
		"CHECK (status IN ('draft','published','disabled'))",
	} {
		require.Truef(t, strings.Contains(sql, fragment), "migration missing %q", fragment)
	}
}
