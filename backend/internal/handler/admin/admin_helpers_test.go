package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseTimeRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/?start_date=2024-01-01&end_date=2024-01-02&timezone=UTC", nil)
	c.Request = req

	start, end := parseTimeRange(c)
	require.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), start)
	require.Equal(t, time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), end)

	req = httptest.NewRequest(http.MethodGet, "/?start_date=bad&timezone=UTC", nil)
	c.Request = req
	start, end = parseTimeRange(c)
	require.False(t, start.IsZero())
	require.False(t, end.IsZero())
}

func TestParseOpsViewParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?view=excluded", nil)
	require.Equal(t, opsListViewExcluded, parseOpsViewParam(c))

	c2, _ := gin.CreateTestContext(w)
	c2.Request = httptest.NewRequest(http.MethodGet, "/?view=all", nil)
	require.Equal(t, opsListViewAll, parseOpsViewParam(c2))

	c3, _ := gin.CreateTestContext(w)
	c3.Request = httptest.NewRequest(http.MethodGet, "/?view=unknown", nil)
	require.Equal(t, opsListViewErrors, parseOpsViewParam(c3))

	require.Equal(t, "", parseOpsViewParam(nil))
}

func TestParseOpsDuration(t *testing.T) {
	dur, ok := parseOpsDuration("1h")
	require.True(t, ok)
	require.Equal(t, time.Hour, dur)

	_, ok = parseOpsDuration("invalid")
	require.False(t, ok)
}

func TestParseOpsOpenAITokenStatsDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		ok    bool
	}{
		{input: "30m", want: 30 * time.Minute, ok: true},
		{input: "1h", want: time.Hour, ok: true},
		{input: "1d", want: 24 * time.Hour, ok: true},
		{input: "15d", want: 15 * 24 * time.Hour, ok: true},
		{input: "30d", want: 30 * 24 * time.Hour, ok: true},
		{input: "7d", want: 0, ok: false},
	}

	for _, tt := range tests {
		got, ok := parseOpsOpenAITokenStatsDuration(tt.input)
		require.Equal(t, tt.ok, ok, "input=%s", tt.input)
		require.Equal(t, tt.want, got, "input=%s", tt.input)
	}
}

func TestParseOpsOpenAITokenStatsFilter_Defaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	before := time.Now().UTC()
	filter, err := parseOpsOpenAITokenStatsFilter(c)
	after := time.Now().UTC()

	require.NoError(t, err)
	require.NotNil(t, filter)
	require.Equal(t, "30d", filter.TimeRange)
	require.Equal(t, 1, filter.Page)
	require.Equal(t, 20, filter.PageSize)
	require.Equal(t, 0, filter.TopN)
	require.Nil(t, filter.GroupID)
	require.Equal(t, "", filter.Platform)
	require.True(t, filter.StartTime.Before(filter.EndTime))
	require.WithinDuration(t, before.Add(-30*24*time.Hour), filter.StartTime, 2*time.Second)
	require.WithinDuration(t, after, filter.EndTime, 2*time.Second)
}

func TestParseOpsOpenAITokenStatsFilter_WithTopN(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/?time_range=1h&platform=openai&group_id=12&top_n=50",
		nil,
	)

	filter, err := parseOpsOpenAITokenStatsFilter(c)
	require.NoError(t, err)
	require.Equal(t, "1h", filter.TimeRange)
	require.Equal(t, "openai", filter.Platform)
	require.NotNil(t, filter.GroupID)
	require.Equal(t, int64(12), *filter.GroupID)
	require.Equal(t, 50, filter.TopN)
	require.Equal(t, 0, filter.Page)
	require.Equal(t, 0, filter.PageSize)
}

func TestParseOpsOpenAITokenStatsFilter_InvalidParams(t *testing.T) {
	tests := []string{
		"/?time_range=7d",
		"/?group_id=0",
		"/?group_id=abc",
		"/?top_n=0",
		"/?top_n=101",
		"/?top_n=10&page=1",
		"/?top_n=10&page_size=20",
		"/?page=0",
		"/?page_size=0",
		"/?page_size=101",
	}

	gin.SetMode(gin.TestMode)
	for _, rawURL := range tests {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, rawURL, nil)

		_, err := parseOpsOpenAITokenStatsFilter(c)
		require.Error(t, err, "url=%s", rawURL)
	}
}

func TestParseOpsTimeRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	now := time.Now().UTC()
	startStr := now.Add(-time.Hour).Format(time.RFC3339)
	endStr := now.Format(time.RFC3339)
	c.Request = httptest.NewRequest(http.MethodGet, "/?start_time="+startStr+"&end_time="+endStr, nil)
	start, end, err := parseOpsTimeRange(c, "1h")
	require.NoError(t, err)
	require.True(t, start.Before(end))

	c2, _ := gin.CreateTestContext(w)
	c2.Request = httptest.NewRequest(http.MethodGet, "/?start_time=bad", nil)
	_, _, err = parseOpsTimeRange(c2, "1h")
	require.Error(t, err)
}

func TestParseOpsRealtimeWindow(t *testing.T) {
	dur, label, ok := parseOpsRealtimeWindow("5m")
	require.True(t, ok)
	require.Equal(t, 5*time.Minute, dur)
	require.Equal(t, "5min", label)

	_, _, ok = parseOpsRealtimeWindow("invalid")
	require.False(t, ok)
}

func TestPickThroughputBucketSeconds(t *testing.T) {
	require.Equal(t, 60, pickThroughputBucketSeconds(30*time.Minute))
	require.Equal(t, 300, pickThroughputBucketSeconds(6*time.Hour))
	require.Equal(t, 3600, pickThroughputBucketSeconds(48*time.Hour))
}

func TestParseOpsQueryMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?mode=raw", nil)
	require.Equal(t, service.ParseOpsQueryMode("raw"), parseOpsQueryMode(c))
	require.Equal(t, service.OpsQueryMode(""), parseOpsQueryMode(nil))
}

func TestOpsAlertRuleValidation(t *testing.T) {
	raw := map[string]json.RawMessage{
		"name":        json.RawMessage(`"High error rate"`),
		"metric_type": json.RawMessage(`"error_rate"`),
		"operator":    json.RawMessage(`">"`),
		"threshold":   json.RawMessage(`90`),
	}

	validated, err := validateOpsAlertRulePayload(raw)
	require.NoError(t, err)
	require.Equal(t, "High error rate", validated.Name)

	_, err = validateOpsAlertRulePayload(map[string]json.RawMessage{})
	require.Error(t, err)

	require.True(t, isPercentOrRateMetric("error_rate"))
	require.False(t, isPercentOrRateMetric("concurrency_queue_depth"))
}

func TestOpsAlertRuleValidationLegacyLatencyMetrics(t *testing.T) {
	for _, metricType := range []string{"p95_latency_ms", "p99_latency_ms"} {
		t.Run(metricType, func(t *testing.T) {
			raw := map[string]json.RawMessage{
				"name":        json.RawMessage(`"High latency"`),
				"metric_type": json.RawMessage(`"` + metricType + `"`),
				"operator":    json.RawMessage(`">"`),
				"threshold":   json.RawMessage(`3000`),
			}
			validated, err := validateOpsAlertRulePayload(raw)
			require.NoError(t, err)
			require.Equal(t, metricType, validated.MetricType)
		})
	}
}

func TestOpsWSHelpers(t *testing.T) {
	prefixes, invalid := parseTrustedProxyList("10.0.0.0/8,invalid")
	require.Len(t, prefixes, 1)
	require.Len(t, invalid, 1)

	host := hostWithoutPort("example.com:443")
	require.Equal(t, "example.com", host)

	addr := netip.MustParseAddr("10.0.0.1")
	require.True(t, isAddrInTrustedProxies(addr, prefixes))
	require.False(t, isAddrInTrustedProxies(netip.MustParseAddr("192.168.0.1"), prefixes))
}

