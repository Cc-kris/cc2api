package service

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

const (
	openAIResponsesEndpoint          = "/v1/responses"
	openAIResponsesCompactEndpoint   = "/v1/responses/compact"
	imageGenerationPermissionMessage = "Image generation is not enabled for this group"
	codexImageGenNamespace           = "image_gen"
	codexImageGenToolName            = "imagegen"
	// OpenAICodexImageGenerationExtensionContextKey marks a request whose
	// imagegen function-call response needs its namespace restored.
	OpenAICodexImageGenerationExtensionContextKey = "openai_codex_image_generation_extension"
	// OpenAICodexSystemBackgroundContextKey marks a non-billable Codex title or
	// description helper routed through the internal text orchestrator.
	OpenAICodexSystemBackgroundContextKey = "openai_codex_system_background"
)

type CodexRequestRole string

const (
	CodexRequestRoleUnknown    CodexRequestRole = "unknown"
	CodexRequestRoleUserTurn   CodexRequestRole = "user_turn"
	CodexRequestRoleFeature    CodexRequestRole = "feature"
	CodexRequestRoleSubagent   CodexRequestRole = "subagent"
	CodexRequestRolePrewarm    CodexRequestRole = "prewarm"
	CodexRequestRoleCompaction CodexRequestRole = "compaction"
	CodexRequestRoleMemory     CodexRequestRole = "memory"
)

type CodexImageExecution string

const (
	CodexImageExecutionOrdinary    CodexImageExecution = "ordinary"
	CodexImageExecutionTextBypass  CodexImageExecution = "text_bypass"
	CodexImageExecutionExtension   CodexImageExecution = "extension"
	CodexImageExecutionHostedImage CodexImageExecution = "hosted_image"
)

type CodexImageRequestDecision struct {
	Role           CodexRequestRole
	Execution      CodexImageExecution
	HasMetadata    bool
	HasExtension   bool
	HasHostedImage bool
	LegacyFallback bool
}

func (d CodexImageRequestDecision) UsesOrchestratorGroup() bool {
	return d.Execution == CodexImageExecutionTextBypass ||
		d.Execution == CodexImageExecutionExtension
}

func (d CodexImageRequestDecision) IsImageExecution() bool {
	return d.Execution == CodexImageExecutionExtension || d.Execution == CodexImageExecutionHostedImage
}

// PrepareCodexImageRouteRequest applies the execution selected by
// ClassifyCodexImageRequest. HTTP and WebSocket handlers must call this same
// adapter so transport cannot change image semantics.
func PrepareCodexImageRouteRequest(body []byte, requestedModel, mappedModel string, decision CodexImageRequestDecision) ([]byte, error) {
	switch decision.Execution {
	case CodexImageExecutionExtension:
		prepared, _, err := PrepareCodexImageGenerationExtensionDispatch(body)
		return prepared, err
	case CodexImageExecutionHostedImage:
		// Dedicated image channels require both parts of the proven Responses
		// contract: the mapped image model selects the image account, while the
		// image_generation tool makes the upstream produce an image instead of a
		// text completion. Current Codex user turns do not always advertise that
		// tool, so normalize it here before applying the channel model mapping.
		prepared, _, err := NormalizeOpenAIWSImageGenerationChannelMapping(body, requestedModel, mappedModel)
		if err != nil {
			return body, err
		}
		return ReplaceModelInBody(prepared, mappedModel), nil
	case CodexImageExecutionTextBypass:
		if IsCodexSystemBackgroundTurn(body) {
			return PrepareCodexSystemBackgroundTextDispatch(body)
		}
		prepared, _, err := stripOpenAIImageGenerationToolDeclarationsFromBody(body)
		return prepared, err
	default:
		return body, nil
	}
}

