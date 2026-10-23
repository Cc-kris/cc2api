package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	xhtml "golang.org/x/net/html"
)

const (
	defaultContentNewsFetchTimeout = 20 * time.Second
	defaultContentNewsMaxBodyBytes = 2 << 20
	maxContentNewsErrorSummary     = 500
)

var (
	ErrContentNewsFetchURLInvalid   = errors.New("content news fetch URL is invalid")
	ErrContentNewsFetchURLBlocked   = errors.New("content news fetch URL resolves to a non-public address")
	ErrContentNewsFeedInvalid       = errors.New("content news feed is invalid")
	ErrContentNewsFeedEmpty         = errors.New("content news feed contains no entries")
	ErrContentNewsArticleIncomplete = errors.New("content news article body is incomplete")
	ErrContentNewsSourceDisabled    = errors.New("content news source is disabled or not authorized")
)

// ContentNewsWorkerSourceRepository owns source selection and source-level run
// bookkeeping. Its implementation should make ListDueNewsSources atomic with its
// own scheduler claim if more than one application instance can run workers.
type ContentNewsWorkerSourceRepository interface {
	GetNewsSource(context.Context, int64, bool) (*ContentNewsSource, error)
	ListDueNewsSources(context.Context, time.Time, int) ([]ContentNewsSource, error)
	RecordNewsSourceFetch(context.Context, int64, time.Time, *time.Time, *string) error
}

// ContentNewsWorkerLeaseRepository is an optional stronger completion path.
// Implementations must only commit the result when the source lease still
// belongs to this run; legacy adapters continue to use RecordNewsSourceFetch.
type ContentNewsWorkerLeaseRepository interface {
	RecordNewsSourceFetchLease(context.Context, int64, time.Time, *time.Time, *string, *time.Time) error
}
type ContentNewsSourceLeaseRenewer interface {
	RenewNewsSourceLease(context.Context, int64, time.Time, time.Time) (*time.Time, error)
}
type ContentNewsSourceLeaseClaimer interface {
	ClaimNewsSourceLease(context.Context, int64, time.Time) (*time.Time, error)
}
type ContentNewsManualLeaseClaimer interface {
	ClaimNewsSourceLeaseForManual(context.Context, int64, time.Time) (*time.Time, error)
}
type ContentNewsSourceLeaseReleaser interface {
	ReleaseNewsSourceLease(context.Context, int64, time.Time) error
}

// ContentNewsWorkerFetchRunRepository persists the observable fetch state
// machine: queued -> running -> succeeded|partial|failed.
type ContentNewsWorkerFetchRunRepository interface {
	CreateFetchRun(context.Context, *ContentFetchRun) (*ContentFetchRun, error)
	UpdateFetchRun(context.Context, *ContentFetchRun) (*ContentFetchRun, error)
}

// ContentNewsQueuedRunRepository is optional for storage adapters that support
// asynchronous manual/retry execution. The legacy run interface remains useful
// for deterministic unit tests and synchronous worker execution.
type ContentNewsQueuedRunRepository interface {
	ContentNewsWorkerFetchRunRepository
	ListQueuedFetchRuns(context.Context, int) ([]ContentFetchRun, error)
	ClaimFetchRun(context.Context, int64, time.Time) (*ContentFetchRun, error)
}
type ContentNewsQueuedRunRecovery interface {
	RecoverStaleFetchRuns(context.Context, time.Time) error
}
type ContentNewsTaskKeyRepository interface {
	GetFetchRunByTaskKey(context.Context, string) (*ContentFetchRun, error)
}

// ContentNewsWorkerItemRepository persists normalized feed entries. The upsert
// operation must deduplicate by source/external ID and content hash without
// overwriting an editor's status or edits on an existing item.
type ContentNewsWorkerItemRepository interface {
	UpsertFetchedNewsItem(context.Context, *ContentNewsItem) (ContentNewsFetchItemResult, error)
	CountAutoPublishedNews(context.Context, time.Time, time.Time) (int, error)
	CountFetchedNews(context.Context, time.Time, time.Time) (int, error)
}
type ContentNewsQuotaRepository interface {
	ReserveNewsQuota(context.Context, string, time.Time, int) (bool, error)
	ReleaseNewsQuota(context.Context, string, time.Time) error
}

// ContentNewsWorkerPolicyRepository returns the database-backed automation
// configuration. A zero daily limit means no product limit, rather than a
// hard-coded worker limit.
type ContentNewsWorkerPolicyRepository interface {
	GetContentNewsWorkerPolicy(context.Context) (*ContentNewsWorkerPolicy, error)
}

type ContentNewsWorkerPolicy struct {
	Enabled           bool
	Timezone          string
	FetchCron         string
	PublishMode       string
	DailyFetchLimit   int
	DailyPublishLimit int
	DedupeWindowHours int
	TargetLocales     []string
	PublishStartAt    *time.Time
	PublishEndAt      *time.Time
}

type ContentNewsFetchItemResult struct {
	Created      bool
	Deduplicated bool
}

type ContentNewsHTTPFetcher interface {
	Fetch(context.Context, string) ([]byte, string, error)
}

// ContentNewsArticleFetcher is optional: feed-only test doubles and providers
// that intentionally do not permit article retrieval can omit it. The built-in
// safe fetcher implements it and applies the same redirect/SSRF checks.
type ContentNewsArticleFetcher interface {
	FetchArticle(context.Context, string) ([]byte, string, error)
}

// ContentNewsWorker is deliberately transport- and storage-independent so it
// can be run by a cron process, a job queue, or a single admin-triggered job.
type ContentNewsWorker struct {
	sources ContentNewsWorkerSourceRepository
	runs    ContentNewsWorkerFetchRunRepository
	items   ContentNewsWorkerItemRepository
	policy  ContentNewsWorkerPolicyRepository
	fetcher ContentNewsHTTPFetcher
	now     func() time.Time
}

func (w *ContentNewsWorker) clock() time.Time {
	if w != nil && w.now != nil {
		return w.now()
	}
	return time.Now()
}

func NewContentNewsWorker(sources ContentNewsWorkerSourceRepository, runs ContentNewsWorkerFetchRunRepository, items ContentNewsWorkerItemRepository, policy ContentNewsWorkerPolicyRepository, fetcher ContentNewsHTTPFetcher) *ContentNewsWorker {
	if fetcher == nil {
		fetcher = NewSafeContentNewsHTTPFetcher(nil, nil)
	}
	return &ContentNewsWorker{sources: sources, runs: runs, items: items, policy: policy, fetcher: fetcher, now: time.Now}
}

