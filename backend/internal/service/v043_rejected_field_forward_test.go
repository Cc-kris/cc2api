package service_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestV043RejectedFieldNativeAndPassthroughForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, test := range []struct{ name, request, param string }{
			{"max_output_tokens", `"max_output_tokens":100,`, "max_output_tokens"},
			{"tool_namespace", ``, "input[0].namespace"},
		} {
			t.Run(fmt.Sprintf("%v/%s", passthrough, test.name), func(t *testing.T) {
				hits := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hits++
					body, _ := io.ReadAll(r.Body)
					if hits == 1 {
						w.WriteHeader(400)
						fmt.Fprintf(w, `{"error":{"type":"invalid_request_error","code":"unsupported_parameter","param":%q,"message":"Unsupported parameter"}}`, test.param)
						return
					}
					if test.name == "max_output_tokens" {
						require.False(t, gjson.GetBytes(body, "max_output_tokens").Exists())
					} else {
						require.False(t, gjson.GetBytes(body, "input.0.namespace").Exists())
					}
					w.Header().Set("Content-Type", "text/event-stream")
					io.WriteString(w, v043SuccessSSE)
				}))
				defer upstream.Close()
				target, _ := url.Parse(upstream.URL)
				cfg := &config.Config{}
				cfg.Gateway.FailoverOnAnyError = true
				svc := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, &v043LoopbackUpstream{target: target, client: upstream.Client()}, nil, nil, nil, nil, nil, nil, nil, nil)
				account := &service.Account{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "pool_mode": true}, Extra: map[string]any{"openai_passthrough": passthrough, "openai_responses_mode": "force_responses"}}
				input := `"hello"`
				if test.name == "tool_namespace" {
					input = `[{"type":"function_call","name":"tool","call_id":"c1","arguments":"{}","namespace":"unwanted"},{"type":"function_call_output","call_id":"c1","output":"okay"}]`
				}
				body := []byte(`{"model":"gpt-4o","stream":true,"instructions":"fixture",` + test.request + `"input":` + input + `}`)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(body))
				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 2, hits)
			})
		}
	}
}

func TestV043Deterministic400NativeAndPassthroughForward(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprint(passthrough), func(t *testing.T) {
			hits := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				w.WriteHeader(400)
				io.WriteString(w, `{"error":{"type":"invalid_request_error","code":"invalid_value","param":"temperature","message":"Invalid temperature"}}`)
			}))
			defer upstream.Close()
			target, _ := url.Parse(upstream.URL)
			cfg := &config.Config{}
			cfg.Gateway.FailoverOnAnyError = true
			svc := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, &v043LoopbackUpstream{target: target, client: upstream.Client()}, nil, nil, nil, nil, nil, nil, nil, nil)
			account := &service.Account{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test", "pool_mode": true}, Extra: map[string]any{"openai_passthrough": passthrough, "openai_responses_mode": "force_responses"}}
			body := []byte(`{"model":"gpt-4o","stream":true,"instructions":"fixture","input":"hello"}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(body))
			_, err := svc.Forward(context.Background(), c, account, body)
			require.Error(t, err)
			var failover *service.UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
			require.Equal(t, 1, hits)
			require.Equal(t, 400, rec.Code)
			require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
			require.Equal(t, "invalid_value", gjson.Get(rec.Body.String(), "error.code").String())
			require.Equal(t, "temperature", gjson.Get(rec.Body.String(), "error.param").String())
		})
	}
}
