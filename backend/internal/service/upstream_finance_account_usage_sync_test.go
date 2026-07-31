package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestUpstreamFinanceAccountUsageSyncPlatformCreditAppliesObservedMultiplier(t *testing.T) {
	svc, snapshots, syncRepo, requests := newAccountUsageSyncTestService(t, FinanceUnitPlatformCredit, []string{
		`{"list_cost":"10","actual_cost":"2.2","unit":"USD","counter_id":"acct-7"}`,
		`{"list_cost":"20","actual_cost":"4.4","unit":"USD","counter_id":"acct-7"}`,
	}, []int64{7})

	for range 2 {
		_, created, err := svc.Enqueue(context.Background(), 11, UpstreamFinanceSyncAccountUsage, nil)
		require.NoError(t, err)
		require.True(t, created)
		ran, err := svc.RunNext(context.Background(), "test-worker")
		require.NoError(t, err)
		require.True(t, ran)
	}

	require.Equal(t, int64(2), syncRepo.accountUsageCollected)
	require.Equal(t, int64(0), syncRepo.accountUsageSkipped)
	require.Len(t, snapshots.snapshots, 2)
	require.Contains(t, snapshots.snapshots[0].ScopeKey, "counter:")
	require.Contains(t, snapshots.snapshots[0].ScopeKey, ":account:7")
	require.Equal(t, AccountFinanceDerivationBaseline, snapshots.snapshots[0].DerivationStatus)
	require.Equal(t, AccountFinanceDerivationApplied, snapshots.snapshots[1].DerivationStatus)
	require.Equal(t, 1, snapshots.updateCalls)
	require.True(t, snapshots.account.UpstreamCostMultiplier.Equal(decimal.RequireFromString("0.22")))
	require.Equal(t, []string{"Bearer account-key", "Bearer account-key"}, requests.authorization)
}

func TestUpstreamFinanceAccountUsageSyncFiatSnapshotsApplyObservedMultiplier(t *testing.T) {
	svc, snapshots, syncRepo, requests := newAccountUsageSyncTestService(t, FinanceUnitFiatCurrency, []string{
		`{"list_cost":"10","actual_cost":"2.5","unit":"USD","counter_id":"acct-7"}`,
		`{"list_cost":"20","actual_cost":"5","unit":"USD","counter_id":"acct-7"}`,
	}, []int64{7})

	for range 2 {
		_, created, err := svc.Enqueue(context.Background(), 11, UpstreamFinanceSyncAccountUsage, nil)
		require.NoError(t, err)
		require.True(t, created)
		ran, runErr := svc.RunNext(context.Background(), "test-worker")
		require.NoError(t, runErr)
		require.True(t, ran)
	}

	require.Equal(t, int64(2), syncRepo.accountUsageCollected)
	require.Len(t, snapshots.snapshots, 2)
	require.Equal(t, AccountFinanceDerivationBaseline, snapshots.snapshots[0].DerivationStatus)
	require.Equal(t, AccountFinanceDerivationApplied, snapshots.snapshots[1].DerivationStatus)
	require.Equal(t, 1, snapshots.updateCalls)
	require.True(t, snapshots.account.UpstreamCostMultiplier.Equal(decimal.RequireFromString("0.25")))
	require.Equal(t, []string{"Bearer account-key", "Bearer account-key"}, requests.authorization)
}

func TestUpstreamFinanceAccountUsageEnqueueRejectsMissingBindingAndCapability(t *testing.T) {
	svc, _, _, _ := newAccountUsageSyncTestService(t, FinanceUnitFiatCurrency, nil, nil)
	_, _, err := svc.Enqueue(context.Background(), 11, UpstreamFinanceSyncAccountUsage, nil)
	require.ErrorContains(t, err, "wallet has no active bound accounts")

	svc, _, _, _ = newAccountUsageSyncTestServiceWithConfig(t, FinanceProtocolConfig{
		Capabilities:   []string{FinanceCapabilityPricing},
		CostMode:       FinanceCostModeManual,
		UnitSemantics:  FinanceUnitNone,
		Authentication: FinanceProtocolAuthentication{Type: "none"},
		Operations: map[string]FinanceProtocolOperation{
			FinanceCapabilityPricing: {Method: http.MethodGet, Path: "/v1/models", Mapping: map[string]string{"models": "$.data"}},
		},
	}, nil, []int64{7})
	_, _, err = svc.Enqueue(context.Background(), 11, UpstreamFinanceSyncAccountUsage, nil)
	require.ErrorContains(t, err, "wallet protocol does not support account_usage")

	config := validFinanceProtocolConfig()
	config.Authentication.CredentialSource = "wallet_finance_credential"
	svc, _, _, _ = newAccountUsageSyncTestServiceWithConfig(t, config, nil, []int64{7, 8})
	_, _, err = svc.Enqueue(context.Background(), 11, UpstreamFinanceSyncAccountUsage, nil)
	require.ErrorContains(t, err, "requires exactly one active bound account")
}

