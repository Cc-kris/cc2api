package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// These tests enter the real Responses handler and Forward routing. The outbound
// transport alone is stubbed: no TLS/SSRF policy is disabled and no provider is hit.
type ac13EntryUpstream struct {
	service.HTTPUpstream
	bodies []string
	status int
	calls  int
}

func (u *ac13EntryUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body := u.bodies[min(u.calls, len(u.bodies)-1)]
	u.calls++
	contentType := "text/event-stream"
	if u.status >= 400 {
		contentType = "application/json"
	}
	return &http.Response{StatusCode: u.status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func runAC13Entry(t *testing.T, path string, passthrough bool, upstream *ac13EntryUpstream) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.RunMode = config.RunModeSimple
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = true
	cfg.Security.URLAllowlist.UpstreamHosts = []string{"api.example.test"}
	cfg.Gateway.FailoverOnAnyError = true
	cfg.Gateway.MaxAccountSwitches = 2
	group := &service.Group{ID: 4913, Platform: service.PlatformOpenAI, Status: service.StatusActive, FailoverOnAnyError: true}
	accounts := []service.Account{passthroughAPIKeyAccount(99131, 0, map[string]any{"api_protocol": "responses"}), passthroughAPIKeyAccount(99132, 1, map[string]any{"api_protocol": "responses"})}
	for i := range accounts {
		accounts[i].Extra["openai_passthrough"] = passthrough
		accounts[i].Extra["openai_compact_supported"] = true
	}
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	gateway := service.NewOpenAIGatewayService(
		&openAIWSSilentRetryAccountRepoStub{accounts: accounts},
		nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCache, upstream,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billingCache,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, cfg)
	router := gin.New()
	router.POST(path, func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1913, GroupID: &group.ID, User: &service.User{ID: 1913, Status: service.StatusActive}, Group: group})
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1913})
		h.Responses(c)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5.5","stream":true,"input":"AC13 actual entry"}`)))
	return rec
}

func ac13Frame(data string) string { return "data: " + data + "\n\n" }
func ac13Created(seq int, id string) string {
	return ac13Frame(fmt.Sprintf(`{"type":"response.created","sequence_number":%d,"response":{"id":%q,"object":"response","created_at":1794848626,"status":"in_progress","output":[]}}`, seq, id))
}
func ac13Terminal(typ string, seq int, id string) string {
	status := strings.TrimPrefix(typ, "response.")
	return ac13Frame(fmt.Sprintf(`{"type":%q,"sequence_number":%d,"response":{"id":%q,"object":"response","created_at":1794848626,"status":%q,"output":[],"error":{"code":"content_policy","message":"authoritative terminal detail"}}}`, typ, seq, id, status))
}
func ac13Payloads(body string) []string {
	var result []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") {
			result = append(result, strings.TrimPrefix(line, "data: "))
		}
	}
	return result
}
func requireAC13Monotonic(t *testing.T, payloads []string, origin int64) {
	t.Helper()
	previous := origin - 1
	for _, payload := range payloads {
		if payload == "[DONE]" {
			continue
		}
		seq := gjson.Get(payload, "sequence_number")
		require.Equal(t, gjson.Number, seq.Type, payload)
		require.Greater(t, seq.Int(), previous, payload)
		previous = seq.Int()
	}
	require.Equal(t, origin, gjson.Get(payloads[0], "sequence_number").Int())
}

