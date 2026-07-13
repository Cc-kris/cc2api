package service

import (
	"encoding/json"
	"strconv"
	"strings"
)

const featureKeyCodexImageGenerationBridge = "codex_image_generation_bridge"

func boolOverridePtr(v bool) *bool {
	return &v
}

func platformBoolOverride(values map[string]any, key string, platform string) *bool {
	if values == nil {
		return nil
	}
	if v, ok := values[key].(bool); ok {
		return boolOverridePtr(v)
	}
	raw, ok := values[key].(map[string]any)
	if !ok {
		return nil
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return nil
	}
	if v, ok := raw[platform].(bool); ok {
		return boolOverridePtr(v)
	}
	return nil
}

// CodexImageGenerationBridgeOverride returns the channel-owned switch for the
// special Codex image-extension route. Nil means disabled at runtime.
func (c *Channel) CodexImageGenerationBridgeOverride(platform string) *bool {
	if c == nil {
		return nil
	}
	return platformBoolOverride(c.FeaturesConfig, featureKeyCodexImageGenerationBridge, platform)
}

// CodexImageGenerationOrchestratorGroupID returns the text-only group used to
// produce the local image_gen tool call. The actual Images request continues to
// use the API key's dedicated image group.
func (c *Channel) CodexImageGenerationOrchestratorGroupID() *int64 {
	if c == nil || c.FeaturesConfig == nil {
		return nil
	}
	raw, ok := c.FeaturesConfig[featureKeyCodexImageGenerationBridge].(map[string]any)
	if !ok {
		return nil
	}
	value, ok := raw["orchestrator_group_id"]
	if !ok {
		return nil
	}
	var groupID int64
	switch typed := value.(type) {
	case int:
		groupID = int64(typed)
	case int64:
		groupID = typed
	case float64:
		candidate := int64(typed)
		if float64(candidate) != typed {
			return nil
		}
		groupID = candidate
	case json.Number:
		groupID, _ = typed.Int64()
	case string:
		groupID, _ = strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
	}
	if groupID <= 0 {
		return nil
	}
	return &groupID
}
