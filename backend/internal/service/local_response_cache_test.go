package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildLocalResponseCacheLookup_GroupIsolation(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupA := int64(1)
	groupB := int64(2)
	body := []byte(`{"model":"gpt-5.5","input":"hello","temperature":0.1}`)

	a := BuildLocalResponseCacheLookup(cfg, 10, &groupA, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false)
	b := BuildLocalResponseCacheLookup(cfg, 10, &groupB, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false)

	require.NotEmpty(t, a.Key)
	require.NotEmpty(t, b.Key)
	require.NotEqual(t, a.Key, b.Key)
	require.True(t, strings.HasPrefix(a.Key, "cache:v2:openai:10:1:/v1/responses:gpt-5.5:"))
	require.NotEmpty(t, a.LegacyKey)
}

func TestBuildLocalResponseCacheLookup_V2Isolation(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupID := int64(1)
	body := []byte(`{"model":"gpt-5.5","input":"hello","temperature":0.1}`)

	base := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false)
	otherAPIKey := BuildLocalResponseCacheLookup(cfg, 11, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false)
	otherModel := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.6", body, false)
	otherPlatform := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformAnthropic, "gpt-5.5", body, false)

	require.NotEmpty(t, base.Key)
	require.NotEqual(t, base.Key, otherAPIKey.Key)
	require.NotEqual(t, base.Key, otherModel.Key)
	require.NotEqual(t, base.Key, otherPlatform.Key)
	require.Contains(t, base.Key, ":10:1:/v1/responses:gpt-5.5:")
}

func TestBuildLocalResponseCacheLookup_HeaderAffectsV2RequestHash(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupID := int64(1)
	body := []byte(`{"model":"gpt-5.5","input":"hello"}`)

	jsonLookup := BuildLocalResponseCacheLookupWithOptions(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false, LocalResponseCacheKeyOptions{
		Headers: map[string]string{"content-type": "application/json"},
	})
	sseLookup := BuildLocalResponseCacheLookupWithOptions(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", body, false, LocalResponseCacheKeyOptions{
		Headers: map[string]string{"content-type": "text/event-stream"},
	})

	require.NotEmpty(t, jsonLookup.Key)
	require.NotEmpty(t, sseLookup.Key)
	require.NotEqual(t, jsonLookup.Key, sseLookup.Key)
	require.Equal(t, jsonLookup.LegacyKey, sseLookup.LegacyKey)

	claudeBetaLookup := BuildLocalResponseCacheLookupWithOptions(cfg, 10, &groupID, "/v1/messages", PlatformAnthropic, "claude-sonnet-4.6", body, false, LocalResponseCacheKeyOptions{
		Headers: map[string]string{"anthropic-beta": "interleaved-thinking-2025-05-14"},
	})
	claudeNoBetaLookup := BuildLocalResponseCacheLookupWithOptions(cfg, 10, &groupID, "/v1/messages", PlatformAnthropic, "claude-sonnet-4.6", body, false, LocalResponseCacheKeyOptions{
		Headers: map[string]string{"anthropic-version": "2023-06-01"},
	})
	require.NotEmpty(t, claudeBetaLookup.Key)
	require.NotEmpty(t, claudeNoBetaLookup.Key)
	require.NotEqual(t, claudeBetaLookup.Key, claudeNoBetaLookup.Key)
}

func TestBuildLocalResponseCacheLookup_CanonicalJSON(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupID := int64(1)

	a := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", []byte(`{"input":"hello","model":"gpt-5.5"}`), false)
	b := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", []byte(`{"model":"gpt-5.5","input":"hello"}`), false)

	require.NotEmpty(t, a.Key)
	require.Equal(t, a.Key, b.Key)
}

func TestBuildLocalResponseCacheLookup_BypassRules(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	cfg.Enabled = true
	groupID := int64(1)

	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "tool", body: `{"model":"gpt-5.5","tools":[]}`, want: "tools_or_functions"},
		{name: "claude tool_use", body: `{"model":"claude-sonnet-4.6","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read","input":{}}]}]}`, want: "tools_or_functions"},
		{name: "claude thinking", body: `{"model":"claude-sonnet-4.6","thinking":{"type":"enabled"},"messages":[{"role":"user","content":"hi"}]}`, want: "tools_or_functions"},
		{name: "claude image", body: `{"model":"claude-sonnet-4.6","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc"}}]}]}`, want: "tools_or_functions"},
		{name: "claude document", body: `{"model":"claude-sonnet-4.6","messages":[{"role":"user","content":[{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"abc"}}]}]}`, want: "tools_or_functions"},
		{name: "temperature", body: `{"model":"gpt-5.5","temperature":0.9}`, want: "temperature_too_high"},
		{name: "sensitive", body: `{"model":"gpt-5.5","input":"password=abc"}`, want: "sensitive_content"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", []byte(tc.body), false)
			require.Empty(t, lookup.Key)
			require.Equal(t, tc.want, lookup.Reason)
		})
	}
}

func TestBuildLocalResponseCacheLookup_Disabled(t *testing.T) {
	cfg := DefaultLocalResponseCacheConfig()
	groupID := int64(1)
	lookup := BuildLocalResponseCacheLookup(cfg, 10, &groupID, "/v1/responses", PlatformOpenAI, "gpt-5.5", []byte(`{"model":"gpt-5.5"}`), false)
	require.Empty(t, lookup.Key)
	require.Equal(t, "disabled", lookup.Reason)
}
