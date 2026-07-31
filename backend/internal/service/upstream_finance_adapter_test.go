package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type financeHTTPDoerFunc func(*http.Request) (*http.Response, error)

func (f financeHTTPDoerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func financeJSONResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestNewAPIAdapterConvertsTokenAndRequestPrices(t *testing.T) {
	doer := financeHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/status":
			return financeJSONResponse(200, `{"data":{"quota_per_unit":500000}}`), nil
		case "/api/pricing":
			return financeJSONResponse(200, `{
				"group_ratio":{"vip":2},
				"data":[
					{"model_name":"gpt-token","quota_type":0,"model_ratio":1,"completion_ratio":3,"cache_ratio":0.5,"create_cache_ratio":1.25,"enable_groups":["vip"]},
					{"model_name":"image-request","quota_type":1,"model_price":100,"enable_groups":["all"]},
					{"model_name":"hidden","quota_type":0,"model_ratio":1,"enable_groups":["other"]}
				]
			}`), nil
		default:
			return nil, errors.New("unexpected path")
		}
	})
	adapter := NewNewAPIUpstreamFinanceAdapter(doer)
	prices, err := adapter.FetchPricing(context.Background(), UpstreamWallet{
		AdapterType: UpstreamAdapterNewAPI, BaseURL: "https://example.com", PricingGroup: "vip",
	}, "")
	require.NoError(t, err)
	require.Len(t, prices, 2)
	require.Equal(t, "gpt-token", prices[0].ModelPattern)
	require.Equal(t, "4", prices[0].PriceDetail["input"])
	require.Equal(t, "12", prices[0].PriceDetail["output"])
	require.Equal(t, "2", prices[0].PriceDetail["cache_read"])
	require.Equal(t, "5", prices[0].PriceDetail["cache_write_5m"])
	require.Equal(t, "per_request", prices[1].BillingMode)
	require.Equal(t, "0.0004", prices[1].PriceDetail["per_request"])
}

func TestProtocolAdapterExecutesPublishedVersionShapeWithoutSiteSpecificCode(t *testing.T) {
	doer := financeHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "Bearer vendor-secret", req.Header.Get("Authorization"))
		switch req.URL.Path {
		case "/finance/prices":
			return financeJSONResponse(200, `{"data":[{"model_pattern":"vendor-model","billing_mode":"token","input":"2","output":"8","currency":"USD"}]}`), nil
		case "/finance/balance":
			return financeJSONResponse(200, `{"available":"123.45","currency":"USD"}`), nil
		default:
			return nil, errors.New("unexpected path")
		}
	})
	config := FinanceProtocolConfig{
		Capabilities: []string{FinanceCapabilityPricing, FinanceCapabilityBalance}, CostMode: FinanceCostModeManual, UnitSemantics: FinanceUnitFiatCurrency,
		Authentication: FinanceProtocolAuthentication{Type: "bearer", CredentialSource: "wallet_finance_credential"},
		Operations: map[string]FinanceProtocolOperation{
			FinanceCapabilityPricing: {Method: http.MethodGet, Path: "/finance/prices", Mapping: map[string]string{"prices": "$.data"}},
			FinanceCapabilityBalance: {Method: http.MethodGet, Path: "/finance/balance", Mapping: map[string]string{"balance": "$.available", "currency": "$.currency"}},
		},
	}
	adapter := NewProtocolUpstreamFinanceAdapter(91, "vendor_x", config, NewUpstreamFinanceHTTPExecutorWithClient(doer))
	wallet := UpstreamWallet{AdapterType: UpstreamAdapterProtocol, BaseURL: "https://vendor.example", Currency: "USD"}
	prices, err := adapter.FetchPricing(context.Background(), wallet, "vendor-secret")
	require.NoError(t, err)
	require.Len(t, prices, 1)
	require.Equal(t, "vendor-model", prices[0].ModelPattern)
	require.Equal(t, "2", prices[0].PriceDetail["input"])
	require.Equal(t, "8", prices[0].PriceDetail["output"])
	require.Equal(t, int64(91), prices[0].SourceSnapshot["protocol_version_id"])
	balance, err := adapter.FetchBalance(context.Background(), wallet, "vendor-secret")
	require.NoError(t, err)
	require.Equal(t, "wallet_cash", balance.BalanceKind)
	require.True(t, balance.BalanceAmount.Equal(decimal.RequireFromString("123.45")))
}

func TestProtocolAdapterKeepsPlatformCreditOutOfWalletCash(t *testing.T) {
	doer := financeHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		return financeJSONResponse(200, `{"balance":"1000","unit":"credit"}`), nil
	})
	config := FinanceProtocolConfig{
		Capabilities: []string{FinanceCapabilityBalance}, CostMode: FinanceCostModeManual, UnitSemantics: FinanceUnitPlatformCredit,
		Authentication: FinanceProtocolAuthentication{Type: "bearer", CredentialSource: "wallet_finance_credential"},
		Operations:     map[string]FinanceProtocolOperation{FinanceCapabilityBalance: {Method: http.MethodGet, Path: "/balance", Mapping: map[string]string{"balance": "$.balance"}}},
	}
	adapter := NewProtocolUpstreamFinanceAdapter(92, "credit_vendor", config, NewUpstreamFinanceHTTPExecutorWithClient(doer))
	balance, err := adapter.FetchBalance(context.Background(), UpstreamWallet{BaseURL: "https://vendor.example"}, "secret")
	require.NoError(t, err)
	require.Equal(t, "token_quota", balance.BalanceKind)
	require.Nil(t, balance.BalanceAmount)
	require.True(t, balance.TotalQuota.Equal(decimal.NewFromInt(1000)))
}

