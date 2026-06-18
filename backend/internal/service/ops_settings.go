package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	opsAlertEvaluatorLeaderLockKeyDefault = "ops:alert:evaluator:leader"
	opsAlertEvaluatorLeaderLockTTLDefault = 30 * time.Second
)

type opsAIAnalysisConfigStored struct {
	Enabled                  bool     `json:"enabled"`
	BaseURL                  string   `json:"base_url"`
	APIKeyEncrypted          string   `json:"api_key_encrypted,omitempty"`
	Model                    string   `json:"model"`
	InterfaceType            string   `json:"interface_type"`
	TimeoutSeconds           int      `json:"timeout_seconds"`
	MaxSamples               int      `json:"max_samples"`
	AutoDedupMinutes         int      `json:"auto_dedup_minutes"`
	GlobalRateLimitPerMinute int      `json:"global_rate_limit_per_minute"`
	AutoLevels               []string `json:"auto_levels"`
	ManualEnabled            bool     `json:"manual_enabled"`
}

// =========================
// Email notification config
// =========================

func (s *OpsService) GetEmailNotificationConfig(ctx context.Context) (*OpsEmailNotificationConfig, error) {
	defaultCfg := defaultOpsEmailNotificationConfig()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsEmailNotificationConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			// Initialize defaults on first read (best-effort).
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsEmailNotificationConfig, string(b))
			}
			return defaultCfg, nil
		}
		return nil, err
	}

	cfg := &OpsEmailNotificationConfig{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		// Corrupted JSON should not break ops UI; fall back to defaults.
		return defaultCfg, nil
	}
	normalizeOpsEmailNotificationConfig(cfg)
	return cfg, nil
}

func (s *OpsService) UpdateEmailNotificationConfig(ctx context.Context, req *OpsEmailNotificationConfigUpdateRequest) (*OpsEmailNotificationConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if req == nil {
		return nil, errors.New("invalid request")
	}

	cfg, err := s.GetEmailNotificationConfig(ctx)
	if err != nil {
		return nil, err
	}

	if req.Alert != nil {
		cfg.Alert.Enabled = req.Alert.Enabled
		if req.Alert.Recipients != nil {
			cfg.Alert.Recipients = req.Alert.Recipients
		}
		cfg.Alert.MinSeverity = strings.TrimSpace(req.Alert.MinSeverity)
		cfg.Alert.RateLimitPerHour = req.Alert.RateLimitPerHour
		cfg.Alert.BatchingWindowSeconds = req.Alert.BatchingWindowSeconds
		cfg.Alert.IncludeResolvedAlerts = req.Alert.IncludeResolvedAlerts
	}

	if req.Report != nil {
		cfg.Report.Enabled = req.Report.Enabled
		if req.Report.Recipients != nil {
			cfg.Report.Recipients = req.Report.Recipients
		}
		cfg.Report.DailySummaryEnabled = req.Report.DailySummaryEnabled
		cfg.Report.DailySummarySchedule = strings.TrimSpace(req.Report.DailySummarySchedule)
		cfg.Report.WeeklySummaryEnabled = req.Report.WeeklySummaryEnabled
		cfg.Report.WeeklySummarySchedule = strings.TrimSpace(req.Report.WeeklySummarySchedule)
		cfg.Report.ErrorDigestEnabled = req.Report.ErrorDigestEnabled
		cfg.Report.ErrorDigestSchedule = strings.TrimSpace(req.Report.ErrorDigestSchedule)
		cfg.Report.ErrorDigestMinCount = req.Report.ErrorDigestMinCount
		cfg.Report.AccountHealthEnabled = req.Report.AccountHealthEnabled
		cfg.Report.AccountHealthSchedule = strings.TrimSpace(req.Report.AccountHealthSchedule)
		cfg.Report.AccountHealthErrorRateThreshold = req.Report.AccountHealthErrorRateThreshold
	}

	if err := validateOpsEmailNotificationConfig(cfg); err != nil {
		return nil, err
	}

	normalizeOpsEmailNotificationConfig(cfg)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsEmailNotificationConfig, string(raw)); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultOpsEmailNotificationConfig() *OpsEmailNotificationConfig {
	return &OpsEmailNotificationConfig{
		Alert: OpsEmailAlertConfig{
			Enabled:               true,
			Recipients:            []string{},
			MinSeverity:           "",
			RateLimitPerHour:      0,
			BatchingWindowSeconds: 0,
			IncludeResolvedAlerts: false,
		},
		Report: OpsEmailReportConfig{
			Enabled:                         false,
			Recipients:                      []string{},
			DailySummaryEnabled:             false,
			DailySummarySchedule:            "0 9 * * *",
			WeeklySummaryEnabled:            false,
			WeeklySummarySchedule:           "0 9 * * 1",
			ErrorDigestEnabled:              false,
			ErrorDigestSchedule:             "0 9 * * *",
			ErrorDigestMinCount:             10,
			AccountHealthEnabled:            false,
			AccountHealthSchedule:           "0 9 * * *",
			AccountHealthErrorRateThreshold: 10.0,
		},
	}
}

