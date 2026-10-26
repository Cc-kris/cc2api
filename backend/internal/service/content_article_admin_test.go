package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	articles []ContentArticle
	updates  map[int64]ContentArticle
	pages    []int
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
	return t, nil
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
				ArticleCategories: []string{"News"}, NewsCategories: []string{}, Tags: []string{}, FeaturedArticleIDs: []int64{7, 8, 7},
			})
			featuredAt := time.Now().UTC().Add(-time.Hour)
			r := &articleRepoStub{article: ContentArticle{
				ID: 7, Status: domain.ContentStatusPublished, Version: 1,
				Tags: []string{"API", "精选", " legacy ", "精选"}, FeaturedAt: &featuredAt,
			}}
			s := NewContentArticleAdminService(r, nil, nil)
			cfg, err := s.MutateTaxonomy(context.Background(), "featured_article", action, "7", "")
			require.NoError(t, err)
			require.Equal(t, []string{"API", " legacy "}, r.article.Tags)
			require.Equal(t, []int64{8}, cfg.FeaturedArticleIDs)
			if action == "set" {
				require.NotNil(t, r.article.FeaturedAt)
			} else {
				require.Nil(t, r.article.FeaturedAt)
			}
			persisted, err := os.ReadFile(contentTaxonomyPath)
			require.NoError(t, err)
			require.NotContains(t, string(persisted), "featuredArticleIds\": [\n    7")
			require.Equal(t, []int64{8}, loadContentTaxonomyConfig().FeaturedArticleIDs)
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
