package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type userVideoAPIKeyGetter interface {
	GetByID(ctx context.Context, id int64) (*service.APIKey, error)
}

type userVideoSeedacePoller interface {
	Poll(ctx context.Context, input service.SeedaceVideoPollInput) (*service.SeedaceVideoResult, error)
}

type userVideoHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// UserVideoGenerationHandler handles authenticated user video-generation helpers.
type UserVideoGenerationHandler struct {
	apiKeyService userVideoAPIKeyGetter
	seedaceVideo  userVideoSeedacePoller
	httpClient    userVideoHTTPDoer
}

func NewUserVideoGenerationHandler(apiKeyService *service.APIKeyService, seedaceVideoService *service.SeedaceVideoService) *UserVideoGenerationHandler {
	return &UserVideoGenerationHandler{
		apiKeyService: apiKeyService,
		seedaceVideo:  seedaceVideoService,
		httpClient:    &http.Client{Timeout: 10 * time.Minute},
	}
}

// Download streams the generated video through the origin so the UI never exposes the upstream URL.
func (h *UserVideoGenerationHandler) Download(c *gin.Context) {
	if h == nil || h.apiKeyService == nil || h.seedaceVideo == nil || h.httpClient == nil {
		response.Error(c, http.StatusServiceUnavailable, "Video download service is unavailable")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		response.BadRequest(c, "Invalid task ID")
		return
	}
	apiKeyIDValue := strings.TrimSpace(c.Query("api_key_id"))
	if apiKeyIDValue == "" {
		apiKeyIDValue = strings.TrimSpace(c.PostForm("api_key_id"))
	}
	apiKeyID, err := strconv.ParseInt(apiKeyIDValue, 10, 64)
	if err != nil || apiKeyID <= 0 {
		response.BadRequest(c, "Invalid API key ID")
		return
	}

	apiKey, err := h.apiKeyService.GetByID(c.Request.Context(), apiKeyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if apiKey == nil || apiKey.UserID != subject.UserID {
		response.Forbidden(c, "Not authorized to use this API key")
		return
	}
	if !apiKey.IsActive() || apiKey.IsExpired() || apiKey.IsQuotaExhausted() {
		response.Forbidden(c, "API key is not available")
		return
	}
	if apiKey.Group == nil || apiKey.Group.Platform != service.PlatformSeedace {
		response.Forbidden(c, "API key is not a seedace key")
		return
	}

	pollResult, err := h.seedaceVideo.Poll(c.Request.Context(), service.SeedaceVideoPollInput{
		APIKey:    apiKey,
		TaskID:    taskID,
		Headers:   c.Request.Header,
		UserAgent: c.Request.UserAgent(),
		IPAddress: c.ClientIP(),
	})
	if err != nil {
		response.Error(c, http.StatusBadGateway, err.Error())
		return
	}
	videoURL := service.ExtractSeedaceVideoURL(pollResult.Body)
	if videoURL == "" {
		response.NotFound(c, "Video URL not found")
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, videoURL, nil)
	if err != nil {
		response.BadRequest(c, "Invalid video URL")
		return
	}
	if c.Request.UserAgent() != "" {
		req.Header.Set("User-Agent", c.Request.UserAgent())
	}

	upstreamResp, err := h.httpClient.Do(req)
	if err != nil {
		response.Error(c, http.StatusBadGateway, fmt.Sprintf("download video: %v", err))
		return
	}
	defer upstreamResp.Body.Close()
	if upstreamResp.StatusCode < 200 || upstreamResp.StatusCode >= 300 {
		response.Error(c, http.StatusBadGateway, "Video download URL is unavailable")
		return
	}

	contentType := strings.TrimSpace(upstreamResp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "video/mp4"
	}
	if contentLength := strings.TrimSpace(upstreamResp.Header.Get("Content-Length")); contentLength != "" {
		c.Header("Content-Length", contentLength)
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "seedance-video-"+safeVideoFilenamePart(taskID)+".mp4"))
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, upstreamResp.Body)
}

func safeVideoFilenamePart(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "task"
	}
	if len(out) > 80 {
		return out[:80]
	}
	return out
}
