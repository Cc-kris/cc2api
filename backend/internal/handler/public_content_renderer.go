package handler

import (
	"bytes"
	"encoding/json"
	stdhtml "html"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	publicHTMLCacheCollection = "public, max-age=60"
	publicHTMLCacheDetail     = "public, max-age=60"
)

type publicDetailView struct {
	Locale              string
	Title               string
	Heading             string
	Description         string
	Canonical           string
	ImageURL            string
	ImageAlt            string
	PublishedAt         string
	OriginalPublishedAt string
	SourceName          string
	Body                template.HTML
	JSONLD              template.JS
	Nonce               string
	Author              string
	IsNews              bool
	Robots              string
	Alternates          []publicAlternateView
	Chrome              publicChromeView
}
type publicAlternateView struct{ HrefLang, Href string }

// publicChromeView carries the locale-aware chrome (navigation and field
// labels) of the server-rendered public documents. The content itself may
// still be the Chinese master when no translation exists, but the surrounding
// interface must follow the requested language (R9-03).
type publicChromeView struct {
	News                string
	Articles            string
	Docs                string
	Source              string
	Author              string
	Published           string
	OriginalPublished   string
}

var publicChromeBySegment = map[string]publicChromeView{
	"zh": {News: "新闻", Articles: "文章", Docs: "API 文档", Source: "来源：", Author: "作者：", Published: "发布时间：", OriginalPublished: "原始发布时间："},
	"en": {News: "News", Articles: "Articles", Docs: "API Docs", Source: "Source: ", Author: "Author: ", Published: "Published: ", OriginalPublished: "Originally published: "},
	"ru": {News: "Новости", Articles: "Статьи", Docs: "Документация API", Source: "Источник: ", Author: "Автор: ", Published: "Опубликовано: ", OriginalPublished: "Первая публикация: "},
	"fr": {News: "Actualités", Articles: "Articles", Docs: "Documentation API", Source: "Source : ", Author: "Auteur : ", Published: "Publié le : ", OriginalPublished: "Publication initiale : "},
	"de": {News: "Nachrichten", Articles: "Artikel", Docs: "Dokumentation", Source: "Quelle: ", Author: "Autor: ", Published: "Veröffentlicht: ", OriginalPublished: "Ursprünglich veröffentlicht: "},
	"ja": {News: "ニュース", Articles: "記事", Docs: "API ドキュメント", Source: "出典：", Author: "著者：", Published: "公開日：", OriginalPublished: "初回公開日："},
}

func publicChrome(locale string) publicChromeView {
	if chrome, ok := publicChromeBySegment[publicLocaleSegment(locale)]; ok {
		return chrome
	}
	return publicChromeBySegment["en"]
}

const publicHTMLNoncePlaceholder = "__CCAI_PUBLIC_NONCE__"

type publicHTMLCacheEntry struct {
	body      []byte
	expiresAt time.Time
}

var publicHTMLCache = struct {
	sync.RWMutex
	items map[string]publicHTMLCacheEntry
}{items: make(map[string]publicHTMLCacheEntry)}

const publicHTMLCacheMaxEntries = 512

// InvalidatePublicContentHTMLCache clears all public rendered documents. The
// cache is intentionally process-local; each instance invalidates on its own
// successful mutation and the 60s TTL remains the cross-instance fallback.
func InvalidatePublicContentHTMLCache() {
	publicHTMLCache.Lock()
	publicHTMLCache.items = make(map[string]publicHTMLCacheEntry)
	publicHTMLCache.Unlock()
}

type publicNewsListView struct {
	Title       string
	Summary     string
	SourceName  string
	PublishedAt string
	OriginalAt  string
	URL         string
	ImageURL    string
}

type publicArticleListView struct {
	Title       string
	Excerpt     string
	PublishedAt string
	URL         string
	ImageURL    string
	ImageAlt    string
}

type publicCollectionView struct {
	Locale       string
	Title        string
	Description  string
	Canonical    string
	JSONLD       template.JS
	Nonce        string
	Kind         string
	NewsItems    []publicNewsListView
	ArticleItems []publicArticleListView
	Page         int
	PageSize     int
	Total        int
	HasNext      bool
	Chrome       publicChromeView
}

type publicDocsView struct {
	Locale      string
	Title       string
	Description string
	Canonical   string
	JSONLD      template.JS
	Nonce       string
	Path        string
	Chrome      publicChromeView
}

var publicClassPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

var publicDetailTemplate = template.Must(template.New("public-detail").Parse(`<!doctype html>
<html lang="{{.Locale}}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>{{.Title}}</title>
  <meta name="description" content="{{.Description}}">
  <link rel="canonical" href="{{.Canonical}}">
  <meta name="robots" content="{{.Robots}}">
  {{range .Alternates}}<link rel="alternate" hreflang="{{.HrefLang}}" href="{{.Href}}">{{end}}
  <meta property="og:type" content="{{if .IsNews}}article{{else}}article{{end}}">
  <meta property="og:title" content="{{.Title}}">
  <meta property="og:description" content="{{.Description}}">
  <meta property="og:url" content="{{.Canonical}}">
  <meta property="og:locale" content="{{.Locale}}">
  {{if .ImageURL}}<meta property="og:image" content="{{.ImageURL}}">{{end}}
  <meta name="twitter:card" content="{{if .ImageURL}}summary_large_image{{else}}summary{{end}}">
  <meta name="twitter:title" content="{{.Title}}">
  <meta name="twitter:description" content="{{.Description}}">
  <script nonce="{{.Nonce}}" type="application/ld+json">{{.JSONLD}}</script>
</head>
<body>
  <header><a href="/">CCAI</a><nav><a href="/news?locale={{.Locale}}">{{.Chrome.News}}</a><a href="/articles?locale={{.Locale}}">{{.Chrome.Articles}}</a><a href="/docs?locale={{.Locale}}">{{.Chrome.Docs}}</a></nav></header>
  <main>
    <article>
      <h1>{{.Heading}}</h1>
      <p class="summary">{{.Description}}</p>
      {{if .SourceName}}<p class="source">{{.Chrome.Source}}{{.SourceName}}</p>{{end}}
      {{if .Author}}<p class="author">{{.Chrome.Author}}{{.Author}}</p>{{end}}
      {{if .OriginalPublishedAt}}<time datetime="{{.OriginalPublishedAt}}">{{.Chrome.OriginalPublished}}{{.OriginalPublishedAt}}</time>{{end}}
      <time datetime="{{.PublishedAt}}">{{.Chrome.Published}}{{.PublishedAt}}</time>
      <section class="content">{{.Body}}</section>
    </article>
  </main>
</body>
</html>`))

