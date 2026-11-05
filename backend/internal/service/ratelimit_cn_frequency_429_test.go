//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 移植自上游 ratelimit_cn_frequency_429_test.go。上游 de908ed6d 的频率判定修的是
// 上游「Coding Plan 429 冷却到 5h 窗口」的问题；CCAI 没有窗口冷却逻辑（也不移植
// 配额快照、IsCodingPlan），国产三家没有重置头的 429 本来就走秒级回退。
// 这里只保留回归用例：频率型 429 仍走秒级回退，余额不足 429 走临时停调。
// 另含余额不足 402 与 DeepSeek 真鉴权 403（V02-AC35）用例。

// 智谱 GLM Coding Plan 实测 429 文案：额度充裕时的「限频」。
const zhipuFrequencyLimitedBody = `{"error":{"code":"1302","message":"您的账户已达到速率限制，请您控制请求频率"}}`

func runCNUpstreamError(service *RateLimitService, account *Account, statusCode int, body string) bool {
	return service.HandleUpstreamError(
		context.Background(),
		account,
		statusCode,
		http.Header{},
		[]byte(body),
	)
}

func requireShort429Cooldown(t *testing.T, repo *rateLimitAccountRepoStub, account *Account) {
	t.Helper()
	require.Equal(t, 0, repo.setErrorCalls, "a frequency 429 must never SetError")
	require.Equal(t, 0, repo.tempCalls)
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Equal(t, account.ID, repo.lastRateLimitedID)
	require.WithinDuration(t, time.Now().Add(defaultRateLimit429CooldownSeconds*time.Second),
		repo.lastRateLimitedAt, 30*time.Second,
		"CN 429 without reset headers must cool down seconds, not a long window")
}

