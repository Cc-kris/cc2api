package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type financeInitializationAccountStub struct {
	accounts []Account
	updates  []UpdateAccountInput
}

func (s *financeInitializationAccountStub) ListAccounts(context.Context, int, int, string, string, string, string, int64, string, string, string) ([]Account, int64, error) {
	return append([]Account(nil), s.accounts...), int64(len(s.accounts)), nil
}

func (s *financeInitializationAccountStub) UpdateAccount(_ context.Context, id int64, input *UpdateAccountInput) (*Account, error) {
	for index := range s.accounts {
		if s.accounts[index].ID == id {
			copy := *input
			s.updates = append(s.updates, copy)
			s.accounts[index].UpstreamCostMultiplier = input.UpstreamCostMultiplier
			return &s.accounts[index], nil
		}
	}
	return nil, errors.New("account not found")
}

type financeInitializationUpstreamStub struct {
	items []*Upstream
	syncs int
}

func (s *financeInitializationUpstreamStub) List(context.Context) ([]*Upstream, error) {
	return s.items, nil
}
func (s *financeInitializationUpstreamStub) SyncFromAccounts(context.Context) (int, error) {
	s.syncs++
	return 0, nil
}
func (s *financeInitializationUpstreamStub) Get(_ context.Context, id int64) (*Upstream, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, errors.New("upstream not found")
}
func (s *financeInitializationUpstreamStub) Update(_ context.Context, id int64, input *UpstreamInput) (*Upstream, error) {
	item, err := s.Get(context.Background(), id)
	if err != nil {
		return nil, err
	}
	item.InitialBalance = input.InitialBalance
	item.CurrentBalance = input.InitialBalance - item.ConsumedBalance
	item.RateMultiplier = input.RateMultiplier
	return item, nil
}

type financeInitializationWalletStub struct {
	byUpstream map[int64][]UpstreamWallet
	created    int
}

func (s *financeInitializationWalletStub) List(_ context.Context, upstreamID int64, _ bool) ([]UpstreamWallet, error) {
	return append([]UpstreamWallet(nil), s.byUpstream[upstreamID]...), nil
}
func (s *financeInitializationWalletStub) Create(_ context.Context, upstreamID int64, input UpstreamWalletInput) (*UpstreamWallet, error) {
	s.created++
	wallet := UpstreamWallet{ID: int64(100 + s.created), UpstreamID: upstreamID, Name: input.Name, AdapterType: input.AdapterType, Currency: input.Currency, BalanceKind: input.BalanceKind, Enabled: input.Enabled != nil && *input.Enabled}
	s.byUpstream[upstreamID] = append(s.byUpstream[upstreamID], wallet)
	return &wallet, nil
}

type financeInitializationFundStub struct{ calls []UpstreamFundEventInput }

func (s *financeInitializationFundStub) RecordBalanceSnapshot(_ context.Context, _ int64, amount decimal.Decimal, currency string, occurredAt time.Time, dedupeKey string) error {
	s.calls = append(s.calls, UpstreamFundEventInput{EventType: "balance_snapshot", OriginalAmount: amount.String(), Currency: currency, OccurredAt: occurredAt, IdempotencyKey: dedupeKey})
	return nil
}

func (s *financeInitializationFundStub) InitializeOpeningBalance(_ context.Context, _ int64, amount decimal.Decimal, currency string, occurredAt time.Time, _ *int64, note, idempotencyKey string) (*UpstreamFundEvent, bool, error) {
	input := UpstreamFundEventInput{EventType: "opening_balance", OriginalAmount: amount.String(), Currency: currency, OccurredAt: occurredAt, Note: note, IdempotencyKey: idempotencyKey}
	s.calls = append(s.calls, input)
	return &UpstreamFundEvent{ID: int64(len(s.calls)), OriginalAmount: amount, Currency: currency, EventType: input.EventType, OccurredAt: occurredAt}, true, nil
}

type financeInitializationProfileStub struct {
	items map[int64]*AccountFinanceProfile
	saves []AccountFinanceProfileInput
}

