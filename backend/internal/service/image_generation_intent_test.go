package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsImageGenerationIntent(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		model    string
		body     []byte
		want     bool
	}{
		{
			name:     "images endpoint",
			endpoint: "/v1/images/generations",
			body:     []byte(`{"model":"gpt-image-2"}`),
			want:     true,
		},
		{
			name:     "image model",
			endpoint: "/v1/responses",
			model:    "gpt-image-2",
			body:     []byte(`{"model":"gpt-image-2"}`),
			want:     true,
		},
		{
			name:     "image tool",
			endpoint: "/v1/responses",
			model:    "gpt-5.4",
			body:     []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation"}]}`),
			want:     true,
		},
		{
			name:     "image tool choice",
			endpoint: "/v1/responses",
			model:    "gpt-5.4",
			body:     []byte(`{"model":"gpt-5.4","tool_choice":{"type":"image_generation"}}`),
			want:     true,
		},
		{
			name:     "required tool choice alone is text",
			endpoint: "/v1/responses",
			model:    "gpt-5.4",
			body:     []byte(`{"model":"gpt-5.4","tool_choice":"required"}`),
			want:     false,
		},
		{
			name:     "text only gpt 5.4",
			endpoint: "/v1/responses",
			model:    "gpt-5.4",
			body:     []byte(`{"model":"gpt-5.4","input":"write code"}`),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsImageGenerationIntent(tt.endpoint, tt.model, tt.body))
		})
	}
}

func TestCodexImageGenerationExtensionDetection(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-terra","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen","parameters":{"type":"object"}}]}]}`)
	require.True(t, HasCodexImageGenerationExtensionTool(body))
	require.True(t, ShouldUseCodexImageGenerationExtension("gpt-5.6-terra", "gpt-image-2", body))
	require.False(t, ShouldUseCodexImageGenerationExtension("gpt-image-2", "gpt-image-2", body))
	require.False(t, ShouldUseCodexImageGenerationExtension("gpt-5.6-terra", "gpt-5.5", body))

	legacy := []byte(`{"tools":[{"type":"image_generation"}]}`)
	require.False(t, HasCodexImageGenerationExtensionTool(legacy))

	responsesLiteTurn := []byte(`{"model":"gpt-5.6-terra","client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"turn\",\"thread_source\":\"user\"}"},"input":[{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}]}]}`)
	require.True(t, HasCodexImageGenerationExtensionTool(responsesLiteTurn))
	require.True(t, IsCodexImageGenerationExtensionTurn(responsesLiteTurn))
	require.True(t, ShouldUseCodexImageGenerationExtension("gpt-5.6-terra", "gpt-image-2", responsesLiteTurn))
	metadataOnlyTurn := []byte(`{"model":"gpt-5.6-terra","client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"turn\",\"thread_source\":\"user\"}"}}`)
	require.True(t, IsCodexImageGenerationExtensionTurn(metadataOnlyTurn))

	backgroundRequest := []byte(`{"model":"gpt-5.4-mini"}`)
	require.False(t, IsCodexImageGenerationExtensionTurn(backgroundRequest))

	prewarmRequest := []byte(`{"model":"gpt-5.4-mini","client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"prewarm\",\"thread_source\":\"user\"}"}}`)
	require.False(t, IsCodexImageGenerationExtensionTurn(prewarmRequest))

	titleRequest := []byte(`{"model":"gpt-5.4-mini","client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"turn\",\"thread_source\":\"system\"}"},"text":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"title":{"type":"string"},"description":{"type":"string"}}}}},"tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}]}`)
	require.False(t, IsCodexImageGenerationExtensionTurn(titleRequest))
	require.True(t, IsCodexSystemBackgroundTurn(titleRequest))
	require.True(t, ShouldBypassCodexSystemBackgroundImageMapping("gpt-5.4-mini", "gpt-image-2", titleRequest))
	require.False(t, ShouldBypassCodexSystemBackgroundImageMapping("gpt-5.4-mini", "gpt-5.5", titleRequest))
	require.False(t, ShouldBypassCodexSystemBackgroundImageMapping("gpt-5.4-mini", "gpt-image-2", responsesLiteTurn))

	preparedTitle, err := PrepareCodexSystemBackgroundTextDispatch(titleRequest)
	require.NoError(t, err)
	require.Equal(t, "none", gjson.GetBytes(preparedTitle, "tool_choice").String())
	require.Empty(t, gjson.GetBytes(preparedTitle, "tools").Array())
	require.Equal(t, int64(36), gjson.GetBytes(preparedTitle, "text.format.schema.properties.title.maxLength").Int())
	require.Equal(t, int64(100), gjson.GetBytes(preparedTitle, "text.format.schema.properties.description.maxLength").Int())
	require.Contains(t, gjson.GetBytes(preparedTitle, "instructions").String(), "Codex desktop metadata helper")
}

