package admin

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ContentArticleHandler exposes the authenticated article CMS, translations and AI writing APIs.
type ContentArticleHandler struct {
	svc *service.ContentArticleAdminService
}

func NewContentArticleHandler(svc *service.ContentArticleAdminService) *ContentArticleHandler {
	return &ContentArticleHandler{svc: svc}
}

type contentArticleRequest struct {
	Locale         string     `json:"locale"`
	Slug           string     `json:"slug"`
	Title          string     `json:"title"`
	Excerpt        *string    `json:"excerpt"`
	BodyMarkdown   *string    `json:"bodyMarkdown"`
	BodyHTML       *string    `json:"bodyHtml"`
	CoverImage     *string    `json:"coverImage"`
	CoverAlt       *string    `json:"coverAlt"`
	Category       *string    `json:"category"`
	Tags           []string   `json:"tags"`
	Status         string     `json:"status"`
	AuthorID       *int64     `json:"authorId"`
	Model          *string    `json:"model"`
	SeoTitle       *string    `json:"seoTitle"`
	SeoDescription *string    `json:"seoDescription"`
	CanonicalURL   *string    `json:"canonicalUrl"`
	OgImage        *string    `json:"ogImage"`
	PublishedAt    *time.Time `json:"publishedAt"`
	Version        int        `json:"version"`
	// SyncLocales 仅用于本次保存后同步创建翻译记录，不参与落库。
	SyncLocales []string `json:"syncLocales"`
}
type contentTranslationRequest struct {
	ContentType      string  `json:"contentType"`
	ContentID        int64   `json:"contentId"`
	SourceVersion    int     `json:"sourceVersion"`
	SourceLocale     string  `json:"sourceLocale"`
	Locale           string  `json:"locale"`
	Slug             string  `json:"slug"`
	Title            string  `json:"title"`
	Excerpt          *string `json:"excerpt"`
	BodyMarkdown     *string `json:"bodyMarkdown"`
	BodyHTML         *string `json:"bodyHtml"`
	SeoTitle         *string `json:"seoTitle"`
	SeoDescription   *string `json:"seoDescription"`
	CanonicalURL     *string `json:"canonicalUrl"`
	TranslationModel *string `json:"translationModel"`
	FailureCode      *string `json:"failureCode"`
	Status           string  `json:"status"`
}

func (h *ContentArticleHandler) List(c *gin.Context) {
	page, size := response.ParsePagination(c)
	f := service.ContentArticleFilters{Status: c.Query("status"), Locale: c.Query("locale"), Category: c.Query("category"), Search: c.Query("search"), Tags: c.Query("tags"), Model: c.Query("model"), IncludeDeleted: c.Query("include_deleted") == "true"}
	if raw := c.Query("author_id"); raw != "" {
		var id int64
		if _, err := fmt.Sscan(raw, &id); err != nil || id <= 0 {
			response.BadRequest(c, "invalid author_id")
			return
		}
		f.AuthorID = &id
	}
	for key, target := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if raw := strings.TrimSpace(c.Query(key)); raw != "" {
			parsed, err := time.Parse("2006-01-02", raw)
			if err != nil {
				response.BadRequest(c, "invalid "+key+" date")
				return
			}
			if key == "to" {
				parsed = parsed.AddDate(0, 0, 1)
			}
			*target = &parsed
		}
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		response.BadRequest(c, "from must be before to")
		return
	}
	for key, target := range map[string]**int{"min_reads": &f.ReadMin, "max_reads": &f.ReadMax} {
		if raw := strings.TrimSpace(c.Query(key)); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				response.BadRequest(c, "invalid "+key)
				return
			}
			*target = &value
		}
	}
	if f.ReadMin != nil && f.ReadMax != nil && *f.ReadMin > *f.ReadMax {
		response.BadRequest(c, "min_reads must not exceed max_reads")
		return
	}
	out, e := h.svc.List(c.Request.Context(), f, page, size)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}
