package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// protocolUpstreamFinanceAdapter executes one immutable protocol version bound
// to a wallet. It never resolves the protocol's mutable current_version_id.
type protocolUpstreamFinanceAdapter struct {
	versionID int64
	source    string
	config    FinanceProtocolConfig
	executor  *UpstreamFinanceHTTPExecutor
}

// UpstreamFinanceAccountUsage is the protocol-neutral cumulative account cost
// fact consumed by AccountFinanceSnapshotService.
type UpstreamFinanceAccountUsage struct {
	ListCostTotal      *decimal.Decimal
	ActualCostTotal    *decimal.Decimal
	UnitCode           string
	UnitSemantics      string
	Currency           *string
	UpstreamCounterID  *string
	CounterPeriod      *string
	UpstreamObservedAt *time.Time
	CollectedAt        time.Time
	SafeSnapshot       map[string]any
	SnapshotChecksum   string
}

type UpstreamFinanceAccountUsageAdapter interface {
	FetchAccountUsage(ctx context.Context, wallet UpstreamWallet, credential string) (*UpstreamFinanceAccountUsage, error)
	AccountUsageCredentialSource() string
	AccountUsageCounterScope() string
	SupportsAccountUsage() bool
}

func NewProtocolUpstreamFinanceAdapter(versionID int64, source string, config FinanceProtocolConfig, executor *UpstreamFinanceHTTPExecutor) UpstreamFinanceAdapter {
	return &protocolUpstreamFinanceAdapter{versionID: versionID, source: strings.TrimSpace(source), config: config, executor: executor}
}

func (a *protocolUpstreamFinanceAdapter) Probe(ctx context.Context, wallet UpstreamWallet, credential string) (UpstreamFinanceProbe, error) {
	started := time.Now()
	probe := newFinanceProbe(UpstreamAdapterProtocol, started)
	probe.Capabilities = UpstreamFinanceCapabilities{
		Pricing:       a.capabilityState(FinanceCapabilityPricing, credential),
		WalletBalance: a.capabilityState(FinanceCapabilityBalance, credential),
		TokenQuota:    a.capabilityState(FinanceCapabilityQuota, credential),
	}
	for _, capability := range []string{FinanceCapabilityPricing, FinanceCapabilityBalance, FinanceCapabilityQuota} {
		if a.capabilityState(capability, credential) != FinanceCapabilitySupported {
			continue
		}
		_, err := a.execute(ctx, capability, wallet, credential)
		probe.Reachable = err == nil
		if err != nil {
			probe.ErrorSummary = sanitizeFinanceProtocolError(err)
		}
		probe.LatencyMS = time.Since(started).Milliseconds()
		return probe, nil
	}
	probe.LatencyMS = time.Since(started).Milliseconds()
	return probe, nil
}

func (a *protocolUpstreamFinanceAdapter) FetchPricing(ctx context.Context, wallet UpstreamWallet, credential string) ([]UpstreamFinancePrice, error) {
	result, err := a.execute(ctx, FinanceCapabilityPricing, wallet, credential)
	if err != nil {
		return nil, err
	}
	rows := financeProtocolFactRows(result.Facts, "prices", "models")
	prices := make([]UpstreamFinancePrice, 0, len(rows))
	for index, row := range rows {
		model := financeProtocolString(row, "model_pattern", "model", "model_name")
		if model == "" {
			return nil, fmt.Errorf("protocol pricing row %d has no model_pattern", index+1)
		}
		detail := financeProtocolPriceDetail(row)
		if len(detail) == 0 {
			return nil, fmt.Errorf("protocol pricing row %d has no price detail", index+1)
		}
		if _, parseErr := FinancePriceDetailFromMap(detail); parseErr != nil {
			return nil, fmt.Errorf("protocol pricing row %d: %w", index+1, parseErr)
		}
		currency := strings.ToUpper(financeProtocolString(row, "currency"))
		if currency == "" {
			currency = wallet.Currency
		}
		if len(currency) != 3 {
			return nil, fmt.Errorf("protocol pricing row %d has invalid currency", index+1)
		}
		effectiveAt := financeProtocolTime(row, "effective_at")
		prices = append(prices, UpstreamFinancePrice{
			ModelPattern:   model,
			IsWildcard:     strings.HasSuffix(model, "*") || financeProtocolBool(row, "is_wildcard"),
			BillingMode:    normalizeFinanceBillingMode(financeProtocolString(row, "billing_mode")),
			ServiceTier:    financeProtocolString(row, "service_tier"),
			PriceDetail:    detail,
			Currency:       currency,
			Source:         FinancePricingSourceUpstreamCatalog,
			SourceSnapshot: a.sourceSnapshot(result),
			EffectiveAt:    effectiveAt,
		})
		if prices[len(prices)-1].BillingMode == "" {
			prices[len(prices)-1].BillingMode = "token"
		}
	}
	if len(prices) == 0 {
		return nil, errors.New("protocol pricing returned no usable rows")
	}
	return prices, nil
}