func TestPrepareCodexImageGenerationExtensionDispatch(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-terra","instructions":"base","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}]}`)
	updated, changed, err := PrepareCodexImageGenerationExtensionDispatch(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, gjson.GetBytes(updated, "instructions").String(), "only when the user's actual intent")
	require.Contains(t, gjson.GetBytes(updated, "instructions").String(), "explicit request not to generate an image")
	require.Equal(t, "function", gjson.GetBytes(updated, "tools.0.type").String())
	require.Equal(t, "imagegen", gjson.GetBytes(updated, "tools.0.name").String())
	require.Contains(t, gjson.GetBytes(updated, "tools.0.description").String(), "Omit both referenced_image_paths and num_last_images_to_include")
	require.Contains(t, gjson.GetBytes(updated, "tools.0.description").String(), "In code mode, pass the result to generatedImage(result)")
	require.Equal(t, int64(5), gjson.GetBytes(updated, "tools.0.parameters.properties.referenced_image_paths.maxItems").Int())
	require.False(t, gjson.GetBytes(updated, "tools.0.parameters.properties.num_last_images_to_include").Exists())
	require.Contains(t, gjson.GetBytes(updated, "instructions").String(), "No conversation images are available")
	require.Equal(t, 1, len(gjson.GetBytes(updated, "tools.0.parameters.required").Array()))
	require.Equal(t, "prompt", gjson.GetBytes(updated, "tools.0.parameters.required.0").String())
	require.False(t, gjson.GetBytes(updated, "tools.0.parameters.additionalProperties").Bool())
	require.Equal(t, "auto", gjson.GetBytes(updated, "tool_choice").String())

	editBody := []byte(`{"model":"gpt-5.6-terra","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}],"input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"},{"type":"input_text","text":"change it"}]}]}`)
	editUpdated, editChanged, err := PrepareCodexImageGenerationExtensionDispatch(editBody)
	require.NoError(t, err)
	require.True(t, editChanged)
	require.Equal(t, int64(1), gjson.GetBytes(editUpdated, "tools.0.parameters.properties.num_last_images_to_include.minimum").Int())
	require.Equal(t, int64(5), gjson.GetBytes(editUpdated, "tools.0.parameters.properties.num_last_images_to_include.maximum").Int())
	require.NotContains(t, gjson.GetBytes(editUpdated, "instructions").String(), "No conversation images are available")

	continuation := []byte(`{"model":"gpt-5.6-terra","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}],"tool_choice":{"type":"function","name":"imagegen"},"input":[{"type":"function_call","namespace":"image_gen","name":"imagegen","call_id":"call_1"},{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"},{"type":"input_text","text":"saved"}]}]}`)
	require.True(t, IsCodexImageGenerationExtensionContinuation(continuation))
	continuationUpdated, changed, err := PrepareCodexImageGenerationExtensionDispatch(continuation)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function", gjson.GetBytes(continuationUpdated, "tools.0.type").String())
	require.False(t, gjson.GetBytes(continuationUpdated, "tool_choice").Exists())
	require.NotContains(t, gjson.GetBytes(continuationUpdated, "instructions").String(), "only when the user's actual intent")
	outputOnlyContinuation := []byte(`{"model":"gpt-5.6-terra","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}],"previous_response_id":"resp_1","input":[{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}]}`)
	require.True(t, IsCodexImageGenerationExtensionContinuation(outputOnlyContinuation))

	mismatched := []byte(`{"model":"gpt-5.6-terra","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}],"input":[{"type":"function_call","namespace":"image_gen","name":"imagegen","call_id":"call_1"},{"type":"function_call_output","call_id":"call_other","output":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}]}`)
	require.False(t, IsCodexImageGenerationExtensionContinuation(mismatched))

	laterEdit := []byte(`{"model":"gpt-5.6-terra","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}],"input":[{"type":"function_call","namespace":"image_gen","name":"imagegen","call_id":"call_1"},{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"change the previous image to green"}]}]}`)
	require.False(t, IsCodexImageGenerationExtensionContinuation(laterEdit))
	laterEditUpdated, laterEditChanged, err := PrepareCodexImageGenerationExtensionDispatch(laterEdit)
	require.NoError(t, err)
	require.True(t, laterEditChanged)
	require.True(t, gjson.GetBytes(laterEditUpdated, "tools.0.parameters.properties.num_last_images_to_include").Exists())

	lite := []byte(`{"model":"gpt-5.6-terra","client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"turn\",\"thread_source\":\"user\"}"},"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"draw"}]},{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}]}]}`)
	liteUpdated, liteChanged, err := PrepareCodexImageGenerationExtensionDispatch(lite)
	require.NoError(t, err)
	require.True(t, liteChanged)
	require.Equal(t, "function", gjson.GetBytes(liteUpdated, "tools.0.type").String())
	require.Equal(t, "imagegen", gjson.GetBytes(liteUpdated, "tools.0.name").String())
	require.Len(t, gjson.GetBytes(liteUpdated, "tools").Array(), 1)
	require.False(t, gjson.GetBytes(liteUpdated, `input.#(type=="additional_tools")`).Exists())
	require.Equal(t, "auto", gjson.GetBytes(liteUpdated, "tool_choice").String())
	require.Contains(t, gjson.GetBytes(liteUpdated, "instructions").String(), "only when the user's actual intent")

	hostTools := []byte(`{"model":"gpt-5.6-terra","client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"turn\",\"thread_source\":\"user\"}"},"input":[{"type":"additional_tools","tools":[{"type":"custom","name":"exec"},{"type":"function","name":"wait"},{"type":"function","name":"request_user_input"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"draw"}]}]}`)
	hostToolsUpdated, hostToolsChanged, err := PrepareCodexImageGenerationExtensionDispatch(hostTools)
	require.NoError(t, err)
	require.True(t, hostToolsChanged)
	require.Len(t, gjson.GetBytes(hostToolsUpdated, "tools").Array(), 1)
	require.Equal(t, "imagegen", gjson.GetBytes(hostToolsUpdated, "tools.0.name").String())
	require.False(t, gjson.GetBytes(hostToolsUpdated, `tools.#(name=="exec")`).Exists())
	require.False(t, gjson.GetBytes(hostToolsUpdated, `tools.#(name=="wait")`).Exists())
	require.False(t, gjson.GetBytes(hostToolsUpdated, `input.#(type=="additional_tools")`).Exists())
}

