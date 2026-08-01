package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

var ErrUpstreamFundIdempotencyConflict = errors.New("Idempotency-Key was already used with different fund event data")
var ErrUpstreamFundEventNotFound = errors.New("upstream fund event not found")
var ErrUpstreamFundDuplicateReference = errors.New("upstream fund reference number already exists for this wallet and event type")

type UpstreamFundEvent struct {
	ID                     int64            `json:"id"`
	WalletID               int64            `json:"wallet_id"`
	EventType              string           `json:"event_type"`
	OriginalAmount         decimal.Decimal  `json:"original_amount"`
	Currency               string           `json:"currency"`
	FXRateToUSD            decimal.Decimal  `json:"fx_rate_to_usd"`
	FXSource               string           `json:"fx_source"`
	FXObservedAt           time.Time        `json:"fx_observed_at"`
	FXRateVersionID        *int64           `json:"fx_rate_version_id,omitempty"`
	USDAmount              decimal.Decimal  `json:"usd_amount"`
	BaseCreditUnits        *decimal.Decimal `json:"base_credit_units,omitempty"`
	BonusCreditUnits       *decimal.Decimal `json:"bonus_credit_units,omitempty"`
	TotalCreditUnits       *decimal.Decimal `json:"total_credit_units,omitempty"`
	BaseRechargeRatio      *decimal.Decimal `json:"base_recharge_ratio,omitempty"`
	EffectiveRechargeRatio *decimal.Decimal `json:"effective_recharge_ratio,omitempty"`
	BonusIncomeOriginal    *decimal.Decimal `json:"bonus_income_original,omitempty"`
	BonusIncomeUSD         *decimal.Decimal `json:"bonus_income_usd,omitempty"`
	BonusStatus            string           `json:"bonus_status"`
	ReversedEventID        *int64           `json:"reversed_event_id,omitempty"`
	OccurredAt             time.Time        `json:"occurred_at"`
	ReferenceNo            string           `json:"reference_no,omitempty"`
	Note                   string           `json:"note"`
	OperatorID             *int64           `json:"operator_id,omitempty"`
	IdempotencyKey         string           `json:"-"`
	CreatedAt              time.Time        `json:"created_at"`
}

type UpstreamFundEventInput struct {
	EventType        string    `json:"event_type"`
	OriginalAmount   string    `json:"original_amount"`
	Currency         string    `json:"currency"`
	FXRateToUSD      string    `json:"fx_rate_to_usd"`
	FXSource         string    `json:"fx_source"`
	FXObservedAt     time.Time `json:"fx_observed_at"`
	USDAmount        string    `json:"usd_amount"`
	BaseCreditUnits  string    `json:"base_credit_units"`
	BonusCreditUnits string    `json:"bonus_credit_units"`
	ReversedEventID  *int64    `json:"reversed_event_id"`
	OccurredAt       time.Time `json:"occurred_at"`
	ReferenceNo      string    `json:"reference_no"`
	Note             string    `json:"note"`
	OperatorID       *int64    `json:"-"`
	IdempotencyKey   string    `json:"-"`
}

type UpstreamFundRepository interface {
	CreateFundEvent(ctx context.Context, event *UpstreamFundEvent) (bool, error)
	GetFundEvent(ctx context.Context, walletID, eventID int64) (*UpstreamFundEvent, error)
	ListFundEvents(ctx context.Context, walletID int64, page, pageSize int) ([]UpstreamFundEvent, int64, error)
}

// UpstreamFundOpeningBalanceRepository may persist the fund event and its
// balance snapshot in one database transaction. The optional capability keeps
// opening-balance initialization from leaving an event without its snapshot.
type UpstreamFundOpeningBalanceRepository interface {
	CreateFundEventWithOpeningBalance(ctx context.Context, event *UpstreamFundEvent) (bool, error)
}

type UpstreamFundService struct {
	wallets  *UpstreamWalletService
	repo     UpstreamFundRepository
	balances UpstreamFundBalanceRecorder
}

