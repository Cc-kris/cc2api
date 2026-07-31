package routes

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterUpstreamRoutesIncludesAccountUsageSyncEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		Upstream:       adminhandler.NewUpstreamHandler(nil),
		UpstreamWallet: adminhandler.NewUpstreamWalletHandler(nil, nil, nil),
	}}
	registerUpstreamRoutes(router.Group("/api/v1/admin"), handlers)

	registered := false
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && route.Path == "/api/v1/admin/upstream-wallets/:id/sync-account-usage" {
			registered = true
			break
		}
	}
	require.True(t, registered)
}
