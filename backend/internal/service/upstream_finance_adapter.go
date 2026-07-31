package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/shopspring/decimal"
)

const (
	FinanceCapabilitySupported          = "supported"
	FinanceCapabilityUnsupported        = "unsupported"
	FinanceCapabilityRequiresCredential = "requires_credential"
	financeResponseMaxBytes             = int64(1 << 20)
	financeHTTPTimeout                  = 10 * time.Second
	financeHTTPMaxRedirects             = 2
)

var ErrUpstreamFinanceCapabilityUnsupported = errors.New("upstream finance capability is unsupported")

type UpstreamFinanceHTTPStatusError struct {
	StatusCode int
}

func (e *UpstreamFinanceHTTPStatusError) Error() string {
	return fmt.Sprintf("upstream finance endpoint returned HTTP %d", e.StatusCode)
}

type UpstreamFinanceCapabilities struct {
	Pricing       string `json:"pricing"`
	WalletBalance string `json:"wallet_balance"`
	TokenQuota    string `json:"token_quota"`
}

type UpstreamFinanceProbe struct {
	Reachable    bool                        `json:"reachable"`
	AdapterType  string                      `json:"adapter_type"`
	Capabilities UpstreamFinanceCapabilities `json:"capabilities"`
	LatencyMS    int64                       `json:"latency_ms"`
	ProbedAt     time.Time                   `json:"probed_at"`
	ErrorSummary string                      `json:"error_summary,omitempty"`
}

type UpstreamFinancePrice struct {
	ModelPattern   string         `json:"model_pattern"`
	IsWildcard     bool           `json:"is_wildcard"`
	BillingMode    string         `json:"billing_mode"`
	ServiceTier    string         `json:"service_tier,omitempty"`
	PriceDetail    map[string]any `json:"price_detail"`
	Currency       string         `json:"currency"`
	Source         string         `json:"source"`
	SourceSnapshot map[string]any `json:"source_snapshot"`
	EffectiveAt    time.Time      `json:"effective_at,omitempty"`
}

type UpstreamFinanceBalance struct {
	BalanceKind   string
	BalanceAmount *decimal.Decimal
	TotalQuota    *decimal.Decimal
	UsedQuota     *decimal.Decimal
	Currency      string
	Source        string
	CollectedAt   time.Time
	SafeSnapshot  map[string]any
}

type UpstreamFinanceAdapter interface {
	Probe(ctx context.Context, wallet UpstreamWallet, credential string) (UpstreamFinanceProbe, error)
	FetchPricing(ctx context.Context, wallet UpstreamWallet, credential string) ([]UpstreamFinancePrice, error)
	FetchBalance(ctx context.Context, wallet UpstreamWallet, credential string) (*UpstreamFinanceBalance, error)
	FetchQuota(ctx context.Context, wallet UpstreamWallet, credential string) (*UpstreamFinanceBalance, error)
}

// UpstreamFinanceFundingAdapter is optional because legacy adapters do not
// expose recharge transactions. Protocol-backed wallets implement it when the
// immutable protocol version declares funding_transactions.
type UpstreamFinanceFundingAdapter interface {
	FetchFundingTransactions(ctx context.Context, wallet UpstreamWallet, credential string) ([]FinanceFundingTransactionFact, error)
}

type FinanceHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type UpstreamFinanceAdapterRegistry struct {
	newAPI            UpstreamFinanceAdapter
	legacy            UpstreamFinanceAdapter
	manual            UpstreamFinanceAdapter
	protocolExecutors map[string]*UpstreamFinanceHTTPExecutor
}

func NewUpstreamFinanceAdapterRegistry() *UpstreamFinanceAdapterRegistry {
	client := newUpstreamFinanceHTTPClient()
	return &UpstreamFinanceAdapterRegistry{
		newAPI:            NewNewAPIUpstreamFinanceAdapter(client),
		legacy:            NewLegacyBillingFinanceAdapter(client),
		manual:            NewManualUpstreamFinanceAdapter(),
		protocolExecutors: make(map[string]*UpstreamFinanceHTTPExecutor),
	}
}

