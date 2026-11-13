package service

import (
	"context"
	"strings"
	"time"
)

type upstreamErrorModelKey struct{}
type upstreamErrorOutcomeKey struct{}
type upstreamErrorOutcome struct{ modelOnly bool }

func upstreamErrorModelContext(ctx context.Context, models []string) context.Context {
	if len(models) > 0 && strings.TrimSpace(models[0]) != "" {
		return context.WithValue(ctx, upstreamErrorModelKey{}, strings.TrimSpace(models[0]))
	}
	return ctx
}
func upstreamErrorModel(ctx context.Context, account *Account) string {
	model, _ := ctx.Value(upstreamErrorModelKey{}).(string)
	if account != nil && model != "" {
		model = account.GetMappedModel(model)
	}
	return strings.TrimSpace(model)
}

// Only explicit model-scoped evidence may isolate a model automatically. Generic
// permission failures still use the existing account policy.
func (s *RateLimitService) handleModelScopedUpstreamError(ctx context.Context, account *Account, status int, body []byte) bool {
	model := upstreamErrorModel(ctx, account)
	if model == "" {
		return false
	}
	code := strings.ToLower(extractUpstreamErrorCode(body))
	message := strings.ToLower(extractUpstreamErrorMessage(body))
	unsupported := (status == 400 || status == 404) && (code == "model_not_found" || strings.Contains(message, "model does not exist") || strings.Contains(message, "model is not supported"))
	policy := status == 403 && (code == "model_access_denied" || code == "model_not_allowed" || strings.Contains(message, "not allowed to access this model"))
	if !unsupported && !policy {
		return false
	}
	until := time.Now().Add(15 * time.Minute)
	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, model, until); err != nil {
		return false
	}
	s.notifyModelCooldownPersisted(ctx, account.ID)
	if outcome, _ := ctx.Value(upstreamErrorOutcomeKey{}).(*upstreamErrorOutcome); outcome != nil {
		outcome.modelOnly = true
	}
	return true
}
