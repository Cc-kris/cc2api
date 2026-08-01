package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const financeInitializationWalletName = "系统财务余额"

var ErrFinanceInitializationInvalid = errors.New("finance initialization request is invalid")

// FinanceInitializationAccountService is intentionally narrow: initialization
// needs the audited account multiplier update path, not the full admin surface.
type FinanceInitializationAccountService interface {
	ListAccounts(ctx context.Context, page, pageSize int, platform, accountType, status, search string, groupID int64, privacyMode string, sortBy, sortOrder string) ([]Account, int64, error)
	UpdateAccount(ctx context.Context, id int64, input *UpdateAccountInput) (*Account, error)
}

type FinanceInitializationUpstreamService interface {
	List(ctx context.Context) ([]*Upstream, error)
	Get(ctx context.Context, id int64) (*Upstream, error)
	Update(ctx context.Context, id int64, input *UpstreamInput) (*Upstream, error)
}

type FinanceInitializationWalletService interface {
	List(ctx context.Context, upstreamID int64, includeDeleted bool) ([]UpstreamWallet, error)
	Create(ctx context.Context, upstreamID int64, input UpstreamWalletInput) (*UpstreamWallet, error)
}

type FinanceInitializationFundService interface {
	InitializeOpeningBalance(ctx context.Context, walletID int64, amount decimal.Decimal, currency string, occurredAt time.Time, operatorID *int64, note, idempotencyKey string) (*UpstreamFundEvent, bool, error)
	RecordBalanceSnapshot(ctx context.Context, walletID int64, amount decimal.Decimal, currency string, occurredAt time.Time, dedupeKey string) error
}

type FinanceInitializationProfileService interface {
	Get(ctx context.Context, accountID int64) (*AccountFinanceProfile, error)
	Save(ctx context.Context, accountID int64, input AccountFinanceProfileInput) (*AccountFinanceProfile, error)
}

type FinanceInitializationAccount struct {
	AccountID              int64   `json:"account_id"`
	AccountName            string  `json:"account_name"`
	Platform               string  `json:"platform"`
	Status                 string  `json:"status"`
	UpstreamID             *int64  `json:"upstream_id,omitempty"`
	UpstreamName           string  `json:"upstream_name,omitempty"`
	CurrentMultiplier      *string `json:"current_multiplier,omitempty"`
	FinanceProfileReady    bool    `json:"finance_profile_ready"`
	NeedsMultiplierConfirm bool    `json:"needs_multiplier_confirm"`
}

type FinanceInitializationUpstream struct {
	UpstreamID       int64   `json:"upstream_id"`
	UpstreamName     string  `json:"upstream_name"`
	BaseURL          string  `json:"base_url"`
	Currency         string  `json:"currency"`
	CurrentBalance   float64 `json:"current_balance"`
	AccountCount     int     `json:"account_count"`
	FinanceWalletSet bool    `json:"finance_wallet_set"`
}

type FinanceInitializationScan struct {
	Accounts  []FinanceInitializationAccount  `json:"accounts"`
	Upstreams []FinanceInitializationUpstream `json:"upstreams"`
}

type FinanceInitializationAccountInput struct {
	AccountID              int64  `json:"account_id"`
	UpstreamCostMultiplier string `json:"upstream_cost_multiplier"`
}

type FinanceInitializationUpstreamInput struct {
	UpstreamID     int64   `json:"upstream_id"`
	CurrentBalance float64 `json:"current_balance"`
}

type FinanceInitializationRequest struct {
	Accounts             []FinanceInitializationAccountInput  `json:"accounts"`
	Upstreams            []FinanceInitializationUpstreamInput `json:"upstreams"`
	Reason               string                               `json:"reason"`
	RecordOpeningBalance *bool                                `json:"record_opening_balance,omitempty"`
	OperatorID           int64                                `json:"-"`
}

type FinanceInitializationResult struct {
	InitializedAccounts  int64 `json:"initialized_accounts"`
	InitializedUpstreams int64 `json:"initialized_upstreams"`
	CreatedWallets       int64 `json:"created_wallets"`
}

type FinanceInitializationService struct {
	accounts  FinanceInitializationAccountService
	upstreams FinanceInitializationUpstreamService
	wallets   FinanceInitializationWalletService
	funds     FinanceInitializationFundService
	profiles  FinanceInitializationProfileService
	now       func() time.Time
}

