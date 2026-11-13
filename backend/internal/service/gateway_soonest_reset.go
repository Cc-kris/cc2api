package service

import "time"

// filterBySoonestReset preserves priority ties while preferring an active window nearest reset.
func filterBySoonestReset(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) < 2 {
		return accounts
	}
	now := time.Now()
	var earliest *time.Time
	for _, item := range accounts {
		if item.account == nil {
			continue
		}
		end := item.account.SessionWindowEnd
		if end != nil && end.After(now) && (earliest == nil || end.Before(*earliest)) {
			earliest = end
		}
	}
	if earliest == nil {
		return accounts
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for _, item := range accounts {
		if item.account != nil && item.account.SessionWindowEnd != nil && item.account.SessionWindowEnd.Equal(*earliest) {
			result = append(result, item)
		}
	}
	return result
}
