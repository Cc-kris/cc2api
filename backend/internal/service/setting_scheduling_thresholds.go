package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const SettingKeyAccountSchedulingThresholds = "account_scheduling_thresholds"

var AllowedSchedulingThresholdPlatforms = []string{PlatformOpenAI, PlatformAnthropic, PlatformGrok}

type cachedAccountSchedulingThresholds struct {
	owner      *SettingService
	thresholds map[string]int
	expiresAt  int64
}

var accountSchedulingThresholdsCache atomic.Value
var accountSchedulingThresholdsSF singleflight.Group

func defaultAccountSchedulingThresholds() map[string]int {
	return map[string]int{PlatformOpenAI: 100, PlatformAnthropic: 100, PlatformGrok: 100}
}
func cloneAccountSchedulingThresholds(input map[string]int) map[string]int {
	out := defaultAccountSchedulingThresholds()
	for k, v := range input {
		out[k] = v
	}
	return out
}
func validateAndNormalizeAccountSchedulingThresholds(input map[string]int) (map[string]int, error) {
	out := defaultAccountSchedulingThresholds()
	for k, v := range input {
		if !isAllowedSchedulingThresholdPlatform(k) || v < 1 || v > 100 {
			return nil, infraerrors.BadRequest("INVALID_SCHEDULING_THRESHOLD", fmt.Sprintf("account_scheduling_thresholds: %s must be a supported platform with threshold 1-100", k))
		}
		out[k] = v
	}
	return out, nil
}
func parseAccountSchedulingThresholdsSetting(raw string) (map[string]int, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultAccountSchedulingThresholds(), nil
	}
	var input map[string]int
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return defaultAccountSchedulingThresholds(), err
	}
	// Ignore removed/unsupported platforms when reading existing settings, but never accept them on write.
	out := defaultAccountSchedulingThresholds()
	for k, v := range input {
		if isAllowedSchedulingThresholdPlatform(k) && v >= 1 && v <= 100 {
			out[k] = v
		}
	}
	return out, nil
}
func (s *SettingService) GetAccountSchedulingThresholds(ctx context.Context) map[string]int {
	if s == nil || s.settingRepo == nil {
		return defaultAccountSchedulingThresholds()
	}
	if cached, ok := accountSchedulingThresholdsCache.Load().(*cachedAccountSchedulingThresholds); ok && cached.owner == s && time.Now().UnixNano() < cached.expiresAt {
		return cloneAccountSchedulingThresholds(cached.thresholds)
	}
	value, _, _ := accountSchedulingThresholdsSF.Do(fmt.Sprintf("%p", s), func() (any, error) {
		if cached, ok := accountSchedulingThresholdsCache.Load().(*cachedAccountSchedulingThresholds); ok && cached.owner == s && time.Now().UnixNano() < cached.expiresAt {
			return cloneAccountSchedulingThresholds(cached.thresholds), nil
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyAccountSchedulingThresholds)
		thresholds := defaultAccountSchedulingThresholds()
		ttl := 5 * time.Second
		if err == nil {
			thresholds, err = parseAccountSchedulingThresholdsSetting(raw)
		}
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			slog.Warn("account_scheduling_thresholds_load_failed", "error", err)
			ttl = time.Second
		}
		accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{owner: s, thresholds: thresholds, expiresAt: time.Now().Add(ttl).UnixNano()})
		return cloneAccountSchedulingThresholds(thresholds), nil
	})
	if thresholds, ok := value.(map[string]int); ok {
		return thresholds
	}
	return defaultAccountSchedulingThresholds()
}
