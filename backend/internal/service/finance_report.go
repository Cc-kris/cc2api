package service

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type FinanceReportFilter struct {
	StartDate      string
	EndDate        string
	Timezone       string
	Location       *time.Location
	StartAt        time.Time
	EndBefore      time.Time
	Granularity    string
	UserID         *int64
	GroupID        *int64
	ChannelID      *int64
	UpstreamID     *int64
	WalletID       *int64
	AccountID      *int64
	RequestedModel string
	UpstreamModel  string
	BillingType    string
	BusinessType   string
	CostStatuses   []string
	DataScope      string
}

type FinanceQuality struct {
	Status                 string `json:"status"`
	ExactCount             int64  `json:"exact_count"`
	EstimatedCount         int64  `json:"estimated_count"`
	MissingProfileCount    int64  `json:"missing_profile_count"`
	MissingPriceCount      int64  `json:"missing_price_count"`
	MissingMultiplierCount int64  `json:"missing_multiplier_count"`
	MissingUsageCount      int64  `json:"missing_usage_count"`
	UnsupportedUsageCount  int64  `json:"unsupported_usage_count"`
	NonBillableCount       int64  `json:"non_billable_count"`
	ExcludedCount          int64  `json:"excluded_count"`
	UnpricedRevenue        string `json:"unpriced_revenue"`
	CostCoverageRate       string `json:"cost_coverage_rate"`
}

type FinanceSummaryFacts struct {
	Revenue                decimal.Decimal
	CoveredRevenue         decimal.Decimal
	UpstreamCost           decimal.Decimal
	LossAmount             decimal.Decimal
	LossRequestCount       int64
	RequestCount           int64
	ExactCount             int64
	EstimatedCount         int64
	MissingProfileCount    int64
	MissingPriceCount      int64
	MissingMultiplierCount int64
	MissingUsageCount      int64
	UnsupportedUsageCount  int64
	NonBillableCount       int64
	ExcludedCount          int64
	UnpricedRevenue        decimal.Decimal
	EstimatedCost          decimal.Decimal
	PendingSettlementCost  decimal.Decimal
	UnconfirmedExactCost   decimal.Decimal
	PaymentFees            decimal.Decimal
	PaymentNetCash         decimal.Decimal
	UpstreamNetCash        decimal.Decimal
	RechargeBonusIncome    decimal.Decimal
	WalletCashTotal        decimal.Decimal
	TokenQuotaWalletCount  int64
	OpenAlertCount         int64
}

type FinanceOverviewMetric struct {
	Amount         string  `json:"amount"`
	Currency       string  `json:"currency"`
	PreviousAmount string  `json:"previous_amount"`
	ChangeRate     *string `json:"change_rate"`
	Status         string  `json:"status"`
}

type FinanceOverview struct {
	Range struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Timezone  string `json:"timezone"`
	} `json:"range"`
	Revenue                  FinanceOverviewMetric `json:"revenue"`
	UpstreamCost             FinanceOverviewMetric `json:"upstream_cost"`
	Profit                   FinanceOverviewMetric `json:"profit"`
	RechargeBonusIncome      FinanceOverviewMetric `json:"recharge_bonus_income"`
	CombinedProfit           FinanceOverviewMetric `json:"combined_profit"`
	TodayProfit              FinanceOverviewMetric `json:"today_profit"`
	MonthProfit              FinanceOverviewMetric `json:"month_profit"`
	HistoricalProfit         FinanceOverviewMetric `json:"historical_profit"`
	HistoricalCombinedProfit FinanceOverviewMetric `json:"historical_combined_profit"`
	HistoricalLossAmount     string                `json:"historical_loss_amount"`
	SettledProfit            FinanceOverviewMetric `json:"settled_profit"`
	PendingSettlementCost    string                `json:"pending_settlement_cost"`
	UnconfiguredExposure     string                `json:"unconfigured_revenue_exposure"`
	SettlementCoverageRate   string                `json:"settlement_coverage_rate"`
	EstimatedCostRisk        string                `json:"estimated_cost_risk"`
	UnconfirmedExactCost     string                `json:"unconfirmed_exact_cost"`
	UnpricedRevenueRisk      string                `json:"unpriced_revenue_risk"`
	MarginRate               *string               `json:"margin_rate"`
	LossAmount               string                `json:"loss_amount"`
	LossRequestCount         int64                 `json:"loss_request_count"`
	PaymentNetCash           string                `json:"payment_net_cash"`
	UpstreamNetCash          string                `json:"upstream_net_cash"`
	WalletCashTotal          string                `json:"wallet_cash_total"`
	TokenQuotaWalletCount    int64                 `json:"token_quota_wallet_count"`
	Quality                  FinanceQuality        `json:"quality"`
	OpenAlertCount           int64                 `json:"open_alert_count"`
	GeneratedAt              time.Time             `json:"generated_at"`
}

type FinanceReportRepository interface {
	SummarizeFinance(ctx context.Context, filter FinanceReportFilter) (*FinanceSummaryFacts, error)
	ListFinanceTrend(ctx context.Context, filter FinanceReportFilter) ([]FinanceTrendFact, error)
	ListFinanceBreakdown(ctx context.Context, filter FinanceReportFilter, request FinanceBreakdownRequest) ([]FinanceBreakdownFact, int64, error)
	ListFinanceDetails(ctx context.Context, filter FinanceReportFilter, request FinanceDetailsRequest) ([]FinanceDetailFact, int64, error)
	GetFinanceFunds(ctx context.Context, filter FinanceReportFilter) (*FinanceFundsFacts, error)
	ListFinanceQualityIssues(ctx context.Context, filter FinanceReportFilter, issueType string, page, pageSize int) ([]FinanceQualityIssueFact, int64, error)
	GetFinanceCashFlow(ctx context.Context, filter FinanceReportFilter, request FinanceCashFlowRequest) (*FinanceCashFlowFacts, error)
}

