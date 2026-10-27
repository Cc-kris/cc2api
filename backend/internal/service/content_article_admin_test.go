package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	article          ContentArticle
	news             *ContentNewsItem
	savedTranslation *ContentTranslation
	expected         int
	articles         []ContentArticle
	updates          map[int64]ContentArticle
	pages            []int
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
	source *ContentArticle
}

type contentSEOExecutorStub struct {
	results []*ContentSEOResult
	errs    []error
	models  []string
	inputs  []ContentSEOInput
	calls   int
}

func (s *contentSEOExecutorStub) GenerateSEO(ctx context.Context, model string, input ContentSEOInput) (*ContentSEOResult, error) {
	s.calls++
	s.models = append(s.models, model)
	s.inputs = append(s.inputs, input)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index := s.calls - 1
	if index < len(s.errs) && s.errs[index] != nil {
		return nil, s.errs[index]
	}
	if index < len(s.results) {
		return s.results[index], nil
	}
	return nil, ErrContentSEOResultInvalid
}

func (s *contentTranslationExecutorStub) Translate(_ context.Context, _ *ContentWritingTask, a *ContentArticle) (*ContentTranslation, error) {
	s.source = a
	return s.result, s.err
}

func (r *articleRepoStub) ListArticles(_ context.Context, _ ContentArticleFilters, page, size int) ([]ContentArticle, int, error) {
	if len(r.articles) == 0 {
		return []ContentArticle{r.article}, 1, nil
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = len(r.articles)
	}
	r.pages = append(r.pages, page)
	start := (page - 1) * size
	if start >= len(r.articles) {
		return []ContentArticle{}, len(r.articles), nil
	}
	end := start + size
	if end > len(r.articles) {
		end = len(r.articles)
	}
	items := append([]ContentArticle(nil), r.articles[start:end]...)
	return items, len(r.articles), nil
}
func (r *articleRepoStub) GetArticle(_ context.Context, id int64, _ bool) (*ContentArticle, error) {
	if len(r.articles) > 0 {
		for i := range r.articles {
			if r.articles[i].ID == id {
				a := r.articles[i]
				return &a, nil
			}
		}
	}
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
	if len(r.articles) > 0 {
		for i := range r.articles {
			if r.articles[i].ID != a.ID {
				continue
			}
			if expected != r.articles[i].Version {
				return nil, ErrContentArticleConflict
			}
			a.Version++
			r.articles[i] = *a
			if r.updates == nil {
				r.updates = make(map[int64]ContentArticle)
			}
			r.updates[a.ID] = *a
			return a, nil
		}
		return nil, ErrContentArticleNotFound
	}
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
	t.ID = 1
	r.savedTranslation = t
	return t, nil
}
func (r *articleRepoStub) GetNewsItem(_ context.Context, id int64, _ bool) (*ContentNewsItem, error) {
	if r.news == nil || r.news.ID != id {
		return nil, ErrContentNewsItemNotFound
	}
	n := *r.news
	return &n, nil
}
func (r *articleRepoStub) UpdateTranslation(_ context.Context, t *ContentTranslation) (*ContentTranslation, error) {
	return t, nil
}
func (r *articleRepoStub) SoftDeleteTranslation(context.Context, int64, time.Time) error { return nil }

func isolateContentTaxonomyForTest(t *testing.T, cfg ContentTaxonomyConfig) {
	t.Helper()
	contentTaxonomyMu.Lock()
	originalPath := contentTaxonomyPath
	contentTaxonomyPath = filepath.Join(t.TempDir(), "content_taxonomy.json")
	contentTaxonomyMu.Unlock()
	t.Cleanup(func() {
		contentTaxonomyMu.Lock()
		contentTaxonomyPath = originalPath
		contentTaxonomyMu.Unlock()
	})
	require.NoError(t, saveContentTaxonomyConfig(cfg))
}