func (h *ContentArticleHandler) Bulk(c *gin.Context) {
	var req struct {
		IDs    []int64 `json:"ids" binding:"required"`
		Action string  `json:"action" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		response.BadRequest(c, "ids and action are required")
		return
	}
	if len(req.IDs) > 200 || (req.Action != "publish" && req.Action != "unpublish" && req.Action != "delete") {
		response.BadRequest(c, "invalid bulk action")
		return
	}
	failed := make([]int64, 0)
	for _, id := range req.IDs {
		var err error
		switch req.Action {
		case "publish":
			_, err = h.svc.Publish(contentMutationContext(c), id, nil)
		case "unpublish":
			_, err = h.svc.Unpublish(contentMutationContext(c), id)
		case "delete":
			err = h.svc.Delete(contentMutationContext(c), id)
		}
		if err != nil {
			failed = append(failed, id)
		}
	}
	response.Success(c, gin.H{"processed": len(req.IDs) - len(failed), "failedIds": failed})
}
func (h *ContentArticleHandler) Get(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}
	a, e := h.svc.Get(c.Request.Context(), id, c.Query("include_deleted") == "true")
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, a)
}
func (h *ContentArticleHandler) Create(c *gin.Context) {
	var req contentArticleRequest
	if e := c.ShouldBindJSON(&req); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	a := articleFromRequest(req)
	setArticleActor(c, &a.AuthorID)
	if a.Status == "" {
		a.Status = domain.ContentStatusDraft
	}
	out, e := h.svc.Create(contentMutationContext(c), &a)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Created(c, out)
}
func (h *ContentArticleHandler) AutoFillSEO(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}
	article, err := h.svc.Get(contentMutationContext(c), id, false)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if article == nil || (article.BodyMarkdown == nil && article.BodyHTML == nil) || (article.BodyMarkdown != nil && strings.TrimSpace(*article.BodyMarkdown) == "" && (article.BodyHTML == nil || strings.TrimSpace(*article.BodyHTML) == "")) {
		response.BadRequest(c, "article body is required")
		return
	}
	result, err := h.svc.GenerateSEO(contentMutationContext(c), service.ContentSEOInput{Title: article.Title, Summary: stringValue(article.Excerpt), BodyMarkdown: stringValue(article.BodyMarkdown), BodyHTML: stringValue(article.BodyHTML), Locale: article.Locale})
	if err != nil {
		if errors.Is(err, service.ErrContentAIModelNotConfigured) {
			c.JSON(400, gin.H{"code": 400, "reason": "model_not_configured", "message": "请先配置默认 AI 模型"})
			return
		}
		if errors.Is(err, service.ErrContentSEOResultInvalid) {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (h *ContentArticleHandler) Update(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}
	var req contentArticleRequest
	if e := c.ShouldBindJSON(&req); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	a := articleFromRequest(req)
	a.ID = id
	wasPublished := a.Status == domain.ContentStatusPublished
	if wasPublished {
		a.Status = domain.ContentStatusDraft
	}
	out, e := h.svc.Update(contentMutationContext(c), &a, req.Version)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	if wasPublished && out != nil {
		if pubOut, pubErr := h.svc.Publish(contentMutationContext(c), id, req.PublishedAt); pubErr == nil && pubOut != nil {
			out = pubOut
		}
	}
	response.Success(c, out)
}
func (h *ContentArticleHandler) Publish(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}
	var req struct {
		PublishedAt *time.Time `json:"publishedAt"`
	}
	_ = c.ShouldBindJSON(&req)
	out, e := h.svc.Publish(contentMutationContext(c), id, req.PublishedAt)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}
func (h *ContentArticleHandler) Unpublish(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}
	out, e := h.svc.Unpublish(contentMutationContext(c), id)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}
func (h *ContentArticleHandler) Delete(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}
	if e := h.svc.Delete(contentMutationContext(c), id); e != nil {
		response.ErrorFrom(c, e)
		return
	}
	c.Status(204)
}
func (h *ContentArticleHandler) Restore(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}
	out, e := h.svc.Restore(contentMutationContext(c), id)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}
func (h *ContentArticleHandler) ListTranslations(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}
	out, e := h.svc.ListTranslations(c.Request.Context(), "article", id, c.Query("include_deleted") == "true")
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}
func (h *ContentArticleHandler) GetTranslation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("translation_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid translation id")
		return
	}
	out, e := h.svc.GetTranslation(c.Request.Context(), id, c.Query("include_deleted") == "true")
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}
func (h *ContentArticleHandler) CreateTranslation(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}
	var req contentTranslationRequest
	if e := c.ShouldBindJSON(&req); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	t := translationFromRequest(req)
	t.ContentID = id
	if t.ContentType == "" {
		t.ContentType = "article"
	}
	if t.Status == "" {
		t.Status = domain.ContentTranslationDraft
	}
	out, e := h.svc.CreateTranslation(contentMutationContext(c), &t)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Created(c, out)
}
func (h *ContentArticleHandler) UpdateTranslation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("translation_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid translation id")
		return
	}
	var req contentTranslationRequest
	if e := c.ShouldBindJSON(&req); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	t := translationFromRequest(req)
	t.ID = id
	out, e := h.svc.UpdateTranslation(contentMutationContext(c), &t)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}
func (h *ContentArticleHandler) DeleteTranslation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("translation_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid translation id")
		return
	}
	if e := h.svc.DeleteTranslation(contentMutationContext(c), id); e != nil {
		response.ErrorFrom(c, e)
		return
	}
	c.Status(204)
}
func (h *ContentArticleHandler) PublishTranslation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("translation_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid translation id")
		return
	}
	out, e := h.svc.PublishTranslation(contentMutationContext(c), id, nil)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}
func (h *ContentArticleHandler) UnpublishTranslation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("translation_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid translation id")
		return
	}
	out, e := h.svc.UnpublishTranslation(contentMutationContext(c), id)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}
func (h *ContentArticleHandler) Models(c *gin.Context) {
	// Empty platform means the complete current CCAI catalog. The admin
	// writing UI uses this to expose every configured provider/model.
	platform := strings.TrimSpace(c.Query("platform"))
	out, e := h.svc.Models(platform)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}

func (h *ContentArticleHandler) GetAIConfig(c *gin.Context) {
	cfg, e := h.svc.GetAIProviderConfig(c.Request.Context())
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	cfg.APIKey = ""
	response.Success(c, cfg)
}

func (h *ContentArticleHandler) UpdateAIConfig(c *gin.Context) {
	var req service.ContentAIProviderConfig
	if e := c.ShouldBindJSON(&req); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	cfg, e := h.svc.UpdateAIProviderConfig(c.Request.Context(), req)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	cfg.APIKey = ""
	response.Success(c, cfg)
}

func (h *ContentArticleHandler) FetchAIModels(c *gin.Context) {
	var req struct {
		BaseURL string `json:"baseUrl"`
		APIKey  string `json:"apiKey"`
	}
	if e := c.ShouldBindJSON(&req); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	models, cfg, e := h.svc.FetchAIProviderModels(c.Request.Context(), req.BaseURL, req.APIKey)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	cfg.APIKey = ""
	response.Success(c, gin.H{
		"models": models,
		"config": cfg,
	})
}
func (h *ContentArticleHandler) ListAIConfigs(c *gin.Context) {
	cs, d, r, e := service.ListAIProviderConfigs()
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	c.Header("ETag", r)
	response.Success(c, gin.H{"configs": cs, "defaultId": d, "revision": r})
}
func (h *ContentArticleHandler) MutateAIConfig(c *gin.Context) {
	var in service.ContentAIProviderConfig
	if e := c.ShouldBindJSON(&in); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	action := c.Param("action")
	id := c.Param("id")
	if id == "" {
		id = in.ID
	}
	if e := h.svc.MutateAIProviderConfig(c.Request.Context(), action, id, c.GetHeader("If-Match"), in); e != nil {
		// Typed service errors (e.g. 503 encryption_not_configured) keep their own
		// status/reason; only untyped errors fall back to the 409 mapping below.
		var appErr *infraerrors.ApplicationError
		if errors.As(e, &appErr) {
			response.ErrorFrom(c, appErr)
			return
		}
		reason := "AI_CONFIG_CONFLICT"
		if strings.Contains(e.Error(), "revision conflict") {
			reason = "config_revision_conflict"
		} else if strings.Contains(e.Error(), "model_in_use") {
			reason = "model_in_use"
		}
		response.ErrorFrom(c, infraerrors.Conflict(reason, e.Error()))
		return
	}
	// 删除与其他变更保持同一响应约定：200 + 最新列表与 revision，便于前端一次刷新状态。
	h.ListAIConfigs(c)
}
func (h *ContentArticleHandler) TestAIConfig(c *gin.Context) {
	var in struct {
		BaseURL string `json:"baseUrl"`
		APIKey  string `json:"apiKey"`
	}
	if e := c.ShouldBindJSON(&in); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	out, e := service.TestAIProviderConnection(c.Request.Context(), in.BaseURL, in.APIKey)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}
func (h *ContentArticleHandler) GetTaxonomy(c *gin.Context) {
	cfg, e := h.svc.GetTaxonomy(c.Request.Context())
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentArticleHandler) MutateTaxonomy(c *gin.Context) {
	var req struct {
		Scope   string `json:"scope" binding:"required"`
		Action  string `json:"action" binding:"required"`
		Name    string `json:"name" binding:"required"`
		NewName string `json:"newName"`
	}
	if e := c.ShouldBindJSON(&req); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	cfg, e := h.svc.MutateTaxonomy(contentMutationContext(c), req.Scope, req.Action, req.Name, req.NewName)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentArticleHandler) CreateWritingTask(c *gin.Context) {
	var req struct {
		Topic          string   `json:"topic" binding:"required"`
		Audience       string   `json:"audience"`
		Language       string   `json:"language"`
		ContentType    string   `json:"contentType"`
		Keywords       []string `json:"keywords"`
		WordCount      int      `json:"wordCount"`
		Tone           string   `json:"tone"`
		Facts          []string `json:"facts"`
		InternalLinks  []string `json:"internalLinks"`
		Model          string   `json:"model" binding:"required"`
		ProviderKeyRef string   `json:"providerKeyRef"`
		ArticleID      int64    `json:"articleId"`
	}
	if e := c.ShouldBindJSON(&req); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	out, e := h.svc.CreateWritingTaskWithInput(contentMutationContext(c), service.ContentWritingInput{Topic: req.Topic, Audience: req.Audience, Language: req.Language, ContentType: req.ContentType, Keywords: req.Keywords, WordCount: req.WordCount, Tone: req.Tone, Facts: req.Facts, InternalLinks: req.InternalLinks, Model: req.Model, ProviderKeyRef: req.ProviderKeyRef, ArticleID: req.ArticleID})
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Accepted(c, out)
}

func (h *ContentArticleHandler) CreateTranslationTask(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}
	var req struct {
		Locale         string   `json:"locale"`
		Locales        []string `json:"locales"`
		Model          string   `json:"model" binding:"required"`
		ProviderKeyRef string   `json:"providerKeyRef"`
	}
	if e := c.ShouldBindJSON(&req); e != nil {
		response.BadRequest(c, e.Error())
		return
	}
	locales := append([]string(nil), req.Locales...)
	if strings.TrimSpace(req.Locale) != "" {
		locales = append(locales, req.Locale)
	}
	out, e := h.svc.CreateTranslationTasks(contentMutationContext(c), id, locales, req.Model, req.ProviderKeyRef)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	if len(out) == 1 {
		response.Accepted(c, out[0])
		return
	}
	response.Accepted(c, gin.H{"tasks": out})
}

// ListWritingTasks returns recent writing tasks (default kind=writing) for
// the AI studio task panel. Query params: kind, limit.
func (h *ContentArticleHandler) ListWritingTasks(c *gin.Context) {
	kind := strings.TrimSpace(c.DefaultQuery("kind", "writing"))
	limit := 50
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	out, e := h.svc.ListWritingTasks(c.Request.Context(), kind, limit)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}

func (h *ContentArticleHandler) GetWritingTask(c *gin.Context) {
	id := strings.TrimSpace(c.Param("task_id"))
	if id == "" {
		response.BadRequest(c, "invalid task id")
		return
	}
	out, e := h.svc.GetWritingTask(c.Request.Context(), id)
	if e != nil {
		if errors.Is(e, service.ErrContentWritingTaskNotFound) {
			response.NotFound(c, "writing task not found")
			return
		}
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}

func (h *ContentArticleHandler) CancelWritingTask(c *gin.Context) {
	id := strings.TrimSpace(c.Param("task_id"))
	if id == "" {
		response.BadRequest(c, "invalid task id")
		return
	}
	out, e := h.svc.CancelWritingTask(contentMutationContext(c), id)
	if e != nil {
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, out)
}

// DeleteWritingTask removes a writing task record from the AI studio panel.
// Queued or running tasks are cancelled first; generated articles are kept.
func (h *ContentArticleHandler) DeleteWritingTask(c *gin.Context) {
	id := strings.TrimSpace(c.Param("task_id"))
	if id == "" {
		response.BadRequest(c, "invalid task id")
		return
	}
	if e := h.svc.DeleteWritingTask(contentMutationContext(c), id); e != nil {
		if errors.Is(e, service.ErrContentWritingTaskNotFound) {
			response.NotFound(c, "writing task not found")
			return
		}
		response.ErrorFrom(c, e)
		return
	}
	response.Success(c, gin.H{"taskId": id, "deleted": true})
}
func articleID(c *gin.Context) (int64, bool) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id <= 0 {
		response.BadRequest(c, "invalid article id")
		return 0, false
	}
	return id, true
}
func articleFromRequest(r contentArticleRequest) service.ContentArticle {
	a := service.ContentArticle{Locale: strings.TrimSpace(r.Locale), Slug: strings.TrimSpace(r.Slug), Title: strings.TrimSpace(r.Title), Excerpt: r.Excerpt, BodyMarkdown: r.BodyMarkdown, BodyHTML: r.BodyHTML, CoverImage: r.CoverImage, CoverAlt: r.CoverAlt, Category: r.Category, Tags: r.Tags, Status: r.Status, AuthorID: r.AuthorID, Model: r.Model, SeoTitle: r.SeoTitle, SeoDescription: r.SeoDescription, CanonicalURL: r.CanonicalURL, OgImage: r.OgImage, PublishedAt: r.PublishedAt, Version: r.Version, SyncLocales: r.SyncLocales}
	autoEnrichArticleSEO(&a)
	return a
}

func autoEnrichArticleSEO(a *service.ContentArticle) {
	if a == nil {
		return
	}
	if a.Locale == "" {
		if containsCJK(a.Title) {
			a.Locale = "zh-CN"
		} else {
			a.Locale = "en-US"
		}
	}
	if a.Slug == "" && a.Title != "" {
		a.Slug = buildAutoSlug(a.Title)
	}
	bodyText := ""
	if a.BodyMarkdown != nil && strings.TrimSpace(*a.BodyMarkdown) != "" {
		bodyText = stripMarkdownPlain(*a.BodyMarkdown)
	} else if a.BodyHTML != nil && strings.TrimSpace(*a.BodyHTML) != "" {
		bodyText = stripMarkdownPlain(*a.BodyHTML)
	}
	if (a.Excerpt == nil || strings.TrimSpace(*a.Excerpt) == "") && bodyText != "" {
		ex := truncateRunes(bodyText, 140)
		a.Excerpt = &ex
	}
	if (a.SeoTitle == nil || strings.TrimSpace(*a.SeoTitle) == "") && a.Title != "" {
		cat := ""
		if a.Category != nil && strings.TrimSpace(*a.Category) != "" {
			cat = " · " + strings.TrimSpace(*a.Category)
		}
		st := truncateRunes(fmt.Sprintf("%s%s | CCAI", a.Title, cat), 150)
		a.SeoTitle = &st
	}
	if a.SeoDescription == nil || strings.TrimSpace(*a.SeoDescription) == "" {
		desc := ""
		if a.Excerpt != nil && strings.TrimSpace(*a.Excerpt) != "" {
			desc = strings.TrimSpace(*a.Excerpt)
		} else if bodyText != "" {
			desc = truncateRunes(bodyText, 150)
		} else if a.Title != "" {
			desc = a.Title
		}
		if len(a.Tags) > 0 && desc != "" {
			desc = truncateRunes(desc+"（关键词："+strings.Join(a.Tags, "、")+"）", 220)
		}
		if desc != "" {
			a.SeoDescription = &desc
		}
	}
	if (a.OgImage == nil || strings.TrimSpace(*a.OgImage) == "") && a.CoverImage != nil && strings.TrimSpace(*a.CoverImage) != "" {
		cp := strings.TrimSpace(*a.CoverImage)
		a.OgImage = &cp
	}
	if (a.CoverAlt == nil || strings.TrimSpace(*a.CoverAlt) == "") && a.Title != "" {
		alt := truncateRunes(a.Title, 120)
		a.CoverAlt = &alt
	}
}

func containsCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func buildAutoSlug(title string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) < 4 {
		slug = fmt.Sprintf("article-%d", time.Now().Unix()%1000000)
	}
	if len(slug) > 72 {
		slug = strings.Trim(slug[:72], "-")
	}
	return slug
}

func stripMarkdownPlain(md string) string {
	lines := strings.Split(md, "\n")
	var parts []string
	inCode := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inCode = !inCode
			continue
		}
		if inCode || t == "" || strings.HasPrefix(t, "!") || strings.HasPrefix(t, "|") || strings.HasPrefix(t, "---") {
			continue
		}
		t = strings.TrimLeft(t, "#>-*+0123456789. ")
		t = strings.ReplaceAll(t, "**", "")
		t = strings.ReplaceAll(t, "`", "")
		t = strings.TrimSpace(t)
		if t != "" {
			parts = append(parts, t)
		}
		if len(strings.Join(parts, " ")) > 220 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func truncateRunes(s string, max int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= max {
		return string(rs)
	}
	return string(rs[:max-1]) + "…"
}
func translationFromRequest(r contentTranslationRequest) service.ContentTranslation {
	return service.ContentTranslation{ContentType: r.ContentType, ContentID: r.ContentID, SourceVersion: r.SourceVersion, SourceLocale: r.SourceLocale, Locale: r.Locale, Slug: r.Slug, Title: r.Title, Excerpt: r.Excerpt, BodyMarkdown: r.BodyMarkdown, BodyHTML: r.BodyHTML, SeoTitle: r.SeoTitle, SeoDescription: r.SeoDescription, CanonicalURL: r.CanonicalURL, TranslationModel: r.TranslationModel, FailureCode: r.FailureCode, Status: r.Status}
}
func setArticleActor(c *gin.Context, id **int64) {
	if s, ok := middleware.GetAuthSubjectFromContext(c); ok && s.UserID > 0 {
		*id = &s.UserID
	}
}

func contentMutationContext(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		return service.WithContentAuditActor(ctx, subject.UserID)
	}
	return ctx
}
