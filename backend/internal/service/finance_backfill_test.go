//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type financeBackfillRepositoryStub struct {
	candidates   []FinanceBackfillCandidate
	job          *FinanceBackfillJob
	created      int
	updates      int
	failures     int
	completed    int
	getCalls     int
	pauseAtGet   int
	acknowledged int
}

func (r *financeBackfillRepositoryStub) CountFinanceBackfillCandidates(context.Context, FinanceBackfillRequest) (int64, error) {
	return int64(len(r.candidates)), nil
}
func (r *financeBackfillRepositoryStub) ListFinanceBackfillCandidates(_ context.Context, _ FinanceBackfillRequest, cursor FinanceBackfillCursor, limit int) ([]FinanceBackfillCandidate, error) {
	result := make([]FinanceBackfillCandidate, 0, limit)
	for _, candidate := range r.candidates {
		if candidate.UsageLog.CreatedAt.Before(cursor.CreatedAt) || (candidate.UsageLog.CreatedAt.Equal(cursor.CreatedAt) && candidate.UsageLog.ID <= cursor.ID) {
			continue
		}
		result = append(result, candidate)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}
func (r *financeBackfillRepositoryStub) CreateFinanceBackfillJob(_ context.Context, request FinanceBackfillRequest, operatorID int64, _, _ string, expiresAt time.Time, total int64) (*FinanceBackfillJob, error) {
	r.created++
	r.job = &FinanceBackfillJob{ID: 44, Status: "queued", StartDate: request.StartDate, EndDate: request.EndDate, Scope: request.Scope, PricingPolicy: request.PricingPolicy, Reason: request.Reason, OperatorID: operatorID, EstimatedTotal: total, PreviewExpiresAt: expiresAt}
	return r.job, nil
}
func (r *financeBackfillRepositoryStub) GetFinanceBackfillJob(context.Context, int64) (*FinanceBackfillJob, error) {
	r.getCalls++
	if r.pauseAtGet > 0 && r.getCalls == r.pauseAtGet {
		r.job.Status = "paused"
	}
	return r.job, nil
}
func (r *financeBackfillRepositoryStub) PauseFinanceBackfillJob(context.Context, int64) (*FinanceBackfillJob, error) {
	r.job.Status = "paused"
	return r.job, nil
}
func (r *financeBackfillRepositoryStub) ResumeFinanceBackfillJob(context.Context, int64) (*FinanceBackfillJob, error) {
	r.job.Status = "queued"
	return r.job, nil
}
func (r *financeBackfillRepositoryStub) ClaimFinanceBackfillJob(context.Context, string, time.Time) (*FinanceBackfillJob, error) {
	if r.job == nil || r.job.Status != "queued" {
		return nil, nil
	}
	r.job.Status = "running"
	return r.job, nil
}
func (r *financeBackfillRepositoryStub) RenewFinanceBackfillLease(context.Context, int64, string, time.Time) error {
	return nil
}
func (r *financeBackfillRepositoryStub) AcknowledgeFinanceBackfillPause(context.Context, int64, string, time.Time) error {
	r.acknowledged++
	return nil
}
func (r *financeBackfillRepositoryStub) UpdateFinanceBackfillProgress(_ context.Context, _ int64, _ string, cursor FinanceBackfillCursor, processed, succeeded int64, progress decimal.Decimal, _ time.Time) error {
	r.updates++
	r.job.Cursor = cursor
	r.job.ProcessedCount += processed
	r.job.SuccessCount += succeeded
	r.job.Progress = progress
	return nil
}
func (r *financeBackfillRepositoryStub) ReleaseFinanceBackfillJob(context.Context, int64, string, time.Time) error {
	r.job.Status = "queued"
	return nil
}
func (r *financeBackfillRepositoryStub) CompleteFinanceBackfillJob(context.Context, int64, string, time.Time) error {
	r.completed++
	r.job.Status = "completed"
	return nil
}
func (r *financeBackfillRepositoryStub) FailFinanceBackfillJob(_ context.Context, _ int64, _ string, message string, _ time.Time) error {
	r.failures++
	r.job.Status = "failed"
	r.job.ErrorSummary = &message
	return nil
}

type financeBackfillLedgerStub struct {
	attempts  map[int64][]UsageUpstreamAttempt
	revisions int
	creates   int
	reviseErr error
	last      *UsageFinanceProjection
	metadata  FinanceRevisionMetadata
}

func (r *financeBackfillLedgerStub) FinanceLaunchAt(context.Context) (time.Time, error) {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), nil
}
func (r *financeBackfillLedgerStub) TryAcquireScannerLease(context.Context) (func(), bool, error) {
	return func() {}, true, nil
}
func (r *financeBackfillLedgerStub) ListPendingUsage(context.Context, FinanceUsageCursor, int) ([]UsageLog, error) {
	return nil, nil
}
func (r *financeBackfillLedgerStub) LoadUsageAttempts(_ context.Context, ids []int64) (map[int64][]UsageUpstreamAttempt, error) {
	result := make(map[int64][]UsageUpstreamAttempt, len(ids))
	for _, id := range ids {
		result[id] = CloneUsageUpstreamAttempts(r.attempts[id])
	}
	return result, nil
}
func (r *financeBackfillLedgerStub) CreateFinanceProjection(_ context.Context, projection *UsageFinanceProjection) (bool, error) {
	r.creates++
	r.last = projection
	return true, nil
}
func (r *financeBackfillLedgerStub) ReviseFinanceProjection(_ context.Context, projection *UsageFinanceProjection, metadata FinanceRevisionMetadata) (bool, error) {
	if r.reviseErr != nil {
		return false, r.reviseErr
	}
	r.revisions++
	r.last = projection
	r.metadata = metadata
	return true, nil
}
func (r *financeBackfillLedgerStub) RecordFinanceProjectionFailure(context.Context, int64, string, time.Time) error {
	return nil
}
func (r *financeBackfillLedgerStub) ResolveFinanceProjectionFailure(context.Context, int64, time.Time) error {
	return nil
}

