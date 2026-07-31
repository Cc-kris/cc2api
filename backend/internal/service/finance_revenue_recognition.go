package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/shopspring/decimal"
)

type FinanceSubscriptionOrder struct {
	OrderID          int64
	UserID           int64
	GroupID          *int64
	Amount           decimal.Decimal
	RefundAmount     decimal.Decimal
	ServiceStartDate time.Time
	ServiceDays      int
	RefundDate       *time.Time
}

type FinanceSubscriptionUsage struct {
	UsageLogID     int64
	UsageListValue decimal.Decimal
}

type FinanceRevenueAllocation struct {
	UsageLogID int64
	Amount     decimal.Decimal
	Method     string
}

type FinanceRevenueRecognition struct {
	OrderID            int64
	UserID             int64
	GroupID            *int64
	RecognitionDate    time.Time
	RecognizedRevenue  decimal.Decimal
	RefundReduction    decimal.Decimal
	AllocatedRevenue   decimal.Decimal
	UnallocatedRevenue decimal.Decimal
	AllocationStatus   string
	CalculationDetail  map[string]any
}

type FinanceRevenueRecognitionRepository interface {
	AcquireSubscriptionRevenueDateLock(ctx context.Context, date time.Time, timezone string) (func() error, error)
	OldestUnrecognizedSubscriptionDate(ctx context.Context, through time.Time, timezone string) (*time.Time, error)
	ListSubscriptionOrdersForDate(ctx context.Context, date time.Time, timezone string) ([]FinanceSubscriptionOrder, error)
	ListSubscriptionUsageForDate(ctx context.Context, order FinanceSubscriptionOrder, date time.Time, timezone string) ([]FinanceSubscriptionUsage, error)
	SaveSubscriptionRecognition(ctx context.Context, recognition FinanceRevenueRecognition, allocations []FinanceRevenueAllocation) error
}

func (s *FinanceRevenueRecognitionService) BackfillUnrecognized(ctx context.Context, through time.Time, timezone string, maxDays int) (int, error) {
	if s == nil || s.repo == nil {
		return 0, errors.New("finance revenue recognition repository is unavailable")
	}
	if maxDays <= 0 {
		maxDays = 366
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return 0, financeValidationError("timezone is invalid")
	}
	through = financeRecognitionDate(through, location)
	start, err := s.repo.OldestUnrecognizedSubscriptionDate(ctx, through, timezone)
	if err != nil || start == nil {
		return 0, err
	}
	return s.RecognizeRange(ctx, *start, through, timezone, maxDays)
}

func (s *FinanceRevenueRecognitionService) RecognizeRange(ctx context.Context, start, end time.Time, timezone string, maxDays int) (int, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return 0, financeValidationError("timezone is invalid")
	}
	start = financeRecognitionDate(start, location)
	end = financeRecognitionDate(end, location)
	if end.Before(start) {
		return 0, financeValidationError("recognition end date must not be earlier than start date")
	}
	if maxDays <= 0 {
		maxDays = 366
	}
	days := 0
	for date := start; !date.After(end) && days < maxDays; date = date.AddDate(0, 0, 1) {
		if _, err = s.RecognizeDate(ctx, date, timezone); err != nil {
			return days, err
		}
		days++
	}
	return days, nil
}

type FinanceRevenueRecognitionService struct {
	repo     FinanceRevenueRecognitionRepository
	timezone string
	now      func() time.Time
}

func NewFinanceRevenueRecognitionService(repo FinanceRevenueRecognitionRepository, cfg *config.Config) *FinanceRevenueRecognitionService {
	timezone := "Asia/Shanghai"
	if cfg != nil && strings.TrimSpace(cfg.Timezone) != "" {
		timezone = strings.TrimSpace(cfg.Timezone)
	}
	return &FinanceRevenueRecognitionService{repo: repo, timezone: timezone, now: time.Now}
}

func (s *FinanceRevenueRecognitionService) Timezone() string {
	if s == nil || strings.TrimSpace(s.timezone) == "" {
		return "Asia/Shanghai"
	}
	return s.timezone
}

func (s *FinanceRevenueRecognitionService) RecognizeDate(ctx context.Context, date time.Time, timezone string) (int, error) {
	if s == nil || s.repo == nil {
		return 0, errors.New("finance revenue recognition repository is unavailable")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return 0, financeValidationError("timezone is invalid")
	}
	date = financeRecognitionDate(date, location)
	release, err := s.repo.AcquireSubscriptionRevenueDateLock(ctx, date, timezone)
	if err != nil {
		return 0, err
	}
	defer func() { _ = release() }()
	orders, err := s.repo.ListSubscriptionOrdersForDate(ctx, date, timezone)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, order := range orders {
		recognized, refundReduction, ok := calculateSubscriptionDailyRevenue(order, date, location)
		if !ok {
			continue
		}
		usages, listErr := s.repo.ListSubscriptionUsageForDate(ctx, order, date, timezone)
		if listErr != nil {
			return processed, listErr
		}
		allocations := allocateSubscriptionRevenue(recognized, usages)
		allocated := decimal.Zero
		for _, allocation := range allocations {
			allocated = allocated.Add(allocation.Amount)
		}
		unallocated := recognized.Sub(allocated)
		status := "allocated"
		if len(allocations) == 0 && !recognized.IsZero() {
			status = "unallocated"
		} else if !unallocated.IsZero() {
			status = "partial"
		}
		recognition := FinanceRevenueRecognition{
			OrderID: order.OrderID, UserID: order.UserID, GroupID: cloneInt64Pointer(order.GroupID), RecognitionDate: date,
			RecognizedRevenue: recognized, RefundReduction: refundReduction, AllocatedRevenue: allocated,
			UnallocatedRevenue: unallocated, AllocationStatus: status,
			CalculationDetail: map[string]any{
				"policy": "straight_line_daily_v1", "service_days": order.ServiceDays,
				"order_amount": order.Amount.String(), "refund_amount": order.RefundAmount.String(),
				"allocation_facts": financeRevenueAllocationFacts(allocations),
			},
		}
		if saveErr := s.repo.SaveSubscriptionRecognition(ctx, recognition, allocations); saveErr != nil {
			return processed, saveErr
		}
		processed++
	}
	return processed, nil
}

