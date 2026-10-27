package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/robfig/cron/v3"
)

var (
	ErrContentNewsSourceNotFound  = errors.New("content news source not found")
	ErrContentNewsItemNotFound    = errors.New("content news item not found")
	ErrContentFetchRunNotFound    = errors.New("content fetch run not found")
	ErrContentAutomationNotFound  = errors.New("content automation setting not found")
	ErrContentNewsInvalidInput    = errors.New("invalid content news input")
	ErrContentNewsInvalidStatus   = errors.New("invalid content news status transition")
	ErrContentNewsNotPublishable  = errors.New("content news item is not publishable")
	ErrContentNewsInvalidCron     = errors.New("invalid content automation cron")
	ErrContentNewsSourceLeaseLost = errors.New("content news source lease lost")
)

type ContentNewsSource struct {
	ID                      int64             `json:"id"`
	CreatedAt               time.Time         `json:"createdAt"`
	UpdatedAt               time.Time         `json:"updatedAt"`
	DeletedAt               *time.Time        `json:"deletedAt,omitempty"`
	Name                    string            `json:"name"`
	SourceType              string            `json:"sourceType"`
	MappingJSON             map[string]string `json:"mappingJson,omitempty"`
	URL                     *string           `json:"url,omitempty"`
	Language                string            `json:"language"`
	Category                *string           `json:"category,omitempty"`
	Enabled                 bool              `json:"enabled"`
	IntervalMinutes         int               `json:"-"`
	IntervalMinutesOverride *int              `json:"intervalMinutes"`
	ScheduleMinutes         int               `json:"scheduleMinutes"`
	ScheduleLabel           string            `json:"scheduleLabel,omitempty"`
	FetchCron               string            `json:"fetchCron,omitempty"`
	RateLimitPerMinute      int               `json:"rateLimitPerMinute"`
	TermsConfirmedAt        *time.Time        `json:"termsConfirmedAt,omitempty"`
	AuthorizationNote       *string           `json:"authorizationNote,omitempty"`
	RobotsPolicy            *string           `json:"robotsPolicy,omitempty"`
	NextRunAt               *time.Time        `json:"nextRunAt,omitempty"`
	LastRunAt               *time.Time        `json:"lastRunAt,omitempty"`
	LastError               *string           `json:"lastError,omitempty"`
	CreatedBy               *int64            `json:"createdBy,omitempty"`
	UpdatedBy               *int64            `json:"updatedBy,omitempty"`
}

type ContentNewsItem struct {
	DedupeWindowHours   int                    `json:"-"`
	ID                  int64                  `json:"id"`
	CreatedAt           time.Time              `json:"createdAt"`
	UpdatedAt           time.Time              `json:"updatedAt"`
	DeletedAt           *time.Time             `json:"deletedAt,omitempty"`
	SourceID            int64                  `json:"sourceId"`
	ExternalID          *string                `json:"externalId,omitempty"`
	SourceCanonicalURL  string                 `json:"sourceCanonicalUrl"`
	Slug                string                 `json:"slug"`
	Locale              string                 `json:"locale"`
	Title               string                 `json:"title"`
	Summary             *string                `json:"summary,omitempty"`
	BodyHTML            *string                `json:"bodyHtml,omitempty"`
	BodyText            *string                `json:"bodyText,omitempty"`
	BodySourceType      *string                `json:"bodySourceType,omitempty"`
	BodyFetchedAt       *time.Time             `json:"bodyFetchedAt,omitempty"`
	BodyHash            *string                `json:"bodyHash,omitempty"`
	MediaJSON           map[string]interface{} `json:"mediaJson,omitempty"`
	ImageURL            *string                `json:"imageUrl,omitempty"`
	SourceName          *string                `json:"sourceName,omitempty"`
	Category            *string                `json:"category,omitempty"`
	Tags                []string               `json:"tags,omitempty"`
	OriginalURL         *string                `json:"originalUrl,omitempty"`
	OriginalPublishedAt *time.Time             `json:"originalPublishedAt,omitempty"`
	PublishedAt         *time.Time             `json:"publishedAt,omitempty"`
	Status              string                 `json:"status"`
	PublishMode         string                 `json:"publishMode"`
	SeoTitle            *string                `json:"seoTitle,omitempty"`
	SeoDescription      *string                `json:"seoDescription,omitempty"`
	FetchedAt           *time.Time             `json:"fetchedAt,omitempty"`
	ContentHash         *string                `json:"contentHash,omitempty"`
	CreatedBy           *int64                 `json:"createdBy,omitempty"`
	UpdatedBy           *int64                 `json:"updatedBy,omitempty"`
}

