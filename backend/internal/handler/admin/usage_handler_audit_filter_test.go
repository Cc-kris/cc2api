package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// AC31（后半）：管理员按 upstream_request_id 查询、按「模型不一致」筛选，列表和统计口径一致。
func TestAdminUsageAuditFilters(t *testing.T) {
	for _, path := range []string{"/admin/usage", "/admin/usage/stats"} {
		repo := &adminUsageRepoCapture{}
		router := newAdminUsageRequestTypeTestRouter(repo)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path+"?upstream_request_id=%20req_abc%20&upstream_model_mismatch=true", nil))
		require.Equal(t, http.StatusOK, rec.Code, path)

		filters := repo.listFilters
		if path == "/admin/usage/stats" {
			filters = repo.statsFilters
		}
		require.Equal(t, "req_abc", filters.UpstreamRequestID, path)
		require.NotNil(t, filters.UpstreamModelMismatch, path)
		require.True(t, *filters.UpstreamModelMismatch, path)
	}

	repo := &adminUsageRepoCapture{}
	router := newAdminUsageRequestTypeTestRouter(repo)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/usage", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "", repo.listFilters.UpstreamRequestID)
	require.Nil(t, repo.listFilters.UpstreamModelMismatch)
}

func TestAdminUsageAuditFilters_RejectInvalidValues(t *testing.T) {
	for _, query := range []string{
		"upstream_model_mismatch=maybe",
		"upstream_request_id=" + strings.Repeat("a", 129),
	} {
		for _, path := range []string{"/admin/usage", "/admin/usage/stats"} {
			router := newAdminUsageRequestTypeTestRouter(&adminUsageRepoCapture{})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path+"?"+query, nil))
			require.Equal(t, http.StatusBadRequest, rec.Code, path+"?"+query)
		}
	}
}

// 管理员 DTO 带全部审计字段；用户 DTO 只带映射前推理强度，不泄露 session_id / 上游请求 ID / 上游模型。
func TestUsageLogDTO_AuditFieldVisibility(t *testing.T) {
	responseModel := "claude-sonnet-4-5"
	mismatch := true
	sessionID := "sess-abc"
	requested := "max"
	forwarded := "high"
	upstreamRequestID := "req_abc"
	log := &service.UsageLog{
		RequestID:       "local-1",
		Model:           "claude-sonnet-4",
		ReasoningEffort: &forwarded,
		UsageAuditFields: service.UsageAuditFields{
			UpstreamResponseModel:    &responseModel,
			UpstreamModelMismatch:    &mismatch,
			SessionID:                &sessionID,
			RequestedReasoningEffort: &requested,
			UpstreamRequestID:        &upstreamRequestID,
		},
	}

	adminJSON, err := json.Marshal(dto.UsageLogFromServiceAdmin(log))
	require.NoError(t, err)
	for _, fragment := range []string{`"upstream_response_model":"claude-sonnet-4-5"`, `"upstream_model_mismatch":true`, `"session_id":"sess-abc"`, `"requested_reasoning_effort":"max"`, `"upstream_request_id":"req_abc"`, `"reasoning_effort":"high"`} {
		require.Contains(t, string(adminJSON), fragment)
	}

	userJSON, err := json.Marshal(dto.UsageLogFromService(log))
	require.NoError(t, err)
	require.Contains(t, string(userJSON), `"requested_reasoning_effort":"max"`)
	for _, hidden := range []string{"upstream_response_model", "upstream_model_mismatch", "session_id", "upstream_request_id"} {
		require.NotContains(t, string(userJSON), hidden)
	}
}
