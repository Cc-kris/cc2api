//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 移植自上游 ratelimit_service_cn_quota_403_test.go。CCAI 没有 Coding Plan
// （credentials.account_mode）和配额快照：删掉依赖快照、IsCodingPlan 的用例，
// 配额 403 的断言改为默认冷却（TempUnschedulable openAI403CooldownMinutesDefault 分钟）。

// newCNQuotaExhaustedTestAccount 构造一个 Kimi API Key 账号。
func newCNQuotaExhaustedTestAccount() *Account {
	return &Account{
		ID:       401,
		Platform: PlatformKimi,
		Type:     AccountTypeAPIKey,
	}
}

const kimiWeeklyQuotaExhaustedBody = `{"error":{"message":"You've reached your weekly (7-day) usage limit. Your quota will reset when the current 7-day window ends. To continue now, purchase extra usage or upgrade your plan: https://www.kimi.com/membership/subscription?tab=quota","type":"access_terminated_error"}}`

// runKimi403 以 403 + 给定响应体驱动 HandleUpstreamError，返回 shouldDisable。
func runKimi403(service *RateLimitService, account *Account, body string) bool {
	return service.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusForbidden,
		http.Header{},
		[]byte(body),
	)
}

func requireCNQuotaDefaultCooldown(t *testing.T, repo *rateLimitAccountRepoStub) {
	t.Helper()
	require.Equal(t, 0, repo.setErrorCalls, "quota exhaustion must never SetError")
	require.Equal(t, 0, repo.rateLimitedCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, cnQuotaExhaustedReasonPrefix)
	require.WithinDuration(t, time.Now().Add(time.Duration(openAI403CooldownMinutesDefault)*time.Minute), repo.lastTempUntil, 5*time.Second)
}

func TestHandleUpstreamError_KimiQuotaExhausted403UsesDefaultCooldown(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	blocker := &runtimeBlockRecorder{}
	counter := &openAI403CounterCacheStub{counts: []int64{openAI403DisableThreshold}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetAccountRuntimeBlocker(blocker)
	service.SetOpenAI403CounterCache(counter)
	account := newCNQuotaExhaustedTestAccount()

	shouldDisable := runKimi403(service, account, kimiWeeklyQuotaExhaustedBody)

	require.True(t, shouldDisable)
	requireCNQuotaDefaultCooldown(t, repo)
	require.Greater(t, len(repo.lastTempReason), len(cnQuotaExhaustedReasonPrefix))
	require.Len(t, blocker.accounts, 1)
	require.Equal(t, cnQuotaExhaustedReasonPrefix, blocker.reasons[0])
	require.Equal(t, []int64{openAI403DisableThreshold}, counter.counts, "quota 403 must not consume the 403 strike budget")
}

// V02-AC33：智谱配额耗尽 403 临时不可调度一段默认冷却时间，状态不变成 error。
func TestHandleUpstreamError_ZhipuQuotaExhausted403UsesDefaultCooldown(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{openAI403DisableThreshold}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := &Account{ID: 402, Platform: PlatformZhipu, Type: AccountTypeAPIKey, Status: StatusActive}

	for i := 0; i < openAI403DisableThreshold+2; i++ {
		repo.tempCalls = 0
		require.True(t, runKimi403(service, account, `{"error":{"code":"1310","message":"Weekly/Monthly usage limit exhausted. Your quota will reset at 2026-11-08 00:00:00"}}`))
		requireCNQuotaDefaultCooldown(t, repo)
	}
	require.Equal(t, StatusActive, account.Status)
	require.Equal(t, []int64{openAI403DisableThreshold}, counter.counts)
}

func TestHandleUpstreamError_KimiQuotaExhausted403MatchedByMessageOnly(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newCNQuotaExhaustedTestAccount()
	// 无 error.type 字段，仅靠文案兜底匹配。
	body := `{"error":{"message":"You've reached your weekly (7-day) usage limit. Your quota will reset when the current 7-day window ends."}}`

	shouldDisable := runKimi403(service, account, body)

	require.True(t, shouldDisable)
	requireCNQuotaDefaultCooldown(t, repo)
}

func TestHandleUpstreamError_KimiQuotaExhausted403MatchedByErrorTypeOnly(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newCNQuotaExhaustedTestAccount()

	shouldDisable := runKimi403(service, account, `{"error":{"message":"access terminated","type":"access_terminated_error"}}`)

	require.True(t, shouldDisable)
	requireCNQuotaDefaultCooldown(t, repo)
}

func TestHandleUpstreamError_KimiConcurrencyLimit403StillUsesConcurrencyPath(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := newCNQuotaExhaustedTestAccount()
	body := fmt.Sprintf(`{"error":{"message":%q,"type":"access_terminated_error"}}`, kimiConcurrentRequestLimitMessage)

	shouldDisable := runKimi403(service, account, body)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.rateLimitedCalls, "concurrency-limit message must keep its dedicated path")
	require.Equal(t, 0, repo.tempCalls)
	require.WithinDuration(t, time.Now().Add(5*time.Second), repo.lastRateLimitedAt, time.Second)
}

func TestHandleUpstreamError_KimiGeneric403StillEscalates(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{3}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := newCNQuotaExhaustedTestAccount()

	shouldDisable := runKimi403(service, account, `{"error":{"message":"policy rejected this request"}}`)

	require.True(t, shouldDisable)
	require.Equal(t, 1, repo.setErrorCalls, "non-quota 403 keeps the generic escalation path")
	require.Equal(t, 0, repo.rateLimitedCalls)
}

func TestHandleUpstreamError_NonCNAccessTerminated403KeepsGenericPath(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	counter := &openAI403CounterCacheStub{counts: []int64{1}}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetOpenAI403CounterCache(counter)
	account := &Account{
		ID:       402,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}

	shouldDisable := runKimi403(service, account, kimiWeeklyQuotaExhaustedBody)

	require.True(t, shouldDisable)
	require.Equal(t, 0, repo.rateLimitedCalls, "non-CN platform must not enter the CN quota path")
	require.Equal(t, 0, repo.setErrorCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, "(1/3)")
}

func TestIsCNProviderQuotaExhausted403_Classification(t *testing.T) {
	kimi := newCNQuotaExhaustedTestAccount()
	require.True(t, isCNProviderQuotaExhausted403(kimi, []byte(kimiWeeklyQuotaExhaustedBody), ""))
	require.True(t, isCNProviderQuotaExhausted403(kimi, []byte(`{"error":{"message":"You've reached your weekly (7-day) usage limit."}}`), "You've reached your weekly (7-day) usage limit."))
	require.False(t, isCNProviderQuotaExhausted403(kimi, []byte(`{"error":{"message":"policy rejected"}}`), "policy rejected"))
	require.False(t, isCNProviderQuotaExhausted403(kimi, nil, ""))

	for _, platform := range []string{PlatformZhipu, PlatformDeepSeek} {
		require.True(t, isCNProviderQuotaExhausted403(&Account{Platform: platform}, []byte(kimiWeeklyQuotaExhaustedBody), ""))
	}

	openai := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.False(t, isCNProviderQuotaExhausted403(openai, []byte(kimiWeeklyQuotaExhaustedBody), ""))
	require.False(t, isCNProviderQuotaExhausted403(&Account{Platform: PlatformGrok}, []byte(kimiWeeklyQuotaExhaustedBody), ""))
	require.False(t, isCNProviderQuotaExhausted403(nil, []byte(kimiWeeklyQuotaExhaustedBody), ""))
}