// TestOpenAIFastPolicySettingsFromDTO_NormalizesServiceTier 验证 admin
// 写入路径会把 ServiceTier 的空字符串/空白/大小写归一化为
// service.OpenAIFastTierAny ("all")，避免落盘时 "" 与 "all" 双语义。
func TestOpenAIFastPolicySettingsFromDTO_NormalizesServiceTier(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		require.Nil(t, openaiFastPolicySettingsFromDTO(nil))
	})

	t.Run("empty service_tier becomes 'all'", func(t *testing.T) {
		in := &dto.OpenAIFastPolicySettings{
			Rules: []dto.OpenAIFastPolicyRule{{
				ServiceTier: "",
				Action:      "filter",
				Scope:       "all",
			}},
		}
		out := openaiFastPolicySettingsFromDTO(in)
		require.NotNil(t, out)
		require.Len(t, out.Rules, 1)
		require.Equal(t, service.OpenAIFastTierAny, out.Rules[0].ServiceTier)
		require.Equal(t, "all", out.Rules[0].ServiceTier)
	})

	t.Run("whitespace-only service_tier becomes 'all'", func(t *testing.T) {
		in := &dto.OpenAIFastPolicySettings{
			Rules: []dto.OpenAIFastPolicyRule{{
				ServiceTier: "   ",
				Action:      "pass",
				Scope:       "all",
			}},
		}
		out := openaiFastPolicySettingsFromDTO(in)
		require.Equal(t, service.OpenAIFastTierAny, out.Rules[0].ServiceTier)
	})

	t.Run("uppercase service_tier is lowercased", func(t *testing.T) {
		in := &dto.OpenAIFastPolicySettings{
			Rules: []dto.OpenAIFastPolicyRule{{
				ServiceTier: "PRIORITY",
				Action:      "filter",
				Scope:       "all",
			}},
		}
		out := openaiFastPolicySettingsFromDTO(in)
		require.Equal(t, service.OpenAIFastTierPriority, out.Rules[0].ServiceTier)
	})

	t.Run("non-empty values pass through (lowercased)", func(t *testing.T) {
		in := &dto.OpenAIFastPolicySettings{
			Rules: []dto.OpenAIFastPolicyRule{
				{ServiceTier: "priority", Action: "filter", Scope: "all"},
				{ServiceTier: "flex", Action: "block", Scope: "oauth"},
				{ServiceTier: "all", Action: "pass", Scope: "apikey"},
			},
		}
		out := openaiFastPolicySettingsFromDTO(in)
		require.Len(t, out.Rules, 3)
		require.Equal(t, service.OpenAIFastTierPriority, out.Rules[0].ServiceTier)
		require.Equal(t, service.OpenAIFastTierFlex, out.Rules[1].ServiceTier)
		require.Equal(t, service.OpenAIFastTierAny, out.Rules[2].ServiceTier)
	})
}

func TestOpsAlertRuleValidationV2CompoundRule(t *testing.T) {
	raw := map[string]json.RawMessage{
		"name":                         json.RawMessage(`"上游账号集中失败"`),
		"enabled":                      json.RawMessage(`true`),
		"time_window":                  json.RawMessage(`"1m"`),
		"error_categories":             json.RawMessage(`["upstream","permission"]`),
		"trigger_level":                json.RawMessage(`"P1"`),
		"min_final_failures":           json.RawMessage(`5`),
		"min_failure_rate":             json.RawMessage(`10.5`),
		"min_sample_count":             json.RawMessage(`50`),
		"impact_scope":                 json.RawMessage(`{"affected_users":2,"affected_upstream_accounts":1}`),
		"recovered_fluctuation_policy": json.RawMessage(`"observe_only"`),
		"min_recovered_fluctuations":   json.RawMessage(`10`),
		"auto_ai_analysis":             json.RawMessage(`true`),
		"notification_channels":        json.RawMessage(`["in_app","email"]`),
		"silence_minutes":              json.RawMessage(`10`),
	}

	validated, err := validateOpsAlertRulePayload(raw)
	require.NoError(t, err)
	require.Equal(t, "v2", validated.RuleVersion)
	require.Equal(t, "P1", validated.TriggerLevel)
	require.Equal(t, "P1", validated.Severity)
	require.Equal(t, []string{"upstream", "permission"}, validated.ErrorCategories)
	require.Equal(t, 5, validated.MinFinalFailures)
	require.InDelta(t, 10.5, validated.MinFailureRate, 0.001)
	require.Equal(t, 50, validated.MinSampleCount)
	require.Equal(t, map[string]int{"affected_users": 2, "affected_upstream_accounts": 1}, validated.ImpactScope)
	require.True(t, validated.NotifyEmail)
}

