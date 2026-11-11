package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// AC32：账号 extra.upstream_request_id_header 为 65 个字符或含换行时，创建、更新、批量更新都返回 400，
// 且不会调用下游服务。合法值会被去空白后保存。
func TestAccountHandler_RejectsInvalidUpstreamRequestIDHeader(t *testing.T) {
	invalid := []string{strings.Repeat("x", 65), "X-Request\nId"}
	routes := []struct {
		method, path string
		body         func(extra map[string]any) map[string]any
	}{
		{http.MethodPost, "/api/v1/admin/accounts", func(extra map[string]any) map[string]any {
			return map[string]any{"name": "acc", "platform": "openai", "type": "apikey", "credentials": map[string]any{"api_key": "sk-x"}, "extra": extra}
		}},
		{http.MethodPut, "/api/v1/admin/accounts/1", func(extra map[string]any) map[string]any {
			return map[string]any{"extra": extra}
		}},
		{http.MethodPost, "/api/v1/admin/accounts/bulk-update", func(extra map[string]any) map[string]any {
			return map[string]any{"account_ids": []int64{1}, "extra": extra}
		}},
	}
	for _, header := range invalid {
		for _, route := range routes {
			adminSvc := newStubAdminService()
			router := setupAccountMixedChannelRouter(adminSvc)
			payload, _ := json.Marshal(route.body(map[string]any{service.AccountExtraUpstreamRequestIDHeader: header}))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(route.method, route.path, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code, "%s %s header=%q body=%s", route.method, route.path, header, rec.Body.String())
			require.Contains(t, rec.Body.String(), "INVALID_UPSTREAM_REQUEST_ID_HEADER")
			require.Empty(t, adminSvc.createdAccounts)
		}
	}

	adminSvc := newStubAdminService()
	router := setupAccountMixedChannelRouter(adminSvc)
	payload, _ := json.Marshal(routes[0].body(map[string]any{service.AccountExtraUpstreamRequestIDHeader: " X-Oneapi-Request-Id "}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, adminSvc.createdAccounts, 1)
	require.Equal(t, "X-Oneapi-Request-Id", adminSvc.createdAccounts[0].Extra[service.AccountExtraUpstreamRequestIDHeader])
}
