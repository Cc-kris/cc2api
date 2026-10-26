package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func TestContentAIConfigStoreV2EncryptsMultipleConfigs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewContentAIConfigStore(path, &contentAIFileEncryptor{key: []byte("0123456789abcdef0123456789abcdef")})
	want := []ContentAIProviderConfig{
		{ID: "a", Label: "OpenAI primary", Provider: "openai", BaseURL: "https://one.test/v1", APIKey: "sk-secret-one", DefaultModel: "gpt-test", Models: []string{"gpt-test"}},
		{ID: "b", Label: "OpenAI backup", Provider: "openai", BaseURL: "https://two.test/v1", APIKey: "sk-secret-two", DefaultModel: "gpt-backup", Models: []string{"gpt-backup"}},
	}
	if err := store.SaveAll(want, "b"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-secret") {
		t.Fatal("file contains plaintext API key")
	}
	var stored struct {
		Version   int    `json:"version"`
		DefaultID string `json:"defaultConfigId"`
		Configs   []struct {
			ID           string `json:"id"`
			EncryptedKey string `json:"apiKeyEncrypted"`
		} `json:"configurations"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Version != 2 || stored.DefaultID != "b" || len(stored.Configs) != 2 || stored.Configs[0].ID != "a" || stored.Configs[0].EncryptedKey == "" {
		t.Fatalf("unexpected v2 JSON fields: %+v", stored)
	}
	got, def, err := store.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if def != "b" || len(got) != 2 || got[0].APIKey != want[0].APIKey || got[1].APIKey != want[1].APIKey {
		t.Fatalf("round trip mismatch: default=%q configs=%+v", def, got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
}

func TestContentAIConfigStoreMigratesLegacyOnceAndRequiresEncryption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := []byte(`{"baseUrl":"https://legacy.test/v1","apiKey":"sk-legacy-secret","defaultModel":"legacy-model","models":["legacy-model"]}`)
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewContentAIConfigStore(path, nil).LoadAll(); !errors.Is(err, ErrContentAIEncryptionNotConfigured) || infraerrors.Code(err) != 503 || infraerrors.Reason(err) != "encryption_not_configured" {
		t.Fatalf("missing encryptor error = %v", err)
	}
	got, def, err := NewContentAIConfigStore(path, &contentAIFileEncryptor{key: []byte("0123456789abcdef0123456789abcdef")}).LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if def != "legacy-default" || len(got) != 1 || got[0].APIKey != "sk-legacy-secret" || got[0].BaseURL != "https://legacy.test/v1" {
		t.Fatalf("legacy data lost: default=%q configs=%+v", def, got)
	}
	backup, err := os.ReadFile(path + ".v1.bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(legacy) {
		t.Fatal("legacy backup differs from source")
	}
	again, againDefault, err := NewContentAIConfigStore(path, &contentAIFileEncryptor{key: []byte("0123456789abcdef0123456789abcdef")}).LoadAll()
	if err != nil || againDefault != def || len(again) != 1 || again[0].APIKey != got[0].APIKey {
		t.Fatalf("repeat load: default=%q configs=%+v err=%v", againDefault, again, err)
	}
}

func TestContentAIConfigStoreAtomicFailurePreservesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewContentAIConfigStore(path, &contentAIFileEncryptor{key: []byte("0123456789abcdef0123456789abcdef")})
	original := []ContentAIProviderConfig{{ID: "stable", Label: "Stable", BaseURL: "https://stable.test/v1"}}
	if err := store.SaveAll(original, "stable"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store.write = func(f *os.File, b []byte) error {
		_, _ = f.Write(b[:len(b)/2])
		return errors.New("simulated interrupted write")
	}
	if err := store.SaveAll([]ContentAIProviderConfig{{ID: "changed", Label: "Changed"}}, "changed"); err == nil {
		t.Fatal("expected simulated write failure")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("failed write replaced original file")
	}
	if _, _, err := NewContentAIConfigStore(path, store.encryptor).LoadAll(); err != nil {
		t.Fatalf("original file no longer readable: %v", err)
	}
}

func TestContentAIConfigStoreRejectsInvalidV2Identity(t *testing.T) {
	cases := []struct {
		name, defaultID string
		configs         []contentAIStored
		wantErr         bool
	}{
		{name: "empty id", defaultID: "", configs: []contentAIStored{{ID: ""}}, wantErr: true},
		{name: "whitespace id", defaultID: " ", configs: []contentAIStored{{ID: " "}}, wantErr: true},
		{name: "duplicate ids", defaultID: "same", configs: []contentAIStored{{ID: "same"}, {ID: "same"}}, wantErr: true},
		{name: "empty configuration list with empty default", defaultID: "", configs: []contentAIStored{}, wantErr: false},
		{name: "missing default", defaultID: "missing", configs: []contentAIStored{{ID: "present"}}, wantErr: true},
		{name: "empty default", defaultID: "", configs: []contentAIStored{{ID: "present"}}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			raw, err := json.Marshal(contentAIFile{Version: 2, DefaultID: tc.defaultID, Configs: tc.configs})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			configs, defaultID, err := NewContentAIConfigStore(path, nil).LoadAll()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected invalid v2 identity to be rejected")
				}
				return
			}
			if err != nil || len(configs) != 0 || defaultID != "" {
				t.Fatalf("empty configuration set: configs=%v default=%q err=%v", configs, defaultID, err)
			}
		})
	}
}

func TestContentAIConfigMissingEncryptionPropagatesAsServiceUnavailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewContentAIConfigStore(path, &contentAIFileEncryptor{key: []byte("0123456789abcdef0123456789abcdef")})
	if err := store.SaveAll([]ContentAIProviderConfig{{ID: "secure", APIKey: "sk-encrypted-key", BaseURL: "https://ai.test/v1"}}, "secure"); err != nil {
		t.Fatal(err)
	}
	originalPath := contentAIConfigPath
	contentAIConfigPath = path
	t.Cleanup(func() { contentAIConfigPath = originalPath })
	t.Setenv("TOTP_ENCRYPTION_KEY", "")

	var err error
	_, err = loadContentAIProviderConfigWithError()
	if !errors.Is(err, ErrContentAIEncryptionNotConfigured) || infraerrors.Code(err) != 503 {
		t.Fatalf("loader error = %v, want encryption-not-configured 503", err)
	}
	_, err = (&ContentArticleAdminService{}).GetAIProviderConfig(context.Background())
	if !errors.Is(err, ErrContentAIEncryptionNotConfigured) || infraerrors.Code(err) != 503 {
		t.Fatalf("GetAIProviderConfig error = %v, want encryption-not-configured 503", err)
	}
	_, _, err = (&ContentArticleAdminService{}).FetchAIProviderModels(context.Background(), "", "")
	if !errors.Is(err, ErrContentAIEncryptionNotConfigured) || infraerrors.Code(err) != 503 {
		t.Fatalf("FetchAIProviderModels error = %v, want encryption-not-configured 503", err)
	}
	_, err = (&GatewayContentWritingExecutor{}).Generate(context.Background(), &ContentWritingTask{Model: "model"})
	if !errors.Is(err, ErrContentAIEncryptionNotConfigured) || infraerrors.Code(err) != 503 {
		t.Fatalf("Generate error = %v, want encryption-not-configured 503", err)
	}
}

func TestContentAIConfigStoreSyncsParentDirectoryAfterRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	store := NewContentAIConfigStore(path, nil)
	called := false
	store.syncDir = func(got string) error {
		called = true
		if got != dir {
			t.Fatalf("synced directory = %q, want %q", got, dir)
		}
		return nil
	}
	if err := store.SaveAll([]ContentAIProviderConfig{}, ""); err != nil {
		t.Fatal(err)
	}
	if configs, defaultID, err := store.LoadAll(); err != nil || len(configs) != 0 || defaultID != "" {
		t.Fatalf("empty configuration round trip: configs=%v default=%q err=%v", configs, defaultID, err)
	}
	called = false
	if err := store.SaveAll([]ContentAIProviderConfig{{ID: "default"}}, "default"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("parent directory was not synced after rename")
	}
}

func TestContentAIConfigStoreRejectsCorruptJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewContentAIConfigStore(path, nil).LoadAll(); err == nil {
		t.Fatal("expected corrupt JSON error")
	}
}
