package repository

import (
	"context"
	"html"
	"sort"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/contentarticle"
	"github.com/Wei-Shaw/sub2api/ent/contentnewsitem"
	"github.com/Wei-Shaw/sub2api/ent/contenttranslation"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/contentmd"
	"github.com/Wei-Shaw/sub2api/internal/pkg/richtext"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type publicContentRepository struct {
	client *dbent.Client
}

func NewPublicContentRepository(client *dbent.Client) service.PublicContentRepository {
	return &publicContentRepository{client: client}
}

func (r *publicContentRepository) ListPublishedNews(ctx context.Context, locale string, limit, offset int) (service.PublicNewsPage, error) {
	client := clientFromContext(ctx, r.client)
	requested := strings.TrimSpace(locale)
	predicates := publicNewsPredicates("")
	if requested != "" {
		predicates = append(predicates, contentnewsitem.LocaleIn(requested, service.PublicContentFallbackLocale))
	}
	rows, err := client.ContentNewsItem.Query().Where(predicates...).Order(
		contentnewsitem.ByPublishedAt(sql.OrderDesc()),
		contentnewsitem.ByID(sql.OrderDesc()),
	).Limit(publicContentMaxListCandidates + 1).All(ctx)
	if err != nil {
		return service.PublicNewsPage{}, err
	}
	if len(rows) > publicContentMaxListCandidates {
		return service.PublicNewsPage{}, service.ErrPublicContentRepositoryContract
	}
	items := publicNewsListItems(rows, requested)
	total := len(items)
	start, end := publicPageBounds(total, limit, offset)
	return service.PublicNewsPage{Items: items[start:end], Total: total}, nil
}

func publicNewsMainContentKey(item *dbent.ContentNewsItem) string {
	if canonical := strings.TrimSpace(item.SourceCanonicalURL); canonical != "" {
		return "canonical:" + canonical
	}
	return "id:" + strconv.FormatInt(item.ID, 10)
}

// publicNewsRowPreferred ranks rows sharing a canonical URL: requested locale
// beats the editorial fallback locale, then fresher rows win.
func publicNewsRowPreferred(candidate, current *dbent.ContentNewsItem, requested string) bool {
	score := func(item *dbent.ContentNewsItem) int {
		if requested != "" && item.Locale == requested {
			return 2
		}
		if item.Locale == service.PublicContentFallbackLocale {
			return 1
		}
		return 0
	}
	if cs, ps := score(candidate), score(current); cs != ps {
		return cs > ps
	}
	if candidate.PublishedAt == nil {
		return false
	}
	if current.PublishedAt == nil {
		return true
	}
	if !candidate.PublishedAt.Equal(*current.PublishedAt) {
		return candidate.PublishedAt.After(*current.PublishedAt)
	}
	return candidate.ID > current.ID
}

// publicNewsListItems folds per-locale rows into one entry per main content
// (source canonical URL), preferring the requested locale and then the
// editorial fallback language, so list totals count stories rather than
// locale variants. Items come back newest-first with deterministic ties.
func publicNewsListItems(rows []*dbent.ContentNewsItem, requested string) []service.PublicNewsListItem {
	chosen := make(map[string]*dbent.ContentNewsItem, len(rows))
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		key := publicNewsMainContentKey(row)
		current, seen := chosen[key]
		if !seen {
			chosen[key] = row
			order = append(order, key)
			continue
		}
		if publicNewsRowPreferred(row, current, requested) {
			chosen[key] = row
		}
	}
	items := make([]service.PublicNewsListItem, 0, len(order))
	for _, key := range order {
		mapped := mapPublicNewsList([]*dbent.ContentNewsItem{chosen[key]})
		if len(mapped) == 0 {
			continue
		}
		items = append(items, mapped[0])
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].PublishedAt.Equal(items[j].PublishedAt) {
			if items[i].ID == items[j].ID {
				return items[i].Slug < items[j].Slug
			}
			return items[i].ID > items[j].ID
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
	return items
}

// publicArticleTranslationItem builds the public list row for a published
// translation, borrowing cover/category/tags from the parent article. It
// reports false when the translation is missing required SEO fields so the
// caller keeps the base row rather than publishing a half-translated entry.
func publicArticleTranslationItem(parent *dbent.ContentArticle, translation *dbent.ContentTranslation) (service.PublicArticleListItem, bool) {
	if parent == nil || translation == nil {
		return service.PublicArticleListItem{}, false
	}
	if publicDerefString(translation.Excerpt) == "" || publicDerefString(translation.SeoTitle) == "" || publicDerefString(translation.SeoDescription) == "" || (publicDerefString(translation.BodyHTML) == "" && publicDerefString(translation.BodyMarkdown) == "") {
		return service.PublicArticleListItem{}, false
	}
	return service.PublicArticleListItem{
		ID: translation.ID, Locale: translation.Locale, Slug: translation.Slug, Title: translation.Title,
		Excerpt: publicDerefString(translation.Excerpt), CoverImage: publicDerefString(parent.CoverImage),
		CoverAlt: publicDerefString(parent.CoverAlt), Category: publicDerefString(parent.Category),
		Tags: append([]string(nil), parent.Tags...), PublishedAt: publicDerefTime(translation.PublishedAt),
		Status: domain.ContentStatusPublished, DeletedAt: translation.DeletedAt,
	}, true
}

func (r *publicContentRepository) ListPublishedNewsForSitemap(ctx context.Context) ([]service.PublicNewsListItem, error) {
	client := clientFromContext(ctx, r.client)
	items, err := client.ContentNewsItem.Query().Where(publicNewsPredicates("")...).Order(
		contentnewsitem.ByPublishedAt(sql.OrderDesc()), contentnewsitem.ByID(sql.OrderDesc()),
	).All(ctx)
	if err != nil {
		return nil, err
	}
	return mapPublicNewsList(items), nil
}

func (r *publicContentRepository) GetPublishedNews(ctx context.Context, locale, slug string) (*service.PublicNews, error) {
	client := clientFromContext(ctx, r.client)
	predicates := publicNewsPredicates(locale)
	predicates = append(predicates, contentnewsitem.SlugEQ(strings.TrimSpace(slug)))
	item, err := client.ContentNewsItem.Query().Where(predicates...).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			gone, goneErr := client.ContentNewsItem.Query().Where(
				contentnewsitem.LocaleEQ(strings.TrimSpace(locale)),
				contentnewsitem.SlugEQ(strings.TrimSpace(slug)),
				contentnewsitem.DeletedAtNotNil(),
			).Exist(ctx)
			if goneErr != nil {
				return nil, goneErr
			}
			if gone {
				return nil, service.ErrPublicContentGone
			}
			return nil, service.ErrPublicContentNotFound
		}
		return nil, err
	}
	public := mapPublicNews([]*dbent.ContentNewsItem{item})
	if len(public) == 0 {
		return nil, service.ErrPublicContentNotFound
	}
	public[0].Alternates, err = publishedNewsAlternates(ctx, client, item.SourceCanonicalURL, item.ID)
	if err != nil {
		return nil, err
	}
	return &public[0], nil
}

