package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// V02-T19 / V02-AC46：采样锁定模型（sonnet-5-5、haiku-5-5）temperature 和 top_p
// 同时出现会被上游 400。上游 d56f5e787、c4bdc4847 的做法是：OAuth 伪装只传 top_p 时
// 不补 temperature；本地另外在出站前把同时出现的两项收成一项。
// 上游 gateway_request_haiku55_test.go 测的是 validateClaude55Request（effort 目录随 V05），
// 这里按 CCAI 的实现改写为出站 body 断言。

func TestStripClaudeSamplingConflict(t *testing.T) {
	const both = `{"model":"m","temperature":1,"top_p":0.99,"messages":[]}`
	cases := []struct {
		name      string
		model     string
		body      string
		wantTemp  bool
		wantTopP  bool
		unchanged bool
	}{
		{name: "sonnet 5.5 both → keep temperature", model: "claude-sonnet-5-5", body: both, wantTemp: true},
		{name: "haiku 5.5 both → keep temperature", model: "claude-haiku-5-5", body: both, wantTemp: true},
		{name: "bedrock sonnet 5.5 id", model: "global.anthropic.claude-sonnet-5-5", body: both, wantTemp: true},
		{name: "openrouter dotted haiku", model: "anthropic/claude-haiku-5.5", body: both, wantTemp: true},
		{name: "sonnet 5.5 tier alias", model: "claude-sonnet-5-5-thinking", body: both, wantTemp: true},
		{name: "only top_p untouched", model: "claude-haiku-5-5", body: `{"top_p":0.99}`, wantTopP: true, unchanged: true},
		{name: "only temperature untouched", model: "claude-sonnet-5-5", body: `{"temperature":1}`, wantTemp: true, unchanged: true},
		{name: "opus 5.5 not sampling locked", model: "claude-opus-5-5", body: both, wantTemp: true, wantTopP: true, unchanged: true},
		{name: "sonnet 4.6 untouched", model: "claude-sonnet-4-6", body: both, wantTemp: true, wantTopP: true, unchanged: true},
		{name: "sonnet 5 untouched", model: "claude-sonnet-5", body: both, wantTemp: true, wantTopP: true, unchanged: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StripClaudeSamplingConflict([]byte(tc.body), tc.model)
			require.Equal(t, tc.wantTemp, gjson.GetBytes(got, "temperature").Exists(), "temperature")
			require.Equal(t, tc.wantTopP, gjson.GetBytes(got, "top_p").Exists(), "top_p")
			if tc.unchanged {
				require.Equal(t, tc.body, string(got))
			}
		})
	}
}

func TestNormalizeClaudeOAuthRequestBody_SamplingLockedModels(t *testing.T) {
	opts := claudeOAuthNormalizeOptions{}
	cases := []struct {
		name     string
		model    string
		body     string
		wantTemp bool
		wantTopP bool
	}{
		// 只传 top_p：不能再补 temperature（否则网关自己制造 400）
		{name: "haiku 5.5 top_p only", model: "claude-haiku-5-5", body: `{"model":"claude-haiku-5-5","top_p":0.99,"messages":[]}`, wantTopP: true},
		{name: "sonnet 5.5 top_p only", model: "claude-sonnet-5-5", body: `{"model":"claude-sonnet-5-5","top_p":0.99,"messages":[]}`, wantTopP: true},
		// 两个都传：出站只留 temperature
		{name: "haiku 5.5 both", model: "claude-haiku-5-5", body: `{"model":"claude-haiku-5-5","temperature":1,"top_p":0.99,"messages":[]}`, wantTemp: true},
		{name: "sonnet 5.5 both", model: "claude-sonnet-5-5", body: `{"model":"claude-sonnet-5-5","temperature":1,"top_p":0.99,"messages":[]}`, wantTemp: true},
		// 都不传：仍按 CLI 习惯补 temperature=1
		{name: "sonnet 5.5 neither", model: "claude-sonnet-5-5", body: `{"model":"claude-sonnet-5-5","messages":[]}`, wantTemp: true},
		// opus-5-5 不补 temperature（上游 c4c2e6607）
		{name: "opus 5.5 neither", model: "claude-opus-5-5", body: `{"model":"claude-opus-5-5","messages":[]}`},
		// 其他模型行为不变
		{name: "sonnet 4.6 top_p only still gets temperature", model: "claude-sonnet-4-6", body: `{"model":"claude-sonnet-4-6","top_p":0.9,"messages":[]}`, wantTemp: true, wantTopP: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := normalizeClaudeOAuthRequestBody([]byte(tc.body), tc.model, opts)
			require.Equal(t, tc.wantTemp, gjson.GetBytes(got, "temperature").Exists(), "temperature in %s", got)
			require.Equal(t, tc.wantTopP, gjson.GetBytes(got, "top_p").Exists(), "top_p in %s", got)
			if tc.wantTemp {
				require.False(t, gjson.GetBytes(got, "top_p").Exists() && isClaudeSamplingLockedModel(tc.model),
					"sampling-locked model must not carry both params: %s", got)
			}
		})
	}
}

func TestStripClaudeSamplingConflict_BedrockBody(t *testing.T) {
	input := []byte(`{"model":"claude-sonnet-5-5","max_tokens":64,"temperature":1,"top_p":0.99,"messages":[{"role":"user","content":"hi"}]}`)
	prepared, err := PrepareBedrockRequestBodyWithTokens(input, "global.anthropic.claude-sonnet-5-5", nil, false)
	require.NoError(t, err)
	got := StripClaudeSamplingConflict(prepared, "global.anthropic.claude-sonnet-5-5")
	require.True(t, gjson.GetBytes(got, "temperature").Exists())
	require.False(t, gjson.GetBytes(got, "top_p").Exists())
}
