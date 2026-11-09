package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newPaymentRoutesTestRouter(redisClient *redis.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	noop := func(c *gin.Context) { c.Next() }

	RegisterPaymentRoutes(
		v1,
		&handler.PaymentHandler{},
		&handler.PaymentWebhookHandler{},
		&admin.PaymentHandler{},
		servermiddleware.JWTAuthMiddleware(noop),
		servermiddleware.AdminAuthMiddleware(noop),
		nil,
		redisClient,
	)
	return router
}

func postPublicOrderVerify(router *gin.Engine, remoteAddr string) *httptest.ResponseRecorder {
	// Empty body fails binding in the handler, so no service is touched and a
	// non-429 response proves the request passed the limiter.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment/public/orders/verify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// V01-AC17
func TestPublicOrderVerifyRateLimitedPerIP(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	router := newPaymentRoutesTestRouter(rdb)

	for i := 1; i <= publicOrderVerifyRateLimit; i++ {
		w := postPublicOrderVerify(router, "198.51.100.20:1234")
		require.Equal(t, http.StatusBadRequest, w.Code, "request %d should reach the handler", i)
	}

	w := postPublicOrderVerify(router, "198.51.100.20:1234")
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Contains(t, w.Body.String(), "rate limit exceeded")

	// A different client IP has its own budget.
	w = postPublicOrderVerify(router, "198.51.100.21:1234")
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// V01-AC18
func TestPublicOrderVerifyRateLimitFailsOpenWhenRedisUnavailable(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() { _ = rdb.Close() })

	router := newPaymentRoutesTestRouter(rdb)
	w := postPublicOrderVerify(router, "203.0.113.30:1234")
	require.Equal(t, http.StatusBadRequest, w.Code, "users mid-payment must not be blocked by a Redis outage")
}

// 签名的 resolve 接口不挂限流（PRD V01-R05 边界）。
func TestPublicOrderResolveNotRateLimited(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	router := newPaymentRoutesTestRouter(rdb)
	for i := 1; i <= publicOrderVerifyRateLimit+5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/payment/public/orders/resolve", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.40:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusTooManyRequests, w.Code, "request %d", i)
	}
}

// V01-AC20：用户端 /payment/channels 已下线。
func TestUserPaymentChannelsRouteRemoved(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	router := newPaymentRoutesTestRouter(rdb)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/payment/channels", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	for _, route := range router.Routes() {
		require.NotEqual(t, "/api/v1/payment/channels", route.Path)
	}
}