// RegisterProtocolExecutor registers an executor family, not an upstream-site enum.
func (r *UpstreamFinanceAdapterRegistry) RegisterProtocolExecutor(protocolType string, executor *UpstreamFinanceHTTPExecutor) {
	if r == nil || executor == nil {
		return
	}
	if r.protocolExecutors == nil {
		r.protocolExecutors = make(map[string]*UpstreamFinanceHTTPExecutor)
	}
	r.protocolExecutors[strings.ToLower(strings.TrimSpace(protocolType))] = executor
}

func (r *UpstreamFinanceAdapterRegistry) ProtocolExecutor(protocolType string) (*UpstreamFinanceHTTPExecutor, error) {
	if r != nil {
		if executor := r.protocolExecutors[strings.ToLower(strings.TrimSpace(protocolType))]; executor != nil {
			return executor, nil
		}
	}
	return nil, fmt.Errorf("unknown upstream finance protocol executor %q", protocolType)
}

func (r *UpstreamFinanceAdapterRegistry) Adapter(adapterType string) (UpstreamFinanceAdapter, error) {
	switch strings.ToLower(strings.TrimSpace(adapterType)) {
	case UpstreamAdapterNewAPI:
		return r.newAPI, nil
	case UpstreamAdapterLegacyOpenAIBilling:
		return r.legacy, nil
	case UpstreamAdapterManual:
		return r.manual, nil
	default:
		return nil, fmt.Errorf("unknown upstream finance adapter %q", adapterType)
	}
}

type newAPIUpstreamFinanceAdapter struct{ client FinanceHTTPDoer }

func NewNewAPIUpstreamFinanceAdapter(client FinanceHTTPDoer) UpstreamFinanceAdapter {
	return &newAPIUpstreamFinanceAdapter{client: client}
}

func (a *newAPIUpstreamFinanceAdapter) Probe(ctx context.Context, wallet UpstreamWallet, credential string) (UpstreamFinanceProbe, error) {
	started := time.Now()
	result := newFinanceProbe(wallet.AdapterType, started)
	_, statusCode, statusErr := financeGetJSON(ctx, a.client, wallet.BaseURL, "/api/status", "")
	_, pricingCode, pricingErr := financeGetJSON(ctx, a.client, wallet.BaseURL, "/api/pricing", "")
	result.Reachable = statusErr == nil || pricingErr == nil
	if pricingErr == nil && pricingCode >= 200 && pricingCode < 300 {
		result.Capabilities.Pricing = FinanceCapabilitySupported
	}
	if credential == "" {
		result.Capabilities.WalletBalance = FinanceCapabilityRequiresCredential
		result.Capabilities.TokenQuota = FinanceCapabilityRequiresCredential
	} else {
		profile, _, profileErr := financeGetJSON(ctx, a.client, wallet.BaseURL, "/api/user/self", credential)
		if profileErr == nil {
			if _, _, ok := extractExplicitCashBalance(profile); ok {
				result.Capabilities.WalletBalance = FinanceCapabilitySupported
			}
			if _, ok := financeDecimalAt(profile, "data", "quota"); ok {
				result.Capabilities.TokenQuota = FinanceCapabilitySupported
			}
		}
	}
	if !result.Reachable {
		result.ErrorSummary = firstFinanceErrorSummary(statusCode, statusErr, pricingCode, pricingErr)
	}
	result.LatencyMS = time.Since(started).Milliseconds()
	return result, nil
}

