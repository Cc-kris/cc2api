package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	FinanceProtocolTypeBuiltin  = "builtin"
	FinanceProtocolTypeHTTPJSON = "http_json"
	FinanceProtocolTypePlugin   = "plugin"

	FinanceProtocolStatusDraft     = "draft"
	FinanceProtocolStatusPublished = "published"
	FinanceProtocolStatusDisabled  = "disabled"

	FinanceCapabilityPricing             = "pricing"
	FinanceCapabilityAccountUsage        = "account_usage"
	FinanceCapabilityRequestCharge       = "request_charge"
	FinanceCapabilityBalance             = "balance"
	FinanceCapabilityFundingTransactions = "funding_transactions"
	FinanceCapabilityQuota               = "quota"

	FinanceCostModeRequestCharge           = "request_charge"
	FinanceCostModeCumulativeListAndActual = "cumulative_list_and_actual"
	FinanceCostModeCumulativeActual        = "cumulative_actual"
	FinanceCostModeContractMultiplier      = "contract_multiplier"
	FinanceCostModeManual                  = "manual"

	FinanceUnitFiatCurrency   = "fiat_currency"
	FinanceUnitPlatformCredit = "platform_credit"
	FinanceUnitNone           = "none"

	FinanceCounterScopeAccount      = "account"
	FinanceCounterScopeWallet       = "wallet"
	FinanceCounterScopeOrganization = "organization"
)

var (
	ErrUpstreamFinanceProtocolNotFound     = errors.New("upstream finance protocol not found")
	ErrUpstreamFinanceProtocolConflict     = errors.New("upstream finance protocol conflict")
	ErrUpstreamFinanceProtocolInvalidState = errors.New("upstream finance protocol has invalid state")
	ErrUpstreamFinanceProtocolUnsafe       = errors.New("upstream finance protocol violates security policy")
	ErrUpstreamFinanceProtocolUnsupported  = errors.New("upstream finance protocol capability is unsupported")
)

type FinanceProtocolAuthentication struct {
	Type             string `json:"type"`
	CredentialSource string `json:"credential_source"`
	HeaderName       string `json:"header_name,omitempty"`
}

type FinanceProtocolRecognitionRule struct {
	Method      string                          `json:"method"`
	Path        string                          `json:"path"`
	Match       FinanceProtocolRecognitionMatch `json:"match"`
	Platform    string                          `json:"platform,omitempty"`
	AccountType string                          `json:"account_type,omitempty"`
}

type FinanceProtocolRecognitionMatch struct {
	Path   string `json:"path"`
	Exists *bool  `json:"exists,omitempty"`
	Equals any    `json:"equals,omitempty"`
	Status int    `json:"status,omitempty"`
}

type FinanceProtocolPagination struct {
	Type            string `json:"type"`
	PageParameter   string `json:"page_parameter,omitempty"`
	CursorPath      string `json:"cursor_path,omitempty"`
	CursorParameter string `json:"cursor_parameter,omitempty"`
	MaxPages        int    `json:"max_pages,omitempty"`
}

type FinanceProtocolOperation struct {
	Method       string                     `json:"method"`
	Path         string                     `json:"path"`
	Headers      map[string]string          `json:"headers,omitempty"`
	Body         any                        `json:"body,omitempty"`
	Mapping      map[string]string          `json:"mapping"`
	Pagination   *FinanceProtocolPagination `json:"pagination,omitempty"`
	SSEEvent     string                     `json:"sse_event,omitempty"`
	EvidenceType string                     `json:"evidence_type,omitempty"`
}

type FinanceProtocolConfig struct {
	Capabilities   []string                            `json:"capabilities"`
	Recognition    []FinanceProtocolRecognitionRule    `json:"recognition,omitempty"`
	Authentication FinanceProtocolAuthentication       `json:"authentication"`
	Operations     map[string]FinanceProtocolOperation `json:"operations"`
	CostMode       string                              `json:"cost_mode"`
	UnitSemantics  string                              `json:"unit_semantics,omitempty"`
	CounterScope   string                              `json:"counter_scope,omitempty"`
	RedactPaths    []string                            `json:"redact_paths,omitempty"`
}

type FinanceProtocolValidationIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type FinanceProtocolValidationResult struct {
	Valid    bool                             `json:"valid"`
	Issues   []FinanceProtocolValidationIssue `json:"issues"`
	Checksum string                           `json:"checksum,omitempty"`
}