func TestUpstreamFinanceAccountUsageUsesAccountBaseURL(t *testing.T) {
	svc, snapshots, _, requests := newAccountUsageSyncTestService(t, FinanceUnitFiatCurrency, []string{
		`{"list_cost":"10","actual_cost":"2.5","unit":"USD","counter_id":"acct-7"}`,
	}, []int64{7})
	snapshots.account.Credentials["base_url"] = "https://account-upstream.example.com"

	_, _, err := svc.Enqueue(context.Background(), 11, UpstreamFinanceSyncAccountUsage, nil)
	require.NoError(t, err)
	ran, err := svc.RunNext(context.Background(), "test-worker")
	require.NoError(t, err)
	require.True(t, ran)
	require.Equal(t, []string{"https://account-upstream.example.com/v1/usage"}, requests.urls)
}

func TestAccountUsageCounterIdentitySurvivesWalletAndProtocolChanges(t *testing.T) {
	counterID := "shared-counter"
	period := "2026-07"
	usage := &UpstreamFinanceAccountUsage{UpstreamCounterID: &counterID, CounterPeriod: &period}
	firstVersion := int64(101)
	secondVersion := int64(202)
	first, err := accountUsageCounterIdentity(UpstreamWallet{ID: 11, UpstreamID: 3, BalanceScopeKey: "shared-main", ProtocolVersionID: &firstVersion}, 7, usage)
	require.NoError(t, err)
	second, err := accountUsageCounterIdentity(UpstreamWallet{ID: 12, UpstreamID: 3, BalanceScopeKey: "shared-main", ProtocolVersionID: &secondVersion}, 7, usage)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestAccountUsageCounterIdentityRequiresUpstreamCounterID(t *testing.T) {
	first, err := accountUsageCounterIdentity(UpstreamWallet{ID: 11, UpstreamID: 3, BalanceScopeKey: "shared-main"}, 7, &UpstreamFinanceAccountUsage{})
	require.NoError(t, err)
	second, err := accountUsageCounterIdentity(UpstreamWallet{ID: 11, UpstreamID: 3, BalanceScopeKey: "shared-main"}, 8, &UpstreamFinanceAccountUsage{})
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}

func TestResolveAccountUsageCredentialUsesDeclaredSource(t *testing.T) {
	account := &Account{ID: 7, Credentials: map[string]any{
		"api_key": "api-key", "access_token": "access-token", "token": "token", "setup_token": "setup-token",
	}}
	for _, testCase := range []struct {
		source   string
		expected string
	}{
		{source: "account_api_key", expected: "api-key"},
		{source: "account_access_token", expected: "access-token"},
		{source: "account_token", expected: "token"},
		{source: "account_setup_token", expected: "setup-token"},
		{source: "wallet_finance_credential", expected: "wallet-key"},
	} {
		t.Run(testCase.source, func(t *testing.T) {
			credential, err := resolveAccountUsageCredential(account, testCase.source, "wallet-key")
			require.NoError(t, err)
			require.Equal(t, testCase.expected, credential)
		})
	}
}

type accountUsageRequestCapture struct {
	authorization []string
	urls          []string
}

func newAccountUsageSyncTestService(t *testing.T, semantics string, responses []string, activeAccountIDs []int64) (*UpstreamFinanceSyncService, *accountFinanceSnapshotFakeRepo, *accountUsageSyncRepositoryStub, *accountUsageRequestCapture) {
	t.Helper()
	config := validFinanceProtocolConfig()
	config.UnitSemantics = semantics
	return newAccountUsageSyncTestServiceWithConfig(t, config, responses, activeAccountIDs)
}

func newAccountUsageSyncTestServiceWithConfig(t *testing.T, config FinanceProtocolConfig, responses []string, activeAccountIDs []int64) (*UpstreamFinanceSyncService, *accountFinanceSnapshotFakeRepo, *accountUsageSyncRepositoryStub, *accountUsageRequestCapture) {
	t.Helper()
	multiplier := decimal.RequireFromString("0.1")
	account := &Account{
		ID: 7, Status: StatusActive, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "account-key"}, UpstreamCostMultiplier: &multiplier,
	}
	profileID := int64(301)
	account.CurrentFinanceProfileID = &profileID
	snapshots := newAccountFinanceSnapshotFakeRepo(account)
	walletVersionID := int64(101)
	walletRepo := &upstreamWalletRepoStub{
		wallet: &UpstreamWallet{
			ID: 11, UpstreamID: 3, Name: "wallet", AdapterType: UpstreamAdapterProtocol,
			BaseURL: "https://example.com", Currency: "USD", Enabled: true,
			EncryptedCredential: []byte("wallet-key"), ProtocolVersionID: &walletVersionID,
		},
		activeAccountIDs: activeAccountIDs,
	}
	publishedAt := time.Now().UTC()
	protocolRepo := &accountUsageProtocolRepositoryStub{
		protocolDetectionRepoStub: &protocolDetectionRepoStub{},
		protocol:                  UpstreamFinanceProtocol{ID: 9, Code: "account-usage", Status: FinanceProtocolStatusPublished},
		version: UpstreamFinanceProtocolVersion{
			ID: walletVersionID, ProtocolID: 9, Config: config,
			ValidationStatus: "valid", PublishedAt: &publishedAt,
		},
	}
	requests := &accountUsageRequestCapture{
		authorization: make([]string, 0, len(responses)),
		urls:          make([]string, 0, len(responses)),
	}
	responseIndex := 0
	executor := NewUpstreamFinanceHTTPExecutorWithClient(financeProtocolDoerFunc(func(request *http.Request) (*http.Response, error) {
		requests.authorization = append(requests.authorization, request.Header.Get("Authorization"))
		requests.urls = append(requests.urls, request.URL.String())
		require.Less(t, responseIndex, len(responses))
		body := responses[responseIndex]
		responseIndex++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	}))
	syncRepo := &accountUsageSyncRepositoryStub{}
	walletService := NewUpstreamWalletService(walletRepo, upstreamWalletEncryptorStub{})
	snapshotService := NewAccountFinanceSnapshotService(snapshots, snapshots, nil)
	return NewUpstreamFinanceSyncService(
		walletService, NewUpstreamFinanceAdapterRegistry(), syncRepo, protocolRepo, executor,
		nil, snapshotService, snapshots,
	), snapshots, syncRepo, requests
}

