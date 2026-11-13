package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func resetOpsErrorLoggerStateForTest(t *testing.T) {
	t.Helper()

	opsErrorLogMu.Lock()
	ch := opsErrorLogQueue
	opsErrorLogQueue = nil
	opsErrorLogStopping = true
	opsErrorLogMu.Unlock()

	if ch != nil {
		close(ch)
	}
	opsErrorLogWorkersWg.Wait()

	opsErrorLogOnce = sync.Once{}
	opsErrorLogStopOnce = sync.Once{}
	opsErrorLogWorkersWg = sync.WaitGroup{}
	opsErrorLogMu = sync.RWMutex{}
	opsErrorLogStopping = false

	opsErrorLogQueueLen.Store(0)
	opsErrorLogEnqueued.Store(0)
	opsErrorLogDropped.Store(0)
	opsErrorLogProcessed.Store(0)
	opsErrorLogSanitized.Store(0)
	opsErrorLogLastDropLogAt.Store(0)

	opsErrorLogShutdownCh = make(chan struct{})
	opsErrorLogShutdownOnce = sync.Once{}
	opsErrorLogDrained.Store(false)
}

func TestFillOpsAccountFromUpstreamEvents_BackfillsMissingAccount(t *testing.T) {
	entry := &service.OpsInsertErrorLogInput{}
	events := []*service.OpsUpstreamErrorEvent{
		{AccountID: 0, Platform: "anthropic"},
		{AccountID: 42, Platform: "openai"},
	}

	fillOpsAccountFromUpstreamEvents(entry, events)

	require.NotNil(t, entry.AccountID)
	require.Equal(t, int64(42), *entry.AccountID)
	require.Equal(t, "openai", entry.Platform)
}

func TestFillOpsAccountFromUpstreamEvents_PreservesExistingAccount(t *testing.T) {
	existing := int64(7)
	entry := &service.OpsInsertErrorLogInput{AccountID: &existing, Platform: "anthropic"}
	events := []*service.OpsUpstreamErrorEvent{{AccountID: 42, Platform: "openai"}}

	fillOpsAccountFromUpstreamEvents(entry, events)

	require.NotNil(t, entry.AccountID)
	require.Equal(t, int64(7), *entry.AccountID)
	require.Equal(t, "anthropic", entry.Platform)
}

func TestEnqueueOpsErrorLog_QueueFullDrop(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)

	// 禁止 enqueueOpsErrorLog 触发 workers，使用测试队列验证满队列降级。
	opsErrorLogOnce.Do(func() {})

	opsErrorLogMu.Lock()
	opsErrorLogQueue = make(chan opsErrorLogJob, 1)
	opsErrorLogMu.Unlock()

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entry := &service.OpsInsertErrorLogInput{ErrorPhase: "upstream", ErrorType: "upstream_error"}

	enqueueOpsErrorLog(ops, entry)
	enqueueOpsErrorLog(ops, entry)

	require.Equal(t, int64(1), OpsErrorLogEnqueuedTotal())
	require.Equal(t, int64(1), OpsErrorLogDroppedTotal())
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
}

func TestEnqueueOpsErrorLog_EarlyReturnBranches(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entry := &service.OpsInsertErrorLogInput{ErrorPhase: "upstream", ErrorType: "upstream_error"}

	// nil 入参分支
	enqueueOpsErrorLog(nil, entry)
	enqueueOpsErrorLog(ops, nil)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// shutdown 分支
	close(opsErrorLogShutdownCh)
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// stopping 分支
	resetOpsErrorLoggerStateForTest(t)
	opsErrorLogMu.Lock()
	opsErrorLogStopping = true
	opsErrorLogMu.Unlock()
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// queue nil 分支（防止启动 worker 干扰）
	resetOpsErrorLoggerStateForTest(t)
	opsErrorLogOnce.Do(func() {})
	opsErrorLogMu.Lock()
	opsErrorLogQueue = nil
	opsErrorLogMu.Unlock()
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())
}

