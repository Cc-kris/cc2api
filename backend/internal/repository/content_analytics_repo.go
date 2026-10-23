package repository

import (
	"context"
	stdsql "database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	entdialectsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/contentanalyticsdaily"
	"github.com/Wei-Shaw/sub2api/ent/contentanalyticsevent"
	"github.com/Wei-Shaw/sub2api/ent/contentarticle"
	"github.com/Wei-Shaw/sub2api/ent/contentnewsitem"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type contentAnalyticsRepository struct {
	client   *dbent.Client
	db       *stdsql.DB
	timezone string
}

func NewContentAnalyticsRepository(c *dbent.Client) service.ContentAnalyticsRepository {
	return NewContentAnalyticsRepositoryWithTimezone(c, "Asia/Shanghai")
}

func NewContentAnalyticsRepositoryWithTimezone(c *dbent.Client, siteTimezone string) service.ContentAnalyticsRepository {
	if strings.TrimSpace(siteTimezone) == "" {
		siteTimezone = "Asia/Shanghai"
	}
	r := &contentAnalyticsRepository{client: c, timezone: siteTimezone}
	if c != nil {
		if drv, ok := c.Driver().(*entdialectsql.Driver); ok {
			r.db = drv.DB()
		}
	}
	return r
}

// CreateEvent is idempotent on event_id. The unique constraint remains the
// authoritative dedupe guard even when multiple browser retries race.
func (r *contentAnalyticsRepository) CreateEvent(ctx context.Context, e *service.ContentAnalyticsEvent) error {
	if r == nil || r.client == nil || e == nil {
		return errors.New("analytics repository is unavailable")
	}
	payload := e.Payload
	if len(payload) == 0 {
		payload = map[string]any{}
	}
	b := r.client.ContentAnalyticsEvent.Create().
		SetEventID(e.EventID).
		SetEventName(e.EventName).
		SetPagePath(e.PagePath).
		SetSessionHash(e.SessionHash).
		SetEventAt(e.EventAt).
		SetPayloadJSON(payload)
	if e.ContentType != nil {
		b.SetContentType(*e.ContentType)
	}
	if e.ContentID != nil {
		b.SetContentID(*e.ContentID)
	}
	if e.Locale != nil {
		b.SetLocale(*e.Locale)
	}
	if e.Placement != nil {
		b.SetPlacement(*e.Placement)
	}
	if e.ReferrerHost != nil {
		b.SetReferrerHost(*e.ReferrerHost)
	}
	if err := b.OnConflict(
		entdialectsql.ConflictColumns(contentanalyticsevent.FieldEventID),
		entdialectsql.ResolveWithIgnore(),
	).Exec(ctx); err != nil {
		return err
	}
	if r.db != nil {
		_ = r.AggregateDaily(ctx, e.EventAt, r.timezone)
	}
	return nil
}

func (r *contentAnalyticsRepository) ListDaily(ctx context.Context, f service.ContentAnalyticsFilters) ([]service.ContentAnalyticsDaily, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("analytics repository is unavailable")
	}
	q := r.client.ContentAnalyticsDaily.Query().Order(contentanalyticsdaily.ByDate())
	if f.From != nil {
		q = q.Where(contentanalyticsdaily.DateGTE(dateOnly(*f.From)))
	}
	if f.To != nil {
		q = q.Where(contentanalyticsdaily.DateLTE(dateOnly(*f.To)))
	}
	if f.Locale != "" {
		q = q.Where(contentanalyticsdaily.LocaleEQ(strings.TrimSpace(f.Locale)))
	}
	if f.ContentType != "" {
		q = q.Where(contentanalyticsdaily.ContentTypeEQ(strings.TrimSpace(f.ContentType)))
	}
	if f.Placement != "" {
		q = q.Where(contentanalyticsdaily.PlacementEQ(strings.TrimSpace(f.Placement)))
	}
	if f.SourceID != nil {
		newsIDs, err := r.client.ContentNewsItem.Query().Where(contentnewsitem.SourceIDEQ(*f.SourceID)).IDs(ctx)
		if err != nil {
			return nil, err
		}
		if len(newsIDs) == 0 {
			return []service.ContentAnalyticsDaily{}, nil
		}
		q = q.Where(contentanalyticsdaily.ContentTypeEQ("news"), contentanalyticsdaily.ContentIDIn(newsIDs...))
	}
	xs, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.ContentAnalyticsDaily, 0, len(xs))
	for _, x := range xs {
		out = append(out, service.ContentAnalyticsDaily{Date: x.Date, Locale: x.Locale, ContentType: x.ContentType, ContentID: x.ContentID, PagePath: x.PagePath, Placement: x.Placement, UniqueVisitors: x.UniqueVisitors, PageViews: x.PageViews, Reads: x.Reads, Impressions: x.Impressions, Clicks: x.Clicks, AvgReadSeconds: x.AvgReadSeconds, CTR: x.Ctr, UpdatedAt: x.UpdatedAt})
	}
	if err := r.populateTitles(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *contentAnalyticsRepository) CountDistinctVisitors(ctx context.Context, f service.ContentAnalyticsFilters) (int, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("analytics SQL repository is unavailable")
	}
	clauses := []string{"1=1"}
	args := make([]any, 0, 5)
	add := func(expr string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(expr, len(args)))
	}
	if f.From != nil {
		args = append(args, r.timezone, dateOnly(*f.From))
		clauses = append(clauses, fmt.Sprintf("(e.event_at AT TIME ZONE $%d)::date >= $%d::date", len(args)-1, len(args)))
	}
	if f.To != nil {
		args = append(args, r.timezone, dateOnly(*f.To))
		clauses = append(clauses, fmt.Sprintf("(e.event_at AT TIME ZONE $%d)::date < ($%d::date + INTERVAL '1 day')", len(args)-1, len(args)))
	}
	if f.Locale != "" {
		add("e.locale = $%d", strings.TrimSpace(f.Locale))
	}
	if f.ContentType != "" {
		add("e.content_type = $%d", strings.TrimSpace(f.ContentType))
	}
	if f.Placement != "" {
		add("e.placement = $%d", strings.TrimSpace(f.Placement))
	}
	if f.SourceID != nil {
		add("(e.content_type = 'news' AND EXISTS (SELECT 1 FROM content_news_items n WHERE n.id = e.content_id AND n.source_id = $%d))", *f.SourceID)
	}
	query := "SELECT COUNT(DISTINCT e.session_hash) FROM content_analytics_events e WHERE " + strings.Join(clauses, " AND ")
	var count int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *contentAnalyticsRepository) populateTitles(ctx context.Context, rows []service.ContentAnalyticsDaily) error {
	newsIDs := make([]int64, 0)
	articleIDs := make([]int64, 0)
	for _, row := range rows {
		if row.ContentID <= 0 {
			continue
		}
		switch row.ContentType {
		case "news":
			newsIDs = append(newsIDs, row.ContentID)
		case "article":
			articleIDs = append(articleIDs, row.ContentID)
		}
	}
	newsTitles := make(map[int64]string)
	if len(newsIDs) > 0 {
		items, err := r.client.ContentNewsItem.Query().Where(contentnewsitem.IDIn(newsIDs...)).Select(contentnewsitem.FieldID, contentnewsitem.FieldTitle).All(ctx)
		if err != nil {
			return err
		}
		for _, item := range items {
			newsTitles[item.ID] = item.Title
		}
	}
	articleTitles := make(map[int64]string)
	if len(articleIDs) > 0 {
		items, err := r.client.ContentArticle.Query().Where(contentarticle.IDIn(articleIDs...)).Select(contentarticle.FieldID, contentarticle.FieldTitle).All(ctx)
		if err != nil {
			return err
		}
		for _, item := range items {
			articleTitles[item.ID] = item.Title
		}
	}
	for i := range rows {
		if rows[i].ContentType == "news" {
			rows[i].Title = newsTitles[rows[i].ContentID]
		}
		if rows[i].ContentType == "article" {
			rows[i].Title = articleTitles[rows[i].ContentID]
		}
	}
	return nil
}

