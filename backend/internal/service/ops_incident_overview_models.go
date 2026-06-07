package service

import "time"

const (
	OpsIncidentStatusNormal    = "normal"
	OpsIncidentStatusObserving = "observing"
	OpsIncidentStatusRisk      = "risk"
	OpsIncidentStatusIncident  = "incident"

	OpsIncidentScoreLevelNormal    = "normal"
	OpsIncidentScoreLevelObserving = "observing"
	OpsIncidentScoreLevelRisk      = "risk"
	OpsIncidentScoreLevelIncident  = "incident"
)

type OpsIncidentAffectedAccount struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type OpsIncidentImpact struct {
	AffectedUsers    int64                         `json:"affected_users"`
	AffectedAPIKeys  int64                         `json:"affected_api_keys"`
	AffectedModels   []string                      `json:"affected_models"`
	AffectedAccounts []*OpsIncidentAffectedAccount `json:"affected_accounts"`
}

type OpsIncidentQuickFilter struct {
	Label  string            `json:"label"`
	Params map[string]string `json:"params"`
}

type OpsIncidentLatestAIAnalysis struct {
	ID        int64     `json:"id"`
	Status    string    `json:"status"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}

type OpsIncidentOverview struct {
	Status          string   `json:"status"`
	HealthRiskScore int      `json:"health_risk_score"`
	ScoreLevel      string   `json:"score_level"`
	ScoreReasons    []string `json:"score_reasons"`
	Summary         string   `json:"summary"`

	FinalFailures         int64   `json:"final_failures"`
	FinalFailureRate      float64 `json:"final_failure_rate"`
	RecoveredFluctuations int64   `json:"recovered_fluctuations"`
	TotalRequests         int64   `json:"total_requests"`

	AffectedUsers    int64                         `json:"affected_users"`
	AffectedAPIKeys  int64                         `json:"affected_api_keys"`
	AffectedModels   []string                      `json:"affected_models"`
	AffectedAccounts []*OpsIncidentAffectedAccount `json:"affected_accounts"`

	LatestAIAnalysis   *OpsIncidentLatestAIAnalysis `json:"latest_ai_analysis"`
	QuickFilters       []*OpsIncidentQuickFilter    `json:"quick_filters"`
	RecommendedActions []string                     `json:"recommended_actions"`
	UpdatedAt          time.Time                    `json:"updated_at"`
}