type FinanceWalletCashFact struct {
	WalletID        int64
	WalletName      string
	Balance         decimal.Decimal
	Currency        string
	CollectedAt     time.Time
	SevenDayCost    decimal.Decimal
	SyncStatus      string
	BalanceScopeKey string
	IncludedInTotal bool
	Stale           bool
}

type FinanceTokenQuotaFact struct {
	WalletID    int64
	WalletName  string
	TotalQuota  decimal.Decimal
	UsedQuota   decimal.Decimal
	Currency    string
	CollectedAt time.Time
	SyncStatus  string
}

type FinanceFundsFacts struct {
	WalletCash          []FinanceWalletCashFact
	TokenQuota          []FinanceTokenQuotaFact
	CustomerBalance     decimal.Decimal
	CustomerPayment     decimal.Decimal
	CustomerRefund      decimal.Decimal
	PaymentFees         decimal.Decimal
	UpstreamTopup       decimal.Decimal
	UpstreamTopupCount  int64
	UpstreamEventCount  int64
	UpstreamRefund      decimal.Decimal
	UpstreamAdjust      decimal.Decimal
	RechargeBonusIncome decimal.Decimal
	StaleWalletCount    int64
	FailedSyncCount     int64
}

type FinanceWalletCashItem struct {
	WalletID        int64     `json:"wallet_id"`
	WalletName      string    `json:"wallet_name"`
	Balance         string    `json:"balance"`
	Currency        string    `json:"currency"`
	DailyCost       string    `json:"daily_cost"`
	AvailableDays   *string   `json:"available_days"`
	CollectedAt     time.Time `json:"collected_at"`
	SyncStatus      string    `json:"sync_status"`
	BalanceScopeKey string    `json:"balance_scope_key"`
	IncludedInTotal bool      `json:"included_in_total"`
	Stale           bool      `json:"stale"`
}

type FinanceTokenQuotaItem struct {
	WalletID       int64     `json:"wallet_id"`
	WalletName     string    `json:"wallet_name"`
	TotalQuota     string    `json:"total_quota"`
	UsedQuota      string    `json:"used_quota"`
	RemainingQuota string    `json:"remaining_quota"`
	Currency       string    `json:"currency"`
	CollectedAt    time.Time `json:"collected_at"`
	SyncStatus     string    `json:"sync_status"`
}

type FinanceFunds struct {
	WalletCash      []FinanceWalletCashItem `json:"wallet_cash"`
	TokenQuota      []FinanceTokenQuotaItem `json:"token_quota"`
	CustomerBalance string                  `json:"customer_balance"`
	CustomerCash    struct {
		Payment string `json:"payment"`
		Refund  string `json:"refund"`
		Fees    string `json:"payment_fees"`
		NetCash string `json:"net_cash"`
	} `json:"customer_cash"`
	UpstreamCash struct {
		Topup               string `json:"topup"`
		TopupAvailable      bool   `json:"topup_available"`
		TopupEventCount     int64  `json:"topup_event_count"`
		NetCashAvailable    bool   `json:"net_cash_available"`
		EventCount          int64  `json:"event_count"`
		Refund              string `json:"refund"`
		Adjustment          string `json:"adjustment"`
		RechargeBonusIncome string `json:"recharge_bonus_income"`
		NetCash             string `json:"net_cash"`
	} `json:"upstream_cash"`
	StaleWalletCount int64 `json:"stale_wallet_count"`
	FailedSyncCount  int64 `json:"failed_sync_count"`
}

