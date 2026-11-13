package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newCompactionWireRouter(t *testing.T, upstreamURL string, enabled bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.RunMode = config.RunModeSimple
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	enabledValue := "true"
	if !enabled {
		enabledValue = "false"
	}
	settings := service.NewSettingService(&contentModerationHandlerSettingRepo{values: map[string]string{service.SettingKeyOpenAIRemoteCompactionV2Enabled: enabledValue}}, cfg)
	accounts := &openAIWSUsageHandlerAccountRepoStub{account: service.Account{
		ID: 701, Name: "compaction-wire", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 2,
		Credentials: map[string]any{"api_key": "test-key", "base_url": upstreamURL, "compact_model_mapping": map[string]any{"gpt-5.4": "legacy-compact"}},
		// A known legacy-compact-negative account must still serve native v2.
		Extra: map[string]any{"openai_passthrough": true, "openai_compact_supported": false},
	}}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	gateway := service.NewOpenAIGatewayService(accounts, nil, nil, nil, nil, nil, nil, cfg, nil, nil, service.NewBillingService(cfg, nil), nil, billing, repository.NewHTTPUpstream(cfg), &service.DeferredService{}, nil, nil, nil, nil, settings, nil, nil)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := &OpenAIGatewayHandler{gatewayService: gateway, billingCacheService: billing, apiKeyService: &service.APIKeyService{}, cfg: cfg, concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)}
	groupID := int64(702)
	key := &service.APIKey{ID: 703, GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive}, User: &service.User{ID: 704, Status: service.StatusActive}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), key)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.User.ID, Concurrency: 2})
		c.Next()
	})
	router.POST("/v1/responses", h.Responses)
	router.POST("/v1/responses/compact", h.Responses)
	router.GET("/v1/responses", h.ResponsesWebSocket)
	return router
}

func TestRemoteCompactionV2Disabled_HTTPAndWSWire(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	router := newCompactionWireRouter(t, upstream.URL, false)
	payload := `{"model":"gpt-5.4","stream":true,"input":[{"type":"compaction_trigger"}]}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload)))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Equal(t, "remote_compaction_v2_disabled", gjson.Get(recorder.Body.String(), "error.type").String())
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
	require.NoError(t, err)
	defer conn.CloseNow()
	// WS response.create is implicitly streaming; omission must not bypass switch.
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","input":[{"type":"compaction_trigger"}]}`)))
	_, message, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "remote_compaction_v2_disabled", gjson.GetBytes(message, "error.code").String())
	_, _, err = conn.Read(ctx)
	require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err))
	require.Zero(t, calls.Load())
}

func TestNativeCompactionV2_ResponsesWireAndBetaWithoutLegacyCapability(t *testing.T) {
	type outbound struct {
		path, beta, model string
		stream            bool
		trigger           string
	}
	observed := make(chan outbound, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		observed <- outbound{r.URL.Path, r.Header.Get("x-codex-beta-features"), gjson.GetBytes(body, "model").String(), gjson.GetBytes(body, "stream").Bool(), gjson.GetBytes(body, "input.0.type").String()}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("x-codex-turn-state", "state-native")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction\",\"encrypted_content\":\"opaque\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_native\",\"output\":[{\"type\":\"compaction\",\"encrypted_content\":\"opaque\"}],\"usage\":{\"input_tokens\":2,\"output_tokens\":1,\"total_tokens\":3}}}\n\n")
	}))
	defer upstream.Close()
	router := newCompactionWireRouter(t, upstream.URL, true)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4","stream":true,"input":[{"type":"compaction_trigger"}]}`))
	request.Header.Set("session-id", "native-session")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "response.completed")
	require.Equal(t, "state-native", recorder.Header().Get("x-codex-turn-state"))
	select {
	case got := <-observed:
		require.Equal(t, "/v1/responses", got.path)
		require.Equal(t, "remote_compaction_v2", got.beta)
		require.Equal(t, "gpt-5.4", got.model)
		require.True(t, got.stream)
		require.Equal(t, "compaction_trigger", got.trigger)
	default:
		t.Fatal("native compaction did not reach upstream")
	}
}
