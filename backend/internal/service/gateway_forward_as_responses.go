package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// ForwardAsResponses accepts an OpenAI Responses API request body, converts it
// to Anthropic Messages format, forwards to the Anthropic upstream, and converts
// the response back to Responses format. This enables OpenAI Responses API
// clients to access Anthropic models through Anthropic platform groups.
//
// The method follows the same pattern as OpenAIGatewayService.ForwardAsAnthropic
// but in reverse direction: Responses → Anthropic upstream → Responses.
func (s *GatewayService) ForwardAsResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *ParsedRequest,
) (*ForwardResult, error) {
	beginUpstreamResponseModelObservation(c) // V03-R11：每次转发尝试重置上游响应模型观测（本路径未接观测时保持为空，避免沿用上次尝试的值）
	startTime := time.Now()

	// 1. Lower Codex client-side tools to function tools understood by Anthropic.
	adaptedBody, clientToolMapping, err := adaptResponsesClientToolsForAnthropic(body)
	if err != nil {
		return nil, fmt.Errorf("adapt responses client tools: %w", err)
	}

	// 2. Parse Responses request
	var responsesReq apicompat.ResponsesRequest
	if err := json.Unmarshal(adaptedBody, &responsesReq); err != nil {
		return nil, fmt.Errorf("parse responses request: %w", err)
	}
	originalModel := responsesReq.Model
	clientStream := responsesReq.Stream

	// 3. Convert Responses → Anthropic
	anthropicReq, err := apicompat.ResponsesToAnthropicRequest(&responsesReq)
	if err != nil {
		writeResponsesError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return nil, fmt.Errorf("convert responses to anthropic: %w", err)
	}

	// 3. Force upstream streaming (Anthropic works best with streaming)
	anthropicReq.Stream = true
	reqStream := true

	// 4. Model mapping
	mappedModel := originalModel
	reasoningEffort := ExtractResponsesReasoningEffortFromBody(body)
	if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
		mappedModel = account.GetMappedModel(originalModel)
	}
	if mappedModel == originalModel && account.Platform == PlatformAnthropic && account.Type == AccountTypeServiceAccount {
		normalized := normalizeVertexAnthropicModelID(claude.NormalizeModelID(originalModel))
		if normalized != originalModel {
			mappedModel = normalized
		}
	} else if mappedModel == originalModel && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		normalized := claude.NormalizeModelID(originalModel)
		if normalized != originalModel {
			mappedModel = normalized
		}
	}
	anthropicReq.Model = mappedModel

	logger.L().Debug("gateway forward_as_responses: model mapping applied",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("mapped_model", mappedModel),
		zap.Bool("client_stream", clientStream),
	)

	// 5. Marshal Anthropic request body
	anthropicBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}
	if sc := StreamContinuationFromContext(c); sc.Active() {
		anthropicBody = sc.BuildAnthropicContinuationBody(anthropicBody)
	}

	// 6. Apply Claude Code mimicry for OAuth accounts (non-Claude-Code endpoints).
	// OpenAI Responses 协议进来的请求永远不是 Claude Code 客户端，所以对 OAuth 账号
	// 必须完整执行 /v1/messages 主路径上的伪装链路（system 重写 + normalize + metadata 注入），
	// 否则会被 Anthropic 判为第三方应用并扣 extra usage。
	// 见 applyClaudeCodeOAuthMimicryToBody 的 godoc。
	isClaudeCode := false
	shouldMimicClaudeCode := account.IsOAuth() && !isClaudeCode

	if shouldMimicClaudeCode {
		anthropicBody = s.applyClaudeCodeOAuthMimicryToBody(ctx, c, account, anthropicBody, anthropicReq.System, mappedModel)
	}

	// 7. Enforce cache_control block limit
	anthropicBody = enforceCacheControlLimit(anthropicBody)

	// 8. Get access token
	token, tokenType, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}

	// 9. Get proxy URL
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	// 10–11. Rebuild each attempt with its own upstream context and the native
	// Messages same-account retry budget; pool retries remain handler-owned.
	upstreamStart := time.Now()
	resp, err := s.doAnthropicCompatUpstream(ctx, c, account, func() (*http.Response, error) {
		upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, reqStream)
		upstreamReq, buildErr := s.buildUpstreamRequest(upstreamCtx, c, account, anthropicBody, token, tokenType, mappedModel, reqStream, shouldMimicClaudeCode)
		releaseUpstreamCtx()
		if buildErr != nil {
			return nil, fmt.Errorf("build upstream request: %w", buildErr)
		}
		return s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	})
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if errors.Is(err, context.Canceled) || ctx.Err() != nil || c.Request.Context().Err() != nil {
			return nil, err
		}
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		if foErr := newAnyErrorTransportFailover(ctx, s.cfg, err); foErr != nil {
			return nil, foErr
		}
		writeResponsesError(c, http.StatusBadGateway, "server_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	// Only configured pool retry statuses bypass service retries, as in /messages.
	if resp.StatusCode >= 400 && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode) && !peekUserContentRejection(resp) {
		return nil, poolModeStatusFailover(resp, account)
	}

	// 12. Handle error response with failover
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)

		// 与原生 Messages 路径共用账号错误策略；即非默认 failover 状态码也执行一次。
		shouldDisable := false
		if s.rateLimitService != nil {
			shouldDisable = s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, mappedModel)
		}
		if shouldDisable || (s.shouldFailoverUpstreamError(ctx, resp.StatusCode) && !isUserContentRejectionBody(respBody)) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: false,
			}
		}

		// Native Messages uses 502 for a retry-exhausted, non-failover error.
		statusCode := mapUpstreamStatusCode(resp.StatusCode)
		if s.shouldRetryUpstreamError(account, resp.StatusCode) && !isUserContentRejectionBody(respBody) {
			statusCode = http.StatusBadGateway
		}
		writeResponsesError(c, statusCode, "server_error", upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	// 13. Handle normal response (convert Anthropic → Responses)
	var result *ForwardResult
	var handleErr error
	if clientStream {
		result, handleErr = s.handleResponsesStreamingResponse(resp, c, originalModel, mappedModel, reasoningEffort, startTime, clientToolMapping)
	} else {
		result, handleErr = s.handleResponsesBufferedStreamingResponse(resp, c, originalModel, mappedModel, reasoningEffort, startTime, clientToolMapping)
	}

	return result, handleErr
}