// V02-AC34：智谱或 Kimi 频率型 429、没有重置头 → 秒级回退冷却（回归，CCAI 现有行为）。
func TestHandleUpstreamError_CNFrequency429UsesShortCooldown(t *testing.T) {
	cases := []struct {
		platform string
		body     string
	}{
		{PlatformZhipu, zhipuFrequencyLimitedBody},
		{PlatformKimi, `{"error":{"message":"Too many requests, please slow down","type":"rate_limit_reached_error"}}`},
		{PlatformDeepSeek, `{"error":{"message":"Rate limit reached for requests","type":"rate_limit_error"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.platform, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			account := &Account{ID: 701, Platform: tc.platform, Type: AccountTypeAPIKey}

			require.False(t, runCNUpstreamError(service, account, http.StatusTooManyRequests, tc.body))
			requireShort429Cooldown(t, repo, account)
		})
	}
}

// 额度类文案的 429 在 CCAI 同样走秒级回退（CCAI 没有窗口冷却，不会进长冷却）。
func TestHandleUpstreamError_ZhipuQuotaWorded429UsesShortCooldown(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 702, Platform: PlatformZhipu, Type: AccountTypeAPIKey}

	runCNUpstreamError(service, account, http.StatusTooManyRequests, `{"error":{"message":"已达速率限制：本月额度已用尽，请升级套餐"}}`)

	requireShort429Cooldown(t, repo, account)
}

func requireCNBalanceLowCooldown(t *testing.T, repo *rateLimitAccountRepoStub, blocker *runtimeBlockRecorder) {
	t.Helper()
	require.Equal(t, 0, repo.setErrorCalls, "insufficient balance must never SetError")
	require.Equal(t, 0, repo.rateLimitedCalls)
	require.Equal(t, 1, repo.tempCalls)
	require.Contains(t, repo.lastTempReason, cnBalanceLowReasonPrefix)
	require.WithinDuration(t, time.Now().Add(20*time.Minute), repo.lastTempUntil, 5*time.Second)
	require.Zero(t, repo.updateExtraCalls, "CCAI 没有周期余额检测任务，不写 balance_low 标记")
	require.Len(t, blocker.accounts, 1)
	require.Equal(t, "cn_insufficient_balance", blocker.reasons[0])
}

// 余额不足 429 走临时停调（20 分钟），不进 429 回退。
func TestHandleUpstreamError_CNInsufficientBalance429TempUnschedulable(t *testing.T) {
	for _, platform := range []string{PlatformZhipu, PlatformKimi, PlatformDeepSeek} {
		t.Run(platform, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			blocker := &runtimeBlockRecorder{}
			service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			service.SetAccountRuntimeBlocker(blocker)
			account := &Account{ID: 703, Platform: platform, Type: AccountTypeAPIKey}

			require.False(t, runCNUpstreamError(service, account, http.StatusTooManyRequests, `{"error":{"message":"余额不足，请充值"}}`))
			requireCNBalanceLowCooldown(t, repo, blocker)
			require.Equal(t, "cn_balance_low: 余额不足，请充值", repo.lastTempReason)
		})
	}
}

// 余额不足 402（DeepSeek 实测 "Insufficient Balance"）：临时停调，不永久禁用。
func TestHandleUpstreamError_CNPaymentRequired402TempUnschedulable(t *testing.T) {
	for _, platform := range []string{PlatformDeepSeek, PlatformKimi, PlatformZhipu} {
		t.Run(platform, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			blocker := &runtimeBlockRecorder{}
			service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			service.SetAccountRuntimeBlocker(blocker)
			account := &Account{ID: 704, Platform: platform, Type: AccountTypeAPIKey}

			require.True(t, runCNUpstreamError(service, account, http.StatusPaymentRequired,
				`{"error":{"message":"Insufficient Balance","type":"unknown_error"}}`), "402 仍触发换号")
			requireCNBalanceLowCooldown(t, repo, blocker)
			require.Contains(t, repo.lastTempReason, "Insufficient Balance")
		})
	}
}

// 反向：非国产平台的 402 仍永久禁用。
func TestHandleUpstreamError_NonCN402StillDisables(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic, PlatformGrok} {
		t.Run(platform, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{}
			service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			account := &Account{ID: 705, Platform: platform, Type: AccountTypeAPIKey}

			require.True(t, runCNUpstreamError(service, account, http.StatusPaymentRequired, `{"error":{"message":"Insufficient Balance"}}`))
			require.Equal(t, 1, repo.setErrorCalls)
			require.Equal(t, 0, repo.tempCalls)
		})
	}
}

// 非国产平台的余额不足文案 429 不走 CN 临时停调。
func TestHandleUpstreamError_NonCNInsufficientBalance429KeepsDefaultPath(t *testing.T) {
	repo := &rateLimitAccountRepoStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 706, Platform: PlatformGrok, Type: AccountTypeAPIKey}

	runCNUpstreamError(service, account, http.StatusTooManyRequests, `{"error":{"message":"insufficient balance"}}`)

	require.Equal(t, 0, repo.tempCalls)
	require.Equal(t, 1, repo.rateLimitedCalls)
}

// V02-AC35：DeepSeek 真鉴权 403（key 被吊销）不能被 CN 冷却路径放过。
// T02 起国产三家的 403 与 OpenAI 同口径走 handleOpenAI403：没有计数缓存时立刻禁用；
// 有计数缓存时首次临时冷却，达到阈值禁用。
func TestHandleUpstreamError_DeepSeekGenuineAuth403StillDisables(t *testing.T) {
	const revokedBody = `{"error":{"message":"Authentication Fails, Your api key: ****abcd is invalid","type":"authentication_error","code":"invalid_request_error"}}`

	t.Run("without_counter_disables_immediately", func(t *testing.T) {
		repo := &rateLimitAccountRepoStub{}
		service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		account := &Account{ID: 707, Platform: PlatformDeepSeek, Type: AccountTypeAPIKey}

		require.True(t, runCNUpstreamError(service, account, http.StatusForbidden, revokedBody))
		require.Equal(t, 1, repo.setErrorCalls)
		require.Contains(t, repo.lastErrorMsg, "Authentication Fails")
		require.Equal(t, 0, repo.rateLimitedCalls)
	})

	t.Run("threshold_disables", func(t *testing.T) {
		repo := &rateLimitAccountRepoStub{}
		counter := &openAI403CounterCacheStub{counts: []int64{openAI403DisableThreshold}}
		service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		service.SetOpenAI403CounterCache(counter)
		account := &Account{ID: 708, Platform: PlatformDeepSeek, Type: AccountTypeAPIKey}

		require.True(t, runCNUpstreamError(service, account, http.StatusForbidden, revokedBody))
		require.Equal(t, 1, repo.setErrorCalls)
		require.Contains(t, repo.lastErrorMsg, "consecutive_403=3/3")
	})

	t.Run("first_hit_counts_toward_disable", func(t *testing.T) {
		repo := &rateLimitAccountRepoStub{}
		counter := &openAI403CounterCacheStub{counts: []int64{1}}
		service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		service.SetOpenAI403CounterCache(counter)
		account := &Account{ID: 709, Platform: PlatformDeepSeek, Type: AccountTypeAPIKey}

		require.True(t, runCNUpstreamError(service, account, http.StatusForbidden, revokedBody))
		require.Equal(t, 0, repo.setErrorCalls)
		require.Equal(t, 1, repo.tempCalls)
		require.Contains(t, repo.lastTempReason, "(1/3)")
		require.NotContains(t, repo.lastTempReason, cnQuotaExhaustedReasonPrefix)
		require.NotContains(t, repo.lastTempReason, cnBalanceLowReasonPrefix)
	})
}

// TestCNProviderResponseIndicatesInsufficientBalance 覆盖中英文余额不足文案与否定用例。
func TestCNProviderResponseIndicatesInsufficientBalance(t *testing.T) {
	t.Parallel()
	positive := []string{
		`{"error":{"message":"余额不足"}}`,
		`{"error":{"message":"Insufficient balance"}}`,
		`{"code":"insufficient_credit"}`,
		`"balance is not enough"`,
		`"no enough balance"`,
	}
	for _, body := range positive {
		require.True(t, cnProviderResponseIndicatesInsufficientBalance([]byte(body)), body)
	}
	negative := []string{
		`{"error":{"message":"rate limit exceeded"}}`,
		`{"error":{"message":"quota exhausted"}}`,
		``,
	}
	for _, body := range negative {
		require.False(t, cnProviderResponseIndicatesInsufficientBalance([]byte(body)), body)
	}
}

// TestCNBalanceLowReason 验证稳定前缀。
func TestCNBalanceLowReason(t *testing.T) {
	t.Parallel()
	require.Equal(t, "cn_balance_low: upstream said x",
		cnBalanceLowReason("upstream said x"))
	require.Equal(t, "cn_balance_low: 余额不足，账号临时停调",
		cnBalanceLowReason("   "))
	require.True(t, len(cnBalanceLowReason("")) > len(cnBalanceLowReasonPrefix))
}

func TestAccountIsCNProvider(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepSeek} {
		require.True(t, (&Account{Platform: platform}).IsCNProvider(), platform)
	}
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok} {
		require.False(t, (&Account{Platform: platform}).IsCNProvider(), platform)
	}
	require.False(t, (*Account)(nil).IsCNProvider())
}