func (a *newAPIUpstreamFinanceAdapter) FetchPricing(ctx context.Context, wallet UpstreamWallet, _ string) ([]UpstreamFinancePrice, error) {
	if strings.TrimSpace(wallet.PricingGroup) == "" {
		return nil, errors.New("pricing_group is required before NewAPI pricing sync")
	}
	status, _, err := financeGetJSON(ctx, a.client, wallet.BaseURL, "/api/status", "")
	if err != nil {
		return nil, err
	}
	pricing, _, err := financeGetJSON(ctx, a.client, wallet.BaseURL, "/api/pricing", "")
	if err != nil {
		return nil, err
	}
	return parseNewAPIPrices(status, pricing, wallet.PricingGroup)
}

func (a *newAPIUpstreamFinanceAdapter) FetchBalance(ctx context.Context, wallet UpstreamWallet, credential string) (*UpstreamFinanceBalance, error) {
	if credential == "" {
		return nil, errors.New("NewAPI user-center credential is required")
	}
	profile, _, err := financeGetJSON(ctx, a.client, wallet.BaseURL, "/api/user/self", credential)
	if err != nil {
		return nil, err
	}
	amount, currency, ok := extractExplicitCashBalance(profile)
	if !ok {
		return nil, fmt.Errorf("%w: /api/user/self did not return explicit cash balance and currency", ErrUpstreamFinanceCapabilityUnsupported)
	}
	return &UpstreamFinanceBalance{
		BalanceKind: "wallet_cash", BalanceAmount: &amount, Currency: currency,
		Source: "newapi_user", CollectedAt: time.Now().UTC(),
		SafeSnapshot: map[string]any{"kind": "wallet_cash", "currency": currency},
	}, nil
}

func (a *newAPIUpstreamFinanceAdapter) FetchQuota(ctx context.Context, wallet UpstreamWallet, credential string) (*UpstreamFinanceBalance, error) {
	if credential == "" {
		return nil, errors.New("NewAPI user-center credential is required")
	}
	status, _, err := financeGetJSON(ctx, a.client, wallet.BaseURL, "/api/status", "")
	if err != nil {
		return nil, err
	}
	profile, _, err := financeGetJSON(ctx, a.client, wallet.BaseURL, "/api/user/self", credential)
	if err != nil {
		return nil, err
	}
	quotaPerUnit, ok := financeDecimalAt(status, "data", "quota_per_unit")
	if !ok || quotaPerUnit.LessThanOrEqual(decimal.Zero) {
		quotaPerUnit, ok = financeDecimalAt(status, "quota_per_unit")
	}
	if !ok || quotaPerUnit.LessThanOrEqual(decimal.Zero) {
		return nil, errors.New("NewAPI status did not provide quota_per_unit")
	}
	remaining, ok := financeDecimalAt(profile, "data", "quota")
	if !ok {
		return nil, fmt.Errorf("%w: /api/user/self did not return quota", ErrUpstreamFinanceCapabilityUnsupported)
	}
	used, _ := financeDecimalAt(profile, "data", "used_quota")
	remaining = remaining.Div(quotaPerUnit)
	used = used.Div(quotaPerUnit)
	total := remaining.Add(used)
	return &UpstreamFinanceBalance{
		BalanceKind: "token_quota", TotalQuota: &total, UsedQuota: &used, Currency: "USD",
		Source: "newapi_user", CollectedAt: time.Now().UTC(),
		SafeSnapshot: map[string]any{"kind": "token_quota", "quota_per_unit": quotaPerUnit.String()},
	}, nil
}

type legacyBillingFinanceAdapter struct{ client FinanceHTTPDoer }

func NewLegacyBillingFinanceAdapter(client FinanceHTTPDoer) UpstreamFinanceAdapter {
	return &legacyBillingFinanceAdapter{client: client}
}

