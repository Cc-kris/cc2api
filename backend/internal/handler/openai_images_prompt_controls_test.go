package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository" //nolint:depguard // integration test exercises the real upstream adapter
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayHandlerImages_CodexPromptControlsApplyOnlyToBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamImage := solidPNGBase64(t, 1254, 1254)
	upstreamPayloads := make(chan []byte, 4)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		upstreamPayloads <- payload
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+upstreamImage+`"}]}`)
	}))
	defer upstreamServer.Close()

	groupID := int64(4270)
	accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: service.Account{
		ID: 124, Name: "codex-prompt-controls-image-account", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": upstreamServer.URL},
		Extra:       map[string]any{"openai_passthrough": true},
	}}
	usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 4)}
	cfg := &config.Config{}
	cfg.RunMode = config.RunModeSimple
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	channelSvc := service.NewChannelService(&openAIWSUsageHandlerChannelRepoStub{
		channels: []service.Channel{{
			ID: 7720, Name: "codex-prompt-controls-channel", Status: service.StatusActive,
			GroupIDs: []int64{groupID},
			ModelMapping: map[string]map[string]string{service.PlatformOpenAI: {
				"gpt-5.6-luna": "gpt-image-2",
			}},
			FeaturesConfig: map[string]any{"codex_image_generation_bridge": map[string]any{
				service.PlatformOpenAI: true, "orchestrator_group_id": int64(20),
			}},
		}},
		groupPlatforms: map[int64]string{groupID: service.PlatformOpenAI},
	}, nil, nil, nil)
	billingCacheSvc := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	gatewaySvc := service.NewOpenAIGatewayService(
		accountRepo, usageRepo, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCacheSvc, repository.NewHTTPUpstream(cfg),
		&service.DeferredService{}, nil, nil, channelSvc, nil, nil, nil, nil,
	)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := &OpenAIGatewayHandler{
		gatewayService: gatewaySvc, billingCacheService: billingCacheSvc, apiKeyService: &service.APIKeyService{},
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}
	apiKey := &service.APIKey{
		ID: 1820, GroupID: &groupID,
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, AllowImageGeneration: true},
		User:  &service.User{ID: 1720, Status: service.StatusActive},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.User.ID, Concurrency: 1})
		c.Next()
	})
	router.POST("/v1/images/edits", h.Images)

	body := []byte(`{"model":"gpt-image-2","prompt":"保持构图，输出 2000*2000 的高质量透明背景 PNG","size":"auto","quality":"auto","background":"auto","images":[{"image_url":"data:image/png;base64,aW5wdXQ="}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Codex Desktop/0.144.5 Windows")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)

	forwarded := <-upstreamPayloads
	require.Equal(t, "2000x2000", gjson.GetBytes(forwarded, "size").String())
	require.Equal(t, "high", gjson.GetBytes(forwarded, "quality").String())
	require.Equal(t, "transparent", gjson.GetBytes(forwarded, "background").String())
	require.Equal(t, "png", gjson.GetBytes(forwarded, "output_format").String())

	outputBytes, err := base64.StdEncoding.DecodeString(gjson.GetBytes(recorder.Body.Bytes(), "data.0.b64_json").String())
	require.NoError(t, err)
	outputConfig, outputFormat, err := image.DecodeConfig(bytes.NewReader(outputBytes))
	require.NoError(t, err)
	require.Equal(t, "png", outputFormat)
	require.Equal(t, 2000, outputConfig.Width)
	require.Equal(t, 2000, outputConfig.Height)
	require.Equal(t, "2000x2000", gjson.GetBytes(recorder.Body.Bytes(), "data.0.size").String())

	usage := <-usageRepo.created
	require.Equal(t, "2000x2000", valueOrEmpty(usage.ImageInputSize))
	require.Equal(t, "2000x2000", valueOrEmpty(usage.ImageOutputSize))

	unsupportedBody := []byte(`{"model":"gpt-image-2","prompt":"输出 2000x2000 JPG","size":"auto","images":[{"image_url":"data:image/png;base64,aW5wdXQ="}]}`)
	unsupportedReq := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(unsupportedBody))
	unsupportedReq.Header.Set("Content-Type", "application/json")
	unsupportedReq.Header.Set("User-Agent", "Codex Desktop/0.144.5 Windows")
	unsupportedRecorder := httptest.NewRecorder()
	router.ServeHTTP(unsupportedRecorder, unsupportedReq)
	require.Equal(t, http.StatusBadRequest, unsupportedRecorder.Code)
	require.Contains(t, unsupportedRecorder.Body.String(), "only saves generated artifacts as PNG")
	select {
	case unexpected := <-upstreamPayloads:
		require.Fail(t, "unsupported Codex output constraints must fail before upstream", "payload=%s", unexpected)
	default:
	}

	ordinaryBody := []byte(`{"model":"gpt-image-2","prompt":"输出 2000x2000 JPG","size":"auto","images":[{"image_url":"data:image/png;base64,aW5wdXQ="}]}`)
	ordinaryReq := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(ordinaryBody))
	ordinaryReq.Header.Set("Content-Type", "application/json")
	ordinaryReq.Header.Set("User-Agent", "curl/8.0")
	ordinaryRecorder := httptest.NewRecorder()
	router.ServeHTTP(ordinaryRecorder, ordinaryReq)
	require.Equal(t, http.StatusOK, ordinaryRecorder.Code)
	ordinaryForwarded := <-upstreamPayloads
	require.Equal(t, "auto", gjson.GetBytes(ordinaryForwarded, "size").String())
	<-usageRepo.created
}

func solidPNGBase64(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.NRGBA{R: 40, G: 100, B: 180, A: 255}}, image.Point{}, draw.Src)
	var output bytes.Buffer
	require.NoError(t, png.Encode(&output, img))
	return base64.StdEncoding.EncodeToString(output.Bytes())
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