// AggregateDaily recomputes the local current and previous day. Replacing
// rows rather than incrementing counters makes retries and delayed events
// deterministic and keeps event_id dedupe independent from aggregation runs.
func (r *contentAnalyticsRepository) AggregateDaily(ctx context.Context, now time.Time, siteTimezone string) error {
	if r == nil || r.db == nil {
		return errors.New("analytics SQL repository is unavailable")
	}
	loc, err := time.LoadLocation(siteTimezone)
	if err != nil {
		return err
	}
	localNow := now.In(loc)
	startDate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)
	endDate := startDate.AddDate(0, 0, 2)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := func(e error) error {
		_ = tx.Rollback()
		return e
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM content_analytics_daily WHERE date >= $1::date AND date < $2::date`, startDate, endDate); err != nil {
		return rollback(err)
	}
	const query = `
WITH bounds AS (
    SELECT (($1::timestamptz AT TIME ZONE $2)::date - 1) AS start_date,
           (($1::timestamptz AT TIME ZONE $2)::date + 1) AS end_date
), aggregated AS (
    SELECT
        (e.event_at AT TIME ZONE $2)::date AS date,
        COALESCE(NULLIF(BTRIM(e.locale), ''), '') AS locale,
        COALESCE(NULLIF(BTRIM(e.content_type), ''), '') AS content_type,
        COALESCE(e.content_id, 0) AS content_id,
        e.page_path,
        COALESCE(NULLIF(BTRIM(e.placement), ''), '') AS placement,
        COUNT(DISTINCT e.session_hash)::integer AS unique_visitors,
        COUNT(*) FILTER (WHERE e.event_name = 'page_view')::integer AS page_views,
        COUNT(*) FILTER (WHERE e.event_name = 'content_read')::integer AS reads,
        COUNT(*) FILTER (WHERE e.event_name = 'content_impression')::integer AS impressions,
        COUNT(*) FILTER (WHERE e.event_name = 'content_click')::integer AS clicks,
        COALESCE(ROUND(AVG(CASE
            WHEN e.event_name = 'content_read'
             AND (e.payload_json->>'read_seconds') ~ '^[0-9]+(\\.[0-9]+)?$'
            THEN (e.payload_json->>'read_seconds')::numeric
            ELSE NULL END))::integer, 0) AS avg_read_seconds,
        CASE WHEN COUNT(*) FILTER (WHERE e.event_name = 'content_impression') > 0
             THEN COUNT(*) FILTER (WHERE e.event_name = 'content_click')::double precision /
                  COUNT(*) FILTER (WHERE e.event_name = 'content_impression')::double precision
             ELSE 0 END AS ctr
    FROM content_analytics_events e, bounds b
    WHERE (e.event_at AT TIME ZONE $2)::date >= b.start_date
      AND (e.event_at AT TIME ZONE $2)::date < b.end_date
    GROUP BY 1, 2, 3, 4, 5, 6
)
INSERT INTO content_analytics_daily
    (date, locale, content_type, content_id, page_path, placement,
     unique_visitors, page_views, reads, impressions, clicks, avg_read_seconds, ctr, updated_at)
SELECT date, locale, content_type, content_id, page_path, placement,
       unique_visitors, page_views, reads, impressions, clicks, avg_read_seconds, ctr, NOW()
FROM aggregated
ON CONFLICT (date, locale, content_type, content_id, page_path, placement)
DO UPDATE SET
    unique_visitors = EXCLUDED.unique_visitors,
    page_views = EXCLUDED.page_views,
    reads = EXCLUDED.reads,
    impressions = EXCLUDED.impressions,
    clicks = EXCLUDED.clicks,
    avg_read_seconds = EXCLUDED.avg_read_seconds,
    ctr = EXCLUDED.ctr,
    updated_at = EXCLUDED.updated_at`
	if _, err := tx.ExecContext(ctx, query, now.UTC(), siteTimezone); err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

var _ service.ContentAnalyticsRepository = (*contentAnalyticsRepository)(nil)
var _ service.ContentAnalyticsDistinctRepository = (*contentAnalyticsRepository)(nil)
