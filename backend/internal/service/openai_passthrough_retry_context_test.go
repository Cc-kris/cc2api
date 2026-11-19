package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type rejectedFieldLifecycleBody struct {
	io.Reader
	ctx context.Context
}

func (b *rejectedFieldLifecycleBody) Close() error {
	ReleaseDetachedUpstreamContext(b.ctx)
	return nil
}

type rejectedFieldLifecycleUpstream struct {
	HTTPUpstream
	calls         int
	bodies        [][]byte
	contexts      []context.Context
	keepRejecting bool
}

func (u *rejectedFieldLifecycleUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	u.calls++
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.bodies = append(u.bodies, body)
	u.contexts = append(u.contexts, req.Context())
	status := http.StatusOK
	payload := `{"id":"resp_retry","object":"response","created_at":123,"status":"completed","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
	if u.calls == 1 || u.keepRejecting {
		status = http.StatusBadRequest
		payload = `{"error":{"code":"unsupported_parameter","type":"invalid_request_error","param":"input[0].status","message":"Unsupported parameter: 'input[0].status'."}}`
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       &rejectedFieldLifecycleBody{Reader: strings.NewReader(payload), ctx: req.Context()},
	}, nil
}

func TestOpenAIPassthroughRejectedFieldRetryOwnsFreshContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, keepRejecting := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "one_retry_only"}[keepRejecting], func(t *testing.T) {
			upstream := &rejectedFieldLifecycleUpstream{keepRejecting: keepRejecting}
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = true
			cfg.Security.URLAllowlist.UpstreamHosts = []string{"compat.example"}
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			account := newOpenAIRejectedFieldTestAccount()
			account.Extra["openai_passthrough"] = true
			body := []byte(`{"model":"gpt-5.5","stream":false,"input":[{"type":"message","role":"user","content":"retry-context","status":"completed"},{"type":"message","role":"assistant","content":"prior","status":"completed"},{"type":"other","status":"completed"}]}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			parent := c.Request.Context()
			result, err := svc.Forward(parent, c, account, body)
			require.Equal(t, 2, upstream.calls, "retry must reach transport with a live context")
			require.ErrorIs(t, upstream.contexts[0].Err(), context.Canceled)
			require.ErrorIs(t, upstream.contexts[1].Err(), context.Canceled, "final response close releases its separate lifecycle")
			require.NoError(t, parent.Err(), "response body cleanup must not cancel original client context")
			require.True(t, gjson.GetBytes(upstream.bodies[0], "input.0.status").Exists())
			require.False(t, gjson.GetBytes(upstream.bodies[1], "input.0.status").Exists())
			require.False(t, gjson.GetBytes(upstream.bodies[1], "input.1.status").Exists(), "rejected status is stripped for the whole message class")
			require.True(t, gjson.GetBytes(upstream.bodies[1], "input.2.status").Exists(), "other item classes remain unchanged")
			if keepRejecting {
				require.Error(t, err)
				require.Equal(t, http.StatusBadRequest, recorder.Code)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, http.StatusOK, recorder.Code)
			}
		})
	}
}
