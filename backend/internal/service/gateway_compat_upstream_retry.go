package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// doAnthropicCompatUpstream preserves the native Messages service retry budget.
// Configured pool retry statuses are exclusively retried by the handler. Account error-code
// exclusions suppress state mutations, not the native same-account retries.
// send must rebuild the request/context on each call (closing an earlier
// response may release its adopted upstream context).
func (s *GatewayService) doAnthropicCompatUpstream(ctx context.Context, c *gin.Context, account *Account, send func() (*http.Response, error)) (*http.Response, error) {
	controlCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if c != nil && c.Request != nil {
		if err := c.Request.Context().Err(); err != nil {
			return nil, err
		}
		stop := context.AfterFunc(c.Request.Context(), cancel)
		defer stop()
	}
	start := time.Now()
	for attempt := 1; ; attempt++ {
		if err := controlCtx.Err(); err != nil {
			return nil, err
		}
		resp, err := send()
		if ctxErr := controlCtx.Err(); ctxErr != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return nil, ctxErr
		}
		if err != nil {
			return resp, err
		}
		if resp == nil || resp.Body == nil {
			return nil, errors.New("upstream request failed: empty response")
		}
		if resp.StatusCode < 400 || !s.shouldRetryUpstreamError(account, resp.StatusCode) || peekUserContentRejection(resp) || (account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)) {
			return resp, nil
		}
		delay := anthropicCompatRetryDelay(attempt, time.Since(start))
		if delay <= 0 {
			return resp, nil
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
			UpstreamStatusCode: resp.StatusCode, UpstreamRequestID: resp.Header.Get("x-request-id"),
			Kind: "retry", Message: sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(body)),
		})
		if err := sleepWithContext(controlCtx, delay); err != nil {
			return nil, err
		}
	}
}

func anthropicCompatRetryDelay(attempt int, elapsed time.Duration) time.Duration {
	remaining := maxRetryElapsed - elapsed
	if attempt >= maxRetryAttempts || remaining <= 0 {
		return 0
	}
	delay := retryBackoffDelay(attempt)
	if delay > remaining {
		return remaining
	}
	return delay
}
