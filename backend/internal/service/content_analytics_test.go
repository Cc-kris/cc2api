package service

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

type analyticsRepoStub struct {
	rows                []ContentAnalyticsDaily
	distributions       []ContentAnalyticsDistribution
	distributionDim     string
	distributionFilters ContentAnalyticsFilters
	aggregated          bool
	tz                  string
}

func (r *analyticsRepoStub) CreateEvent(context.Context, *ContentAnalyticsEvent) error { return nil }
func (r *analyticsRepoStub) ListDaily(context.Context, ContentAnalyticsFilters) ([]ContentAnalyticsDaily, error) {
	return append([]ContentAnalyticsDaily(nil), r.rows...), nil
}
func (r *analyticsRepoStub) AggregateDaily(_ context.Context, _ time.Time, tz string) error {
	r.aggregated, r.tz = true, tz
	return nil
}
func (r *analyticsRepoStub) ContentAnalyticsDistribution(_ context.Context, dim string, f ContentAnalyticsFilters) ([]ContentAnalyticsDistribution, error) {
	r.distributionDim, r.distributionFilters = dim, f
	return append([]ContentAnalyticsDistribution(nil), r.distributions...), nil
}

func TestContentAnalyticsDistributionDimensionsAndFilters(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	sourceID := int64(42)
	filters := ContentAnalyticsFilters{From: &from, To: &to, Locale: "en-US", ContentType: "news", Placement: "home", SourceID: &sourceID}
	repo := &analyticsRepoStub{distributions: []ContentAnalyticsDistribution{{Key: "direct", Count: 3, Ratio: 1}}}
	svc := NewContentAnalyticsService(repo, "secret")
	for _, dim := range []string{"category", "locale", "source", "referrer"} {
		got, err := svc.Distribution(context.Background(), dim, filters)
		if err != nil || len(got) != 1 || got[0].Key != "direct" || got[0].Count != 3 || repo.distributionDim != dim {
			t.Fatalf("Distribution(%q) = %+v, %v", dim, got, err)
		}
		if repo.distributionFilters.From != &from || repo.distributionFilters.To != &to || repo.distributionFilters.Locale != filters.Locale || repo.distributionFilters.ContentType != filters.ContentType || repo.distributionFilters.Placement != filters.Placement || repo.distributionFilters.SourceID != &sourceID {
			t.Fatalf("Distribution(%q) filters not forwarded: %+v", dim, repo.distributionFilters)
		}
	}
	for _, dim := range []string{"", "unknown", "visitors"} {
		if _, err := svc.Distribution(context.Background(), dim, filters); err == nil {
			t.Errorf("Distribution(%q) accepted unsupported dimension", dim)
		}
	}
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

func TestContentAnalyticsViewLifecycleValidation(t *testing.T) {
	svc := NewContentAnalyticsService(&analyticsRepoStub{}, "secret")
	cases := []struct {
		name    string
		event   string
		payload map[string]any
		wantErr bool
	}{
		{"heartbeat", "heartbeat", map[string]any{"view_id": "view-1", "duration_ms": float64(30_000)}, false},
		{"leave", "page_leave", map[string]any{"viewId": "view-1", "duration_ms": float64(maxAnalyticsViewDurationMS)}, false},
		{"missing view id", "page_heartbeat", map[string]any{"duration_ms": float64(30_000)}, true},
		{"missing duration", "page_leave", map[string]any{"view_id": "view-1"}, true},
		{"negative duration", "page_heartbeat", map[string]any{"view_id": "view-1", "duration_ms": float64(-1)}, true},
		{"over four hours", "page_leave", map[string]any{"view_id": "view-1", "duration_ms": float64(maxAnalyticsViewDurationMS + 1)}, true},
		{"fractional duration", "page_heartbeat", map[string]any{"view_id": "view-1", "duration_ms": 1.5}, true},
		{"page view missing id", "page_view", map[string]any{}, true},
		{"impression missing id", "content_impression", map[string]any{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &ContentAnalyticsEvent{
				EventID:   "event-" + tc.name,
				EventName: tc.event,
				PagePath:  "/articles/a",
				EventAt:   time.Now().UTC(),
				Payload:   tc.payload,
			}
			err := svc.PrepareEvent(e, "192.0.2.1", "test-agent")
			if (err != nil) != tc.wantErr {
				t.Fatalf("PrepareEvent() error = %v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestContentAnalyticsViewDurationUsesLatestAndImpressionViewDeduplication(t *testing.T) {
	dsn := os.Getenv("CONTENT_ANALYTICS_TEST_DSN")
	if dsn == "" {
		t.Skip("set CONTENT_ANALYTICS_TEST_DSN to run PostgreSQL aggregation regression")
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	source, err := os.ReadFile(filepath.Join(root, "internal/repository/content_analytics_repo.go"))
	if err != nil {
		t.Fatal(err)
	}
	const start, end = "const query = `", "\n\tif _, err := tx.ExecContext(ctx, query"
	text := string(source)
	i, j := strings.Index(text, start), strings.Index(text, end)
	if i < 0 || j <= i {
		t.Fatal("aggregate SQL not found")
	}
	query := strings.TrimSuffix(text[i+len(start):j], "`")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ddl := `CREATE TEMP TABLE content_analytics_events(event_id text, event_name text, event_at timestamptz,
	 payload_json jsonb, session_hash text, locale text, content_type text, content_id bigint,
	 page_path text, placement text);
	 CREATE TEMP TABLE content_analytics_daily(date date, locale text, content_type text, content_id bigint,
	 page_path text, placement text, unique_visitors integer, page_views integer, reads integer,
	 impressions integer, clicks integer, avg_read_seconds integer, ctr double precision, updated_at timestamptz,
	 UNIQUE(date,locale,content_type,content_id,page_path,placement));`
	if _, err = tx.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	rows := [][]any{
		{"p1", "page_view", base, `{"view_id":"v1"}`}, {"p2", "page_view", base, `{"view_id":"v1"}`},
		{"p3", "page_view", base, `{}`}, {"i1", "content_impression", base, `{"view_id":"v1"}`},
		{"i2", "content_impression", base, `{"view_id":"v1"}`}, {"i3", "content_impression", base, `{}`},
		{"h1", "heartbeat", base.Add(30 * time.Second), `{"view_id":"v2","duration_ms":90000}`},
		{"h2", "heartbeat", base.Add(15 * time.Second), `{"view_id":"v2","duration_ms":60000}`},
		{"h3", "heartbeat", base.Add(30 * time.Second), `{"view_id":"v2","duration_ms":45000}`},
		{"h4", "heartbeat", base.Add(30 * time.Second), `{"view_id":"v2","duration_ms":70000}`},
		{"l1", "page_leave", base, `{"view_id":"v2","duration_ms":20000}`},
		{"l2", "page_leave", base.Add(time.Second), `{"view_id":"v2","duration_ms":25000}`},
		{"z1", "heartbeat", base, `{"view_id":"v3","duration_ms":5000}`},
		{"old", "heartbeat", base.Add(25 * time.Second), `{"view_id":"v4","duration_ms":120000}`},
		{"tie-a", "heartbeat", base.Add(30 * time.Second), `{"view_id":"v4","duration_ms":45000}`},
		{"tie-z", "heartbeat", base.Add(30 * time.Second), `{"view_id":"v4","duration_ms":70000}`},
	}
	for _, r := range rows {
		if _, err = tx.Exec(`INSERT INTO content_analytics_events VALUES($1,$2,$3,$4,'s','en','article',1,'/p','home')`, r...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(query, base, "UTC"); err != nil {
		t.Fatal(err)
	}
	var views, impressions, seconds int
	if err = tx.QueryRow(`SELECT page_views, impressions, avg_read_seconds FROM content_analytics_daily WHERE date=$1`, base).Scan(&views, &impressions, &seconds); err != nil {
		t.Fatal(err)
	}
	if views != 1 || impressions != 1 || seconds != 33 {
		t.Fatalf("got page_views=%d impressions=%d avg_read_seconds=%d; want 1,1,33", views, impressions, seconds)
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