var publicCollectionTemplate = template.Must(template.New("public-collection").Parse(`<!doctype html>
<html lang="{{.Locale}}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>{{.Title}}</title>
  <meta name="description" content="{{.Description}}">
  <link rel="canonical" href="{{.Canonical}}">
  <meta property="og:type" content="website">
  <meta property="og:title" content="{{.Title}}">
  <meta property="og:description" content="{{.Description}}">
  <meta property="og:url" content="{{.Canonical}}">
  <meta property="og:locale" content="{{.Locale}}">
  <meta name="twitter:card" content="summary">
  <meta name="twitter:title" content="{{.Title}}">
  <meta name="twitter:description" content="{{.Description}}">
  <script nonce="{{.Nonce}}" type="application/ld+json">{{.JSONLD}}</script>
</head>
<body>
  <header><a href="/">CCAI</a><nav><a href="/news?locale={{.Locale}}">{{.Chrome.News}}</a><a href="/articles?locale={{.Locale}}">{{.Chrome.Articles}}</a><a href="/docs?locale={{.Locale}}">{{.Chrome.Docs}}</a></nav></header>
  <main>
    {{if eq .Kind "home"}}
    <section class="hero"><p class="eyebrow">CCAI</p><h1>面向开发者的 AI API 平台</h1><p>统一接入 CCAI 当前可用模型，公开获取新闻、文章和 API 文档。</p><p><a href="/login">登录控制台</a><a href="/docs?locale={{.Locale}}">查看 API 文档</a></p></section>
    <section aria-label="CCAI capabilities"><p>一个端点，统一的 API 表面，持续更新的模型能力。</p><div><strong>统一接入</strong><span>兼容 OpenAI 的 Chat、Responses 与多模态接口。</span></div><div><strong>生产可用</strong><span>让模型选择变化不影响你的应用集成。</span></div><div><strong>知识沉淀</strong><span>新闻、文章和开发者文档公开可检索。</span></div></section>
    <section aria-label="API example"><h2>从熟悉的请求开始</h2><pre><code>POST /v1/chat/completions
Authorization: Bearer YOUR_API_KEY</code></pre><a href="/docs?locale={{.Locale}}">查看 API 文档 →</a></section>
    <section aria-label="model capabilities"><h2>为每个任务选择合适的模型，应用保持稳定。</h2><p>CCAI 当前可用模型统一通过兼容接口提供；模型广场和密钥管理仅在登录后的控制台开放。</p></section>
    <section aria-labelledby="news-heading"><h2 id="news-heading">最新新闻</h2><div class="news-list">{{range .NewsItems}}<article><h3><a href="{{.URL}}">{{.Title}}</a></h3><p>{{.Summary}}</p><p><span>{{.SourceName}}</span> · <time datetime="{{.PublishedAt}}">{{.PublishedAt}}</time></p></article>{{else}}<p>暂无已发布新闻。</p>{{end}}</div><a href="/news?locale={{.Locale}}">查看全部新闻 →</a></section>
    <section aria-labelledby="articles-heading"><h2 id="articles-heading">最新文章</h2><div class="article-list">{{range .ArticleItems}}<article><h3><a href="{{.URL}}">{{.Title}}</a></h3><p>{{.Excerpt}}</p><time datetime="{{.PublishedAt}}">{{.PublishedAt}}</time></article>{{else}}<p>暂无已发布文章。</p>{{end}}</div><a href="/articles?locale={{.Locale}}">查看全部文章 →</a></section>
    <section aria-label="call to action"><h2>用 CCAI 开始构建。</h2><a href="/login">打开控制台</a><a href="/docs?locale={{.Locale}}">阅读文档</a></section>
    {{else if eq .Kind "news"}}
    <section aria-labelledby="page-heading"><h1 id="page-heading">CCAI 新闻</h1><p>{{.Description}}</p><div class="news-list">{{range .NewsItems}}<article><h2><a href="{{.URL}}">{{.Title}}</a></h2><p>{{.Summary}}</p><p><span>{{.SourceName}}</span> · <time datetime="{{.PublishedAt}}">{{.PublishedAt}}</time></p></article>{{else}}<p>暂无已发布新闻。</p>{{end}}</div></section>
    {{else}}
    <section aria-labelledby="page-heading"><h1 id="page-heading">CCAI 文章</h1><p>{{.Description}}</p><div class="article-list">{{range .ArticleItems}}<article><h2><a href="{{.URL}}">{{.Title}}</a></h2><p>{{.Excerpt}}</p><time datetime="{{.PublishedAt}}">{{.PublishedAt}}</time></article>{{else}}<p>暂无已发布文章。</p>{{end}}</div></section>
    {{end}}
  </main>
</body>
</html>`))

var publicDocsTemplate = template.Must(template.New("public-docs").Parse(`<!doctype html>
<html lang="{{.Locale}}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>{{.Title}}</title>
  <meta name="description" content="{{.Description}}">
  <link rel="canonical" href="{{.Canonical}}">
  <meta property="og:type" content="website">
  <meta property="og:title" content="{{.Title}}">
  <meta property="og:description" content="{{.Description}}">
  <meta property="og:url" content="{{.Canonical}}">
  <meta property="og:locale" content="{{.Locale}}">
  <meta name="twitter:card" content="summary">
  <script nonce="{{.Nonce}}" type="application/ld+json">{{.JSONLD}}</script>
</head>
<body>
  <header><a href="/">CCAI</a><nav><a href="/news?locale={{.Locale}}">{{.Chrome.News}}</a><a href="/articles?locale={{.Locale}}">{{.Chrome.Articles}}</a></nav></header>
  <main><article><h1>CCAI API 文档</h1><p>通过兼容 OpenAI 的接口访问 CCAI 当前可用模型。模型广场和密钥管理仅在登录后的控制台提供。</p><h2>基础地址</h2><pre><code>/v1</code></pre><h2>认证</h2><p>在请求头中使用你在控制台创建的 API Key：<code>Authorization: Bearer YOUR_API_KEY</code></p><h2>常用接口</h2><ul><li><code>POST /v1/chat/completions</code>：对话补全</li><li><code>POST /v1/responses</code>：Responses API</li><li><code>GET /api/v1/public/news</code>：公开新闻数据</li><li><code>GET /api/v1/public/articles</code>：公开文章数据</li></ul>{{if .Path}}<p>当前文档路径：<code>{{.Path}}</code></p>{{end}}</article></main>
</body>
</html>`))

