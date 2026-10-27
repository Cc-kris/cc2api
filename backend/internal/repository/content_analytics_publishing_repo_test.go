package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newContentAnalyticsPublishingRepo(t *testing.T) (*contentAnalyticsRepository, *dbent.Client) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:content_analytics_publishing?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	repo, ok := NewContentAnalyticsRepositoryWithTimezone(client, "Asia/Shanghai").(*contentAnalyticsRepository)
	require.True(t, ok)
	return repo, client
}

// TestContentAnalyticsCountPublishedByDayE2E exercises the real ent query:
// only published, non-deleted rows in range are counted, locale filtering uses
// a prefix match, and days are bucketed in the site timezone.
func TestContentAnalyticsCountPublishedByDayE2E(t *testing.T) {
	ctx := context.Background()
	repo, client := newContentAnalyticsPublishingRepo(t)

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	// 2026-10-20 00:30 +08:00 == 2026-10-19 16:30 UTC: must land on 10-20.
	earlyLocal := time.Date(2026, 10, 20, 0, 30, 0, 0, shanghai)

	mkArticle := func(locale, slug, status string, published *time.Time) {
		b := client.ContentArticle.Create().
			SetLocale(locale).
			SetSlug(slug).
			SetTitle("title-" + slug).
			SetTags([]string{}).
			SetStatus(status)
		if published != nil {
			b = b.SetPublishedAt(*published)
		}
		_, err := b.Save(ctx)
		require.NoError(t, err)
	}

	mkArticle("zh-CN", "a-1", "published", &earlyLocal)
	mkArticle("zh-CN", "a-2", "published", &earlyLocal)
	mkArticle("zh-CN", "a-3", "draft", nil)
	mkArticle("en-US", "a-4", "published", &earlyLocal)

	source, err := client.ContentNewsSource.Create().
		SetName("src").
		SetURL("https://example.com/rss").
		Save(ctx)
	require.NoError(t, err)
	mkNews := func(locale, slug string, published *time.Time) {
		_, err := client.ContentNewsItem.Create().
			SetSourceID(source.ID).
			SetSourceCanonicalURL("https://example.com/" + slug).
			SetSlug(slug).
			SetLocale(locale).
			SetTitle("news-" + slug).
			SetStatus("published").
			SetMediaJSON(map[string]interface{}{}).
			SetPublishedAt(*published).
			Save(ctx)
		require.NoError(t, err)
	}
	mkNews("zh-CN", "n-1", &earlyLocal)
	mkNews("en-US", "n-2", &earlyLocal)

	from := time.Date(2026, 10, 19, 0, 0, 0, 0, shanghai)
	to := time.Date(2026, 10, 21, 0, 0, 0, 0, shanghai)

	all, err := repo.CountPublishedByDay(ctx, &from, &to, "")
	require.NoError(t, err)
	require.Equal(t, service.ContentAnalyticsPublishCount{Articles: 3, News: 2}, all["2026-10-20"])
	require.Len(t, all, 1)

	zh, err := repo.CountPublishedByDay(ctx, &from, &to, "zh")
	require.NoError(t, err)
	require.Equal(t, service.ContentAnalyticsPublishCount{Articles: 2, News: 1}, zh["2026-10-20"])

	// Out-of-range window yields nothing rather than leaking counts.
	narrow := time.Date(2026, 10, 25, 0, 0, 0, 0, shanghai)
	empty, err := repo.CountPublishedByDay(ctx, &narrow, &to, "")
	require.NoError(t, err)
	require.Empty(t, empty)
}
