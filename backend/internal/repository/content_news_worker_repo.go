package repository

import (
	"context"
	stdsql "database/sql"
	"errors"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/contentautomationsetting"
	"github.com/Wei-Shaw/sub2api/ent/contentfetchrun"
	"github.com/Wei-Shaw/sub2api/ent/contentnewsitem"
	"github.com/Wei-Shaw/sub2api/ent/contentnewssource"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/robfig/cron/v3"
)

// The worker adapter deliberately reuses the admin repository so manual edits
// and scheduled writes share the same mapping and soft-delete semantics.
func (r *contentNewsAdminRepository) ListDueNewsSources(ctx context.Context, now time.Time, limit int) ([]service.ContentNewsSource, error) {
	if limit <= 0 {
		return []service.ContentNewsSource{}, nil
	}
	policy, err := r.GetContentNewsWorkerPolicy(ctx)
	if err != nil {
		return nil, err
	}
	globalInterval := contentNewsGlobalIntervalMinutes(policy.FetchCron)
	q := r.client.ContentNewsSource.Query().Where(
		contentnewssource.EnabledEQ(true),
		contentnewssource.Or(contentnewssource.NextRunAtIsNil(), contentnewssource.NextRunAtLTE(now)),
	).Order(contentnewssource.ByNextRunAt(sql.OrderAsc()), contentnewssource.ByID(sql.OrderAsc())).Limit(limit)
	items, err := q.Limit(limit * 2).All(ctx)
	if err != nil {
		return nil, err
	}
	// Selection is intentionally read-only. The execution path performs the
	// atomic source lease claim immediately before fetching, which avoids a
	// queued run inheriting an already-expired scheduler lease.
	sources := mapContentNewsSources(items)
	for i := range sources {
		if sources[i].IntervalMinutesOverride != nil {
			sources[i].IntervalMinutes = *sources[i].IntervalMinutesOverride
			continue
		}
		if globalInterval <= 0 {
			return nil, errors.New("global content news fetch interval is invalid")
		}
		sources[i].IntervalMinutes = globalInterval
	}
	return sources, nil
}

func contentNewsGlobalIntervalMinutes(expr string) int {
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(strings.TrimSpace(expr))
	if err != nil {
		return 0
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first := schedule.Next(base)
	second := schedule.Next(first)
	minutes := int(second.Sub(first) / time.Minute)
	if minutes <= 0 {
		return 0
	}
	return minutes
}

func (r *contentNewsAdminRepository) ClaimNewsSourceLease(ctx context.Context, id int64, now time.Time) (*time.Time, error) {
	leaseUntil := now.Add(5 * time.Minute)
	n, err := r.client.ContentNewsSource.Update().Where(
		contentnewssource.IDEQ(id), contentnewssource.EnabledEQ(true),
		contentnewssource.Or(contentnewssource.NextRunAtIsNil(), contentnewssource.NextRunAtLTE(now)),
	).SetNextRunAt(leaseUntil).Save(ctx)
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, service.ErrContentNewsSourceLeaseLost
	}
	return &leaseUntil, nil
}

// ClaimNewsSourceLeaseForManual ignores a future scheduler slot but still
// rejects a currently active five-minute worker lease.
func (r *contentNewsAdminRepository) ClaimNewsSourceLeaseForManual(ctx context.Context, id int64, now time.Time) (*time.Time, error) {
	leaseUntil := now.Add(5 * time.Minute)
	n, err := r.client.ContentNewsSource.Update().Where(
		contentnewssource.IDEQ(id), contentnewssource.EnabledEQ(true),
		contentnewssource.Or(contentnewssource.NextRunAtIsNil(), contentnewssource.NextRunAtLTE(now), contentnewssource.NextRunAtGT(leaseUntil)),
	).SetNextRunAt(leaseUntil).Save(ctx)
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, service.ErrContentNewsSourceLeaseLost
	}
	return &leaseUntil, nil
}

