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
	"github.com/tidwall/sjson"
)

// TestForward_OAuthWebSearchHistoryDeclaresTool drives the Codex compaction
// shape (web_search_call history, tools:[]) through Forward and asserts the
// body reaching chatgpt.com declares web_search (#7927): top-level for the
// standard wire, input additional_tools for Responses Lite.
func TestForward_OAuthWebSearchHistoryDeclaresTool(t *testing.T) {
	for _, tc := range []struct {
		name        string
		passthrough bool
		lite        bool
	}{
		{"transform", false, false},
		{"passthrough", true, false},
		{"transform lite", false, true},
		{"passthrough lite", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
			if tc.lite {
				c.Request.Header.Set(responsesLiteHeader, "true")
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_web_search\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n" +
						"data: [DONE]\n\n",
				)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{
				ID: 504, Name: "web-search-history", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Concurrency: 1, Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1),
				Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
				Extra:       map[string]any{"openai_passthrough": tc.passthrough},
			}
			body, err := sjson.SetRawBytes([]byte(openAIWebSearchHistoryCompactionBody), "input.-1", []byte(`{"type":"compaction_trigger"}`))
			require.NoError(t, err)

			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "https://chatgpt.com/backend-api/codex/responses", upstream.lastReq.URL.String())

			forwarded := upstream.lastBody
			require.Equal(t, "web_search_call", gjson.GetBytes(forwarded, "input.1.type").String())
			require.Equal(t, "none", gjson.GetBytes(forwarded, "tool_choice").String())
			items := gjson.GetBytes(forwarded, "input").Array()
			require.Equal(t, "compaction_trigger", items[len(items)-1].Get("type").String())
			if !tc.lite {
				tools := gjson.GetBytes(forwarded, "tools").Array()
				require.Len(t, tools, 1)
				require.Equal(t, "web_search", tools[0].Get("type").String())
				return
			}
			require.False(t, gjsonToolsContainWebSearch(gjson.GetBytes(forwarded, "tools")))
			additional := items[len(items)-2]
			require.Equal(t, "additional_tools", additional.Get("type").String())
			require.Equal(t, "web_search", additional.Get("tools.0.type").String())
		})
	}
}
