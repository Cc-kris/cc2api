package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ContentNewsHandler exposes the admin news-source, news-item, scheduler and
// fetch-run APIs. Worker execution is intentionally separate: POST /fetch only
// creates an idempotent queued run for the worker to claim.
type ContentNewsHandler struct {
	svc *service.ContentNewsAdminService
}

func NewContentNewsHandler(svc *service.ContentNewsAdminService) *ContentNewsHandler {
	return &ContentNewsHandler{svc: svc}
}

type contentNewsSourceRequest struct {
	Name              string            `json:"name" binding:"required"`
	SourceType        string            `json:"sourceType" binding:"required"`
	MappingJSON       map[string]string `json:"mappingJson"`
	URL               *string           `json:"url"`
	Language          string            `json:"language"`
	Category          *string           `json:"category"`
	Enabled           *bool             `json:"enabled"`
	IntervalMinutes   *int              `json:"intervalMinutes"`
	RateLimitPerMin   int               `json:"rateLimitPerMinute"`
	TermsConfirmedAt  *time.Time        `json:"termsConfirmedAt"`
	AuthorizationNote *string           `json:"authorizationNote"`
	RobotsPolicy      *string           `json:"robotsPolicy"`
}

type contentNewsItemRequest struct {
	SourceID            int64                  `json:"sourceId"`
	ExternalID          *string                `json:"externalId"`
	SourceCanonicalURL  string                 `json:"sourceCanonicalUrl"`
	Slug                string                 `json:"slug"`
	Locale              string                 `json:"locale"`
	Title               string                 `json:"title" binding:"required"`
	Summary             *string                `json:"summary"`
	BodyHTML            *string                `json:"bodyHtml"`
	BodyText            *string                `json:"bodyText"`
	BodySourceType      *string                `json:"bodySourceType"`
	ImageURL            *string                `json:"imageUrl"`
	SourceName          *string                `json:"sourceName"`
	Category            *string                `json:"category"`
	Tags                []string               `json:"tags"`
	OriginalURL         *string                `json:"originalUrl"`
	OriginalPublishedAt *time.Time             `json:"originalPublishedAt"`
	PublishedAt         *time.Time             `json:"publishedAt"`
	Status              string                 `json:"status"`
	PublishMode         string                 `json:"publishMode"`
	SeoTitle            *string                `json:"seoTitle"`
	SeoDescription      *string                `json:"seoDescription"`
	MediaJSON           map[string]interface{} `json:"mediaJson"`
}

type contentAutomationRequest struct {
	Enabled            bool       `json:"enabled"`
	Timezone           string     `json:"timezone" binding:"required"`
	FetchCron          string     `json:"fetchCron" binding:"required"`
	PublishMode        string     `json:"publishMode" binding:"required"`
	DailyFetchLimit    int        `json:"dailyFetchLimit"`
	DailyPublishLimit  int        `json:"dailyPublishLimit"`
	PublishStartAt     *time.Time `json:"publishStartAt"`
	PublishEndAt       *time.Time `json:"publishEndAt"`
	PublishWindowStart string     `json:"publishWindowStart"`
	PublishWindowEnd   string     `json:"publishWindowEnd"`
	DedupeWindowHours  int        `json:"dedupeWindowHours"`
	TargetLocales      []string   `json:"targetLocales" binding:"required"`
}

