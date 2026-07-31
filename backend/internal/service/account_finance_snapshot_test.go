package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestAccountFinanceSnapshotService_FiatSnapshotsApplyObservedMultiplierWithAudit(t *testing.T) {
	accountMultiplier := decimal.RequireFromString("0.1000")
	repo := newAccountFinanceSnapshotFakeRepo(&Account{ID: 7, Status: StatusActive, UpstreamCostMultiplier: &accountMultiplier})
	service := NewAccountFinanceSnapshotService(repo, repo, nil)
	effectiveNow := time.Date(2026, 7, 29, 12, 30, 0, 0, time.UTC)
	service.now = func() time.Time { return effectiveNow }

	first := fiatCounterObservation("sync-1", "10", "2.5", effectiveNow.Add(-10*time.Minute))
	first.SafeSnapshot = map[string]any{"usage": map[string]any{"cost": "10"}, "api_key": "secret"}
	baseline, err := service.ObserveCounter(context.Background(), first)
	if err != nil {
		t.Fatalf("record baseline: %v", err)
	}
	if baseline.DerivationStatus != AccountFinanceDerivationBaseline {
		t.Fatalf("baseline status = %s", baseline.DerivationStatus)
	}
	if baseline.SafeSnapshot["api_key"] != "[REDACTED]" {
		t.Fatalf("sensitive snapshot was not redacted: %#v", baseline.SafeSnapshot)
	}
	if repo.updateCalls != 0 {
		t.Fatalf("baseline must not update multiplier")
	}

	second := fiatCounterObservation("sync-2", "20", "5", effectiveNow.Add(-5*time.Minute))
	applied, err := service.ObserveCounter(context.Background(), second)
	if err != nil {
		t.Fatalf("record second snapshot: %v", err)
	}
	if applied.DerivationStatus != AccountFinanceDerivationApplied {
		t.Fatalf("status = %s", applied.DerivationStatus)
	}
	assertFinanceDecimal(t, applied.ListCostDelta, "10")
	assertFinanceDecimal(t, applied.ActualCostDelta, "2.5")
	assertFinanceDecimal(t, applied.ObservedMultiplier, "0.25")
	if repo.updateCalls != 1 || !repo.account.UpstreamCostMultiplier.Equal(decimal.RequireFromString("0.25")) {
		t.Fatalf("observed multiplier was not applied: calls=%d multiplier=%v", repo.updateCalls, repo.account.UpstreamCostMultiplier)
	}
	if repo.account.UpstreamCostMultiplierSource != AccountFinanceMultiplierSourceUpstreamUsage {
		t.Fatalf("source = %q", repo.account.UpstreamCostMultiplierSource)
	}
	if applied.MultiplierChangeID == nil || applied.MultiplierEffectiveAt == nil {
		t.Fatalf("applied snapshot is missing multiplier audit identity: %#v", applied)
	}
	if applied.AnomalyCode == nil || *applied.AnomalyCode != AccountFinanceAnomalyMultiplierJump {
		t.Fatalf("expected multiplier jump evidence, got %v", applied.AnomalyCode)
	}

	oldVersion, err := service.ResolveMultiplierVersionAt(context.Background(), 7, applied.MultiplierEffectiveAt.Add(-time.Nanosecond))
	if err != nil {
		t.Fatalf("resolve old version: %v", err)
	}
	if oldVersion != nil {
		t.Fatalf("new multiplier must not be visible before effective time: %#v", oldVersion)
	}
	newVersion, err := service.ResolveMultiplierVersionAt(context.Background(), 7, *applied.MultiplierEffectiveAt)
	if err != nil || newVersion == nil || !newVersion.NewMultiplier.Equal(decimal.RequireFromString("0.25")) {
		t.Fatalf("observation version was not created: version=%#v err=%v", newVersion, err)
	}
}

func TestAccountFinanceSnapshotService_PreservesJobProfileSnapshotWhenAccountChanges(t *testing.T) {
	multiplier := decimal.RequireFromString("0.2000")
	currentProfileID := int64(900)
	repo := newAccountFinanceSnapshotFakeRepo(&Account{ID: 7, Status: StatusActive, CurrentFinanceProfileID: &currentProfileID, UpstreamCostMultiplier: &multiplier})
	service := NewAccountFinanceSnapshotService(repo, repo, nil)
	jobProfileID := int64(901)
	observation := fiatCounterObservation("job-profile", "10", "2", time.Now().UTC())
	observation.AccountFinanceProfileID = &jobProfileID
	stored, err := service.ObserveCounter(context.Background(), observation)
	require.NoError(t, err)
	require.NotNil(t, stored.AccountFinanceProfileID)
	require.Equal(t, jobProfileID, *stored.AccountFinanceProfileID)
}

