package service

import (
	"bytes"
	"encoding/json"
	"math"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ValidatePoolModeRetryStatusCodes validates writes, not only the defensive read
// path. Missing means use local defaults; an explicit [] disables status retries.
func ValidatePoolModeRetryStatusCodes(credentials map[string]any) error {
	value, provided := credentials["pool_mode_retry_status_codes"]
	if !provided {
		return nil
	}
	invalid := func() error {
		return infraerrors.BadRequest("INVALID_POOL_RETRY_STATUS_CODES", "pool_mode_retry_status_codes must be an array of integer HTTP status codes from 100 to 599")
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) == 0 || encoded[0] != '[' {
		return invalid()
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(encoded, &entries); err != nil {
		return invalid()
	}
	for _, entry := range entries {
		decoder := json.NewDecoder(bytes.NewReader(entry))
		decoder.UseNumber()
		var item any
		if err := decoder.Decode(&item); err != nil {
			return invalid()
		}
		number, ok := item.(json.Number)
		if !ok {
			return invalid()
		}
		status, err := number.Float64()
		if err != nil || math.IsNaN(status) || math.IsInf(status, 0) || math.Trunc(status) != status || status < 100 || status > 599 {
			return invalid()
		}
	}
	return nil
}
