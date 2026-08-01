package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestFinanceJSONPathRestrictedTraversal(t *testing.T) {
	payload := map[string]any{"data": []any{map[string]any{"cost": json.Number("1.25")}, map[string]any{"cost": "2.50"}}}
	values, err := FinanceJSONPath(payload, "$.data[*].cost")
	require.NoError(t, err)
	require.Equal(t, []any{json.Number("1.25"), "2.50"}, values)

	for _, path := range []string{"$..token", "$[?(@.cost)]", "$.data[abc]", "data.cost"} {
		_, err = FinanceJSONPath(payload, path)
		require.Error(t, err, path)
	}
}

func TestFinanceDecimalPreservesPrecision(t *testing.T) {
	value, err := FinanceDecimal(json.Number("1234567890.123456789"))
	require.NoError(t, err)
	require.Equal(t, "1234567890.123456789", value.String())
	_, err = FinanceDecimal(true)
	require.Error(t, err)
}

func TestEvaluateFinanceExpressionRestrictedArithmetic(t *testing.T) {
	value, err := EvaluateFinanceExpression("(actual - previous) / 2", map[string]decimal.Decimal{"actual": decimal.NewFromInt(9), "previous": decimal.NewFromInt(1)})
	require.NoError(t, err)
	require.True(t, value.Equal(decimal.NewFromInt(4)))
	for _, expression := range []string{"system(1)", "missing + 1", "1 / 0", "a[0]"} {
		_, err = EvaluateFinanceExpression(expression, map[string]decimal.Decimal{})
		require.Error(t, err, expression)
	}
}

func TestValidateFinanceProtocolConfigSecurityAndFundingEvidence(t *testing.T) {
	config := validFinanceProtocolConfig()
	config.Operations[FinanceCapabilityAccountUsage] = FinanceProtocolOperation{Method: http.MethodPost, Path: "https://metadata.google.internal/", Headers: map[string]string{"Host": "attacker"}, Mapping: map[string]string{"cost": "$..cost"}}
	result := ValidateFinanceProtocolConfig(FinanceProtocolTypeHTTPJSON, config)
	require.False(t, result.Valid)
	requireIssueCode(t, result, "unsafe_path")
	requireIssueCode(t, result, "unsafe_header")
	requireIssueCode(t, result, "invalid_json_path")

	config = validFinanceProtocolConfig()
	config.Capabilities = []string{FinanceCapabilityFundingTransactions}
	config.Operations = map[string]FinanceProtocolOperation{FinanceCapabilityFundingTransactions: {
		Method: http.MethodGet, Path: "/shop/products", EvidenceType: "public_storefront",
		Mapping: map[string]string{"transaction_id": "$.data[*].id", "paid_amount": "$.data[*].amount", "paid_currency": "$.data[*].currency", "base_credit_units": "$.data[*].credit", "bonus_credit_units": "$.data[*].bonus", "occurred_at": "$.data[*].time"},
	}}
	result = ValidateFinanceProtocolConfig(FinanceProtocolTypeHTTPJSON, config)
	require.False(t, result.Valid)
	requireIssueCode(t, result, "public_storefront_forbidden")
}

func TestFinanceProtocolChecksumStableAcrossMapOrder(t *testing.T) {
	first := validFinanceProtocolConfig()
	second := validFinanceProtocolConfig()
	secondOperation := second.Operations[FinanceCapabilityAccountUsage]
	secondOperation.Mapping = map[string]string{"currency": "$.unit", "actual_cost": "$.actual_cost"}
	second.Operations[FinanceCapabilityAccountUsage] = secondOperation
	firstOperation := first.Operations[FinanceCapabilityAccountUsage]
	firstOperation.Mapping = map[string]string{"actual_cost": "$.actual_cost", "currency": "$.unit"}
	first.Operations[FinanceCapabilityAccountUsage] = firstOperation
	one, err := FinanceProtocolChecksum(first)
	require.NoError(t, err)
	two, err := FinanceProtocolChecksum(second)
	require.NoError(t, err)
	require.Equal(t, one, two)
}