func adaptResponsesClientToolsForAnthropic(body []byte) ([]byte, apicompat.ResponsesClientToolMapping, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var requestBody map[string]any
	if err := decoder.Decode(&requestBody); err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, err
	}
	additionalToolsChanged, err := liftResponsesAdditionalTools(requestBody)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, err
	}

	mapping, changed, err := apicompat.AdaptResponsesClientTools(requestBody)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, err
	}
	changed = changed || additionalToolsChanged
	if !changed {
		return body, mapping, nil
	}
	rebuilt, err := json.Marshal(requestBody)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, err
	}
	return rebuilt, mapping, nil
}

func liftResponsesAdditionalTools(requestBody map[string]any) (bool, error) {
	input, ok := requestBody["input"].([]any)
	if !ok {
		return false, nil
	}

	tools, _ := requestBody["tools"].([]any)
	kept := make([]any, 0, len(input))
	changed := false
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(fmt.Sprint(item["type"])) != "additional_tools" {
			kept = append(kept, raw)
			continue
		}
		additional, ok := item["tools"].([]any)
		if !ok {
			return false, fmt.Errorf("additional_tools.tools must be an array")
		}
		tools = append(tools, additional...)
		changed = true
	}
	if !changed {
		return false, nil
	}
	requestBody["tools"] = tools
	requestBody["input"] = kept
	return true, nil
}

