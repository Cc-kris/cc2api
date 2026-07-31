package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	UpstreamFinanceSyncPricing       = "pricing"
	UpstreamFinanceSyncBalance       = "balance"
	UpstreamFinanceSyncQuota         = "quota"
	UpstreamFinanceSyncFunding       = "funding"
	UpstreamFinanceSyncAccountUsage  = "account_usage"
	upstreamFinanceSyncLeaseDuration = 2 * time.Minute
)

var ErrUpstreamFinanceSyncLeaseLost = errors.New("upstream finance sync job lease lost")

type UpstreamFinanceSyncJob struct {
	ID             int64      `json:"id"`
	WalletID       int64      `json:"wallet_id"`
	SyncType       string     `json:"sync_type"`
	Status         string     `json:"status"`
	Progress       string     `json:"progress"`
	ProcessedCount int64      `json:"processed_count"`
	SuccessCount   int64      `json:"success_count"`
	FailedCount    int64      `json:"failed_count"`
	ErrorSummary   string     `json:"error_summary,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

type UpstreamFinanceSyncHistory struct {
	ID             int64      `json:"id"`
	AsyncJobID     *int64     `json:"async_job_id"`
	WalletID       int64      `json:"wallet_id"`
	SyncType       string     `json:"sync_type"`
	Status         string     `json:"status"`
	CollectedCount int64      `json:"collected_count"`
	SkippedCount   int64      `json:"skipped_count"`
	UpstreamStatus *int       `json:"upstream_status"`
	DurationMS     *int64     `json:"duration_ms"`
	ErrorSummary   string     `json:"error_summary,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type UpstreamFinancePriceVersion struct {
	ID            int64          `json:"id"`
	WalletID      int64          `json:"wallet_id"`
	ModelPattern  string         `json:"model_pattern"`
	IsWildcard    bool           `json:"is_wildcard"`
	BillingMode   string         `json:"billing_mode"`
	ServiceTier   string         `json:"service_tier,omitempty"`
	PriceDetail   map[string]any `json:"price_detail"`
	Currency      string         `json:"currency"`
	Source        string         `json:"source"`
	Checksum      string         `json:"checksum"`
	EffectiveFrom time.Time      `json:"effective_from"`
	EffectiveTo   *time.Time     `json:"effective_to"`
	CreatedAt     time.Time      `json:"created_at"`
}

type UpstreamFinancePriceListFilter struct {
	Model       string
	EffectiveAt *time.Time
	Page        int
	PageSize    int
}

type UpstreamFinanceSyncHistoryFilter struct {
	SyncType string
	Status   string
	Page     int
	PageSize int
}

type UpstreamFinanceSyncRequest struct {
	WalletID int64
	SyncType string
}

type UpstreamFinanceSyncRepository interface {
	CreateOrGetActiveSyncJob(ctx context.Context, walletID int64, syncType string, operatorID *int64) (*UpstreamFinanceSyncJob, bool, error)
	ClaimNextSyncJob(ctx context.Context, leaseOwner string, leaseUntil time.Time) (*UpstreamFinanceSyncJob, error)
	RenewSyncJobLease(ctx context.Context, jobID int64, leaseOwner string, leaseUntil time.Time) error
	CompletePricingSync(ctx context.Context, job *UpstreamFinanceSyncJob, leaseOwner string, prices []UpstreamFinancePrice, finishedAt time.Time) error
	CompleteBalanceSync(ctx context.Context, job *UpstreamFinanceSyncJob, leaseOwner string, balance *UpstreamFinanceBalance, finishedAt time.Time) error
	CompleteFundingSync(ctx context.Context, job *UpstreamFinanceSyncJob, leaseOwner string, collected, skipped int64, finishedAt time.Time) error
	CompleteAccountUsageSync(ctx context.Context, job *UpstreamFinanceSyncJob, leaseOwner string, collected, skipped int64, finishedAt time.Time) error
	FailSyncJob(ctx context.Context, job *UpstreamFinanceSyncJob, leaseOwner, summary string, finishedAt time.Time) error
	RecordProbe(ctx context.Context, walletID int64, probe UpstreamFinanceProbe) error
	ListPriceVersions(ctx context.Context, walletID int64, filter UpstreamFinancePriceListFilter) ([]UpstreamFinancePriceVersion, int64, error)
	ImportPriceVersions(ctx context.Context, walletID int64, prices []UpstreamFinancePrice, effectiveAt time.Time) (int64, int64, error)
	ListSyncHistory(ctx context.Context, walletID int64, filter UpstreamFinanceSyncHistoryFilter) ([]UpstreamFinanceSyncHistory, int64, error)
	ListDueSyncRequests(ctx context.Context, now time.Time) ([]UpstreamFinanceSyncRequest, error)
}