func TestAC13EntryNormalizedSyntheticSequence(t *testing.T) {
	for _, origin := range []int{0, 40} {
		for _, itemOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("origin_%d_item_only_%t", origin, itemOnly), func(t *testing.T) {
				body := ac13Created(origin, "resp_original")
				if !itemOnly {
					body += ac13Frame(fmt.Sprintf(`{"type":"response.output_item.added","sequence_number":%d,"output_index":0,"item":{"id":"msg_a","type":"message","role":"assistant","content":[]}}`, origin+1))
					body += ac13Frame(fmt.Sprintf(`{"type":"response.output_text.delta","sequence_number":%d,"item_id":"msg_a","output_index":0,"content_index":0,"delta":"ALPHA"}`, origin+2))
					body += ac13Frame(fmt.Sprintf(`{"type":"response.output_text.done","sequence_number":%d,"item_id":"msg_a","output_index":0,"content_index":0,"text":"ALPHA"}`, origin+3))
				}
				body += ac13Frame(fmt.Sprintf(`{"type":"response.output_item.done","sequence_number":%d,"output_index":0,"item":{"id":"msg_a","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ALPHA"}]}}`, origin+4))
				body += ac13Terminal("response.completed", origin+5, "resp_original") + ac13Frame("[DONE]")
				upstream := &ac13EntryUpstream{status: http.StatusOK, bodies: []string{body}}
				rec := runAC13Entry(t, "/v1/responses", false, upstream)
				require.Equal(t, 1, upstream.calls)
				require.Equal(t, http.StatusOK, rec.Code)
				requireAC13Monotonic(t, ac13Payloads(rec.Body.String()), int64(origin))
				require.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"response.content_part.added"`))
				require.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"response.content_part.done"`))
			})
		}
	}
}

func TestAC13EntryCompactEarly400PureDetailedSSE(t *testing.T) {
	for _, passthrough := range []bool{true, false} {
		t.Run(fmt.Sprint(passthrough), func(t *testing.T) {
			upstream := &ac13EntryUpstream{status: http.StatusBadRequest, bodies: []string{`{"error":{"code":"content_policy","type":"invalid_request_error","message":"exact compact rejection detail"}}`}}
			rec := runAC13Entry(t, "/v1/responses/compact", passthrough, upstream)
			require.Equal(t, 1, upstream.calls)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
			require.True(t, strings.HasPrefix(rec.Body.String(), "event: response.failed\ndata: "), rec.Body.String())
			payloads := ac13Payloads(rec.Body.String())
			require.Len(t, payloads, 1)
			require.Equal(t, "response.failed", gjson.Get(payloads[0], "type").String())
			require.Equal(t, "content_policy", gjson.Get(payloads[0], "response.error.code").String())
			require.Equal(t, "exact compact rejection detail", gjson.Get(payloads[0], "response.error.message").String())
			requireAC13Monotonic(t, payloads, 0)
		})
	}
}

func TestAC13EntryBareErrorBeforeDoneAndAuthoritativeTerminalControls(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, terminal := range []string{"", "response.completed", "response.failed", "response.incomplete"} {
			for _, done := range []bool{false, true} {
				t.Run(fmt.Sprintf("passthrough_%t_terminal_%s_done_%t", passthrough, terminal, done), func(t *testing.T) {
					body := ac13Created(40, "resp_original") + ac13Frame(`{"type":"error","sequence_number":41,"error":{"code":"content_policy","type":"invalid_request_error","message":"exact bare rejection detail"}}`)
					if terminal != "" {
						body += ac13Terminal(terminal, 42, "resp_original")
					}
					if done {
						body += ac13Frame("[DONE]")
					}
					upstream := &ac13EntryUpstream{status: http.StatusOK, bodies: []string{body}}
					rec := runAC13Entry(t, "/v1/responses", passthrough, upstream)
					require.Equal(t, 1, upstream.calls)
					payloads := ac13Payloads(rec.Body.String())
					wantLen := 2
					if done {
						wantLen++
					}
					require.Len(t, payloads, wantLen, rec.Body.String())
					requireAC13Monotonic(t, payloads, 40)
					wantTerminal := terminal
					if wantTerminal == "" {
						wantTerminal = "response.failed"
					}
					require.Equal(t, wantTerminal, gjson.Get(payloads[1], "type").String())
					require.Equal(t, "resp_original", gjson.Get(payloads[1], "response.id").String())
					if terminal == "" {
						require.Equal(t, "exact bare rejection detail", gjson.Get(payloads[1], "response.error.message").String())
					} else {
						require.Equal(t, "authoritative terminal detail", gjson.Get(payloads[1], "response.error.message").String())
					}
					if done {
						require.Equal(t, "[DONE]", payloads[len(payloads)-1])
					}
				})
			}
		}
	}
}

