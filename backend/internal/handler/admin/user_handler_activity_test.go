//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserHandlerListIncludesActivityFieldsAndSortParams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	lastLoginAt := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	lastActiveAt := lastLoginAt.Add(30 * time.Minute)
	lastUsedAt := lastLoginAt.Add(90 * time.Minute)

	adminSvc := newStubAdminService()
	adminSvc.users = []service.User{
		{
			ID:           7,
			Email:        "activity@example.com",
			Username:     "activity-user",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
			LastActiveAt: &lastActiveAt,
			LastUsedAt:   &lastUsedAt,
			CreatedAt:    lastLoginAt.Add(-24 * time.Hour),
			UpdatedAt:    lastLoginAt,
		},
	}
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/users?sort_by=last_used_at&sort_order=asc&search=activity",
		nil,
	)

	handler.List(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "last_used_at", adminSvc.lastListUsers.sortBy)
	require.Equal(t, "asc", adminSvc.lastListUsers.sortOrder)
	require.Equal(t, "activity", adminSvc.lastListUsers.filters.Search)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				LastActiveAt *time.Time `json:"last_active_at"`
				LastUsedAt   *time.Time `json:"last_used_at"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Items, 1)
	require.WithinDuration(t, lastActiveAt, *resp.Data.Items[0].LastActiveAt, time.Second)
	require.WithinDuration(t, lastUsedAt, *resp.Data.Items[0].LastUsedAt, time.Second)
}

func TestUserHandlerListPassesBalanceFilterParams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newStubAdminService()
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/users?status=active&balance_filter_type=between&balance_min=-1.25&balance_max=100.0001",
		nil,
	)

	handler.List(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, service.StatusActive, adminSvc.lastListUsers.filters.Status)
	require.Equal(t, "between", adminSvc.lastListUsers.filters.BalanceFilterType)
	require.NotNil(t, adminSvc.lastListUsers.filters.BalanceMin)
	require.NotNil(t, adminSvc.lastListUsers.filters.BalanceMax)
	require.Equal(t, -1.25, *adminSvc.lastListUsers.filters.BalanceMin)
	require.Equal(t, 100.0001, *adminSvc.lastListUsers.filters.BalanceMax)
}

func TestUserHandlerListRejectsInvalidBalanceFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		url  string
	}{
		{
			name: "unknown filter type",
			url:  "/api/v1/admin/users?balance_filter_type=ne&balance_min=1",
		},
		{
			name: "missing minimum",
			url:  "/api/v1/admin/users?balance_filter_type=gte",
		},
		{
			name: "missing maximum",
			url:  "/api/v1/admin/users?balance_filter_type=lte",
		},
		{
			name: "between max below min",
			url:  "/api/v1/admin/users?balance_filter_type=between&balance_min=100&balance_max=10",
		},
		{
			name: "too many decimals",
			url:  "/api/v1/admin/users?balance_filter_type=eq&balance_min=1.12345",
		},
		{
			name: "non numeric",
			url:  "/api/v1/admin/users?balance_filter_type=gt&balance_min=abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adminSvc := newStubAdminService()
			handler := NewUserHandler(adminSvc, nil, nil, nil)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, tt.url, nil)

			handler.List(c)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Zero(t, adminSvc.lastListUsers.calls)
		})
	}
}

func TestUserHandlerGetByIDIncludesActivityFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	lastLoginAt := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	lastActiveAt := lastLoginAt.Add(30 * time.Minute)
	lastUsedAt := lastLoginAt.Add(90 * time.Minute)

	adminSvc := newStubAdminService()
	adminSvc.users = []service.User{
		{
			ID:           8,
			Email:        "detail@example.com",
			Username:     "detail-user",
			Role:         service.RoleUser,
			Status:       service.StatusActive,
			LastActiveAt: &lastActiveAt,
			LastUsedAt:   &lastUsedAt,
			CreatedAt:    lastLoginAt.Add(-24 * time.Hour),
			UpdatedAt:    lastLoginAt,
		},
	}
	handler := NewUserHandler(adminSvc, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "8"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/8", nil)

	handler.GetByID(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			LastActiveAt *time.Time `json:"last_active_at"`
			LastUsedAt   *time.Time `json:"last_used_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.WithinDuration(t, lastActiveAt, *resp.Data.LastActiveAt, time.Second)
	require.WithinDuration(t, lastUsedAt, *resp.Data.LastUsedAt, time.Second)
}
