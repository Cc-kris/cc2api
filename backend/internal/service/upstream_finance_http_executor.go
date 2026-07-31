package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const financeProtocolResponseMaxBytes = int64(1 << 20)

type UpstreamFinanceHTTPExecutor struct{ client FinanceHTTPDoer }

func NewUpstreamFinanceHTTPExecutor() *UpstreamFinanceHTTPExecutor {
	return &UpstreamFinanceHTTPExecutor{client: newUpstreamFinanceHTTPClient()}
}

func NewUpstreamFinanceHTTPExecutorWithClient(client FinanceHTTPDoer) *UpstreamFinanceHTTPExecutor {
	return &UpstreamFinanceHTTPExecutor{client: client}
}

func (e *UpstreamFinanceHTTPExecutor) Execute(ctx context.Context, config FinanceProtocolConfig, capability, baseURL, credential string) (*FinanceProtocolExecutionResult, error) {
	validation := ValidateFinanceProtocolConfig(FinanceProtocolTypeHTTPJSON, config)
	if !validation.Valid {
		return nil, financeValidationError("protocol config is invalid")
	}
	operation, ok := config.Operations[capability]
	if !ok {
		return nil, ErrUpstreamFinanceProtocolUnsupported
	}
	started := time.Now()
	result, err := e.executeOperation(ctx, config, capability, operation, baseURL, credential)
	if result == nil {
		result = &FinanceProtocolExecutionResult{Capability: capability}
	}
	result.UnitSemantics = config.UnitSemantics
	result.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		result.ErrorCode = classifyFinanceProtocolError(err)
		result.ErrorSummary = sanitizeFinanceProtocolError(err)
		return result, err
	}
	return result, nil
}

func (e *UpstreamFinanceHTTPExecutor) executeOperation(ctx context.Context, config FinanceProtocolConfig, capability string, operation FinanceProtocolOperation, baseURL, credential string) (*FinanceProtocolExecutionResult, error) {
	maxPages := 1
	if operation.Pagination != nil {
		maxPages = operation.Pagination.MaxPages
	}
	path := operation.Path
	allFacts := map[string]any{}
	var snapshots []any
	statusCode := 0
	cursor := ""
	for page := 1; page <= maxPages; page++ {
		pagePath, err := financePaginationPath(path, operation.Pagination, page, cursor)
		if err != nil {
			return nil, err
		}
		request, err := buildFinanceProtocolRequest(ctx, config.Authentication, operation, baseURL, pagePath, credential)
		if err != nil {
			return nil, err
		}
		response, err := e.client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("finance protocol request failed: %w", err)
		}
		statusCode = response.StatusCode
		payload, readErr := readFinanceProtocolResponse(response, operation.SSEEvent)
		if readErr != nil {
			return nil, readErr
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, &UpstreamFinanceHTTPStatusError{StatusCode: response.StatusCode}
		}
		facts, err := mapFinanceProtocolFacts(payload, operation.Mapping)
		if err != nil {
			return nil, err
		}
		mergeFinanceProtocolFacts(allFacts, facts)
		if operation.Pagination != nil && operation.Pagination.Type == "cursor" && operation.Pagination.CursorPath != "" {
			values, _ := FinanceJSONPath(payload, operation.Pagination.CursorPath)
			if len(values) > 0 {
				cursor = fmt.Sprint(values[0])
			}
		}
		safeSnapshot, err := redactFinanceProtocolSnapshot(payload, config.RedactPaths)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, safeSnapshot)
		if operation.Pagination == nil || !financeProtocolHasNextPage(operation.Pagination, payload, page) {
			break
		}
	}
	var snapshot any = snapshots
	if len(snapshots) == 1 {
		snapshot = snapshots[0]
	}
	if capability == FinanceCapabilityFundingTransactions {
		transactions, normalizeErr := normalizeFinanceFundingTransactions(allFacts)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		allFacts["transactions"] = transactions
	}
	checksum, err := checksumFinanceSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	return &FinanceProtocolExecutionResult{Capability: capability, Facts: allFacts, SafeSnapshot: snapshot, SnapshotChecksum: checksum, StatusCode: statusCode}, nil
}

func buildFinanceProtocolRequest(ctx context.Context, auth FinanceProtocolAuthentication, operation FinanceProtocolOperation, baseURL, path, credential string) (*http.Request, error) {
	if err := validateFinanceRelativePath(path); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstreamFinanceProtocolUnsafe, err)
	}
	endpoint, err := resolveFinanceEndpoint(baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid finance endpoint", ErrUpstreamFinanceProtocolUnsafe)
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || !strings.EqualFold(parsedEndpoint.Scheme, "https") {
		return nil, fmt.Errorf("%w: generic finance protocols require HTTPS", ErrUpstreamFinanceProtocolUnsafe)
	}
	var body io.Reader
	if operation.Body != nil {
		bodyValue, err := interpolateFinanceProtocolValue(operation.Body, map[string]string{})
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(bodyValue)
		if err != nil {
			return nil, fmt.Errorf("marshal finance protocol body: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, strings.ToUpper(operation.Method), endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("build finance protocol request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if operation.Body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range operation.Headers {
		if err := validateFinanceHeaderName(name); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUpstreamFinanceProtocolUnsafe, err)
		}
		request.Header.Set(name, value)
	}
	switch auth.Type {
	case "bearer":
		if credential != "" {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
	case "api_key_header":
		if err := validateFinanceHeaderName(auth.HeaderName); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUpstreamFinanceProtocolUnsafe, err)
		}
		request.Header.Set(auth.HeaderName, credential)
	case "none":
	default:
		return nil, financeValidationError("unsupported authentication type")
	}
	return request, nil
}