// InitializeOpeningBalance stores the financial opening balance even when it
// is zero. Positive balances also receive an immutable fund event; zero does
// not represent a recharge and therefore only needs a balance observation.
func (s *UpstreamFundService) InitializeOpeningBalance(ctx context.Context, walletID int64, amount decimal.Decimal, currency string, occurredAt time.Time, operatorID *int64, note, idempotencyKey string) (*UpstreamFundEvent, bool, error) {
	wallet, err := s.wallets.Get(ctx, walletID)
	if err != nil {
		return nil, false, err
	}
	if wallet.BalanceKind != "wallet_cash" {
		return nil, false, financeValidationError("opening balance requires a cash wallet")
	}
	if amount.IsNegative() {
		return nil, false, financeValidationError("opening balance must not be negative")
	}
	if occurredAt.IsZero() {
		return nil, false, financeValidationError("opening balance occurred_at is required")
	}
	if amount.IsZero() {
		if s.balances == nil {
			return nil, false, errors.New("opening balance recorder is unavailable")
		}
		if err := s.balances.RecordOpeningBalance(ctx, walletID, amount, strings.ToUpper(strings.TrimSpace(currency)), occurredAt.UTC(), "opening-balance-initialization-"+strings.TrimSpace(idempotencyKey)); err != nil {
			return nil, false, err
		}
		return nil, true, nil
	}
	return s.Create(ctx, walletID, UpstreamFundEventInput{EventType: "opening_balance", OriginalAmount: amount.String(), Currency: currency, FXRateToUSD: "1", FXSource: "finance_initialization", FXObservedAt: occurredAt, USDAmount: amount.String(), OccurredAt: occurredAt, Note: note, OperatorID: operatorID, IdempotencyKey: idempotencyKey})
}

// RecordBalanceSnapshot records a manually observed balance without creating
// a recharge/opening fund event. It is used when an existing upstream's
// current balance is corrected from the management screen.
func (s *UpstreamFundService) RecordBalanceSnapshot(ctx context.Context, walletID int64, amount decimal.Decimal, currency string, occurredAt time.Time, dedupeKey string) error {
	wallet, err := s.wallets.Get(ctx, walletID)
	if err != nil {
		return err
	}
	if wallet.BalanceKind != "wallet_cash" {
		return financeValidationError("balance snapshot requires a cash wallet")
	}
	if amount.IsNegative() {
		return financeValidationError("balance snapshot must not be negative")
	}
	if occurredAt.IsZero() || strings.TrimSpace(dedupeKey) == "" {
		return financeValidationError("balance snapshot timestamp and dedupe key are required")
	}
	if s.balances == nil {
		return errors.New("balance recorder is unavailable")
	}
	return s.balances.RecordOpeningBalance(ctx, walletID, amount, strings.ToUpper(strings.TrimSpace(currency)), occurredAt.UTC(), dedupeKey)
}

func NewUpstreamFundService(wallets *UpstreamWalletService, repo UpstreamFundRepository, balanceRecorders ...UpstreamFundBalanceRecorder) *UpstreamFundService {
	service := &UpstreamFundService{wallets: wallets, repo: repo}
	if len(balanceRecorders) > 0 {
		service.balances = balanceRecorders[0]
	}
	return service
}

func (s *UpstreamFundService) List(ctx context.Context, walletID int64, page, pageSize int) ([]UpstreamFundEvent, int64, error) {
	if _, err := s.wallets.Get(ctx, walletID); err != nil {
		return nil, 0, err
	}
	normalizeFinancePage(&page, &pageSize)
	return s.repo.ListFundEvents(ctx, walletID, page, pageSize)
}