func (r *contentNewsAdminRepository) RenewNewsSourceLease(ctx context.Context, id int64, currentLease, now time.Time) (*time.Time, error) {
	nextLease := now.Add(5 * time.Minute)
	n, err := r.client.ContentNewsSource.Update().Where(contentnewssource.IDEQ(id), contentnewssource.NextRunAtEQ(currentLease)).SetNextRunAt(nextLease).Save(ctx)
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, service.ErrContentNewsSourceLeaseLost
	}
	return &nextLease, nil
}

func (r *contentNewsAdminRepository) ReleaseNewsSourceLease(ctx context.Context, id int64, lease time.Time) error {
	_, err := r.client.ContentNewsSource.Update().Where(contentnewssource.IDEQ(id), contentnewssource.NextRunAtEQ(lease)).ClearNextRunAt().Save(ctx)
	return err
}

func (r *contentNewsAdminRepository) RecordNewsSourceFetch(ctx context.Context, id int64, lastRun time.Time, nextRun *time.Time, lastErr *string) error {
	b := r.client.ContentNewsSource.UpdateOneID(id).SetLastRunAt(lastRun)
	setOrClearTime(b.SetNextRunAt, b.ClearNextRunAt, nextRun)
	setOrClearString(b.SetLastError, b.ClearLastError, lastErr)
	_, err := b.Save(ctx)
	return err
}

// RecordNewsSourceFetchLease prevents an expired worker from overwriting a
// newer scheduler claim. The lease value is the claim token for the current
// schema: only the row carrying that exact lease may be completed.
func (r *contentNewsAdminRepository) RecordNewsSourceFetchLease(ctx context.Context, id int64, lastRun time.Time, nextRun *time.Time, lastErr *string, leaseUntil *time.Time) error {
	if leaseUntil == nil {
		return service.ErrContentNewsSourceLeaseLost
	}
	b := r.client.ContentNewsSource.Update().Where(contentnewssource.IDEQ(id), contentnewssource.NextRunAtEQ(*leaseUntil)).SetLastRunAt(lastRun)
	setOrClearTime(b.SetNextRunAt, b.ClearNextRunAt, nextRun)
	setOrClearString(b.SetLastError, b.ClearLastError, lastErr)
	n, err := b.Save(ctx)
	if err != nil {
		return err
	}
	if n != 1 {
		return service.ErrContentNewsSourceLeaseLost
	}
	return nil
}

func (r *contentNewsAdminRepository) ListQueuedFetchRuns(ctx context.Context, limit int) ([]service.ContentFetchRun, error) {
	rows, err := r.client.ContentFetchRun.Query().Where(contentfetchrun.StatusEQ(domain.ContentFetchQueued)).Order(contentfetchrun.ByCreatedAt(sql.OrderAsc()), contentfetchrun.ByID(sql.OrderAsc())).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.ContentFetchRun, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapContentFetchRun(row))
	}
	return out, nil
}

func (r *contentNewsAdminRepository) ClaimFetchRun(ctx context.Context, id int64, now time.Time) (*service.ContentFetchRun, error) {
	updated, err := r.client.ContentFetchRun.Update().Where(contentfetchrun.IDEQ(id), contentfetchrun.StatusEQ(domain.ContentFetchQueued)).SetStatus(domain.ContentFetchRunning).SetStartedAt(now).Save(ctx)
	if err != nil {
		return nil, err
	}
	if updated == 0 {
		return nil, nil
	}
	return r.GetFetchRun(ctx, id)
}

func (r *contentNewsAdminRepository) RecoverStaleFetchRuns(ctx context.Context, olderThan time.Time) error {
	now := time.Now().UTC()
	_, err := r.client.ContentFetchRun.Update().Where(contentfetchrun.StatusEQ(domain.ContentFetchRunning), contentfetchrun.UpdatedAtLT(olderThan)).SetStatus(domain.ContentFetchQueued).SetStartedAt(now).Save(ctx)
	return err
}