func TestContentArticleTaxonomyFeaturedMutationReconcilesLegacyState(t *testing.T) {
	for _, action := range []string{"set", "unset"} {
		t.Run(action, func(t *testing.T) {
			isolateContentTaxonomyForTest(t, ContentTaxonomyConfig{
				ArticleCategories: []string{"News"}, NewsCategories: []string{}, Tags: []string{},
			})
			featuredAt := time.Now().UTC().Add(-time.Hour)
			r := &articleRepoStub{article: ContentArticle{
				ID: 7, Status: domain.ContentStatusPublished, Version: 1,
				Tags: []string{"API", "精选", " legacy ", "精选"}, FeaturedAt: &featuredAt,
			}}
			s := NewContentArticleAdminService(r, nil, nil)
			_, err := s.MutateTaxonomy(context.Background(), "featured_article", action, "7", "")
			require.NoError(t, err)
			require.Equal(t, []string{"API", " legacy "}, r.article.Tags)
			if action == "set" {
				require.NotNil(t, r.article.FeaturedAt)
			} else {
				require.Nil(t, r.article.FeaturedAt)
			}
			persisted, err := os.ReadFile(contentTaxonomyPath)
			require.NoError(t, err)
			require.NotContains(t, string(persisted), "featuredArticleIds")
			require.NotContains(t, string(persisted), "精选")
		})
	}
}

func TestContentArticleTaxonomyRenameAndDeleteTraverseAllPages(t *testing.T) {
	for _, tc := range []struct {
		scope, action string
		category      bool
	}{
		{scope: "article_category", action: "rename", category: true},
		{scope: "article_category", action: "delete", category: true},
		{scope: "tag", action: "rename"},
		{scope: "tag", action: "delete"},
	} {
		t.Run(tc.scope+"/"+tc.action, func(t *testing.T) {
			cfg := ContentTaxonomyConfig{ArticleCategories: []string{"Old", "Other"}, NewsCategories: []string{}, Tags: []string{"OldTag", "OtherTag"}}
			isolateContentTaxonomyForTest(t, cfg)
			articles := make([]ContentArticle, 205)
			for i := range articles {
				art := ContentArticle{ID: int64(i + 1), Version: 1, Status: domain.ContentStatusDraft}
				if tc.category {
					value := "Other"
					if i == 0 || i == len(articles)-1 {
						value = "Old"
					}
					art.Category = &value
				} else if i == 0 || i == len(articles)-1 {
					art.Tags = []string{"OldTag", "OtherTag", "OldTag"}
				} else {
					art.Tags = []string{"OtherTag"}
				}
				articles[i] = art
			}
			r := &articleRepoStub{articles: articles}
			s := NewContentArticleAdminService(r, nil, nil)
			newName := "New"
			if !tc.category {
				newName = "NewTag"
			}
			_, err := s.MutateTaxonomy(context.Background(), tc.scope, tc.action, func() string {
				if tc.category {
					return "Old"
				}
				return "OldTag"
			}(), newName)
			require.NoError(t, err)
			require.Equal(t, []int{1, 2}, r.pages, "must fetch every page before updating records")
			require.Len(t, r.updates, 2, "first and last page records should both be reconciled")
			for _, id := range []int64{1, 205} {
				updated := r.updates[id]
				if tc.category {
					want := "Other"
					if tc.action == "rename" {
						want = newName
					}
					require.Equal(t, want, *updated.Category)
				} else if tc.action == "rename" {
					require.Equal(t, []string{"NewTag", "OtherTag"}, updated.Tags)
				} else {
					require.Equal(t, []string{"OtherTag"}, updated.Tags)
				}
			}
		})
	}
}

func TestContentArticleTaxonomyRejectsInvalidFeaturedAction(t *testing.T) {
	r := &articleRepoStub{article: ContentArticle{ID: 1, Status: domain.ContentStatusPublished, Version: 1}}
	s := NewContentArticleAdminService(r, nil, nil)
	_, err := s.MutateTaxonomy(context.Background(), "featured_article", "delete", "1", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be set or unset")
	require.Equal(t, 0, r.expected, "invalid action must not update the article")
}

func TestContentArticleTaxonomyRejectsDraftFeaturedArticle(t *testing.T) {
	r := &articleRepoStub{article: ContentArticle{ID: 1, Status: domain.ContentStatusDraft, Version: 1}}
	s := NewContentArticleAdminService(r, nil, nil)
	_, err := s.MutateTaxonomy(context.Background(), "featured_article", "set", "1", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "only published")
	require.Nil(t, r.article.FeaturedAt)
	require.Equal(t, 0, r.expected, "draft must not update the article")
}

func TestContentArticleTaxonomyPersistsFeaturedAtOnSetAndUnset(t *testing.T) {
	isolateContentTaxonomyForTest(t, ContentTaxonomyConfig{ArticleCategories: []string{}, NewsCategories: []string{}, Tags: []string{}})
	r := &articleRepoStub{article: ContentArticle{ID: 1, Status: domain.ContentStatusPublished, Version: 1}}
	s := NewContentArticleAdminService(r, nil, nil)
	_, err := s.MutateTaxonomy(context.Background(), "featured_article", "set", "1", "")
	require.NoError(t, err)
	require.NotNil(t, r.article.FeaturedAt)
	require.False(t, r.article.IsFeatured, "repository state derives IsFeatured at read/decorate time")

	_, err = s.MutateTaxonomy(context.Background(), "featured_article", "unset", "1", "")
	require.NoError(t, err)
	require.Nil(t, r.article.FeaturedAt)
}

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
	s.SetTranslationExecutor(&contentTranslationExecutorStub{result: &ContentTranslation{Title: "Translated", BodyMarkdown: stringPtr("translated"), Status: domain.ContentTranslationDraft}})
	task, err := s.CreateTranslationTask(context.Background(), 7, "en-US", "model", "key-ref")
	require.NoError(t, err)
	require.Equal(t, 2, task.SourceVersion)
	require.NoError(t, s.ProcessNextWritingTask(context.Background()))
	got, err := s.GetWritingTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, ContentWritingSucceeded, got.Status)
	require.Contains(t, *got.Result, "translation:")
}

