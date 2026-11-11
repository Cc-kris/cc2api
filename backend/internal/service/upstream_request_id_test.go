package service

// 移植自上游 de27905e8 upstream_request_id_test.go。差异：本地账户未配置头名时默认读 x-request-id
// （技术方案 3.11 / AC31），上游的「未配置不记录」用例改为断言默认头。

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpstreamRequestIDFromHeaders_DefaultsToXRequestID(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-ID", " req_abc ")
	h.Set("Request-Id", "req_official")

	require.Equal(t, "req_abc", UpstreamRequestIDFromHeaders(nil, h))
	for _, platform := range []string{PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity} {
		require.Equal(t, "req_abc", UpstreamRequestIDFromHeaders(&Account{Platform: platform}, h), platform)
	}
	blank := &Account{Platform: PlatformOpenAI, Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "   "}}
	require.Equal(t, "req_abc", UpstreamRequestIDFromHeaders(blank, h))
	require.Equal(t, "", UpstreamRequestIDFromHeaders(nil, nil))
}

func TestUpstreamRequestIDFromHeaders_ReadsOnlyConfiguredHeader(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Extra:    map[string]any{AccountExtraUpstreamRequestIDHeader: " x-oneapi-request-id "},
	}
	h := http.Header{}
	h.Set("X-Request-ID", "passthrough-from-real-upstream")
	require.Equal(t, "", UpstreamRequestIDFromHeaders(account, h), "configured header must not fall back to x-request-id")

	h.Set("X-Oneapi-Request-Id", " oneapi-2 ")
	require.Equal(t, "oneapi-2", UpstreamRequestIDFromHeaders(account, h))

	official := &Account{Platform: PlatformAnthropic, Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "request-id"}}
	only := http.Header{}
	only.Set("Request-Id", "req_official")
	require.Equal(t, "req_official", UpstreamRequestIDFromHeaders(official, only))
}

func TestUsageUpstreamRequestIDPtr(t *testing.T) {
	account := &Account{Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "X-Request-ID"}}
	h := http.Header{}
	h.Set("X-Request-ID", strings.Repeat("a", 200))
	require.Nil(t, usageUpstreamRequestIDPtr(account, h, true), "WS turns stay NULL")
	require.Nil(t, usageUpstreamRequestIDPtr(account, http.Header{}, false))

	got := usageUpstreamRequestIDPtr(account, h, false)
	require.NotNil(t, got)
	require.Len(t, *got, maxUsageUpstreamRequestIDLen)

	multibyte := http.Header{}
	multibyte.Set("X-Request-ID", strings.Repeat("好", 50))
	got = usageUpstreamRequestIDPtr(nil, multibyte, false)
	require.NotNil(t, got)
	require.LessOrEqual(t, len(*got), maxUsageUpstreamRequestIDLen)
	require.Equal(t, strings.ToValidUTF8(*got, ""), *got, "truncation must not split a UTF-8 rune")

	ctrl := http.Header{}
	ctrl["X-Request-Id"] = []string{"req\x01abc"}
	require.Nil(t, usageUpstreamRequestIDPtr(nil, ctrl, false))
}

// AC32（校验部分）：65 字符或含换行的头名被拒；空白头名视为未配置并删除。
func TestValidateUpstreamRequestIDHeaderExtra(t *testing.T) {
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(nil))
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{}))

	blank := map[string]any{AccountExtraUpstreamRequestIDHeader: "   "}
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(blank))
	_, present := blank[AccountExtraUpstreamRequestIDHeader]
	require.False(t, present, "blank header name must be removed")

	valid := map[string]any{AccountExtraUpstreamRequestIDHeader: " X-Oneapi-Request-Id "}
	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(valid))
	require.Equal(t, "X-Oneapi-Request-Id", valid[AccountExtraUpstreamRequestIDHeader])

	require.NoError(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{AccountExtraUpstreamRequestIDHeader: strings.Repeat("x", 64)}))
	for _, bad := range []any{
		1,
		"X Request Id",
		"X-Request-Id:",
		"X-Request\nId",
		"X-Request-Id\r\nSet-Cookie: a=b",
		strings.Repeat("x", 65),
	} {
		require.Error(t, ValidateUpstreamRequestIDHeaderExtra(map[string]any{AccountExtraUpstreamRequestIDHeader: bad}), "%q", bad)
	}
}

