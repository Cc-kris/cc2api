package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

const (
	DefaultPublicContentPageSize = 12
	MaxPublicContentPageSize     = 100
	MaxPublicFeaturedArticles    = 10
)

var ErrPublicContentNotFound = errors.New("public content not found")
var ErrPublicContentGone = errors.New("public content gone")
var ErrPublicContentRepositoryContract = errors.New("public content repository contract violation")

// PublicNews is the public projection of a published news item. It deliberately
// excludes source URLs, source IDs, audit fields, and fetch diagnostics.
type PublicNews struct {
	ID      int64  `json:"id"`
	Locale  string `json:"locale"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	// BodyHTML is consumed by the server-side renderer only. Anonymous JSON
	// clients must not receive raw captured HTML, because they could otherwise
	// render it unsafely outside the server sanitizer.
	BodyHTML            string            `json:"-"`
	BodyText            string            `json:"bodyText,omitempty"`
	SourceName          string            `json:"sourceName"`
	Category            string            `json:"category,omitempty"`
	Tags                []string          `json:"tags,omitempty"`
	TitleZh             string            `json:"titleZh,omitempty"`
	SummaryZh           string            `json:"summaryZh,omitempty"`
	BodyTextZh          string            `json:"bodyTextZh,omitempty"`
	CategoryZh          string            `json:"categoryZh,omitempty"`
	TitleEn             string            `json:"titleEn,omitempty"`
	SummaryEn           string            `json:"summaryEn,omitempty"`
	BodyTextEn          string            `json:"bodyTextEn,omitempty"`
	CategoryEn          string            `json:"categoryEn,omitempty"`
	OriginalPublishedAt *time.Time        `json:"originalPublishedAt,omitempty"`
	PublishedAt         time.Time         `json:"publishedAt"`
	ImageURL            string            `json:"imageUrl,omitempty"`
	SeoTitle            string            `json:"seoTitle,omitempty"`
	SeoDescription      string            `json:"seoDescription,omitempty"`
	Status              string            `json:"-"`
	DeletedAt           *time.Time        `json:"-"`
	Alternates          []PublicAlternate `json:"alternates,omitempty"`
}

type PublicNewsListItem struct {
	ID                  int64             `json:"id"`
	Locale              string            `json:"locale"`
	Slug                string            `json:"slug"`
	Title               string            `json:"title"`
	Summary             string            `json:"summary"`
	SourceName          string            `json:"sourceName"`
	Category            string            `json:"category,omitempty"`
	Tags                []string          `json:"tags,omitempty"`
	TitleZh             string            `json:"titleZh,omitempty"`
	SummaryZh           string            `json:"summaryZh,omitempty"`
	CategoryZh          string            `json:"categoryZh,omitempty"`
	TitleEn             string            `json:"titleEn,omitempty"`
	SummaryEn           string            `json:"summaryEn,omitempty"`
	CategoryEn          string            `json:"categoryEn,omitempty"`
	OriginalPublishedAt *time.Time        `json:"originalPublishedAt,omitempty"`
	PublishedAt         time.Time         `json:"publishedAt"`
	ImageURL            string            `json:"imageUrl,omitempty"`
	SeoTitle            string            `json:"seoTitle,omitempty"`
	SeoDescription      string            `json:"seoDescription,omitempty"`
	Status              string            `json:"-"`
	DeletedAt           *time.Time        `json:"-"`
	Alternates          []PublicAlternate `json:"alternates,omitempty"`
}

type PublicArticle struct {
	ID      int64  `json:"id"`
	Locale  string `json:"locale"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"`
	Author  string `json:"author,omitempty"`
	// See PublicNews.BodyHTML. Public article HTML is rendered server-side.
	BodyHTML       string            `json:"-"`
	BodyText       string            `json:"bodyText,omitempty"`
	CoverImage     string            `json:"coverImage,omitempty"`
	CoverAlt       string            `json:"coverAlt,omitempty"`
	Category       string            `json:"category,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	SeoTitle       string            `json:"seoTitle,omitempty"`
	SeoDescription string            `json:"seoDescription,omitempty"`
	CanonicalURL   string            `json:"-"`
	OgImage        string            `json:"ogImage,omitempty"`
	PublishedAt    time.Time         `json:"publishedAt"`
	Status         string            `json:"-"`
	DeletedAt      *time.Time        `json:"-"`
	Alternates     []PublicAlternate `json:"alternates,omitempty"`
}

type PublicAlternate struct {
	Locale string `json:"locale"`
	Slug   string `json:"slug"`
}

type PublicArticleListItem struct {
	ID          int64      `json:"id"`
	Locale      string     `json:"locale"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Excerpt     string     `json:"excerpt"`
	CoverImage  string     `json:"coverImage,omitempty"`
	CoverAlt    string     `json:"coverAlt,omitempty"`
	Category    string     `json:"category,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	PublishedAt time.Time  `json:"publishedAt"`
	Status      string     `json:"-"`
	DeletedAt   *time.Time `json:"-"`
}

type PublicNewsPage struct {
	Items    []PublicNewsListItem `json:"items"`
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"pageSize"`
	HasNext  bool                 `json:"hasNext"`
}

