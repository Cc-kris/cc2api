//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestFinanceExportJobPersistsAndDownloadTokenIsSingleUse(t *testing.T) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var operatorID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, "finance-export-"+suffix+"@example.test",
	).Scan(&operatorID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_export_jobs WHERE async_job_id IN (SELECT id FROM finance_async_jobs WHERE job_type=$1 AND operator_id=$2)`, service.FinanceExportJobType, operatorID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM finance_async_jobs WHERE job_type=$1 AND operator_id=$2`, service.FinanceExportJobType, operatorID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, operatorID)
	})

	repo := NewFinanceExportRepository(integrationDB)
	request := service.FinanceExportRequest{
		Report: "breakdown", Format: "csv", Timezone: "UTC",
		Filters: service.FinanceExportFilters{StartDate: "2026-07-01", EndDate: "2026-07-31", Dimension: "requested_model", DataScope: "all", SortBy: "profit", SortOrder: "asc"},
	}
	created, err := repo.CreateFinanceExportJob(ctx, request, operatorID, "idem-"+suffix, "checksum-a")
	require.NoError(t, err)
	require.Equal(t, "queued", created.Status)

	duplicate, err := repo.CreateFinanceExportJob(ctx, request, operatorID, "idem-"+suffix, "checksum-a")
	require.NoError(t, err)
	require.Equal(t, created.ID, duplicate.ID)
	_, err = repo.CreateFinanceExportJob(ctx, request, operatorID, "idem-"+suffix, "checksum-b")
	require.True(t, service.IsFinanceExportError(err, "IDEMPOTENCY_KEY_REUSED"))

	now := time.Now().UTC().Truncate(time.Second)
	claimed, err := repo.ClaimFinanceExportJob(ctx, "worker-a", now)
	require.NoError(t, err)
	require.Equal(t, created.ID, claimed.ID)
	require.NoError(t, repo.UpdateFinanceExportProgress(ctx, created.ID, "worker-a", 2, decimal.RequireFromString("0.5"), now.Add(time.Second)))
	require.NoError(t, repo.CompleteFinanceExportJob(ctx, created.ID, "worker-a", "/tmp/finance-export.csv", 120, 2, now.Add(time.Hour), now.Add(2*time.Second)))

	completed, err := repo.GetFinanceExportJob(ctx, created.ID, operatorID)
	require.NoError(t, err)
	require.Equal(t, "completed", completed.Status)
	require.Equal(t, int64(2), completed.ProcessedCount)
	require.Equal(t, int64(2), *completed.RowCount)
	_, err = repo.GetFinanceExportJob(ctx, created.ID, operatorID+999999)
	require.True(t, service.IsFinanceExportError(err, "JOB_NOT_FOUND"))

	tokenHash := "token-hash-" + suffix
	require.NoError(t, repo.SetFinanceExportDownloadToken(ctx, created.ID, operatorID, tokenHash, now.Add(15*time.Minute)))
	consumed, err := repo.ConsumeFinanceExportDownloadToken(ctx, created.ID, operatorID, tokenHash, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, created.ID, consumed.ID)
	_, err = repo.ConsumeFinanceExportDownloadToken(ctx, created.ID, operatorID, tokenHash, now.Add(2*time.Minute))
	require.True(t, service.IsFinanceExportError(err, "DOWNLOAD_TOKEN_INVALID"))
}