func renderNewsDocument(c *gin.Context, baseURL string, item *service.PublicNews) {
	requested := firstPublicNonEmpty(item.RequestedLocale, item.Locale)
	seoLocale := requested
	if item.Fallback != "" {
		seoLocale = item.Fallback
	}
	view := publicDetailView{
		Locale:      requested,
		Title:       firstPublicNonEmpty(item.SeoTitle, item.Title),
		Heading:     item.Title,
		Description: firstPublicNonEmpty(item.SeoDescription, item.Summary),
		Canonical:   publicAbsoluteURL(c, baseURL, publicContentPath("/news", publicLocaleSegment(seoLocale), item.Slug), ""),
		Robots:      publicDetailRobots(item.Fallback),
		ImageURL:    safePublicImageURL(item.ImageURL),
		ImageAlt:    item.Title,
		PublishedAt: formatPublicTime(item.PublishedAt),
		SourceName:  strings.TrimSpace(item.SourceName),
		Body:        sanitizePublicBody(item.BodyHTML),
		Nonce:       middleware.GetNonceFromContext(c),
		Chrome:      publicChrome(requested),
		IsNews:      true,
	}
	view.Alternates = publicAlternateViews(c, baseURL, "/news", seoLocale, item.Slug, item.Alternates, false)
	if item.OriginalPublishedAt != nil {
		view.OriginalPublishedAt = formatPublicTime(*item.OriginalPublishedAt)
	}
	view.JSONLD = publicJSONLD(newsJSONLD(item, view.Title, view.Description, view.Canonical))
	renderPublicTemplate(c, publicDetailTemplate, view, publicHTMLCacheDetail, view.Robots)
}

func renderArticleDocument(c *gin.Context, baseURL string, item *service.PublicArticle) {
	requested := firstPublicNonEmpty(item.RequestedLocale, item.Locale)
	seoLocale := requested
	if item.Fallback != "" {
		seoLocale = item.Fallback
	}
	view := publicDetailView{
		Locale:      requested,
		Title:       firstPublicNonEmpty(item.SeoTitle, item.Title),
		Heading:     item.Title,
		Description: firstPublicNonEmpty(item.SeoDescription, item.Excerpt),
		Canonical:   publicAbsoluteURL(c, baseURL, publicContentPath("/articles", publicLocaleSegment(seoLocale), item.Slug), ""),
		Robots:      publicDetailRobots(item.Fallback),
		ImageURL:    safePublicImageURL(firstPublicNonEmpty(item.OgImage, item.CoverImage)),
		ImageAlt:    item.CoverAlt,
		PublishedAt: formatPublicTime(item.PublishedAt),
		Body:        sanitizePublicBody(item.BodyHTML),
		Nonce:       middleware.GetNonceFromContext(c),
		Chrome:      publicChrome(requested),
		Author:      item.Author,
	}
	view.Alternates = publicAlternateViews(c, baseURL, "/articles", seoLocale, item.Slug, item.Alternates, true)
	view.JSONLD = publicJSONLD(articleJSONLD(item, view.Title, view.Description, view.Canonical))
	renderPublicTemplate(c, publicDetailTemplate, view, publicHTMLCacheDetail, view.Robots)
}

// publicDetailRobots keeps fallback pages out of the index (R9-05) while
// leaving translated and Chinese pages crawlable.
func publicDetailRobots(fallback string) string {
	if strings.TrimSpace(fallback) != "" {
		return "noindex, follow"
	}
	return "index, follow"
}

