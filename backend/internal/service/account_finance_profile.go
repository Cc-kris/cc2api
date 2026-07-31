package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	AccountFinanceReadinessReadyExact        = "ready_exact"
	AccountFinanceReadinessReadyPriced       = "ready_priced"
	AccountFinanceReadinessReadyContract     = "ready_contract"
	AccountFinanceReadinessPendingSettlement = "pending_settlement"
	AccountFinanceReadinessSyncError         = "sync_error"
	AccountFinanceReadinessUnconfigured      = "unconfigured"
)

var (
	ErrAccountFinanceProfileInvalid  = errors.New("account finance profile is invalid")
	ErrAccountFinanceProfileConflict = errors.New("account finance profile version conflict")
	ErrAccountFinanceProfileNotFound = errors.New("account finance profile not found")
)

type AccountFinanceProfile struct {
	ID                         int64            `json:"id"`
	AccountID                  int64            `json:"account_id"`
	WalletID                   *int64           `json:"wallet_id,omitempty"`
	ProtocolVersionID          *int64           `json:"protocol_version_id,omitempty"`
	CostMode                   string           `json:"cost_mode"`
	PricingGroup               *string          `json:"pricing_group,omitempty"`
	EndpointSource             string           `json:"endpoint_source"`
	EndpointBaseURLSnapshot    string           `json:"endpoint_base_url_snapshot"`
	CredentialSource           string           `json:"credential_source"`
	CounterScope               string           `json:"counter_scope"`
	CounterScopeKey            *string          `json:"counter_scope_key,omitempty"`
	BalanceUnitSemantics       string           `json:"balance_unit_semantics"`
	RechargeOwnerType          *string          `json:"recharge_owner_type,omitempty"`
	RechargeOwnerID            *int64           `json:"recharge_owner_id,omitempty"`
	AccountMultiplierChangeID  *int64           `json:"account_multiplier_change_id,omitempty"`
	AccountMultiplierSnapshot  *decimal.Decimal `json:"account_multiplier_snapshot,omitempty"`
	RawUpstreamMultiplier      *decimal.Decimal `json:"raw_upstream_multiplier,omitempty"`
	ContractType               *string          `json:"contract_type,omitempty"`
	ContractMultiplier         *decimal.Decimal `json:"contract_multiplier,omitempty"`
	ContractMultiplierChangeID *int64           `json:"contract_multiplier_change_id,omitempty"`
	ReadinessStatus            string           `json:"readiness_status"`
	ReadinessDetail            map[string]any   `json:"readiness_detail"`
	Version                    int              `json:"version"`
	EffectiveFrom              time.Time        `json:"effective_from"`
	EffectiveTo                *time.Time       `json:"effective_to,omitempty"`
	CreatedBy                  *int64           `json:"created_by,omitempty"`
	Reason                     string           `json:"reason"`
	CreatedAt                  time.Time        `json:"created_at"`
}

type AccountFinanceProfileInput struct {
	WalletID                *int64           `json:"wallet_id"`
	ProtocolVersionID       *int64           `json:"protocol_version_id"`
	CostMode                string           `json:"cost_mode"`
	PricingGroup            *string          `json:"pricing_group"`
	EndpointSource          string           `json:"endpoint_source"`
	EndpointBaseURLSnapshot string           `json:"endpoint_base_url_snapshot"`
	CredentialSource        string           `json:"credential_source"`
	CounterScope            string           `json:"counter_scope"`
	CounterScopeKey         *string          `json:"counter_scope_key"`
	BalanceUnitSemantics    string           `json:"balance_unit_semantics"`
	RechargeOwnerType       *string          `json:"recharge_owner_type"`
	RechargeOwnerID         *int64           `json:"recharge_owner_id"`
	ContractType            *string          `json:"contract_type"`
	ContractMultiplier      *decimal.Decimal `json:"contract_multiplier"`
	EffectiveFrom           time.Time        `json:"effective_from"`
	ExpectedVersion         int              `json:"expected_version"`
	Reason                  string           `json:"reason"`
	OperatorID              int64            `json:"-"`
}

type AccountFinanceReadinessEvidence struct {
	AccountMultiplierChangeID *int64
	AccountMultiplier         *decimal.Decimal
	LatestSyncFailed          bool
	HasSettledInterval        bool
	HasActiveCatalogPrice     bool
	ProtocolReady             bool
}

type AccountFinanceReadiness struct {
	AccountID int64                  `json:"account_id"`
	Status    string                 `json:"status"`
	Issues    []string               `json:"issues"`
	Actions   []string               `json:"actions"`
	Profile   *AccountFinanceProfile `json:"profile,omitempty"`
}

type AccountFinanceProfileRepository interface {
	CurrentAccountFinanceProfile(ctx context.Context, accountID int64) (*AccountFinanceProfile, error)
	ReplaceAccountFinanceProfile(ctx context.Context, accountID int64, input AccountFinanceProfileInput, profile AccountFinanceProfile) (*AccountFinanceProfile, error)
	AccountFinanceReadinessEvidence(ctx context.Context, accountID int64, profile *AccountFinanceProfile) (AccountFinanceReadinessEvidence, error)
}