func normalizeOpsEmailNotificationConfig(cfg *OpsEmailNotificationConfig) {
	if cfg == nil {
		return
	}
	if cfg.Alert.Recipients == nil {
		cfg.Alert.Recipients = []string{}
	}
	if cfg.Report.Recipients == nil {
		cfg.Report.Recipients = []string{}
	}

	cfg.Alert.MinSeverity = strings.TrimSpace(cfg.Alert.MinSeverity)
	cfg.Report.DailySummarySchedule = strings.TrimSpace(cfg.Report.DailySummarySchedule)
	cfg.Report.WeeklySummarySchedule = strings.TrimSpace(cfg.Report.WeeklySummarySchedule)
	cfg.Report.ErrorDigestSchedule = strings.TrimSpace(cfg.Report.ErrorDigestSchedule)
	cfg.Report.AccountHealthSchedule = strings.TrimSpace(cfg.Report.AccountHealthSchedule)

	// Fill missing schedules with defaults to avoid breaking cron logic if clients send empty strings.
	if cfg.Report.DailySummarySchedule == "" {
		cfg.Report.DailySummarySchedule = "0 9 * * *"
	}
	if cfg.Report.WeeklySummarySchedule == "" {
		cfg.Report.WeeklySummarySchedule = "0 9 * * 1"
	}
	if cfg.Report.ErrorDigestSchedule == "" {
		cfg.Report.ErrorDigestSchedule = "0 9 * * *"
	}
	if cfg.Report.AccountHealthSchedule == "" {
		cfg.Report.AccountHealthSchedule = "0 9 * * *"
	}
}

func validateOpsEmailNotificationConfig(cfg *OpsEmailNotificationConfig) error {
	if cfg == nil {
		return errors.New("invalid config")
	}

	if cfg.Alert.RateLimitPerHour < 0 {
		return errors.New("alert.rate_limit_per_hour must be >= 0")
	}
	if cfg.Alert.BatchingWindowSeconds < 0 {
		return errors.New("alert.batching_window_seconds must be >= 0")
	}
	switch strings.TrimSpace(cfg.Alert.MinSeverity) {
	case "", "critical", "warning", "info":
	default:
		return errors.New("alert.min_severity must be one of: critical, warning, info, or empty")
	}

	if cfg.Report.ErrorDigestMinCount < 0 {
		return errors.New("report.error_digest_min_count must be >= 0")
	}
	if cfg.Report.AccountHealthErrorRateThreshold < 0 || cfg.Report.AccountHealthErrorRateThreshold > 100 {
		return errors.New("report.account_health_error_rate_threshold must be between 0 and 100")
	}
	return nil
}

