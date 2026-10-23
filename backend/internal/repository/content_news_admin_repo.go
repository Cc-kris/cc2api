package repository

import (
	"context"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/contentautomationsetting"
	"github.com/Wei-Shaw/sub2api/ent/contentfetchrun"
	"github.com/Wei-Shaw/sub2api/ent/contentnewsitem"
	"github.com/Wei-Shaw/sub2api/ent/contentnewssource"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type contentNewsAdminRepository struct{ client *dbent.Client }

func NewContentNewsAdminRepository(client *dbent.Client) *contentNewsAdminRepository {
	return &contentNewsAdminRepository{client: client}
}

func (r *contentNewsAdminRepository) ListNewsSources(ctx context.Context, filters service.ContentNewsSourceFilters, page, pageSize int, includeDeleted bool) ([]service.ContentNewsSource, int, error) {
	qctx := ctx
	if includeDeleted {
		qctx = mixins.SkipSoftDelete(ctx)
	}
	query := r.client.ContentNewsSource.Query().Order(contentnewssource.ByCreatedAt(sql.OrderDesc()), contentnewssource.ByID(sql.OrderDesc()))
	if filters.Enabled != nil {
		query = query.Where(contentnewssource.EnabledEQ(*filters.Enabled))
	}
	if v := strings.TrimSpace(filters.SourceType); v != "" {
		query = query.Where(contentnewssource.SourceTypeEQ(v))
	}
	if v := strings.TrimSpace(filters.Language); v != "" {
		query = query.Where(contentnewssource.LanguageEQ(v))
	}
	total, err := query.Count(qctx)
	if err != nil {
		return nil, 0, err
	}
	items, err := query.Offset((page - 1) * pageSize).Limit(pageSize).All(qctx)
	if err != nil {
		return nil, 0, err
	}
	return mapContentNewsSources(items), total, nil
}

func (r *contentNewsAdminRepository) GetNewsSource(ctx context.Context, id int64, includeDeleted bool) (*service.ContentNewsSource, error) {
	qctx := ctx
	if includeDeleted {
		qctx = mixins.SkipSoftDelete(ctx)
	}
	item, err := r.client.ContentNewsSource.Query().Where(contentnewssource.IDEQ(id)).Only(qctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrContentNewsSourceNotFound
		}
		return nil, err
	}
	out := mapContentNewsSource(item)
	return &out, nil
}

func (r *contentNewsAdminRepository) CreateNewsSource(ctx context.Context, source *service.ContentNewsSource) (*service.ContentNewsSource, error) {
	b := r.client.ContentNewsSource.Create().SetName(source.Name).SetSourceType(source.SourceType).SetMappingJSON(source.MappingJSON).SetLanguage(source.Language).SetEnabled(source.Enabled).SetIntervalMinutes(source.IntervalMinutes).SetRateLimitPerMinute(source.RateLimitPerMinute)
	if source.URL != nil {
		b.SetURL(*source.URL)
	}
	if source.Category != nil {
		b.SetCategory(*source.Category)
	}
	if source.TermsConfirmedAt != nil {
		b.SetTermsConfirmedAt(*source.TermsConfirmedAt)
	}
	if source.AuthorizationNote != nil {
		b.SetAuthorizationNote(*source.AuthorizationNote)
	}
	if source.RobotsPolicy != nil {
		b.SetRobotsPolicy(*source.RobotsPolicy)
	}
	if source.NextRunAt != nil {
		b.SetNextRunAt(*source.NextRunAt)
	}
	if source.LastRunAt != nil {
		b.SetLastRunAt(*source.LastRunAt)
	}
	if source.LastError != nil {
		b.SetLastError(*source.LastError)
	}
	if source.CreatedBy != nil {
		b.SetCreatedBy(*source.CreatedBy)
	}
	if source.UpdatedBy != nil {
		b.SetUpdatedBy(*source.UpdatedBy)
	}
	item, err := b.Save(ctx)
	if err != nil {
		return nil, err
	}
	out := mapContentNewsSource(item)
	return &out, nil
}