// RunDueFetches runs all sources due at now. maxSources is supplied by the
// scheduler configuration; it is a work-batch size, not a hidden daily quota.
func (w *ContentNewsWorker) RunDueFetches(ctx context.Context, maxSources int) ([]ContentFetchRun, error) {
	if w == nil || w.sources == nil {
		return nil, errors.New("content news worker source repository is required")
	}
	if maxSources <= 0 {
		return []ContentFetchRun{}, nil
	}
	if w.policy != nil {
		policy, err := w.policy.GetContentNewsWorkerPolicy(ctx)
		if err != nil {
			return nil, err
		}
		if policy == nil || !policy.Enabled {
			return []ContentFetchRun{}, nil
		}
	}
	now := w.clock().UTC()
	sources, err := w.sources.ListDueNewsSources(ctx, now, maxSources)
	if err != nil {
		return nil, err
	}
	runs := make([]ContentFetchRun, 0, len(sources))
	var errs []error
	for _, source := range sources {
		run, runErr := w.RunSource(ctx, source.ID, domain.ContentFetchTriggerCron)
		if run != nil {
			runs = append(runs, *run)
		}
		if runErr != nil {
			errs = append(errs, fmt.Errorf("source %d: %w", source.ID, runErr))
		}
	}
	return runs, errors.Join(errs...)
}

// RunSource performs one persisted fetch run. A transport or parser failure is
// recorded as a failed run; per-entry failures result in partial success when at
// least one entry was saved.
func (w *ContentNewsWorker) RunSource(ctx context.Context, sourceID int64, trigger string) (*ContentFetchRun, error) {
	return w.runSource(ctx, sourceID, trigger, "", nil)
}

// RunSourceWithTaskKey is used by manual/retry endpoints so the persisted
// source+task key unique index provides idempotency for repeated clicks.
func (w *ContentNewsWorker) RunSourceWithTaskKey(ctx context.Context, sourceID int64, trigger, taskKey string) (*ContentFetchRun, error) {
	if existing, ok := w.runs.(ContentNewsTaskKeyRepository); ok && strings.TrimSpace(taskKey) != "" {
		if found, err := existing.GetFetchRunByTaskKey(ctx, strings.TrimSpace(taskKey)); err == nil && found != nil {
			return found, nil
		}
	}
	return w.runSource(ctx, sourceID, trigger, strings.TrimSpace(taskKey), nil)
}

// EnqueueSource persists a queued run and returns immediately. A scheduler
// heartbeat later claims and executes it through ProcessQueuedRuns.
func (w *ContentNewsWorker) EnqueueSource(ctx context.Context, sourceID int64, trigger, taskKey string) (*ContentFetchRun, error) {
	if w == nil || w.sources == nil || w.runs == nil {
		return nil, errors.New("content news worker dependencies are required")
	}
	source, err := w.sources.GetNewsSource(ctx, sourceID, false)
	if err != nil {
		return nil, err
	}
	if err := contentNewsSourceMayFetch(source); err != nil {
		return nil, err
	}
	taskKey = strings.TrimSpace(taskKey)
	if taskKey == "" {
		return nil, ErrContentNewsInvalidInput
	}
	if existing, ok := w.runs.(ContentNewsTaskKeyRepository); ok {
		if found, getErr := existing.GetFetchRunByTaskKey(ctx, taskKey); getErr == nil && found != nil {
			return found, nil
		}
	}
	// Queueing is deliberately lease-free. The source lease is acquired by
	// runSource immediately before execution, so a delayed queued row cannot
	// reserve a five-minute lease and later run after that lease has expired.
	run, err := w.runs.CreateFetchRun(ctx, &ContentFetchRun{SourceID: sourceID, TriggerType: trigger, Status: domain.ContentFetchQueued, TaskKey: taskKey})
	if err != nil {
		if existing, ok := w.runs.(ContentNewsTaskKeyRepository); ok {
			if found, getErr := existing.GetFetchRunByTaskKey(ctx, taskKey); getErr == nil && found != nil {
				return found, nil
			}
		}
		return nil, err
	}
	return run, nil
}

func (w *ContentNewsWorker) ProcessQueuedRuns(ctx context.Context, limit int) error {
	q, ok := w.runs.(ContentNewsQueuedRunRepository)
	if !ok {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}
	if recovery, ok := w.runs.(ContentNewsQueuedRunRecovery); ok {
		if err := recovery.RecoverStaleFetchRuns(ctx, w.clock().UTC().Add(-15*time.Minute)); err != nil {
			return err
		}
	}
	runs, err := q.ListQueuedFetchRuns(ctx, limit)
	if err != nil {
		return err
	}
	var runErrors []error
	for _, queued := range runs {
		claimed, claimErr := q.ClaimFetchRun(ctx, queued.ID, w.clock().UTC())
		if claimErr != nil {
			return claimErr
		}
		if claimed == nil {
			continue
		}
		if _, runErr := w.runSourceForExistingRun(ctx, claimed); runErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.ErrorContext(ctx, "content news queued run failed", "run_id", claimed.ID, "source_id", claimed.SourceID, "error", runErr)
			runErrors = append(runErrors, fmt.Errorf("run %d: %w", claimed.ID, runErr))
			// A queued row must never remain running when source validation or
			// lease acquisition fails before the fetch pipeline can finish it.
			claimed.Status = domain.ContentFetchFailed
			finished := w.clock().UTC()
			claimed.FinishedAt = &finished
			summary := contentNewsSafeError(runErr)
			claimed.ErrorSummary = &summary
			if _, updateErr := q.UpdateFetchRun(ctx, claimed); updateErr != nil {
				runErrors = append(runErrors, fmt.Errorf("run %d state update: %w", claimed.ID, updateErr))
			}
		}
	}
	return errors.Join(runErrors...)
}

func (w *ContentNewsWorker) runSourceForExistingRun(ctx context.Context, run *ContentFetchRun) (*ContentFetchRun, error) {
	if run == nil {
		return nil, ErrContentNewsInvalidInput
	}
	// Existing queued rows are claimed before execution; runSourceWithTaskKey
	// would create a second row, so reuse the persisted task key and cleanly
	// execute through the same pipeline after the claim.
	return w.runSource(ctx, run.SourceID, run.TriggerType, run.TaskKey, run)
}