func (s *OpsService) GetAccountQuotaNotifyEmails(ctx context.Context) []string {
	if s == nil || s.settingRepo == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAccountQuotaNotifyEmails)
	if err != nil || strings.TrimSpace(raw) == "" || raw == "[]" {
		return nil
	}
	return filterVerifiedEmails(ParseNotifyEmails(raw))
}

// =========================
// Alert runtime settings
// =========================

func defaultOpsAlertRuntimeSettings() *OpsAlertRuntimeSettings {
	return &OpsAlertRuntimeSettings{
		EvaluationIntervalSeconds: 60,
		DistributedLock: OpsDistributedLockSettings{
			Enabled:    true,
			Key:        opsAlertEvaluatorLeaderLockKeyDefault,
			TTLSeconds: int(opsAlertEvaluatorLeaderLockTTLDefault.Seconds()),
		},
		Silencing: OpsAlertSilencingSettings{
			Enabled:            false,
			GlobalUntilRFC3339: "",
			GlobalReason:       "",
			Entries:            []OpsAlertSilenceEntry{},
		},
	}
}

func normalizeOpsDistributedLockSettings(s *OpsDistributedLockSettings, defaultKey string, defaultTTLSeconds int) {
	if s == nil {
		return
	}
	s.Key = strings.TrimSpace(s.Key)
	if s.Key == "" {
		s.Key = defaultKey
	}
	if s.TTLSeconds <= 0 {
		s.TTLSeconds = defaultTTLSeconds
	}
}

func normalizeOpsAlertSilencingSettings(s *OpsAlertSilencingSettings) {
	if s == nil {
		return
	}
	s.GlobalUntilRFC3339 = strings.TrimSpace(s.GlobalUntilRFC3339)
	s.GlobalReason = strings.TrimSpace(s.GlobalReason)
	if s.Entries == nil {
		s.Entries = []OpsAlertSilenceEntry{}
	}
	for i := range s.Entries {
		s.Entries[i].UntilRFC3339 = strings.TrimSpace(s.Entries[i].UntilRFC3339)
		s.Entries[i].Reason = strings.TrimSpace(s.Entries[i].Reason)
	}
}

func validateOpsDistributedLockSettings(s OpsDistributedLockSettings) error {
	if strings.TrimSpace(s.Key) == "" {
		return errors.New("distributed_lock.key is required")
	}
	if s.TTLSeconds <= 0 || s.TTLSeconds > int((24*time.Hour).Seconds()) {
		return errors.New("distributed_lock.ttl_seconds must be between 1 and 86400")
	}
	return nil
}

func validateOpsAlertSilencingSettings(s OpsAlertSilencingSettings) error {
	parse := func(raw string) error {
		if strings.TrimSpace(raw) == "" {
			return nil
		}
		if _, err := time.Parse(time.RFC3339, raw); err != nil {
			return errors.New("silencing time must be RFC3339")
		}
		return nil
	}

	if err := parse(s.GlobalUntilRFC3339); err != nil {
		return err
	}
	for _, entry := range s.Entries {
		if strings.TrimSpace(entry.UntilRFC3339) == "" {
			return errors.New("silencing.entries.until_rfc3339 is required")
		}
		if _, err := time.Parse(time.RFC3339, entry.UntilRFC3339); err != nil {
			return errors.New("silencing.entries.until_rfc3339 must be RFC3339")
		}
	}
	return nil
}

func (s *OpsService) GetOpsAlertRuntimeSettings(ctx context.Context) (*OpsAlertRuntimeSettings, error) {
	defaultCfg := defaultOpsAlertRuntimeSettings()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsAlertRuntimeSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsAlertRuntimeSettings, string(b))
			}
			return defaultCfg, nil
		}
		return nil, err
	}

	cfg := &OpsAlertRuntimeSettings{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return defaultCfg, nil
	}

	if cfg.EvaluationIntervalSeconds <= 0 {
		cfg.EvaluationIntervalSeconds = defaultCfg.EvaluationIntervalSeconds
	}
	normalizeOpsDistributedLockSettings(&cfg.DistributedLock, opsAlertEvaluatorLeaderLockKeyDefault, defaultCfg.DistributedLock.TTLSeconds)
	normalizeOpsAlertSilencingSettings(&cfg.Silencing)

	return cfg, nil
}