func (s *financeInitializationProfileStub) Get(_ context.Context, accountID int64) (*AccountFinanceProfile, error) {
	if item := s.items[accountID]; item != nil {
		return item, nil
	}
	return nil, ErrAccountFinanceProfileNotFound
}
func (s *financeInitializationProfileStub) Save(_ context.Context, accountID int64, input AccountFinanceProfileInput) (*AccountFinanceProfile, error) {
	s.saves = append(s.saves, input)
	item := &AccountFinanceProfile{AccountID: accountID, Version: input.ExpectedVersion + 1, EffectiveFrom: input.EffectiveFrom, CostMode: input.CostMode}
	s.items[accountID] = item
	return item, nil
}

func TestFinanceInitializationScanGroupsAccountsAndShowsOnlyRequiredInput(t *testing.T) {
	multiplier := decimal.RequireFromString("0.22")
	accounts := &financeInitializationAccountStub{accounts: []Account{{ID: 1, Name: "known", Platform: "openai", Status: StatusActive, UpstreamCostMultiplier: &multiplier}, {ID: 2, Name: "missing", Platform: "openai", Status: StatusActive}}}
	upstreams := &financeInitializationUpstreamStub{items: []*Upstream{{ID: 9, Name: "openai", BaseURL: "https://api.openai.com", NormalizedBaseURL: "https://api.openai.com", CurrentBalance: 12}}}
	wallets := &financeInitializationWalletStub{byUpstream: map[int64][]UpstreamWallet{9: {{ID: 8, Name: financeInitializationWalletName, BalanceKind: "wallet_cash"}}}}
	svc := NewFinanceInitializationService(accounts, upstreams, wallets, &financeInitializationFundStub{}, &financeInitializationProfileStub{items: map[int64]*AccountFinanceProfile{}})

	result, err := svc.Scan(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, upstreams.syncs)
	require.Len(t, result.Accounts, 2)
	require.Equal(t, "0.22", *result.Accounts[0].CurrentMultiplier)
	require.False(t, result.Accounts[0].NeedsMultiplierConfirm)
	require.True(t, result.Accounts[1].NeedsMultiplierConfirm)
	require.True(t, result.Upstreams[0].FinanceWalletSet)
}

func TestFinanceInitializationApplyCreatesContractProfileAndOpeningBalance(t *testing.T) {
	accounts := &financeInitializationAccountStub{accounts: []Account{{ID: 1, Name: "upstream account", Platform: "openai", Status: StatusActive, Credentials: map[string]any{"base_url": "https://upstream.test"}}}}
	upstreams := &financeInitializationUpstreamStub{items: []*Upstream{{ID: 5, Name: "upstream", BaseURL: "https://upstream.test", NormalizedBaseURL: "https://upstream.test", RateMultiplier: 0.6, ConsumedBalance: 3, CurrentBalance: 7}}}
	wallets := &financeInitializationWalletStub{byUpstream: map[int64][]UpstreamWallet{}}
	funds := &financeInitializationFundStub{}
	profiles := &financeInitializationProfileStub{items: map[int64]*AccountFinanceProfile{}}
	svc := NewFinanceInitializationService(accounts, upstreams, wallets, funds, profiles)
	svc.now = func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) }

	result, err := svc.Apply(context.Background(), FinanceInitializationRequest{OperatorID: 99, Reason: "首次财务初始化", Accounts: []FinanceInitializationAccountInput{{AccountID: 1, UpstreamCostMultiplier: "0.22"}}, Upstreams: []FinanceInitializationUpstreamInput{{UpstreamID: 5, CurrentBalance: 7}}})
	require.NoError(t, err)
	require.EqualValues(t, 1, result.InitializedAccounts)
	require.EqualValues(t, 1, result.InitializedUpstreams)
	require.EqualValues(t, 1, result.CreatedWallets)
	require.Len(t, accounts.updates, 1)
	require.Equal(t, "0.22", accounts.updates[0].UpstreamCostMultiplier.String())
	require.Len(t, profiles.saves, 1)
	require.Equal(t, FinanceCostModeContractMultiplier, profiles.saves[0].CostMode)
	require.Len(t, funds.calls, 1)
	require.Equal(t, "opening_balance", funds.calls[0].EventType)
	require.Equal(t, "7", funds.calls[0].OriginalAmount)
	require.Equal(t, 10.0, upstreams.items[0].InitialBalance)
	require.Equal(t, 7.0, upstreams.items[0].CurrentBalance)
	require.Equal(t, 0.6, upstreams.items[0].RateMultiplier)
}