func publishedNewsAlternates(ctx context.Context, client *dbent.Client, canonical string, currentID int64) ([]service.PublicAlternate, error) {
	rows, err := client.ContentNewsItem.Query().Where(contentnewsitem.SourceCanonicalURLEQ(canonical), contentnewsitem.StatusEQ(domain.ContentStatusPublished), contentnewsitem.DeletedAtIsNil(), contentnewsitem.PublishedAtNotNil()).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.PublicAlternate, 0, len(rows))
	for _, row := range rows {
		if row.ID != currentID {
			out = append(out, service.PublicAlternate{Locale: row.Locale, Slug: row.Slug})
		}
	}
	return out, nil
}

func publishedArticleAlternates(ctx context.Context, client *dbent.Client, contentID int64, sourceVersion int) ([]service.PublicAlternate, error) {
	rows, err := client.ContentTranslation.Query().Where(contenttranslation.ContentTypeEQ("article"), contenttranslation.ContentID(contentID), contenttranslation.SourceVersionEQ(sourceVersion), contenttranslation.StatusEQ(domain.ContentTranslationPublished), contenttranslation.DeletedAtIsNil(), contenttranslation.PublishedAtNotNil()).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.PublicAlternate, 0, len(rows))
	for _, row := range rows {
		out = append(out, service.PublicAlternate{Locale: row.Locale, Slug: row.Slug})
	}
	return out, nil
}