func (a *legacyBillingFinanceAdapter) Probe(ctx context.Context, wallet UpstreamWallet, credential string) (UpstreamFinanceProbe, error) {
	started := time.Now()
	result := newFinanceProbe(wallet.AdapterType, started)
	if credential == "" {
		result.Capabilities.TokenQuota = FinanceCapabilityRequiresCredential
		result.LatencyMS = time.Since(started).Milliseconds()
		return result, nil
	}
	_, status, err := financeGetJSON(ctx, a.client, wallet.BaseURL, "/dashboard/billing/subscription", credential)
	result.Reachable = err == nil
	if err == nil {
		result.Capabilities.TokenQuota = FinanceCapabilitySupported
	} else {
		result.ErrorSummary = financeHTTPErrorSummary(status, err)
	}
	result.LatencyMS = time.Since(started).Milliseconds()
	return result, nil
}

func (a *legacyBillingFinanceAdapter) FetchPricing(context.Context, UpstreamWallet, string) ([]UpstreamFinancePrice, error) {
	return nil, ErrUpstreamFinanceCapabilityUnsupported
}

func (a *legacyBillingFinanceAdapter) FetchBalance(context.Context, UpstreamWallet, string) (*UpstreamFinanceBalance, error) {
	return nil, ErrUpstreamFinanceCapabilityUnsupported
}

func (a *legacyBillingFinanceAdapter) FetchQuota(ctx context.Context, wallet UpstreamWallet, credential string) (*UpstreamFinanceBalance, error) {
	if credential == "" {
		return nil, errors.New("Legacy Billing credential is required")
	}
	subscription, _, err := financeGetJSON(ctx, a.client, wallet.BaseURL, "/dashboard/billing/subscription", credential)
	if err != nil {
		return nil, err
	}
	end := time.Now().UTC()
	start := end.AddDate(0, -1, 0)
	path := "/dashboard/billing/usage?start_date=" + start.Format("2006-01-02") + "&end_date=" + end.Format("2006-01-02")
	usage, _, err := financeGetJSON(ctx, a.client, wallet.BaseURL, path, credential)
	if err != nil {
		return nil, err
	}
	hardLimit, ok := financeDecimalAt(subscription, "hard_limit_usd")
	if !ok {
		return nil, errors.New("Legacy Billing subscription did not return hard_limit_usd")
	}
	totalUsage, ok := financeDecimalAt(usage, "total_usage")
	if !ok {
		return nil, errors.New("Legacy Billing usage did not return total_usage")
	}
	used := totalUsage.Div(decimal.NewFromInt(100))
	return &UpstreamFinanceBalance{
		BalanceKind: "token_quota", TotalQuota: &hardLimit, UsedQuota: &used, Currency: "USD",
		Source: "legacy_openai", CollectedAt: end,
		SafeSnapshot: map[string]any{"kind": "token_quota", "hard_limit_usd": hardLimit.String(), "total_usage_cents": totalUsage.String()},
	}, nil
}

type manualUpstreamFinanceAdapter struct{}

func NewManualUpstreamFinanceAdapter() UpstreamFinanceAdapter { return &manualUpstreamFinanceAdapter{} }

func (a *manualUpstreamFinanceAdapter) Probe(_ context.Context, wallet UpstreamWallet, _ string) (UpstreamFinanceProbe, error) {
	return newFinanceProbe(wallet.AdapterType, time.Now()), nil
}
func (a *manualUpstreamFinanceAdapter) FetchPricing(context.Context, UpstreamWallet, string) ([]UpstreamFinancePrice, error) {
	return nil, ErrUpstreamFinanceCapabilityUnsupported
}
func (a *manualUpstreamFinanceAdapter) FetchBalance(context.Context, UpstreamWallet, string) (*UpstreamFinanceBalance, error) {
	return nil, ErrUpstreamFinanceCapabilityUnsupported
}
func (a *manualUpstreamFinanceAdapter) FetchQuota(context.Context, UpstreamWallet, string) (*UpstreamFinanceBalance, error) {
	return nil, ErrUpstreamFinanceCapabilityUnsupported
}

func newFinanceProbe(adapter string, started time.Time) UpstreamFinanceProbe {
	return UpstreamFinanceProbe{
		AdapterType: adapter, ProbedAt: started.UTC(),
		Capabilities: UpstreamFinanceCapabilities{
			Pricing: FinanceCapabilityUnsupported, WalletBalance: FinanceCapabilityUnsupported, TokenQuota: FinanceCapabilityUnsupported,
		},
	}
}

