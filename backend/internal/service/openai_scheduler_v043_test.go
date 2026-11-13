//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestToolCallOutputContextCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		complete    bool
	}{
		{"none", `[]`, false},
		{"full", `[{"type":"function_call","call_id":"a"},{"type":"function_call_output","call_id":"a"}]`, true},
		{"partial", `[{"type":"function_call","call_id":"a"},{"type":"function_call_output","call_id":"a"},{"type":"function_call_output","call_id":"b"}]`, false},
		{"missing", `[{"type":"function_call","call_id":"a"},{"type":"function_call_output"}]`, false},
		{"reference", `[{"type":"item_reference","id":"a"},{"type":"function_call_output","call_id":"a"}]`, true},
		{"mixed", `[{"type":"function_call","call_id":"a"},{"type":"item_reference","id":"b"},{"type":"function_call_output","call_id":"a"},{"type":"function_call_output","call_id":"b"}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"previous_response_id":"resp_1","input":` + tc.input + `}`)
			coverage := AnalyzeToolCallOutputContextCoverageBytes(body)
			require.Equal(t, tc.complete, coverage.ContextCoversAllCallIDs)
			stripped := RemovePreviousResponseIDFromBody(body)
			require.Equal(t, coverage.HasFunctionCallOutput && !tc.complete, gjson.GetBytes(stripped, "previous_response_id").Exists())
		})
	}
}

func TestQuotaHeadroomFreshnessAndNumbers(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{Extra: map[string]any{"codex_usage_updated_at": now.Format(time.RFC3339), "codex_7d_used_percent": 20.0}}
	require.InDelta(t, .8, openAIQuotaHeadroomFactor(account, now), .0001)
	account.Extra["codex_5h_used_percent"] = 95.0
	require.InDelta(t, .4, openAIQuotaHeadroomFactor(account, now), .0001)
	account.Extra["codex_usage_updated_at"] = now.Add(-9 * time.Hour).Format(time.RFC3339)
	require.Equal(t, .5, openAIQuotaHeadroomFactor(account, now))
	account.Extra["codex_usage_updated_at"] = now.Format(time.RFC3339)
	account.Extra["codex_7d_used_percent"] = "invalid"
	require.Equal(t, .5, openAIQuotaHeadroomFactor(account, now))
}

func TestSchedulerControlsRoundTripAndValidation(t *testing.T) {
	previous := OpenAISchedulerSettings{StickyWeighted: true, Priority: "2", LBTopK: "8"}
	var update OpenAISchedulerSettingsUpdate
	require.NoError(t, json.Unmarshal([]byte(`{"openai_advanced_scheduler_weight_priority":"0","openai_advanced_scheduler_subscription_priority_enabled":true}`), &update))
	merged := update.Merge(previous)
	require.True(t, merged.StickyWeighted)
	require.True(t, merged.SubscriptionPriority)
	require.Equal(t, "8", merged.LBTopK)
	values, err := merged.settingValues()
	require.NoError(t, err)
	require.Equal(t, merged, parseOpenAISchedulerSettings(values))
	for _, raw := range []string{"-1", "NaN", "Inf", "bad"} {
		invalid := OpenAISchedulerSettings{Load: raw}
		_, err = invalid.settingValues()
		require.Error(t, err)
	}
	_, err = (OpenAISchedulerSettings{LBTopK: "1.5"}).settingValues()
	require.Error(t, err)
}

func TestThresholdSettingsDefaultMissingAndValidation(t *testing.T) {
	repo := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{}}
	svc := &SettingService{settingRepo: repo}
	values := svc.GetAccountSchedulingThresholds(context.Background())
	require.Equal(t, 100, values[PlatformOpenAI])
	values[PlatformOpenAI] = 1
	require.Equal(t, 100, svc.GetAccountSchedulingThresholds(context.Background())[PlatformOpenAI])
	normalized, err := validateAndNormalizeAccountSchedulingThresholds(map[string]int{PlatformOpenAI: 90})
	require.NoError(t, err)
	require.Equal(t, 100, normalized[PlatformGrok])
	for _, input := range []map[string]int{{PlatformOpenAI: 0}, {PlatformOpenAI: 101}, {"kiro": 90}} {
		_, err = validateAndNormalizeAccountSchedulingThresholds(input)
		require.Error(t, err)
	}
	parsed, err := parseAccountSchedulingThresholdsSetting(`{"openai":90,"grok":20,"kiro":90}`)
	require.NoError(t, err)
	require.Equal(t, 90, parsed[PlatformOpenAI])
	require.NotContains(t, parsed, "kiro")
}

func TestFilterBySoonestReset(t *testing.T) {
	now := time.Now()
	early, late, expired := now.Add(time.Hour), now.Add(2*time.Hour), now.Add(-time.Hour)
	accounts := []accountWithLoad{{account: &Account{ID: 1, SessionWindowEnd: &late}}, {account: &Account{ID: 2, SessionWindowEnd: &early}}, {account: &Account{ID: 3, SessionWindowEnd: &early}}, {account: &Account{ID: 4, SessionWindowEnd: &expired}}, {account: &Account{ID: 5}}}
	result := filterBySoonestReset(accounts)
	require.Len(t, result, 2)
	require.Equal(t, int64(2), result[0].account.ID)
	require.Equal(t, int64(3), result[1].account.ID)
	require.Len(t, filterBySoonestReset(accounts[3:]), 2)
}

func TestStickyMembershipRejectsOtherGroup(t *testing.T) {
	group := int64(7)
	require.False(t, openAIStickyAccountMatchesGroup(&Account{GroupIDs: []int64{8}}, &group))
	require.True(t, openAIStickyAccountMatchesGroup(&Account{GroupIDs: []int64{7}}, &group))
	require.False(t, openAIStickyAccountMatchesGroup(&Account{GroupIDs: []int64{7}}, nil))
}
