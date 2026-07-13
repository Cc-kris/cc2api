package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestOpenAIGatewayService_CodexImageGenerationRouteIsChannelOwned(t *testing.T) {
	groupID := int64(4242)
	orchestratorID := int64(7)
	channel := &Channel{
		ID: 1, Status: StatusActive, GroupIDs: []int64{groupID},
		ModelMapping: map[string]map[string]string{PlatformOpenAI: {"gpt-5.6": "gpt-image-2"}},
		FeaturesConfig: map[string]any{featureKeyCodexImageGenerationBridge: map[string]any{
			PlatformOpenAI: true, "orchestrator_group_id": orchestratorID,
		}},
	}
	svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
	svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = false
	svc.channelService = newOpenAIImageGenerationControlChannelService(groupID, channel)
	route := svc.ResolveCodexImageGenerationRoute(context.Background(), &groupID, "gpt-5.6")
	require.True(t, route.Enabled)
	require.Equal(t, "gpt-image-2", route.Mapping.MappedModel)
	require.Equal(t, orchestratorID, *route.OrchestratorGroupID)

	t.Run("enabled image mapping reports missing orchestrator as configuration error", func(t *testing.T) {
		invalidChannel := channel.Clone()
		invalidChannel.FeaturesConfig = map[string]any{featureKeyCodexImageGenerationBridge: map[string]any{
			PlatformOpenAI: true,
		}}
		invalidService := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
		invalidService.channelService = newOpenAIImageGenerationControlChannelService(groupID, invalidChannel)
		invalidRoute := invalidService.ResolveCodexImageGenerationRoute(context.Background(), &groupID, "gpt-5.6")
		require.False(t, invalidRoute.Enabled)
		require.Equal(t, "Codex image orchestration group is not configured", invalidRoute.ConfigurationError)
	})

	// Deprecated global/account values cannot grant or deny the route.
	svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = true
	channel.FeaturesConfig[featureKeyCodexImageGenerationBridge] = map[string]any{PlatformOpenAI: false, "orchestrator_group_id": orchestratorID}
	svc.channelService = newOpenAIImageGenerationControlChannelService(groupID, channel)
	route = svc.ResolveCodexImageGenerationRoute(context.Background(), &groupID, "gpt-5.6")
	require.False(t, route.Enabled)

	t.Run("ordinary groups and non image mappings stay ordinary", func(t *testing.T) {
		plainService := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
		plainService.cfg.Gateway.CodexImageGenerationBridgeEnabled = true
		require.False(t, plainService.ResolveCodexImageGenerationRoute(context.Background(), &groupID, "gpt-5.6").Enabled)

		textChannel := &Channel{
			ID: 2, Status: StatusActive, GroupIDs: []int64{groupID},
			ModelMapping: map[string]map[string]string{PlatformOpenAI: {"gpt-5.6": "gpt-5.5"}},
			FeaturesConfig: map[string]any{featureKeyCodexImageGenerationBridge: map[string]any{
				PlatformOpenAI: true, "orchestrator_group_id": orchestratorID,
			}},
		}
		plainService.channelService = newOpenAIImageGenerationControlChannelService(groupID, textChannel)
		require.False(t, plainService.ResolveCodexImageGenerationRoute(context.Background(), &groupID, "gpt-5.6").Enabled)
		require.False(t, plainService.ResolveCodexImageGenerationRoute(context.Background(), &groupID, "gpt-image-2").Enabled)

		textChannel.Status = StatusDisabled
		plainService.channelService = newOpenAIImageGenerationControlChannelService(groupID, textChannel)
		require.False(t, plainService.ResolveCodexImageGenerationRoute(context.Background(), &groupID, "gpt-5.6").Enabled)
	})
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

func TestOpenAIGatewayServiceStreamingPathsNormalizeCodexImageToolCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newResponse := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"name\":\"imagegen\",\"call_id\":\"call_1\",\"arguments\":\"{\\\"prompt\\\":\\\"new image\\\",\\\"referenced_image_paths\\\":[],\\\"num_last_images_to_include\\\":0}\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[{\"type\":\"function_call\",\"name\":\"imagegen\",\"call_id\":\"call_1\",\"arguments\":\"{\\\"prompt\\\":\\\"new image\\\",\\\"referenced_image_paths\\\":[],\\\"num_last_images_to_include\\\":0}\"}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n",
			)),
		}
	}

	for _, tc := range []struct {
		name string
		run  func(*OpenAIGatewayService, *http.Response, *gin.Context) error
	}{
		{
			name: "passthrough",
			run: func(svc *OpenAIGatewayService, resp *http.Response, c *gin.Context) error {
				_, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "gpt-5.6-terra", "gpt-5.6-terra")
				return err
			},
		},
		{
			name: "standard",
			run: func(svc *OpenAIGatewayService, resp *http.Response, c *gin.Context) error {
				_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "gpt-5.6-terra", "gpt-5.6-terra")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
			c, recorder := newOpenAIImageGenerationControlTestContext(true, "Codex Desktop/0.144.0-alpha.4")
			c.Set(OpenAICodexImageGenerationExtensionContextKey, true)
			require.NoError(t, tc.run(svc, newResponse(), c))
			body := recorder.Body.String()
			require.Contains(t, body, `"namespace":"image_gen"`)
			require.Contains(t, body, `"arguments":"{\"prompt\":\"new image\"}"`)
			require.NotContains(t, body, "num_last_images_to_include")
		})
	}
}