func TestAC13EntryBareErrorWithoutSequenceAdvancesTerminal(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, origin := range []int{0, 40} {
			for _, done := range []bool{false, true} {
				for _, errorSequence := range []string{"", fmt.Sprintf(`,"sequence_number":%d`, origin)} {
					t.Run(fmt.Sprintf("passthrough_%t_origin_%d_done_%t_error_seq_%s", passthrough, origin, done, errorSequence), func(t *testing.T) {
						body := ac13Created(origin, "resp_missing_error_seq") +
							ac13Frame(`{"type":"error"`+errorSequence+`,"error":{"code":"content_policy","type":"invalid_request_error","message":"bare error without sequence"}}`)
						if done {
							body += ac13Frame("[DONE]")
						}
						upstream := &ac13EntryUpstream{status: http.StatusOK, bodies: []string{body}}
						rec := runAC13Entry(t, "/v1/responses", passthrough, upstream)
						require.Equal(t, 1, upstream.calls)
						payloads := ac13Payloads(rec.Body.String())
						wantLen := 2
						if done {
							wantLen++
						}
						require.Len(t, payloads, wantLen, rec.Body.String())
						requireAC13Monotonic(t, payloads, int64(origin))
						failed := payloads[1]
						require.Equal(t, "response.failed", gjson.Get(failed, "type").String())
						require.Equal(t, int64(origin+1), gjson.Get(failed, "sequence_number").Int())
						require.Equal(t, "resp_missing_error_seq", gjson.Get(failed, "response.id").String())
						require.Equal(t, "content_policy", gjson.Get(failed, "response.error.code").String())
						require.Equal(t, "bare error without sequence", gjson.Get(failed, "response.error.message").String())
						if done {
							require.Equal(t, "[DONE]", payloads[len(payloads)-1])
						}
					})
				}
			}
		}
	}
}

func TestAC13EntryContinuationSequenceAndIdentity(t *testing.T) {
	first := ac13Created(40, "resp_original") +
		ac13Frame(`{"type":"response.output_item.added","sequence_number":41,"output_index":0,"item":{"id":"msg_a","type":"message","role":"assistant","content":[]}}`) +
		ac13Frame(`{"type":"response.output_text.delta","sequence_number":42,"item_id":"msg_a","output_index":0,"content_index":0,"delta":"ALPHA"}`)
	second := ac13Created(0, "resp_retry") +
		ac13Frame(`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"msg_b","type":"message","role":"assistant","content":[]}}`) +
		ac13Frame(`{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_b","output_index":0,"content_index":0,"delta":"BETA"}`) +
		ac13Frame(`{"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":{"id":"msg_b","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"BETA"}]}}`) +
		ac13Terminal("response.completed", 4, "resp_retry") + ac13Frame("[DONE]")
	upstream := &ac13EntryUpstream{status: http.StatusOK, bodies: []string{first, second}}
	rec := runAC13Entry(t, "/v1/responses", false, upstream)
	require.Equal(t, 2, upstream.calls, rec.Body.String())
	payloads := ac13Payloads(rec.Body.String())
	requireAC13Monotonic(t, payloads, 40)
	created, terminals := 0, 0
	for _, payload := range payloads {
		switch gjson.Get(payload, "type").String() {
		case "response.created":
			created++
		case "response.completed", "response.failed", "response.incomplete":
			terminals++
			require.Equal(t, "resp_original", gjson.Get(payload, "response.id").String())
			require.Equal(t, "response.completed", gjson.Get(payload, "type").String())
		}
	}
	require.Equal(t, 1, created)
	require.Equal(t, 1, terminals)
}
