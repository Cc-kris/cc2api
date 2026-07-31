//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type accountFinanceProfileServiceRepoStub struct {
	evidence AccountFinanceReadinessEvidence
	saved    *AccountFinanceProfile
}

func (s *accountFinanceProfileServiceRepoStub) CurrentAccountFinanceProfile(context.Context, int64) (*AccountFinanceProfile, error) {
	return nil, ErrAccountFinanceProfileNotFound
}

func (s *accountFinanceProfileServiceRepoStub) ReplaceAccountFinanceProfile(_ context.Context, _ int64, _ AccountFinanceProfileInput, profile AccountFinanceProfile) (*AccountFinanceProfile, error) {
	copy := profile
	s.saved = &copy
	return &copy, nil
}

func (s *accountFinanceProfileServiceRepoStub) AccountFinanceReadinessEvidence(context.Context, int64, *AccountFinanceProfile) (AccountFinanceReadinessEvidence, error) {
	return s.evidence, nil
}

func TestAccountFinanceProfileSaveUsesAccountMultiplierWhenContractOverrideIsBlank(t *testing.T) {
	multiplier := decimal.RequireFromString("0.22")
	changeID := int64(91)
	repo := &accountFinanceProfileServiceRepoStub{evidence: AccountFinanceReadinessEvidence{
		AccountMultiplier: &multiplier, AccountMultiplierChangeID: &changeID,
	}}
	svc := NewAccountFinanceProfileService(repo)

	profile, err := svc.Save(context.Background(), 7, AccountFinanceProfileInput{
		CostMode: FinanceCostModeContractMultiplier, EndpointSource: "account_base_url",
		CounterScope: FinanceCounterScopeAccount, BalanceUnitSemantics: FinanceUnitNone,
		EffectiveFrom: time.Now().UTC(), ExpectedVersion: 0, Reason: "使用账号上游倍率", OperatorID: 3,
	})
	require.NoError(t, err)
	require.NotNil(t, profile)
	require.NotNil(t, repo.saved)
	require.True(t, repo.saved.ContractMultiplier.Equal(multiplier))
	require.Equal(t, "multiplier", *repo.saved.ContractType)
	require.Equal(t, changeID, *repo.saved.ContractMultiplierChangeID)
	require.Equal(t, AccountFinanceReadinessReadyContract, repo.saved.ReadinessStatus)
}

func TestAccountFinanceProfileSaveAllowsUnconfiguredAccountMultiplier(t *testing.T) {
	repo := &accountFinanceProfileServiceRepoStub{}
	svc := NewAccountFinanceProfileService(repo)

	profile, err := svc.Save(context.Background(), 7, AccountFinanceProfileInput{
		CostMode: FinanceCostModeContractMultiplier, EndpointSource: "account_base_url",
		CounterScope: FinanceCounterScopeAccount, BalanceUnitSemantics: FinanceUnitNone,
		EffectiveFrom: time.Now().UTC(), ExpectedVersion: 0, Reason: "保留未配置状态", OperatorID: 3,
	})
	require.NoError(t, err)
	require.Nil(t, profile.ContractMultiplier)
	require.Nil(t, profile.ContractType)
	require.Equal(t, AccountFinanceReadinessUnconfigured, profile.ReadinessStatus)
}