// ExtractResponsesReasoningEffortFromBody reads Responses API reasoning.effort
// and normalizes it for usage logging.
func ExtractResponsesReasoningEffortFromBody(body []byte) *string {
	raw := strings.TrimSpace(gjson.GetBytes(body, "reasoning.effort").String())
	if raw == "" {
		return nil
	}
	normalized := normalizeOpenAIReasoningEffort(raw)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func mergeAnthropicUsage(dst *ClaudeUsage, src apicompat.AnthropicUsage) {
	if dst == nil {
		return
	}
	if src.InputTokens > 0 {
		dst.InputTokens = src.InputTokens
	}
	if src.OutputTokens > 0 {
		dst.OutputTokens = src.OutputTokens
	}
	if src.CacheReadInputTokens > 0 {
		dst.CacheReadInputTokens = src.CacheReadInputTokens
	}
	if src.CacheCreationInputTokens > 0 {
		dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	}
}

// handleResponsesBufferedStreamingResponse reads all Anthropic SSE events from
// the upstream streaming response, assembles them into a complete Anthropic
// response, converts to Responses API JSON format, and writes it to the client.
func (s *GatewayService) handleResponsesBufferedStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	mappedModel string,
	reasoningEffort *string,
	startTime time.Time,
	clientToolMapping apicompat.ResponsesClientToolMapping,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	// Accumulate the final Anthropic response from streaming events
	var finalResp *apicompat.AnthropicResponse
	var usage ClaudeUsage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "event: ") {
			continue
		}
		eventType := strings.TrimPrefix(line, "event: ")

		// Read the data line
		if !scanner.Scan() {
			break
		}
		dataLine := scanner.Text()
		if !strings.HasPrefix(dataLine, "data: ") {
			continue
		}
		payload := dataLine[6:]

		var event apicompat.AnthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			logger.L().Warn("forward_as_responses buffered: failed to parse event",
				zap.Error(err),
				zap.String("request_id", requestID),
				zap.String("event_type", eventType),
			)
			continue
		}

		// message_start carries the initial response structure
		if event.Type == "message_start" && event.Message != nil {
			finalResp = event.Message
			mergeAnthropicUsage(&usage, event.Message.Usage)
		}

		// message_delta carries final usage and stop_reason
		if event.Type == "message_delta" {
			if event.Usage != nil {
				mergeAnthropicUsage(&usage, *event.Usage)
			}
			if event.Delta != nil && event.Delta.StopReason != "" && finalResp != nil {
				finalResp.StopReason = apicompat.AnthropicStopReasonPtr(event.Delta.StopReason)
			}
		}

		// Accumulate content blocks
		if event.Type == "content_block_start" && event.ContentBlock != nil && finalResp != nil {
			finalResp.Content = append(finalResp.Content, *event.ContentBlock)
		}
		if event.Type == "content_block_delta" && event.Delta != nil && finalResp != nil && event.Index != nil {
			idx := *event.Index
			if idx >= 0 && idx < len(finalResp.Content) {
				switch event.Delta.Type {
				case "text_delta":
					finalResp.Content[idx].Text += event.Delta.Text
				case "thinking_delta":
					finalResp.Content[idx].Thinking += event.Delta.Thinking
				case "input_json_delta":
					finalResp.Content[idx].Input = appendRawJSON(finalResp.Content[idx].Input, event.Delta.PartialJSON)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("forward_as_responses buffered: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}

	if finalResp == nil {
		writeResponsesError(c, http.StatusBadGateway, "server_error", "Upstream stream ended without a response")
		return nil, fmt.Errorf("upstream stream ended without response")
	}

	// Update usage from accumulated delta
	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		finalResp.Usage = apicompat.AnthropicUsage{
			InputTokens:              usage.InputTokens,
			OutputTokens:             usage.OutputTokens,
			CacheCreationInputTokens: usage.CacheCreationInputTokens,
			CacheReadInputTokens:     usage.CacheReadInputTokens,
		}
	}

	// Convert to Responses format
	responsesResp := apicompat.AnthropicToResponsesResponse(finalResp)
	responsesResp.Model = originalModel // Use original model name

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	if respBytes, err := json.Marshal(responsesResp); err == nil {
		respBytes = reverseToolNamesIfPresent(c, respBytes)
		respBytes, _, err = apicompat.RestoreResponsesClientToolPayload(respBytes, clientToolMapping)
		if err != nil {
			return nil, fmt.Errorf("restore responses client tools: %w", err)
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", respBytes)
	} else {
		c.JSON(http.StatusOK, responsesResp)
	}

	return &ForwardResult{
		RequestID:       requestID,
		Usage:           usage,
		Model:           originalModel,
		UpstreamModel:   mappedModel,
		ReasoningEffort: reasoningEffort,
		Stream:          false,
		Duration:        time.Since(startTime),
	}, nil
}

// handleResponsesStreamingResponse reads Anthropic SSE events from upstream,
// converts each to Responses SSE events, and writes them to the client.
func (s *GatewayService) handleResponsesStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	mappedModel string,
	reasoningEffort *string,
	startTime time.Time,
	clientToolMapping apicompat.ResponsesClientToolMapping,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	state := apicompat.NewAnthropicEventToResponsesState()
	state.Model = originalModel
	clientToolRestorer := apicompat.NewResponsesClientToolStreamRestorer(clientToolMapping)
	var usage ClaudeUsage
	var firstTokenMs *int
	firstChunk := true
	// 客户端断开后停止向下游写入，继续读完上游以合并 message_delta 的最终 usage（上游 94c7d6b02）。
	clientDisconnected := false
	streamCont := StreamContinuationFromContext(c)
	streamCont.BeginAttempt()

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	resultWithUsage := func() *ForwardResult {
		return &ForwardResult{
			RequestID:        requestID,
			Usage:            usage,
			Model:            originalModel,
			UpstreamModel:    mappedModel,
			ReasoningEffort:  reasoningEffort,
			Stream:           true,
			Duration:         time.Since(startTime),
			FirstTokenMs:     firstTokenMs,
			ClientDisconnect: clientDisconnected,
		}
	}

	writeEvent := func(evt apicompat.ResponsesStreamEvent) bool {
		payload, err := json.Marshal(evt)
		if err != nil {
			logger.L().Warn("forward_as_responses stream: failed to marshal event",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			return true
		}
		payload = reverseToolNamesIfPresent(c, payload)
		payloads, _, err := clientToolRestorer.RestoreEvent(payload)
		if err != nil {
			logger.L().Warn("forward_as_responses stream: failed to restore client tools",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			return true
		}
		for _, restored := range payloads {
			eventType := gjson.GetBytes(restored, "type").String()
			if streamCont != nil {
				startsOutput := eventType != "response.created" && eventType != "response.in_progress"
				rewritten, keep := streamCont.RewriteResponsesData(string(restored), startsOutput)
				if !keep {
					continue
				}
				restored = []byte(rewritten)
			}
			if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, restored); err != nil {
				clientDisconnected = true
				logger.L().Info("forward_as_responses stream: client disconnected, continuing to drain upstream for billing",
					zap.String("request_id", requestID),
				)
				return false
			}
		}
		return true
	}

	// processEvent handles a single parsed Anthropic SSE event.
	processEvent := func(event *apicompat.AnthropicStreamEvent) {
		if firstChunk {
			firstChunk = false
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}

		// Extract usage from message_delta
		if event.Type == "message_delta" && event.Usage != nil {
			mergeAnthropicUsage(&usage, *event.Usage)
		}
		// Also capture usage from message_start
		if event.Type == "message_start" && event.Message != nil {
			mergeAnthropicUsage(&usage, event.Message.Usage)
		}
		if clientDisconnected {
			return
		}

		// Convert to Responses events
		events := apicompat.AnthropicEventToResponsesEvents(event, state)
		for _, evt := range events {
			if !writeEvent(evt) {
				return
			}
		}
		if len(events) > 0 {
			c.Writer.Flush()
		}
	}

	finalizeStream := func() (*ForwardResult, error) {
		if clientDisconnected {
			return resultWithUsage(), nil
		}
		if finalEvents := apicompat.FinalizeAnthropicResponsesStream(state); len(finalEvents) > 0 {
			for _, evt := range finalEvents {
				if !writeEvent(evt) {
					return resultWithUsage(), nil
				}
			}
			c.Writer.Flush()
		}
		return resultWithUsage(), nil
	}

	// Read Anthropic SSE events
	interruptForContinuation := func(message string) *UpstreamFailoverError {
		if streamCont == nil || clientDisconnected || !streamCont.Active() || !streamCont.Recoverable() {
			return nil
		}
		foErr := streamCont.NewStreamInterruptedFailover(message)
		attachObservedClaudeUsage(foErr, &usage, originalModel, mappedModel)
		return foErr
	}
	sawMessageStop := false
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "event: ") {
			continue
		}
		eventType := strings.TrimPrefix(line, "event: ")

		// Read data line
		if !scanner.Scan() {
			break
		}
		dataLine := scanner.Text()
		if !strings.HasPrefix(dataLine, "data: ") {
			continue
		}
		payload := dataLine[6:]

		var event apicompat.AnthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			logger.L().Warn("forward_as_responses stream: failed to parse event",
				zap.Error(err),
				zap.String("request_id", requestID),
				zap.String("event_type", eventType),
			)
			continue
		}
		if event.Type == "content_block_delta" && event.Index != nil && *event.Index < 0 {
			continue
		}
		if eventType == "error" || event.Type == "error" {
			foErr := &UpstreamFailoverError{
				StatusCode:   http.StatusBadGateway,
				ResponseBody: []byte(payload),
			}
			if s.shouldFailoverUpstreamError(c.Request.Context(), http.StatusBadGateway) {
				if continuationErr := interruptForContinuation("upstream sent error event mid-stream"); continuationErr != nil {
					return nil, continuationErr
				}
			}
			return nil, foErr
		}
		if event.Type == "message_stop" {
			sawMessageStop = true
		}

		processEvent(&event)
	}

	if err := scanner.Err(); err != nil && !sawMessageStop {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("forward_as_responses stream: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			if foErr := interruptForContinuation("upstream stream disconnected: " + sanitizeStreamError(err)); foErr != nil {
				return nil, foErr
			}
		}
	}
	if !sawMessageStop {
		if foErr := interruptForContinuation("upstream stream ended without message_stop"); foErr != nil {
			return nil, foErr
		}
	}

	return finalizeStream()
}

// appendRawJSON appends a JSON fragment string to existing raw JSON.
func appendRawJSON(existing json.RawMessage, fragment string) json.RawMessage {
	if len(existing) == 0 {
		return json.RawMessage(fragment)
	}
	return json.RawMessage(string(existing) + fragment)
}

// writeResponsesError writes an error response in OpenAI Responses API format.
func writeResponsesError(c *gin.Context, statusCode int, code, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

// mapUpstreamStatusCode maps upstream HTTP status codes to appropriate client-facing codes.
func mapUpstreamStatusCode(code int) int {
	if code >= 500 {
		return http.StatusBadGateway
	}
	return code
}