type financeBackfillPriceRepo struct{}

func (financeBackfillPriceRepo) FindAccountFinanceProfileByID(context.Context, int64) (*AccountFinanceProfile, error) {
	multiplier := decimal.RequireFromString("0.5")
	return &AccountFinanceProfile{ID: 7, AccountID: 3, WalletID: int64Snapshot(5), CostMode: FinanceCostModeContractMultiplier, ContractMultiplier: &multiplier}, nil
}
func (financeBackfillPriceRepo) FindWalletByID(context.Context, int64) (*FinanceWalletAssignment, error) {
	return &FinanceWalletAssignment{WalletID: 5, UpstreamID: 6, Currency: "USD"}, nil
}
func (financeBackfillPriceRepo) FindUpstreamPriceAt(context.Context, int64, string, string, string, time.Time) (*FinancePriceQuote, error) {
	return nil, nil
}
func (financeBackfillPriceRepo) FindSystemPriceAt(context.Context, string, string, time.Time) (*FinancePriceQuote, error) {
	input := decimal.NewFromInt(2)
	return &FinancePriceQuote{VersionID: 9, Source: FinancePricingSourceSystem, BillingMode: "token", Currency: "USD", USDExchangeRate: decimal.NewFromInt(1), Detail: FinancePriceDetail{Standard: FinanceRateCard{Input: &input}}}, nil
}

func newFinanceBackfillTestService(repo *financeBackfillRepositoryStub, ledger *financeBackfillLedgerStub, now time.Time) *FinanceBackfillService {
	scanner := NewFinanceUsageScanner(ledger, NewFinancePriceSelector(financeBackfillPriceRepo{}), NewFinanceCostCalculator())
	service := NewFinanceBackfillService(repo, ledger, scanner, nil)
	service.tokenKey = []byte("01234567890123456789012345678901")
	service.now = func() time.Time { return now }
	return service
}

func financeBackfillTestRequest() FinanceBackfillRequest {
	return FinanceBackfillRequest{StartDate: "2026-07-01", EndDate: "2026-07-02", PricingPolicy: FinanceBackfillPricingHistorical, DryRunSampleSize: 100, Reason: "历史价格补齐"}
}

func financeBackfillTestCandidate() (FinanceBackfillCandidate, UsageUpstreamAttempt) {
	createdAt := time.Date(2026, 7, 1, 3, 0, 0, 0, time.UTC)
	multiplier := decimal.RequireFromString("0.5")
	profileID := int64(7)
	log := UsageLog{ID: 11, UserID: 2, AccountID: 3, Model: "gpt-test", RequestedModel: "gpt-test", BillingMode: stringSnapshot("token"), ActualCost: 3, AccountFinanceProfileID: &profileID, CreatedAt: createdAt}
	attempt := UsageUpstreamAttempt{UsageLogID: 11, AttemptNo: 1, AccountID: 3, UpstreamModel: "gpt-test", InputTokens: 1_000_000, RequestCount: 1, UpstreamCostMultiplier: &multiplier, AccountFinanceProfileID: &profileID, Billable: true, CreatedAt: createdAt}
	return FinanceBackfillCandidate{UsageLog: log, HasProjection: true}, attempt
}

func TestFinanceBackfillPreviewIsReadOnlyAndTokenBindsConditions(t *testing.T) {
	candidate, attempt := financeBackfillTestCandidate()
	repo := &financeBackfillRepositoryStub{candidates: []FinanceBackfillCandidate{candidate}}
	ledger := &financeBackfillLedgerStub{attempts: map[int64][]UsageUpstreamAttempt{11: {attempt}}}
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	service := newFinanceBackfillTestService(repo, ledger, now)

	request := financeBackfillTestRequest()
	preview, err := service.Preview(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, int64(1), preview.EstimatedRecords)
	require.Zero(t, preview.ExactRepairable)
	require.Equal(t, int64(1), preview.EstimatedOnly)
	require.Equal(t, now.Add(30*time.Minute), preview.ExpiresAt)
	require.Zero(t, ledger.creates)
	require.Zero(t, ledger.revisions)

	request.PreviewToken = preview.PreviewToken
	request.EndDate = "2026-07-03"
	_, err = service.Run(context.Background(), request, 7)
	require.True(t, IsFinanceBackfillError(err, "BACKFILL_PRECONDITION_FAILED"))
	require.Zero(t, repo.created)
}

