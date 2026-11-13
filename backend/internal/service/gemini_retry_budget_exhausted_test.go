//go:build unit

package service

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestGeminiRetryBudgetEarlyFailureAndPool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fw := range geminiSingleCheckForwarders {
		for _, pool := range []bool{false, true} {
			name := fw.name + "/early"
			status := http.StatusUnauthorized
			if pool {
				name = fw.name + "/pool"
				status = http.StatusBadGateway
			}
			t.Run(name, func(t *testing.T) {
				svc, upstream, _ := newGeminiSingleCheckService(geminiSequenceUpstreamResponse{status: status, body: `{"error":{"message":"fixture failure"}}`})
				svc.cfg.Gateway.MaxSameAccountRetriesGemini = 3
				a := geminiSameAccountRetryTestAccount()
				if pool {
					a.Credentials["pool_mode"] = true
					a.Credentials["pool_mode_retry_status_codes"] = []int{502}
				}
				err, _ := fw.forward(t, svc, a)
				var failure *UpstreamFailoverError
				require.ErrorAs(t, err, &failure)
				require.Equal(t, 1, upstream.calls)
				require.False(t, failure.SameAccountRetryBudgetExhausted)
				if pool {
					require.Equal(t, GatewayFailureReasonPoolStatus, failure.Reason)
					require.True(t, failure.RetryableOnSameAccount)
				}
			})
		}
	}
}
