package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

var financeAlertTypes = []string{
	"negative_profit",
	"missing_price",
	"missing_multiplier",
	"missing_usage",
	"wallet_low_balance",
	"wallet_sync_failed",
	"pricing_sync_failed",
	"reconciliation_difference",
	"payment_fee_uncollected",
}

var ErrFinanceAlertNotFound = errors.New("finance alert not found")

type FinanceAlertSignal struct {
	AlertType      string
	Severity       string
	AggregationKey string
	Title          string
	Description    string
	DimensionType  string
	DimensionID    *int64
	ImpactAmount   *decimal.Decimal
	RequestCount   int64
	OccurredAt     time.Time
}

type FinanceAlert struct {
	ID              int64      `json:"id"`
	AlertType       string     `json:"alert_type"`
	Severity        string     `json:"severity"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	DimensionType   string     `json:"dimension_type,omitempty"`
	DimensionID     *int64     `json:"dimension_id"`
	ImpactAmount    *string    `json:"impact_amount"`
	RequestCount    int64      `json:"request_count"`
	OccurrenceCount int64      `json:"occurrence_count"`
	Status          string     `json:"status"`
	FirstOccurredAt time.Time  `json:"first_occurred_at"`
	LastOccurredAt  time.Time  `json:"last_occurred_at"`
	AssigneeID      *int64     `json:"assignee_id"`
	HandledBy       *int64     `json:"handled_by"`
	HandledNote     string     `json:"handled_note,omitempty"`
	HandledAt       *time.Time `json:"handled_at"`
}

type FinanceAlertListRequest struct {
	AlertType string
	Severity  string
	Status    string
	Page      int
	PageSize  int
}

type FinanceAlertStatusUpdate struct {
	Status string `json:"status" binding:"required"`
	Note   string `json:"note" binding:"required"`
}

type FinanceAlertRepository interface {
	CollectFinanceAlertSignals(ctx context.Context, now time.Time) ([]FinanceAlertSignal, error)
	UpsertFinanceAlertSignals(ctx context.Context, signals []FinanceAlertSignal) error
	ListFinanceAlerts(ctx context.Context, filter FinanceReportFilter, request FinanceAlertListRequest) ([]FinanceAlert, int64, error)
	UpdateFinanceAlertStatus(ctx context.Context, id int64, status, note string, actorID int64, now time.Time) (*FinanceAlert, error)
}

type FinanceAlertService struct {
	repo FinanceAlertRepository
	now  func() time.Time
}

func NewFinanceAlertService(repo FinanceAlertRepository) *FinanceAlertService {
	return &FinanceAlertService{repo: repo, now: time.Now}
}

func (s *FinanceAlertService) Scan(ctx context.Context) (int, error) {
	signals, err := s.repo.CollectFinanceAlertSignals(ctx, s.now().UTC())
	if err != nil {
		return 0, err
	}
	if err = s.repo.UpsertFinanceAlertSignals(ctx, signals); err != nil {
		return 0, err
	}
	return len(signals), nil
}

func (s *FinanceAlertService) List(ctx context.Context, filter FinanceReportFilter, request FinanceAlertListRequest) ([]FinanceAlert, int64, error) {
	request.AlertType = strings.TrimSpace(request.AlertType)
	request.Severity = strings.TrimSpace(request.Severity)
	request.Status = strings.TrimSpace(request.Status)
	if request.AlertType != "" && !financeAllowed(request.AlertType, financeAlertTypes...) {
		return nil, 0, financeValidationError("alert_type is invalid")
	}
	if request.Severity != "" && !financeAllowed(request.Severity, "info", "warning", "critical") {
		return nil, 0, financeValidationError("severity is invalid")
	}
	if request.Status != "" && !financeAllowed(request.Status, "open", "acknowledged", "resolved", "ignored") {
		return nil, 0, financeValidationError("status is invalid")
	}
	normalizeFinancePage(&request.Page, &request.PageSize)
	return s.repo.ListFinanceAlerts(ctx, filter, request)
}

func (s *FinanceAlertService) UpdateStatus(ctx context.Context, id int64, request FinanceAlertStatusUpdate, actorID int64) (*FinanceAlert, error) {
	if id <= 0 {
		return nil, financeValidationError("alert id is invalid")
	}
	request.Status = strings.TrimSpace(request.Status)
	request.Note = strings.TrimSpace(request.Note)
	if !financeAllowed(request.Status, "open", "acknowledged", "resolved", "ignored") {
		return nil, financeValidationError("status is invalid")
	}
	if request.Note == "" {
		return nil, financeValidationError("note is required")
	}
	if len(request.Note) > 2000 {
		return nil, financeValidationError("note is too long")
	}
	return s.repo.UpdateFinanceAlertStatus(ctx, id, request.Status, request.Note, actorID, s.now().UTC())
}
