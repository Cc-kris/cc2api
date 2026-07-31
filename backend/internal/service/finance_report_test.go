package service

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type financeReportRepoStub struct {
	summaries   []*FinanceSummaryFacts
	filters     []FinanceReportFilter
	trend       []FinanceTrendFact
	breakdown   []FinanceBreakdownFact
	breakdownFn func(FinanceBreakdownRequest) ([]FinanceBreakdownFact, int64, error)
	details     []FinanceDetailFact
	funds       *FinanceFundsFacts
	issues      []FinanceQualityIssueFact
	cashFlow    *FinanceCashFlowFacts
	cashReq     FinanceCashFlowRequest
}

func (s *financeReportRepoStub) SummarizeFinance(_ context.Context, filter FinanceReportFilter) (*FinanceSummaryFacts, error) {
	s.filters = append(s.filters, filter)
	result := s.summaries[0]
	s.summaries = s.summaries[1:]
	return result, nil
}
func (s *financeReportRepoStub) ListFinanceTrend(context.Context, FinanceReportFilter) ([]FinanceTrendFact, error) {
	return s.trend, nil
}
func (s *financeReportRepoStub) ListFinanceBreakdown(_ context.Context, _ FinanceReportFilter, request FinanceBreakdownRequest) ([]FinanceBreakdownFact, int64, error) {
	if s.breakdownFn != nil {
		return s.breakdownFn(request)
	}
	return s.breakdown, int64(len(s.breakdown)), nil
}
func (s *financeReportRepoStub) ListFinanceDetails(context.Context, FinanceReportFilter, FinanceDetailsRequest) ([]FinanceDetailFact, int64, error) {
	return s.details, int64(len(s.details)), nil
}
func (s *financeReportRepoStub) GetFinanceFunds(context.Context, FinanceReportFilter) (*FinanceFundsFacts, error) {
	return s.funds, nil
}
func (s *financeReportRepoStub) ListFinanceQualityIssues(context.Context, FinanceReportFilter, string, int, int) ([]FinanceQualityIssueFact, int64, error) {
	return s.issues, int64(len(s.issues)), nil
}
func (s *financeReportRepoStub) GetFinanceCashFlow(_ context.Context, _ FinanceReportFilter, request FinanceCashFlowRequest) (*FinanceCashFlowFacts, error) {
	s.cashReq = request
	if s.cashFlow == nil {
		return &FinanceCashFlowFacts{}, nil
	}
	return s.cashFlow, nil
}

func TestFinanceReportTrendFillsMissingBucketsWithoutForgingMargin(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, location).UTC()
	repo := &financeReportRepoStub{trend: []FinanceTrendFact{{
		BucketStart: start, Revenue: decimal.NewFromInt(10), CoveredRevenue: decimal.NewFromInt(8), UpstreamCost: decimal.NewFromInt(6), PaymentFees: decimal.NewFromInt(1), RechargeBonusIncome: decimal.NewFromInt(3), ExactCount: 1, RequestCount: 1,
	}}}
	svc := NewFinanceReportService(repo)
	items, err := svc.Trend(context.Background(), FinanceReportFilter{
		StartAt: start, EndBefore: time.Date(2026, 7, 3, 0, 0, 0, 0, location).UTC(), Location: location, Granularity: "day",
	})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "1.00000000", items[0].Profit)
	require.Equal(t, "4.00000000", items[0].CombinedProfit)
	require.Equal(t, "0.1250", *items[0].MarginRate)
	require.Equal(t, "0.00000000", items[1].Revenue)
	require.Nil(t, items[1].MarginRate)
}

func TestFinanceReportBreakdownRejectsUnlistedDimension(t *testing.T) {
	svc := NewFinanceReportService(&financeReportRepoStub{})
	_, _, err := svc.Breakdown(context.Background(), FinanceReportFilter{}, FinanceBreakdownRequest{Dimension: "1;drop table"})
	require.EqualError(t, err, "dimension is invalid")
}

