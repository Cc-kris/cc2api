package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayServiceForward_RejectsDisabledImageGenerationIntents(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "image model",
			body: []byte(`{"model":"gpt-image-2","input":"draw"}`),
		},
		{
			name: "image tool choice",
			body: []byte(`{"model":"gpt-5.4","input":"draw","tool_choice":{"type":"image_generation"}}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{}
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, recorder := newOpenAIImageGenerationControlTestContext(false, "unit-test-agent/1.0")
			account := newOpenAIImageGenerationControlTestAccount()

			result, err := svc.Forward(context.Background(), c, account, tt.body)

			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, http.StatusForbidden, recorder.Code)
			require.Equal(t, "permission_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
			require.Nil(t, upstream.lastReq, "disabled image request must not reach upstream")
		})
	}
}

func TestOpenAIGatewayServiceForward_DisabledGroupStripsImageToolDeclarationOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_text_with_tools","model":"gpt-5.4","usage":{"input_tokens":4,"output_tokens":2}}`)),
		},
	}
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, recorder := newOpenAIImageGenerationControlTestContext(false, "Codex Desktop/0.137.0-alpha.4")
	account := newOpenAIImageGenerationControlTestAccount()
	body := []byte(`{"model":"gpt-5.4","input":"write code","stream":false,"tools":[{"type":"image_generation","format":"png"},{"type":"function","name":"shell"}]}`)

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, upstream.lastReq)
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
	require.True(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="function")`).Exists())
	require.Equal(t, 0, result.ImageCount)
}

func TestOpenAIGatewayServiceForward_DisabledGroupAllowsTextOnlyResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_text","model":"gpt-5.4","usage":{"input_tokens":3,"output_tokens":2}}`)),
		},
	}
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, recorder := newOpenAIImageGenerationControlTestContext(false, "unit-test-agent/1.0")
	account := newOpenAIImageGenerationControlTestAccount()

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","input":"write code","stream":false}`))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 0, result.ImageCount)
	require.NotNil(t, upstream.lastReq)
}

func TestOpenAIGatewayServiceForward_CodexImageBridgeDoesNotInjectOrDirectToImages(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_passthrough_image","model":"gpt-5.4","usage":{"input_tokens":2,"output_tokens":1}}`)),
		},
	}
	svc := newOpenAIImageGenerationControlTestService(upstream)
	svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = true
	c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.98.0")
	account := newOpenAIImageGenerationControlTestAccount()
	body := []byte(`{"model":"gpt-5.4","input":"draw","stream":false,"tools":[{"type":"image_generation","format":"jpeg"}]}`)

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Contains(t, upstream.lastReq.URL.Path, "/v1/responses")
	require.NotContains(t, upstream.lastReq.URL.Path, "/v1/images/generations")
	imageTools := gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")#`).Array()
	require.Len(t, imageTools, 1)
	require.Equal(t, "jpeg", gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation").output_format`).String())
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation").format`).Exists())
	instructions := gjson.GetBytes(upstream.lastBody, "instructions").String()
	require.NotContains(t, instructions, "image_generation")
	require.Equal(t, 0, result.ImageCount)
}

