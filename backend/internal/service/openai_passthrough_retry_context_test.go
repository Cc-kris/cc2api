package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	firstStatus   int
	firstPayload  string
	authHeaders   []string
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
	u.authHeaders = append(u.authHeaders, req.Header.Get("Authorization"))
	status := http.StatusOK
	payload := `{"id":"resp_retry","object":"response","created_at":123,"status":"completed","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
	if u.calls == 1 || u.keepRejecting {
		status = http.StatusBadRequest
		payload = `{"error":{"code":"unsupported_parameter","type":"invalid_request_error","param":"input[0].status","message":"Unsupported parameter: 'input[0].status'."}}`
		if u.firstStatus != 0 {
			status, payload = u.firstStatus, u.firstPayload
		}
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       &rejectedFieldLifecycleBody{Reader: strings.NewReader(payload), ctx: req.Context()},
	}, nil
}

func TestResponsesEncryptedContentRetryOwnsFreshContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const invalidEncrypted = `{"error":{"code":"invalid_encrypted_content","message":"Could not decrypt the provided encrypted_content."}}`
	const invalidCompaction = `{"error":{"code":"compaction_decode_error","message":"Could not decode the compaction blob."}}`
	const unrelated = `{"code":"invalid-argument","error":"Invalid temperature value."}`
	for _, tt := range []struct {
		name, item, payload string
		grok, terminal      bool
		status, attempts    int
	}{
		{"passthrough 400", "reasoning", invalidEncrypted, false, false, 400, 2},
		{"passthrough terminal", "reasoning", invalidEncrypted, false, true, 400, 2},
		{"Grok 400", "reasoning", invalidEncrypted, true, false, 400, 2},
		{"Grok 422", "reasoning", invalidEncrypted, true, false, 422, 2},
		{"Grok compaction 422", "compaction", invalidCompaction, true, false, 422, 2},
		{"Grok terminal 422", "reasoning", invalidEncrypted, true, true, 422, 2},
		{"Grok unrelated 400", "reasoning", unrelated, true, true, 400, 1},
		{"Grok unrelated 422", "reasoning", unrelated, true, true, 422, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = true
			cfg.Security.URLAllowlist.UpstreamHosts = []string{"compat.example"}
			upstream := &rejectedFieldLifecycleUpstream{firstStatus: tt.status, firstPayload: tt.payload, keepRejecting: tt.terminal}
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			account := newOpenAIRejectedFieldTestAccount()
			account.Extra["openai_passthrough"] = true
			model := "gpt-5.5"
			if tt.grok {
				account.Platform = PlatformGrok
				model = "grok-4.5"
			}
			item := `{"type":"` + tt.item + `","encrypted_content":"synthetic-cipher"`
			if tt.item == "reasoning" {
				item += `,"summary":[{"type":"summary_text","text":"keep summary"}]`
			}
			body := []byte(`{"model":"` + model + `","stream":false,"input":[` + item + `},{"type":"message","role":"user","content":"keep message"}]}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			parent := c.Request.Context()
			var result *OpenAIForwardResult
			var err error
			if tt.grok {
				result, err = svc.forwardGrokResponses(parent, c, account, body, model, false, time.Now())
			} else {
				result, err = svc.Forward(parent, c, account, body)
			}
			require.Equal(t, tt.attempts, upstream.calls)
			require.NotEmpty(t, upstream.authHeaders[0])
			require.Equal(t, "synthetic-cipher", gjson.GetBytes(upstream.bodies[0], "input.0.encrypted_content").String())
			for _, upstreamCtx := range upstream.contexts {
				require.ErrorIs(t, upstreamCtx.Err(), context.Canceled)
			}
			require.NoError(t, parent.Err())
			if tt.attempts == 2 {
				require.Equal(t, upstream.authHeaders[0], upstream.authHeaders[1])
				require.False(t, gjson.GetBytes(upstream.bodies[1], "input.0.encrypted_content").Exists())
				if tt.item == "reasoning" {
					require.Equal(t, "keep summary", gjson.GetBytes(upstream.bodies[1], "input.0.summary.0.text").String())
					require.Equal(t, "keep message", gjson.GetBytes(upstream.bodies[1], "input.1.content").String())
				} else {
					require.Equal(t, "reasoning", gjson.GetBytes(upstream.bodies[0], "input.0.type").String(), "Grok maps compaction to its reasoning wire format")
					require.Equal(t, "keep message", gjson.GetBytes(upstream.bodies[1], `input.#(type=="message").content`).String())
				}
			}
			if tt.terminal {
				require.Error(t, err)
				require.Nil(t, result)
				require.Equal(t, tt.status, c.GetInt(OpsUpstreamStatusCodeKey))
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, http.StatusOK, recorder.Code)
			}
		})
	}
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