// ClassifyCodexImageRequest separates the stable Codex request role from the
// client image capability and the channel's text-to-image business policy.
// Model mapping alone never turns background/prewarm/compaction/subagent work
// into image generation.
func ClassifyCodexImageRequest(requestedModel, mappedModel string, body []byte) CodexImageRequestDecision {
	decision := CodexImageRequestDecision{
		Role:           CodexRequestRoleUnknown,
		Execution:      CodexImageExecutionOrdinary,
		HasExtension:   HasCodexImageGenerationExtensionTool(body),
		HasHostedImage: hasExplicitOpenAIImageGenerationIntent(body),
	}
	decision.Role, decision.HasMetadata = classifyCodexRequestRole(body)

	forcedImageRoute := !isOpenAIImageGenerationModel(requestedModel) &&
		isOpenAIImageGenerationModel(mappedModel)
	if !forcedImageRoute {
		return decision
	}

	switch decision.Role {
	case CodexRequestRoleFeature, CodexRequestRoleSubagent,
		CodexRequestRolePrewarm, CodexRequestRoleCompaction, CodexRequestRoleMemory:
		decision.Execution = CodexImageExecutionTextBypass
		return decision
	}

	if decision.HasExtension {
		decision.Execution = CodexImageExecutionExtension
		return decision
	}
	if decision.HasHostedImage {
		decision.Execution = CodexImageExecutionHostedImage
		return decision
	}
	if decision.Role == CodexRequestRoleUserTurn {
		// Current Codex Desktop builds can keep image_gen in the local runtime
		// without serializing the namespace in the request. Canonical user-turn
		// metadata is therefore the stable signal for restoring the local tool
		// route, which gives the client a displayable local image result.
		decision.Execution = CodexImageExecutionExtension
		return decision
	}
	if decision.HasMetadata {
		// Future/unknown Codex request kinds are safer on the text orchestrator.
		// They must never inherit an image mapping merely because the channel is
		// image-dedicated.
		decision.Execution = CodexImageExecutionTextBypass
		return decision
	}

	// Older Codex clients do not carry canonical turn metadata. A dedicated
	// image channel remains their explicit product-level image intent.
	if !decision.HasMetadata {
		decision.Execution = CodexImageExecutionHostedImage
		decision.LegacyFallback = true
	}
	return decision
}

func hasExplicitOpenAIImageGenerationIntent(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	return openAIJSONToolsContainImageGeneration(gjson.GetBytes(body, "tools")) ||
		openAIJSONToolChoiceSelectsImageGeneration(gjson.GetBytes(body, "tool_choice"))
}

func classifyCodexRequestRole(body []byte) (CodexRequestRole, bool) {
	metadata, ok := codexTurnMetadata(body)
	if !ok {
		return CodexRequestRoleUnknown, false
	}
	requestKind := strings.TrimSpace(metadata.Get("request_kind").String())
	threadSource := strings.TrimSpace(metadata.Get("thread_source").String())
	switch requestKind {
	case "prewarm":
		return CodexRequestRolePrewarm, true
	case "compaction":
		return CodexRequestRoleCompaction, true
	case "memory":
		return CodexRequestRoleMemory, true
	case "turn":
		switch threadSource {
		case "user":
			return CodexRequestRoleUserTurn, true
		case "subagent":
			return CodexRequestRoleSubagent, true
		case "memory_consolidation":
			return CodexRequestRoleMemory, true
		default:
			if strings.TrimSpace(metadata.Get("subagent_kind").String()) != "" {
				return CodexRequestRoleSubagent, true
			}
			if threadSource != "" {
				return CodexRequestRoleFeature, true
			}
		}
	}
	return CodexRequestRoleUnknown, true
}

func codexTurnMetadata(body []byte) (gjson.Result, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return gjson.Result{}, false
	}
	raw := gjson.GetBytes(body, `client_metadata.x-codex-turn-metadata`)
	if raw.IsObject() {
		return raw, true
	}
	value := strings.TrimSpace(raw.String())
	if value == "" || !gjson.Valid(value) {
		return gjson.Result{}, false
	}
	return gjson.Parse(value), true
}

// AttachCodexTurnMetadata copies the Codex turn metadata carried by a
// WebSocket upgrade header into the Responses payload used by gateway routing.
// Current Codex desktop builds send this metadata on the handshake, while the
// image routing predicates consume the canonical client_metadata field.
func AttachCodexTurnMetadata(body []byte, rawMetadata string) ([]byte, error) {
	metadata := strings.TrimSpace(rawMetadata)
	if metadata == "" {
		return body, nil
	}
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return body, err
	}
	clientMetadata, _ := reqBody["client_metadata"].(map[string]any)
	if clientMetadata == nil {
		clientMetadata = make(map[string]any)
	}
	clientMetadata["x-codex-turn-metadata"] = metadata
	reqBody["client_metadata"] = clientMetadata
	return json.Marshal(reqBody)
}