func (r *publicContentRepository) ListPublishedArticles(ctx context.Context, locale string, limit, offset int) (service.PublicArticlePage, error) {
	client := clientFromContext(ctx, r.client)
	requested := strings.TrimSpace(locale)
	predicates := publicArticlePredicates("")
	if requested != "" {
		predicates = append(predicates, contentarticle.LocaleIn(requested, service.PublicContentFallbackLocale))
	}
	rows, err := client.ContentArticle.Query().Where(predicates...).Order(
		contentarticle.ByPublishedAt(sql.OrderDesc()),
		contentarticle.ByID(sql.OrderDesc()),
	).Limit(publicContentMaxListCandidates + 1).All(ctx)
	if err != nil {
		return service.PublicArticlePage{}, err
	}
	if len(rows) > publicContentMaxListCandidates {
		return service.PublicArticlePage{}, service.ErrPublicContentRepositoryContract
	}
	// Key entries by parent article so a published translation replaces the
	// fallback row instead of being counted twice.
	entries := make(map[int64]service.PublicArticleListItem, len(rows))
	order := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		if _, seen := entries[row.ID]; seen {
			continue
		}
		mapped := mapPublicArticlesList([]*dbent.ContentArticle{row})
		if len(mapped) == 0 {
			continue
		}
		entries[row.ID] = mapped[0]
		order = append(order, row.ID)
	}
	if requested != "" && requested != service.PublicContentFallbackLocale {
		translations, translationErr := client.ContentTranslation.Query().Where(
			contenttranslation.ContentTypeEQ("article"),
			contenttranslation.LocaleEQ(requested),
			contenttranslation.StatusEQ(domain.ContentTranslationPublished),
			contenttranslation.DeletedAtIsNil(),
			contenttranslation.PublishedAtNotNil(),
		).Limit(publicContentMaxListCandidates + 1).All(ctx)
		if translationErr != nil {
			return service.PublicArticlePage{}, translationErr
		}
		if len(translations) > publicContentMaxListCandidates {
			return service.PublicArticlePage{}, service.ErrPublicContentRepositoryContract
		}
		for _, translation := range translations {
			if _, tracked := entries[translation.ContentID]; !tracked {
				continue
			}
			validTranslation, validationErr := publishedArticleTranslation(ctx, client, translation.ContentID, translation.SourceVersion)
			if validationErr != nil {
				return service.PublicArticlePage{}, validationErr
			}
			if !validTranslation {
				continue
			}
			parent, parentErr := client.ContentArticle.Query().Where(contentarticle.IDEQ(translation.ContentID)).Only(ctx)
			if parentErr != nil {
				if !dbent.IsNotFound(parentErr) {
					return service.PublicArticlePage{}, parentErr
				}
				continue
			}
			entry, ok := publicArticleTranslationItem(parent, translation)
			if !ok {
				continue
			}
			entries[translation.ContentID] = entry
		}
	}
	items := make([]service.PublicArticleListItem, 0, len(order))
	for _, id := range order {
		if entry, ok := entries[id]; ok {
			items = append(items, entry)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].PublishedAt.Equal(items[j].PublishedAt) {
			if items[i].ID == items[j].ID {
				return items[i].Slug < items[j].Slug
			}
			return items[i].ID > items[j].ID
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
	total := len(items)
	start, end := publicPageBounds(total, limit, offset)
	return service.PublicArticlePage{Items: items[start:end], Total: total}, nil
}

func (r *publicContentRepository) ListFeaturedArticles(ctx context.Context, locale string, limit int) ([]service.PublicArticleListItem, error) {
	client := clientFromContext(ctx, r.client)
	if limit <= 0 || limit > service.MaxPublicFeaturedArticles {
		return nil, service.ErrPublicContentRepositoryContract
	}
	requested := strings.TrimSpace(locale)
	// 与 ListPublishedArticles 保持一致：精选区同样必须做语言回退。
	// 之前这里用 publicArticlePredicates(locale) 精确匹配 locale，导致「精选文章都在
	// 中文、浏览英文站」时精选区整块消失（PRD R9：无译文时回落中文正文）。
	predicates := publicArticlePredicates("")
	predicates = append(predicates, contentarticle.FeaturedAtNotNil())
	if requested != "" {
		predicates = append(predicates, contentarticle.LocaleIn(requested, service.PublicContentFallbackLocale))
	}
	// 多取一批候选：同一篇文章可能同时存在请求语言与回退语言两行，归并后仍要凑满 limit。
	items, err := client.ContentArticle.Query().Where(predicates...).Order(
		contentarticle.ByFeaturedAt(sql.OrderDesc()),
		contentarticle.ByID(sql.OrderDesc()),
	).Limit(limit * 2).All(ctx)
	if err != nil {
		return nil, err
	}
	// 按父文章 id 归并，优先请求语言，其次回退语言，保持 featured_at 倒序。
	chosen := make(map[int64]*dbent.ContentArticle, len(items))
	order := make([]int64, 0, len(items))
	for _, row := range items {
		if row == nil {
			continue
		}
		current, seen := chosen[row.ID]
		if !seen {
			chosen[row.ID] = row
			order = append(order, row.ID)
			continue
		}
		if strings.TrimSpace(row.Locale) == requested {
			chosen[row.ID] = row
		} else if strings.TrimSpace(current.Locale) != requested && row.Locale == service.PublicContentFallbackLocale {
			chosen[row.ID] = row
		}
	}
	ordered := make([]*dbent.ContentArticle, 0, len(order))
	for _, id := range order {
		if row, ok := chosen[id]; ok {
			ordered = append(ordered, row)
		}
	}
	mapped := mapPublicArticlesList(ordered)
	// 请求语言存在已发布译文时用译文覆盖标题/摘要，与列表页行为一致。
	if requested != "" && requested != service.PublicContentFallbackLocale && len(mapped) > 0 {
		byID := make(map[int64]int, len(mapped))
		for i, item := range mapped {
			byID[item.ID] = i
		}
		itemIDs := make([]int64, 0, len(byID))
		for id := range byID {
			itemIDs = append(itemIDs, id)
		}
		translations, translationErr := client.ContentTranslation.Query().Where(
			contenttranslation.ContentTypeEQ("article"),
			contenttranslation.LocaleEQ(requested),
			contenttranslation.StatusEQ(domain.ContentTranslationPublished),
			contenttranslation.DeletedAtIsNil(),
			contenttranslation.PublishedAtNotNil(),
			contenttranslation.ContentIDIn(itemIDs...),
		).All(ctx)
		if translationErr != nil {
			return nil, translationErr
		}
		for _, translation := range translations {
			idx, tracked := byID[translation.ContentID]
			if !tracked {
				continue
			}
			validTranslation, validationErr := publishedArticleTranslation(ctx, client, translation.ContentID, translation.SourceVersion)
			if validationErr != nil {
				return nil, validationErr
			}
			if !validTranslation {
				continue
			}
			parent, parentErr := client.ContentArticle.Query().Where(contentarticle.IDEQ(translation.ContentID)).Only(ctx)
			if parentErr != nil {
				if !dbent.IsNotFound(parentErr) {
					return nil, parentErr
				}
				continue
			}
			if entry, ok := publicArticleTranslationItem(parent, translation); ok {
				mapped[idx] = entry
			}
		}
	}
	if len(mapped) > limit {
		mapped = mapped[:limit]
	}
	return mapped, nil
}

func (r *publicContentRepository) ListPublishedArticlesForSitemap(ctx context.Context) ([]service.PublicArticleListItem, error) {
	client := clientFromContext(ctx, r.client)
	items, err := client.ContentArticle.Query().Where(publicArticlePredicates("")...).Order(
		contentarticle.ByPublishedAt(sql.OrderDesc()), contentarticle.ByID(sql.OrderDesc()),
	).All(ctx)
	if err != nil {
		return nil, err
	}
	out := mapPublicArticlesList(items)
	translations, translationErr := client.ContentTranslation.Query().Where(contenttranslation.ContentTypeEQ("article"), contenttranslation.StatusEQ(domain.ContentTranslationPublished), contenttranslation.DeletedAtIsNil(), contenttranslation.PublishedAtNotNil()).Order(contenttranslation.ByPublishedAt(sql.OrderDesc()), contenttranslation.ByID(sql.OrderDesc())).All(ctx)
	if translationErr != nil {
		return nil, translationErr
	}
	for _, item := range translations {
		validTranslation, validationErr := publishedArticleTranslation(ctx, client, item.ContentID, item.SourceVersion)
		if validationErr != nil {
			return nil, validationErr
		}
		if !validTranslation {
			continue
		}
		// Sitemap entries must satisfy the same renderability and SEO contract
		// as public article detail pages, otherwise crawlers receive 404 URLs.
		if strings.TrimSpace(item.Title) == "" || publicDerefString(item.Excerpt) == "" || publicDerefString(item.SeoTitle) == "" || publicDerefString(item.SeoDescription) == "" || (publicDerefString(item.BodyHTML) == "" && publicDerefString(item.BodyMarkdown) == "") {
			continue
		}
		if parent, err := client.ContentArticle.Query().Where(contentarticle.IDEQ(item.ContentID)).Only(ctx); err == nil {
			out = append(out, service.PublicArticleListItem{ID: item.ID, Locale: item.Locale, Slug: item.Slug, Title: item.Title, Excerpt: publicDerefString(item.Excerpt), CoverImage: publicDerefString(parent.CoverImage), CoverAlt: publicDerefString(parent.CoverAlt), Category: publicDerefString(parent.Category), Tags: append([]string(nil), parent.Tags...), PublishedAt: publicDerefTime(item.PublishedAt), Status: domain.ContentStatusPublished, DeletedAt: item.DeletedAt})
		} else if !dbent.IsNotFound(err) {
			return nil, err
		}
	}
	return out, nil
}

func (r *publicContentRepository) GetPublishedArticle(ctx context.Context, locale, slug string) (*service.PublicArticle, error) {
	client := clientFromContext(ctx, r.client)
	predicates := publicArticlePredicates(locale)
	predicates = append(predicates, contentarticle.SlugEQ(strings.TrimSpace(slug)))
	item, err := client.ContentArticle.Query().Where(predicates...).Only(ctx)
	if err != nil {
		if !dbent.IsNotFound(err) {
			return nil, err
		}
		translation, translationErr := client.ContentTranslation.Query().Where(contenttranslation.ContentTypeEQ("article"), contenttranslation.LocaleEQ(strings.TrimSpace(locale)), contenttranslation.SlugEQ(strings.TrimSpace(slug)), contenttranslation.StatusEQ(domain.ContentTranslationPublished), contenttranslation.DeletedAtIsNil(), contenttranslation.PublishedAtNotNil()).Only(ctx)
		if translationErr != nil {
			gone, goneErr := client.ContentTranslation.Query().Where(
				contenttranslation.ContentTypeEQ("article"),
				contenttranslation.LocaleEQ(strings.TrimSpace(locale)),
				contenttranslation.SlugEQ(strings.TrimSpace(slug)),
				contenttranslation.DeletedAtNotNil(),
			).Exist(ctx)
			if goneErr != nil {
				return nil, goneErr
			}
			if gone {
				return nil, service.ErrPublicContentGone
			}
			return nil, service.ErrPublicContentNotFound
		}
		validTranslation, validationErr := publishedArticleTranslation(ctx, client, translation.ContentID, translation.SourceVersion)
		if validationErr != nil {
			return nil, validationErr
		}
		if !validTranslation {
			return nil, service.ErrPublicContentNotFound
		}
		parent, parentErr := client.ContentArticle.Query().Where(contentarticle.IDEQ(translation.ContentID)).Only(ctx)
		if parentErr != nil {
			if dbent.IsNotFound(parentErr) {
				gone, goneErr := client.ContentArticle.Query().Where(contentarticle.IDEQ(translation.ContentID), contentarticle.DeletedAtNotNil()).Exist(ctx)
				if goneErr != nil {
					return nil, goneErr
				}
				if gone {
					return nil, service.ErrPublicContentGone
				}
			}
			return nil, service.ErrPublicContentNotFound
		}
		alternates, alternateErr := publishedArticleAlternates(ctx, client, translation.ContentID, translation.SourceVersion)
		if alternateErr != nil {
			return nil, alternateErr
		}
		articleHTML := publicBodyHTML(publicDerefString(translation.BodyHTML), publicDerefString(translation.BodyMarkdown))
		return &service.PublicArticle{ID: translation.ID, Locale: translation.Locale, Slug: translation.Slug, Title: translation.Title, Excerpt: publicDerefString(translation.Excerpt), BodyHTML: articleHTML, BodyText: publicPlainText("", articleHTML, publicDerefString(translation.BodyMarkdown)), CoverImage: publicDerefString(parent.CoverImage), CoverAlt: publicDerefString(parent.CoverAlt), Category: publicDerefString(parent.Category), Tags: append([]string(nil), parent.Tags...), PublishedAt: publicDerefTime(translation.PublishedAt), Author: "CCAI 编辑部", SeoTitle: publicDerefString(translation.SeoTitle), SeoDescription: publicDerefString(translation.SeoDescription), OgImage: publicDerefString(parent.OgImage), Status: domain.ContentStatusPublished, DeletedAt: translation.DeletedAt, Alternates: alternates}, nil
	}
	public := mapPublicArticles([]*dbent.ContentArticle{item})
	if len(public) == 0 {
		return nil, service.ErrPublicContentNotFound
	}
	public[0].Alternates, err = publishedArticleAlternates(ctx, client, item.ID, item.Version)
	if err != nil {
		return nil, err
	}
	return &public[0], nil
}

func publishedArticleTranslation(ctx context.Context, client *dbent.Client, contentID int64, sourceVersion int) (bool, error) {
	if contentID <= 0 || sourceVersion <= 0 {
		return false, nil
	}
	article, err := client.ContentArticle.Query().Where(
		contentarticle.IDEQ(contentID),
		contentarticle.StatusEQ(domain.ContentStatusPublished),
		contentarticle.DeletedAtIsNil(),
		contentarticle.PublishedAtNotNil(),
		contentarticle.VersionEQ(sourceVersion),
	).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return article != nil, nil
}

func publicNewsPredicates(locale string) []predicate.ContentNewsItem {
	predicates := []predicate.ContentNewsItem{
		contentnewsitem.StatusEQ(domain.ContentStatusPublished),
		contentnewsitem.DeletedAtIsNil(),
		contentnewsitem.PublishedAtNotNil(),
		publicNewsNonBlank(contentnewsitem.FieldBodyHTML),
		publicNewsNonBlank(contentnewsitem.FieldTitle),
		publicNewsNonBlank(contentnewsitem.FieldSummary),
		publicNewsNonBlank(contentnewsitem.FieldSourceName),
		publicNewsNonBlank(contentnewsitem.FieldSlug),
	}
	if strings.TrimSpace(locale) != "" {
		predicates = append(predicates, contentnewsitem.LocaleEQ(strings.TrimSpace(locale)))
	}
	return predicates
}

func publicArticlePredicates(locale string) []predicate.ContentArticle {
	predicates := []predicate.ContentArticle{
		contentarticle.StatusEQ(domain.ContentStatusPublished),
		contentarticle.DeletedAtIsNil(),
		contentarticle.PublishedAtNotNil(),
		contentarticle.Or(publicArticleNonBlank(contentarticle.FieldBodyHTML), publicArticleNonBlank(contentarticle.FieldBodyMarkdown)),
		publicArticleNonBlank(contentarticle.FieldTitle),
		publicArticleNonBlank(contentarticle.FieldExcerpt),
		publicArticleNonBlank(contentarticle.FieldSlug),
	}
	if strings.TrimSpace(locale) != "" {
		predicates = append(predicates, contentarticle.LocaleEQ(strings.TrimSpace(locale)))
	}
	return predicates
}

func publicNewsNonBlank(field string) predicate.ContentNewsItem {
	return predicate.ContentNewsItem(func(s *sql.Selector) {
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("NULLIF(BTRIM(").Ident(s.C(field)).WriteString("), '') IS NOT NULL")
		}))
	})
}