func (r *contentNewsAdminRepository) GetFetchRunByTaskKey(ctx context.Context, key string) (*service.ContentFetchRun, error) {
	row, err := r.client.ContentFetchRun.Query().Where(contentfetchrun.TaskKeyEQ(strings.TrimSpace(key))).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, service.ErrContentFetchRunNotFound
		}
		return nil, err
	}
	out := mapContentFetchRun(row)
	return &out, nil
}

func (r *contentNewsAdminRepository) UpsertFetchedNewsItem(ctx context.Context, item *service.ContentNewsItem) (service.ContentNewsFetchItemResult, error) {
	if item == nil {
		return service.ContentNewsFetchItemResult{}, service.ErrContentNewsInvalidInput
	}
	var existing *ent.ContentNewsItem
	var err error
	identity := make([]predicate.ContentNewsItem, 0, 3)
	if item.ExternalID != nil && strings.TrimSpace(*item.ExternalID) != "" {
		identity = append(identity, contentnewsitem.And(contentnewsitem.SourceIDEQ(item.SourceID), contentnewsitem.ExternalIDEQ(strings.TrimSpace(*item.ExternalID))))
	}
	if item.ContentHash != nil && strings.TrimSpace(*item.ContentHash) != "" {
		identity = append(identity, contentnewsitem.ContentHashEQ(strings.TrimSpace(*item.ContentHash)))
	}
	if strings.TrimSpace(item.SourceCanonicalURL) != "" {
		identity = append(identity, contentnewsitem.And(contentnewsitem.SourceIDEQ(item.SourceID), contentnewsitem.SourceCanonicalURLEQ(strings.TrimSpace(item.SourceCanonicalURL))))
	}
	if len(identity) > 0 {
		existing, err = r.client.ContentNewsItem.Query().Where(contentnewsitem.Or(identity...)).First(ctx)
	}
	if err == nil && existing != nil {
		return service.ContentNewsFetchItemResult{Deduplicated: true}, nil
	}
	if err != nil && !ent.IsNotFound(err) {
		return service.ContentNewsFetchItemResult{}, err
	}
	// Fuzzy title dedupe catches syndicated entries whose IDs, URLs and body
	// hashes differ slightly. Restrict the scan to the same source and recent
	// window so it remains bounded and never merges unrelated publishers.
	if existing == nil && strings.TrimSpace(item.Title) != "" {
		window := item.DedupeWindowHours
		if window <= 0 {
			window = 24
		}
		candidates, queryErr := r.client.ContentNewsItem.Query().Where(contentnewsitem.SourceIDEQ(item.SourceID), contentnewsitem.CreatedAtGTE(time.Now().UTC().Add(-time.Duration(window)*time.Hour))).Limit(200).All(ctx)
		if queryErr != nil {
			return service.ContentNewsFetchItemResult{}, queryErr
		}
		for _, candidate := range candidates {
			if newsTitleSimilarity(item.Title, candidate.Title) >= 0.92 {
				return service.ContentNewsFetchItemResult{Deduplicated: true}, nil
			}
		}
	}
	created, err := r.CreateNewsItem(ctx, item)
	if err != nil {
		// A concurrent insert is an idempotent duplicate; the unique index is
		// authoritative even when two workers race.
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return service.ContentNewsFetchItemResult{Deduplicated: true}, nil
		}
		return service.ContentNewsFetchItemResult{}, err
	}
	_ = created
	return service.ContentNewsFetchItemResult{Created: true}, nil
}

func newsTitleSimilarity(left, right string) float64 {
	leftSet, rightSet := newsTitleTokens(left), newsTitleTokens(right)
	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}
	common := 0
	for token := range leftSet {
		if _, ok := rightSet[token]; ok {
			common++
		}
	}
	return float64(common) / float64(len(leftSet)+len(rightSet)-common)
}

func newsTitleTokens(value string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, token := range strings.Fields(strings.ToLower(strings.TrimSpace(value))) {
		if len([]rune(token)) >= 2 {
			set[token] = struct{}{}
		}
	}
	return set
}

