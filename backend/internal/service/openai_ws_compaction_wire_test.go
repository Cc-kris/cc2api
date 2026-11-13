package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Only redirect the upstream address: the real coder transport and HTTP client
// still exchange frames/requests over loopback through the production proxy.
type compactionWireDialer struct {
	target string
	calls  atomic.Int32
}

func (d *compactionWireDialer) Dial(ctx context.Context, _ string, headers http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	d.calls.Add(1)
	return newDefaultOpenAIWSClientDialer().Dial(ctx, d.target, headers, "")
}

type compactionWireHTTP struct {
	httpUpstreamRecorder
	target string
}

func (u *compactionWireHTTP) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	clone := req.Clone(req.Context())
	target, err := url.Parse(u.target)
	if err != nil {
		return nil, err
	}
	target.Path = req.URL.Path
	clone.URL = target
	clone.Host = target.Host
	return http.DefaultClient.Do(clone)
}

func TestOpenAIWSIngressWire_ExplicitHTTPBridgeAndOAuthFactorOne(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeHTTPBridge, OpenAIWSIngressModeCtxPool} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
			cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
			cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = false // Explicit mode is not the large-payload heuristic.
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
			cfg.Gateway.OpenAIWS.DynamicMaxConnsByAccountConcurrencyEnabled = true
			cfg.Gateway.OpenAIWS.OAuthMaxConnsFactor = 1.0
			cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
			arrived := make(chan string, 4)
			release := make(chan struct{})
			upstreamErrors := make(chan error, 4)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload []byte
				var conn *coderws.Conn
				if coderwsIsUpgrade := strings.EqualFold(r.Header.Get("Upgrade"), "websocket"); coderwsIsUpgrade {
					var err error
					conn, err = coderws.Accept(w, r, nil)
					if err != nil {
						upstreamErrors <- err
						return
					}
					defer conn.CloseNow()
					_, payload, err = conn.Read(r.Context())
					if err != nil {
						upstreamErrors <- err
						return
					}
					arrived <- "ws"
				} else {
					payload, _ = io.ReadAll(r.Body)
					arrived <- "http"
				}
				if !gjson.ValidBytes(payload) {
					upstreamErrors <- io.ErrUnexpectedEOF
					return
				}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				event := []byte(`{"type":"response.completed","response":{"id":"resp_wire","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)
				if conn != nil {
					for {
						if err := conn.Write(r.Context(), coderws.MessageText, event); err != nil {
							upstreamErrors <- err
							return
						}
						// A preempted clean lease may be reused for the replacement.
						if _, _, err := conn.Read(r.Context()); err != nil {
							return
						}
					}
				} else {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: "+string(event)+"\n\n")
				}
			}))
			defer upstream.Close()
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			dialer := &compactionWireDialer{target: "ws" + strings.TrimPrefix(upstream.URL, "http")}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: &compactionWireHTTP{target: upstream.URL}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool}
			account := &Account{ID: 851, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 2, Credentials: map[string]any{"access_token": "test"}, Extra: map[string]any{"openai_oauth_responses_websockets_v2_mode": mode, "responses_websockets_v2_enabled": true}}
			serverErrors := make(chan error, 4)
			ingress := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					serverErrors <- err
					return
				}
				defer conn.CloseNow()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r.Clone(r.Context())
				groupID := int64(852)
				c.Set("api_key", &APIKey{ID: 853, GroupID: &groupID})
				_, first, err := conn.Read(r.Context())
				if err != nil {
					serverErrors <- err
					return
				}
				serverErrors <- svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, account, "test", first, nil)
			}))
			defer ingress.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			connect := func(thread string) *coderws.Conn {
				headers := http.Header{"Session-Id": []string{"shared-session"}, "Thread-Id": []string{thread}}
				conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(ingress.URL, "http"), &coderws.DialOptions{HTTPHeader: headers})
				require.NoError(t, err)
				t.Cleanup(func() { conn.CloseNow() })
				require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","input":[{"role":"user","content":"hi"}]}`)))
				return conn
			}
			first := connect("thread-a")
			defer first.CloseNow()
			// In bridge mode even the same execution identity must not preempt an
			// independent HTTP request. Ctx_pool sibling threads must not preempt either.
			secondThread := "thread-b"
			if mode == OpenAIWSIngressModeHTTPBridge {
				secondThread = "thread-a"
			}
			second := connect(secondThread)
			defer second.CloseNow()
			expectedTransport := "ws"
			if mode == OpenAIWSIngressModeHTTPBridge {
				expectedTransport = "http"
			}
			for i := 0; i < 2; i++ {
				select {
				case got := <-arrived:
					require.Equal(t, expectedTransport, got)
				case err := <-serverErrors:
					t.Fatalf("proxy ended early: %v", err)
				case <-ctx.Done():
					t.Fatal("two concurrent OAuth slots did not reach upstream at factor 1.0")
				}
			}
			if mode == OpenAIWSIngressModeHTTPBridge {
				require.Zero(t, dialer.calls.Load())
			} else {
				require.Equal(t, int32(2), dialer.calls.Load())
			}
			close(release)
			for _, conn := range []*coderws.Conn{first, second} {
				_, event, err := conn.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
			}
			if mode == OpenAIWSIngressModeCtxPool {
				replacement := connect("thread-a")
				_, _, err := first.Read(ctx)
				require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err), "same-thread replacement must preempt the original real ingress connection")
				_, event, err := replacement.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
				replacement.CloseNow()
			}
			first.CloseNow()
			second.CloseNow()
			select {
			case err := <-upstreamErrors:
				t.Fatalf("upstream wire failure: %v", err)
			default:
			}
		})
	}
}
