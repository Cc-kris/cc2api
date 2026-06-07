package service

import "time"

const (
	OpsUnifiedErrorResultFinalFailed   = "final_failed"
	OpsUnifiedErrorResultRecovered     = "recovered"
	OpsUnifiedErrorResultClientAborted = "client_aborted"
	OpsUnifiedErrorResultUnknown       = "unknown"

	OpsUnifiedAIAnalysisAll         = "all"
	OpsUnifiedAIAnalysisAnalyzed    = "analyzed"
	OpsUnifiedAIAnalysisNotAnalyzed = "not_analyzed"
)

type OpsUnifiedErrorListFilter struct {
	StartTime *time.Time
	EndTime   *time.Time

	ErrorCategories          []string
	ErrorSubcategories       []string
	ClientErrorSubcategories []string
	ErrorResults             []string
	Severities               []string
	StatusCodes              []int

	UserID            *int64
	APIKeyID          *int64
	GroupID           *int64
	Platform          string
	Model             string
	UpstreamAccountID *int64
	RequestID         string
	Keyword           string
	AIAnalysis        string

	SortBy    string
	SortOrder string
	Page      int
	PageSize  int
}

type OpsUnifiedErrorExportResult struct {
	Rows      [][]string
	Total     int
	Truncated bool
}

type OpsUnifiedErrorList struct {
	Items    []*OpsUnifiedErrorItem `json:"items"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

type OpsUnifiedEntityRef struct {
	ID      int64  `json:"id"`
	Name    string `json:"name,omitempty"`
	Email   string `json:"email,omitempty"`
	Display string `json:"display,omitempty"`
}

type OpsUnifiedErrorItem struct {
	ID                     int64     `json:"id"`
	OccurredAt             time.Time `json:"occurred_at"`
	ErrorCategory          string    `json:"error_category"`
	ErrorSubcategory       string    `json:"error_subcategory"`
	ClientErrorSubcategory *string   `json:"client_error_subcategory"`
	ErrorResult            string    `json:"error_result"`
	Severity               string    `json:"severity"`
	StatusCode             int       `json:"status_code"`

	User            *OpsUnifiedEntityRef `json:"user,omitempty"`
	APIKey          *OpsUnifiedEntityRef `json:"api_key,omitempty"`
	Group           *OpsUnifiedEntityRef `json:"group,omitempty"`
	Platform        string               `json:"platform"`
	Model           string               `json:"model"`
	UpstreamAccount *OpsUnifiedEntityRef `json:"upstream_account,omitempty"`

	Summary          string `json:"summary"`
	SameKindCount    int    `json:"same_kind_count"`
	AIAnalysisStatus string `json:"ai_analysis_status"`
}

func IsValidOpsUnifiedErrorResult(result string) bool {
	switch result {
	case OpsUnifiedErrorResultFinalFailed, OpsUnifiedErrorResultRecovered, OpsUnifiedErrorResultClientAborted, OpsUnifiedErrorResultUnknown:
		return true
	default:
		return false
	}
}

func IsValidOpsUnifiedSeverity(severity string) bool {
	switch severity {
	case "P0", "P1", "P2", "observe", "normal":
		return true
	default:
		return false
	}
}

func IsValidOpsUnifiedAIAnalysis(v string) bool {
	switch v {
	case "", OpsUnifiedAIAnalysisAll, OpsUnifiedAIAnalysisAnalyzed, OpsUnifiedAIAnalysisNotAnalyzed:
		return true
	default:
		return false
	}
}

type OpsUnifiedErrorDetail struct {
	Conclusion     OpsUnifiedErrorConclusion     `json:"conclusion"`
	RequestChain   OpsUnifiedErrorRequestChain   `json:"request_chain"`
	Classification OpsUnifiedErrorClassification `json:"classification"`
	ImpactScope    OpsUnifiedErrorImpactScope    `json:"impact_scope"`
	Recovery       OpsUnifiedErrorRecovery       `json:"recovery"`
	AIAnalysis     OpsUnifiedErrorAIAnalysis     `json:"ai_analysis"`
	RawRecord      OpsUnifiedErrorRawRecord      `json:"raw_record"`
	SameKindErrors []*OpsUnifiedErrorItem        `json:"same_kind_errors"`
}

type OpsUnifiedErrorConclusion struct {
	Title              string   `json:"title"`
	Summary            string   `json:"summary"`
	ErrorResult        string   `json:"error_result"`
	FinalFailed        bool     `json:"final_failed"`
	Recovered          bool     `json:"recovered"`
	AffectsUser        bool     `json:"affects_user"`
	RecommendedActions []string `json:"recommended_actions"`
}

type OpsUnifiedErrorRequestChain struct {
	User             *OpsUnifiedEntityRef `json:"user,omitempty"`
	APIKey           *OpsUnifiedEntityRef `json:"api_key,omitempty"`
	Group            *OpsUnifiedEntityRef `json:"group,omitempty"`
	Platform         string               `json:"platform"`
	Model            string               `json:"model"`
	RequestedModel   string               `json:"requested_model"`
	UpstreamModel    string               `json:"upstream_model"`
	RequestPath      string               `json:"request_path"`
	InboundEndpoint  string               `json:"inbound_endpoint"`
	UpstreamEndpoint string               `json:"upstream_endpoint"`
	UpstreamAccount  *OpsUnifiedEntityRef `json:"upstream_account,omitempty"`
	RequestID        string               `json:"request_id"`
	ClientRequestID  string               `json:"client_request_id"`
}

type OpsUnifiedErrorClassification struct {
	ErrorCategory            string   `json:"error_category"`
	ErrorSubcategory         string   `json:"error_subcategory"`
	ClientErrorSubcategory   *string  `json:"client_error_subcategory"`
	ClassificationConfidence string   `json:"classification_confidence"`
	ClassificationReason     string   `json:"classification_reason"`
	MissingEvidence          []string `json:"missing_evidence,omitempty"`
	StatusCode               int      `json:"status_code"`
	ClientStatusCode         int      `json:"client_status_code"`
	ErrorSource              string   `json:"error_source"`
	ErrorOwner               string   `json:"error_owner"`
}

type OpsUnifiedErrorImpactScope struct {
	SameKindCount            int `json:"same_kind_count"`
	AffectedUsers            int `json:"affected_users"`
	AffectedAPIKeys          int `json:"affected_api_keys"`
	AffectedGroups           int `json:"affected_groups"`
	AffectedModels           int `json:"affected_models"`
	AffectedUpstreamAccounts int `json:"affected_upstream_accounts"`
}

type OpsUnifiedErrorRecovery struct {
	FinalFailed    bool       `json:"final_failed"`
	Recovered      bool       `json:"recovered"`
	RecoveryMethod string     `json:"recovery_method"`
	Resolved       bool       `json:"resolved"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

type OpsUnifiedErrorAIAnalysis struct {
	Status  string `json:"status"`
	TaskID  *int64 `json:"task_id,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type OpsUnifiedErrorRawRecord struct {
	ErrorLog         *OpsErrorLogDetail `json:"error_log"`
	ErrorBodyPreview string             `json:"error_body_preview"`
	UpstreamErrors   string             `json:"upstream_errors,omitempty"`
}
