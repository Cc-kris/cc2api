package handler

import (
	"encoding/xml"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	defaultPublicLocale  = "zh-CN"
	defaultPublicBaseURL = "https://ccai.xyz"
)

// PublicContentHandler exposes the anonymous, SEO-indexable content endpoints.
type PublicContentHandler struct {
	service *service.PublicContentService
	baseURL string
}

func NewPublicContentHandler(contentService *service.PublicContentService, cfg *config.Config) *PublicContentHandler {
	baseURL := defaultPublicBaseURL
	if cfg != nil {
		if configured := strings.TrimRight(strings.TrimSpace(cfg.Server.FrontendURL), "/"); configured != "" {
			baseURL = configured
		}
	}
	service.RegisterPublicContentCacheInvalidator(InvalidatePublicContentHTMLCache)
	return &PublicContentHandler{service: contentService, baseURL: baseURL}
}

func (h *PublicContentHandler) ListNews(c *gin.Context) {
	page, pageSize, ok := publicPageQuery(c)
	if !ok {
		return
	}
	data, err := h.service.ListNews(c.Request.Context(), publicLocaleQuery(c), page, pageSize)
	if err != nil {
		publicContentError(c, err)
		return
	}
	response.Success(c, data)
}

func (h *PublicContentHandler) GetNews(c *gin.Context) {
	item, err := h.service.GetNews(c.Request.Context(), c.Param("locale"), c.Param("slug"))
	if err != nil {
		publicContentError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *PublicContentHandler) ListArticles(c *gin.Context) {
	page, pageSize, ok := publicPageQuery(c)
	if !ok {
		return
	}
	data, err := h.service.ListArticles(c.Request.Context(), publicLocaleQuery(c), page, pageSize)
	if err != nil {
		publicContentError(c, err)
		return
	}
	response.Success(c, data)
}

func (h *PublicContentHandler) ListFeaturedArticles(c *gin.Context) {
	data, err := h.service.ListFeaturedArticles(c.Request.Context(), publicLocaleQuery(c))
	if err != nil {
		publicContentError(c, err)
		return
	}
	response.Success(c, data)
}

func (h *PublicContentHandler) GetArticle(c *gin.Context) {
	item, err := h.service.GetArticle(c.Request.Context(), c.Param("locale"), c.Param("slug"))
	if err != nil {
		publicContentError(c, err)
		return
	}
	response.Success(c, item)
}

// RenderNewsHTML and RenderArticleHTML provide the server-rendered detail shell.
// BodyHTML is trusted content produced by the publishing pipeline; all metadata is escaped.
func (h *PublicContentHandler) RenderNewsHTML(c *gin.Context) {
	item, err := h.service.GetNews(c.Request.Context(), c.Param("locale"), c.Param("slug"))
	if err != nil {
		publicContentHTMLError(c, err)
		return
	}
	renderNewsDocument(c, h.baseURL, item)
}

func (h *PublicContentHandler) RenderArticleHTML(c *gin.Context) {
	item, err := h.service.GetArticle(c.Request.Context(), c.Param("locale"), c.Param("slug"))
	if err != nil {
		publicContentHTMLError(c, err)
		return
	}
	renderArticleDocument(c, h.baseURL, item)
}

// RenderHomeHTML serves the anonymous public homepage. It intentionally uses
// the same published projections as the list pages so crawlers see real data.
func (h *PublicContentHandler) RenderHomeHTML(c *gin.Context) {
	locale := publicLocaleQuery(c)
	news, err := h.service.ListNews(c.Request.Context(), locale, 1, 6)
	if err != nil {
		publicContentHTMLError(c, err)
		return
	}
	articles, err := h.service.ListArticles(c.Request.Context(), locale, 1, 6)
	if err != nil {
		publicContentHTMLError(c, err)
		return
	}
	renderHomeDocument(c, h.baseURL, locale, news, articles)
}

// RenderNewsListHTML serves the indexable news collection page.
func (h *PublicContentHandler) RenderNewsListHTML(c *gin.Context) {
	page, pageSize, err := parsePublicPageQuery(c)
	if err != nil {
		publicContentHTMLError(c, err)
		return
	}
	locale := publicLocaleQuery(c)
	items, err := h.service.ListNews(c.Request.Context(), locale, page, pageSize)
	if err != nil {
		publicContentHTMLError(c, err)
		return
	}
	renderNewsListDocument(c, h.baseURL, locale, items)
}

// RenderArticleListHTML serves the indexable article collection page.
func (h *PublicContentHandler) RenderArticleListHTML(c *gin.Context) {
	page, pageSize, err := parsePublicPageQuery(c)
	if err != nil {
		publicContentHTMLError(c, err)
		return
	}
	locale := publicLocaleQuery(c)
	items, err := h.service.ListArticles(c.Request.Context(), locale, page, pageSize)
	if err != nil {
		publicContentHTMLError(c, err)
		return
	}
	renderArticleListDocument(c, h.baseURL, locale, items)
}

// RenderDocsHTML serves the public API documentation entry point and nested
// documentation paths. Documentation is static for now and contains no
// account, key, or model-square data.
func (h *PublicContentHandler) RenderDocsHTML(c *gin.Context) {
	renderDocsDocument(c, h.baseURL, publicLocaleQuery(c), c.Param("path"))
}

func (h *PublicContentHandler) RenderRobotsTXT(c *gin.Context) {
	base := strings.TrimRight(h.baseURL, "/")
	c.Header("Cache-Control", "public, max-age=60")
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte("User-agent: *\nAllow: /\nDisallow: /model-square\nDisallow: /api/v1/model-square\nDisallow: /dashboard\nDisallow: /keys\nDisallow: /admin/\nDisallow: /api/v1/admin/\nDisallow: /login\nSitemap: "+base+"/sitemap.xml\n"))
	c.Abort()
}