func TestOpenAIGatewayService_CodexImageGenerationBridgeOverridePrecedence(t *testing.T) {
	groupID := int64(4242)

	tests := []struct {
		name    string
		global  bool
		channel *Channel
		account *Account
		want    bool
	}{
		{
			name:   "global default enables bridge",
			global: true,
			account: &Account{
				Platform: PlatformOpenAI,
			},
			want: true,
		},
		{
			name:   "channel true overrides disabled global",
			global: false,
			channel: &Channel{ID: 1, Status: StatusActive, FeaturesConfig: map[string]any{
				featureKeyCodexImageGenerationBridge: map[string]any{PlatformOpenAI: true},
			}},
			account: &Account{Platform: PlatformOpenAI},
			want:    true,
		},
		{
			name:   "channel false overrides enabled global",
			global: true,
			channel: &Channel{ID: 1, Status: StatusActive, FeaturesConfig: map[string]any{
				featureKeyCodexImageGenerationBridge: map[string]any{PlatformOpenAI: false},
			}},
			account: &Account{Platform: PlatformOpenAI},
			want:    false,
		},
		{
			name:   "account false overrides channel and global true",
			global: true,
			channel: &Channel{ID: 1, Status: StatusActive, FeaturesConfig: map[string]any{
				featureKeyCodexImageGenerationBridge: map[string]any{PlatformOpenAI: true},
			}},
			account: &Account{
				Platform: PlatformOpenAI,
				Extra:    map[string]any{featureKeyCodexImageGenerationBridge: false},
			},
			want: false,
		},
		{
			name:   "nested account true overrides channel false",
			global: false,
			channel: &Channel{ID: 1, Status: StatusActive, FeaturesConfig: map[string]any{
				featureKeyCodexImageGenerationBridge: map[string]any{PlatformOpenAI: false},
			}},
			account: &Account{
				Platform: PlatformOpenAI,
				Extra: map[string]any{
					PlatformOpenAI: map[string]any{"codex_image_generation_bridge_enabled": true},
				},
			},
			want: true,
		},
		{
			name:   "non openai account extra is ignored",
			global: false,
			account: &Account{
				Platform: PlatformAnthropic,
				Extra:    map[string]any{featureKeyCodexImageGenerationBridge: true},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
			svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = tt.global
			if tt.channel != nil {
				svc.channelService = newOpenAIImageGenerationControlChannelService(groupID, tt.channel)
			}
			apiKey := &APIKey{GroupID: &groupID}

			got := svc.isCodexImageGenerationBridgeEnabled(context.Background(), tt.account, apiKey)

			require.Equal(t, tt.want, got)
		})
	}
}

