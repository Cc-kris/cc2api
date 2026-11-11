package service

import (
	"log/slog"
	"strings"

	"github.com/tidwall/gjson"
)

// V03-T13：Anthropic Fast 识别与只降不升（移植上游 7dae055f2、75faedda9）。

// ServiceTierBillingResolution describes how the billable service tier of one
// request was settled between the tier the client asked for and the tier the
// upstream reports having used.
type ServiceTierBillingResolution struct {
	Requested  string // tier carried by the request sent upstream ("" when none)
	Observed   string // tier declared by the upstream response ("" when none)
	Billing    string // tier used for billing and the usage log
	Downgraded bool   // Billing is cheaper than Requested
}

// ResolveBillingServiceTier picks the tier to bill. The upstream declaration is
// trusted only to lower the bill: a request served below the tier it asked for
// (OpenAI "priority" answered with service_tier "default", Anthropic speed=fast
// answered with usage.speed "standard") is billed at the cheaper tier. A response
// that claims a more expensive tier, an unknown one, or none at all leaves the
// requested tier in place.
func ResolveBillingServiceTier(requested, observed string) ServiceTierBillingResolution {
	requested = normalizeBillingServiceTier(requested)
	observed = normalizeBillingServiceTier(observed)
	resolution := ServiceTierBillingResolution{Requested: requested, Observed: observed, Billing: requested}
	if observed == "" || observed == requested {
		return resolution
	}
	observedRank, known := serviceTierCostRank(observed)
	if !known {
		return resolution
	}
	requestedRank, _ := serviceTierCostRank(requested)
	if observedRank >= requestedRank {
		return resolution
	}
	resolution.Billing = observed
	resolution.Downgraded = true
	return resolution
}

// serviceTierCostRank orders tiers by their cost relative to the base rate, so a
// lower rank is always cheaper. Unknown tiers rank as the base rate and report
// known=false so callers can refuse to act on them.
func serviceTierCostRank(tier string) (rank int, known bool) {
	switch normalizeBillingServiceTier(tier) {
	case "flex":
		return 0, true
	case "", "default", "standard", "auto", "scale":
		return 1, true
	case "priority", "fast":
		return 2, true
	default:
		return 1, false
	}
}

// ResolveOpenAIServiceTierBilling applies the response-tier contract for the
// selected credential (upstream 3a9070359/50ad6e2e5 on top of 75faedda9). Public
// OpenAI API responses declare the actual tier and may lower billing. The private
// ChatGPT Codex endpoint does not: it commonly reports "default" for effective
// priority turns, so OAuth credentials keep the outbound tier.
func ResolveOpenAIServiceTierBilling(account *Account, requested, observed string) ServiceTierBillingResolution {
	if account != nil && account.IsOpenAIOAuth() && normalizeBillingServiceTier(observed) == "default" {
		return ServiceTierBillingResolution{
			Requested: normalizeBillingServiceTier(requested),
			Observed:  normalizeBillingServiceTier(observed),
			Billing:   normalizeBillingServiceTier(requested),
		}
	}
	return ResolveBillingServiceTier(requested, observed)
}

// ApplyOpenAIServiceTierBillingResolution lowers result.ServiceTier to the tier
// the upstream reports having used, so cost calculation and the usage log share
// one billable tier. The returned resolution is meant for the audit log.
func ApplyOpenAIServiceTierBillingResolution(account *Account, result *OpenAIForwardResult) ServiceTierBillingResolution {
	if result == nil {
		return ServiceTierBillingResolution{}
	}
	resolution := ResolveOpenAIServiceTierBilling(account, optionalStringValue(result.ServiceTier), result.UpstreamResponseServiceTier)
	if resolution.Downgraded {
		billing := resolution.Billing
		result.ServiceTier = &billing
	}
	return resolution
}

// ApplyForwardServiceTierBillingResolution is the ForwardResult counterpart of
// ApplyOpenAIServiceTierBillingResolution.
func ApplyForwardServiceTierBillingResolution(result *ForwardResult) ServiceTierBillingResolution {
	if result == nil {
		return ServiceTierBillingResolution{}
	}
	resolution := ResolveBillingServiceTier(optionalStringValue(result.ServiceTier), result.UpstreamResponseServiceTier)
	if resolution.Downgraded {
		billing := resolution.Billing
		result.ServiceTier = &billing
	}
	return resolution
}

