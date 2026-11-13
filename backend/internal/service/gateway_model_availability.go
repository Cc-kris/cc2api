package service

// 移植自上游 fcd3bc127（V02-T28）。V04 起 Anthropic/Gemini handler 也会调用，
// 因此实现按平台枚举持久化可见账号，而不是只看 OpenAI handler 的临时列表。

import (
	"context"
	"strings"
)

// ModelAvailabilityDiagnosis describes whether the requested model can be
// served by any configured account in the group, ignoring transient state
// (rate limits, quota auto-pause, runtime blocks). Handlers use this on the
// "no available accounts" error path to distinguish 404 model_not_found from
// 503 service_unavailable.
type ModelAvailabilityDiagnosis struct {
	// HasAccountsInPool is true if the group has at least one schedulable
	// account on the queried platform (or, for Anthropic/Gemini, on the
	// platform plus mixed-scheduled Antigravity accounts).
	HasAccountsInPool bool
	// HasModelSupport is true if at least one account's model mapping admits
	// the requested model.
	HasModelSupport bool
	// HasRetiredSubscriptionModel records a ChatGPT upstream blocked by model
	// retirement, so an unsupported request can include migration guidance (26aab6dc0).
	HasRetiredSubscriptionModel bool
}

// ModelAvailabilityDiagnoser is implemented by gateway services that can
// report whether the requested model is configured to be served by any
// account. Both *GatewayService and *OpenAIGatewayService implement this so
// handlers in either package can share a single classifier.
type ModelAvailabilityDiagnoser interface {
	DiagnoseModelAvailabilityForPlatform(
		ctx context.Context,
		groupID *int64,
		requestedModel string,
		platform string,
	) ModelAvailabilityDiagnosis
}

// DiagnoseModelAvailabilityForPlatform inspects persistently active,
// scheduling-enabled accounts of the given platform and checks model mappings.
// It deliberately ignores transient cooldowns, quotas, and runtime blocks; the
// persisted enabled flag is still an eligibility requirement (V04).
//
// Safe to call on the error path: returns {true,true} on any internal failure
// or when the inputs preclude meaningful diagnosis (empty model, etc.), so
// callers stay on the 503 fallback branch.
func (s *GatewayService) DiagnoseModelAvailabilityForPlatform(
	ctx context.Context,
	groupID *int64,
	requestedModel string,
	platform string,
) ModelAvailabilityDiagnosis {
	if s == nil {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		// No model specified — cannot decide model_not_found. Caller falls back to 503.
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	if strings.TrimSpace(platform) == "" {
		// Without a platform we cannot scope the lookup; bail out to the
		// 503 branch rather than make an unscoped scan.
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}

	// hasForcePlatform=false so Anthropic/Gemini also surface mixed-scheduled
	// Antigravity accounts, matching what selection would consider.
	if s.accountRepo == nil {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	var accounts []Account
	var err error
	if groupID == nil {
		// Preserve the existing simple/ungrouped selection scope. A missing
		// group must not silently turn every unsupported model into a 503.
		accounts, _, err = s.listSchedulableAccounts(ctx, nil, platform, false)
	} else {
		accounts, err = s.accountRepo.ListByGroup(ctx, *groupID)
	}
	if err != nil {
		// Conservative fallback: pretend everything is fine so the caller
		// returns 503 (we don't want to flip to 404 just because a lookup
		// hiccup'd).
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}

	// Only persisted active + scheduling-enabled accounts on the requested
	// platform (plus mixed-scheduled Antigravity) count as an actionable pool:
	// a paused/retired account is not something the operator can fix by
	// editing model_mapping, so it must not produce a 404 (V04).
	mixed := platform == PlatformAnthropic || platform == PlatformGemini
	diag := ModelAvailabilityDiagnosis{}
	for i := range accounts {
		if !accounts[i].IsActive() || !accounts[i].Schedulable {
			continue
		}
		if isPlatformFilteredForSelection(&accounts[i], platform, mixed) {
			continue
		}
		diag.HasAccountsInPool = true
		if s.isModelSupportedByAccountWithContext(ctx, &accounts[i], requestedModel) {
			diag.HasModelSupport = true
			return diag
		}
	}
	return diag
}
