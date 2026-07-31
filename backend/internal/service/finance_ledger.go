package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type FinanceUsageCursor struct {
	CreatedAt time.Time
	ID        int64
}

type UsageFinanceCostSegment struct {
	AttemptNo                     int
	AccountID                     int64
	WalletID                      *int64
	UpstreamID                    *int64
	ChannelID                     *int64
	UpstreamModel                 string
	ServiceTier                   *string
	UsageDetail                   map[string]any
	UpstreamCostMultiplier        *decimal.Decimal
	UpstreamMultiplierChangeID    *int64
	UpstreamMultiplierSource      string
	UpstreamMultiplierEffectiveAt *time.Time
	AccountFinanceProfileID       *int64
	PriceVersionID                *int64
	FXRateVersionID               *int64
	SourceCurrency                string
	FXRateToUSD                   *decimal.Decimal
	FXSource                      string
	FXObservedAt                  *time.Time
	PricingSource                 string
	CostStatus                    FinanceCostStatus
	CostAmount                    *decimal.Decimal
	CalculationDetail             map[string]any
}

type UsageFinanceProjection struct {
	ID                             int64
	UsageLogID                     int64
	UserID                         int64
	GroupID                        *int64
	ChannelID                      *int64
	AccountID                      *int64
	WalletID                       *int64
	UpstreamID                     *int64
	UsageCreatedAt                 time.Time
	RequestedModel                 string
	UpstreamModel                  *string
	ServiceTier                    *string
	BillingType                    string
	BusinessType                   string
	CustomerBillingType            int8
	UsageListValue                 *decimal.Decimal
	UpstreamCost                   *decimal.Decimal
	CostStatus                     FinanceCostStatus
	PricingSource                  string
	PriceVersionID                 *int64
	UpstreamCostMultiplierSnapshot *decimal.Decimal
	UpstreamMultiplierChangeID     *int64
	UpstreamMultiplierSource       string
	UpstreamMultiplierEffectiveAt  *time.Time
	AccountFinanceProfileID        *int64
	FXRateVersionID                *int64
	SourceCurrency                 string
	FXRateToUSD                    *decimal.Decimal
	FXSource                       string
	FXObservedAt                   *time.Time
	CurrentRevision                int
	CalculationDetail              map[string]any
	CalculatedAt                   time.Time
	Segments                       []UsageFinanceCostSegment
}

type FinanceRevisionMetadata struct {
	Reason     string
	JobID      *int64
	OperatorID *int64
}

type FinanceLedgerRepository interface {
	FinanceLaunchAt(ctx context.Context) (time.Time, error)
	TryAcquireScannerLease(ctx context.Context) (release func(), acquired bool, err error)
	ListPendingUsage(ctx context.Context, cursor FinanceUsageCursor, limit int) ([]UsageLog, error)
	LoadUsageAttempts(ctx context.Context, usageLogIDs []int64) (map[int64][]UsageUpstreamAttempt, error)
	CreateFinanceProjection(ctx context.Context, projection *UsageFinanceProjection) (bool, error)
	ReviseFinanceProjection(ctx context.Context, projection *UsageFinanceProjection, metadata FinanceRevisionMetadata) (bool, error)
	RecordFinanceProjectionFailure(ctx context.Context, usageLogID int64, message string, failedAt time.Time) error
	ResolveFinanceProjectionFailure(ctx context.Context, usageLogID int64, resolvedAt time.Time) error
}

type FinanceUsageScanner struct {
	repository FinanceLedgerRepository
	selector   *FinancePriceSelector
	calculator *FinanceCostCalculator
	batchSize  int
	cursor     FinanceUsageCursor
	now        func() time.Time
}

type FinanceScanResult struct {
	Acquired    bool
	Processed   int
	Succeeded   int
	Failed      int
	Cursor      FinanceUsageCursor
	SucceededAt []time.Time
	Errors      []string
}

