//go:build unit

package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestV043OpenAIFivePathsPoolStatusBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"responses_native", "responses_passthrough", "chat_completions", "messages", "responses_chat_fallback"} {
		t.Run(path, func(t *testing.T) {
			upstream := &geminiSequenceUpstreamStub{responses: []geminiSequenceUpstreamResponse{{status: 418, body: `{"error":{"message":"busy"}}`}}}
			cfg := &config.Config{}
			repo := &errorPolicyRepoStub{}
			rate := NewRateLimitService(repo, nil, cfg, nil, nil)
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, rateLimitService: rate}
			a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture", "pool_mode": true, "pool_mode_retry_count": 2, "pool_mode_retry_status_codes": []any{float64(418)}}, Extra: map[string]any{"openai_responses_mode": "force_responses", "openai_passthrough": path == "responses_passthrough"}}
			responses := []byte(`{"model":"gpt-4o","instructions":"fixture","input":"hi","stream":false}`)
			chat := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
			messages := []byte(`{"model":"gpt-4o","max_tokens":20,"messages":[{"role":"user","content":"hi"}]}`)
			for range 3 {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				var err error
				switch path {
				case "responses_native", "responses_passthrough":
					_, err = svc.Forward(context.Background(), c, a, responses)
				case "chat_completions":
					_, err = svc.ForwardAsChatCompletions(context.Background(), c, a, chat, "", "")
				case "messages":
					_, err = svc.ForwardAsAnthropic(context.Background(), c, a, messages, "", "")
				default:
					_, err = svc.forwardResponsesViaRawChatCompletions(context.Background(), c, a, responses)
				}
				var failure *UpstreamFailoverError
				require.True(t, errors.As(err, &failure), "%v", err)
				require.Equal(t, 418, failure.StatusCode)
				require.True(t, failure.RetryableOnSameAccount)
				require.Equal(t, GatewayFailureReasonPoolStatus, failure.Reason)
			}
			require.Equal(t, 3, upstream.calls)
			require.Zero(t, repo.tempCalls)
			require.Zero(t, repo.setErrCalls)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(a))
		})
	}
}
