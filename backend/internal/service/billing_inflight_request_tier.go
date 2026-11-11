package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

// ============================================
// 准入时的请求档位（结算只降不升，请求档位是上界）
// ============================================

// InflightAnthropicSpeedTier Anthropic /v1/messages 请求的上界档位：speed=fast 且请求模型或渠道映射
// 模型属于 Opus 5 / 4.8 时返回 "fast"（与 anthropicSpeedServiceTier 的模型判断一致）。准入时还不知道
// 账号，Bedrock 等不加价的账号在结算时按标准价，估算偏高是安全方向。
func InflightAnthropicSpeedTier(speed string, models ...string) string {
	if strings.ToLower(strings.TrimSpace(speed)) != "fast" {
		return ""
	}
	for _, m := range models {
		if modelSupportsAnthropicFastMode(m) {
			return "fast"
		}
	}
	return ""
}

// InflightOpenAIServiceTier OpenAI 请求体 service_tier 的上界档位（fast 归一为 priority）。
func InflightOpenAIServiceTier(body []byte) string {
	if tier := extractOpenAIServiceTierFromBody(body); tier != nil {
		return *tier
	}
	return ""
}

// InflightOpenAIMessagesServiceTier OpenAI 平台 /v1/messages：anthropic-beta 含 fast-mode 时转发为
// service_tier=priority（openai_gateway_messages.go），否则看请求体 service_tier。
func InflightOpenAIMessagesServiceTier(anthropicBeta string, body []byte) string {
	if containsBetaToken(anthropicBeta, claude.BetaFastMode) {
		return "priority"
	}
	return InflightOpenAIServiceTier(body)
}