func (s *OpsService) UpdateOpsAlertRuntimeSettings(ctx context.Context, cfg *OpsAlertRuntimeSettings) (*OpsAlertRuntimeSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return nil, errors.New("invalid config")
	}

	if cfg.EvaluationIntervalSeconds < 1 || cfg.EvaluationIntervalSeconds > int((24*time.Hour).Seconds()) {
		return nil, errors.New("evaluation_interval_seconds must be between 1 and 86400")
	}
	if cfg.DistributedLock.Enabled {
		if err := validateOpsDistributedLockSettings(cfg.DistributedLock); err != nil {
			return nil, err
		}
	}
	if cfg.Silencing.Enabled {
		if err := validateOpsAlertSilencingSettings(cfg.Silencing); err != nil {
			return nil, err
		}
	}

	defaultCfg := defaultOpsAlertRuntimeSettings()
	normalizeOpsDistributedLockSettings(&cfg.DistributedLock, opsAlertEvaluatorLeaderLockKeyDefault, defaultCfg.DistributedLock.TTLSeconds)
	normalizeOpsAlertSilencingSettings(&cfg.Silencing)

	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsAlertRuntimeSettings, string(raw)); err != nil {
		return nil, err
	}

	// Return a fresh copy (avoid callers holding pointers into internal slices that may be mutated).
	updated := &OpsAlertRuntimeSettings{}
	_ = json.Unmarshal(raw, updated)
	return updated, nil
}

func defaultOpsAIAnalysisConfig() *OpsAIAnalysisConfig {
	return &OpsAIAnalysisConfig{
		Enabled:                  false,
		BaseURL:                  "",
		Model:                    "",
		InterfaceType:            "responses",
		TimeoutSeconds:           60,
		MaxSamples:               50,
		AutoDedupMinutes:         10,
		GlobalRateLimitPerMinute: 10,
		AutoLevels:               []string{"P0", "P1"},
		ManualEnabled:            true,
	}
}

func normalizeOpsAIAnalysisConfig(cfg *OpsAIAnalysisConfig) {
	if cfg == nil {
		return
	}
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.APIKeyEncrypted = strings.TrimSpace(cfg.APIKeyEncrypted)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.InterfaceType = strings.TrimSpace(cfg.InterfaceType)
	if cfg.InterfaceType == "" {
		cfg.InterfaceType = "responses"
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 60
	}
	if cfg.MaxSamples <= 0 {
		cfg.MaxSamples = 50
	}
	if cfg.AutoDedupMinutes < 0 {
		cfg.AutoDedupMinutes = 10
	}
	if cfg.GlobalRateLimitPerMinute <= 0 {
		cfg.GlobalRateLimitPerMinute = 10
	}
	normalizedLevels := make([]string, 0, len(cfg.AutoLevels))
	seen := map[string]struct{}{}
	for _, level := range cfg.AutoLevels {
		level = strings.TrimSpace(level)
		if level == "" {
			continue
		}
		if _, ok := seen[level]; ok {
			continue
		}
		seen[level] = struct{}{}
		normalizedLevels = append(normalizedLevels, level)
	}
	if len(normalizedLevels) == 0 {
		normalizedLevels = []string{"P0", "P1"}
	}
	cfg.AutoLevels = normalizedLevels
}