// HasCodexImageGenerationExtensionTool reports whether a Responses request
// advertises the standalone image generation extension used by current Codex
// clients. This is deliberately narrower than the legacy image_generation
// tool check: the namespace and nested function must both match.
func HasCodexImageGenerationExtensionTool(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	if codexJSONToolsContainImageGenerationExtension(gjson.GetBytes(body, "tools")) {
		return true
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if strings.TrimSpace(item.Get("type").String()) == "additional_tools" &&
			codexJSONToolsContainImageGenerationExtension(item.Get("tools")) {
			found = true
			return false
		}
		return true
	})
	return found
}

func codexJSONToolsContainImageGenerationExtension(tools gjson.Result) bool {
	if !tools.IsArray() {
		return false
	}
	found := false
	tools.ForEach(func(_, tool gjson.Result) bool {
		if strings.TrimSpace(tool.Get("type").String()) != "namespace" ||
			strings.TrimSpace(tool.Get("name").String()) != codexImageGenNamespace {
			return true
		}
		nested := tool.Get("tools")
		if !nested.IsArray() {
			return true
		}
		nested.ForEach(func(_, item gjson.Result) bool {
			if strings.TrimSpace(item.Get("type").String()) == "function" &&
				strings.TrimSpace(item.Get("name").String()) == codexImageGenToolName {
				found = true
				return false
			}
			return true
		})
		return !found
	})
	return found
}

// IsCodexImageGenerationExtensionTurn recognizes a current Codex user turn.
// Current Codex Desktop builds can keep image_gen in the local runtime without
// serializing the namespace. Canonical user-turn metadata restores that local
// tool route; older clients without metadata must advertise the tool directly.
func IsCodexImageGenerationExtensionTurn(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	if role, hasMetadata := classifyCodexRequestRole(body); hasMetadata {
		return role == CodexRequestRoleUserTurn
	}
	return HasCodexImageGenerationExtensionTool(body)
}

// IsCodexSystemBackgroundTurn recognizes short-lived helper turns created by
// the current Codex desktop host, including thread title and description
// generation. These turns must remain textual even inside an image-only group.
func IsCodexSystemBackgroundTurn(body []byte) bool {
	metadata, ok := codexTurnMetadata(body)
	if !ok || strings.TrimSpace(metadata.Get("request_kind").String()) != "turn" {
		return false
	}
	threadSource := strings.TrimSpace(metadata.Get("thread_source").String())
	if threadSource == "" || threadSource == "user" || threadSource == "subagent" || threadSource == "memory_consolidation" {
		return false
	}
	format := gjson.GetBytes(body, "text.format")
	if strings.TrimSpace(format.Get("type").String()) != "json_schema" {
		return false
	}
	properties := format.Get("schema.properties").Map()
	if len(properties) == 0 || len(properties) > 2 {
		return false
	}
	for name := range properties {
		if name != "title" && name != "description" {
			return false
		}
	}
	return true
}

