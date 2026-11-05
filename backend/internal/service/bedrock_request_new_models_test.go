package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// V02-T19：新 Claude 模型在 Bedrock 路径上的映射和请求修正。
// 用例移植自上游 d662c9730、34b8bf1a6、514ac5c6a、6c9b84cc7、dd6cb410e。

func newModelsBedrockAccount(region string, mapping map[string]any) *Account {
	creds := map[string]any{"aws_region": region}
	if mapping != nil {
		creds["model_mapping"] = mapping
	}
	return &Account{Platform: PlatformAnthropic, Type: AccountTypeBedrock, Credentials: creds}
}

func TestResolveBedrockModelID_NewClaudeModels(t *testing.T) {
	cases := []struct {
		name   string
		region string
		model  string
		want   string
	}{
		{"fable 5 uses official non-regional id", "eu-west-1", "claude-fable-5", "anthropic.claude-fable-5"},
		{"fable 5.1 uses official non-regional id", "eu-west-1", "claude-fable-5-1", "anthropic.claude-fable-5-1"},
		{"opus 4.8 adjusts region prefix", "eu-west-1", "claude-opus-4-8", "eu.anthropic.claude-opus-4-8-v1"},
		{"opus 5 adjusts region prefix", "ap-northeast-1", "claude-opus-5", "jp.anthropic.claude-opus-5-v1"},
		{"sonnet 5 adjusts region prefix", "us-east-1", "claude-sonnet-5", "us.anthropic.claude-sonnet-5-v1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			modelID, ok := ResolveBedrockModelID(newModelsBedrockAccount(tc.region, nil), tc.model)
			require.True(t, ok)
			assert.Equal(t, tc.want, modelID)
		})
	}

	t.Run("sonnet 5.5 uses the global inference profile in every region", func(t *testing.T) {
		for _, region := range []string{"us-east-1", "eu-west-1", "ap-southeast-2"} {
			modelID, ok := ResolveBedrockModelID(newModelsBedrockAccount(region, nil), "claude-sonnet-5-5")
			require.True(t, ok, region)
			assert.Equal(t, "global.anthropic.claude-sonnet-5-5", modelID, region)
		}
	})

	t.Run("explicit global sonnet 5.5 profile is not rewritten", func(t *testing.T) {
		account := newModelsBedrockAccount("eu-west-1", map[string]any{
			"public-sonnet": "global.anthropic.claude-sonnet-5-5",
		})
		modelID, ok := ResolveBedrockModelID(account, "public-sonnet")
		require.True(t, ok)
		assert.Equal(t, "global.anthropic.claude-sonnet-5-5", modelID)
	})

	t.Run("opus 5.5 and haiku 5.5 have no default Bedrock id", func(t *testing.T) {
		for _, model := range []string{"claude-opus-5-5", "claude-haiku-5-5"} {
			_, ok := ResolveBedrockModelID(newModelsBedrockAccount("us-east-1", nil), model)
			assert.False(t, ok, model)
		}
	})
}

// 只有主版本号的 claude-opus-5 / claude-sonnet-5 必须被版本闸门识别（6c9b84cc7）。
func TestBedrockCapabilityGates_NewClaudeModels(t *testing.T) {
	tests := []struct {
		modelID       string
		claude45Newer bool
		toolSearch    bool
		opus47Newer   bool
	}{
		{"claude-opus-5", true, true, true},
		{"us.anthropic.claude-opus-5-v1", true, true, true},
		{"eu.anthropic.claude-opus-5-v1", true, true, true},
		{"claude-sonnet-5", true, true, false},
		{"us.anthropic.claude-sonnet-5-v1", true, true, false},
		{"global.anthropic.claude-sonnet-5-5", true, true, false},
		{"us.anthropic.claude-opus-4-8-v1", true, true, true},
		{"anthropic.claude-fable-5", true, false, false},
		{"anthropic.claude-fable-5-1", true, false, false},
		// 回归保护：旧模型不能因为 minor 可选而被误判为新版本
		{"anthropic.claude-opus-4-1-v1", false, false, false},
		{"anthropic.claude-sonnet-4-0-v1", false, false, false},
		{"anthropic.claude-3-opus-20240229-v1:0", false, false, false},
		{"us.anthropic.claude-haiku-4-5-20251001-v1:0", true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			assert.Equal(t, tt.claude45Newer, isBedrockClaude45OrNewer(tt.modelID), "isBedrockClaude45OrNewer")
			assert.Equal(t, tt.toolSearch, bedrockModelSupportsToolSearch(tt.modelID), "bedrockModelSupportsToolSearch")
			assert.Equal(t, tt.opus47Newer, isBedrockOpus47OrNewer(tt.modelID), "isBedrockOpus47OrNewer")
		})
	}
}