func (s *FinanceReportService) Funds(ctx context.Context, filter FinanceReportFilter) (*FinanceFunds, error) {
	facts, err := s.repo.GetFinanceFunds(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := &FinanceFunds{StaleWalletCount: facts.StaleWalletCount, FailedSyncCount: facts.FailedSyncCount}
	result.CustomerBalance = financeMoney(facts.CustomerBalance)
	for _, wallet := range facts.WalletCash {
		dailyCost := wallet.SevenDayCost.Div(decimal.NewFromInt(7))
		item := FinanceWalletCashItem{
			WalletID: wallet.WalletID, WalletName: wallet.WalletName, Balance: financeMoney(wallet.Balance), Currency: wallet.Currency,
			DailyCost: financeMoney(dailyCost), CollectedAt: wallet.CollectedAt, SyncStatus: wallet.SyncStatus,
			BalanceScopeKey: wallet.BalanceScopeKey, IncludedInTotal: wallet.IncludedInTotal, Stale: wallet.Stale,
		}
		if strings.EqualFold(wallet.Currency, "USD") && dailyCost.GreaterThan(decimal.Zero) {
			days := wallet.Balance.Div(dailyCost).Round(2).StringFixed(2)
			item.AvailableDays = &days
		}
		result.WalletCash = append(result.WalletCash, item)
	}
	for _, quota := range facts.TokenQuota {
		result.TokenQuota = append(result.TokenQuota, FinanceTokenQuotaItem{
			WalletID: quota.WalletID, WalletName: quota.WalletName, TotalQuota: financeMoney(quota.TotalQuota),
			UsedQuota: financeMoney(quota.UsedQuota), RemainingQuota: financeMoney(quota.TotalQuota.Sub(quota.UsedQuota)),
			Currency: quota.Currency, CollectedAt: quota.CollectedAt, SyncStatus: quota.SyncStatus,
		})
	}
	result.CustomerCash.Payment = financeMoney(facts.CustomerPayment)
	result.CustomerCash.Refund = financeMoney(facts.CustomerRefund)
	result.CustomerCash.Fees = financeMoney(facts.PaymentFees)
	result.CustomerCash.NetCash = financeMoney(facts.CustomerPayment.Sub(facts.CustomerRefund).Sub(facts.PaymentFees))
	result.UpstreamCash.Topup = financeMoney(facts.UpstreamTopup)
	result.UpstreamCash.TopupAvailable = facts.UpstreamTopupCount > 0
	result.UpstreamCash.TopupEventCount = facts.UpstreamTopupCount
	result.UpstreamCash.NetCashAvailable = facts.UpstreamEventCount > 0
	result.UpstreamCash.EventCount = facts.UpstreamEventCount
	result.UpstreamCash.Refund = financeMoney(facts.UpstreamRefund)
	result.UpstreamCash.Adjustment = financeMoney(facts.UpstreamAdjust)
	result.UpstreamCash.RechargeBonusIncome = financeMoney(facts.RechargeBonusIncome)
	result.UpstreamCash.NetCash = financeMoney(facts.UpstreamRefund.Add(facts.UpstreamAdjust).Sub(facts.UpstreamTopup))
	return result, nil
}

type FinanceQualityIssueFact struct {
	UsageLogID      int64
	IssueType       string
	RelatedType     string
	RelatedID       *int64
	ExposedRevenue  decimal.Decimal
	FirstDetectedAt time.Time
	LastScannedAt   time.Time
}

type FinanceQualityIssue struct {
	UsageLogID      int64     `json:"usage_log_id"`
	IssueType       string    `json:"issue_type"`
	RelatedType     string    `json:"related_type"`
	RelatedID       *int64    `json:"related_id"`
	ExposedRevenue  string    `json:"exposed_revenue"`
	FirstDetectedAt time.Time `json:"first_detected_at"`
	LastScannedAt   time.Time `json:"last_scanned_at"`
	Recalculable    bool      `json:"recalculable"`
}

type FinanceDataQuality struct {
	Quality  FinanceQuality            `json:"quality"`
	Trend    []FinanceQualityTrendItem `json:"trend"`
	Items    []FinanceQualityIssue     `json:"items"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

type FinanceQualityTrendItem struct {
	BucketStart time.Time      `json:"bucket_start"`
	BucketEnd   time.Time      `json:"bucket_end"`
	Quality     FinanceQuality `json:"quality"`
}

func (s *FinanceReportService) DataQuality(ctx context.Context, filter FinanceReportFilter, issueType string, page, pageSize int) (*FinanceDataQuality, error) {
	issueType = strings.TrimSpace(issueType)
	if issueType != "" && !financeAllowed(issueType, "missing_profile", "missing_price", "missing_multiplier", "missing_usage", "unsupported_usage") {
		return nil, financeValidationError("issue_type is invalid")
	}
	normalizeFinancePage(&page, &pageSize)
	summary, err := s.repo.SummarizeFinance(ctx, filter)
	if err != nil {
		return nil, err
	}
	facts, total, err := s.repo.ListFinanceQualityIssues(ctx, filter, issueType, page, pageSize)
	if err != nil {
		return nil, err
	}
	result := &FinanceDataQuality{Quality: financeQualityFromSummary(summary), Total: total, Page: page, PageSize: pageSize}
	trendFilter := filter
	trendFilter.Granularity = "day"
	trend, err := s.Trend(ctx, trendFilter)
	if err != nil {
		return nil, err
	}
	for _, item := range trend {
		result.Trend = append(result.Trend, FinanceQualityTrendItem{
			BucketStart: item.BucketStart,
			BucketEnd:   item.BucketEnd,
			Quality:     item.Quality,
		})
	}
	for _, fact := range facts {
		result.Items = append(result.Items, FinanceQualityIssue{
			UsageLogID: fact.UsageLogID, IssueType: fact.IssueType, RelatedType: fact.RelatedType, RelatedID: fact.RelatedID,
			ExposedRevenue: financeMoney(fact.ExposedRevenue), FirstDetectedAt: fact.FirstDetectedAt, LastScannedAt: fact.LastScannedAt,
			Recalculable: fact.IssueType != "missing_usage" && fact.IssueType != "unsupported_usage",
		})
	}
	return result, nil
}

type FinanceCashFlowItem struct {
	EventType      string    `json:"event_type"`
	SourceType     string    `json:"source_type"`
	SourceID       int64     `json:"source_id"`
	OriginalAmount string    `json:"original_amount"`
	Currency       string    `json:"currency"`
	FXRateToUSD    string    `json:"fx_rate_to_usd"`
	USDAmount      string    `json:"usd_amount"`
	OccurredAt     time.Time `json:"occurred_at"`
	ReferenceNo    string    `json:"reference_no,omitempty"`
}

type FinanceCashFlowFacts struct {
	Items               []FinanceCashFlowItem
	Total               int64
	CustomerPayments    decimal.Decimal
	CustomerRefunds     decimal.Decimal
	PaymentSurcharges   decimal.Decimal
	PaymentFees         decimal.Decimal
	UpstreamTopups      decimal.Decimal
	UpstreamRefunds     decimal.Decimal
	UpstreamAdjustments decimal.Decimal
}

type FinanceCashFlow struct {
	Summary  map[string]string     `json:"summary"`
	Items    []FinanceCashFlowItem `json:"items"`
	Total    int64                 `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
}

type FinanceCashFlowRequest struct {
	EventType string
	Currency  string
	Page      int
	PageSize  int
}

func (s *FinanceReportService) CashFlow(ctx context.Context, filter FinanceReportFilter, request FinanceCashFlowRequest) (*FinanceCashFlow, error) {
	request.EventType = strings.TrimSpace(request.EventType)
	if request.EventType != "" && !financeAllowed(request.EventType,
		"customer_payment", "customer_refund", "payment_surcharge", "payment_fee",
		"upstream_topup", "upstream_refund", "upstream_adjustment",
	) {
		return nil, financeValidationError("event_type is invalid")
	}
	request.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	if request.Currency != "" && !validFinanceCurrency(request.Currency) {
		return nil, financeValidationError("currency is invalid")
	}
	normalizeFinancePage(&request.Page, &request.PageSize)
	facts, err := s.repo.GetFinanceCashFlow(ctx, filter, request)
	if err != nil {
		return nil, err
	}
	net := facts.CustomerPayments.Sub(facts.CustomerRefunds).Sub(facts.PaymentFees).Sub(facts.UpstreamTopups).Add(facts.UpstreamRefunds).Add(facts.UpstreamAdjustments)
	return &FinanceCashFlow{Summary: map[string]string{
		"customer_payments": financeMoney(facts.CustomerPayments), "customer_refunds": financeMoney(facts.CustomerRefunds),
		"payment_surcharges": financeMoney(facts.PaymentSurcharges), "payment_fees": financeMoney(facts.PaymentFees),
		"upstream_topups": financeMoney(facts.UpstreamTopups), "upstream_refunds": financeMoney(facts.UpstreamRefunds),
		"upstream_adjustments": financeMoney(facts.UpstreamAdjustments), "net_cash_flow": financeMoney(net),
	}, Items: facts.Items, Total: facts.Total, Page: request.Page, PageSize: request.PageSize}, nil
}

type FinanceDetailsRequest struct {
	ProfitDirection string
	LossReason      string
	LossStatus      string
	RequestID       string
	SortBy          string
	SortOrder       string
	Page            int
	PageSize        int
}

type FinanceDetailFact struct {
	UsageLogID      int64
	RequestID       string
	UsageCreatedAt  time.Time
	UserID          int64
	UserName        string
	GroupID         *int64
	GroupName       string
	ChannelID       *int64
	ChannelName     string
	AccountID       *int64
	AccountName     string
	WalletID        *int64
	WalletName      string
	UpstreamID      *int64
	UpstreamName    string
	RequestedModel  string
	UpstreamModel   string
	ServiceTier     string
	PricingSource   string
	SalesVersion    string
	Revenue         decimal.Decimal
	UpstreamCost    *decimal.Decimal
	CostStatus      string
	SegmentCount    int64
	CacheTokenCount int64
	AlertID         *int64
	AlertStatus     string
	AssigneeID      *int64
	HandledBy       *int64
	HandledNote     string
	HandledAt       *time.Time
}

type FinanceDetailItem struct {
	UsageLogID     int64     `json:"usage_log_id"`
	RequestID      string    `json:"request_id"`
	UsageCreatedAt time.Time `json:"usage_created_at"`
	UserID         int64     `json:"user_id"`
	UserName       string    `json:"user_name"`
	GroupID        *int64    `json:"group_id"`
	GroupName      string    `json:"group_name"`
	ChannelID      *int64    `json:"channel_id"`
	ChannelName    string    `json:"channel_name"`
	AccountID      *int64    `json:"account_id"`
	AccountName    string    `json:"account_name"`
	WalletID       *int64    `json:"wallet_id"`
	WalletName     string    `json:"wallet_name"`
	UpstreamID     *int64    `json:"upstream_id"`
	UpstreamName   string    `json:"upstream_name"`
	RequestedModel string    `json:"requested_model"`
	UpstreamModel  string    `json:"upstream_model"`
	SalesVersion   string    `json:"sales_pricing_version"`
	Revenue        string    `json:"revenue"`
	UpstreamCost   *string   `json:"upstream_cost"`
	Profit         *string   `json:"profit"`
	MarginRate     *string   `json:"margin_rate"`
	CostStatus     string    `json:"cost_status"`
}

type FinanceLossItem struct {
	FinanceDetailItem
	LossAmount  string     `json:"loss_amount"`
	LossReason  string     `json:"loss_reason"`
	AlertID     *int64     `json:"alert_id"`
	Status      string     `json:"status"`
	AssigneeID  *int64     `json:"assignee_id"`
	HandledBy   *int64     `json:"handled_by"`
	HandledNote string     `json:"handled_note,omitempty"`
	HandledAt   *time.Time `json:"handled_at"`
}

func (s *FinanceReportService) Details(ctx context.Context, filter FinanceReportFilter, request FinanceDetailsRequest) ([]FinanceDetailItem, int64, error) {
	if err := normalizeFinanceDetailsRequest(&request); err != nil {
		return nil, 0, err
	}
	facts, total, err := s.repo.ListFinanceDetails(ctx, filter, request)
	if err != nil {
		return nil, 0, err
	}
	items := make([]FinanceDetailItem, 0, len(facts))
	for _, fact := range facts {
		items = append(items, financeDetailItemFromFact(fact))
	}
	return items, total, nil
}

func (s *FinanceReportService) Losses(ctx context.Context, filter FinanceReportFilter, request FinanceDetailsRequest, lossReason string) ([]FinanceLossItem, int64, error) {
	request.ProfitDirection = "loss"
	if err := normalizeFinanceDetailsRequest(&request); err != nil {
		return nil, 0, err
	}
	lossReason = strings.TrimSpace(lossReason)
	if lossReason != "" && !financeAllowed(lossReason, "sales_multiplier_too_low", "upstream_multiplier_increased", "channel_price_mismatch", "fast_cost_not_covered", "cache_cost_not_covered", "multi_attempt_cost", "other") {
		return nil, 0, financeValidationError("loss_reason is invalid")
	}
	request.LossReason = lossReason
	facts, total, err := s.repo.ListFinanceDetails(ctx, filter, request)
	if err != nil {
		return nil, 0, err
	}
	items := make([]FinanceLossItem, 0, len(facts))
	for _, fact := range facts {
		reason := classifyFinanceLoss(fact)
		base := financeDetailItemFromFact(fact)
		loss := fact.UpstreamCost.Sub(fact.Revenue)
		status := fact.AlertStatus
		if status == "" {
			status = "untracked"
		}
		items = append(items, FinanceLossItem{
			FinanceDetailItem: base, LossAmount: financeMoney(loss), LossReason: reason,
			AlertID: fact.AlertID, Status: status, AssigneeID: fact.AssigneeID,
			HandledBy: fact.HandledBy, HandledNote: fact.HandledNote, HandledAt: fact.HandledAt,
		})
	}
	return items, total, nil
}

func normalizeFinanceDetailsRequest(request *FinanceDetailsRequest) error {
	request.ProfitDirection = defaultFinanceValue(request.ProfitDirection, "all")
	if !financeAllowed(request.ProfitDirection, "all", "profit", "loss", "zero", "unknown") {
		return financeValidationError("profit_direction is invalid")
	}
	request.SortBy = defaultFinanceValue(request.SortBy, "usage_created_at")
	if !financeAllowed(request.SortBy, "usage_created_at", "revenue", "upstream_cost", "profit", "margin_rate") {
		return financeValidationError("sort_by is invalid")
	}
	request.SortOrder = defaultFinanceValue(request.SortOrder, "desc")
	if !financeAllowed(request.SortOrder, "asc", "desc") {
		return financeValidationError("sort_order must be asc or desc")
	}
	normalizeFinancePage(&request.Page, &request.PageSize)
	request.RequestID = strings.TrimSpace(request.RequestID)
	request.LossStatus = strings.TrimSpace(request.LossStatus)
	if request.LossStatus != "" && !financeAllowed(request.LossStatus, "open", "acknowledged", "resolved", "ignored", "untracked") {
		return financeValidationError("status is invalid")
	}
	return nil
}

func financeDetailItemFromFact(fact FinanceDetailFact) FinanceDetailItem {
	item := FinanceDetailItem{
		UsageLogID: fact.UsageLogID, RequestID: fact.RequestID, UsageCreatedAt: fact.UsageCreatedAt,
		UserID: fact.UserID, UserName: fact.UserName, GroupID: fact.GroupID, GroupName: fact.GroupName,
		ChannelID: fact.ChannelID, ChannelName: fact.ChannelName, AccountID: fact.AccountID, AccountName: fact.AccountName,
		WalletID: fact.WalletID, WalletName: fact.WalletName, UpstreamID: fact.UpstreamID, UpstreamName: fact.UpstreamName,
		RequestedModel: fact.RequestedModel, UpstreamModel: fact.UpstreamModel, SalesVersion: fact.SalesVersion,
		Revenue: financeMoney(fact.Revenue), CostStatus: fact.CostStatus,
	}
	if fact.UpstreamCost != nil {
		cost := financeMoney(*fact.UpstreamCost)
		profitValue := fact.Revenue.Sub(*fact.UpstreamCost)
		profit := financeMoney(profitValue)
		item.UpstreamCost = &cost
		item.Profit = &profit
		if !fact.Revenue.IsZero() {
			margin := profitValue.Div(fact.Revenue).Round(4).StringFixed(4)
			item.MarginRate = &margin
		}
	}
	return item
}

func classifyFinanceLoss(fact FinanceDetailFact) string {
	if fact.SegmentCount > 1 {
		return "multi_attempt_cost"
	}
	if strings.EqualFold(fact.ServiceTier, "fast") || strings.EqualFold(fact.ServiceTier, "priority") {
		return "fast_cost_not_covered"
	}
	if fact.CacheTokenCount > 0 {
		return "cache_cost_not_covered"
	}
	if fact.PricingSource == FinancePricingSourceChannel {
		return "channel_price_mismatch"
	}
	if fact.PricingSource == FinancePricingSourceEstimatedSystem {
		return "other"
	}
	if fact.Revenue.IsPositive() {
		return "sales_multiplier_too_low"
	}
	return "other"
}

type FinanceReportService struct {
	repo FinanceReportRepository
	now  func() time.Time
}

func NewFinanceReportService(repo FinanceReportRepository) *FinanceReportService {
	return &FinanceReportService{repo: repo, now: time.Now}
}

func (s *FinanceReportService) Overview(ctx context.Context, filter FinanceReportFilter) (*FinanceOverview, error) {
	bookFilter := filter
	bookFilter.DataScope = "all"
	current, err := s.repo.SummarizeFinance(ctx, bookFilter)
	if err != nil {
		return nil, err
	}
	duration := filter.EndBefore.Sub(filter.StartAt)
	previousFilter := bookFilter
	previousFilter.EndBefore = filter.StartAt
	previousFilter.StartAt = filter.StartAt.Add(-duration)
	previous, err := s.repo.SummarizeFinance(ctx, previousFilter)
	if err != nil {
		return nil, err
	}
	profit := current.CoveredRevenue.Sub(current.UpstreamCost).Sub(current.PaymentFees)
	previousProfit := previous.CoveredRevenue.Sub(previous.UpstreamCost).Sub(previous.PaymentFees)
	combinedProfit := profit.Add(current.RechargeBonusIncome)
	previousCombinedProfit := previousProfit.Add(previous.RechargeBonusIncome)
	estimatedCostRisk := current.EstimatedCost
	quality := financeQualityFromSummary(current)
	status := quality.Status
	// Exact settlement, today, month and all-history figures are advanced
	// reports. They are intentionally not loaded by the default overview.
	settledProfitMetric := unavailableFinanceMetric()
	overview := &FinanceOverview{
		Revenue:                  financeOverviewMetric(current.Revenue, previous.Revenue, "complete"),
		UpstreamCost:             financeOverviewMetric(current.UpstreamCost, previous.UpstreamCost, status),
		Profit:                   financeOverviewMetric(profit, previousProfit, status),
		RechargeBonusIncome:      financeOverviewMetric(current.RechargeBonusIncome, previous.RechargeBonusIncome, "complete"),
		CombinedProfit:           financeOverviewMetric(combinedProfit, previousCombinedProfit, status),
		TodayProfit:              unavailableFinanceMetric(),
		MonthProfit:              unavailableFinanceMetric(),
		HistoricalProfit:         unavailableFinanceMetric(),
		HistoricalCombinedProfit: unavailableFinanceMetric(),
		HistoricalLossAmount:     "",
		SettledProfit:            settledProfitMetric,
		PendingSettlementCost:    financeMoney(current.PendingSettlementCost),
		UnconfiguredExposure:     financeMoney(current.UnpricedRevenue),
		SettlementCoverageRate:   quality.CostCoverageRate,
		EstimatedCostRisk:        financeMoney(estimatedCostRisk),
		UnconfirmedExactCost:     financeMoney(current.UnconfirmedExactCost),
		UnpricedRevenueRisk:      financeMoney(current.UnpricedRevenue),
		LossAmount:               financeMoney(current.LossAmount), LossRequestCount: current.LossRequestCount,
		PaymentNetCash: financeMoney(current.PaymentNetCash), UpstreamNetCash: financeMoney(current.UpstreamNetCash),
		WalletCashTotal: financeMoney(current.WalletCashTotal), TokenQuotaWalletCount: current.TokenQuotaWalletCount,
		Quality: quality, OpenAlertCount: current.OpenAlertCount, GeneratedAt: s.now().UTC(),
	}
	overview.Range.StartDate = filter.StartDate
	overview.Range.EndDate = filter.EndDate
	overview.Range.Timezone = filter.Timezone
	if !current.CoveredRevenue.IsZero() {
		margin := profit.Div(current.CoveredRevenue).Round(4).StringFixed(4)
		overview.MarginRate = &margin
	}
	return overview, nil
}

func unavailableFinanceMetric() FinanceOverviewMetric {
	return FinanceOverviewMetric{Amount: "", Currency: "USD", PreviousAmount: "", Status: "unavailable"}
}

type FinanceTrendFact struct {
	BucketStart         time.Time
	BucketEnd           time.Time
	Revenue             decimal.Decimal
	CoveredRevenue      decimal.Decimal
	UpstreamCost        decimal.Decimal
	PaymentFees         decimal.Decimal
	RechargeBonusIncome decimal.Decimal
	LossAmount          decimal.Decimal
	RequestCount        int64
	ExactCount          int64
	EstimatedCount      int64
	MissingProfile      int64
	MissingPrice        int64
	MissingMultiplier   int64
	MissingUsage        int64
	UnsupportedUsage    int64
	ExcludedCount       int64
}

type FinanceTrendItem struct {
	BucketStart              time.Time      `json:"bucket_start"`
	BucketEnd                time.Time      `json:"bucket_end"`
	Revenue                  string         `json:"revenue"`
	CoveredRevenue           string         `json:"covered_revenue"`
	UpstreamCost             string         `json:"upstream_cost"`
	RechargeBonusIncome      string         `json:"recharge_bonus_income"`
	Profit                   string         `json:"profit"`
	CombinedProfit           string         `json:"combined_profit"`
	CumulativeProfit         string         `json:"cumulative_profit"`
	CumulativeCombinedProfit string         `json:"cumulative_combined_profit"`
	CostCoverageRate         string         `json:"cost_coverage_rate"`
	LossAmount               string         `json:"loss_amount"`
	MarginRate               *string        `json:"margin_rate"`
	RequestCount             int64          `json:"request_count"`
	Quality                  FinanceQuality `json:"quality"`
}

func (s *FinanceReportService) Trend(ctx context.Context, filter FinanceReportFilter) ([]FinanceTrendItem, error) {
	facts, err := s.repo.ListFinanceTrend(ctx, filter)
	if err != nil {
		return nil, err
	}
	byBucket := make(map[int64]FinanceTrendFact, len(facts))
	for _, fact := range facts {
		byBucket[fact.BucketStart.Unix()] = fact
	}
	items := make([]FinanceTrendItem, 0)
	cumulativeProfit := decimal.Zero
	cumulativeCombinedProfit := decimal.Zero
	for cursor := truncateFinanceBucket(filter.StartAt.In(filter.Location), filter.Granularity); cursor.Before(filter.EndBefore.In(filter.Location)); cursor = nextFinanceBucket(cursor, filter.Granularity) {
		next := nextFinanceBucket(cursor, filter.Granularity)
		fact := byBucket[cursor.UTC().Unix()]
		fact.BucketStart = cursor.UTC()
		fact.BucketEnd = next.UTC()
		profit := fact.CoveredRevenue.Sub(fact.UpstreamCost).Sub(fact.PaymentFees)
		combinedProfit := profit.Add(fact.RechargeBonusIncome)
		cumulativeProfit = cumulativeProfit.Add(profit)
		cumulativeCombinedProfit = cumulativeCombinedProfit.Add(combinedProfit)
		summary := &FinanceSummaryFacts{
			ExactCount: fact.ExactCount, EstimatedCount: fact.EstimatedCount,
			MissingProfileCount: fact.MissingProfile, MissingPriceCount: fact.MissingPrice,
			MissingMultiplierCount: fact.MissingMultiplier, MissingUsageCount: fact.MissingUsage,
			UnsupportedUsageCount: fact.UnsupportedUsage, ExcludedCount: fact.ExcludedCount,
		}
		item := FinanceTrendItem{
			BucketStart: fact.BucketStart, BucketEnd: fact.BucketEnd, Revenue: financeMoney(fact.Revenue), CoveredRevenue: financeMoney(fact.CoveredRevenue),
			UpstreamCost: financeMoney(fact.UpstreamCost), RechargeBonusIncome: financeMoney(fact.RechargeBonusIncome), Profit: financeMoney(profit), CombinedProfit: financeMoney(combinedProfit), CumulativeProfit: financeMoney(cumulativeProfit), CumulativeCombinedProfit: financeMoney(cumulativeCombinedProfit), LossAmount: financeMoney(fact.LossAmount),
			RequestCount: fact.RequestCount, Quality: financeQualityFromSummary(summary),
		}
		if fact.RequestCount > 0 {
			coveredCount := fact.ExactCount
			item.CostCoverageRate = decimal.NewFromInt(coveredCount).Div(decimal.NewFromInt(fact.RequestCount)).Round(4).StringFixed(4)
		} else {
			item.CostCoverageRate = "0.0000"
		}
		if !fact.CoveredRevenue.IsZero() {
			margin := profit.Div(fact.CoveredRevenue).Round(4).StringFixed(4)
			item.MarginRate = &margin
		}
		items = append(items, item)
	}
	return items, nil
}

type FinanceBreakdownRequest struct {
	Dimension string
	SortBy    string
	SortOrder string
	Page      int
	PageSize  int
}

type FinanceBreakdownFact struct {
	DimensionKey   string
	DimensionName  string
	Revenue        decimal.Decimal
	CoveredRevenue decimal.Decimal
	UpstreamCost   decimal.Decimal
	LossAmount     decimal.Decimal
	RequestCount   int64
	ExactCount     int64
	EstimatedCount int64
	MissingCount   int64
	InputCost      decimal.Decimal
	OutputCost     decimal.Decimal
	CacheCost      decimal.Decimal
	FastCost       decimal.Decimal
	ImageCost      decimal.Decimal
	VideoCost      decimal.Decimal
	OtherCost      decimal.Decimal
}

type FinanceBreakdownItem struct {
	DimensionKey   string  `json:"dimension_key"`
	DimensionName  string  `json:"dimension_name"`
	Revenue        string  `json:"revenue"`
	UpstreamCost   string  `json:"upstream_cost"`
	Profit         string  `json:"profit"`
	MarginRate     *string `json:"margin_rate"`
	LossAmount     string  `json:"loss_amount"`
	RequestCount   int64   `json:"request_count"`
	ExactCount     int64   `json:"exact_count"`
	EstimatedCount int64   `json:"estimated_count"`
	MissingCount   int64   `json:"missing_count"`
	InputCost      string  `json:"input_cost"`
	OutputCost     string  `json:"output_cost"`
	CacheCost      string  `json:"cache_cost"`
	FastCost       string  `json:"fast_cost"`
	ImageCost      string  `json:"image_cost"`
	VideoCost      string  `json:"video_cost"`
	OtherCost      string  `json:"other_cost"`
}

func (s *FinanceReportService) Breakdown(ctx context.Context, filter FinanceReportFilter, request FinanceBreakdownRequest) ([]FinanceBreakdownItem, int64, error) {
	request.Dimension = strings.ToLower(strings.TrimSpace(request.Dimension))
	if !financeAllowed(request.Dimension, "user", "group", "channel", "upstream", "wallet", "account", "requested_model", "upstream_model", "billing_type", "business_type") {
		return nil, 0, financeValidationError("dimension is invalid")
	}
	request.SortBy = defaultFinanceValue(request.SortBy, "profit")
	if !financeAllowed(request.SortBy, "revenue", "upstream_cost", "profit", "loss_amount", "margin_rate", "request_count") {
		return nil, 0, financeValidationError("sort_by is invalid")
	}
	request.SortOrder = defaultFinanceValue(request.SortOrder, "desc")
	if !financeAllowed(request.SortOrder, "asc", "desc") {
		return nil, 0, financeValidationError("sort_order must be asc or desc")
	}
	normalizeFinancePage(&request.Page, &request.PageSize)
	facts, total, err := s.repo.ListFinanceBreakdown(ctx, filter, request)
	if err != nil {
		return nil, 0, err
	}
	items := make([]FinanceBreakdownItem, 0, len(facts))
	for _, fact := range facts {
		profit := fact.CoveredRevenue.Sub(fact.UpstreamCost)
		item := FinanceBreakdownItem{
			DimensionKey: fact.DimensionKey, DimensionName: fact.DimensionName,
			Revenue: financeMoney(fact.Revenue), UpstreamCost: financeMoney(fact.UpstreamCost), Profit: financeMoney(profit),
			LossAmount: financeMoney(fact.LossAmount), RequestCount: fact.RequestCount,
			ExactCount: fact.ExactCount, EstimatedCount: fact.EstimatedCount, MissingCount: fact.MissingCount,
			InputCost: financeMoney(fact.InputCost), OutputCost: financeMoney(fact.OutputCost),
			CacheCost: financeMoney(fact.CacheCost), FastCost: financeMoney(fact.FastCost),
			ImageCost: financeMoney(fact.ImageCost), VideoCost: financeMoney(fact.VideoCost), OtherCost: financeMoney(fact.OtherCost),
		}
		if !fact.CoveredRevenue.IsZero() {
			margin := profit.Div(fact.CoveredRevenue).Round(4).StringFixed(4)
			item.MarginRate = &margin
		}
		items = append(items, item)
	}
	return items, total, nil
}

func truncateFinanceBucket(value time.Time, granularity string) time.Time {
	switch granularity {
	case "hour":
		return value.Truncate(time.Hour)
	case "week":
		day := (int(value.Weekday()) + 6) % 7
		return time.Date(value.Year(), value.Month(), value.Day()-day, 0, 0, 0, 0, value.Location())
	case "month":
		return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, value.Location())
	default:
		return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
	}
}