func (w *ContentNewsWorker) runSource(ctx context.Context, sourceID int64, trigger, taskKey string, existing *ContentFetchRun) (*ContentFetchRun, error) {
	if w == nil || w.sources == nil || w.runs == nil || w.items == nil || w.fetcher == nil {
		return nil, errors.New("content news worker dependencies are required")
	}
	if sourceID <= 0 {
		return nil, ErrContentNewsSourceNotFound
	}
	if trigger == "" {
		trigger = domain.ContentFetchTriggerManual
	}
	if !domain.IsContentFetchTrigger(trigger) {
		return nil, ErrContentNewsInvalidInput
	}
	source, err := w.sources.GetNewsSource(ctx, sourceID, false)
	if err != nil {
		return nil, err
	}
	if err := contentNewsSourceMayFetch(source); err != nil {
		return nil, err
	}
	now := w.clock().UTC()
	if claimer, ok := w.sources.(ContentNewsSourceLeaseClaimer); ok {
		var leaseErr error
		var lease *time.Time
		if trigger == domain.ContentFetchTriggerManual {
			if manual, manualOK := w.sources.(ContentNewsManualLeaseClaimer); manualOK {
				lease, leaseErr = manual.ClaimNewsSourceLeaseForManual(ctx, sourceID, now)
			} else {
				lease, leaseErr = claimer.ClaimNewsSourceLease(ctx, sourceID, now)
			}
		} else {
			lease, leaseErr = claimer.ClaimNewsSourceLease(ctx, sourceID, now)
		}
		if leaseErr != nil || lease == nil {
			if leaseErr == nil {
				leaseErr = ErrContentNewsSourceLeaseLost
			}
			return nil, leaseErr
		}
		// Keep the newly acquired token on the in-memory source so renewal and
		// completion use the same CAS value, including delayed queued runs.
		source.NextRunAt = lease
	}
	if taskKey == "" {
		taskKey = fmt.Sprintf("news-%d-%d", source.ID, now.UnixNano())
	}
	run := existing
	if run == nil {
		run, err = w.runs.CreateFetchRun(ctx, &ContentFetchRun{SourceID: source.ID, TriggerType: trigger, Status: domain.ContentFetchQueued, TaskKey: taskKey})
		if err != nil {
			return nil, err
		}
	}
	run.Status, run.StartedAt = domain.ContentFetchRunning, &now
	run, err = w.runs.UpdateFetchRun(ctx, run)
	if err != nil {
		return nil, err
	}

	body, finalURL, fetchErr := w.fetcher.Fetch(ctx, strings.TrimSpace(ptrString(source.URL)))
	if fetchErr != nil {
		return w.finishFailedRun(ctx, source, run, now, fetchErr)
	}
	entries, parseErr := ParseContentNewsFeedWithMapping(body, source.SourceType, source.MappingJSON)
	if parseErr != nil {
		return w.finishFailedRun(ctx, source, run, now, parseErr)
	}
	if len(entries) == 0 {
		return w.finishFailedRun(ctx, source, run, now, ErrContentNewsFeedEmpty)
	}
	policy, policyErr := w.workerPolicy(ctx)
	if policyErr != nil {
		return w.finishFailedRun(ctx, source, run, now, policyErr)
	}
	quota, hasQuota := w.items.(ContentNewsQuotaRepository)
	publishedToday := 0
	fetchedToday := 0
	// The repository-backed reservation is the authoritative atomic path. Keep
	// the count fallback only for legacy/unit repositories that do not implement
	// the reservation contract.
	if !hasQuota && policy.DailyFetchLimit > 0 {
		start, end := localDayBounds(now, policy.Timezone)
		fetchedToday, policyErr = w.items.CountFetchedNews(ctx, start, end)
		if policyErr != nil {
			return w.finishFailedRun(ctx, source, run, now, policyErr)
		}
	}
	if !hasQuota && policy.DailyPublishLimit > 0 {
		start, end := localDayBounds(now, policy.Timezone)
		publishedToday, policyErr = w.items.CountAutoPublishedNews(ctx, start, end)
		if policyErr != nil {
			return w.finishFailedRun(ctx, source, run, now, policyErr)
		}
	}
	var firstErr error
	articleFetches := 0
	articleFetchBudget := source.RateLimitPerMinute - 1 // the feed request consumes one request
	if articleFetchBudget < 0 {
		articleFetchBudget = 0
	}
	for _, entry := range entries {
		if renewer, ok := w.sources.(ContentNewsSourceLeaseRenewer); ok && source.NextRunAt != nil {
			lease, renewErr := renewer.RenewNewsSourceLease(ctx, source.ID, *source.NextRunAt, w.clock().UTC())
			if renewErr != nil {
				return w.finishFailedRun(ctx, source, run, now, renewErr)
			}
			if lease == nil {
				return w.finishFailedRun(ctx, source, run, now, ErrContentNewsSourceLeaseLost)
			}
			source.NextRunAt = lease
		}
		run.FetchedCount++
		if !hasQuota && policy.DailyFetchLimit > 0 && fetchedToday >= policy.DailyFetchLimit {
			continue
		}
		if !entry.BodyComplete && strings.TrimSpace(entry.CanonicalURL) != "" {
			var articleErr error
			if articleFetcher, ok := w.fetcher.(ContentNewsArticleFetcher); ok && articleFetches < articleFetchBudget {
				if source.RateLimitPerMinute > 0 && articleFetches > 0 {
					delay := time.Minute / time.Duration(source.RateLimitPerMinute)
					timer := time.NewTimer(delay)
					select {
					case <-ctx.Done():
						timer.Stop()
						return w.finishFailedRun(ctx, source, run, now, ctx.Err())
					case <-timer.C:
					}
				}
				articleFetches++
				articleBody, articleURL, fetchErr := articleFetcher.FetchArticle(ctx, entry.CanonicalURL)
				articleErr = fetchErr
				if articleErr == nil {
					articleHTML, articleText := ExtractContentNewsArticleHTML(articleBody)
					if articleText != "" {
						entry.BodyHTML, entry.BodyComplete, entry.BodySourceType = articleHTML, true, domain.ContentNewsBodySourceHTMLExtract
						if articleURL != "" {
							entry.CanonicalURL = articleURL
						}
					}
				}
			}
			if !entry.BodyComplete {
				run.FailedCount++
				if firstErr == nil {
					if articleErr != nil {
						firstErr = articleErr
					} else {
						firstErr = ErrContentNewsArticleIncomplete
					}
				}
				// Persist as pending_review so an operator can complete/review it later.
				entry.BodyComplete = false
			}
		}
		item, itemErr := buildFetchedContentNewsItem(source, entry, finalURL, now, policy)
		if itemErr != nil {
			run.FailedCount++
			if firstErr == nil {
				firstErr = itemErr
			}
			continue
		}
		item.DedupeWindowHours = policy.DedupeWindowHours
		if !hasQuota && item.Status == domain.ContentStatusPublished && policy.DailyPublishLimit > 0 {
			if publishedToday >= policy.DailyPublishLimit {
				item.Status, item.PublishedAt = domain.ContentStatusDraft, nil
			}
		}
		fetchReserved, publishReserved := false, false
		if hasQuota {
			day, _ := localDayBounds(now, policy.Timezone)
			if policy.DailyFetchLimit > 0 {
				reserved, quotaErr := quota.ReserveNewsQuota(ctx, "fetch", day, policy.DailyFetchLimit)
				if quotaErr != nil {
					run.FailedCount++
					if firstErr == nil {
						firstErr = quotaErr
					}
					continue
				}
				fetchReserved = reserved
				if !reserved {
					item.Status, item.PublishedAt = domain.ContentStatusDraft, nil
				}
			}
			if item.Status == domain.ContentStatusPublished && policy.DailyPublishLimit > 0 {
				reserved, quotaErr := quota.ReserveNewsQuota(ctx, "publish", day, policy.DailyPublishLimit)
				if quotaErr != nil {
					if fetchReserved {
						_ = quota.ReleaseNewsQuota(ctx, "fetch", day)
					}
					run.FailedCount++
					if firstErr == nil {
						firstErr = quotaErr
					}
					continue
				}
				publishReserved = reserved
				if !reserved {
					item.Status, item.PublishedAt = domain.ContentStatusDraft, nil
				}
			}
		}
		result, saveErr := w.items.UpsertFetchedNewsItem(ctx, item)
		if saveErr != nil {
			if hasQuota {
				day, _ := localDayBounds(now, policy.Timezone)
				if fetchReserved {
					_ = quota.ReleaseNewsQuota(ctx, "fetch", day)
				}
				if publishReserved {
					_ = quota.ReleaseNewsQuota(ctx, "publish", day)
				}
			}
			run.FailedCount++
			if firstErr == nil {
				firstErr = saveErr
			}
			continue
		}
		if result.Deduplicated {
			if hasQuota {
				day, _ := localDayBounds(now, policy.Timezone)
				if fetchReserved {
					_ = quota.ReleaseNewsQuota(ctx, "fetch", day)
				}
				if publishReserved {
					_ = quota.ReleaseNewsQuota(ctx, "publish", day)
				}
			}
			run.DuplicateCount++
		} else if result.Created {
			run.CreatedCount++
			if !hasQuota && policy.DailyFetchLimit > 0 {
				fetchedToday++
			}
			if !hasQuota && item.Status == domain.ContentStatusPublished {
				publishedToday++
			}
		}
	}
	return w.finishRun(ctx, source, run, now, firstErr)
}