func normalizeOpsAIAnalysisConfigRequest(cfg *OpsAIAnalysisConfig) {
	if cfg == nil {
		return
	}
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.APIKeyEncrypted = strings.TrimSpace(cfg.APIKeyEncrypted)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.InterfaceType = strings.TrimSpace(cfg.InterfaceType)
	normalizedLevels := make([]string, 0, len(cfg.AutoLevels))
	seen := map[string]struct{}{}
	for _, level := range cfg.AutoLevels {
		level = strings.TrimSpace(level)
		if level == "" {
			continue
		}
		if _, ok := seen[level]; ok {
			continue
		}
		seen[level] = struct{}{}
		normalizedLevels = append(normalizedLevels, level)
	}
	cfg.AutoLevels = normalizedLevels
}

func validateOpsAIAnalysisConfig(cfg *OpsAIAnalysisConfig) error {
	if cfg == nil {
		return errors.New("invalid config")
	}
	validInterfaceTypes := map[string]struct{}{
		"openai_compatible":    {},
		"responses":            {},
		"anthropic_compatible": {},
		"gemini_compatible":    {},
	}
	if _, ok := validInterfaceTypes[cfg.InterfaceType]; !ok {
		return errors.New("interface_type must be one of openai_compatible, responses, anthropic_compatible or gemini_compatible")
	}
	if cfg.TimeoutSeconds < 5 || cfg.TimeoutSeconds > 300 {
		return errors.New("timeout_seconds must be between 5 and 300")
	}
	if cfg.MaxSamples < 1 || cfg.MaxSamples > 500 {
		return errors.New("max_samples must be between 1 and 500")
	}
	if cfg.AutoDedupMinutes < 1 || cfg.AutoDedupMinutes > 1440 {
		return errors.New("auto_dedup_minutes must be between 1 and 1440")
	}
	if cfg.GlobalRateLimitPerMinute < 1 || cfg.GlobalRateLimitPerMinute > 1000 {
		return errors.New("global_rate_limit_per_minute must be between 1 and 1000")
	}
	if len(cfg.BaseURL) > 500 {
		return errors.New("base_url must not exceed 500 characters")
	}
	if len(cfg.Model) > 100 {
		return errors.New("model must not exceed 100 characters")
	}
	validLevels := map[string]struct{}{"P0": {}, "P1": {}, "P2": {}}
	for _, level := range cfg.AutoLevels {
		if _, ok := validLevels[level]; !ok {
			return errors.New("auto_levels contains invalid level")
		}
	}
	if cfg.BaseURL != "" {
		u, err := url.Parse(cfg.BaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("base_url must be a valid http(s) URL")
		}
	}
	if cfg.Enabled {
		if cfg.BaseURL == "" {
			return errors.New("base_url is required when enabled")
		}
		if cfg.Model == "" {
			return errors.New("model is required when enabled")
		}
		if cfg.APIKeyEncrypted == "" {
			return errors.New("api_key is required when enabled")
		}
	}
	return nil
}

func opsAIConfigToStored(cfg *OpsAIAnalysisConfig) *opsAIAnalysisConfigStored {
	if cfg == nil {
		cfg = defaultOpsAIAnalysisConfig()
	}
	return &opsAIAnalysisConfigStored{
		Enabled:                  cfg.Enabled,
		BaseURL:                  cfg.BaseURL,
		APIKeyEncrypted:          cfg.APIKeyEncrypted,
		Model:                    cfg.Model,
		InterfaceType:            cfg.InterfaceType,
		TimeoutSeconds:           cfg.TimeoutSeconds,
		MaxSamples:               cfg.MaxSamples,
		AutoDedupMinutes:         cfg.AutoDedupMinutes,
		GlobalRateLimitPerMinute: cfg.GlobalRateLimitPerMinute,
		AutoLevels:               append([]string(nil), cfg.AutoLevels...),
		ManualEnabled:            cfg.ManualEnabled,
	}
}

