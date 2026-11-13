package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

// Blank overrides inherit the deployment configuration. Quota headroom remains
// YAML-only: gateway.openai_ws.scheduler_score_weights.quota_headroom.
type OpenAISchedulerSettings struct {
	StickyWeighted       bool   `json:"openai_advanced_scheduler_sticky_weighted_enabled"`
	SubscriptionPriority bool   `json:"openai_advanced_scheduler_subscription_priority_enabled"`
	LBTopK               string `json:"openai_advanced_scheduler_lb_top_k"`
	Priority             string `json:"openai_advanced_scheduler_weight_priority"`
	Load                 string `json:"openai_advanced_scheduler_weight_load"`
	Queue                string `json:"openai_advanced_scheduler_weight_queue"`
	ErrorRate            string `json:"openai_advanced_scheduler_weight_error_rate"`
	TTFT                 string `json:"openai_advanced_scheduler_weight_ttft"`
	Reset                string `json:"openai_advanced_scheduler_weight_reset"`
	PreviousResponse     string `json:"openai_advanced_scheduler_weight_previous_response"`
	SessionSticky        string `json:"openai_advanced_scheduler_weight_session_sticky"`
}

type OpenAISchedulerSettingsUpdate struct {
	StickyWeighted       *bool   `json:"openai_advanced_scheduler_sticky_weighted_enabled"`
	SubscriptionPriority *bool   `json:"openai_advanced_scheduler_subscription_priority_enabled"`
	LBTopK               *string `json:"openai_advanced_scheduler_lb_top_k"`
	Priority             *string `json:"openai_advanced_scheduler_weight_priority"`
	Load                 *string `json:"openai_advanced_scheduler_weight_load"`
	Queue                *string `json:"openai_advanced_scheduler_weight_queue"`
	ErrorRate            *string `json:"openai_advanced_scheduler_weight_error_rate"`
	TTFT                 *string `json:"openai_advanced_scheduler_weight_ttft"`
	Reset                *string `json:"openai_advanced_scheduler_weight_reset"`
	PreviousResponse     *string `json:"openai_advanced_scheduler_weight_previous_response"`
	SessionSticky        *string `json:"openai_advanced_scheduler_weight_session_sticky"`
}

func (u OpenAISchedulerSettingsUpdate) Merge(previous OpenAISchedulerSettings) OpenAISchedulerSettings {
	raw, _ := json.Marshal(u)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for k, v := range fields {
		if string(v) == "null" {
			delete(fields, k)
		}
	}
	raw, _ = json.Marshal(fields)
	_ = json.Unmarshal(raw, &previous)
	return previous
}

func (v OpenAISchedulerSettings) settingValues() (map[string]string, error) {
	raw, _ := json.Marshal(v)
	var values map[string]any
	_ = json.Unmarshal(raw, &values)
	out := make(map[string]string, len(values))
	for k, value := range values {
		if b, ok := value.(bool); ok {
			out[k] = strconv.FormatBool(b)
			continue
		}
		str := strings.TrimSpace(value.(string))
		if str != "" {
			n, err := strconv.ParseFloat(str, 64)
			invalid := err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0
			if strings.HasSuffix(k, "lb_top_k") {
				invalid = invalid || n < 1 || n > 100 || math.Trunc(n) != n
			}
			if invalid {
				return nil, infraerrors.BadRequest("INVALID_SCHEDULER_OVERRIDE", k+" must be blank or a nonnegative finite number (top K: integer 1-100)")
			}
		}
		out[k] = str
	}
	allZero := true
	for _, raw := range []string{v.Priority, v.Load, v.Queue, v.ErrorRate, v.TTFT, v.Reset} {
		if strings.TrimSpace(raw) == "" || overrideSchedulerNumber(raw, 1) > 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return nil, infraerrors.BadRequest("INVALID_SCHEDULER_OVERRIDE", "base scheduler weights must not all be zero")
	}
	return out, nil
}
func parseOpenAISchedulerSettings(values map[string]string) OpenAISchedulerSettings {
	defaults, _ := (OpenAISchedulerSettings{}).settingValues()
	raw := map[string]any{}
	for k := range defaults {
		if strings.HasSuffix(k, "enabled") {
			raw[k] = values[k] == "true"
		} else {
			raw[k] = values[k]
		}
	}
	blob, _ := json.Marshal(raw)
	var result OpenAISchedulerSettings
	_ = json.Unmarshal(blob, &result)
	return result
}

type openAISchedulerControlsCacheEntry struct {
	owner   *SettingService
	value   OpenAISchedulerSettings
	expires time.Time
}

var openAISchedulerControlsCache atomic.Pointer[openAISchedulerControlsCacheEntry]
var openAISchedulerControlsSF singleflight.Group

func (s *OpenAIGatewayService) schedulerControls(ctx context.Context) OpenAISchedulerSettings {
	if s == nil || s.rateLimitService == nil || s.rateLimitService.settingService == nil {
		return OpenAISchedulerSettings{}
	}
	owner := s.rateLimitService.settingService
	if c := openAISchedulerControlsCache.Load(); c != nil && c.owner == owner && time.Now().Before(c.expires) {
		return c.value
	}
	result, _, _ := openAISchedulerControlsSF.Do(fmt.Sprintf("%p", owner), func() (any, error) {
		if c := openAISchedulerControlsCache.Load(); c != nil && c.owner == owner && time.Now().Before(c.expires) {
			return c.value, nil
		}
		if owner.settingRepo == nil {
			return OpenAISchedulerSettings{}, nil
		}
		keysMap, _ := (OpenAISchedulerSettings{}).settingValues()
		keys := make([]string, 0, len(keysMap))
		for key := range keysMap {
			keys = append(keys, key)
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		values, err := owner.settingRepo.GetMultiple(dbCtx, keys)
		if err != nil {
			slog.Warn("openai_scheduler_settings_batch_failed", "error", err)
			values = map[string]string{}
			for _, key := range keys {
				if raw, getErr := owner.settingRepo.GetValue(dbCtx, key); getErr == nil {
					values[key] = raw
				}
			}
		}
		value := parseOpenAISchedulerSettings(values)
		ttl := 5 * time.Second
		if err != nil {
			ttl = time.Second
		}
		openAISchedulerControlsCache.Store(&openAISchedulerControlsCacheEntry{owner: owner, value: value, expires: time.Now().Add(ttl)})
		return value, nil
	})
	value, _ := result.(OpenAISchedulerSettings)
	return value
}
func overrideSchedulerNumber(raw string, fallback float64) float64 {
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return fallback
	}
	return n
}
func (v OpenAISchedulerSettings) weights(base GatewayOpenAIWSSchedulerScoreWeightsView) GatewayOpenAIWSSchedulerScoreWeightsView {
	base.Priority = overrideSchedulerNumber(v.Priority, base.Priority)
	base.Load = overrideSchedulerNumber(v.Load, base.Load)
	base.Queue = overrideSchedulerNumber(v.Queue, base.Queue)
	base.ErrorRate = overrideSchedulerNumber(v.ErrorRate, base.ErrorRate)
	base.TTFT = overrideSchedulerNumber(v.TTFT, base.TTFT)
	base.Reset = overrideSchedulerNumber(v.Reset, base.Reset)
	return base
}
