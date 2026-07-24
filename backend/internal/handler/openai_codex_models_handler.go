package handler

import (
	"net/http"

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

		manifest, fetchErr := h.gatewayService.FetchCodexModelsManifest(c.Request.Context(), account, c.Query("client_version"), c.GetHeader("If-None-Match"))
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