func TestAccountFinanceSnapshotService_PlatformCreditDerivesAndAppliesMultiplier(t *testing.T) {
	accountMultiplier := decimal.RequireFromString("0.1000")
	repo := newAccountFinanceSnapshotFakeRepo(&Account{ID: 7, Status: StatusActive, UpstreamCostMultiplier: &accountMultiplier})
	service := NewAccountFinanceSnapshotService(repo, repo, nil)
	base := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

	for index, values := range [][2]string{{"10", "2.2"}, {"20", "4.4"}} {
		listValue := decimal.RequireFromString(values[0])
		actualValue := decimal.RequireFromString(values[1])
		observation := AccountFinanceCounterObservation{
			AccountID: 7, WalletID: 11, CounterIdentityKey: "wallet-11-credit-7", ScopeKey: "account:7", IdempotencyKey: fmt.Sprintf("credit-%d", index),
			ListCostTotal: &listValue, ActualCostTotal: &actualValue,
			UnitCode: "USD", UnitSemantics: AccountFinanceUnitPlatformCredit,
			CollectedAt: base.Add(time.Duration(index) * time.Minute),
		}
		stored, err := service.ObserveCounter(context.Background(), observation)
		if err != nil {
			t.Fatalf("record platform credit %d: %v", index, err)
		}
		if index == 0 && stored.DerivationStatus != AccountFinanceDerivationBaseline {
			t.Fatalf("platform credit baseline: %#v", stored)
		}
		if index == 1 && (stored.DerivationStatus != AccountFinanceDerivationApplied || stored.ObservedMultiplier == nil || !stored.ObservedMultiplier.Equal(decimal.RequireFromString("0.22"))) {
			t.Fatalf("platform credit multiplier was not applied: %#v", stored)
		}
	}
	if repo.updateCalls != 1 || !repo.account.UpstreamCostMultiplier.Equal(decimal.RequireFromString("0.22")) {
		t.Fatalf("platform credit multiplier update failed: calls=%d multiplier=%v", repo.updateCalls, repo.account.UpstreamCostMultiplier)
	}
}

func TestAccountFinanceSnapshotService_ContinuesCounterAcrossMultiplierOnlyProfileRollover(t *testing.T) {
	accountMultiplier := decimal.RequireFromString("0.1000")
	profileID := int64(100)
	walletID := int64(11)
	protocolVersionID := int64(22)
	repo := newAccountFinanceSnapshotFakeRepo(&Account{
		ID: 7, Status: StatusActive, CurrentFinanceProfileID: &profileID, UpstreamCostMultiplier: &accountMultiplier,
	})
	repo.profiles[profileID] = &AccountFinanceProfile{
		ID: profileID, AccountID: 7, WalletID: &walletID, ProtocolVersionID: &protocolVersionID,
		CostMode: FinanceCostModeCumulativeListAndActual, EndpointSource: "account_base_url",
		EndpointBaseURLSnapshot: "https://upstream.example.com", CredentialSource: "account_api_key",
		CounterScope: FinanceCounterScopeAccount, BalanceUnitSemantics: FinanceUnitPlatformCredit, Version: 2,
	}
	repo.rolloverProfileOnUpdate = true
	service := NewAccountFinanceSnapshotService(repo, repo, nil)
	base := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

	first, err := service.ObserveCounter(context.Background(), platformCreditCounterObservation("sync-1", "10", "6", base))
	require.NoError(t, err)
	require.Equal(t, AccountFinanceDerivationBaseline, first.DerivationStatus)

	second, err := service.ObserveCounter(context.Background(), platformCreditCounterObservation("sync-2", "20", "12", base.Add(time.Minute)))
	require.NoError(t, err)
	require.Equal(t, AccountFinanceDerivationApplied, second.DerivationStatus)
	require.NotNil(t, repo.account.CurrentFinanceProfileID)
	require.NotEqual(t, profileID, *repo.account.CurrentFinanceProfileID)

	third, err := service.ObserveCounter(context.Background(), platformCreditCounterObservation("sync-3", "30", "18", base.Add(2*time.Minute)))
	require.NoError(t, err)
	require.Equal(t, AccountFinanceDerivationUnchanged, third.DerivationStatus)
	require.NotNil(t, third.ObservedMultiplier)
	require.True(t, third.ObservedMultiplier.Equal(decimal.RequireFromString("0.6")))
	require.Equal(t, 1, repo.updateCalls)
	require.Len(t, repo.versions, 1)
}