func TestAttachCodexTurnMetadataEnablesWebSocketRouting(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"gpt-5.6-terra"}`)
	updated, err := AttachCodexTurnMetadata(body, `{"request_kind":"turn","thread_source":"user"}`)
	require.NoError(t, err)
	require.True(t, IsCodexImageGenerationExtensionTurn(updated), "canonical user metadata must restore the client-local image tool route")
	require.Equal(t, "turn", gjson.Get(gjson.GetBytes(updated, "client_metadata.x-codex-turn-metadata").String(), "request_kind").String())
}

func TestCodexDesktopImageGenerationTransportFallbackShapes(t *testing.T) {
	// Current Desktop builds can keep the local extension out of the serialized
	// request. Canonical user metadata must still restore the proven image_gen
	// route so the client receives a local image result it can display.
	wsAttempt := []byte(`{"type":"response.create","model":"gpt-5.6-sol","client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"turn\",\"thread_source\":\"user\"}"},"input":[{"role":"user","content":[{"type":"input_text","text":"draw a blue paper airplane"}]}]}`)
	require.True(t, ShouldUseCodexImageGenerationExtension("gpt-5.6-sol", "gpt-image-2", wsAttempt))
	require.True(t, IsImageGenerationIntent("/v1/responses", "gpt-image-2", wsAttempt))

	// The Desktop HTTP fallback carries the concrete image_gen namespace. No
	// actor-authorization header is part of this structural routing decision.
	httpFallback := []byte(`{"model":"gpt-5.6-sol","input":[{"role":"user","content":[{"type":"input_text","text":"draw a blue paper airplane"}]}],"tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}]}`)
	require.True(t, ShouldUseCodexImageGenerationExtension("gpt-5.6-sol", "gpt-image-2", httpFallback))
	prepared, changed, err := PrepareCodexImageGenerationExtensionDispatch(httpFallback)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function", gjson.GetBytes(prepared, "tools.0.type").String())
	require.Equal(t, "imagegen", gjson.GetBytes(prepared, "tools.0.name").String())
}

