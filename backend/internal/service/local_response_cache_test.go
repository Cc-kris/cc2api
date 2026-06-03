package service

import (
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
