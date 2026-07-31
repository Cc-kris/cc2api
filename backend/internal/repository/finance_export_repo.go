package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type financeExportRepository struct{ db *sql.DB }

func NewFinanceExportRepository(db *sql.DB) service.FinanceExportRepository {
	return &financeExportRepository{db: db}
}

func (r *financeExportRepository) CreateFinanceExportJob(ctx context.Context, request service.FinanceExportRequest, operatorID int64, idempotencyKey, requestChecksum string) (*service.FinanceExportJob, error) {
	filtersJSON, err := json.Marshal(request.Filters)
	if err != nil {
		return nil, err
	}
	parametersJSON, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var jobID int64
	if idempotencyKey == "" {
		err = tx.QueryRowContext(ctx, `
INSERT INTO finance_async_jobs(job_type,status,request_checksum,parameters,operator_id)
VALUES($1,'queued',$2,$3::jsonb,$4) RETURNING id`,
			service.FinanceExportJobType, requestChecksum, string(parametersJSON), operatorID).Scan(&jobID)
	} else {
		err = tx.QueryRowContext(ctx, `
INSERT INTO finance_async_jobs(job_type,status,idempotency_key,request_checksum,parameters,operator_id)
VALUES($1,'queued',$2,$3,$4::jsonb,$5)
ON CONFLICT(job_type,operator_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
RETURNING id`, service.FinanceExportJobType, idempotencyKey, requestChecksum, string(parametersJSON), operatorID).Scan(&jobID)
		if errors.Is(err, sql.ErrNoRows) {
			var existingChecksum string
			err = tx.QueryRowContext(ctx, `SELECT id,COALESCE(request_checksum,'') FROM finance_async_jobs WHERE job_type=$1 AND operator_id=$2 AND idempotency_key=$3`,
				service.FinanceExportJobType, operatorID, idempotencyKey).Scan(&jobID, &existingChecksum)
			if err != nil {
				return nil, err
			}
			if existingChecksum != requestChecksum {
				return nil, &service.FinanceExportError{Code: "IDEMPOTENCY_KEY_REUSED", Message: "idempotency key was already used with a different finance export request"}
			}
			if err = tx.Commit(); err != nil {
				return nil, err
			}
			return r.GetFinanceExportJob(ctx, jobID, operatorID)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("create finance export async job: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO finance_export_jobs(async_job_id,report,format,filters,timezone)
VALUES($1,$2,$3,$4::jsonb,$5)`, jobID, request.Report, request.Format, string(filtersJSON), request.Timezone)
	if err != nil {
		return nil, fmt.Errorf("create finance export job: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetFinanceExportJob(ctx, jobID, operatorID)
}

func (r *financeExportRepository) GetFinanceExportJob(ctx context.Context, jobID, operatorID int64) (*service.FinanceExportJob, error) {
	return scanFinanceExportJob(r.db.QueryRowContext(ctx, financeExportJobSelect+` WHERE a.id=$1 AND a.job_type=$2 AND a.operator_id=$3`, jobID, service.FinanceExportJobType, operatorID))
}

func (r *financeExportRepository) ClaimFinanceExportJob(ctx context.Context, leaseOwner string, now time.Time) (*service.FinanceExportJob, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var jobID, operatorID int64
	err = tx.QueryRowContext(ctx, `
SELECT id,COALESCE(operator_id,0) FROM finance_async_jobs
WHERE job_type=$1 AND (status='queued' OR (status='running' AND (lease_expires_at IS NULL OR lease_expires_at<$2)))
ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, service.FinanceExportJobType, now).Scan(&jobID, &operatorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE finance_async_jobs SET status='running',started_at=COALESCE(started_at,$2),lease_owner=$3,
lease_expires_at=$2+INTERVAL '2 minutes',updated_at=$2 WHERE id=$1`, jobID, now, leaseOwner)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetFinanceExportJob(ctx, jobID, operatorID)
}

func (r *financeExportRepository) UpdateFinanceExportProgress(ctx context.Context, jobID int64, leaseOwner string, processed int64, progress decimal.Decimal, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE finance_async_jobs SET progress=$3,processed_count=processed_count+$4,success_count=success_count+$4,
lease_expires_at=$5::timestamptz+INTERVAL '2 minutes',updated_at=$5::timestamptz
WHERE id=$1 AND lease_owner=$2 AND job_type=$6 AND status='running'`,
		jobID, leaseOwner, progress, processed, now, service.FinanceExportJobType)
	return requireFinanceExportLease(result, err)
}

func (r *financeExportRepository) RenewFinanceExportLease(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE finance_async_jobs
SET lease_expires_at=$3::timestamptz+INTERVAL '2 minutes',updated_at=$3::timestamptz
WHERE id=$1 AND lease_owner=$2 AND job_type=$4 AND status='running'`,
		jobID, leaseOwner, now, service.FinanceExportJobType)
	return requireFinanceExportLease(result, err)
}

func (r *financeExportRepository) CompleteFinanceExportJob(ctx context.Context, jobID int64, leaseOwner, storageKey string, fileSize, rowCount int64, expiresAt, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
UPDATE finance_async_jobs SET status='completed',progress=1,finished_at=$3,lease_owner=NULL,lease_expires_at=NULL,updated_at=$3
WHERE id=$1 AND lease_owner=$2 AND job_type=$4 AND status='running'`, jobID, leaseOwner, now, service.FinanceExportJobType)
	if err = requireFinanceExportLease(result, err); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE finance_export_jobs SET storage_key=$2,file_size=$3,row_count=$4,expires_at=$5 WHERE async_job_id=$1`,
		jobID, storageKey, fileSize, rowCount, expiresAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *financeExportRepository) FailFinanceExportJob(ctx context.Context, jobID int64, leaseOwner, message string, now time.Time) error {
	if len(message) > 4000 {
		message = message[:4000]
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE finance_async_jobs SET status='failed',failed_count=failed_count+1,error_summary=$3,finished_at=$4,
lease_owner=NULL,lease_expires_at=NULL,updated_at=$4
WHERE id=$1 AND lease_owner=$2 AND job_type=$5 AND status='running'`, jobID, leaseOwner, message, now, service.FinanceExportJobType)
	return requireFinanceExportLease(result, err)
}

