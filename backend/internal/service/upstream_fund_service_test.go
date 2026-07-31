package service

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type upstreamFundRepoStub struct {
	event    *UpstreamFundEvent
	created  bool
	existing *UpstreamFundEvent
}

func (s *upstreamFundRepoStub) CreateFundEvent(_ context.Context, event *UpstreamFundEvent) (bool, error) {
	s.event = event
	if !s.created && s.existing != nil {
		*event = *s.existing
	}
	event.ID = 15
	return s.created, nil
}

func (s *upstreamFundRepoStub) GetFundEvent(_ context.Context, walletID, eventID int64) (*UpstreamFundEvent, error) {
	if s.existing == nil || s.existing.WalletID != walletID || s.existing.ID != eventID {
		return nil, ErrUpstreamFundEventNotFound
	}
	return s.existing, nil
}

func (s *upstreamFundRepoStub) ListFundEvents(_ context.Context, _ int64, _, _ int) ([]UpstreamFundEvent, int64, error) {
	if s.existing == nil {
		return nil, 0, nil
	}
	return []UpstreamFundEvent{*s.existing}, 1, nil
}

func TestUpstreamFundServiceRejectsIdempotencyKeyReusedWithDifferentPayload(t *testing.T) {
	walletRepo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, UpstreamID: 2, Enabled: true}}
	occurredAt := time.Now().UTC().Truncate(time.Microsecond)
	fundRepo := &upstreamFundRepoStub{existing: &UpstreamFundEvent{
		WalletID: 7, EventType: "topup", OriginalAmount: decimal.NewFromInt(500), Currency: "CNY",
		FXRateToUSD: decimal.RequireFromString("0.1378"), USDAmount: decimal.RequireFromString("68.9"),
		OccurredAt: occurredAt, ReferenceNo: "topup-existing", Note: "different upstream topup", IdempotencyKey: "fund-reused",
	}}
	svc := NewUpstreamFundService(NewUpstreamWalletService(walletRepo, upstreamWalletEncryptorStub{}), fundRepo)
	_, created, err := svc.Create(context.Background(), 7, UpstreamFundEventInput{
		EventType: "topup", OriginalAmount: "1000", Currency: "CNY", FXRateToUSD: "0.1378",
		USDAmount: "137.8", OccurredAt: occurredAt, ReferenceNo: "topup-request", Note: "upstream wallet topup", IdempotencyKey: "fund-reused",
	})
	require.False(t, created)
	require.ErrorIs(t, err, ErrUpstreamFundIdempotencyConflict)
}

func TestUpstreamFundServiceValidatesAndPersistsDecimalAmounts(t *testing.T) {
	walletRepo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, UpstreamID: 2, Enabled: true}}
	walletSvc := NewUpstreamWalletService(walletRepo, upstreamWalletEncryptorStub{})
	fundRepo := &upstreamFundRepoStub{created: true}
	svc := NewUpstreamFundService(walletSvc, fundRepo)
	event, created, err := svc.Create(context.Background(), 7, UpstreamFundEventInput{
		EventType: "topup", OriginalAmount: "1000", Currency: "cny", FXRateToUSD: "0.1378",
		USDAmount: "137.80000000", OccurredAt: time.Now().UTC(), ReferenceNo: "topup-001", Note: "upstream wallet topup", IdempotencyKey: "fund-001",
	})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(15), event.ID)
	require.True(t, event.USDAmount.Equal(decimal.RequireFromString("137.8")))
	require.Equal(t, "CNY", event.Currency)
}

func TestUpstreamFundServiceRejectsInconsistentUSD(t *testing.T) {
	walletRepo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, UpstreamID: 2, Enabled: true}}
	svc := NewUpstreamFundService(NewUpstreamWalletService(walletRepo, upstreamWalletEncryptorStub{}), &upstreamFundRepoStub{})
	_, _, err := svc.Create(context.Background(), 7, UpstreamFundEventInput{
		EventType: "topup", OriginalAmount: "1000", Currency: "CNY", FXRateToUSD: "0.1378",
		USDAmount: "138", OccurredAt: time.Now().UTC(), Note: "upstream wallet topup", IdempotencyKey: "fund-002",
	})
	require.EqualError(t, err, "usd_amount does not match original_amount multiplied by fx_rate_to_usd")
}

func TestUpstreamFundServiceRequiresReferenceAndEnabledWallet(t *testing.T) {
	input := UpstreamFundEventInput{
		EventType: "topup", OriginalAmount: "10", Currency: "USD", FXRateToUSD: "1", USDAmount: "10",
		OccurredAt: time.Now().UTC(), Note: "upstream wallet topup", IdempotencyKey: "fund-reference-required",
	}
	disabled := NewUpstreamFundService(NewUpstreamWalletService(&upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, Enabled: false}}, upstreamWalletEncryptorStub{}), &upstreamFundRepoStub{})
	_, _, err := disabled.Create(context.Background(), 7, input)
	require.ErrorIs(t, err, ErrUpstreamWalletDisabled)

	enabled := NewUpstreamFundService(NewUpstreamWalletService(&upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, Enabled: true}}, upstreamWalletEncryptorStub{}), &upstreamFundRepoStub{})
	_, _, err = enabled.Create(context.Background(), 7, input)
	require.EqualError(t, err, "reference_no is required for topup and refund events")
}