func opsAIConfigFromStored(stored *opsAIAnalysisConfigStored) *OpsAIAnalysisConfig {
	cfg := defaultOpsAIAnalysisConfig()
	if stored == nil {
		return cfg
	}
	cfg.Enabled = stored.Enabled
	cfg.BaseURL = stored.BaseURL
	cfg.APIKeyEncrypted = stored.APIKeyEncrypted
	cfg.Model = stored.Model
	cfg.InterfaceType = stored.InterfaceType
	cfg.TimeoutSeconds = stored.TimeoutSeconds
	cfg.MaxSamples = stored.MaxSamples
	cfg.AutoDedupMinutes = stored.AutoDedupMinutes
	cfg.GlobalRateLimitPerMinute = stored.GlobalRateLimitPerMinute
	cfg.AutoLevels = append([]string(nil), stored.AutoLevels...)
	cfg.ManualEnabled = stored.ManualEnabled
	normalizeOpsAIAnalysisConfig(cfg)
	return cfg
}

func (s *OpsService) maskOpsAIAPIKey(encrypted string) string {
	encrypted = strings.TrimSpace(encrypted)
	if encrypted == "" {
		return ""
	}
	if s == nil || s.secretEncryptor == nil {
		return "****"
	}
	plain, err := s.secretEncryptor.Decrypt(encrypted)
	if err != nil {
		return "****"
	}
	plain = strings.TrimSpace(plain)
	if len(plain) <= 4 {
		return "****"
	}
	return "****" + plain[len(plain)-4:]
}

func (s *OpsService) GetOpsAIAnalysisConfig(ctx context.Context) (*OpsAIAnalysisConfig, error) {
	defaultCfg := defaultOpsAIAnalysisConfig()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsAIAnalysisConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			if b, mErr := json.Marshal(opsAIConfigToStored(defaultCfg)); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsAIAnalysisConfig, string(b))
			}
			return defaultCfg, nil
		}
		return nil, err
	}

	stored := &opsAIAnalysisConfigStored{}
	if err := json.Unmarshal([]byte(raw), stored); err != nil {
		return defaultCfg, nil
	}

	cfg := opsAIConfigFromStored(stored)
	cfg.APIKeyMasked = s.maskOpsAIAPIKey(cfg.APIKeyEncrypted)
	cfg.APIKeyEncrypted = ""
	return cfg, nil
}

func (s *OpsService) UpdateOpsAIAnalysisConfig(ctx context.Context, req *OpsAIAnalysisConfigUpdateRequest) (*OpsAIAnalysisConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if req == nil {
		return nil, errors.New("invalid config")
	}

	existing, err := s.loadOpsAIAnalysisConfigForUpdate(ctx)
	if err != nil {
		return nil, err
	}

	cfg := &OpsAIAnalysisConfig{
		Enabled:                  req.Enabled,
		BaseURL:                  req.BaseURL,
		APIKeyEncrypted:          existing.APIKeyEncrypted,
		Model:                    req.Model,
		InterfaceType:            req.InterfaceType,
		TimeoutSeconds:           req.TimeoutSeconds,
		MaxSamples:               req.MaxSamples,
		AutoDedupMinutes:         req.AutoDedupMinutes,
		GlobalRateLimitPerMinute: req.GlobalRateLimitPerMinute,
		AutoLevels:               append([]string(nil), req.AutoLevels...),
		ManualEnabled:            req.ManualEnabled,
	}
	normalizeOpsAIAnalysisConfigRequest(cfg)

	if plainKey := strings.TrimSpace(req.APIKey); plainKey != "" {
		if s.secretEncryptor == nil {
			return nil, errors.New("secret encryptor not initialized")
		}
		encrypted, err := s.secretEncryptor.Encrypt(plainKey)
		if err != nil {
			return nil, err
		}
		cfg.APIKeyEncrypted = encrypted
	}

	if err := validateOpsAIAnalysisConfig(cfg); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(opsAIConfigToStored(cfg))
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsAIAnalysisConfig, string(raw)); err != nil {
		return nil, err
	}

	cfg.APIKeyMasked = s.maskOpsAIAPIKey(cfg.APIKeyEncrypted)
	cfg.APIKeyEncrypted = ""
	return cfg, nil
}

