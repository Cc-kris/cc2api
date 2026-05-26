package service

import (
	"encoding/json"
	"log/slog"
	"sort"
)

func normalizeBalanceLowNotifyExcludedUserIDs(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(values))
	normalized := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	return normalized
}

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
