package service

import "fmt"

// Use the actual outbound model key, including compact mapping and OAuth
// normalization, rather than treating a model cooldown as an account pause.
func openAIRequestModelRateLimited(account *Account, model string, compact bool) bool {
	return account != nil && model != "" && account.isRateLimitActiveForKey(resolveOpenAIAccountUpstreamModelForRequest(account, model, compact))
}

// Preserve the selection error identity and compact diagnosis, while exposing
// model cooldowns to the shared HTTP error classifier. Unsupported models never
// contribute a cooldown hint; persistent support diagnosis still wins over it.
func openAISelectionModelCooldownError(err error, accounts []Account, model string, compact bool, excluded map[int64]struct{}) error {
	count := 0
	for i := range accounts {
		a := &accounts[i]
		if _, skip := excluded[a.ID]; skip {
			continue
		}
		if a.IsOpenAI() && a.IsSchedulable() && supportsOpenAIRequestModel(a, model, compact) && openAIRequestModelRateLimited(a, model, compact) {
			count++
		}
	}
	if count > 0 {
		return fmt.Errorf("%w (model_rate_limited=%d)", err, count)
	}
	return err
}