type ContentAutomationSetting struct {
	ID                 int64      `json:"id"`
	Enabled            bool       `json:"enabled"`
	Timezone           string     `json:"timezone"`
	FetchCron          string     `json:"fetchCron"`
	PublishMode        string     `json:"publishMode"`
	DailyFetchLimit    int        `json:"dailyFetchLimit"`
	DailyPublishLimit  int        `json:"dailyPublishLimit"`
	PublishStartAt     *time.Time `json:"publishStartAt,omitempty"`
	PublishEndAt       *time.Time `json:"publishEndAt,omitempty"`
	DedupeWindowHours  int        `json:"dedupeWindowHours"`
	PublishWindowStart string     `json:"publishWindowStart"`
	PublishWindowEnd   string     `json:"publishWindowEnd"`
	TargetLocales      []string   `json:"targetLocales"`
	UpdatedBy          *int64     `json:"updatedBy,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// MarshalJSON emits the publish window bounds as "HH:MM" because the admin form
// binds them to <input type="time">, which only accepts a clock value. The
// stored columns still hold a full timestamp (reference-date based).
func (s ContentAutomationSetting) MarshalJSON() ([]byte, error) {
	type alias ContentAutomationSetting
	clock := func(v *time.Time) *string {
		if v == nil {
			return nil
		}
		out := v.Format("15:04")
		return &out
	}
	return json.Marshal(struct {
		alias
		PublishStartAt *string `json:"publishStartAt,omitempty"`
		PublishEndAt   *string `json:"publishEndAt,omitempty"`
	}{alias(s), clock(s.PublishStartAt), clock(s.PublishEndAt)})
}

type ContentFetchRun struct {
	ID             int64      `json:"id"`
	SourceID       int64      `json:"sourceId"`
	SourceName     *string    `json:"sourceName,omitempty"`
	SourceDeleted  bool       `json:"sourceDeleted,omitempty"`
	TriggerType    string     `json:"triggerType"`
	Status         string     `json:"status"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
	FetchedCount   int        `json:"fetchedCount"`
	CreatedCount   int        `json:"createdCount"`
	DuplicateCount int        `json:"duplicateCount"`
	FailedCount    int        `json:"failedCount"`
	ErrorSummary   *string    `json:"errorSummary,omitempty"`
	TaskKey        string     `json:"taskKey"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type ContentNewsListFilters struct {
	Status   string
	SourceID *int64
	Locale   string
	Search   string
	From     *time.Time
	To       *time.Time
}

type ContentNewsSourceFilters struct {
	Enabled    *bool
	SourceType string
	Language   string
}

type ContentFetchRunFilters struct {
	SourceID *int64
	Status   string
	From     *time.Time
	To       *time.Time
}

type ContentPage[T any] struct {
	Items    []T  `json:"items"`
	Page     int  `json:"page"`
	PageSize int  `json:"pageSize"`
	Total    int  `json:"total"`
	HasNext  bool `json:"hasNext"`
}

// ContentNewsAdminRepository is the persistence port for content-management APIs.
// The repository must include soft-deleted rows only when includeDeleted is true.
type ContentNewsAdminRepository interface {
	ListNewsSources(context.Context, ContentNewsSourceFilters, int, int, bool) ([]ContentNewsSource, int, error)
	GetNewsSource(context.Context, int64, bool) (*ContentNewsSource, error)
	CreateNewsSource(context.Context, *ContentNewsSource) (*ContentNewsSource, error)
	UpdateNewsSource(context.Context, *ContentNewsSource) (*ContentNewsSource, error)
	SoftDeleteNewsSource(context.Context, int64, time.Time) error
	RestoreNewsSource(context.Context, int64) (*ContentNewsSource, error)

	ListNewsItems(context.Context, ContentNewsListFilters, int, int, bool) ([]ContentNewsItem, int, error)
	GetNewsItem(context.Context, int64, bool) (*ContentNewsItem, error)
	CreateNewsItem(context.Context, *ContentNewsItem) (*ContentNewsItem, error)
	UpdateNewsItem(context.Context, *ContentNewsItem) (*ContentNewsItem, error)
	SoftDeleteNewsItem(context.Context, int64, time.Time) error
	RestoreNewsItem(context.Context, int64) (*ContentNewsItem, error)

	GetAutomationSetting(context.Context) (*ContentAutomationSetting, error)
	UpsertAutomationSetting(context.Context, *ContentAutomationSetting) (*ContentAutomationSetting, error)

	ListFetchRuns(context.Context, ContentFetchRunFilters, int, int) ([]ContentFetchRun, int, error)
	GetFetchRun(context.Context, int64) (*ContentFetchRun, error)
	CreateFetchRun(context.Context, *ContentFetchRun) (*ContentFetchRun, error)
	UpdateFetchRun(context.Context, *ContentFetchRun) (*ContentFetchRun, error)
}

type ContentNewsAdminService struct {
	repo   ContentNewsAdminRepository
	worker *ContentNewsWorker
	audit  ContentAuditLogger
}

func NewContentNewsAdminService(repo ContentNewsAdminRepository, audit ...ContentAuditLogger) *ContentNewsAdminService {
	s := &ContentNewsAdminService{repo: repo}
	if len(audit) > 0 {
		s.audit = audit[0]
	}
	return s
}
func (s *ContentNewsAdminService) SetAuditLogger(a ContentAuditLogger) { s.audit = a }
func (s *ContentNewsAdminService) ListNewsTranslationTasks(ctx context.Context, newsID int64) ([]*ContentWritingTask, error) {
	repo, ok := s.repo.(interface {
		ListNewsTranslationTasks(context.Context, int64) ([]*ContentWritingTask, error)
	})
	if !ok {
		return []*ContentWritingTask{}, nil
	}
	return repo.ListNewsTranslationTasks(ctx, newsID)
}
func (s *ContentNewsAdminService) GetNewsTranslationTask(ctx context.Context, newsID int64, taskID string) (*ContentWritingTask, error) {
	repo, ok := s.repo.(interface {
		GetNewsTranslationTask(context.Context, int64, string) (*ContentWritingTask, error)
	})
	if !ok {
		return nil, ErrContentWritingTaskNotFound
	}
	return repo.GetNewsTranslationTask(ctx, newsID, taskID)
}
func (s *ContentNewsAdminService) auditNews(ctx context.Context, action string, before, after *ContentNewsItem) error {
	if s == nil || s.audit == nil {
		return nil
	}
	e := ContentAuditEntry{ContentType: "news", Action: action, ActorID: contentAuditActor(ctx)}
	if requestID, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		e.RequestID = requestID
	}
	if before != nil {
		e.ContentID = &before.ID
		e.Before = newsAuditSnapshot(before)
	}
	if after != nil {
		if e.ContentID == nil {
			e.ContentID = &after.ID
		}
		e.After = newsAuditSnapshot(after)
	}
	return s.audit.Log(ctx, e)
}

func newsAuditSnapshot(item *ContentNewsItem) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{
		"id": item.ID, "source_id": item.SourceID, "external_id": item.ExternalID,
		"source_canonical_url": item.SourceCanonicalURL, "slug": item.Slug, "locale": item.Locale,
		"title": item.Title, "summary": item.Summary, "body_html": item.BodyHTML, "body_text": item.BodyText,
		"body_source_type": item.BodySourceType, "body_fetched_at": item.BodyFetchedAt, "body_hash": item.BodyHash,
		"media_json": item.MediaJSON, "image_url": item.ImageURL, "source_name": item.SourceName,
		"original_published_at": item.OriginalPublishedAt, "published_at": item.PublishedAt,
		"status": item.Status, "publish_mode": item.PublishMode, "seo_title": item.SeoTitle,
		"seo_description": item.SeoDescription, "fetched_at": item.FetchedAt, "content_hash": item.ContentHash,
	}
}

func (s *ContentNewsAdminService) auditNewsResource(ctx context.Context, contentType, action string, id int64, before, after map[string]any) error {
	if s == nil || s.audit == nil {
		return nil
	}
	entry := ContentAuditEntry{ContentType: contentType, Action: action, ActorID: contentAuditActor(ctx), Before: before, After: after}
	if id > 0 {
		entry.ContentID = &id
	}
	if requestID, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		entry.RequestID = requestID
	}
	return s.audit.Log(ctx, entry)
}

func (s *ContentNewsAdminService) SetWorker(w *ContentNewsWorker) {
	if s != nil {
		s.worker = w
	}
}
func (s *ContentNewsAdminService) RunSourceNow(ctx context.Context, id int64, trigger string) (*ContentFetchRun, error) {
	if s == nil || s.worker == nil {
		return nil, errors.New("content news worker unavailable")
	}
	return s.worker.RunSource(ctx, id, trigger)
}

func (s *ContentNewsAdminService) RunSourceNowWithTaskKey(ctx context.Context, id int64, trigger, taskKey string) (*ContentFetchRun, error) {
	if s == nil || s.worker == nil {
		return nil, errors.New("content news worker unavailable")
	}
	return s.worker.EnqueueSource(ctx, id, trigger, taskKey)
}

func (s *ContentNewsAdminService) ListNewsSources(ctx context.Context, filters ContentNewsSourceFilters, page, pageSize int, includeDeleted bool) (ContentPage[ContentNewsSource], error) {
	page, pageSize = normalizeContentPage(page, pageSize)
	items, total, err := s.repo.ListNewsSources(ctx, filters, page, pageSize, includeDeleted)
	if err != nil {
		return makeContentPage(items, total, page, pageSize), err
	}
	s.decorateNewsSchedules(ctx, items)
	return makeContentPage(items, total, page, pageSize), nil
}

func (s *ContentNewsAdminService) GetNewsSource(ctx context.Context, id int64, includeDeleted bool) (*ContentNewsSource, error) {
	if id <= 0 {
		return nil, ErrContentNewsSourceNotFound
	}
	item, err := s.repo.GetNewsSource(ctx, id, includeDeleted)
	if err != nil || item == nil {
		return item, err
	}
	s.decorateNewsSchedule(ctx, item)
	return item, nil
}

// decorateNewsSchedules keeps the per-source interval in sync with the global
// fetch cron. The worker schedules from the cron, so the admin list must not show
// a stale interval_minutes column next to the real schedule.
func (s *ContentNewsAdminService) decorateNewsSchedules(ctx context.Context, items []ContentNewsSource) {
	if s == nil || len(items) == 0 {
		return
	}
	setting, err := s.repo.GetAutomationSetting(ctx)
	if err != nil || setting == nil {
		return
	}
	minutes, label := contentScheduleMinutes(setting.FetchCron)
	if minutes <= 0 {
		return
	}
	for i := range items {
		applyNewsSchedule(&items[i], minutes, label, setting.FetchCron)
	}
}

func (s *ContentNewsAdminService) decorateNewsSchedule(ctx context.Context, item *ContentNewsSource) {
	if item == nil {
		return
	}
	setting, err := s.repo.GetAutomationSetting(ctx)
	if err != nil || setting == nil {
		return
	}
	if minutes, label := contentScheduleMinutes(setting.FetchCron); minutes > 0 {
		applyNewsSchedule(item, minutes, label, setting.FetchCron)
	}
}

func applyNewsSchedule(item *ContentNewsSource, minutes int, label, cronExpr string) {
	item.ScheduleMinutes = minutes
	item.FetchCron = cronExpr
	if item.IntervalMinutesOverride == nil {
		item.IntervalMinutes = minutes
		item.ScheduleLabel = fmt.Sprintf("跟随全局（%d 分钟）", minutes)
		return
	}
	item.IntervalMinutes = *item.IntervalMinutesOverride
	item.ScheduleLabel = fmt.Sprintf("每 %d 分钟", item.IntervalMinutes)
}

// contentScheduleMinutes derives the effective interval from a cron expression.
func contentScheduleMinutes(expr string) (int, string) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, ""
	}
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expr)
	if err != nil {
		return 0, ""
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first := schedule.Next(base)
	second := schedule.Next(first)
	minutes := int(second.Sub(first) / time.Minute)
	if minutes <= 0 {
		return 0, ""
	}
	if minutes%(24*60) == 0 {
		return minutes, fmt.Sprintf("每 %d 天", minutes/(24*60))
	}
	if minutes >= 60 && minutes%60 == 0 {
		return minutes, fmt.Sprintf("每 %d 小时", minutes/60)
	}
	return minutes, fmt.Sprintf("每 %d 分钟", minutes)
}

func (s *ContentNewsAdminService) CreateNewsSource(ctx context.Context, source *ContentNewsSource) (*ContentNewsSource, error) {
	if source == nil {
		return nil, ErrContentNewsInvalidInput
	}
	applyNewsSourceDefaults(source)
	if err := validateNewsSource(ctx, source); err != nil {
		return nil, err
	}
	out, err := s.repo.CreateNewsSource(ctx, source)
	if err == nil {
		InvalidatePublicContentCache()
		err = s.auditNewsResource(ctx, "news_source", "create", out.ID, nil, map[string]any{"name": out.Name, "enabled": out.Enabled})
	}
	if err != nil {
		return out, err
	}
	s.decorateNewsSchedule(ctx, out)
	return out, nil
}

// applyNewsSourceDefaults records the terms confirmation implied by creating a
// source from the admin form, so a freshly created source can be enabled and
// fetched immediately instead of dead-ending on a 409.
func applyNewsSourceDefaults(source *ContentNewsSource) {
	if source == nil {
		return
	}
	now := time.Now().UTC()
	if source.TermsConfirmedAt == nil {
		source.TermsConfirmedAt = &now
	}
	if source.AuthorizationNote == nil || strings.TrimSpace(*source.AuthorizationNote) == "" {
		note := "管理员创建新闻源时确认：仅抓取公开内容，遵守来源站点 robots.txt 与服务条款。"
		source.AuthorizationNote = &note
	}
	if source.RobotsPolicy == nil || strings.TrimSpace(*source.RobotsPolicy) == "" {
		policy := "respect_robots"
		source.RobotsPolicy = &policy
	}
}

func (s *ContentNewsAdminService) UpdateNewsSource(ctx context.Context, source *ContentNewsSource) (*ContentNewsSource, error) {
	if source == nil || source.ID <= 0 {
		return nil, ErrContentNewsSourceNotFound
	}
	if err := validateNewsSource(ctx, source); err != nil {
		return nil, err
	}
	before, err := s.repo.GetNewsSource(ctx, source.ID, false)
	if err != nil {
		return nil, err
	}
	out, err := s.repo.UpdateNewsSource(ctx, source)
	if err == nil {
		InvalidatePublicContentCache()
		var beforeSnapshot map[string]any
		if before != nil {
			beforeSnapshot = map[string]any{"name": before.Name, "enabled": before.Enabled}
		}
		err = s.auditNewsResource(ctx, "news_source", "update", out.ID, beforeSnapshot, map[string]any{"name": out.Name, "enabled": out.Enabled})
	}
	if err != nil {
		return out, err
	}
	s.decorateNewsSchedule(ctx, out)
	return out, nil
}

func (s *ContentNewsAdminService) SetNewsSourceEnabled(ctx context.Context, id int64, enabled bool) (*ContentNewsSource, error) {
	source, err := s.GetNewsSource(ctx, id, false)
	if err != nil {
		return nil, err
	}
	source.Enabled = enabled
	if enabled {
		// Enabling a source from the admin console is itself the terms
		// confirmation, so legacy rows created before the field existed can
		// still be turned on instead of dead-ending on a validation error.
		applyNewsSourceDefaults(source)
		if err := validateNewsSource(ctx, source); err != nil {
			return nil, err
		}
	}
	out, err := s.repo.UpdateNewsSource(ctx, source)
	if err == nil {
		InvalidatePublicContentCache()
		err = s.auditNewsResource(ctx, "news_source", "set_enabled", out.ID, map[string]any{"enabled": !enabled}, map[string]any{"enabled": enabled})
	}
	if err != nil {
		return out, err
	}
	s.decorateNewsSchedule(ctx, out)
	return out, nil
}

func (s *ContentNewsAdminService) DeleteNewsSource(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrContentNewsSourceNotFound
	}
	before, err := s.repo.GetNewsSource(ctx, id, false)
	if err != nil {
		return err
	}
	err = s.repo.SoftDeleteNewsSource(ctx, id, time.Now().UTC())
	if err == nil {
		InvalidatePublicContentCache()
		err = s.auditNewsResource(ctx, "news_source", "delete", id, map[string]any{"name": before.Name, "enabled": before.Enabled}, map[string]any{"name": before.Name, "enabled": false})
	}
	return err
}

func (s *ContentNewsAdminService) RestoreNewsSource(ctx context.Context, id int64) (*ContentNewsSource, error) {
	if id <= 0 {
		return nil, ErrContentNewsSourceNotFound
	}
	before, err := s.repo.GetNewsSource(ctx, id, true)
	if err != nil {
		return nil, err
	}
	out, err := s.repo.RestoreNewsSource(ctx, id)
	if err == nil {
		InvalidatePublicContentCache()
		err = s.auditNewsResource(ctx, "news_source", "restore", id, map[string]any{"name": before.Name, "enabled": before.Enabled}, map[string]any{"name": out.Name, "enabled": out.Enabled})
	}
	return out, err
}

func (s *ContentNewsAdminService) ListNewsItems(ctx context.Context, filters ContentNewsListFilters, page, pageSize int, includeDeleted bool) (ContentPage[ContentNewsItem], error) {
	page, pageSize = normalizeContentPage(page, pageSize)
	items, total, err := s.repo.ListNewsItems(ctx, filters, page, pageSize, includeDeleted)
	return makeContentPage(items, total, page, pageSize), err
}

func (s *ContentNewsAdminService) GetNewsItem(ctx context.Context, id int64, includeDeleted bool) (*ContentNewsItem, error) {
	if id <= 0 {
		return nil, ErrContentNewsItemNotFound
	}
	return s.repo.GetNewsItem(ctx, id, includeDeleted)
}

func (s *ContentNewsAdminService) CreateNewsItem(ctx context.Context, item *ContentNewsItem) (*ContentNewsItem, error) {
	if err := validateNewsItem(item); err != nil {
		return nil, err
	}
	out, err := s.repo.CreateNewsItem(ctx, item)
	if err == nil {
		InvalidatePublicContentCache()
		if auditErr := s.auditNews(ctx, "create", nil, out); auditErr != nil {
			return out, auditErr
		}
	}
	return out, err
}

func (s *ContentNewsAdminService) UpdateNewsItem(ctx context.Context, item *ContentNewsItem) (*ContentNewsItem, error) {
	if item == nil || item.ID <= 0 {
		return nil, ErrContentNewsItemNotFound
	}
	if err := validateNewsItem(item); err != nil {
		return nil, err
	}
	if item.Status == domain.ContentStatusPublished {
		if err := validateNewsItemPublishable(item); err != nil {
			return nil, err
		}
	}
	before, err := s.repo.GetNewsItem(ctx, item.ID, false)
	if err != nil {
		return nil, err
	}
	out, err := s.repo.UpdateNewsItem(ctx, item)
	if err == nil {
		InvalidatePublicContentCache()
		if auditErr := s.auditNews(ctx, "update", before, out); auditErr != nil {
			return out, auditErr
		}
	}
	return out, err
}

func (s *ContentNewsAdminService) PublishNewsItem(ctx context.Context, id int64, publishAt *time.Time) (*ContentNewsItem, error) {
	item, err := s.GetNewsItem(ctx, id, false)
	if err != nil {
		return nil, err
	}
	if err := validateNewsItemPublishable(item); err != nil {
		return nil, err
	}
	before := *item
	item.Status = domain.ContentStatusPublished
	if publishAt == nil {
		now := time.Now().UTC()
		publishAt = &now
	}
	item.PublishedAt = publishAt
	out, err := s.repo.UpdateNewsItem(ctx, item)
	if err == nil {
		InvalidatePublicContentCache()
		if auditErr := s.auditNews(ctx, "publish", &before, out); auditErr != nil {
			return out, auditErr
		}
	}
	return out, err
}

func (s *ContentNewsAdminService) UnpublishNewsItem(ctx context.Context, id int64) (*ContentNewsItem, error) {
	item, err := s.GetNewsItem(ctx, id, false)
	if err != nil {
		return nil, err
	}
	if item.Status != domain.ContentStatusPublished {
		return nil, ErrContentNewsInvalidStatus
	}
	before := *item
	item.Status = domain.ContentStatusUnpublished
	out, err := s.repo.UpdateNewsItem(ctx, item)
	if err == nil {
		InvalidatePublicContentCache()
		if auditErr := s.auditNews(ctx, "unpublish", &before, out); auditErr != nil {
			return out, auditErr
		}
	}
	return out, err
}

func (s *ContentNewsAdminService) DeleteNewsItem(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrContentNewsItemNotFound
	}
	item, err := s.repo.GetNewsItem(ctx, id, false)
	if err != nil {
		return err
	}
	err = s.repo.SoftDeleteNewsItem(ctx, id, time.Now().UTC())
	if err == nil {
		InvalidatePublicContentCache()
		err = s.auditNews(ctx, "delete", item, nil)
	}
	return err
}

func (s *ContentNewsAdminService) RestoreNewsItem(ctx context.Context, id int64) (*ContentNewsItem, error) {
	if id <= 0 {
		return nil, ErrContentNewsItemNotFound
	}
	before, err := s.repo.GetNewsItem(ctx, id, true)
	if err != nil {
		return nil, err
	}
	out, err := s.repo.RestoreNewsItem(ctx, id)
	if err == nil {
		InvalidatePublicContentCache()
		if auditErr := s.auditNews(ctx, "restore", before, out); auditErr != nil {
			return out, auditErr
		}
	}
	return out, err
}

func (s *ContentNewsAdminService) GetAutomationSetting(ctx context.Context) (*ContentAutomationSetting, error) {
	setting, err := s.repo.GetAutomationSetting(ctx)
	if errors.Is(err, ErrContentAutomationNotFound) {
		return defaultContentAutomationSetting(), nil
	}
	return setting, err
}

func (s *ContentNewsAdminService) UpsertAutomationSetting(ctx context.Context, setting *ContentAutomationSetting) (*ContentAutomationSetting, error) {
	if err := validateAutomationSetting(setting); err != nil {
		return nil, err
	}
	before, getErr := s.repo.GetAutomationSetting(ctx)
	if getErr != nil && !errors.Is(getErr, ErrContentAutomationNotFound) {
		return nil, getErr
	}
	out, err := s.repo.UpsertAutomationSetting(ctx, setting)
	if err == nil {
		InvalidatePublicContentCache()
		var beforeSnapshot map[string]any
		if before != nil {
			beforeSnapshot = map[string]any{"enabled": before.Enabled, "timezone": before.Timezone, "fetchCron": before.FetchCron, "publishMode": before.PublishMode}
		}
		err = s.auditNewsResource(ctx, "automation", "update", 1, beforeSnapshot, map[string]any{"enabled": out.Enabled, "timezone": out.Timezone, "fetchCron": out.FetchCron, "publishMode": out.PublishMode})
	}
	return out, err
}

func (s *ContentNewsAdminService) PauseAutomation(ctx context.Context) (*ContentAutomationSetting, error) {
	setting, err := s.GetAutomationSetting(ctx)
	if err != nil {
		return nil, err
	}
	setting.Enabled = false
	out, err := s.repo.UpsertAutomationSetting(ctx, setting)
	if err == nil {
		InvalidatePublicContentCache()
		err = s.auditNewsResource(ctx, "automation", "pause", 1, map[string]any{"enabled": true}, map[string]any{"enabled": false})
	}
	return out, err
}

func (s *ContentNewsAdminService) ResumeAutomation(ctx context.Context) (*ContentAutomationSetting, error) {
	setting, err := s.GetAutomationSetting(ctx)
	if err != nil {
		return nil, err
	}
	setting.Enabled = true
	if err := validateAutomationSetting(setting); err != nil {
		return nil, err
	}
	out, err := s.repo.UpsertAutomationSetting(ctx, setting)
	if err == nil {
		InvalidatePublicContentCache()
		err = s.auditNewsResource(ctx, "automation", "resume", 1, map[string]any{"enabled": false}, map[string]any{"enabled": true})
	}
	return out, err
}

func (s *ContentNewsAdminService) ListFetchRuns(ctx context.Context, filters ContentFetchRunFilters, page, pageSize int) (ContentPage[ContentFetchRun], error) {
	page, pageSize = normalizeContentPage(page, pageSize)
	items, total, err := s.repo.ListFetchRuns(ctx, filters, page, pageSize)
	if err != nil {
		return makeContentPage(items, total, page, pageSize), err
	}
	s.decorateFetchRuns(ctx, items)
	return makeContentPage(items, total, page, pageSize), nil
}

// decorateFetchRuns resolves the source name for each run so the admin table can
// label a run even after its source was renamed or soft-deleted.
func (s *ContentNewsAdminService) decorateFetchRuns(ctx context.Context, items []ContentFetchRun) {
	names := make(map[int64]*string, len(items))
	deleted := make(map[int64]bool, len(items))
	for i := range items {
		id := items[i].SourceID
		if _, ok := names[id]; !ok {
			src, err := s.repo.GetNewsSource(ctx, id, true)
			if err != nil || src == nil {
				names[id] = nil
			} else {
				name := src.Name
				names[id] = &name
				deleted[id] = src.DeletedAt != nil
			}
		}
		items[i].SourceName = names[id]
		items[i].SourceDeleted = deleted[id]
	}
}

func (s *ContentNewsAdminService) GetFetchRun(ctx context.Context, id int64) (*ContentFetchRun, error) {
	if id <= 0 {
		return nil, ErrContentFetchRunNotFound
	}
	return s.repo.GetFetchRun(ctx, id)
}

func (s *ContentNewsAdminService) CreateFetchRun(ctx context.Context, run *ContentFetchRun) (*ContentFetchRun, error) {
	if run == nil || run.SourceID <= 0 {
		return nil, ErrContentNewsInvalidInput
	}
	if run.TriggerType == "" {
		run.TriggerType = domain.ContentFetchTriggerManual
	}
	if !domain.IsContentFetchTrigger(run.TriggerType) {
		return nil, ErrContentNewsInvalidInput
	}
	if run.Status == "" {
		run.Status = domain.ContentFetchQueued
	}
	if !domain.IsContentFetchStatus(run.Status) {
		return nil, ErrContentNewsInvalidInput
	}
	if strings.TrimSpace(run.TaskKey) == "" {
		return nil, ErrContentNewsInvalidInput
	}
	return s.repo.CreateFetchRun(ctx, run)
}

func (s *ContentNewsAdminService) RetryFetchRun(ctx context.Context, id int64) (*ContentFetchRun, error) {
	return s.retryFetchRun(ctx, id, fmt.Sprintf("retry-%d", id))
}

func (s *ContentNewsAdminService) RetryFetchRunWithTaskKey(ctx context.Context, id int64, key string) (*ContentFetchRun, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrContentNewsInvalidInput
	}
	return s.retryFetchRun(ctx, id, "retry-"+key)
}

func (s *ContentNewsAdminService) retryFetchRun(ctx context.Context, id int64, taskKey string) (*ContentFetchRun, error) {
	previous, err := s.GetFetchRun(ctx, id)
	if err != nil {
		return nil, err
	}
	switch previous.Status {
	case domain.ContentFetchSucceeded, domain.ContentFetchPartial, domain.ContentFetchFailed, domain.ContentFetchCancelled, domain.ContentFetchInterrupted:
	default:
		return nil, ErrContentNewsInvalidStatus
	}
	if s.worker != nil {
		// A retry is a new attempt. The previous run remains immutable and its
		// task key is never reused, while the key remains idempotent on retries of
		// the same HTTP request.
		return s.worker.EnqueueSource(ctx, previous.SourceID, domain.ContentFetchTriggerManual, taskKey)
	}
	return s.CreateFetchRun(ctx, &ContentFetchRun{
		SourceID:    previous.SourceID,
		TriggerType: domain.ContentFetchTriggerManual,
		Status:      domain.ContentFetchQueued,
		TaskKey:     taskKey,
	})
}

func (s *ContentNewsAdminService) UpdateFetchRun(ctx context.Context, run *ContentFetchRun) (*ContentFetchRun, error) {
	if run == nil || run.ID <= 0 || !domain.IsContentFetchStatus(run.Status) {
		return nil, ErrContentNewsInvalidInput
	}
	return s.repo.UpdateFetchRun(ctx, run)
}

func validateNewsSource(ctx context.Context, source *ContentNewsSource) error {
	if source == nil {
		return ErrContentNewsInvalidInput
	}
	source.Language = domain.NormalizeContentLocale(source.Language)
	if strings.TrimSpace(source.Name) == "" || len(strings.TrimSpace(source.Name)) > 80 || !domain.IsContentNewsSourceType(strings.TrimSpace(source.SourceType)) {
		return ErrContentNewsInvalidInput
	}
	source.Name = strings.TrimSpace(source.Name)
	source.SourceType = strings.TrimSpace(source.SourceType)
	source.Language = strings.TrimSpace(source.Language)
	if source.Language == "" {
		source.Language = "zh-CN"
	}
	if source.IntervalMinutesOverride != nil && (*source.IntervalMinutesOverride < 15 || *source.IntervalMinutesOverride > 1440) {
		return ErrContentNewsInvalidInput
	}
	if source.RateLimitPerMinute <= 0 {
		return ErrContentNewsInvalidInput
	}
	if source.URL != nil && strings.TrimSpace(*source.URL) != "" {
		if err := domain.ValidateContentNewsSourceURL(strings.TrimSpace(*source.URL)); err != nil {
			return ErrContentNewsInvalidInput
		}
		if source.SourceType != domain.ContentNewsSourceManual {
			if err := ValidateContentNewsFetchURL(ctx, strings.TrimSpace(*source.URL), nil); err != nil {
				return ErrContentNewsInvalidInput
			}
		}
	}
	if source.Enabled {
		return validateNewsSourceEnabled(source)
	}
	return nil
}

func validateNewsSourceEnabled(source *ContentNewsSource) error {
	if source == nil {
		return ErrContentNewsInvalidInput
	}
	if source.SourceType != domain.ContentNewsSourceManual && (source.URL == nil || strings.TrimSpace(*source.URL) == "") {
		return ErrContentNewsInvalidInput
	}
	if source.TermsConfirmedAt == nil || source.AuthorizationNote == nil || strings.TrimSpace(*source.AuthorizationNote) == "" {
		return ErrContentNewsInvalidInput
	}
	if source.RobotsPolicy == nil || strings.TrimSpace(*source.RobotsPolicy) == "" {
		return ErrContentNewsInvalidInput
	}
	return nil
}

func validateNewsItem(item *ContentNewsItem) error {
	if item == nil || item.SourceID <= 0 || strings.TrimSpace(item.Slug) == "" || strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Locale) == "" || !domain.IsContentStatus(item.Status) || !domain.IsContentNewsPublishMode(item.PublishMode) {
		return ErrContentNewsInvalidInput
	}
	item.Slug, item.Title = strings.TrimSpace(item.Slug), strings.TrimSpace(item.Title)
	// Locale is normalized on write so a "zh" admin write and a "zh-CN" public
	// read resolve to the same row.
	item.Locale = domain.NormalizeContentLocale(item.Locale)
	if item.OriginalURL != nil {
		setContentNewsOriginalURL(item, strings.TrimSpace(*item.OriginalURL))
	}
	if item.Status == domain.ContentStatusPublished {
		return validateNewsItemPublishable(item)
	}
	return nil
}

// setContentNewsOriginalURL stores the upstream link inside media_json as well as
// the dedicated field, so admin list/detail responses can render it directly.
func setContentNewsOriginalURL(item *ContentNewsItem, raw string) {
	if item == nil || raw == "" {
		return
	}
	value := raw
	item.OriginalURL = &value
	if item.MediaJSON == nil {
		item.MediaJSON = map[string]interface{}{}
	}
	item.MediaJSON["originalUrl"] = raw
}

func validateNewsItemPublishable(item *ContentNewsItem) error {
	if item == nil || item.BodyHTML == nil || strings.TrimSpace(*item.BodyHTML) == "" || item.BodyText == nil || strings.TrimSpace(*item.BodyText) == "" || item.Summary == nil || strings.TrimSpace(*item.Summary) == "" || item.SourceName == nil || strings.TrimSpace(*item.SourceName) == "" {
		return ErrContentNewsNotPublishable
	}
	return nil
}

func validateAutomationSetting(setting *ContentAutomationSetting) error {
	if setting == nil || strings.TrimSpace(setting.Timezone) == "" || setting.DailyFetchLimit < 0 || setting.DailyPublishLimit < 0 || !domain.IsContentAutomationPublishMode(setting.PublishMode) {
		return ErrContentNewsInvalidInput
	}
	// The admin form omits dedupeWindowHours; treat a missing value as the
	// documented default instead of rejecting the whole save.
	if setting.DedupeWindowHours <= 0 {
		setting.DedupeWindowHours = 24
	}
	if _, err := time.LoadLocation(strings.TrimSpace(setting.Timezone)); err != nil {
		return ErrContentNewsInvalidInput
	}
	if _, err := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse(strings.TrimSpace(setting.FetchCron)); err != nil {
		return fmt.Errorf("%w: %v", ErrContentNewsInvalidCron, err)
	}
	if err := applyAutomationPublishWindow(setting); err != nil {
		return err
	}
	if len(setting.TargetLocales) == 0 {
		return ErrContentNewsInvalidInput
	}
	locales := make([]string, 0, len(setting.TargetLocales))
	for _, locale := range setting.TargetLocales {
		if normalized := domain.NormalizeContentLocale(locale); normalized != "" {
			locales = append(locales, normalized)
		}
	}
	if len(locales) == 0 {
		return ErrContentNewsInvalidInput
	}
	setting.TargetLocales = locales
	setting.Timezone, setting.FetchCron = strings.TrimSpace(setting.Timezone), strings.TrimSpace(setting.FetchCron)
	return nil
}

// applyAutomationPublishWindow accepts the "HH:MM" window the admin form posts and
// maps it onto the publish_start_at/publish_end_at columns, which the worker reads
// as a clock-of-day window.
func applyAutomationPublishWindow(setting *ContentAutomationSetting) error {
	startRaw := strings.TrimSpace(setting.PublishWindowStart)
	endRaw := strings.TrimSpace(setting.PublishWindowEnd)
	if startRaw == "" && endRaw == "" {
		// Legacy clients post RFC3339 timestamps instead of a clock window.
		if setting.PublishStartAt != nil && setting.PublishEndAt != nil && setting.PublishStartAt.After(*setting.PublishEndAt) {
			return fmt.Errorf("%w: %s", ErrContentNewsInvalidInput, "发布时间窗开始需早于结束")
		}
		setting.PublishWindowStart = domain.PublishWindowClock(setting.PublishStartAt)
		setting.PublishWindowEnd = domain.PublishWindowClock(setting.PublishEndAt)
		return nil
	}
	start, err := domain.PublishWindowTime(startRaw)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrContentNewsInvalidInput, "发布时间窗开始格式应为 HH:MM")
	}
	end, err := domain.PublishWindowTime(endRaw)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrContentNewsInvalidInput, "发布时间窗结束格式应为 HH:MM")
	}
	if start.After(*end) {
		return fmt.Errorf("%w: %s", ErrContentNewsInvalidInput, "发布时间窗开始需早于结束")
	}
	setting.PublishStartAt, setting.PublishEndAt = start, end
	setting.PublishWindowStart = start.Format("15:04")
	setting.PublishWindowEnd = end.Format("15:04")
	return nil
}

func defaultContentAutomationSetting() *ContentAutomationSetting {
	return &ContentAutomationSetting{Timezone: "Asia/Shanghai", FetchCron: "0 */15 * * * *", PublishMode: domain.ContentAutomationManualReview, DedupeWindowHours: 24, TargetLocales: []string{"zh-CN"}}
}

func normalizeContentPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func makeContentPage[T any](items []T, total, page, pageSize int) ContentPage[T] {
	return ContentPage[T]{Items: items, Total: total, Page: page, PageSize: pageSize, HasNext: page*pageSize < total}
}
