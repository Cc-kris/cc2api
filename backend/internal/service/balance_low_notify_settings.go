package service

import (
	"encoding/json"
	"log/slog"
)

func ParseBalanceLowNotifyExcludedUserIDs(raw string) []int64 {
	if raw == "" {
		return nil
	}
	var values []int64
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		slog.Warn("failed to parse balance low notify excluded user ids", "error", err)
		return nil
	}
	return normalizeBalanceLowNotifyExcludedUserIDs(values)
}

func MarshalBalanceLowNotifyExcludedUserIDs(values []int64) string {
	normalized := normalizeBalanceLowNotifyExcludedUserIDs(values)
	if normalized == nil {
		return "[]"
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(payload)
}