func NewFinanceInitializationService(accounts FinanceInitializationAccountService, upstreams FinanceInitializationUpstreamService, wallets FinanceInitializationWalletService, funds FinanceInitializationFundService, profiles FinanceInitializationProfileService) *FinanceInitializationService {
	return &FinanceInitializationService{accounts: accounts, upstreams: upstreams, wallets: wallets, funds: funds, profiles: profiles, now: time.Now}
}

func (s *FinanceInitializationService) Scan(ctx context.Context) (*FinanceInitializationScan, error) {
	if s == nil || s.accounts == nil || s.upstreams == nil || s.wallets == nil {
		return nil, ErrFinanceInitializationInvalid
	}
	upstreams, err := s.upstreams.List(ctx)
	if err != nil {
		return nil, err
	}
	byURL := make(map[string]*Upstream, len(upstreams))
	for _, upstream := range upstreams {
		if upstream != nil {
			byURL[strings.ToLower(strings.TrimSpace(upstream.NormalizedBaseURL))] = upstream
		}
	}
	accounts, err := s.listAllAccounts(ctx)
	if err != nil {
		return nil, err
	}
	result := &FinanceInitializationScan{Accounts: make([]FinanceInitializationAccount, 0, len(accounts)), Upstreams: make([]FinanceInitializationUpstream, 0, len(upstreams))}
	accountCounts := make(map[int64]int)
	for _, account := range accounts {
		upstream := byURL[financeInitializationAccountURL(&account)]
		item := FinanceInitializationAccount{AccountID: account.ID, AccountName: account.Name, Platform: account.Platform, Status: account.Status, FinanceProfileReady: account.CurrentFinanceProfileID != nil, NeedsMultiplierConfirm: account.UpstreamCostMultiplier == nil}
		if account.UpstreamCostMultiplier != nil {
			value := account.UpstreamCostMultiplier.String()
			item.CurrentMultiplier = &value
		}
		if upstream != nil {
			id := upstream.ID
			item.UpstreamID = &id
			item.UpstreamName = upstream.Name
			accountCounts[id]++
		}
		result.Accounts = append(result.Accounts, item)
	}
	for _, upstream := range upstreams {
		if upstream == nil {
			continue
		}
		wallets, walletErr := s.wallets.List(ctx, upstream.ID, false)
		if walletErr != nil {
			return nil, walletErr
		}
		currency := "USD"
		for _, wallet := range wallets {
			if strings.EqualFold(strings.TrimSpace(wallet.Name), financeInitializationWalletName) && wallet.BalanceKind == "wallet_cash" {
				if value := strings.ToUpper(strings.TrimSpace(wallet.Currency)); value != "" {
					currency = value
				}
				break
			}
		}
		result.Upstreams = append(result.Upstreams, FinanceInitializationUpstream{UpstreamID: upstream.ID, UpstreamName: upstream.Name, BaseURL: upstream.BaseURL, Currency: currency, CurrentBalance: upstream.CurrentBalance, AccountCount: accountCounts[upstream.ID], FinanceWalletSet: hasFinanceInitializationWallet(wallets)})
	}
	sort.Slice(result.Accounts, func(i, j int) bool { return result.Accounts[i].AccountID < result.Accounts[j].AccountID })
	sort.Slice(result.Upstreams, func(i, j int) bool { return result.Upstreams[i].UpstreamID < result.Upstreams[j].UpstreamID })
	return result, nil
}

