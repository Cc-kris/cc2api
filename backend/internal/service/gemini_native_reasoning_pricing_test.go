//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiNativeReasoningPricingUsesExplicitForwardedLevel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, provider := range []string{"api_key", "oauth_ai_studio", "oauth_code_assist"} {
		for _, stream := range []bool{false, true} {
			for _, tc := range []struct {
				name       string
				thinking   map[string]any
				wantEffort string
				multiplier float64
			}{
				{"high", map[string]any{"thinkingLevel": "high"}, "high", 2},
				{"uppercase_high", map[string]any{"thinkingLevel": "HIGH"}, "high", 2},
				{"snake_case_high", map[string]any{"thinking_level": "high"}, "high", 2},
				{"minimal", map[string]any{"thinkingLevel": "minimal"}, "minimal", 1.1},
				{"low", map[string]any{"thinkingLevel": "low"}, "low", 1.2},
				{"medium", map[string]any{"thinkingLevel": "medium"}, "medium", 1.5},
				{"missing_level", nil, "", 1},
				{"budget_only", map[string]any{"thinkingBudget": 24576}, "", 1},
				{"unspecified_level", map[string]any{"thinkingLevel": "THINKING_LEVEL_UNSPECIFIED"}, "", 1},
			} {
				mode := "buffered"
				if stream {
					mode = "streaming"
				}
				t.Run(provider+"/"+mode+"/"+tc.name, func(t *testing.T) {
					const model = "gemini-3.1-pro-high"
					request := map[string]any{
						"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": "hello"}}}},
					}
					if tc.name == "snake_case_high" {
						request["generation_config"] = map[string]any{"thinking_config": tc.thinking}
					} else if tc.thinking != nil {
						request["generationConfig"] = map[string]any{"thinkingConfig": tc.thinking}
					}
					body, err := json.Marshal(request)
					require.NoError(t, err)
					action := "generateContent"
					if stream {
						action = "streamGenerateContent"
					}
					path := "/v1beta/models/" + model + ":" + action
					c, recorder := newAntigravityCompatContext(http.MethodPost, path, body)
					responseBody := `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
					wrapped := provider == "oauth_code_assist" || provider == "antigravity"
					if wrapped {
						responseBody = `{"response":` + responseBody + `}`
					}
					contentType := "application/json"
					if stream || wrapped {
						contentType = "text/event-stream"
						responseBody = "data: " + responseBody + "\n\n"
					}
					upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{contentType}},
						Body:       io.NopCloser(strings.NewReader(responseBody)),
					}}}
					account := geminiSignalTestAccount()
					if provider != "api_key" {
						account.Type = AccountTypeOAuth
						account.Credentials["access_token"] = "test-token"
					}
					if provider == "oauth_code_assist" {
						account.Credentials["project_id"] = "test-project"
					}
					svc := &GeminiMessagesCompatService{
						httpUpstream: upstream, cfg: &config.Config{}, tokenProvider: &GeminiTokenProvider{},
					}
					result, err := svc.ForwardNative(context.Background(), c, account, model, action, stream, body)
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Len(t, upstream.requestBodies, 1)
					forwardedPath := "generationConfig.thinkingConfig.thinkingLevel"
					requestedLevel, _ := tc.thinking["thinkingLevel"].(string)
					if tc.name == "snake_case_high" {
						forwardedPath = "generation_config.thinking_config.thinking_level"
						requestedLevel, _ = tc.thinking["thinking_level"].(string)
					}
					if wrapped {
						forwardedPath = "request." + forwardedPath
					}
					require.Equal(t, requestedLevel, gjson.GetBytes(upstream.requestBodies[0], forwardedPath).String())
					require.Equal(t, tc.wantEffort, optionalStringValue(result.ReasoningEffort))
					if tc.wantEffort == "" {
						require.Nil(t, result.ReasoningEffort)
					}

					verifyGeminiReasoningEffortPersisted(t, result)
				})
			}
		}
	}
}

func TestGeminiChatCompatReasoningPricingUsesForwardedEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		mode := "buffered"
		if stream {
			mode = "streaming"
		}
		t.Run(mode, func(t *testing.T) {
			const model = "gemini-3.1-pro"
			body, err := json.Marshal(map[string]any{
				"model": model, "stream": stream, "reasoning_effort": "high",
				"messages": []map[string]string{{"role": "user", "content": "hello"}},
			})
			require.NoError(t, err)
			responseBody := `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
			contentType := "application/json"
			if stream {
				contentType = "text/event-stream"
				responseBody = "data: " + responseBody + "\n\n"
			}
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{contentType}},
				Body:       io.NopCloser(strings.NewReader(responseBody)),
			}}}
			svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
			c, recorder := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiSignalTestAccount(), body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Len(t, upstream.requestBodies, 1)
			require.Equal(t, "high", gjson.GetBytes(upstream.requestBodies[0], "generationConfig.thinkingConfig.thinkingLevel").String())
			require.Equal(t, stream, gjson.GetBytes(upstream.requestBodies[0], "generationConfig.thinkingConfig.includeThoughts").Bool())
			require.Equal(t, "high", optionalStringValue(result.ReasoningEffort))

			verifyGeminiReasoningEffortPersisted(t, result)
		})
	}
}