type AccountFinanceProfileService struct {
	repo AccountFinanceProfileRepository
}

func NewAccountFinanceProfileService(repo AccountFinanceProfileRepository) *AccountFinanceProfileService {
	return &AccountFinanceProfileService{repo: repo}
}

func (s *AccountFinanceProfileService) Get(ctx context.Context, accountID int64) (*AccountFinanceProfile, error) {
	if s == nil || s.repo == nil || accountID <= 0 {
		return nil, ErrAccountFinanceProfileInvalid
	}
	return s.repo.CurrentAccountFinanceProfile(ctx, accountID)
}

func (s *AccountFinanceProfileService) Save(ctx context.Context, accountID int64, input AccountFinanceProfileInput) (*AccountFinanceProfile, error) {
	if s == nil || s.repo == nil || accountID <= 0 {
		return nil, ErrAccountFinanceProfileInvalid
	}
	profile, err := normalizeAccountFinanceProfile(accountID, input)
	if err != nil {
		return nil, err
	}
	evidence, err := s.repo.AccountFinanceReadinessEvidence(ctx, accountID, &profile)
	if err != nil {
		return nil, err
	}
	profile.AccountMultiplierChangeID = evidence.AccountMultiplierChangeID
	profile.AccountMultiplierSnapshot = cloneFinanceDecimal(evidence.AccountMultiplier)
	// A contract-multiplier profile may intentionally leave the override blank:
	// in that case the account's versioned upstream multiplier is the contract
	// value for this new profile version. This never rewrites historical
	// profiles or request snapshots; it only materializes the value being saved.
	if profile.CostMode == FinanceCostModeContractMultiplier {
		if profile.ContractMultiplier == nil && evidence.AccountMultiplier != nil {
			profile.ContractMultiplier = cloneFinanceDecimal(evidence.AccountMultiplier)
			profile.ContractType = financeProfileStringPointer("multiplier")
			profile.ContractMultiplierChangeID = cloneFinanceProfileInt64Pointer(evidence.AccountMultiplierChangeID)
		} else if profile.ContractMultiplier != nil && profile.ContractType == nil {
			profile.ContractType = financeProfileStringPointer("multiplier")
		}
	}
	profile.ReadinessStatus, profile.ReadinessDetail = calculateAccountFinanceReadiness(&profile, evidence)
	return s.repo.ReplaceAccountFinanceProfile(ctx, accountID, input, profile)
}

func (s *AccountFinanceProfileService) Readiness(ctx context.Context, accountID int64) (*AccountFinanceReadiness, error) {
	profile, err := s.Get(ctx, accountID)
	if errors.Is(err, ErrAccountFinanceProfileNotFound) {
		return &AccountFinanceReadiness{AccountID: accountID, Status: AccountFinanceReadinessUnconfigured, Issues: []string{"尚未创建账号财务配置"}, Actions: []string{"配置成本模式和财务协议"}}, nil
	}
	if err != nil {
		return nil, err
	}
	evidence, err := s.repo.AccountFinanceReadinessEvidence(ctx, accountID, profile)
	if err != nil {
		return nil, err
	}
	status, detail := calculateAccountFinanceReadiness(profile, evidence)
	issues := financeReadinessStrings(detail["issues"])
	actions := financeReadinessStrings(detail["actions"])
	profile.ReadinessStatus, profile.ReadinessDetail = status, detail
	return &AccountFinanceReadiness{AccountID: accountID, Status: status, Issues: issues, Actions: actions, Profile: profile}, nil
}

