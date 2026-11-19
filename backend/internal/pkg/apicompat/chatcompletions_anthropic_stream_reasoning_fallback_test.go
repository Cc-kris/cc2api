package apicompat

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func ccReasoningChunk(reasoningContent, reasoning, content *string, finish *string) *ChatCompletionsChunk {
	return &ChatCompletionsChunk{
		ID: "chatcmpl-ac75",
		Choices: []ChatChunkChoice{{
			Delta: ChatDelta{
				ReasoningContent: reasoningContent,
				Reasoning:        reasoning,
				Content:          content,
			},
			FinishReason: finish,
		}},
	}
}

// runCCAnthropicStream feeds chunks through the stream converter and returns
// the concatenated text_delta and thinking_delta payloads plus the number of
// text blocks opened.
func runCCAnthropicStream(chunks ...*ChatCompletionsChunk) (text, thinking string, textBlocks int, events []AnthropicStreamEvent) {
	state := NewChatCompletionsToAnthropicStreamState("claude-test")
	for _, c := range chunks {
		events = append(events, ChatCompletionsChunkToAnthropicEvents(c, state)...)
	}
	events = append(events, FinalizeChatCompletionsAnthropicStream(state)...)
	var tb, tk strings.Builder
	for _, ev := range events {
		if ev.Type == "content_block_start" && ev.ContentBlock != nil && ev.ContentBlock.Type == "text" {
			textBlocks++
		}
		if ev.Type == "content_block_delta" && ev.Delta != nil {
			switch ev.Delta.Type {
			case "text_delta":
				tb.WriteString(ev.Delta.Text)
			case "thinking_delta":
				tk.WriteString(ev.Delta.Thinking)
			}
		}
	}
	return tb.String(), tk.String(), textBlocks, events
}

func TestChatAnthropicStream_ReasoningOnlyFallsBackToText(t *testing.T) {
	stop := "stop"
	cases := []struct {
		name   string
		chunks []*ChatCompletionsChunk
		want   string
	}{
		{"reasoning_content only", []*ChatCompletionsChunk{
			ccReasoningChunk(strPtr("RC-"), nil, nil, nil),
			ccReasoningChunk(strPtr("TEXT"), nil, nil, &stop),
		}, "RC-TEXT"},
		{"reasoning only", []*ChatCompletionsChunk{
			ccReasoningChunk(nil, strPtr("R-"), nil, nil),
			ccReasoningChunk(nil, strPtr("TEXT"), nil, &stop),
		}, "R-TEXT"},
		{"both differ, reasoning_content wins", []*ChatCompletionsChunk{
			ccReasoningChunk(strPtr("RC-PREF"), strPtr("R-LOSE"), nil, &stop),
		}, "RC-PREF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, thinking, textBlocks, events := runCCAnthropicStream(tc.chunks...)
			require.Equal(t, tc.want, text, "reasoning-only stream must yield visible text")
			require.Equal(t, tc.want, thinking, "thinking stream preserved")
			require.Equal(t, 1, textBlocks)
			require.Equal(t, "message_stop", events[len(events)-1].Type)
		})
	}
}

func TestChatAnthropicStream_ReasoningFallbackNotAppliedWithContentOrTools(t *testing.T) {
	stop := "stop"
	t.Run("content present: no duplicate", func(t *testing.T) {
		text, thinking, textBlocks, _ := runCCAnthropicStream(
			ccReasoningChunk(strPtr("think"), nil, nil, nil),
			ccReasoningChunk(nil, nil, strPtr("answer"), &stop),
		)
		require.Equal(t, "answer", text)
		require.Equal(t, "think", thinking)
		require.Equal(t, 1, textBlocks)
	})
	t.Run("tool call present: no fallback", func(t *testing.T) {
		idx := 0
		tools := "tool_calls"
		toolChunk := &ChatCompletionsChunk{ID: "x", Choices: []ChatChunkChoice{{
			Delta: ChatDelta{ToolCalls: []ChatToolCall{{
				Index:    &idx,
				ID:       "call_1",
				Type:     "function",
				Function: ChatFunctionCall{Name: "f", Arguments: "{}"},
			}}},
			FinishReason: &tools,
		}}}
		text, _, textBlocks, _ := runCCAnthropicStream(
			ccReasoningChunk(strPtr("think"), nil, nil, nil),
			toolChunk,
		)
		require.Empty(t, text)
		require.Zero(t, textBlocks)
	})
	t.Run("whitespace-only reasoning: no fallback", func(t *testing.T) {
		text, _, textBlocks, _ := runCCAnthropicStream(
			ccReasoningChunk(strPtr("  \n"), nil, nil, &stop),
		)
		require.Empty(t, text)
		require.Zero(t, textBlocks)
	})
}

func TestChatAnthropicStream_ReasoningOnlyMatchesBuffered(t *testing.T) {
	stop := "stop"
	text, _, _, _ := runCCAnthropicStream(ccReasoningChunk(strPtr("same"), nil, nil, &stop))
	buffered := chatMessageToAnthropicBlocks(ChatMessage{Role: "assistant", ReasoningContent: "same"})
	var bufText string
	for _, b := range buffered {
		if b.Type == "text" {
			bufText += b.Text
		}
	}
	require.Equal(t, bufText, text)
}