func TestOpsCaptureWriterPool_ResetOnRelease(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	writer := acquireOpsCaptureWriter(c.Writer)
	require.NotNil(t, writer)
	_, err := writer.buf.WriteString("temp-error-body")
	require.NoError(t, err)

	releaseOpsCaptureWriter(writer)

	reused := acquireOpsCaptureWriter(c.Writer)
	defer releaseOpsCaptureWriter(reused)

	require.Zero(t, reused.buf.Len(), "writer should be reset before reuse")
}

func TestOpsErrorLoggerMiddleware_DoesNotBreakOuterMiddlewares(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware2.Recovery())
	r.Use(middleware2.RequestLogger())
	r.Use(middleware2.Logger())
	r.GET("/v1/messages", OpsErrorLoggerMiddleware(nil), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)

	require.NotPanics(t, func() {
		r.ServeHTTP(rec, req)
	})
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestOpsErrorLoggerMiddleware_WritesRequestBodyDiagnosticsToErrorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetOpsErrorLoggerStateForTest(t)
	t.Cleanup(func() {
		resetOpsErrorLoggerStateForTest(t)
	})

	captured := make(chan *service.OpsInsertErrorLogInput, 1)
	ops := service.NewOpsService(&opsErrorLoggerRepoStub{
		insertErrorLogFn: func(ctx context.Context, input *service.OpsInsertErrorLogInput) (int64, error) {
			captured <- input
			return 1, nil
		},
	}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	r := gin.New()
	r.Use(OpsErrorLoggerMiddleware(ops))
	r.POST("/v1/responses", func(c *gin.Context) {
		setOpsRequestBodyDiagnostics(c, &httputil.RequestBodyReadError{
			Kind:          httputil.RequestBodyReadIncompleteBody,
			BytesRead:     12,
			ContentLength: 128,
			Err:           context.Canceled,
		}, 35*time.Millisecond)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "Request body is incomplete",
			},
		})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var entry *service.OpsInsertErrorLogInput
	select {
	case entry = <-captured:
	case <-time.After(time.Second):
		t.Fatal("等待 ops error log 写入超时")
	}
	require.NotNil(t, entry)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(entry.ErrorBody), &payload))
	diagnostics, ok := payload["diagnostics"].(map[string]any)
	require.True(t, ok, "missing diagnostics in error_body: %s", entry.ErrorBody)
	require.Equal(t, "incomplete_body", diagnostics["kind"])
	require.Equal(t, float64(12), diagnostics["bytes_read"])
	require.Equal(t, float64(128), diagnostics["content_length"])
	require.Equal(t, float64(35), diagnostics["read_duration_ms"])

	response, ok := payload["response"].(map[string]any)
	require.True(t, ok, "missing original response in error_body: %s", entry.ErrorBody)
	responseErr, ok := response["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "Request body is incomplete", responseErr["message"])
}

type opsErrorLoggerRepoStub struct {
	service.OpsRepository
	insertErrorLogFn func(ctx context.Context, input *service.OpsInsertErrorLogInput) (int64, error)
}

func (s *opsErrorLoggerRepoStub) InsertErrorLog(ctx context.Context, input *service.OpsInsertErrorLogInput) (int64, error) {
	if s.insertErrorLogFn != nil {
		return s.insertErrorLogFn(ctx, input)
	}
	return 1, nil
}

func TestIsKnownOpsErrorType(t *testing.T) {
	known := []string{
		"invalid_request_error",
		"authentication_error",
		"rate_limit_error",
		"billing_error",
		"subscription_error",
		"upstream_error",
		"overloaded_error",
		"api_error",
		"not_found_error",
		"forbidden_error",
	}
	for _, k := range known {
		require.True(t, isKnownOpsErrorType(k), "expected known: %s", k)
	}

	unknown := []string{"<nil>", "null", "", "random_error", "some_new_type", "<nil>\u003e"}
	for _, u := range unknown {
		require.False(t, isKnownOpsErrorType(u), "expected unknown: %q", u)
	}
}

