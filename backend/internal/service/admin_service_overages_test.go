//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateAccountOveragesRepoStub struct {
	mockAccountRepoForGemini
	account     *Account
	created     *Account
	updateCalls int
}

func (r *updateAccountOveragesRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	return r.account, nil
}

func (r *updateAccountOveragesRepoStub) Update(ctx context.Context, account *Account) error {
	r.updateCalls++
	r.account = account
	return nil
}

func (r *updateAccountOveragesRepoStub) Create(ctx context.Context, account *Account) error {
	r.created = account
	return nil
}

func TestUpdateAccount_DisableOveragesClearsAICreditsKey(t *testing.T) {
	accountID := int64(101)
	repo := &updateAccountOveragesRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAntigravity,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra: map[string]any{
				"allow_overages":   true,
				"mixed_scheduling": true,
				modelRateLimitsKey: map[string]any{
					"claude-sonnet-4-5": map[string]any{
						"rate_limited_at":     "2026-03-15T00:00:00Z",
						"rate_limit_reset_at": "2099-03-15T00:00:00Z",
					},
					creditsExhaustedKey: map[string]any{
						"rate_limited_at":     "2026-03-15T00:00:00Z",
						"rate_limit_reset_at": time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339),
					},
				},
			},
		},
	}

	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			"mixed_scheduling": true,
			modelRateLimitsKey: map[string]any{
				"claude-sonnet-4-5": map[string]any{
					"rate_limited_at":     "2026-03-15T00:00:00Z",
					"rate_limit_reset_at": "2099-03-15T00:00:00Z",
				},
				creditsExhaustedKey: map[string]any{
					"rate_limited_at":     "2026-03-15T00:00:00Z",
					"rate_limit_reset_at": time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339),
				},
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 1, repo.updateCalls)
	require.False(t, updated.IsOveragesEnabled())

	// 关闭 overages 后，AICredits key 应被清除
	rawLimits, ok := repo.account.Extra[modelRateLimitsKey].(map[string]any)
	if ok {
		_, exists := rawLimits[creditsExhaustedKey]
		require.False(t, exists, "关闭 overages 时应清除 AICredits 限流 key")
	}
	// 普通模型限流应保留
	require.True(t, ok)
	_, exists := rawLimits["claude-sonnet-4-5"]
	require.True(t, exists, "普通模型限流应保留")
}

func TestUpdateAccount_EnableOveragesClearsModelRateLimitsBeforePersist(t *testing.T) {
	accountID := int64(102)
	repo := &updateAccountOveragesRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAntigravity,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			Extra: map[string]any{
				"mixed_scheduling": true,
				modelRateLimitsKey: map[string]any{
					"claude-sonnet-4-5": map[string]any{
						"rate_limited_at":     "2026-03-15T00:00:00Z",
						"rate_limit_reset_at": "2099-03-15T00:00:00Z",
					},
				},
			},
		},
	}

	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			"mixed_scheduling": true,
			"allow_overages":   true,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 1, repo.updateCalls)
	require.True(t, updated.IsOveragesEnabled())

	_, exists := repo.account.Extra[modelRateLimitsKey]
	require.False(t, exists, "开启 overages 时应在持久化前清掉旧模型限流")
}

func TestUpdateAccount_EmptyExtraPayloadCanClearQuotaLimits(t *testing.T) {
	accountID := int64(103)
	repo := &updateAccountOveragesRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
			Extra: map[string]any{
				"quota_limit":        100.0,
				"quota_daily_limit":  10.0,
				"quota_weekly_limit": 40.0,
			},
		},
	}

	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		// 显式空对象：语义是“清空 extra 中的可配置键”（例如关闭配额限制）
		Extra: map[string]any{},
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 1, repo.updateCalls)
	require.NotNil(t, repo.account.Extra)
	require.NotContains(t, repo.account.Extra, "quota_limit")
	require.NotContains(t, repo.account.Extra, "quota_daily_limit")
	require.NotContains(t, repo.account.Extra, "quota_weekly_limit")
	require.Len(t, repo.account.Extra, 0)
}

func TestUpdateAccount_DropsDeprecatedUpstreamWarningExtra(t *testing.T) {
	accountID := int64(104)
	repo := &updateAccountOveragesRepoStub{
		account: &Account{
			ID:       accountID,
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Status:   StatusActive,
			Extra: map[string]any{
				"quota_used": 3.0,
			},
		},
	}

	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Extra: map[string]any{
			"upstream_prepaid_amount": 25.5,
			"upstream_warning_amount": 5.0,
			"upstream_notify_enabled": true,
			"quota_daily_limit":       20.0,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 1, repo.updateCalls)
	require.Equal(t, 25.5, repo.account.Extra["upstream_prepaid_amount"])
	require.Equal(t, 3.0, repo.account.Extra["quota_used"])
	require.NotContains(t, repo.account.Extra, "upstream_warning_amount")
	require.NotContains(t, repo.account.Extra, "upstream_notify_enabled")
}

func TestCreateAccount_DropsDeprecatedUpstreamWarningExtra(t *testing.T) {
	repo := &updateAccountOveragesRepoStub{}
	svc := &adminServiceImpl{accountRepo: repo}

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "legacy-client-account",
		Platform:             PlatformAnthropic,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "test-key"},
		SkipDefaultGroupBind: true,
		Extra: map[string]any{
			"upstream_prepaid_amount": 25.5,
			"upstream_warning_amount": 5.0,
			"upstream_notify_enabled": true,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, repo.created)
	require.Equal(t, 25.5, repo.created.Extra["upstream_prepaid_amount"])
	require.NotContains(t, repo.created.Extra, "upstream_warning_amount")
	require.NotContains(t, repo.created.Extra, "upstream_notify_enabled")
}