// publicAlternateViews emits short-segment hreflang links. The canonical
// language is always listed; the requested language is only listed when the
// page really is that language, so a fallback page never advertises itself.
func publicAlternateViews(c *gin.Context, baseURL, prefix, selfLocale, selfSlug string, alternates []service.PublicAlternate, ensureChineseMaster bool) []publicAlternateView {
	seen := make(map[string]bool, len(alternates)+2)
	out := make([]publicAlternateView, 0, len(alternates)+2)
	add := func(locale, slug string) {
		locale, slug = strings.TrimSpace(locale), strings.TrimSpace(slug)
		if locale == "" || slug == "" {
			return
		}
		segment := publicLocaleSegment(locale)
		if seen[segment] {
			return
		}
		seen[segment] = true
		out = append(out, publicAlternateView{HrefLang: segment, Href: publicAbsoluteURL(c, baseURL, publicContentPath(prefix, segment, slug), "")})
	}
	add(selfLocale, selfSlug)
	for _, a := range alternates {
		add(a.Locale, a.Slug)
	}
	// Every translated page shares the public slug of its Chinese master, so the
	// canonical zh URL is always addressable and must be advertised: without it
	// search engines never learn the master version of a translated article.
	if ensureChineseMaster && !seen["zh"] {
		add("zh-CN", selfSlug)
	}
	for _, candidate := range out {
		if candidate.HrefLang == "zh" {
			return append(out, publicAlternateView{HrefLang: "x-default", Href: candidate.Href})
		}
	}
	return out
}

func renderHomeDocument(c *gin.Context, baseURL, locale string, news service.PublicNewsPage, articles service.PublicArticlePage) {
	locale = firstPublicNonEmpty(news.Locale, locale)
	view := publicCollectionView{
		Locale:       locale,
		Title:        "CCAI | AI API 平台",
		Description:  "CCAI 提供统一的 AI API 接入、模型服务、新闻、文章与开发者文档。",
		Canonical:    canonicalPublicCollectionURL(c, baseURL, locale, ""),
		Kind:         "home",
		Nonce:        middleware.GetNonceFromContext(c),
		Chrome:       publicChrome(locale),
		NewsItems:    newsListViews(news.Items),
		ArticleItems: articleListViews(articles.Items),
	}
	view.JSONLD = publicJSONLD(map[string]any{
		"@context":   "https://schema.org",
		"@type":      "WebSite",
		"name":       "CCAI",
		"url":        view.Canonical,
		"inLanguage": locale,
	})
	renderPublicTemplate(c, publicCollectionTemplate, view, publicHTMLCacheCollection, "index, follow")
}

func renderNewsListDocument(c *gin.Context, baseURL, locale string, page service.PublicNewsPage) {
	locale = firstPublicNonEmpty(page.Locale, locale)
	view := publicCollectionView{
		Locale:      locale,
		Title:       "CCAI 新闻",
		Description: "CCAI 新闻与 AI 行业动态，展示标题、摘要、发布时间和来源。",
		Canonical:   canonicalPublicCollectionURL(c, baseURL, locale, "/news"),
		Kind:        "news",
		Nonce:       middleware.GetNonceFromContext(c),
		Chrome:      publicChrome(locale),
		NewsItems:   newsListViews(page.Items),
		Page:        page.Page,
		PageSize:    page.PageSize,
		Total:       page.Total,
		HasNext:     page.HasNext,
	}
	view.JSONLD = publicJSONLD(map[string]any{
		"@context":   "https://schema.org",
		"@type":      "CollectionPage",
		"name":       view.Title,
		"url":        view.Canonical,
		"inLanguage": locale,
	})
	renderPublicTemplate(c, publicCollectionTemplate, view, publicHTMLCacheCollection, "index, follow")
}

func renderArticleListDocument(c *gin.Context, baseURL, locale string, page service.PublicArticlePage) {
	view := publicCollectionView{
		Locale:       locale,
		Title:        "CCAI 文章",
		Description:  "CCAI 技术文章、产品实践与 AI 开发者内容。",
		Canonical:    canonicalPublicCollectionURL(c, baseURL, locale, "/articles"),
		Kind:         "articles",
		Nonce:        middleware.GetNonceFromContext(c),
		Chrome:       publicChrome(locale),
		ArticleItems: articleListViews(page.Items),
		Page:         page.Page,
		PageSize:     page.PageSize,
		Total:        page.Total,
		HasNext:      page.HasNext,
	}
	view.JSONLD = publicJSONLD(map[string]any{
		"@context":   "https://schema.org",
		"@type":      "CollectionPage",
		"name":       view.Title,
		"url":        view.Canonical,
		"inLanguage": locale,
	})
	renderPublicTemplate(c, publicCollectionTemplate, view, publicHTMLCacheCollection, "index, follow")
}

