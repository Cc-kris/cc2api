package service

import (
	"context"
	"log/slog"
)

// Model cooldowns refresh the shared snapshot without installing an account-wide
// runtime block. Re-read durable state rather than overwriting concurrent model
// cooldowns with a potentially stale request account.
type modelCooldownSnapshotRefresher interface {
	RefreshModelCooldownSnapshot(context.Context, int64)
}

func (s *RateLimitService) notifyModelCooldownPersisted(ctx context.Context, accountID int64) {
	if refresher, ok := s.runtimeBlocker.(modelCooldownSnapshotRefresher); ok {
		refresher.RefreshModelCooldownSnapshot(ctx, accountID)
	}
}
func (s *OpenAIGatewayService) RefreshModelCooldownSnapshot(ctx context.Context, accountID int64) {
	if s == nil || s.schedulerSnapshot == nil || s.accountRepo == nil {
		return
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err == nil && account != nil {
		err = s.schedulerSnapshot.UpdateAccountInCache(ctx, account)
	}
	if err != nil {
		slog.Warn("model_cooldown_snapshot_refresh_failed", "account_id", accountID, "error", err)
	}
}
