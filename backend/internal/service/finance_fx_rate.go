package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type FinanceFXRateVersion struct {
	ID             int64      `json:"id"`
	Currency       string     `json:"currency"`
	RateToUSD      string     `json:"rate_to_usd"`
	Source         string     `json:"source"`
	ObservedAt     time.Time  `json:"observed_at"`
	EffectiveFrom  time.Time  `json:"effective_from"`
	EffectiveTo    *time.Time `json:"effective_to,omitempty"`
	Checksum       string     `json:"checksum"`
	OperatorID     *int64     `json:"operator_id,omitempty"`
	ChangeReason   string     `json:"change_reason,omitempty"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type FinanceFXRateCreateInput struct {
	Currency       string    `json:"currency"`
	RateToUSD      string    `json:"rate_to_usd"`
	Source         string    `json:"source"`
	ObservedAt     time.Time `json:"observed_at"`
	EffectiveFrom  time.Time `json:"effective_from"`
	ChangeReason   string    `json:"change_reason"`
	OperatorID     int64     `json:"-"`
	IdempotencyKey string    `json:"-"`
}

type FinanceFXRateRepository interface {
	ListFinanceFXRates(ctx context.Context, currency string, page, pageSize int) ([]FinanceFXRateVersion, int64, error)
	CreateFinanceFXRate(ctx context.Context, input FinanceFXRateCreateInput, rate decimal.Decimal, checksum string) (*FinanceFXRateVersion, error)
}

type FinanceFXRateService struct{ repo FinanceFXRateRepository }

func NewFinanceFXRateService(repo FinanceFXRateRepository) *FinanceFXRateService {
	return &FinanceFXRateService{repo: repo}
}

func (s *FinanceFXRateService) List(ctx context.Context, currency string, page, pageSize int) ([]FinanceFXRateVersion, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, financeValidationError("fx rate service is unavailable")
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency != "" && !validFinanceCurrency(currency) {
		return nil, 0, financeValidationError("currency must be a three-letter code")
	}
	normalizeFinancePage(&page, &pageSize)
	return s.repo.ListFinanceFXRates(ctx, currency, page, pageSize)
}

func (s *FinanceFXRateService) Create(ctx context.Context, input FinanceFXRateCreateInput) (*FinanceFXRateVersion, error) {
	if s == nil || s.repo == nil {
		return nil, financeValidationError("fx rate service is unavailable")
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if !validFinanceCurrency(input.Currency) {
		return nil, financeValidationError("currency must be a three-letter code")
	}
	rate, err := decimal.NewFromString(strings.TrimSpace(input.RateToUSD))
	if err != nil || !rate.IsPositive() {
		return nil, financeValidationError("rate_to_usd must be a positive decimal string")
	}
	if input.Currency == "USD" && !rate.Equal(decimal.NewFromInt(1)) {
		return nil, financeValidationError("USD rate_to_usd must equal 1")
	}
	input.Source = strings.TrimSpace(input.Source)
	if input.Source == "" {
		input.Source = "manual_admin"
	}
	input.ChangeReason = strings.TrimSpace(input.ChangeReason)
	if len([]rune(input.ChangeReason)) < 5 || len([]rune(input.ChangeReason)) > 500 {
		return nil, financeValidationError("change_reason must be between 5 and 500 characters")
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if len(input.IdempotencyKey) > 200 {
		return nil, financeValidationError("idempotency_key is too long")
	}
	if len(input.Source) > 80 {
		return nil, financeValidationError("source is too long")
	}
	if input.ObservedAt.IsZero() {
		input.ObservedAt = time.Now().UTC()
	} else {
		input.ObservedAt = input.ObservedAt.UTC()
	}
	if input.EffectiveFrom.IsZero() {
		input.EffectiveFrom = input.ObservedAt
	} else {
		input.EffectiveFrom = input.EffectiveFrom.UTC()
	}
	input.RateToUSD = rate.String()
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(input.Currency+"|"+rate.String()+"|"+input.Source+"|"+input.ObservedAt.Format(time.RFC3339Nano)+"|"+input.EffectiveFrom.Format(time.RFC3339Nano)+"|"+input.ChangeReason)))
	return s.repo.CreateFinanceFXRate(ctx, input, rate, checksum)
}