func renderDocsDocument(c *gin.Context, baseURL, locale, path string) {
	docPath := "/docs"
	if trimmed := strings.Trim(path, "/"); trimmed != "" {
		docPath += "/" + strings.Join(strings.Split(trimmed, "/"), "/")
	}
	view := publicDocsView{
		Locale:      publicLocaleSegment(locale),
		Title:       "CCAI API 文档",
		Description: "CCAI API 接入、认证和常用接口文档。",
		Canonical:   publicAbsoluteURL(c, baseURL, docPath, canonicalLocaleQuery(locale)),
		Path:        strings.Trim(path, "/"),
		Nonce:       middleware.GetNonceFromContext(c),
		Chrome:      publicChrome(locale),
	}
	view.JSONLD = publicJSONLD(map[string]any{
		"@context":    "https://schema.org",
		"@type":       "TechArticle",
		"headline":    view.Title,
		"description": view.Description,
		"url":         view.Canonical,
		"inLanguage":  view.Locale,
	})
	renderPublicTemplate(c, publicDocsTemplate, view, publicHTMLCacheCollection, "index, follow")
}

func renderPublicTemplate(c *gin.Context, tmpl *template.Template, data any, cacheControl, robots string) {
	cacheKey := publicHTMLCacheKey(c.Request.URL)
	nonce := middleware.GetNonceFromContext(c)
	publicCacheEnabled := nonce != ""
	publicHTMLCache.RLock()
	entry, hit := publicHTMLCache.items[cacheKey]
	publicHTMLCache.RUnlock()
	if publicCacheEnabled && hit && time.Now().Before(entry.expiresAt) {
		body := bytes.ReplaceAll(entry.body, []byte(publicHTMLNoncePlaceholder), []byte(nonce))
		c.Header("X-Robots-Tag", robots)
		c.Header("Cache-Control", cacheControl)
		c.Data(http.StatusOK, "text/html; charset=utf-8", body)
		c.Abort()
		return
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		renderPublicError(c, http.StatusInternalServerError, "CCAI", "页面暂时不可用")
		return
	}
	body := out.Bytes()
	if nonce != "" {
		body = bytes.ReplaceAll(body, []byte(nonce), []byte(publicHTMLNoncePlaceholder))
	}
	if publicCacheEnabled {
		publicHTMLCache.Lock()
		now := time.Now()
		for key, cached := range publicHTMLCache.items {
			if now.After(cached.expiresAt) {
				delete(publicHTMLCache.items, key)
			}
		}
		if len(publicHTMLCache.items) >= publicHTMLCacheMaxEntries {
			publicHTMLCache.items = make(map[string]publicHTMLCacheEntry)
		}
		publicHTMLCache.items[cacheKey] = publicHTMLCacheEntry{body: append([]byte(nil), body...), expiresAt: time.Now().Add(60 * time.Second)}
		publicHTMLCache.Unlock()
	}
	body = bytes.ReplaceAll(body, []byte(publicHTMLNoncePlaceholder), []byte(nonce))
	c.Header("X-Robots-Tag", robots)
	c.Header("Cache-Control", cacheControl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", body)
	c.Abort()
}

// Only canonical public route dimensions participate in the process cache.
// Arbitrary query strings must not create unbounded entries.
func publicHTMLCacheKey(u *url.URL) string {
	if u == nil {
		return "/"
	}
	key := u.Path
	q := u.Query()
	separator := "?"
	for _, name := range []string{"locale", "page", "page_size"} {
		if value := strings.TrimSpace(q.Get(name)); value != "" {
			key += separator + name + "=" + url.QueryEscape(value)
			separator = "&"
		}
	}
	return key
}

func renderPublicError(c *gin.Context, status int, title, message string) {
	c.Header("X-Robots-Tag", "noindex, nofollow, noarchive")
	c.Header("Cache-Control", "no-store")
	c.Data(status, "text/html; charset=utf-8", []byte("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>"+stdhtml.EscapeString(title)+"</title></head><body><main><h1>"+stdhtml.EscapeString(message)+"</h1><p><a href=\"/\">返回 CCAI 首页</a></p></main></body></html>"))
	c.Abort()
}

func newsListViews(items []service.PublicNewsListItem) []publicNewsListView {
	views := make([]publicNewsListView, 0, len(items))
	for _, item := range items {
		views = append(views, publicNewsListView{
			Title:       item.Title,
			Summary:     item.Summary,
			SourceName:  item.SourceName,
			PublishedAt: formatPublicTime(item.PublishedAt),
			OriginalAt:  formatOptionalPublicTime(item.OriginalPublishedAt),
			URL:         publicContentPath("/news", publicLocaleSegment(item.Locale), item.Slug),
			ImageURL:    safePublicImageURL(item.ImageURL),
		})
	}
	return views
}

func articleListViews(items []service.PublicArticleListItem) []publicArticleListView {
	views := make([]publicArticleListView, 0, len(items))
	for _, item := range items {
		views = append(views, publicArticleListView{
			Title:       item.Title,
			Excerpt:     item.Excerpt,
			PublishedAt: formatPublicTime(item.PublishedAt),
			URL:         publicContentPath("/articles", publicLocaleSegment(item.Locale), item.Slug),
			ImageURL:    safePublicImageURL(item.CoverImage),
			ImageAlt:    item.CoverAlt,
		})
	}
	return views
}

func publicContentPath(prefix, locale, slug string) string {
	return strings.TrimRight(prefix, "/") + "/" + url.PathEscape(locale) + "/" + url.PathEscape(slug)
}

// longLocaleSegment maps a supported locale onto the short public URL segment
// (R9-07); unknown values are passed through lowercased so they still 404.
func publicLocaleSegment(locale string) string {
	if segment, ok := service.PublicLocaleSegment(locale); ok {
		return segment
	}
	if trimmed := strings.ToLower(strings.TrimSpace(locale)); trimmed != "" {
		return strings.SplitN(trimmed, "-", 2)[0]
	}
	return "zh"
}

// canonicalLocaleQuery keeps list/home canonical URLs short-segmented
// (?locale=en) instead of echoing the stored spelling (?locale=en-US).
func canonicalPublicCollectionURL(c *gin.Context, baseURL, locale, suffix string) string {
	segment, ok := service.PublicLocaleSegment(locale)
	if !ok {
		segment = "zh"
	}
	if suffix == "" {
		suffix = "/"
	}
	return publicAbsoluteURL(c, baseURL, "/"+segment+suffix, "")
}

func canonicalLocaleQuery(locale string) string {
	segment := publicLocaleSegment(locale)
	if segment == "zh" {
		return ""
	}
	return url.Values{"locale": {segment}}.Encode()
}

func publicAbsoluteURL(c *gin.Context, configuredBase, path, rawQuery string) string {
	base := strings.TrimRight(strings.TrimSpace(configuredBase), "/")
	if base == "" {
		base = defaultPublicBaseURL
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	result := base + path
	if rawQuery != "" {
		result += "?" + rawQuery
	}
	return result
}

func formatPublicTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func formatOptionalPublicTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatPublicTime(*value)
}

func firstPublicNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func publicJSONLD(value map[string]any) template.JS {
	raw, err := jsonMarshal(value)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(raw)
}

// jsonMarshal is kept behind a tiny function so the renderer's template code
// remains easy to audit: all values are generated from the public projection.
var jsonMarshal = func(value any) (string, error) {
	data, err := json.Marshal(value)
	return string(data), err
}

func newsJSONLD(item *service.PublicNews, title, description, canonical string) map[string]any {
	result := map[string]any{
		"@context":         "https://schema.org",
		"@type":            "NewsArticle",
		"headline":         title,
		"description":      description,
		"datePublished":    formatPublicTime(item.PublishedAt),
		"inLanguage":       item.Locale,
		"mainEntityOfPage": map[string]any{"@type": "WebPage", "@id": canonical},
		"publisher":        map[string]any{"@type": "Organization", "name": item.SourceName},
	}
	if image := safePublicImageURL(item.ImageURL); image != "" {
		result["image"] = image
	}
	return result
}

func articleJSONLD(item *service.PublicArticle, title, description, canonical string) map[string]any {
	result := map[string]any{
		"@context":         "https://schema.org",
		"@type":            "Article",
		"headline":         title,
		"description":      description,
		"datePublished":    formatPublicTime(item.PublishedAt),
		"inLanguage":       item.Locale,
		"author":           map[string]any{"@type": "Person", "name": firstPublicNonEmpty(item.Author, "CCAI 编辑部")},
		"mainEntityOfPage": map[string]any{"@type": "WebPage", "@id": canonical},
		"publisher":        map[string]any{"@type": "Organization", "name": "CCAI"},
	}
	if image := safePublicImageURL(firstPublicNonEmpty(item.OgImage, item.CoverImage)); image != "" {
		result["image"] = image
	}
	return result
}

func safePublicImageURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n\\") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	// Captured source images must be copied to, or served through, CCAI's media
	// proxy before publication. Rendering arbitrary remote URLs would disclose
	// source URLs and make visitors fetch untrusted resources directly.
	if u.IsAbs() {
		return ""
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}
	return ""
}

