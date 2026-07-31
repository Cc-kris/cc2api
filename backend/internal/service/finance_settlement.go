package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	FinanceSettlementPending     = "pending"
	FinanceSettlementSettled     = "settled"
	FinanceSettlementNeedsReview = "needs_review"
	FinanceSettlementFailed      = "failed"
)

var (
	ErrFinanceSettlementInvalid    = errors.New("finance settlement input is invalid")
	ErrFinanceSettlementZeroWeight = errors.New("finance settlement has no positive standard-cost weight")
)

type FinanceSettlementError struct {
	Code    string
	Message string
}

func (e *FinanceSettlementError) Error() string { return e.Message }

func IsFinanceSettlementError(err error, code string) bool {
	var target *FinanceSettlementError
	return errors.As(err, &target) && target.Code == code
}

// FinanceSettlementSegment is the immutable request-attempt identity and its
// standard list-cost weight. Recharge facts are intentionally absent: they can
// affect bonus income, but never request-cost allocation.
type FinanceSettlementSegment struct {
	UsageLogID     int64
	AttemptNo      int
	UsageCreatedAt time.Time
	StandardCost   decimal.Decimal // USD standard list-cost weight.
}

type FinanceSettlementAllocation struct {
	UsageLogID     int64
	AttemptNo      int
	StandardCost   decimal.Decimal // USD standard list-cost weight.
	AllocationRate decimal.Decimal
	AllocatedCost  decimal.Decimal // USD upstream cost written to the finance ledger.
}

type FinanceSettlementAllocationResult struct {
	StandardCostTotal decimal.Decimal
	ActualCostTotal   decimal.Decimal
	AllocatedTotal    decimal.Decimal
	Difference        decimal.Decimal
	Allocations       []FinanceSettlementAllocation
}

type FinanceSettlementInterval struct {
	ID                      int64            `json:"id"`
	OwnerType               string           `json:"owner_type"`
	OwnerID                 int64            `json:"owner_id"`
	AccountID               *int64           `json:"account_id,omitempty"`
	AccountFinanceProfileID *int64           `json:"account_finance_profile_id,omitempty"`
	WalletID                *int64           `json:"wallet_id,omitempty"`
	ScopeKey                string           `json:"scope_key"`
	PreviousSnapshotID      int64            `json:"previous_snapshot_id"`
	CurrentSnapshotID       int64            `json:"current_snapshot_id"`
	PeriodStart             time.Time        `json:"period_start"`
	PeriodEnd               time.Time        `json:"period_end"`
	UnitSemantics           string           `json:"unit_semantics"`
	Currency                *string          `json:"currency,omitempty"`
	FXRateVersionID         *int64           `json:"fx_rate_version_id,omitempty"`
	FXRateToUSD             *decimal.Decimal `json:"fx_rate_to_usd,omitempty"`
	FXSource                string           `json:"fx_source,omitempty"`
	FXObservedAt            *time.Time       `json:"fx_observed_at,omitempty"`
	ListCostDelta           *decimal.Decimal `json:"list_cost_delta,omitempty"` // Original upstream currency, identified by Currency.
	ActualCostDelta         decimal.Decimal  `json:"actual_cost_delta"`         // Original upstream currency, identified by Currency.
	ObservedMultiplier      *decimal.Decimal `json:"observed_multiplier,omitempty"`
	Status                  string           `json:"status"`
	CurrentRevision         int              `json:"current_revision"`
	RequestCount            int64            `json:"request_count"`
	SegmentCount            int64            `json:"segment_count"`
	StandardCostTotal       *decimal.Decimal `json:"standard_cost_total,omitempty"`  // USD.
	AllocatedCostTotal      *decimal.Decimal `json:"allocated_cost_total,omitempty"` // USD.
	DifferenceAmount        *decimal.Decimal `json:"difference_amount,omitempty"`
	ErrorSummary            string           `json:"error_summary,omitempty"`
	SettledAt               *time.Time       `json:"settled_at,omitempty"`
}

type FinanceSettlementListFilter struct {
	Status    string
	AccountID *int64
	Page      int
	PageSize  int
}

