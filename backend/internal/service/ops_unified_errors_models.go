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
