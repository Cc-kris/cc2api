package service

// 移植自上游 8e92c892b 的 service/openai_model_retirement.go（26aab6dc0、86f844304，V02-T18）。

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// GPT-5.5 retired from Codex with ChatGPT sign-in on 2026-10-14 at
// 19:00 UTC. API-key upstreams remain supported. Do not infer upstream
// eligibility from the API key a client uses to authenticate to this gateway.
// https://learn.chatgpt.com/docs/models#gpt-55-retirement
//
// CCAI 没有影子账号，去掉了上游的 IsShadow 分支（V02-T18）。
func isRetiredOpenAISubscriptionModel(account *Account, upstreamModel string) bool {
	if account == nil || !account.IsOpenAIOAuthLike() {
		return false
	}
	model := canonicalizeOpenAIModelAliasSpelling(upstreamModel)
	return model == "gpt-5.5" || strings.HasPrefix(model, "gpt-5.5-")
}

// Only remove entries blocked by retirement. Unknown models and other platforms
// retain their existing listing policy. An explicitly mapped replacement is
// allowed, but a public alias targeting retired GPT-5.5 is not.
func openAIModelRetiredForAccounts(accounts []Account, modelID string) bool {
	retired := false
	for i := range accounts {
		account := &accounts[i]
		if !account.IsOpenAI() {
			continue
		}
		target := resolveOpenAIAccountUpstreamModelForRequest(account, modelID, false)
		if isRetiredOpenAISubscriptionModel(account, target) {
			retired = true
		} else if account.IsModelSupported(modelID) {
			return false
		}
	}
	return retired
}

func filterRetiredOpenAIModelIDs(accounts []Account, models []string) []string {
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		if !openAIModelRetiredForAccounts(accounts, model) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

// Entries must already use client-visible names, not raw upstream IDs. Resolve
// each public name exactly as routing does, after projection and before the
// final client ETag is calculated. Preserve unknown fields.
func filterRetiredOpenAIPublicModelsBody(body []byte, accounts []Account, codex bool) ([]byte, bool, error) {
	field, idField := "data", "id"
	if codex {
		field, idField = "models", "slug"
	}
	envelope, entries, err := modelCatalogEntries(body, field)
	if err != nil {
		return nil, false, err
	}
	filtered := make([]json.RawMessage, 0, len(entries))
	for _, raw := range entries {
		var entry map[string]json.RawMessage
		var id string
		if json.Unmarshal(raw, &entry) == nil {
			_ = json.Unmarshal(entry[idField], &id)
		}
		if !openAIModelRetiredForAccounts(accounts, id) {
			filtered = append(filtered, raw)
		}
	}
	if len(filtered) == len(entries) {
		return body, false, nil
	}
	envelope[field], err = json.Marshal(filtered)
	if err != nil {
		return nil, false, err
	}
	encoded, err := json.Marshal(envelope)
	return encoded, true, err
}

// Compact-only administrator mappings are evaluated against their actual
// target, without treating an old client-visible name as a retired target.
func supportsOpenAIRequestModel(account *Account, requestedModel string, requireCompact bool) bool {
	if account == nil {
		return false
	}
	if account.IsOpenAIOAuthLike() &&
		isRetiredOpenAISubscriptionModel(account, resolveOpenAIAccountUpstreamModelForRequest(account, requestedModel, requireCompact)) {
		return false
	}
	return account.isModelMappingSupported(requestedModel)
}

// modelCatalogEntries 拆出模型目录的外层对象和条目数组（上游 openai_models_list.go:84，
// CCAI 没有那个文件，单独拷过来供 Codex 清单过滤用）。
func modelCatalogEntries(body []byte, field string) (map[string]json.RawMessage, []json.RawMessage, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, nil, err
	}
	raw := bytes.TrimSpace(envelope[field])
	if len(raw) == 0 || raw[0] != '[' {
		return nil, nil, fmt.Errorf("missing or invalid %s array", field)
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, nil, err
	}
	return envelope, entries, nil
}

// FilterRetiredCodexModelsManifest 从 Codex 模型清单里去掉对该分组已退役的 slug（V02-T18）。
// CCAI 的清单是单个账号上游 manifest 原样回给客户端，没有上游的分组目录合并和公开别名
// 投影层，所以在这里按分组账号过滤 models[].slug。changed=false 时 body 原样返回。
// 读账号失败时不过滤（返回原 body），不让清单接口因此失败。
func (s *OpenAIGatewayService) FilterRetiredCodexModelsManifest(ctx context.Context, groupID *int64, body []byte) ([]byte, bool) {
	if s == nil || len(body) == 0 {
		return body, false
	}
	accounts, err := s.listSchedulableAccounts(ctx, groupID)
	if err != nil || len(accounts) == 0 {
		return body, false
	}
	filtered, changed, err := filterRetiredOpenAIPublicModelsBody(body, accounts, true)
	if err != nil || !changed {
		return body, false
	}
	return filtered, true
}

// CodexModelsManifestETag 给过滤后的清单算一个强 ETag。过滤改了内容就不能再用上游 ETag，
// 否则客户端拿上游 ETag 发 If-None-Match 会得到 304，继续用缓存里含退役模型的旧列表。
func CodexModelsManifestETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"sub2api-` + hex.EncodeToString(sum[:16]) + `"`
}
