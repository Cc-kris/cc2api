package admin

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

var validOpsAlertMetricTypes = []string{
	"success_rate",
	"error_rate",
	"upstream_error_rate",
	"p95_latency_ms",
	"p99_latency_ms",
	"cpu_usage_percent",
	"memory_usage_percent",
	"concurrency_queue_depth",
	"group_available_accounts",
	"group_available_ratio",
	"group_rate_limit_ratio",
	"account_rate_limited_count",
	"account_error_count",
	"account_error_ratio",
	"overload_account_count",
}

var validOpsAlertMetricTypeSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(validOpsAlertMetricTypes))
	for _, v := range validOpsAlertMetricTypes {
		set[v] = struct{}{}
	}
	return set
}()

var validOpsAlertOperators = []string{">", "<", ">=", "<=", "==", "!="}

var validOpsAlertOperatorSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(validOpsAlertOperators))
	for _, v := range validOpsAlertOperators {
		set[v] = struct{}{}
	}
	return set
}()

var validOpsAlertSeverities = []string{"P0", "P1", "P2", "P3"}

var validOpsAlertSeveritySet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(validOpsAlertSeverities))
	for _, v := range validOpsAlertSeverities {
		set[v] = struct{}{}
	}
	return set
}()

var validOpsAlertErrorCategories = service.AllOpsErrorCategories

var validOpsAlertErrorCategorySet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(validOpsAlertErrorCategories))
	for _, v := range validOpsAlertErrorCategories {
		set[v] = struct{}{}
	}
	return set
}()

var validOpsAlertTriggerLevels = []string{"P0", "P1", "P2", "observe"}

var validOpsAlertTriggerLevelSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(validOpsAlertTriggerLevels))
	for _, v := range validOpsAlertTriggerLevels {
		set[v] = struct{}{}
	}
	return set
}()

var validOpsAlertRecoveredPolicies = []string{"record_only", "observe_only", "alert"}

var validOpsAlertRecoveredPolicySet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(validOpsAlertRecoveredPolicies))
	for _, v := range validOpsAlertRecoveredPolicies {
		set[v] = struct{}{}
	}
	return set
}()

var validOpsAlertNotificationChannels = []string{"in_app", "email", "none"}

var validOpsAlertNotificationChannelSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(validOpsAlertNotificationChannels))
	for _, v := range validOpsAlertNotificationChannels {
		set[v] = struct{}{}
	}
	return set
}()

type opsAlertRuleValidatedInput struct {
	Name       string
	MetricType string
	Operator   string
	Threshold  float64

	Severity string

	WindowMinutes    int
	SustainedMinutes int
	CooldownMinutes  int

	Enabled     bool
	NotifyEmail bool

	RuleVersion                string
	ErrorCategories            []string
	TriggerLevel               string
	MinFinalFailures           int
	MinFailureRate             float64
	MinSampleCount             int
	ImpactScope                map[string]int
	RecoveredFluctuationPolicy string
	MinRecoveredFluctuations   int
	AutoAIAnalysis             bool
	NotificationChannels       []string
	SilenceMinutes             int
	MigrationState             string
}

func isPercentOrRateMetric(metricType string) bool {
	switch metricType {
	case "success_rate",
		"error_rate",
		"upstream_error_rate",
		"cpu_usage_percent",
		"memory_usage_percent",
		"group_available_ratio",
		"group_rate_limit_ratio",
		"account_error_ratio":
		return true
	default:
		return false
	}
}

func validateOpsAlertRulePayload(raw map[string]json.RawMessage) (*opsAlertRuleValidatedInput, error) {
	if raw == nil {
		return nil, fmt.Errorf("invalid request body")
	}
	if isOpsAlertRuleV2Payload(raw) {
		return validateOpsAlertRuleV2Payload(raw)
	}
	return validateOpsAlertRuleLegacyPayload(raw)
}

func isOpsAlertRuleV2Payload(raw map[string]json.RawMessage) bool {
	for _, field := range []string{
		"time_window",
		"error_categories",
		"trigger_level",
		"min_final_failures",
		"min_failure_rate",
		"min_sample_count",
		"impact_scope",
		"recovered_fluctuation_policy",
		"auto_ai_analysis",
		"notification_channels",
		"silence_minutes",
	} {
		if _, ok := raw[field]; ok {
			return true
		}
	}
	return false
}