func (r *contentNewsAdminRepository) CountAutoPublishedNews(ctx context.Context, from, to time.Time) (int, error) {
	return r.client.ContentNewsItem.Query().Where(contentnewsitem.StatusEQ(domain.ContentStatusPublished), contentnewsitem.PublishModeEQ(domain.ContentNewsPublishAuto), contentnewsitem.PublishedAtGTE(from), contentnewsitem.PublishedAtLT(to)).Count(ctx)
}

func (r *contentNewsAdminRepository) CountFetchedNews(ctx context.Context, from, to time.Time) (int, error) {
	return r.client.ContentNewsItem.Query().Where(contentnewsitem.FetchedAtGTE(from), contentnewsitem.FetchedAtLT(to)).Count(ctx)
}

// ReserveNewsQuota serializes the daily reservation per kind/day. The
// advisory transaction lock makes the check-and-reserve atomic across worker
// instances while keeping the existing content tables unchanged.
func (r *contentNewsAdminRepository) ReserveNewsQuota(ctx context.Context, kind string, day time.Time, limit int) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	drv, ok := r.client.Driver().(*sql.Driver)
	if !ok {
		return false, errors.New("news quota SQL repository unavailable")
	}
	db := drv.DB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	calendarDay := day.Format("2006-01-02")
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`, kind, calendarDay); err != nil {
		return false, err
	}
	var used int
	if err = tx.QueryRowContext(ctx, `INSERT INTO content_news_quota_reservations (quota_day, quota_kind, reserved_count) VALUES ($1::date,$2,1) ON CONFLICT (quota_day,quota_kind) DO UPDATE SET reserved_count=content_news_quota_reservations.reserved_count+1 WHERE content_news_quota_reservations.reserved_count < $3 RETURNING reserved_count`, calendarDay, kind, limit).Scan(&used); err != nil {
		if errors.Is(err, stdsql.ErrNoRows) {
			return false, tx.Commit()
		}
		return false, err
	}
	return true, tx.Commit()
}

func (r *contentNewsAdminRepository) ReleaseNewsQuota(ctx context.Context, kind string, day time.Time) error {
	drv, ok := r.client.Driver().(*sql.Driver)
	if !ok {
		return errors.New("news quota SQL repository unavailable")
	}
	_, err := drv.DB().ExecContext(ctx, `UPDATE content_news_quota_reservations SET reserved_count=GREATEST(reserved_count-1,0), updated_at=NOW() WHERE quota_day=$1::date AND quota_kind=$2`, day.Format("2006-01-02"), kind)
	return err
}

func (r *contentNewsAdminRepository) GetContentNewsWorkerPolicy(ctx context.Context) (*service.ContentNewsWorkerPolicy, error) {
	setting, err := r.client.ContentAutomationSetting.Query().Order(contentautomationsetting.ByID(sql.OrderAsc())).First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return &service.ContentNewsWorkerPolicy{PublishMode: domain.ContentAutomationManualReview}, nil
		}
		return nil, err
	}
	return &service.ContentNewsWorkerPolicy{Enabled: setting.Enabled, Timezone: setting.Timezone, FetchCron: setting.FetchCron, PublishMode: setting.PublishMode, DailyFetchLimit: setting.DailyFetchLimit, DailyPublishLimit: setting.DailyPublishLimit, DedupeWindowHours: setting.DedupeWindowHours, TargetLocales: append([]string(nil), setting.TargetLocales...), PublishStartAt: setting.PublishStartAt, PublishEndAt: setting.PublishEndAt}, nil
}

var _ service.ContentNewsWorkerSourceRepository = (*contentNewsAdminRepository)(nil)
var _ service.ContentNewsWorkerFetchRunRepository = (*contentNewsAdminRepository)(nil)
var _ service.ContentNewsWorkerItemRepository = (*contentNewsAdminRepository)(nil)
var _ service.ContentNewsWorkerPolicyRepository = (*contentNewsAdminRepository)(nil)

// Keep the soft-delete mixin imported in this adapter's compilation unit when
// generated Ent versions change their default query interceptor behavior.
var _ = mixins.SkipSoftDelete
