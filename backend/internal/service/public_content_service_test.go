package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type publicContentRepoStub struct {
	news           []PublicNewsListItem
	articles       []PublicArticleListItem
	detailNews     []PublicNews
	detailArticles []PublicArticle
}

func (r *publicContentRepoStub) ListPublishedNews(context.Context, string, int, int) (PublicNewsPage, error) {
	return PublicNewsPage{Items: r.news, Total: len(r.news)}, nil
}

func (r *publicContentRepoStub) GetPublishedNews(_ context.Context, locale, slug string) (*PublicNews, error) {
	for i := range r.detailNews {
		if r.detailNews[i].Locale == locale && r.detailNews[i].Slug == slug {
			return &r.detailNews[i], nil
		}
	}
	return nil, ErrPublicContentNotFound
}

func (r *publicContentRepoStub) ListPublishedArticles(context.Context, string, int, int) (PublicArticlePage, error) {
	return PublicArticlePage{Items: r.articles, Total: len(r.articles)}, nil
}

func (r *publicContentRepoStub) GetPublishedArticle(_ context.Context, locale, slug string) (*PublicArticle, error) {
	for i := range r.detailArticles {
		if r.detailArticles[i].Locale == locale && r.detailArticles[i].Slug == slug {
			return &r.detailArticles[i], nil
		}
	}
	return nil, ErrPublicContentNotFound
}

func TestPublicContentServiceFiltersNonPublishedNews(t *testing.T) {
	publishedAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	service := NewPublicContentService(&publicContentRepoStub{news: []PublicNewsListItem{
		{Locale: "zh-CN", Slug: "published", Title: "Published", Summary: "Summary", SourceName: "Source", PublishedAt: publishedAt, Status: "published"},
		{Locale: "en-US", Slug: "wrong-locale", Title: "Wrong locale", PublishedAt: publishedAt, Status: "published"},
		{Locale: "zh-CN", Slug: "draft", Title: "Draft", PublishedAt: publishedAt, Status: "draft"},
		{Locale: "zh-CN", Slug: "deleted", Title: "Deleted", PublishedAt: publishedAt, Status: "published", DeletedAt: &publishedAt},
	}})

	_, err := service.ListNews(context.Background(), "zh-CN", 1, 12)
	require.ErrorIs(t, err, ErrPublicContentRepositoryContract)
}

func TestPublicContentServiceRejectsIncompletePublicLists(t *testing.T) {
	publishedAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	newsSvc := NewPublicContentService(&publicContentRepoStub{news: []PublicNewsListItem{{
		Locale: "zh-CN", Slug: "missing-summary", Title: "Title", SourceName: "Source", PublishedAt: publishedAt, Status: "published",
	}}})
	_, err := newsSvc.ListNews(context.Background(), "zh-CN", 1, 12)
	require.ErrorIs(t, err, ErrPublicContentRepositoryContract)

	articleSvc := NewPublicContentService(&publicContentRepoStub{articles: []PublicArticleListItem{{
		Locale: "zh-CN", Slug: "missing-excerpt", Title: "Title", PublishedAt: publishedAt, Status: "published",
	}}})
	_, err = articleSvc.ListArticles(context.Background(), "zh-CN", 1, 12)
	require.ErrorIs(t, err, ErrPublicContentRepositoryContract)
}

func TestPublicContentServiceListMetadata(t *testing.T) {
	publishedAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	svc := NewPublicContentService(&publicContentRepoStub{news: []PublicNewsListItem{
		{Locale: "zh-CN", Slug: "one", Title: "One", Summary: "Summary", SourceName: "Source", PublishedAt: publishedAt, Status: "published"},
	}})
	page, err := svc.ListNews(context.Background(), "zh-CN", 2, 12)
	require.NoError(t, err)
	require.Equal(t, 2, page.Page)
	require.Equal(t, 12, page.PageSize)
	require.False(t, page.HasNext)
}

func TestPublicContentServiceRejectsNonPublishedArticle(t *testing.T) {
	publishedAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	service := NewPublicContentService(&publicContentRepoStub{detailArticles: []PublicArticle{
		{Locale: "en-US", Slug: "draft", Title: "Draft", Excerpt: "Excerpt", BodyHTML: "<p>draft</p>", PublishedAt: publishedAt, Status: "draft"},
	}})

	_, err := service.GetArticle(context.Background(), "en-US", "draft")
	require.ErrorIs(t, err, ErrPublicContentNotFound)
}

