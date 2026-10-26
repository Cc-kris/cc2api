package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	service "github.com/Wei-Shaw/sub2api/internal/service"
	_ "modernc.org/sqlite"
)

type translationModelStub struct {
	configs []service.ContentAIProviderConfig
	def     string
}

func (m translationModelStub) LoadAll() ([]service.ContentAIProviderConfig, string, error) {
	return m.configs, m.def, nil
}

type translationTaskStub struct {
	tasks map[string]*service.ContentWritingTask
}

func (s *translationTaskStub) Create(_ context.Context, t *service.ContentWritingTask) (*service.ContentWritingTask, error) {
	if s.tasks == nil {
		s.tasks = map[string]*service.ContentWritingTask{}
	}
	v := *t
	s.tasks[t.ID] = &v
	return &v, nil
}
func (s *translationTaskStub) Get(_ context.Context, id string) (*service.ContentWritingTask, error) {
	t := s.tasks[id]
	if t == nil {
		return nil, service.ErrContentWritingTaskNotFound
	}
	v := *t
	return &v, nil
}
func (s *translationTaskStub) Update(_ context.Context, t *service.ContentWritingTask) error {
	if s.tasks[t.ID] == nil {
		return service.ErrContentWritingTaskNotFound
	}
	v := *t
	s.tasks[t.ID] = &v
	return nil
}
func (s *translationTaskStub) List(context.Context, string, int) ([]*service.ContentWritingTask, error) {
	return nil, nil
}
func (s *translationTaskStub) Delete(_ context.Context, id string) error {
	delete(s.tasks, id)
	return nil
}

type translationStoreStub struct {
	rows map[int64]*service.ContentTranslation
	next int64
}

func (s *translationStoreStub) UpsertDraft(_ context.Context, t *service.ContentTranslation) (*service.ContentTranslation, error) {
	if s.rows == nil {
		s.rows = map[int64]*service.ContentTranslation{}
	}
	for _, r := range s.rows {
		if r.ContentType == t.ContentType && r.ContentID == t.ContentID && r.Locale == t.Locale && r.SourceVersion == t.SourceVersion {
			r.Status = t.Status
			r.FailureCode = nil
			return r, nil
		}
	}
	v := *t
	s.next++
	v.ID = s.next
	s.rows[v.ID] = &v
	return &v, nil
}
func (s *translationStoreStub) Get(_ context.Context, id int64) (*service.ContentTranslation, error) {
	if s.rows[id] == nil {
		return nil, service.ErrContentTranslationNotFound
	}
	v := *s.rows[id]
	return &v, nil
}
func (s *translationStoreStub) Save(_ context.Context, t *service.ContentTranslation) (*service.ContentTranslation, error) {
	v := *t
	s.rows[t.ID] = &v
	return &v, nil
}
func (s *translationStoreStub) PublishCurrent(_ context.Context, id int64, version int, at time.Time) (*service.ContentTranslation, error) {
	v := s.rows[id]
	if v == nil || v.SourceVersion != version {
		return nil, service.ErrContentTranslationNotFound
	}
	for _, r := range s.rows {
		if r.ContentType == v.ContentType && r.ContentID == v.ContentID && r.Locale == v.Locale && r.ID != id && r.Status == domain.ContentTranslationPublished {
			r.Status = domain.ContentTranslationUnpublished
			r.PublishedAt = nil
		}
	}
	v.Status = domain.ContentTranslationPublished
	v.PublishedAt = &at
	return v, nil
}
func (s *translationStoreStub) Unpublish(_ context.Context, id int64, at time.Time) (*service.ContentTranslation, error) {
	v := s.rows[id]
	if v == nil {
		return nil, service.ErrContentTranslationNotFound
	}
	v.Status = domain.ContentTranslationUnpublished
	v.PublishedAt = nil
	return v, nil
}

type lifecycleTestHarness struct {
	*service.ContentTranslationLifecycle
	store *translationStoreStub
	tasks *translationTaskStub
}

