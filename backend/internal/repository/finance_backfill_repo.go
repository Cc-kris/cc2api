package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type financeBackfillRepository struct {
	sql *sql.DB
}

func NewFinanceBackfillRepository(sqlDB *sql.DB) service.FinanceBackfillRepository {
	return &financeBackfillRepository{sql: sqlDB}
}

func (r *financeBackfillRepository) CountFinanceBackfillCandidates(ctx context.Context, request service.FinanceBackfillRequest) (int64, error) {
	where, args := financeBackfillCandidateWhere(request, service.FinanceBackfillCursor{})
	var count int64
	err := r.sql.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM usage_logs u
LEFT JOIN usage_finance_records f ON f.usage_log_id=u.id
WHERE `+where, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count finance backfill candidates: %w", err)
	}
	return count, nil
}

func (r *financeBackfillRepository) ListFinanceBackfillCandidates(ctx context.Context, request service.FinanceBackfillRequest, cursor service.FinanceBackfillCursor, limit int) ([]service.FinanceBackfillCandidate, error) {
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	where, args := financeBackfillCandidateWhere(request, cursor)
	args = append(args, limit)
	columns := "u." + strings.ReplaceAll(usageLogSelectColumns, ", ", ", u.")
	rows, err := r.sql.QueryContext(ctx, `
SELECT `+columns+`
FROM usage_logs u
LEFT JOIN usage_finance_records f ON f.usage_log_id=u.id
WHERE `+where+`
ORDER BY u.created_at ASC,u.id ASC
LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list finance backfill candidates: %w", err)
	}
	defer rows.Close()
	logs := make([]service.UsageLog, 0, limit)
	ids := make([]int64, 0, limit)
	for rows.Next() {
		log, scanErr := scanUsageLog(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		logs = append(logs, *log)
		ids = append(ids, log.ID)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = attachFinanceUsageClassifications(ctx, r.sql, logs); err != nil {
		return nil, err
	}
	existing := make(map[int64]string, len(ids))
	if len(ids) > 0 {
		existingRows, queryErr := r.sql.QueryContext(ctx, `SELECT usage_log_id,business_type FROM usage_finance_records WHERE usage_log_id=ANY($1)`, pq.Array(ids))
		if queryErr != nil {
			return nil, fmt.Errorf("load finance backfill projection existence: %w", queryErr)
		}
		defer existingRows.Close()
		for existingRows.Next() {
			var id int64
			var businessType string
			if scanErr := existingRows.Scan(&id, &businessType); scanErr != nil {
				return nil, scanErr
			}
			existing[id] = businessType
		}
		if queryErr = existingRows.Err(); queryErr != nil {
			return nil, queryErr
		}
	}
	result := make([]service.FinanceBackfillCandidate, 0, len(logs))
	for i := range logs {
		businessType, hasProjection := existing[logs[i].ID]
		if hasProjection {
			logs[i].FinanceBusinessTypeSnapshot = businessType
		}
		result = append(result, service.FinanceBackfillCandidate{UsageLog: logs[i], HasProjection: hasProjection})
	}
	return result, nil
}

func financeBackfillCandidateWhere(request service.FinanceBackfillRequest, cursor service.FinanceBackfillCursor) (string, []any) {
	args := []any{request.StartDate, request.EndDate}
	conditions := []string{
		"u.created_at >= $1::date",
		"u.created_at < ($2::date + INTERVAL '1 day')",
	}
	if !cursor.CreatedAt.IsZero() || cursor.ID > 0 {
		args = append(args, cursor.CreatedAt, cursor.ID)
		conditions = append(conditions, fmt.Sprintf("(u.created_at > $%d OR (u.created_at=$%d AND u.id>$%d))", len(args)-1, len(args)-1, len(args)))
	}
	if len(request.Scope.CostStatus) > 0 {
		args = append(args, pq.Array(request.Scope.CostStatus))
		conditions = append(conditions, fmt.Sprintf("COALESCE(f.cost_status,'missing_usage')=ANY($%d)", len(args)))
	}
	if len(request.Scope.AccountIDs) > 0 {
		args = append(args, pq.Array(request.Scope.AccountIDs))
		conditions = append(conditions, fmt.Sprintf("u.account_id=ANY($%d)", len(args)))
	}
	if len(request.Scope.WalletIDs) > 0 {
		args = append(args, pq.Array(request.Scope.WalletIDs))
		conditions = append(conditions, fmt.Sprintf(`(
f.wallet_id=ANY($%d) OR EXISTS(
 SELECT 1 FROM usage_finance_cost_segments s
 WHERE s.usage_finance_record_id=f.id AND s.wallet_id=ANY($%d)
))`, len(args), len(args)))
	}
	return strings.Join(conditions, " AND "), args
}

func (r *financeBackfillRepository) CreateFinanceBackfillJob(ctx context.Context, request service.FinanceBackfillRequest, operatorID int64, requestChecksum, previewTokenHash string, previewExpiresAt time.Time, estimatedTotal int64) (*service.FinanceBackfillJob, error) {
	tx, err := r.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	parameters := map[string]any{"estimated_total": estimatedTotal, "dry_run_sample_size": request.DryRunSampleSize}
	parametersJSON, err := json.Marshal(parameters)
	if err != nil {
		return nil, err
	}
	var jobID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO finance_async_jobs(job_type,status,idempotency_key,request_checksum,parameters,operator_id)
VALUES($1,'queued',$2,$3,$4::jsonb,$5)
ON CONFLICT(job_type,operator_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
RETURNING id`, service.FinanceBackfillJobType, previewTokenHash, requestChecksum, string(parametersJSON), operatorID).Scan(&jobID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `
SELECT id FROM finance_async_jobs
WHERE job_type=$1 AND operator_id=$2 AND idempotency_key=$3`, service.FinanceBackfillJobType, operatorID, previewTokenHash).Scan(&jobID)
		if err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return r.GetFinanceBackfillJob(ctx, jobID)
	}
	if err != nil {
		return nil, fmt.Errorf("create finance backfill async job: %w", err)
	}
	scopeJSON, err := json.Marshal(request.Scope)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO finance_backfill_jobs(async_job_id,start_date,end_date,mode,pricing_policy,preview_token_hash,preview_expires_at,scope,reason)
VALUES($1,$2::date,$3::date,'recalculate',$4,$5,$6,$7::jsonb,$8)`,
		jobID, request.StartDate, request.EndDate, request.PricingPolicy, previewTokenHash, previewExpiresAt, string(scopeJSON), request.Reason)
	if err != nil {
		return nil, fmt.Errorf("create finance backfill job: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetFinanceBackfillJob(ctx, jobID)
}

func (r *financeBackfillRepository) GetFinanceBackfillJob(ctx context.Context, jobID int64) (*service.FinanceBackfillJob, error) {
	return scanFinanceBackfillJob(r.sql.QueryRowContext(ctx, financeBackfillJobSelect+" WHERE a.id=$1 AND a.job_type=$2", jobID, service.FinanceBackfillJobType))
}

func (r *financeBackfillRepository) PauseFinanceBackfillJob(ctx context.Context, jobID int64) (*service.FinanceBackfillJob, error) {
	result, err := r.sql.ExecContext(ctx, `
UPDATE finance_async_jobs
SET status='paused',updated_at=NOW()
WHERE id=$1 AND job_type=$2 AND status IN('queued','running')`, jobID, service.FinanceBackfillJobType)
	if err != nil {
		return nil, err
	}
	return r.financeBackfillStateChangeResult(ctx, jobID, result)
}

func (r *financeBackfillRepository) ResumeFinanceBackfillJob(ctx context.Context, jobID int64) (*service.FinanceBackfillJob, error) {
	result, err := r.sql.ExecContext(ctx, `
UPDATE finance_async_jobs
SET status='queued',lease_owner=NULL,lease_expires_at=NULL,error_summary=NULL,finished_at=NULL,updated_at=NOW()
WHERE id=$1 AND job_type=$2 AND (
 status='failed' OR (status='paused' AND (lease_owner IS NULL OR lease_expires_at IS NULL OR lease_expires_at<NOW()))
)`, jobID, service.FinanceBackfillJobType)
	if err != nil {
		return nil, err
	}
	return r.financeBackfillStateChangeResult(ctx, jobID, result)
}

func (r *financeBackfillRepository) financeBackfillStateChangeResult(ctx context.Context, jobID int64, result sql.Result) (*service.FinanceBackfillJob, error) {
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	job, getErr := r.GetFinanceBackfillJob(ctx, jobID)
	if getErr != nil {
		return nil, getErr
	}
	if rows == 0 {
		return nil, &service.FinanceBackfillError{Code: "JOB_STATE_CONFLICT", Message: fmt.Sprintf("finance backfill job cannot transition from status %s", job.Status)}
	}
	return job, nil
}

func (r *financeBackfillRepository) ClaimFinanceBackfillJob(ctx context.Context, leaseOwner string, now time.Time) (*service.FinanceBackfillJob, error) {
	tx, err := r.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var jobID int64
	err = tx.QueryRowContext(ctx, `
SELECT id FROM finance_async_jobs
WHERE job_type=$1 AND (
 status='queued' OR (status='running' AND (lease_expires_at IS NULL OR lease_expires_at<$2))
)
ORDER BY created_at ASC,id ASC
FOR UPDATE SKIP LOCKED
LIMIT 1`, service.FinanceBackfillJobType, now).Scan(&jobID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE finance_async_jobs
SET status='running',started_at=COALESCE(started_at,$2),lease_owner=$3,lease_expires_at=$2+INTERVAL '2 minutes',updated_at=$2
WHERE id=$1`, jobID, now, leaseOwner)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetFinanceBackfillJob(ctx, jobID)
}

func (r *financeBackfillRepository) RenewFinanceBackfillLease(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error {
	result, err := r.sql.ExecContext(ctx, `
UPDATE finance_async_jobs
SET lease_expires_at=$3::timestamptz+INTERVAL '2 minutes',updated_at=$3::timestamptz
WHERE id=$1 AND lease_owner=$2 AND job_type=$4 AND status='running'`,
		jobID, leaseOwner, now, service.FinanceBackfillJobType)
	return requireFinanceBackfillLeaseResult(result, err)
}

func (r *financeBackfillRepository) AcknowledgeFinanceBackfillPause(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error {
	result, err := r.sql.ExecContext(ctx, `
UPDATE finance_async_jobs
SET lease_owner=NULL,lease_expires_at=NULL,updated_at=$3
WHERE id=$1 AND lease_owner=$2 AND job_type=$4 AND status='paused'`,
		jobID, leaseOwner, now, service.FinanceBackfillJobType)
	return requireFinanceBackfillLeaseResult(result, err)
}

func (r *financeBackfillRepository) UpdateFinanceBackfillProgress(ctx context.Context, jobID int64, leaseOwner string, cursor service.FinanceBackfillCursor, processed, succeeded int64, progress decimal.Decimal, now time.Time) error {
	cursorJSON, err := json.Marshal(cursor)
	if err != nil {
		return err
	}
	result, err := r.sql.ExecContext(ctx, `
UPDATE finance_async_jobs
SET cursor=$3::jsonb,processed_count=processed_count+$4,success_count=success_count+$5,
progress=$6,updated_at=$7
WHERE id=$1 AND lease_owner=$2 AND job_type=$8 AND status='running'`,
		jobID, leaseOwner, string(cursorJSON), processed, succeeded, progress, now, service.FinanceBackfillJobType)
	return requireFinanceBackfillLeaseResult(result, err)
}

func (r *financeBackfillRepository) ReleaseFinanceBackfillJob(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error {
	result, err := r.sql.ExecContext(ctx, `
UPDATE finance_async_jobs
SET status='queued',lease_owner=NULL,lease_expires_at=NULL,updated_at=$3
WHERE id=$1 AND lease_owner=$2 AND job_type=$4 AND status='running'`,
		jobID, leaseOwner, now, service.FinanceBackfillJobType)
	return requireFinanceBackfillLeaseResult(result, err)
}

func (r *financeBackfillRepository) CompleteFinanceBackfillJob(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error {
	result, err := r.sql.ExecContext(ctx, `
UPDATE finance_async_jobs
SET status='completed',progress=1,finished_at=$3,lease_owner=NULL,lease_expires_at=NULL,updated_at=$3
WHERE id=$1 AND lease_owner=$2 AND job_type=$4 AND status='running'`, jobID, leaseOwner, now, service.FinanceBackfillJobType)
	return requireFinanceBackfillLeaseResult(result, err)
}

func (r *financeBackfillRepository) FailFinanceBackfillJob(ctx context.Context, jobID int64, leaseOwner, message string, now time.Time) error {
	if len(message) > 4000 {
		message = message[:4000]
	}
	result, err := r.sql.ExecContext(ctx, `
UPDATE finance_async_jobs
SET status='failed',failed_count=failed_count+1,error_summary=$3,finished_at=$4,
lease_owner=NULL,lease_expires_at=NULL,updated_at=$4
WHERE id=$1 AND lease_owner=$2 AND job_type=$5 AND status='running'`, jobID, leaseOwner, message, now, service.FinanceBackfillJobType)
	return requireFinanceBackfillLeaseResult(result, err)
}

func requireFinanceBackfillLeaseResult(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return &service.FinanceBackfillError{Code: "JOB_LEASE_LOST", Message: "finance backfill job lease is no longer owned by this worker"}
	}
	return nil
}

const financeBackfillJobSelect = `
SELECT a.id,a.status,b.start_date,b.end_date,b.scope,b.pricing_policy,b.reason,
       a.progress,a.processed_count,a.success_count,a.failed_count,
       COALESCE((a.parameters->>'estimated_total')::bigint,0),a.cursor,a.error_summary,
       COALESCE(a.operator_id,0),a.created_at,a.started_at,a.finished_at,a.updated_at,b.preview_expires_at
FROM finance_async_jobs a
JOIN finance_backfill_jobs b ON b.async_job_id=a.id`

func scanFinanceBackfillJob(row *sql.Row) (*service.FinanceBackfillJob, error) {
	var (
		job        service.FinanceBackfillJob
		startDate  time.Time
		endDate    time.Time
		scopeJSON  []byte
		cursorJSON []byte
		errorText  sql.NullString
		startedAt  sql.NullTime
		finishedAt sql.NullTime
	)
	err := row.Scan(
		&job.ID, &job.Status, &startDate, &endDate, &scopeJSON, &job.PricingPolicy, &job.Reason,
		&job.Progress, &job.ProcessedCount, &job.SuccessCount, &job.FailedCount, &job.EstimatedTotal,
		&cursorJSON, &errorText, &job.OperatorID, &job.CreatedAt, &startedAt, &finishedAt, &job.UpdatedAt, &job.PreviewExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &service.FinanceBackfillError{Code: "JOB_NOT_FOUND", Message: "finance backfill job not found"}
	}
	if err != nil {
		return nil, err
	}
	job.StartDate = startDate.Format("2006-01-02")
	job.EndDate = endDate.Format("2006-01-02")
	if len(scopeJSON) > 0 {
		if err = json.Unmarshal(scopeJSON, &job.Scope); err != nil {
			return nil, err
		}
	}
	if len(cursorJSON) > 0 {
		if err = json.Unmarshal(cursorJSON, &job.Cursor); err != nil {
			return nil, err
		}
	}
	if errorText.Valid {
		job.ErrorSummary = &errorText.String
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		job.FinishedAt = &finishedAt.Time
	}
	return &job, nil
}
