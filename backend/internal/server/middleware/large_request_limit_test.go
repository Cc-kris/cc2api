package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLargeRequestLimiter_QueuesLargeRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	releaseFirst := make(chan struct{})
	firstStarted := make(chan struct{})
	firstDone := make(chan struct{})
	var mu sync.Mutex
	started := 0

	r := gin.New()
	r.Use(LargeRequestLimiter(LargeRequestLimiterOptions{
		Enabled:               true,
		ThresholdBytes:        10,
		MaxConcurrentRequests: 1,
		WaitTimeout:           time.Second,
	}))
	r.POST("/t", func(c *gin.Context) {
		mu.Lock()
		started++
		current := started
		mu.Unlock()
		if current == 1 {
			close(firstStarted)
			<-releaseFirst
			close(firstDone)
		}
		c.Status(http.StatusOK)
	})

	req1 := httptest.NewRequest(http.MethodPost, "/t", nil)
	req1.ContentLength = 16
	rec1 := httptest.NewRecorder()
	go r.ServeHTTP(rec1, req1)
	<-firstStarted

	req2 := httptest.NewRequest(http.MethodPost, "/t", nil)
	req2.ContentLength = 16
	rec2 := httptest.NewRecorder()
	done2 := make(chan struct{})
	go func() {
		r.ServeHTTP(rec2, req2)
		close(done2)
	}()

	select {
	case <-done2:
		t.Fatal("second large request should wait for the first slot")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseFirst)
	<-firstDone
	<-done2

	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, http.StatusOK, rec2.Code)
}

func TestLargeRequestLimiter_ReturnsBusyWhenQueueTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	block := make(chan struct{})
	firstStarted := make(chan struct{})

	r := gin.New()
	r.Use(LargeRequestLimiter(LargeRequestLimiterOptions{
		Enabled:               true,
		ThresholdBytes:        10,
		MaxConcurrentRequests: 1,
		WaitTimeout:           time.Millisecond,
	}))
	r.POST("/t", func(c *gin.Context) {
		close(firstStarted)
		<-block
		c.Status(http.StatusOK)
	})

	req1 := httptest.NewRequest(http.MethodPost, "/t", nil)
	req1.ContentLength = 16
	rec1 := httptest.NewRecorder()
	go r.ServeHTTP(rec1, req1)
	<-firstStarted

	req2 := httptest.NewRequest(http.MethodPost, "/t", nil)
	req2.ContentLength = 16
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	close(block)

	require.Equal(t, http.StatusServiceUnavailable, rec2.Code)
	require.Contains(t, rec2.Body.String(), "大图片请求正在排队，请稍后重试")
}
