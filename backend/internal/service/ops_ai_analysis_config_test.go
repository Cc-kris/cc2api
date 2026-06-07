package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type opsAIConfigEncryptorStub struct{}

func (opsAIConfigEncryptorStub) Encrypt(plaintext string) (string, error) {
	runes := []rune(plaintext)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return "enc:" + string(runes), nil
}

func (opsAIConfigEncryptorStub) Decrypt(ciphertext string) (string, error) {
	plain := []rune(strings.TrimPrefix(ciphertext, "enc:"))
	for i, j := 0, len(plain)-1; i < j; i, j = i+1, j-1 {
		plain[i], plain[j] = plain[j], plain[i]
	}
	return string(plain), nil
}

func validOpsAIConfigUpdate(apiKey string) *OpsAIAnalysisConfigUpdateRequest {
	return &OpsAIAnalysisConfigUpdateRequest{
		Enabled:                  true,
		BaseURL:                  " https://ai.example.com/v1 ",
		APIKey:                   apiKey,
		Model:                    " gpt-5.5 ",
		InterfaceType:            "responses",
		TimeoutSeconds:           60,
		MaxSamples:               50,
		AutoDedupMinutes:         10,
		GlobalRateLimitPerMinute: 10,
		AutoLevels:               []string{"P0", "P1", "P1"},
		ManualEnabled:            true,
	}
}

func TestOpsAIAnalysisConfig_DefaultReadPersistsMaskedSafeConfig(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{settingRepo: repo, secretEncryptor: opsAIConfigEncryptorStub{}}

	cfg, err := svc.GetOpsAIAnalysisConfig(context.Background())
	if err != nil {
		t.Fatalf("GetOpsAIAnalysisConfig() error = %v", err)
	}
	if cfg.Enabled {
		t.Fatalf("Enabled = true, want false by default")
	}
	if cfg.APIKeyMasked != "" || cfg.APIKeyEncrypted != "" {
		t.Fatalf("default config leaked key fields: %+v", cfg)
	}
	if cfg.InterfaceType != "responses" || cfg.TimeoutSeconds != 60 || cfg.MaxSamples != 50 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if repo.setCalls != 1 {
		t.Fatalf("expected defaults to be persisted once, got %d", repo.setCalls)
	}
}

func TestOpsAIAnalysisConfig_EncryptsMasksAndPreservesAPIKey(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{settingRepo: repo, secretEncryptor: opsAIConfigEncryptorStub{}}

	updated, err := svc.UpdateOpsAIAnalysisConfig(context.Background(), validOpsAIConfigUpdate("sk-live-secret"))
	if err != nil {
		t.Fatalf("UpdateOpsAIAnalysisConfig() error = %v", err)
	}
	if updated.APIKeyMasked != "****cret" {
		t.Fatalf("APIKeyMasked = %q, want tail mask", updated.APIKeyMasked)
	}
	if updated.APIKeyEncrypted != "" {
		t.Fatalf("APIKeyEncrypted leaked in response: %q", updated.APIKeyEncrypted)
	}
	if updated.BaseURL != "https://ai.example.com/v1" || updated.Model != "gpt-5.5" {
		t.Fatalf("expected normalized fields, got %+v", updated)
	}
	if got := strings.Join(updated.AutoLevels, ","); got != "P0,P1" {
		t.Fatalf("AutoLevels = %q, want deduplicated P0,P1", got)
	}

	raw := repo.values[SettingKeyOpsAIAnalysisConfig]
	if strings.Contains(raw, "sk-live-secret") || strings.Contains(raw, "api_key_masked") {
		t.Fatalf("stored config leaked plaintext or response-only field: %s", raw)
	}
	var stored map[string]any
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("stored config invalid JSON: %v", err)
	}
	if stored["api_key_encrypted"] != "enc:terces-evil-ks" {
		t.Fatalf("api_key_encrypted = %v", stored["api_key_encrypted"])
	}

	preserve := validOpsAIConfigUpdate("")
	preserve.Model = "gpt-5.6"
	preserved, err := svc.UpdateOpsAIAnalysisConfig(context.Background(), preserve)
	if err != nil {
		t.Fatalf("UpdateOpsAIAnalysisConfig() preserve error = %v", err)
	}
	if preserved.APIKeyMasked != "****cret" {
		t.Fatalf("preserved APIKeyMasked = %q", preserved.APIKeyMasked)
	}
	if !strings.Contains(repo.values[SettingKeyOpsAIAnalysisConfig], "enc:terces-evil-ks") {
		t.Fatalf("empty api_key update did not preserve existing encrypted key: %s", repo.values[SettingKeyOpsAIAnalysisConfig])
	}
}

func TestOpsAIAnalysisConfig_ReplacesAPIKeyAndValidatesBounds(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{settingRepo: repo, secretEncryptor: opsAIConfigEncryptorStub{}}

	if _, err := svc.UpdateOpsAIAnalysisConfig(context.Background(), validOpsAIConfigUpdate("old-secret")); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if _, err := svc.UpdateOpsAIAnalysisConfig(context.Background(), validOpsAIConfigUpdate("new-secret")); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	if raw := repo.values[SettingKeyOpsAIAnalysisConfig]; !strings.Contains(raw, "enc:terces-wen") || strings.Contains(raw, "enc:terces-dlo") {
		t.Fatalf("new api_key did not replace old encrypted key: %s", raw)
	}

	cases := []struct {
		name string
		mut  func(*OpsAIAnalysisConfigUpdateRequest)
	}{
		{name: "invalid_url", mut: func(r *OpsAIAnalysisConfigUpdateRequest) { r.BaseURL = "ftp://ai.example.com" }},
		{name: "missing_key_when_enabled", mut: func(r *OpsAIAnalysisConfigUpdateRequest) {
			r.APIKey = ""
			delete(repo.values, SettingKeyOpsAIAnalysisConfig)
		}},
		{name: "invalid_interface", mut: func(r *OpsAIAnalysisConfigUpdateRequest) { r.InterfaceType = "legacy" }},
		{name: "base_url_too_long", mut: func(r *OpsAIAnalysisConfigUpdateRequest) {
			r.BaseURL = "https://" + strings.Repeat("a", 489) + ".com"
		}},
		{name: "model_too_long", mut: func(r *OpsAIAnalysisConfigUpdateRequest) { r.Model = strings.Repeat("m", 101) }},
		{name: "timeout_low", mut: func(r *OpsAIAnalysisConfigUpdateRequest) { r.TimeoutSeconds = 4 }},
		{name: "samples_high", mut: func(r *OpsAIAnalysisConfigUpdateRequest) { r.MaxSamples = 501 }},
		{name: "dedup_zero", mut: func(r *OpsAIAnalysisConfigUpdateRequest) { r.AutoDedupMinutes = 0 }},
		{name: "rate_high", mut: func(r *OpsAIAnalysisConfigUpdateRequest) { r.GlobalRateLimitPerMinute = 1001 }},
		{name: "level_invalid", mut: func(r *OpsAIAnalysisConfigUpdateRequest) { r.AutoLevels = []string{"P9"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validOpsAIConfigUpdate("sk-test")
			tc.mut(req)
			if _, err := svc.UpdateOpsAIAnalysisConfig(context.Background(), req); err == nil {
				t.Fatalf("UpdateOpsAIAnalysisConfig() error = nil, want validation error")
			}
		})
	}
}