func validateOpsAlertRuleV2Payload(raw map[string]json.RawMessage) (*opsAlertRuleValidatedInput, error) {
	name, err := parseOpsAlertRequiredString(raw, "name", "请输入规则名称")
	if err != nil {
		return nil, err
	}
	if l := len([]rune(name)); l < 2 || l > 50 {
		return nil, fmt.Errorf("规则名称需为 2～50 字")
	}

	enabled := true
	if v, ok := raw["enabled"]; ok {
		if err := json.Unmarshal(v, &enabled); err != nil {
			return nil, fmt.Errorf("enabled must be a boolean")
		}
	}

	windowMinutes := 1
	if v, ok := raw["time_window"]; ok {
		var timeWindow string
		if err := json.Unmarshal(v, &timeWindow); err != nil {
			return nil, fmt.Errorf("time_window must be a string")
		}
		if strings.TrimSpace(timeWindow) != "1m" {
			return nil, fmt.Errorf("本版本固定 1 分钟窗口")
		}
	} else if v, ok := raw["window_minutes"]; ok {
		if err := json.Unmarshal(v, &windowMinutes); err != nil {
			return nil, fmt.Errorf("window_minutes must be an integer")
		}
		if windowMinutes != 1 {
			return nil, fmt.Errorf("本版本固定 1 分钟窗口")
		}
	}

	errorCategories, err := parseOpsAlertStringList(raw, "error_categories", true, "请选择错误分类")
	if err != nil {
		return nil, err
	}
	if len(errorCategories) > 20 {
		return nil, fmt.Errorf("错误分类最多选择 20 项")
	}
	for _, category := range errorCategories {
		if _, ok := validOpsAlertErrorCategorySet[category]; !ok {
			return nil, fmt.Errorf("错误分类必须为：%s", strings.Join(validOpsAlertErrorCategories, ", "))
		}
	}

	triggerLevel, err := parseOpsAlertRequiredString(raw, "trigger_level", "trigger_level is required")
	if err != nil {
		return nil, err
	}
	triggerLevel = normalizeOpsAlertTriggerLevel(triggerLevel)
	if _, ok := validOpsAlertTriggerLevelSet[triggerLevel]; !ok {
		return nil, fmt.Errorf("trigger_level must be one of: %s", strings.Join(validOpsAlertTriggerLevels, ", "))
	}

	minFinalFailures, err := parseOpsAlertRequiredInt(raw, "min_final_failures", "min_final_failures is required")
	if err != nil {
		return nil, err
	}
	if minFinalFailures < 1 || minFinalFailures > 100000 {
		return nil, fmt.Errorf("最小最终失败数需为 1～100000 的整数")
	}

	minFailureRate, err := parseOpsAlertRequiredFloat(raw, "min_failure_rate", "min_failure_rate is required")
	if err != nil {
		return nil, err
	}
	if minFailureRate < 0 || minFailureRate > 100 || math.Round(minFailureRate*100) != minFailureRate*100 {
		return nil, fmt.Errorf("请输入 0～100 的百分比")
	}

	minSampleCount, err := parseOpsAlertRequiredInt(raw, "min_sample_count", "min_sample_count is required")
	if err != nil {
		return nil, err
	}
	if minSampleCount < 1 || minSampleCount > 1000000 {
		return nil, fmt.Errorf("请输入大于 0 的整数")
	}
	if minFailureRate > 0 && minFinalFailures > minSampleCount {
		return nil, fmt.Errorf("最小最终失败数不能大于最小样本量")
	}
	if minFailureRate > 0 && (minFinalFailures <= 0 || minSampleCount <= 0) {
		return nil, fmt.Errorf("百分比规则必须配置最小失败数和最小样本量")
	}

	impactScope, err := parseOpsAlertImpactScope(raw["impact_scope"])
	if err != nil {
		return nil, err
	}

	recoveredPolicy := "record_only"
	if v, ok := raw["recovered_fluctuation_policy"]; ok {
		if err := json.Unmarshal(v, &recoveredPolicy); err != nil {
			return nil, fmt.Errorf("recovered_fluctuation_policy must be a string")
		}
		recoveredPolicy = strings.TrimSpace(recoveredPolicy)
	}
	if _, ok := validOpsAlertRecoveredPolicySet[recoveredPolicy]; !ok {
		return nil, fmt.Errorf("recovered_fluctuation_policy must be one of: %s", strings.Join(validOpsAlertRecoveredPolicies, ", "))
	}

	minRecovered := 0
	if v, ok := raw["min_recovered_fluctuations"]; ok {
		if err := json.Unmarshal(v, &minRecovered); err != nil {
			return nil, fmt.Errorf("min_recovered_fluctuations must be an integer")
		}
	}
	if recoveredPolicy != "record_only" {
		if minRecovered < 1 || minRecovered > 100000 {
			return nil, fmt.Errorf("已恢复波动参与告警需配置最小波动数")
		}
	} else if minRecovered < 0 || minRecovered > 100000 {
		return nil, fmt.Errorf("min_recovered_fluctuations must be between 0 and 100000")
	}

	autoAIAnalysis := triggerLevel == "P0" || triggerLevel == "P1"
	if v, ok := raw["auto_ai_analysis"]; ok {
		if err := json.Unmarshal(v, &autoAIAnalysis); err != nil {
			return nil, fmt.Errorf("auto_ai_analysis must be a boolean")
		}
	}

	notificationChannels, err := parseOpsAlertStringList(raw, "notification_channels", false, "")
	if err != nil {
		return nil, err
	}
	if len(notificationChannels) == 0 {
		notificationChannels = []string{"in_app"}
	}
	for _, channel := range notificationChannels {
		if _, ok := validOpsAlertNotificationChannelSet[channel]; !ok {
			return nil, fmt.Errorf("notification_channels must be one of: %s", strings.Join(validOpsAlertNotificationChannels, ", "))
		}
	}
	if containsString(notificationChannels, "none") && len(notificationChannels) > 1 {
		return nil, fmt.Errorf("notification_channels 选择 none 时不能同时选择其他方式")
	}

	silenceMinutes := 10
	if v, ok := raw["silence_minutes"]; ok {
		if err := json.Unmarshal(v, &silenceMinutes); err != nil {
			return nil, fmt.Errorf("silence_minutes must be an integer")
		}
	}
	if silenceMinutes < 0 || silenceMinutes > 1440 {
		return nil, fmt.Errorf("请输入 0～1440 的整数分钟")
	}

	description := ""
	if v, ok := raw["description"]; ok {
		if err := json.Unmarshal(v, &description); err != nil {
			return nil, fmt.Errorf("description must be a string")
		}
		if len([]rune(strings.TrimSpace(description))) > 500 {
			return nil, fmt.Errorf("最多 500 字")
		}
	}

	return &opsAlertRuleValidatedInput{
		Name:                       name,
		MetricType:                 "compound_rule",
		Operator:                   ">=",
		Threshold:                  float64(minFinalFailures),
		Severity:                   opsAlertSeverityFromTriggerLevel(triggerLevel),
		WindowMinutes:              windowMinutes,
		SustainedMinutes:           1,
		CooldownMinutes:            silenceMinutes,
		Enabled:                    enabled,
		NotifyEmail:                containsString(notificationChannels, "email"),
		RuleVersion:                "v2",
		ErrorCategories:            errorCategories,
		TriggerLevel:               triggerLevel,
		MinFinalFailures:           minFinalFailures,
		MinFailureRate:             minFailureRate,
		MinSampleCount:             minSampleCount,
		ImpactScope:                impactScope,
		RecoveredFluctuationPolicy: recoveredPolicy,
		MinRecoveredFluctuations:   minRecovered,
		AutoAIAnalysis:             autoAIAnalysis,
		NotificationChannels:       notificationChannels,
		SilenceMinutes:             silenceMinutes,
		MigrationState:             "normal",
	}, nil
}

