package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/tidwall/gjson"
)

// anyErrorTransportFailoverBody 网络/传输错误转为 failover 时附带的错误体。
// 与各平台原先直接写回的 502 文案一致，failover 耗尽后客户端看到的内容不变。
var anyErrorTransportFailoverBody = []byte(`{"error":{"type":"upstream_error","message":"Upstream request failed"}}`)

// FailoverOnAnyErrorEnabled 报告"任意上游错误都切换账号"模式是否生效。
//
// 生效条件（同时满足）：
//  1. 全局总开关 gateway.failover_on_any_error 开启（默认开启，用于紧急一键回滚）；
//  2. 请求所属分组的 failover_on_any_error 开启（分组级开关，默认开启）。
//
// cfg 为 nil 时视为关闭，保持旧行为（单测、未注入配置的场景）。
// ctx 中没有分组信息时，仅按全局开关判断。
func FailoverOnAnyErrorEnabled(ctx context.Context, cfg *config.Config) bool {
	if cfg == nil || !cfg.Gateway.FailoverOnAnyError {
		return false
	}
	if g := groupFromFailoverContext(ctx); g != nil {
		return g.FailoverOnAnyError
	}
	return true
}

func groupFromFailoverContext(ctx context.Context) *Group {
	if ctx == nil {
		return nil
	}
	if g, ok := ctx.Value(ctxkey.Group).(*Group); ok && g != nil {
		return g
	}
	return nil
}

// failoverOnAnyHTTPError 在开启模式下，任意 >=400 状态码都触发账号切换。
// 用于覆盖"分组内不同账号模型列表不一致"——账号返回 400/404 model not found 时
// 继续尝试分组内其他账号。
func failoverOnAnyHTTPError(ctx context.Context, cfg *config.Config, statusCode int) bool {
	return statusCode >= http.StatusBadRequest && FailoverOnAnyErrorEnabled(ctx, cfg)
}

// isUserContentRejectionBody 识别"内容审核拒绝 / 图片生成用户错误"这类由请求内容本身导致的错误。
// 换账号不会改变结果，这类错误不触发任意错误换号，直接返回给用户。
func isUserContentRejectionBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "error.code").String()))
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "error.type").String()))
	switch code {
	case "moderation_blocked", "content_policy_violation", "content_filter":
		return true
	}
	switch errType {
	case "image_generation_user_error", "content_policy_violation":
		return true
	}
	return false
}

// peekUserContentRejection 读取 4xx 响应体判断是否为内容审核/用户内容错误，读取后恢复 Body 供后续处理。
// 这类错误换号、重试都不会改变结果，调用方应跳过同账号重试与 failover，直接把错误返回给用户。
func peekUserContentRejection(resp *http.Response) bool {
	if resp == nil || resp.Body == nil || resp.StatusCode < 400 || resp.StatusCode >= 500 {
		return false
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return isUserContentRejectionBody(body)
}

// failoverOnAnyHTTPErrorWithBody 是带响应体的版本：内容审核/图片用户错误不换号。
func failoverOnAnyHTTPErrorWithBody(ctx context.Context, cfg *config.Config, statusCode int, body []byte) bool { //nolint:unused // local-fork helper retained from earlier upstream syncs; re-wiring is out of scope for the lint fix
	if isUserContentRejectionBody(body) {
		return false
	}
	return failoverOnAnyHTTPError(ctx, cfg, statusCode)
}

// newAnyErrorTransportFailover 在开启模式下把网络/传输错误（未收到 HTTP 状态码）
// 转换为 UpstreamFailoverError，让 handler 切换到下一个账号，而不是直接把 502 写回客户端。
// 客户端已断开（context canceled）时返回 nil，调用方保持原有处理。
func newAnyErrorTransportFailover(ctx context.Context, cfg *config.Config, err error) *UpstreamFailoverError {
	if err == nil || !FailoverOnAnyErrorEnabled(ctx, cfg) {
		return nil
	}
	if errors.Is(err, context.Canceled) || (ctx != nil && ctx.Err() != nil) {
		return nil
	}
	return &UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: anyErrorTransportFailoverBody,
	}
}

// antigravityConfig 返回 Antigravity 服务可用的全局配置（经 settingService 注入，可能为 nil）。
func (s *AntigravityGatewayService) antigravityConfig() *config.Config {
	if s == nil || s.settingService == nil {
		return nil
	}
	return s.settingService.cfg
}