// The local V03 pricing model has no reasoning-effort multipliers; preserve
// the upstream persistence assertions without importing V09 pricing behavior.
func verifyGeminiReasoningEffortPersisted(t *testing.T, result *ForwardResult) {
	t.Helper()
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billing := newGatewayRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{})
	groupID := int64(77)
	require.NoError(t, billing.RecordUsage(context.Background(), &RecordUsageInput{
		Result: result,
		APIKey: &APIKey{ID: 10, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGemini, Status: StatusActive, Hydrated: true, RateMultiplier: 1}},
		User:   &User{ID: 20}, Account: geminiSignalTestAccount(),
	}))
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, optionalStringValue(result.ReasoningEffort), optionalStringValue(usageRepo.lastLog.ReasoningEffort))
}

type geminiVertexSharedCapacityRepo struct {
	mockAccountRepoForGemini
	rateLimitCalls int
	tempCalls      int
	setErrorCalls  int
}

func (r *geminiVertexSharedCapacityRepo) SetRateLimited(context.Context, int64, time.Time) error {
	r.rateLimitCalls++
	return nil
}
func (r *geminiVertexSharedCapacityRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.tempCalls++
	return nil
}
func (r *geminiVertexSharedCapacityRepo) SetError(context.Context, int64, string) error {
	r.setErrorCalls++
	return nil
}

func TestGeminiForwardNative_VertexSharedCapacity429RetriesSameAccountWithoutCooldown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &geminiSequenceUpstreamStub{responses: []geminiSequenceUpstreamResponse{
		{status: http.StatusTooManyRequests, body: `{"error":{"code":429,"message":"Resource exhausted. Please try again later.","status":"RESOURCE_EXHAUSTED"}}`},
		{status: http.StatusOK, body: geminiRetryTestOKBody},
	}}
	repo := &geminiVertexSharedCapacityRepo{}
	svc := &GeminiMessagesCompatService{
		httpUpstream:     upstream,
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxSameAccountRetriesGemini: 1}},
		accountRepo:      repo,
		tokenProvider:    &GeminiTokenProvider{tokenCache: newGeminiVertexSharedCapacityTokenCache()},
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
	}
	account := &Account{ID: 601, Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{
		"project_id": "test-project", "location": "global",
		"service_account_json": `{"project_id":"test-project","client_email":"test@example.invalid","private_key":"test-only-not-used"}`,
	}}
	c, rec := newGeminiNativeTestContext(t)
	result, err := svc.ForwardNative(context.Background(), c, account, "gemini-3.1-pro", "generateContent", false, geminiSignalTestRequest())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 2, upstream.calls)
	require.Zero(t, repo.rateLimitCalls)
	require.Zero(t, repo.tempCalls)
	require.Zero(t, repo.setErrorCalls)
}

type geminiVertexSharedCapacityTokenCache struct{}

func newGeminiVertexSharedCapacityTokenCache() GeminiTokenCache {
	return geminiVertexSharedCapacityTokenCache{}
}
func (geminiVertexSharedCapacityTokenCache) GetAccessToken(context.Context, string) (string, error) {
	return "test-vertex-token", nil
}
func (geminiVertexSharedCapacityTokenCache) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}
func (geminiVertexSharedCapacityTokenCache) DeleteAccessToken(context.Context, string) error {
	return nil
}
func (geminiVertexSharedCapacityTokenCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}
func (geminiVertexSharedCapacityTokenCache) ReleaseRefreshLock(context.Context, string) error {
	return nil
}