func validateOpsAlertRuleLegacyPayload(raw map[string]json.RawMessage) (*opsAlertRuleValidatedInput, error) {
	requiredFields := []string{"name", "metric_type", "operator", "threshold"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			return nil, fmt.Errorf("%s is required", field)
		}
	}

	var name string
	if err := json.Unmarshal(raw["name"], &name); err != nil || strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	name = strings.TrimSpace(name)
	if l := len([]rune(name)); l < 2 || l > 50 {
		return nil, fmt.Errorf("规则名称需为 2～50 字")
	}

	var metricType string
	if err := json.Unmarshal(raw["metric_type"], &metricType); err != nil || strings.TrimSpace(metricType) == "" {
		return nil, fmt.Errorf("metric_type is required")
	}
	metricType = strings.TrimSpace(metricType)
	if _, ok := validOpsAlertMetricTypeSet[metricType]; !ok {
		return nil, fmt.Errorf("metric_type must be one of: %s", strings.Join(validOpsAlertMetricTypes, ", "))
	}

	var operator string
	if err := json.Unmarshal(raw["operator"], &operator); err != nil || strings.TrimSpace(operator) == "" {
		return nil, fmt.Errorf("operator is required")
	}
	operator = strings.TrimSpace(operator)
	if _, ok := validOpsAlertOperatorSet[operator]; !ok {
		return nil, fmt.Errorf("operator must be one of: %s", strings.Join(validOpsAlertOperators, ", "))
	}

	var threshold float64
	if err := json.Unmarshal(raw["threshold"], &threshold); err != nil {
		return nil, fmt.Errorf("threshold must be a number")
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return nil, fmt.Errorf("threshold must be a finite number")
	}
	if isPercentOrRateMetric(metricType) {
		if threshold < 0 || threshold > 100 {
			return nil, fmt.Errorf("threshold must be between 0 and 100 for metric_type %s", metricType)
		}
	} else if threshold < 0 {
		return nil, fmt.Errorf("threshold must be >= 0")
	}

	validated := &opsAlertRuleValidatedInput{
		Name:                       name,
		MetricType:                 metricType,
		Operator:                   operator,
		Threshold:                  threshold,
		RuleVersion:                "v1",
		ErrorCategories:            []string{},
		TriggerLevel:               "P2",
		MinFinalFailures:           1,
		MinFailureRate:             0,
		MinSampleCount:             1,
		ImpactScope:                map[string]int{},
		RecoveredFluctuationPolicy: "record_only",
		NotificationChannels:       []string{"in_app"},
		SilenceMinutes:             0,
		MigrationState:             "readonly_legacy",
	}

	if v, ok := raw["severity"]; ok {
		var sev string
		if err := json.Unmarshal(v, &sev); err != nil {
			return nil, fmt.Errorf("severity must be a string")
		}
		sev = strings.ToUpper(strings.TrimSpace(sev))
		if sev != "" {
			if _, ok := validOpsAlertSeveritySet[sev]; !ok {
				return nil, fmt.Errorf("severity must be one of: %s", strings.Join(validOpsAlertSeverities, ", "))
			}
			validated.Severity = sev
			validated.TriggerLevel = triggerLevelFromOpsAlertSeverity(sev)
		}
	}
	if validated.Severity == "" {
		validated.Severity = "P2"
	}

	if v, ok := raw["enabled"]; ok {
		if err := json.Unmarshal(v, &validated.Enabled); err != nil {
			return nil, fmt.Errorf("enabled must be a boolean")
		}
	} else {
		validated.Enabled = true
	}

	if v, ok := raw["notify_email"]; ok {
		if err := json.Unmarshal(v, &validated.NotifyEmail); err != nil {
			return nil, fmt.Errorf("notify_email must be a boolean")
		}
	} else {
		validated.NotifyEmail = true
	}
	if validated.NotifyEmail {
		validated.NotificationChannels = []string{"in_app", "email"}
	}

	if v, ok := raw["window_minutes"]; ok {
		if err := json.Unmarshal(v, &validated.WindowMinutes); err != nil {
			return nil, fmt.Errorf("window_minutes must be an integer")
		}
		switch validated.WindowMinutes {
		case 1, 5, 60:
		default:
			return nil, fmt.Errorf("window_minutes must be one of: 1, 5, 60")
		}
	} else {
		validated.WindowMinutes = 1
	}

	if v, ok := raw["sustained_minutes"]; ok {
		if err := json.Unmarshal(v, &validated.SustainedMinutes); err != nil {
			return nil, fmt.Errorf("sustained_minutes must be an integer")
		}
		if validated.SustainedMinutes < 1 || validated.SustainedMinutes > 1440 {
			return nil, fmt.Errorf("sustained_minutes must be between 1 and 1440")
		}
	} else {
		validated.SustainedMinutes = 1
	}

	if v, ok := raw["cooldown_minutes"]; ok {
		if err := json.Unmarshal(v, &validated.CooldownMinutes); err != nil {
			return nil, fmt.Errorf("cooldown_minutes must be an integer")
		}
		if validated.CooldownMinutes < 0 || validated.CooldownMinutes > 1440 {
			return nil, fmt.Errorf("cooldown_minutes must be between 0 and 1440")
		}
	} else {
		validated.CooldownMinutes = 0
	}
	validated.SilenceMinutes = validated.CooldownMinutes

	return validated, nil
}

