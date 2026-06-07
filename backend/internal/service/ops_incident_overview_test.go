package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpsServiceGetIncidentOverview_P0AlertBuildsIncidentSummary(t *testing.T) {
	start := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	groupID := int64(8)
	seenImpactModel := ""

	repo := &opsRepoMock{
		GetDashboardOverviewFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
			require.Equal(t, "openai", filter.Platform)
			require.Equal(t, "gpt-5.5", filter.Model)
			return &OpsDashboardOverview{
				StartTime:                    filter.StartTime,
				EndTime:                      filter.EndTime,
				Platform:                     filter.Platform,
				GroupID:                      filter.GroupID,
				SuccessCount:                 98,
				ErrorCountTotal:              3,
				ErrorCountSLA:                3,
				RequestCountTotal:            101,
				RequestCountSLA:              101,
				UpstreamErrorCount:           3,
				UpstreamErrorCountExcl429529: 3,
			}, nil
		},
		GetIncidentImpactFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsIncidentImpact, error) {
			seenImpactModel = filter.Model
			return &OpsIncidentImpact{
				AffectedUsers:    2,
				AffectedAPIKeys:  2,
				AffectedModels:   []string{"gpt-5.5"},
				AffectedAccounts: []*OpsIncidentAffectedAccount{{ID: 11, Name: "upstream-main"}},
			}, nil
		},
		ListAlertEventsFn: func(ctx context.Context, filter *OpsAlertEventFilter) ([]*OpsAlertEvent, error) {
			require.Equal(t, "openai", filter.Platform)
			require.Equal(t, "gpt-5.5", filter.Model)
			require.NotNil(t, filter.GroupID)
			if filter.Status != OpsAlertStatusFiring {
				return nil, nil
			}
			return []*OpsAlertEvent{{
				ID:          99,
				Severity:    "P0",
				Status:      OpsAlertStatusFiring,
				Title:       "OpenAI 上游连续失败",
				Description: "切换备用上游账号并观察恢复。",
				FiredAt:     start.Add(-5 * time.Minute),
			}}, nil
		},
	}

	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	got, err := svc.GetIncidentOverview(context.Background(), &OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  "openai",
		Model:     "gpt-5.5",
		GroupID:   &groupID,
	})
	require.NoError(t, err)
	require.Equal(t, "gpt-5.5", seenImpactModel)
	require.Equal(t, OpsIncidentStatusIncident, got.Status)
	require.Equal(t, int64(3), got.FinalFailures)
	require.InDelta(t, 0.0297, got.FinalFailureRate, 0.0001)
	require.Equal(t, int64(101), got.TotalRequests)
	require.Equal(t, int64(2), got.AffectedUsers)
	require.Equal(t, []string{"gpt-5.5"}, got.AffectedModels)
	require.Len(t, got.AffectedAccounts, 1)
	require.Contains(t, got.Summary, "OpenAI 上游连续失败")
	require.NotEmpty(t, got.ScoreReasons)
	require.NotEmpty(t, got.RecommendedActions)
	require.NotEmpty(t, got.QuickFilters)
}

func TestOpsServiceGetIncidentOverview_NormalEmptyWindow(t *testing.T) {
	start := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	repo := &opsRepoMock{
		GetDashboardOverviewFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
			return &OpsDashboardOverview{
				StartTime:         filter.StartTime,
				EndTime:           filter.EndTime,
				SuccessCount:      1200,
				RequestCountTotal: 1200,
				RequestCountSLA:   1200,
			}, nil
		},
		GetIncidentImpactFn: func(ctx context.Context, filter *OpsDashboardFilter) (*OpsIncidentImpact, error) {
			return &OpsIncidentImpact{}, nil
		},
	}

	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	got, err := svc.GetIncidentOverview(context.Background(), &OpsDashboardFilter{StartTime: start, EndTime: end})
	require.NoError(t, err)
	require.Equal(t, OpsIncidentStatusNormal, got.Status)
	require.Equal(t, OpsIncidentScoreLevelNormal, got.ScoreLevel)
	require.Equal(t, int64(0), got.FinalFailures)
	require.Empty(t, got.QuickFilters)
	require.Contains(t, got.Summary, "当前系统运行正常")
}
