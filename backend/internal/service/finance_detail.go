package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type FinanceUsageDetailFacts struct {
	Usage        *UsageLog
	Projection   *UsageFinanceProjection
	Revenue      *decimal.Decimal
	AccountNames map[int64]string
}

type FinanceDetailRepository interface {
	GetUsageFinanceDetailFacts(ctx context.Context, usageLogID int64) (*FinanceUsageDetailFacts, error)
}

type FinanceDetailService struct {
	repository FinanceDetailRepository
}

func NewFinanceDetailService(repository FinanceDetailRepository) *FinanceDetailService {
	return &FinanceDetailService{repository: repository}
}

type FinanceDetailMoney struct {
	Amount     *string `json:"amount"`
	MarginRate *string `json:"margin_rate,omitempty"`
}

type FinanceDetailSales struct {
	PricingVersion    string  `json:"pricing_version,omitempty"`
	PricingSource     string  `json:"pricing_source,omitempty"`
	PricingChecksum   string  `json:"pricing_checksum,omitempty"`
	GroupMultiplier   string  `json:"group_multiplier,omitempty"`
	UsageListValue    *string `json:"usage_list_value"`
	RecognizedRevenue *string `json:"recognized_revenue"`
}

type FinanceDetailCost struct {
	Amount                     *string           `json:"amount"`
	Status                     FinanceCostStatus `json:"status"`
	PricingSource              string            `json:"pricing_source,omitempty"`
	UpstreamMultiplierSnapshot *string           `json:"upstream_multiplier_snapshot"`
	UpstreamMultiplierChangeID *int64            `json:"upstream_multiplier_change_id,omitempty"`
	AccountFinanceProfileID    *int64            `json:"account_finance_profile_id,omitempty"`
	FXRateVersionID            *int64            `json:"fx_rate_version_id,omitempty"`
	SourceCurrency             string            `json:"source_currency,omitempty"`
	FXRateToUSD                *string           `json:"fx_rate_to_usd,omitempty"`
	FXSource                   string            `json:"fx_source,omitempty"`
	FXObservedAt               *time.Time        `json:"fx_observed_at,omitempty"`
	CalculatedAt               *time.Time        `json:"calculated_at"`
	CurrentRevision            int               `json:"current_revision"`
}

type FinanceDetailSegment struct {
	AttemptNo                  int               `json:"attempt_no"`
	AccountID                  int64             `json:"account_id"`
	AccountName                string            `json:"account_name,omitempty"`
	ChannelID                  *int64            `json:"channel_id"`
	WalletID                   *int64            `json:"wallet_id"`
	UpstreamID                 *int64            `json:"upstream_id"`
	UpstreamModel              string            `json:"upstream_model"`
	Billable                   bool              `json:"billable"`
	CostStatus                 FinanceCostStatus `json:"cost_status"`
	CostAmount                 *string           `json:"cost_amount"`
	UpstreamMultiplierChangeID *int64            `json:"upstream_multiplier_change_id,omitempty"`
	AccountFinanceProfileID    *int64            `json:"account_finance_profile_id,omitempty"`
	FXRateVersionID            *int64            `json:"fx_rate_version_id,omitempty"`
	SourceCurrency             string            `json:"source_currency,omitempty"`
	FXRateToUSD                *string           `json:"fx_rate_to_usd,omitempty"`
	FXSource                   string            `json:"fx_source,omitempty"`
	FXObservedAt               *time.Time        `json:"fx_observed_at,omitempty"`
}

type FinanceUsageDetail struct {
	UsageLogID     int64                  `json:"usage_log_id"`
	RequestID      string                 `json:"request_id"`
	UsageCreatedAt time.Time              `json:"usage_created_at"`
	RequestedModel string                 `json:"requested_model"`
	UpstreamModel  string                 `json:"upstream_model,omitempty"`
	ServiceTier    string                 `json:"service_tier,omitempty"`
	BillingType    string                 `json:"billing_type"`
	BusinessType   string                 `json:"business_type"`
	Sales          FinanceDetailSales     `json:"sales"`
	Cost           FinanceDetailCost      `json:"cost"`
	Profit         FinanceDetailMoney     `json:"profit"`
	UsageItems     []any                  `json:"usage_items"`
	CostSegments   []FinanceDetailSegment `json:"cost_segments"`
	QualityIssues  []string               `json:"quality_issues"`
}