func TestClassifyCodexImageRequest(t *testing.T) {
	metadata := func(kind, source string) string {
		return `"client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"` + kind + `\",\"thread_source\":\"` + source + `\"}"}`
	}
	extension := `"tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}]`
	hosted := `"tools":[{"type":"image_generation"}]`

	tests := []struct {
		name     string
		body     string
		wantRole CodexRequestRole
		wantExec CodexImageExecution
		legacy   bool
	}{
		{"current extension user turn", `{` + metadata("turn", "user") + `,` + extension + `}`, CodexRequestRoleUserTurn, CodexImageExecutionExtension, false},
		{"current metadata-only user turn restores local extension", `{` + metadata("turn", "user") + `}`, CodexRequestRoleUserTurn, CodexImageExecutionExtension, false},
		{"current hosted image user turn", `{` + metadata("turn", "user") + `,` + hosted + `}`, CodexRequestRoleUserTurn, CodexImageExecutionHostedImage, false},
		{"feature turn bypasses extension", `{` + metadata("turn", "automation") + `,` + extension + `}`, CodexRequestRoleFeature, CodexImageExecutionTextBypass, false},
		{"subagent bypasses image mapping", `{` + metadata("turn", "subagent") + `}`, CodexRequestRoleSubagent, CodexImageExecutionTextBypass, false},
		{"memory consolidation source bypasses image mapping", `{` + metadata("turn", "memory_consolidation") + `}`, CodexRequestRoleMemory, CodexImageExecutionTextBypass, false},
		{"prewarm bypasses image mapping", `{` + metadata("prewarm", "user") + `}`, CodexRequestRolePrewarm, CodexImageExecutionTextBypass, false},
		{"compaction bypasses image mapping", `{` + metadata("compaction", "user") + `}`, CodexRequestRoleCompaction, CodexImageExecutionTextBypass, false},
		{"memory bypasses image mapping", `{` + metadata("memory", "") + `}`, CodexRequestRoleMemory, CodexImageExecutionTextBypass, false},
		{"future metadata kind fails safe to text", `{` + metadata("future_kind", "user") + `}`, CodexRequestRoleUnknown, CodexImageExecutionTextBypass, false},
		{"legacy hosted tool", `{` + hosted + `}`, CodexRequestRoleUnknown, CodexImageExecutionHostedImage, false},
		{"legacy request without image tool stays text", `{"input":"explain an API"}`, CodexRequestRoleUnknown, CodexImageExecutionTextBypass, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := ClassifyCodexImageRequest("gpt-5.6-terra", "gpt-image-2", []byte(tt.body))
			require.Equal(t, tt.wantRole, decision.Role)
			require.Equal(t, tt.wantExec, decision.Execution)
			require.Equal(t, tt.legacy, decision.LegacyFallback)
		})
	}

	t.Run("ordinary mappings never become image routes", func(t *testing.T) {
		decision := ClassifyCodexImageRequest("gpt-5.6-terra", "gpt-5.6-sol", []byte(`{`+metadata("turn", "user")+`,`+extension+`}`))
		require.Equal(t, CodexImageExecutionOrdinary, decision.Execution)
	})

	t.Run("only text orchestration paths leave the image group", func(t *testing.T) {
		require.True(t, CodexImageRequestDecision{Execution: CodexImageExecutionTextBypass}.UsesOrchestratorGroup())
		require.True(t, CodexImageRequestDecision{Execution: CodexImageExecutionExtension}.UsesOrchestratorGroup())
		require.False(t, CodexImageRequestDecision{Execution: CodexImageExecutionHostedImage}.UsesOrchestratorGroup())
	})

	t.Run("metadata object uses the same canonical role", func(t *testing.T) {
		body := []byte(`{"client_metadata":{"x-codex-turn-metadata":{"request_kind":"turn","thread_source":"user"}}}`)
		decision := ClassifyCodexImageRequest("gpt-5.6-terra", "gpt-image-2", body)
		require.Equal(t, CodexRequestRoleUserTurn, decision.Role)
		require.Equal(t, CodexImageExecutionExtension, decision.Execution)
	})
}

