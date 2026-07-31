package service

import "net/http"

func BuiltinUpstreamFinanceProtocols() map[string]FinanceProtocolConfig {
	return map[string]FinanceProtocolConfig{
		"sub2api": {
			Capabilities: []string{FinanceCapabilityAccountUsage}, CostMode: FinanceCostModeCumulativeListAndActual,
			CounterScope:   FinanceCounterScopeAccount,
			Authentication: FinanceProtocolAuthentication{Type: "bearer", CredentialSource: "account_api_key"},
			Recognition: []FinanceProtocolRecognitionRule{
				{Method: http.MethodGet, Path: "/health", Match: FinanceProtocolRecognitionMatch{Status: http.StatusOK}},
				{Method: http.MethodGet, Path: "/api/v1/settings/public", Match: FinanceProtocolRecognitionMatch{Path: "$.data", Exists: boolPointer(true)}},
			},
			Operations:    map[string]FinanceProtocolOperation{FinanceCapabilityAccountUsage: {Method: http.MethodGet, Path: "/v1/usage", Mapping: map[string]string{"list_cost": "$.usage.total.cost", "actual_cost": "$.usage.total.actual_cost", "currency": "$.unit", "upstream_counter_id": "$.usage.counter_id", "counter_period": "$.usage.period"}}},
			UnitSemantics: FinanceUnitPlatformCredit, RedactPaths: []string{"$.token", "$.api_key"},
		},
		"newapi": {
			Capabilities: []string{FinanceCapabilityPricing, FinanceCapabilityBalance}, CostMode: FinanceCostModeManual,
			Authentication: FinanceProtocolAuthentication{Type: "bearer", CredentialSource: "account_api_key"},
			Recognition:    []FinanceProtocolRecognitionRule{{Method: http.MethodGet, Path: "/api/status", Match: FinanceProtocolRecognitionMatch{Path: "$.data", Exists: boolPointer(true)}}},
			Operations: map[string]FinanceProtocolOperation{
				FinanceCapabilityPricing: {Method: http.MethodGet, Path: "/api/pricing", Mapping: map[string]string{"models": "$.data"}},
				FinanceCapabilityBalance: {Method: http.MethodGet, Path: "/api/user/self", Mapping: map[string]string{"balance": "$.data.quota", "used": "$.data.used_quota"}},
			}, UnitSemantics: FinanceUnitPlatformCredit, RedactPaths: []string{"$.data.token", "$.data.api_key"},
		},
		"legacy_openai_billing": {
			Capabilities: []string{FinanceCapabilityQuota}, CostMode: FinanceCostModeManual,
			Authentication: FinanceProtocolAuthentication{Type: "bearer", CredentialSource: "account_api_key"},
			Recognition:    []FinanceProtocolRecognitionRule{{Method: http.MethodGet, Path: "/dashboard/billing/subscription", Match: FinanceProtocolRecognitionMatch{Path: "$.hard_limit_usd", Exists: boolPointer(true)}}},
			Operations:     map[string]FinanceProtocolOperation{FinanceCapabilityQuota: {Method: http.MethodGet, Path: "/dashboard/billing/subscription", Mapping: map[string]string{"quota": "$.hard_limit_usd"}}},
			UnitSemantics:  FinanceUnitFiatCurrency,
		},
		"manual_contract": {
			Capabilities: []string{FinanceCapabilityAccountUsage}, CostMode: FinanceCostModeManual,
			Authentication: FinanceProtocolAuthentication{Type: "none"}, Operations: map[string]FinanceProtocolOperation{}, UnitSemantics: FinanceUnitNone,
		},
	}
}

func boolPointer(value bool) *bool { return &value }