type PublicArticlePage struct {
	Items    []PublicArticleListItem `json:"items"`
	Total    int                     `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"pageSize"`
	HasNext  bool                    `json:"hasNext"`
}

// PublicFeaturedArticlesRepository exposes only articles selected through the
// featured_at source of truth. Implementations return at most the requested limit.
type PublicFeaturedArticlesRepository interface {
	ListFeaturedArticles(ctx context.Context, locale string, limit int) ([]PublicArticleListItem, error)
}

// PublicContentRepository is the narrow read port used by anonymous content
// services. Implementations must query the published projection only.
type PublicContentRepository interface {
	ListPublishedNews(ctx context.Context, locale string, limit, offset int) (PublicNewsPage, error)
	GetPublishedNews(ctx context.Context, locale, slug string) (*PublicNews, error)
	ListPublishedArticles(ctx context.Context, locale string, limit, offset int) (PublicArticlePage, error)
	GetPublishedArticle(ctx context.Context, locale, slug string) (*PublicArticle, error)
}

// PublicContentSitemapRepository is implemented by repositories that can
// enumerate every published locale/slug without exposing management fields.
type PublicContentSitemapRepository interface {
	ListPublishedNewsForSitemap(ctx context.Context) ([]PublicNewsListItem, error)
	ListPublishedArticlesForSitemap(ctx context.Context) ([]PublicArticleListItem, error)
}

type PublicSitemapEntry struct {
	Kind         string
	Locale       string
	Slug         string
	LastModified time.Time
}

type PublicContentService struct {
	repo PublicContentRepository
}

func NewPublicContentService(repo PublicContentRepository) *PublicContentService {
	return &PublicContentService{repo: repo}
}

func (s *PublicContentService) ListSitemapEntries(ctx context.Context) ([]PublicSitemapEntry, error) {
	repo, ok := s.repo.(PublicContentSitemapRepository)
	if !ok {
		return nil, ErrPublicContentRepositoryContract
	}
	news, err := repo.ListPublishedNewsForSitemap(ctx)
	if err != nil {
		return nil, err
	}
	articles, err := repo.ListPublishedArticlesForSitemap(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]PublicSitemapEntry, 0, len(news)+len(articles))
	for _, item := range news {
		if strings.TrimSpace(item.Locale) != "" && isPublicPublishedNewsListItem(item, item.Locale) {
			entries = append(entries, PublicSitemapEntry{Kind: "news", Locale: item.Locale, Slug: item.Slug, LastModified: item.PublishedAt})
		}
	}
	for _, item := range articles {
		if strings.TrimSpace(item.Locale) != "" && isPublicPublishedArticleListItem(item, item.Locale) {
			entries = append(entries, PublicSitemapEntry{Kind: "article", Locale: item.Locale, Slug: item.Slug, LastModified: item.PublishedAt})
		}
	}
	return entries, nil
}

func (s *PublicContentService) ListNews(ctx context.Context, locale string, page, pageSize int) (PublicNewsPage, error) {
	locale, err := normalizePublicLocale(locale)
	if err != nil {
		return PublicNewsPage{}, err
	}
	limit, offset, err := normalizePublicPage(page, pageSize)
	if err != nil {
		return PublicNewsPage{}, err
	}
	result, err := s.repo.ListPublishedNews(ctx, locale, limit, offset)
	if err != nil {
		return PublicNewsPage{}, err
	}
	if !allPublicNewsListItemsValid(locale, result.Items) {
		return PublicNewsPage{}, ErrPublicContentRepositoryContract
	}
	for i := range result.Items {
		result.Items[i].ImageURL = publicLocalMediaPath(result.Items[i].ImageURL)
	}
	result.Page, result.PageSize, result.HasNext = offset/limit+1, limit, result.Total > offset && result.Total-offset > len(result.Items)
	return result, nil
}