func publicArticleNonBlank(field string) predicate.ContentArticle {
	return predicate.ContentArticle(func(s *sql.Selector) {
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("NULLIF(BTRIM(").Ident(s.C(field)).WriteString("), '') IS NOT NULL")
		}))
	})
}

func mapPublicNews(items []*dbent.ContentNewsItem) []service.PublicNews {
	result := make([]service.PublicNews, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		cat, catZh, catEn, titleZh, summaryZh, bodyTextZh, titleEn, summaryEn, bodyTextEn, tags := extractNewsMediaMeta(item.MediaJSON)
		result = append(result, service.PublicNews{
			ID:                  item.ID,
			Locale:              item.Locale,
			Slug:                item.Slug,
			Title:               item.Title,
			Summary:             service.CleanContentNewsSummaryWithFallback(publicDerefString(item.Summary), publicPlainText(publicDerefString(item.BodyText), publicDerefString(item.BodyHTML), "")),
			BodyHTML:            publicDerefString(item.BodyHTML),
			BodyText:            publicPlainText(publicDerefString(item.BodyText), publicDerefString(item.BodyHTML), ""),
			SourceName:          publicDerefString(item.SourceName),
			Category:            cat,
			Tags:                tags,
			TitleZh:             titleZh,
			SummaryZh:           service.CleanContentNewsSummary(summaryZh),
			BodyTextZh:          bodyTextZh,
			CategoryZh:          catZh,
			TitleEn:             titleEn,
			SummaryEn:           service.CleanContentNewsSummary(summaryEn),
			BodyTextEn:          bodyTextEn,
			CategoryEn:          catEn,
			OriginalPublishedAt: item.OriginalPublishedAt,
			PublishedAt:         publicDerefTime(item.PublishedAt),
			ImageURL:            publicDerefString(item.ImageURL),
			SeoTitle:            publicDerefString(item.SeoTitle),
			SeoDescription:      publicDerefString(item.SeoDescription),
			Status:              item.Status,
			DeletedAt:           item.DeletedAt,
		})
	}
	return result
}