func TestContentArticleTranslationBatchDeduplicatesAndIsQueryable(t *testing.T) {
	repo := &articleRepoStub{article: ContentArticle{ID: 7, Locale: "zh-CN", Status: domain.ContentStatusDraft, Version: 4}}
	store := NewMemoryContentWritingTaskStore()
	s := NewContentArticleAdminService(repo, nil, store)

	tasks, err := s.CreateTranslationTasks(context.Background(), 7, []string{"en-US", "EN-us", "fr-FR"}, "model", "provider-ref")
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	require.Equal(t, "en-US", tasks[0].TargetLocale)
	require.Equal(t, "fr-FR", tasks[1].TargetLocale)
	for _, task := range tasks {
		require.Equal(t, ContentTranslationKind, task.Kind)
		require.Equal(t, 4, task.SourceVersion)
		require.Equal(t, ContentWritingQueued, task.Status)
		got, getErr := s.GetWritingTask(context.Background(), task.ID)
		require.NoError(t, getErr)
		require.Equal(t, task.TargetLocale, got.TargetLocale)
		require.Equal(t, ContentWritingQueued, got.Status)
	}

	listed, err := s.ListWritingTasks(context.Background(), ContentTranslationKind, 10)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	_, err = s.CreateTranslationTasks(context.Background(), 7, nil, "model", "provider-ref")
	require.ErrorIs(t, err, ErrContentArticleInvalid)
	listed, err = s.ListWritingTasks(context.Background(), ContentTranslationKind, 10)
	require.NoError(t, err)
	require.Len(t, listed, 2)
}

func TestAIConfigRevisionRejectsStaleToken(t *testing.T) {
	old := contentAIConfigPath
	contentAIConfigPath = filepath.Join(t.TempDir(), "ai.json")
	defer func() { contentAIConfigPath = old }()
	require.ErrorContains(t, (&ContentArticleAdminService{}).MutateAIProviderConfig(context.Background(), "default", "missing", "stale", ContentAIProviderConfig{}), "revision conflict")
}

func TestContentSEOValidationAndSingleBoundaryRetry(t *testing.T) {
	oldPath := contentAIConfigPath
	contentAIConfigPath = filepath.Join(t.TempDir(), "ai.json")
	defer func() { contentAIConfigPath = oldPath }()
	require.NoError(t, saveContentAIProviderConfig(ContentAIProviderConfig{ID: "default", Label: "Default", Provider: "custom", IsDefault: true, BaseURL: "https://example.test/v1", Models: []string{"seo-model"}, DefaultModel: "seo-model"}))
	invalid := &ContentSEOResult{Description: string(make([]byte, 0)), Keywords: []string{"one"}}
	invalid.Description = strings.Repeat("x", 119)
	valid := &ContentSEOResult{Description: strings.Repeat("描述", 65), Keywords: []string{" one ", "two", "three", "four", "five"}}
	exec := &contentSEOExecutorStub{results: []*ContentSEOResult{invalid, valid}}
	svc := NewContentArticleAdminService(nil, nil, nil)
	svc.SetSEOExecutor(exec)
	got, err := svc.GenerateSEO(context.Background(), ContentSEOInput{Title: "title", BodyMarkdown: "body"})
	require.NoError(t, err)
	require.Equal(t, 2, exec.calls)
	require.Equal(t, "seo-model", exec.models[0])
	require.Len(t, []rune(got.Description), 130)
	require.Equal(t, []string{"one", "two", "three", "four", "five"}, got.Keywords)
}

