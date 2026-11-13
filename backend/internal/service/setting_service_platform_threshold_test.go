//go:build unit

package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSchedulingThresholdSettingsPersistGetAndInvalidate(t *testing.T) {
	repo := &settingUpdateRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	require.Equal(t, 100, svc.GetAccountSchedulingThresholds(context.Background())[PlatformOpenAI])
	require.NoError(t, svc.UpdateSettings(context.Background(), &SystemSettings{AccountSchedulingThresholds: map[string]int{PlatformOpenAI: 90, PlatformAnthropic: 85}, OpenAISchedulerSettings: OpenAISchedulerSettings{LBTopK: "9", Load: "2", StickyWeighted: true}}))
	require.JSONEq(t, `{"openai":90,"anthropic":85,"grok":100}`, repo.updates[SettingKeyAccountSchedulingThresholds])
	require.Equal(t, 90, svc.GetAccountSchedulingThresholds(context.Background())[PlatformOpenAI])
	require.Equal(t, "9", repo.updates["openai_advanced_scheduler_lb_top_k"])
	require.Equal(t, "true", repo.updates["openai_advanced_scheduler_sticky_weighted_enabled"])
	repo.updates = nil
	for _, invalid := range []map[string]int{{PlatformOpenAI: 0}, {PlatformOpenAI: 101}, {"kiro": 90}} {
		err := svc.UpdateSettings(context.Background(), &SystemSettings{AccountSchedulingThresholds: invalid})
		require.Equal(t, "INVALID_SCHEDULING_THRESHOLD", infraerrors.Reason(err))
		require.Nil(t, repo.updates, "validation must precede all settings writes")
	}
	require.Equal(t, 90, svc.GetAccountSchedulingThresholds(context.Background())[PlatformOpenAI])
}
