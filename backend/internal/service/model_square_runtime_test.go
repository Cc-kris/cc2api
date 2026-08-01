//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGetModelSquareRuntimeUsesDedicatedSetting(t *testing.T) {
	svc := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyAvailableChannelsEnabled: "true",
		SettingKeyModelSquareEnabled:       "false",
	}}, &config.Config{})
	require.False(t, svc.GetModelSquareRuntime(context.Background()).Enabled)

	svc = NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyAvailableChannelsEnabled: "false",
		SettingKeyModelSquareEnabled:       "true",
	}}, &config.Config{})
	require.True(t, svc.GetModelSquareRuntime(context.Background()).Enabled)
}

func TestGetModelSquareRuntimeDoesNotDependOnSalesPricingVersion(t *testing.T) {
	svc := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyModelSquareEnabled:  "true",
		SettingKeySalesPricingVersion: string(SalesPricingVersionLegacy),
	}}, &config.Config{})
	runtime := svc.GetModelSquareRuntime(context.Background())
	require.True(t, runtime.Enabled)
}

func TestGetModelSquareRuntimeFailsClosed(t *testing.T) {
	var nilService *SettingService
	require.False(t, nilService.GetModelSquareRuntime(context.Background()).Enabled)

	svc := NewSettingService(nil, &config.Config{})
	require.False(t, svc.GetModelSquareRuntime(context.Background()).Enabled)

	svc = NewSettingService(&settingRepoStub{err: errors.New("read failed")}, &config.Config{})
	require.False(t, svc.GetModelSquareRuntime(context.Background()).Enabled)
}
