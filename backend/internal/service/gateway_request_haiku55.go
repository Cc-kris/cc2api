package service

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
)

// ValidateHaiku55Request enforces the V05 native Haiku 5.5 contract before
// mimicry or sampling-conflict cleanup can hide the client's original fields.
// Responses requests are validated by apicompat.ResponsesToAnthropicRequest.
func ValidateHaiku55Request(body []byte, model string) error {
	if !claude.IsHaiku55(model) {
		return nil
	}

	effort := gjson.GetBytes(body, "output_config.effort")
	if effort.Exists() {
		if effort.Type != gjson.String {
			return fmt.Errorf("claude-haiku-5-5 output_config.effort must be a string")
		}
		switch effort.String() {
		case "low", "medium", "high", "xhigh", "max":
		default:
			return fmt.Errorf("claude-haiku-5-5 does not support output_config.effort %q", effort.String())
		}
	}

	switch thinkingType := gjson.GetBytes(body, "thinking.type").String(); thinkingType {
	case "", "adaptive":
	case "disabled":
		switch effort.String() {
		case "xhigh", "max":
			return fmt.Errorf("claude-haiku-5-5 thinking.type=disabled supports only low, medium or high effort")
		}
	case "between_tools":
		return fmt.Errorf("claude-haiku-5-5 requires adaptive thinking; thinking.type=between_tools is unsupported (use thinking.type=disabled at high effort or below)")
	case "enabled":
		return fmt.Errorf("claude-haiku-5-5 requires adaptive thinking; omit thinking or use thinking.type=adaptive and output_config.effort")
	}

	temperature := gjson.GetBytes(body, "temperature")
	topP := gjson.GetBytes(body, "top_p")
	if temperature.Exists() && topP.Exists() {
		return fmt.Errorf("claude-haiku-5-5 does not allow temperature and top_p together; omit both")
	}
	if temperature.Exists() && (temperature.Type != gjson.Number || temperature.Float() != 1) {
		return fmt.Errorf("claude-haiku-5-5 does not support non-default temperature")
	}
	if topP.Exists() && (topP.Type != gjson.Number || topP.Float() < 0.99 || topP.Float() > 1) {
		return fmt.Errorf("claude-haiku-5-5 does not support non-default top_p")
	}
	return nil
}
