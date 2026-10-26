package repository

import (
	"context"
	"html"
	"sort"
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
	predicates := publicNewsPredicates(locale)
	query := client.ContentNewsItem.Query().Where(predicates...).Order(
		contentnewsitem.ByPublishedAt(sql.OrderDesc()),
		contentnewsitem.ByID(sql.OrderDesc()),
	).Limit(limit).Offset(offset)
	items, err := query.All(ctx)
	if err != nil {
		return service.PublicNewsPage{}, err
	}
	total, err := client.ContentNewsItem.Query().Where(predicates...).Count(ctx)
	if err != nil {
		return service.PublicNewsPage{}, err
	}
	return service.PublicNewsPage{Items: mapPublicNewsList(items), Total: total}, nil
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
	const maxArticleCandidates = 10000
	client := clientFromContext(ctx, r.client)
	predicates := publicArticlePredicates(locale)
	items, err := client.ContentArticle.Query().Where(predicates...).Order(contentarticle.ByPublishedAt(sql.OrderDesc()), contentarticle.ByID(sql.OrderDesc())).Limit(maxArticleCandidates + 1).All(ctx)
	if err != nil {
		return service.PublicArticlePage{}, err
	}
	if len(items) > maxArticleCandidates {
		return service.PublicArticlePage{}, service.ErrPublicContentRepositoryContract
	}
	translations, translationErr := client.ContentTranslation.Query().Where(contenttranslation.ContentTypeEQ("article"), contenttranslation.LocaleEQ(strings.TrimSpace(locale)), contenttranslation.StatusEQ(domain.ContentTranslationPublished), contenttranslation.DeletedAtIsNil(), contenttranslation.PublishedAtNotNil()).Limit(maxArticleCandidates + 1).All(ctx)
	if translationErr != nil {
		return service.PublicArticlePage{}, translationErr
	}
	if len(translations) > maxArticleCandidates {
		return service.PublicArticlePage{}, service.ErrPublicContentRepositoryContract
	}
	merged := mapPublicArticlesList(items)
	for _, item := range translations {
		validTranslation, validationErr := publishedArticleTranslation(ctx, client, item.ContentID, item.SourceVersion)
		if validationErr != nil {
			return service.PublicArticlePage{}, validationErr
		}
		if !validTranslation {
			continue
		}
		if publicDerefString(item.Excerpt) == "" || publicDerefString(item.SeoTitle) == "" || publicDerefString(item.SeoDescription) == "" || (publicDerefString(item.BodyHTML) == "" && publicDerefString(item.BodyMarkdown) == "") {
			continue
		}
		parent, parentErr := client.ContentArticle.Query().Where(contentarticle.IDEQ(item.ContentID)).Only(ctx)
		if parentErr != nil {
			if !dbent.IsNotFound(parentErr) {
				return service.PublicArticlePage{}, parentErr
			}
			continue
		}
		merged = append(merged, service.PublicArticleListItem{ID: item.ID, Locale: item.Locale, Slug: item.Slug, Title: item.Title, Excerpt: publicDerefString(item.Excerpt), CoverImage: publicDerefString(parent.CoverImage), CoverAlt: publicDerefString(parent.CoverAlt), Category: publicDerefString(parent.Category), Tags: append([]string(nil), parent.Tags...), PublishedAt: publicDerefTime(item.PublishedAt), Status: domain.ContentStatusPublished, DeletedAt: item.DeletedAt})
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].PublishedAt.Equal(merged[j].PublishedAt) {
			if merged[i].ID == merged[j].ID {
				return merged[i].Slug < merged[j].Slug
			}
			return merged[i].ID > merged[j].ID
		}
		return merged[i].PublishedAt.After(merged[j].PublishedAt)
	})
	total := len(merged)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return service.PublicArticlePage{Items: merged[offset:end], Total: total}, nil
}

func (r *publicContentRepository) ListFeaturedArticles(ctx context.Context, locale string, limit int) ([]service.PublicArticleListItem, error) {
	client := clientFromContext(ctx, r.client)
	if limit <= 0 || limit > service.MaxPublicFeaturedArticles {
		return nil, service.ErrPublicContentRepositoryContract
	}
	predicates := publicArticlePredicates(locale)
	predicates = append(predicates, contentarticle.FeaturedAtNotNil())
	items, err := client.ContentArticle.Query().Where(predicates...).Order(
		contentarticle.ByFeaturedAt(sql.OrderDesc()),
		contentarticle.ByID(sql.OrderDesc()),
	).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	return mapPublicArticlesList(items), nil
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
			Summary:             publicDerefString(item.Summary),
			BodyHTML:            publicDerefString(item.BodyHTML),
			BodyText:            publicPlainText(publicDerefString(item.BodyText), publicDerefString(item.BodyHTML), ""),
			SourceName:          publicDerefString(item.SourceName),
			Category:            cat,
			Tags:                tags,
			TitleZh:             titleZh,
			SummaryZh:           summaryZh,
			BodyTextZh:          bodyTextZh,
			CategoryZh:          catZh,
			TitleEn:             titleEn,
			SummaryEn:           summaryEn,
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
			Summary:             publicDerefString(item.Summary),
			SourceName:          publicDerefString(item.SourceName),
			Category:            cat,
			Tags:                tags,
			TitleZh:             titleZh,
			SummaryZh:           summaryZh,
			CategoryZh:          catZh,
			TitleEn:             titleEn,
			SummaryEn:           summaryEn,
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
	if strings.TrimSpace(htmlBody) != "" {
		return htmlBody
	}
	return contentmd.Render(markdown, contentmd.Options{MinHeadingLevel: 1})
}
