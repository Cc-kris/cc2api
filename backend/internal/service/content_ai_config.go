package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrContentAIEncryptionNotConfigured = infraerrors.ServiceUnavailable("encryption_not_configured", "AI config encryption is not configured")

type contentAIFile struct {
	Version   int               `json:"version"`
	DefaultID string            `json:"defaultConfigId"`
	Configs   []contentAIStored `json:"configurations"`
}
type contentAIStored struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Provider string   `json:"provider"`
	BaseURL  string   `json:"baseUrl"`
	Key      string   `json:"apiKeyEncrypted,omitempty"`
	Model    string   `json:"defaultModel"`
	Models   []string `json:"models"`
	Updated  string   `json:"updatedAt,omitempty"`
}
type ContentAIConfigStore struct {
	mu        sync.Mutex
	path      string
	encryptor SecretEncryptor
	write     func(*os.File, []byte) error
	syncDir   func(string) error
}
type contentAIFileEncryptor struct{ key []byte }

func NewContentAIConfigStore(path string, e SecretEncryptor) *ContentAIConfigStore {
	return &ContentAIConfigStore{path: path, encryptor: e, write: func(f *os.File, b []byte) error {
		if _, e := f.Write(b); e != nil {
			return e
		}
		return f.Sync()
	}, syncDir: syncContentAIConfigDir}
}
func syncContentAIConfigDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
func contentAIEncryptorFromEnv() SecretEncryptor {
	k, e := hex.DecodeString(strings.TrimSpace(os.Getenv("TOTP_ENCRYPTION_KEY")))
	if e != nil || len(k) != 32 {
		return nil
	}
	return &contentAIFileEncryptor{k}
}
func (e *contentAIFileEncryptor) Encrypt(s string) (string, error) {
	b, x := aes.NewCipher(e.key)
	if x != nil {
		return "", x
	}
	g, x := cipher.NewGCM(b)
	if x != nil {
		return "", x
	}
	n := make([]byte, g.NonceSize())
	if _, x = io.ReadFull(rand.Reader, n); x != nil {
		return "", x
	}
	return base64.StdEncoding.EncodeToString(g.Seal(n, n, []byte(s), nil)), nil
}
func (e *contentAIFileEncryptor) Decrypt(s string) (string, error) {
	b, x := base64.StdEncoding.DecodeString(s)
	if x != nil {
		return "", x
	}
	c, x := aes.NewCipher(e.key)
	if x != nil {
		return "", x
	}
	g, x := cipher.NewGCM(c)
	if x != nil {
		return "", x
	}
	if len(b) < g.NonceSize() {
		return "", errors.New("invalid encrypted key")
	}
	p, x := g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], nil)
	return string(p), x
}
func defaultContentAIConfig() ContentAIProviderConfig {
	return ContentAIProviderConfig{ID: "default", Label: "Default", Provider: "custom", BaseURL: "https://cc-ai.xyz/v1", DefaultModel: "claude-opus-5.5", Models: []string{"claude-opus-5.5", "claude-sonnet-4.6", "claude-haiku-4.5", "gpt-5.4", "gpt-5.3-codex", "grok-4.7", "deepseek-reasoner", "deepseek-chat"}}
}
func (s *ContentAIConfigStore) LoadAll() ([]ContentAIProviderConfig, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}
func (s *ContentAIConfigStore) load() ([]ContentAIProviderConfig, string, error) {
	raw, e := os.ReadFile(s.path)
	if errors.Is(e, os.ErrNotExist) {
		d := defaultContentAIConfig()
		return []ContentAIProviderConfig{d}, d.ID, nil
	}
	if e != nil {
		return nil, "", e
	}
	var f contentAIFile
	if e = json.Unmarshal(raw, &f); e != nil {
		return nil, "", fmt.Errorf("invalid AI config JSON: %w", e)
	}
	if f.Version == 0 {
		var c ContentAIProviderConfig
		if e = json.Unmarshal(raw, &c); e != nil || c.BaseURL == "" && c.APIKey == "" && c.DefaultModel == "" && c.Models == nil {
			return nil, "", errors.New("invalid legacy AI config")
		}
		c.ID = "legacy-default"
		c.Label = "Legacy"
		if c.Provider == "" {
			c.Provider = "custom"
		}
		if c.BaseURL == "" {
			c.BaseURL = defaultContentAIConfig().BaseURL
		}
		if c.APIKey != "" && s.encryptor == nil {
			return nil, "", ErrContentAIEncryptionNotConfigured
		}
		if e = s.backup(raw); e != nil {
			return nil, "", e
		}
		if e = s.save([]ContentAIProviderConfig{c}, c.ID); e != nil {
			return nil, "", e
		}
		return []ContentAIProviderConfig{c}, c.ID, nil
	}
	if f.Version != 2 {
		return nil, "", fmt.Errorf("unsupported AI config version %d", f.Version)
	}
	seenIDs := make(map[string]struct{}, len(f.Configs))
	out := make([]ContentAIProviderConfig, 0, len(f.Configs))
	for _, c := range f.Configs {
		if strings.TrimSpace(c.ID) == "" {
			return nil, "", errors.New("AI config v2 contains an empty configuration id")
		}
		if _, exists := seenIDs[c.ID]; exists {
			return nil, "", errors.New("AI config v2 contains duplicate configuration ids")
		}
		seenIDs[c.ID] = struct{}{}
		x := ContentAIProviderConfig{ID: c.ID, Label: c.Label, Provider: c.Provider, BaseURL: c.BaseURL, DefaultModel: c.Model, Models: c.Models, UpdatedAt: c.Updated}
		// 原密文始终随配置带上（json:"-" 不对外暴露），保存时未填新 Key 就复用它，
		// 否则“只改名称”会把已存的 Key 抹掉。
		x.EncryptedKey = c.Key
		if c.Key != "" {
			if s.encryptor == nil {
				return nil, "", ErrContentAIEncryptionNotConfigured
			}
			// 单条解密失败不能拖垮整个列表：换过加密钥/手工改过配置后，旧密文解不开，
			// 以前这里直接 500，管理端就表现为“保存失败、一个都删不掉”。
			// 现在降级为“Key 不可读”，用户可重填 Key 或直接删除该配置。
			if x.APIKey, e = s.encryptor.Decrypt(c.Key); e != nil {
				x.APIKey = ""
				x.EncryptedKey = c.Key
				x.KeyUnreadable = true
			}
		}
		x.HasAPIKey = x.APIKey != ""
		x.APIKeyMasked = maskAPIKey(x.APIKey)
		out = append(out, x)
	}
	if len(out) == 0 {
		if strings.TrimSpace(f.DefaultID) != "" {
			return nil, "", errors.New("AI config v2 default configuration does not exist")
		}
		return []ContentAIProviderConfig{}, "", nil
	}
	if _, exists := seenIDs[f.DefaultID]; strings.TrimSpace(f.DefaultID) == "" || !exists {
		return nil, "", errors.New("AI config v2 default configuration does not exist")
	}
	return out, f.DefaultID, nil
}
func (s *ContentAIConfigStore) SaveAll(c []ContentAIProviderConfig, d string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(c, d)
}
func (s *ContentAIConfigStore) save(configs []ContentAIProviderConfig, def string) error {
	if len(configs) == 0 && def != "" {
		return errors.New("default AI config not found")
	}
	f := contentAIFile{Version: 2, DefaultID: def}
	seen := map[string]bool{}
	for _, c := range configs {
		if strings.TrimSpace(c.ID) == "" {
			return errors.New("AI config id must not be empty")
		}
		if seen[c.ID] {
			return fmt.Errorf("duplicate AI config id %q", c.ID)
		}
		seen[c.ID] = true
		k := strings.TrimSpace(c.APIKey)
		if k != "" {
			if s.encryptor == nil {
				return ErrContentAIEncryptionNotConfigured
			}
			var e error
			c.EncryptedKey, e = s.encryptor.Encrypt(k)
			if e != nil {
				return e
			}
		} else if strings.TrimSpace(c.EncryptedKey) != "" {
			// 未填新 Key 且已有密文：原样保留，避免解密降级后保存把 Key 抹掉。
			k = ""
		}
		f.Configs = append(f.Configs, contentAIStored{c.ID, c.Label, c.Provider, c.BaseURL, c.EncryptedKey, c.DefaultModel, c.Models, c.UpdatedAt})
	}
	if len(f.Configs) == 0 {
		if strings.TrimSpace(f.DefaultID) != "" {
			return errors.New("default AI config not found")
		}
	} else if strings.TrimSpace(f.DefaultID) == "" || !seen[f.DefaultID] {
		return errors.New("default AI config not found")
	}
	b, e := json.MarshalIndent(f, "", "  ")
	if e != nil {
		return e
	}
	dir := filepath.Dir(s.path)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	tmp, e := os.CreateTemp(dir, ".content-ai-*.tmp")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(0600); e == nil {
		e = s.write(tmp, b)
	}
	if x := tmp.Close(); e == nil {
		e = x
	}
	if e != nil {
		return e
	}
	if e = os.Rename(name, s.path); e != nil {
		return e
	}
	if s.syncDir != nil {
		return s.syncDir(dir)
	}
	return syncContentAIConfigDir(dir)
}
func (s *ContentAIConfigStore) backup(b []byte) error {
	f, e := os.OpenFile(s.path+".v1.bak", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(e, os.ErrExist) {
		return nil
	}
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	if x := f.Close(); e == nil {
		e = x
	}
	if e != nil {
		_ = os.Remove(s.path + ".v1.bak")
	}
	return e
}