func (r *contentNewsAdminRepository) UpdateNewsSource(ctx context.Context, source *service.ContentNewsSource) (*service.ContentNewsSource, error) {
	b := r.client.ContentNewsSource.UpdateOneID(source.ID).SetName(source.Name).SetSourceType(source.SourceType).SetMappingJSON(source.MappingJSON).SetLanguage(source.Language).SetEnabled(source.Enabled).SetIntervalMinutes(source.IntervalMinutes).SetRateLimitPerMinute(source.RateLimitPerMinute)
	setOrClearString(b.SetURL, b.ClearURL, source.URL)
	setOrClearString(b.SetCategory, b.ClearCategory, source.Category)
	setOrClearTime(b.SetTermsConfirmedAt, b.ClearTermsConfirmedAt, source.TermsConfirmedAt)
	setOrClearString(b.SetAuthorizationNote, b.ClearAuthorizationNote, source.AuthorizationNote)
	setOrClearString(b.SetRobotsPolicy, b.ClearRobotsPolicy, source.RobotsPolicy)
	setOrClearTime(b.SetNextRunAt, b.ClearNextRunAt, source.NextRunAt)
	setOrClearTime(b.SetLastRunAt, b.ClearLastRunAt, source.LastRunAt)
	setOrClearString(b.SetLastError, b.ClearLastError, source.LastError)
	setOrClearInt64(b.SetCreatedBy, b.ClearCreatedBy, source.CreatedBy)
	setOrClearInt64(b.SetUpdatedBy, b.ClearUpdatedBy, source.UpdatedBy)
	item, err := b.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrContentNewsSourceNotFound
		}
		return nil, err
	}
	out := mapContentNewsSource(item)
	return &out, nil
}

func (r *contentNewsAdminRepository) SoftDeleteNewsSource(ctx context.Context, id int64, at time.Time) error {
	_, err := r.client.ContentNewsSource.Update().Where(contentnewssource.IDEQ(id), contentnewssource.DeletedAtIsNil()).SetDeletedAt(at).SetEnabled(false).Save(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (r *contentNewsAdminRepository) RestoreNewsSource(ctx context.Context, id int64) (*service.ContentNewsSource, error) {
	item, err := r.client.ContentNewsSource.Query().Where(contentnewssource.IDEQ(id)).Only(mixins.SkipSoftDelete(ctx))
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrContentNewsSourceNotFound
		}
		return nil, err
	}
	updated, err := r.client.ContentNewsSource.UpdateOneID(id).ClearDeletedAt().Save(ctx)
	if err != nil {
		return nil, err
	}
	out := mapContentNewsSource(updated)
	_ = item
	return &out, nil
}

func (r *contentNewsAdminRepository) ListNewsItems(ctx context.Context, filters service.ContentNewsListFilters, page, pageSize int, includeDeleted bool) ([]service.ContentNewsItem, int, error) {
	qctx := ctx
	if includeDeleted {
		qctx = mixins.SkipSoftDelete(ctx)
	}
	query := r.client.ContentNewsItem.Query().Order(contentnewsitem.ByCreatedAt(sql.OrderDesc()), contentnewsitem.ByID(sql.OrderDesc()))
	if filters.Status != "" {
		query = query.Where(contentnewsitem.StatusEQ(strings.TrimSpace(filters.Status)))
	}
	if filters.SourceID != nil {
		query = query.Where(contentnewsitem.SourceIDEQ(*filters.SourceID))
	}
	if filters.Locale != "" {
		query = query.Where(contentnewsitem.LocaleEQ(strings.TrimSpace(filters.Locale)))
	}
	if filters.Search != "" {
		query = query.Where(contentnewsitem.Or(contentnewsitem.TitleContainsFold(filters.Search), contentnewsitem.SummaryContainsFold(filters.Search)))
	}
	if filters.From != nil {
		query = query.Where(contentnewsitem.CreatedAtGTE(*filters.From))
	}
	if filters.To != nil {
		query = query.Where(contentnewsitem.CreatedAtLT(*filters.To))
	}
	total, err := query.Count(qctx)
	if err != nil {
		return nil, 0, err
	}
	items, err := query.Offset((page - 1) * pageSize).Limit(pageSize).All(qctx)
	if err != nil {
		return nil, 0, err
	}
	return mapContentNewsItems(items), total, nil
}

func (r *contentNewsAdminRepository) GetNewsItem(ctx context.Context, id int64, includeDeleted bool) (*service.ContentNewsItem, error) {
	qctx := ctx
	if includeDeleted {
		qctx = mixins.SkipSoftDelete(ctx)
	}
	item, err := r.client.ContentNewsItem.Query().Where(contentnewsitem.IDEQ(id)).Only(qctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrContentNewsItemNotFound
		}
		return nil, err
	}
	out := mapContentNewsItem(item)
	return &out, nil
}