func TestPrepareCodexImageRouteRequest(t *testing.T) {
	userMetadata := `"client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"turn\",\"thread_source\":\"user\"}"}`

	t.Run("hosted image restores dedicated top-level model mapping", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.6-terra",` + userMetadata + `,"input":"draw","tools":[{"type":"image_generation"}]}`)
		decision := ClassifyCodexImageRequest("gpt-5.6-terra", "gpt-image-2", body)
		prepared, err := PrepareCodexImageRouteRequest(body, "gpt-5.6-terra", "gpt-image-2", decision)
		require.NoError(t, err)
		require.Equal(t, "gpt-image-2", gjson.GetBytes(prepared, "model").String())
		require.Equal(t, "image_generation", gjson.GetBytes(prepared, "tools.0.type").String())
		require.Equal(t, "gpt-image-2", gjson.GetBytes(prepared, "tools.0.model").String())
		require.Equal(t, "image_generation", gjson.GetBytes(prepared, "tool_choice.type").String())
	})

	t.Run("metadata-only user turn receives local image extension contract", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.6-terra",` + userMetadata + `,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"draw"}]}]}`)
		decision := ClassifyCodexImageRequest("gpt-5.6-terra", "gpt-image-2", body)
		prepared, err := PrepareCodexImageRouteRequest(body, "gpt-5.6-terra", "gpt-image-2", decision)
		require.NoError(t, err)
		require.Equal(t, CodexImageExecutionExtension, decision.Execution)
		require.Equal(t, "gpt-5.6-terra", gjson.GetBytes(prepared, "model").String())
		require.Equal(t, "function", gjson.GetBytes(prepared, "tools.0.type").String())
		require.Equal(t, "imagegen", gjson.GetBytes(prepared, "tools.0.name").String())
		require.Equal(t, "auto", gjson.GetBytes(prepared, "tool_choice").String())
		require.Contains(t, gjson.GetBytes(prepared, "instructions").String(), "only when the user's actual intent")
		require.False(t, gjson.GetBytes(prepared, `input.#(type=="additional_tools")`).Exists())
		require.Empty(t, gjson.GetBytes(prepared, `tools.#(type=="image_generation")#`).Array())
	})

	t.Run("feature turn strips image namespace but preserves text model", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.4-mini","client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"turn\",\"thread_source\":\"automation\"}"},"tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]},{"type":"function","name":"shell"}],"tool_choice":{"type":"function","name":"imagegen"},"input":"metadata work"}`)
		decision := ClassifyCodexImageRequest("gpt-5.4-mini", "gpt-image-2", body)
		prepared, err := PrepareCodexImageRouteRequest(body, "gpt-5.4-mini", "gpt-image-2", decision)
		require.NoError(t, err)
		require.Equal(t, CodexImageExecutionTextBypass, decision.Execution)
		require.Equal(t, "gpt-5.4-mini", gjson.GetBytes(prepared, "model").String())
		require.Equal(t, "shell", gjson.GetBytes(prepared, "tools.0.name").String())
		require.Equal(t, "none", gjson.GetBytes(prepared, "tool_choice").String())
	})

	t.Run("feature metadata helper strips responses lite image namespace", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.4-mini","client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"turn\",\"thread_source\":\"automation\"}"},"text":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"title":{"type":"string"}}}}},"input":[{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}]}]}`)
		decision := ClassifyCodexImageRequest("gpt-5.4-mini", "gpt-image-2", body)
		prepared, err := PrepareCodexImageRouteRequest(body, "gpt-5.4-mini", "gpt-image-2", decision)
		require.NoError(t, err)
		require.Equal(t, CodexImageExecutionTextBypass, decision.Execution)
		require.False(t, gjson.GetBytes(prepared, `input.#(type=="additional_tools")`).Exists())
		require.Equal(t, "none", gjson.GetBytes(prepared, "tool_choice").String())
	})

	t.Run("prewarm remains byte-for-byte text", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.4-mini","client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"prewarm\",\"thread_source\":\"user\"}"},"input":"warm"}`)
		decision := ClassifyCodexImageRequest("gpt-5.4-mini", "gpt-image-2", body)
		prepared, err := PrepareCodexImageRouteRequest(body, "gpt-5.4-mini", "gpt-image-2", decision)
		require.NoError(t, err)
		require.Equal(t, body, prepared)
	})
}

func TestIsImageGenerationPermissionIntentIgnoresToolDeclarationOnly(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want bool
	}{
		{
			name: "codex desktop capability declaration only",
			body: []byte(`{"model":"gpt-5.4","input":"write code","tools":[{"type":"image_generation"},{"type":"function","name":"shell"}]}`),
			want: false,
		},
		{
			name: "explicit image tool choice",
			body: []byte(`{"model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation"}],"tool_choice":{"type":"image_generation"}}`),
			want: true,
		},
		{
			name: "image model",
			body: []byte(`{"model":"gpt-image-2","input":"draw"}`),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsImageGenerationPermissionIntent("/v1/responses", "gpt-5.4", tt.body))
		})
	}
}

func TestResolveOpenAIResponsesImageBillingConfigUsesCurrentBodyModel(t *testing.T) {
	imageModel, imageSize, err := resolveOpenAIResponsesImageBillingConfigFromBody(
		[]byte(`{"model":"mapped-image-model","tools":[{"type":"image_generation","size":"1024x1024"}]}`),
		"requested-model",
	)
	require.NoError(t, err)
	require.Equal(t, "mapped-image-model", imageModel)
	require.Equal(t, "1K", imageSize)
}

func TestResolveOpenAIResponsesImageBillingConfigToolModelWins(t *testing.T) {
	imageModel, imageSize, err := resolveOpenAIResponsesImageBillingConfigFromBody(
		[]byte(`{"model":"mapped-text-model","tools":[{"type":"image_generation","model":"gpt-image-2","size":"1536x1024"}]}`),
		"requested-model",
	)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", imageModel)
	require.Equal(t, "2K", imageSize)
}

func TestResolveOpenAIResponsesImageBillingConfigSupportsOfficialAndCustomSizes(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		wantTier string
	}{
		{
			name:     "official 2k landscape",
			body:     []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation","model":"gpt-image-2","size":"2048x1152"}]}`),
			wantTier: "2K",
		},
		{
			name:     "official 4k landscape",
			body:     []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation","model":"gpt-image-2","size":"3840x2160"}]}`),
			wantTier: "4K",
		},
		{
			name:     "custom valid 2k",
			body:     []byte(`{"model":"gpt-5.5","tools":[{"type":"image_generation","model":"gpt-image-2","size":"1280x768"}]}`),
			wantTier: "2K",
		},
		{
			name:     "default image tool model supports flexible size",
			body:     []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation","size":"2048x1152"}]}`),
			wantTier: "2K",
		},
		{
			name:     "top level image size is moved into billing",
			body:     []byte(`{"model":"gpt-image-2","size":"2048x2048","tools":[{"type":"image_generation","model":"gpt-image-2"}]}`),
			wantTier: "2K",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageModel, imageSize, err := resolveOpenAIResponsesImageBillingConfigFromBody(tt.body, "requested-model")
			require.NoError(t, err)
			require.NotEmpty(t, imageModel)
			require.Equal(t, tt.wantTier, imageSize)
		})
	}
}