func (w *ContentNewsWorker) finishFailedRun(ctx context.Context, source *ContentNewsSource, run *ContentFetchRun, now time.Time, cause error) (*ContentFetchRun, error) {
	run.FailedCount++
	return w.finishRun(ctx, source, run, now, cause)
}

func (w *ContentNewsWorker) finishRun(ctx context.Context, source *ContentNewsSource, run *ContentFetchRun, now time.Time, cause error) (*ContentFetchRun, error) {
	finished := w.clock().UTC()
	run.FinishedAt = &finished
	if cause == nil && run.FailedCount == 0 {
		run.Status = domain.ContentFetchSucceeded
	} else if run.CreatedCount > 0 || run.DuplicateCount > 0 {
		run.Status = domain.ContentFetchPartial
	} else {
		run.Status = domain.ContentFetchFailed
	}
	if cause != nil {
		summary := contentNewsSafeError(cause)
		run.ErrorSummary = &summary
	}
	updated, updateErr := w.runs.UpdateFetchRun(ctx, run)
	if updateErr != nil {
		return nil, updateErr
	}
	var sourceErr *string
	if cause != nil {
		value := contentNewsSafeError(cause)
		sourceErr = &value
	}
	nextRun := now.Add(time.Duration(source.IntervalMinutes) * time.Minute)
	if leaseRepo, ok := w.sources.(ContentNewsWorkerLeaseRepository); ok {
		var leaseUntil *time.Time
		if source.NextRunAt != nil {
			lease := source.NextRunAt.UTC()
			leaseUntil = &lease
		}
		if err := leaseRepo.RecordNewsSourceFetchLease(ctx, source.ID, now, &nextRun, sourceErr, leaseUntil); err != nil {
			return updated, err
		}
	} else if err := w.sources.RecordNewsSourceFetch(ctx, source.ID, now, &nextRun, sourceErr); err != nil {
		return updated, err
	}
	return updated, cause
}

