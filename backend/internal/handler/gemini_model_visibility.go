package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Gemini discovery must use the same group/platform model_mapping keys as
// /v1/models, not the catalogue belonging to whichever upstream account wins
// selection. In particular, public aliases are keys, never mapping targets.
// A nil list retains the existing unrestricted upstream/fallback behaviour;
// a non-nil empty list is authoritative and must not revive default models.
type geminiModelVisibility struct {
	configured bool
	patterns   []string
}

func (h *GatewayHandler) geminiModelVisibility(c *gin.Context, apiKey *service.APIKey, platform string) geminiModelVisibility {
	if h.gatewayService == nil {
		return geminiModelVisibility{}
	}
	groupID := apiKey.GroupID
	if groupID == nil && apiKey.Group != nil {
		groupID = &apiKey.Group.ID
	}
	ids := h.gatewayService.GetAvailableModels(c.Request.Context(), groupID, platform)
	return newGeminiModelVisibility(ids)
}

func newGeminiModelVisibility(ids []string) geminiModelVisibility {
	v := geminiModelVisibility{configured: ids != nil}
	for _, id := range ids {
		id = strings.TrimPrefix(strings.TrimSpace(id), "models/")
		if id != "" {
			v.patterns = append(v.patterns, id)
		}
	}
	return v
}

func (v geminiModelVisibility) allows(model string) bool {
	if !v.configured {
		return true
	}
	model = strings.TrimPrefix(model, "models/")
	for _, pattern := range v.patterns {
		if model == pattern || (strings.HasSuffix(pattern, "*") && strings.HasPrefix(model, strings.TrimSuffix(pattern, "*"))) {
			return true
		}
	}
	return false
}

func (v geminiModelVisibility) hasExplicitModel(model string) bool {
	for _, pattern := range v.patterns {
		if pattern == model && !strings.HasSuffix(pattern, "*") {
			return true
		}
	}
	return false
}

func (v geminiModelVisibility) hasWildcard() bool {
	for _, pattern := range v.patterns {
		if strings.HasSuffix(pattern, "*") {
			return true
		}
	}
	return false
}

// finiteList avoids an upstream catalogue request entirely for explicit
// whitelists. This also exposes aliases absent from Google's catalogue and
// prevents upstream pagination or scope errors from leaking extra models.
func (v geminiModelVisibility) finiteList() gemini.ModelsListResponse {
	models := make([]gemini.Model, 0, len(v.patterns))
	seen := make(map[string]bool, len(v.patterns))
	for _, id := range v.patterns {
		if seen[id] || strings.HasSuffix(id, "*") || !service.IsSafeGeminiModelPathSegment(id) {
			continue
		}
		seen[id] = true
		models = append(models, gemini.FallbackModel(id))
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	return gemini.ModelsListResponse{Models: models}
}

// filterList preserves upstream metadata and wildcard matching, while adding
// explicit aliases that need not exist in the upstream's own model catalogue.
func (v geminiModelVisibility) filterList(body []byte) ([]byte, error) {
	if !v.configured {
		return body, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	var models []json.RawMessage
	if err := json.Unmarshal(payload["models"], &models); err != nil {
		return nil, err
	}
	visible := make([]json.RawMessage, 0, len(models))
	seen := make(map[string]bool)
	for _, raw := range models {
		var model struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &model); err != nil {
			return nil, err
		}
		id := strings.TrimPrefix(model.Name, "models/")
		if !service.IsSafeGeminiModelPathSegment(id) || !v.allows(id) || seen[id] {
			continue
		}
		seen[id] = true
		visible = append(visible, raw)
	}
	for _, model := range v.finiteList().Models {
		id := strings.TrimPrefix(model.Name, "models/")
		if !seen[id] {
			raw, err := json.Marshal(model)
			if err != nil {
				return nil, err
			}
			visible = append(visible, raw)
		}
	}
	raw, err := json.Marshal(visible)
	if err != nil {
		return nil, err
	}
	payload["models"] = raw
	return json.Marshal(payload)
}

func writeVisibleGeminiFallback(c *gin.Context, visibility geminiModelVisibility, fallback any) {
	body, err := json.Marshal(fallback)
	if err == nil {
		body, err = visibility.filterList(body)
	}
	if err != nil {
		googleError(c, http.StatusBadGateway, "Invalid Gemini model catalogue")
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}

func writeVisibleGeminiUpstream(c *gin.Context, visibility geminiModelVisibility, res *service.UpstreamHTTPResult) {
	if res.StatusCode == http.StatusOK && visibility.configured {
		body, err := visibility.filterList(res.Body)
		if err != nil {
			googleError(c, http.StatusBadGateway, "Invalid Gemini model catalogue")
			return
		}
		// A transformed body must not inherit an upstream Content-Length,
		// Content-Encoding, or cache validator for the unfiltered catalogue.
		c.Data(http.StatusOK, "application/json", body)
		return
	}
	writeUpstreamResponse(c, res)
}