func TestOpenAIGatewayServiceNonStreamingPathsNormalizeCodexImageToolCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newResponse := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_1","output":[{"type":"function_call","name":"imagegen","call_id":"call_1","arguments":"{\"prompt\":\"new image\",\"referenced_image_paths\":[],\"num_last_images_to_include\":0}"}],"usage":{"input_tokens":1,"output_tokens":1}}`)),
		}
	}

	for _, tc := range []struct {
		name string
		run  func(*OpenAIGatewayService, *http.Response, *gin.Context) error
	}{
		{
			name: "passthrough",
			run: func(svc *OpenAIGatewayService, resp *http.Response, c *gin.Context) error {
				_, err := svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, "gpt-5.6-terra", "gpt-5.6-terra")
				return err
			},
		},
		{
			name: "standard",
			run: func(svc *OpenAIGatewayService, resp *http.Response, c *gin.Context) error {
				_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, "gpt-5.6-terra", "gpt-5.6-terra")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
			c, recorder := newOpenAIImageGenerationControlTestContext(true, "Codex Desktop/0.144.0-alpha.4")
			c.Set(OpenAICodexImageGenerationExtensionContextKey, true)
			require.NoError(t, tc.run(svc, newResponse(), c))
			arguments := gjson.Get(recorder.Body.String(), "output.0.arguments").String()
			require.JSONEq(t, `{"prompt":"new image"}`, arguments)
			require.Equal(t, "image_gen", gjson.Get(recorder.Body.String(), "output.0.namespace").String())
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

	argumentTests := []struct {
		name          string
		arguments     string
		wantArguments string
	}{
		{
			name:          "brand new image omits empty selectors and invalid zero",
			arguments:     `{"num_last_images_to_include":0,"prompt":"麒麟头像","referenced_image_paths":[]}`,
			wantArguments: `{"prompt":"麒麟头像"}`,
		},
		{
			name:          "local edit prefers explicit paths",
			arguments:     `{"prompt":"change color","referenced_image_paths":["/tmp/a.png"],"num_last_images_to_include":1}`,
			wantArguments: `{"prompt":"change color","referenced_image_paths":["/tmp/a.png"]}`,
		},
		{
			name:          "conversation edit preserves valid count",
			arguments:     `{"prompt":"change color","num_last_images_to_include":2}`,
			wantArguments: `{"prompt":"change color","num_last_images_to_include":2}`,
		},
		{
			name:          "conversation edit clamps excessive count",
			arguments:     `{"prompt":"change color","num_last_images_to_include":9}`,
			wantArguments: `{"prompt":"change color","num_last_images_to_include":5}`,
		},
		{
			name:          "fractional count is omitted",
			arguments:     `{"prompt":"new image","num_last_images_to_include":1.5}`,
			wantArguments: `{"prompt":"new image"}`,
		},
		{
			name:          "local paths are limited to client maximum",
			arguments:     `{"prompt":"collage","referenced_image_paths":["/1.png","/2.png","/3.png","/4.png","/5.png","/6.png"]}`,
			wantArguments: `{"prompt":"collage","referenced_image_paths":["/1.png","/2.png","/3.png","/4.png","/5.png"]}`,
		},
	}
	for _, tt := range argumentTests {
		t.Run(tt.name, func(t *testing.T) {
			payload := []byte(`{"type":"response.output_item.done","item":{"type":"function_call","name":"imagegen","namespace":"image_gen","call_id":"call_1","arguments":` + strconv.Quote(tt.arguments) + `}}`)
			normalized, changed := normalizeCodexImageGenerationFunctionCallNamespace(payload)
			if tt.arguments == tt.wantArguments {
				require.False(t, changed)
			} else {
				require.True(t, changed)
			}
			require.JSONEq(t, tt.wantArguments, gjson.GetBytes(normalized, "item.arguments").String())
		})
	}
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
		if len(ch.GroupIDs) == 0 {
			ch.GroupIDs = []int64{groupID}
		}
		cache = populateChannelCache([]Channel{*ch}, map[int64]string{groupID: PlatformOpenAI})
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
