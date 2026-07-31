package service

import (
	"context"
	"encoding/csv"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type financeExportRepoStub struct {
	claimed            *FinanceExportJob
	progressRows       []int64
	completed          *FinanceExportJob
	failed             string
	renewals           int
	completeErrByOwner map[string]error
	completedPaths     []string
}

func (s *financeExportRepoStub) CreateFinanceExportJob(context.Context, FinanceExportRequest, int64, string, string) (*FinanceExportJob, error) {
	return s.claimed, nil
}
func (s *financeExportRepoStub) GetFinanceExportJob(context.Context, int64, int64) (*FinanceExportJob, error) {
	return s.claimed, nil
}
func (s *financeExportRepoStub) ClaimFinanceExportJob(context.Context, string, time.Time) (*FinanceExportJob, error) {
	job := s.claimed
	s.claimed = nil
	return job, nil
}
func (s *financeExportRepoStub) RenewFinanceExportLease(context.Context, int64, string, time.Time) error {
	s.renewals++
	return nil
}
func (s *financeExportRepoStub) UpdateFinanceExportProgress(_ context.Context, _ int64, _ string, processed int64, _ decimal.Decimal, _ time.Time) error {
	s.progressRows = append(s.progressRows, processed)
	return nil
}
func (s *financeExportRepoStub) CompleteFinanceExportJob(_ context.Context, jobID int64, owner, storageKey string, fileSize, rowCount int64, expiresAt, _ time.Time) error {
	s.completedPaths = append(s.completedPaths, storageKey)
	if err := s.completeErrByOwner[owner]; err != nil {
		return err
	}
	s.completed = &FinanceExportJob{ID: jobID, StorageKey: storageKey, FileSize: &fileSize, RowCount: &rowCount, ExpiresAt: &expiresAt}
	return nil
}

func TestFinanceExportLeaseTakeoverKeepsOnlyWinningWorkerFile(t *testing.T) {
	reportRepo := &financeReportRepoStub{breakdownFn: func(FinanceBreakdownRequest) ([]FinanceBreakdownFact, int64, error) { return nil, 0, nil }}
	repo := &financeExportRepoStub{completeErrByOwner: map[string]error{"worker-a": &FinanceExportError{Code: "JOB_LEASE_LOST", Message: "lease lost"}}}
	job := &FinanceExportJob{ID: 88, Request: FinanceExportRequest{Report: "breakdown", Format: "csv", Timezone: "UTC", Filters: FinanceExportFilters{StartDate: "2026-07-01", EndDate: "2026-07-01", Dimension: "requested_model", DataScope: "all"}}}
	dir := t.TempDir()
	first := NewFinanceExportService(repo, NewFinanceReportService(reportRepo), nil)
	first.exportDir, first.leaseOwner = dir, "worker-a"
	require.Error(t, first.writeBreakdown(context.Background(), job))
	require.Len(t, repo.completedPaths, 1)
	_, err := os.Stat(repo.completedPaths[0])
	require.True(t, os.IsNotExist(err), "losing worker must delete only its own temporary file")

	second := NewFinanceExportService(repo, NewFinanceReportService(reportRepo), nil)
	second.exportDir, second.leaseOwner = dir, "worker-b"
	require.NoError(t, second.writeBreakdown(context.Background(), job))
	require.Len(t, repo.completedPaths, 2)
	require.NotEqual(t, repo.completedPaths[0], repo.completedPaths[1])
	_, err = os.Stat(repo.completed.StorageKey)
	require.NoError(t, err, "winner storage key remains available")
}
func (s *financeExportRepoStub) FailFinanceExportJob(_ context.Context, _ int64, _, message string, _ time.Time) error {
	s.failed = message
	return nil
}
func (s *financeExportRepoStub) ReleaseFinanceExportJob(context.Context, int64, string, time.Time) error {
	return nil
}
func (s *financeExportRepoStub) SetFinanceExportDownloadToken(context.Context, int64, int64, string, time.Time) error {
	return nil
}
func (s *financeExportRepoStub) ConsumeFinanceExportDownloadToken(context.Context, int64, int64, string, time.Time) (*FinanceExportJob, error) {
	return s.completed, nil
}

func TestFinanceExportStreamsBreakdownInBoundedPagesAndEscapesFormulaCells(t *testing.T) {
	facts := make([]FinanceBreakdownFact, 1001)
	for index := range facts {
		facts[index] = FinanceBreakdownFact{
			DimensionKey: "key", DimensionName: "normal", Revenue: decimal.NewFromInt(10), CoveredRevenue: decimal.NewFromInt(10),
			UpstreamCost: decimal.NewFromInt(6), InputCost: decimal.NewFromInt(1), OutputCost: decimal.NewFromInt(2),
			CacheCost: decimal.NewFromInt(1), FastCost: decimal.NewFromInt(1), OtherCost: decimal.NewFromInt(1), RequestCount: 1, ExactCount: 1,
		}
	}
	facts[0].DimensionName = "=HYPERLINK(\"https://example.test\")"
	pageCalls := make([]FinanceBreakdownRequest, 0, 2)
	reportRepo := &financeReportRepoStub{breakdownFn: func(request FinanceBreakdownRequest) ([]FinanceBreakdownFact, int64, error) {
		pageCalls = append(pageCalls, request)
		start := (request.Page - 1) * request.PageSize
		if start >= len(facts) {
			return nil, int64(len(facts)), nil
		}
		end := start + request.PageSize
		if end > len(facts) {
			end = len(facts)
		}
		return facts[start:end], int64(len(facts)), nil
	}}
	request := FinanceExportRequest{
		Report: "breakdown", Format: "csv", Timezone: "UTC",
		Filters: FinanceExportFilters{StartDate: "2026-07-01", EndDate: "2026-07-31", Dimension: "requested_model", DataScope: "all", SortBy: "profit", SortOrder: "asc"},
	}
	repo := &financeExportRepoStub{claimed: &FinanceExportJob{ID: 42, Request: request, Report: "breakdown", Format: "csv", Status: "queued"}}
	svc := NewFinanceExportService(repo, NewFinanceReportService(reportRepo), nil)
	svc.exportDir = t.TempDir()
	fixedNow := time.Date(2026, 7, 28, 1, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	require.NoError(t, svc.RunNext(context.Background()))
	require.Empty(t, repo.failed)
	require.Equal(t, []int64{200, 200, 200, 200, 200, 1}, repo.progressRows)
	require.Len(t, pageCalls, 6)
	require.Equal(t, financeExportPageSize, pageCalls[0].PageSize)
	require.NotNil(t, repo.completed)
	require.GreaterOrEqual(t, repo.renewals, 1)
	require.Equal(t, int64(1001), *repo.completed.RowCount)

	content, err := os.ReadFile(repo.completed.StorageKey)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(content), "\ufeff"))
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(content), "\ufeff")))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	require.NoError(t, err)
	require.Equal(t, "# generated_at=2026-07-28T01:00:00Z", records[0][0])
	require.Equal(t, "dimension_key", records[3][0])
	require.Equal(t, "'=HYPERLINK(\"https://example.test\")", records[4][1])
	require.Len(t, records, 1005)
}