type UpstreamFinanceSyncService struct {
	wallets          *UpstreamWalletService
	registry         *UpstreamFinanceAdapterRegistry
	repo             UpstreamFinanceSyncRepository
	protocols        UpstreamFinanceProtocolRepository
	protocolExecutor *UpstreamFinanceHTTPExecutor
	funds            *UpstreamFundService
	accountSnapshots *AccountFinanceSnapshotService
	accounts         UpstreamFinanceSyncAccountRepository
}

type UpstreamFinanceSyncAccountRepository interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
	GetFinanceProfileByID(ctx context.Context, id int64) (*AccountFinanceProfile, error)
}

func NewUpstreamFinanceSyncService(wallets *UpstreamWalletService, registry *UpstreamFinanceAdapterRegistry, repo UpstreamFinanceSyncRepository, protocols UpstreamFinanceProtocolRepository, protocolExecutor *UpstreamFinanceHTTPExecutor, funds *UpstreamFundService, accountSnapshots *AccountFinanceSnapshotService, accounts UpstreamFinanceSyncAccountRepository) *UpstreamFinanceSyncService {
	return &UpstreamFinanceSyncService{
		wallets: wallets, registry: registry, repo: repo, protocols: protocols, protocolExecutor: protocolExecutor,
		funds: funds, accountSnapshots: accountSnapshots, accounts: accounts,
	}
}

func (s *UpstreamFinanceSyncService) Probe(ctx context.Context, walletID int64) (*UpstreamFinanceProbe, error) {
	wallet, credential, adapter, err := s.walletAdapter(ctx, walletID)
	if err != nil {
		return nil, err
	}
	probe, err := adapter.Probe(ctx, *wallet, credential)
	if err != nil {
		return nil, err
	}
	if err = s.repo.RecordProbe(ctx, walletID, probe); err != nil {
		return nil, err
	}
	return &probe, nil
}

func (s *UpstreamFinanceSyncService) Enqueue(ctx context.Context, walletID int64, syncType string, operatorID *int64) (*UpstreamFinanceSyncJob, bool, error) {
	wallet, err := s.wallets.Get(ctx, walletID)
	if err != nil {
		return nil, false, err
	}
	if !wallet.Enabled {
		return nil, false, ErrUpstreamWalletDisabled
	}
	syncType = strings.ToLower(strings.TrimSpace(syncType))
	if syncType != UpstreamFinanceSyncPricing && syncType != UpstreamFinanceSyncBalance && syncType != UpstreamFinanceSyncQuota && syncType != UpstreamFinanceSyncFunding && syncType != UpstreamFinanceSyncAccountUsage {
		return nil, false, financeValidationError("sync_type must be pricing, balance, quota, funding or account_usage")
	}
	if syncType == UpstreamFinanceSyncAccountUsage {
		_, walletCredential, adapter, adapterErr := s.walletAdapter(ctx, walletID)
		if adapterErr != nil {
			return nil, false, adapterErr
		}
		usageAdapter, ok := adapter.(UpstreamFinanceAccountUsageAdapter)
		if !ok || !usageAdapter.SupportsAccountUsage() {
			return nil, false, financeValidationError("wallet protocol does not support account_usage")
		}
		if usageAdapter.AccountUsageCredentialSource() == "wallet_finance_credential" && strings.TrimSpace(walletCredential) == "" {
			return nil, false, financeValidationError("wallet finance credential is required for account_usage")
		}
		accountIDs, listErr := s.wallets.ListActiveAccountIDs(ctx, walletID, time.Now().UTC())
		if listErr != nil {
			return nil, false, listErr
		}
		if len(accountIDs) == 0 {
			return nil, false, financeValidationError("wallet has no active bound accounts")
		}
		if usageAdapter.AccountUsageCredentialSource() == "wallet_finance_credential" && len(accountIDs) != 1 {
			return nil, false, financeValidationError("wallet finance credential account_usage requires exactly one active bound account")
		}
	}
	return s.repo.CreateOrGetActiveSyncJob(ctx, walletID, syncType, operatorID)
}