func TestFinanceReportDetailsKeepsProfitNullWhenCostMissing(t *testing.T) {
	repo := &financeReportRepoStub{details: []FinanceDetailFact{{
		UsageLogID: 1, RequestID: "req", Revenue: decimal.NewFromInt(2), CostStatus: string(FinanceCostStatusMissingPrice),
	}}}
	items, _, err := NewFinanceReportService(repo).Details(context.Background(), FinanceReportFilter{}, FinanceDetailsRequest{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Nil(t, items[0].UpstreamCost)
	require.Nil(t, items[0].Profit)
	require.Nil(t, items[0].MarginRate)
}

func TestFinanceReportLossClassifierPrioritizesMultiAttemptAndFast(t *testing.T) {
	cost := decimal.NewFromInt(3)
	repo := &financeReportRepoStub{details: []FinanceDetailFact{
		{UsageLogID: 1, Revenue: decimal.NewFromInt(1), UpstreamCost: &cost, SegmentCount: 2},
		{UsageLogID: 2, Revenue: decimal.NewFromInt(1), UpstreamCost: &cost, ServiceTier: "fast"},
	}}
	items, _, err := NewFinanceReportService(repo).Losses(context.Background(), FinanceReportFilter{}, FinanceDetailsRequest{}, "")
	require.NoError(t, err)
	require.Equal(t, "multi_attempt_cost", items[0].LossReason)
	require.Equal(t, "fast_cost_not_covered", items[1].LossReason)
}

func TestParseFinanceReportFilterBuildsInclusiveLocalDateRange(t *testing.T) {
	filter, err := ParseFinanceReportFilter(url.Values{
		"start_date": {"2026-07-01"}, "end_date": {"2026-07-25"}, "timezone": {"Asia/Shanghai"},
		"cost_status": {"exact,missing_price,exact"}, "account_id": {"9"},
	})
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC), filter.StartAt)
	require.Equal(t, time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC), filter.EndBefore)
	require.Equal(t, []string{"exact", "missing_price"}, filter.CostStatuses)
	require.Equal(t, int64(9), *filter.AccountID)
	require.Equal(t, "exact_only", filter.DataScope)
}

func TestParseFinanceReportFilterRejectsInvalidInputs(t *testing.T) {
	_, err := ParseFinanceReportFilter(url.Values{"start_date": {"2026-07-25"}, "end_date": {"2026-07-01"}})
	require.EqualError(t, err, "end_date must not be earlier than start_date")
	_, err = ParseFinanceReportFilter(url.Values{"start_date": {"2026-07-01"}, "end_date": {"2026-07-25"}, "data_scope": {"unsafe"}})
	require.EqualError(t, err, "data_scope must be all or exact_only")
}

func TestFinanceReportOverviewUsesPreviousPeriodAndExposesPartialQuality(t *testing.T) {
	repo := &financeReportRepoStub{summaries: []*FinanceSummaryFacts{
		{Revenue: decimal.NewFromInt(100), CoveredRevenue: decimal.NewFromInt(90), UpstreamCost: decimal.NewFromInt(75), PaymentFees: decimal.NewFromInt(3), RechargeBonusIncome: decimal.NewFromInt(5), LossAmount: decimal.NewFromInt(5), LossRequestCount: 2, ExactCount: 8, EstimatedCount: 1, MissingPriceCount: 1, UnpricedRevenue: decimal.NewFromInt(10), EstimatedCost: decimal.NewFromInt(5), PendingSettlementCost: decimal.NewFromInt(5), UnconfirmedExactCost: decimal.NewFromInt(2)},
		{Revenue: decimal.NewFromInt(80), CoveredRevenue: decimal.NewFromInt(80), UpstreamCost: decimal.NewFromInt(60), PaymentFees: decimal.NewFromInt(2), RechargeBonusIncome: decimal.NewFromInt(2)},
		{CoveredRevenue: decimal.NewFromInt(70), UpstreamCost: decimal.NewFromInt(50), PaymentFees: decimal.NewFromInt(3), ExactCount: 8},
		{CoveredRevenue: decimal.NewFromInt(60), UpstreamCost: decimal.NewFromInt(45), PaymentFees: decimal.NewFromInt(2), ExactCount: 6},
		{CoveredRevenue: decimal.NewFromInt(8), UpstreamCost: decimal.NewFromInt(6), PaymentFees: decimal.NewFromInt(1), ExactCount: 1},
		{CoveredRevenue: decimal.NewFromInt(40), UpstreamCost: decimal.NewFromInt(30), PaymentFees: decimal.NewFromInt(2), ExactCount: 4},
		{CoveredRevenue: decimal.NewFromInt(500), UpstreamCost: decimal.NewFromInt(350), PaymentFees: decimal.NewFromInt(10), RechargeBonusIncome: decimal.NewFromInt(20), LossAmount: decimal.NewFromInt(25), ExactCount: 50},
	}}
	svc := NewFinanceReportService(repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC) }
	filter := FinanceReportFilter{StartDate: "2026-07-01", EndDate: "2026-07-10", Timezone: "UTC", StartAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), EndBefore: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)}
	overview, err := svc.Overview(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, "12.00000000", overview.Profit.Amount)
	require.Equal(t, "5.00000000", overview.RechargeBonusIncome.Amount)
	require.Equal(t, "17.00000000", overview.CombinedProfit.Amount)
	require.Equal(t, "17.66666667", overview.SettledProfit.Amount)
	require.Equal(t, "5.00000000", overview.PendingSettlementCost)
	require.Equal(t, "10.00000000", overview.UnconfiguredExposure)
	require.Equal(t, "0.8000", overview.SettlementCoverageRate)
	require.Equal(t, "1.00000000", overview.TodayProfit.Amount)
	require.Equal(t, "8.00000000", overview.MonthProfit.Amount)
	require.Equal(t, "140.00000000", overview.HistoricalProfit.Amount)
	require.Equal(t, "160.00000000", overview.HistoricalCombinedProfit.Amount)
	require.Equal(t, "25.00000000", overview.HistoricalLossAmount)
	require.Equal(t, "5.00000000", overview.EstimatedCostRisk)
	require.Equal(t, "2.00000000", overview.UnconfirmedExactCost)
	require.Equal(t, "10.00000000", overview.UnpricedRevenueRisk)
	require.Equal(t, "0.1333", *overview.MarginRate)
	require.Equal(t, "partial", overview.Quality.Status)
	require.Equal(t, "0.8000", overview.Quality.CostCoverageRate)
	require.Equal(t, "2026-07-27T08:00:00Z", overview.GeneratedAt.Format(time.RFC3339))
	require.Equal(t, filter.StartAt, repo.filters[1].EndBefore)
	require.Equal(t, "all", repo.filters[0].DataScope)
	require.Equal(t, "exact_only", repo.filters[2].DataScope)
	require.Equal(t, time.Unix(0, 0).UTC(), repo.filters[6].StartAt)
}