func (r *contentNewsAdminRepository) CreateNewsItem(ctx context.Context, item *service.ContentNewsItem) (*service.ContentNewsItem, error) {
	b := r.client.ContentNewsItem.Create().SetSourceID(item.SourceID).SetSourceCanonicalURL(item.SourceCanonicalURL).SetSlug(item.Slug).SetLocale(item.Locale).SetTitle(item.Title).SetMediaJSON(item.MediaJSON).SetStatus(item.Status).SetPublishMode(item.PublishMode)
	if item.ExternalID != nil {
		b.SetExternalID(*item.ExternalID)
	}
	setCreateNewsItemOptional(b, item)
	out, err := b.Save(ctx)
	if err != nil {
		return nil, err
	}
	result := mapContentNewsItem(out)
	return &result, nil
}

func (r *contentNewsAdminRepository) UpdateNewsItem(ctx context.Context, item *service.ContentNewsItem) (*service.ContentNewsItem, error) {
	b := r.client.ContentNewsItem.UpdateOneID(item.ID).SetSourceID(item.SourceID).SetSourceCanonicalURL(item.SourceCanonicalURL).SetSlug(item.Slug).SetLocale(item.Locale).SetTitle(item.Title).SetMediaJSON(item.MediaJSON).SetStatus(item.Status).SetPublishMode(item.PublishMode)
	setOrClearString(b.SetExternalID, b.ClearExternalID, item.ExternalID)
	setOrClearString(b.SetSummary, b.ClearSummary, item.Summary)
	setOrClearString(b.SetBodyHTML, b.ClearBodyHTML, item.BodyHTML)
	setOrClearString(b.SetBodyText, b.ClearBodyText, item.BodyText)
	setOrClearString(b.SetBodySourceType, b.ClearBodySourceType, item.BodySourceType)
	setOrClearTime(b.SetBodyFetchedAt, b.ClearBodyFetchedAt, item.BodyFetchedAt)
	setOrClearString(b.SetBodyHash, b.ClearBodyHash, item.BodyHash)
	setOrClearString(b.SetImageURL, b.ClearImageURL, item.ImageURL)
	setOrClearString(b.SetSourceName, b.ClearSourceName, item.SourceName)
	setOrClearTime(b.SetOriginalPublishedAt, b.ClearOriginalPublishedAt, item.OriginalPublishedAt)
	setOrClearTime(b.SetPublishedAt, b.ClearPublishedAt, item.PublishedAt)
	setOrClearString(b.SetSeoTitle, b.ClearSeoTitle, item.SeoTitle)
	setOrClearString(b.SetSeoDescription, b.ClearSeoDescription, item.SeoDescription)
	setOrClearTime(b.SetFetchedAt, b.ClearFetchedAt, item.FetchedAt)
	setOrClearString(b.SetContentHash, b.ClearContentHash, item.ContentHash)
	setOrClearInt64(b.SetCreatedBy, b.ClearCreatedBy, item.CreatedBy)
	setOrClearInt64(b.SetUpdatedBy, b.ClearUpdatedBy, item.UpdatedBy)
	out, err := b.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrContentNewsItemNotFound
		}
		return nil, err
	}
	result := mapContentNewsItem(out)
	return &result, nil
}

func (r *contentNewsAdminRepository) SoftDeleteNewsItem(ctx context.Context, id int64, at time.Time) error {
	_, err := r.client.ContentNewsItem.Update().Where(contentnewsitem.IDEQ(id), contentnewsitem.DeletedAtIsNil()).SetDeletedAt(at).SetStatus(domain.ContentStatusDeleted).Save(ctx)
	return err
}
func (r *contentNewsAdminRepository) RestoreNewsItem(ctx context.Context, id int64) (*service.ContentNewsItem, error) {
	if _, err := r.client.ContentNewsItem.Query().Where(contentnewsitem.IDEQ(id)).Only(mixins.SkipSoftDelete(ctx)); err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrContentNewsItemNotFound
		}
		return nil, err
	}
	out, err := r.client.ContentNewsItem.UpdateOneID(id).ClearDeletedAt().SetStatus(domain.ContentStatusDraft).Save(ctx)
	if err != nil {
		return nil, err
	}
	result := mapContentNewsItem(out)
	return &result, nil
}