func nextFinanceBucket(value time.Time, granularity string) time.Time {
	switch granularity {
	case "hour":
		return value.Add(time.Hour)
	case "week":
		return value.AddDate(0, 0, 7)
	case "month":
		return value.AddDate(0, 1, 0)
	default:
		return value.AddDate(0, 0, 1)
	}
}

func ParseFinanceReportFilter(values url.Values) (FinanceReportFilter, error) {
	startDate := strings.TrimSpace(values.Get("start_date"))
	endDate := strings.TrimSpace(values.Get("end_date"))
	if startDate == "" || endDate == "" {
		return FinanceReportFilter{}, financeValidationError("start_date and end_date are required")
	}
	timezone := strings.TrimSpace(values.Get("timezone"))
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return FinanceReportFilter{}, financeValidationError("timezone is invalid")
	}
	startAt, err := time.ParseInLocation("2006-01-02", startDate, location)
	if err != nil {
		return FinanceReportFilter{}, financeValidationError("start_date must be YYYY-MM-DD")
	}
	endAt, err := time.ParseInLocation("2006-01-02", endDate, location)
	if err != nil {
		return FinanceReportFilter{}, financeValidationError("end_date must be YYYY-MM-DD")
	}
	if endAt.Before(startAt) {
		return FinanceReportFilter{}, financeValidationError("end_date must not be earlier than start_date")
	}
	if endAt.Sub(startAt) > 731*24*time.Hour {
		return FinanceReportFilter{}, financeValidationError("date range must not exceed 732 days")
	}
	filter := FinanceReportFilter{
		StartDate: startDate, EndDate: endDate, Timezone: timezone, Location: location,
		StartAt: startAt.UTC(), EndBefore: endAt.AddDate(0, 0, 1).UTC(),
		Granularity:    defaultFinanceValue(values.Get("granularity"), "day"),
		RequestedModel: strings.TrimSpace(values.Get("requested_model")), UpstreamModel: strings.TrimSpace(values.Get("upstream_model")),
		BillingType: strings.TrimSpace(values.Get("billing_type")), BusinessType: strings.TrimSpace(values.Get("business_type")),
		DataScope: defaultFinanceValue(values.Get("data_scope"), "exact_only"),
	}
	if !financeAllowed(filter.Granularity, "hour", "day", "week", "month") {
		return FinanceReportFilter{}, financeValidationError("granularity must be hour, day, week or month")
	}
	if !financeAllowed(filter.DataScope, "all", "exact_only") {
		return FinanceReportFilter{}, financeValidationError("data_scope must be all or exact_only")
	}
	if filter.BillingType != "" && !financeAllowed(filter.BillingType, "token", "request", "per_request", "image", "video", "per_second", "subscription") {
		return FinanceReportFilter{}, financeValidationError("billing_type is invalid")
	}
	if filter.BusinessType != "" && !financeAllowed(filter.BusinessType, "api", "balance", "subscription", "promotion", "admin", "other") {
		return FinanceReportFilter{}, financeValidationError("business_type is invalid")
	}
	for queryName, target := range map[string]**int64{
		"user_id": &filter.UserID, "group_id": &filter.GroupID, "channel_id": &filter.ChannelID,
		"upstream_id": &filter.UpstreamID, "wallet_id": &filter.WalletID, "account_id": &filter.AccountID,
	} {
		if raw := strings.TrimSpace(values.Get(queryName)); raw != "" {
			parsed, parseErr := strconv.ParseInt(raw, 10, 64)
			if parseErr != nil || parsed <= 0 {
				return FinanceReportFilter{}, financeValidationErrorf("%s must be a positive integer", queryName)
			}
			*target = &parsed
		}
	}
	if raw := strings.TrimSpace(values.Get("cost_status")); raw != "" {
		seen := map[string]struct{}{}
		for _, status := range strings.Split(raw, ",") {
			status = strings.TrimSpace(status)
			if !financeAllowed(status, string(FinanceCostStatusExact), string(FinanceCostStatusEstimated), string(FinanceCostStatusMissingProfile), string(FinanceCostStatusMissingPrice), string(FinanceCostStatusMissingMultiplier), string(FinanceCostStatusMissingUsage), string(FinanceCostStatusUnsupportedUsage), string(FinanceCostStatusNonBillable), string(FinanceCostStatusExcluded)) {
				return FinanceReportFilter{}, financeValidationErrorf("invalid cost_status %q", status)
			}
			if _, ok := seen[status]; !ok {
				seen[status] = struct{}{}
				filter.CostStatuses = append(filter.CostStatuses, status)
			}
		}
	}
	return filter, nil
}

