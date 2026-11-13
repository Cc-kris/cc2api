package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// OpenAICompactionContext is the request-scoped protocol decision shared by
// HTTP, websocket and account-test forwarding paths.
type OpenAICompactionContext struct {
	RemoteCompactionV2 bool
	CompactionTrigger  string
	NativeResponses    bool
	Compact            bool
	SessionBeta        string
}

type openAICompactionContextKey struct{}

func ParseOpenAICompactionContext(body []byte, headers http.Header, path string) OpenAICompactionContext {
	ctx := OpenAICompactionContext{SessionBeta: "responses=experimental"}
	ctx.RemoteCompactionV2 = remoteCompactionV2FromBody(body)
	ctx.CompactionTrigger = compactionTriggerFromBody(body)
	ctx.Compact = strings.HasSuffix(strings.TrimRight(strings.TrimSpace(path), "/"), "/responses/compact")
	if ctx.CompactionTrigger != "" {
		ctx.Compact = true
	}
	for _, raw := range headers.Values("x-codex-beta-features") {
		for _, feature := range strings.Split(raw, ",") {
			feature = strings.TrimSpace(feature)
			name, _, _ := strings.Cut(feature, "=")
			if strings.EqualFold(strings.TrimSpace(name), "remote_compaction_v2") {
				ctx.RemoteCompactionV2 = true
			}
		}
	}
	// A trigger on the streaming Responses wire is sufficient evidence of v2,
	// even when an intermediate gateway stripped the client's beta header.
	legacy := strings.HasSuffix(strings.TrimRight(strings.TrimSpace(path), "/"), "/responses/compact")
	ctx.NativeResponses = !legacy && ctx.CompactionTrigger != "" && (ctx.RemoteCompactionV2 || gjson.GetBytes(body, "stream").Bool())
	if ctx.NativeResponses {
		ctx.RemoteCompactionV2 = true
	}
	if ctx.NativeResponses {
		ctx.SessionBeta = "responses=experimental,remote_compaction_v2"
	}
	return ctx
}

// ParseOpenAIWSCompactionContext treats a WebSocket response.create as a
// streaming wire even when the client omits the redundant stream field.
func ParseOpenAIWSCompactionContext(body []byte, headers http.Header) OpenAICompactionContext {
	ctx := ParseOpenAICompactionContext(body, headers, "/v1/responses")
	if ctx.CompactionTrigger != "" {
		ctx.NativeResponses = true
		ctx.RemoteCompactionV2 = true
		ctx.SessionBeta = "responses=experimental,remote_compaction_v2"
	}
	return ctx
}

func remoteCompactionV2FromBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var envelope struct {
		RemoteCompactionV2 bool `json:"remote_compaction_v2"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}
	return envelope.RemoteCompactionV2
}

func WithOpenAICompactionContext(ctx context.Context, value OpenAICompactionContext) context.Context {
	return context.WithValue(ctx, openAICompactionContextKey{}, value)
}

func OpenAICompactionContextFrom(ctx context.Context) (OpenAICompactionContext, bool) {
	if ctx == nil {
		return OpenAICompactionContext{}, false
	}
	value, ok := ctx.Value(openAICompactionContextKey{}).(OpenAICompactionContext)
	return value, ok
}

func compactionTriggerFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var trigger string
	forEachInputItem(body, func(item map[string]any) bool {
		if typ, _ := item["type"].(string); typ == "compaction_trigger" {
			trigger, _ = item["trigger"].(string)
			if trigger == "" {
				trigger = typ
			}
			return false
		}
		return true
	})
	return strings.TrimSpace(trigger)
}

type openAIForwardModelContextKey struct{}

type openAIForwardModel struct {
	model                  string
	useCompactModelMapping bool
}

// WithOpenAIForwardModel records the model present in the forwarded request
// body after channel mapping and whether the legacy /responses/compact-only
// model mapping applies. Native remote compaction v2 keeps this false, so
// channel restriction checks follow the same model chain used by Forward.
func WithOpenAIForwardModel(ctx context.Context, forwardModel string, useCompactModelMapping bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIForwardModelContextKey{}, openAIForwardModel{
		model:                  forwardModel,
		useCompactModelMapping: useCompactModelMapping,
	})
}

func openAIForwardModelFromContext(ctx context.Context) (openAIForwardModel, bool) {
	if ctx == nil {
		return openAIForwardModel{}, false
	}
	forwardModel, ok := ctx.Value(openAIForwardModelContextKey{}).(openAIForwardModel)
	return forwardModel, ok
}
