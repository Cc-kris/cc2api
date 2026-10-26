package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type contentNewsAdminRepoStub struct {
	source             *ContentNewsSource
	item               *ContentNewsItem
	setting            *ContentAutomationSetting
	run                *ContentFetchRun
	sourceCalls        int
	itemCalls          int
	itemErr            error
	lastIncludeDeleted bool
}

type contentNewsAuditStub struct{ entries []ContentAuditEntry }

func (s *contentNewsAuditStub) Log(_ context.Context, entry ContentAuditEntry) error {
	s.entries = append(s.entries, entry)
	return nil
}

func (s *contentNewsAdminRepoStub) ListNewsSources(context.Context, ContentNewsSourceFilters, int, int, bool) ([]ContentNewsSource, int, error) {
	return nil, 0, nil
}
func (s *contentNewsAdminRepoStub) GetNewsSource(context.Context, int64, bool) (*ContentNewsSource, error) {
	s.sourceCalls++
	return s.source, nil
}
func (s *contentNewsAdminRepoStub) CreateNewsSource(_ context.Context, v *ContentNewsSource) (*ContentNewsSource, error) {
	return v, nil
}
func (s *contentNewsAdminRepoStub) UpdateNewsSource(_ context.Context, v *ContentNewsSource) (*ContentNewsSource, error) {
	return v, nil
}
func (s *contentNewsAdminRepoStub) SoftDeleteNewsSource(context.Context, int64, time.Time) error {
	return nil
}
func (s *contentNewsAdminRepoStub) RestoreNewsSource(context.Context, int64) (*ContentNewsSource, error) {
	return s.source, nil
}
func (s *contentNewsAdminRepoStub) ListNewsItems(context.Context, ContentNewsListFilters, int, int, bool) ([]ContentNewsItem, int, error) {
	return nil, 0, nil
}
func (s *contentNewsAdminRepoStub) GetNewsItem(_ context.Context, _ int64, includeDeleted bool) (*ContentNewsItem, error) {
	s.itemCalls++
	s.lastIncludeDeleted = includeDeleted
	if s.itemErr != nil {
		return nil, s.itemErr
	}
	return s.item, nil
}
func (s *contentNewsAdminRepoStub) CreateNewsItem(_ context.Context, v *ContentNewsItem) (*ContentNewsItem, error) {
	return v, nil
}
func (s *contentNewsAdminRepoStub) UpdateNewsItem(_ context.Context, v *ContentNewsItem) (*ContentNewsItem, error) {
	return v, nil
}
func (s *contentNewsAdminRepoStub) SoftDeleteNewsItem(context.Context, int64, time.Time) error {
	return nil
}
func (s *contentNewsAdminRepoStub) RestoreNewsItem(context.Context, int64) (*ContentNewsItem, error) {
	return s.item, nil
}
func (s *contentNewsAdminRepoStub) GetAutomationSetting(context.Context) (*ContentAutomationSetting, error) {
	if s.setting == nil {
		return nil, ErrContentAutomationNotFound
	}
	return s.setting, nil
}
func (s *contentNewsAdminRepoStub) UpsertAutomationSetting(_ context.Context, v *ContentAutomationSetting) (*ContentAutomationSetting, error) {
	s.setting = v
	return v, nil
}
func (s *contentNewsAdminRepoStub) ListFetchRuns(context.Context, ContentFetchRunFilters, int, int) ([]ContentFetchRun, int, error) {
	return nil, 0, nil
}
func (s *contentNewsAdminRepoStub) GetFetchRun(context.Context, int64) (*ContentFetchRun, error) {
	return s.run, nil
}
func (s *contentNewsAdminRepoStub) CreateFetchRun(_ context.Context, v *ContentFetchRun) (*ContentFetchRun, error) {
	s.run = v
	return v, nil
}
func (s *contentNewsAdminRepoStub) UpdateFetchRun(_ context.Context, v *ContentFetchRun) (*ContentFetchRun, error) {
	s.run = v
	return v, nil
}

func TestContentNewsAdminServicePublishRequiresCleanBody(t *testing.T) {
	repo := &contentNewsAdminRepoStub{item: &ContentNewsItem{ID: 9, Status: domain.ContentStatusDraft}}
	svc := NewContentNewsAdminService(repo)
	_, err := svc.PublishNewsItem(context.Background(), 9, nil)
	if !errors.Is(err, ErrContentNewsNotPublishable) {
		t.Fatalf("expected publish gate, got %v", err)
	}
	if repo.itemCalls != 1 {
		t.Fatalf("expected one item lookup, got %d", repo.itemCalls)
	}
}

