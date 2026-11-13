package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"
)

func (s *OpenAIGatewayService) newStickySelectionResult(ctx context.Context, account *Account, acquired bool, release func(), plan *AccountWaitPlan) (*AccountSelectionResult, error) {
	result, err := s.newSelectionResult(ctx, account, acquired, release, plan)
	if result != nil {
		result.stickySessionHit = true
	}
	return result, err
}
func applyLegacySelectionDecision(decision *OpenAIAccountScheduleDecision, selection *AccountSelectionResult) {
	if selection == nil || selection.Account == nil {
		return
	}
	decision.SelectedAccountID = selection.Account.ID
	decision.SelectedAccountType = selection.Account.Type
	if selection.stickySessionHit {
		decision.Layer = openAIAccountScheduleLayerSessionSticky
		decision.StickySessionHit = true
	}
}

// Membership must be rechecked after resolving a sticky ID: binding TTLs outlive group removals.
func openAIStickyAccountMatchesGroup(account *Account, groupID *int64) bool {
	if account == nil {
		return false
	}
	if groupID == nil {
		return len(account.AccountGroups) == 0 && len(account.GroupIDs) == 0 && len(account.Groups) == 0
	}
	for _, g := range account.AccountGroups {
		if g.GroupID == *groupID {
			return true
		}
	}
	for _, id := range account.GroupIDs {
		if id == *groupID {
			return true
		}
	}
	for _, g := range account.Groups {
		if g != nil && g.ID == *groupID {
			return true
		}
	}
	return false
}
func (s *OpenAIGatewayService) openAIAccountMatchesCurrentGroup(ctx context.Context, account *Account, groupID *int64) bool {
	if account == nil {
		return false
	}
	if openAIStickyAccountMatchesGroup(account, groupID) {
		return true
	}
	// Legacy snapshots may omit memberships. Verify through the scoped repository
	// query instead of treating missing metadata as authorization.
	if groupID == nil || len(account.AccountGroups)+len(account.GroupIDs)+len(account.Groups) > 0 || s.accountRepo == nil {
		return false
	}
	pool, err := s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformOpenAI)
	if err != nil {
		return false
	}
	for _, candidate := range pool {
		if candidate.ID == account.ID {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) selectLegacyAccountByPreviousResponse(ctx context.Context, groupID *int64, previousID, sessionHash, model string, excluded map[int64]struct{}, transport OpenAIUpstreamTransport, image OpenAIImagesCapability, compact bool) (*AccountSelectionResult, bool, error) {
	if previousID == "" {
		return nil, false, nil
	}
	if s.checkChannelPricingRestriction(ctx, groupID, model) {
		return nil, false, fmt.Errorf("%w supporting model: %s (channel pricing restriction)", ErrNoAvailableAccounts, model)
	}
	selected, err := s.SelectAccountByPreviousResponseID(ctx, groupID, previousID, model, excluded, compact)
	if err != nil {
		return nil, false, err
	}
	if selected == nil || selected.Account == nil {
		return nil, false, nil
	}
	scheduler := &defaultOpenAIAccountScheduler{service: s}
	request := OpenAIAccountScheduleRequest{GroupID: groupID, RequestedModel: model, RequiredTransport: transport, RequiredImageCapability: image, RequireCompact: compact, ExcludedIDs: excluded}
	if !s.openAIAccountMatchesCurrentGroup(ctx, selected.Account, groupID) || !scheduler.isAccountRequestCompatible(ctx, selected.Account, request) || !scheduler.isAccountTransportCompatible(selected.Account, transport) {
		if selected.ReleaseFunc != nil {
			selected.ReleaseFunc()
		}
		return nil, false, nil
	}
	if sessionHash != "" {
		_ = s.BindStickySession(ctx, groupID, sessionHash, selected.Account.ID)
	}
	return selected, true, nil
}

const openAIQuotaHeadroomNeutralFactor = 0.5
const openAIQuotaHeadroomSecondaryLowRemain = 0.10
const openAIQuotaHeadroomSnapshotStaleAfter = 8 * time.Hour

func openAIQuotaHeadroomFactor(account *Account, now time.Time) float64 {
	if account == nil || len(account.Extra) == 0 || openAIQuotaHeadroomSnapshotStale(account.Extra, now) {
		return openAIQuotaHeadroomNeutralFactor
	}
	used, ok := resolveAccountExtraNumber(account.Extra, "codex_primary_used_percent", "codex_7d_used_percent")
	if !ok || openAIQuotaWindowResetAny(account.Extra, now, "primary", "7d") {
		return openAIQuotaHeadroomNeutralFactor
	}
	factor := 1 - clamp01(used/100)
	if secondary, ok := resolveAccountExtraNumber(account.Extra, "codex_secondary_used_percent", "codex_5h_used_percent"); ok && !openAIQuotaWindowResetAny(account.Extra, now, "secondary", "5h") && (1-clamp01(secondary/100)) < openAIQuotaHeadroomSecondaryLowRemain {
		factor *= openAIQuotaHeadroomNeutralFactor
	}
	return factor
}
func resolveAccountExtraNumber(extra map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := extra[key]; ok {
			n, err := strconv.ParseFloat(fmt.Sprint(value), 64)
			if err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 {
				return n, true
			}
		}
	}
	return 0, false
}
func openAIQuotaHeadroomSnapshotStale(extra map[string]any, now time.Time) bool {
	t, err := parseTime(fmt.Sprint(extra["codex_usage_updated_at"]))
	return err != nil || now.Sub(t) >= openAIQuotaHeadroomSnapshotStaleAfter
}
func openAIQuotaWindowResetAny(extra map[string]any, now time.Time, windows ...string) bool {
	for _, w := range windows {
		if openAIQuotaWindowReset(extra, w, now) {
			return true
		}
	}
	return false
}