func NewFinanceUsageScanner(repository FinanceLedgerRepository, selector *FinancePriceSelector, calculator *FinanceCostCalculator) *FinanceUsageScanner {
	return &FinanceUsageScanner{
		repository: repository,
		selector:   selector,
		calculator: calculator,
		batchSize:  500,
		now:        time.Now,
	}
}

// BuildHistoricalProjection rebuilds a finance projection exclusively from the
// immutable usage log, recorded upstream attempts, request-time multiplier
// snapshots and price versions effective at the request timestamp.
func (s *FinanceUsageScanner) BuildHistoricalProjection(ctx context.Context, log *UsageLog, attempts []UsageUpstreamAttempt) (*UsageFinanceProjection, error) {
	if s == nil || s.repository == nil || s.selector == nil || s.calculator == nil {
		return nil, errors.New("finance scanner dependencies are unavailable")
	}
	launchAt, err := s.repository.FinanceLaunchAt(ctx)
	if err != nil {
		return nil, err
	}
	return s.buildProjection(ctx, log, attempts, launchAt)
}

func (s *FinanceUsageScanner) RunBatch(ctx context.Context) (FinanceScanResult, error) {
	if s == nil || s.repository == nil || s.selector == nil || s.calculator == nil {
		return FinanceScanResult{}, errors.New("finance scanner dependencies are unavailable")
	}
	release, acquired, err := s.repository.TryAcquireScannerLease(ctx)
	if err != nil {
		return FinanceScanResult{}, err
	}
	result := FinanceScanResult{Acquired: acquired, Cursor: s.cursor}
	if !acquired {
		return result, nil
	}
	defer release()
	launchAt, err := s.repository.FinanceLaunchAt(ctx)
	if err != nil {
		return result, err
	}
	logs, err := s.repository.ListPendingUsage(ctx, s.cursor, s.batchSize)
	if err != nil {
		return result, err
	}
	if len(logs) == 0 && (!s.cursor.CreatedAt.IsZero() || s.cursor.ID != 0) {
		s.cursor = FinanceUsageCursor{}
		result.Cursor = s.cursor
		return result, nil
	}
	ids := make([]int64, 0, len(logs))
	for i := range logs {
		ids = append(ids, logs[i].ID)
	}
	attemptsByUsage, err := s.repository.LoadUsageAttempts(ctx, ids)
	if err != nil {
		return result, err
	}
	for i := range logs {
		log := &logs[i]
		result.Processed++
		projection, buildErr := s.buildProjection(ctx, log, attemptsByUsage[log.ID], launchAt)
		if buildErr == nil {
			_, buildErr = s.repository.CreateFinanceProjection(ctx, projection)
		}
		if buildErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("usage_log_id=%d: %v", log.ID, buildErr))
			if retryErr := s.repository.RecordFinanceProjectionFailure(ctx, log.ID, buildErr.Error(), s.now()); retryErr != nil {
				return result, fmt.Errorf("persist finance retry for usage_log_id=%d: %w", log.ID, retryErr)
			}
		} else {
			result.Succeeded++
			result.SucceededAt = append(result.SucceededAt, log.CreatedAt)
			if retryErr := s.repository.ResolveFinanceProjectionFailure(ctx, log.ID, s.now()); retryErr != nil {
				return result, fmt.Errorf("resolve finance retry for usage_log_id=%d: %w", log.ID, retryErr)
			}
		}
		if log.CreatedAt.After(s.cursor.CreatedAt) || (log.CreatedAt.Equal(s.cursor.CreatedAt) && log.ID > s.cursor.ID) {
			s.cursor = FinanceUsageCursor{CreatedAt: log.CreatedAt, ID: log.ID}
		}
		result.Cursor = s.cursor
	}
	return result, nil
}