func TestBuiltinFinanceProtocolsValidateAndKeepUnitSemantics(t *testing.T) {
	protocols := BuiltinUpstreamFinanceProtocols()
	require.Equal(t, FinanceUnitPlatformCredit, protocols["sub2api"].UnitSemantics)
	require.Equal(t, "$.usage.total.cost", protocols["sub2api"].Operations[FinanceCapabilityAccountUsage].Mapping["list_cost"])
	require.Equal(t, "$.usage.total.actual_cost", protocols["sub2api"].Operations[FinanceCapabilityAccountUsage].Mapping["actual_cost"])
	require.Equal(t, FinanceUnitPlatformCredit, protocols["newapi"].UnitSemantics)
	require.Equal(t, FinanceUnitFiatCurrency, protocols["legacy_openai_billing"].UnitSemantics)
	for code, config := range protocols {
		result := ValidateFinanceProtocolConfig(FinanceProtocolTypeBuiltin, config)
		require.Truef(t, result.Valid, "%s: %+v", code, result.Issues)
	}
}

func TestValidateFinanceProtocolConfigSupportsCumulativeActualOnly(t *testing.T) {
	config := validFinanceProtocolConfig()
	config.CostMode = FinanceCostModeCumulativeActual
	operation := config.Operations[FinanceCapabilityAccountUsage]
	delete(operation.Mapping, "list_cost")
	config.Operations[FinanceCapabilityAccountUsage] = operation
	require.True(t, ValidateFinanceProtocolConfig(FinanceProtocolTypeHTTPJSON, config).Valid)

	delete(operation.Mapping, "actual_cost")
	config.Operations[FinanceCapabilityAccountUsage] = operation
	result := ValidateFinanceProtocolConfig(FinanceProtocolTypeHTTPJSON, config)
	require.False(t, result.Valid)
	requireIssueCode(t, result, "required")
}

func TestValidateAndExtractFinanceRequestCharge(t *testing.T) {
	config := FinanceProtocolConfig{
		Capabilities:   []string{FinanceCapabilityRequestCharge},
		CostMode:       FinanceCostModeRequestCharge,
		UnitSemantics:  FinanceUnitFiatCurrency,
		Authentication: FinanceProtocolAuthentication{Type: "bearer", CredentialSource: "account_api_key"},
		Operations: map[string]FinanceProtocolOperation{
			FinanceCapabilityRequestCharge: {
				Mapping: map[string]string{
					"actual_cost":        "$.billing.amount",
					"currency":           "$.billing.currency",
					"standard_cost":      "$.billing.list_amount",
					"fx_rate_to_usd":     "$.billing.fx_rate_to_usd",
					"billing_request_id": "$.request_id",
					"observed_at":        "$.billing.observed_at",
				},
			},
		},
		RedactPaths: []string{"$.billing.secret"},
	}
	validation := ValidateFinanceProtocolConfig(FinanceProtocolTypeHTTPJSON, config)
	require.Truef(t, validation.Valid, "%+v", validation.Issues)
	charge, err := ExtractFinanceRequestCharge([]byte(`{"request_id":"bill-7","billing":{"amount":"2.50","list_amount":"3.00","currency":"CNY","fx_rate_to_usd":"0.14","observed_at":"2026-07-29T00:00:00Z","secret":"remove-me"}}`), config)
	require.NoError(t, err)
	require.Equal(t, "2.5", charge.ActualCharge.String())
	require.Equal(t, "0.35", charge.ActualChargeUSD.String())
	require.Equal(t, "CNY", charge.Currency)
	require.Equal(t, "bill-7", charge.BillingRequestID)
	require.NotContains(t, charge.SafeSnapshot, "secret")

	_, err = ExtractFinanceRequestCharge([]byte(`{"billing":{"amount":"1","currency":"CNY"}}`), config)
	require.Error(t, err)
	config.Operations[FinanceCapabilityRequestCharge] = FinanceProtocolOperation{
		SSEEvent: "billing.completed",
		Mapping:  map[string]string{"actual_cost": "$.amount", "currency": "$.currency", "fx_rate_to_usd": "$.fx"},
	}
	charge, err = ExtractFinanceRequestCharge([]byte("event: progress\ndata: {\"amount\":\"1\",\"currency\":\"USD\"}\n\nevent: billing.completed\ndata: {\"amount\":\"3\",\"currency\":\"USD\"}\n\n"), config)
	require.NoError(t, err)
	require.Equal(t, "3", charge.ActualCharge.String())
}