// AC32：管理服务入口（创建、更新、批量更新）拒绝非法头名，HTTP 之外的调用方也被拦住。
func TestAdminServiceRejectsInvalidUpstreamRequestIDHeader(t *testing.T) {
	svc := &adminServiceImpl{}
	bad := map[string]any{AccountExtraUpstreamRequestIDHeader: strings.Repeat("x", 65)}

	_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{Platform: PlatformOpenAI, Extra: bad})
	require.Error(t, err)
	_, err = svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "bad\nname"}})
	require.Error(t, err)
	_, err = svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{1}, Extra: bad})
	require.Error(t, err)
}

// AC31：响应头 x-request-id=req_abc 落到 upstream_request_id；费用不变；与财务上游计费单号互不影响。
func TestRecordUsage_UpstreamRequestIDDoesNotChangeBilling(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Request-ID", "req_abc")

	runGateway := func(h http.Header) (*UsageLog, float64) {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		userRepo := &openAIRecordUsageUserRepoStub{}
		svc := newUsageAuditGatewayServiceForTest(usageRepo, userRepo)
		require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{
			Result: &ForwardResult{
				RequestID: "gw_upstream_request_id", Model: "claude-sonnet-4", Duration: time.Second,
				Usage:           ClaudeUsage{InputTokens: 800, OutputTokens: 200},
				UpstreamHeaders: h,
			},
			APIKey: &APIKey{ID: 503}, User: &User{ID: 603}, Account: &Account{ID: 703, Platform: PlatformAnthropic},
			UpstreamAttempts: []UsageUpstreamAttempt{{AttemptNo: 1, AccountID: 703, UpstreamBillingRequestID: "bill_999"}},
		}))
		return usageRepo.lastLog, userRepo.lastAmount
	}
	plain, plainDeducted := runGateway(nil)
	audited, auditedDeducted := runGateway(headers)
	require.Equal(t, snapshotUsageAuditBilling(plain, plainDeducted), snapshotUsageAuditBilling(audited, auditedDeducted))
	require.Nil(t, plain.UpstreamRequestID)
	require.NotNil(t, audited.UpstreamRequestID)
	require.Equal(t, "req_abc", *audited.UpstreamRequestID)
	require.NotEmpty(t, audited.UpstreamAttempts)
	require.Equal(t, "bill_999", audited.UpstreamAttempts[0].UpstreamBillingRequestID, "financial billing request id stays separate")
	for _, attempt := range audited.UpstreamAttempts {
		require.NotEqual(t, "req_abc", attempt.UpstreamBillingRequestID, "upstream_request_id must not leak into upstream_billing_request_id")
	}
	require.Equal(t, len(plain.UpstreamAttempts), len(audited.UpstreamAttempts))

	runOpenAI := func(h http.Header, ws bool) *UsageLog {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
		require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
			Result: &OpenAIForwardResult{
				RequestID: "oa_upstream_request_id", Model: "gpt-5.1", Duration: time.Second,
				Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 20}, UpstreamHeaders: h, OpenAIWSMode: ws,
			},
			APIKey: &APIKey{ID: 504}, User: &User{ID: 604},
			Account: &Account{ID: 704, Platform: PlatformOpenAI, Extra: map[string]any{AccountExtraUpstreamRequestIDHeader: "X-Oneapi-Request-Id"}},
		}))
		return usageRepo.lastLog
	}
	oneapi := http.Header{}
	oneapi.Set("X-Request-ID", "local-passthrough")
	oneapi.Set("X-Oneapi-Request-Id", "oneapi-1")
	require.Equal(t, "oneapi-1", *runOpenAI(oneapi, false).UpstreamRequestID)
	require.Nil(t, runOpenAI(oneapi, true).UpstreamRequestID, "WS turns stay NULL")
}
