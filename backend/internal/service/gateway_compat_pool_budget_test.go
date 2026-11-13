//go:build unit

package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnthropicCompatibilityPoolFailoverPreservesPolicy(t *testing.T) {
	for _, path := range []string{"chat", "responses"} {
		for _, tc := range []struct {
			name      string
			codes     []any
			status    int
			retryable bool
		}{
			{"included", []any{float64(502)}, 502, true},
			{"excluded", []any{float64(502)}, 401, false},
			{"disabled", []any{}, 502, false},
			{"custom", []any{float64(418)}, 418, true},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				upstream := &geminiSequenceUpstreamStub{responses: []geminiSequenceUpstreamResponse{{status: tc.status, body: `{"error":{"message":"busy"}}`}}}
				cfg := &config.Config{}
				repo := &errorPolicyRepoStub{}
				svc := &GatewayService{cfg: cfg, httpUpstream: upstream, rateLimitService: NewRateLimitService(repo, nil, cfg, nil, nil)}
				a := newAnthropicAPIKeyAccountForTest()
				a.Credentials["pool_mode"] = true
				a.Credentials["pool_mode_retry_status_codes"] = tc.codes
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/"+path, nil)
				var err error
				if path == "chat" {
					_, err = svc.ForwardAsChatCompletions(context.Background(), c, a, []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],"stream":true}`), nil)
				} else {
					_, err = svc.ForwardAsResponses(context.Background(), c, a, []byte(`{"model":"claude-sonnet-4-5","input":"hi","stream":true}`), nil)
				}
				var failover *UpstreamFailoverError
				require.True(t, errors.As(err, &failover), "%v", err)
				require.Equal(t, tc.retryable, failover.RetryableOnSameAccount)
				require.Equal(t, GatewayFailureReasonPoolStatus, failover.Reason)
				require.Equal(t, tc.status, failover.StatusCode)
				require.JSONEq(t, `{"error":{"message":"busy"}}`, string(failover.ResponseBody))
				require.Equal(t, 1, upstream.calls)
				require.Zero(t, repo.setErrCalls)
				require.Zero(t, repo.tempCalls)
			})
		}
	}
}