func parseNewAPIPrices(status, pricing map[string]any, pricingGroup string) ([]UpstreamFinancePrice, error) {
	quotaPerUnit, ok := financeDecimalAt(status, "data", "quota_per_unit")
	if !ok {
		quotaPerUnit, ok = financeDecimalAt(status, "quota_per_unit")
	}
	if !ok || quotaPerUnit.LessThanOrEqual(decimal.Zero) {
		return nil, errors.New("NewAPI status did not provide a positive quota_per_unit")
	}
	groupRatio, ok := financeDecimalAt(pricing, "group_ratio", pricingGroup)
	if !ok {
		return nil, fmt.Errorf("pricing_group %q is not present in NewAPI pricing", pricingGroup)
	}
	data, ok := pricing["data"].([]any)
	if !ok {
		return nil, errors.New("NewAPI pricing data must be an array")
	}
	prices := make([]UpstreamFinancePrice, 0, len(data))
	for _, raw := range data {
		model, ok := raw.(map[string]any)
		if !ok || !newAPIModelEnabledForGroup(model, pricingGroup) {
			continue
		}
		name := strings.TrimSpace(fmt.Sprint(model["model_name"]))
		if name == "" || name == "<nil>" {
			continue
		}
		quotaType, _ := financeDecimalValue(model["quota_type"])
		price := UpstreamFinancePrice{
			ModelPattern: name, BillingMode: "token", Currency: "USD", Source: FinancePricingSourceUpstreamCatalog,
			SourceSnapshot: map[string]any{"adapter": "newapi", "pricing_group": pricingGroup, "quota_per_unit": quotaPerUnit.String(), "group_ratio": groupRatio.String()},
		}
		if quotaType.Equal(decimal.NewFromInt(1)) {
			modelPrice, exists := financeDecimalValue(model["model_price"])
			if !exists {
				continue
			}
			perRequest := modelPrice.Mul(groupRatio).Div(quotaPerUnit)
			price.BillingMode = "per_request"
			price.PriceDetail = map[string]any{"per_request": perRequest.String()}
		} else {
			modelRatio, exists := financeDecimalValue(model["model_ratio"])
			if !exists {
				continue
			}
			completionRatio, exists := financeDecimalValue(model["completion_ratio"])
			if !exists {
				completionRatio = decimal.NewFromInt(1)
			}
			input := modelRatio.Mul(groupRatio).Div(quotaPerUnit).Mul(decimal.NewFromInt(1_000_000))
			detail := map[string]any{
				"input":  input.String(),
				"output": input.Mul(completionRatio).String(),
			}
			if ratio, exists := financeDecimalValue(model["cache_ratio"]); exists {
				detail["cache_read"] = input.Mul(ratio).String()
			}
			if ratio, exists := financeDecimalValue(model["create_cache_ratio"]); exists {
				detail["cache_write_5m"] = input.Mul(ratio).String()
			}
			price.PriceDetail = detail
		}
		prices = append(prices, price)
	}
	if len(prices) == 0 {
		return nil, errors.New("NewAPI pricing returned no usable models for selected group")
	}
	return prices, nil
}

func newAPIModelEnabledForGroup(model map[string]any, group string) bool {
	groups, ok := model["enable_groups"].([]any)
	if !ok || len(groups) == 0 {
		return true
	}
	for _, value := range groups {
		candidate := strings.TrimSpace(fmt.Sprint(value))
		if candidate == "all" || candidate == group {
			return true
		}
	}
	return false
}