func TestResolveOpenAIResponsesImageBillingConfigDoesNotRejectUnknownSizes(t *testing.T) {
	imageModel, imageSize, err := resolveOpenAIResponsesImageBillingConfigFromBody(
		[]byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation","model":"gpt-image-1.5","size":"2048x1152"}]}`),
		"requested-model",
	)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-1.5", imageModel)
	require.Equal(t, "2K", imageSize)
}

func TestOpenAIImageOutputCounterDeduplicatesFinalImages(t *testing.T) {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEData([]byte(`{"type":"response.image_generation_call.partial_image","partial_image_b64":"abc"}`))
	counter.AddSSEData([]byte(`{"type":"response.output_item.done","item":{"id":"ig_1","type":"image_generation_call","result":"final-a","size":"1024x1024"}}`))
	counter.AddSSEData([]byte(`{"type":"response.completed","response":{"output":[{"id":"ig_1","type":"image_generation_call","result":"final-a"},{"id":"ig_2","type":"image_generation_call","result":"final-b","size":"3840x2160"}]}}`))
	require.Equal(t, 2, counter.Count())
	require.Equal(t, []string{"1024x1024", "3840x2160"}, counter.Sizes())
}

func TestOpenAIImageOutputCounterCountsImagesAPIStreamShapes(t *testing.T) {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEData([]byte(`{"type":"image_generation.completed","id":"ig_complete","b64_json":"final-a"}`))
	counter.AddSSEData([]byte(`{"type":"response.output_item.done","item":{"id":"ig_item","type":"image_generation_call","result":"final-b"}}`))
	counter.AddSSEData([]byte(`{"type":"response.completed","response":{"output":[{"id":"ig_done","type":"image_generation_call","result":"final-c"}]}}`))
	require.Equal(t, 3, counter.Count())

	dataCounter := newOpenAIImageOutputCounter()
	dataCounter.AddSSEData([]byte(`{"data":[{"b64_json":"a"},{"b64_json":"b"}]}`))
	dataCounter.AddSSEData([]byte(`{"data":[{"b64_json":"a"},{"b64_json":"b"},{"b64_json":"c"}]}`))
	require.Equal(t, 3, dataCounter.Count())
}