type FinanceSettlementAllocationView struct {
	ID                 int64           `json:"id"`
	SettlementInterval int64           `json:"settlement_interval_id"`
	UsageLogID         int64           `json:"usage_log_id"`
	RequestID          string          `json:"request_id"`
	AttemptNo          int             `json:"attempt_no"`
	Revision           int             `json:"revision"`
	StandardCostWeight decimal.Decimal `json:"standard_cost_weight"`
	AllocationRate     decimal.Decimal `json:"allocation_rate"`
	AllocatedCost      decimal.Decimal `json:"allocated_cost"`
	InvalidatedAt      *time.Time      `json:"invalidated_at,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}

type FinanceSettlementDetail struct {
	Interval    *FinanceSettlementInterval        `json:"interval"`
	Allocations []FinanceSettlementAllocationView `json:"allocations"`
}

type FinanceSettlementIntervalInput struct {
	AccountID               int64
	AccountFinanceProfileID *int64
	ScopeKey                string
	PreviousSnapshotID      int64
	CurrentSnapshotID       int64
	PeriodStart             time.Time
	PeriodEnd               time.Time
	UnitSemantics           string
	Currency                *string
	FXRateVersionID         *int64
	FXRateToUSD             *decimal.Decimal
	FXSource                string
	FXObservedAt            *time.Time
	ListCostDelta           *decimal.Decimal // Original upstream currency, identified by Currency.
	ActualCostDelta         decimal.Decimal  // Original upstream currency, identified by Currency.
	ObservedMultiplier      *decimal.Decimal
}

type FinanceSettlementRepository interface {
	CreateOrGetSettlementInterval(ctx context.Context, input FinanceSettlementIntervalInput) (*FinanceSettlementInterval, bool, error)
	ListSettlementSegments(ctx context.Context, interval *FinanceSettlementInterval) ([]FinanceSettlementSegment, error)
	MarkSettlementNeedsReview(ctx context.Context, intervalID int64, requestCount, segmentCount int64, standardCost, difference decimal.Decimal, summary string) error
	ApplySettlement(ctx context.Context, interval *FinanceSettlementInterval, result FinanceSettlementAllocationResult, auditReason string, operatorID *int64) error
	ListSettlementIntervals(ctx context.Context, filter FinanceSettlementListFilter) ([]FinanceSettlementInterval, int64, error)
	GetSettlementInterval(ctx context.Context, intervalID int64) (*FinanceSettlementInterval, error)
	ListSettlementAllocations(ctx context.Context, intervalID int64) ([]FinanceSettlementAllocationView, error)
	ReallocateSettlement(ctx context.Context, intervalID int64, expectedRevision int, reason string, operatorID int64) (*FinanceSettlementInterval, error)
}

type FinanceFXRateResolver interface {
	FindFinanceFXRateAt(ctx context.Context, currency string, at time.Time) (*FinanceFXRateVersion, error)
}

type AccountFinanceSettlementProcessor interface {
	ProcessSnapshotInterval(ctx context.Context, previous, current *AccountFinanceCounterSnapshot) error
}

type AccountFinanceSettlementService struct {
	repo       FinanceSettlementRepository
	fxResolver FinanceFXRateResolver
	now        func() time.Time
}

func NewAccountFinanceSettlementService(repo FinanceSettlementRepository) *AccountFinanceSettlementService {
	service := &AccountFinanceSettlementService{repo: repo, now: time.Now}
	if resolver, ok := repo.(FinanceFXRateResolver); ok {
		service.fxResolver = resolver
	}
	return service
}

func (s *AccountFinanceSettlementService) ProcessSnapshotInterval(ctx context.Context, previous, current *AccountFinanceCounterSnapshot) error {
	if s == nil || s.repo == nil || previous == nil || current == nil {
		return nil
	}
	if current.UnitSemantics != AccountFinanceUnitFiatCurrency || current.ActualCostDelta == nil {
		return nil
	}
	periodStart := previous.CollectedAt
	periodEnd := current.CollectedAt
	if previous.UpstreamObservedAt != nil && current.UpstreamObservedAt != nil {
		periodStart = previous.UpstreamObservedAt.UTC()
		periodEnd = current.UpstreamObservedAt.UTC()
	}
	if !periodEnd.After(periodStart) {
		return fmt.Errorf("%w: snapshot interval is not increasing", ErrFinanceSettlementInvalid)
	}
	fxVersionID, fxRate, fxSource, fxObservedAt := financeFXSnapshotEvidence(current.SafeSnapshot, current.Currency, current.CollectedAt)
	if s.fxResolver != nil && current.Currency != nil && !strings.EqualFold(strings.TrimSpace(*current.Currency), "USD") {
		observedAt := current.CollectedAt
		if current.UpstreamObservedAt != nil {
			observedAt = current.UpstreamObservedAt.UTC()
		}
		manual, resolveErr := s.fxResolver.FindFinanceFXRateAt(ctx, *current.Currency, observedAt)
		if resolveErr != nil {
			return resolveErr
		}
		if manual != nil {
			versionID := manual.ID
			rate, parseErr := decimal.NewFromString(manual.RateToUSD)
			if parseErr != nil || !rate.IsPositive() {
				return fmt.Errorf("%w: frozen manual FX rate is invalid", ErrFinanceSettlementInvalid)
			}
			fxVersionID, fxRate, fxSource, fxObservedAt = &versionID, &rate, "manual_fx_version", &manual.ObservedAt
		} else if fxVersionID != nil {
			// An upstream-provided integer is not a local database identity. Do
			// not persist it unless the local historical resolver can validate it.
			fxVersionID = nil
		}
	}
	interval, _, err := s.repo.CreateOrGetSettlementInterval(ctx, FinanceSettlementIntervalInput{
		AccountID: current.AccountID, AccountFinanceProfileID: cloneInt64Pointer(current.AccountFinanceProfileID), ScopeKey: current.ScopeKey,
		PreviousSnapshotID: previous.ID, CurrentSnapshotID: current.ID,
		PeriodStart: periodStart, PeriodEnd: periodEnd, UnitSemantics: current.UnitSemantics,
		Currency: current.Currency, ListCostDelta: cloneFinanceDecimal(current.ListCostDelta),
		ActualCostDelta: *current.ActualCostDelta, ObservedMultiplier: cloneFinanceDecimal(current.ObservedMultiplier),
		FXRateVersionID: fxVersionID, FXRateToUSD: fxRate, FXSource: fxSource, FXObservedAt: fxObservedAt,
	})
	if err != nil || interval == nil || interval.Status == FinanceSettlementSettled {
		return err
	}
	return s.settleInterval(ctx, interval)
}

func (s *AccountFinanceSettlementService) List(ctx context.Context, filter FinanceSettlementListFilter) ([]FinanceSettlementInterval, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	filter.Status = strings.TrimSpace(filter.Status)
	if filter.Status != "" && filter.Status != FinanceSettlementPending && filter.Status != FinanceSettlementSettled && filter.Status != FinanceSettlementNeedsReview && filter.Status != FinanceSettlementFailed {
		return nil, 0, &FinanceSettlementError{Code: "SETTLEMENT_INVALID", Message: "结算状态无效"}
	}
	return s.repo.ListSettlementIntervals(ctx, filter)
}

func (s *AccountFinanceSettlementService) Detail(ctx context.Context, intervalID int64) (*FinanceSettlementDetail, error) {
	if intervalID <= 0 {
		return nil, &FinanceSettlementError{Code: "SETTLEMENT_INVALID", Message: "结算区间 ID 无效"}
	}
	interval, err := s.repo.GetSettlementInterval(ctx, intervalID)
	if err != nil {
		return nil, err
	}
	allocations, err := s.repo.ListSettlementAllocations(ctx, intervalID)
	if err != nil {
		return nil, err
	}
	return &FinanceSettlementDetail{Interval: interval, Allocations: allocations}, nil
}

func (s *AccountFinanceSettlementService) Retry(ctx context.Context, intervalID int64, operatorID int64) (*FinanceSettlementDetail, error) {
	if operatorID <= 0 {
		return nil, &FinanceSettlementError{Code: "SETTLEMENT_INVALID", Message: "操作人无效"}
	}
	interval, err := s.repo.GetSettlementInterval(ctx, intervalID)
	if err != nil {
		return nil, err
	}
	if interval.Status == FinanceSettlementSettled {
		return nil, &FinanceSettlementError{Code: "SETTLEMENT_STATE_CONFLICT", Message: "已结算区间不能重试"}
	}
	if interval.UnitSemantics != AccountFinanceUnitFiatCurrency {
		return nil, &FinanceSettlementError{Code: "SETTLEMENT_STATE_CONFLICT", Message: "平台记账单位区间只用于对账，不能分摊成本"}
	}
	if err = s.settleIntervalWithAudit(ctx, interval, "manual settlement retry", &operatorID); err != nil {
		return nil, err
	}
	return s.Detail(ctx, intervalID)
}

func (s *AccountFinanceSettlementService) Reallocate(ctx context.Context, intervalID int64, expectedRevision int, reason string, operatorID int64) (*FinanceSettlementDetail, error) {
	reason = strings.TrimSpace(reason)
	if intervalID <= 0 || expectedRevision <= 0 || operatorID <= 0 || len([]rune(reason)) < 5 || len([]rune(reason)) > 500 {
		return nil, &FinanceSettlementError{Code: "SETTLEMENT_INVALID", Message: "重新分摊原因需为 5 至 500 个字符"}
	}
	if _, err := s.repo.ReallocateSettlement(ctx, intervalID, expectedRevision, reason, operatorID); err != nil {
		return nil, err
	}
	return s.Detail(ctx, intervalID)
}

func (s *AccountFinanceSettlementService) settleInterval(ctx context.Context, interval *FinanceSettlementInterval) error {
	return s.settleIntervalWithAudit(ctx, interval, "", nil)
}

func (s *AccountFinanceSettlementService) settleIntervalWithAudit(ctx context.Context, interval *FinanceSettlementInterval, auditReason string, operatorID *int64) error {
	segments, err := s.repo.ListSettlementSegments(ctx, interval)
	if err != nil {
		return err
	}
	actualCostUSD, listCostUSD, conversionErr := FinanceSettlementDeltasUSD(interval)
	if conversionErr != nil {
		return s.repo.MarkSettlementNeedsReview(ctx, interval.ID, distinctSettlementRequestCount(segments), int64(len(segments)), decimal.Zero, decimal.Zero, conversionErr.Error())
	}
	result, allocationErr := AllocateFinanceSettlement(actualCostUSD, segments)
	if allocationErr != nil {
		difference := decimal.Zero
		if listCostUSD != nil {
			difference = listCostUSD.Neg()
		}
		return s.repo.MarkSettlementNeedsReview(ctx, interval.ID, distinctSettlementRequestCount(segments), int64(len(segments)), decimal.Zero, difference, allocationErr.Error())
	}
	if listCostUSD == nil {
		return s.repo.ApplySettlement(ctx, interval, result, auditReason, operatorID)
	}
	difference := result.StandardCostTotal.Sub(*listCostUSD).Round(financeAmountScale)
	tolerance := listCostUSD.Abs().Mul(decimal.RequireFromString("0.0001"))
	minimumTolerance := decimal.RequireFromString("0.000001")
	if tolerance.LessThan(minimumTolerance) {
		tolerance = minimumTolerance
	}
	if difference.Abs().GreaterThan(tolerance) {
		summary := fmt.Sprintf("local standard cost USD %s does not match upstream list delta USD %s", result.StandardCostTotal.String(), listCostUSD.String())
		return s.repo.MarkSettlementNeedsReview(ctx, interval.ID, distinctSettlementRequestCount(segments), int64(len(segments)), result.StandardCostTotal, difference, summary)
	}
	return s.repo.ApplySettlement(ctx, interval, result, auditReason, operatorID)
}

// FinanceSettlementDeltasUSD converts the immutable upstream counter deltas into
// the USD unit used by finance records and reports. The interval fields remain in
// the upstream source currency as audit evidence; only the returned values are USD.
func FinanceSettlementDeltasUSD(interval *FinanceSettlementInterval) (decimal.Decimal, *decimal.Decimal, error) {
	if interval == nil || interval.UnitSemantics != AccountFinanceUnitFiatCurrency {
		return decimal.Zero, nil, fmt.Errorf("%w: fiat settlement interval is required", ErrFinanceSettlementInvalid)
	}
	currency := strings.ToUpper(strings.TrimSpace(dereferenceString(interval.Currency)))
	if len(currency) != 3 {
		return decimal.Zero, nil, fmt.Errorf("%w: settlement source currency is missing", ErrFinanceSettlementInvalid)
	}
	rate := decimal.NewFromInt(1)
	if currency == "USD" {
		if interval.FXRateToUSD != nil && !interval.FXRateToUSD.Equal(decimal.NewFromInt(1)) {
			return decimal.Zero, nil, fmt.Errorf("%w: USD settlement FX rate must equal 1", ErrFinanceSettlementInvalid)
		}
	} else {
		if interval.FXRateVersionID == nil || *interval.FXRateVersionID <= 0 || interval.FXRateToUSD == nil || !interval.FXRateToUSD.IsPositive() {
			return decimal.Zero, nil, fmt.Errorf("%w: frozen FX rate version and rate are required for %s settlement", ErrFinanceSettlementInvalid, currency)
		}
		rate = *interval.FXRateToUSD
	}
	actualUSD := interval.ActualCostDelta.Mul(rate).Round(financeAmountScale)
	var listUSD *decimal.Decimal
	if interval.ListCostDelta != nil {
		value := interval.ListCostDelta.Mul(rate).Round(financeAmountScale)
		listUSD = &value
	}
	return actualUSD, listUSD, nil
}

func distinctSettlementRequestCount(segments []FinanceSettlementSegment) int64 {
	seen := make(map[int64]struct{}, len(segments))
	for _, segment := range segments {
		seen[segment.UsageLogID] = struct{}{}
	}
	return int64(len(seen))
}

func AllocateFinanceSettlement(actualCost decimal.Decimal, segments []FinanceSettlementSegment) (FinanceSettlementAllocationResult, error) {
	if actualCost.IsNegative() {
		return FinanceSettlementAllocationResult{}, fmt.Errorf("%w: actual cost must not be negative", ErrFinanceSettlementInvalid)
	}
	if len(segments) == 0 {
		return FinanceSettlementAllocationResult{}, fmt.Errorf("%w: segments are required", ErrFinanceSettlementInvalid)
	}

	normalized := append([]FinanceSettlementSegment(nil), segments...)
	seen := make(map[string]struct{}, len(normalized))
	standardTotal := decimal.Zero
	for i := range normalized {
		segment := &normalized[i]
		if segment.UsageLogID <= 0 || segment.AttemptNo <= 0 || segment.UsageCreatedAt.IsZero() {
			return FinanceSettlementAllocationResult{}, fmt.Errorf("%w: segment identity is incomplete", ErrFinanceSettlementInvalid)
		}
		if segment.StandardCost.IsNegative() {
			return FinanceSettlementAllocationResult{}, fmt.Errorf("%w: standard cost must not be negative", ErrFinanceSettlementInvalid)
		}
		key := fmt.Sprintf("%d:%d", segment.UsageLogID, segment.AttemptNo)
		if _, duplicate := seen[key]; duplicate {
			return FinanceSettlementAllocationResult{}, fmt.Errorf("%w: duplicate segment %s", ErrFinanceSettlementInvalid, key)
		}
		seen[key] = struct{}{}
		standardTotal = standardTotal.Add(segment.StandardCost)
	}
	if standardTotal.IsZero() {
		return FinanceSettlementAllocationResult{}, ErrFinanceSettlementZeroWeight
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		if !normalized[i].UsageCreatedAt.Equal(normalized[j].UsageCreatedAt) {
			return normalized[i].UsageCreatedAt.Before(normalized[j].UsageCreatedAt)
		}
		if normalized[i].UsageLogID != normalized[j].UsageLogID {
			return normalized[i].UsageLogID < normalized[j].UsageLogID
		}
		return normalized[i].AttemptNo < normalized[j].AttemptNo
	})

	allocations := make([]FinanceSettlementAllocation, 0, len(normalized))
	allocated := decimal.Zero
	for i, segment := range normalized {
		rate := segment.StandardCost.DivRound(standardTotal, financeAmountScale+6)
		amount := decimal.Zero
		if i == len(normalized)-1 {
			amount = actualCost.Sub(allocated).Round(financeAmountScale)
		} else {
			amount = actualCost.Mul(segment.StandardCost).Div(standardTotal).Round(financeAmountScale)
			allocated = allocated.Add(amount)
		}
		allocations = append(allocations, FinanceSettlementAllocation{
			UsageLogID: segment.UsageLogID, AttemptNo: segment.AttemptNo,
			StandardCost:   segment.StandardCost.Round(financeAmountScale),
			AllocationRate: rate, AllocatedCost: amount,
		})
	}
	allocatedTotal := decimal.Zero
	for _, allocation := range allocations {
		allocatedTotal = allocatedTotal.Add(allocation.AllocatedCost)
	}
	difference := actualCost.Round(financeAmountScale).Sub(allocatedTotal).Round(financeAmountScale)
	if !difference.IsZero() {
		return FinanceSettlementAllocationResult{}, fmt.Errorf("%w: allocation difference %s", ErrFinanceSettlementInvalid, difference.String())
	}
	return FinanceSettlementAllocationResult{
		StandardCostTotal: standardTotal.Round(financeAmountScale),
		ActualCostTotal:   actualCost.Round(financeAmountScale),
		AllocatedTotal:    allocatedTotal.Round(financeAmountScale),
		Difference:        difference, Allocations: allocations,
	}, nil
}

func ValidateFinanceSettlementTransition(from, to string) error {
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))
	allowed := map[string]map[string]struct{}{
		FinanceSettlementPending: {
			FinanceSettlementSettled: {}, FinanceSettlementNeedsReview: {}, FinanceSettlementFailed: {},
		},
		FinanceSettlementNeedsReview: {FinanceSettlementPending: {}, FinanceSettlementFailed: {}},
		FinanceSettlementFailed:      {FinanceSettlementPending: {}},
		FinanceSettlementSettled:     {},
	}
	if _, known := allowed[from]; !known {
		return fmt.Errorf("%w: unknown current status", ErrFinanceSettlementInvalid)
	}
	if _, ok := allowed[from][to]; !ok {
		return fmt.Errorf("%w: transition %s to %s is not allowed", ErrFinanceSettlementInvalid, from, to)
	}
	return nil
}

func financeFXSnapshotEvidence(snapshot map[string]any, currency *string, fallback time.Time) (*int64, *decimal.Decimal, string, *time.Time) {
	if snapshot == nil {
		snapshot = map[string]any{}
	}
	if nested, ok := snapshot["payload"].(map[string]any); ok {
		merged := cloneFinanceSnapshot(nested)
		for key, value := range snapshot {
			if key != "payload" {
				merged[key] = value
			}
		}
		snapshot = merged
	}
	var versionID *int64
	if raw, ok := snapshot["fx_rate_version_id"]; ok {
		if parsed, err := decimal.NewFromString(strings.TrimSpace(fmt.Sprint(raw))); err == nil && parsed.IsInteger() && parsed.IsPositive() {
			value := parsed.IntPart()
			versionID = &value
		}
	}
	rate, _ := ParseFinanceDecimal(snapshot["fx_rate_to_usd"])
	if rate == nil && strings.EqualFold(strings.TrimSpace(dereferenceString(currency)), "USD") {
		identity := decimal.NewFromInt(1)
		rate = &identity
	}
	source := strings.TrimSpace(fmt.Sprint(snapshot["fx_source"]))
	if source == "<nil>" {
		source = ""
	}
	var observedAt *time.Time
	if raw := strings.TrimSpace(fmt.Sprint(snapshot["fx_observed_at"])); raw != "" && raw != "<nil>" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			observedAt = &parsed
		}
	}
	if observedAt == nil && !fallback.IsZero() && rate != nil {
		observedAt = &fallback
	}
	return versionID, rate, source, observedAt
}
