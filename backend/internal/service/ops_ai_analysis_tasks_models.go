package service

import "time"

const (
	OpsAIAnalysisSourceAlertEvent    = "alert_event"
	OpsAIAnalysisSourceUnifiedErrors = "unified_errors"
	OpsAIAnalysisSourceManualFilter  = "manual_filter"

	OpsAIAnalysisTriggerAuto   = "auto"
	OpsAIAnalysisTriggerManual = "manual"

	OpsAIAnalysisStatusPending   = "pending"
	OpsAIAnalysisStatusRunning   = "running"
	OpsAIAnalysisStatusCompleted = "completed"
	OpsAIAnalysisStatusFailed    = "failed"
	OpsAIAnalysisStatusExpired   = "expired"
)

type OpsAIAnalysisTask struct {
	ID            int64      `json:"id"`
	SourceType    string     `json:"source_type"`
	SourceID      *int64     `json:"source_id,omitempty"`
	TriggerType   string     `json:"trigger_type"`
	TriggerUserID *int64     `json:"trigger_user_id,omitempty"`
	TimeStart     time.Time  `json:"time_start"`
	TimeEnd       time.Time  `json:"time_end"`
	FiltersJSON   string     `json:"-"`
	Status        string     `json:"status"`
	SampleCount   int        `json:"sample_count"`
	Provider      string     `json:"provider,omitempty"`
	Model         string     `json:"model,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type OpsAIAnalysisTaskCreateInput struct {
	SourceType    string
	SourceID      *int64
	TriggerType   string
	TriggerUserID *int64
	TimeStart     time.Time
	TimeEnd       time.Time
	FiltersJSON   string
	Status        string
	SampleCount   int
	Provider      string
	Model         string
}

type OpsAIAnalysisTaskCreateRequest struct {
	SourceType string         `json:"source_type"`
	SourceID   *int64         `json:"source_id"`
	TimeStart  string         `json:"time_start"`
	TimeEnd    string         `json:"time_end"`
	Filters    map[string]any `json:"filters"`
}

type OpsAIAnalysisTaskCreateResult string

const (
	OpsAIAnalysisTaskCreateResultCreated   OpsAIAnalysisTaskCreateResult = "created"
	OpsAIAnalysisTaskCreateResultDuplicate OpsAIAnalysisTaskCreateResult = "duplicate"
	OpsAIAnalysisTaskCreateResultQueueBusy OpsAIAnalysisTaskCreateResult = "queue_busy"
)

type OpsAIAnalysisTaskCreateResponse struct {
	TaskID            int64  `json:"task_id"`
	Status            string `json:"status"`
	SampleCount       int    `json:"sample_count"`
	MatchedErrorCount int    `json:"matched_error_count"`
	Message           string `json:"message"`
}

type OpsAIAnalysisTaskUpdate struct {
	Status       string
	SampleCount  *int
	ErrorMessage *string
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

type OpsAIAnalysisContext struct {
	Task    *OpsAIAnalysisTask     `json:"task"`
	Samples []*OpsAIAnalysisSample `json:"samples"`
	Total   int                    `json:"total"`
}

type OpsAIAnalysisSample struct {
	ID                     int64     `json:"id"`
	OccurredAt             time.Time `json:"occurred_at"`
	ErrorCategory          string    `json:"error_category"`
	ErrorSubcategory       string    `json:"error_subcategory"`
	ClientErrorSubcategory *string   `json:"client_error_subcategory,omitempty"`
	ErrorResult            string    `json:"error_result"`
	Severity               string    `json:"severity"`
	StatusCode             int       `json:"status_code"`
	Platform               string    `json:"platform"`
	Model                  string    `json:"model"`
	GroupID                *int64    `json:"group_id,omitempty"`
	APIKeyID               *int64    `json:"api_key_id,omitempty"`
	UserID                 *int64    `json:"user_id,omitempty"`
	UpstreamAccountID      *int64    `json:"upstream_account_id,omitempty"`
	Summary                string    `json:"summary"`
	SameKindCount          int       `json:"same_kind_count"`
}