func (s *UpstreamFinanceSyncService) RunNext(ctx context.Context, leaseOwner string) (bool, error) {
	job, err := s.repo.ClaimNextSyncJob(ctx, leaseOwner, time.Now().UTC().Add(upstreamFinanceSyncLeaseDuration))
	if err != nil || job == nil {
		return false, err
	}
	syncCtx, stopHeartbeat, err := s.startLeaseHeartbeat(ctx, job.ID, leaseOwner)
	if err != nil {
		return true, err
	}
	wallet, credential, adapter, err := s.walletAdapter(syncCtx, job.WalletID)
	if err != nil {
		if leaseErr := stopHeartbeat(); leaseErr != nil {
			return true, leaseErr
		}
		return true, s.failJob(ctx, job, leaseOwner, err)
	}
	switch job.SyncType {
	case UpstreamFinanceSyncPricing:
		prices, fetchErr := runFinanceSyncWithRetry(syncCtx, func() ([]UpstreamFinancePrice, error) {
			return adapter.FetchPricing(syncCtx, *wallet, credential)
		})
		if leaseErr := stopHeartbeat(); leaseErr != nil {
			return true, leaseErr
		}
		if fetchErr != nil {
			return true, s.failJob(ctx, job, leaseOwner, fetchErr)
		}
		if err = s.repo.CompletePricingSync(ctx, job, leaseOwner, prices, time.Now().UTC()); err != nil {
			return true, err
		}
	case UpstreamFinanceSyncBalance:
		balance, fetchErr := runFinanceSyncWithRetry(syncCtx, func() (*UpstreamFinanceBalance, error) {
			return adapter.FetchBalance(syncCtx, *wallet, credential)
		})
		if leaseErr := stopHeartbeat(); leaseErr != nil {
			return true, leaseErr
		}
		if fetchErr != nil {
			return true, s.failJob(ctx, job, leaseOwner, fetchErr)
		}
		if err = s.repo.CompleteBalanceSync(ctx, job, leaseOwner, balance, time.Now().UTC()); err != nil {
			return true, err
		}
	case UpstreamFinanceSyncQuota:
		quota, fetchErr := runFinanceSyncWithRetry(syncCtx, func() (*UpstreamFinanceBalance, error) {
			return adapter.FetchQuota(syncCtx, *wallet, credential)
		})
		if leaseErr := stopHeartbeat(); leaseErr != nil {
			return true, leaseErr
		}
		if fetchErr != nil {
			return true, s.failJob(ctx, job, leaseOwner, fetchErr)
		}
		if err = s.repo.CompleteBalanceSync(ctx, job, leaseOwner, quota, time.Now().UTC()); err != nil {
			return true, err
		}
	case UpstreamFinanceSyncFunding:
		fundingAdapter, ok := adapter.(UpstreamFinanceFundingAdapter)
		if !ok || s.funds == nil {
			if leaseErr := stopHeartbeat(); leaseErr != nil {
				return true, leaseErr
			}
			return true, s.failJob(ctx, job, leaseOwner, ErrUpstreamFinanceCapabilityUnsupported)
		}
		transactions, fetchErr := runFinanceSyncWithRetry(syncCtx, func() ([]FinanceFundingTransactionFact, error) {
			return fundingAdapter.FetchFundingTransactions(syncCtx, *wallet, credential)
		})
		if fetchErr == nil {
			var created, skipped int64
			created, skipped, fetchErr = s.persistFundingTransactions(syncCtx, job.WalletID, transactions)
			if fetchErr == nil {
				if leaseErr := stopHeartbeat(); leaseErr != nil {
					return true, leaseErr
				}
				if err = s.repo.CompleteFundingSync(ctx, job, leaseOwner, created, skipped, time.Now().UTC()); err != nil {
					return true, err
				}
				break
			}
		}
		if leaseErr := stopHeartbeat(); leaseErr != nil {
			return true, leaseErr
		}
		return true, s.failJob(ctx, job, leaseOwner, fetchErr)
	case UpstreamFinanceSyncAccountUsage:
		collected, skipped, syncErr := s.syncAccountUsage(syncCtx, *wallet, credential, adapter)
		if leaseErr := stopHeartbeat(); leaseErr != nil {
			return true, leaseErr
		}
		if syncErr != nil {
			return true, s.failJob(ctx, job, leaseOwner, syncErr)
		}
		if err = s.repo.CompleteAccountUsageSync(ctx, job, leaseOwner, collected, skipped, time.Now().UTC()); err != nil {
			return true, err
		}
	default:
		if leaseErr := stopHeartbeat(); leaseErr != nil {
			return true, leaseErr
		}
		return true, s.failJob(ctx, job, leaseOwner, errors.New("unknown sync type"))
	}
	return true, nil
}