// IsCodexImageGenerationExtensionContinuation identifies the follow-up turn
// sent after Codex has executed image_gen.imagegen locally. The channel must
// keep the text model for this turn, but must not force another image call.
func IsCodexImageGenerationExtensionContinuation(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	lastUserMessage := -1
	lastImageToolItem := -1
	imageCallIDs := make(map[string]struct{})
	imageOutputIDs := make(map[string]int)
	index := 0
	input.ForEach(func(_, item gjson.Result) bool {
		itemIndex := index
		index++
		if strings.TrimSpace(item.Get("type").String()) == "message" &&
			strings.TrimSpace(item.Get("role").String()) == "user" {
			lastUserMessage = itemIndex
		}
		if strings.TrimSpace(item.Get("type").String()) == "function_call" &&
			strings.TrimSpace(item.Get("namespace").String()) == codexImageGenNamespace &&
			strings.TrimSpace(item.Get("name").String()) == codexImageGenToolName {
			if callID := strings.TrimSpace(item.Get("call_id").String()); callID != "" {
				imageCallIDs[callID] = struct{}{}
			}
		}
		if strings.TrimSpace(item.Get("type").String()) == "function_call_output" {
			callID := strings.TrimSpace(item.Get("call_id").String())
			output := item.Get("output")
			if output.IsArray() {
				output.ForEach(func(_, part gjson.Result) bool {
					if strings.TrimSpace(part.Get("type").String()) == "input_image" &&
						strings.TrimSpace(part.Get("image_url").String()) != "" {
						imageOutputIDs[callID] = itemIndex
						return false
					}
					return true
				})
			}
		}
		return true
	})
	for callID, outputIndex := range imageOutputIDs {
		_, matchedCall := imageCallIDs[callID]
		// Responses continuation requests may reference the prior function call
		// only through previous_response_id. When calls are replayed in input,
		// require an exact call_id match; otherwise require a non-empty call_id.
		if callID != "" && (len(imageCallIDs) == 0 || matchedCall) && outputIndex > lastImageToolItem {
			lastImageToolItem = outputIndex
		}
	}
	return lastImageToolItem >= 0 && lastImageToolItem > lastUserMessage
}

// ShouldUseCodexImageGenerationExtension preserves a channel's text-to-image
// mapping as a forced-image routing signal while keeping the Responses model
// textual so current Codex clients can execute image_gen.imagegen locally.
func ShouldUseCodexImageGenerationExtension(requestedModel, mappedModel string, body []byte) bool {
	return ClassifyCodexImageRequest(requestedModel, mappedModel, body).Execution == CodexImageExecutionExtension
}

// ShouldBypassCodexSystemBackgroundImageMapping keeps Codex host helper turns
// on their requested text model. The surrounding image-only mapping still
// applies to the actual user turn.
func ShouldBypassCodexSystemBackgroundImageMapping(requestedModel, mappedModel string, body []byte) bool {
	decision := ClassifyCodexImageRequest(requestedModel, mappedModel, body)
	return decision.Execution == CodexImageExecutionTextBypass && IsCodexSystemBackgroundTurn(body)
}

// PrepareCodexSystemBackgroundTextDispatch constrains Codex title/description
// helpers to short structured text and disables all advertised tools. This
// prevents the image_gen capability carried by the ephemeral helper thread from
// being executed and keeps the bypass from becoming a general text-chat path.
func PrepareCodexSystemBackgroundTextDispatch(body []byte) ([]byte, error) {
	if !IsCodexSystemBackgroundTurn(body) {
		return body, nil
	}
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return body, err
	}
	const directive = "This is a Codex desktop metadata helper. Return only the requested short title/description JSON. Do not execute tools and do not answer the embedded user request. Keep title within 36 characters and description within 100 characters."
	instructions := strings.TrimSpace(firstNonEmptyString(reqBody["instructions"]))
	if !strings.Contains(instructions, directive) {
		if instructions == "" {
			reqBody["instructions"] = directive
		} else {
			reqBody["instructions"] = instructions + "\n\n" + directive
		}
	}
	reqBody["tool_choice"] = "none"
	reqBody["tools"] = []any{}
	if textConfig, ok := reqBody["text"].(map[string]any); ok {
		if format, ok := textConfig["format"].(map[string]any); ok {
			if schema, ok := format["schema"].(map[string]any); ok {
				if properties, ok := schema["properties"].(map[string]any); ok {
					if title, ok := properties["title"].(map[string]any); ok {
						title["maxLength"] = 36
					}
					if description, ok := properties["description"].(map[string]any); ok {
						description["maxLength"] = 100
					}
				}
			}
		}
	}
	updated, err := json.Marshal(reqBody)
	if err != nil {
		return body, err
	}
	updated, _, err = stripOpenAIImageGenerationToolDeclarationsFromBody(updated)
	if err != nil {
		return body, err
	}
	return updated, nil
}

