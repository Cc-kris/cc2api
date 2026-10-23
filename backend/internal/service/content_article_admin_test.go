package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

type contentAuditLoggerStub struct {
	entry ContentAuditEntry
	err   error
}

func (s *contentAuditLoggerStub) Log(_ context.Context, e ContentAuditEntry) error {
	s.entry = e
	return s.err
}

func TestContentArticleAuditCarriesActorAndPropagatesError(t *testing.T) {
	r := &articleRepoStub{article: ContentArticle{Locale: "zh-CN", Slug: "a", Title: "A", Status: domain.ContentStatusDraft, Version: 1}}
	auditErr := errors.New("audit unavailable")
	logger := &contentAuditLoggerStub{err: auditErr}
	s := NewContentArticleAdminService(r, nil, nil, logger)
	ctx := WithContentAuditActor(context.Background(), 42)
	_, err := s.Create(ctx, &ContentArticle{Locale: "zh-CN", Slug: "a", Title: "A", Status: domain.ContentStatusDraft, Excerpt: stringPtr("e"), BodyMarkdown: stringPtr("body")})
	if !errors.Is(err, auditErr) {
		t.Fatalf("expected audit error, got %v", err)
	}
	if logger.entry.ActorID == nil || *logger.entry.ActorID != 42 {
		t.Fatalf("actor not recorded: %#v", logger.entry)
	}
}

type articleRepoStub struct {
	article  ContentArticle
	expected int
}

type contentWritingExecutorStub struct {
	result *ContentWritingResult
	err    error
}

func (s contentWritingExecutorStub) Generate(context.Context, *ContentWritingTask) (*ContentWritingResult, error) {
	return s.result, s.err
}

type contentTranslationExecutorStub struct {
	result *ContentTranslation
	err    error
}

func (s contentTranslationExecutorStub) Translate(context.Context, *ContentWritingTask, *ContentArticle) (*ContentTranslation, error) {
	return s.result, s.err
}

func (r *articleRepoStub) ListArticles(context.Context, ContentArticleFilters, int, int) ([]ContentArticle, int, error) {
	return []ContentArticle{r.article}, 1, nil
}
func (r *articleRepoStub) GetArticle(context.Context, int64, bool) (*ContentArticle, error) {
	a := r.article
	return &a, nil
}
func (r *articleRepoStub) CreateArticle(_ context.Context, a *ContentArticle) (*ContentArticle, error) {
	a.ID = 1
	a.Version = 1
	r.article = *a
	return a, nil
}
func (r *articleRepoStub) UpdateArticle(_ context.Context, a *ContentArticle, expected int) (*ContentArticle, error) {
	r.expected = expected
	if expected != r.article.Version {
		return nil, ErrContentArticleConflict
	}
	a.Version++
	r.article = *a
	return a, nil
}
func (r *articleRepoStub) SetArticleStatus(_ context.Context, id int64, status string, at *time.Time) (*ContentArticle, error) {
	r.article.Status = status
	r.article.PublishedAt = at
	return &r.article, nil
}
func (r *articleRepoStub) SoftDeleteArticle(context.Context, int64, time.Time) error { return nil }
func (r *articleRepoStub) RestoreArticle(context.Context, int64) (*ContentArticle, error) {
	return &r.article, nil
}
func (r *articleRepoStub) ListTranslations(context.Context, string, int64, bool) ([]ContentTranslation, error) {
	return nil, nil
}
func (r *articleRepoStub) GetTranslation(context.Context, int64, bool) (*ContentTranslation, error) {
	return nil, nil
}
func (r *articleRepoStub) CreateTranslation(_ context.Context, t *ContentTranslation) (*ContentTranslation, error) {
	return t, nil
}
func (r *articleRepoStub) UpdateTranslation(_ context.Context, t *ContentTranslation) (*ContentTranslation, error) {
	return t, nil
}
func (r *articleRepoStub) SoftDeleteTranslation(context.Context, int64, time.Time) error { return nil }

func TestContentArticleAdminPublishRequiresSEOAndBody(t *testing.T) {
	r := &articleRepoStub{article: ContentArticle{ID: 1, Locale: "zh-CN", Slug: "seo", Title: "SEO", Status: "draft", Version: 1}}
	s := NewContentArticleAdminService(r, nil, nil)
	_, err := s.Publish(context.Background(), 1, nil)
	require.ErrorIs(t, err, ErrContentArticleNotPublishable)
}

func TestContentArticleAdminUpdateUsesOptimisticVersion(t *testing.T) {
	r := &articleRepoStub{article: ContentArticle{ID: 1, Locale: "zh-CN", Slug: "draft", Title: "Draft", Status: "draft", Version: 3}}
	s := NewContentArticleAdminService(r, nil, nil)
	a := r.article
	_, err := s.Update(context.Background(), &a, 2)
	require.ErrorIs(t, err, ErrContentArticleConflict)
	_, err = s.Update(context.Background(), &a, 3)
	require.NoError(t, err)
	require.Equal(t, 3, r.expected)
}

func TestContentArticleAdminWritingTaskDoesNotStorePromptInModelKey(t *testing.T) {
	store := NewMemoryContentWritingTaskStore()
	s := NewContentArticleAdminService(nil, nil, store)
	task, err := s.CreateWritingTask(context.Background(), "  选题  ", "gpt-4o", "key-reference-only", 0)
	require.NoError(t, err)
	require.Equal(t, "选题", task.Topic)
	require.NotContains(t, task.ProviderKeyRef, "sk-")
	_, err = s.CreateWritingTask(context.Background(), "", "gpt-4o", "", 0)
	require.Error(t, err)
}

func TestContentArticleWritingWorkerPersistsDraftOnly(t *testing.T) {
	repo := &articleRepoStub{}
	store := NewMemoryContentWritingTaskStore()
	s := NewContentArticleAdminService(repo, nil, store)
	s.SetWritingExecutor(contentWritingExecutorStub{result: &ContentWritingResult{Title: "Generated", Excerpt: "Summary", BodyMarkdown: "# Body", SeoTitle: "SEO", SeoDescription: "Description"}})
	task, err := s.CreateWritingTaskWithInput(context.Background(), ContentWritingInput{Topic: "topic", Audience: "developers", Language: "en-US", ContentType: "guide", Keywords: []string{"api"}, WordCount: 800, Tone: "direct", Facts: []string{"fact"}, InternalLinks: []string{"/docs"}, Model: "current-model"})
	require.NoError(t, err)
	require.Equal(t, ContentWritingQueued, task.Status)
	require.NoError(t, s.ProcessNextWritingTask(context.Background()))
	got, err := s.GetWritingTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, ContentWritingSucceeded, got.Status)
	require.Contains(t, *got.Result, "article:")
	require.Equal(t, int64(1), repo.article.ID)
	require.Equal(t, domain.ContentStatusDraft, repo.article.Status)
	require.Equal(t, "# Body", *repo.article.BodyMarkdown)
}

func TestContentArticleWritingWorkerFailsSafeWithoutExecutor(t *testing.T) {
	store := NewMemoryContentWritingTaskStore()
	s := NewContentArticleAdminService(&articleRepoStub{}, nil, store)
	task, err := s.CreateWritingTask(context.Background(), "topic", "model", "key-ref", 0)
	require.NoError(t, err)
	require.NoError(t, s.ProcessNextWritingTask(context.Background()))
	got, err := s.GetWritingTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, ContentWritingFailed, got.Status)
	require.Equal(t, "content_ai_unavailable", *got.ErrorCode)
	got, err = s.CancelWritingTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, ContentWritingFailed, got.Status)
}

func TestContentArticleTranslationWorkerBindsSourceVersion(t *testing.T) {
	repo := &articleRepoStub{article: ContentArticle{ID: 7, Locale: "zh-CN", Slug: "source", Title: "Source", Status: domain.ContentStatusDraft, Version: 2, BodyMarkdown: stringPtr("body")}}
	store := NewMemoryContentWritingTaskStore()
	s := NewContentArticleAdminService(repo, nil, store)
	s.SetTranslationExecutor(contentTranslationExecutorStub{result: &ContentTranslation{Title: "Translated", BodyMarkdown: stringPtr("translated"), Status: domain.ContentTranslationDraft}})
	task, err := s.CreateTranslationTask(context.Background(), 7, "en-US", "model", "key-ref")
	require.NoError(t, err)
	require.Equal(t, 2, task.SourceVersion)
	require.NoError(t, s.ProcessNextWritingTask(context.Background()))
	got, err := s.GetWritingTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, ContentWritingSucceeded, got.Status)
	require.Contains(t, *got.Result, "translation:")
}