func (a *protocolUpstreamFinanceAdapter) FetchBalance(ctx context.Context, wallet UpstreamWallet, credential string) (*UpstreamFinanceBalance, error) {
	result, err := a.execute(ctx, FinanceCapabilityBalance, wallet, credential)
	if err != nil {
		return nil, err
	}
	amount, ok := financeProtocolDecimal(result.Facts, "balance_amount", "balance", "available_balance")
	if !ok {
		return nil, errors.New("protocol balance response has no balance amount")
	}
	balance := &UpstreamFinanceBalance{
		Currency: a.resultCurrency(result.Facts, wallet.Currency), Source: a.source,
		CollectedAt: financeProtocolCollectedAt(result.Facts), SafeSnapshot: a.sourceSnapshot(result),
	}
	switch strings.ToLower(strings.TrimSpace(result.UnitSemantics)) {
	case FinanceUnitFiatCurrency:
		balance.BalanceKind = "wallet_cash"
		balance.BalanceAmount = &amount
	case FinanceUnitPlatformCredit:
		balance.BalanceKind = "token_quota"
		balance.TotalQuota = &amount
	default:
		return nil, errors.New("protocol balance unit semantics is not classified")
	}
	return balance, nil
}

func (a *protocolUpstreamFinanceAdapter) FetchQuota(ctx context.Context, wallet UpstreamWallet, credential string) (*UpstreamFinanceBalance, error) {
	result, err := a.execute(ctx, FinanceCapabilityQuota, wallet, credential)
	if err != nil {
		return nil, err
	}
	total, totalOK := financeProtocolDecimal(result.Facts, "total_quota", "quota")
	used, usedOK := financeProtocolDecimal(result.Facts, "used_quota", "used")
	if !totalOK && !usedOK {
		return nil, errors.New("protocol quota response has no total or used quota")
	}
	quota := &UpstreamFinanceBalance{
		BalanceKind: "token_quota", Currency: a.resultCurrency(result.Facts, wallet.Currency), Source: a.source,
		CollectedAt: financeProtocolCollectedAt(result.Facts), SafeSnapshot: a.sourceSnapshot(result),
	}
	if totalOK {
		quota.TotalQuota = &total
	}
	if usedOK {
		quota.UsedQuota = &used
	}
	return quota, nil
}

func (a *protocolUpstreamFinanceAdapter) FetchFundingTransactions(ctx context.Context, wallet UpstreamWallet, credential string) ([]FinanceFundingTransactionFact, error) {
	result, err := a.execute(ctx, FinanceCapabilityFundingTransactions, wallet, credential)
	if err != nil {
		return nil, err
	}
	transactions, ok := result.Facts["transactions"].([]FinanceFundingTransactionFact)
	if !ok {
		return nil, errors.New("protocol funding transaction result is invalid")
	}
	return transactions, nil
}