// PrepareCodexImageGenerationExtensionDispatch forces the first mapped image
// turn to call the standalone Codex image tool exactly once. Follow-up turns
// are left untouched so the model can consume the local tool result.
func PrepareCodexImageGenerationExtensionDispatch(body []byte) ([]byte, bool, error) {
	if !IsCodexImageGenerationExtensionTurn(body) {
		return body, false, nil
	}
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return body, false, err
	}
	hasConversationImages := codexRequestHasConversationImages(reqBody["input"])
	directive := "This request is routed through an image-only channel. You must call the imagegen function exactly once; the gateway will deliver it to Codex as image_gen.imagegen. Do not answer with text before or after the tool call. Preserve the user's requested image or edit intent when building the tool arguments. For a brand new image, provide only prompt and omit both referenced_image_paths and num_last_images_to_include. For an edit, use referenced_image_paths only when every target has a local path; otherwise use num_last_images_to_include with the smallest available recent-image count from 1 through 5. Never provide both image selectors."
	if !hasConversationImages {
		directive += " No conversation images are available in this turn, so you must omit num_last_images_to_include."
	}
	continuation := IsCodexImageGenerationExtensionContinuation(body)
	input, _ := reqBody["input"].([]any)
	_, additionalToolsIndex := codexAdditionalTools(reqBody["input"])
	// A forced image-only orchestration turn exposes exactly one public function.
	// Codex may advertise exec, wait, or other host tools in additional_tools, but
	// forwarding those private host contracts to the upstream makes otherwise
	// valid Responses providers reject the entire request.
	flattenedTools := make([]any, 0, 1)
	properties := map[string]any{
		"prompt": map[string]any{"type": "string"},
		"referenced_image_paths": map[string]any{
			"type":     "array",
			"items":    map[string]any{"type": "string"},
			"maxItems": 5,
		},
	}
	if hasConversationImages {
		properties["num_last_images_to_include"] = map[string]any{
			"type":    "integer",
			"minimum": 1,
			"maximum": 5,
		}
	}
	flattenedTools = append(flattenedTools, map[string]any{
		"type": "function",
		"name": codexImageGenToolName,
		"description": `The image_gen.imagegen tool enables image generation from descriptions and editing of existing images based on specific instructions. Use it when the user requests a new image or wants to modify an attached or previously generated image.

Guidelines:
- In code mode, pass the result to generatedImage(result).
- Omit both referenced_image_paths and num_last_images_to_include when generating a brand new image.
- For edits, use referenced_image_paths when every target image has a local file path.
- If you have not seen a local image yet, use view_image to inspect it before editing.
- Use num_last_images_to_include only when at least one target image has no local file path.
- Set num_last_images_to_include to the smallest number of recent conversation images that includes every target image, up to 5.
- Never provide both referenced_image_paths and num_last_images_to_include.
- If neither mechanism can include every target image, ask the user to attach the missing images again.
- Directly generate the image without reconfirmation or clarification unless required images must be attached again.
- After image generation, do not add text, mention downloads, summarize the image, or ask a follow-up question.
- Always use this tool for image editing unless the user explicitly requests otherwise.`,
		"parameters": map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             []string{"prompt"},
			"additionalProperties": false,
		},
	})

	// The model-facing contract is always the public Responses function-tool
	// shape. Codex Desktop may omit its local image_gen namespace or older
	// clients may carry it in a private additional_tools item; neither changes
	// what the upstream orchestration model receives.
	reqBody["tools"] = flattenedTools
	if additionalToolsIndex >= 0 && additionalToolsIndex < len(input) {
		input = append(input[:additionalToolsIndex], input[additionalToolsIndex+1:]...)
		reqBody["input"] = input
	}
	if !continuation {
		instructions := strings.TrimSpace(firstNonEmptyString(reqBody["instructions"]))
		if !strings.Contains(instructions, directive) {
			if instructions == "" {
				reqBody["instructions"] = directive
			} else {
				reqBody["instructions"] = instructions + "\n\n" + directive
			}
		}
		reqBody["tool_choice"] = map[string]any{
			"type": "function",
			"name": codexImageGenToolName,
		}
	} else if codexImageToolChoiceSelected(reqBody["tool_choice"]) {
		delete(reqBody, "tool_choice")
	}
	updated, err := json.Marshal(reqBody)
	if err != nil {
		return body, false, err
	}
	return updated, true, nil
}

