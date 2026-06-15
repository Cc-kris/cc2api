package handler

import (
	"errors"
	"net/http"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
)

func extractMaxBytesError(err error) (*http.MaxBytesError, bool) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return maxErr, true
	}
	return nil, false
}

func buildBodyTooLargeMessage(limit int64) string {
	return pkghttputil.RequestBodyTooLargeClientMessage
}