func TestNewAPIAdapterDoesNotTreatQuotaAsWalletCash(t *testing.T) {
	doer := financeHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "Bearer user-token", req.Header.Get("Authorization"))
		return financeJSONResponse(200, `{"success":true,"data":{"quota":100000,"used_quota":50000}}`), nil
	})
	adapter := NewNewAPIUpstreamFinanceAdapter(doer)
	_, err := adapter.FetchBalance(context.Background(), UpstreamWallet{BaseURL: "https://example.com"}, "user-token")
	require.ErrorIs(t, err, ErrUpstreamFinanceCapabilityUnsupported)
}

func TestNewAPIAdapterAcceptsOnlyExplicitCashWithCurrency(t *testing.T) {
	doer := financeHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		return financeJSONResponse(200, `{"data":{"cash_balance":"12.34","currency":"CNY","quota":999999}}`), nil
	})
	adapter := NewNewAPIUpstreamFinanceAdapter(doer)
	balance, err := adapter.FetchBalance(context.Background(), UpstreamWallet{BaseURL: "https://example.com"}, "user-token")
	require.NoError(t, err)
	require.Equal(t, "wallet_cash", balance.BalanceKind)
	require.True(t, balance.BalanceAmount.Equal(decimal.RequireFromString("12.34")))
	require.Nil(t, balance.TotalQuota)
	require.Equal(t, "CNY", balance.Currency)
}

func TestLegacyBillingAdapterClassifiesResultAsTokenQuota(t *testing.T) {
	doer := financeHTTPDoerFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/dashboard/billing/subscription":
			return financeJSONResponse(200, `{"hard_limit_usd":100}`), nil
		case "/dashboard/billing/usage":
			return financeJSONResponse(200, `{"total_usage":250}`), nil
		default:
			return nil, errors.New("unexpected path")
		}
	})
	adapter := NewLegacyBillingFinanceAdapter(doer)
	quota, err := adapter.FetchQuota(context.Background(), UpstreamWallet{BaseURL: "https://example.com"}, "model-key")
	require.NoError(t, err)
	require.Equal(t, "token_quota", quota.BalanceKind)
	require.Nil(t, quota.BalanceAmount)
	require.True(t, quota.TotalQuota.Equal(decimal.NewFromInt(100)))
	require.True(t, quota.UsedQuota.Equal(decimal.RequireFromString("2.5")))
}

func TestFinanceHTTPResponseLimitAndInvalidJSON(t *testing.T) {
	tooLarge := strings.Repeat("x", int(financeResponseMaxBytes)+1)
	_, _, err := financeGetJSON(context.Background(), financeHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		return financeJSONResponse(200, tooLarge), nil
	}), "https://example.com", "/api/status", "")
	require.Contains(t, err.Error(), "exceeds")
	_, _, err = financeGetJSON(context.Background(), financeHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		return financeJSONResponse(200, "<html>not json</html>"), nil
	}), "https://example.com", "/api/status", "")
	require.EqualError(t, err, "upstream finance endpoint returned invalid JSON")
}

func TestFinanceEndpointAndRedirectRejectSSRF(t *testing.T) {
	_, err := resolveFinanceEndpoint("https://169.254.169.254", "/api/status")
	require.Error(t, err)
	client := newUpstreamFinanceHTTPClient()
	redirect, err := http.NewRequest(http.MethodGet, "https://127.0.0.1/api/pricing", nil)
	require.NoError(t, err)
	original, err := http.NewRequest(http.MethodGet, "https://127.0.0.1/api/status", nil)
	require.NoError(t, err)
	require.Error(t, client.CheckRedirect(redirect, []*http.Request{original}))
	crossOrigin, err := http.NewRequest(http.MethodGet, "https://evil.example/api/pricing", nil)
	require.NoError(t, err)
	safeOriginal, err := http.NewRequest(http.MethodGet, "https://good.example/api/status", nil)
	require.NoError(t, err)
	require.ErrorContains(t, client.CheckRedirect(crossOrigin, []*http.Request{safeOriginal}), "cross-origin")
}

func TestManualFinanceAdapterDoesNotUseNetwork(t *testing.T) {
	adapter := NewManualUpstreamFinanceAdapter()
	probe, err := adapter.Probe(context.Background(), UpstreamWallet{AdapterType: UpstreamAdapterManual}, "secret")
	require.NoError(t, err)
	require.False(t, probe.Reachable)
	require.Equal(t, FinanceCapabilityUnsupported, probe.Capabilities.Pricing)
}