func (s *UpstreamFinanceSyncService) syncAccountUsage(ctx context.Context, wallet UpstreamWallet, walletCredential string, adapter UpstreamFinanceAdapter) (int64, int64, error) {
	if adapter == nil || s.accountSnapshots == nil || s.accounts == nil {
		return 0, 0, ErrUpstreamFinanceCapabilityUnsupported
	}
	accountIDs, err := s.wallets.ListActiveAccountIDs(ctx, wallet.ID, time.Now().UTC())
	if err != nil {
		return 0, 0, err
	}
	if len(accountIDs) == 0 {
		return 0, 0, financeValidationError("wallet has no active bound accounts")
	}
	var collected, skipped int64
	for _, accountID := range accountIDs {
		account, accountErr := s.accounts.GetByID(ctx, accountID)
		if accountErr != nil {
			return collected, skipped, accountErr
		}
		if !account.IsActive() {
			skipped++
			continue
		}
		if account.CurrentFinanceProfileID == nil {
			skipped++
			continue
		}
		profile, profileErr := s.accounts.GetFinanceProfileByID(ctx, *account.CurrentFinanceProfileID)
		if profileErr != nil {
			return collected, skipped, profileErr
		}
		if profile == nil || profile.AccountID != account.ID || profile.WalletID == nil || *profile.WalletID != wallet.ID ||
			(profile.CostMode != FinanceCostModeCumulativeListAndActual && profile.CostMode != FinanceCostModeCumulativeActual) {
			skipped++
			continue
		}
		profileID := profile.ID
		protocolVersionID := cloneInt64Pointer(profile.ProtocolVersionID)
		if protocolVersionID == nil {
			protocolVersionID = cloneInt64Pointer(wallet.ProtocolVersionID)
		}
		accountAdapter := adapter
		if profile.ProtocolVersionID != nil {
			accountAdapter, err = s.protocolAdapterForVersion(ctx, *profile.ProtocolVersionID)
			if err != nil {
				return collected, skipped, err
			}
		}
		usageAdapter, ok := accountAdapter.(UpstreamFinanceAccountUsageAdapter)
		if !ok || !usageAdapter.SupportsAccountUsage() {
			skipped++
			continue
		}
		credentialSource := strings.ToLower(strings.TrimSpace(profile.CredentialSource))
		if credentialSource == "" {
			credentialSource = usageAdapter.AccountUsageCredentialSource()
		}
		if usageAdapter.AccountUsageCounterScope() != FinanceCounterScopeAccount {
			return collected, skipped, financeValidationError("shared or unknown account_usage counter scope is observation-only until wallet-level allocation is configured")
		}
		if credentialSource == "wallet_finance_credential" && len(accountIDs) != 1 {
			return collected, skipped, financeValidationError("wallet finance credential account_usage requires exactly one active bound account")
		}
		credential, credentialErr := resolveAccountUsageCredential(account, credentialSource, walletCredential)
		if credentialErr != nil {
			return collected, skipped, credentialErr
		}
		requestWallet := wallet
		if credentialSource != "wallet_finance_credential" {
			if profileBaseURL := strings.TrimSpace(profile.EndpointBaseURLSnapshot); profileBaseURL != "" {
				requestWallet.BaseURL = profileBaseURL
			} else if accountBaseURL := strings.TrimSpace(account.GetCredential("base_url")); accountBaseURL != "" {
				requestWallet.BaseURL = accountBaseURL
			}
		}
		usage, fetchErr := runFinanceSyncWithRetry(ctx, func() (*UpstreamFinanceAccountUsage, error) {
			return usageAdapter.FetchAccountUsage(ctx, requestWallet, credential)
		})
		if fetchErr != nil {
			return collected, skipped, fetchErr
		}
		if usage == nil || strings.TrimSpace(usage.SnapshotChecksum) == "" {
			return collected, skipped, errors.New("account_usage result is incomplete")
		}
		counterIdentity, identityErr := accountUsageCounterIdentity(wallet, account.ID, usage)
		if identityErr != nil {
			return collected, skipped, identityErr
		}
		idempotencyKey := fmt.Sprintf("account-usage:%s:%d:%s", counterIdentity, account.ID, usage.SnapshotChecksum)
		scopeKey := fmt.Sprintf("counter:%s:account:%d", counterIdentity, account.ID)
		_, observeErr := s.accountSnapshots.ObserveCounter(ctx, AccountFinanceCounterObservation{
			AccountID: account.ID, AccountFinanceProfileID: &profileID, WalletID: wallet.ID, ProtocolVersionID: protocolVersionID,
			CounterIdentityKey: counterIdentity, ScopeKey: scopeKey, IdempotencyKey: idempotencyKey,
			UpstreamCounterID: usage.UpstreamCounterID, CounterPeriod: usage.CounterPeriod,
			ListCostTotal: usage.ListCostTotal, ActualCostTotal: usage.ActualCostTotal,
			UnitCode: usage.UnitCode, UnitSemantics: usage.UnitSemantics, Currency: usage.Currency,
			UpstreamObservedAt: usage.UpstreamObservedAt, CollectedAt: usage.CollectedAt, SafeSnapshot: usage.SafeSnapshot,
		})
		if observeErr != nil {
			return collected, skipped, observeErr
		}
		collected++
	}
	return collected, skipped, nil
}