func TestNormalizeOpsErrorType(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		code    string
		want    string
	}{
		// Known types pass through.
		{"known invalid_request_error", "invalid_request_error", "", "invalid_request_error"},
		{"known rate_limit_error", "rate_limit_error", "", "rate_limit_error"},
		{"known upstream_error", "upstream_error", "", "upstream_error"},

		// Unknown/garbage types are rejected and fall through to code-based or default.
		{"nil literal from upstream", "<nil>", "", "api_error"},
		{"null string", "null", "", "api_error"},
		{"random string", "something_weird", "", "api_error"},

		// Unknown type but known code still maps correctly.
		{"nil with INSUFFICIENT_BALANCE code", "<nil>", "INSUFFICIENT_BALANCE", "billing_error"},
		{"nil with USAGE_LIMIT_EXCEEDED code", "<nil>", "USAGE_LIMIT_EXCEEDED", "subscription_error"},

		// Empty type falls through to code-based mapping.
		{"empty type with balance code", "", "INSUFFICIENT_BALANCE", "billing_error"},
		{"empty type with subscription code", "", "SUBSCRIPTION_NOT_FOUND", "subscription_error"},
		{"empty type no code", "", "", "api_error"},

		// Known type overrides conflicting code-based mapping.
		{"known type overrides conflicting code", "rate_limit_error", "INSUFFICIENT_BALANCE", "rate_limit_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeOpsErrorType(tt.errType, tt.code)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestClassifyOpsNoAvailableAccountsExcludedFromSLA(t *testing.T) {
	const message = "No available accounts"
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	markOpsRoutingCapacityLimited(c)

	errType := normalizeOpsErrorType("api_error", "")
	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, message, "", http.StatusServiceUnavailable)

	require.Equal(t, "api_error", errType)
	require.Equal(t, "routing", phase)
	require.True(t, isBusinessLimited)
	require.Equal(t, "platform", errorOwner)
	require.Equal(t, "gateway", errorSource)
}

func TestClassifyOpsRoutingCapacityMarkerExcludesMaskedSelectionFailureFromSLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	markOpsRoutingCapacityLimited(c)

	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(
		c,
		"api_error",
		"Service temporarily unavailable",
		"",
		http.StatusServiceUnavailable,
	)

	require.Equal(t, "routing", phase)
	require.True(t, isBusinessLimited)
	require.Equal(t, "platform", errorOwner)
	require.Equal(t, "gateway", errorSource)
}

func TestClassifyOpsAuthClientErrorsExcludedFromSLA(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		message string
		code    string
		status  int
	}{
		{
			name:    "standard invalid API key",
			errType: "api_error",
			message: "Invalid API key",
			code:    "INVALID_API_KEY",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "standard missing API key",
			errType: "api_error",
			message: "API key is required in Authorization header (Bearer scheme), x-api-key header, or x-goog-api-key header",
			code:    "API_KEY_REQUIRED",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "expired local API key",
			errType: "api_error",
			message: "API key 已过期",
			code:    "API_KEY_EXPIRED",
			status:  http.StatusForbidden,
		},
		{
			name:    "disabled local API key",
			errType: "api_error",
			message: "API key is disabled",
			code:    "API_KEY_DISABLED",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "local API key user missing",
			errType: "api_error",
			message: "User associated with API key not found",
			code:    "USER_NOT_FOUND",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "inactive local API key user",
			errType: "api_error",
			message: "User account is not active",
			code:    "USER_INACTIVE",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "google invalid API key",
			errType: "api_error",
			message: "Invalid API key",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "google missing API key",
			errType: "api_error",
			message: "API key is required",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "google disabled API key",
			errType: "api_error",
			message: "API key is disabled",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "google local API key user missing",
			errType: "api_error",
			message: "User associated with API key not found",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "google inactive local API key user",
			errType: "api_error",
			message: "User account is not active",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)

			errType := normalizeOpsErrorType(tt.errType, tt.code)
			phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, tt.message, tt.code, tt.status)

			require.Equal(t, "api_error", errType)
			require.Equal(t, "auth", phase)
			require.True(t, isBusinessLimited)
			require.Equal(t, "client", errorOwner)
			require.Equal(t, "client_request", errorSource)
		})
	}
}