func (s *PublicContentService) GetNews(ctx context.Context, locale, slug string) (*PublicNews, error) {
	locale, slug, err := normalizePublicContentKey(locale, slug)
	if err != nil {
		return nil, err
	}
	item, err := s.findPublishedNews(ctx, locale, slug)
	if err != nil {
		return nil, err
	}
	item.ImageURL = publicLocalMediaPath(item.ImageURL)
	return item, nil
}

// findPublishedNews resolves a slug across locale aliases (zh vs zh-CN, en vs
// en-US) so a shared deep link keeps working regardless of which spelling the
// row was written with.
func (s *PublicContentService) findPublishedNews(ctx context.Context, locale, slug string) (*PublicNews, error) {
	for _, candidate := range domain.ContentLocaleCandidates(locale) {
		item, err := s.repo.GetPublishedNews(ctx, candidate, slug)
		if err != nil {
			if errors.Is(err, ErrPublicContentNotFound) {
				continue
			}
			return nil, err
		}
		if item == nil || item.Slug != slug || !isPublicPublishedNews(*item) {
			continue
		}
		return item, nil
	}
	return nil, ErrPublicContentNotFound
}

func (s *PublicContentService) ListArticles(ctx context.Context, locale string, page, pageSize int) (PublicArticlePage, error) {
	locale, err := normalizePublicLocale(locale)
	if err != nil {
		return PublicArticlePage{}, err
	}
	limit, offset, err := normalizePublicPage(page, pageSize)
	if err != nil {
		return PublicArticlePage{}, err
	}
	result, err := s.repo.ListPublishedArticles(ctx, locale, limit, offset)
	if err != nil {
		return PublicArticlePage{}, err
	}
	if !allPublicArticleListItemsValid(locale, result.Items) {
		return PublicArticlePage{}, ErrPublicContentRepositoryContract
	}
	for i := range result.Items {
		result.Items[i].CoverImage = publicLocalMediaPath(result.Items[i].CoverImage)
	}
	result.Page, result.PageSize, result.HasNext = offset/limit+1, limit, result.Total > offset && result.Total-offset > len(result.Items)
	return result, nil
}

// ListFeaturedArticles returns the selected public articles, newest selection first.
// An empty slice is a valid result and lets clients hide the section entirely.
func (s *PublicContentService) ListFeaturedArticles(ctx context.Context, locale string) ([]PublicArticleListItem, error) {
	locale, err := normalizePublicLocale(locale)
	if err != nil {
		return nil, err
	}
	repo, ok := s.repo.(PublicFeaturedArticlesRepository)
	if !ok {
		return nil, ErrPublicContentRepositoryContract
	}
	items, err := repo.ListFeaturedArticles(ctx, locale, MaxPublicFeaturedArticles)
	if err != nil {
		return nil, err
	}
	if len(items) > MaxPublicFeaturedArticles || !allPublicArticleListItemsValid(locale, items) {
		return nil, ErrPublicContentRepositoryContract
	}
	for i := range items {
		items[i].CoverImage = publicLocalMediaPath(items[i].CoverImage)
	}
	if items == nil {
		items = []PublicArticleListItem{}
	}
	return items, nil
}

func (s *PublicContentService) GetArticle(ctx context.Context, locale, slug string) (*PublicArticle, error) {
	locale, slug, err := normalizePublicContentKey(locale, slug)
	if err != nil {
		return nil, err
	}
	item, err := s.findPublishedArticle(ctx, locale, slug)
	if err != nil {
		return nil, err
	}
	item.CoverImage = publicLocalMediaPath(item.CoverImage)
	item.OgImage = publicLocalMediaPath(item.OgImage)
	item.CanonicalURL = ""
	if strings.TrimSpace(item.Author) == "" {
		item.Author = "CCAI 编辑部"
	}
	return item, nil
}

