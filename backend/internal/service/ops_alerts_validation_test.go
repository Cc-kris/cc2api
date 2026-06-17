package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validCompoundAlertRuleForTest() *OpsAlertRule {
	return &OpsAlertRule{
		Name:                       "上游账号集中失败",
		Enabled:                    true,
		RuleVersion:                "v2",
		ErrorCategories:            []string{"upstream", "permission"},
		TriggerLevel:               "P1",
		MinFinalFailures:           5,
		MinFailureRate:             10,
		MinSampleCount:             50,
		ImpactScope:                map[string]int{"affected_api_keys": 2},
		RecoveredFluctuationPolicy: "observe_only",
		MinRecoveredFluctuations:   10,
		AutoAIAnalysis:             true,
		NotificationChannels:       []string{"in_app", "email"},
		SilenceMinutes:             10,
		WindowMinutes:              1,
		SustainedMinutes:           1,
		MetricType:                 "compound_rule",
		Operator:                   ">=",
		Threshold:                  5,
	}
}

func TestOpsServiceCreateAlertRuleRejectsDuplicateName(t *testing.T) {
	repo := &opsRepoMock{
		ListAlertRulesFn: func(ctx context.Context) ([]*OpsAlertRule, error) {
			return []*OpsAlertRule{{ID: 7, Name: "上游账号集中失败"}}, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.CreateAlertRule(context.Background(), validCompoundAlertRuleForTest())
	require.Error(t, err)
	require.Contains(t, err.Error(), "规则名称已存在")
}

func TestOpsServiceCreateAlertRuleRejectsInvalidCompoundThresholds(t *testing.T) {
	repo := &opsRepoMock{}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	rule := validCompoundAlertRuleForTest()
	rule.MinFinalFailures = 100
	rule.MinSampleCount = 50
	_, err := svc.CreateAlertRule(context.Background(), rule)
	require.Error(t, err)
	require.Contains(t, err.Error(), "最小最终失败数不能大于最小样本量")
}

func TestOpsServiceCreateAlertRuleNormalizesCompoundRule(t *testing.T) {
	var saved *OpsAlertRule
	repo := &opsRepoMock{
		CreateAlertRuleFn: func(ctx context.Context, input *OpsAlertRule) (*OpsAlertRule, error) {
			saved = input
			return input, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	rule := validCompoundAlertRuleForTest()
	rule.Severity = ""
	rule.NotifyEmail = false
	got, err := svc.CreateAlertRule(context.Background(), rule)
	require.NoError(t, err)
	require.Same(t, saved, got)
	require.Equal(t, "P1", saved.Severity)
	require.True(t, saved.NotifyEmail)
	require.Equal(t, "compound_rule", saved.MetricType)
}

func TestOpsServiceCreateMetricDrivenAlertRules(t *testing.T) {
	tests := []struct {
		name       string
		rule       *OpsAlertRule
		wantMetric string
		wantOp     string
		wantValue  float64
	}{
		{
			name: "health score",
			rule: &OpsAlertRule{
				Name:                 "健康分过低",
				Enabled:              true,
				RuleVersion:          "v2",
				MetricType:           "health_score",
				Operator:             "<",
				Threshold:            70,
				TriggerLevel:         "P1",
				MinFinalFailures:     1,
				MinSampleCount:       1,
				NotificationChannels: []string{"in_app", "email"},
				WindowMinutes:        1,
			},
			wantMetric: "health_score",
			wantOp:     "<",
			wantValue:  70,
		},
		{
			name: "final failure rate",
			rule: &OpsAlertRule{
				Name:                 "事故失败率过高",
				Enabled:              true,
				RuleVersion:          "v2",
				MetricType:           "final_failure_rate",
				TriggerLevel:         "P0",
				MinFinalFailures:     5,
				MinFailureRate:       20,
				MinSampleCount:       50,
				NotificationChannels: []string{"in_app", "email"},
				WindowMinutes:        1,
			},
			wantMetric: "final_failure_rate",
			wantOp:     ">=",
			wantValue:  20,
		},
		{
			name: "final failures",
			rule: &OpsAlertRule{
				Name:                 "事故失败数过高",
				Enabled:              true,
				RuleVersion:          "v2",
				MetricType:           "final_failures",
				TriggerLevel:         "P1",
				MinFinalFailures:     5,
				MinSampleCount:       1,
				NotificationChannels: []string{"in_app"},
				WindowMinutes:        1,
			},
			wantMetric: "final_failures",
			wantOp:     ">=",
			wantValue:  5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var saved *OpsAlertRule
			repo := &opsRepoMock{
				CreateAlertRuleFn: func(ctx context.Context, input *OpsAlertRule) (*OpsAlertRule, error) {
					saved = input
					return input, nil
				},
			}
			svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			_, err := svc.CreateAlertRule(context.Background(), tt.rule)
			require.NoError(t, err)
			require.Equal(t, tt.wantMetric, saved.MetricType)
			require.Equal(t, tt.wantOp, saved.Operator)
			require.InDelta(t, tt.wantValue, saved.Threshold, 0.001)
			require.Empty(t, saved.ErrorCategories)
		})
	}
}

func TestOpsServiceUpdateAlertRuleRejectsMigratedLegacyRule(t *testing.T) {
	tests := []struct {
		name           string
		migrationState string
	}{
		{name: "migrated", migrationState: "migrated"},
		{name: "readonly_legacy", migrationState: "readonly_legacy"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			repo := &opsRepoMock{
				ListAlertRulesFn: func(ctx context.Context) ([]*OpsAlertRule, error) {
					return []*OpsAlertRule{{ID: 9, Name: "旧错误率规则", RuleVersion: "v1", MigrationState: tt.migrationState}}, nil
				},
			}
			svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			rule := validCompoundAlertRuleForTest()
			rule.ID = 9
			_, err := svc.UpdateAlertRule(context.Background(), rule)
			require.Error(t, err)
			require.Contains(t, err.Error(), "该规则已按新告警模型迁移")
		})
	}
}

func TestOpsServiceUpdateAlertRuleFailsClosedWhenExistingRuleCannotBeLoaded(t *testing.T) {
	repoErr := errors.New("list alert rules failed")
	repo := &opsRepoMock{
		ListAlertRulesFn: func(ctx context.Context) ([]*OpsAlertRule, error) {
			return nil, repoErr
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	rule := validCompoundAlertRuleForTest()
	rule.ID = 9

	_, err := svc.UpdateAlertRule(context.Background(), rule)

	require.ErrorIs(t, err, repoErr)
}

func TestOpsServiceCreateAlertEventMergesByEventKeyWithinSilenceWindow(t *testing.T) {
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	mergeStart := now.Add(-10 * time.Minute)
	var lookedUpKey string
	var lookedUpSince time.Time
	var created bool
	var mergedID int64
	repo := &opsRepoMock{
		GetMergeableAlertEventFn: func(ctx context.Context, eventKey string, since time.Time) (*OpsAlertEvent, error) {
			lookedUpKey = eventKey
			lookedUpSince = since
			return &OpsAlertEvent{ID: 99, EventKey: eventKey, MergedCount: 2}, nil
		},
		CreateAlertEventFn: func(ctx context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
			created = true
			return event, nil
		},
		MergeAlertEventFn: func(ctx context.Context, eventID int64, event *OpsAlertEvent) (*OpsAlertEvent, error) {
			mergedID = eventID
			return &OpsAlertEvent{ID: eventID, EventKey: event.EventKey, MergedCount: 3, LastSeenAt: event.LastSeenAt}, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	got, err := svc.CreateAlertEvent(context.Background(), &OpsAlertEvent{
		EventKey:         "upstream|rate_limit|1|gpt-5.5|acct-1|P1",
		Status:           OpsAlertStatusFiring,
		LifecycleStatus:  OpsAlertStatusFiring,
		FiredAt:          now,
		LastSeenAt:       now,
		MergeWindowStart: &mergeStart,
	})

	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, int64(99), mergedID)
	require.Equal(t, int64(99), got.ID)
	require.Equal(t, 3, got.MergedCount)
	require.Equal(t, "upstream|rate_limit|1|gpt-5.5|acct-1|P1", lookedUpKey)
	require.True(t, lookedUpSince.Equal(mergeStart))
}

func TestBuildOpsAlertEventKeyUsesPRDDedupDimensions(t *testing.T) {
	rule := &OpsAlertRule{
		MetricType:      "error_rate",
		Severity:        "P1",
		ErrorCategories: []string{"upstream", "rate_limit"},
		TriggerLevel:    "P1",
	}

	got := buildOpsAlertEventKey(rule, map[string]any{
		"group_id":            int64(7),
		"model":               "gpt-5.5",
		"upstream_account_id": int64(42),
	})

	require.Equal(t, "upstream,rate_limit|error_rate|7|gpt-5.5|42|P1", got)
}

func TestOpsServiceUpdateAlertEventStatusSupportsLifecycleStatuses(t *testing.T) {
	operatorID := int64(6)
	resolvedAt := time.Date(2026, 6, 7, 10, 5, 0, 0, time.UTC)
	tests := []struct {
		name       string
		input      string
		want       string
		resolvedAt *time.Time
	}{
		{name: "acknowledged", input: OpsAlertStatusAcknowledged, want: OpsAlertStatusAcknowledged},
		{name: "processing", input: OpsAlertStatusProcessing, want: OpsAlertStatusProcessing},
		{name: "recovered", input: OpsAlertStatusRecovered, want: OpsAlertStatusRecovered, resolvedAt: &resolvedAt},
		{name: "closed", input: OpsAlertStatusClosed, want: OpsAlertStatusClosed, resolvedAt: &resolvedAt},
		{name: "silenced", input: OpsAlertStatusSilenced, want: OpsAlertStatusSilenced},
		{name: "legacy resolved", input: OpsAlertStatusResolved, want: OpsAlertStatusRecovered, resolvedAt: &resolvedAt},
		{name: "legacy manual resolved", input: OpsAlertStatusManualResolved, want: OpsAlertStatusClosed, resolvedAt: &resolvedAt},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var gotStatus string
			var gotNote string
			var gotProcessingAction string
			var gotOperatorID *int64
			repo := &opsRepoMock{
				UpdateAlertEventStatusFn: func(ctx context.Context, eventID int64, status string, note string, processingAction string, operatorID *int64, resolvedAt *time.Time) error {
					gotStatus = status
					gotNote = note
					gotProcessingAction = processingAction
					gotOperatorID = operatorID
					return nil
				},
			}
			svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

			err := svc.UpdateAlertEventStatus(context.Background(), 31, tt.input, "备注", "独立处理动作", &operatorID, tt.resolvedAt)

			require.NoError(t, err)
			require.Equal(t, tt.want, gotStatus)
			require.Equal(t, "备注", gotNote)
			require.Equal(t, "独立处理动作", gotProcessingAction)
			require.NotNil(t, gotOperatorID)
			require.Equal(t, operatorID, *gotOperatorID)
		})
	}
}

func TestOpsServiceUpdateAlertEventStatusRejectsInvalidLifecycleStatus(t *testing.T) {
	repo := &opsRepoMock{}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	err := svc.UpdateAlertEventStatus(context.Background(), 31, "invalid", "", "", nil, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid status")
}
