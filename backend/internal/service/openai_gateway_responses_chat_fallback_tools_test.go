//go:build unit

package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const openAIResponsesChatFallbackToolsRequest = `{
	"model":"gpt-5.4","input":[
		{"type":"message","role":"user","content":"use all declared tools"},
		{"type":"additional_tools","tools":[{"type":"custom","name":"exec","format":{"type":"text"}}]}
	],
	"tools":[
		{"type":"tool_search"},
		{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"}}]}
	]
}`

func TestForwardResponses_ChatFallbackRestoresClientToolsBuffered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl_tools","model":"gpt-5.4",
			"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[
				{"id":"call_custom","type":"function","function":{"name":"exec","arguments":"{\"input\":\"pwd\"}"}},
				{"id":"call_search","type":"function","function":{"name":"tool_search","arguments":"{\"query\":\"gmail\"}"}},
				{"id":"call_namespace","type":"function","function":{"name":"collaboration__spawn_agent","arguments":"{}"}}
			]},"finish_reason":"tool_calls"}]
		}`)),
	}}
	rec := forwardOpenAIResponsesChatFallbackToolsRequest(t, upstream, false)

	require.Equal(t, "tool_search", gjson.GetBytes(upstream.lastBody, "tools.0.function.name").String())
	require.Equal(t, "collaboration__spawn_agent", gjson.GetBytes(upstream.lastBody, "tools.1.function.name").String())
	require.Equal(t, "exec", gjson.GetBytes(upstream.lastBody, "tools.2.function.name").String())
	assertOpenAIResponsesChatFallbackBufferedTool(t, rec.Body.Bytes(), 0, "custom_tool_call", "ctc_", "call_custom", "exec", "", "input", "pwd")
	assertOpenAIResponsesChatFallbackBufferedTool(t, rec.Body.Bytes(), 1, "tool_search_call", "tsc_", "call_search", "", "", "arguments.query", "gmail")
	assertOpenAIResponsesChatFallbackBufferedTool(t, rec.Body.Bytes(), 2, "function_call", "fc_", "call_namespace", "spawn_agent", "collaboration", "arguments", "{}")
}

func TestForwardResponses_ChatFallbackRestoresClientToolsStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_tools","model":"gpt-5.4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_custom","type":"function","function":{"name":"exec","arguments":"{\"input\":\"pwd\"}"}}]}}]}`,
		"",
		`data: {"id":"chatcmpl_tools","model":"gpt-5.4","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_search","type":"function","function":{"name":"tool_search","arguments":"{\"query\":\"gmail\"}"}}]}}]}`,
		"",
		`data: {"id":"chatcmpl_tools","model":"gpt-5.4","choices":[{"index":0,"delta":{"tool_calls":[{"index":2,"id":"call_namespace","type":"function","function":{"name":"collaboration__spawn_agent","arguments":"{}"}}]}}]}`,
		"",
		`data: {"id":"chatcmpl_tools","model":"gpt-5.4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	rec := forwardOpenAIResponsesChatFallbackToolsRequest(t, upstream, true)

	events := decodeOpenAIResponsesChatFallbackSSE(t, rec.Body.Bytes())
	added := make(map[string]apicompat.ResponsesOutput)
	done := make(map[string]apicompat.ResponsesOutput)
	var completed []apicompat.ResponsesOutput
	for _, event := range events {
		switch event.Type {
		case "response.output_item.added":
			require.NotNil(t, event.Item)
			added[event.Item.CallID] = *event.Item
		case "response.output_item.done":
			require.NotNil(t, event.Item)
			done[event.Item.CallID] = *event.Item
		case "response.completed":
			require.NotNil(t, event.Response)
			completed = event.Response.Output
		}
	}
	require.Len(t, added, 3)
	require.Len(t, done, 3)
	require.Len(t, completed, 3)
	for i, callID := range []string{"call_custom", "call_search", "call_namespace"} {
		expectedType := []string{"custom_tool_call", "tool_search_call", "function_call"}[i]
		prefix := []string{"ctc_", "tsc_", "fc_"}[i]
		require.Equal(t, expectedType, added[callID].Type)
		require.Equal(t, expectedType, done[callID].Type)
		require.Equal(t, expectedType, completed[i].Type)
		require.True(t, strings.HasPrefix(added[callID].ID, prefix), added[callID].ID)
		require.Equal(t, added[callID].ID, done[callID].ID)
		require.Equal(t, added[callID].ID, completed[i].ID)
	}
	require.Equal(t, "exec", done["call_custom"].Name)
	require.Equal(t, "pwd", done["call_custom"].Input)
	require.JSONEq(t, `{"query":"gmail"}`, done["call_search"].Arguments)
	require.Equal(t, "spawn_agent", done["call_namespace"].Name)
	require.Equal(t, "collaboration", done["call_namespace"].Namespace)
	require.Equal(t, "{}", done["call_namespace"].Arguments)
}

func TestForwardResponses_ChatFallbackPreservesToolChoiceAndCollisionRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	messageResponse := `{"id":"chatcmpl_ok","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
	for _, tc := range []struct {
		name         string
		body         string
		wantChoice   bool
		wantUpstream bool
	}{
		{name: "declared tool search", body: `{"model":"gpt-5.4","input":"hi","tools":[{"type":"tool_search"}],"tool_choice":{"type":"tool_search"}}`, wantChoice: true, wantUpstream: true},
		{name: "undeclared tool search", body: `{"model":"gpt-5.4","input":"hi","tools":[{"type":"function","name":"shell","parameters":{"type":"object"}}],"tool_choice":{"type":"tool_search"}}`, wantUpstream: true},
		{name: "built in collision", body: `{"model":"gpt-5.4","input":"hi","tools":[{"type":"tool_search"},{"type":"function","name":"tool_search"}]}`},
		{name: "namespace collision", body: `{"model":"gpt-5.4","input":"hi","tools":[{"type":"namespace","name":"a","tools":[{"type":"function","name":"b__c"}]},{"type":"namespace","name":"a__b","tools":[{"type":"function","name":"c"}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(messageResponse)),
			}}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(tc.body))
			_, err := (&OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}).Forward(context.Background(), c, forceChatResponsesFallbackAccount(), []byte(tc.body))
			if !tc.wantUpstream {
				require.Error(t, err)
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Contains(t, rec.Body.String(), "cannot disambiguate")
				require.Nil(t, upstream.lastReq)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantChoice, gjson.GetBytes(upstream.lastBody, "tool_choice").Exists())
			if tc.wantChoice {
				require.Equal(t, "function", gjson.GetBytes(upstream.lastBody, "tool_choice.type").String())
				require.Equal(t, "tool_search", gjson.GetBytes(upstream.lastBody, "tool_choice.function.name").String())
			}
		})
	}
}

func forwardOpenAIResponsesChatFallbackToolsRequest(t *testing.T, upstream *httpUpstreamRecorder, stream bool) *httptest.ResponseRecorder {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(openAIResponsesChatFallbackToolsRequest), &body))
	if stream {
		body["stream"] = true
	}
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(encoded))
	result, err := (&OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}).Forward(context.Background(), c, forceChatResponsesFallbackAccount(), encoded)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "/v1/chat/completions", upstream.lastReq.URL.Path)
	require.Equal(t, http.StatusOK, rec.Code)
	return rec
}

func assertOpenAIResponsesChatFallbackBufferedTool(t *testing.T, body []byte, index int, itemType, idPrefix, callID, name, namespace, valuePath, want string) {
	t.Helper()
	base := fmt.Sprintf("output.%d.", index)
	require.Equal(t, itemType, gjson.GetBytes(body, base+"type").String())
	require.True(t, strings.HasPrefix(gjson.GetBytes(body, base+"id").String(), idPrefix))
	require.Equal(t, callID, gjson.GetBytes(body, base+"call_id").String())
	require.Equal(t, name, gjson.GetBytes(body, base+"name").String())
	require.Equal(t, namespace, gjson.GetBytes(body, base+"namespace").String())
	require.Equal(t, want, gjson.GetBytes(body, base+valuePath).String())
}

func decodeOpenAIResponsesChatFallbackSSE(t *testing.T, body []byte) []apicompat.ResponsesStreamEvent {
	t.Helper()
	var events []apicompat.ResponsesStreamEvent
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		var event apicompat.ResponsesStreamEvent
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		events = append(events, event)
	}
	require.NoError(t, scanner.Err())
	return events
}
