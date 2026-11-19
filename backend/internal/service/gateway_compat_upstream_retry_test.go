//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type compatRetryUpstream struct {
	calls    int
	requests []*http.Request
	status   int
	body     string
	onSend   func()
}

func (u *compatRetryUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	u.requests = append(u.requests, req)
	if u.onSend != nil {
		u.onSend()
	}
	return &http.Response{StatusCode: u.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(u.body))}, nil
}
func (u *compatRetryUpstream) DoWithTLS(req *http.Request, proxy string, id int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, n)
}

func TestAnthropicCompatRetryBudgetAndElapsed(t *testing.T) {
	require.Equal(t, retryBackoffDelay(1), anthropicCompatRetryDelay(1, 0))
	require.Equal(t, retryBackoffDelay(2), anthropicCompatRetryDelay(2, 0))
	require.Zero(t, anthropicCompatRetryDelay(maxRetryAttempts, 0))
	require.Zero(t, anthropicCompatRetryDelay(1, maxRetryElapsed))
	require.Zero(t, anthropicCompatRetryDelay(1, maxRetryElapsed+time.Second))
	require.Equal(t, time.Millisecond, anthropicCompatRetryDelay(2, maxRetryElapsed-time.Millisecond))
}

func TestAnthropicCompatExcludedErrorRetriesMatchNativeBudget(t *testing.T) {
	for _, chat := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run("chat="+strconv.FormatBool(chat)+"/stream="+strconv.FormatBool(stream), func(t *testing.T) {
				cfg := &config.Config{}
				cfg.Gateway.FailoverOnAnyError = true
				account := newAnthropicAPIKeyAccountForTest()
				account.Credentials["custom_error_codes_enabled"] = true
				account.Credentials["custom_error_codes"] = []any{float64(401)}
				repo := &rateLimitAccountRepoStub{}
				upstream := &compatRetryUpstream{status: 410, body: `{"error":{"type":"invalid_request_error","message":"Service moved"}}`}
				svc := &GatewayService{cfg: cfg, httpUpstream: upstream, rateLimitService: NewRateLimitService(repo, nil, cfg, nil, nil)}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				body := `{"model":"claude-sonnet-4-5","input":"hello","stream":` + strconv.FormatBool(stream) + `}`
				if chat {
					body = `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}],"stream":` + strconv.FormatBool(stream) + `}`
				}
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
				var err error
				if chat {
					_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(body), nil)
				} else {
					_, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(body), nil)
				}
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, 410, failover.StatusCode)
				require.Equal(t, maxRetryAttempts, upstream.calls)
				require.Equal(t, 0, repo.setErrorCalls)
				require.Equal(t, 0, repo.tempCalls)
				require.False(t, c.Writer.Written())
				for i := 1; i < len(upstream.requests); i++ {
					require.NotSame(t, upstream.requests[i-1], upstream.requests[i])
					require.NoError(t, upstream.requests[i].Context().Err())
				}
			})
		}
	}
}

func TestAnthropicCompatRetryPoolAndContentRejection(t *testing.T) {
	for _, pool := range []bool{false, true} {
		for _, chat := range []bool{false, true} {
			t.Run("pool="+strconv.FormatBool(pool)+"/chat="+strconv.FormatBool(chat), func(t *testing.T) {
				cfg := &config.Config{}
				cfg.Gateway.FailoverOnAnyError = true
				account := newAnthropicAPIKeyAccountForTest()
				account.Credentials["custom_error_codes_enabled"] = true
				account.Credentials["custom_error_codes"] = []any{float64(401)}
				upstream := &compatRetryUpstream{status: 400, body: `{"error":{"code":"content_policy_violation","message":"policy"}}`}
				if pool {
					account.Credentials["pool_mode"] = true
					account.Credentials["pool_mode_retry_status_codes"] = []any{float64(410)}
					upstream.status = 410
					upstream.body = `{"error":{"message":"Service moved"}}`
				}
				svc := &GatewayService{cfg: cfg, httpUpstream: upstream}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				body := `{"model":"claude-sonnet-4-5","input":"hello"}`
				if chat {
					body = `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}]}`
				}
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
				var err error
				if chat {
					_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(body), nil)
				} else {
					_, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(body), nil)
				}
				require.Error(t, err)
				require.Equal(t, 1, upstream.calls)
				var failover *UpstreamFailoverError
				if pool {
					require.ErrorAs(t, err, &failover)
					require.Equal(t, GatewayFailureReasonPoolStatus, failover.Reason)
					require.True(t, failover.RetryableOnSameAccount)
					require.False(t, c.Writer.Written())
				} else {
					require.NotErrorAs(t, err, &failover)
					require.Equal(t, 400, rec.Code)
				}
			})
		}
	}
}

func TestAnthropicCompatRetryPoolNonmatchingStatusUsesNativeBudget(t *testing.T) {
	account := newAnthropicAPIKeyAccountForTest()
	account.Credentials["pool_mode"] = true
	account.Credentials["pool_mode_retry_status_codes"] = []any{float64(503)}
	account.Credentials["custom_error_codes_enabled"] = true
	account.Credentials["custom_error_codes"] = []any{float64(401)}
	svc := &GatewayService{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	calls := 0
	resp, err := svc.doAnthropicCompatUpstream(context.Background(), c, account, func() (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 410, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"moved"}}`))}, nil
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 410, resp.StatusCode)
	require.Equal(t, maxRetryAttempts, calls)
}

func TestAnthropicCompatRetryCancellation(t *testing.T) {
	for _, cancelBefore := range []bool{false, true} {
		t.Run("before="+strconv.FormatBool(cancelBefore), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(ctx)
			account := newAnthropicAPIKeyAccountForTest()
			account.Credentials["custom_error_codes_enabled"] = true
			account.Credentials["custom_error_codes"] = []any{float64(401)}
			svc := &GatewayService{}
			calls := 0
			if cancelBefore {
				cancel()
			}
			start := time.Now()
			_, err := svc.doAnthropicCompatUpstream(context.Background(), c, account, func() (*http.Response, error) {
				calls++
				time.AfterFunc(10*time.Millisecond, cancel)
				return &http.Response{StatusCode: 410, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"moved"}}`))}, nil
			})
			require.ErrorIs(t, err, context.Canceled)
			if cancelBefore {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			require.Less(t, time.Since(start), time.Second)
		})
	}
}

func TestAnthropicCompatCanceledForwardDoesNotFailover(t *testing.T) {
	for _, chat := range []bool{false, true} {
		t.Run(strconv.FormatBool(chat), func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.FailoverOnAnyError = true
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			upstream := &compatRetryUpstream{status: 410}
			svc := &GatewayService{cfg: cfg, httpUpstream: upstream}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(ctx)
			var err error
			if chat {
				_, err = svc.ForwardAsChatCompletions(context.Background(), c, newAnthropicAPIKeyAccountForTest(), []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}]}`), nil)
			} else {
				_, err = svc.ForwardAsResponses(context.Background(), c, newAnthropicAPIKeyAccountForTest(), []byte(`{"model":"claude-sonnet-4-5","input":"hello"}`), nil)
			}
			require.True(t, errors.Is(err, context.Canceled))
			require.Zero(t, upstream.calls)
			require.False(t, c.Writer.Written())
			var fo *UpstreamFailoverError
			require.NotErrorAs(t, err, &fo)
		})
	}
}