func (s *OpsService) loadOpsAIAnalysisConfigForUpdate(ctx context.Context) (*OpsAIAnalysisConfig, error) {
	defaultCfg := defaultOpsAIAnalysisConfig()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsAIAnalysisConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return defaultCfg, nil
		}
		return nil, err
	}
	stored := &opsAIAnalysisConfigStored{}
	if err := json.Unmarshal([]byte(raw), stored); err != nil {
		return defaultCfg, nil
	}
	return opsAIConfigFromStored(stored), nil
}

// =========================
// Advanced settings
// =========================

func defaultOpsAdvancedSettings() *OpsAdvancedSettings {
	return &OpsAdvancedSettings{
		DataRetention: OpsDataRetentionSettings{
			CleanupEnabled:             false,
			CleanupSchedule:            opsCleanupDefaultSchedule,
			ErrorLogRetentionDays:      30,
			MinuteMetricsRetentionDays: 30,
			HourlyMetricsRetentionDays: 30,
		},
		Aggregation: OpsAggregationSettings{
			AggregationEnabled: false,
		},
		IgnoreCountTokensErrors:         true,  // count_tokens 404 是预期行为，默认忽略
		IgnoreContextCanceled:           true,  // Default to true - client disconnects are not errors
		IgnoreNoAvailableAccounts:       false, // Default to false - this is a real routing issue
		IgnoreInsufficientBalanceErrors: false, // 默认不忽略，余额不足可能需要关注
		DisplayOpenAITokenStats:         false,
		DisplayAlertEvents:              true,
		AutoRefreshEnabled:              false,
		AutoRefreshIntervalSec:          30,
	}
}

func normalizeOpsAdvancedSettings(cfg *OpsAdvancedSettings) {
	if cfg == nil {
		return
	}
	cfg.DataRetention.CleanupSchedule = strings.TrimSpace(cfg.DataRetention.CleanupSchedule)
	if cfg.DataRetention.CleanupSchedule == "" {
		cfg.DataRetention.CleanupSchedule = opsCleanupDefaultSchedule
	}
	// 保留天数：0 表示每次定时清理全部（清空所有），> 0 表示按天数保留；
	// 仅在拿到非法的负数时回填默认值，避免覆盖用户主动设的 0。
	if cfg.DataRetention.ErrorLogRetentionDays < 0 {
		cfg.DataRetention.ErrorLogRetentionDays = 30
	}
	if cfg.DataRetention.MinuteMetricsRetentionDays < 0 {
		cfg.DataRetention.MinuteMetricsRetentionDays = 30
	}
	if cfg.DataRetention.HourlyMetricsRetentionDays < 0 {
		cfg.DataRetention.HourlyMetricsRetentionDays = 30
	}
	// Normalize auto refresh interval (default 30 seconds)
	if cfg.AutoRefreshIntervalSec <= 0 {
		cfg.AutoRefreshIntervalSec = 30
	}
}

func validateOpsAdvancedSettings(cfg *OpsAdvancedSettings) error {
	if cfg == nil {
		return errors.New("invalid config")
	}
	// 保留天数：0 表示每次清理全部，1-365 表示按天数保留。
	if cfg.DataRetention.ErrorLogRetentionDays < 0 || cfg.DataRetention.ErrorLogRetentionDays > 365 {
		return errors.New("error_log_retention_days must be between 0 and 365")
	}
	if cfg.DataRetention.MinuteMetricsRetentionDays < 0 || cfg.DataRetention.MinuteMetricsRetentionDays > 365 {
		return errors.New("minute_metrics_retention_days must be between 0 and 365")
	}
	if cfg.DataRetention.HourlyMetricsRetentionDays < 0 || cfg.DataRetention.HourlyMetricsRetentionDays > 365 {
		return errors.New("hourly_metrics_retention_days must be between 0 and 365")
	}
	if cfg.AutoRefreshIntervalSec < 15 || cfg.AutoRefreshIntervalSec > 300 {
		return errors.New("auto_refresh_interval_seconds must be between 15 and 300")
	}
	return nil
}

