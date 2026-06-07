package service

import "time"

// Ops alert rule/event models.
//
// NOTE: These are admin-facing DTOs and intentionally keep JSON naming aligned
// with the existing ops dashboard frontend (backup style).

const (
	OpsAlertStatusFiring         = "firing"
	OpsAlertStatusAcknowledged   = "acknowledged"
	OpsAlertStatusProcessing     = "processing"
	OpsAlertStatusRecovered      = "recovered"
	OpsAlertStatusClosed         = "closed"
	OpsAlertStatusSilenced       = "silenced"
	OpsAlertStatusResolved       = "resolved"
	OpsAlertStatusManualResolved = "manual_resolved"
)

type OpsAlertRule struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	Enabled  bool   `json:"enabled"`
	Severity string `json:"severity"`

	MetricType string  `json:"metric_type"`
	Operator   string  `json:"operator"`
	Threshold  float64 `json:"threshold"`

	WindowMinutes    int `json:"window_minutes"`
	SustainedMinutes int `json:"sustained_minutes"`
	CooldownMinutes  int `json:"cooldown_minutes"`

	NotifyEmail bool `json:"notify_email"`

	Filters map[string]any `json:"filters,omitempty"`

	// v2 compound alert rule fields. They coexist with legacy metric/operator/threshold
	// fields until the evaluator migration is complete.
	RuleVersion                string         `json:"rule_version"`
	ErrorCategories            []string       `json:"error_categories"`
	TriggerLevel               string         `json:"trigger_level"`
	MinFinalFailures           int            `json:"min_final_failures"`
	MinFailureRate             float64        `json:"min_failure_rate"`
	MinSampleCount             int            `json:"min_sample_count"`
	ImpactScope                map[string]int `json:"impact_scope"`
	RecoveredFluctuationPolicy string         `json:"recovered_fluctuation_policy"`
	MinRecoveredFluctuations   int            `json:"min_recovered_fluctuations"`
	AutoAIAnalysis             bool           `json:"auto_ai_analysis"`
	NotificationChannels       []string       `json:"notification_channels"`
	SilenceMinutes             int            `json:"silence_minutes"`
	MigrationState             string         `json:"migration_state"`

	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type OpsAlertEvent struct {
	ID       int64  `json:"id"`
	RuleID   int64  `json:"rule_id"`
	Severity string `json:"severity"`
	Status   string `json:"status"`

	EventKey        string    `json:"event_key,omitempty"`
	LifecycleStatus string    `json:"lifecycle_status"`
	MergedCount     int       `json:"merged_count"`
	LastSeenAt      time.Time `json:"last_seen_at"`

	Title       string `json:"title"`
	Description string `json:"description"`

	MetricValue    *float64 `json:"metric_value,omitempty"`
	ThresholdValue *float64 `json:"threshold_value,omitempty"`

	Dimensions      map[string]any `json:"dimensions,omitempty"`
	TriggerSnapshot map[string]any `json:"trigger_snapshot,omitempty"`
	ScoreSnapshot   map[string]any `json:"score_snapshot,omitempty"`

	FiredAt     time.Time  `json:"fired_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	RecoveredAt *time.Time `json:"recovered_at,omitempty"`

	AcknowledgedAt   *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy   *int64     `json:"acknowledged_by,omitempty"`
	AcknowledgedNote string     `json:"acknowledged_note,omitempty"`

	ProcessingAt     *time.Time `json:"processing_at,omitempty"`
	ProcessingBy     *int64     `json:"processing_by,omitempty"`
	ProcessingNote   string     `json:"processing_note,omitempty"`
	ProcessingAction string     `json:"processing_action,omitempty"`

	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	ClosedBy     *int64     `json:"closed_by,omitempty"`
	ClosedReason string     `json:"closed_reason,omitempty"`
	AITaskID     *int64     `json:"ai_task_id,omitempty"`

	EmailSent        bool       `json:"email_sent"`
	CreatedAt        time.Time  `json:"created_at"`
	MergeWindowStart *time.Time `json:"-"`
}

type OpsAlertSilence struct {
	ID int64 `json:"id"`

	RuleID   int64   `json:"rule_id"`
	Platform string  `json:"platform"`
	GroupID  *int64  `json:"group_id,omitempty"`
	Region   *string `json:"region,omitempty"`

	Until  time.Time `json:"until"`
	Reason string    `json:"reason"`

	CreatedBy *int64    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type OpsAlertEventFilter struct {
	Limit int

	// Cursor pagination (descending by fired_at, then id).
	BeforeFiredAt *time.Time
	BeforeID      *int64

	// Optional filters.
	Status    string
	Severity  string
	EmailSent *bool

	StartTime *time.Time
	EndTime   *time.Time

	// Dimensions filters (best-effort).
	Platform string
	Model    string
	GroupID  *int64
}
