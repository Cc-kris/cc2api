//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPublicContentRepositoryIntegrationNews(t *testing.T) {
	tx := testEntTx(t)
	client := tx.Client()
	ctx := context.Background()
	repo := NewPublicContentRepository(client)
	svc := service.NewPublicContentService(repo)

	source, err := client.ContentNewsSource.Create().
		SetName("integration source").
		SetSourceType("manual").
		Save(ctx)
	require.NoError(t, err)

	publishedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	createNews := func(slug, locale, status string, at time.Time) *dbent.ContentNewsItem {
		item, err := client.ContentNewsItem.Create().
			SetSourceID(source.ID).
			SetSourceCanonicalURL("https://example.com/" + slug).
			SetSlug(slug).
			SetLocale(locale).
			SetTitle("Title " + slug).
			SetSummary("Summary " + slug).
			SetBodyHTML("<p>Body " + slug + "</p>").
			SetBodyText("Body " + slug).
			SetMediaJSON(map[string]interface{}{}).
			SetSourceName("Example").
			SetPublishedAt(at).
			SetStatus(status).
			Save(ctx)
		require.NoError(t, err)
		return item
	}

	newer := createNews("news-newer", "zh-CN", domain.ContentStatusPublished, publishedAt.Add(time.Hour))
	tieLow := createNews("news-tie-low", "zh-CN", domain.ContentStatusPublished, publishedAt)
	tieHigh := createNews("news-tie-high", "zh-CN", domain.ContentStatusPublished, publishedAt)
	older := createNews("news-older", "zh-CN", domain.ContentStatusPublished, publishedAt.Add(-time.Hour))
	createNews("news-wrong-locale", "en-US", domain.ContentStatusPublished, publishedAt.Add(time.Hour))
	createNews("news-draft", "zh-CN", domain.ContentStatusDraft, publishedAt.Add(time.Hour))
	deleted := createNews("news-deleted", "zh-CN", domain.ContentStatusPublished, publishedAt.Add(time.Hour))
	_, err = client.ContentNewsItem.UpdateOneID(deleted.ID).SetDeletedAt(time.Now()).Save(ctx)
	require.NoError(t, err)

	page, err := svc.ListNews(ctx, " zh-CN ", 1, 2)
	require.NoError(t, err)
	require.Equal(t, 4, page.Total)
	require.Equal(t, 1, page.Page)
	require.Equal(t, 2, page.PageSize)
	require.True(t, page.HasNext)
	require.Equal(t, []string{newer.Slug, tieHigh.Slug}, []string{page.Items[0].Slug, page.Items[1].Slug})

	page, err = svc.ListNews(ctx, "zh-CN", 2, 2)
	require.NoError(t, err)
	require.False(t, page.HasNext)
	require.Equal(t, []string{tieLow.Slug, older.Slug}, []string{page.Items[0].Slug, page.Items[1].Slug})

	empty, err := svc.ListNews(ctx, "zh-CN", 3, 2)
	require.NoError(t, err)
	require.Empty(t, empty.Items)
	require.Equal(t, 4, empty.Total)
	require.False(t, empty.HasNext)

	detail, err := svc.GetNews(ctx, "zh-CN", "news-newer")
	require.NoError(t, err)
	require.Equal(t, "<p>Body news-newer</p>", detail.BodyHTML)
	require.Equal(t, "Example", detail.SourceName)
	require.Nil(t, detail.DeletedAt)
	_, err = svc.GetNews(ctx, "en-US", "news-newer")
	require.ErrorIs(t, err, service.ErrPublicContentNotFound)
	_, err = svc.GetNews(ctx, "zh-CN", "news-deleted")
	require.ErrorIs(t, err, service.ErrPublicContentNotFound)

	payload, err := json.Marshal(page.Items)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "bodyHtml")
	require.NotContains(t, string(payload), "status")
	require.NotContains(t, string(payload), "deletedAt")
}