func TestSanitizeBedrockThinking_NewClaudeModels(t *testing.T) {
	t.Run("opus 5 converts enabled to adaptive", func(t *testing.T) {
		got := sanitizeBedrockThinking([]byte(`{"thinking":{"type":"enabled","budget_tokens":10000}}`), "us.anthropic.claude-opus-5-v1")
		assert.JSONEq(t, `{"thinking":{"type":"adaptive"}}`, string(got))
	})

	t.Run("fable 5 converts enabled to adaptive and drops budget", func(t *testing.T) {
		got := sanitizeBedrockThinking([]byte(`{"thinking":{"type":"enabled","budget_tokens":10000},"messages":[]}`), "anthropic.claude-fable-5")
		assert.Equal(t, "adaptive", gjson.GetBytes(got, "thinking.type").String())
		assert.False(t, gjson.GetBytes(got, "thinking.budget_tokens").Exists())
	})

	t.Run("fable 5 adaptive drops budget", func(t *testing.T) {
		got := sanitizeBedrockThinking([]byte(`{"thinking":{"type":"adaptive","budget_tokens":10000},"messages":[]}`), "claude-fable-5")
		assert.Equal(t, "adaptive", gjson.GetBytes(got, "thinking.type").String())
		assert.False(t, gjson.GetBytes(got, "thinking.budget_tokens").Exists())
	})

	t.Run("sonnet 5.5 converts legacy enabled to adaptive", func(t *testing.T) {
		got := sanitizeBedrockThinking([]byte(`{"thinking":{"type":"enabled","budget_tokens":10000},"messages":[]}`), "global.anthropic.claude-sonnet-5-5")
		assert.Equal(t, "adaptive", gjson.GetBytes(got, "thinking.type").String())
		assert.False(t, gjson.GetBytes(got, "thinking.budget_tokens").Exists())
	})

	t.Run("sonnet 5.5 converts legacy disabled to between_tools", func(t *testing.T) {
		got := sanitizeBedrockThinking([]byte(`{"thinking":{"type":"disabled"},"messages":[]}`), "global.anthropic.claude-sonnet-5-5")
		assert.Equal(t, "between_tools", gjson.GetBytes(got, "thinking.type").String())
	})
}

func TestPrepareBedrockSonnet55PreservesEffort(t *testing.T) {
	input := []byte(`{"model":"claude-sonnet-5-5","max_tokens":1024,"output_config":{"effort":"medium","format":{"type":"json_schema","schema":{"type":"object"}}},"messages":[{"role":"user","content":"hello"}]}`)
	result, err := PrepareBedrockRequestBodyWithTokens(input, "global.anthropic.claude-sonnet-5-5", nil, false)
	require.NoError(t, err)
	assert.Equal(t, "medium", gjson.GetBytes(result, "output_config.effort").String())
	assert.False(t, gjson.GetBytes(result, "output_config.format").Exists())
	assert.Contains(t, gjson.GetBytes(result, "messages.0.content.1.text").String(), `"type":"object"`, "schema should still be included in the final user message")
	assert.False(t, gjson.GetBytes(result, "model").Exists())

	oldModel, err := PrepareBedrockRequestBodyWithTokens(input, "us.anthropic.claude-sonnet-4-6", nil, false)
	require.NoError(t, err)
	assert.False(t, gjson.GetBytes(oldModel, "output_config").Exists())
}