func TestUpstreamFundServiceCalculatesRechargeBonusWithoutChangingUsageMultiplier(t *testing.T) {
	walletRepo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, UpstreamID: 2, Enabled: true}}
	fundRepo := &upstreamFundRepoStub{created: true}
	svc := NewUpstreamFundService(NewUpstreamWalletService(walletRepo, upstreamWalletEncryptorStub{}), fundRepo)
	event, created, err := svc.Create(context.Background(), 7, UpstreamFundEventInput{
		EventType: "topup", OriginalAmount: "500", Currency: "CNY", FXRateToUSD: "0.1378", USDAmount: "68.9",
		BaseCreditUnits: "5000", BonusCreditUnits: "500", OccurredAt: time.Now().UTC(), ReferenceNo: "topup-bonus-001", Note: "1:10 充值赠送", IdempotencyKey: "fund-bonus-001",
	})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, "confirmed", event.BonusStatus)
	require.True(t, event.BaseRechargeRatio.Equal(decimal.NewFromInt(10)))
	require.True(t, event.EffectiveRechargeRatio.Equal(decimal.NewFromInt(11)))
	require.True(t, event.BonusIncomeOriginal.Equal(decimal.NewFromInt(50)))
	require.True(t, event.BonusIncomeUSD.Equal(decimal.RequireFromString("6.89")))
	require.Nil(t, event.ReversedEventID)
}

func TestUpstreamFundServiceMarksUnknownRechargeBonusAsUnresolved(t *testing.T) {
	walletRepo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, UpstreamID: 2, Enabled: true}}
	svc := NewUpstreamFundService(NewUpstreamWalletService(walletRepo, upstreamWalletEncryptorStub{}), &upstreamFundRepoStub{created: true})
	event, _, err := svc.Create(context.Background(), 7, UpstreamFundEventInput{
		EventType: "topup", OriginalAmount: "10", Currency: "USD", FXRateToUSD: "1", USDAmount: "10",
		OccurredAt: time.Now().UTC(), ReferenceNo: "topup-bonus-002", Note: "未提供到账额度", IdempotencyKey: "fund-bonus-002",
	})
	require.NoError(t, err)
	require.Equal(t, "unresolved", event.BonusStatus)
	require.Nil(t, event.BonusIncomeUSD)
}

func TestUpstreamFundServiceReversesRechargeBonusWithFullRefund(t *testing.T) {
	base := decimal.NewFromInt(5000)
	bonus := decimal.NewFromInt(500)
	total := decimal.NewFromInt(5500)
	baseRatio := decimal.NewFromInt(10)
	effectiveRatio := decimal.NewFromInt(11)
	bonusOriginal := decimal.NewFromInt(50)
	bonusUSD := decimal.RequireFromString("6.89")
	original := &UpstreamFundEvent{ID: 77, WalletID: 7, EventType: "topup", OriginalAmount: decimal.NewFromInt(500), Currency: "CNY", FXRateToUSD: decimal.RequireFromString("0.1378"), USDAmount: decimal.RequireFromString("68.9"), BonusStatus: "confirmed", BaseCreditUnits: &base, BonusCreditUnits: &bonus, TotalCreditUnits: &total, BaseRechargeRatio: &baseRatio, EffectiveRechargeRatio: &effectiveRatio, BonusIncomeOriginal: &bonusOriginal, BonusIncomeUSD: &bonusUSD}
	walletRepo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, UpstreamID: 2, Enabled: true}}
	svc := NewUpstreamFundService(NewUpstreamWalletService(walletRepo, upstreamWalletEncryptorStub{}), &upstreamFundRepoStub{created: true, existing: original})
	event, _, err := svc.Create(context.Background(), 7, UpstreamFundEventInput{
		EventType: "refund", OriginalAmount: "500", Currency: "CNY", FXRateToUSD: "0.1378", USDAmount: "68.9", ReversedEventID: upstreamFundTestInt64(77),
		OccurredAt: time.Now().UTC(), ReferenceNo: "refund-bonus-003", Note: "全额退款冲正赠送", IdempotencyKey: "fund-bonus-003",
	})
	require.NoError(t, err)
	require.Equal(t, "reversed", event.BonusStatus)
	require.True(t, event.BonusIncomeOriginal.Equal(decimal.NewFromInt(-50)))
	require.True(t, event.BonusIncomeUSD.Equal(decimal.RequireFromString("-6.89")))
}

func TestUpstreamFundServiceRejectsRefundCurrencyMismatch(t *testing.T) {
	original := &UpstreamFundEvent{ID: 77, WalletID: 7, EventType: "topup", OriginalAmount: decimal.NewFromInt(500), Currency: "CNY", BonusStatus: "confirmed"}
	walletRepo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, UpstreamID: 2, Enabled: true}}
	svc := NewUpstreamFundService(NewUpstreamWalletService(walletRepo, upstreamWalletEncryptorStub{}), &upstreamFundRepoStub{created: true, existing: original})
	_, _, err := svc.Create(context.Background(), 7, UpstreamFundEventInput{
		EventType: "refund", OriginalAmount: "500", Currency: "USD", FXRateToUSD: "1", USDAmount: "500", ReversedEventID: upstreamFundTestInt64(77),
		OccurredAt: time.Now().UTC(), ReferenceNo: "refund-wrong-currency", Note: "refund wrong currency", IdempotencyKey: "fund-refund-currency",
	})
	require.EqualError(t, err, "refund currency must equal the reversed topup currency")
}

func upstreamFundTestInt64(value int64) *int64 { return &value }