func parseOpsAlertRequiredString(raw map[string]json.RawMessage, field string, message string) (string, error) {
	v, ok := raw[field]
	if !ok {
		return "", fmt.Errorf("%s", message)
	}
	var out string
	if err := json.Unmarshal(v, &out); err != nil || strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("%s", message)
	}
	return strings.TrimSpace(out), nil
}

func parseOpsAlertRequiredInt(raw map[string]json.RawMessage, field string, message string) (int, error) {
	v, ok := raw[field]
	if !ok {
		return 0, fmt.Errorf("%s", message)
	}
	var out int
	if err := json.Unmarshal(v, &out); err != nil {
		return 0, fmt.Errorf("%s must be an integer", field)
	}
	return out, nil
}

func parseOpsAlertRequiredFloat(raw map[string]json.RawMessage, field string, message string) (float64, error) {
	v, ok := raw[field]
	if !ok {
		return 0, fmt.Errorf("%s", message)
	}
	var out float64
	if err := json.Unmarshal(v, &out); err != nil || math.IsNaN(out) || math.IsInf(out, 0) {
		return 0, fmt.Errorf("%s must be a finite number", field)
	}
	return out, nil
}

func parseOpsAlertStringList(raw map[string]json.RawMessage, field string, required bool, requiredMessage string) ([]string, error) {
	v, ok := raw[field]
	if !ok {
		if required {
			return nil, fmt.Errorf("%s", requiredMessage)
		}
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal(v, &values); err != nil {
		return nil, fmt.Errorf("%s must be a string array", field)
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if required && len(out) == 0 {
		return nil, fmt.Errorf("%s", requiredMessage)
	}
	return out, nil
}

func parseOpsAlertImpactScope(raw json.RawMessage) (map[string]int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]int{}, nil
	}
	var scope map[string]int
	if err := json.Unmarshal(raw, &scope); err != nil {
		return nil, fmt.Errorf("impact_scope must be an object")
	}
	validKeys := map[string]struct{}{
		"affected_users":             {},
		"affected_api_keys":          {},
		"affected_groups":            {},
		"affected_models":            {},
		"affected_upstream_accounts": {},
	}
	out := map[string]int{}
	for key, value := range scope {
		if _, ok := validKeys[key]; !ok {
			return nil, fmt.Errorf("impact_scope contains unsupported key: %s", key)
		}
		if value < 1 || value > 100000 {
			return nil, fmt.Errorf("impact_scope values must be between 1 and 100000")
		}
		out[key] = value
	}
	return out, nil
}

