package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPersistFundingTransactionsKeepsRechargeSeparateFromMultiplier(t *testing.T) {
	walletRepo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, UpstreamID: 2, Enabled: true, Currency: "CNY"}}
	fundRepo := &upstreamFundRepoStub{created: true}
	funds := NewUpstreamFundService(NewUpstreamWalletService(walletRepo, upstreamWalletEncryptorStub{}), fundRepo)
	svc := &UpstreamFinanceSyncService{funds: funds}

	created, skipped, err := svc.persistFundingTransactions(context.Background(), 7, []FinanceFundingTransactionFact{{
		TransactionID: "tx-cny-1", PaidAmount: "500", PaidCurrency: "CNY", FXRateToUSD: "0.138",
		FXSource: "bank_receipt", FXObservedAt: "2026-07-29T08:00:00Z", BaseCreditUnits: "5000",
		BonusCreditUnits: "500", OccurredAt: "2026-07-29T08:00:00Z",
	}})

	require.NoError(t, err)
	require.Equal(t, int64(1), created)
	require.Equal(t, int64(0), skipped)
	require.Equal(t, "tx-cny-1", fundRepo.event.ReferenceNo)
	require.Equal(t, "bank_receipt", fundRepo.event.FXSource)
	require.Equal(t, "10", fundRepo.event.BaseRechargeRatio.String())
	require.Equal(t, "11", fundRepo.event.EffectiveRechargeRatio.String())
	require.Equal(t, "50", fundRepo.event.BonusIncomeOriginal.String())
}

func TestPersistFundingTransactionsRequiresFrozenFXForNonUSD(t *testing.T) {
	walletRepo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, UpstreamID: 2, Enabled: true, Currency: "CNY"}}
	fundRepo := &upstreamFundRepoStub{created: true}
	funds := NewUpstreamFundService(NewUpstreamWalletService(walletRepo, upstreamWalletEncryptorStub{}), fundRepo)
	svc := &UpstreamFinanceSyncService{funds: funds}

	_, _, err := svc.persistFundingTransactions(context.Background(), 7, []FinanceFundingTransactionFact{{
		TransactionID: "tx-cny-2", PaidAmount: "500", PaidCurrency: "CNY", BaseCreditUnits: "5000",
		BonusCreditUnits: "500", OccurredAt: "2026-07-29T08:00:00Z",
	}})

	require.ErrorContains(t, err, "requires frozen fx_rate_to_usd and fx_source")
	require.Nil(t, fundRepo.event)
}

func TestPersistFundingTransactionsUsesIdentityFXForUSD(t *testing.T) {
	walletRepo := &upstreamWalletRepoStub{wallet: &UpstreamWallet{ID: 7, UpstreamID: 2, Enabled: true, Currency: "USD"}}
	fundRepo := &upstreamFundRepoStub{created: true}
	funds := NewUpstreamFundService(NewUpstreamWalletService(walletRepo, upstreamWalletEncryptorStub{}), fundRepo)
	svc := &UpstreamFinanceSyncService{funds: funds}

	created, _, err := svc.persistFundingTransactions(context.Background(), 7, []FinanceFundingTransactionFact{{
		TransactionID: "tx-usd-1", PaidAmount: "100", PaidCurrency: "USD", BaseCreditUnits: "1000",
		BonusCreditUnits: "100", OccurredAt: "2026-07-29T08:00:00Z",
	}})

	require.NoError(t, err)
	require.Equal(t, int64(1), created)
	require.Equal(t, "1", fundRepo.event.FXRateToUSD.String())
	require.Equal(t, "currency_identity", fundRepo.event.FXSource)
}