func sanitizePublicBody(raw string) template.HTML {
	if strings.TrimSpace(raw) == "" {
		return template.HTML("<p>正文内容暂不可用。</p>")
	}
	root := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(strings.NewReader(raw), root)
	if err != nil {
		return template.HTML("<p>正文内容暂不可用。</p>")
	}
	var out bytes.Buffer
	for _, node := range nodes {
		renderSanitizedNode(&out, node)
	}
	if strings.TrimSpace(out.String()) == "" {
		return template.HTML("<p>正文内容暂不可用。</p>")
	}
	return template.HTML(out.String())
}

var allowedPublicElements = map[string]bool{
	"p": true, "div": true, "span": true, "br": true, "hr": true,
	"h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"ul": true, "ol": true, "li": true, "blockquote": true,
	"pre": true, "code": true, "strong": true, "em": true, "b": true,
	"i": true, "u": true, "s": true, "del": true, "sup": true, "sub": true,
	"figure": true, "figcaption": true, "img": true, "video": true, "audio": true, "source": true,
	"table": true, "thead": true, "tbody": true, "tfoot": true,
	"tr": true, "th": true, "td": true,
}

var voidPublicElements = map[string]bool{"br": true, "hr": true, "img": true, "source": true}

func renderSanitizedNode(out *bytes.Buffer, node *xhtml.Node) {
	if node == nil {
		return
	}
	switch node.Type {
	case xhtml.TextNode:
		out.WriteString(stdhtml.EscapeString(node.Data))
	case xhtml.DocumentNode:
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			renderSanitizedNode(out, child)
		}
	case xhtml.ElementNode:
		name := strings.ToLower(node.Data)
		if name == "script" || name == "style" || name == "iframe" || name == "object" || name == "embed" || name == "form" || name == "input" || name == "button" || name == "textarea" || name == "select" {
			return
		}
		// Anchors are intentionally flattened. This prevents a captured source
		// link from becoming a public "read original" action.
		if name == "a" || !allowedPublicElements[name] {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				renderSanitizedNode(out, child)
			}
			return
		}
		out.WriteByte('<')
		out.WriteString(name)
		for _, attr := range node.Attr {
			key, value, ok := safePublicAttribute(name, attr.Key, attr.Val)
			if !ok {
				continue
			}
			out.WriteByte(' ')
			out.WriteString(key)
			out.WriteString(`="`)
			out.WriteString(stdhtml.EscapeString(value))
			out.WriteByte('"')
		}
		if voidPublicElements[name] {
			out.WriteString(" />")
			return
		}
		out.WriteByte('>')
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			renderSanitizedNode(out, child)
		}
		out.WriteString("</")
		out.WriteString(name)
		out.WriteByte('>')
	}
}

func safePublicAttribute(element, key, value string) (string, string, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return "", "", false
	}
	switch key {
	case "alt", "title":
		if element == "img" || key == "title" {
			return key, value, true
		}
	case "src", "poster":
		if (element == "img" || element == "video" || element == "audio" || element == "source") && safePublicMediaURL(value) != "" {
			return key, value, true
		}
	case "class":
		if publicClassPattern.MatchString(value) {
			return key, value, true
		}
	case "width", "height", "colspan", "rowspan", "start", "value":
		if _, err := strconv.Atoi(value); err == nil && len(value) <= 6 {
			return key, value, true
		}
	case "scope":
		if element == "th" && (value == "row" || value == "col" || value == "rowgroup" || value == "colgroup") {
			return key, value, true
		}
	}
	return "", "", false
}

func safePublicMediaURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n\\") {
		return ""
	}
	// Captured media must be copied to CCAI storage or served through a
	// controlled proxy before publication. Never render arbitrary source URLs.
	if strings.HasPrefix(raw, "/media/content/") || strings.HasPrefix(raw, "/public/media/") {
		return raw
	}
	return ""
}