func normalizeAccountFinanceProfile(accountID int64, input AccountFinanceProfileInput) (AccountFinanceProfile, error) {
	input.CostMode = strings.ToLower(strings.TrimSpace(input.CostMode))
	if input.CostMode != FinanceCostModeRequestCharge && input.CostMode != FinanceCostModeCumulativeListAndActual && input.CostMode != FinanceCostModeCumulativeActual && input.CostMode != FinanceCostModeContractMultiplier && input.CostMode != FinanceCostModeManual {
		return AccountFinanceProfile{}, ErrAccountFinanceProfileInvalid
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len([]rune(input.Reason)) < 5 || len([]rune(input.Reason)) > 500 || input.OperatorID <= 0 {
		return AccountFinanceProfile{}, ErrAccountFinanceProfileInvalid
	}
	if input.EffectiveFrom.IsZero() {
		input.EffectiveFrom = time.Now().UTC()
	} else {
		input.EffectiveFrom = input.EffectiveFrom.UTC()
	}
	endpointSource := strings.TrimSpace(input.EndpointSource)
	if endpointSource == "" {
		endpointSource = "account_base_url"
	}
	if endpointSource != "account_base_url" && endpointSource != "wallet_base_url" {
		return AccountFinanceProfile{}, ErrAccountFinanceProfileInvalid
	}
	endpointSnapshot := strings.TrimSpace(input.EndpointBaseURLSnapshot)
	if endpointSnapshot != "" {
		var err error
		endpointSnapshot, err = sanitizeUpstreamWalletBaseURL(endpointSnapshot)
		if err != nil {
			return AccountFinanceProfile{}, ErrAccountFinanceProfileInvalid
		}
	}
	counterScope := strings.ToLower(strings.TrimSpace(input.CounterScope))
	if counterScope == "" {
		counterScope = FinanceCounterScopeAccount
	}
	if counterScope != FinanceCounterScopeAccount && counterScope != FinanceCounterScopeWallet && counterScope != FinanceCounterScopeOrganization {
		return AccountFinanceProfile{}, ErrAccountFinanceProfileInvalid
	}
	if (input.CostMode == FinanceCostModeCumulativeListAndActual || input.CostMode == FinanceCostModeCumulativeActual) && (input.WalletID == nil || input.ProtocolVersionID == nil) {
		return AccountFinanceProfile{}, ErrAccountFinanceProfileInvalid
	}
	if input.CostMode == FinanceCostModeRequestCharge && input.ProtocolVersionID == nil {
		return AccountFinanceProfile{}, ErrAccountFinanceProfileInvalid
	}
	contractType := normalizeOptionalString(input.ContractType)
	if input.CostMode == FinanceCostModeContractMultiplier && input.ContractMultiplier != nil && input.ContractMultiplier.IsNegative() {
		return AccountFinanceProfile{}, ErrAccountFinanceProfileInvalid
	}
	semantics := strings.ToLower(strings.TrimSpace(input.BalanceUnitSemantics))
	if semantics == "" {
		semantics = FinanceUnitNone
	}
	if semantics != FinanceUnitFiatCurrency && semantics != FinanceUnitPlatformCredit && semantics != FinanceUnitNone {
		return AccountFinanceProfile{}, ErrAccountFinanceProfileInvalid
	}
	return AccountFinanceProfile{
		AccountID: accountID, WalletID: input.WalletID, ProtocolVersionID: input.ProtocolVersionID,
		CostMode: input.CostMode, PricingGroup: normalizeOptionalString(input.PricingGroup), EndpointSource: endpointSource,
		EndpointBaseURLSnapshot: endpointSnapshot, CredentialSource: strings.TrimSpace(input.CredentialSource), CounterScope: counterScope,
		CounterScopeKey: normalizeOptionalString(input.CounterScopeKey), BalanceUnitSemantics: semantics,
		RechargeOwnerType: normalizeOptionalString(input.RechargeOwnerType), RechargeOwnerID: input.RechargeOwnerID,
		ContractType: contractType, ContractMultiplier: cloneFinanceDecimal(input.ContractMultiplier),
		EffectiveFrom: input.EffectiveFrom, CreatedBy: &input.OperatorID, Reason: input.Reason,
	}, nil
}

func financeProfileStringPointer(value string) *string {
	return &value
}

func cloneFinanceProfileInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func calculateAccountFinanceReadiness(profile *AccountFinanceProfile, evidence AccountFinanceReadinessEvidence) (string, map[string]any) {
	issues := make([]string, 0)
	actions := make([]string, 0)
	status := AccountFinanceReadinessUnconfigured
	if evidence.LatestSyncFailed {
		status = AccountFinanceReadinessSyncError
		issues = append(issues, "最近一次上游财务同步失败")
		actions = append(actions, "检查协议、凭据和上游接口")
	} else if profile != nil {
		switch profile.CostMode {
		case FinanceCostModeCumulativeListAndActual, FinanceCostModeCumulativeActual:
			if profile.CounterScope != FinanceCounterScopeAccount {
				issues = append(issues, "共享计数器尚未配置钱包级分摊")
				actions = append(actions, "改为账号独立计数器或配置钱包级分摊")
			} else if evidence.HasSettledInterval {
				status = AccountFinanceReadinessReadyExact
			} else {
				status = AccountFinanceReadinessPendingSettlement
				issues = append(issues, "等待第二个累计快照形成结算区间")
			}
		case FinanceCostModeRequestCharge:
			if profile.ProtocolVersionID != nil && evidence.ProtocolReady {
				status = AccountFinanceReadinessReadyExact
			} else {
				issues = append(issues, "未配置单次请求扣费协议")
				actions = append(actions, "绑定已发布的上游财务协议版本")
			}
		case FinanceCostModeContractMultiplier:
			if profile.ContractMultiplier != nil || evidence.AccountMultiplier != nil {
				status = AccountFinanceReadinessReadyContract
			} else {
				issues = append(issues, "未配置合同倍率")
				actions = append(actions, "填写账号上游倍率或合同倍率")
			}
		case FinanceCostModeManual:
			if evidence.HasActiveCatalogPrice {
				status = AccountFinanceReadinessReadyPriced
			} else if evidence.AccountMultiplier != nil {
				status = AccountFinanceReadinessReadyContract
			} else {
				issues = append(issues, "没有有效采购价格或账号倍率")
				actions = append(actions, "同步采购价格或配置账号上游倍率")
			}
		}
	}
	return status, map[string]any{"issues": issues, "actions": actions}
}

func financeReadinessStrings(value any) []string {
	items, _ := value.([]string)
	if items != nil {
		return items
	}
	raw, _ := value.([]any)
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}