type UpstreamFinanceProtocol struct {
	ID               int64                           `json:"id"`
	Code             string                          `json:"code"`
	Name             string                          `json:"name"`
	ProtocolType     string                          `json:"protocol_type"`
	Status           string                          `json:"status"`
	CurrentVersionID *int64                          `json:"current_version_id,omitempty"`
	CreatedBy        *int64                          `json:"created_by,omitempty"`
	UpdatedBy        *int64                          `json:"updated_by,omitempty"`
	CreatedAt        time.Time                       `json:"created_at"`
	UpdatedAt        time.Time                       `json:"updated_at"`
	CurrentVersion   *UpstreamFinanceProtocolVersion `json:"current_version,omitempty"`
}

type UpstreamFinanceProtocolVersion struct {
	ID               int64                           `json:"id"`
	ProtocolID       int64                           `json:"protocol_id"`
	Version          int                             `json:"version"`
	Config           FinanceProtocolConfig           `json:"config"`
	Checksum         string                          `json:"checksum"`
	ValidationStatus string                          `json:"validation_status"`
	ValidationResult FinanceProtocolValidationResult `json:"validation_result"`
	PublishedAt      *time.Time                      `json:"published_at,omitempty"`
	CreatedBy        *int64                          `json:"created_by,omitempty"`
	CreatedAt        time.Time                       `json:"created_at"`
}

type FinanceProtocolListFilter struct {
	Status       string
	ProtocolType string
	Page         int
	PageSize     int
}

type FinanceProtocolCreateInput struct {
	Code         string                `json:"code"`
	Name         string                `json:"name"`
	ProtocolType string                `json:"protocol_type"`
	Config       FinanceProtocolConfig `json:"config"`
	OperatorID   *int64                `json:"-"`
}

type FinanceProtocolDraftInput struct {
	Name       string                `json:"name"`
	Config     FinanceProtocolConfig `json:"config"`
	OperatorID *int64                `json:"-"`
}

type FinanceProtocolTestInput struct {
	BaseURL    string `json:"base_url"`
	Credential string `json:"credential,omitempty"`
	Operation  string `json:"operation,omitempty"`
}

type FinanceProtocolExecutionResult struct {
	Capability       string         `json:"capability"`
	UnitSemantics    string         `json:"unit_semantics"`
	Facts            map[string]any `json:"facts"`
	SafeSnapshot     any            `json:"safe_snapshot,omitempty"`
	SnapshotChecksum string         `json:"snapshot_checksum,omitempty"`
	StatusCode       int            `json:"status_code,omitempty"`
	DurationMS       int64          `json:"duration_ms"`
	ErrorCode        string         `json:"error_code,omitempty"`
	ErrorSummary     string         `json:"error_summary,omitempty"`
}

// FinanceProtocolDetectionAudit is intentionally credential-free. The base URL
// is represented by a SHA-256 digest so an operator can trace a decision
// without exposing supplier endpoints or account secrets in finance reports.
type FinanceProtocolDetectionAudit struct {
	AccountID         int64
	ProtocolID        *int64
	ProtocolVersionID *int64
	Status            string
	Reason            string
	Platform          string
	AccountType       string
	BaseURLHash       string
	Candidates        []FinanceProtocolDetectionCandidate
	OperatorID        *int64
}

type FinanceFundingTransactionFact struct {
	TransactionID    string `json:"transaction_id"`
	PaidAmount       string `json:"paid_amount"`
	PaidCurrency     string `json:"paid_currency"`
	FXRateToUSD      string `json:"fx_rate_to_usd,omitempty"`
	FXSource         string `json:"fx_source,omitempty"`
	FXObservedAt     string `json:"fx_observed_at,omitempty"`
	BaseCreditUnits  string `json:"base_credit_units"`
	BonusCreditUnits string `json:"bonus_credit_units"`
	OccurredAt       string `json:"occurred_at"`
}

type UpstreamFinanceProtocolRepository interface {
	ListProtocols(context.Context, FinanceProtocolListFilter) ([]UpstreamFinanceProtocol, int64, error)
	GetProtocol(context.Context, int64) (*UpstreamFinanceProtocol, error)
	GetVersion(context.Context, int64) (*UpstreamFinanceProtocolVersion, error)
	ListVersions(context.Context, int64) ([]UpstreamFinanceProtocolVersion, error)
	CreateProtocol(context.Context, FinanceProtocolCreateInput, FinanceProtocolValidationResult) (*UpstreamFinanceProtocol, error)
	CreateDraftVersion(context.Context, int64, FinanceProtocolDraftInput, FinanceProtocolValidationResult) (*UpstreamFinanceProtocolVersion, error)
	PublishVersion(context.Context, int64, int64, *int64) error
	DisableProtocol(context.Context, int64, *int64) error
	DeleteDraft(context.Context, int64) error
	CreateDetectionAudit(context.Context, FinanceProtocolDetectionAudit) error
}