func codexAdditionalTools(value any) ([]any, int) {
	input, _ := value.([]any)
	for i, rawItem := range input {
		item, _ := rawItem.(map[string]any)
		if strings.TrimSpace(firstNonEmptyString(item["type"])) != "additional_tools" {
			continue
		}
		tools, _ := item["tools"].([]any)
		return tools, i
	}
	return nil, -1
}

func codexRequestHasConversationImages(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if codexRequestHasConversationImages(item) {
				return true
			}
		}
	case map[string]any:
		itemType := strings.TrimSpace(firstNonEmptyString(typed["type"]))
		if itemType == "input_image" && strings.TrimSpace(firstNonEmptyString(typed["image_url"])) != "" {
			return true
		}
		if itemType == "image_generation_call" && strings.TrimSpace(firstNonEmptyString(typed["result"])) != "" {
			return true
		}
		for _, nested := range typed {
			if codexRequestHasConversationImages(nested) {
				return true
			}
		}
	}
	return false
}

// ImageGenerationPermissionMessage returns the stable end-user error text for disabled groups.
func ImageGenerationPermissionMessage() string {
	return imageGenerationPermissionMessage
}

// GroupAllowsImageGeneration preserves ungrouped-key behavior and enforces the flag when a group is present.
func GroupAllowsImageGeneration(group *Group) bool {
	return group == nil || group.AllowImageGeneration
}