func readFinanceProtocolResponse(response *http.Response, sseEvent string) (any, error) {
	defer func() { _ = response.Body.Close() }()
	limited := io.LimitReader(response.Body, financeProtocolResponseMaxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read finance protocol response: %w", err)
	}
	if int64(len(body)) > financeProtocolResponseMaxBytes {
		return nil, fmt.Errorf("finance protocol response exceeds %d bytes", financeProtocolResponseMaxBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return map[string]any{"status": response.StatusCode}, nil
	}
	if sseEvent != "" {
		return parseFinanceSSEEvent(body, sseEvent)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload any
	if err = decoder.Decode(&payload); err != nil {
		return nil, errors.New("finance protocol response is not valid JSON")
	}
	return payload, nil
}

func parseFinanceSSEEvent(body []byte, target string) (any, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 4096), int(financeProtocolResponseMaxBytes))
	event := ""
	data := ""
	var lastPayload any
	foundPayload := false
	flush := func() (any, bool, error) {
		if target != "" && event != target {
			event, data = "", ""
			return nil, false, nil
		}
		if strings.TrimSpace(data) == "" {
			event, data = "", ""
			return nil, false, nil
		}
		decoder := json.NewDecoder(strings.NewReader(data))
		decoder.UseNumber()
		var payload any
		if err := decoder.Decode(&payload); err != nil {
			return nil, false, errors.New("finance SSE event contains invalid JSON")
		}
		if target == "" {
			lastPayload = payload
			foundPayload = true
			event, data = "", ""
			return nil, false, nil
		}
		return payload, true, nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if payload, ok, err := flush(); ok || err != nil {
				return payload, err
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			if data != "" {
				data += "\n"
			}
			data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	if payload, ok, err := flush(); ok || err != nil {
		return payload, err
	}
	if target == "" && foundPayload {
		return lastPayload, nil
	}
	return nil, fmt.Errorf("finance SSE event %q not found", target)
}

func mapFinanceProtocolFacts(payload any, mapping map[string]string) (map[string]any, error) {
	facts := make(map[string]any, len(mapping))
	for field, path := range mapping {
		values, err := FinanceJSONPath(payload, path)
		if err != nil {
			return nil, fmt.Errorf("map %s: %w", field, err)
		}
		if len(values) == 0 {
			continue
		}
		if len(values) == 1 {
			facts[field] = values[0]
		} else {
			facts[field] = values
		}
	}
	return facts, nil
}

func redactFinanceProtocolSnapshot(payload any, paths []string) (any, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var clone any
	if err = decoder.Decode(&clone); err != nil {
		return nil, err
	}
	for _, path := range paths {
		tokens, err := parseFinanceJSONPath(path)
		if err != nil {
			return nil, err
		}
		redactFinanceTokens(clone, tokens)
	}
	redactFinanceSensitiveKeys(clone)
	return clone, nil
}

func redactFinanceSensitiveKeys(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "_", "-"))
			if normalized == "authorization" || normalized == "cookie" || normalized == "api-key" || normalized == "x-api-key" || strings.Contains(normalized, "access-token") || strings.Contains(normalized, "refresh-token") || normalized == "token" || normalized == "secret" || normalized == "password" {
				delete(typed, key)
				continue
			}
			redactFinanceSensitiveKeys(child)
		}
	case []any:
		for _, child := range typed {
			redactFinanceSensitiveKeys(child)
		}
	}
}

func redactFinanceTokens(value any, tokens []financeJSONPathToken) {
	if len(tokens) == 0 {
		return
	}
	token := tokens[0]
	if len(tokens) == 1 && token.field != "" {
		if object, ok := value.(map[string]any); ok {
			delete(object, token.field)
		}
		return
	}
	children := make([]any, 0)
	if token.field != "" {
		if object, ok := value.(map[string]any); ok {
			if child, exists := object[token.field]; exists {
				children = append(children, child)
			}
		}
	}
	if token.index != nil {
		if array, ok := value.([]any); ok && *token.index < len(array) {
			children = append(children, array[*token.index])
		}
	}
	if token.wildcard {
		if array, ok := value.([]any); ok {
			children = append(children, array...)
		}
	}
	for _, child := range children {
		redactFinanceTokens(child, tokens[1:])
	}
}