// logServiceTierBillingDowngrade leaves an audit trail for every request billed
// below the tier it asked for; unchanged tiers are not logged.
func logServiceTierBillingDowngrade(component string, account *Account, requestID string, resolution ServiceTierBillingResolution) {
	if !resolution.Downgraded {
		return
	}
	attrs := []any{
		"component", component,
		"request_id", strings.TrimSpace(requestID),
		"requested_tier", resolution.Requested,
		"response_tier", resolution.Observed,
		"billed_tier", resolution.Billing,
	}
	if account != nil {
		attrs = append(attrs, "platform", account.Platform, "account_id", account.ID)
	}
	slog.Info("billing.service_tier_downgraded", attrs...)
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// anthropicSpeedModel returns the model used to decide Fast eligibility: the
// mapped upstream model when known, otherwise the requested model.
func anthropicSpeedModel(parsed *ParsedRequest, result *ForwardResult) string {
	if result != nil {
		if upstreamModel := strings.TrimSpace(result.UpstreamModel); upstreamModel != "" {
			return upstreamModel
		}
	}
	if parsed == nil {
		return ""
	}
	return parsed.Model
}

// anthropicSpeedServiceTier normalizes Anthropic speed=fast into the billable
// "fast" tier. Fast mode only exists on Claude Opus 5 / Opus 4.8 and is not
// offered on Bedrock, so other models and platforms are never billed at the
// fast premium. The upstream response's usage.speed can still lower it via
// ResolveBillingServiceTier.
func anthropicSpeedServiceTier(account *Account, speed, model string) *string {
	if account == nil || account.Platform != PlatformAnthropic || speed != "fast" {
		return nil
	}
	if account.IsBedrock() || !modelSupportsAnthropicFastMode(model) {
		return nil
	}
	tier := "fast"
	return &tier
}

// modelSupportsAnthropicFastMode reports whether the model belongs to Opus 5 / Opus 4.8.
func modelSupportsAnthropicFastMode(model string) bool {
	modelLower := strings.ToLower(strings.TrimSpace(model))
	if !strings.Contains(modelLower, "opus") {
		return false
	}
	// "opus-5" must be checked first; a bare "5" would match claude-opus-4-5.
	if strings.Contains(modelLower, "opus-5") || strings.Contains(modelLower, "opus5") {
		return true
	}
	return strings.Contains(modelLower, "4.8") || strings.Contains(modelLower, "4-8")
}

// normalizeObservedAnthropicSpeed maps Anthropic usage.speed onto the billing
// vocabulary: "fast" is the billable fast-mode tier, "standard" the base rate.
func normalizeObservedAnthropicSpeed(raw string) string {
	switch value := strings.ToLower(strings.TrimSpace(raw)); value {
	case "fast", "standard":
		return value
	default:
		return ""
	}
}

// normalizeObservedOpenAIServiceTier maps a tier reported by an OpenAI response
// onto the billing vocabulary. "auto" never describes a processing tier and
// unknown values are ignored rather than guessed at.
func normalizeObservedOpenAIServiceTier(raw string) string {
	switch value := strings.ToLower(strings.TrimSpace(raw)); value {
	case "priority", "fast":
		return "priority"
	case "default", "flex", "scale":
		return value
	default:
		return ""
	}
}

// observeAnthropicUsageSpeed returns usage.speed from an Anthropic response body
// or SSE frame (message_start carries it under message.usage, non-streaming
// bodies at the top level, message_delta under usage).
func observeAnthropicUsageSpeed(payload []byte) string { //nolint:unused // local-fork helper retained from earlier upstream syncs; re-wiring is out of scope for the lint fix
	if len(payload) == 0 || !strings.Contains(string(payload), `"speed"`) {
		return ""
	}
	for _, path := range []string{"message.usage.speed", "usage.speed"} {
		if v := gjson.GetBytes(payload, path); v.Exists() {
			if tier := normalizeObservedAnthropicSpeed(v.String()); tier != "" {
				return tier
			}
		}
	}
	return ""
}

// openAIResponseServiceTierObserver collects the processing tier an OpenAI
// upstream declares while a response streams by. It mirrors the tier half of
// upstream's upstreamResponseModelObserver (75faedda9); CCAI has no shared
// per-request observer, so each compat path owns one of these.
//
// Typed non-terminal Responses API events (response.created / in_progress)
// echo the requested tier and are ignored. Terminal events always win.
// Untyped payloads (Chat Completions chunks and bodies, non-streaming JSON) are
// trusted only while they agree with each other.
type openAIResponseServiceTierObserver struct {
	firstTier         string
	firstTierConflict bool
	terminalTier      string
}

// ObservePayload inspects one upstream event or body. eventType is the
// Responses API event type ("" for untyped payloads).
func (o *openAIResponseServiceTierObserver) ObservePayload(payload []byte, eventType string) {
	if o == nil || len(payload) == 0 || !strings.Contains(string(payload), `"service_tier"`) {
		return
	}
	eventType = strings.TrimSpace(eventType)
	terminal := isOpenAIServiceTierTerminalEvent(eventType)
	if !terminal && eventType != "" {
		return
	}
	o.observe(observeOpenAIResponseServiceTier(payload), terminal)
}

func (o *openAIResponseServiceTierObserver) observe(tier string, terminal bool) {
	if tier == "" {
		return
	}
	if terminal {
		o.terminalTier = tier
		return
	}
	if o.firstTier == "" {
		o.firstTier = tier
		return
	}
	if o.firstTier != tier {
		o.firstTierConflict = true
	}
}

// ServiceTier returns the declared tier, or "" when none was declared
// unambiguously.
func (o *openAIResponseServiceTierObserver) ServiceTier() string {
	if o == nil {
		return ""
	}
	if o.terminalTier != "" {
		return o.terminalTier
	}
	if o.firstTierConflict {
		return ""
	}
	return o.firstTier
}

func isOpenAIServiceTierTerminalEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}

// observeOpenAIResponseServiceTier returns the processing tier declared by an
// OpenAI response body or terminal event. Non-terminal Responses API events echo
// the requested tier and must not be passed here.
func observeOpenAIResponseServiceTier(payload []byte) string {
	if len(payload) == 0 || !strings.Contains(string(payload), `"service_tier"`) {
		return ""
	}
	for _, path := range []string{"response.service_tier", "service_tier"} {
		if v := gjson.GetBytes(payload, path); v.Exists() {
			if tier := normalizeObservedOpenAIServiceTier(v.String()); tier != "" {
				return tier
			}
		}
	}
	return ""
}