func (s *OpsService) GetOpsAdvancedSettings(ctx context.Context) (*OpsAdvancedSettings, error) {
	defaultCfg := defaultOpsAdvancedSettings()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsAdvancedSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsAdvancedSettings, string(b))
			}
			return defaultCfg, nil
		}
		return nil, err
	}

	cfg := defaultOpsAdvancedSettings()
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return defaultCfg, nil
	}

	normalizeOpsAdvancedSettings(cfg)
	return cfg, nil
}

func (s *OpsService) UpdateOpsAdvancedSettings(ctx context.Context, cfg *OpsAdvancedSettings) (*OpsAdvancedSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return nil, errors.New("invalid config")
	}

	if err := validateOpsAdvancedSettings(cfg); err != nil {
		return nil, err
	}

	normalizeOpsAdvancedSettings(cfg)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsAdvancedSettings, string(raw)); err != nil {
		return nil, err
	}

	// notify cleanup service to reload schedule/enabled.
	if s.cleanupReloader != nil {
		if rerr := s.cleanupReloader.Reload(ctx); rerr != nil {
			logger.LegacyPrintf("service.ops_settings",
				"[OpsSettings] cleanup reload after advanced-settings update failed: %v", rerr)
		}
	}

	updated := &OpsAdvancedSettings{}
	_ = json.Unmarshal(raw, updated)
	return updated, nil
}

// =========================
// Metric thresholds
// =========================

const SettingKeyOpsMetricThresholds = "ops_metric_thresholds"

func defaultOpsMetricThresholds() *OpsMetricThresholds {
	slaMin := 99.5
	ttftMax := 500.0
	reqErrMax := 5.0
	upstreamErrMax := 5.0
	return &OpsMetricThresholds{
		SLAPercentMin:               &slaMin,
		TTFTp99MsMax:                &ttftMax,
		RequestErrorRatePercentMax:  &reqErrMax,
		UpstreamErrorRatePercentMax: &upstreamErrMax,
	}
}

func (s *OpsService) GetMetricThresholds(ctx context.Context) (*OpsMetricThresholds, error) {
	defaultCfg := defaultOpsMetricThresholds()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsMetricThresholds)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsMetricThresholds, string(b))
			}
			return defaultCfg, nil
		}
		return nil, err
	}

	cfg := &OpsMetricThresholds{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return defaultCfg, nil
	}

	return cfg, nil
}

func (s *OpsService) UpdateMetricThresholds(ctx context.Context, cfg *OpsMetricThresholds) (*OpsMetricThresholds, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg == nil {
		return nil, errors.New("invalid config")
	}

	// Validate thresholds
	if cfg.SLAPercentMin != nil && (*cfg.SLAPercentMin < 0 || *cfg.SLAPercentMin > 100) {
		return nil, errors.New("sla_percent_min must be between 0 and 100")
	}
	if cfg.TTFTp99MsMax != nil && *cfg.TTFTp99MsMax < 0 {
		return nil, errors.New("ttft_p99_ms_max must be >= 0")
	}
	if cfg.RequestErrorRatePercentMax != nil && (*cfg.RequestErrorRatePercentMax < 0 || *cfg.RequestErrorRatePercentMax > 100) {
		return nil, errors.New("request_error_rate_percent_max must be between 0 and 100")
	}
	if cfg.UpstreamErrorRatePercentMax != nil && (*cfg.UpstreamErrorRatePercentMax < 0 || *cfg.UpstreamErrorRatePercentMax > 100) {
		return nil, errors.New("upstream_error_rate_percent_max must be between 0 and 100")
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsMetricThresholds, string(raw)); err != nil {
		return nil, err
	}

	updated := &OpsMetricThresholds{}
	_ = json.Unmarshal(raw, updated)
	return updated, nil
}