func accountUsageCounterIdentity(wallet UpstreamWallet, accountID int64, usage *UpstreamFinanceAccountUsage) (string, error) {
	counterID := ""
	if usage != nil && usage.UpstreamCounterID != nil {
		counterID = strings.TrimSpace(*usage.UpstreamCounterID)
	}
	if counterID == "" {
		if accountID <= 0 {
			return "", financeValidationError("account_usage account identity is required when upstream_counter_id is absent")
		}
		counterID = fmt.Sprintf("local-account:%d", accountID)
	}
	period := ""
	if usage != nil && usage.CounterPeriod != nil {
		period = strings.TrimSpace(*usage.CounterPeriod)
	}
	financialDomain := strings.TrimSpace(wallet.BalanceScopeKey)
	if financialDomain == "" {
		financialDomain = "upstream"
	}
	identity := fmt.Sprintf("upstream=%d|financial-domain=%s|counter=%s|period=%s", wallet.UpstreamID, financialDomain, counterID, period)
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:]), nil
}

func resolveAccountUsageCredential(account *Account, source, walletCredential string) (string, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return "", nil
	}
	if source == "wallet_finance_credential" {
		if credential := strings.TrimSpace(walletCredential); credential != "" {
			return credential, nil
		}
		return "", errors.New("wallet finance credential is unavailable")
	}
	credential := accountFinanceProtocolCredential(account, source)
	if credential == "" {
		return "", fmt.Errorf("account %d credential source %s is unavailable", account.ID, source)
	}
	return credential, nil
}