func financeOverviewMetric(current, previous decimal.Decimal, status string) FinanceOverviewMetric {
	metric := FinanceOverviewMetric{Amount: financeMoney(current), Currency: "USD", PreviousAmount: financeMoney(previous), Status: status}
	if !previous.IsZero() {
		rate := current.Sub(previous).Div(previous.Abs()).Round(4).StringFixed(4)
		metric.ChangeRate = &rate
	}
	return metric
}

func financeQualityFromSummary(summary *FinanceSummaryFacts) FinanceQuality {
	missing := summary.MissingProfileCount + summary.MissingPriceCount + summary.MissingMultiplierCount + summary.MissingUsageCount + summary.UnsupportedUsageCount
	eligible := summary.ExactCount + summary.EstimatedCount + missing
	coverage := decimal.Zero
	if eligible > 0 {
		coverage = decimal.NewFromInt(summary.ExactCount).Div(decimal.NewFromInt(eligible))
	}
	status := "complete"
	// Excluded rows are intentionally not part of revenue or profit. They still
	// make a period incomplete, otherwise a historical period containing only
	// legacy/unverified rows would be reported as "complete" with zero profit.
	if missing > 0 || summary.EstimatedCount > 0 || summary.ExcludedCount > 0 {
		status = "partial"
	}
	return FinanceQuality{
		Status: status, ExactCount: summary.ExactCount, EstimatedCount: summary.EstimatedCount,
		MissingProfileCount: summary.MissingProfileCount, MissingPriceCount: summary.MissingPriceCount,
		MissingMultiplierCount: summary.MissingMultiplierCount, MissingUsageCount: summary.MissingUsageCount,
		UnsupportedUsageCount: summary.UnsupportedUsageCount, NonBillableCount: summary.NonBillableCount,
		ExcludedCount:   summary.ExcludedCount,
		UnpricedRevenue: financeMoney(summary.UnpricedRevenue), CostCoverageRate: coverage.Round(4).StringFixed(4),
	}
}

func financeMoney(value decimal.Decimal) string { return value.Round(8).StringFixed(8) }

func defaultFinanceValue(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func financeAllowed(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func validFinanceCurrency(value string) bool {
	if len(value) != 3 {
		return false
	}
	for i := range value {
		if value[i] < 'A' || value[i] > 'Z' {
			return false
		}
	}
	return true
}