func TestFinanceBackfillPreviewTokenExpiresAfterThirtyMinutes(t *testing.T) {
	candidate, attempt := financeBackfillTestCandidate()
	repo := &financeBackfillRepositoryStub{candidates: []FinanceBackfillCandidate{candidate}}
	ledger := &financeBackfillLedgerStub{attempts: map[int64][]UsageUpstreamAttempt{11: {attempt}}}
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	service := newFinanceBackfillTestService(repo, ledger, now)
	request := financeBackfillTestRequest()
	preview, err := service.Preview(context.Background(), request)
	require.NoError(t, err)
	request.PreviewToken = preview.PreviewToken
	service.now = func() time.Time { return now.Add(30 * time.Minute) }
	_, err = service.Run(context.Background(), request, 7)
	require.True(t, IsFinanceBackfillError(err, "BACKFILL_PRECONDITION_FAILED"))
}

func TestFinanceBackfillRunNextBatchUsesHistoricalSnapshotAndRevision(t *testing.T) {
	candidate, attempt := financeBackfillTestCandidate()
	repo := &financeBackfillRepositoryStub{candidates: []FinanceBackfillCandidate{candidate}}
	ledger := &financeBackfillLedgerStub{attempts: map[int64][]UsageUpstreamAttempt{11: {attempt}}}
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	service := newFinanceBackfillTestService(repo, ledger, now)
	request := financeBackfillTestRequest()
	preview, err := service.Preview(context.Background(), request)
	require.NoError(t, err)
	request.PreviewToken = preview.PreviewToken
	job, err := service.Run(context.Background(), request, 7)
	require.NoError(t, err)

	require.NoError(t, service.RunNextBatch(context.Background()))
	require.Equal(t, 1, ledger.revisions)
	require.Zero(t, ledger.creates)
	require.NotNil(t, ledger.last.UpstreamCost)
	require.Equal(t, "1", ledger.last.UpstreamCost.String())
	require.Equal(t, FinanceCostStatusEstimated, ledger.last.CostStatus)
	require.Equal(t, FinancePricingSourceSystem, ledger.last.PricingSource)
	require.Equal(t, job.ID, *ledger.metadata.JobID)
	require.Equal(t, int64(7), *ledger.metadata.OperatorID)
	require.Equal(t, "历史价格补齐", ledger.metadata.Reason)
	require.Equal(t, "completed", repo.job.Status)
	require.Equal(t, int64(1), repo.job.ProcessedCount)
}

func TestFinanceBackfillFailedBatchKeepsCursorForResume(t *testing.T) {
	candidate, attempt := financeBackfillTestCandidate()
	repo := &financeBackfillRepositoryStub{candidates: []FinanceBackfillCandidate{candidate}}
	ledger := &financeBackfillLedgerStub{attempts: map[int64][]UsageUpstreamAttempt{11: {attempt}}, reviseErr: errors.New("temporary database failure")}
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	service := newFinanceBackfillTestService(repo, ledger, now)
	request := financeBackfillTestRequest()
	preview, err := service.Preview(context.Background(), request)
	require.NoError(t, err)
	request.PreviewToken = preview.PreviewToken
	job, err := service.Run(context.Background(), request, 7)
	require.NoError(t, err)

	require.Error(t, service.RunNextBatch(context.Background()))
	require.Equal(t, "failed", repo.job.Status)
	require.Zero(t, repo.job.Cursor.ID)
	require.Zero(t, repo.updates)

	ledger.reviseErr = nil
	_, err = service.Resume(context.Background(), job.ID)
	require.NoError(t, err)
	require.NoError(t, service.RunNextBatch(context.Background()))
	require.Equal(t, int64(11), repo.job.Cursor.ID)
	require.Equal(t, "completed", repo.job.Status)
}

func TestFinanceBackfillWorkerAcknowledgesPauseAfterLastWriteBeforeProgress(t *testing.T) {
	candidate, attempt := financeBackfillTestCandidate()
	repo := &financeBackfillRepositoryStub{
		candidates: []FinanceBackfillCandidate{candidate},
		pauseAtGet: 3,
	}
	ledger := &financeBackfillLedgerStub{attempts: map[int64][]UsageUpstreamAttempt{11: {attempt}}}
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	service := newFinanceBackfillTestService(repo, ledger, now)
	request := financeBackfillTestRequest()
	preview, err := service.Preview(context.Background(), request)
	require.NoError(t, err)
	request.PreviewToken = preview.PreviewToken
	_, err = service.Run(context.Background(), request, 7)
	require.NoError(t, err)

	require.NoError(t, service.RunNextBatch(context.Background()))
	require.Equal(t, "paused", repo.job.Status)
	require.Equal(t, 1, repo.acknowledged)
	require.Equal(t, 1, ledger.revisions)
	require.Zero(t, repo.updates)
	require.Zero(t, repo.completed)
}