func (w *ContentNewsWorker) workerPolicy(ctx context.Context) (*ContentNewsWorkerPolicy, error) {
	if w.policy == nil {
		return &ContentNewsWorkerPolicy{Timezone: "UTC", PublishMode: domain.ContentAutomationManualReview}, nil
	}
	policy, err := w.policy.GetContentNewsWorkerPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if policy == nil || !policy.Enabled {
		return &ContentNewsWorkerPolicy{Timezone: "UTC", PublishMode: domain.ContentAutomationManualReview}, nil
	}
	if !domain.IsContentAutomationPublishMode(policy.PublishMode) {
		return nil, ErrContentNewsInvalidInput
	}
	if strings.TrimSpace(policy.Timezone) == "" {
		policy.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(policy.Timezone); err != nil {
		return nil, ErrContentNewsInvalidInput
	}
	return policy, nil
}

func contentNewsSourceMayFetch(source *ContentNewsSource) error {
	if source == nil || !source.Enabled || source.URL == nil || strings.TrimSpace(*source.URL) == "" || source.SourceType == domain.ContentNewsSourceManual {
		return ErrContentNewsSourceDisabled
	}
	if source.TermsConfirmedAt == nil || source.AuthorizationNote == nil || strings.TrimSpace(*source.AuthorizationNote) == "" || source.RobotsPolicy == nil || strings.TrimSpace(*source.RobotsPolicy) == "" {
		return ErrContentNewsSourceDisabled
	}
	return nil
}

type ContentNewsFeedEntry struct {
	ExternalID          string
	CanonicalURL        string
	Title               string
	Summary             string
	BodyHTML            string
	BodyComplete        bool
	BodySourceType      string
	SourceName          string
	ImageURL            string
	OriginalPublishedAt *time.Time
}

// ParseContentNewsFeed parses only authorized RSS/Atom/JSON payloads. It never
// follows item links itself; RunSource may use the same SSRF-safe article fetcher
// to capture a complete body before persistence.
func ParseContentNewsFeed(body []byte, sourceType string) ([]ContentNewsFeedEntry, error) {
	return ParseContentNewsFeedWithMapping(body, sourceType, nil)
}

func ParseContentNewsFeedWithMapping(body []byte, sourceType string, mapping map[string]string) ([]ContentNewsFeedEntry, error) {
	if len(body) == 0 || len(body) > defaultContentNewsMaxBodyBytes {
		return nil, ErrContentNewsFeedInvalid
	}
	if sourceType == domain.ContentNewsSourcePublicJSON {
		return parseJSONContentNewsFeedWithMapping(body, mapping)
	}
	var root struct{ XMLName xml.Name }
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrContentNewsFeedInvalid, err)
	}
	switch strings.ToLower(root.XMLName.Local) {
	case "rss", "rdf":
		if sourceType != "" && sourceType != domain.ContentNewsSourceRSS {
			return nil, ErrContentNewsFeedInvalid
		}
		return parseRSSContentNewsFeed(body)
	case "feed":
		if sourceType != "" && sourceType != domain.ContentNewsSourceAtom {
			return nil, ErrContentNewsFeedInvalid
		}
		return parseAtomContentNewsFeed(body)
	default:
		return nil, ErrContentNewsFeedInvalid
	}
}

// publicJSON feeds intentionally accept a small, documented envelope rather
// than arbitrary JSON. This makes social feed adapters deterministic and avoids
// storing unknown provider fields in the content database.
type contentNewsJSONEnvelope struct {
	Items []contentNewsJSONItem `json:"items"`
	Data  []contentNewsJSONItem `json:"data"`
}