// UnmarshalJSON accepts the admin form's clock-only publish window ("09:00")
// as well as the legacy RFC3339 timestamps, so both clients can post here.
func (r *contentAutomationRequest) UnmarshalJSON(data []byte) error {
	type plain contentAutomationRequest
	var raw struct {
		plain
		StartAt json.RawMessage `json:"publishStartAt"`
		EndAt   json.RawMessage `json:"publishEndAt"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = contentAutomationRequest(raw.plain)
	var err error
	if r.PublishStartAt, r.PublishWindowStart, err = parsePublishWindowValue(raw.StartAt); err != nil {
		return fmt.Errorf("publishStartAt: %w", err)
	}
	if r.PublishEndAt, r.PublishWindowEnd, err = parsePublishWindowValue(raw.EndAt); err != nil {
		return fmt.Errorf("publishEndAt: %w", err)
	}
	return nil
}

// parsePublishWindowValue decodes either "HH:MM" or an RFC3339 timestamp.
func parsePublishWindowValue(raw json.RawMessage) (*time.Time, string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, "", nil
	}
	if clock, err := domain.PublishWindowTime(text); err == nil {
		return clock, text, nil
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil, "", err
	}
	return &parsed, domain.PublishWindowClock(&parsed), nil
}

// ListNewsSources handles GET /content/news-sources.
func (h *ContentNewsHandler) ListNewsSources(c *gin.Context) {
	page, size := response.ParsePagination(c)
	filters := service.ContentNewsSourceFilters{SourceType: c.Query("type"), Language: c.Query("language")}
	if raw := c.Query("enabled"); raw != "" {
		v := strings.EqualFold(raw, "true")
		filters.Enabled = &v
	}
	result, err := h.svc.ListNewsSources(c.Request.Context(), filters, page, size, c.Query("include_deleted") == "true")
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentNewsHandler) GetNewsSource(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	item, err := h.svc.GetNewsSource(c.Request.Context(), id, c.Query("include_deleted") == "true")
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *ContentNewsHandler) CreateNewsSource(c *gin.Context) {
	var req contentNewsSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	source := sourceFromRequest(req)
	actorID(c, &source.CreatedBy, &source.UpdatedBy)
	item, err := h.svc.CreateNewsSource(contentMutationContext(c), &source)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Created(c, item)
}

func (h *ContentNewsHandler) UpdateNewsSource(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	var req contentNewsSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	source := sourceFromRequest(req)
	source.ID = id
	actorID(c, nil, &source.UpdatedBy)
	item, err := h.svc.UpdateNewsSource(contentMutationContext(c), &source)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *ContentNewsHandler) EnableNewsSource(c *gin.Context)  { h.setSourceEnabled(c, true) }
func (h *ContentNewsHandler) DisableNewsSource(c *gin.Context) { h.setSourceEnabled(c, false) }
func (h *ContentNewsHandler) setSourceEnabled(c *gin.Context, enabled bool) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	item, err := h.svc.SetNewsSourceEnabled(contentMutationContext(c), id, enabled)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, item)
}
func (h *ContentNewsHandler) DeleteNewsSource(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteNewsSource(contentMutationContext(c), id); err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, gin.H{"id": id, "deleted": true})
}
func (h *ContentNewsHandler) RestoreNewsSource(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	item, err := h.svc.RestoreNewsSource(contentMutationContext(c), id)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, item)
}

func (h *ContentNewsHandler) ListNews(c *gin.Context) {
	page, size := response.ParsePagination(c)
	filters := service.ContentNewsListFilters{Status: c.Query("status"), Locale: c.Query("locale"), Search: strings.TrimSpace(c.Query("search"))}
	for key, target := range map[string]**time.Time{"from": &filters.From, "to": &filters.To} {
		if value := strings.TrimSpace(c.Query(key)); value != "" {
			parsed, err := time.Parse("2006-01-02", value)
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
	if filters.From != nil && filters.To != nil && !filters.From.Before(*filters.To) {
		response.BadRequest(c, "from must be before to")
		return
	}
	if raw := c.Query("source_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			filters.SourceID = &id
		}
	}
	result, err := h.svc.ListNewsItems(c.Request.Context(), filters, page, size, c.Query("include_deleted") == "true")
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentNewsHandler) BulkNews(c *gin.Context) {
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
			_, err = h.svc.PublishNewsItem(contentMutationContext(c), id, nil)
		case "unpublish":
			_, err = h.svc.UnpublishNewsItem(contentMutationContext(c), id)
		case "delete":
			err = h.svc.DeleteNewsItem(contentMutationContext(c), id)
		}
		if err != nil {
			failed = append(failed, id)
		}
	}
	response.Success(c, gin.H{"processed": len(req.IDs) - len(failed), "failedIds": failed})
}

func (h *ContentNewsHandler) GetNews(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	item, err := h.svc.GetNewsItem(c.Request.Context(), id, c.Query("include_deleted") == "true")
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, item)
}
func (h *ContentNewsHandler) CreateNews(c *gin.Context) {
	var req contentNewsItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.SourceID <= 0 || strings.TrimSpace(req.Slug) == "" {
		response.BadRequest(c, "sourceId and slug are required")
		return
	}
	item := newsFromRequest(req)
	actorID(c, &item.CreatedBy, &item.UpdatedBy)
	out, err := h.svc.CreateNewsItem(contentMutationContext(c), &item)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Created(c, out)
}
func (h *ContentNewsHandler) UpdateNews(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	var req contentNewsItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	existing, _ := h.svc.GetNewsItem(c.Request.Context(), id, false)
	item := newsFromRequest(req)
	item.ID = id
	if existing != nil {
		if item.SourceID <= 0 {
			item.SourceID = existing.SourceID
		}
		if strings.TrimSpace(item.SourceCanonicalURL) == "" {
			item.SourceCanonicalURL = existing.SourceCanonicalURL
		}
		if strings.TrimSpace(item.Slug) == "" {
			item.Slug = existing.Slug
		}
		if strings.TrimSpace(req.Locale) == "" && existing.Locale != "" {
			item.Locale = existing.Locale
		}
		if strings.TrimSpace(req.Status) == "" && existing.Status != "" {
			item.Status = existing.Status
		}
		if strings.TrimSpace(req.PublishMode) == "" && existing.PublishMode != "" {
			item.PublishMode = existing.PublishMode
		}
		if item.SourceName == nil || strings.TrimSpace(*item.SourceName) == "" {
			item.SourceName = existing.SourceName
		}
		if item.Summary == nil {
			item.Summary = existing.Summary
		}
		if item.BodyText == nil {
			item.BodyText = existing.BodyText
		}
		if item.BodyHTML == nil {
			item.BodyHTML = existing.BodyHTML
		}
		if item.ImageURL == nil {
			item.ImageURL = existing.ImageURL
		}
		if item.OriginalPublishedAt == nil {
			item.OriginalPublishedAt = existing.OriginalPublishedAt
		}
		if item.PublishedAt == nil {
			item.PublishedAt = existing.PublishedAt
		}
		if item.BodySourceType == nil || strings.TrimSpace(*item.BodySourceType) == "" {
			item.BodySourceType = existing.BodySourceType
		}
		mergedMedia := make(map[string]interface{})
		for k, v := range existing.MediaJSON {
			mergedMedia[k] = v
		}
		for k, v := range item.MediaJSON {
			mergedMedia[k] = v
		}
		if req.Category != nil {
			mergedMedia["category"] = strings.TrimSpace(*req.Category)
		}
		if req.Tags != nil {
			mergedMedia["tags"] = req.Tags
		}
		if req.OriginalURL != nil {
			if value := strings.TrimSpace(*req.OriginalURL); value != "" {
				mergedMedia["originalUrl"] = value
			}
		}
		if len(mergedMedia) > 0 {
			item.MediaJSON = mergedMedia
		}
		if item.SeoTitle == nil || strings.TrimSpace(*item.SeoTitle) == "" {
			if existing.SeoTitle != nil && strings.TrimSpace(*existing.SeoTitle) != "" {
				item.SeoTitle = existing.SeoTitle
			}
		}
		if item.SeoDescription == nil || strings.TrimSpace(*item.SeoDescription) == "" {
			if existing.SeoDescription != nil && strings.TrimSpace(*existing.SeoDescription) != "" {
				item.SeoDescription = existing.SeoDescription
			}
		}
	} else {
		mergedMedia := make(map[string]interface{})
		for k, v := range item.MediaJSON {
			mergedMedia[k] = v
		}
		if req.Category != nil {
			mergedMedia["category"] = strings.TrimSpace(*req.Category)
		}
		if req.Tags != nil {
			mergedMedia["tags"] = req.Tags
		}
		if req.OriginalURL != nil {
			if value := strings.TrimSpace(*req.OriginalURL); value != "" {
				mergedMedia["originalUrl"] = value
			}
		}
		if len(mergedMedia) > 0 {
			item.MediaJSON = mergedMedia
		}
	}
	if item.SeoTitle == nil || strings.TrimSpace(*item.SeoTitle) == "" {
		st := truncateRunes(strings.TrimSpace(item.Title)+" | CCAI", 150)
		item.SeoTitle = &st
	}
	if item.SeoDescription == nil || strings.TrimSpace(*item.SeoDescription) == "" {
		desc := strings.TrimSpace(item.Title)
		if item.Summary != nil && strings.TrimSpace(*item.Summary) != "" {
			desc = truncateRunes(strings.TrimSpace(*item.Summary), 180)
		}
		item.SeoDescription = &desc
	}
	actorID(c, nil, &item.UpdatedBy)
	out, err := h.svc.UpdateNewsItem(contentMutationContext(c), &item)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, out)
}
func (h *ContentNewsHandler) PublishNews(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	var req struct {
		PublishAt *time.Time `json:"publishAt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, err.Error())
		return
	}
	out, err := h.svc.PublishNewsItem(contentMutationContext(c), id, req.PublishAt)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, out)
}
func (h *ContentNewsHandler) UnpublishNews(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	out, err := h.svc.UnpublishNewsItem(contentMutationContext(c), id)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, out)
}
func (h *ContentNewsHandler) DeleteNews(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteNewsItem(contentMutationContext(c), id); err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, gin.H{"id": id, "deleted": true})
}
func (h *ContentNewsHandler) RestoreNews(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	out, err := h.svc.RestoreNewsItem(contentMutationContext(c), id)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, out)
}

