package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository" //nolint:depguard // 清单请求需要真实 HTTP 上游打到 httptest 服务器
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// V02-T18：GPT-5.5 订阅退役在 handler 层的表现。

func oauthOnlyGPT55Account(id int64) service.Account {
	return service.Account{
		ID: id, Name: "oauth", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "acct"},
	}
}

// AC28：OAuth-only 分组请求 gpt-5.5 → 404 model_not_found，带迁移说明，上游没收到请求。
func TestOpenAIResponses_GPT55OnOAuthOnlyGroupReturnsRetirementGuidance(t *testing.T) {
	upstream := &openAIHTTPPassthroughSSEStatusUpstream{}
	h := newOpenAINoAccountTestHandler(t, []service.Account{oauthOnlyGPT55Account(9951)}, upstream)
	c, rec := newOpenAINoAccountTestContext("/openai/v1/responses", `{"model":"gpt-5.5","input":"hi","stream":false}`)

	h.Responses(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "model_not_found", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	msg := gjson.GetBytes(rec.Body.Bytes(), "error.message").String()
	require.Contains(t, msg, "retired")
	require.Contains(t, msg, "API-key")
	require.Empty(t, upstream.calls())
}

// AC29：Messages dispatch 把 claude 模型映射到 gpt-5.5，OAuth-only 分组同样被拦住（404，不打上游）。
func TestOpenAIMessages_DispatchToGPT55OnOAuthOnlyGroupIsBlocked(t *testing.T) {
	upstream := &openAIHTTPPassthroughSSEStatusUpstream{}
	h := newOpenAINoAccountTestHandler(t, []service.Account{oauthOnlyGPT55Account(9952)}, upstream)
	c, rec := newOpenAINoAccountTestContext("/v1/messages", `{"model":"claude-opus-4-6","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)
	apiKey, ok := c.MustGet(string(middleware2.ContextKeyAPIKey)).(*service.APIKey)
	require.True(t, ok, "api key in context should be *service.APIKey")
	apiKey.Group.AllowMessagesDispatch = true
	apiKey.Group.MessagesDispatchModelConfig = service.OpenAIMessagesDispatchModelConfig{OpusMappedModel: "gpt-5.5"}

	h.Messages(c)

	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "retired")
	require.Empty(t, upstream.calls())
}

func gatewayModelsIDs(t *testing.T, body []byte) []string {
	t.Helper()
	var resp gatewayModelsResponseForTest
	require.NoError(t, json.Unmarshal(body, &resp))
	ids := make([]string, 0, len(resp.Data))
	for _, item := range resp.Data {
		ids = append(ids, item.ID)
	}
	return ids
}

// AC31：/v1/models 在 OAuth-only 分组不列 gpt-5.5（有映射和无映射两种），不回落默认列表；
// 有 API Key 账号的分组仍列。
func TestGatewayModels_GPT55RetirementFiltersOpenAIListing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name      string
		accounts  []service.Account
		wantGPT55 bool
		wantEmpty bool
	}{
		{name: "oauth without mapping", accounts: []service.Account{{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}}},
		{name: "oauth with mapping", accounts: []service.Account{{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
			Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.5": "gpt-5.5", "gpt-5.4": "gpt-5.4"}}}}},
		{name: "oauth mapping only retired", accounts: []service.Account{{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
			Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.5": "gpt-5.5"}}}}, wantEmpty: true},
		{name: "api key group keeps gpt-5.5", accounts: []service.Account{
			{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth},
			{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		}, wantGPT55: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			groupID := int64(30)
			h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{groupID: tc.accounts}})
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI}})

			h.Models(c)

			require.Equal(t, http.StatusOK, rec.Code)
			ids := gatewayModelsIDs(t, rec.Body.Bytes())
			require.Equal(t, tc.wantGPT55, contains(ids, "gpt-5.5"), ids)
			if tc.wantEmpty {
				require.Empty(t, ids)
			} else {
				require.Contains(t, ids, "gpt-5.4")
			}
		})
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// AC31：Codex 清单在 OAuth-only 分组过滤 gpt-5.5，ETag 用过滤后内容重算，不透传上游 ETag；
// 带上游 ETag 的 If-None-Match 不会拿到 304；带新 ETag 的拿到 304。
func TestOpenAICodexModels_GPT55RetirementFiltersManifestAndRecomputesETag(t *testing.T) {
	var upstreamIfNoneMatch []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamIfNoneMatch = append(upstreamIfNoneMatch, r.Header.Get("If-None-Match"))
		w.Header().Set("ETag", `"upstream-v1"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"models":[`+codexManifestTestEntry("gpt-5.5")+`,`+codexManifestTestEntry("gpt-5.4")+`]}`)
	}))
	defer server.Close()
	t.Cleanup(service.SetChatGPTCodexModelsURLForTest(server.URL + "/backend-api/codex/models"))

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	gatewaySvc := service.NewOpenAIGatewayService(
		&openAIWSSilentRetryAccountRepoStub{accounts: []service.Account{oauthOnlyGPT55Account(9953)}},
		nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, repository.NewHTTPUpstream(cfg),
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	h := NewOpenAIGatewayHandler(gatewaySvc, service.NewConcurrencyService(nil), nil, nil, nil, nil, nil, cfg)

	request := func(ifNoneMatch string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/backend-api/codex/models?client_version=0.146.0", nil)
		if ifNoneMatch != "" {
			c.Request.Header.Set("If-None-Match", ifNoneMatch)
		}
		groupID := int64(4301)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID: 1, GroupID: &groupID,
			Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
		})
		h.CodexModels(c)
		// 直接调 handler 不经过 gin engine，c.Status(304) 需要手动落到 recorder。
		c.Writer.WriteHeaderNow()
		return rec
	}

	first := request("")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.False(t, strings.Contains(first.Body.String(), "gpt-5.5"))
	require.Contains(t, first.Body.String(), "gpt-5.4")
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)
	require.NotEqual(t, `"upstream-v1"`, etag)

	stale := request(`"upstream-v1"`)
	require.Equal(t, http.StatusOK, stale.Code, "上游旧 ETag 不能让客户端继续用含 gpt-5.5 的缓存")
	require.False(t, strings.Contains(stale.Body.String(), "gpt-5.5"))

	fresh := request(etag)
	require.Equal(t, http.StatusNotModified, fresh.Code)

	for _, v := range upstreamIfNoneMatch {
		require.Empty(t, v, "OpenAI 分组不把客户端 If-None-Match 转给上游")
	}
}

// codexManifestTestEntry 是能通过 FetchCodexModelsManifest 校验的最小清单条目
// （字段同 service 包的 codexModelsNativeTestManifest）。
func codexManifestTestEntry(slug string) string {
	return `{"slug":"` + slug + `","display_name":"` + slug + `","description":null,` +
		`"default_reasoning_level":"low","supported_reasoning_levels":[{"effort":"low","description":"Fast"}],` +
		`"shell_type":"shell_command","visibility":"list","supported_in_api":true,"priority":1,` +
		`"availability_nux":null,"upgrade":null,"base_instructions":"You are Codex.","support_verbosity":false,` +
		`"default_verbosity":null,"apply_patch_tool_type":null,"truncation_policy":{"mode":"bytes","limit":10000},` +
		`"supports_parallel_tool_calls":true,"experimental_supported_tools":[],"input_modalities":["text","image"]}`
}