func (r *contentNewsAdminRepository) GetAutomationSetting(ctx context.Context) (*service.ContentAutomationSetting, error) {
	item, err := r.client.ContentAutomationSetting.Query().Order(contentautomationsetting.ByID(sql.OrderAsc())).First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrContentAutomationNotFound
		}
		return nil, err
	}
	out := mapContentAutomationSetting(item)
	return &out, nil
}
func (r *contentNewsAdminRepository) UpsertAutomationSetting(ctx context.Context, setting *service.ContentAutomationSetting) (*service.ContentAutomationSetting, error) {
	var item *dbent.ContentAutomationSetting
	var err error
	item, err = r.client.ContentAutomationSetting.Query().Order(contentautomationsetting.ByID(sql.OrderAsc())).First(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, err
	}
	var saved *dbent.ContentAutomationSetting
	if dbent.IsNotFound(err) {
		b := r.client.ContentAutomationSetting.Create().SetEnabled(setting.Enabled).SetTimezone(setting.Timezone).SetFetchCron(setting.FetchCron).SetPublishMode(setting.PublishMode).SetDailyFetchLimit(setting.DailyFetchLimit).SetDailyPublishLimit(setting.DailyPublishLimit).SetDedupeWindowHours(setting.DedupeWindowHours).SetTargetLocales(setting.TargetLocales)
		if setting.PublishStartAt != nil {
			b.SetPublishStartAt(*setting.PublishStartAt)
		}
		if setting.PublishEndAt != nil {
			b.SetPublishEndAt(*setting.PublishEndAt)
		}
		if setting.UpdatedBy != nil {
			b.SetUpdatedBy(*setting.UpdatedBy)
		}
		saved, err = b.Save(ctx)
	} else {
		b := r.client.ContentAutomationSetting.UpdateOneID(item.ID).SetEnabled(setting.Enabled).SetTimezone(setting.Timezone).SetFetchCron(setting.FetchCron).SetPublishMode(setting.PublishMode).SetDailyFetchLimit(setting.DailyFetchLimit).SetDailyPublishLimit(setting.DailyPublishLimit).SetDedupeWindowHours(setting.DedupeWindowHours).SetTargetLocales(setting.TargetLocales)
		setOrClearTime(b.SetPublishStartAt, b.ClearPublishStartAt, setting.PublishStartAt)
		setOrClearTime(b.SetPublishEndAt, b.ClearPublishEndAt, setting.PublishEndAt)
		setOrClearInt64(b.SetUpdatedBy, b.ClearUpdatedBy, setting.UpdatedBy)
		saved, err = b.Save(ctx)
	}
	if err != nil {
		return nil, err
	}
	out := mapContentAutomationSetting(saved)
	return &out, nil
}

func (r *contentNewsAdminRepository) ListFetchRuns(ctx context.Context, filters service.ContentFetchRunFilters, page, pageSize int) ([]service.ContentFetchRun, int, error) {
	query := r.client.ContentFetchRun.Query().Order(contentfetchrun.ByCreatedAt(sql.OrderDesc()), contentfetchrun.ByID(sql.OrderDesc()))
	if filters.SourceID != nil {
		query = query.Where(contentfetchrun.SourceIDEQ(*filters.SourceID))
	}
	if filters.Status != "" {
		query = query.Where(contentfetchrun.StatusEQ(strings.TrimSpace(filters.Status)))
	}
	if filters.From != nil {
		query = query.Where(contentfetchrun.CreatedAtGTE(*filters.From))
	}
	if filters.To != nil {
		query = query.Where(contentfetchrun.CreatedAtLT(*filters.To))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	items, err := query.Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return mapContentFetchRuns(items), total, nil
}
func (r *contentNewsAdminRepository) GetFetchRun(ctx context.Context, id int64) (*service.ContentFetchRun, error) {
	item, err := r.client.ContentFetchRun.Query().Where(contentfetchrun.IDEQ(id)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrContentFetchRunNotFound
		}
		return nil, err
	}
	out := mapContentFetchRun(item)
	return &out, nil
}
func (r *contentNewsAdminRepository) CreateFetchRun(ctx context.Context, run *service.ContentFetchRun) (*service.ContentFetchRun, error) {
	b := r.client.ContentFetchRun.Create().SetSourceID(run.SourceID).SetTriggerType(run.TriggerType).SetStatus(run.Status).SetTaskKey(run.TaskKey).SetFetchedCount(run.FetchedCount).SetCreatedCount(run.CreatedCount).SetDuplicateCount(run.DuplicateCount).SetFailedCount(run.FailedCount)
	if run.StartedAt != nil {
		b.SetStartedAt(*run.StartedAt)
	}
	if run.FinishedAt != nil {
		b.SetFinishedAt(*run.FinishedAt)
	}
	if run.ErrorSummary != nil {
		b.SetErrorSummary(*run.ErrorSummary)
	}
	out, err := b.Save(ctx)
	if err != nil {
		return nil, err
	}
	result := mapContentFetchRun(out)
	return &result, nil
}
func (r *contentNewsAdminRepository) UpdateFetchRun(ctx context.Context, run *service.ContentFetchRun) (*service.ContentFetchRun, error) {
	b := r.client.ContentFetchRun.UpdateOneID(run.ID).SetSourceID(run.SourceID).SetTriggerType(run.TriggerType).SetStatus(run.Status).SetTaskKey(run.TaskKey).SetFetchedCount(run.FetchedCount).SetCreatedCount(run.CreatedCount).SetDuplicateCount(run.DuplicateCount).SetFailedCount(run.FailedCount)
	setOrClearTime(b.SetStartedAt, b.ClearStartedAt, run.StartedAt)
	setOrClearTime(b.SetFinishedAt, b.ClearFinishedAt, run.FinishedAt)
	setOrClearString(b.SetErrorSummary, b.ClearErrorSummary, run.ErrorSummary)
	out, err := b.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrContentFetchRunNotFound
		}
		return nil, err
	}
	result := mapContentFetchRun(out)
	return &result, nil
}

