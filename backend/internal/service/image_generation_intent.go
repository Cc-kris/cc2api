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
	tools := gjson.GetBytes(body, "tools")
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
// Responses-lite/custom-provider requests can keep extension tools in the
// local runtime without serializing the image_gen namespace in top-level
// tools, so the client-owned turn metadata is the stable fallback signal.
func IsCodexImageGenerationExtensionTurn(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	rawMetadata := strings.TrimSpace(gjson.GetBytes(body, `client_metadata.x-codex-turn-metadata`).String())
	if rawMetadata != "" && gjson.Valid(rawMetadata) {
		return strings.TrimSpace(gjson.Get(rawMetadata, "request_kind").String()) == "turn" &&
			strings.TrimSpace(gjson.Get(rawMetadata, "thread_source").String()) == "user"
	}
	return HasCodexImageGenerationExtensionTool(body)
}

// IsCodexSystemBackgroundTurn recognizes short-lived helper turns created by
// the current Codex desktop host, including thread title and description
// generation. These turns must remain textual even inside an image-only group.
func IsCodexSystemBackgroundTurn(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	rawMetadata := strings.TrimSpace(gjson.GetBytes(body, `client_metadata.x-codex-turn-metadata`).String())
	if rawMetadata == "" || !gjson.Valid(rawMetadata) {
		return false
	}
	if strings.TrimSpace(gjson.Get(rawMetadata, "request_kind").String()) != "turn" ||
		strings.TrimSpace(gjson.Get(rawMetadata, "thread_source").String()) != "system" {
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
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if strings.TrimSpace(item.Get("type").String()) == "function_call" &&
			strings.TrimSpace(item.Get("namespace").String()) == codexImageGenNamespace &&
			strings.TrimSpace(item.Get("name").String()) == codexImageGenToolName {
			found = true
			return false
		}
		if strings.TrimSpace(item.Get("type").String()) == "function_call_output" {
			output := item.Get("output")
			if output.IsArray() {
				output.ForEach(func(_, part gjson.Result) bool {
					if strings.TrimSpace(part.Get("type").String()) == "input_image" {
						found = true
						return false
					}
					return true
				})
				if found {
					return false
				}
			}
		}
		return true
	})
	return found
}

// ShouldUseCodexImageGenerationExtension preserves a channel's text-to-image
// mapping as a forced-image routing signal while keeping the Responses model
// textual so current Codex clients can execute image_gen.imagegen locally.
func ShouldUseCodexImageGenerationExtension(requestedModel, mappedModel string, body []byte) bool {
	return !isOpenAIImageGenerationModel(requestedModel) &&
		isOpenAIImageGenerationModel(mappedModel) &&
		IsCodexImageGenerationExtensionTurn(body)
}

// ShouldBypassCodexSystemBackgroundImageMapping keeps Codex host helper turns
// on their requested text model. The surrounding image-only mapping still
// applies to the actual user turn.
func ShouldBypassCodexSystemBackgroundImageMapping(requestedModel, mappedModel string, body []byte) bool {
	return !isOpenAIImageGenerationModel(requestedModel) &&
		isOpenAIImageGenerationModel(mappedModel) &&
		IsCodexSystemBackgroundTurn(body)
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
	return updated, nil
}

// PrepareCodexImageGenerationExtensionDispatch forces the first mapped image
// turn to call the standalone Codex image tool exactly once. Follow-up turns
// are left untouched so the model can consume the local tool result.
func PrepareCodexImageGenerationExtensionDispatch(body []byte) ([]byte, bool, error) {
	if !IsCodexImageGenerationExtensionTurn(body) || IsCodexImageGenerationExtensionContinuation(body) {
		return body, false, nil
	}
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return body, false, err
	}
	const directive = "This request is routed through an image-only channel. You must call the imagegen function exactly once; the gateway will deliver it to Codex as image_gen.imagegen. Do not answer with text before the tool call. Preserve the user's requested image or edit intent when building the tool arguments. If you provide num_last_images_to_include, it must be an integer from 1 through 5."
	instructions := strings.TrimSpace(firstNonEmptyString(reqBody["instructions"]))
	if strings.Contains(instructions, directive) {
		return body, false, nil
	}
	if instructions == "" {
		reqBody["instructions"] = directive
	} else {
		reqBody["instructions"] = instructions + "\n\n" + directive
	}
	tools, _ := reqBody["tools"].([]any)
	flattenedTools := make([]any, 0, len(tools)+1)
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			flattenedTools = append(flattenedTools, rawTool)
			continue
		}
		toolType := strings.TrimSpace(firstNonEmptyString(tool["type"]))
		toolName := strings.TrimSpace(firstNonEmptyString(tool["name"]))
		if (toolType == "namespace" && toolName == codexImageGenNamespace) ||
			(toolType == "function" && (toolName == codexImageGenToolName || toolName == codexImageGenNamespace+"__"+codexImageGenToolName)) {
			continue
		}
		flattenedTools = append(flattenedTools, tool)
	}
	flattenedTools = append(flattenedTools, map[string]any{
		"type":        "function",
		"name":        codexImageGenToolName,
		"description": "Generate or edit an image in the local Codex client.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{"type": "string"},
				"referenced_image_paths": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
				"num_last_images_to_include": map[string]any{
					"type":    "integer",
					"minimum": 1,
					"maximum": 5,
				},
			},
			"required":             []string{"prompt"},
			"additionalProperties": false,
		},
	})
	reqBody["tools"] = flattenedTools
	reqBody["tool_choice"] = "auto"
	updated, err := json.Marshal(reqBody)
	if err != nil {
		return body, false, err
	}
	return updated, true, nil
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
	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		return false
	}
	tools, ok := rawTools.([]any)
	if !ok {
		return false
	}

	filtered := make([]any, 0, len(tools))
	removed := false
	for _, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]any)
		if ok && strings.TrimSpace(firstNonEmptyString(toolMap["type"])) == "image_generation" {
			removed = true
			continue
		}
		filtered = append(filtered, rawTool)
	}
	if !removed {
		return false
	}
	if len(filtered) == 0 {
		delete(reqBody, "tools")
		return true
	}
	reqBody["tools"] = filtered
	return true
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
