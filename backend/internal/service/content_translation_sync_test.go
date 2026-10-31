package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// syncStoreStub 是 ContentTranslationDraftStore 的内存实现，
// 用来验证「保存后按 syncLocales 建待翻译记录、重复保存不重复建」。
type syncStoreStub struct {
	rows    []*service.ContentTranslation
	nextID  int64
	upserts int
}

func (s *syncStoreStub) UpsertDraft(_ context.Context, draft *service.ContentTranslation) (*service.ContentTranslation, error) {
	s.upserts++
	for _, row := range s.rows {
		if row.ContentType == draft.ContentType && row.ContentID == draft.ContentID &&
			strings.EqualFold(row.Locale, draft.Locale) && row.SourceVersion == draft.SourceVersion {
			return row, nil
		}
	}
	s.nextID++
	clone := *draft
	clone.ID = s.nextID
	s.rows = append(s.rows, &clone)
	return &clone, nil
}

func (s *syncStoreStub) ListUnfinished(_ context.Context, contentType string, contentID int64) ([]service.ContentTranslation, error) {
	out := make([]service.ContentTranslation, 0, len(s.rows))
	for _, row := range s.rows {
		if row.ContentType == contentType && row.ContentID == contentID &&
			row.Status != domain.ContentTranslationPublished && row.Status != domain.ContentTranslationDeleted {
			out = append(out, *row)
		}
	}
	return out, nil
}

func TestContentTranslationSyncerCreatesPendingDraftsPerLocale(t *testing.T) {
	store := &syncStoreStub{}
	syncer := service.NewContentTranslationSyncer(store)

	first, errs := syncer.SyncTranslationDrafts(context.Background(), service.TranslationDraftRequest{
		ContentType:   "news",
		ContentID:     7,
		SourceVersion: 1,
		SourceLocale:  "zh-CN",
		Slug:          "hello",
		Title:         "你好",
		Locales:       []string{"en-US", "ru-RU", "fr-FR", "de-DE"},
	})
	if first == nil {
		t.Fatalf("expected first draft, got nil (errs=%v)", errs)
	}
	for _, err := range errs {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if len(store.rows) != 4 {
		t.Fatalf("expected 4 drafts, got %d", len(store.rows))
	}
	for _, row := range store.rows {
		if row.Status != domain.ContentTranslationDraft {
			t.Errorf("draft %s status = %q, want draft", row.Locale, row.Status)
		}
		if row.Slug == "" {
			t.Errorf("draft %s slug is empty", row.Locale)
		}
	}
}

func TestContentTranslationSyncerDoesNotDuplicateOnResave(t *testing.T) {
	store := &syncStoreStub{}
	syncer := service.NewContentTranslationSyncer(store)
	req := service.TranslationDraftRequest{
		ContentType: "article", ContentID: 9, SourceVersion: 1, SourceLocale: "zh-CN",
		Slug: "post", Title: "标题", Locales: []string{"en-US", "fr-FR"},
	}
	if _, errs := syncer.SyncTranslationDrafts(context.Background(), req); hasError(errs) {
		t.Fatalf("first sync errored: %v", errs)
	}
	req.SourceVersion = 2 // 同一内容再次保存，版本前进也不应新建同一 locale 的草稿
	if _, errs := syncer.SyncTranslationDrafts(context.Background(), req); hasError(errs) {
		t.Fatalf("second sync errored: %v", errs)
	}
	if len(store.rows) != 2 {
		t.Fatalf("expected 2 drafts after resave, got %d", len(store.rows))
	}
}

func TestContentTranslationSyncerSkipsSourceLocaleAndDedupesInput(t *testing.T) {
	store := &syncStoreStub{}
	syncer := service.NewContentTranslationSyncer(store)
	_, errs := syncer.SyncTranslationDrafts(context.Background(), service.TranslationDraftRequest{
		ContentType: "news", ContentID: 11, SourceVersion: 1, SourceLocale: "zh-CN",
		Slug: "s", Title: "t", Locales: []string{"zh-CN", "en-US", "en-us", "EN-US"},
	})
	if hasError(errs) {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(store.rows) != 1 {
		t.Fatalf("expected only the en-US draft, got %d rows", len(store.rows))
	}
	if !strings.EqualFold(store.rows[0].Locale, "en-US") {
		t.Fatalf("unexpected locale %q", store.rows[0].Locale)
	}
}

func TestContentTranslationSyncerRejectsUnsupportedTargetLocale(t *testing.T) {
	store := &syncStoreStub{}
	syncer := service.NewContentTranslationSyncer(store)
	_, errs := syncer.SyncTranslationDrafts(context.Background(), service.TranslationDraftRequest{
		ContentType: "article", ContentID: 13, SourceVersion: 1, SourceLocale: "zh-CN",
		Slug: "s", Title: "t", Locales: []string{"en-US", "ko-KR"},
	})
	if len(errs) != 2 {
		t.Fatalf("expected 2 error slots, got %d", len(errs))
	}
	if errs[0] != nil {
		t.Errorf("en-US should succeed, got %v", errs[0])
	}
	if !errors.Is(errs[1], service.ErrContentTranslationLocaleUnsupported) {
		t.Fatalf("ko-KR should be rejected, got %v", errs[1])
	}
	if len(store.rows) != 1 {
		t.Fatalf("expected only en-US draft persisted, got %d", len(store.rows))
	}
}

func TestContentTranslationSyncerNilStoreIsNoop(t *testing.T) {
	syncer := service.NewContentTranslationSyncer(nil)
	row, errs := syncer.SyncTranslationDrafts(context.Background(), service.TranslationDraftRequest{
		ContentType: "news", ContentID: 1, SourceLocale: "zh-CN", Locales: []string{"en-US"},
	})
	if row != nil || errs != nil {
		t.Fatalf("nil store should be a no-op, got row=%v errs=%v", row, errs)
	}
	var nilSyncer *service.ContentTranslationSyncer
	if _, errs := nilSyncer.SyncTranslationDrafts(context.Background(), service.TranslationDraftRequest{ContentID: 1}); errs != nil {
		t.Fatalf("nil syncer should be a no-op, got %v", errs)
	}
}

func hasError(errs []error) bool {
	for _, err := range errs {
		if err != nil {
			return true
		}
	}
	return false
}
