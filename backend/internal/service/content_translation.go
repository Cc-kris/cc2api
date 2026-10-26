package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

var ErrContentTranslationState = errors.New("invalid content translation state transition")
var ErrContentTranslationModelUnavailable = errors.New("translation model is not configured")

// ContentTranslationStore is shared by article and news translations.
type ContentTranslationStore interface {
	UpsertDraft(context.Context, *ContentTranslation) (*ContentTranslation, error)
	Get(context.Context, int64) (*ContentTranslation, error)
	Save(context.Context, *ContentTranslation) (*ContentTranslation, error)
	PublishCurrent(context.Context, int64, int, time.Time) (*ContentTranslation, error)
	Unpublish(context.Context, int64, time.Time) (*ContentTranslation, error)
}
type ContentTranslationModelConfig interface {
	LoadAll() ([]ContentAIProviderConfig, string, error)
}
type ContentTranslationLifecycle struct {
	store  ContentTranslationStore
	tasks  ContentWritingTaskStore
	models ContentTranslationModelConfig
}

func NewContentTranslationLifecycle(s ContentTranslationStore, tasks ContentWritingTaskStore, models ContentTranslationModelConfig) *ContentTranslationLifecycle {
	return &ContentTranslationLifecycle{store: s, tasks: tasks, models: models}
}