func TestOpsAlertRuleValidationV2RejectsInvalidRules(t *testing.T) {
	base := map[string]json.RawMessage{
		"name":                         json.RawMessage(`"上游账号集中失败"`),
		"time_window":                  json.RawMessage(`"1m"`),
		"error_categories":             json.RawMessage(`["upstream"]`),
		"trigger_level":                json.RawMessage(`"P1"`),
		"min_final_failures":           json.RawMessage(`5`),
		"min_failure_rate":             json.RawMessage(`10`),
		"min_sample_count":             json.RawMessage(`50`),
		"recovered_fluctuation_policy": json.RawMessage(`"record_only"`),
		"notification_channels":        json.RawMessage(`["in_app"]`),
		"silence_minutes":              json.RawMessage(`10`),
	}

	cases := []struct {
		name  string
		patch map[string]json.RawMessage
		want  string
	}{
		{name: "empty categories", patch: map[string]json.RawMessage{"error_categories": json.RawMessage(`[]`)}, want: "请选择错误分类"},
		{name: "bad time window", patch: map[string]json.RawMessage{"time_window": json.RawMessage(`"5m"`)}, want: "本版本固定 1 分钟窗口"},
		{name: "bad trigger", patch: map[string]json.RawMessage{"trigger_level": json.RawMessage(`"P4"`)}, want: "trigger_level"},
		{name: "failures greater than sample", patch: map[string]json.RawMessage{"min_final_failures": json.RawMessage(`51`)}, want: "最小最终失败数不能大于最小样本量"},
		{name: "none with email", patch: map[string]json.RawMessage{"notification_channels": json.RawMessage(`["none","email"]`)}, want: "none"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			raw := map[string]json.RawMessage{}
			for k, v := range base {
				raw[k] = v
			}
			for k, v := range tt.patch {
				raw[k] = v
			}
			_, err := validateOpsAlertRulePayload(raw)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestParseOpsUnifiedErrorListFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	start := "2026-06-08T08:00:00Z"
	end := "2026-06-08T08:30:00Z"
	c.Request = httptest.NewRequest(http.MethodGet, "/?page=2&page_size=50&start_time="+start+"&end_time="+end+"&error_categories=client,balance&client_error_subcategories=client_parameter_error&error_results=final_failed&severity=P1,observe&status_code=400,500-502&user_id=6&api_key_id=12&group_id=3&platform=openai&model=gpt-5.5&upstream_account_id=88&request_id=req-1&keyword=rate&ai_analysis=not_analyzed&sort_by=status_code&sort_order=asc", nil)

	filter, err := parseOpsUnifiedErrorListFilter(c)
	require.NoError(t, err)
	require.Equal(t, 2, filter.Page)
	require.Equal(t, 50, filter.PageSize)
	require.Equal(t, []string{"client", "balance"}, filter.ErrorCategories)
	require.Equal(t, []string{"client_parameter_error"}, filter.ClientErrorSubcategories)
	require.Equal(t, []string{"final_failed"}, filter.ErrorResults)
	require.Equal(t, []string{"P1", "observe"}, filter.Severities)
	require.Equal(t, []int{400, 500, 501, 502}, filter.StatusCodes)
	require.NotNil(t, filter.UserID)
	require.Equal(t, int64(6), *filter.UserID)
	require.Equal(t, "openai", filter.Platform)
	require.Equal(t, "gpt-5.5", filter.Model)
	require.Equal(t, "req-1", filter.RequestID)
	require.Equal(t, "rate", filter.Keyword)
	require.Equal(t, service.OpsUnifiedAIAnalysisNotAnalyzed, filter.AIAnalysis)
	require.Equal(t, "status_code", filter.SortBy)
	require.Equal(t, "asc", filter.SortOrder)
}

func TestParseOpsUnifiedErrorListFilterRejectsInvalidInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []string{
		"/?page_size=10",
		"/?time_range=30d",
		"/?error_categories=legacy_error",
		"/?client_error_subcategories=client_unknown",
		"/?status_code=99",
		"/?status_code=600",
		"/?status_code=500-1200",
		"/?keyword=x",
		"/?ai_analysis=maybe",
		"/?sort_by=model",
		"/?sort_order=random",
	}
	for _, target := range tests {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, target, nil)
		_, err := parseOpsUnifiedErrorListFilter(c)
		require.Error(t, err, target)
	}
}

func TestParseOpsStatusCodeFilterBounds(t *testing.T) {
	codes, err := parseOpsStatusCodeFilter("100,599")
	require.NoError(t, err)
	require.Equal(t, []int{100, 599}, codes)

	codes, err = parseOpsStatusCodeFilter("200-202")
	require.NoError(t, err)
	require.Equal(t, []int{200, 201, 202}, codes)

	_, err = parseOpsStatusCodeFilter("99")
	require.Error(t, err)
	_, err = parseOpsStatusCodeFilter("600")
	require.Error(t, err)
	_, err = parseOpsStatusCodeFilter("99-100")
	require.Error(t, err)
	_, err = parseOpsStatusCodeFilter("599-600")
	require.Error(t, err)
}