func TestExtractFinanceRequestChargeSupportsPlatformCreditWithoutUSDConversion(t *testing.T) {
	config := FinanceProtocolConfig{
		Capabilities:   []string{FinanceCapabilityRequestCharge},
		CostMode:       FinanceCostModeRequestCharge,
		UnitSemantics:  FinanceUnitPlatformCredit,
		Authentication: FinanceProtocolAuthentication{Type: "bearer", CredentialSource: "account_api_key"},
		Operations: map[string]FinanceProtocolOperation{FinanceCapabilityRequestCharge: {
			Mapping: map[string]string{"actual_cost": "$.usage.charge", "unit_code": "$.usage.unit"},
		}},
	}
	validation := ValidateFinanceProtocolConfig(FinanceProtocolTypeHTTPJSON, config)
	require.Truef(t, validation.Valid, "%+v", validation.Issues)

	charge, err := ExtractFinanceRequestCharge([]byte(`{"usage":{"charge":"12.5","unit":"xiong_credit"}}`), config)
	require.NoError(t, err)
	require.Equal(t, "12.5", charge.ActualCharge.String())
	require.Equal(t, "XIONG_CREDIT", charge.Currency)
	require.Equal(t, FinanceUnitPlatformCredit, charge.UnitSemantics)
	require.False(t, charge.USDConversionAvailable)

	attempt := UsageUpstreamAttempt{AccountID: 1, InputTokens: 1, Billable: true}
	ApplyFinanceRequestChargeToUsageAttempt(&attempt, charge)
	require.Equal(t, "12.5", attempt.UpstreamActualCharge.String())
	require.Nil(t, attempt.UpstreamActualChargeUSD)
	result := NewFinanceCostCalculator().Calculate(FinanceCostCalculatorInput{Attempt: attempt, BillingMode: "token", RequestChargeExpected: true})
	require.Equal(t, FinanceCostStatusMissingPrice, result.Status)
	require.Equal(t, "request_charge_missing", result.Detail["reason"])
}

func TestExtractFinanceRequestChargeSupportsSSEWithoutNamedEvent(t *testing.T) {
	config := FinanceProtocolConfig{
		Capabilities:  []string{FinanceCapabilityRequestCharge},
		CostMode:      FinanceCostModeRequestCharge,
		UnitSemantics: FinanceUnitFiatCurrency,
		Operations: map[string]FinanceProtocolOperation{FinanceCapabilityRequestCharge: {
			Mapping: map[string]string{"actual_cost": "$.charge", "currency": "$.currency", "fx_rate_to_usd": "$.fx"},
		}},
	}
	charge, err := ExtractFinanceRequestCharge([]byte("data: {\"charge\":\"1\",\"currency\":\"USD\"}\n\ndata: {\"charge\":\"2\",\"currency\":\"CNY\",\"fx\":\"0.14\"}\n\n"), config)
	require.NoError(t, err)
	require.Equal(t, "2", charge.ActualCharge.String())
	require.Equal(t, "0.28", charge.ActualChargeUSD.String())
}

func TestAppendFinanceBillingPayloadKeepsTailForLongSSE(t *testing.T) {
	current := appendFinanceBillingPayload(nil, []byte(strings.Repeat("x", financeBillingPayloadMaxBytes)))
	current = appendFinanceBillingPayload(current, []byte("event: billing.completed\ndata: {\"amount\":\"3\"}\n\n"))
	require.Contains(t, string(current), "billing.completed")
	require.Contains(t, string(current), "\"amount\":\"3\"")
}