func (s *PublicContentService) findPublishedArticle(ctx context.Context, locale, slug string) (*PublicArticle, error) {
	for _, candidate := range domain.ContentLocaleCandidates(locale) {
		item, err := s.repo.GetPublishedArticle(ctx, candidate, slug)
		if err != nil {
			if errors.Is(err, ErrPublicContentNotFound) {
				continue
			}
			return nil, err
		}
		if item == nil || item.Slug != slug || !isPublicPublishedArticle(*item) {
			continue
		}
		return item, nil
	}
	return nil, ErrPublicContentNotFound
}

// publicLocalMediaPath prevents anonymous JSON clients from being given source
// URLs or arbitrary remote media URLs. The fetch pipeline may expose only a
// CCAI-owned local media path in the public projection.
func publicLocalMediaPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n\\") || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return ""
	}
	return raw
}

func normalizePublicPage(page, pageSize int) (int, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = DefaultPublicContentPageSize
	}
	if pageSize < 1 || pageSize > MaxPublicContentPageSize {
		return 0, 0, errors.New("invalid public content page size")
	}
	if page-1 > int(^uint(0)>>1)/pageSize {
		return 0, 0, errors.New("invalid public content page")
	}
	return pageSize, (page - 1) * pageSize, nil
}

func normalizePublicContentKey(locale, slug string) (string, string, error) {
	locale, err := normalizePublicLocale(locale)
	if err != nil {
		return "", "", err
	}
	slug = strings.TrimSpace(slug)
	if slug == "" || len(slug) > 80 || strings.ContainsAny(slug, "/?#\\") {
		return "", "", errors.New("invalid public content slug")
	}
	return locale, slug, nil
}

func normalizePublicLocale(locale string) (string, error) {
	locale = strings.TrimSpace(locale)
	if locale == "" || len(locale) > 16 || strings.ContainsAny(locale, "/?#") {
		return "", errors.New("invalid public content locale")
	}
	return locale, nil
}

func isPublicPublishedNews(item PublicNews) bool {
	return strings.TrimSpace(item.Locale) != "" && strings.TrimSpace(item.Slug) != "" &&
		strings.TrimSpace(item.Title) != "" && strings.TrimSpace(item.Summary) != "" &&
		strings.TrimSpace(item.SourceName) != "" && strings.TrimSpace(item.BodyHTML) != "" &&
		!item.PublishedAt.IsZero() && item.Status == domain.ContentStatusPublished && item.DeletedAt == nil
}

func isPublicPublishedArticle(item PublicArticle) bool {
	return strings.TrimSpace(item.Locale) != "" && strings.TrimSpace(item.Slug) != "" &&
		strings.TrimSpace(item.Title) != "" && strings.TrimSpace(item.Excerpt) != "" &&
		strings.TrimSpace(item.BodyHTML) != "" &&
		!item.PublishedAt.IsZero() && item.Status == domain.ContentStatusPublished && item.DeletedAt == nil
}

func isPublicPublishedNewsListItem(item PublicNewsListItem, locale string) bool {
	return strings.TrimSpace(item.Locale) == locale && strings.TrimSpace(item.Slug) != "" &&
		strings.TrimSpace(item.Title) != "" && strings.TrimSpace(item.Summary) != "" &&
		strings.TrimSpace(item.SourceName) != "" && !item.PublishedAt.IsZero() &&
		item.Status == domain.ContentStatusPublished && item.DeletedAt == nil
}

func isPublicPublishedArticleListItem(item PublicArticleListItem, locale string) bool {
	return strings.TrimSpace(item.Locale) == locale && strings.TrimSpace(item.Slug) != "" &&
		strings.TrimSpace(item.Title) != "" && strings.TrimSpace(item.Excerpt) != "" &&
		!item.PublishedAt.IsZero() &&
		item.Status == domain.ContentStatusPublished && item.DeletedAt == nil
}

func allPublicNewsListItemsValid(locale string, items []PublicNewsListItem) bool {
	for _, item := range items {
		if !isPublicPublishedNewsListItem(item, locale) {
			return false
		}
	}
	return true
}

func allPublicArticleListItemsValid(locale string, items []PublicArticleListItem) bool {
	for _, item := range items {
		if !isPublicPublishedArticleListItem(item, locale) {
			return false
		}
	}
	return true
}
