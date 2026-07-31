package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

var financeProtocolCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,79}$`)

var allowedFinanceCapabilities = map[string]struct{}{
	FinanceCapabilityPricing: {}, FinanceCapabilityAccountUsage: {}, FinanceCapabilityRequestCharge: {},
	FinanceCapabilityBalance: {}, FinanceCapabilityFundingTransactions: {}, FinanceCapabilityQuota: {},
}

var allowedFinanceCostModes = map[string]struct{}{
	FinanceCostModeRequestCharge: {}, FinanceCostModeCumulativeListAndActual: {},
	FinanceCostModeCumulativeActual:   {},
	FinanceCostModeContractMultiplier: {}, FinanceCostModeManual: {},
}

var forbiddenFinanceHeaders = map[string]struct{}{
	"host": {}, "connection": {}, "proxy-authorization": {}, "proxy-connection": {},
	"forwarded": {}, "x-forwarded-for": {}, "x-forwarded-host": {}, "x-real-ip": {}, "transfer-encoding": {},
	"authorization": {}, "cookie": {}, "set-cookie": {}, "x-api-key": {}, "api-key": {}, "x-auth-token": {},
}

func ValidateFinanceProtocolConfig(protocolType string, config FinanceProtocolConfig) FinanceProtocolValidationResult {
	issues := make([]FinanceProtocolValidationIssue, 0)
	add := func(path, code, message string) {
		issues = append(issues, FinanceProtocolValidationIssue{Path: path, Code: code, Message: message})
	}
	if protocolType != FinanceProtocolTypeBuiltin && protocolType != FinanceProtocolTypeHTTPJSON && protocolType != FinanceProtocolTypePlugin {
		add("protocol_type", "invalid_enum", "protocol_type must be builtin, http_json or plugin")
	}
	if protocolType == FinanceProtocolTypePlugin {
		add("protocol_type", "unsupported_runtime", "plugin protocols cannot be published until a plugin runtime is configured")
	}
	if _, ok := allowedFinanceCostModes[config.CostMode]; !ok {
		add("cost_mode", "invalid_enum", "unsupported cost mode")
	}
	if config.UnitSemantics != "" && config.UnitSemantics != FinanceUnitFiatCurrency && config.UnitSemantics != FinanceUnitPlatformCredit && config.UnitSemantics != FinanceUnitNone {
		add("unit_semantics", "invalid_enum", "unit semantics must be fiat_currency, platform_credit or none")
	}
	if len(config.Capabilities) == 0 {
		add("capabilities", "required", "at least one capability is required")
	}
	seen := map[string]struct{}{}
	for i, capability := range config.Capabilities {
		if _, ok := allowedFinanceCapabilities[capability]; !ok {
			add(fmt.Sprintf("capabilities[%d]", i), "invalid_enum", "unsupported capability")
		}
		if _, duplicate := seen[capability]; duplicate {
			add(fmt.Sprintf("capabilities[%d]", i), "duplicate", "capability is duplicated")
		}
		seen[capability] = struct{}{}
		if protocolType != FinanceProtocolTypePlugin {
			if _, ok := config.Operations[capability]; !ok && config.CostMode != FinanceCostModeManual {
				add("operations."+capability, "required", "capability requires an operation")
			}
		}
	}
	if config.CostMode == FinanceCostModeRequestCharge {
		if _, ok := seen[FinanceCapabilityRequestCharge]; !ok {
			add("cost_mode", "capability_required", "request_charge cost mode requires request_charge capability")
		}
		if config.UnitSemantics != FinanceUnitFiatCurrency && config.UnitSemantics != FinanceUnitPlatformCredit {
			add("unit_semantics", "invalid_request_charge_unit", "request_charge requires fiat_currency or platform_credit unit semantics")
		}
	}
	if config.CostMode == FinanceCostModeCumulativeListAndActual || config.CostMode == FinanceCostModeCumulativeActual {
		if _, ok := seen[FinanceCapabilityAccountUsage]; !ok {
			add("cost_mode", "capability_required", "cumulative cost mode requires account_usage capability")
		}
		if config.CounterScope != FinanceCounterScopeAccount && config.CounterScope != FinanceCounterScopeWallet && config.CounterScope != FinanceCounterScopeOrganization {
			add("counter_scope", "required", "cumulative account usage requires explicit counter_scope: account, wallet or organization")
		}
	}
	if config.Authentication.Type != "none" && config.Authentication.Type != "bearer" && config.Authentication.Type != "api_key_header" {
		add("authentication.type", "invalid_enum", "authentication type must be none, bearer or api_key_header")
	}
	if config.Authentication.Type != "none" {
		source := strings.ToLower(strings.TrimSpace(config.Authentication.CredentialSource))
		if source != "account_api_key" && source != "account_access_token" && source != "account_token" && source != "account_setup_token" && source != "wallet_finance_credential" {
			add("authentication.credential_source", "invalid_enum", "credential_source must identify an approved account or wallet credential")
		}
	}
	if config.Authentication.Type == "api_key_header" {
		if err := validateFinanceHeaderName(config.Authentication.HeaderName); err != nil {
			add("authentication.header_name", "unsafe_header", err.Error())
		}
	}
	for name, op := range config.Operations {
		base := "operations." + name
		if _, ok := allowedFinanceCapabilities[name]; !ok && name != "recognition" {
			add(base, "invalid_operation", "operation name must be a supported capability")
		}
		if name != FinanceCapabilityRequestCharge {
			method := strings.ToUpper(strings.TrimSpace(op.Method))
			if method != http.MethodGet && method != http.MethodPost {
				add(base+".method", "invalid_method", "only GET and POST are supported")
			}
			if err := validateFinanceRelativePath(op.Path); err != nil {
				add(base+".path", "unsafe_path", err.Error())
			}
		}
		for header := range op.Headers {
			if err := validateFinanceHeaderName(header); err != nil {
				add(base+".headers."+header, "unsafe_header", err.Error())
			}
		}
		for header, value := range op.Headers {
			if strings.ContainsAny(value, "\r\n") {
				add(base+".headers."+header, "unsafe_header_value", "header value cannot contain line breaks")
			}
		}
		for field, path := range op.Mapping {
			if _, err := parseFinanceJSONPath(path); err != nil {
				add(base+".mapping."+field, "invalid_json_path", err.Error())
			}
		}
		if op.Pagination != nil {
			if op.Pagination.Type != "page" && op.Pagination.Type != "cursor" {
				add(base+".pagination.type", "invalid_enum", "pagination type must be page or cursor")
			}
			if op.Pagination.MaxPages < 1 || op.Pagination.MaxPages > 100 {
				add(base+".pagination.max_pages", "out_of_range", "max_pages must be 1 to 100")
			}
		}
		if name == FinanceCapabilityFundingTransactions {
			if op.EvidenceType == "public_storefront" || strings.Contains(strings.ToLower(op.Path), "shop/products") || strings.Contains(strings.ToLower(op.Path), "store/products") {
				add(base+".evidence_type", "public_storefront_forbidden", "anonymous storefront data cannot be used as recharge transaction evidence")
			}
			required := []string{"transaction_id", "paid_amount", "paid_currency", "base_credit_units", "bonus_credit_units", "occurred_at"}
			for _, field := range required {
				if _, ok := op.Mapping[field]; !ok {
					add(base+".mapping."+field, "required", "funding transaction field is required")
				}
			}
		}
		if name == FinanceCapabilityAccountUsage && config.CostMode == FinanceCostModeCumulativeListAndActual {
			for _, field := range []string{"list_cost", "actual_cost"} {
				if _, ok := op.Mapping[field]; !ok {
					add(base+".mapping."+field, "required", "cumulative account usage field is required")
				}
			}
		}
		if name == FinanceCapabilityAccountUsage && config.CostMode == FinanceCostModeCumulativeActual {
			if _, ok := op.Mapping["actual_cost"]; !ok {
				add(base+".mapping.actual_cost", "required", "cumulative actual cost field is required")
			}
		}
		if name == FinanceCapabilityRequestCharge {
			requiredFields := []string{"actual_cost"}
			if config.UnitSemantics == FinanceUnitPlatformCredit {
				requiredFields = append(requiredFields, "unit_code")
			} else {
				requiredFields = append(requiredFields, "currency")
			}
			for _, field := range requiredFields {
				if _, ok := op.Mapping[field]; !ok {
					add(base+".mapping."+field, "required", "request charge field is required")
				}
			}
		}
	}
	for i, rule := range config.Recognition {
		if strings.ToUpper(rule.Method) != http.MethodGet {
			add(fmt.Sprintf("recognition[%d].method", i), "unsafe_method", "recognition requests must use GET")
		}
		if err := validateFinanceRelativePath(rule.Path); err != nil {
			add(fmt.Sprintf("recognition[%d].path", i), "unsafe_path", err.Error())
		}
		if rule.Match.Path != "" {
			if _, err := parseFinanceJSONPath(rule.Match.Path); err != nil {
				add(fmt.Sprintf("recognition[%d].match.path", i), "invalid_json_path", err.Error())
			}
		}
	}
	for i, path := range config.RedactPaths {
		if _, err := parseFinanceJSONPath(path); err != nil {
			add(fmt.Sprintf("redact_paths[%d]", i), "invalid_json_path", err.Error())
		}
	}
	checksum, err := FinanceProtocolChecksum(config)
	if err != nil {
		add("config", "checksum_failed", err.Error())
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Path == issues[j].Path {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Path < issues[j].Path
	})
	return FinanceProtocolValidationResult{Valid: len(issues) == 0, Issues: issues, Checksum: checksum}
}

func FinanceProtocolChecksum(config FinanceProtocolConfig) (string, error) {
	payload, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal protocol config: %w", err)
	}
	var canonical any
	if err = json.Unmarshal(payload, &canonical); err != nil {
		return "", fmt.Errorf("normalize protocol config: %w", err)
	}
	payload, err = json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshal normalized protocol config: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func validateFinanceRelativePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must be an absolute-path reference")
	}
	if strings.HasPrefix(path, "//") || strings.Contains(path, "://") || strings.ContainsAny(path, "\r\n\\") {
		return fmt.Errorf("path cannot override host or contain control characters")
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal is not allowed")
	}
	return nil
}

func validateFinanceHeaderName(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return fmt.Errorf("header name is required")
	}
	if _, blocked := forbiddenFinanceHeaders[name]; blocked {
		return fmt.Errorf("header %s is not allowed", name)
	}
	if strings.ContainsAny(name, "\r\n:") {
		return fmt.Errorf("invalid header name")
	}
	return nil
}

func ValidateFinanceProtocolCode(code string) error {
	if !financeProtocolCodePattern.MatchString(code) {
		return financeValidationError("code must match ^[a-z][a-z0-9_-]{2,79}$")
	}
	return nil
}