type contentNewsJSONItem struct {
	ID          string `json:"id"`
	ExternalID  string `json:"external_id"`
	URL         string `json:"url"`
	Link        string `json:"link"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Body        string `json:"body"`
	Content     string `json:"content"`
	Source      string `json:"source"`
	ImageURL    string `json:"image_url"`
	PublishedAt string `json:"published_at"`
}

func parseJSONContentNewsFeed(body []byte) ([]ContentNewsFeedEntry, error) {
	return parseJSONContentNewsFeedWithMapping(body, nil)
}

func parseJSONContentNewsFeedWithMapping(body []byte, mapping map[string]string) ([]ContentNewsFeedEntry, error) {
	if len(mapping) > 0 {
		return parseMappedJSONContentNewsFeed(body, mapping)
	}
	var envelope contentNewsJSONEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		// Also accept a top-level array for simple public feeds.
		var items []contentNewsJSONItem
		if arrayErr := json.Unmarshal(body, &items); arrayErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrContentNewsFeedInvalid, err)
		}
		envelope.Items = items
	}
	items := envelope.Items
	if len(items) == 0 {
		items = envelope.Data
	}
	if len(items) == 0 {
		return nil, ErrContentNewsFeedInvalid
	}
	entries := make([]ContentNewsFeedEntry, 0, len(items))
	for _, item := range items {
		bodyHTML := strings.TrimSpace(item.Body)
		if bodyHTML == "" {
			bodyHTML = strings.TrimSpace(item.Content)
		}
		complete := bodyHTML != ""
		if bodyHTML == "" {
			bodyHTML = strings.TrimSpace(item.Description)
		}
		externalID := strings.TrimSpace(item.ID)
		if externalID == "" {
			externalID = strings.TrimSpace(item.ExternalID)
		}
		canonical := strings.TrimSpace(item.URL)
		if canonical == "" {
			canonical = strings.TrimSpace(item.Link)
		}
		published := parseContentNewsTime(item.PublishedAt)
		entries = append(entries, ContentNewsFeedEntry{
			ExternalID: externalID, CanonicalURL: canonical, Title: strings.TrimSpace(item.Title),
			Summary: strings.TrimSpace(item.Summary), BodyHTML: bodyHTML, BodyComplete: complete, BodySourceType: domain.ContentNewsBodySourcePublicAPI,
			SourceName: strings.TrimSpace(item.Source), ImageURL: strings.TrimSpace(item.ImageURL), OriginalPublishedAt: published,
		})
	}
	return entries, nil
}

func parseMappedJSONContentNewsFeed(body []byte, mapping map[string]string) ([]ContentNewsFeedEntry, error) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrContentNewsFeedInvalid, err)
	}
	var items []any
	switch value := root.(type) {
	case []any:
		items = value
	case map[string]any:
		if values, ok := value["items"].([]any); ok {
			items = values
		} else if values, ok := value["data"].([]any); ok {
			items = values
		}
	}
	if len(items) == 0 {
		return nil, ErrContentNewsFeedInvalid
	}
	value := func(item map[string]any, name string) string {
		raw := jsonPathValue(item, mapping[name])
		text, _ := raw.(string)
		return strings.TrimSpace(text)
	}
	entries := make([]ContentNewsFeedEntry, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		bodyHTML := firstContentNewsNonEmpty(value(item, "body"), value(item, "content"), value(item, "description"))
		entries = append(entries, ContentNewsFeedEntry{ExternalID: firstContentNewsNonEmpty(value(item, "id"), value(item, "external_id")), CanonicalURL: firstContentNewsNonEmpty(value(item, "url"), value(item, "link")), Title: value(item, "title"), Summary: value(item, "summary"), BodyHTML: bodyHTML, BodyComplete: bodyHTML != "", BodySourceType: domain.ContentNewsBodySourcePublicAPI, SourceName: value(item, "source"), ImageURL: value(item, "image_url"), OriginalPublishedAt: parseContentNewsTime(value(item, "published_at"))})
	}
	if len(entries) == 0 {
		return nil, ErrContentNewsFeedInvalid
	}
	return entries, nil
}

func firstContentNewsNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func jsonPathValue(root map[string]any, path string) any {
	var current any = root
	for _, part := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[part]
	}
	return current
}

type contentNewsRSS struct {
	Channel struct {
		Items []struct {
			GUID        string `xml:"guid"`
			Link        string `xml:"link"`
			Title       string `xml:"title"`
			Description string `xml:"description"`
			Encoded     string `xml:"encoded"`
			PubDate     string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

func parseRSSContentNewsFeed(body []byte) ([]ContentNewsFeedEntry, error) {
	var feed contentNewsRSS
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrContentNewsFeedInvalid, err)
	}
	entries := make([]ContentNewsFeedEntry, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		content := strings.TrimSpace(item.Encoded)
		complete := content != ""
		if content == "" {
			content = item.Description
		}
		entries = append(entries, ContentNewsFeedEntry{
			ExternalID: strings.TrimSpace(item.GUID), CanonicalURL: strings.TrimSpace(item.Link), Title: strings.TrimSpace(item.Title),
			Summary: textFromContentNewsHTML(item.Description), BodyHTML: content, BodyComplete: complete,
			OriginalPublishedAt: parseContentNewsTime(item.PubDate),
		})
	}
	return entries, nil
}

type contentNewsAtom struct {
	Entries []struct {
		ID      string `xml:"id"`
		Title   string `xml:"title"`
		Summary string `xml:"summary"`
		Content string `xml:"content"`
		Updated string `xml:"updated"`
		Publish string `xml:"published"`
		Links   []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

func parseAtomContentNewsFeed(body []byte) ([]ContentNewsFeedEntry, error) {
	var feed contentNewsAtom
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrContentNewsFeedInvalid, err)
	}
	entries := make([]ContentNewsFeedEntry, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		link := ""
		for _, candidate := range entry.Links {
			if candidate.Rel == "" || candidate.Rel == "alternate" {
				link = strings.TrimSpace(candidate.Href)
				break
			}
		}
		content := strings.TrimSpace(entry.Content)
		complete := content != ""
		if content == "" {
			content = entry.Summary
		}
		published := parseContentNewsTime(entry.Publish)
		if published == nil {
			published = parseContentNewsTime(entry.Updated)
		}
		entries = append(entries, ContentNewsFeedEntry{
			ExternalID: strings.TrimSpace(entry.ID), CanonicalURL: link, Title: strings.TrimSpace(entry.Title),
			Summary: textFromContentNewsHTML(entry.Summary), BodyHTML: content, BodyComplete: complete, OriginalPublishedAt: published,
		})
	}
	return entries, nil
}

func buildFetchedContentNewsItem(source *ContentNewsSource, entry ContentNewsFeedEntry, finalURL string, now time.Time, policy *ContentNewsWorkerPolicy) (*ContentNewsItem, error) {
	if source == nil || strings.TrimSpace(entry.Title) == "" {
		return nil, ErrContentNewsFeedInvalid
	}
	cleanHTML, bodyText := SanitizeContentNewsHTML(entry.BodyHTML)
	incomplete := !entry.BodyComplete || strings.TrimSpace(bodyText) == ""
	canonical := strings.TrimSpace(entry.CanonicalURL)
	if canonical == "" {
		canonical = finalURL
	}
	if canonical == "" {
		return nil, ErrContentNewsFeedInvalid
	}
	summary := strings.TrimSpace(entry.Summary)
	if summary == "" {
		summary = contentNewsSummary(bodyText)
	}
	hash := contentNewsHash(canonical + "\n" + entry.Title + "\n" + bodyText)
	bodyHash := contentNewsHash(bodyText)
	// Normalize at the write boundary: sources store free-form language tags
	// ("en", "zh-Hans"), while the public routes and the admin list match on the
	// canonical form ("en-US", "zh-CN"). Without this, freshly fetched items are
	// invisible on the site until a manual DB fix.
	locale := domain.NormalizeContentLocale(source.Language)
	if locale == "" {
		locale = "zh-CN"
	}
	status := domain.ContentStatusDraft
	if incomplete {
		status = domain.ContentStatusPendingReview
	}
	var publishedAt *time.Time
	if !incomplete && policy != nil && policy.Enabled && policy.PublishMode == domain.ContentAutomationAutoPublish && contentNewsAutoPublishable(entry, cleanHTML, bodyText, summary) && contentNewsLocaleAllowed(locale, policy.TargetLocales) && contentNewsWithinPublishWindow(now, policy) {
		status = domain.ContentStatusPublished
		at := now.UTC()
		publishedAt = &at
	}
	fetchedAt := now.UTC()
	bodyFetchedAt := now.UTC()
	name := source.Name
	if strings.TrimSpace(entry.SourceName) != "" {
		name = strings.TrimSpace(entry.SourceName)
	}
	imageURL := optionalString(entry.ImageURL)
	media := map[string]any{}
	if imageURL != nil {
		media["image_url"] = *imageURL
	}
	var bodyHTMLPtr, bodyTextPtr *string
	if !incomplete {
		bodyHTMLPtr, bodyTextPtr = optionalStringIfNonBlank(cleanHTML), optionalStringIfNonBlank(bodyText)
	}
	return &ContentNewsItem{
		SourceID: source.ID, ExternalID: optionalString(entry.ExternalID), SourceCanonicalURL: canonical,
		Slug: contentNewsSlug(entry.Title, hash), Locale: locale, Title: strings.TrimSpace(entry.Title), Summary: &summary,
		BodyHTML: bodyHTMLPtr, BodyText: bodyTextPtr, BodySourceType: stringPointer(contentNewsBodySourceType(entry)), BodyFetchedAt: func() *time.Time {
			if incomplete {
				return nil
			}
			return &bodyFetchedAt
		}(),
		BodyHash: &bodyHash, ImageURL: imageURL, SourceName: &name, OriginalPublishedAt: entry.OriginalPublishedAt, PublishedAt: publishedAt,
		MediaJSON: media, Status: status, PublishMode: contentNewsPublishMode(policy), FetchedAt: &fetchedAt, ContentHash: &hash,
	}, nil
}

func contentNewsBodySourceType(entry ContentNewsFeedEntry) string {
	if entry.BodySourceType != "" {
		return entry.BodySourceType
	}
	return domain.ContentNewsBodySourceFeedContent
}

func contentNewsAutoPublishable(entry ContentNewsFeedEntry, cleanHTML, bodyText, summary string) bool {
	return entry.BodyComplete && strings.TrimSpace(cleanHTML) != "" && strings.TrimSpace(bodyText) != "" && strings.TrimSpace(summary) != ""
}

func contentNewsWithinPublishWindow(now time.Time, policy *ContentNewsWorkerPolicy) bool {
	if policy == nil || (policy.PublishStartAt == nil && policy.PublishEndAt == nil) {
		return true
	}
	location := time.UTC
	if policy.Timezone != "" {
		if loaded, err := time.LoadLocation(policy.Timezone); err == nil {
			location = loaded
		}
	}
	localNow := now.In(location)
	minutes := localNow.Hour()*60 + localNow.Minute()
	start, end := 0, 24*60
	if policy.PublishStartAt != nil {
		start = policy.PublishStartAt.Hour()*60 + policy.PublishStartAt.Minute()
	}
	if policy.PublishEndAt != nil {
		end = policy.PublishEndAt.Hour()*60 + policy.PublishEndAt.Minute()
	}
	if start <= end {
		return minutes >= start && minutes <= end
	}
	return minutes >= start || minutes <= end
}

func contentNewsPublishMode(policy *ContentNewsWorkerPolicy) string {
	if policy != nil && policy.Enabled && policy.PublishMode == domain.ContentAutomationAutoPublish {
		return domain.ContentNewsPublishAuto
	}
	return domain.ContentNewsPublishManual
}

func contentNewsLocaleAllowed(locale string, target []string) bool {
	if len(target) == 0 {
		return true
	}
	for _, item := range target {
		if strings.EqualFold(strings.TrimSpace(item), locale) {
			return true
		}
	}
	return false
}

func contentNewsHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func contentNewsSlug(title, hash string) string {
	var b strings.Builder
	previousDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if r <= unicode.MaxASCII {
				b.WriteRune(r)
				previousDash = false
			}
			continue
		}
		if b.Len() > 0 && !previousDash {
			b.WriteByte('-')
			previousDash = true
		}
	}
	base := strings.Trim(b.String(), "-")
	if base == "" {
		base = "news"
	}
	if len(base) > 64 {
		base = strings.TrimRight(base[:64], "-")
	}
	return base + "-" + hash[:12]
}

func contentNewsSummary(text string) string {
	value := []rune(strings.TrimSpace(text))
	if len(value) > 220 {
		return strings.TrimSpace(string(value[:220]))
	}
	return string(value)
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalStringIfNonBlank(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func stringPointer(value string) *string { return &value }
func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func parseContentNewsTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC850, time.RFC3339, "2006-01-02T15:04:05Z0700"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}

func localDayBounds(now time.Time, timezone ...string) (time.Time, time.Time) {
	location := now.Location()
	if len(timezone) > 0 && strings.TrimSpace(timezone[0]) != "" {
		if configured, err := time.LoadLocation(strings.TrimSpace(timezone[0])); err == nil {
			location = configured
		}
	}
	local := now.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	return start, start.AddDate(0, 0, 1)
}

func contentNewsSafeError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(err.Error()), "\n", " "), "\r", " ")
	if len(value) > maxContentNewsErrorSummary {
		value = value[:maxContentNewsErrorSummary]
	}
	return value
}

// SanitizeContentNewsHTML retains semantic prose only. Links lose their href,
// and all active or external-media elements are removed, so public pages never
// make the captured source content an outbound navigation or embed surface.
func SanitizeContentNewsHTML(raw string) (string, string) {
	doc, err := xhtml.Parse(strings.NewReader(raw))
	if err != nil {
		return "", ""
	}
	var rendered strings.Builder
	var plain strings.Builder
	var walk func(*xhtml.Node)
	allowed := map[string]bool{"p": true, "br": true, "h2": true, "h3": true, "h4": true, "blockquote": true, "ul": true, "ol": true, "li": true, "strong": true, "em": true, "b": true, "i": true, "code": true, "pre": true, "a": true, "table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true, "th": true, "td": true, "caption": true, "figure": true, "figcaption": true, "img": true, "video": true, "audio": true, "source": true}
	discard := map[string]bool{"script": true, "style": true, "iframe": true, "object": true, "embed": true, "svg": true, "canvas": true, "form": true, "input": true, "button": true, "noscript": true, "nav": true, "header": true, "footer": true, "aside": true}
	walk = func(node *xhtml.Node) {
		switch node.Type {
		case xhtml.TextNode:
			text := strings.TrimSpace(node.Data)
			if text != "" {
				rendered.WriteString(html.EscapeString(text))
				if plain.Len() > 0 {
					plain.WriteByte(' ')
				}
				plain.WriteString(text)
			}
			return
		case xhtml.ElementNode:
			name := strings.ToLower(node.Data)
			if discard[name] {
				return
			}
			if allowed[name] {
				if name == "br" {
					rendered.WriteString("<br>")
					plain.WriteByte(' ')
					return
				}
				// An anchor is retained as text formatting only; no source URL leaks.
				if name == "a" {
					rendered.WriteString("<span>")
				} else {
					rendered.WriteByte('<')
					rendered.WriteString(name)
					if name == "img" || name == "video" || name == "audio" || name == "source" {
						for _, attr := range node.Attr {
							if (attr.Key == "src" || attr.Key == "poster") && contentNewsSafeMediaURL(attr.Val) {
								rendered.WriteByte(' ')
								rendered.WriteString(attr.Key)
								rendered.WriteString("=\"")
								rendered.WriteString(html.EscapeString(attr.Val))
								rendered.WriteByte('"')
							}
						}
					}
					rendered.WriteByte('>')
				}
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					walk(child)
				}
				if name == "a" {
					rendered.WriteString("</span>")
				} else {
					rendered.WriteString("</")
					rendered.WriteString(name)
					rendered.WriteByte('>')
				}
				if name == "p" || name == "li" || strings.HasPrefix(name, "h") || name == "blockquote" {
					plain.WriteByte(' ')
				}
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return strings.TrimSpace(rendered.String()), strings.Join(strings.Fields(plain.String()), " ")
}

func contentNewsSafeMediaURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "metadata.google.internal" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
	}
	return true
}

// ExtractContentNewsArticleHTML keeps the most likely article container before
// applying the common sanitizer. When publishers omit semantic containers, the
// document body is used as a conservative fallback.
func ExtractContentNewsArticleHTML(raw []byte) (string, string) {
	doc, err := xhtml.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return "", ""
	}
	var candidate *xhtml.Node
	var find func(*xhtml.Node)
	find = func(node *xhtml.Node) {
		if candidate != nil {
			return
		}
		if node.Type == xhtml.ElementNode && (strings.EqualFold(node.Data, "article") || strings.EqualFold(node.Data, "main")) {
			candidate = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(doc)
	if candidate == nil {
		var body *xhtml.Node
		var findBody func(*xhtml.Node)
		findBody = func(node *xhtml.Node) {
			if body != nil {
				return
			}
			if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, "body") {
				body = node
				return
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				findBody(child)
			}
		}
		findBody(doc)
		candidate = body
	}
	if candidate == nil {
		return "", ""
	}
	var rawHTML strings.Builder
	for child := candidate.FirstChild; child != nil; child = child.NextSibling {
		if err := xhtml.Render(&rawHTML, child); err != nil {
			return "", ""
		}
	}
	return SanitizeContentNewsHTML(rawHTML.String())
}

func textFromContentNewsHTML(raw string) string {
	_, text := SanitizeContentNewsHTML(raw)
	return text
}

type ContentNewsHostResolver func(context.Context, string) ([]net.IP, error)

// SafeContentNewsHTTPFetcher validates both the initial URL and redirects before
// any fetch, and a default transport resolves again at dial time to limit DNS
// rebinding. Custom transports are intended only for controlled tests.
type SafeContentNewsHTTPFetcher struct {
	client   *http.Client
	resolver ContentNewsHostResolver
	maxBytes int64
}

func NewSafeContentNewsHTTPFetcher(client *http.Client, resolver ContentNewsHostResolver) *SafeContentNewsHTTPFetcher {
	if resolver == nil {
		resolver = func(ctx context.Context, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip", host)
		}
	}
	fetcher := &SafeContentNewsHTTPFetcher{resolver: resolver, maxBytes: defaultContentNewsMaxBodyBytes}
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.DialContext = fetcher.safeDialContext()
		client = &http.Client{Transport: transport, Timeout: defaultContentNewsFetchTimeout}
	}
	baseRedirect := client.CheckRedirect
	clone := *client
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("content news redirect limit exceeded")
		}
		if err := ValidateContentNewsFetchURL(req.Context(), req.URL.String(), resolver); err != nil {
			return err
		}
		if baseRedirect != nil {
			return baseRedirect(req, via)
		}
		return nil
	}
	fetcher.client = &clone
	return fetcher
}

func (f *SafeContentNewsHTTPFetcher) Fetch(ctx context.Context, rawURL string) ([]byte, string, error) {
	if err := ValidateContentNewsFetchURL(ctx, rawURL, f.resolver); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrContentNewsFetchURLInvalid, err)
	}
	req.Header.Set("Accept", "application/atom+xml, application/rss+xml, application/xml, text/xml;q=0.9")
	req.Header.Set("User-Agent", "CCAI-News-Fetcher/1.0")
	for attempt := 0; attempt < 3; attempt++ {
		response, err := f.client.Do(req)
		if err != nil {
			if attempt < 2 && ctx.Err() == nil {
				if !waitContentNewsRetry(ctx, attempt, "") {
					return nil, "", ctx.Err()
				}
				continue
			}
			return nil, "", err
		}
		retry := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			response.Body.Close()
			if retry && attempt < 2 {
				if !waitContentNewsRetry(ctx, attempt, response.Header.Get("Retry-After")) {
					return nil, "", ctx.Err()
				}
				continue
			}
			return nil, "", fmt.Errorf("content news fetch returned HTTP %d", response.StatusCode)
		}
		if response.ContentLength > f.maxBytes {
			response.Body.Close()
			return nil, "", fmt.Errorf("content news feed exceeds maximum size")
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, f.maxBytes+1))
		response.Body.Close()
		if readErr != nil {
			if attempt < 2 && ctx.Err() == nil {
				if !waitContentNewsRetry(ctx, attempt, "") {
					return nil, "", ctx.Err()
				}
				continue
			}
			return nil, "", readErr
		}
		if int64(len(body)) > f.maxBytes {
			return nil, "", fmt.Errorf("content news feed exceeds maximum size")
		}
		finalURL := req.URL.String()
		if response.Request != nil && response.Request.URL != nil {
			finalURL = response.Request.URL.String()
		}
		return body, finalURL, nil
	}
	return nil, "", errors.New("content news fetch retry exhausted")
}

func waitContentNewsRetry(ctx context.Context, attempt int, retryAfter string) bool {
	delay := time.Duration(100*(1<<attempt)) * time.Millisecond
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
		candidate := time.Duration(seconds) * time.Second
		if candidate < 5*time.Second {
			delay = candidate
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (f *SafeContentNewsHTTPFetcher) FetchArticle(ctx context.Context, rawURL string) ([]byte, string, error) {
	return f.Fetch(ctx, rawURL)
}

func (f *SafeContentNewsHTTPFetcher) safeDialContext() func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := safeContentNewsHostIPs(ctx, host, f.resolver)
		if err != nil {
			return nil, err
		}
		// A concrete validated IP prevents a second resolver lookup inside Dial.
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
}

func ValidateContentNewsFetchURL(ctx context.Context, rawURL string, resolver ContentNewsHostResolver) error {
	if strings.ContainsAny(rawURL, "\r\n") {
		return ErrContentNewsFetchURLInvalid
	}
	u, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil || u == nil || u.User != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ErrContentNewsFetchURLInvalid
	}
	if u.Port() != "" {
		if port, portErr := strconv.Atoi(u.Port()); portErr != nil || port < 1 || port > 65535 {
			return ErrContentNewsFetchURLInvalid
		}
	}
	if resolver == nil {
		resolver = func(ctx context.Context, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip", host)
		}
	}
	_, err = safeContentNewsHostIPs(ctx, u.Hostname(), resolver)
	return err
}

func safeContentNewsHostIPs(ctx context.Context, host string, resolver ContentNewsHostResolver) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicContentNewsIP(ip) {
			return nil, ErrContentNewsFetchURLBlocked
		}
		return []net.IP{ip}, nil
	}
	ips, err := resolver(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("%w: unable to resolve host", ErrContentNewsFetchURLInvalid)
	}
	valid := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if !isPublicContentNewsIP(ip) {
			return nil, ErrContentNewsFetchURLBlocked
		}
		valid = append(valid, ip)
	}
	return valid, nil
}

func isPublicContentNewsIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		// Carrier-grade NAT is not private according to net.IP.IsPrivate.
		return !(ipv4[0] == 100 && ipv4[1] >= 64 && ipv4[1] <= 127)
	}
	return true
}