func mapPublicNewsList(items []*dbent.ContentNewsItem) []service.PublicNewsListItem {
	result := make([]service.PublicNewsListItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		cat, catZh, catEn, titleZh, summaryZh, _, titleEn, summaryEn, _, tags := extractNewsMediaMeta(item.MediaJSON)
		result = append(result, service.PublicNewsListItem{
			ID:                  item.ID,
			Locale:              item.Locale,
			Slug:                item.Slug,
			Title:               item.Title,
			Summary:             service.CleanContentNewsSummaryWithFallback(publicDerefString(item.Summary), publicPlainText(publicDerefString(item.BodyText), publicDerefString(item.BodyHTML), "")),
			SourceName:          publicDerefString(item.SourceName),
			Category:            cat,
			Tags:                tags,
			TitleZh:             titleZh,
			SummaryZh:           service.CleanContentNewsSummary(summaryZh),
			CategoryZh:          catZh,
			TitleEn:             titleEn,
			SummaryEn:           service.CleanContentNewsSummary(summaryEn),
			CategoryEn:          catEn,
			OriginalPublishedAt: item.OriginalPublishedAt,
			PublishedAt:         publicDerefTime(item.PublishedAt),
			ImageURL:            publicDerefString(item.ImageURL),
			SeoTitle:            publicDerefString(item.SeoTitle),
			SeoDescription:      publicDerefString(item.SeoDescription),
			Status:              item.Status,
			DeletedAt:           item.DeletedAt,
		})
	}
	return result
}