func (a *protocolUpstreamFinanceAdapter) FetchAccountUsage(ctx context.Context, wallet UpstreamWallet, credential string) (*UpstreamFinanceAccountUsage, error) {
	result, err := a.execute(ctx, FinanceCapabilityAccountUsage, wallet, credential)
	if err != nil {
		return nil, err
	}
	listCost, listPresent, err := financeProtocolOptionalDecimal(result.Facts, "list_cost", "list_cost_total")
	if err != nil {
		return nil, err
	}
	actualCost, actualPresent, err := financeProtocolOptionalDecimal(result.Facts, "actual_cost", "actual_cost_total")
	if err != nil {
		return nil, err
	}
	if !listPresent && !actualPresent {
		return nil, errors.New("protocol account usage response has no cumulative cost")
	}
	semantics := strings.ToLower(strings.TrimSpace(result.UnitSemantics))
	if semantics != FinanceUnitFiatCurrency && semantics != FinanceUnitPlatformCredit {
		return nil, errors.New("protocol account usage unit semantics is not cost-bearing")
	}
	unitCode := strings.ToUpper(financeProtocolString(result.Facts, "unit_code", "unit", "currency"))
	if unitCode == "" {
		unitCode = strings.ToUpper(strings.TrimSpace(wallet.Currency))
	}
	if unitCode == "" || len(unitCode) > 30 {
		return nil, errors.New("protocol account usage unit code is invalid")
	}
	usage := &UpstreamFinanceAccountUsage{
		ListCostTotal: listCost, ActualCostTotal: actualCost,
		UnitCode: unitCode, UnitSemantics: semantics,
		UpstreamCounterID: optionalFinanceProtocolString(result.Facts, "upstream_counter_id", "counter_id"),
		CounterPeriod:     optionalFinanceProtocolString(result.Facts, "counter_period", "period"),
		CollectedAt:       financeProtocolCollectedAt(result.Facts), SafeSnapshot: a.sourceSnapshot(result),
		SnapshotChecksum: result.SnapshotChecksum,
	}
	if observedAt := financeProtocolTime(result.Facts, "upstream_observed_at"); !observedAt.IsZero() {
		usage.UpstreamObservedAt = &observedAt
	} else if observedAt = financeProtocolTime(result.Facts, "observed_at"); !observedAt.IsZero() {
		usage.UpstreamObservedAt = &observedAt
	}
	if semantics == FinanceUnitFiatCurrency {
		currency := strings.ToUpper(financeProtocolString(result.Facts, "currency"))
		if currency == "" {
			currency = strings.ToUpper(strings.TrimSpace(wallet.Currency))
		}
		if len(currency) != 3 {
			return nil, errors.New("protocol fiat account usage currency is invalid")
		}
		usage.Currency = &currency
	}
	return usage, nil
}

func (a *protocolUpstreamFinanceAdapter) AccountUsageCredentialSource() string {
	return strings.ToLower(strings.TrimSpace(a.config.Authentication.CredentialSource))
}

func (a *protocolUpstreamFinanceAdapter) AccountUsageCounterScope() string {
	return strings.ToLower(strings.TrimSpace(a.config.CounterScope))
}

func (a *protocolUpstreamFinanceAdapter) SupportsAccountUsage() bool {
	return a.supports(FinanceCapabilityAccountUsage)
}

func (a *protocolUpstreamFinanceAdapter) execute(ctx context.Context, capability string, wallet UpstreamWallet, credential string) (*FinanceProtocolExecutionResult, error) {
	if a == nil || a.executor == nil {
		return nil, errors.New("protocol finance executor is unavailable")
	}
	if !a.supports(capability) {
		return nil, ErrUpstreamFinanceCapabilityUnsupported
	}
	return a.executor.Execute(ctx, a.config, capability, wallet.BaseURL, credential)
}

func (a *protocolUpstreamFinanceAdapter) supports(capability string) bool {
	for _, configured := range a.config.Capabilities {
		if configured == capability {
			_, hasOperation := a.config.Operations[capability]
			return hasOperation
		}
	}
	return false
}