// IsImageGenerationIntent classifies requests that can produce generated images.
func IsImageGenerationIntent(endpoint string, requestedModel string, body []byte) bool {
	if IsImageGenerationEndpoint(endpoint) {
		return true
	}
	if isOpenAIImageGenerationModel(requestedModel) {
		return true
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	if model := strings.TrimSpace(gjson.GetBytes(body, "model").String()); isOpenAIImageGenerationModel(model) {
		return true
	}
	if openAIJSONToolsContainImageGeneration(gjson.GetBytes(body, "tools")) {
		return true
	}
	return openAIJSONToolChoiceSelectsImageGeneration(gjson.GetBytes(body, "tool_choice"))
}

// IsImageGenerationPermissionIntent classifies requests that must be blocked when image generation is disabled.
// A plain image_generation tool declaration is only a capability advertisement for Codex Desktop and is not enough.
func IsImageGenerationPermissionIntent(endpoint string, requestedModel string, body []byte) bool {
	if IsImageGenerationEndpoint(endpoint) {
		return true
	}
	if isOpenAIImageGenerationModel(requestedModel) {
		return true
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	if model := strings.TrimSpace(gjson.GetBytes(body, "model").String()); isOpenAIImageGenerationModel(model) {
		return true
	}
	return openAIJSONToolChoiceSelectsImageGeneration(gjson.GetBytes(body, "tool_choice"))
}

// IsImageGenerationIntentMap is the map-backed variant used after service-side request mutation.
func IsImageGenerationIntentMap(endpoint string, requestedModel string, reqBody map[string]any) bool {
	if IsImageGenerationEndpoint(endpoint) {
		return true
	}
	if isOpenAIImageGenerationModel(requestedModel) {
		return true
	}
	if reqBody == nil {
		return false
	}
	if isOpenAIImageGenerationModel(firstNonEmptyString(reqBody["model"])) {
		return true
	}
	if hasOpenAIImageGenerationTool(reqBody) {
		return true
	}
	return openAIAnyToolChoiceSelectsImageGeneration(reqBody["tool_choice"])
}

// IsImageGenerationPermissionIntentMap is the map-backed permission-gate variant.
func IsImageGenerationPermissionIntentMap(endpoint string, requestedModel string, reqBody map[string]any) bool {
	if IsImageGenerationEndpoint(endpoint) {
		return true
	}
	if isOpenAIImageGenerationModel(requestedModel) {
		return true
	}
	if reqBody == nil {
		return false
	}
	if isOpenAIImageGenerationModel(firstNonEmptyString(reqBody["model"])) {
		return true
	}
	return openAIAnyToolChoiceSelectsImageGeneration(reqBody["tool_choice"])
}

// IsImageGenerationEndpoint identifies dedicated generated-image endpoints.
func IsImageGenerationEndpoint(endpoint string) bool {
	switch normalizeImageGenerationEndpoint(endpoint) {
	case "/v1/images/generations", "/v1/images/edits", "/images/generations", "/images/edits":
		return true
	default:
		return false
	}
}

func normalizeImageGenerationEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(strings.ToLower(endpoint))
	if endpoint == "" {
		return ""
	}
	endpoint = strings.TrimPrefix(endpoint, "https://api.openai.com")
	if idx := strings.IndexByte(endpoint, '?'); idx >= 0 {
		endpoint = endpoint[:idx]
	}
	return strings.TrimRight(endpoint, "/")
}

func openAIJSONToolsContainImageGeneration(tools gjson.Result) bool {
	if !tools.IsArray() {
		return false
	}
	found := false
	tools.ForEach(func(_, item gjson.Result) bool {
		if strings.TrimSpace(item.Get("type").String()) == "image_generation" {
			found = true
			return false
		}
		return true
	})
	return found
}

func openAIJSONToolChoiceSelectsImageGeneration(choice gjson.Result) bool {
	if !choice.Exists() {
		return false
	}
	if choice.Type == gjson.String {
		return strings.TrimSpace(choice.String()) == "image_generation"
	}
	if !choice.IsObject() {
		return false
	}
	if strings.TrimSpace(choice.Get("type").String()) == "image_generation" {
		return true
	}
	if strings.TrimSpace(choice.Get("tool.type").String()) == "image_generation" {
		return true
	}
	if strings.TrimSpace(choice.Get("function.name").String()) == "image_generation" {
		return true
	}
	return false
}

func openAIAnyToolChoiceSelectsImageGeneration(choice any) bool {
	switch v := choice.(type) {
	case string:
		return strings.TrimSpace(v) == "image_generation"
	case map[string]any:
		if strings.TrimSpace(firstNonEmptyString(v["type"])) == "image_generation" {
			return true
		}
		if tool, ok := v["tool"].(map[string]any); ok && strings.TrimSpace(firstNonEmptyString(tool["type"])) == "image_generation" {
			return true
		}
		if fn, ok := v["function"].(map[string]any); ok && strings.TrimSpace(firstNonEmptyString(fn["name"])) == "image_generation" {
			return true
		}
	}
	return false
}

func stripOpenAIImageGenerationToolDeclarations(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	removed := false
	filterTools := func(tools []any) []any {
		filtered := make([]any, 0, len(tools))
		for _, rawTool := range tools {
			toolMap, ok := rawTool.(map[string]any)
			if ok {
				toolType := strings.TrimSpace(firstNonEmptyString(toolMap["type"]))
				toolName := strings.TrimSpace(firstNonEmptyString(toolMap["name"]))
				if toolType == "image_generation" || (toolType == "namespace" && toolName == codexImageGenNamespace) {
					removed = true
					continue
				}
			}
			filtered = append(filtered, rawTool)
		}
		return filtered
	}

	if tools, ok := reqBody["tools"].([]any); ok {
		filtered := filterTools(tools)
		if len(filtered) == 0 {
			delete(reqBody, "tools")
		} else {
			reqBody["tools"] = filtered
		}
	}
	if input, ok := reqBody["input"].([]any); ok {
		filteredInput := make([]any, 0, len(input))
		for _, rawItem := range input {
			item, ok := rawItem.(map[string]any)
			if !ok || strings.TrimSpace(firstNonEmptyString(item["type"])) != "additional_tools" {
				filteredInput = append(filteredInput, rawItem)
				continue
			}
			tools, _ := item["tools"].([]any)
			filteredTools := filterTools(tools)
			if len(filteredTools) == 0 {
				removed = true
				continue
			}
			item["tools"] = filteredTools
			filteredInput = append(filteredInput, item)
		}
		reqBody["input"] = filteredInput
	}
	if removed && (openAIAnyToolChoiceSelectsImageGeneration(reqBody["tool_choice"]) || codexImageToolChoiceSelected(reqBody["tool_choice"])) {
		reqBody["tool_choice"] = "none"
	}
	return removed
}

func codexImageToolChoiceSelected(choice any) bool {
	choiceMap, ok := choice.(map[string]any)
	if !ok {
		return false
	}
	name := strings.TrimSpace(firstNonEmptyString(choiceMap["name"]))
	if function, ok := choiceMap["function"].(map[string]any); ok && name == "" {
		name = strings.TrimSpace(firstNonEmptyString(function["name"]))
	}
	return name == codexImageGenToolName || name == codexImageGenNamespace+"."+codexImageGenToolName
}

func stripOpenAIImageGenerationToolDeclarationsFromBody(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return body, false, err
	}
	if !stripOpenAIImageGenerationToolDeclarations(reqBody) {
		return body, false, nil
	}
	updated, err := json.Marshal(reqBody)
	if err != nil {
		return body, false, err
	}
	return updated, true, nil
}