func (s *UpstreamFinanceSyncService) persistFundingTransactions(ctx context.Context, walletID int64, transactions []FinanceFundingTransactionFact) (int64, int64, error) {
	var created, skipped int64
	for _, transaction := range transactions {
		occurredAt, err := time.Parse(time.RFC3339, strings.TrimSpace(transaction.OccurredAt))
		if err != nil {
			return created, skipped, fmt.Errorf("funding transaction %q has invalid occurred_at", transaction.TransactionID)
		}
		paid, err := decimal.NewFromString(strings.TrimSpace(transaction.PaidAmount))
		if err != nil || paid.LessThanOrEqual(decimal.Zero) {
			return created, skipped, fmt.Errorf("funding transaction %q has invalid paid_amount", transaction.TransactionID)
		}
		currency := strings.ToUpper(strings.TrimSpace(transaction.PaidCurrency))
		fxRate := strings.TrimSpace(transaction.FXRateToUSD)
		fxSource := strings.TrimSpace(transaction.FXSource)
		fxObservedAt := occurredAt
		if strings.TrimSpace(transaction.FXObservedAt) != "" {
			fxObservedAt, err = time.Parse(time.RFC3339, strings.TrimSpace(transaction.FXObservedAt))
			if err != nil {
				return created, skipped, fmt.Errorf("funding transaction %q has invalid fx_observed_at", transaction.TransactionID)
			}
		}
		if currency == "USD" && fxRate == "" {
			fxRate, fxSource = "1", "currency_identity"
		}
		if fxRate == "" || fxSource == "" {
			return created, skipped, fmt.Errorf("funding transaction %q requires frozen fx_rate_to_usd and fx_source for %s", transaction.TransactionID, currency)
		}
		rate, err := decimal.NewFromString(fxRate)
		if err != nil || rate.LessThanOrEqual(decimal.Zero) {
			return created, skipped, fmt.Errorf("funding transaction %q has invalid fx_rate_to_usd", transaction.TransactionID)
		}
		transactionID := strings.TrimSpace(transaction.TransactionID)
		if transactionID == "" {
			return created, skipped, errors.New("funding transaction has empty transaction_id")
		}
		_, inserted, err := s.funds.Create(ctx, walletID, UpstreamFundEventInput{
			EventType: "topup", OriginalAmount: paid.String(), Currency: currency, FXRateToUSD: rate.String(), FXSource: fxSource,
			FXObservedAt: fxObservedAt, USDAmount: paid.Mul(rate).Round(8).String(), BaseCreditUnits: transaction.BaseCreditUnits,
			BonusCreditUnits: transaction.BonusCreditUnits, OccurredAt: occurredAt, ReferenceNo: transactionID,
			Note: "上游充值交易自动同步", IdempotencyKey: fmt.Sprintf("upstream-funding:%d:%s", walletID, transactionID),
		})
		if err != nil {
			return created, skipped, fmt.Errorf("persist funding transaction %q: %w", transactionID, err)
		}
		if inserted {
			created++
		} else {
			skipped++
		}
	}
	return created, skipped, nil
}

func (s *UpstreamFinanceSyncService) startLeaseHeartbeat(ctx context.Context, jobID int64, leaseOwner string) (context.Context, func() error, error) {
	if err := s.repo.RenewSyncJobLease(ctx, jobID, leaseOwner, time.Now().UTC().Add(upstreamFinanceSyncLeaseDuration)); err != nil {
		return nil, nil, err
	}
	syncCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(upstreamFinanceSyncLeaseDuration / 4)
		defer ticker.Stop()
		for {
			select {
			case <-syncCtx.Done():
				done <- nil
				return
			case <-ticker.C:
				if err := s.repo.RenewSyncJobLease(syncCtx, jobID, leaseOwner, time.Now().UTC().Add(upstreamFinanceSyncLeaseDuration)); err != nil {
					cancel()
					done <- err
					return
				}
			}
		}
	}()
	return syncCtx, func() error {
		cancel()
		return <-done
	}, nil
}

func runFinanceSyncWithRetry[T any](ctx context.Context, operation func() (T, error)) (T, error) {
	var zero T
	delays := []time.Duration{0, 200 * time.Millisecond, time.Second}
	for attempt, delay := range delays {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return zero, ctx.Err()
			case <-timer.C:
			}
		}
		value, err := operation()
		if err == nil {
			return value, nil
		}
		if attempt == len(delays)-1 || !isRetriableFinanceSyncError(err) {
			return zero, err
		}
	}
	return zero, errors.New("finance sync retry exhausted")
}

func isRetriableFinanceSyncError(err error) bool {
	var statusErr *UpstreamFinanceHTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == 429 || statusErr.StatusCode >= 500
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "request failed") || strings.Contains(message, "timeout") || strings.Contains(message, "connection")
}

func (s *UpstreamFinanceSyncService) ListPrices(ctx context.Context, walletID int64, filter UpstreamFinancePriceListFilter) ([]UpstreamFinancePriceVersion, int64, error) {
	if _, err := s.wallets.Get(ctx, walletID); err != nil {
		return nil, 0, err
	}
	normalizeFinancePage(&filter.Page, &filter.PageSize)
	return s.repo.ListPriceVersions(ctx, walletID, filter)
}

