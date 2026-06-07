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

func TestOpsAlertEvaluatorMergesRepeatedEventKeyWithinSilenceWindow(t *testing.T) {
	var created bool
	var mergedID int64
	var lookedUpKey string
	var lookedUpSince time.Time
	var latestLookedUp bool
	repo := &opsRepoMock{
		ListAlertRulesFn: func(ctx context.Context) ([]*OpsAlertRule, error) {
			return []*OpsAlertRule{
				{
					ID:              10,
					Name:            "上游错误率",
					Enabled:         true,
					RuleVersion:     "v2",
					MigrationState:  "normal",
					ErrorCategories: []string{"upstream"},
					TriggerLevel:    "P1",
					MetricType:      "error_rate",
					Operator:        ">=",
					Threshold:       1,
					WindowMinutes:   1,
					SilenceMinutes:  10,
				},
			}, nil
		},
		GetDashboardOverviewFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
			return &OpsDashboardOverview{
				RequestCountSLA: 100,
				ErrorRate:       0.5,
			}, nil
		},
		GetMergeableAlertEventFn: func(ctx context.Context, eventKey string, since time.Time) (*OpsAlertEvent, error) {
			lookedUpKey = eventKey
			lookedUpSince = since
			return &OpsAlertEvent{ID: 88, EventKey: eventKey, MergedCount: 1}, nil
		},
		GetLatestAlertEventFn: func(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
			latestLookedUp = true
			return &OpsAlertEvent{ID: 77, RuleID: ruleID, FiredAt: time.Now().UTC().Add(-time.Minute)}, nil
		},
		CreateAlertEventFn: func(ctx context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
			created = true
			return event, nil
		},
		MergeAlertEventFn: func(ctx context.Context, eventID int64, event *OpsAlertEvent) (*OpsAlertEvent, error) {
			mergedID = eventID
			return &OpsAlertEvent{ID: eventID, EventKey: event.EventKey, MergedCount: 2, LastSeenAt: event.LastSeenAt}, nil
		},
	}
	opsService := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	svc := NewOpsAlertEvaluatorService(opsService, repo, nil, nil, nil)

	svc.evaluateOnce(time.Minute)

	require.False(t, created)
	require.False(t, latestLookedUp)
	require.Equal(t, int64(88), mergedID)
	require.Equal(t, "upstream|error_rate|all|all|all|P1", lookedUpKey)
	require.False(t, lookedUpSince.IsZero())
	require.WithinDuration(t, time.Now().UTC().Add(-10*time.Minute), lookedUpSince, 5*time.Second)
}