func TestContentNewsAdminServiceRequiresAuditBeforeSnapshot(t *testing.T) {
	lookupErr := errors.New("snapshot unavailable")
	repo := &contentNewsAdminRepoStub{item: &ContentNewsItem{ID: 9, Status: domain.ContentStatusDraft}, itemErr: lookupErr}
	svc := NewContentNewsAdminService(repo)
	item := &ContentNewsItem{ID: 9, SourceID: 1, Status: domain.ContentStatusDraft, PublishMode: domain.ContentNewsPublishManual, Title: "Title", Slug: "title", Locale: "zh-CN", Summary: stringPtr("Summary"), BodyHTML: stringPtr("<p>Body</p>"), SourceName: stringPtr("Source")}
	_, err := svc.UpdateNewsItem(context.Background(), item)
	if !errors.Is(err, lookupErr) {
		t.Fatalf("expected snapshot lookup error, got %v", err)
	}
	if err := svc.DeleteNewsItem(context.Background(), 9); !errors.Is(err, lookupErr) {
		t.Fatalf("expected delete snapshot lookup error, got %v", err)
	}
	if repo.itemCalls != 2 {
		t.Fatalf("expected two guarded lookups, got %d", repo.itemCalls)
	}
}

func TestContentNewsAdminServiceRestoreReadsDeletedBeforeSnapshot(t *testing.T) {
	repo := &contentNewsAdminRepoStub{item: &ContentNewsItem{ID: 9, Status: domain.ContentStatusDeleted}}
	svc := NewContentNewsAdminService(repo)
	_, err := svc.RestoreNewsItem(context.Background(), 9)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if !repo.lastIncludeDeleted {
		t.Fatal("restore must read the deleted item before changing it")
	}
}

func TestContentNewsAdminServiceAuditCapturesNewsFields(t *testing.T) {
	item := &ContentNewsItem{ID: 9, SourceID: 2, Status: domain.ContentStatusDraft, PublishMode: domain.ContentNewsPublishManual,
		Slug: "ccai-update", Locale: "zh-CN", Title: "CCAI Update", SourceCanonicalURL: "https://example.test/news",
		Summary: stringPtr("summary"), BodyHTML: stringPtr("<p>full</p>"), BodyText: stringPtr("full"), SourceName: stringPtr("CCAI")}
	audit := &contentNewsAuditStub{}
	svc := NewContentNewsAdminService(&contentNewsAdminRepoStub{item: item}, audit)
	if _, err := svc.PublishNewsItem(context.Background(), item.ID, nil); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if len(audit.entries) != 1 {
		t.Fatalf("audit entries=%d", len(audit.entries))
	}
	before := audit.entries[0].Before
	if before["status"] != domain.ContentStatusDraft || before["body_html"] != item.BodyHTML || before["source_canonical_url"] != item.SourceCanonicalURL {
		t.Fatalf("incomplete before snapshot: %#v", before)
	}
	if audit.entries[0].After["status"] != domain.ContentStatusPublished {
		t.Fatalf("after status missing: %#v", audit.entries[0].After)
	}
}

func TestContentNewsAdminServiceValidatesSourceEnablement(t *testing.T) {
	repo := &contentNewsAdminRepoStub{source: &ContentNewsSource{ID: 1, Name: "Feed", SourceType: domain.ContentNewsSourceRSS, IntervalMinutes: 15, RateLimitPerMinute: 60}}
	svc := NewContentNewsAdminService(repo)
	if _, err := svc.SetNewsSourceEnabled(context.Background(), 1, true); !errors.Is(err, ErrContentNewsInvalidInput) {
		t.Fatalf("expected enablement validation, got %v", err)
	}
}

func TestContentNewsAdminServiceAutomationCronAndDefaults(t *testing.T) {
	repo := &contentNewsAdminRepoStub{}
	svc := NewContentNewsAdminService(repo)
	got, err := svc.GetAutomationSetting(context.Background())
	if err != nil || got.FetchCron == "" || got.DailyFetchLimit != 0 {
		t.Fatalf("unexpected defaults: %#v %v", got, err)
	}
	bad := &ContentAutomationSetting{Timezone: "Asia/Shanghai", FetchCron: "invalid", PublishMode: domain.ContentAutomationManualReview, DedupeWindowHours: 24, TargetLocales: []string{"zh-CN"}}
	if _, err := svc.UpsertAutomationSetting(context.Background(), bad); !errors.Is(err, ErrContentNewsInvalidCron) && !errors.Is(errors.Unwrap(err), ErrContentNewsInvalidCron) {
		t.Fatalf("expected invalid cron, got %v", err)
	}
}