func TestUpstreamFinanceHTTPExecutorMapsAndRedacts(t *testing.T) {
	client := financeProtocolDoerFunc(func(request *http.Request) (*http.Response, error) {
		require.Equal(t, "https://example.com/v1/usage", request.URL.String())
		require.Equal(t, "Bearer secret-key", request.Header.Get("Authorization"))
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"actual_cost":"2.2","unit":"USD","token":"secret"}`)), Header: make(http.Header)}, nil
	})
	executor := NewUpstreamFinanceHTTPExecutorWithClient(client)
	config := validFinanceProtocolConfig()
	result, err := executor.Execute(context.Background(), config, FinanceCapabilityAccountUsage, "https://example.com", "secret-key")
	require.NoError(t, err)
	require.Equal(t, "2.2", result.Facts["actual_cost"])
	require.Equal(t, FinanceUnitPlatformCredit, result.UnitSemantics)
	snapshot, ok := result.SafeSnapshot.(map[string]any)
	require.True(t, ok)
	require.NotContains(t, snapshot, "token")
	require.Len(t, result.SnapshotChecksum, 64)
}

func TestProtocolUpstreamFinanceAdapterNormalizesPlatformCreditAccountUsage(t *testing.T) {
	executor := NewUpstreamFinanceHTTPExecutorWithClient(financeProtocolDoerFunc(func(request *http.Request) (*http.Response, error) {
		require.Equal(t, "Bearer account-key", request.Header.Get("Authorization"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"list_cost":"10","actual_cost":"2.2","unit":"USD"}`)),
			Header:     make(http.Header),
		}, nil
	}))
	adapter := NewProtocolUpstreamFinanceAdapter(7, "sub2api", validFinanceProtocolConfig(), executor)
	usageAdapter, ok := adapter.(UpstreamFinanceAccountUsageAdapter)
	require.True(t, ok)

	usage, err := usageAdapter.FetchAccountUsage(context.Background(), UpstreamWallet{BaseURL: "https://example.com", Currency: "USD"}, "account-key")
	require.NoError(t, err)
	require.Equal(t, FinanceUnitPlatformCredit, usage.UnitSemantics)
	require.Equal(t, "USD", usage.UnitCode)
	require.Nil(t, usage.Currency)
	require.Equal(t, "10", usage.ListCostTotal.String())
	require.Equal(t, "2.2", usage.ActualCostTotal.String())
	require.Len(t, usage.SnapshotChecksum, 64)
	require.NotEmpty(t, usage.SafeSnapshot)
}