func TestContentSEORejectsMissingDefaultModelAndCancellation(t *testing.T) {
	oldPath := contentAIConfigPath
	contentAIConfigPath = filepath.Join(t.TempDir(), "ai.json")
	defer func() { contentAIConfigPath = oldPath }()
	require.NoError(t, NewContentAIConfigStore(contentAIConfigPath, nil).SaveAll(nil, ""))
	exec := &contentSEOExecutorStub{}
	svc := NewContentArticleAdminService(nil, nil, nil)
	svc.SetSEOExecutor(exec)
	_, err := svc.GenerateSEO(context.Background(), ContentSEOInput{BodyMarkdown: "body"})
	require.ErrorIs(t, err, ErrContentAIModelNotConfigured)
	require.Zero(t, exec.calls)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = svc.GenerateSEO(ctx, ContentSEOInput{BodyMarkdown: "body"})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, exec.calls)
}

func TestContentNewsTranslationExecutionAndSourceSnapshot(t *testing.T) {
	ctx := context.Background()
	body, cover := "original news", "https://img.example/news.jpg"
	news := &ContentNewsItem{ID: 42, Title: "Headline", BodyText: &body, Slug: "headline", Locale: "en", ImageURL: &cover}
	repo := &articleRepoStub{news: news}
	svc := NewContentArticleAdminService(repo, nil, nil)
	svc.defaultModel = "model-a"
	translatedBody := "translated news"
	executor := &contentTranslationExecutorStub{result: &ContentTranslation{Locale: "ru", Slug: "headline", Title: "Заголовок", BodyMarkdown: &translatedBody, Status: domain.ContentTranslationDraft}}
	svc.SetTranslationExecutor(executor)
	task := &ContentWritingTask{ID: ContentNewsTranslationTaskID(news, "ru"), Kind: ContentTranslationKind, ContentType: "news", SourceVersion: ContentNewsSourceVersion(news), TargetLocale: "ru"}
	store := NewMemoryContentWritingTaskStore()
	_, err := store.Create(ctx, task)
	require.NoError(t, err)
	require.NoError(t, svc.executeTranslationTask(ctx, store, task))
	persisted, err := store.Get(ctx, task.ID)
	require.NoError(t, err)
	t.Logf("news translation error=%q result=%+v", contentNewsStringValue(persisted.ErrorCode), persisted.Result)
	require.Equal(t, ContentWritingSucceeded, persisted.Status)
	require.NotNil(t, repo.savedTranslation)
	require.Equal(t, "news", repo.savedTranslation.ContentType)
	require.Equal(t, int64(42), repo.savedTranslation.ContentID)
	require.Equal(t, task.SourceVersion, repo.savedTranslation.SourceVersion)
	require.NotNil(t, executor.source)
	require.Equal(t, cover, contentNewsStringValue(executor.source.CoverImage))

	news.Title = "Edited after enqueue"
	stale := &ContentWritingTask{ID: task.ID, Kind: ContentTranslationKind, ContentType: "news", SourceVersion: task.SourceVersion, TargetLocale: "ru", Model: "model-a"}
	require.NoError(t, svc.executeTranslationTask(ctx, store, stale))
	persistedStale, err := store.Get(ctx, stale.ID)
	require.NoError(t, err)
	require.Equal(t, ContentWritingFailed, persistedStale.Status)
	require.Equal(t, "content_source_version_conflict", contentNewsStringValue(persistedStale.ErrorCode))
}

func TestContentNewsTranslationRequiresDefaultModel(t *testing.T) {
	news := &ContentNewsItem{ID: 7, Title: "Headline"}
	repo := &articleRepoStub{news: news}
	svc := NewContentArticleAdminService(repo, nil, nil)
	svc.defaultModel = ""
	svc.SetTranslationExecutor(&contentTranslationExecutorStub{})
	task := &ContentWritingTask{ID: ContentNewsTranslationTaskID(news, "fr"), Kind: ContentTranslationKind, ContentType: "news", SourceVersion: ContentNewsSourceVersion(news)}
	failStore := NewMemoryContentWritingTaskStore()
	_, err := failStore.Create(context.Background(), task)
	require.NoError(t, err)
	require.NoError(t, svc.executeTranslationTask(context.Background(), failStore, task))
	persistedFailure, err := failStore.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, ContentWritingFailed, persistedFailure.Status)
	require.Equal(t, "content_ai_model_not_configured", contentNewsStringValue(persistedFailure.ErrorCode))
}
