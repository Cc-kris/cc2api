package service

import (
	"context"
	"strings"
)

// DiagnoseModelAvailabilityForPlatform reports whether the requested model is
// configured to be served by an active, scheduling-enabled account of the
// requested platform. Transient cooldowns do not change model support (V04).
//
// Safe to call on the error path: returns {true,true} on any internal
// failure or when the inputs preclude meaningful diagnosis (empty model,
// nil service), so callers stay on the 503 fallback branch.
//
// CCAI 的 OpenAI handler 也承接 Grok、Kimi、智谱、DeepSeek 等兼容平台的请求，
// 因此这里按请求平台过滤，避免把一个平台上的账号池误判成另一个平台的 404。
func (s *OpenAIGatewayService) DiagnoseModelAvailabilityForPlatform(
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
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}

	platform = strings.TrimSpace(platform)
	if platform == "" {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	if s.accountRepo == nil {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	var accounts []Account
	var err error
	if groupID == nil {
		// Ungrouped compatible-platform routing has no group policy to
		// diagnose. Keep its legacy conservative error classification.
		if platform != PlatformOpenAI {
			return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
		}
		accounts, err = s.listSchedulableAccounts(ctx, nil)
	} else {
		accounts, err = s.accountRepo.ListByGroup(ctx, *groupID)
	}
	if err != nil {
		// Conservative fallback so the caller keeps returning 503; we do not
		// want a transient lookup failure to flip into 404 model_not_found.
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}

	mixed := platform == PlatformAnthropic || platform == PlatformGemini
	diag := ModelAvailabilityDiagnosis{}
	for i := range accounts {
		if accounts[i].Platform != platform || !accounts[i].IsActive() || !accounts[i].Schedulable {
			continue
		}
		if isPlatformFilteredForSelection(&accounts[i], platform, mixed) {
			continue
		}
		diag.HasAccountsInPool = true
		if isRetiredOpenAISubscriptionModel(&accounts[i], resolveOpenAIAccountUpstreamModelForRequest(&accounts[i], requestedModel, false)) {
			diag.HasRetiredSubscriptionModel = true
		}
		// Mirrors the per-candidate model filter used during account selection
		// (openai_account_scheduler.isAccountRequestCompatible): empty
		// model_mapping accepts everything; otherwise the explicit / wildcard
		// mapping must match.
		if supportsOpenAIRequestModel(&accounts[i], requestedModel, false) {
			diag.HasModelSupport = true
			return diag
		}
	}
	return diag
}
