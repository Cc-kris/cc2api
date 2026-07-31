//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestFinancePriceLookupUsesFrozenManualFXRate(t *testing.T) {
	ctx := context.Background()
	effective := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	source := "manual-test-" + uuid.NewString()
	created, err := NewFinanceFXRateRepository(integrationDB).CreateFinanceFXRate(ctx, service.FinanceFXRateCreateInput{
		Currency: "CNY", RateToUSD: "0.138", Source: source, ObservedAt: effective, EffectiveFrom: effective, ChangeReason: "集成测试汇率版本",
	}, decimal.RequireFromString("0.138"), uuid.NewString())
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_fx_rate_versions WHERE id=$1`, created.ID)
	})

	repo := &financePriceLookupRepository{client: integrationEntClient}
	selected, err := repo.findFXRateAt(ctx, "CNY", effective.Add(time.Minute))
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, created.ID, selected.ID)
	require.Equal(t, "0.138", selected.RateToUsd.String())
	require.Equal(t, source, selected.Source)
}
