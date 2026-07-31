//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestFinanceDetailServiceCalculatesProfitAndSegments(t *testing.T) {
	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	revenue := decimal.RequireFromString("2.0000000000")
	cost := decimal.RequireFromString("1.2500000000")
	multiplier := decimal.RequireFromString("1.2500")
	usageValue := decimal.RequireFromString("2.0000000000")
	pricingVersion := "v2"
	pricingSource := "system"
	checksum := "sha256:test"
	repository := &financeDetailRepositoryStub{facts: &FinanceUsageDetailFacts{
		Usage: &UsageLog{
			ID: 1, RequestID: "req", Model: "model-a", RequestedModel: "model-a", CreatedAt: now,
			UsageListValue: &usageValue, SalesPricingVersion: &pricingVersion, SalesPricingSource: &pricingSource,
			SalesPricingChecksum: &checksum, SalesPricingSnapshot: map[string]any{"multiplier": "1.1000"},
		},
		Projection: &UsageFinanceProjection{
			UsageLogID: 1, UpstreamCost: &cost, CostStatus: FinanceCostStatusExact,
			PricingSource: FinancePricingSourceUpstreamExact, UpstreamCostMultiplierSnapshot: &multiplier,
			CurrentRevision: 1, CalculatedAt: now,
			Segments: []UsageFinanceCostSegment{{
				AttemptNo: 1, AccountID: 9, UpstreamModel: "model-upstream", CostStatus: FinanceCostStatusExact,
				CostAmount: &cost, CalculationDetail: map[string]any{"items": []FinanceCostItem{{Item: "input", Quantity: 100}}},
			}},
		},
		Revenue: &revenue, AccountNames: map[int64]string{9: "上游账号"},
	}}
	service := NewFinanceDetailService(repository)

	detail, err := service.GetUsageDetail(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "0.7500000000", *detail.Profit.Amount)
	require.Equal(t, "0.3750", *detail.Profit.MarginRate)
	require.Equal(t, "1.2500000000", *detail.Cost.Amount)
	require.Equal(t, "1.2500", *detail.Cost.UpstreamMultiplierSnapshot)
	require.Equal(t, "1.1000", detail.Sales.GroupMultiplier)
	require.Len(t, detail.CostSegments, 1)
	require.Equal(t, "上游账号", detail.CostSegments[0].AccountName)
	require.Len(t, detail.UsageItems, 1)
}

func TestFinanceDetailServiceDoesNotForgeProfitWhenCostMissing(t *testing.T) {
	revenue := decimal.NewFromInt(2)
	repository := &financeDetailRepositoryStub{facts: &FinanceUsageDetailFacts{
		Usage:      &UsageLog{ID: 1, RequestID: "req", Model: "model-a", CreatedAt: time.Now()},
		Projection: &UsageFinanceProjection{UsageLogID: 1, CostStatus: FinanceCostStatusMissingPrice},
		Revenue:    &revenue,
	}}
	detail, err := NewFinanceDetailService(repository).GetUsageDetail(context.Background(), 1)
	require.NoError(t, err)
	require.Nil(t, detail.Cost.Amount)
	require.Nil(t, detail.Profit.Amount)
	require.Contains(t, detail.QualityIssues, "missing_price")
}

type financeDetailRepositoryStub struct {
	facts *FinanceUsageDetailFacts
	err   error
}

func (s *financeDetailRepositoryStub) GetUsageFinanceDetailFacts(context.Context, int64) (*FinanceUsageDetailFacts, error) {
	return s.facts, s.err
}