func financeRevenueAllocationFacts(allocations []FinanceRevenueAllocation) []map[string]any {
	facts := make([]map[string]any, 0, len(allocations))
	for _, allocation := range allocations {
		facts = append(facts, map[string]any{
			"usage_log_id": allocation.UsageLogID,
			"amount":       allocation.Amount.String(),
			"method":       allocation.Method,
		})
	}
	return facts
}

func financeRecognitionDate(value time.Time, location *time.Location) time.Time {
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

func financeCivilDayIndex(start, end time.Time, location *time.Location) int {
	start = financeRecognitionDate(start, location)
	end = financeRecognitionDate(end, location)
	startUTC := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endUTC := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	return int(endUTC.Sub(startUTC) / (24 * time.Hour))
}

func calculateSubscriptionDailyRevenue(order FinanceSubscriptionOrder, date time.Time, location *time.Location) (decimal.Decimal, decimal.Decimal, bool) {
	if order.ServiceDays <= 0 || order.Amount.IsNegative() {
		return decimal.Zero, decimal.Zero, false
	}
	start := financeRecognitionDate(order.ServiceStartDate, location)
	date = financeRecognitionDate(date, location)
	dayIndex := financeCivilDayIndex(start, date, location)
	refundDayIndex := -1
	if order.RefundDate != nil {
		refundDate := financeRecognitionDate(*order.RefundDate, location)
		refundDayIndex = financeCivilDayIndex(start, refundDate, location)
		if refundDayIndex >= order.ServiceDays && date.Equal(refundDate) {
			refund := financeClampedRefund(order)
			return refund.Neg(), refund, true
		}
	}
	if dayIndex < 0 || dayIndex >= order.ServiceDays {
		return decimal.Zero, decimal.Zero, false
	}

	baseDaily := order.Amount.Div(decimal.NewFromInt(int64(order.ServiceDays)))
	recognized := baseDaily
	refundReduction := decimal.Zero
	refund := financeClampedRefund(order)
	if refundDayIndex >= 0 && dayIndex >= refundDayIndex && refund.IsPositive() {
		netDaily := order.Amount.Sub(refund).Div(decimal.NewFromInt(int64(order.ServiceDays)))
		recognized = netDaily
		refundReduction = baseDaily.Sub(netDaily)
		if dayIndex == refundDayIndex {
			pastReduction := refund.Mul(decimal.NewFromInt(int64(max(refundDayIndex, 0)))).Div(decimal.NewFromInt(int64(order.ServiceDays)))
			recognized = recognized.Sub(pastReduction)
			refundReduction = refundReduction.Add(pastReduction)
		}
	}
	return recognized, refundReduction, true
}

func financeClampedRefund(order FinanceSubscriptionOrder) decimal.Decimal {
	refund := order.RefundAmount
	if refund.IsNegative() {
		refund = decimal.Zero
	}
	if refund.GreaterThan(order.Amount) {
		refund = order.Amount
	}
	return refund
}

func allocateSubscriptionRevenue(amount decimal.Decimal, usages []FinanceSubscriptionUsage) []FinanceRevenueAllocation {
	if len(usages) == 0 || amount.IsZero() {
		return nil
	}
	totalWeight := decimal.Zero
	for _, usage := range usages {
		if usage.UsageListValue.IsPositive() {
			totalWeight = totalWeight.Add(usage.UsageListValue)
		}
	}
	method := "usage_list_value"
	lastWeightedIndex := -1
	if totalWeight.IsZero() {
		totalWeight = decimal.NewFromInt(int64(len(usages)))
		method = "request_count"
		lastWeightedIndex = len(usages) - 1
	} else {
		for index := len(usages) - 1; index >= 0; index-- {
			if usages[index].UsageListValue.IsPositive() {
				lastWeightedIndex = index
				break
			}
		}
	}
	allocations := make([]FinanceRevenueAllocation, 0, len(usages))
	allocated := decimal.Zero
	for index, usage := range usages {
		weight := usage.UsageListValue
		if method == "request_count" {
			weight = decimal.NewFromInt(1)
		} else if !weight.IsPositive() {
			weight = decimal.Zero
		}
		allocation := decimal.Zero
		if index == lastWeightedIndex {
			allocation = amount.Sub(allocated)
		} else if weight.IsPositive() {
			allocation = amount.Mul(weight).Div(totalWeight)
		}
		allocations = append(allocations, FinanceRevenueAllocation{UsageLogID: usage.UsageLogID, Amount: allocation, Method: method})
		allocated = allocated.Add(allocation)
	}
	return allocations
}
