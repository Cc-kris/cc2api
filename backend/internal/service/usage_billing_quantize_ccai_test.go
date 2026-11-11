package service

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// V03-AC01：原始费用 0.000078125（gemini-2.5-pro 输入 10、输出 5，分组倍率 1.25）。
// 量化后余额扣减、Key 配额、用量记录 actual_cost、财务销售值 usage_list_value 四者相等。
func TestApplyUsageBilling_QuantizedUsageLogMatchesBalance(t *testing.T) {
	const raw = 0.000078125
	const want = 0.00007813

	listValue := decimal.NewFromFloat(raw)
	usageLog := &UsageLog{
		RequestID:      "req-ac01",
		Model:          "gemini-2.5-pro",
		InputTokens:    10,
		OutputTokens:   5,
		TotalCost:      0.0000625,
		ActualCost:     raw,
		RateMultiplier: 1.25,
		UsageListValue: &listValue,
	}
	cost := &CostBreakdown{TotalCost: 0.0000625, ActualCost: raw}
	params := &postUsageBillingParams{
		Cost:          cost,
		User:          &User{ID: 1},
		APIKey:        &APIKey{ID: 2, Quota: 100},
		Account:       &Account{ID: 3, Type: AccountTypeOAuth},
		APIKeyService: noopAPIKeyQuotaUpdater{},
	}
	repo := &openAIRecordUsageBillingRepoStub{}
	deps := &billingDeps{billingCacheService: &BillingCacheService{}, deferredService: &DeferredService{}}

	applied, err := applyUsageBilling(context.Background(), "req-ac01", usageLog, params, deps, repo)
	require.NoError(t, err)
	require.True(t, applied)
	require.NotNil(t, repo.lastCmd)

	require.Equal(t, want, repo.lastCmd.BalanceCost, "余额扣减")
	require.Equal(t, want, repo.lastCmd.APIKeyQuotaCost, "Key 配额")
	require.Equal(t, want, usageLog.ActualCost, "usage_logs.actual_cost")
	require.NotNil(t, usageLog.UsageListValue)
	require.True(t, usageLog.UsageListValue.Equal(decimal.RequireFromString("0.00007813")),
		"usage_list_value=%s", usageLog.UsageListValue)
	// Redis 缓存扣减、平台配额使用 p.Cost.ActualCost，同样是量化值。
	require.Equal(t, want, cost.ActualCost, "缓存扣减金额")

	// 余额 10000 减去量化金额正好落在 8 位刻度上：9999.99992187。
	balance := decimal.NewFromInt(10000).Sub(decimal.NewFromFloat(repo.lastCmd.BalanceCost))
	require.True(t, balance.Equal(decimal.RequireFromString("9999.99992187")), "balance=%s", balance)

	// 分项不参与扣款，保持原值。
	require.Equal(t, 0.0000625, usageLog.TotalCost)
	require.Equal(t, 0.0000625, cost.TotalCost)
}

// V03-AC02：升级前用原始金额写过 usage_billing_dedup，升级后同一 request_id 重放，指纹不变。
func TestUsageBillingFingerprintUsesPreQuantizedAmount(t *testing.T) {
	const raw = 0.000078125

	// 升级前的指纹：直接由原始金额计算（旧版 Normalize 不量化）。
	legacy := &UsageBillingCommand{
		RequestID: "req-ac02", UserID: 1, APIKeyID: 2, AccountID: 3,
		Model: "gemini-2.5-pro", InputTokens: 10, OutputTokens: 5,
		BalanceCost: raw, APIKeyQuotaCost: raw,
	}
	legacyFingerprint := buildUsageBillingFingerprint(legacy)

	// 升级后经 applyUsageBilling 构建的命令。
	usageLog := &UsageLog{RequestID: "req-ac02", Model: "gemini-2.5-pro", InputTokens: 10, OutputTokens: 5, ActualCost: raw}
	params := &postUsageBillingParams{
		Cost:          &CostBreakdown{TotalCost: 0.0000625, ActualCost: raw},
		User:          &User{ID: 1},
		APIKey:        &APIKey{ID: 2, Quota: 100},
		Account:       &Account{ID: 3},
		APIKeyService: noopAPIKeyQuotaUpdater{},
	}
	repo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: false}}
	deps := &billingDeps{billingCacheService: &BillingCacheService{}, deferredService: &DeferredService{}}

	applied, err := applyUsageBilling(context.Background(), "req-ac02", usageLog, params, deps, repo)
	require.NoError(t, err)
	require.False(t, applied, "重放命令不应再次扣费")
	require.Equal(t, legacyFingerprint, repo.lastCmd.RequestFingerprint)
	require.Equal(t, 0.00007813, repo.lastCmd.BalanceCost)

	// repository 内会再次调用 Normalize：量化幂等，指纹不重算。
	replay := *repo.lastCmd
	replay.Normalize()
	require.Equal(t, legacyFingerprint, replay.RequestFingerprint)
	require.Equal(t, 0.00007813, replay.BalanceCost)
}

// 订阅计费同样量化：SubscriptionCost 与用量记录一致。
func TestApplyUsageBilling_QuantizesSubscriptionCost(t *testing.T) {
	const raw = 0.000078125
	usageLog := &UsageLog{RequestID: "req-sub", ActualCost: raw}
	params := &postUsageBillingParams{
		Cost:               &CostBreakdown{TotalCost: 0.0000625, ActualCost: raw},
		User:               &User{ID: 1},
		APIKey:             &APIKey{ID: 2},
		Account:            &Account{ID: 3},
		Subscription:       &UserSubscription{ID: 9},
		IsSubscriptionBill: true,
	}
	repo := &openAIRecordUsageBillingRepoStub{}
	deps := &billingDeps{billingCacheService: &BillingCacheService{}, deferredService: &DeferredService{}}

	_, err := applyUsageBilling(context.Background(), "req-sub", usageLog, params, deps, repo)
	require.NoError(t, err)
	require.Equal(t, 0.00007813, repo.lastCmd.SubscriptionCost)
	require.Equal(t, 0.0, repo.lastCmd.BalanceCost)
	require.Equal(t, 0.00007813, usageLog.ActualCost)
}

type noopAPIKeyQuotaUpdater struct{}

func (noopAPIKeyQuotaUpdater) UpdateQuotaUsed(context.Context, int64, float64) error { return nil }
func (noopAPIKeyQuotaUpdater) UpdateRateLimitUsage(context.Context, int64, float64) error {
	return nil
}
