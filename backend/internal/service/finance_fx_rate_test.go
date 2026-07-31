package service

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type financeFXRateRepoStub struct {
	input    FinanceFXRateCreateInput
	rate     decimal.Decimal
	checksum string
}

func (s *financeFXRateRepoStub) ListFinanceFXRates(context.Context, string, int, int) ([]FinanceFXRateVersion, int64, error) {
	return nil, 0, nil
}

func (s *financeFXRateRepoStub) CreateFinanceFXRate(_ context.Context, input FinanceFXRateCreateInput, rate decimal.Decimal, checksum string) (*FinanceFXRateVersion, error) {
	s.input, s.rate, s.checksum = input, rate, checksum
	return &FinanceFXRateVersion{ID: 1, Currency: input.Currency, RateToUSD: rate.String(), Source: input.Source}, nil
}

func TestFinanceFXRateCreateNormalizesAndFreezesEvidence(t *testing.T) {
	repo := &financeFXRateRepoStub{}
	svc := NewFinanceFXRateService(repo)
	observed := time.Date(2026, 7, 30, 8, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	item, err := svc.Create(context.Background(), FinanceFXRateCreateInput{Currency: "cny", RateToUSD: "0.1380", ObservedAt: observed, ChangeReason: "月度财务汇率校准"})
	require.NoError(t, err)
	require.Equal(t, "CNY", item.Currency)
	require.Equal(t, "0.138", repo.rate.String())
	require.Equal(t, "manual_admin", repo.input.Source)
	require.Equal(t, observed.UTC(), repo.input.ObservedAt)
	require.Equal(t, "月度财务汇率校准", repo.input.ChangeReason)
	require.Equal(t, repo.input.ObservedAt, repo.input.EffectiveFrom)
	require.Len(t, repo.checksum, 64)
}

func TestFinanceFXRateCreateRejectsInvalidUSDIdentity(t *testing.T) {
	_, err := NewFinanceFXRateService(&financeFXRateRepoStub{}).Create(context.Background(), FinanceFXRateCreateInput{Currency: "USD", RateToUSD: "0.9", ChangeReason: "测试原因"})
	require.EqualError(t, err, "USD rate_to_usd must equal 1")
}
