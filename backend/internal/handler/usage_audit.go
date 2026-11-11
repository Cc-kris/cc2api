package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// V03-R11 用量审计：在提交异步用量任务前（仍持有 gin.Context 时）补齐只读审计字段。
// 这些字段只落库展示，不参与计费。

func stampForwardUsageAudit(c *gin.Context, result *service.ForwardResult) {
	if result == nil {
		return
	}
	if result.UpstreamResponseModel == "" {
		result.UpstreamResponseModel = service.ObservedUpstreamResponseModel(c)
	}
	if result.RequestedReasoningEffort == nil {
		result.RequestedReasoningEffort = service.RequestedReasoningEffortFromGin(c)
	}
	if result.UpstreamHeaders == nil {
		result.UpstreamHeaders = service.ObservedUpstreamResponseHeaders(c)
	}
}

func stampOpenAIUsageAudit(c *gin.Context, result *service.OpenAIForwardResult) {
	if result == nil {
		return
	}
	// WS 轮次不经过 HTTP 响应观测器，gin 上下文里的值可能属于同连接的其他请求，不采用。
	if !result.OpenAIWSMode {
		if result.UpstreamResponseModel == "" {
			result.UpstreamResponseModel = service.ObservedUpstreamResponseModel(c)
		}
		if result.UpstreamHeaders == nil {
			result.UpstreamHeaders = service.ObservedUpstreamResponseHeaders(c)
		}
	}
	if result.RequestedReasoningEffort == nil {
		result.RequestedReasoningEffort = service.RequestedReasoningEffortFromGin(c)
	}
}
