package repository

import (
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

func TestPublicBodyHTMLMarkdownFallback(t *testing.T) {
	got := publicBodyHTML("", "# CCAI\n\n正文 **安全**\n\n```go\nfmt.Println(1)\n```")
	if !strings.Contains(got, "<h1>CCAI</h1>") || !strings.Contains(got, "<p>正文 **安全**</p>") || !strings.Contains(got, "<pre><code>") {
		t.Fatalf("unexpected markdown fallback: %s", got)
	}
	if got := publicBodyHTML("<p>html</p>", "# markdown"); got != "<p>html</p>" {
		t.Fatalf("html body should win, got %q", got)
	}
}

func publicTestNewsRow(id int64, locale, slug, canonical string, published time.Time) *dbent.ContentNewsItem {
	summary, source := "Summary of "+slug, "Source"
	return &dbent.ContentNewsItem{
		ID: id, Locale: locale, Slug: slug, Title: "Title " + slug,
		Summary: &summary, SourceName: &source, SourceCanonicalURL: canonical,
		PublishedAt: &published, Status: domain.ContentStatusPublished,
	}
}

// REQ-009: one story may exist as several locale rows sharing the source
// canonical URL; the list counts the story once and prefers the requested
// locale, then the editorial fallback language.
func TestPublicNewsListItemsDedupesCanonicalAndPrefersRequestedLocale(t *testing.T) {
	newer := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rows := []*dbent.ContentNewsItem{
		publicTestNewsRow(1, "zh-CN", "zh-translated", "https://src.example/a", newer),
		publicTestNewsRow(2, "en-US", "en-translated", "https://src.example/a", newer),
		publicTestNewsRow(3, "zh-CN", "zh-only", "https://src.example/b", older),
	}

	items := publicNewsListItems(rows, "en-US")
	if len(items) != 2 {
		t.Fatalf("expected one entry per story, got %d: %+v", len(items), items)
	}
	if items[0].Slug != "en-translated" || items[0].Locale != "en-US" {
		t.Fatalf("requested locale must win for translated stories, got %+v", items[0])
	}
	if items[1].Slug != "zh-only" || items[1].Locale != "zh-CN" {
		t.Fatalf("untranslated stories fall back to the base locale, got %+v", items[1])
	}

	base := publicNewsListItems(rows, "zh-CN")
	if len(base) != 2 || base[0].Slug != "zh-translated" {
		t.Fatalf("base locale keeps its own row, got %+v", base)
	}
}

func TestPublicNewsListItemsSortsNewestFirstWithStableTies(t *testing.T) {
	same := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	items := publicNewsListItems([]*dbent.ContentNewsItem{
		publicTestNewsRow(7, "zh-CN", "b", "https://src.example/b", same),
		publicTestNewsRow(9, "zh-CN", "a", "https://src.example/a", same),
		publicTestNewsRow(8, "zh-CN", "c", "https://src.example/c", same.Add(24*time.Hour)),
	}, "zh-CN")
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].Slug != "c" {
		t.Fatalf("newest item must come first, got %+v", items[0])
	}
	if items[1].ID != 9 || items[2].ID != 7 {
		t.Fatalf("ties must order by descending id, got %+v", items[1:])
	}
}

// REQ-009: a published translation replaces its parent row but only when all
// SEO fields (and a body) are present; otherwise the base row stays visible.
func TestPublicArticleTranslationItemRequiresCompleteSEO(t *testing.T) {
	published := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	cover, coverAlt, category, excerpt := "https://cdn.example/cover.png", "cover alt", "AI", "摘要"
	parent := &dbent.ContentArticle{ID: 11, CoverImage: &cover, CoverAlt: &coverAlt, Category: &category, Tags: []string{"a", "b"}}
	seoTitle, seoDesc, bodyHTML := "标题", "描述", "<p>body</p>"

	complete := &dbent.ContentTranslation{ID: 21, ContentID: 11, Locale: "en-US", Slug: "en-article", Title: "Title",
		Excerpt: &excerpt, SeoTitle: &seoTitle, SeoDescription: &seoDesc, BodyHTML: &bodyHTML, PublishedAt: &published}
	item, ok := publicArticleTranslationItem(parent, complete)
	if !ok {
		t.Fatal("complete translation should produce a public list item")
	}
	if item.ID != 21 || item.Locale != "en-US" || item.Slug != "en-article" {
		t.Fatalf("translation identity must be used, got %+v", item)
	}
	if item.CoverImage != cover || item.CoverAlt != coverAlt || item.Category != category || len(item.Tags) != 2 {
		t.Fatalf("parent media/category/tags must be borrowed, got %+v", item)
	}

	missingDesc := *complete
	missingDesc.SeoDescription = nil
	if _, ok := publicArticleTranslationItem(parent, &missingDesc); ok {
		t.Fatal("translation without seo description must be rejected")
	}
	noBody := *complete
	noBody.BodyHTML = nil
	if _, ok := publicArticleTranslationItem(parent, &noBody); ok {
		t.Fatal("translation without body must be rejected")
	}
}

func TestPublicPageBounds(t *testing.T) {
	cases := []struct{ total, limit, offset, wantStart, wantEnd int }{
		{5, 2, 0, 0, 2},
		{5, 2, 2, 2, 4},
		{5, 2, 4, 4, 5},
		{5, 2, 9, 5, 5},
		{0, 20, 0, 0, 0},
	}
	for _, c := range cases {
		start, end := publicPageBounds(c.total, c.limit, c.offset)
		if start != c.wantStart || end != c.wantEnd {
			t.Fatalf("publicPageBounds(%d,%d,%d) = (%d,%d), want (%d,%d)", c.total, c.limit, c.offset, start, end, c.wantStart, c.wantEnd)
		}
	}
}
