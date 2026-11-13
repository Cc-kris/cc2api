package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const anthropicFableRateLimitKey = "anthropic_fable"
const openAICodexAutoPauseStaleAfter = 2 * time.Hour

func openAICodexSnapshotStaleForPause(extra map[string]any, now time.Time) bool {
	updated, err := parseTime(fmt.Sprint(extra["codex_usage_updated_at"]))
	return err == nil && now.Sub(updated) >= openAICodexAutoPauseStaleAfter
}
func openAICodexWindowResetAt(extra map[string]any, window string) (time.Time, bool) {
	if raw, ok := extra["codex_"+window+"_reset_at"]; ok {
		if t := parseSchedulingResetAt(raw); t != nil {
			return *t, true
		}
	}
	seconds := parseExtraInt(extra["codex_"+window+"_reset_after_seconds"])
	updated, err := parseTime(fmt.Sprint(extra["codex_usage_updated_at"]))
	if seconds <= 0 || err != nil {
		return time.Time{}, false
	}
	return updated.Add(time.Duration(seconds) * time.Second), true
}
func openAIQuotaWindowReset(extra map[string]any, window string, now time.Time) bool {
	reset, ok := openAICodexWindowResetAt(extra, window)
	return ok && !now.Before(reset)
}

// ApplyModelSchedulingThreshold keeps the Fable-only weekly quota scoped to the
// requested model; a depleted Fable window must not pause Sonnet/Haiku traffic.
func (s *RateLimitService) ApplyModelSchedulingThreshold(ctx context.Context, account *Account, model string) bool {
	if s == nil || s.settingService == nil || s.accountRepo == nil || account == nil || account.Platform != PlatformAnthropic || !strings.Contains(strings.ToLower(model), "fable") {
		return false
	}
	now := time.Now().UTC()
	decision := evaluateAnthropicFableSchedulingThreshold(account, s.settingService.GetAccountSchedulingThresholds(ctx), now)
	if !decision.ShouldPause || decision.Until == nil {
		return false
	}
	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, model, *decision.Until); err != nil {
		slog.Warn("model_scheduling_threshold_set_failed", "account_id", account.ID, "error", err)
	}
	return true
}

// Threshold decisions never mutate cached account objects. Persistence invalidates
// scheduler snapshots through the existing account repository outbox.
func (s *RateLimitService) ApplyAccountSchedulingThreshold(ctx context.Context, account *Account) bool {
	if s == nil || s.settingService == nil || s.accountRepo == nil || account == nil || account.ID <= 0 || !account.IsActive() || !account.Schedulable {
		return false
	}
	now := time.Now().UTC()
	decision := EvaluateAccountSchedulingThreshold(account, s.settingService.GetAccountSchedulingThresholds(ctx), now)
	if !decision.ShouldPause || decision.Until == nil {
		return false
	}
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(now) {
		return IsAccountSchedulingThresholdReason(account.TempUnschedulableReason)
	}
	if !account.IsSchedulable() {
		return false
	}
	reason := BuildDetailedAccountSchedulingThresholdReason(AccountSchedulingThresholdReasonInput{Platform: decision.Platform, Window: decision.Window, Scope: decision.Scope, ThresholdPercent: decision.ThresholdPercent, UsedPercent: decision.UsedPercent, Until: *decision.Until, Now: now})
	if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, *decision.Until, reason); err != nil {
		slog.Warn("account_scheduling_threshold_set_failed", "account_id", account.ID, "error", err)
	}
	if s.tempUnschedCache != nil {
		_ = s.tempUnschedCache.SetTempUnsched(ctx, account.ID, tempUnschedStateFromStoredReason(reason, decision.Until.Unix()))
	}
	return true
}