func (s *FinanceUsageScanner) buildProjection(ctx context.Context, log *UsageLog, attempts []UsageUpstreamAttempt, launchAt time.Time) (*UsageFinanceProjection, error) {
	if log == nil {
		return nil, errors.New("usage log is required")
	}
	// Historical usage before the finance launch may have upstream attempts
	// without a frozen account profile. Keep those records on their historical
	// system-price path instead of treating the missing profile as a current
	// configuration error. A synthetic attempt is still built when no attempt
	// row exists at all.
	legacyFallback := log.CreatedAt.Before(launchAt)
	if len(attempts) == 0 && legacyFallback {
		if attempt, ok := BuildFinalUsageUpstreamAttempt(log); ok {
			attempts = []UsageUpstreamAttempt{attempt}
			legacyFallback = true
		}
	}
	mode := financeBillingModeForUsage(log)
	businessType := financeBusinessTypeForUsage(log)
	excluded := businessType == "admin" || log.FinanceExcluded
	inputs := make([]FinanceCostCalculatorInput, 0, len(attempts))
	selections := make([]*FinanceSelectedPrice, 0, len(attempts))
	for i := range attempts {
		attempt := attempts[i]
		at := attempt.CreatedAt
		if at.IsZero() {
			at = log.CreatedAt
		}
		tier := dereferenceString(attempt.ServiceTier)
		var selected *FinanceSelectedPrice
		var err error
		if legacyFallback && attempt.AccountFinanceProfileID == nil {
			selected, err = s.selector.SelectLegacy(ctx, attempt.UpstreamModel, mode, tier, at)
		} else {
			selected, err = s.selector.Select(ctx, attempt.AccountID, attempt.AccountFinanceProfileID, attempt.UpstreamModel, mode, tier, at)
		}
		if err != nil {
			return nil, fmt.Errorf("select attempt %d price: %w", attempt.AttemptNo, err)
		}
		if selected != nil && selected.CostMultiplier != nil {
			attempt.UpstreamCostMultiplier = cloneDecimal(selected.CostMultiplier)
			attempts[i] = attempt
		}
		if !strings.EqualFold(strings.TrimSpace(attempt.UpstreamChargeCurrency), "USD") {
			if attempt.UpstreamChargeSnapshot == nil {
				attempt.UpstreamChargeSnapshot = map[string]any{}
			}
			fxVersion, fxErr := s.selector.ResolveFXRateAt(ctx, attempt.UpstreamChargeCurrency, at)
			if fxErr != nil {
				return nil, fmt.Errorf("resolve request charge FX version: %w", fxErr)
			}
			if fxVersion == nil || attempt.UpstreamActualCharge == nil {
				// A non-USD request charge is not exact until a local immutable
				// FX version and original-currency amount can prove the USD conversion.
				attempt.UpstreamActualChargeUSD = nil
				attempt.UpstreamChargeSnapshot["fx_source"] = "missing_manual_fx_rate"
			} else {
				rate, parseErr := decimal.NewFromString(fxVersion.RateToUSD)
				if parseErr != nil || !rate.IsPositive() {
					return nil, fmt.Errorf("invalid request charge FX rate version %d", fxVersion.ID)
				}
				converted := attempt.UpstreamActualCharge.Mul(rate)
				attempt.UpstreamActualChargeUSD = &converted
				attempt.UpstreamChargeSnapshot["fx_rate_version_id"] = fxVersion.ID
				attempt.UpstreamChargeSnapshot["fx_rate_to_usd"] = rate.String()
				attempt.UpstreamChargeSnapshot["fx_source"] = "manual_fx_version"
				attempt.UpstreamChargeSnapshot["fx_observed_at"] = fxVersion.ObservedAt.UTC().Format(time.RFC3339Nano)
			}
		}
		attempts[i] = attempt
		inputs = append(inputs, FinanceCostCalculatorInput{
			Attempt:               attempt,
			BillingMode:           mode,
			ServiceTier:           tier,
			ImageOutputTokens:     int64(max(log.ImageOutputTokens, 0)),
			Price:                 selected.Quote,
			MissingProfile:        selected.MissingProfile,
			RequestChargeExpected: selected.Profile != nil && selected.Profile.CostMode == FinanceCostModeRequestCharge && selected.Profile.BalanceUnitSemantics != FinanceUnitPlatformCredit,
			Excluded:              excluded,
		})
		selections = append(selections, selected)
	}
	aggregate := s.calculator.Aggregate(inputs)
	if excluded && len(attempts) == 0 {
		zero := decimal.Zero
		aggregate = FinanceCostAggregate{
			Status: FinanceCostStatusExcluded,
			Amount: &zero,
			Detail: map[string]any{"reason": "usage_excluded_from_finance", "segment_count": 0, "total": zero.StringFixed(financeAmountScale), "status": FinanceCostStatusExcluded},
		}
	} else if len(attempts) == 0 {
		aggregate = FinanceCostAggregate{
			Status: FinanceCostStatusMissingUsage,
			Detail: map[string]any{"reason": "usage_upstream_attempts_missing", "segment_count": 0},
		}
	}
	segments := make([]UsageFinanceCostSegment, 0, len(aggregate.Segments))
	for i, calculated := range aggregate.Segments {
		var walletID, upstreamID *int64
		var financeCostMode string
		if i < len(selections) && selections[i] != nil && selections[i].Wallet != nil {
			walletID = int64Snapshot(selections[i].Wallet.WalletID)
			upstreamID = int64Snapshot(selections[i].Wallet.UpstreamID)
		}
		if i < len(selections) && selections[i] != nil && selections[i].Profile != nil {
			financeCostMode = selections[i].Profile.CostMode
		}
		if calculated.CalculationDetail == nil {
			calculated.CalculationDetail = make(map[string]any)
		}
		if financeCostMode != "" {
			calculated.CalculationDetail["finance_cost_mode"] = financeCostMode
		}
		segments = append(segments, UsageFinanceCostSegment{
			AttemptNo:                     calculated.AttemptNo,
			AccountID:                     calculated.AccountID,
			WalletID:                      walletID,
			UpstreamID:                    upstreamID,
			ChannelID:                     calculated.ChannelID,
			UpstreamModel:                 calculated.UpstreamModel,
			ServiceTier:                   calculated.ServiceTier,
			UsageDetail:                   calculated.UsageDetail,
			UpstreamCostMultiplier:        calculated.UpstreamMultiplier,
			UpstreamMultiplierChangeID:    cloneInt64Pointer(attempts[i].UpstreamMultiplierChangeID),
			UpstreamMultiplierSource:      attempts[i].UpstreamMultiplierSource,
			UpstreamMultiplierEffectiveAt: cloneFinanceTime(attempts[i].UpstreamMultiplierEffectiveAt),
			AccountFinanceProfileID:       cloneInt64Pointer(attempts[i].AccountFinanceProfileID),
			PriceVersionID:                calculated.PriceVersionID,
			FXRateVersionID:               calculated.FXRateVersionID,
			SourceCurrency:                calculated.SourceCurrency,
			FXRateToUSD:                   cloneDecimal(calculated.FXRateToUSD),
			FXSource:                      calculated.FXSource,
			FXObservedAt:                  cloneFinanceTime(calculated.FXObservedAt),
			PricingSource:                 calculated.PricingSource,
			CostStatus:                    calculated.CostStatus,
			CostAmount:                    calculated.CostAmount,
			CalculationDetail:             calculated.CalculationDetail,
		})
	}
	requestedModel := strings.TrimSpace(log.RequestedModel)
	if requestedModel == "" {
		requestedModel = strings.TrimSpace(log.Model)
	}
	upstreamModel := log.UpstreamModel
	if upstreamModel == nil && strings.TrimSpace(log.Model) != "" {
		upstreamModel = stringSnapshot(strings.TrimSpace(log.Model))
	}
	accountID := int64Snapshot(log.AccountID)
	projection := &UsageFinanceProjection{
		UsageLogID:                     log.ID,
		UserID:                         log.UserID,
		GroupID:                        cloneInt64Pointer(log.GroupID),
		ChannelID:                      cloneInt64Pointer(log.ChannelID),
		AccountID:                      accountID,
		UsageCreatedAt:                 log.CreatedAt,
		RequestedModel:                 requestedModel,
		UpstreamModel:                  upstreamModel,
		ServiceTier:                    cloneStringPointer(log.ServiceTier),
		BillingType:                    mode,
		BusinessType:                   businessType,
		CustomerBillingType:            log.BillingType,
		UsageListValue:                 cloneDecimal(log.UsageListValue),
		UpstreamCost:                   cloneDecimal(aggregate.Amount),
		CostStatus:                     aggregate.Status,
		UpstreamCostMultiplierSnapshot: cloneDecimal(log.UpstreamCostMultiplier),
		UpstreamMultiplierChangeID:     cloneInt64Pointer(log.UpstreamMultiplierChangeID),
		UpstreamMultiplierSource:       log.UpstreamMultiplierSource,
		UpstreamMultiplierEffectiveAt:  cloneFinanceTime(log.UpstreamMultiplierEffectiveAt),
		AccountFinanceProfileID:        cloneInt64Pointer(log.AccountFinanceProfileID),
		CurrentRevision:                1,
		CalculationDetail:              aggregate.Detail,
		CalculatedAt:                   s.now(),
		Segments:                       segments,
	}
	if projection.CalculationDetail == nil {
		projection.CalculationDetail = make(map[string]any)
	}
	if len(selections) == 1 && selections[0] != nil && selections[0].Profile != nil {
		projection.CalculationDetail["finance_cost_mode"] = selections[0].Profile.CostMode
	}
	if projection.UsageListValue == nil {
		legacyValue := decimal.NewFromFloat(log.ActualCost)
		projection.UsageListValue = &legacyValue
		projection.CalculationDetail["sales_value_source"] = "legacy_actual_cost"
	}
	if businessType == "promotion" && log.PromotionCreditUsed != nil && log.PromotionCreditUsed.IsPositive() {
		paidRevenue := projection.UsageListValue.Sub(*log.PromotionCreditUsed)
		if paidRevenue.IsNegative() {
			paidRevenue = decimal.Zero
		}
		projection.UsageListValue = &paidRevenue
		if projection.CalculationDetail == nil {
			projection.CalculationDetail = make(map[string]any)
		}
		projection.CalculationDetail["promotion_credit_used"] = log.PromotionCreditUsed.StringFixed(financeAmountScale)
		projection.CalculationDetail["sales_value_source"] = "usage_value_less_promotion_credit"
	}
	if log.FinanceExcluded {
		if projection.CalculationDetail == nil {
			projection.CalculationDetail = make(map[string]any)
		}
		projection.CalculationDetail["finance_exclusion_reason"] = strings.TrimSpace(log.FinanceExclusionReason)
	}
	if legacyFallback {
		projection.CalculationDetail["attempt_source"] = "legacy_usage_log"
	}
	applyFinanceProjectionSummary(projection)
	return projection, nil
}

