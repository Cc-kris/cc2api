package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// CodexModels serves the live capability manifest used by Codex Desktop.
func (h *OpenAIGatewayHandler) CodexModels(c *gin.Context) {
	apiKey, ok := servermiddleware.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "invalid_request_error", "API key group is required")
		return
	}
	if apiKey.Group.Platform != service.PlatformOpenAI && apiKey.Group.Platform != service.PlatformGrok {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Codex models manifest is only available for OpenAI-compatible groups")
		return
	}

	manifestGroupID := apiKey.GroupID
	if apiKey.Group.Platform == service.PlatformOpenAI {
		var err error
		manifestGroupID, err = h.gatewayService.ResolveCodexModelsManifestGroupID(c.Request.Context(), apiKey.GroupID)
		if err != nil {
			h.errorResponse(c, http.StatusServiceUnavailable, "configuration_error", "Codex models manifest configuration is unavailable")
			return
		}
	}

	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	failedAccountIDs := make(map[int64]struct{})
	for switchCount := 0; ; switchCount++ {
		var account *service.Account
		var release func()
		var selectErr error
		if apiKey.Group.Platform == service.PlatformGrok {
			selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
				c.Request.Context(), manifestGroupID, "", "", "", failedAccountIDs,
				service.OpenAIUpstreamTransportHTTPSSE, service.OpenAIEndpointCapabilityResponses,
				false, false, false, service.PlatformGrok,
			)
			selectErr = err
			if selection != nil {
				account = selection.Account
				release = selection.ReleaseFunc
			}
		} else {
			account, selectErr = h.gatewayService.SelectAccountForModelWithExclusions(c.Request.Context(), manifestGroupID, "", "", failedAccountIDs)
		}
		if selectErr != nil {
			h.errorResponse(c, http.StatusServiceUnavailable, "upstream_error", "No available compatible accounts")
			return
		}

		// OpenAI 分组的清单要按分组账号过滤退役模型并重算 ETag（V02-T18），所以不把客户端的
		// If-None-Match 转给上游：上游 304 时网关拿不到 body，无法判断过滤后的内容是否变化。
		// 条件请求改在网关这一侧按最终 ETag 判断。Grok 分组保持原样透传。
		upstreamIfNoneMatch := c.GetHeader("If-None-Match")
		if apiKey.Group.Platform == service.PlatformOpenAI {
			upstreamIfNoneMatch = ""
		}
		manifest, fetchErr := h.gatewayService.FetchCodexModelsManifest(c.Request.Context(), account, c.Query("client_version"), upstreamIfNoneMatch)
		if release != nil {
			release()
		}
		if fetchErr != nil {
			if service.IsRetryableCodexModelsManifestError(fetchErr) && switchCount < maxAccountSwitches {
				failedAccountIDs[account.ID] = struct{}{}
				continue
			}
			h.errorResponse(c, infraerrors.Code(fetchErr), "upstream_error", infraerrors.Message(fetchErr))
			return
		}

		if apiKey.Group.Platform == service.PlatformOpenAI && !manifest.NotModified {
			body, changed := h.gatewayService.FilterRetiredCodexModelsManifest(c.Request.Context(), manifestGroupID, manifest.Body)
			etag := manifest.ETag
			if changed || etag == "" {
				etag = service.CodexModelsManifestETag(body)
			}
			c.Header("ETag", etag)
			if codexManifestETagMatches(c.GetHeader("If-None-Match"), etag) {
				c.Status(http.StatusNotModified)
				return
			}
			c.Data(http.StatusOK, "application/json", body)
			return
		}

		if manifest.ETag != "" {
			c.Header("ETag", manifest.ETag)
		}
		if manifest.NotModified {
			c.Status(http.StatusNotModified)
			return
		}
		c.Data(http.StatusOK, "application/json", manifest.Body)
		return
	}
}

// codexManifestETagMatches 判断客户端 If-None-Match 是否命中当前 ETag（弱比较，支持列表和 *）。
func codexManifestETagMatches(ifNoneMatch, etag string) bool {
	etag = strings.TrimPrefix(strings.TrimSpace(etag), "W/")
	if etag == "" {
		return false
	}
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}
