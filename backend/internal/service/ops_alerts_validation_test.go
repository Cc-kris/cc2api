package service

import (
	"context"
	"errors"
	"testing"

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