func (h *ContentNewsHandler) GetAutomationSettings(c *gin.Context) {
	out, err := h.svc.GetAutomationSetting(c.Request.Context())
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, out)
}
func (h *ContentNewsHandler) UpdateAutomationSettings(c *gin.Context) {
	var req contentAutomationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	setting := service.ContentAutomationSetting{Enabled: req.Enabled, Timezone: req.Timezone, FetchCron: req.FetchCron, PublishMode: req.PublishMode, DailyFetchLimit: req.DailyFetchLimit, DailyPublishLimit: req.DailyPublishLimit, PublishStartAt: req.PublishStartAt, PublishEndAt: req.PublishEndAt, PublishWindowStart: req.PublishWindowStart, PublishWindowEnd: req.PublishWindowEnd, DedupeWindowHours: req.DedupeWindowHours, TargetLocales: req.TargetLocales}
	actorID(c, nil, &setting.UpdatedBy)
	out, err := h.svc.UpsertAutomationSetting(contentMutationContext(c), &setting)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, out)
}
func (h *ContentNewsHandler) PauseAutomation(c *gin.Context) {
	out, err := h.svc.PauseAutomation(contentMutationContext(c))
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, out)
}
func (h *ContentNewsHandler) ResumeAutomation(c *gin.Context) {
	out, err := h.svc.ResumeAutomation(contentMutationContext(c))
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, out)
}

