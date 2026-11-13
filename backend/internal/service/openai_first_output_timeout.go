package service

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

type openAIHighEffortContextKey struct{}

func prefixHeartbeat(c *gin.Context, anthropic bool) func() error {
	if c == nil || c.Writer == nil {
		return nil
	}
	return func() error {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Accel-Buffering", "no")
		heartbeat := ":\n\n"
		if anthropic {
			heartbeat = "event: ping\ndata: {\"type\": \"ping\"}\n\n"
		}
		_, err := fmt.Fprint(c.Writer, heartbeat)
		if err == nil {
			c.Writer.Flush()
		}
		return err
	}
}

func (s *OpenAIGatewayService) openAIFirstOutputTimeout(ctx context.Context) time.Duration {
	if s == nil || s.cfg == nil {
		return 0
	}
	seconds := s.cfg.Gateway.OpenAIFirstOutputTimeoutSeconds
	if high, _ := ctx.Value(openAIHighEffortContextKey{}).(bool); high && s.cfg.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds > 0 {
		seconds = s.cfg.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}
func (s *OpenAIGatewayService) guardResponsesFirstOutput(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, passthrough bool) error {
	var idle time.Duration
	if s.cfg != nil {
		idle = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	var keepalive time.Duration
	if s.cfg != nil {
		keepalive = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}
	r := guardStreamFirstOutputWithKeepalive(ctx, resp, s.openAIFirstOutputTimeout(ctx), false, idle, keepalive, prefixHeartbeat(c, false))
	if r.status == 0 {
		return r.err
	}
	message := extractOpenAISSEErrorMessage(r.payload)
	if message == "" {
		message = r.reason
	}
	failure := s.newOpenAIStreamFailoverError(c, account, passthrough, resp.Header.Get("x-request-id"), r.payload, message)
	failure.StatusCode = r.status
	failure.Reason = GatewayFailureReasonPreOutput
	failure.ResponseHeaders = resp.Header.Clone()
	// Metadata and capacity are not account health failures. Let the common
	// handler own the sole retry budget; don't enter the internal disconnect loop.
	failure.RetryableOnSameAccount = account != nil && account.IsPoolMode() && account.IsPoolModeRetryableStatus(r.status)
	// Keep the canonical error envelope produced above. Replacing it with a
	// response.failed event loses error.type and breaks V02 failover diagnostics.
	return failure
}
func (s *GatewayService) guardAnthropicFirstOutput(ctx context.Context, resp *http.Response, c *gin.Context, account *Account) error {
	var idle time.Duration
	if s.cfg != nil {
		idle = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	var keepalive time.Duration
	if s.cfg != nil {
		keepalive = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}
	r := guardStreamFirstOutputWithKeepalive(ctx, resp, 0, true, idle, keepalive, prefixHeartbeat(c, true))
	if r.reason == "upstream stream data interval timeout before output" {
		return r.err
	}
	if r.status == 0 {
		return r.err
	}
	return &UpstreamFailoverError{StatusCode: r.status, Reason: GatewayFailureReasonPreOutput, ResponseBody: r.payload, ResponseHeaders: resp.Header.Clone(), RetryableOnSameAccount: account != nil && account.IsPoolMode() && account.IsPoolModeRetryableStatus(r.status)}
}