func extractNewsMediaMeta(media map[string]any) (cat, catZh, catEn, titleZh, summaryZh, bodyTextZh, titleEn, summaryEn, bodyTextEn string, tags []string) {
	if media == nil {
		return
	}
	str := func(k string) string {
		if v, ok := media[k].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}
	cat = str("category")
	catZh = str("category_zh")
	catEn = str("category_en")
	titleZh = str("title_zh")
	summaryZh = str("summary_zh")
	bodyTextZh = str("body_text_zh")
	titleEn = str("title_en")
	summaryEn = str("summary_en")
	bodyTextEn = str("body_text_en")
	if rawTags, ok := media["tags"].([]any); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok && strings.TrimSpace(s) != "" {
				tags = append(tags, strings.TrimSpace(s))
			}
		}
	}
	return
}

func mapPublicArticles(items []*dbent.ContentArticle) []service.PublicArticle {
	result := make([]service.PublicArticle, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, service.PublicArticle{
			ID:             item.ID,
			Locale:         item.Locale,
			Slug:           item.Slug,
			Title:          item.Title,
			Author:         "CCAI 编辑部",
			Excerpt:        publicDerefString(item.Excerpt),
			BodyHTML:       publicBodyHTML(publicDerefString(item.BodyHTML), publicDerefString(item.BodyMarkdown)),
			BodyText:       publicPlainText("", publicDerefString(item.BodyHTML), publicDerefString(item.BodyMarkdown)),
			CoverImage:     publicDerefString(item.CoverImage),
			CoverAlt:       publicDerefString(item.CoverAlt),
			Category:       publicDerefString(item.Category),
			Tags:           append([]string(nil), item.Tags...),
			SeoTitle:       publicDerefString(item.SeoTitle),
			SeoDescription: publicDerefString(item.SeoDescription),
			CanonicalURL:   publicDerefString(item.CanonicalURL),
			OgImage:        publicDerefString(item.OgImage),
			PublishedAt:    publicDerefTime(item.PublishedAt),
			Status:         item.Status,
			DeletedAt:      item.DeletedAt,
		})
	}
	return result
}