func (s *FinanceInitializationService) Apply(ctx context.Context, input FinanceInitializationRequest) (*FinanceInitializationResult, error) {
	if s == nil || s.accounts == nil || s.upstreams == nil || s.wallets == nil || s.funds == nil || s.profiles == nil || input.OperatorID <= 0 {
		return nil, ErrFinanceInitializationInvalid
	}
	reason := strings.TrimSpace(input.Reason)
	if len([]rune(reason)) < 5 || len([]rune(reason)) > 500 {
		return nil, fmt.Errorf("%w: reason must be 5 to 500 characters", ErrFinanceInitializationInvalid)
	}
	seenAccounts := map[int64]struct{}{}
	seenUpstreams := map[int64]struct{}{}
	for _, item := range input.Accounts {
		if item.AccountID <= 0 {
			return nil, fmt.Errorf("%w: account_id must be positive", ErrFinanceInitializationInvalid)
		}
		if _, exists := seenAccounts[item.AccountID]; exists {
			return nil, fmt.Errorf("%w: duplicate account_id", ErrFinanceInitializationInvalid)
		}
		if _, err := decimal.NewFromString(strings.TrimSpace(item.UpstreamCostMultiplier)); err != nil {
			return nil, fmt.Errorf("%w: upstream_cost_multiplier must be a decimal", ErrFinanceInitializationInvalid)
		}
		seenAccounts[item.AccountID] = struct{}{}
	}
	for _, item := range input.Upstreams {
		if item.UpstreamID <= 0 || item.CurrentBalance < 0 {
			return nil, fmt.Errorf("%w: upstream_id and current_balance are invalid", ErrFinanceInitializationInvalid)
		}
		if _, exists := seenUpstreams[item.UpstreamID]; exists {
			return nil, fmt.Errorf("%w: duplicate upstream_id", ErrFinanceInitializationInvalid)
		}
		seenUpstreams[item.UpstreamID] = struct{}{}
	}
	// Resolve every upstream and its finance wallet before the first write.
	// This prevents predictable lookup/wallet failures from leaving a batch
	// half-applied; concurrent database failures remain retryable and idempotent.
	upstreamPlans := make(map[int64]*Upstream, len(input.Upstreams))
	for _, item := range input.Upstreams {
		upstream, err := s.upstreams.Get(ctx, item.UpstreamID)
		if err != nil {
			return nil, err
		}
		if upstream == nil {
			return nil, fmt.Errorf("%w: upstream %d is unavailable", ErrFinanceInitializationInvalid, item.UpstreamID)
		}
		wallets, err := s.wallets.List(ctx, upstream.ID, false)
		if err != nil {
			return nil, err
		}
		for _, wallet := range wallets {
			if strings.EqualFold(strings.TrimSpace(wallet.Name), financeInitializationWalletName) && wallet.BalanceKind == "wallet_cash" && !wallet.Enabled {
				return nil, fmt.Errorf("%w: finance wallet for upstream %d is disabled", ErrFinanceInitializationInvalid, upstream.ID)
			}
		}
		upstreamPlans[item.UpstreamID] = upstream
	}

	result := &FinanceInitializationResult{}
	operatorID := input.OperatorID
	for _, item := range input.Accounts {
		multiplier := decimal.RequireFromString(strings.TrimSpace(item.UpstreamCostMultiplier))
		if err := ValidateUpstreamCostMultiplier(multiplier); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrFinanceInitializationInvalid, err)
		}
		_, profileErr := s.profiles.Get(ctx, item.AccountID)
		profileMissing := errors.Is(profileErr, ErrAccountFinanceProfileNotFound)
		if profileErr != nil && !profileMissing {
			return nil, profileErr
		}
		account, err := s.accounts.UpdateAccount(ctx, item.AccountID, &UpdateAccountInput{UpstreamCostMultiplier: &multiplier, UpstreamCostMultiplierChangeReason: "财务初始化：" + reason, OperatorID: &operatorID})
		if err != nil {
			return nil, err
		}
		// Updating the account multiplier already rolls the current finance
		// profile forward atomically when one exists. Only create a contract
		// profile for accounts that genuinely have no current profile; never
		// replace an existing request-charge or cumulative profile here.
		if profileMissing && account.CurrentFinanceProfileID == nil {
			effectiveAt := s.now().UTC()
			if _, err = s.profiles.Save(ctx, item.AccountID, AccountFinanceProfileInput{CostMode: FinanceCostModeContractMultiplier, EndpointSource: "account_base_url", EndpointBaseURLSnapshot: financeInitializationAccountEndpoint(account), CredentialSource: "", CounterScope: FinanceCounterScopeAccount, BalanceUnitSemantics: FinanceUnitNone, EffectiveFrom: effectiveAt, ExpectedVersion: 0, Reason: "财务初始化：" + reason, OperatorID: operatorID}); err != nil {
				return nil, err
			}
		}
		result.InitializedAccounts++
	}
	for _, item := range input.Upstreams {
		upstream := upstreamPlans[item.UpstreamID]
		var err error
		wallet, created, err := s.ensureFinanceWallet(ctx, upstream)
		if err != nil {
			return nil, err
		}
		if created {
			result.CreatedWallets++
		}
		currency := strings.ToUpper(strings.TrimSpace(wallet.Currency))
		if currency == "" {
			currency = "USD"
		}
		now := s.now().UTC()
		amount := decimal.NewFromFloat(item.CurrentBalance)
		dedupeKey := fmt.Sprintf("finance-initialization:%d:%0.10f", upstream.ID, item.CurrentBalance)
		if input.RecordOpeningBalance != nil && !*input.RecordOpeningBalance {
			err = s.funds.RecordBalanceSnapshot(ctx, wallet.ID, amount, currency, now, "manual-balance:"+dedupeKey)
		} else {
			_, _, err = s.funds.InitializeOpeningBalance(ctx, wallet.ID, amount, currency, now, &operatorID, "财务初始化期初余额："+reason, dedupeKey)
		}
		if err != nil {
			return nil, err
		}
		// The legacy upstream balance is a derived compatibility field. Persist
		// the authoritative finance wallet snapshot/event first so a failure in
		// the legacy update cannot falsely report a balance that finance never
		// accepted; retries remain idempotent.
		initialBalance := item.CurrentBalance + upstream.ConsumedBalance
		if _, err = s.upstreams.Update(ctx, upstream.ID, &UpstreamInput{BaseURL: upstream.BaseURL, Name: upstream.Name, RateMultiplier: upstream.RateMultiplier, PlatformRates: upstream.PlatformRates, InitialBalance: initialBalance, BalanceAlertEnabled: upstream.BalanceAlertEnabled, AlertBalance: upstream.AlertBalance, Notes: upstream.Notes}); err != nil {
			return nil, err
		}
		result.InitializedUpstreams++
	}
	return result, nil
}

