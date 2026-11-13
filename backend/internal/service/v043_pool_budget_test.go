//go:build unit

package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestV043PoolGeminiThreePathsUseOnlyHandlerBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, forwarder := range geminiSingleCheckForwarders {
		t.Run(forwarder.name, func(t *testing.T) {
			for _, status := range []int{429, 418} {
				t.Run(http.StatusText(status), func(t *testing.T) {
					svc, upstream, repo := newGeminiSingleCheckService(geminiSequenceUpstreamResponse{status: status, body: `{"error":{"message":"busy"}}`})
					svc.cfg.Gateway.MaxSameAccountRetriesGemini = 4
					account := &Account{ID: 5, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture", "pool_mode": true, "pool_mode_retry_count": 2, "pool_mode_retry_status_codes": []any{float64(status)}}}
					for range account.GetPoolModeRetryCount() + 1 {
						err, _ := forwarder.forward(t, svc, account)
						var failover *UpstreamFailoverError
						require.True(t, errors.As(err, &failover), "%v", err)
						require.True(t, failover.RetryableOnSameAccount)
						require.Equal(t, GatewayFailureReasonPoolStatus, failover.Reason)
					}
					require.Equal(t, 3, upstream.calls, "Gemini internal retry budget must not multiply pool budget")
					require.Zero(t, repo.setErrCalls)
					require.Zero(t, repo.tempCalls)
				})
			}
		})
	}
}

func TestV043PoolAnthropicForwardUsesOnlyHandlerBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "passthrough"}[passthrough], func(t *testing.T) {
			upstream := &geminiSequenceUpstreamStub{responses: []geminiSequenceUpstreamResponse{{status: 418, body: `{"error":{"message":"busy"}}`}}}
			cfg := &config.Config{}
			cfg.Gateway.MaxLineSize = defaultMaxLineSize
			repo := &errorPolicyRepoStub{}
			svc := &GatewayService{cfg: cfg, httpUpstream: upstream, rateLimitService: NewRateLimitService(repo, nil, cfg, nil, nil)}
			account := newAnthropicAPIKeyAccountForTest()
			account.Extra["anthropic_passthrough"] = passthrough
			account.Credentials["pool_mode"] = true
			account.Credentials["pool_mode_retry_count"] = 2
			account.Credentials["pool_mode_retry_status_codes"] = []any{float64(418)}
			body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`)
			for range 3 {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
				_, err := svc.Forward(context.Background(), c, account, &ParsedRequest{Body: body, Model: "claude-sonnet-4-5"})
				var failover *UpstreamFailoverError
				require.True(t, errors.As(err, &failover), "%v", err)
				require.True(t, failover.RetryableOnSameAccount)
				svc.TempUnscheduleRetryableError(context.Background(), account.ID, failover)
			}
			require.Equal(t, 3, upstream.calls)
			require.Zero(t, repo.setErrCalls)
			require.Zero(t, repo.tempCalls)
		})
	}
}

func TestV043PoolRejectsFractionalStatusCodes(t *testing.T) {
	a := &Account{Credentials: map[string]any{"pool_mode_retry_status_codes": []any{429.5, 401.1, 503.0}}}
	require.Equal(t, []int{503}, a.GetPoolModeRetryStatusCodes())
	for _, status := range []int{400, 401, 403, 429, 502, 503, 504, 529} {
		require.True(t, (&Account{}).IsPoolModeRetryableStatus(status))
	}
}