func (a *protocolUpstreamFinanceAdapter) capabilityState(capability, credential string) string {
	if !a.supports(capability) {
		return FinanceCapabilityUnsupported
	}
	if a.config.Authentication.Type != "none" && strings.TrimSpace(credential) == "" {
		return FinanceCapabilityRequiresCredential
	}
	return FinanceCapabilitySupported
}

func (a *protocolUpstreamFinanceAdapter) sourceSnapshot(result *FinanceProtocolExecutionResult) map[string]any {
	snapshot := map[string]any{
		"adapter": "protocol", "protocol": a.source, "protocol_version_id": a.versionID,
		"snapshot_checksum": result.SnapshotChecksum, "payload": result.SafeSnapshot,
	}
	for _, key := range []string{"fx_rate_version_id", "fx_rate_to_usd", "fx_source", "fx_observed_at"} {
		if value, ok := result.Facts[key]; ok {
			snapshot[key] = value
		}
	}
	return snapshot
}

func (a *protocolUpstreamFinanceAdapter) resultCurrency(facts map[string]any, fallback string) string {
	currency := strings.ToUpper(financeProtocolString(facts, "currency"))
	if len(currency) == 3 {
		return currency
	}
	return strings.ToUpper(fallback)
}

func financeProtocolOptionalDecimal(values map[string]any, keys ...string) (*decimal.Decimal, bool, error) {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		parsed, err := FinanceDecimal(value)
		if err != nil {
			return nil, true, fmt.Errorf("protocol account usage %s is invalid: %w", key, err)
		}
		return &parsed, true, nil
	}
	return nil, false, nil
}

func optionalFinanceProtocolString(values map[string]any, keys ...string) *string {
	value := financeProtocolString(values, keys...)
	if value == "" {
		return nil
	}
	return &value
}

func financeProtocolFactRows(facts map[string]any, collectionKeys ...string) []map[string]any {
	for _, key := range collectionKeys {
		if rows := financeProtocolMaps(facts[key]); len(rows) > 0 {
			return rows
		}
	}
	count := 1
	for _, value := range facts {
		if list, ok := value.([]any); ok && len(list) > count {
			count = len(list)
		}
	}
	rows := make([]map[string]any, count)
	for index := range rows {
		rows[index] = make(map[string]any, len(facts))
		for key, value := range facts {
			if list, ok := value.([]any); ok {
				if index < len(list) {
					rows[index][key] = list[index]
				}
				continue
			}
			rows[index][key] = value
		}
	}
	return rows
}

func financeProtocolMaps(value any) []map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return []map[string]any{typed}
	case []any:
		rows := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
		return rows
	default:
		return nil
	}
}

func financeProtocolPriceDetail(row map[string]any) map[string]any {
	for _, key := range []string{"price_detail", "prices"} {
		if detail, ok := row[key].(map[string]any); ok {
			return detail
		}
	}
	detail := map[string]any{}
	for _, key := range []string{"input", "output", "cache_read", "cache_write_5m", "cache_write_1h", "image_output", "per_request", "per_image", "per_second", "fast", "tiers"} {
		if value, ok := row[key]; ok {
			detail[key] = value
		}
	}
	return detail
}

func financeProtocolString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok && value != nil {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func financeProtocolBool(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func financeProtocolDecimal(values map[string]any, keys ...string) (decimal.Decimal, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			parsed, err := FinanceDecimal(value)
			if err == nil {
				return parsed, true
			}
		}
	}
	return decimal.Zero, false
}

func financeProtocolTime(values map[string]any, key string) time.Time {
	text := financeProtocolString(values, key)
	if text == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func financeProtocolCollectedAt(values map[string]any) time.Time {
	if parsed := financeProtocolTime(values, "collected_at"); !parsed.IsZero() {
		return parsed
	}
	return time.Now().UTC()
}
