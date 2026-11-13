package service

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"strings"
)

func isOpenAIUpstreamCapacityShedEvent(payload []byte) bool {
	for _, path := range []string{"response.error.code", "error.code"} {
		switch strings.ToLower(gjson.GetBytes(payload, path).String()) {
		case "server_is_overloaded", "slow_down":
			return true
		}
	}
	message := strings.ToLower(gjson.GetBytes(payload, "response.error.message").String() + " " + gjson.GetBytes(payload, "error.message").String())
	return strings.Contains(message, "model is at capacity") || strings.Contains(message, "model at capacity") || strings.Contains(message, "server is overloaded")
}

const openAICapacityShedRetryableClientCode = "server_error"

func sanitizeOpenAICapacityShedErrorCodeForClient(payload []byte) ([]byte, bool) {
	if !gjson.ValidBytes(payload) || !isOpenAIUpstreamCapacityShedEvent(payload) {
		return payload, false
	}
	updated := append([]byte(nil), payload...)
	changed := false
	for _, path := range []string{"response.error.code", "error.code"} {
		parent := strings.TrimSuffix(path, ".code")
		if !gjson.GetBytes(updated, parent).Exists() {
			continue
		}
		code := strings.ToLower(gjson.GetBytes(updated, path).String())
		if code != "" && code != "server_is_overloaded" && code != "slow_down" {
			continue
		}
		next, err := sjson.SetBytes(updated, path, openAICapacityShedRetryableClientCode)
		if err != nil {
			return payload, false
		}
		updated = next
		changed = true
	}
	return updated, changed
}