func extractExplicitCashBalance(profile map[string]any) (decimal.Decimal, string, bool) {
	data, ok := profile["data"].(map[string]any)
	if !ok {
		return decimal.Zero, "", false
	}
	currency := strings.ToUpper(strings.TrimSpace(fmt.Sprint(data["currency"])))
	if len(currency) != 3 {
		return decimal.Zero, "", false
	}
	for _, key := range []string{"cash_balance", "balance"} {
		if amount, exists := financeDecimalValue(data[key]); exists {
			return amount, currency, true
		}
	}
	return decimal.Zero, "", false
}

func financeDecimalAt(root map[string]any, path ...string) (decimal.Decimal, bool) {
	var current any = root
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return decimal.Zero, false
		}
		current, ok = object[key]
		if !ok {
			return decimal.Zero, false
		}
	}
	return financeDecimalValue(current)
}

func financeDecimalValue(value any) (decimal.Decimal, bool) {
	if value == nil {
		return decimal.Zero, false
	}
	var raw string
	switch typed := value.(type) {
	case json.Number:
		raw = typed.String()
	case string:
		raw = typed
	case float64:
		raw = strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		raw = strconv.Itoa(typed)
	case int64:
		raw = strconv.FormatInt(typed, 10)
	default:
		return decimal.Zero, false
	}
	parsed, err := decimal.NewFromString(strings.TrimSpace(raw))
	return parsed, err == nil
}

func financeGetJSON(ctx context.Context, client FinanceHTTPDoer, baseURL, path, credential string) (map[string]any, int, error) {
	endpoint, err := resolveFinanceEndpoint(baseURL, path)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build upstream finance request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("upstream finance request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, resp.StatusCode, &UpstreamFinanceHTTPStatusError{StatusCode: resp.StatusCode}
	}
	limited := io.LimitReader(resp.Body, financeResponseMaxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read upstream finance response: %w", err)
	}
	if int64(len(body)) > financeResponseMaxBytes {
		return nil, resp.StatusCode, fmt.Errorf("upstream finance response exceeds %d bytes", financeResponseMaxBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload map[string]any
	if err = decoder.Decode(&payload); err != nil {
		return nil, resp.StatusCode, errors.New("upstream finance endpoint returned invalid JSON")
	}
	return payload, resp.StatusCode, nil
}

func resolveFinanceEndpoint(baseURL, path string) (string, error) {
	validated, err := urlvalidator.ValidateHTTPURL(strings.TrimSpace(baseURL), false, urlvalidator.ValidationOptions{})
	if err != nil {
		return "", fmt.Errorf("invalid upstream finance base URL: %w", err)
	}
	base, err := url.Parse(validated + "/")
	if err != nil {
		return "", err
	}
	relative, err := url.Parse(strings.TrimPrefix(path, "/"))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(relative).String(), nil
}

func newUpstreamFinanceHTTPClient() *http.Client {
	transport := &http.Transport{
		DialContext: safeDialContext, ForceAttemptHTTP2: true,
		MaxIdleConns: 32, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
	}
	return &http.Client{
		Timeout: financeHTTPTimeout, Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > financeHTTPMaxRedirects {
				return fmt.Errorf("upstream finance redirect limit exceeded")
			}
			if req == nil || req.URL == nil || len(via) == 0 || via[0].URL == nil {
				return errors.New("invalid upstream finance redirect")
			}
			if !strings.EqualFold(req.URL.Scheme, via[0].URL.Scheme) || !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
				return errors.New("cross-origin upstream finance redirect is not allowed")
			}
			_, err := urlvalidator.ValidateHTTPURL(req.URL.String(), false, urlvalidator.ValidationOptions{})
			return err
		},
	}
}

func financeHTTPErrorSummary(status int, err error) string {
	if err == nil {
		return ""
	}
	if status > 0 {
		return fmt.Sprintf("HTTP %d", status)
	}
	return "connection failed"
}

func firstFinanceErrorSummary(statusA int, errA error, statusB int, errB error) string {
	if errA != nil {
		return financeHTTPErrorSummary(statusA, errA)
	}
	return financeHTTPErrorSummary(statusB, errB)
}