func TestContentNewsAdminServiceNewsSourceIntervalInheritance(t *testing.T) {
	ctx := context.Background()
	globalMinutes, globalLabel := contentScheduleMinutes("0 */30 * * * *")
	if globalMinutes != 30 || globalLabel != "每 30 分钟" {
		t.Fatalf("unexpected global schedule: %d %q", globalMinutes, globalLabel)
	}

	inherited := &ContentNewsSource{IntervalMinutesOverride: nil}
	applyNewsSchedule(inherited, globalMinutes, globalLabel, "0 */30 * * * *")
	if inherited.IntervalMinutes != 30 || inherited.ScheduleMinutes != 30 || inherited.ScheduleLabel != "跟随全局（30 分钟）" {
		t.Fatalf("NULL interval did not inherit global schedule: %#v", inherited)
	}

	explicitValue := 1440
	explicit := &ContentNewsSource{IntervalMinutesOverride: &explicitValue}
	applyNewsSchedule(explicit, globalMinutes, globalLabel, "0 */30 * * * *")
	if explicit.IntervalMinutes != 1440 || explicit.ScheduleLabel != "每 1440 分钟" {
		t.Fatalf("explicit interval was overwritten: %#v", explicit)
	}

	valid := &ContentNewsSource{Name: "Feed", SourceType: domain.ContentNewsSourceRSS, RateLimitPerMinute: 60}
	if err := validateNewsSource(ctx, valid); err != nil {
		t.Fatalf("NULL interval should be valid: %v", err)
	}
	for _, value := range []int{0, 14, 1441} {
		invalid := *valid
		invalid.IntervalMinutesOverride = &value
		if err := validateNewsSource(ctx, &invalid); !errors.Is(err, ErrContentNewsInvalidInput) {
			t.Fatalf("interval %d should be rejected, got %v", value, err)
		}
	}
	for _, value := range []int{15, 1440} {
		validExplicit := *valid
		validExplicit.IntervalMinutesOverride = &value
		if err := validateNewsSource(ctx, &validExplicit); err != nil {
			t.Fatalf("interval %d should be accepted: %v", value, err)
		}
	}
}

func TestContentNewsAdminServiceValidatesFetchRun(t *testing.T) {
	svc := NewContentNewsAdminService(&contentNewsAdminRepoStub{})
	if _, err := svc.CreateFetchRun(context.Background(), &ContentFetchRun{SourceID: 1, TaskKey: "x", Status: "bogus"}); !errors.Is(err, ErrContentNewsInvalidInput) {
		t.Fatalf("expected invalid status, got %v", err)
	}
}

func TestContentNewsAdminServiceRetryOnlyTerminalRun(t *testing.T) {
	repo := &contentNewsAdminRepoStub{run: &ContentFetchRun{ID: 7, SourceID: 2, Status: domain.ContentFetchRunning}}
	svc := NewContentNewsAdminService(repo)
	if _, err := svc.RetryFetchRun(context.Background(), 7); !errors.Is(err, ErrContentNewsInvalidStatus) {
		t.Fatalf("expected running retry to be rejected, got %v", err)
	}
	repo.run.Status = domain.ContentFetchFailed
	out, err := svc.RetryFetchRun(context.Background(), 7)
	if err != nil || out.Status != domain.ContentFetchQueued || out.SourceID != 2 {
		t.Fatalf("unexpected retry run: %#v %v", out, err)
	}
}

type contentNewsScheduleSourceStub struct {
	nextRun *time.Time
}

func (s *contentNewsScheduleSourceStub) GetNewsSource(context.Context, int64, bool) (*ContentNewsSource, error) {
	return nil, nil
}
func (s *contentNewsScheduleSourceStub) ListDueNewsSources(context.Context, time.Time, int) ([]ContentNewsSource, error) {
	return nil, nil
}
func (s *contentNewsScheduleSourceStub) RecordNewsSourceFetch(_ context.Context, _ int64, _ time.Time, nextRun *time.Time, _ *string) error {
	s.nextRun = nextRun
	return nil
}

func TestContentNewsWorkerFinishRunUsesGlobalEffectiveIntervalForNullOverride(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	sources := &contentNewsScheduleSourceStub{}
	worker := &ContentNewsWorker{
		sources: sources,
		runs:    &contentNewsWorkerRunStub{},
		now:     func() time.Time { return now },
	}
	globalInterval := 45
	source := &ContentNewsSource{ID: 1, IntervalMinutes: globalInterval, IntervalMinutesOverride: nil}
	if _, err := worker.finishRun(context.Background(), source, &ContentFetchRun{ID: 1}, now, nil); err != nil {
		t.Fatalf("finishRun failed: %v", err)
	}
	if sources.nextRun == nil || !sources.nextRun.Equal(now.Add(45*time.Minute)) {
		t.Fatalf("NULL override did not use effective global interval: got %v", sources.nextRun)
	}
}

func TestContentNewsWorkerFinishRunOverrideWinsOverGlobalEffectiveInterval(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	sources := &contentNewsScheduleSourceStub{}
	worker := &ContentNewsWorker{
		sources: sources,
		runs:    &contentNewsWorkerRunStub{},
		now:     func() time.Time { return now },
	}
	globalInterval := 45
	override := 120
	source := &ContentNewsSource{ID: 1, IntervalMinutes: globalInterval, IntervalMinutesOverride: &override}
	if _, err := worker.finishRun(context.Background(), source, &ContentFetchRun{ID: 1}, now, nil); err != nil {
		t.Fatalf("finishRun failed: %v", err)
	}
	if sources.nextRun == nil || !sources.nextRun.Equal(now.Add(120*time.Minute)) {
		t.Fatalf("override did not win over effective global interval: got %v", sources.nextRun)
	}
}