func (s *FinanceInitializationService) listAllAccounts(ctx context.Context) ([]Account, error) {
	items := make([]Account, 0)
	for page := 1; ; page++ {
		rows, total, err := s.accounts.ListAccounts(ctx, page, 200, "", "", "", "", 0, "", "id", "asc")
		if err != nil {
			return nil, err
		}
		items = append(items, rows...)
		if int64(len(items)) >= total || len(rows) == 0 {
			return items, nil
		}
	}
}

func (s *FinanceInitializationService) ensureFinanceWallet(ctx context.Context, upstream *Upstream) (*UpstreamWallet, bool, error) {
	wallets, err := s.wallets.List(ctx, upstream.ID, false)
	if err != nil {
		return nil, false, err
	}
	for index := range wallets {
		if strings.EqualFold(strings.TrimSpace(wallets[index].Name), financeInitializationWalletName) && wallets[index].BalanceKind == "wallet_cash" {
			return &wallets[index], false, nil
		}
	}
	enabled := true
	wallet, err := s.wallets.Create(ctx, upstream.ID, UpstreamWalletInput{Name: financeInitializationWalletName, AdapterType: UpstreamAdapterManual, Currency: "USD", BalanceKind: "wallet_cash", Enabled: &enabled})
	if err != nil {
		return nil, false, err
	}
	return wallet, true, nil
}

func hasFinanceInitializationWallet(wallets []UpstreamWallet) bool {
	for _, wallet := range wallets {
		if strings.EqualFold(strings.TrimSpace(wallet.Name), financeInitializationWalletName) && wallet.BalanceKind == "wallet_cash" {
			return true
		}
	}
	return false
}

func financeInitializationAccountURL(account *Account) string {
	if account == nil {
		return ""
	}
	if raw := strings.TrimSpace(account.GetCredential("base_url")); raw != "" {
		return strings.ToLower(NormalizeUpstreamBaseURLForRepo(raw))
	}
	switch strings.ToLower(strings.TrimSpace(account.Platform)) {
	case "openai":
		return "https://api.openai.com"
	case "gemini":
		return "https://generativelanguage.googleapis.com"
	case "antigravity":
		if account.Type == "api_key" || account.Type == "apikey" {
			return "https://api.anthropic.com/antigravity"
		}
	}
	if account.Type == "api_key" || account.Type == "apikey" {
		return "https://api.anthropic.com"
	}
	return ""
}

func financeInitializationAccountEndpoint(account *Account) string {
	if account == nil {
		return ""
	}
	return strings.TrimSpace(account.GetCredential("base_url"))
}
