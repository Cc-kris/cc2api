package service

import (
	"io"
	"net/http"
)

// The handler owns the sole same-account counter for pool-mode responses.
// Never run the legacy transport retries or cooldown writes before returning it.
func poolModeStatusFailover(resp *http.Response, account *Account) *UpstreamFailoverError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	_ = resp.Body.Close()
	return &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: body, Reason: GatewayFailureReasonPoolStatus, ResponseHeaders: resp.Header.Clone(), RetryableOnSameAccount: account != nil && account.IsPoolModeRetryableStatus(resp.StatusCode)}
}