func mapContentNewsSource(m *dbent.ContentNewsSource) service.ContentNewsSource {
	return service.ContentNewsSource{ID: m.ID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, DeletedAt: m.DeletedAt, Name: m.Name, SourceType: m.SourceType, MappingJSON: m.MappingJSON, URL: m.URL, Language: m.Language, Category: m.Category, Enabled: m.Enabled, IntervalMinutes: m.IntervalMinutes, RateLimitPerMinute: m.RateLimitPerMinute, TermsConfirmedAt: m.TermsConfirmedAt, AuthorizationNote: m.AuthorizationNote, RobotsPolicy: m.RobotsPolicy, NextRunAt: m.NextRunAt, LastRunAt: m.LastRunAt, LastError: m.LastError, CreatedBy: m.CreatedBy, UpdatedBy: m.UpdatedBy}
}
func mapContentNewsSources(items []*dbent.ContentNewsSource) []service.ContentNewsSource {
	out := make([]service.ContentNewsSource, len(items))
	for i, m := range items {
		out[i] = mapContentNewsSource(m)
	}
	return out
}
func mapContentNewsItem(m *dbent.ContentNewsItem) service.ContentNewsItem {
	item := service.ContentNewsItem{ID: m.ID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, DeletedAt: m.DeletedAt, SourceID: m.SourceID, ExternalID: m.ExternalID, SourceCanonicalURL: m.SourceCanonicalURL, Slug: m.Slug, Locale: m.Locale, Title: m.Title, Summary: m.Summary, BodyHTML: m.BodyHTML, BodyText: m.BodyText, BodySourceType: m.BodySourceType, BodyFetchedAt: m.BodyFetchedAt, BodyHash: m.BodyHash, MediaJSON: m.MediaJSON, ImageURL: m.ImageURL, SourceName: m.SourceName, OriginalPublishedAt: m.OriginalPublishedAt, PublishedAt: m.PublishedAt, Status: m.Status, PublishMode: m.PublishMode, SeoTitle: m.SeoTitle, SeoDescription: m.SeoDescription, FetchedAt: m.FetchedAt, ContentHash: m.ContentHash, CreatedBy: m.CreatedBy, UpdatedBy: m.UpdatedBy}
	// media_json is the storage home for category/tags/originalUrl; surface them as
	// first-class fields so admin lists and edit forms do not have to dig into it.
	if m.MediaJSON != nil {
		if raw, ok := m.MediaJSON["category"].(string); ok {
			if value := strings.TrimSpace(raw); value != "" {
				item.Category = &value
			}
		}
		item.Tags = mediaJSONStringSlice(m.MediaJSON["tags"])
		if raw, ok := m.MediaJSON["originalUrl"].(string); ok {
			if value := strings.TrimSpace(raw); value != "" {
				item.OriginalURL = &value
			}
		}
	}
	if item.OriginalURL == nil && strings.TrimSpace(m.SourceCanonicalURL) != "" {
		value := strings.TrimSpace(m.SourceCanonicalURL)
		item.OriginalURL = &value
	}
	return item
}