func mapPublicArticlesList(items []*dbent.ContentArticle) []service.PublicArticleListItem {
	result := make([]service.PublicArticleListItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, service.PublicArticleListItem{
			ID:          item.ID,
			Locale:      item.Locale,
			Slug:        item.Slug,
			Title:       item.Title,
			Excerpt:     publicDerefString(item.Excerpt),
			CoverImage:  publicDerefString(item.CoverImage),
			CoverAlt:    publicDerefString(item.CoverAlt),
			Category:    publicDerefString(item.Category),
			Tags:        append([]string(nil), item.Tags...),
			PublishedAt: publicDerefTime(item.PublishedAt),
			Status:      item.Status,
			DeletedAt:   item.DeletedAt,
		})
	}
	return result
}

const publicContentMaxListCandidates = 10000

// publicPageBounds clamps an offset/limit window to a total count.
func publicPageBounds(total, limit, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return offset, end
}

func publicDerefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func publicPlainText(text, htmlBody, markdown string) string {
	plain := strings.TrimSpace(text)
	if plain == "" {
		plain = strings.TrimSpace(htmlBody)
		plain = strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n", "</h1>", "\n", "</h2>", "\n", "</h3>", "\n", "</li>", "\n").Replace(plain)
		var stripped strings.Builder
		inTag := false
		for _, r := range plain {
			switch {
			case r == '<':
				inTag = true
			case r == '>':
				inTag = false
			case !inTag:
				stripped.WriteRune(r)
			}
		}
		plain = html.UnescapeString(stripped.String())
	}
	if strings.TrimSpace(plain) == "" {
		plain = markdown
	}
	return strings.TrimSpace(plain)
}

func publicDerefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

// publicBodyHTML provides a conservative markdown fallback for drafts created
// by the AI writer. The renderer applies its final HTML allowlist afterwards.
// The conversion itself lives in internal/pkg/contentmd so the AI writer can
// persist the same HTML at ingest time.
func publicBodyHTML(htmlBody, markdown string) string {
	raw := htmlBody
	if strings.TrimSpace(raw) == "" {
		raw = contentmd.Render(markdown, contentmd.Options{MinHeadingLevel: 1})
	}
	// 出口再清洗一次：库里可能存在清洗规则上线前写入的历史数据，
	// 而 bodyHtml 现在会随公开详情 JSON 一起返回给匿名客户端。
	return richtext.Sanitize(raw)
}
