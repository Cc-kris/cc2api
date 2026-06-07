package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
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

func TestComputeCompoundRuleMetricUsesOneMinuteFinalFailuresAndImpact(t *testing.T) {
	groupID := int64(7)
	start := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	rule := &OpsAlertRule{
		MetricType:       "compound_rule",
		MinFinalFailures: 5,
		MinFailureRate:   20,
		MinSampleCount:   20,
		ImpactScope:      map[string]int{"affected_users": 2, "affected_api_keys": 2, "affected_models": 1, "affected_upstream_accounts": 1},
	}

	tests := []struct {
		name     string
		overview *OpsDashboardOverview
		stats    *OpsCompoundAlertStats
		want     float64
	}{
		{
			name: "满足最终失败数、失败率、样本量和影响范围",
			overview: &OpsDashboardOverview{
				RequestCountSLA: 20,
				ErrorCountSLA:   5,
				ErrorRate:       0.25,
			},
			stats: &OpsCompoundAlertStats{
				FinalFailures: 5, AffectedUsers: 2, AffectedAPIKeys: 2, AffectedModels: 1, AffectedUpstreamAccounts: 1,
				MaxFailuresByUser: 3, MaxFailuresByAPIKey: 3, MaxFailuresByModel: 5, MaxFailuresByUpstreamAccount: 5,
				DominantModel: "gpt-5.5",
			},
			want: 5,
		},
		{
			name: "样本量不足不触发",
			overview: &OpsDashboardOverview{
				RequestCountSLA: 10,
				ErrorCountSLA:   5,
				ErrorRate:       0.5,
			},
			stats: &OpsCompoundAlertStats{
				FinalFailures: 5, AffectedUsers: 2, AffectedAPIKeys: 2, AffectedModels: 1, AffectedUpstreamAccounts: 1,
				MaxFailuresByUser: 3, MaxFailuresByAPIKey: 3, MaxFailuresByModel: 5, MaxFailuresByUpstreamAccount: 5,
				DominantModel: "gpt-5.5",
			},
			want: 0,
		},
		{
			name: "影响范围不足不触发",
			overview: &OpsDashboardOverview{
				RequestCountSLA: 20,
				ErrorCountSLA:   5,
				ErrorRate:       0.25,
			},
			stats: &OpsCompoundAlertStats{
				FinalFailures: 5, AffectedUsers: 1, AffectedAPIKeys: 2, AffectedModels: 1, AffectedUpstreamAccounts: 1,
				MaxFailuresByUser: 1, MaxFailuresByAPIKey: 3, MaxFailuresByModel: 5, MaxFailuresByUpstreamAccount: 5,
				DominantModel: "gpt-5.5",
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			repo := &opsRepoMock{
				GetCompoundAlertStatsFn: func(ctx context.Context, filter *OpsCompoundAlertStatsFilter) (*OpsCompoundAlertStats, error) {
					require.True(t, filter.StartTime.Equal(start))
					require.True(t, filter.EndTime.Equal(end))
					require.Equal(t, "openai", filter.Platform)
					require.NotNil(t, filter.GroupID)
					require.Equal(t, groupID, *filter.GroupID)
					return tt.stats, nil
				},
				GetDashboardOverviewFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
					require.Equal(t, "gpt-5.5", filter.Model)
					return tt.overview, nil
				},
			}
			svc := NewOpsAlertEvaluatorService(nil, repo, nil, nil, nil)

			got, ok := svc.computeCompoundRuleMetric(context.Background(), rule, tt.overview, start, end, "openai", &groupID)

			require.True(t, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestComputeCompoundRuleMetricRejectsDistributedModelFailuresForSingleModelRule(t *testing.T) {
	start := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	rule := &OpsAlertRule{
		MetricType:       "compound_rule",
		MinFinalFailures: 10,
		MinFailureRate:   20,
		MinSampleCount:   50,
		ImpactScope:      map[string]int{"affected_models": 1},
	}
	repo := &opsRepoMock{
		GetCompoundAlertStatsFn: func(ctx context.Context, filter *OpsCompoundAlertStatsFilter) (*OpsCompoundAlertStats, error) {
			return &OpsCompoundAlertStats{
				FinalFailures:       10,
				AffectedModels:      5,
				MaxFailuresByModel:  2,
				AffectedUsers:       5,
				MaxFailuresByUser:   2,
				AffectedAPIKeys:     5,
				MaxFailuresByAPIKey: 2,
			}, nil
		},
	}
	svc := NewOpsAlertEvaluatorService(nil, repo, nil, nil, nil)

	got, ok := svc.computeCompoundRuleMetric(context.Background(), rule, &OpsDashboardOverview{
		RequestCountSLA: 50,
		ErrorCountSLA:   10,
		ErrorRate:       0.2,
	}, start, end, "", nil)

	require.True(t, ok)
	require.Equal(t, float64(0), got)
}

func TestOpsAlertEvaluatorCreatesEventForCompoundRuleWithinOneMinuteWindow(t *testing.T) {
	var created *OpsAlertEvent
	repo := &opsRepoMock{
		ListAlertRulesFn: func(ctx context.Context) ([]*OpsAlertRule, error) {
			return []*OpsAlertRule{
				{
					ID:               20,
					Name:             "P1 最终失败",
					Enabled:          true,
					RuleVersion:      "v2",
					MigrationState:   "normal",
					MetricType:       "compound_rule",
					Operator:         ">=",
					Threshold:        5,
					WindowMinutes:    1,
					MinFinalFailures: 5,
					MinFailureRate:   20,
					MinSampleCount:   20,
					TriggerLevel:     "P1",
					Severity:         "P1",
					ErrorCategories:  []string{"upstream"},
				},
			}, nil
		},
		GetDashboardOverviewFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
			require.Equal(t, OpsQueryModeRaw, filter.QueryMode)
			require.Equal(t, time.Minute, filter.EndTime.Sub(filter.StartTime))
			return &OpsDashboardOverview{
				RequestCountSLA: 20,
				ErrorCountSLA:   5,
				ErrorRate:       0.25,
			}, nil
		},
		GetCompoundAlertStatsFn: func(ctx context.Context, filter *OpsCompoundAlertStatsFilter) (*OpsCompoundAlertStats, error) {
			return &OpsCompoundAlertStats{FinalFailures: 5}, nil
		},
		CreateAlertEventFn: func(ctx context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
			created = event
			event.ID = 200
			return event, nil
		},
	}
	svc := NewOpsAlertEvaluatorService(nil, repo, nil, nil, nil)

	svc.evaluateOnce(time.Minute)

	require.NotNil(t, created)
	require.Equal(t, int64(20), created.RuleID)
	require.Equal(t, "upstream|compound_rule|all|all|all|P1", created.EventKey)
	require.Equal(t, float64(5), *created.MetricValue)
	require.Equal(t, float64(5), *created.ThresholdValue)
	require.Equal(t, "compound_rule", created.TriggerSnapshot["metric_type"])
	require.Equal(t, float64(5), created.TriggerSnapshot["metric_value"])
}

func TestOpsAlertEvaluatorCreatesAutoAIAnalysisForNewP1Event(t *testing.T) {
	var createdEvent *OpsAlertEvent
	var createdTask *OpsAIAnalysisTaskCreateInput
	var linkedEventID int64
	var linkedTaskID int64
	repo := &opsRepoMock{
		ListAlertRulesFn: func(ctx context.Context) ([]*OpsAlertRule, error) {
			return []*OpsAlertRule{
				{
					ID:              31,
					Name:            "P1 上游错误",
					Enabled:         true,
					RuleVersion:     "v2",
					MigrationState:  "normal",
					MetricType:      "error_rate",
					Operator:        ">=",
					Threshold:       1,
					WindowMinutes:   1,
					TriggerLevel:    "P1",
					Severity:        "P1",
					ErrorCategories: []string{"upstream"},
					AutoAIAnalysis:  true,
				},
			}, nil
		},
		GetDashboardOverviewFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
			require.Equal(t, OpsQueryModeRaw, filter.QueryMode)
			require.Equal(t, time.Minute, filter.EndTime.Sub(filter.StartTime))
			return &OpsDashboardOverview{RequestCountSLA: 100, ErrorCountSLA: 5, ErrorRate: 0.05}, nil
		},
		CreateAlertEventFn: func(ctx context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
			createdEvent = event
			event.ID = 903
			return event, nil
		},
		CreateAIAnalysisTaskIfAllowedFn: func(ctx context.Context, input *OpsAIAnalysisTaskCreateInput, maxActive int) (*OpsAIAnalysisTask, OpsAIAnalysisTaskCreateResult, error) {
			createdTask = input
			require.Equal(t, opsAIAutoMaxActiveTasks, maxActive)
			return &OpsAIAnalysisTask{ID: 904, Status: OpsAIAnalysisStatusPending}, OpsAIAnalysisTaskCreateResultCreated, nil
		},
		UpdateAlertEventAITaskIDFn: func(ctx context.Context, eventID int64, taskID int64) error {
			linkedEventID = eventID
			linkedTaskID = taskID
			return nil
		},
	}
	opsService := NewOpsService(repo, newRuntimeSettingRepoStub(), &config.Config{Ops: config.OpsConfig{Enabled: true}}, nil, nil, nil, nil, nil, nil, nil, nil)
	seedManualAIConfig(t, opsService)
	svc := NewOpsAlertEvaluatorService(opsService, repo, nil, nil, nil)

	svc.evaluateOnce(time.Minute)

	require.NotNil(t, createdEvent)
	require.Equal(t, int64(31), createdEvent.RuleID)
	require.Equal(t, "upstream|error_rate|all|all|all|P1", createdEvent.EventKey)
	require.NotNil(t, createdTask)
	require.Equal(t, OpsAIAnalysisSourceAlertEvent, createdTask.SourceType)
	require.NotNil(t, createdTask.SourceID)
	require.Equal(t, int64(903), *createdTask.SourceID)
	require.Equal(t, OpsAIAnalysisTriggerAuto, createdTask.TriggerType)
	require.Nil(t, createdTask.TriggerUserID)
	require.Contains(t, createdTask.FiltersJSON, `"alert_event_key":"upstream|error_rate|all|all|all|P1"`)
	require.Contains(t, createdTask.FiltersJSON, `"error_categories":["upstream"]`)
	require.Equal(t, int64(903), linkedEventID)
	require.Equal(t, int64(904), linkedTaskID)
	require.NotNil(t, createdEvent.AITaskID)
	require.Equal(t, int64(904), *createdEvent.AITaskID)
}