func TestNormalizeFinanceExportRequestUsesReportTimeRangeAndWhitelist(t *testing.T) {
	request, filter, err := normalizeFinanceExportRequest(FinanceExportRequest{
		Report: " BREAKDOWN ", Timezone: "Asia/Shanghai",
		Filters: FinanceExportFilters{StartDate: "2026-07-01", EndDate: "2026-07-31", Dimension: "account", DataScope: "exact_only"},
	})
	require.NoError(t, err)
	require.Equal(t, "csv", request.Format)
	require.Equal(t, "profit", request.Filters.SortBy)
	require.Equal(t, time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC), filter.StartAt)
	require.Equal(t, time.Date(2026, 7, 31, 16, 0, 0, 0, time.UTC), filter.EndBefore)

	_, _, err = normalizeFinanceExportRequest(FinanceExportRequest{Report: "details", Format: "csv"})
	require.ErrorContains(t, err, "report must be breakdown")
	_, _, err = normalizeFinanceExportRequest(FinanceExportRequest{
		Report: "breakdown", Format: "csv", Timezone: "UTC",
		Filters: FinanceExportFilters{StartDate: "2026-07-01", EndDate: "2026-07-31", Dimension: "password", DataScope: "all"},
	})
	require.ErrorContains(t, err, "dimension is invalid")
}

func TestSafeFinanceCSVTextBlocksSpreadsheetFormulaPrefixes(t *testing.T) {
	for _, value := range []string{"=1+1", "+SUM(A1:A2)", "-2+3", "@cmd", "\tformula", "\rformula", "  =1+1"} {
		require.True(t, strings.HasPrefix(safeFinanceCSVText(value), "'"), value)
	}
	require.Equal(t, "model-1", safeFinanceCSVText("model-1"))
}