type publicSitemapURLSet struct {
	XMLName xml.Name           `xml:"urlset"`
	XMLNS   string             `xml:"xmlns,attr"`
	URLs    []publicSitemapURL `xml:"url"`
}

type publicSitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

func (h *PublicContentHandler) RenderSitemapXML(c *gin.Context) {
	entries, err := h.service.ListSitemapEntries(c.Request.Context())
	if err != nil {
		c.Header("X-Robots-Tag", "noindex, nofollow, noarchive")
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusInternalServerError, "sitemap temporarily unavailable")
		c.Abort()
		return
	}
	urls := []publicSitemapURL{{Loc: publicAbsoluteURL(c, h.baseURL, "/", "")}, {Loc: publicAbsoluteURL(c, h.baseURL, "/news", "")}, {Loc: publicAbsoluteURL(c, h.baseURL, "/articles", "")}, {Loc: publicAbsoluteURL(c, h.baseURL, "/docs", "")}}
	seenURLs := make(map[string]bool, len(urls))
	for _, item := range urls {
		seenURLs[item.Loc] = true
	}
	seenLocales := map[string]bool{}
	for _, entry := range entries {
		if !seenLocales[entry.Locale] {
			seenLocales[entry.Locale] = true
			for _, listPath := range []string{"/news", "/articles"} {
				loc := publicAbsoluteURL(c, h.baseURL, listPath, canonicalLocaleQuery(entry.Locale))
				if !seenURLs[loc] {
					seenURLs[loc] = true
					urls = append(urls, publicSitemapURL{Loc: loc})
				}
			}
		}
		prefix := "/news"
		if entry.Kind == "article" {
			prefix = "/articles"
		}
		loc := publicAbsoluteURL(c, h.baseURL, publicContentPath(prefix, entry.Locale, entry.Slug), "")
		if !seenURLs[loc] {
			seenURLs[loc] = true
			urls = append(urls, publicSitemapURL{Loc: loc, LastMod: formatPublicTime(entry.LastModified)})
		}
	}
	doc, err := xml.Marshal(publicSitemapURLSet{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: urls})
	if err != nil {
		renderPublicError(c, http.StatusInternalServerError, "CCAI", "页面暂时不可用")
		return
	}
	c.Header("X-Robots-Tag", "noindex, nofollow, noarchive")
	c.Header("Cache-Control", "public, max-age=60")
	c.Data(http.StatusOK, "application/xml; charset=utf-8", append([]byte(xml.Header), doc...))
	c.Abort()
}

func publicPageQuery(c *gin.Context) (int, int, bool) {
	page, pageSize, err := parsePublicPageQuery(c)
	if err != nil {
		response.BadRequest(c, "invalid pagination parameters")
		return 0, 0, false
	}
	return page, pageSize, true
}

func parsePublicPageQuery(c *gin.Context) (int, int, error) {
	page, pageSize := 1, 0
	var err error
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		page, err = strconv.Atoi(raw)
	}
	if err == nil {
		if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
			pageSize, err = strconv.Atoi(raw)
		}
	}
	if err != nil || page < 1 || pageSize < 0 || pageSize > service.MaxPublicContentPageSize {
		return 0, 0, errors.New("invalid public content pagination parameters")
	}
	return page, pageSize, nil
}

func publicLocaleQuery(c *gin.Context) string {
	if locale := strings.TrimSpace(c.Query("locale")); locale != "" {
		return locale
	}
	return defaultPublicLocale
}

func publicContentError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrPublicContentGone) {
		response.Error(c, http.StatusGone, "content gone")
		return
	}
	if errors.Is(err, service.ErrPublicContentNotFound) {
		response.Error(c, http.StatusNotFound, "content not found")
		return
	}
	if strings.Contains(err.Error(), "invalid public content") {
		response.BadRequest(c, "invalid public content parameters")
		return
	}
	response.Error(c, http.StatusInternalServerError, "internal error")
}

func publicContentHTMLError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	title := "CCAI"
	message := "内容暂时不可用"
	if errors.Is(err, service.ErrPublicContentGone) {
		status = http.StatusGone
		title = "内容已删除 | CCAI"
		message = "该内容已被删除"
	} else if errors.Is(err, service.ErrPublicContentNotFound) {
		status = http.StatusNotFound
		title = "内容未找到 | CCAI"
		message = "找不到要查看的内容"
	} else if strings.Contains(err.Error(), "invalid public content") {
		status = http.StatusBadRequest
		title = "请求无效 | CCAI"
		message = "请求参数无效"
	}
	renderPublicError(c, status, title, message)
}
