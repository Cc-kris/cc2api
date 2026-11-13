package service

import "strings"

func isOpenAIDeterministicRequestError(status int, body []byte) bool {
	return status == 400 && !isOpenAITransientProcessingError(status, extractUpstreamErrorMessage(body), body) && !isOpenAIModelUnavailableError(body)
}

// Model-account incompatibility must retain local any-error account switching;
// unlike an invalid schema/parameter, another account may serve this request.
func isOpenAIModelUnavailableError(body []byte) bool {
	code := strings.ToLower(extractUpstreamErrorCode(body))
	if code == "model_not_found" || code == "model_not_available" {
		return true
	}
	message := strings.ToLower(extractUpstreamErrorMessage(body))
	for _, marker := range []string{"model not found", "model does not exist", "unsupported model", "model is not supported", "no such model", "不支持该模型", "模型不存在"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