func TestAccountFinanceSnapshotService_StopsCounterAcrossProtocolChange(t *testing.T) {
	accountMultiplier := decimal.RequireFromString("0.6000")
	firstProfileID := int64(100)
	secondProfileID := int64(101)
	walletID := int64(11)
	firstProtocolVersionID := int64(22)
	secondProtocolVersionID := int64(23)
	repo := newAccountFinanceSnapshotFakeRepo(&Account{
		ID: 7, Status: StatusActive, CurrentFinanceProfileID: &firstProfileID, UpstreamCostMultiplier: &accountMultiplier,
	})
	baseProfile := AccountFinanceProfile{
		AccountID: 7, WalletID: &walletID, CostMode: FinanceCostModeCumulativeListAndActual,
		EndpointSource: "account_base_url", EndpointBaseURLSnapshot: "https://upstream.example.com",
		CredentialSource: "account_api_key", CounterScope: FinanceCounterScopeAccount,
		BalanceUnitSemantics: FinanceUnitPlatformCredit,
	}
	firstProfile := baseProfile
	firstProfile.ID = firstProfileID
	firstProfile.ProtocolVersionID = &firstProtocolVersionID
	secondProfile := baseProfile
	secondProfile.ID = secondProfileID
	secondProfile.ProtocolVersionID = &secondProtocolVersionID
	repo.profiles[firstProfileID] = &firstProfile
	repo.profiles[secondProfileID] = &secondProfile
	service := NewAccountFinanceSnapshotService(repo, repo, nil)
	base := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

	first, err := service.ObserveCounter(context.Background(), platformCreditCounterObservation("sync-1", "10", "6", base))
	require.NoError(t, err)
	require.Equal(t, AccountFinanceDerivationBaseline, first.DerivationStatus)

	repo.account.CurrentFinanceProfileID = &secondProfileID
	second, err := service.ObserveCounter(context.Background(), platformCreditCounterObservation("sync-2", "20", "12", base.Add(time.Minute)))
	require.NoError(t, err)
	require.Equal(t, AccountFinanceDerivationBoundaryChanged, second.DerivationStatus)
	require.Nil(t, second.ObservedMultiplier)
	require.Equal(t, 0, repo.updateCalls)
}

func TestAccountFinanceSnapshotService_InvalidObservedMultiplierIsEvidenceOnly(t *testing.T) {
	accountMultiplier := decimal.RequireFromString("0.2200")
	repo := newAccountFinanceSnapshotFakeRepo(&Account{ID: 7, Status: StatusActive, UpstreamCostMultiplier: &accountMultiplier})
	service := NewAccountFinanceSnapshotService(repo, repo, nil)
	base := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	_, _ = service.ObserveCounter(context.Background(), fiatCounterObservation("sync-1", "1", "0", base))
	stored, err := service.ObserveCounter(context.Background(), fiatCounterObservation("sync-2", "2", "10000", base.Add(time.Minute)))
	if err == nil {
		t.Fatal("expected account multiplier range validation error")
	}
	if stored == nil || stored.DerivationStatus != AccountFinanceDerivationInvalidMultiplier || stored.ObservedMultiplier == nil {
		t.Fatalf("invalid observation evidence was not preserved: %#v", stored)
	}
	if repo.updateCalls != 0 || !repo.account.UpstreamCostMultiplier.Equal(accountMultiplier) {
		t.Fatalf("invalid observed multiplier changed account state")
	}
}