func testLifecycle() *lifecycleTestHarness {
	m := translationModelStub{configs: []service.ContentAIProviderConfig{{ID: "default", DefaultModel: "model-a", Models: []string{"model-a", "model-b"}}}, def: "default"}
	store, tasks := &translationStoreStub{}, &translationTaskStub{}
	return &lifecycleTestHarness{ContentTranslationLifecycle: service.NewContentTranslationLifecycle(store, tasks, m), store: store, tasks: tasks}
}
func translationDraft(kind string, version int) *service.ContentTranslation {
	excerpt, body, title, desc := "excerpt", "body", "seo", "description"
	return &service.ContentTranslation{ContentType: kind, ContentID: 8, SourceVersion: version, SourceLocale: "zh-CN", Locale: "en-US", Slug: "hello", Title: "Hello", Excerpt: &excerpt, BodyMarkdown: &body, SeoTitle: &title, SeoDescription: &desc}
}
func TestContentTranslationSharedVersionLifecycle(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"article", "news"} {
		t.Run(kind, func(t *testing.T) {
			s := testLifecycle()
			one, e := s.CreateDraft(ctx, translationDraft(kind, 1))
			if e != nil {
				t.Fatal(e)
			}
			if one.TranslationModel == nil || *one.TranslationModel != "model-a" {
				t.Fatalf("default model=%v", one.TranslationModel)
			}
			if _, e = s.Publish(ctx, one.ID, 2); !errors.Is(e, service.ErrContentArticleNotPublishable) {
				t.Fatalf("stale source publish: %v", e)
			}
			if _, e = s.Publish(ctx, one.ID, 1); e != nil {
				t.Fatal(e)
			}
			two, e := s.CreateDraft(ctx, translationDraft(kind, 2))
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.Publish(ctx, two.ID, 2); e != nil {
				t.Fatal(e)
			}
			old, _ := s.store.Get(ctx, one.ID)
			if old.Status != domain.ContentTranslationUnpublished {
				t.Fatalf("old version status=%s", old.Status)
			}
			if _, e = s.Unpublish(ctx, two.ID); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestContentTranslationFailureRetryAndTaskCancelRestore(t *testing.T) {
	ctx := context.Background()
	s := testLifecycle()
	v, e := s.CreateDraft(ctx, translationDraft("news", 1))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Fail(ctx, v.ID, "provider_timeout"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Retry(ctx, v.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Queue(ctx, &service.ContentWritingTask{ID: "task-1", ContentType: "news", ArticleID: 8, SourceVersion: 1}, ""); e != nil {
		t.Fatal(e)
	}
	if e = s.CancelTask(ctx, "task-1"); e != nil {
		t.Fatal(e)
	}
	if e = s.RetryTask(ctx, "task-1"); e != nil {
		t.Fatal(e)
	}
	q, _ := s.tasks.Get(ctx, "task-1")
	if q.Status != service.ContentWritingQueued || q.Model != "model-a" {
		t.Fatalf("recovered task=%+v", q)
	}
	v.Status = domain.ContentTranslationDeleted
	if _, e = s.store.Save(ctx, v); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Restore(ctx, v.ID); e != nil {
		t.Fatal(e)
	}
}
func TestContentTranslationModelSelectionAndInvalidTransitions(t *testing.T) {
	s := testLifecycle()
	m, e := s.ResolveModel("model-b")
	if e != nil || m != "model-b" {
		t.Fatalf("manual model: %q %v", m, e)
	}
	if _, e = s.ResolveModel("unknown"); !errors.Is(e, service.ErrContentTranslationModelUnavailable) {
		t.Fatalf("unconfigured model accepted: %v", e)
	}
	if _, e = s.Publish(context.Background(), 99, 1); !errors.Is(e, service.ErrContentTranslationNotFound) {
		t.Fatalf("missing translation: %v", e)
	}
}

func newContentTranslationDB(t *testing.T) *dbent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:content_translation_retry_regression?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatal(err)
	}
	return client
}

func TestContentTranslationRetryAndDraftUpsertClearFailureCodeInDatabase(t *testing.T) {
	ctx := context.Background()
	store := repository.NewContentTranslationRepository(newContentTranslationDB(t))
	models := translationModelStub{configs: []service.ContentAIProviderConfig{{ID: "default", DefaultModel: "model-a", Models: []string{"model-a"}}}, def: "default"}
	lifecycle := service.NewContentTranslationLifecycle(store, &translationTaskStub{}, models)

	created, err := lifecycle.CreateDraft(ctx, translationDraft("news", 1))
	if err != nil {
		t.Fatal(err)
	}
	if err = lifecycle.Fail(ctx, created.ID, "provider_timeout"); err != nil {
		t.Fatal(err)
	}
	if _, err = lifecycle.Retry(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	persisted, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.FailureCode != nil {
		t.Fatalf("Retry left failure_code in database: %q", *persisted.FailureCode)
	}

	if err = lifecycle.Fail(ctx, created.ID, "provider_timeout"); err != nil {
		t.Fatal(err)
	}
	if _, err = lifecycle.CreateDraft(ctx, translationDraft("news", 1)); err != nil {
		t.Fatal(err)
	}
	persisted, err = store.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.FailureCode != nil {
		t.Fatalf("CreateDraft upsert left failure_code in database: %q", *persisted.FailureCode)
	}
}