func TestPublicContentRepositoryIntegrationArticles(t *testing.T) {
	tx := testEntTx(t)
	client := tx.Client()
	ctx := context.Background()
	svc := service.NewPublicContentService(NewPublicContentRepository(client))
	publishedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

	createArticle := func(slug, title string, body *string, status, locale string, at *time.Time, deleted bool) *dbent.ContentArticle {
		create := client.ContentArticle.Create().
			SetSlug(slug).
			SetTitle(title).
			SetExcerpt("Excerpt " + slug).
			SetLocale(locale).
			SetStatus(status).
			SetTags([]string{"integration"})
		if body != nil {
			create.SetBodyHTML(*body)
		}
		if at != nil {
			create.SetPublishedAt(*at)
		}
		if deleted {
			create.SetDeletedAt(time.Now())
		}
		item, err := create.Save(ctx)
		require.NoError(t, err)
		return item
	}

	newerAt := publishedAt.Add(time.Hour)
	olderAt := publishedAt.Add(-time.Hour)
	newerBody := "<p>newer body</p>"
	olderBody := "<p>older body</p>"
	tieBody := "<p>tie body</p>"
	newer := createArticle("article-newer", "Newer", &newerBody, domain.ContentStatusPublished, "zh-CN", &newerAt, false)
	tieLow := createArticle("article-tie-low", "Tie low", &tieBody, domain.ContentStatusPublished, "zh-CN", &publishedAt, false)
	tieHigh := createArticle("article-tie-high", "Tie high", &tieBody, domain.ContentStatusPublished, "zh-CN", &publishedAt, false)
	older := createArticle("article-older", "Older", &olderBody, domain.ContentStatusPublished, "zh-CN", &olderAt, false)
	draftBody := "<p>draft</p>"
	enBody := "<p>en</p>"
	deletedBody := "<p>deleted</p>"
	createArticle("article-draft", "Draft", &draftBody, domain.ContentStatusDraft, "zh-CN", &publishedAt, false)
	createArticle("article-en", "English", &enBody, domain.ContentStatusPublished, "en-US", &publishedAt, false)
	createArticle("article-deleted", "Deleted", &deletedBody, domain.ContentStatusPublished, "zh-CN", &publishedAt, true)
	createArticle("article-null-body", "Null body", nil, domain.ContentStatusPublished, "zh-CN", &publishedAt, false)
	emptyBody := ""
	blankBody := "   "
	createArticle("article-empty-body", "Empty body", &emptyBody, domain.ContentStatusPublished, "zh-CN", &publishedAt, false)
	createArticle("article-blank-body", "Blank body", &blankBody, domain.ContentStatusPublished, "zh-CN", &publishedAt, false)
	createArticle("article-no-date", "No date", &olderBody, domain.ContentStatusDraft, "zh-CN", nil, false)
	createArticle("article-blank-title", "   ", &olderBody, domain.ContentStatusPublished, "zh-CN", &publishedAt, false)
	createArticle("   ", "Blank slug", &olderBody, domain.ContentStatusPublished, "zh-CN", &publishedAt, false)

	page, err := svc.ListArticles(ctx, "zh-CN", 1, 2)
	require.NoError(t, err)
	require.Equal(t, 4, page.Total)
	require.True(t, page.HasNext)
	require.Equal(t, []string{newer.Slug, tieHigh.Slug}, []string{page.Items[0].Slug, page.Items[1].Slug})

	page, err = svc.ListArticles(ctx, "zh-CN", 2, 2)
	require.NoError(t, err)
	require.False(t, page.HasNext)
	require.Equal(t, []string{tieLow.Slug, older.Slug}, []string{page.Items[0].Slug, page.Items[1].Slug})

	empty, err := svc.ListArticles(ctx, "zh-CN", 3, 2)
	require.NoError(t, err)
	require.Empty(t, empty.Items)
	require.Equal(t, 4, empty.Total)

	detail, err := svc.GetArticle(ctx, " zh-CN ", " article-newer ")
	require.NoError(t, err)
	require.Equal(t, "<p>newer body</p>", detail.BodyHTML)
	require.Equal(t, []string{"integration"}, detail.Tags)
	_, err = svc.GetArticle(ctx, "zh-CN", "article-empty-body")
	require.ErrorIs(t, err, service.ErrPublicContentNotFound)
	_, err = svc.GetArticle(ctx, "zh-CN", "article-deleted")
	require.ErrorIs(t, err, service.ErrPublicContentNotFound)

	payload, err := json.Marshal(page.Items)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "bodyHtml")
	require.NotContains(t, string(payload), "status")
	require.NotContains(t, string(payload), "deletedAt")
	detailPayload, err := json.Marshal(detail)
	require.NoError(t, err)
	require.Equal(t, "<p>newer body</p>", detail.BodyHTML)
	require.NotContains(t, string(detailPayload), "newer body")
	require.NotContains(t, string(detailPayload), "model")
	require.NotContains(t, string(detailPayload), "authorId")
}
