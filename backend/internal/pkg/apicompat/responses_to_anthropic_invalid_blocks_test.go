package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// anthropicInboundBlockTypes 是 Anthropic Messages 请求体里合法的 content block
// 类型。转换结果只能落在这个集合内。
var anthropicInboundBlockTypes = map[string]bool{
	"text":              true,
	"image":             true,
	"document":          true,
	"tool_use":          true,
	"tool_result":       true,
	"thinking":          true,
	"redacted_thinking": true,
}

func responsesToAnthropicMessages(t *testing.T, input string) []AnthropicMessage {
	t.Helper()
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"glm-5.2","input":`+input+`}`), &req))
	out, err := ResponsesToAnthropicRequest(&req)
	require.NoError(t, err)
	return out.Messages
}

// requireAnthropicMessagesAreSendable 断言消息序列不含 Anthropic 会拒收的形态：
// 未知 block 类型、空内容消息、纯空白 text 块。
func requireAnthropicMessagesAreSendable(t *testing.T, messages []AnthropicMessage) {
	t.Helper()
	for i, m := range messages {
		raw := strings.TrimSpace(string(m.Content))
		require.NotContains(t, []string{"", "null", `""`, "[]"}, raw,
			"messages[%d] 内容为空，Anthropic 拒收空内容消息", i)

		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			require.NotEmpty(t, strings.TrimSpace(s), "messages[%d] 字符串内容不能全为空白", i)
			continue
		}
		blocks := parseContentBlocks(m.Content)
		require.NotEmpty(t, blocks, "messages[%d] 解析不出任何 block", i)
		for j, b := range blocks {
			require.True(t, anthropicInboundBlockTypes[b.Type],
				"messages[%d].content[%d] 是 Anthropic 不认识的 block 类型 %q", i, j, b.Type)
			if b.Type == "text" {
				require.NotEmpty(t, strings.TrimSpace(b.Text),
					"messages[%d].content[%d] 是空白 text 块，Anthropic 拒收", i, j)
			}
		}
	}
}

// data URI 形式的 input_file 要变成 Anthropic document，供后续 Gemini inlineData 使用。
func TestResponsesToAnthropic_InputFileDataURIBecomesDocument(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"message","role":"user","content":[
			{"type":"input_text","text":"read this"},
			{"type":"input_file","filename":"token.pdf","file_data":"data:application/pdf;base64,JVBERi0="}
		]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Len(t, messages, 1)
	require.Contains(t, string(messages[0].Content), `"type":"document"`)
	require.Contains(t, string(messages[0].Content), `"media_type":"application/pdf"`)
	require.Contains(t, string(messages[0].Content), `"data":"JVBERi0="`)
}