func TestFinanceReportFundsOnlyCalculatesAvailableDaysForUSDWallets(t *testing.T) {
	repo := &financeReportRepoStub{funds: &FinanceFundsFacts{WalletCash: []FinanceWalletCashFact{
		{WalletID: 1, Balance: decimal.NewFromInt(70), Currency: "USD", SevenDayCost: decimal.NewFromInt(35)},
		{WalletID: 2, Balance: decimal.NewFromInt(70), Currency: "CNY", SevenDayCost: decimal.NewFromInt(35)},
	}}}
	result, err := NewFinanceReportService(repo).Funds(context.Background(), FinanceReportFilter{})
	require.NoError(t, err)
	require.Equal(t, "14.00", *result.WalletCash[0].AvailableDays)
	require.Nil(t, result.WalletCash[1].AvailableDays)
}

func TestFinanceReportDataQualityIncludesDailyTrend(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	repo := &financeReportRepoStub{
		summaries: []*FinanceSummaryFacts{{MissingPriceCount: 1}},
		trend:     []FinanceTrendFact{{BucketStart: start, MissingPrice: 1, RequestCount: 1}},
		issues:    []FinanceQualityIssueFact{{UsageLogID: 10, IssueType: "missing_price"}},
	}
	result, err := NewFinanceReportService(repo).DataQuality(context.Background(), FinanceReportFilter{
		StartAt: start, EndBefore: start.AddDate(0, 0, 1), Location: time.UTC,
	}, "missing_price", 1, 20)
	require.NoError(t, err)
	require.Len(t, result.Trend, 1)
	require.Equal(t, int64(1), result.Trend[0].Quality.MissingPriceCount)
	require.Equal(t, "partial", result.Trend[0].Quality.Status)
	require.Len(t, result.Items, 1)
}

func TestFinanceReportCashFlowDoesNotDoubleCountSurcharge(t *testing.T) {
	repo := &financeReportRepoStub{cashFlow: &FinanceCashFlowFacts{
		CustomerPayments: decimal.NewFromInt(110), PaymentSurcharges: decimal.NewFromInt(10), PaymentFees: decimal.NewFromInt(3),
		UpstreamTopups: decimal.NewFromInt(50), UpstreamRefunds: decimal.NewFromInt(2), UpstreamAdjustments: decimal.NewFromInt(1),
	}}
	result, err := NewFinanceReportService(repo).CashFlow(context.Background(), FinanceReportFilter{}, FinanceCashFlowRequest{
		EventType: "payment_surcharge", Currency: "usd", Page: 2, PageSize: 50,
	})
	require.NoError(t, err)
	require.Equal(t, "60.00000000", result.Summary["net_cash_flow"])
	require.Equal(t, "USD", repo.cashReq.Currency)
	require.Equal(t, 2, repo.cashReq.Page)
}

func TestFinanceReportCashFlowRejectsInvalidFilters(t *testing.T) {
	svc := NewFinanceReportService(&financeReportRepoStub{})
	_, err := svc.CashFlow(context.Background(), FinanceReportFilter{}, FinanceCashFlowRequest{EventType: "drop table"})
	require.EqualError(t, err, "event_type is invalid")
	_, err = svc.CashFlow(context.Background(), FinanceReportFilter{}, FinanceCashFlowRequest{Currency: "US$"})
	require.EqualError(t, err, "currency is invalid")
}
