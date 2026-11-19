package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHaiku55ResponsesEffortRegressionMatrix(t *testing.T) {
	for _, tc := range []struct {
		effort     string
		wantEffort string
	}{
		{"", "medium"},
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"xhigh", "xhigh"},
		{"max", "max"},
	} {
		t.Run("adaptive_"+tc.effort, func(t *testing.T) {
			var reasoning *ResponsesReasoning
			if tc.effort != "" {
				reasoning = &ResponsesReasoning{Effort: tc.effort}
			}
			out, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Reasoning: reasoning})
			require.NoError(t, err)
			require.Equal(t, "adaptive", out.Thinking.Type)
			require.Zero(t, out.Thinking.BudgetTokens)
			require.Equal(t, tc.wantEffort, out.OutputConfig.Effort)
		})
	}

	out, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: "none"}})
	require.NoError(t, err)
	require.Equal(t, "disabled", out.Thinking.Type)
	require.Equal(t, "low", out.OutputConfig.Effort)

	for _, choice := range []string{`"required"`, `"auto"`, `"none"`, `{"type":"function","name":"lookup"}`} {
		out, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), ToolChoice: json.RawMessage(choice)})
		require.NoError(t, err, choice)
		require.NotEmpty(t, out.ToolChoice)
	}
}

func TestHaiku55ResponsesInvalidRegressionMatrix(t *testing.T) {
	temperature, topP := 1.0, 0.99
	for _, tc := range []struct {
		name  string
		req   *ResponsesRequest
		match string
	}{
		{"minimal", &ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: "minimal"}}, "reasoning effort"},
		{"unknown", &ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Reasoning: &ResponsesReasoning{Effort: "unknown"}}, "reasoning effort"},
		{"temperature", &ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Temperature: float64PtrForAC48(0.2)}, "temperature"},
		{"top_p", &ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), TopP: float64PtrForAC48(0.3)}, "top_p"},
		{"both", &ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Temperature: &temperature, TopP: &topP}, "together"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ResponsesToAnthropicRequest(tc.req)
			require.Nil(t, out)
			require.ErrorContains(t, err, tc.match)
		})
	}

	_, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), Temperature: &temperature})
	require.NoError(t, err)
	_, err = ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: json.RawMessage(`"hello"`), TopP: &topP})
	require.NoError(t, err)
}

func float64PtrForAC48(v float64) *float64 { return &v }

func TestHaiku55SignedResponseHistoryRoundTrip(t *testing.T) {
	block := AnthropicContentBlock{Type: "thinking", Thinking: "signed progress", Signature: "signed-haiku-block"}
	response := AnthropicToResponsesResponse(&AnthropicResponse{Model: "claude-haiku-5-5", Content: []AnthropicContentBlock{
		block,
		{Type: "text", Text: "first message"},
		{Type: "text", Text: "second message"},
		{Type: "tool_use", ID: "toolu_1", Name: "lookup", Input: json.RawMessage(`{"q":"x"}`)},
	}})
	require.Len(t, response.Output, 4)
	require.NotEmpty(t, response.Output[0].EncryptedContent)
	for i, wantType := range []string{"reasoning", "message", "message", "function_call"} {
		require.Equal(t, wantType, response.Output[i].Type)
	}

	raw, err := json.Marshal(response.Output)
	require.NoError(t, err)
	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(raw, &items))
	items = append(items, ResponsesInputItem{Type: "function_call_output", CallID: response.Output[3].CallID, Output: "ok"})
	raw, err = json.Marshal(items)
	require.NoError(t, err)
	converted, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: raw})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 2)
	require.Equal(t, "assistant", converted.Messages[0].Role)
	require.Equal(t, "user", converted.Messages[1].Role)

	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(converted.Messages[0].Content, &blocks))
	require.Equal(t, block, blocks[0])
	require.Equal(t, "first message", blocks[1].Text)
	require.Equal(t, "second message", blocks[2].Text)
	require.Equal(t, "tool_use", blocks[3].Type)
	var results []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(converted.Messages[1].Content, &results))
	require.Len(t, results, 1)
	require.Equal(t, "tool_result", results[0].Type)
	require.Equal(t, "toolu_1", results[0].ToolUseID)
}

func TestHaiku55StreamSignedReasoningRoundTrip(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	index := 0
	events := []*AnthropicStreamEvent{
		{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "claude-haiku-5-5"}},
		{Type: "content_block_start", Index: &index, ContentBlock: &AnthropicContentBlock{Type: "thinking"}},
		{Type: "content_block_delta", Index: &index, Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "stream progress"}},
		{Type: "content_block_delta", Index: &index, Delta: &AnthropicDelta{Type: "signature_delta", Signature: "signed-haiku-stream"}},
		{Type: "content_block_stop", Index: &index},
		{Type: "message_stop"},
	}
	var output []ResponsesOutput
	for _, event := range events {
		for _, converted := range AnthropicEventToResponsesEvents(event, state) {
			if converted.Type == "response.output_item.done" {
				output = append(output, *converted.Item)
			}
		}
	}
	require.Len(t, output, 1)
	require.NotEmpty(t, output[0].EncryptedContent)
	raw, err := json.Marshal(output)
	require.NoError(t, err)
	converted, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-haiku-5-5", Input: raw})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)
	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(converted.Messages[0].Content, &blocks))
	require.Equal(t, []AnthropicContentBlock{{Type: "thinking", Thinking: "stream progress", Signature: "signed-haiku-stream"}}, blocks)
}

func TestAnthropicToResponsesNegativeIndexEventsAreFiltered(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	zero, negative := 0, -1
	for _, event := range []*AnthropicStreamEvent{
		{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "claude-sonnet-4-5"}},
		{Type: "content_block_start", Index: &zero, ContentBlock: &AnthropicContentBlock{Type: "text"}},
		{Type: "content_block_delta", Index: &negative, Delta: &AnthropicDelta{Type: "text_delta", Text: "MUST_NOT_APPEAR"}},
		{Type: "content_block_delta", Index: &zero, Delta: &AnthropicDelta{Type: "text_delta", Text: "valid"}},
	} {
		for _, out := range AnthropicEventToResponsesEvents(event, state) {
			require.NotContains(t, out.Delta, "MUST_NOT_APPEAR")
			if out.Type == "response.output_text.delta" {
				require.Equal(t, "valid", out.Delta)
			}
		}
	}
	require.Equal(t, "valid", state.TextAccum)
	for _, eventType := range []string{"content_block_start", "content_block_delta", "content_block_stop"} {
		require.Empty(t, AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: eventType, Index: &negative, ContentBlock: &AnthropicContentBlock{Type: "text"}, Delta: &AnthropicDelta{Type: "text_delta", Text: "bad"}}, state))
	}
}