func TestDeriveAccountFinanceMultiplier_RejectsCounterBoundaries(t *testing.T) {
	base := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	previous := snapshotForDerivation(1, "USD", "cycle-a", "10", "2.5", base)

	tests := []struct {
		name    string
		current *AccountFinanceCounterSnapshot
		status  string
	}{
		{"currency", snapshotForDerivation(2, "CNY", "cycle-a", "20", "5", base.Add(time.Minute)), AccountFinanceDerivationBoundaryChanged},
		{"period", snapshotForDerivation(2, "USD", "cycle-b", "20", "5", base.Add(time.Minute)), AccountFinanceDerivationBoundaryChanged},
		{"reset", snapshotForDerivation(2, "USD", "cycle-a", "8", "2", base.Add(time.Minute)), AccountFinanceDerivationCounterReset},
		{"time", snapshotForDerivation(2, "USD", "cycle-a", "20", "5", base), AccountFinanceDerivationTimeReversed},
		{"no activity", snapshotForDerivation(2, "USD", "cycle-a", "10", "2.5", base.Add(time.Minute)), AccountFinanceDerivationNoActivity},
		{"zero list", snapshotForDerivation(2, "USD", "cycle-a", "10", "3", base.Add(time.Minute)), AccountFinanceDerivationInvalidList},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deriveAccountFinanceMultiplier(test.current, previous, true)
			if test.current.DerivationStatus != test.status || test.current.ObservedMultiplier != nil {
				t.Fatalf("status=%s multiplier=%v", test.current.DerivationStatus, test.current.ObservedMultiplier)
			}
		})
	}
}

func TestDeriveAccountFinanceMultiplierSupportsCumulativeActualOnly(t *testing.T) {
	base := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	previousActual := decimal.RequireFromString("2.5")
	currentActual := decimal.RequireFromString("4.7")
	previous := &AccountFinanceCounterSnapshot{ID: 1, UnitCode: "USD", UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: financeSnapshotStringPtr("USD"), CounterPeriod: financeSnapshotStringPtr("cycle-a"), ActualCostTotal: &previousActual, CollectedAt: base, DerivationStatus: AccountFinanceDerivationBaseline}
	current := &AccountFinanceCounterSnapshot{ID: 2, UnitCode: "USD", UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: financeSnapshotStringPtr("USD"), CounterPeriod: financeSnapshotStringPtr("cycle-a"), ActualCostTotal: &currentActual, CollectedAt: base.Add(time.Minute)}

	deriveAccountFinanceMultiplier(current, previous, true)
	require.Equal(t, AccountFinanceDerivationSettlementReady, current.DerivationStatus)
	require.Nil(t, current.ListCostDelta)
	require.Nil(t, current.ObservedMultiplier)
	require.NotNil(t, current.ActualCostDelta)
	require.True(t, current.ActualCostDelta.Equal(decimal.RequireFromString("2.2")))
}

func financeSnapshotStringPtr(value string) *string { return &value }

func TestAccountFinanceSnapshotService_IdempotentRetryDoesNotDuplicateAudit(t *testing.T) {
	accountMultiplier := decimal.RequireFromString("0.2000")
	repo := newAccountFinanceSnapshotFakeRepo(&Account{ID: 7, Status: StatusActive, UpstreamCostMultiplier: &accountMultiplier})
	service := NewAccountFinanceSnapshotService(repo, repo, nil)
	base := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return base.Add(10 * time.Minute) }
	_, _ = service.ObserveCounter(context.Background(), fiatCounterObservation("sync-1", "10", "2", base))
	input := fiatCounterObservation("sync-2", "20", "5", base.Add(time.Minute))
	first, err := service.ObserveCounter(context.Background(), input)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	second, err := service.ObserveCounter(context.Background(), input)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if first.ID != second.ID || repo.updateCalls != 1 || len(repo.versions) != 1 {
		t.Fatalf("retry duplicated facts: first=%d second=%d updates=%d versions=%d", first.ID, second.ID, repo.updateCalls, len(repo.versions))
	}
}

func TestAccountFinanceSnapshotService_RetriesSettlementWithoutDuplicatingMultiplierAudit(t *testing.T) {
	accountMultiplier := decimal.RequireFromString("0.2000")
	repo := newAccountFinanceSnapshotFakeRepo(&Account{ID: 7, Status: StatusActive, UpstreamCostMultiplier: &accountMultiplier})
	processor := &accountFinanceSettlementProcessorStub{failuresRemaining: 1}
	service := NewAccountFinanceSnapshotService(repo, repo, processor)
	base := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return base.Add(10 * time.Minute) }
	_, _ = service.ObserveCounter(context.Background(), fiatCounterObservation("sync-1", "10", "2", base))
	input := fiatCounterObservation("sync-2", "20", "5", base.Add(time.Minute))

	_, err := service.ObserveCounter(context.Background(), input)
	if err == nil {
		t.Fatal("expected first settlement attempt to fail")
	}
	_, err = service.ObserveCounter(context.Background(), input)
	if err != nil {
		t.Fatalf("retry settlement: %v", err)
	}
	if processor.calls != 2 || repo.updateCalls != 1 || len(repo.versions) != 1 {
		t.Fatalf("retry calls=%d multiplier_updates=%d versions=%d", processor.calls, repo.updateCalls, len(repo.versions))
	}
}