func TestOpenAIGatewayServiceHandleResponsesImageOutputs_NonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
	c, _ := newOpenAIImageGenerationControlTestContext(true, "unit-test-agent/1.0")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_image_json",
			"model":"gpt-5.4",
			"output":[{"id":"ig_json_1","type":"image_generation_call","result":"final-image"}],
			"usage":{"input_tokens":7,"output_tokens":3,"output_tokens_details":{"image_tokens":2}}
		}`)),
	}

	result, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Type: AccountTypeAPIKey}, "gpt-5.4", "gpt-5.4")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.imageCount)
	require.NotNil(t, result.usage)
	require.Equal(t, 7, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.Equal(t, 2, result.usage.ImageOutputTokens)
}

func TestChannelCodexImageGenerationOrchestratorGroupID(t *testing.T) {
	channel := &Channel{FeaturesConfig: map[string]any{
		featureKeyCodexImageGenerationBridge: map[string]any{
			PlatformOpenAI:          true,
			"orchestrator_group_id": float64(7),
		},
	}}
	require.NotNil(t, channel.CodexImageGenerationOrchestratorGroupID())
	require.Equal(t, int64(7), *channel.CodexImageGenerationOrchestratorGroupID())

	channel.FeaturesConfig[featureKeyCodexImageGenerationBridge] = map[string]any{
		PlatformOpenAI:          true,
		"orchestrator_group_id": "14",
	}
	require.Equal(t, int64(14), *channel.CodexImageGenerationOrchestratorGroupID())
}

func TestOpenAIGatewayServiceHandleResponsesImageOutputs_Streaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
	c, _ := newOpenAIImageGenerationControlTestContext(true, "unit-test-agent/1.0")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_stream_1\",\"type\":\"image_generation_call\",\"result\":\"final-image\"}}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_image_stream\",\"model\":\"gpt-5.5\",\"output\":[{\"id\":\"ig_stream_1\",\"type\":\"image_generation_call\",\"result\":\"final-image\"}],\"usage\":{\"input_tokens\":11,\"output_tokens\":5,\"output_tokens_details\":{\"image_tokens\":4}}}}\n\n",
		)),
	}

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "gpt-5.5", "gpt-5.5")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.imageCount)
	require.NotNil(t, result.usage)
	require.Equal(t, 11, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)
	require.Equal(t, 4, result.usage.ImageOutputTokens)
}

func TestOpenAIGatewayServiceHandleStreamingResponsePassthrough_PreservesCodexImageGenerationCall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newResponse := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"created_at\":1710000000}}\n\n" +
					"event: response.image_generation_call.partial_image\ndata: {\"type\":\"response.image_generation_call.partial_image\",\"partial_image_b64\":\"partial-image\",\"partial_image_index\":0,\"output_format\":\"png\"}\n\n" +
					"event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_1\",\"type\":\"image_generation_call\",\"status\":\"generating\",\"revised_prompt\":\"test image\",\"result\":\"final-image\",\"output_format\":\"png\"}}\n\n" +
					"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[{\"id\":\"ig_1\",\"type\":\"image_generation_call\",\"status\":\"generating\",\"result\":\"final-image\",\"output_format\":\"png\"}]}}\n\n",
			)),
		}
	}

	for _, mode := range []string{"Codex mode", "Work mode"} {
		t.Run(mode, func(t *testing.T) {
			svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
			c, recorder := newOpenAIImageGenerationControlTestContext(true, "Codex Desktop/0.144.0-alpha.4")
			result, err := svc.handleStreamingResponsePassthrough(context.Background(), newResponse(), c, &Account{ID: 1}, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1, result.imageCount)
			assertCodexResponsesImageGenerationCallPreserved(t, recorder.Body.String())
		})
	}
}

func TestOpenAIGatewayServiceHandleStreamingResponse_PreservesCodexImageGenerationCall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newResponse := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"created_at\":1710000000}}\n\n" +
					"event: response.image_generation_call.partial_image\ndata: {\"type\":\"response.image_generation_call.partial_image\",\"partial_image_b64\":\"partial-image\",\"partial_image_index\":0,\"output_format\":\"png\"}\n\n" +
					"event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_1\",\"type\":\"image_generation_call\",\"status\":\"generating\",\"revised_prompt\":\"test image\",\"result\":\"final-image\",\"output_format\":\"png\"}}\n\n" +
					"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[{\"id\":\"ig_1\",\"type\":\"image_generation_call\",\"status\":\"generating\",\"result\":\"final-image\",\"output_format\":\"png\"}]}}\n\n",
			)),
		}
	}

	for _, mode := range []string{"Codex mode", "Work mode"} {
		t.Run(mode, func(t *testing.T) {
			svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
			c, recorder := newOpenAIImageGenerationControlTestContext(true, "Codex Desktop/0.144.0-alpha.4")
			result, err := svc.handleStreamingResponse(context.Background(), newResponse(), c, &Account{ID: 1}, time.Now(), "gpt-5.6-sol", "gpt-5.6-sol")

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1, result.imageCount)
			assertCodexResponsesImageGenerationCallPreserved(t, recorder.Body.String())
		})
	}
}

func assertCodexResponsesImageGenerationCallPreserved(t *testing.T, body string) {
	t.Helper()
	require.Contains(t, body, `"type":"response.image_generation_call.partial_image"`)
	require.Contains(t, body, `"partial_image_b64":"partial-image"`)
	require.Contains(t, body, `"type":"response.output_item.done"`)
	require.Contains(t, body, `"id":"ig_1"`)
	require.Contains(t, body, `"type":"image_generation_call"`)
	require.Contains(t, body, `"status":"completed"`)
	require.NotContains(t, body, `"status":"generating"`)
	require.Contains(t, body, `"revised_prompt":"test image"`)
	require.Contains(t, body, `"result":"final-image"`)
	require.NotContains(t, body, "event: image_generation.completed")
	require.NotContains(t, body, `"b64_json"`)
	require.Contains(t, body, `"type":"response.completed"`)
}

func TestNormalizeCodexImageGenerationFunctionCallNamespace(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		path      string
		wantValue string
	}{
		{
			name:      "output item done",
			payload:   `{"type":"response.output_item.done","item":{"type":"function_call","name":"imagegen","call_id":"call_1","arguments":"{}"}}`,
			path:      "item.namespace",
			wantValue: "image_gen",
		},
		{
			name:      "completed response",
			payload:   `{"type":"response.completed","response":{"output":[{"type":"function_call","name":"imagegen","call_id":"call_1","arguments":"{}"}]}}`,
			path:      "response.output.0.namespace",
			wantValue: "image_gen",
		},
		{
			name:      "non streaming response",
			payload:   `{"output":[{"type":"function_call","name":"image_gen__imagegen","call_id":"call_1","arguments":"{}"}]}`,
			path:      "output.0.namespace",
			wantValue: "image_gen",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, changed := normalizeCodexImageGenerationFunctionCallNamespace([]byte(tt.payload))
			require.True(t, changed)
			require.Equal(t, tt.wantValue, gjson.GetBytes(normalized, tt.path).String())
			require.Equal(t, "imagegen", gjson.GetBytes(normalized, strings.TrimSuffix(tt.path, ".namespace")+".name").String())
		})
	}

	unchanged, changed := normalizeCodexImageGenerationFunctionCallNamespace([]byte(`{"output":[{"type":"function_call","name":"shell"}]}`))
	require.False(t, changed)
	require.JSONEq(t, `{"output":[{"type":"function_call","name":"shell"}]}`, string(unchanged))
}

func TestNormalizeCompletedImageGenerationSSEData(t *testing.T) {
	largeResult := strings.Repeat("a", 2_838_984)
	tests := []struct {
		name        string
		payload     string
		wantChanged bool
		wantStatus  string
	}{
		{
			name:        "real sized final image marked generating",
			payload:     `{"type":"response.output_item.done","item":{"id":"ig_1","type":"image_generation_call","status":"generating","result":"` + largeResult + `"}}`,
			wantChanged: true,
			wantStatus:  "completed",
		},
		{
			name:        "completed response output marked generating",
			payload:     `{"type":"response.completed","response":{"output":[{"id":"ig_1","type":"image_generation_call","status":"generating","result":"final-image"}]}}`,
			wantChanged: true,
			wantStatus:  "completed",
		},
		{
			name:        "in progress image without final result",
			payload:     `{"type":"response.output_item.added","item":{"id":"ig_1","type":"image_generation_call","status":"generating"}}`,
			wantChanged: false,
			wantStatus:  "generating",
		},
		{
			name:        "title text response",
			payload:     `{"type":"response.output_item.done","item":{"id":"msg_1","type":"message","status":"completed"}}`,
			wantChanged: false,
			wantStatus:  "completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, changed := normalizeCompletedImageGenerationSSEData([]byte(tt.payload))
			require.Equal(t, tt.wantChanged, changed)
			if strings.Contains(tt.payload, `"type":"response.completed"`) {
				require.Equal(t, tt.wantStatus, gjson.GetBytes(normalized, "response.output.0.status").String())
			} else {
				require.Equal(t, tt.wantStatus, gjson.GetBytes(normalized, "item.status").String())
			}
			if strings.Contains(tt.payload, largeResult) {
				require.Equal(t, largeResult, gjson.GetBytes(normalized, "item.result").String())
			}
		})
	}
}

func newOpenAIImageGenerationControlTestService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	cfg := &config.Config{}
	return &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}
}

func newOpenAIImageGenerationControlChannelService(groupID int64, ch *Channel) *ChannelService {
	svc := &ChannelService{}
	cache := newEmptyChannelCache()
	if ch != nil {
		cache.channelByGroupID[groupID] = ch
		cache.byID[ch.ID] = ch
	}
	cache.loadedAt = time.Now()
	svc.cache.Store(cache)
	return svc
}

func newOpenAIImageGenerationControlTestContext(allowImages bool, userAgent string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", userAgent)
	groupID := int64(4242)
	c.Set("api_key", &APIKey{
		ID:      2424,
		GroupID: &groupID,
		Group: &Group{
			ID:                   groupID,
			AllowImageGeneration: allowImages,
			RateMultiplier:       1,
			ImageRateMultiplier:  1,
		},
	})
	return c, recorder
}

func newOpenAIImageGenerationControlTestAccount() *Account {
	return &Account{
		ID:          5151,
		Name:        "openai-image-controls",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
	}
}