func (r *financeExportRepository) ReleaseFinanceExportJob(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE finance_async_jobs SET status='queued',lease_owner=NULL,lease_expires_at=NULL,updated_at=$3
WHERE id=$1 AND lease_owner=$2 AND job_type=$4 AND status='running'`, jobID, leaseOwner, now, service.FinanceExportJobType)
	return requireFinanceExportLease(result, err)
}

func (r *financeExportRepository) SetFinanceExportDownloadToken(ctx context.Context, jobID, operatorID int64, tokenHash string, expiresAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE finance_export_jobs e SET download_token_hash=$3,download_token_expires_at=$4,downloaded_at=NULL
FROM finance_async_jobs a
WHERE e.async_job_id=a.id AND a.id=$1 AND a.operator_id=$2 AND a.status='completed' AND e.expires_at>NOW()`,
		jobID, operatorID, tokenHash, expiresAt)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return &service.FinanceExportError{Code: "EXPORT_NOT_DOWNLOADABLE", Message: "finance export is not available for download"}
	}
	return nil
}

func (r *financeExportRepository) ConsumeFinanceExportDownloadToken(ctx context.Context, jobID, operatorID int64, tokenHash string, now time.Time) (*service.FinanceExportJob, error) {
	result, err := r.db.ExecContext(ctx, `
UPDATE finance_export_jobs e SET downloaded_at=$4,download_token_hash=NULL,download_token_expires_at=NULL
FROM finance_async_jobs a
WHERE e.async_job_id=a.id AND a.id=$1 AND a.operator_id=$2 AND a.status='completed'
  AND e.download_token_hash=$3 AND e.download_token_expires_at>$4 AND e.downloaded_at IS NULL AND e.expires_at>$4`,
		jobID, operatorID, tokenHash, now)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows != 1 {
		return nil, &service.FinanceExportError{Code: "DOWNLOAD_TOKEN_INVALID", Message: "finance export download token is invalid, expired, or already used"}
	}
	return r.GetFinanceExportJob(ctx, jobID, operatorID)
}

func requireFinanceExportLease(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return &service.FinanceExportError{Code: "JOB_LEASE_LOST", Message: "finance export job lease is no longer owned by this worker"}
	}
	return nil
}

const financeExportJobSelect = `
SELECT a.id,a.status,a.progress,a.processed_count,a.success_count,a.failed_count,
       e.report,e.format,e.filters,e.timezone,COALESCE(a.operator_id,0),COALESCE(e.storage_key,''),
       e.file_size,e.row_count,e.expires_at,a.created_at,a.started_at,a.finished_at,a.error_summary
FROM finance_async_jobs a JOIN finance_export_jobs e ON e.async_job_id=a.id`

type financeExportScanner interface{ Scan(dest ...any) error }

func scanFinanceExportJob(row financeExportScanner) (*service.FinanceExportJob, error) {
	var job service.FinanceExportJob
	var filtersJSON []byte
	var fileSize, rowCount sql.NullInt64
	var expiresAt, startedAt, finishedAt sql.NullTime
	var errorSummary sql.NullString
	err := row.Scan(
		&job.ID, &job.Status, &job.Progress, &job.ProcessedCount, &job.SuccessCount, &job.FailedCount,
		&job.Report, &job.Format, &filtersJSON, &job.Request.Timezone, &job.OperatorID, &job.StorageKey,
		&fileSize, &rowCount, &expiresAt, &job.CreatedAt, &startedAt, &finishedAt, &errorSummary,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &service.FinanceExportError{Code: "JOB_NOT_FOUND", Message: "finance export job not found"}
	}
	if err != nil {
		return nil, err
	}
	job.Type = service.FinanceExportJobType
	job.Request.Report = job.Report
	job.Request.Format = job.Format
	if err = json.Unmarshal(filtersJSON, &job.Request.Filters); err != nil {
		return nil, err
	}
	if fileSize.Valid {
		job.FileSize = &fileSize.Int64
	}
	if rowCount.Valid {
		job.RowCount = &rowCount.Int64
	}
	if expiresAt.Valid {
		job.ExpiresAt = &expiresAt.Time
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		job.FinishedAt = &finishedAt.Time
	}
	if errorSummary.Valid {
		job.ErrorSummary = &errorSummary.String
	}
	return &job, nil
}