func (s *UpstreamFinanceSyncService) ImportPrices(ctx context.Context, walletID int64, prices []UpstreamFinancePrice, effectiveAt time.Time) (int64, int64, error) {
	if _, err := s.wallets.Get(ctx, walletID); err != nil {
		return 0, 0, err
	}
	if len(prices) == 0 {
		return 0, 0, financeValidationError("prices are required")
	}
	for index := range prices {
		price := &prices[index]
		if price.EffectiveAt.IsZero() {
			if effectiveAt.IsZero() {
				return 0, 0, financeValidationErrorf("price row %d effective_at is required", index+1)
			}
			price.EffectiveAt = effectiveAt.UTC()
		}
		price.ModelPattern = strings.TrimSpace(price.ModelPattern)
		price.BillingMode = normalizeFinanceBillingMode(price.BillingMode)
		price.Currency = strings.ToUpper(strings.TrimSpace(price.Currency))
		price.Source = FinancePricingSourceManual
		if price.ModelPattern == "" || len(price.Currency) != 3 || price.PriceDetail == nil {
			return 0, 0, financeValidationErrorf("price row %d is incomplete", index+1)
		}
		if _, err := FinancePriceDetailFromMap(price.PriceDetail); err != nil {
			return 0, 0, financeValidationErrorf("price row %d: %v", index+1, err)
		}
	}
	return s.repo.ImportPriceVersions(ctx, walletID, prices, effectiveAt.UTC())
}

func (s *UpstreamFinanceSyncService) ListHistory(ctx context.Context, walletID int64, filter UpstreamFinanceSyncHistoryFilter) ([]UpstreamFinanceSyncHistory, int64, error) {
	if _, err := s.wallets.Get(ctx, walletID); err != nil {
		return nil, 0, err
	}
	normalizeFinancePage(&filter.Page, &filter.PageSize)
	return s.repo.ListSyncHistory(ctx, walletID, filter)
}

func (s *UpstreamFinanceSyncService) walletAdapter(ctx context.Context, walletID int64) (*UpstreamWallet, string, UpstreamFinanceAdapter, error) {
	wallet, err := s.wallets.Get(ctx, walletID)
	if err != nil {
		return nil, "", nil, err
	}
	if !wallet.Enabled {
		return nil, "", nil, ErrUpstreamWalletDisabled
	}
	credential, err := s.wallets.DecryptCredential(ctx, walletID)
	if err != nil {
		return nil, "", nil, err
	}
	if wallet.AdapterType == UpstreamAdapterProtocol {
		if wallet.ProtocolVersionID == nil || s.protocols == nil || s.protocolExecutor == nil {
			return nil, "", nil, errors.New("protocol wallet binding is incomplete")
		}
		adapter, adapterErr := s.protocolAdapterForVersion(ctx, *wallet.ProtocolVersionID)
		if adapterErr != nil {
			return nil, "", nil, adapterErr
		}
		return wallet, credential, adapter, nil
	}
	adapter, err := s.registry.Adapter(wallet.AdapterType)
	if err != nil {
		return nil, "", nil, err
	}
	return wallet, credential, adapter, nil
}

func (s *UpstreamFinanceSyncService) protocolAdapterForVersion(ctx context.Context, versionID int64) (UpstreamFinanceAdapter, error) {
	if versionID <= 0 || s.protocols == nil || s.protocolExecutor == nil {
		return nil, errors.New("protocol version binding is incomplete")
	}
	version, err := s.protocols.GetVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if version == nil || version.PublishedAt == nil || version.ValidationStatus != "valid" {
		return nil, ErrUpstreamFinanceProtocolInvalidState
	}
	protocol, err := s.protocols.GetProtocol(ctx, version.ProtocolID)
	if err != nil {
		return nil, err
	}
	if protocol == nil || protocol.Status != FinanceProtocolStatusPublished {
		return nil, ErrUpstreamFinanceProtocolInvalidState
	}
	return NewProtocolUpstreamFinanceAdapter(version.ID, protocol.Code, version.Config, s.protocolExecutor), nil
}

func (s *UpstreamFinanceSyncService) failJob(ctx context.Context, job *UpstreamFinanceSyncJob, leaseOwner string, cause error) error {
	summary := "sync failed"
	if errors.Is(cause, ErrUpstreamFinanceCapabilityUnsupported) {
		summary = "capability unsupported"
	} else if strings.Contains(strings.ToLower(cause.Error()), "credential") {
		summary = "credential unavailable or rejected"
	} else if strings.Contains(cause.Error(), "HTTP ") {
		summary = cause.Error()
	}
	if err := s.repo.FailSyncJob(ctx, job, leaseOwner, summary, time.Now().UTC()); err != nil {
		return err
	}
	return cause
}

func normalizeFinancePage(page, pageSize *int) {
	if *page <= 0 {
		*page = 1
	}
	if *pageSize <= 0 {
		*pageSize = 20
	}
	if *pageSize > 200 {
		*pageSize = 200
	}
}