func TestUpstreamFinanceHTTPExecutorNormalizesFundingTransactions(t *testing.T) {
	client := financeProtocolDoerFunc(func(*http.Request) (*http.Response, error) {
		body := `{"data":[{"id":"tx-1","paid":"52","currency":"cny","fx":"0.138","fx_source":"bank_receipt","base":"500","bonus":"20","at":"2026-07-29T00:00:00Z"}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	config := FinanceProtocolConfig{
		Capabilities: []string{FinanceCapabilityFundingTransactions}, CostMode: FinanceCostModeManual, UnitSemantics: FinanceUnitPlatformCredit,
		Authentication: FinanceProtocolAuthentication{Type: "bearer", CredentialSource: "account_api_key"},
		Operations: map[string]FinanceProtocolOperation{FinanceCapabilityFundingTransactions: {Method: http.MethodGet, Path: "/api/wallet/recharges", EvidenceType: "authenticated_transaction_api", Mapping: map[string]string{
			"transaction_id": "$.data[*].id", "paid_amount": "$.data[*].paid", "paid_currency": "$.data[*].currency", "fx_rate_to_usd": "$.data[*].fx", "fx_source": "$.data[*].fx_source", "base_credit_units": "$.data[*].base", "bonus_credit_units": "$.data[*].bonus", "occurred_at": "$.data[*].at",
		}}},
	}
	result, err := NewUpstreamFinanceHTTPExecutorWithClient(client).Execute(context.Background(), config, FinanceCapabilityFundingTransactions, "https://example.com", "key")
	require.NoError(t, err)
	transactions, ok := result.Facts["transactions"].([]FinanceFundingTransactionFact)
	require.True(t, ok)
	require.Equal(t, []FinanceFundingTransactionFact{{TransactionID: "tx-1", PaidAmount: "52", PaidCurrency: "CNY", FXRateToUSD: "0.138", FXSource: "bank_receipt", BaseCreditUnits: "500", BonusCreditUnits: "20", OccurredAt: "2026-07-29T00:00:00Z"}}, transactions)
}

func TestUpstreamFinanceHTTPExecutorRejectsInsecureSchemeBeforeRequest(t *testing.T) {
	called := false
	executor := NewUpstreamFinanceHTTPExecutorWithClient(financeProtocolDoerFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	}))
	result, err := executor.Execute(context.Background(), validFinanceProtocolConfig(), FinanceCapabilityAccountUsage, "http://example.com", "")
	require.Error(t, err)
	require.False(t, called)
	require.Equal(t, "security_policy", result.ErrorCode)
}

func TestUpstreamFinanceHTTPExecutorMapsSSECompletion(t *testing.T) {
	client := financeProtocolDoerFunc(func(*http.Request) (*http.Response, error) {
		body := "event: progress\ndata: {\"cost\":\"1\"}\n\nevent: completed\ndata: {\"cost\":\"2.50\"}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	config := validFinanceProtocolConfig()
	op := config.Operations[FinanceCapabilityAccountUsage]
	op.SSEEvent = "completed"
	op.Mapping = map[string]string{"list_cost": "$.cost", "actual_cost": "$.cost"}
	config.Operations[FinanceCapabilityAccountUsage] = op
	result, err := NewUpstreamFinanceHTTPExecutorWithClient(client).Execute(context.Background(), config, FinanceCapabilityAccountUsage, "https://example.com", "")
	require.NoError(t, err)
	require.Equal(t, "2.50", result.Facts["actual_cost"])
}

func TestUpstreamFinanceAdapterRegistrySupportsExecutorFamilies(t *testing.T) {
	registry := NewUpstreamFinanceAdapterRegistry()
	executor := NewUpstreamFinanceHTTPExecutorWithClient(financeProtocolDoerFunc(func(*http.Request) (*http.Response, error) { return nil, nil }))
	registry.RegisterProtocolExecutor(FinanceProtocolTypeHTTPJSON, executor)
	actual, err := registry.ProtocolExecutor(FinanceProtocolTypeHTTPJSON)
	require.NoError(t, err)
	require.Same(t, executor, actual)
	_, err = registry.ProtocolExecutor("site-specific-enum")
	require.Error(t, err)
}

func validFinanceProtocolConfig() FinanceProtocolConfig {
	return FinanceProtocolConfig{
		Capabilities: []string{FinanceCapabilityAccountUsage}, CostMode: FinanceCostModeCumulativeListAndActual,
		CounterScope:  FinanceCounterScopeAccount,
		UnitSemantics: FinanceUnitPlatformCredit, Authentication: FinanceProtocolAuthentication{Type: "bearer", CredentialSource: "account_api_key"},
		Operations:  map[string]FinanceProtocolOperation{FinanceCapabilityAccountUsage: {Method: http.MethodGet, Path: "/v1/usage", Mapping: map[string]string{"list_cost": "$.list_cost", "actual_cost": "$.actual_cost", "currency": "$.unit", "upstream_counter_id": "$.counter_id"}}},
		RedactPaths: []string{"$.token", "$.api_key"},
	}
}

func requireIssueCode(t *testing.T, result FinanceProtocolValidationResult, code string) {
	t.Helper()
	for _, issue := range result.Issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("issue code %q not found in %+v", code, result.Issues)
}

type financeProtocolDoerFunc func(*http.Request) (*http.Response, error)

func (f financeProtocolDoerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }
