package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSystemHandlerGetVersionUsesLocalVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/version", nil)

	handler := NewSystemHandler(service.NewUpdateService(nil, nil, "0.2", "source"), nil)
	handler.GetVersion(ctx)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, "success", body.Message)
	require.Equal(t, "0.2", body.Data.Version)
}
