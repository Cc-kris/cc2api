package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

type analyticsRepoStub struct {
	rows       []ContentAnalyticsDaily
	aggregated bool
	tz         string
}

func (r *analyticsRepoStub) CreateEvent(context.Context, *ContentAnalyticsEvent) error { return nil }
func (r *analyticsRepoStub) ListDaily(context.Context, ContentAnalyticsFilters) ([]ContentAnalyticsDaily, error) {
	return append([]ContentAnalyticsDaily(nil), r.rows...), nil
}
func (r *analyticsRepoStub) AggregateDaily(_ context.Context, _ time.Time, tz string) error {
	r.aggregated, r.tz = true, tz
	return nil
}

func TestContentAnalyticsAggregateUsesSiteTimezone(t *testing.T) {
	repo := &analyticsRepoStub{}
	svc := NewContentAnalyticsService(repo, "secret")
	if err := svc.Aggregate(context.Background(), time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC), "Asia/Shanghai"); err != nil {
		t.Fatalf("Aggregate() error = %v", err)
	}
	if !repo.aggregated || repo.tz != "Asia/Shanghai" {
		t.Fatalf("Aggregate() did not pass site timezone: aggregated=%v tz=%q", repo.aggregated, repo.tz)
	}
	if err := svc.Aggregate(context.Background(), time.Now(), "not/a-timezone"); err == nil {
		t.Fatal("Aggregate() accepted invalid timezone")
	}
}

func TestContentAnalyticsViewsComputeMetricsAndCSV(t *testing.T) {
	day1 := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	day2 := day1.Add(24 * time.Hour)
	repo := &analyticsRepoStub{rows: []ContentAnalyticsDaily{
		{Date: day1, Locale: "zh-CN", ContentType: "article", ContentID: 1, PagePath: "/articles/a", Placement: "home", UniqueVisitors: 2, PageViews: 5, Reads: 2, Impressions: 10, Clicks: 3, AvgReadSeconds: 20, UpdatedAt: day1.Add(time.Hour)},
		{Date: day2, Locale: "zh-CN", ContentType: "article", ContentID: 1, PagePath: "/articles/a", Placement: "home", UniqueVisitors: 1, PageViews: 3, Reads: 1, Impressions: 5, Clicks: 1, AvgReadSeconds: 40, UpdatedAt: day2.Add(time.Hour)},
		{Date: day2, Locale: "zh-CN", ContentType: "news", ContentID: 9, PagePath: "/news/n", Placement: "related", UniqueVisitors: 4, PageViews: 4, Reads: 4, Impressions: 8, Clicks: 4, AvgReadSeconds: 10, UpdatedAt: day2.Add(2 * time.Hour)},
	}}
	svc := NewContentAnalyticsService(repo, "secret")
	overview, err := svc.Overview(context.Background(), ContentAnalyticsFilters{})
	if err != nil {
		t.Fatalf("Overview() error = %v", err)
	}
	if overview.PageViews != 12 || overview.Reads != 7 || overview.Impressions != 23 || overview.Clicks != 8 || overview.AvgReadSeconds != 17 {
		t.Fatalf("unexpected overview: %+v", overview)
	}
	if overview.CTR != 8.0/23.0 {
		t.Fatalf("overview CTR = %v", overview.CTR)
	}
	trends, err := svc.Trends(context.Background(), ContentAnalyticsFilters{})
	if err != nil || len(trends) != 2 || trends[1].Reads != 5 {
		t.Fatalf("unexpected trends: err=%v rows=%+v", err, trends)
	}
	rankings, err := svc.Rankings(context.Background(), ContentAnalyticsFilters{Page: 1, PageSize: 1})
	if err != nil || rankings.Total != 2 || len(rankings.Items) != 1 || rankings.Items[0].ContentID != 9 || rankings.Items[0].AvgReadSeconds != 10 {
		t.Fatalf("unexpected rankings: err=%v result=%+v", err, rankings)
	}
	ctrs, err := svc.CTR(context.Background(), ContentAnalyticsFilters{})
	if err != nil || len(ctrs) != 2 || ctrs[0].Placement != "related" {
		t.Fatalf("unexpected CTR rows: err=%v rows=%+v", err, ctrs)
	}
	csvBytes, err := svc.ExportCSV(context.Background(), ContentAnalyticsFilters{})
	if err != nil {
		t.Fatalf("ExportCSV() error = %v", err)
	}
	csvText := string(csvBytes)
	if !strings.Contains(csvText, "unique_visitors") || strings.Contains(csvText, "session_hash") || strings.Contains(csvText, "source_url") {
		t.Fatalf("CSV contains invalid fields: %s", csvText)
	}
}