func TestFinanceInitializationApplyPreservesExistingFinanceProfile(t *testing.T) {
	multiplier := decimal.RequireFromString("0.60")
	profileID := int64(42)
	accounts := &financeInitializationAccountStub{accounts: []Account{{ID: 1, Name: "request-charge account", Platform: "openai", Status: StatusActive, UpstreamCostMultiplier: &multiplier, CurrentFinanceProfileID: &profileID}}}
	profiles := &financeInitializationProfileStub{items: map[int64]*AccountFinanceProfile{1: {ID: profileID, AccountID: 1, Version: 3, CostMode: FinanceCostModeRequestCharge}}}
	upstreams := &financeInitializationUpstreamStub{}
	wallets := &financeInitializationWalletStub{byUpstream: map[int64][]UpstreamWallet{}}
	svc := NewFinanceInitializationService(accounts, upstreams, wallets, &financeInitializationFundStub{}, profiles)

	_, err := svc.Apply(context.Background(), FinanceInitializationRequest{OperatorID: 99, Reason: "确认已有财务档案", Accounts: []FinanceInitializationAccountInput{{AccountID: 1, UpstreamCostMultiplier: "0.60"}}})
	require.NoError(t, err)
	require.Empty(t, profiles.saves)
}

func TestFinanceInitializationApplyCanRecordBalanceWithoutFundEvent(t *testing.T) {
	accounts := &financeInitializationAccountStub{}
	upstreams := &financeInitializationUpstreamStub{items: []*Upstream{{ID: 5, Name: "upstream", BaseURL: "https://upstream.test", NormalizedBaseURL: "https://upstream.test", ConsumedBalance: 1}}}
	wallets := &financeInitializationWalletStub{byUpstream: map[int64][]UpstreamWallet{5: {{ID: 8, Name: financeInitializationWalletName, Currency: "CNY", BalanceKind: "wallet_cash", Enabled: true}}}}
	funds := &financeInitializationFundStub{}
	profiles := &financeInitializationProfileStub{items: map[int64]*AccountFinanceProfile{}}
	svc := NewFinanceInitializationService(accounts, upstreams, wallets, funds, profiles)
	recordOpening := false
	_, err := svc.Apply(context.Background(), FinanceInitializationRequest{OperatorID: 99, Reason: "修正当前余额", RecordOpeningBalance: &recordOpening, Upstreams: []FinanceInitializationUpstreamInput{{UpstreamID: 5, CurrentBalance: 12}}})
	require.NoError(t, err)
	require.Len(t, funds.calls, 1)
	require.Equal(t, "balance_snapshot", funds.calls[0].EventType)
	require.Equal(t, "CNY", funds.calls[0].Currency)
}

func TestFinanceInitializationApplyUsesSnapshotWhenFinanceWalletAlreadyExists(t *testing.T) {
	accounts := &financeInitializationAccountStub{}
	upstreams := &financeInitializationUpstreamStub{items: []*Upstream{{ID: 5, Name: "upstream", BaseURL: "https://upstream.test", NormalizedBaseURL: "https://upstream.test", ConsumedBalance: 1}}}
	wallets := &financeInitializationWalletStub{byUpstream: map[int64][]UpstreamWallet{5: {{ID: 8, Name: financeInitializationWalletName, Currency: "USD", BalanceKind: "wallet_cash", Enabled: true}}}}
	funds := &financeInitializationFundStub{}
	profiles := &financeInitializationProfileStub{items: map[int64]*AccountFinanceProfile{}}
	svc := NewFinanceInitializationService(accounts, upstreams, wallets, funds, profiles)

	_, err := svc.Apply(context.Background(), FinanceInitializationRequest{OperatorID: 99, Reason: "重复确认当前余额", Upstreams: []FinanceInitializationUpstreamInput{{UpstreamID: 5, CurrentBalance: 12}}})
	require.NoError(t, err)
	require.Len(t, funds.calls, 1)
	require.Equal(t, "balance_snapshot", funds.calls[0].EventType)
}