func getAPIKeyFromContext(c interface{ Get(string) (any, bool) }) *APIKey {
	if c == nil {
		return nil
	}
	v, exists := c.Get("api_key")
	if !exists {
		return nil
	}
	apiKey, _ := v.(*APIKey)
	return apiKey
}

func apiKeyGroup(apiKey *APIKey) *Group {
	if apiKey == nil {
		return nil
	}
	return apiKey.Group
}

func cloneRequestMapForImageIntent(body []byte) map[string]any {
	if len(body) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil
	}
	return out
}

type OpenAIResponsesImageBillingConfig struct {
	Model     string
	SizeTier  string
	InputSize string
}

func resolveOpenAIResponsesImageBillingConfigDetailed(reqBody map[string]any, fallbackModel string) (OpenAIResponsesImageBillingConfig, error) {
	imageModel := ""
	imageSize := ""
	hasImageTool := false
	if reqBody != nil {
		rawTools, _ := reqBody["tools"].([]any)
		for _, rawTool := range rawTools {
			toolMap, ok := rawTool.(map[string]any)
			if !ok || strings.TrimSpace(firstNonEmptyString(toolMap["type"])) != "image_generation" {
				continue
			}
			hasImageTool = true
			imageModel = strings.TrimSpace(firstNonEmptyString(toolMap["model"]))
			imageSize = strings.TrimSpace(firstNonEmptyString(toolMap["size"]))
			break
		}
		if imageSize == "" {
			imageSize = strings.TrimSpace(firstNonEmptyString(reqBody["size"]))
		}
	}
	if imageModel == "" && reqBody != nil {
		bodyModel := strings.TrimSpace(firstNonEmptyString(reqBody["model"]))
		if isOpenAIImageBillingModelAlias(bodyModel) || !hasImageTool {
			imageModel = bodyModel
		}
	}
	if imageModel == "" && hasImageTool {
		imageModel = "gpt-image-2"
	}
	if imageModel == "" {
		imageModel = strings.TrimSpace(fallbackModel)
	}
	sizeTier := normalizeOpenAIImageSizeTier(imageSize)
	return OpenAIResponsesImageBillingConfig{
		Model:     imageModel,
		SizeTier:  sizeTier,
		InputSize: imageSize,
	}, nil
}

func resolveOpenAIResponsesImageBillingConfigFromBody(body []byte, fallbackModel string) (string, string, error) {
	cfg, err := resolveOpenAIResponsesImageBillingConfigDetailedFromBody(body, fallbackModel)
	if err != nil {
		return "", "", err
	}
	return cfg.Model, cfg.SizeTier, nil
}

func resolveOpenAIResponsesImageBillingConfigDetailedFromBody(body []byte, fallbackModel string) (OpenAIResponsesImageBillingConfig, error) {
	reqBody := cloneRequestMapForImageIntent(body)
	return resolveOpenAIResponsesImageBillingConfigDetailed(reqBody, fallbackModel)
}

func ResolveOpenAIResponsesImageBillingConfigDetailedForBridge(body []byte, fallbackModel string) (OpenAIResponsesImageBillingConfig, error) {
	return resolveOpenAIResponsesImageBillingConfigDetailedFromBody(body, fallbackModel)
}

func ExtractUpstreamErrorMessageForBridge(body []byte) string {
	return extractUpstreamErrorMessage(body)
}

func isOpenAIImageBillingModelAlias(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return false
	}
	return isOpenAIImageGenerationModel(normalized) || strings.Contains(normalized, "image")
}