type accountUsageProtocolRepositoryStub struct {
	*protocolDetectionRepoStub
	protocol UpstreamFinanceProtocol
	version  UpstreamFinanceProtocolVersion
}

func (s *accountUsageProtocolRepositoryStub) GetProtocol(context.Context, int64) (*UpstreamFinanceProtocol, error) {
	copy := s.protocol
	return &copy, nil
}

func (s *accountUsageProtocolRepositoryStub) GetVersion(context.Context, int64) (*UpstreamFinanceProtocolVersion, error) {
	copy := s.version
	return &copy, nil
}

type accountUsageSyncRepositoryStub struct {
	jobs                  []*UpstreamFinanceSyncJob
	nextClaim             int
	accountUsageCollected int64
	accountUsageSkipped   int64
}

func (s *accountUsageSyncRepositoryStub) CreateOrGetActiveSyncJob(_ context.Context, walletID int64, syncType string, _ *int64) (*UpstreamFinanceSyncJob, bool, error) {
	job := &UpstreamFinanceSyncJob{ID: int64(len(s.jobs) + 1), WalletID: walletID, SyncType: syncType, Status: "pending"}
	s.jobs = append(s.jobs, job)
	return job, true, nil
}

func (s *accountUsageSyncRepositoryStub) ClaimNextSyncJob(context.Context, string, time.Time) (*UpstreamFinanceSyncJob, error) {
	if s.nextClaim >= len(s.jobs) {
		return nil, nil
	}
	job := s.jobs[s.nextClaim]
	s.nextClaim++
	return job, nil
}

func (s *accountUsageSyncRepositoryStub) RenewSyncJobLease(context.Context, int64, string, time.Time) error {
	return nil
}

func (s *accountUsageSyncRepositoryStub) CompletePricingSync(context.Context, *UpstreamFinanceSyncJob, string, []UpstreamFinancePrice, time.Time) error {
	return nil
}

func (s *accountUsageSyncRepositoryStub) CompleteBalanceSync(context.Context, *UpstreamFinanceSyncJob, string, *UpstreamFinanceBalance, time.Time) error {
	return nil
}

func (s *accountUsageSyncRepositoryStub) CompleteFundingSync(context.Context, *UpstreamFinanceSyncJob, string, int64, int64, time.Time) error {
	return nil
}

func (s *accountUsageSyncRepositoryStub) CompleteAccountUsageSync(_ context.Context, _ *UpstreamFinanceSyncJob, _ string, collected, skipped int64, _ time.Time) error {
	s.accountUsageCollected += collected
	s.accountUsageSkipped += skipped
	return nil
}

func (s *accountUsageSyncRepositoryStub) FailSyncJob(context.Context, *UpstreamFinanceSyncJob, string, string, time.Time) error {
	return nil
}

func (s *accountUsageSyncRepositoryStub) RecordProbe(context.Context, int64, UpstreamFinanceProbe) error {
	return nil
}

func (s *accountUsageSyncRepositoryStub) ListPriceVersions(context.Context, int64, UpstreamFinancePriceListFilter) ([]UpstreamFinancePriceVersion, int64, error) {
	return nil, 0, nil
}

func (s *accountUsageSyncRepositoryStub) ImportPriceVersions(context.Context, int64, []UpstreamFinancePrice, time.Time) (int64, int64, error) {
	return 0, 0, nil
}

func (s *accountUsageSyncRepositoryStub) ListSyncHistory(context.Context, int64, UpstreamFinanceSyncHistoryFilter) ([]UpstreamFinanceSyncHistory, int64, error) {
	return nil, 0, nil
}

func (s *accountUsageSyncRepositoryStub) ListDueSyncRequests(context.Context, time.Time) ([]UpstreamFinanceSyncRequest, error) {
	return nil, nil
}