func (s *UpstreamFundService) Create(ctx context.Context, walletID int64, input UpstreamFundEventInput) (*UpstreamFundEvent, bool, error) {
	wallet, err := s.wallets.Get(ctx, walletID)
	if err != nil {
		return nil, false, err
	}
	if !wallet.Enabled {
		return nil, false, ErrUpstreamWalletDisabled
	}
	eventType := strings.ToLower(strings.TrimSpace(input.EventType))
	if eventType != "opening_balance" && eventType != "topup" && eventType != "refund" && eventType != "adjustment" {
		return nil, false, financeValidationError("event_type must be opening_balance, topup, refund or adjustment")
	}
	original, err := decimal.NewFromString(strings.TrimSpace(input.OriginalAmount))
	if err != nil {
		return nil, false, financeValidationError("original_amount must be a decimal string")
	}
	fxRate, err := decimal.NewFromString(strings.TrimSpace(input.FXRateToUSD))
	if err != nil || fxRate.LessThanOrEqual(decimal.Zero) {
		return nil, false, financeValidationError("fx_rate_to_usd must be a positive decimal string")
	}
	usdAmount, err := decimal.NewFromString(strings.TrimSpace(input.USDAmount))
	if err != nil {
		return nil, false, financeValidationError("usd_amount must be a decimal string")
	}
	if eventType != "adjustment" && original.LessThanOrEqual(decimal.Zero) {
		return nil, false, financeValidationError("original_amount must be positive for this event type")
	}
	if original.IsZero() {
		return nil, false, financeValidationError("original_amount must not be zero")
	}
	expectedUSD := original.Mul(fxRate).Round(8)
	if expectedUSD.Sub(usdAmount.Round(8)).Abs().GreaterThan(decimal.New(1, -8)) {
		return nil, false, financeValidationError("usd_amount does not match original_amount multiplied by fx_rate_to_usd")
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if len(currency) != 3 {
		return nil, false, financeValidationError("currency must be a 3-letter ISO code")
	}
	if input.OccurredAt.IsZero() {
		return nil, false, financeValidationError("occurred_at is required")
	}
	fxSource := strings.TrimSpace(input.FXSource)
	if fxSource == "" {
		fxSource = "manual"
	}
	if len(fxSource) > 80 {
		return nil, false, financeValidationError("fx_source must not exceed 80 characters")
	}
	fxObservedAt := input.FXObservedAt
	if fxObservedAt.IsZero() {
		fxObservedAt = input.OccurredAt
	}
	note := strings.TrimSpace(input.Note)
	if len([]rune(note)) < 3 || len([]rune(note)) > 1000 {
		return nil, false, financeValidationError("note must be 3 to 1000 characters")
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 200 {
		return nil, false, financeValidationError("Idempotency-Key header is required and must not exceed 200 characters")
	}
	referenceNo := strings.TrimSpace(input.ReferenceNo)
	if (eventType == "topup" || eventType == "refund") && referenceNo == "" {
		return nil, false, financeValidationError("reference_no is required for topup and refund events")
	}
	if len(referenceNo) > 200 {
		return nil, false, financeValidationError("reference_no must not exceed 200 characters")
	}
	event := &UpstreamFundEvent{
		WalletID: walletID, EventType: eventType, OriginalAmount: original, Currency: currency,
		FXRateToUSD: fxRate, FXSource: fxSource, FXObservedAt: fxObservedAt.UTC(), USDAmount: usdAmount, OccurredAt: input.OccurredAt.UTC(),
		ReferenceNo: referenceNo, Note: note, OperatorID: input.OperatorID, IdempotencyKey: idempotencyKey,
		BonusStatus: "not_applicable", ReversedEventID: input.ReversedEventID,
	}
	if err = s.applyRechargeBonus(ctx, event, input); err != nil {
		return nil, false, err
	}
	requested := *event
	var created bool
	atomicOpening := false
	if event.EventType == "opening_balance" && s.balances != nil {
		if atomicRepo, ok := s.repo.(UpstreamFundOpeningBalanceRepository); ok {
			atomicOpening = true
			created, err = atomicRepo.CreateFundEventWithOpeningBalance(ctx, event)
		} else {
			created, err = s.repo.CreateFundEvent(ctx, event)
		}
	} else {
		created, err = s.repo.CreateFundEvent(ctx, event)
	}
	if err != nil {
		return nil, false, err
	}
	if !created && !sameUpstreamFundEvent(&requested, event) {
		return nil, false, ErrUpstreamFundIdempotencyConflict
	}
	if event.EventType == "opening_balance" && s.balances != nil && !atomicOpening {
		if err := s.balances.RecordOpeningBalance(ctx, walletID, event.OriginalAmount, event.Currency, event.OccurredAt, "opening-balance-event-"+strconv.FormatInt(event.ID, 10)); err != nil {
			return nil, created, err
		}
	}
	return event, created, nil
}

func (s *UpstreamFundService) applyRechargeBonus(ctx context.Context, event *UpstreamFundEvent, input UpstreamFundEventInput) error {
	if event.EventType == "topup" {
		baseRaw := strings.TrimSpace(input.BaseCreditUnits)
		bonusRaw := strings.TrimSpace(input.BonusCreditUnits)
		if baseRaw == "" {
			if bonusRaw != "" {
				return financeValidationError("base_credit_units is required when bonus_credit_units is provided")
			}
			event.BonusStatus = "unresolved"
			return nil
		}
		base, err := decimal.NewFromString(baseRaw)
		if err != nil || base.LessThanOrEqual(decimal.Zero) {
			return financeValidationError("base_credit_units must be a positive decimal string")
		}
		bonus := decimal.Zero
		if bonusRaw != "" {
			bonus, err = decimal.NewFromString(bonusRaw)
			if err != nil || bonus.IsNegative() {
				return financeValidationError("bonus_credit_units must be a non-negative decimal string")
			}
		}
		total := base.Add(bonus)
		baseUnitValue := event.OriginalAmount.Div(base)
		baseRatio := base.Div(event.OriginalAmount)
		effectiveRatio := total.Div(event.OriginalAmount)
		bonusOriginal := bonus.Mul(baseUnitValue)
		bonusUSD := bonusOriginal.Mul(event.FXRateToUSD)
		event.BaseCreditUnits = decimalPointer(base)
		event.BonusCreditUnits = decimalPointer(bonus)
		event.TotalCreditUnits = decimalPointer(total)
		event.BaseRechargeRatio = decimalPointer(baseRatio)
		event.EffectiveRechargeRatio = decimalPointer(effectiveRatio)
		event.BonusIncomeOriginal = decimalPointer(bonusOriginal)
		event.BonusIncomeUSD = decimalPointer(bonusUSD)
		event.BonusStatus = "confirmed"
		return nil
	}
	if event.ReversedEventID == nil {
		return nil
	}
	if event.EventType != "refund" {
		return financeValidationError("reversed_event_id is only allowed for refund events")
	}
	original, err := s.repo.GetFundEvent(ctx, event.WalletID, *event.ReversedEventID)
	if err != nil {
		return err
	}
	if original.EventType != "topup" || original.BonusStatus != "confirmed" {
		return financeValidationError("reversed_event_id must reference a confirmed topup")
	}
	if !event.OriginalAmount.Equal(original.OriginalAmount) {
		return financeValidationError("refund original_amount must equal the reversed topup amount")
	}
	if event.Currency != original.Currency {
		return financeValidationError("refund currency must equal the reversed topup currency")
	}
	event.BaseCreditUnits = cloneDecimal(original.BaseCreditUnits)
	event.BonusCreditUnits = cloneDecimal(original.BonusCreditUnits)
	event.TotalCreditUnits = cloneDecimal(original.TotalCreditUnits)
	event.BaseRechargeRatio = cloneDecimal(original.BaseRechargeRatio)
	event.EffectiveRechargeRatio = cloneDecimal(original.EffectiveRechargeRatio)
	if original.BonusIncomeOriginal != nil {
		event.BonusIncomeOriginal = decimalPointer(original.BonusIncomeOriginal.Neg())
	}
	if original.BonusIncomeUSD != nil {
		event.BonusIncomeUSD = decimalPointer(original.BonusIncomeUSD.Neg())
	}
	event.BonusStatus = "reversed"
	return nil
}

func decimalPointer(value decimal.Decimal) *decimal.Decimal {
	copyValue := value
	return &copyValue
}

func sameUpstreamFundEvent(requested, existing *UpstreamFundEvent) bool {
	if requested == nil || existing == nil {
		return false
	}
	return requested.WalletID == existing.WalletID &&
		requested.EventType == existing.EventType &&
		requested.OriginalAmount.Equal(existing.OriginalAmount) &&
		requested.Currency == existing.Currency &&
		requested.FXRateToUSD.Equal(existing.FXRateToUSD) &&
		sameOptionalInt64(requested.FXRateVersionID, existing.FXRateVersionID) &&
		requested.FXSource == existing.FXSource &&
		requested.FXObservedAt.Equal(existing.FXObservedAt) &&
		requested.USDAmount.Equal(existing.USDAmount) &&
		sameOptionalDecimal(requested.BaseCreditUnits, existing.BaseCreditUnits) &&
		sameOptionalDecimal(requested.BonusCreditUnits, existing.BonusCreditUnits) &&
		sameOptionalDecimal(requested.TotalCreditUnits, existing.TotalCreditUnits) &&
		sameOptionalDecimal(requested.BaseRechargeRatio, existing.BaseRechargeRatio) &&
		sameOptionalDecimal(requested.EffectiveRechargeRatio, existing.EffectiveRechargeRatio) &&
		sameOptionalDecimal(requested.BonusIncomeOriginal, existing.BonusIncomeOriginal) &&
		sameOptionalDecimal(requested.BonusIncomeUSD, existing.BonusIncomeUSD) &&
		requested.BonusStatus == existing.BonusStatus &&
		sameOptionalInt64(requested.ReversedEventID, existing.ReversedEventID) &&
		requested.OccurredAt.Equal(existing.OccurredAt) &&
		requested.ReferenceNo == existing.ReferenceNo &&
		requested.Note == existing.Note &&
		sameOptionalInt64(requested.OperatorID, existing.OperatorID)
}

func sameOptionalDecimal(left, right *decimal.Decimal) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func sameOptionalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