func TestClassifyOpsLocalBusinessLimitErrorsExcludedFromSLA(t *testing.T) {
	tests := []struct {
		name        string
		errType     string
		message     string
		code        string
		status      int
		wantErrType string
		wantPhase   string
	}{
		{
			name:        "standard API key quota exhausted",
			errType:     "api_error",
			message:     "API key 额度已用完",
			code:        "API_KEY_QUOTA_EXHAUSTED",
			status:      http.StatusTooManyRequests,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "standard query API key deprecated",
			errType:     "api_error",
			message:     "API key in query parameter is deprecated. Please use Authorization header instead.",
			code:        "api_key_in_query_deprecated",
			status:      http.StatusBadRequest,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "google query API key deprecated",
			errType:     "api_error",
			message:     "Query parameter api_key is deprecated. Use Authorization header or key instead.",
			code:        "400",
			status:      http.StatusBadRequest,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "google no active subscription",
			errType:     "api_error",
			message:     "No active subscription found for this group",
			code:        "403",
			status:      http.StatusForbidden,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "google insufficient account balance",
			errType:     "api_error",
			message:     "Insufficient account balance",
			code:        "403",
			status:      http.StatusForbidden,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "gateway billing cache insufficient balance",
			errType:     "billing_error",
			message:     "insufficient balance",
			code:        "",
			status:      http.StatusForbidden,
			wantErrType: "billing_error",
			wantPhase:   "request",
		},
		{
			name:        "gemini group platform mismatch",
			errType:     "api_error",
			message:     "API key group platform is not gemini",
			code:        "400",
			status:      http.StatusBadRequest,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "gateway API key 5h rate limit",
			errType:     "api_error",
			message:     "api key 5小时限额已用完",
			code:        "rate_limit_exceeded",
			status:      http.StatusTooManyRequests,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "gateway group RPM limit",
			errType:     "api_error",
			message:     "group requests-per-minute limit exceeded",
			code:        "rate_limit_exceeded",
			status:      http.StatusTooManyRequests,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
		{
			name:        "google subscription daily limit",
			errType:     "api_error",
			message:     "daily usage limit exceeded",
			code:        "429",
			status:      http.StatusTooManyRequests,
			wantErrType: "api_error",
			wantPhase:   "request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)

			errType := normalizeOpsErrorType(tt.errType, tt.code)
			phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, tt.message, tt.code, tt.status)

			require.Equal(t, tt.wantErrType, errType)
			require.Equal(t, tt.wantPhase, phase)
			require.True(t, isBusinessLimited)
			require.Equal(t, "client", errorOwner)
			require.Equal(t, "client_request", errorSource)
		})
	}
}

func TestClassifyOpsIPRestrictionAccessDeniedExcludedFromSLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonIPRestriction)

	errType := normalizeOpsErrorType("api_error", "ACCESS_DENIED")
	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, "Access denied", "ACCESS_DENIED", http.StatusForbidden)

	require.Equal(t, "api_error", errType)
	require.Equal(t, "auth", phase)
	require.True(t, isBusinessLimited)
	require.Equal(t, "client", errorOwner)
	require.Equal(t, "client_request", errorSource)
}

func TestClassifyOpsOtherErrorsStillCountForSLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	errType := normalizeOpsErrorType("api_error", "INTERNAL_ERROR")
	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, "Failed to validate API key", "INTERNAL_ERROR", http.StatusInternalServerError)

	require.Equal(t, "api_error", errType)
	require.Equal(t, "internal", phase)
	require.False(t, isBusinessLimited)
	require.Equal(t, "platform", errorOwner)
	require.Equal(t, "gateway", errorSource)
}

func TestClassifyOpsUnsupportedModelExcludedFromSLA(t *testing.T) {
	tests := []string{
		"No available accounts: no available accounts supporting model: made-up-model",
		"No available accounts: no available OpenAI accounts supporting model: made-up-model",
		"No available Gemini accounts: no available Gemini accounts supporting model: made-up-model",
		"No available accounts: no available accounts supporting model: made-up-model (channel pricing restriction)",
	}

	for _, message := range tests {
		t.Run(message, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			markOpsRoutingCapacityLimited(c)

			errType := normalizeOpsErrorType("api_error", "")
			phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(c, errType, message, "", http.StatusServiceUnavailable)

			require.Equal(t, "api_error", errType)
			require.Equal(t, "routing", phase)
			require.True(t, isBusinessLimited)
			require.Equal(t, "platform", errorOwner)
			require.Equal(t, "gateway", errorSource)
		})
	}
}

