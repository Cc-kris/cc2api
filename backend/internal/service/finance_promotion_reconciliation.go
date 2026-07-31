package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

const (
	PromotionCreditReconciliationRequired = "requires_reconciliation"
	PromotionCreditReconciliationResolved = "resolved"
)

var (
	ErrPromotionCreditReconciliationNotFound = errors.New("promotion credit reconciliation not found")
	ErrPromotionCreditReconciliationResolved = errors.New("promotion credit reconciliation already resolved")
)

type PromotionCreditReconciliation struct {
	UserID                   int64      `json:"user_id"`
	UserEmail                string     `json:"user_email"`
	Username                 string     `json:"username"`
	DetectedHistoricalBonus  string     `json:"detected_historical_bonus"`
	CurrentRemainingAmount   string     `json:"current_remaining_amount"`
	ConfirmedRemainingAmount *string    `json:"confirmed_remaining_amount"`
	Status                   string     `json:"status"`
	CutoverAt                time.Time  `json:"cutover_at"`
	CreatedAt                time.Time  `json:"created_at"`
	ResolvedAt               *time.Time `json:"resolved_at"`
	ResolvedBy               *int64     `json:"resolved_by"`
	Notes                    string     `json:"notes,omitempty"`
}

type PromotionCreditReconciliationListRequest struct {
	Status   string
	Page     int
	PageSize int
}

type ResolvePromotionCreditReconciliationRequest struct {
	ConfirmedRemainingAmount string `json:"confirmed_remaining_amount" binding:"required"`
	Note                     string `json:"note" binding:"required"`
}

type PromotionCreditReconciliationRepository interface {
	ListPromotionCreditReconciliations(context.Context, PromotionCreditReconciliationListRequest) ([]PromotionCreditReconciliation, int64, error)
	ResolvePromotionCreditReconciliation(context.Context, int64, decimal.Decimal, string, int64, time.Time) (*PromotionCreditReconciliation, error)
}

type PromotionCreditReconciliationService struct {
	repo PromotionCreditReconciliationRepository
	now  func() time.Time
}

func NewPromotionCreditReconciliationService(repo PromotionCreditReconciliationRepository) *PromotionCreditReconciliationService {
	return &PromotionCreditReconciliationService{repo: repo, now: time.Now}
}

func (s *PromotionCreditReconciliationService) List(ctx context.Context, request PromotionCreditReconciliationListRequest) ([]PromotionCreditReconciliation, int64, error) {
	request.Status = strings.TrimSpace(request.Status)
	if request.Status != "" && request.Status != PromotionCreditReconciliationRequired && request.Status != PromotionCreditReconciliationResolved {
		return nil, 0, financeValidationError("status is invalid")
	}
	if request.Page <= 0 {
		request.Page = 1
	}
	if request.PageSize <= 0 || request.PageSize > 100 {
		request.PageSize = 20
	}
	return s.repo.ListPromotionCreditReconciliations(ctx, request)
}

func (s *PromotionCreditReconciliationService) Resolve(ctx context.Context, userID int64, request ResolvePromotionCreditReconciliationRequest, operatorID int64) (*PromotionCreditReconciliation, error) {
	if userID <= 0 || operatorID <= 0 {
		return nil, financeValidationError("user_id or operator_id is invalid")
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(request.ConfirmedRemainingAmount))
	if err != nil || amount.IsNegative() || amount.Exponent() < -10 {
		return nil, financeValidationError("confirmed_remaining_amount must be a non-negative decimal with at most 10 decimal places")
	}
	request.Note = strings.TrimSpace(request.Note)
	if request.Note == "" || utf8.RuneCountInString(request.Note) > 2000 {
		return nil, financeValidationError("note is required and must not exceed 2000 characters")
	}
	return s.repo.ResolvePromotionCreditReconciliation(ctx, userID, amount, request.Note, operatorID, s.now().UTC())
}
