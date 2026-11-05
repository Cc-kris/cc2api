package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// V02-T28：OpenAIGatewayService 的模型可用性诊断。上游 gateway_model_availability_test.go
// 只测 GatewayService 且带 unit 标签；CCAI 的 OpenAI handler 用的是这个实现，单独补用例。

type modelAvailabilityAccountRepo struct {
	stubOpenAIAccountRepo
	err error
}

func (r *modelAvailabilityAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.stubOpenAIAccountRepo.ListSchedulableByPlatform(ctx, platform)
}

func newModelAvailabilityService(accounts []Account, err error) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		accountRepo: &modelAvailabilityAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}, err: err},
		cfg:         &config.Config{RunMode: config.RunModeSimple},
	}
}

func TestOpenAIDiagnoseModelAvailability(t *testing.T) {
	restricted := Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4", "gpt-5*": "gpt-5.4"}},
	}
	open := Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	ctx := context.Background()

	t.Run("no account supports model", func(t *testing.T) {
		diag := newModelAvailabilityService([]Account{restricted}, nil).DiagnoseModelAvailabilityForPlatform(ctx, nil, "o9-mini", PlatformOpenAI)
		require.True(t, diag.HasAccountsInPool)
		require.False(t, diag.HasModelSupport)
	})
	t.Run("wildcard mapping supports", func(t *testing.T) {
		diag := newModelAvailabilityService([]Account{restricted}, nil).DiagnoseModelAvailabilityForPlatform(ctx, nil, "gpt-5.9", PlatformOpenAI)
		require.True(t, diag.HasModelSupport)
	})
	t.Run("empty mapping supports everything", func(t *testing.T) {
		diag := newModelAvailabilityService([]Account{restricted, open}, nil).DiagnoseModelAvailabilityForPlatform(ctx, nil, "o9-mini", PlatformOpenAI)
		require.True(t, diag.HasModelSupport)
	})
	t.Run("empty pool stays 503", func(t *testing.T) {
		diag := newModelAvailabilityService(nil, nil).DiagnoseModelAvailabilityForPlatform(ctx, nil, "o9-mini", PlatformOpenAI)
		require.False(t, diag.HasAccountsInPool)
	})
	t.Run("lookup error stays 503", func(t *testing.T) {
		diag := newModelAvailabilityService(nil, errors.New("db down")).DiagnoseModelAvailabilityForPlatform(ctx, nil, "o9-mini", PlatformOpenAI)
		require.True(t, diag.HasAccountsInPool)
		require.True(t, diag.HasModelSupport)
	})
	t.Run("empty model stays 503", func(t *testing.T) {
		diag := newModelAvailabilityService([]Account{restricted}, nil).DiagnoseModelAvailabilityForPlatform(ctx, nil, " ", PlatformOpenAI)
		require.True(t, diag.HasModelSupport)
	})
	t.Run("non-openai platform stays 503", func(t *testing.T) {
		diag := newModelAvailabilityService([]Account{restricted}, nil).DiagnoseModelAvailabilityForPlatform(ctx, nil, "o9-mini", PlatformGrok)
		require.True(t, diag.HasAccountsInPool)
		require.True(t, diag.HasModelSupport)
	})
	t.Run("nil receiver", func(t *testing.T) {
		var svc *OpenAIGatewayService
		diag := svc.DiagnoseModelAvailabilityForPlatform(ctx, nil, "o9-mini", PlatformOpenAI)
		require.True(t, diag.HasModelSupport)
	})
}