func TestPublicContentServiceRejectsRepositoryKeyMismatch(t *testing.T) {
	publishedAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	service := NewPublicContentService(&publicContentRepoStub{detailNews: []PublicNews{
		{Locale: "en-US", Slug: "other", Title: "Other", Summary: "Summary", SourceName: "Source", BodyHTML: "<p>body</p>", PublishedAt: publishedAt, Status: "published"},
	}})

	_, err := service.GetNews(context.Background(), "en-US", "requested")
	require.ErrorIs(t, err, ErrPublicContentNotFound)
}

func TestPublicContentServiceValidatesLocaleSlugAndPageSize(t *testing.T) {
	service := NewPublicContentService(&publicContentRepoStub{})

	_, err := service.ListNews(context.Background(), "", 1, 12)
	require.Error(t, err)
	_, err = service.GetNews(context.Background(), "zh-CN", "")
	require.Error(t, err)
	_, err = service.ListArticles(context.Background(), "zh-CN", 1, MaxPublicContentPageSize+1)
	require.Error(t, err)
	_, err = service.ListArticles(context.Background(), "zh-CN", int(^uint(0)>>1), 100)
	require.Error(t, err)
}

func TestPublicContentServicePreservesRepositoryErrors(t *testing.T) {
	expected := errors.New("database unavailable")
	repo := &errorPublicContentRepo{err: expected}
	service := NewPublicContentService(repo)

	_, err := service.ListNews(context.Background(), "zh-CN", 1, 12)
	require.ErrorIs(t, err, expected)
}

func TestPublicContentProjectionDoesNotExposeInternalFields(t *testing.T) {
	item := PublicNews{
		Locale:   "zh-CN",
		Slug:     "public-news",
		Title:    "Public news",
		BodyHTML: "<p>body</p>",
		Status:   "published",
	}
	payload, err := json.Marshal(item)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "status")
	require.NotContains(t, string(payload), "deletedAt")
	require.NotContains(t, string(payload), "sourceCanonicalUrl")
	require.NotContains(t, string(payload), "bodyHtml")
	articlePayload, err := json.Marshal(PublicArticle{
		Locale:   "zh-CN",
		Slug:     "article",
		Title:    "Article",
		BodyHTML: "<p>body</p>",
	})
	require.NoError(t, err)
	require.NotContains(t, string(articlePayload), "version")
	require.NotContains(t, string(articlePayload), "bodyHtml")
	listPayload, err := json.Marshal(PublicArticleListItem{Locale: "zh-CN", Slug: "article", Title: "Article"})
	require.NoError(t, err)
	require.NotContains(t, string(listPayload), "bodyHtml")
}

func TestPublicContentProjectionRedactsRemoteMediaAndCanonicalURL(t *testing.T) {
	publishedAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	svc := NewPublicContentService(&publicContentRepoStub{
		detailNews:     []PublicNews{{Locale: "zh-CN", Slug: "news", Title: "News", Summary: "Summary", SourceName: "Source", BodyHTML: "<p>body</p>", PublishedAt: publishedAt, Status: "published", ImageURL: `/\\evil.example/image.png`}},
		detailArticles: []PublicArticle{{Locale: "zh-CN", Slug: "article", Title: "Article", Excerpt: "Excerpt", BodyHTML: "<p>body</p>", PublishedAt: publishedAt, Status: "published", CoverImage: "https://source.example/cover.png", OgImage: "//source.example/og.png", CanonicalURL: "https://source.example/article"}},
	})

	news, err := svc.GetNews(context.Background(), "zh-CN", "news")
	require.NoError(t, err)
	require.Empty(t, news.ImageURL)
	article, err := svc.GetArticle(context.Background(), "zh-CN", "article")
	require.NoError(t, err)
	require.Empty(t, article.CoverImage)
	require.Empty(t, article.OgImage)
	require.Empty(t, article.CanonicalURL)
	require.Equal(t, "CCAI 编辑部", article.Author)
	newsPayload, err := json.Marshal(news)
	require.NoError(t, err)
	require.NotContains(t, string(newsPayload), "source.example")
	articlePayload, err := json.Marshal(article)
	require.NoError(t, err)
	require.NotContains(t, string(articlePayload), "source.example")
	require.NotContains(t, string(articlePayload), "canonicalUrl")
}

type errorPublicContentRepo struct{ err error }

func (r *errorPublicContentRepo) ListPublishedNews(context.Context, string, int, int) (PublicNewsPage, error) {
	return PublicNewsPage{}, r.err
}
func (r *errorPublicContentRepo) GetPublishedNews(context.Context, string, string) (*PublicNews, error) {
	return nil, r.err
}
func (r *errorPublicContentRepo) ListPublishedArticles(context.Context, string, int, int) (PublicArticlePage, error) {
	return PublicArticlePage{}, r.err
}
func (r *errorPublicContentRepo) GetPublishedArticle(context.Context, string, string) (*PublicArticle, error) {
	return nil, r.err
}