// mediaJSONStringSlice accepts the slice shapes that can round-trip through JSON.
func mediaJSONStringSlice(raw interface{}) []string {
	switch value := raw.(type) {
	case []string:
		return value
	case []interface{}:
		out := make([]string, 0, len(value))
		for _, entry := range value {
			if text, ok := entry.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	}
	return nil
}
func mapContentNewsItems(items []*dbent.ContentNewsItem) []service.ContentNewsItem {
	out := make([]service.ContentNewsItem, len(items))
	for i, m := range items {
		out[i] = mapContentNewsItem(m)
	}
	return out
}
func mapContentAutomationSetting(m *dbent.ContentAutomationSetting) service.ContentAutomationSetting {
	// publish_start_at/publish_end_at are stored as clock-of-day bounds; expose
	// them as "HH:MM" strings so the admin form can round-trip a <input type=time>.
	return service.ContentAutomationSetting{ID: m.ID, Enabled: m.Enabled, Timezone: m.Timezone, FetchCron: m.FetchCron, PublishMode: m.PublishMode, DailyFetchLimit: m.DailyFetchLimit, DailyPublishLimit: m.DailyPublishLimit, PublishStartAt: m.PublishStartAt, PublishEndAt: m.PublishEndAt, PublishWindowStart: domain.PublishWindowClock(m.PublishStartAt), PublishWindowEnd: domain.PublishWindowClock(m.PublishEndAt), DedupeWindowHours: m.DedupeWindowHours, TargetLocales: m.TargetLocales, UpdatedBy: m.UpdatedBy, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}
func mapContentFetchRun(m *dbent.ContentFetchRun) service.ContentFetchRun {
	return service.ContentFetchRun{ID: m.ID, SourceID: m.SourceID, TriggerType: m.TriggerType, Status: m.Status, StartedAt: m.StartedAt, FinishedAt: m.FinishedAt, FetchedCount: m.FetchedCount, CreatedCount: m.CreatedCount, DuplicateCount: m.DuplicateCount, FailedCount: m.FailedCount, ErrorSummary: m.ErrorSummary, TaskKey: m.TaskKey, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}
func mapContentFetchRuns(items []*dbent.ContentFetchRun) []service.ContentFetchRun {
	out := make([]service.ContentFetchRun, len(items))
	for i, m := range items {
		out[i] = mapContentFetchRun(m)
	}
	return out
}

func setCreateNewsItemOptional(b *dbent.ContentNewsItemCreate, item *service.ContentNewsItem) {
	if item.Summary != nil {
		b.SetSummary(*item.Summary)
	}
	if item.BodyHTML != nil {
		b.SetBodyHTML(*item.BodyHTML)
	}
	if item.BodyText != nil {
		b.SetBodyText(*item.BodyText)
	}
	if item.BodySourceType != nil {
		b.SetBodySourceType(*item.BodySourceType)
	}
	if item.BodyFetchedAt != nil {
		b.SetBodyFetchedAt(*item.BodyFetchedAt)
	}
	if item.BodyHash != nil {
		b.SetBodyHash(*item.BodyHash)
	}
	if item.ImageURL != nil {
		b.SetImageURL(*item.ImageURL)
	}
	if item.SourceName != nil {
		b.SetSourceName(*item.SourceName)
	}
	if item.OriginalPublishedAt != nil {
		b.SetOriginalPublishedAt(*item.OriginalPublishedAt)
	}
	if item.PublishedAt != nil {
		b.SetPublishedAt(*item.PublishedAt)
	}
	if item.SeoTitle != nil {
		b.SetSeoTitle(*item.SeoTitle)
	}
	if item.SeoDescription != nil {
		b.SetSeoDescription(*item.SeoDescription)
	}
	if item.FetchedAt != nil {
		b.SetFetchedAt(*item.FetchedAt)
	}
	if item.ContentHash != nil {
		b.SetContentHash(*item.ContentHash)
	}
	if item.CreatedBy != nil {
		b.SetCreatedBy(*item.CreatedBy)
	}
	if item.UpdatedBy != nil {
		b.SetUpdatedBy(*item.UpdatedBy)
	}
}
func setOrClearString[T any](set func(string) T, clear func() T, v *string) {
	if v != nil {
		set(*v)
	} else {
		clear()
	}
}
func setOrClearTime[T any](set func(time.Time) T, clear func() T, v *time.Time) {
	if v != nil {
		set(*v)
	} else {
		clear()
	}
}
func setOrClearInt64[T any](set func(int64) T, clear func() T, v *int64) {
	if v != nil {
		set(*v)
	} else {
		clear()
	}
}