func (s *ContentTranslationLifecycle) ResolveModel(requested string) (string, error) {
	if s == nil || s.models == nil {
		return "", ErrContentTranslationModelUnavailable
	}
	configs, defID, err := s.models.LoadAll()
	if err != nil {
		return "", err
	}
	var defaultModel string
	allowed := map[string]bool{}
	for _, c := range configs {
		for _, m := range c.Models {
			if strings.TrimSpace(m) != "" {
				allowed[m] = true
			}
		}
		if c.ID == defID {
			defaultModel = strings.TrimSpace(c.DefaultModel)
			if defaultModel != "" {
				allowed[defaultModel] = true
			}
		}
	}
	model := strings.TrimSpace(requested)
	if model == "" {
		model = defaultModel
	}
	if model == "" || !allowed[model] {
		return "", ErrContentTranslationModelUnavailable
	}
	return model, nil
}
func (s *ContentTranslationLifecycle) CreateDraft(ctx context.Context, t *ContentTranslation) (*ContentTranslation, error) {
	if s == nil || s.store == nil || t == nil || (t.ContentType != "article" && t.ContentType != "news") || t.ContentID <= 0 || t.SourceVersion <= 0 || strings.TrimSpace(t.SourceLocale) == "" || strings.TrimSpace(t.Locale) == "" || strings.EqualFold(t.SourceLocale, t.Locale) || strings.TrimSpace(t.Title) == "" || strings.TrimSpace(t.Slug) == "" {
		return nil, ErrContentArticleInvalid
	}
	model, err := s.ResolveModel(derefTranslation(t.TranslationModel))
	if err != nil {
		return nil, err
	}
	t.ContentType = strings.TrimSpace(t.ContentType)
	t.Status = domain.ContentTranslationDraft
	t.TranslationModel = &model
	t.FailureCode = nil
	return s.store.UpsertDraft(ctx, t)
}
func (s *ContentTranslationLifecycle) Fail(ctx context.Context, id int64, code string) error {
	t, e := s.store.Get(ctx, id)
	if e != nil {
		return e
	}
	if t.Status == domain.ContentTranslationPublished || t.Status == domain.ContentTranslationDeleted {
		return ErrContentTranslationState
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrContentArticleInvalid
	}
	t.Status = domain.ContentTranslationDraft
	t.FailureCode = &code
	_, e = s.store.Save(ctx, t)
	return e
}
func (s *ContentTranslationLifecycle) Retry(ctx context.Context, id int64) (*ContentTranslation, error) {
	t, e := s.store.Get(ctx, id)
	if e != nil {
		return nil, e
	}
	if t.FailureCode == nil || t.Status == domain.ContentTranslationPublished || t.Status == domain.ContentTranslationDeleted {
		return nil, ErrContentTranslationState
	}
	t.FailureCode = nil
	t.Status = domain.ContentTranslationDraft
	return s.store.Save(ctx, t)
}
func (s *ContentTranslationLifecycle) Publish(ctx context.Context, id int64, currentVersion int) (*ContentTranslation, error) {
	if s == nil || s.store == nil {
		return nil, ErrContentArticleInvalid
	}
	t, e := s.store.Get(ctx, id)
	if e != nil {
		return nil, e
	}
	if t.SourceVersion != currentVersion || (t.Status != domain.ContentTranslationDraft && t.Status != domain.ContentTranslationReview && t.Status != domain.ContentTranslationUnpublished) || t.FailureCode != nil || !hasNonEmptyTranslationBody(t) || t.Excerpt == nil || strings.TrimSpace(*t.Excerpt) == "" || t.SeoTitle == nil || strings.TrimSpace(*t.SeoTitle) == "" || t.SeoDescription == nil || strings.TrimSpace(*t.SeoDescription) == "" {
		return nil, ErrContentArticleNotPublishable
	}
	return s.store.PublishCurrent(ctx, id, currentVersion, time.Now().UTC())
}
func (s *ContentTranslationLifecycle) Unpublish(ctx context.Context, id int64) (*ContentTranslation, error) {
	if s == nil || s.store == nil {
		return nil, ErrContentArticleInvalid
	}
	return s.store.Unpublish(ctx, id, time.Now().UTC())
}
func (s *ContentTranslationLifecycle) Queue(ctx context.Context, t *ContentWritingTask, model string) (*ContentWritingTask, error) {
	if s == nil || s.tasks == nil || t == nil || t.ID == "" || t.ArticleID <= 0 || t.SourceVersion <= 0 || (t.ContentType != "article" && t.ContentType != "news") {
		return nil, ErrContentArticleInvalid
	}
	resolved, e := s.ResolveModel(model)
	if e != nil {
		return nil, e
	}
	t.Kind = ContentTranslationKind
	t.Model = resolved
	t.Status = ContentWritingQueued
	t.CreatedAt = time.Now().UTC()
	t.UpdatedAt = t.CreatedAt
	return s.tasks.Create(ctx, t)
}
func (s *ContentTranslationLifecycle) CancelTask(ctx context.Context, id string) error {
	t, e := s.tasks.Get(ctx, id)
	if e != nil {
		return e
	}
	if t.Kind != ContentTranslationKind || (t.Status != ContentWritingQueued && t.Status != ContentWritingRunning) {
		return ErrContentTranslationState
	}
	t.Status = ContentWritingCancelled
	t.UpdatedAt = time.Now().UTC()
	return s.tasks.Update(ctx, t)
}
func (s *ContentTranslationLifecycle) RetryTask(ctx context.Context, id string) error {
	t, e := s.tasks.Get(ctx, id)
	if e != nil {
		return e
	}
	if t.Kind != ContentTranslationKind || (t.Status != ContentWritingFailed && t.Status != ContentWritingCancelled) {
		return ErrContentTranslationState
	}
	t.Status = ContentWritingQueued
	t.ErrorCode = nil
	t.Result = nil
	t.UpdatedAt = time.Now().UTC()
	return s.tasks.Update(ctx, t)
}
func (s *ContentTranslationLifecycle) Restore(ctx context.Context, id int64) (*ContentTranslation, error) {
	t, e := s.store.Get(ctx, id)
	if e != nil {
		return nil, e
	}
	if t.Status != domain.ContentTranslationDeleted {
		return nil, fmt.Errorf("%w: translation is not deleted", ErrContentTranslationState)
	}
	t.Status = domain.ContentTranslationUnpublished
	t.DeletedAt = nil
	t.PublishedAt = nil
	return s.store.Save(ctx, t)
}
func derefTranslation(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