func (s *FinanceDetailService) GetUsageDetail(ctx context.Context, usageLogID int64) (*FinanceUsageDetail, error) {
	if usageLogID <= 0 {
		return nil, ErrUsageLogNotFound
	}
	facts, err := s.repository.GetUsageFinanceDetailFacts(ctx, usageLogID)
	if err != nil {
		return nil, err
	}
	if facts == nil || facts.Usage == nil {
		return nil, ErrUsageLogNotFound
	}
	usage := facts.Usage
	requestedModel := strings.TrimSpace(usage.RequestedModel)
	if requestedModel == "" {
		requestedModel = strings.TrimSpace(usage.Model)
	}
	detail := &FinanceUsageDetail{
		UsageLogID:     usage.ID,
		RequestID:      usage.RequestID,
		UsageCreatedAt: usage.CreatedAt,
		RequestedModel: requestedModel,
		UpstreamModel:  dereferenceString(usage.UpstreamModel),
		ServiceTier:    dereferenceString(usage.ServiceTier),
		BillingType:    financeBillingModeForUsage(usage),
		BusinessType:   "api",
		Sales: FinanceDetailSales{
			PricingVersion:    dereferenceString(usage.SalesPricingVersion),
			PricingSource:     dereferenceString(usage.SalesPricingSource),
			PricingChecksum:   dereferenceString(usage.SalesPricingChecksum),
			GroupMultiplier:   financeSalesMultiplier(usage.SalesPricingSnapshot),
			UsageListValue:    financeDecimalStringPointer(usage.UsageListValue),
			RecognizedRevenue: financeDecimalStringPointer(facts.Revenue),
		},
		Cost:          FinanceDetailCost{Status: FinanceCostStatusMissingUsage},
		Profit:        FinanceDetailMoney{},
		UsageItems:    []any{},
		CostSegments:  []FinanceDetailSegment{},
		QualityIssues: []string{},
	}
	if detail.UpstreamModel == "" {
		detail.UpstreamModel = strings.TrimSpace(usage.Model)
	}
	projection := facts.Projection
	if projection == nil {
		detail.QualityIssues = append(detail.QualityIssues, "finance_calculation_pending")
		return detail, nil
	}
	detail.Cost = FinanceDetailCost{
		Amount:                     financeDecimalStringPointer(projection.UpstreamCost),
		Status:                     projection.CostStatus,
		PricingSource:              projection.PricingSource,
		UpstreamMultiplierSnapshot: financeDecimalFixedPointer(projection.UpstreamCostMultiplierSnapshot, 4),
		UpstreamMultiplierChangeID: cloneInt64Pointer(projection.UpstreamMultiplierChangeID),
		AccountFinanceProfileID:    cloneInt64Pointer(projection.AccountFinanceProfileID),
		FXRateVersionID:            cloneInt64Pointer(projection.FXRateVersionID),
		SourceCurrency:             projection.SourceCurrency,
		FXRateToUSD:                financeDecimalStringPointer(projection.FXRateToUSD),
		FXSource:                   projection.FXSource,
		FXObservedAt:               cloneFinanceTime(projection.FXObservedAt),
		CalculatedAt:               &projection.CalculatedAt,
		CurrentRevision:            projection.CurrentRevision,
	}
	for _, segment := range projection.Segments {
		billable := segment.CostStatus != FinanceCostStatusNonBillable
		detail.CostSegments = append(detail.CostSegments, FinanceDetailSegment{
			AttemptNo:                  segment.AttemptNo,
			AccountID:                  segment.AccountID,
			AccountName:                facts.AccountNames[segment.AccountID],
			ChannelID:                  cloneInt64Pointer(segment.ChannelID),
			WalletID:                   cloneInt64Pointer(segment.WalletID),
			UpstreamID:                 cloneInt64Pointer(segment.UpstreamID),
			UpstreamModel:              segment.UpstreamModel,
			Billable:                   billable,
			CostStatus:                 segment.CostStatus,
			CostAmount:                 financeDecimalStringPointer(segment.CostAmount),
			UpstreamMultiplierChangeID: cloneInt64Pointer(segment.UpstreamMultiplierChangeID),
			AccountFinanceProfileID:    cloneInt64Pointer(segment.AccountFinanceProfileID),
			FXRateVersionID:            cloneInt64Pointer(segment.FXRateVersionID),
			SourceCurrency:             segment.SourceCurrency,
			FXRateToUSD:                financeDecimalStringPointer(segment.FXRateToUSD),
			FXSource:                   segment.FXSource,
			FXObservedAt:               cloneFinanceTime(segment.FXObservedAt),
		})
		if items, ok := segment.CalculationDetail["items"].([]any); ok {
			detail.UsageItems = append(detail.UsageItems, items...)
		} else if typed, ok := segment.CalculationDetail["items"].([]FinanceCostItem); ok {
			for _, item := range typed {
				detail.UsageItems = append(detail.UsageItems, item)
			}
		}
	}
	if projection.UpstreamCost == nil {
		detail.QualityIssues = append(detail.QualityIssues, string(projection.CostStatus))
		return detail, nil
	}
	if facts.Revenue == nil {
		detail.QualityIssues = append(detail.QualityIssues, "missing_revenue")
		return detail, nil
	}
	profit := facts.Revenue.Sub(*projection.UpstreamCost)
	detail.Profit.Amount = financeDecimalFixedPointer(&profit, 10)
	if !facts.Revenue.IsZero() {
		margin := profit.Div(*facts.Revenue).Round(4)
		detail.Profit.MarginRate = financeDecimalFixedPointer(&margin, 4)
	}
	return detail, nil
}

func financeDecimalStringPointer(value *decimal.Decimal) *string {
	return financeDecimalFixedPointer(value, 10)
}

func financeDecimalFixedPointer(value *decimal.Decimal, scale int32) *string {
	if value == nil {
		return nil
	}
	formatted := value.StringFixed(scale)
	return &formatted
}

func financeSalesMultiplier(snapshot map[string]any) string {
	if snapshot == nil {
		return ""
	}
	for _, key := range []string{"group_multiplier", "multiplier", "rate_multiplier"} {
		if value, ok := snapshot[key]; ok {
			parsed, err := ParseFinanceDecimal(value)
			if err == nil && parsed != nil {
				return parsed.StringFixed(4)
			}
		}
	}
	return ""
}

func ValidateFinanceUsageDetail(detail *FinanceUsageDetail) error {
	if detail == nil || detail.UsageLogID <= 0 {
		return fmt.Errorf("invalid finance usage detail")
	}
	return nil
}
