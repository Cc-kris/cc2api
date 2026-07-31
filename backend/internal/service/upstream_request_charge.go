package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const financeBillingPayloadMaxBytes = 256 * 1024

type FinanceRequestCharge struct {
	ActualCharge           decimal.Decimal
	ActualChargeUSD        decimal.Decimal
	USDConversionAvailable bool
	StandardCharge         *decimal.Decimal
	Currency               string
	UnitSemantics          string
	BillingRequestID       string
	ObservedAt             time.Time
	SafeSnapshot           map[string]any
}

func ExtractFinanceRequestCharge(payload []byte, config FinanceProtocolConfig) (*FinanceRequestCharge, error) {
	if config.CostMode != FinanceCostModeRequestCharge {
		return nil, ErrUpstreamFinanceProtocolUnsupported
	}
	operation, ok := config.Operations[FinanceCapabilityRequestCharge]
	if !ok {
		return nil, ErrUpstreamFinanceProtocolUnsupported
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var body any
	decodeErr := errors.New("request charge payload is not valid JSON")
	if len(payload) > 0 {
		decodeErr = decoder.Decode(&body)
	}
	if decodeErr != nil {
		body, decodeErr = parseFinanceSSEEvent(payload, strings.TrimSpace(operation.SSEEvent))
	}
	if decodeErr != nil {
		return nil, errors.New("request charge payload is not valid JSON or configured SSE event")
	}
	facts, err := mapFinanceProtocolFacts(body, operation.Mapping)
	if err != nil {
		return nil, err
	}
	actual, ok := financeProtocolDecimal(facts, "actual_cost", "actual_charge")
	if !ok || actual.IsNegative() {
		return nil, errors.New("request charge actual_cost is missing or invalid")
	}
	semantics := strings.ToLower(strings.TrimSpace(config.UnitSemantics))
	currency := strings.ToUpper(financeProtocolString(facts, "currency", "unit_code", "unit"))
	if currency == "" {
		return nil, errors.New("request charge currency or unit_code is required")
	}
	if semantics == FinanceUnitFiatCurrency && !validFinanceCurrency(currency) {
		return nil, errors.New("request charge currency is invalid")
	}
	if semantics != FinanceUnitFiatCurrency && semantics != FinanceUnitPlatformCredit {
		return nil, errors.New("request charge requires fiat_currency or platform_credit unit semantics")
	}
	rate := decimal.NewFromInt(1)
	conversionAvailable := false
	if semantics == FinanceUnitFiatCurrency && currency == "USD" {
		conversionAvailable = true
	} else if parsedRate, rateOK := financeProtocolDecimal(facts, "fx_rate_to_usd", "exchange_rate_to_usd"); rateOK && parsedRate.IsPositive() {
		rate = parsedRate
		conversionAvailable = true
	} else if semantics == FinanceUnitFiatCurrency {
		return nil, errors.New("request charge fx_rate_to_usd is required for non-USD currency")
	}
	var standard *decimal.Decimal
	if value, present := financeProtocolDecimal(facts, "standard_cost", "list_cost"); present && !value.IsNegative() {
		standard = &value
	}
	observedAt := financeProtocolTime(facts, "observed_at")
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	snapshot, err := redactFinanceProtocolSnapshot(body, config.RedactPaths)
	if err != nil {
		return nil, err
	}
	safe, _ := snapshot.(map[string]any)
	return &FinanceRequestCharge{
		ActualCharge: actual, ActualChargeUSD: actual.Mul(rate), USDConversionAvailable: conversionAvailable, StandardCharge: standard, Currency: currency,
		UnitSemantics: semantics, BillingRequestID: financeProtocolString(facts, "billing_request_id", "request_id"),
		ObservedAt: observedAt.UTC(), SafeSnapshot: safe,
	}, nil
}

func appendFinanceBillingPayload(current, next []byte) []byte {
	if len(next) == 0 {
		return current
	}
	if len(next) >= financeBillingPayloadMaxBytes {
		return append([]byte(nil), next[len(next)-financeBillingPayloadMaxBytes:]...)
	}
	if overflow := len(current) + len(next) - financeBillingPayloadMaxBytes; overflow > 0 {
		if overflow >= len(current) {
			current = current[:0]
		} else {
			current = current[overflow:]
		}
	}
	return append(current, next...)
}

func ApplyFinanceRequestChargeToAttempt(attempt *UsageUpstreamAttempt, payload []byte, account *Account) error {
	if attempt == nil || account == nil || account.FinanceCostMode != FinanceCostModeRequestCharge || account.FinanceProtocolConfig == nil {
		return nil
	}
	charge, err := ExtractFinanceRequestCharge(payload, *account.FinanceProtocolConfig)
	if err != nil {
		return err
	}
	ApplyFinanceRequestChargeToUsageAttempt(attempt, charge)
	return nil
}

func ApplyFinanceRequestChargeToUsageAttempt(attempt *UsageUpstreamAttempt, charge *FinanceRequestCharge) {
	if attempt == nil || charge == nil {
		return
	}
	attempt.UpstreamActualCharge = cloneDecimal(&charge.ActualCharge)
	if charge.USDConversionAvailable {
		attempt.UpstreamActualChargeUSD = cloneDecimal(&charge.ActualChargeUSD)
	} else {
		attempt.UpstreamActualChargeUSD = nil
	}
	attempt.UpstreamStandardCharge = cloneDecimal(charge.StandardCharge)
	attempt.UpstreamChargeCurrency = charge.Currency
	attempt.UpstreamChargeUnitSemantics = charge.UnitSemantics
	attempt.UpstreamBillingRequestID = charge.BillingRequestID
	attempt.UpstreamChargeSnapshot = cloneFinanceSnapshot(charge.SafeSnapshot)
	attempt.BillingObservedAt = cloneFinanceTime(&charge.ObservedAt)
}

func cloneFinanceSnapshot(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneFinanceSnapshotValue(value)
	}
	return cloned
}

func cloneFinanceSnapshotValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneFinanceSnapshot(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index := range typed {
			cloned[index] = cloneFinanceSnapshotValue(typed[index])
		}
		return cloned
	default:
		return typed
	}
}