func TestAccountFinanceSnapshotService_SerializesSameAccountScope(t *testing.T) {
	accountMultiplier := decimal.RequireFromString("0.2000")
	repo := newAccountFinanceSnapshotFakeRepo(&Account{ID: 7, Status: StatusActive, UpstreamCostMultiplier: &accountMultiplier})
	service := NewAccountFinanceSnapshotService(repo, repo, nil)
	base := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			input := fiatCounterObservation(fmt.Sprintf("sync-%d", index), fmt.Sprintf("%d", 10+index*10), fmt.Sprintf("%d", 2+index*3), base.Add(time.Duration(index)*time.Minute))
			_, _ = service.ObserveCounter(context.Background(), input)
		}(index)
	}
	wg.Wait()
	if repo.maxLockHolders != 1 {
		t.Fatalf("same account scope was processed concurrently: max=%d", repo.maxLockHolders)
	}
}

func fiatCounterObservation(idempotency, list, actual string, collectedAt time.Time) AccountFinanceCounterObservation {
	listValue := decimal.RequireFromString(list)
	actualValue := decimal.RequireFromString(actual)
	currency := "USD"
	period := "cycle-a"
	counter := "usage-total"
	return AccountFinanceCounterObservation{
		AccountID: 7, WalletID: 11, CounterIdentityKey: "wallet-11-counter-usage-total", ScopeKey: "account:7", IdempotencyKey: idempotency,
		UpstreamCounterID: &counter, CounterPeriod: &period,
		ListCostTotal: &listValue, ActualCostTotal: &actualValue,
		UnitCode: "USD", UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency,
		CollectedAt: collectedAt,
	}
}

func platformCreditCounterObservation(idempotency, list, actual string, collectedAt time.Time) AccountFinanceCounterObservation {
	observation := fiatCounterObservation(idempotency, list, actual, collectedAt)
	observation.Currency = nil
	observation.UnitSemantics = AccountFinanceUnitPlatformCredit
	return observation
}

func snapshotForDerivation(id int64, currency, period, list, actual string, at time.Time) *AccountFinanceCounterSnapshot {
	listValue := decimal.RequireFromString(list)
	actualValue := decimal.RequireFromString(actual)
	counter := "usage-total"
	return &AccountFinanceCounterSnapshot{
		ID: id, AccountID: 7, ScopeKey: "account:7", UpstreamCounterID: &counter, CounterPeriod: &period,
		ListCostTotal: &listValue, ActualCostTotal: &actualValue,
		UnitCode: currency, UnitSemantics: AccountFinanceUnitFiatCurrency, Currency: &currency,
		CollectedAt: at,
	}
}

func assertFinanceDecimal(t *testing.T, value *decimal.Decimal, expected string) {
	t.Helper()
	if value == nil || !value.Equal(decimal.RequireFromString(expected)) {
		t.Fatalf("decimal = %v, want %s", value, expected)
	}
}

type accountFinanceSnapshotFakeRepo struct {
	mu                      sync.Mutex
	locks                   map[string]*sync.Mutex
	counterOwners           map[string]int64
	account                 *Account
	profiles                map[int64]*AccountFinanceProfile
	snapshots               []*AccountFinanceCounterSnapshot
	versions                []*AccountFinanceMultiplierVersion
	nextSnapshotID          int64
	nextVersionID           int64
	updateCalls             int
	lastEffectiveAt         time.Time
	lastReason              string
	lockHolders             int
	maxLockHolders          int
	rolloverProfileOnUpdate bool
}

type accountFinanceSettlementProcessorStub struct {
	calls             int
	failuresRemaining int
}

func (s *accountFinanceSettlementProcessorStub) ProcessSnapshotInterval(_ context.Context, previous, current *AccountFinanceCounterSnapshot) error {
	s.calls++
	if previous == nil || current == nil {
		return errors.New("missing snapshot interval")
	}
	if s.failuresRemaining > 0 {
		s.failuresRemaining--
		return errors.New("temporary settlement failure")
	}
	return nil
}