func (h *ContentNewsHandler) ListFetchRuns(c *gin.Context) {
	page, size := response.ParsePagination(c)
	f := service.ContentFetchRunFilters{Status: c.Query("status")}
	if raw := c.Query("source_id"); raw != "" {
		if id, e := strconv.ParseInt(raw, 10, 64); e == nil && id > 0 {
			f.SourceID = &id
		}
	}
	out, err := h.svc.ListFetchRuns(c.Request.Context(), f, page, size)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, out)
}
func (h *ContentNewsHandler) GetFetchRun(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	out, err := h.svc.GetFetchRun(c.Request.Context(), id)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Success(c, out)
}
func (h *ContentNewsHandler) FetchNewsSource(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	source, err := h.svc.GetNewsSource(c.Request.Context(), id, false)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	if !source.Enabled {
		response.Error(c, http.StatusConflict, "news source is disabled")
		return
	}
	var req struct {
		IdempotencyKey string `json:"idempotencyKey"`
	}
	_ = c.ShouldBindJSON(&req)
	key := strings.TrimSpace(req.IdempotencyKey)
	if key == "" {
		key = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	}
	if key == "" {
		response.BadRequest(c, "Idempotency-Key is required")
		return
	}
	out, err := h.svc.RunSourceNowWithTaskKey(c.Request.Context(), id, domain.ContentFetchTriggerManual, key)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Accepted(c, out)
}
func (h *ContentNewsHandler) RetryFetchRun(c *gin.Context) {
	id, ok := contentID(c)
	if !ok {
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		var req struct {
			IdempotencyKey string `json:"idempotencyKey"`
		}
		_ = c.ShouldBindJSON(&req)
		key = strings.TrimSpace(req.IdempotencyKey)
	}
	if key == "" {
		response.BadRequest(c, "Idempotency-Key is required")
		return
	}
	out, err := h.svc.RetryFetchRunWithTaskKey(c.Request.Context(), id, key)
	if err != nil {
		writeContentNewsError(c, err)
		return
	}
	response.Accepted(c, out)
}

