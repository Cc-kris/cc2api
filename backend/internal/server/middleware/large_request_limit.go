package middleware

import (
	"net/http"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
)

type LargeRequestLimiterOptions struct {
	Enabled               bool
	ThresholdBytes        int64
	MaxConcurrentRequests int
	WaitTimeout           time.Duration
}

func LargeRequestLimiter(opts LargeRequestLimiterOptions) gin.HandlerFunc {
	if !opts.Enabled || opts.ThresholdBytes <= 0 || opts.MaxConcurrentRequests <= 0 {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	sem := make(chan struct{}, opts.MaxConcurrentRequests)
	return func(c *gin.Context) {
		if c.Request == nil || c.Request.Method == http.MethodGet || c.Request.ContentLength < opts.ThresholdBytes {
			c.Next()
			return
		}

		acquired := false
		if opts.WaitTimeout <= 0 {
			select {
			case sem <- struct{}{}:
				acquired = true
			default:
			}
		} else {
			timer := time.NewTimer(opts.WaitTimeout)
			defer timer.Stop()
			select {
			case sem <- struct{}{}:
				acquired = true
			case <-timer.C:
			case <-c.Request.Context().Done():
			}
		}
		if !acquired {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{
					"type":    "server_overloaded",
					"message": "大图片请求正在排队，请稍后重试",
				},
			})
			return
		}
		defer func() {
			<-sem
		}()
		c.Next()
	}
}

func LargeRequestTooLargeResponse(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
		"error": gin.H{
			"type":    "invalid_request_error",
			"message": pkghttputil.RequestBodyTooLargeClientMessage,
		},
	})
}
