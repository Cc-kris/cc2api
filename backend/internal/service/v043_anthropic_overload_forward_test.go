//go:build unit

package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type v043AnthropicLoopback struct {
	target *url.URL
	client *http.Client
}

func (u *v043AnthropicLoopback) Do(r *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	r.URL.Scheme = u.target.Scheme
	r.URL.Host = u.target.Host
	r.Host = u.target.Host
	return u.client.Do(r)
}
func (u *v043AnthropicLoopback) DoWithTLS(r *http.Request, p string, id int64, c int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, p, id, c)
}
func TestV043AnthropicPreOutputOverloadRealForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "passthrough"}[passthrough], func(t *testing.T) {
			hits := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				w.Header().Set("Content-Type", "text/event-stream")
				if hits <= 3 {
					io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"failed-attempt\",\"usage\":{\"input_tokens\":1}}}\n\nevent: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n")
					return
				}
				io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"success\",\"usage\":{\"input_tokens\":1}}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
			}))
			defer upstream.Close()
			target, _ := url.Parse(upstream.URL)
			cfg := &config.Config{}
			cfg.Gateway.MaxLineSize = defaultMaxLineSize
			repo := &errorPolicyRepoStub{}
			svc := &GatewayService{cfg: cfg, httpUpstream: &v043AnthropicLoopback{target: target, client: upstream.Client()}, rateLimitService: NewRateLimitService(repo, nil, cfg, nil, nil)}
			a := newAnthropicAPIKeyAccountForTest()
			a.Extra["anthropic_passthrough"] = passthrough
			a.Credentials["pool_mode"] = true
			a.Credentials["pool_mode_retry_count"] = 2
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":20,"stream":true,"messages":[{"role":"user","content":"hello"}]}`)
			parsed := &ParsedRequest{Body: body, Model: "claude-sonnet-4-5", Stream: true}
			for range 3 {
				_, err := svc.Forward(context.Background(), c, a, parsed)
				var failure *UpstreamFailoverError
				require.True(t, errors.As(err, &failure), "%v", err)
				require.Equal(t, 529, failure.StatusCode)
				require.True(t, failure.RetryableOnSameAccount)
				require.Empty(t, rec.Body.String())
				svc.TempUnscheduleRetryableError(context.Background(), a.ID, failure)
			}
			a.ID++ // The exhausted account is switched by the handler; emulate its next candidate.
			result, err := svc.Forward(context.Background(), c, a, parsed)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 4, hits)
			require.Contains(t, rec.Body.String(), "hello")
			require.NotContains(t, rec.Body.String(), "failed-attempt")
			require.Zero(t, repo.tempCalls)
			require.Zero(t, repo.setErrCalls)
		})
	}
}