func financeBusinessTypeForUsage(log *UsageLog) string {
	if log == nil {
		return "balance"
	}
	if snapshot := strings.ToLower(strings.TrimSpace(log.FinanceBusinessTypeSnapshot)); snapshot != "" {
		return snapshot
	}
	if log.User != nil && strings.EqualFold(strings.TrimSpace(log.User.Role), "admin") {
		return "admin"
	}
	if log.BillingType == BillingTypeSubscription || log.SubscriptionID != nil {
		return "subscription"
	}
	return "balance"
}

func applyFinanceProjectionSummary(projection *UsageFinanceProjection) {
	if projection == nil || len(projection.Segments) == 0 {
		return
	}
	last := projection.Segments[len(projection.Segments)-1]
	projection.WalletID = cloneInt64Pointer(last.WalletID)
	projection.UpstreamID = cloneInt64Pointer(last.UpstreamID)
	projection.UpstreamCostMultiplierSnapshot = cloneDecimal(last.UpstreamCostMultiplier)
	pricingSource := last.PricingSource
	priceVersionID := cloneInt64Pointer(last.PriceVersionID)
	for _, segment := range projection.Segments[:len(projection.Segments)-1] {
		if segment.PricingSource != pricingSource {
			pricingSource = "mixed"
		}
		if !equalInt64Pointers(segment.PriceVersionID, priceVersionID) {
			priceVersionID = nil
		}
	}
	projection.PricingSource = pricingSource
	projection.PriceVersionID = priceVersionID
	fxVersionID := cloneInt64Pointer(last.FXRateVersionID)
	sourceCurrency := last.SourceCurrency
	fxRate := cloneDecimal(last.FXRateToUSD)
	fxSource := last.FXSource
	fxObservedAt := cloneFinanceTime(last.FXObservedAt)
	for _, segment := range projection.Segments[:len(projection.Segments)-1] {
		if !equalInt64Pointers(segment.FXRateVersionID, fxVersionID) || segment.SourceCurrency != sourceCurrency || !equalDecimalPointers(segment.FXRateToUSD, fxRate) {
			fxVersionID = nil
			fxRate = nil
			sourceCurrency = ""
			fxSource = "mixed"
			fxObservedAt = nil
			break
		}
	}
	projection.FXRateVersionID = fxVersionID
	projection.SourceCurrency = sourceCurrency
	projection.FXRateToUSD = fxRate
	projection.FXSource = fxSource
	projection.FXObservedAt = fxObservedAt
}