func normalizeOpsAlertTriggerLevel(v string) string {
	v = strings.TrimSpace(v)
	if strings.EqualFold(v, "observe") || v == "观察" {
		return "observe"
	}
	return strings.ToUpper(v)
}

func opsAlertSeverityFromTriggerLevel(level string) string {
	if level == "observe" {
		return "P3"
	}
	return level
}

func triggerLevelFromOpsAlertSeverity(severity string) string {
	if strings.EqualFold(severity, "P3") {
		return "observe"
	}
	return strings.ToUpper(strings.TrimSpace(severity))
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// ListAlertRules returns all ops alert rules.
// GET /api/v1/admin/ops/alert-rules
func (h *OpsHandler) ListAlertRules(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	rules, err := h.opsService.ListAlertRules(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rules)
}

// CreateAlertRule creates an ops alert rule.
// POST /api/v1/admin/ops/alert-rules
func (h *OpsHandler) CreateAlertRule(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var raw map[string]json.RawMessage
	if err := c.ShouldBindBodyWith(&raw, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	validated, err := validateOpsAlertRulePayload(raw)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var rule service.OpsAlertRule
	if err := c.ShouldBindBodyWith(&rule, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	rule.Name = validated.Name
	rule.MetricType = validated.MetricType
	rule.Operator = validated.Operator
	rule.Threshold = validated.Threshold
	rule.WindowMinutes = validated.WindowMinutes
	rule.SustainedMinutes = validated.SustainedMinutes
	rule.CooldownMinutes = validated.CooldownMinutes
	rule.Severity = validated.Severity
	rule.Enabled = validated.Enabled
	rule.NotifyEmail = validated.NotifyEmail
	rule.RuleVersion = validated.RuleVersion
	rule.ErrorCategories = validated.ErrorCategories
	rule.TriggerLevel = validated.TriggerLevel
	rule.MinFinalFailures = validated.MinFinalFailures
	rule.MinFailureRate = validated.MinFailureRate
	rule.MinSampleCount = validated.MinSampleCount
	rule.ImpactScope = validated.ImpactScope
	rule.RecoveredFluctuationPolicy = validated.RecoveredFluctuationPolicy
	rule.MinRecoveredFluctuations = validated.MinRecoveredFluctuations
	rule.AutoAIAnalysis = validated.AutoAIAnalysis
	rule.NotificationChannels = validated.NotificationChannels
	rule.SilenceMinutes = validated.SilenceMinutes
	rule.MigrationState = validated.MigrationState

	created, err := h.opsService.CreateAlertRule(c.Request.Context(), &rule)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, created)
}

// UpdateAlertRule updates an existing ops alert rule.
// PUT /api/v1/admin/ops/alert-rules/:id
func (h *OpsHandler) UpdateAlertRule(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rule ID")
		return
	}

	var raw map[string]json.RawMessage
	if err := c.ShouldBindBodyWith(&raw, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	validated, err := validateOpsAlertRulePayload(raw)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var rule service.OpsAlertRule
	if err := c.ShouldBindBodyWith(&rule, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	rule.ID = id
	rule.Name = validated.Name
	rule.MetricType = validated.MetricType
	rule.Operator = validated.Operator
	rule.Threshold = validated.Threshold
	rule.WindowMinutes = validated.WindowMinutes
	rule.SustainedMinutes = validated.SustainedMinutes
	rule.CooldownMinutes = validated.CooldownMinutes
	rule.Severity = validated.Severity
	rule.Enabled = validated.Enabled
	rule.NotifyEmail = validated.NotifyEmail
	rule.RuleVersion = validated.RuleVersion
	rule.ErrorCategories = validated.ErrorCategories
	rule.TriggerLevel = validated.TriggerLevel
	rule.MinFinalFailures = validated.MinFinalFailures
	rule.MinFailureRate = validated.MinFailureRate
	rule.MinSampleCount = validated.MinSampleCount
	rule.ImpactScope = validated.ImpactScope
	rule.RecoveredFluctuationPolicy = validated.RecoveredFluctuationPolicy
	rule.MinRecoveredFluctuations = validated.MinRecoveredFluctuations
	rule.AutoAIAnalysis = validated.AutoAIAnalysis
	rule.NotificationChannels = validated.NotificationChannels
	rule.SilenceMinutes = validated.SilenceMinutes
	rule.MigrationState = validated.MigrationState

	updated, err := h.opsService.UpdateAlertRule(c.Request.Context(), &rule)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

// DeleteAlertRule deletes an ops alert rule.
// DELETE /api/v1/admin/ops/alert-rules/:id
func (h *OpsHandler) DeleteAlertRule(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid rule ID")
		return
	}

	if err := h.opsService.DeleteAlertRule(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// GetAlertEvent returns a single ops alert event.
// GET /api/v1/admin/ops/alert-events/:id
func (h *OpsHandler) GetAlertEvent(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid event ID")
		return
	}

	ev, err := h.opsService.GetAlertEventByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, ev)
}

// UpdateAlertEventStatus updates an ops alert event status.
// PUT /api/v1/admin/ops/alert-events/:id/status
func (h *OpsHandler) UpdateAlertEventStatus(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid event ID")
		return
	}

	var payload struct {
		Status           string `json:"status"`
		Note             string `json:"note"`
		ProcessingAction string `json:"processing_action"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	payload.Status = strings.TrimSpace(payload.Status)
	if payload.Status == "" {
		response.BadRequest(c, "Invalid status")
		return
	}
	status := service.NormalizeOpsAlertLifecycleStatusForAPI(payload.Status)
	if !service.IsValidOpsAlertLifecycleStatusForAPI(status) {
		response.BadRequest(c, "Invalid status")
		return
	}

	var resolvedAt *time.Time
	if status == service.OpsAlertStatusRecovered || status == service.OpsAlertStatusClosed {
		now := time.Now().UTC()
		resolvedAt = &now
	}
	operatorID := (*int64)(nil)
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		uid := subject.UserID
		operatorID = &uid
	}
	note := strings.TrimSpace(payload.Note)
	processingAction := strings.TrimSpace(payload.ProcessingAction)
	if err := h.opsService.UpdateAlertEventStatus(c.Request.Context(), id, status, note, processingAction, operatorID, resolvedAt); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"updated": true})
}

// ListAlertEvents lists recent ops alert events.
// GET /api/v1/admin/ops/alert-events
// CreateAlertSilence creates a scoped silence for ops alerts.
// POST /api/v1/admin/ops/alert-silences
func (h *OpsHandler) CreateAlertSilence(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var payload struct {
		RuleID   int64   `json:"rule_id"`
		Platform string  `json:"platform"`
		GroupID  *int64  `json:"group_id"`
		Region   *string `json:"region"`
		Until    string  `json:"until"`
		Reason   string  `json:"reason"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.Until))
	if err != nil {
		response.BadRequest(c, "Invalid until")
		return
	}

	createdBy := (*int64)(nil)
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		uid := subject.UserID
		createdBy = &uid
	}

	silence := &service.OpsAlertSilence{
		RuleID:    payload.RuleID,
		Platform:  strings.TrimSpace(payload.Platform),
		GroupID:   payload.GroupID,
		Region:    payload.Region,
		Until:     until,
		Reason:    strings.TrimSpace(payload.Reason),
		CreatedBy: createdBy,
	}

	created, err := h.opsService.CreateAlertSilence(c.Request.Context(), silence)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, created)
}

