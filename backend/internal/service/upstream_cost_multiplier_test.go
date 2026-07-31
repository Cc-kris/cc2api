package service

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestValidateUpstreamCostMultiplier(t *testing.T) {
	for _, valid := range []string{"0", "0.0001", "1", "1.2500", "9999.9999"} {
		require.NoError(t, ValidateUpstreamCostMultiplier(decimal.RequireFromString(valid)), valid)
	}
	for _, invalid := range []string{"-1", "0.00001", "10000"} {
		require.Error(t, ValidateUpstreamCostMultiplier(decimal.RequireFromString(invalid)), invalid)
	}
}

func TestResolveUpstreamCostMultiplierSnapshot(t *testing.T) {
	accountValue := decimal.RequireFromString("0.8000")
	requestValue := decimal.RequireFromString("0.7250")
	account := &Account{UpstreamCostMultiplier: &accountValue}

	snapshot := ResolveUpstreamCostMultiplierSnapshot(&requestValue, account)
	require.NotNil(t, snapshot)
	require.True(t, snapshot.Equal(decimal.RequireFromString("0.7250")))

	requestValue = decimal.RequireFromString("9.9999")
	accountValue = decimal.RequireFromString("8.8888")
	require.True(t, snapshot.Equal(decimal.RequireFromString("0.7250")), "snapshot must remain immutable")

	fallback := ResolveUpstreamCostMultiplierSnapshot(nil, account)
	require.Nil(t, fallback)
	require.Nil(t, ResolveUpstreamCostMultiplierSnapshot(nil, &Account{}))
}