func newAccountFinanceSnapshotFakeRepo(account *Account) *accountFinanceSnapshotFakeRepo {
	return &accountFinanceSnapshotFakeRepo{locks: map[string]*sync.Mutex{}, counterOwners: map[string]int64{}, account: account, profiles: map[int64]*AccountFinanceProfile{}, nextSnapshotID: 1, nextVersionID: 1}
}

func (r *accountFinanceSnapshotFakeRepo) ClaimCounterOwner(_ context.Context, identityKey string, _ int64, _ *int64, accountID int64, _, _ *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if owner, exists := r.counterOwners[identityKey]; exists && owner != accountID {
		return ErrAccountFinanceCounterOwnerConflict
	}
	r.counterOwners[identityKey] = accountID
	return nil
}

func (r *accountFinanceSnapshotFakeRepo) WithAccountSyncLock(ctx context.Context, accountID int64, scopeKey string, fn func(context.Context) error) error {
	key := fmt.Sprintf("%d:%s", accountID, scopeKey)
	r.mu.Lock()
	lock := r.locks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		r.locks[key] = lock
	}
	r.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	r.mu.Lock()
	r.lockHolders++
	if r.lockHolders > r.maxLockHolders {
		r.maxLockHolders = r.lockHolders
	}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.lockHolders--
		r.mu.Unlock()
	}()
	return fn(ctx)
}

func (r *accountFinanceSnapshotFakeRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account == nil || r.account.ID != id {
		return nil, ErrAccountNotFound
	}
	copy := *r.account
	copy.UpstreamCostMultiplier = cloneFinanceDecimal(r.account.UpstreamCostMultiplier)
	return &copy, nil
}

func (r *accountFinanceSnapshotFakeRepo) GetFinanceProfileByID(_ context.Context, id int64) (*AccountFinanceProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if profile := r.profiles[id]; profile != nil {
		copy := *profile
		return &copy, nil
	}
	if r.account == nil || r.account.CurrentFinanceProfileID == nil || *r.account.CurrentFinanceProfileID != id {
		return nil, ErrAccountFinanceProfileNotFound
	}
	walletID := int64(11)
	return &AccountFinanceProfile{
		ID: id, AccountID: r.account.ID, WalletID: &walletID,
		CostMode: FinanceCostModeCumulativeListAndActual,
	}, nil
}

func (r *accountFinanceSnapshotFakeRepo) CounterSnapshotByID(_ context.Context, id int64) (*AccountFinanceCounterSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, snapshot := range r.snapshots {
		if snapshot.ID == id {
			copy := *snapshot
			return &copy, nil
		}
	}
	return nil, ErrAccountFinanceSnapshotInvalid
}

func (r *accountFinanceSnapshotFakeRepo) LatestCounterSnapshot(_ context.Context, accountID int64, scopeKey string) (*AccountFinanceCounterSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := len(r.snapshots) - 1; index >= 0; index-- {
		item := r.snapshots[index]
		if item.AccountID == accountID && item.ScopeKey == scopeKey {
			return cloneAccountFinanceSnapshot(item), nil
		}
	}
	return nil, nil
}

func (r *accountFinanceSnapshotFakeRepo) CreateCounterSnapshot(_ context.Context, snapshot *AccountFinanceCounterSnapshot) (*AccountFinanceCounterSnapshot, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.snapshots {
		if item.AccountID == snapshot.AccountID && item.ScopeKey == snapshot.ScopeKey && item.IdempotencyKey == snapshot.IdempotencyKey {
			return cloneAccountFinanceSnapshot(item), false, nil
		}
	}
	copy := cloneAccountFinanceSnapshot(snapshot)
	copy.ID = r.nextSnapshotID
	r.nextSnapshotID++
	copy.CreatedAt = time.Now()
	r.snapshots = append(r.snapshots, copy)
	return cloneAccountFinanceSnapshot(copy), true, nil
}

func (r *accountFinanceSnapshotFakeRepo) MarkCounterSnapshotMultiplierResult(_ context.Context, snapshotID int64, status string, anomalyCode *string, multiplierChangeID *int64, effectiveAt *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.snapshots {
		if item.ID == snapshotID {
			item.DerivationStatus = status
			item.AnomalyCode = cloneFinanceString(anomalyCode)
			item.MultiplierChangeID = multiplierChangeID
			item.MultiplierEffectiveAt = cloneFinanceTime(effectiveAt)
			return nil
		}
	}
	return errors.New("snapshot not found")
}