func (h *OpsHandler) ListAlertEvents(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			response.BadRequest(c, "Invalid limit")
			return
		}
		limit = n
	}

	filter := &service.OpsAlertEventFilter{
		Limit:    limit,
		Status:   strings.TrimSpace(c.Query("status")),
		Severity: strings.TrimSpace(c.Query("severity")),
	}

	if v := strings.TrimSpace(c.Query("email_sent")); v != "" {
		vv := strings.ToLower(v)
		switch vv {
		case "true", "1":
			b := true
			filter.EmailSent = &b
		case "false", "0":
			b := false
			filter.EmailSent = &b
		default:
			response.BadRequest(c, "Invalid email_sent")
			return
		}
	}

	// Cursor pagination: both params must be provided together.
	rawTS := strings.TrimSpace(c.Query("before_fired_at"))
	rawID := strings.TrimSpace(c.Query("before_id"))
	if (rawTS == "") != (rawID == "") {
		response.BadRequest(c, "before_fired_at and before_id must be provided together")
		return
	}
	if rawTS != "" {
		ts, err := time.Parse(time.RFC3339Nano, rawTS)
		if err != nil {
			if t2, err2 := time.Parse(time.RFC3339, rawTS); err2 == nil {
				ts = t2
			} else {
				response.BadRequest(c, "Invalid before_fired_at")
				return
			}
		}
		filter.BeforeFiredAt = &ts
	}
	if rawID != "" {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid before_id")
			return
		}
		filter.BeforeID = &id
	}

	// Optional global filter support (platform/group/time range).
	if platform := strings.TrimSpace(c.Query("platform")); platform != "" {
		filter.Platform = platform
	}
	if v := strings.TrimSpace(c.Query("group_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &id
	}
	if startTime, endTime, err := parseOpsTimeRange(c, "24h"); err == nil {
		// Only apply when explicitly provided to avoid surprising default narrowing.
		if strings.TrimSpace(c.Query("start_time")) != "" || strings.TrimSpace(c.Query("end_time")) != "" || strings.TrimSpace(c.Query("time_range")) != "" {
			filter.StartTime = &startTime
			filter.EndTime = &endTime
		}
	} else {
		response.BadRequest(c, err.Error())
		return
	}

	events, err := h.opsService.ListAlertEvents(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, events)
}