func checksumFinanceSnapshot(snapshot any) (string, error) {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func interpolateFinanceProtocolValue(value any, fields map[string]string) (any, error) {
	switch typed := value.(type) {
	case string:
		result := typed
		for key, replacement := range fields {
			result = strings.ReplaceAll(result, "{{"+key+"}}", replacement)
		}
		if strings.Contains(result, "{{") {
			return nil, fmt.Errorf("unknown body interpolation field")
		}
		return result, nil
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			v, err := interpolateFinanceProtocolValue(typed[i], fields)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			v, err := interpolateFinanceProtocolValue(item, fields)
			if err != nil {
				return nil, err
			}
			out[key] = v
		}
		return out, nil
	case nil, bool, json.Number, float64:
		return typed, nil
	default:
		return nil, fmt.Errorf("unsupported body value type")
	}
}

func financePaginationPath(path string, pagination *FinanceProtocolPagination, page int, cursor string) (string, error) {
	if pagination == nil {
		return path, nil
	}
	parsed, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	if pagination.Type == "page" {
		parameter := pagination.PageParameter
		if parameter == "" {
			parameter = "page"
		}
		query.Set(parameter, strconv.Itoa(page))
	}
	if pagination.Type == "cursor" && page > 1 {
		if cursor == "" {
			return path, nil
		}
		parameter := pagination.CursorParameter
		if parameter == "" {
			parameter = "cursor"
		}
		query.Set(parameter, cursor)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
func financeProtocolHasNextPage(pagination *FinanceProtocolPagination, payload any, page int) bool {
	if page >= pagination.MaxPages {
		return false
	}
	if pagination.Type == "page" {
		values, _ := FinanceJSONPath(payload, "$.has_more")
		return len(values) > 0 && values[0] == true
	}
	if pagination.CursorPath == "" {
		return false
	}
	values, _ := FinanceJSONPath(payload, pagination.CursorPath)
	return len(values) > 0 && fmt.Sprint(values[0]) != ""
}
func mergeFinanceProtocolFacts(destination, source map[string]any) {
	for key, value := range source {
		if existing, ok := destination[key]; ok {
			destination[key] = appendFinanceFact(existing, value)
		} else {
			destination[key] = value
		}
	}
}
func appendFinanceFact(left, right any) any {
	l, lok := left.([]any)
	if !lok {
		l = []any{left}
	}
	if r, ok := right.([]any); ok {
		return append(l, r...)
	}
	return append(l, right)
}

func normalizeFinanceFundingTransactions(facts map[string]any) ([]FinanceFundingTransactionFact, error) {
	requiredFields := []string{"transaction_id", "paid_amount", "paid_currency", "base_credit_units", "bonus_credit_units", "occurred_at"}
	optionalFields := []string{"fx_rate_to_usd", "fx_source", "fx_observed_at"}
	count := 0
	for _, field := range requiredFields {
		if values, ok := facts[field].([]any); ok && len(values) > count {
			count = len(values)
		} else if _, exists := facts[field]; exists && count == 0 {
			count = 1
		}
	}
	transactions := make([]FinanceFundingTransactionFact, count)
	for index := 0; index < count; index++ {
		values := make(map[string]string, len(requiredFields)+len(optionalFields))
		for _, field := range requiredFields {
			value, ok := financeFactAt(facts[field], index)
			if !ok {
				return nil, fmt.Errorf("funding transaction mapping field %s has inconsistent length", field)
			}
			values[field] = fmt.Sprint(value)
		}
		for _, field := range optionalFields {
			if value, ok := financeFactAt(facts[field], index); ok {
				values[field] = fmt.Sprint(value)
			}
		}
		for _, field := range []string{"paid_amount", "base_credit_units", "bonus_credit_units"} {
			if _, err := FinanceDecimal(values[field]); err != nil {
				return nil, fmt.Errorf("funding %s is invalid: %w", field, err)
			}
		}
		if values["fx_rate_to_usd"] != "" {
			if rate, err := FinanceDecimal(values["fx_rate_to_usd"]); err != nil || rate.LessThanOrEqual(decimal.Zero) {
				return nil, fmt.Errorf("funding fx_rate_to_usd is invalid")
			}
		}
		transactions[index] = FinanceFundingTransactionFact{
			TransactionID: values["transaction_id"], PaidAmount: values["paid_amount"], PaidCurrency: strings.ToUpper(values["paid_currency"]),
			FXRateToUSD: values["fx_rate_to_usd"], FXSource: values["fx_source"], FXObservedAt: values["fx_observed_at"],
			BaseCreditUnits: values["base_credit_units"], BonusCreditUnits: values["bonus_credit_units"], OccurredAt: values["occurred_at"],
		}
	}
	return transactions, nil
}

func financeFactAt(value any, index int) (any, bool) {
	if values, ok := value.([]any); ok {
		if index < len(values) {
			return values[index], true
		}
		return nil, false
	}
	if index == 0 && value != nil {
		return value, true
	}
	return nil, false
}
func classifyFinanceProtocolError(err error) string {
	var statusErr *UpstreamFinanceHTTPStatusError
	switch {
	case errors.As(err, &statusErr):
		return "http_status"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrUpstreamFinanceProtocolUnsafe):
		return "security_policy"
	default:
		return "execution_failed"
	}
}
func sanitizeFinanceProtocolError(err error) string {
	message := err.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}