func TestClassifyOpsUnmarkedNoAvailableTextStillCountsForSLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(
		c,
		"api_error",
		"No available accounts",
		"",
		http.StatusServiceUnavailable,
	)

	require.Equal(t, "routing", phase)
	require.False(t, isBusinessLimited)
	require.Equal(t, "platform", errorOwner)
	require.Equal(t, "gateway", errorSource)
}

func TestClassifyOpsUpstreamAuthTextStillCountsForSLA(t *testing.T) {
	tests := []struct {
		name    string
		message string
		code    string
		status  int
	}{
		{
			name:    "invalid API key",
			message: "Invalid API key",
			code:    "401",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "disabled API key",
			message: "API key is disabled",
			code:    "API_KEY_DISABLED",
			status:  http.StatusUnauthorized,
		},
		{
			name:    "gemini group platform mismatch",
			message: "API key group platform is not gemini",
			code:    "400",
			status:  http.StatusBadRequest,
		},
		{
			name:    "provider balance error",
			message: "Insufficient account balance",
			code:    "INSUFFICIENT_BALANCE",
			status:  http.StatusForbidden,
		},
		{
			name:    "provider subscription error",
			message: "No active subscription found for this group",
			code:    "SUBSCRIPTION_NOT_FOUND",
			status:  http.StatusForbidden,
		},
		{
			name:    "provider quota error",
			message: "api key 额度已用完",
			code:    "API_KEY_QUOTA_EXHAUSTED",
			status:  http.StatusTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			service.SetOpsUpstreamError(c, tt.status, tt.message, "")

			phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(
				c,
				"api_error",
				tt.message,
				tt.code,
				tt.status,
			)

			require.Equal(t, "upstream", phase)
			require.False(t, isBusinessLimited)
			require.Equal(t, "provider", errorOwner)
			require.Equal(t, "upstream_http", errorSource)
		})
	}
}

func TestClassifyOpsUpstreamNoAvailableTextStillCountsForSLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	service.SetOpsUpstreamError(c, http.StatusServiceUnavailable, "No available accounts", "")

	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(
		c,
		"api_error",
		"No available accounts",
		"",
		http.StatusServiceUnavailable,
	)

	require.Equal(t, "upstream", phase)
	require.False(t, isBusinessLimited)
	require.Equal(t, "provider", errorOwner)
	require.Equal(t, "upstream_http", errorSource)
}

func TestSetOpsEndpointContext_SetsContextKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	setOpsEndpointContext(c, "claude-3-5-sonnet-20241022", int16(2)) // stream

	v, ok := c.Get(opsUpstreamModelKey)
	require.True(t, ok)
	vStr, ok := v.(string)
	require.True(t, ok)
	require.Equal(t, "claude-3-5-sonnet-20241022", vStr)

	rt, ok := c.Get(opsRequestTypeKey)
	require.True(t, ok)
	rtVal, ok := rt.(int16)
	require.True(t, ok)
	require.Equal(t, int16(2), rtVal)
}

func TestSetOpsEndpointContext_EmptyModelNotStored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	setOpsEndpointContext(c, "", int16(1))

	_, ok := c.Get(opsUpstreamModelKey)
	require.False(t, ok, "empty upstream model should not be stored")

	rt, ok := c.Get(opsRequestTypeKey)
	require.True(t, ok)
	rtVal, ok := rt.(int16)
	require.True(t, ok)
	require.Equal(t, int16(1), rtVal)
}

func TestSetOpsEndpointContext_NilContext(t *testing.T) {
	require.NotPanics(t, func() {
		setOpsEndpointContext(nil, "model", int16(1))
	})
}

func setupGeminiOpsErrorLogTestQueue(t *testing.T) {
	t.Helper()
	resetOpsErrorLoggerStateForTest(t)
	t.Cleanup(func() { resetOpsErrorLoggerStateForTest(t) })
	opsErrorLogOnce.Do(func() {})
	opsErrorLogMu.Lock()
	opsErrorLogQueue = make(chan opsErrorLogJob, 4)
	opsErrorLogMu.Unlock()
}

