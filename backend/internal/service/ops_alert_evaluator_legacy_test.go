package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpsAlertEvaluatorSkipsMigratedLegacyPercentRules(t *testing.T) {
	var createdRuleIDs []int64
	repo := &opsRepoMock{
		ListAlertRulesFn: func(ctx context.Context) ([]*OpsAlertRule, error) {
			return []*OpsAlertRule{
				{
					ID:             1,
					Name:           "已迁移旧错误率规则",
					Enabled:        true,
					RuleVersion:    "v1",
					MigrationState: "migrated",
					MetricType:     "error_rate",
					Operator:       ">=",
					Threshold:      1,
					WindowMinutes:  1,
				},
				{
					ID:             2,
					Name:           "未迁移旧错误率规则",
					Enabled:        true,
					RuleVersion:    "v1",
					MigrationState: "normal",
					MetricType:     "error_rate",
					Operator:       ">=",
					Threshold:      1,
					WindowMinutes:  1,
				},
				{
					ID:             3,
					Name:           "只读旧错误率规则",
					Enabled:        true,
					RuleVersion:    "v1",
					MigrationState: "readonly_legacy",
					MetricType:     "error_rate",
					Operator:       ">=",
					Threshold:      1,
					WindowMinutes:  1,
				},
			}, nil
		},
		GetDashboardOverviewFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
			return &OpsDashboardOverview{
				RequestCountSLA: 100,
				ErrorRate:       0.5,
			}, nil
		},
		CreateAlertEventFn: func(ctx context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
			createdRuleIDs = append(createdRuleIDs, event.RuleID)
			event.ID = event.RuleID
			return event, nil
		},
	}
	svc := NewOpsAlertEvaluatorService(nil, repo, nil, nil, nil)

	svc.evaluateOnce(time.Minute)

	require.Equal(t, []int64{2}, createdRuleIDs)
}