type UpstreamFinanceProtocolService struct {
	repo     UpstreamFinanceProtocolRepository
	executor *UpstreamFinanceHTTPExecutor
	detector *UpstreamFinanceProtocolDetector
}

func NewUpstreamFinanceProtocolService(repo UpstreamFinanceProtocolRepository, executor *UpstreamFinanceHTTPExecutor) *UpstreamFinanceProtocolService {
	return &UpstreamFinanceProtocolService{repo: repo, executor: executor, detector: NewUpstreamFinanceProtocolDetector(executor)}
}

func (s *UpstreamFinanceProtocolService) List(ctx context.Context, filter FinanceProtocolListFilter) ([]UpstreamFinanceProtocol, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	return s.repo.ListProtocols(ctx, filter)
}

func (s *UpstreamFinanceProtocolService) Get(ctx context.Context, id int64) (*UpstreamFinanceProtocol, error) {
	if id <= 0 {
		return nil, financeValidationError("protocol id must be positive")
	}
	return s.repo.GetProtocol(ctx, id)
}

func (s *UpstreamFinanceProtocolService) Create(ctx context.Context, input FinanceProtocolCreateInput) (*UpstreamFinanceProtocol, error) {
	input.Code = strings.TrimSpace(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	if err := ValidateFinanceProtocolCode(input.Code); err != nil {
		return nil, err
	}
	if input.Name == "" || len([]rune(input.Name)) > 120 {
		return nil, financeValidationError("name must be 1 to 120 characters")
	}
	result := ValidateFinanceProtocolConfig(input.ProtocolType, input.Config)
	if !result.Valid {
		return nil, financeValidationError("protocol config is invalid")
	}
	return s.repo.CreateProtocol(ctx, input, result)
}

func (s *UpstreamFinanceProtocolService) UpdateDraft(ctx context.Context, id int64, input FinanceProtocolDraftInput) (*UpstreamFinanceProtocolVersion, error) {
	protocol, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	result := ValidateFinanceProtocolConfig(protocol.ProtocolType, input.Config)
	if !result.Valid {
		return nil, financeValidationError("protocol config is invalid")
	}
	return s.repo.CreateDraftVersion(ctx, id, input, result)
}

func (s *UpstreamFinanceProtocolService) Test(ctx context.Context, id int64, input FinanceProtocolTestInput) (*FinanceProtocolExecutionResult, error) {
	protocol, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	var version *UpstreamFinanceProtocolVersion
	if protocol.CurrentVersion != nil {
		version = protocol.CurrentVersion
	} else {
		versions, listErr := s.repo.ListVersions(ctx, id)
		if listErr != nil || len(versions) == 0 {
			return nil, listErr
		}
		version = &versions[0]
	}
	operation := input.Operation
	if operation == "" && len(version.Config.Capabilities) > 0 {
		operation = version.Config.Capabilities[0]
	}
	return s.executor.Execute(ctx, version.Config, operation, input.BaseURL, input.Credential)
}

func (s *UpstreamFinanceProtocolService) Publish(ctx context.Context, id, versionID int64, operatorID *int64) error {
	version, err := s.repo.GetVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if version.ProtocolID != id || version.ValidationStatus != "valid" {
		return ErrUpstreamFinanceProtocolInvalidState
	}
	return s.repo.PublishVersion(ctx, id, versionID, operatorID)
}

func (s *UpstreamFinanceProtocolService) Disable(ctx context.Context, id int64, operatorID *int64) error {
	return s.repo.DisableProtocol(ctx, id, operatorID)
}

func (s *UpstreamFinanceProtocolService) Copy(ctx context.Context, id int64, input FinanceProtocolCreateInput) (*UpstreamFinanceProtocol, error) {
	source, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if source.CurrentVersion == nil {
		return nil, ErrUpstreamFinanceProtocolInvalidState
	}
	input.ProtocolType = source.ProtocolType
	input.Config = source.CurrentVersion.Config
	return s.Create(ctx, input)
}

func (s *UpstreamFinanceProtocolService) DeleteDraft(ctx context.Context, id int64) error {
	return s.repo.DeleteDraft(ctx, id)
}

func (s *UpstreamFinanceProtocolService) Versions(ctx context.Context, id int64) ([]UpstreamFinanceProtocolVersion, error) {
	return s.repo.ListVersions(ctx, id)
}