func sourceFromRequest(req contentNewsSourceRequest) service.ContentNewsSource {
	enabled := false
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	interval := 15 // worker-safe fallback; repository persists NULL when override is nil
	var override *int
	if req.IntervalMinutes != nil {
		interval = *req.IntervalMinutes
		override = req.IntervalMinutes
	}
	rate := req.RateLimitPerMin
	if rate == 0 {
		rate = 60
	}
	return service.ContentNewsSource{Name: req.Name, SourceType: req.SourceType, MappingJSON: req.MappingJSON, URL: req.URL, Language: req.Language, Category: req.Category, Enabled: enabled, IntervalMinutes: interval, IntervalMinutesOverride: override, RateLimitPerMinute: rate, TermsConfirmedAt: req.TermsConfirmedAt, AuthorizationNote: req.AuthorizationNote, RobotsPolicy: req.RobotsPolicy}
}
func newsFromRequest(req contentNewsItemRequest) service.ContentNewsItem {
	status := req.Status
	if status == "" {
		status = domain.ContentStatusDraft
	}
	mode := req.PublishMode
	if mode == "" {
		mode = domain.ContentNewsPublishManual
	}
	locale := req.Locale
	if locale == "" {
		locale = "zh-CN"
	}
	return service.ContentNewsItem{SourceID: req.SourceID, ExternalID: req.ExternalID, SourceCanonicalURL: req.SourceCanonicalURL, Slug: req.Slug, Locale: locale, Title: req.Title, Summary: req.Summary, BodyHTML: req.BodyHTML, BodyText: req.BodyText, BodySourceType: req.BodySourceType, ImageURL: req.ImageURL, SourceName: req.SourceName, Category: req.Category, Tags: req.Tags, OriginalURL: req.OriginalURL, OriginalPublishedAt: req.OriginalPublishedAt, PublishedAt: req.PublishedAt, Status: status, PublishMode: mode, SeoTitle: req.SeoTitle, SeoDescription: req.SeoDescription, MediaJSON: req.MediaJSON}
}
func contentID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid content id")
		return 0, false
	}
	return id, true
}
func actorID(c *gin.Context, created, updated **int64) {
	if sub, ok := middleware.GetAuthSubjectFromContext(c); ok && sub.UserID > 0 {
		if created != nil {
			*created = &sub.UserID
		}
		if updated != nil {
			*updated = &sub.UserID
		}
	}
}
func writeContentNewsError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrContentNewsSourceNotFound), errors.Is(err, service.ErrContentNewsItemNotFound), errors.Is(err, service.ErrContentFetchRunNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrContentNewsNotPublishable), errors.Is(err, service.ErrContentNewsInvalidStatus):
		response.Error(c, 409, err.Error())
	case errors.Is(err, service.ErrContentNewsSourceDisabled):
		response.Error(c, 409, "新闻源未满足抓取条件：请先在编辑弹窗保存来源授权说明与 robots 策略，再启用后重试")
	case errors.Is(err, service.ErrContentNewsInvalidInput), errors.Is(err, service.ErrContentNewsInvalidCron):
		response.BadRequest(c, err.Error())
	default:
		response.ErrorFrom(c, err)
	}
}