// 带内错误也可能出现在非流式 2xx 响应体里（如 Gemini generateContent 的 finishReason）：
// NonStream 标记落库 stream=false，未标记的沿用流式。
func TestLogOpsStreamError_NonStreamInBandContentPolicy(t *testing.T) {
	setupGeminiOpsErrorLogTestQueue(t)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3.7-flash:generateContent", nil)
	setOpsRequestContext(c, "gemini-3.7-flash", false)

	service.MarkOpsStreamErrorValue(c, service.OpsStreamError{
		ErrType:        "invalid_request_error",
		Code:           "PROHIBITED_CONTENT",
		Message:        "Gemini content policy stop (finishReason=PROHIBITED_CONTENT): Response stopped due to prohibited content.",
		IntendedStatus: http.StatusBadRequest,
		RequestScoped:  true,
		NonStream:      true,
	})

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	logOpsStreamError(c, ops, http.StatusOK)

	job := <-opsErrorLogQueue
	require.NotNil(t, job.entry)
	require.False(t, job.entry.Stream)
	require.Equal(t, "invalid_request_error", job.entry.ErrorType)
	require.Equal(t, "request", job.entry.ErrorPhase)
	require.Equal(t, "client", job.entry.ErrorOwner)
	require.True(t, job.entry.IsBusinessLimited)
	require.Equal(t, http.StatusBadRequest, job.entry.StatusCode, "请求级带内结果按 IntendedStatus 落库，才能进入错误列表")
	require.Equal(t, "P3", job.entry.Severity)
	require.Equal(t, "gemini-3.7-flash", job.entry.Model)
	require.Contains(t, job.entry.ErrorBody, "PROHIBITED_CONTENT")
}

// 请求级带内结果（RequestScoped）：此前尝试残留的上游错误上下文不参与分类、不做上游归因，
// 按业务限制计；透传规则的 skip_monitoring 残留也不影响它落库。
func TestLogOpsStreamError_RequestScopedIgnoresResidualUpstreamContext(t *testing.T) {
	setupGeminiOpsErrorLogTestQueue(t)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3.7-flash:streamGenerateContent", nil)
	setOpsRequestContext(c, "gemini-3.7-flash", true)
	c.Set(service.OpsUpstreamStatusCodeKey, http.StatusTooManyRequests)
	c.Set(service.OpsUpstreamErrorMessageKey, "earlier attempt was rate limited")
	c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
		UpstreamStatusCode: http.StatusTooManyRequests,
		Message:            "earlier attempt was rate limited",
		SkipMonitoring:     true,
	}})

	service.MarkOpsStreamErrorValue(c, service.OpsStreamError{
		ErrType:        "invalid_request_error",
		Code:           "PROHIBITED_CONTENT",
		Message:        "Gemini content policy stop (finishReason=PROHIBITED_CONTENT): Response stopped due to prohibited content.",
		IntendedStatus: http.StatusBadRequest,
		RequestScoped:  true,
	})

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	logOpsStreamError(c, ops, http.StatusOK)

	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.NotNil(t, job.entry)
	require.Equal(t, "invalid_request_error", job.entry.ErrorType)
	require.Equal(t, "request", job.entry.ErrorPhase)
	require.Equal(t, "client", job.entry.ErrorOwner)
	require.Equal(t, "client_request", job.entry.ErrorSource)
	require.True(t, job.entry.IsBusinessLimited)
	require.True(t, job.entry.Stream)
	require.Equal(t, http.StatusBadRequest, job.entry.StatusCode)
	require.Nil(t, job.entry.UpstreamStatusCode, "残留的 429 不得贴到内容策略行上")
	require.Nil(t, job.entry.UpstreamErrorMessage)
	require.Nil(t, job.entry.UpstreamErrorsJSON)
	require.Contains(t, job.entry.ErrorBody, "PROHIBITED_CONTENT")
}