func (r *accountFinanceSnapshotFakeRepo) UpdateUpstreamMultiplierWithAudit(_ context.Context, accountID int64, expectedOld *decimal.Decimal, newMultiplier decimal.Decimal, effectiveAt time.Time, _ *int64, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account.ID != accountID || !financeDecimalPointersEqual(r.account.UpstreamCostMultiplier, expectedOld) {
		return ErrAccountUpstreamMultiplierConflict
	}
	r.updateCalls++
	r.lastEffectiveAt = effectiveAt
	r.lastReason = reason
	version := &AccountFinanceMultiplierVersion{
		ID: r.nextVersionID, AccountID: accountID, OldMultiplier: cloneFinanceDecimal(expectedOld),
		NewMultiplier: newMultiplier, EffectiveAt: effectiveAt, Reason: reason,
	}
	r.nextVersionID++
	r.versions = append(r.versions, version)
	r.account.UpstreamCostMultiplier = cloneFinanceDecimal(&newMultiplier)
	return nil
}

func (r *accountFinanceSnapshotFakeRepo) UpdateObservedUpstreamMultiplierWithAudit(ctx context.Context, accountID int64, expectedOld *decimal.Decimal, newMultiplier decimal.Decimal, effectiveAt time.Time, reason string) (int64, error) {
	if err := r.UpdateUpstreamMultiplierWithAudit(ctx, accountID, expectedOld, newMultiplier, effectiveAt, nil, reason); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	changeID := r.versions[len(r.versions)-1].ID
	r.account.UpstreamCostMultiplierChangeID = &changeID
	r.account.UpstreamCostMultiplierUpdatedAt = cloneFinanceTime(&effectiveAt)
	r.account.UpstreamCostMultiplierSource = AccountFinanceMultiplierSourceUpstreamUsage
	if r.rolloverProfileOnUpdate && r.account.CurrentFinanceProfileID != nil {
		previousID := *r.account.CurrentFinanceProfileID
		previous := r.profiles[previousID]
		if previous != nil {
			current := *previous
			current.ID = previousID + 1
			current.Version++
			current.AccountMultiplierSnapshot = cloneFinanceDecimal(&newMultiplier)
			current.AccountMultiplierChangeID = &changeID
			r.profiles[current.ID] = &current
			r.account.CurrentFinanceProfileID = &current.ID
		}
	}
	return changeID, nil
}

func (r *accountFinanceSnapshotFakeRepo) ResolveEffectiveMultiplierVersion(_ context.Context, accountID int64, effectiveAt time.Time) (*AccountFinanceMultiplierVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := len(r.versions) - 1; index >= 0; index-- {
		version := r.versions[index]
		if version.AccountID == accountID && !version.EffectiveAt.After(effectiveAt) {
			copy := *version
			copy.OldMultiplier = cloneFinanceDecimal(version.OldMultiplier)
			return &copy, nil
		}
	}
	return nil, nil
}

func cloneAccountFinanceSnapshot(input *AccountFinanceCounterSnapshot) *AccountFinanceCounterSnapshot {
	if input == nil {
		return nil
	}
	copy := *input
	copy.UpstreamCounterID = cloneFinanceString(input.UpstreamCounterID)
	copy.CounterPeriod = cloneFinanceString(input.CounterPeriod)
	copy.ListCostTotal = cloneFinanceDecimal(input.ListCostTotal)
	copy.ActualCostTotal = cloneFinanceDecimal(input.ActualCostTotal)
	copy.Currency = cloneFinanceString(input.Currency)
	copy.UpstreamObservedAt = cloneFinanceTime(input.UpstreamObservedAt)
	copy.ListCostDelta = cloneFinanceDecimal(input.ListCostDelta)
	copy.ActualCostDelta = cloneFinanceDecimal(input.ActualCostDelta)
	copy.ObservedMultiplier = cloneFinanceDecimal(input.ObservedMultiplier)
	copy.AnomalyCode = cloneFinanceString(input.AnomalyCode)
	copy.MultiplierEffectiveAt = cloneFinanceTime(input.MultiplierEffectiveAt)
	return &copy
}

func financeDecimalPointersEqual(left, right *decimal.Decimal) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
