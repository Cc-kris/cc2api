//go:build unit

package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type thresholdRuntimeRepo struct {
	AccountRepository
	writes      int
	reason      string
	fail        bool
	modelWrites int
}

func (r *thresholdRuntimeRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, reason string) error {
	r.writes++
	r.reason = reason
	if r.fail {
		return errors.New("db down")
	}
	return nil
}
func (r *thresholdRuntimeRepo) SetModelRateLimit(_ context.Context, _ int64, _ string, _ time.Time) error {
	r.modelWrites++
	return nil
}

func TestThresholdRuntimePersistsWithoutMutatingSharedSnapshot(t *testing.T) {
	repo := &thresholdRuntimeRepo{}
	settings := &SettingService{settingRepo: &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{SettingKeyAccountSchedulingThresholds: `{"openai":90}`}}}
	rl := &RateLimitService{accountRepo: repo, settingService: settings}
	until := time.Now().Add(time.Hour)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Extra: map[string]any{"codex_5h_used_percent": 95.0, "codex_5h_reset_at": until.Format(time.RFC3339)}}
	require.True(t, rl.ApplyAccountSchedulingThreshold(context.Background(), account))
	require.Equal(t, 1, repo.writes)
	require.Nil(t, account.TempUnschedulableUntil)
	require.Empty(t, account.TempUnschedulableReason)
	require.True(t, IsAccountSchedulingThresholdReason(repo.reason))
	account.Extra["codex_5h_reset_at"] = time.Now().Add(-time.Minute).Format(time.RFC3339)
	require.False(t, rl.ApplyAccountSchedulingThreshold(context.Background(), account))
	repo.fail = true
	account.Extra["codex_5h_reset_at"] = until.Format(time.RFC3339)
	require.True(t, rl.ApplyAccountSchedulingThreshold(context.Background(), account))
	repo.fail = false
	require.True(t, rl.ApplyAccountSchedulingThreshold(context.Background(), account))
	require.Equal(t, 3, repo.writes)
}

func TestThresholdRuntimeDisabledAndModelScope(t *testing.T) {
	repo := &thresholdRuntimeRepo{}
	settings := &SettingService{settingRepo: &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{}}}
	rl := &RateLimitService{accountRepo: repo, settingService: settings}
	until := time.Now().Add(time.Hour).Format(time.RFC3339)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Extra: map[string]any{"codex_5h_used_percent": 99.0, "codex_5h_reset_at": until}}
	require.False(t, rl.ApplyAccountSchedulingThreshold(context.Background(), account))
	require.Zero(t, repo.writes)
	settings = &SettingService{settingRepo: &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{SettingKeyAccountSchedulingThresholds: `{"anthropic":90}`}}}
	rl.settingService = settings
	account.Platform = PlatformAnthropic
	account.Extra = map[string]any{"passive_usage_7d_oi_utilization": .95, "passive_usage_7d_oi_reset": until}
	require.False(t, rl.ApplyAccountSchedulingThreshold(context.Background(), account))
	require.False(t, rl.ApplyModelSchedulingThreshold(context.Background(), account, "claude-sonnet-4-6"))
	require.True(t, rl.ApplyModelSchedulingThreshold(context.Background(), account, "claude-fable-5-1"))
	require.Equal(t, 1, repo.modelWrites)
}
