//go:build unit

package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestResponsesChatFallbackStreamCustomToolKeepsItemID(t *testing.T) {
	state := apicompat.NewChatCompletionsToResponsesStreamState("gpt-test")
	state.CustomTools = map[string]bool{"exec": true}
	var chunk apicompat.ChatCompletionsChunk
	require.NoError(t, json.Unmarshal([]byte(`{"id":"cc-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"exec","arguments":"{\"input\":\"pwd\"}"}}]}}]}`), &chunk))

	var doneID, completedID string
	events := apicompat.ChatCompletionsChunkToResponsesEvents(&chunk, state)
	events = append(events, apicompat.FinalizeChatCompletionsResponsesStream(state)...)
	for _, event := range events {
		if event.Type == "response.output_item.done" && event.Item != nil && event.Item.Type == "custom_tool_call" {
			doneID = event.Item.ID
		}
		if event.Type == "response.completed" && event.Response != nil {
			for _, item := range event.Response.Output {
				if item.Type == "custom_tool_call" {
					completedID = item.ID
				}
			}
		}
	}
	require.True(t, strings.HasPrefix(doneID, "ctc_"), doneID)
	require.Equal(t, doneID, completedID)
}

func TestResponsesChatFallbackStreamNamespaceKeepsItemID(t *testing.T) {
	state := apicompat.NewChatCompletionsToResponsesStreamState("gpt-test")
	state.NamespaceTools = map[string]apicompat.NamespacedToolName{"team__send": {Namespace: "team", Name: "send"}}
	var chunk apicompat.ChatCompletionsChunk
	require.NoError(t, json.Unmarshal([]byte(`{"id":"cc-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"team__send","arguments":"{}"}}]}}]}`), &chunk))

	var added, done string
	events := apicompat.ChatCompletionsChunkToResponsesEvents(&chunk, state)
	events = append(events, apicompat.FinalizeChatCompletionsResponsesStream(state)...)
	for _, event := range events {
		if event.Item == nil || event.Item.Name != "send" {
			continue
		}
		switch event.Type {
		case "response.output_item.added":
			added = event.Item.ID
		case "response.output_item.done":
			done = event.Item.ID
		}
	}
	require.True(t, strings.HasPrefix(added, "fc_"), added)
	require.Equal(t, added, done)
}
