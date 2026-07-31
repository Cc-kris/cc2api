package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

const (
	UpstreamAdapterNewAPI              = "newapi"
	UpstreamAdapterLegacyOpenAIBilling = "legacy_openai_billing"
	UpstreamAdapterManual              = "manual"
	UpstreamAdapterProtocol            = "protocol"
)

var (
	ErrUpstreamWalletNotFound           = errors.New("upstream wallet not found")
	ErrUpstreamWalletDisabled           = errors.New("upstream wallet is disabled")
	ErrUpstreamWalletAssignmentConflict = errors.New("upstream wallet assignment conflicts with an existing effective range")
	ErrUpstreamWalletAssignmentTooEarly = errors.New("effective_at is earlier than the latest confirmed finance record")
	ErrUpstreamWalletAccountMismatch    = errors.New("account does not belong to the wallet upstream")
)

type UpstreamWallet struct {
	ID                   int64      `json:"id"`
	UpstreamID           int64      `json:"upstream_id"`
	Name                 string     `json:"name"`
	AdapterType          string     `json:"adapter_type"`
	BaseURL              string     `json:"base_url,omitempty"`
	CredentialConfigured bool       `json:"credential_configured"`
	Currency             string     `json:"currency"`
	BalanceKind          string     `json:"balance_kind"`
	BalanceScopeKey      string     `json:"balance_scope_key,omitempty"`
	PricingGroup         string     `json:"pricing_group,omitempty"`
	Enabled              bool       `json:"enabled"`
	LastPricingSyncAt    *time.Time `json:"last_pricing_sync_at"`
	PricingSyncStatus    string     `json:"pricing_sync_status"`
	PricingSyncError     string     `json:"pricing_sync_error,omitempty"`
	LastBalanceSyncAt    *time.Time `json:"last_balance_sync_at"`
	BalanceSyncStatus    string     `json:"balance_sync_status"`
	BalanceSyncError     string     `json:"balance_sync_error,omitempty"`
	LastQuotaSyncAt      *time.Time `json:"last_quota_sync_at"`
	QuotaSyncStatus      string     `json:"quota_sync_status"`
	QuotaSyncError       string     `json:"quota_sync_error,omitempty"`
	AssignedAccountCount int        `json:"assigned_account_count"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	DeletedAt            *time.Time `json:"deleted_at"`
	EncryptedCredential  []byte     `json:"-"`
	ProtocolVersionID    *int64     `json:"protocol_version_id,omitempty"`
}

type UpstreamWalletInput struct {
	Name              string  `json:"name"`
	AdapterType       string  `json:"adapter_type"`
	BaseURL           string  `json:"base_url"`
	Credential        *string `json:"credential,omitempty"`
	Currency          string  `json:"currency"`
	BalanceKind       string  `json:"balance_kind"`
	BalanceScopeKey   string  `json:"balance_scope_key,omitempty"`
	PricingGroup      string  `json:"pricing_group,omitempty"`
	Enabled           *bool   `json:"enabled,omitempty"`
	ProtocolVersionID *int64  `json:"protocol_version_id,omitempty"`
}

type UpstreamWalletAssignmentInput struct {
	AccountIDs  []int64   `json:"account_ids"`
	EffectiveAt time.Time `json:"effective_at"`
	Reason      string    `json:"reason"`
	OperatorID  *int64    `json:"-"`
}

type UpstreamWalletRepository interface {
	ListWallets(ctx context.Context, upstreamID int64, includeDeleted bool) ([]UpstreamWallet, error)
	GetWallet(ctx context.Context, id int64) (*UpstreamWallet, error)
	CreateWallet(ctx context.Context, wallet *UpstreamWallet, pricingAdapter, balanceAdapter, quotaAdapter string) error
	UpdateWallet(ctx context.Context, wallet *UpstreamWallet, pricingAdapter, balanceAdapter, quotaAdapter string, credentialProvided bool) error
	SoftDeleteWallet(ctx context.Context, id int64, deletedAt time.Time) error
	AssignWalletAccounts(ctx context.Context, walletID int64, input UpstreamWalletAssignmentInput) error
	ListActiveWalletAccountIDs(ctx context.Context, walletID int64, effectiveAt time.Time) ([]int64, error)
	IsBindableProtocolVersion(ctx context.Context, versionID int64) (bool, error)
}

type UpstreamWalletService struct {
	repository UpstreamWalletRepository
	encryptor  SecretEncryptor
}

func NewUpstreamWalletService(repository UpstreamWalletRepository, encryptor SecretEncryptor) *UpstreamWalletService {
	return &UpstreamWalletService{repository: repository, encryptor: encryptor}
}

func (s *UpstreamWalletService) List(ctx context.Context, upstreamID int64, includeDeleted bool) ([]UpstreamWallet, error) {
	if upstreamID <= 0 {
		return nil, financeValidationError("upstream_id must be positive")
	}
	return s.repository.ListWallets(ctx, upstreamID, includeDeleted)
}

func (s *UpstreamWalletService) Get(ctx context.Context, id int64) (*UpstreamWallet, error) {
	if id <= 0 {
		return nil, financeValidationError("wallet id must be positive")
	}
	return s.repository.GetWallet(ctx, id)
}

func (s *UpstreamWalletService) DecryptCredential(ctx context.Context, id int64) (string, error) {
	wallet, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if len(wallet.EncryptedCredential) == 0 {
		return "", nil
	}
	if s.encryptor == nil {
		return "", errors.New("credential decryption is unavailable")
	}
	credential, err := s.encryptor.Decrypt(string(wallet.EncryptedCredential))
	if err != nil {
		return "", fmt.Errorf("decrypt finance credential: %w", err)
	}
	return credential, nil
}

func (s *UpstreamWalletService) Create(ctx context.Context, upstreamID int64, input UpstreamWalletInput) (*UpstreamWallet, error) {
	if upstreamID <= 0 {
		return nil, financeValidationError("upstream_id must be positive")
	}
	wallet, pricingAdapter, balanceAdapter, quotaAdapter, _, err := s.prepareWallet(nil, upstreamID, input)
	if err != nil {
		return nil, err
	}
	if err = s.validateProtocolBinding(ctx, nil, wallet); err != nil {
		return nil, err
	}
	if err = s.repository.CreateWallet(ctx, wallet, pricingAdapter, balanceAdapter, quotaAdapter); err != nil {
		return nil, err
	}
	return wallet, nil
}

func (s *UpstreamWalletService) Update(ctx context.Context, id int64, input UpstreamWalletInput) (*UpstreamWallet, error) {
	current, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	wallet, pricingAdapter, balanceAdapter, quotaAdapter, credentialProvided, err := s.prepareWallet(current, current.UpstreamID, input)
	if err != nil {
		return nil, err
	}
	if err = s.validateProtocolBinding(ctx, current, wallet); err != nil {
		return nil, err
	}
	if err = s.repository.UpdateWallet(ctx, wallet, pricingAdapter, balanceAdapter, quotaAdapter, credentialProvided); err != nil {
		return nil, err
	}
	return wallet, nil
}

func (s *UpstreamWalletService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return financeValidationError("wallet id must be positive")
	}
	return s.repository.SoftDeleteWallet(ctx, id, time.Now())
}

func (s *UpstreamWalletService) AssignAccounts(ctx context.Context, walletID int64, input UpstreamWalletAssignmentInput) error {
	if walletID <= 0 || len(input.AccountIDs) == 0 {
		return financeValidationError("wallet id and account_ids are required")
	}
	if input.EffectiveAt.IsZero() {
		return financeValidationError("effective_at is required")
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len([]rune(input.Reason)) < 5 || len([]rune(input.Reason)) > 500 {
		return financeValidationError("reason must be 5 to 500 characters")
	}
	seen := map[int64]struct{}{}
	ids := make([]int64, 0, len(input.AccountIDs))
	for _, id := range input.AccountIDs {
		if id <= 0 {
			return financeValidationError("account_ids must contain positive values")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	input.AccountIDs = ids
	return s.repository.AssignWalletAccounts(ctx, walletID, input)
}

func (s *UpstreamWalletService) ListActiveAccountIDs(ctx context.Context, walletID int64, effectiveAt time.Time) ([]int64, error) {
	if walletID <= 0 {
		return nil, financeValidationError("wallet id must be positive")
	}
	if effectiveAt.IsZero() {
		effectiveAt = time.Now().UTC()
	}
	return s.repository.ListActiveWalletAccountIDs(ctx, walletID, effectiveAt.UTC())
}

func (s *UpstreamWalletService) prepareWallet(current *UpstreamWallet, upstreamID int64, input UpstreamWalletInput) (*UpstreamWallet, string, string, string, bool, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 120 {
		return nil, "", "", "", false, financeValidationError("wallet name is required and must not exceed 120 characters")
	}
	adapter := strings.ToLower(strings.TrimSpace(input.AdapterType))
	pricingAdapter, balanceAdapter, quotaAdapter, err := upstreamAdapterColumns(adapter)
	if err != nil {
		return nil, "", "", "", false, err
	}
	baseURL := ""
	if adapter != UpstreamAdapterManual || strings.TrimSpace(input.BaseURL) != "" {
		baseURL, err = sanitizeUpstreamWalletBaseURL(input.BaseURL)
		if err != nil {
			return nil, "", "", "", false, err
		}
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if len(currency) != 3 {
		return nil, "", "", "", false, financeValidationError("currency must be a 3-letter ISO code")
	}
	for _, char := range currency {
		if char < 'A' || char > 'Z' {
			return nil, "", "", "", false, financeValidationError("currency must be a 3-letter ISO code")
		}
	}
	balanceKind := strings.ToLower(strings.TrimSpace(input.BalanceKind))
	if balanceKind != "wallet_cash" && balanceKind != "token_quota" {
		return nil, "", "", "", false, financeValidationError("balance_kind must be wallet_cash or token_quota")
	}
	balanceScopeKey := strings.TrimSpace(input.BalanceScopeKey)
	if len([]rune(balanceScopeKey)) > 160 {
		return nil, "", "", "", false, financeValidationError("balance_scope_key must not exceed 160 characters")
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	} else if current != nil {
		enabled = current.Enabled
	}
	wallet := &UpstreamWallet{
		UpstreamID: upstreamID, Name: name, AdapterType: adapter, BaseURL: baseURL,
		Currency: currency, BalanceKind: balanceKind, BalanceScopeKey: balanceScopeKey,
		PricingGroup: strings.TrimSpace(input.PricingGroup), Enabled: enabled,
	}
	if adapter == UpstreamAdapterProtocol {
		wallet.ProtocolVersionID = input.ProtocolVersionID
		if wallet.ProtocolVersionID == nil && current != nil && current.AdapterType == UpstreamAdapterProtocol {
			wallet.ProtocolVersionID = current.ProtocolVersionID
		}
		if wallet.ProtocolVersionID == nil || *wallet.ProtocolVersionID <= 0 {
			return nil, "", "", "", false, financeValidationError("protocol_version_id is required for protocol wallets")
		}
	} else if input.ProtocolVersionID != nil {
		return nil, "", "", "", false, financeValidationError("protocol_version_id is only allowed for protocol wallets")
	}
	if current != nil {
		wallet.ID = current.ID
		wallet.EncryptedCredential = append([]byte(nil), current.EncryptedCredential...)
		wallet.CredentialConfigured = current.CredentialConfigured
		wallet.CreatedAt = current.CreatedAt
		wallet.LastPricingSyncAt, wallet.PricingSyncStatus, wallet.PricingSyncError = current.LastPricingSyncAt, current.PricingSyncStatus, current.PricingSyncError
		wallet.LastBalanceSyncAt, wallet.BalanceSyncStatus, wallet.BalanceSyncError = current.LastBalanceSyncAt, current.BalanceSyncStatus, current.BalanceSyncError
		wallet.LastQuotaSyncAt, wallet.QuotaSyncStatus, wallet.QuotaSyncError = current.LastQuotaSyncAt, current.QuotaSyncStatus, current.QuotaSyncError
	}
	credentialProvided := input.Credential != nil
	if credentialProvided {
		credential := strings.TrimSpace(*input.Credential)
		if credential == "" {
			return nil, "", "", "", false, financeValidationError("credential must not be empty when provided")
		}
		if s.encryptor == nil {
			return nil, "", "", "", false, errors.New("credential encryption is unavailable")
		}
		encrypted, encryptErr := s.encryptor.Encrypt(credential)
		if encryptErr != nil {
			return nil, "", "", "", false, fmt.Errorf("encrypt finance credential: %w", encryptErr)
		}
		wallet.EncryptedCredential = []byte(encrypted)
		wallet.CredentialConfigured = true
	}
	return wallet, pricingAdapter, balanceAdapter, quotaAdapter, credentialProvided, nil
}

func upstreamAdapterColumns(adapter string) (string, string, string, error) {
	switch adapter {
	case UpstreamAdapterNewAPI:
		return "newapi", "newapi_user", "newapi", nil
	case UpstreamAdapterLegacyOpenAIBilling:
		return "manual", "manual", "legacy_openai", nil
	case UpstreamAdapterManual:
		return "manual", "manual", "none", nil
	case UpstreamAdapterProtocol:
		return "protocol", "protocol", "protocol", nil
	default:
		return "", "", "", financeValidationError("adapter_type must be newapi, legacy_openai_billing, protocol or manual")
	}
}

func (s *UpstreamWalletService) validateProtocolBinding(ctx context.Context, current, wallet *UpstreamWallet) error {
	if wallet.AdapterType != UpstreamAdapterProtocol || wallet.ProtocolVersionID == nil {
		return nil
	}
	if current != nil && current.AdapterType == UpstreamAdapterProtocol && current.ProtocolVersionID != nil && *current.ProtocolVersionID == *wallet.ProtocolVersionID {
		return nil
	}
	bindable, err := s.repository.IsBindableProtocolVersion(ctx, *wallet.ProtocolVersionID)
	if err != nil {
		return err
	}
	if !bindable {
		return financeValidationError("protocol_version_id must reference a valid published protocol version")
	}
	return nil
}

func sanitizeUpstreamWalletBaseURL(raw string) (string, error) {
	normalized, err := urlvalidator.ValidateHTTPURL(strings.TrimSpace(raw), false, urlvalidator.ValidationOptions{})
	if err != nil {
		return "", financeValidationErrorf("invalid base_url: %v", err)
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", financeValidationErrorf("invalid base_url: %v", err)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}