// 请求级带内结果与此前尝试的上游错误并存时，中间件既落带内错误行，也保留恢复行的 provider 遥测。
func TestOpsErrorLoggerMiddleware_RequestScopedInBandErrorKeepsRecoveredTelemetry(t *testing.T) {
	setupGeminiOpsErrorLogTestQueue(t)
	gin.SetMode(gin.TestMode)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1beta/models/gemini-3.7-flash:generateContent", func(c *gin.Context) {
		setOpsRequestContext(c, "gemini-3.7-flash", false)
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
			UpstreamStatusCode: http.StatusTooManyRequests,
			Message:            "earlier attempt was rate limited",
		}})
		service.MarkOpsStreamErrorValue(c, service.OpsStreamError{
			ErrType:        "invalid_request_error",
			Code:           "SAFETY",
			Message:        "Gemini content policy stop (finishReason=SAFETY): Response stopped due to safety reasons.",
			IntendedStatus: http.StatusBadRequest,
			RequestScoped:  true,
			NonStream:      true,
		})
		c.JSON(http.StatusOK, gin.H{"candidates": []any{}})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3.7-flash:generateContent", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	require.Equal(t, int64(2), OpsErrorLogQueueLength())
	inBand := <-opsErrorLogQueue
	require.Equal(t, "invalid_request_error", inBand.entry.ErrorType)
	require.Equal(t, "request", inBand.entry.ErrorPhase)
	require.Equal(t, "client", inBand.entry.ErrorOwner)
	require.True(t, inBand.entry.IsBusinessLimited)
	require.False(t, inBand.entry.Stream)
	require.Equal(t, http.StatusBadRequest, inBand.entry.StatusCode)
	require.Nil(t, inBand.entry.UpstreamStatusCode)
	require.Nil(t, inBand.entry.UpstreamErrorsJSON)

	recovered := <-opsErrorLogQueue
	require.Equal(t, "upstream", recovered.entry.ErrorPhase)
	require.Equal(t, "provider", recovered.entry.ErrorOwner)
	require.Equal(t, http.StatusOK, recovered.entry.StatusCode)
	require.Equal(t, "Recovered upstream error 429: earlier attempt was rate limited", recovered.entry.ErrorMessage)
}

// 非请求级带内失败（如上游错误信封）仍与既有语义一致：上游上下文参与分类并做归因，只落一行。
func TestOpsErrorLoggerMiddleware_UpstreamInBandFailureStillSingleRow(t *testing.T) {
	setupGeminiOpsErrorLogTestQueue(t)
	gin.SetMode(gin.TestMode)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(OpsErrorLoggerMiddleware(ops))
	router.POST("/v1beta/models/gemini-3.7-flash:streamGenerateContent", func(c *gin.Context) {
		setOpsRequestContext(c, "gemini-3.7-flash", true)
		service.SetOpsUpstreamError(c, http.StatusTooManyRequests, "quota", "")
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
			UpstreamStatusCode: http.StatusTooManyRequests,
			Message:            "quota",
			Kind:               "stream_failed",
		}})
		service.MarkOpsStreamFailure(c, "rate_limit_error", "RESOURCE_EXHAUSTED", "quota", http.StatusTooManyRequests)
		c.Data(http.StatusOK, "text/event-stream", []byte("data: {\"error\":{\"code\":429}}\n\n"))
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3.7-flash:streamGenerateContent", nil))

	require.Equal(t, int64(1), OpsErrorLogQueueLength())
	job := <-opsErrorLogQueue
	require.Equal(t, "rate_limit_error", job.entry.ErrorType)
	require.Equal(t, "upstream", job.entry.ErrorPhase)
	require.Equal(t, "provider", job.entry.ErrorOwner)
	require.Equal(t, http.StatusTooManyRequests, job.entry.StatusCode)
	require.True(t, job.entry.Stream)
	require.NotNil(t, job.entry.UpstreamStatusCode)
	require.Equal(t, http.StatusTooManyRequests, *job.entry.UpstreamStatusCode)
}

// Gemini 带内信号登记使用的错误类型必须都在 ops 白名单里，否则会被归一化成 api_error。
func TestNormalizeOpsErrorType_KeepsGeminiInBandSignalTypes(t *testing.T) {
	for _, errType := range []string{
		"invalid_request_error", "authentication_error", "permission_error",
		"not_found_error", "rate_limit_error", "upstream_error",
	} {
		require.Equal(t, errType, normalizeOpsErrorType(errType, "PROHIBITED_CONTENT"), errType)
	}
}
