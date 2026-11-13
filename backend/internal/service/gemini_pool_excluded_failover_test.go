//go:build unit

package service

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"testing"
)

// Excluding a pool HTTP status disables same-account retry, not group-enabled
// failover. When any-error is disabled the legacy direct response is retained.
func TestGeminiPoolExcludedStatusAnyError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fw := range geminiSingleCheckForwarders[:2] {
		for _, enabled := range []bool{false, true} {
			for _, status := range []int{502, 401} {
				svc, upstream, _ := newGeminiSingleCheckService(geminiSequenceUpstreamResponse{status: status, body: `{"error":{"code":502,"message":"fixture upstream failure"}}`})
				svc.cfg.Gateway.FailoverOnAnyError = enabled
				a := geminiSameAccountRetryTestAccount()
				a.Credentials["pool_mode"] = true
				a.Credentials["pool_mode_retry_count"] = 3
				if status == 502 {
					a.Credentials["pool_mode_retry_status_codes"] = []int{}
				} else {
					a.Credentials["pool_mode_retry_status_codes"] = []int{502}
				}
				err, c := fw.forward(t, svc, a)
				require.Error(t, err)
				require.Equal(t, 1, upstream.calls)
				if enabled {
					var failure *UpstreamFailoverError
					require.ErrorAs(t, err, &failure)
					require.Equal(t, GatewayFailureReasonPoolStatus, failure.Reason)
					require.False(t, failure.RetryableOnSameAccount)
					require.False(t, failure.SameAccountRetryBudgetExhausted)
					require.False(t, c.Writer.Written(), "must not leak failed attempt before changing account")
				} else {
					_, failover := err.(*UpstreamFailoverError)
					require.False(t, failover)
					require.True(t, c.Writer.Written())
				}
			}
		}
	}
}
