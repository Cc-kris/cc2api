package handler

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const openAICompatClientDefaultModelPlaceholder = "codex-current"

func shouldFallbackOpenAIClientModel(model string) bool {
	model = strings.TrimSpace(model)
	return model == "" || strings.EqualFold(model, openAICompatClientDefaultModelPlaceholder)
}

func applyOpenAIClientDefaultModelFallback(body []byte, requestedModel, fallbackModel string) ([]byte, string, bool, error) {
	fallbackModel = strings.TrimSpace(fallbackModel)
	if fallbackModel == "" || !shouldFallbackOpenAIClientModel(requestedModel) {
		return body, strings.TrimSpace(requestedModel), false, nil
	}
	updated, err := sjson.SetBytes(body, "model", fallbackModel)
	if err != nil {
		return nil, "", false, err
	}
	return updated, fallbackModel, true, nil
}

func extractOpenAIRequestModel(body []byte) (string, bool) {
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() {
		return "", true
	}
	if modelResult.Type != gjson.String {
		return "", false
	}
	return strings.TrimSpace(modelResult.String()), true
}