func TestOpenAIImageOutputCounterCountsMultilineSSEDataPayload(t *testing.T) {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEData([]byte("{\"type\":\"image_generation.completed\",\n\"b64_json\":\"final-a\"}"))
	require.Equal(t, 1, counter.Count())
}

func TestOpenAIImageOutputCounterCountsMultilineSSEBodyPayload(t *testing.T) {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEBody(
		"data: {\"type\":\"image_generation.completed\",\n" +
			"data: \"b64_json\":\"final-a\"}\n\n" +
			"data: [DONE]\n\n",
	)
	require.Equal(t, 1, counter.Count())
}

func TestOpenAIImageOutputCounterFallsBackForInvalidMultilineSSEBody(t *testing.T) {
	counter := newOpenAIImageOutputCounter()
	counter.AddSSEBody(
		"data: {\"type\":\"image_generation.completed\",\"b64_json\":\"final-a\"}\n" +
			"data: {\"type\":\"image_generation.completed\",\"b64_json\":\"final-b\"}\n\n",
	)
	require.Equal(t, 2, counter.Count())
}

func TestCollectOpenAIResponseImageOutputSizesFromJSONBytes(t *testing.T) {
	body := []byte(`{
		"output": [
			{"id":"ig_1","type":"image_generation_call","result":"final-a","size":"3840x2160"},
			{"id":"ig_2","type":"image_generation_call","result":"final-b","size":"1024x1024"}
		]
	}`)

	require.Equal(t, 2, countOpenAIResponseImageOutputsFromJSONBytes(body))
	require.Equal(t, []string{"3840x2160", "1024x1024"}, collectOpenAIResponseImageOutputSizesFromJSONBytes(body))
}

func TestCollectOpenAIResponseImageOutputSizesFromImagesAPIData(t *testing.T) {
	body := []byte(`{
		"data": [
			{"b64_json":"final-a","size":"2048x1152"},
			{"b64_json":"final-b","size":"2048x1152"}
		]
	}`)

	require.Equal(t, 2, countOpenAIResponseImageOutputsFromJSONBytes(body))
	require.Equal(t, []string{"2048x1152", "2048x1152"}, collectOpenAIResponseImageOutputSizesFromJSONBytes(body))
}

func TestCollectOpenAIImageOutputSizesFromSSEBody(t *testing.T) {
	body := "data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_1\",\"type\":\"image_generation_call\",\"result\":\"final-a\",\"size\":\"3840x2160\"}}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"id\":\"ig_1\",\"type\":\"image_generation_call\",\"result\":\"final-a\"},{\"id\":\"ig_2\",\"type\":\"image_generation_call\",\"result\":\"final-b\",\"size\":\"1024x1024\"}]}}\n\n" +
		"data: [DONE]\n\n"

	require.Equal(t, 2, countOpenAIImageOutputsFromSSEBody(body))
	require.Equal(t, []string{"3840x2160", "1024x1024"}, collectOpenAIImageOutputSizesFromSSEBody(body))
}