func financeBillingModeForUsage(log *UsageLog) string {
	if log != nil && log.BillingMode != nil {
		if mode := normalizeFinanceBillingMode(*log.BillingMode); mode != "" {
			return mode
		}
	}
	if log != nil {
		if log.VideoDurationSeconds != nil && *log.VideoDurationSeconds > 0 {
			return "per_second"
		}
		if log.ImageCount > 0 {
			return "image"
		}
	}
	return "token"
}

func financeProjectionSnapshot(projection *UsageFinanceProjection) map[string]any {
	if projection == nil {
		return map[string]any{}
	}
	segments := make([]map[string]any, 0, len(projection.Segments))
	for _, segment := range projection.Segments {
		segments = append(segments, map[string]any{
			"attempt_no":                    segment.AttemptNo,
			"account_id":                    segment.AccountID,
			"wallet_id":                     segment.WalletID,
			"upstream_id":                   segment.UpstreamID,
			"cost_status":                   segment.CostStatus,
			"cost_amount":                   decimalPointerString(segment.CostAmount),
			"price_version_id":              segment.PriceVersionID,
			"pricing_source":                segment.PricingSource,
			"upstream_multiplier_change_id": segment.UpstreamMultiplierChangeID,
			"account_finance_profile_id":    segment.AccountFinanceProfileID,
			"fx_rate_version_id":            segment.FXRateVersionID,
			"source_currency":               segment.SourceCurrency,
			"fx_rate_to_usd":                decimalPointerString(segment.FXRateToUSD),
			"calculation_detail":            segment.CalculationDetail,
		})
	}
	return map[string]any{
		"usage_log_id":                      projection.UsageLogID,
		"upstream_cost":                     decimalPointerString(projection.UpstreamCost),
		"cost_status":                       projection.CostStatus,
		"pricing_source":                    projection.PricingSource,
		"price_version_id":                  projection.PriceVersionID,
		"upstream_cost_multiplier_snapshot": projection.UpstreamCostMultiplierSnapshot,
		"upstream_multiplier_change_id":     projection.UpstreamMultiplierChangeID,
		"account_finance_profile_id":        projection.AccountFinanceProfileID,
		"fx_rate_version_id":                projection.FXRateVersionID,
		"source_currency":                   projection.SourceCurrency,
		"fx_rate_to_usd":                    decimalPointerString(projection.FXRateToUSD),
		"calculation_detail":                projection.CalculationDetail,
		"segments":                          segments,
	}
}

func financeProjectionEqual(left, right *UsageFinanceProjection) bool {
	leftJSON, _ := json.Marshal(financeProjectionSnapshot(left))
	rightJSON, _ := json.Marshal(financeProjectionSnapshot(right))
	return string(leftJSON) == string(rightJSON)
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func int64Snapshot(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func equalInt64Pointers(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalDecimalPointers(left, right *decimal.Decimal) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
