//go:build unit

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSchedulerThresholdMetadataWhitelist(t *testing.T) {
	extra := map[string]any{}
	for _, key := range []string{"codex_usage_updated_at", "codex_5h_used_percent", "codex_7d_used_percent", "codex_5h_reset_at", "codex_7d_reset_after_seconds", "session_window_utilization", "passive_usage_7d_utilization", "passive_usage_7d_reset", "passive_usage_7d_oi_utilization", "passive_usage_7d_oi_reset", "email", "workspace_id", "grok_sched_utilization", "grok_sched_reset_at"} {
		extra[key] = "keep"
	}
	extra["unused_large_field"] = "drop"
	account := service.Account{Extra: extra, Credentials: map[string]any{"account_scheduling_threshold": 90, "email": "identity", "workspace_id": "workspace", "plan_type": "plus", "refresh_token": "secret"}}
	result := buildSchedulerMetadataAccount(account)
	for key := range extra {
		if key != "unused_large_field" {
			require.Equal(t, "keep", result.Extra[key], key)
		}
	}
	require.NotContains(t, result.Extra, "unused_large_field")
	require.Equal(t, 90, result.Credentials["account_scheduling_threshold"])
	require.Equal(t, "identity", result.Credentials["email"])
	require.Equal(t, "plus", result.Credentials["plan_type"])
	require.NotContains(t, result.Credentials, "refresh_token")
}
