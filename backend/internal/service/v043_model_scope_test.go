package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type v043ScopeRepo struct {
	AccountRepository
	model                    string
	until                    time.Time
	modelCalls, accountCalls int
	err                      error
}

func (r *v043ScopeRepo) SetModelRateLimit(_ context.Context, _ int64, model string, until time.Time) error {
	r.modelCalls++
	r.model = model
	r.until = until
	return r.err
}
func (r *v043ScopeRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.accountCalls++
	return r.err
}

type v043ScopeRuntime struct {
	accountCalls, modelCalls int
	model                    string
	repo                     *v043ScopeRepo
	account                  *Account
}

func (r *v043ScopeRuntime) BlockAccountScheduling(*Account, time.Time, string) { r.accountCalls++ }
func (r *v043ScopeRuntime) ClearAccountSchedulingBlock(int64)                  {}
func (r *v043ScopeRuntime) RefreshModelCooldownSnapshot(context.Context, int64) {
	r.modelCalls++
	r.account.Extra = map[string]any{modelRateLimitsKey: map[string]any{r.repo.model: map[string]any{"rate_limit_reset_at": r.repo.until.Format(time.RFC3339)}}}
}
func TestV043ModelScopeRuntimeIsolation(t *testing.T) {
	ctx := context.Background()
	account := &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"alias-a": "model-a"}}}
	repo := &v043ScopeRepo{}
	runtime := &v043ScopeRuntime{repo: repo, account: account}
	rate := NewRateLimitService(repo, nil, nil, nil, nil)
	rate.SetAccountRuntimeBlocker(runtime)
	gateway := &OpenAIGatewayService{rateLimitService: rate}
	require.True(t, gateway.handleOpenAIAccountUpstreamError(ctx, account, 404, nil, []byte(`{"error":{"code":"model_not_found","message":"model does not exist"}}`), "alias-a"))
	require.Equal(t, "model-a", repo.model)
	require.Equal(t, 1, runtime.modelCalls)
	require.Zero(t, runtime.accountCalls)
	require.True(t, account.isModelRateLimitedWithContext(ctx, "alias-a"))
	require.False(t, account.isModelRateLimitedWithContext(ctx, "model-b"))
	require.False(t, gateway.isOpenAIAccountRuntimeBlocked(account))
	rule := TempUnschedulableRule{Scope: "model", DurationMinutes: 5}
	require.True(t, rate.triggerTempUnschedulable(upstreamErrorModelContext(ctx, []string{"model-b"}), account, rule, 0, 503, "busy", nil))
	require.Equal(t, "model-b", repo.model)
	require.Zero(t, repo.accountCalls)
	before := repo.modelCalls
	require.False(t, rate.triggerTempUnschedulable(ctx, account, rule, 0, 503, "busy", nil))
	require.Equal(t, before, repo.modelCalls, "missing model cannot widen to account scope")
	repo.err = errors.New("database unavailable")
	before = runtime.modelCalls
	require.False(t, rate.triggerTempUnschedulable(upstreamErrorModelContext(ctx, []string{"model-c"}), account, rule, 0, 503, "busy", nil))
	require.Equal(t, before, runtime.modelCalls, "failed DB write cannot publish successful cooldown")
	require.Zero(t, repo.accountCalls)
	repo.err = nil
	rule.Scope = ""
	require.True(t, rate.triggerTempUnschedulable(ctx, account, rule, 0, 503, "busy", nil))
	require.Equal(t, 1, repo.accountCalls, "legacy scope still pauses entire account")
	require.Equal(t, 1, runtime.accountCalls)
}
