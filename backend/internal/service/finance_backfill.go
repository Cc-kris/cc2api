package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/shopspring/decimal"
)

const (
	FinanceBackfillJobType           = "finance_backfill"
	FinanceBackfillPricingHistorical = "historical_only"
	financeBackfillPreviewTTL        = 30 * time.Minute
	financeBackfillBatchSize         = 200
	financeBackfillLeaseDuration     = 2 * time.Minute
)

type FinanceBackfillScope struct {
	CostStatus []string `json:"cost_status,omitempty"`
	AccountIDs []int64  `json:"account_ids,omitempty"`
	WalletIDs  []int64  `json:"wallet_ids,omitempty"`
}

type FinanceBackfillRequest struct {
	StartDate        string               `json:"start_date"`
	EndDate          string               `json:"end_date"`
	Scope            FinanceBackfillScope `json:"scope"`
	PricingPolicy    string               `json:"pricing_policy"`
	DryRunSampleSize int                  `json:"dry_run_sample_size,omitempty"`
	Reason           string               `json:"reason"`
	PreviewToken     string               `json:"preview_token,omitempty"`
}

type FinanceBackfillPreview struct {
	EstimatedRecords    int64     `json:"estimated_records"`
	ExactRepairable     int64     `json:"exact_repairable"`
	EstimatedOnly       int64     `json:"estimated_only"`
	Unrepairable        int64     `json:"unrepairable"`
	EstimatedScanBytes  int64     `json:"estimated_scan_bytes"`
	SampleSize          int       `json:"sample_size"`
	PendingModels       []string  `json:"pending_models"`
	AmbiguousAccountIDs []int64   `json:"ambiguous_account_ids"`
	Blockers            []string  `json:"blockers"`
	PreviewToken        string    `json:"preview_token"`
	ExpiresAt           time.Time `json:"expires_at"`
}

type FinanceBackfillCursor struct {
	CreatedAt time.Time `json:"created_at,omitempty"`
	ID        int64     `json:"id,omitempty"`
}

type FinanceBackfillCandidate struct {
	UsageLog      UsageLog
	HasProjection bool
}

type FinanceBackfillJob struct {
	ID               int64                 `json:"job_id"`
	Status           string                `json:"status"`
	StartDate        string                `json:"start_date"`
	EndDate          string                `json:"end_date"`
	Scope            FinanceBackfillScope  `json:"scope"`
	PricingPolicy    string                `json:"pricing_policy"`
	Reason           string                `json:"reason"`
	Progress         decimal.Decimal       `json:"progress"`
	ProcessedCount   int64                 `json:"processed_count"`
	SuccessCount     int64                 `json:"success_count"`
	FailedCount      int64                 `json:"failed_count"`
	EstimatedTotal   int64                 `json:"estimated_total"`
	Cursor           FinanceBackfillCursor `json:"cursor"`
	ErrorSummary     *string               `json:"error_summary,omitempty"`
	OperatorID       int64                 `json:"operator_id"`
	CreatedAt        time.Time             `json:"created_at"`
	StartedAt        *time.Time            `json:"started_at,omitempty"`
	FinishedAt       *time.Time            `json:"finished_at,omitempty"`
	UpdatedAt        time.Time             `json:"updated_at"`
	PreviewExpiresAt time.Time             `json:"preview_expires_at"`
}

type FinanceBackfillRepository interface {
	CountFinanceBackfillCandidates(ctx context.Context, request FinanceBackfillRequest) (int64, error)
	ListFinanceBackfillCandidates(ctx context.Context, request FinanceBackfillRequest, cursor FinanceBackfillCursor, limit int) ([]FinanceBackfillCandidate, error)
	CreateFinanceBackfillJob(ctx context.Context, request FinanceBackfillRequest, operatorID int64, requestChecksum, previewTokenHash string, previewExpiresAt time.Time, estimatedTotal int64) (*FinanceBackfillJob, error)
	GetFinanceBackfillJob(ctx context.Context, jobID int64) (*FinanceBackfillJob, error)
	PauseFinanceBackfillJob(ctx context.Context, jobID int64) (*FinanceBackfillJob, error)
	ResumeFinanceBackfillJob(ctx context.Context, jobID int64) (*FinanceBackfillJob, error)
	ClaimFinanceBackfillJob(ctx context.Context, leaseOwner string, now time.Time) (*FinanceBackfillJob, error)
	RenewFinanceBackfillLease(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error
	AcknowledgeFinanceBackfillPause(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error
	UpdateFinanceBackfillProgress(ctx context.Context, jobID int64, leaseOwner string, cursor FinanceBackfillCursor, processed, succeeded int64, progress decimal.Decimal, now time.Time) error
	ReleaseFinanceBackfillJob(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error
	CompleteFinanceBackfillJob(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error
	FailFinanceBackfillJob(ctx context.Context, jobID int64, leaseOwner, message string, now time.Time) error
}

type FinanceBackfillError struct {
	Code    string
	Message string
}

func (e *FinanceBackfillError) Error() string { return e.Message }

func IsFinanceBackfillError(err error, code string) bool {
	var target *FinanceBackfillError
	return errors.As(err, &target) && target.Code == code
}

type financeBackfillPreviewToken struct {
	Checksum string `json:"checksum"`
	Expires  int64  `json:"expires"`
	Blocked  bool   `json:"blocked"`
}

type FinanceBackfillService struct {
	repository FinanceBackfillRepository
	ledger     FinanceLedgerRepository
	scanner    *FinanceUsageScanner
	revenue    *FinanceRevenueRecognitionService
	timezone   string
	tokenKey   []byte
	leaseOwner string
	now        func() time.Time
}

func NewFinanceBackfillService(repository FinanceBackfillRepository, ledger FinanceLedgerRepository, scanner *FinanceUsageScanner, cfg *config.Config) *FinanceBackfillService {
	timezone := "Asia/Shanghai"
	if cfg != nil && strings.TrimSpace(cfg.Timezone) != "" {
		timezone = strings.TrimSpace(cfg.Timezone)
	}
	var key []byte
	if cfg != nil && strings.TrimSpace(cfg.JWT.Secret) != "" {
		digest := sha256.Sum256([]byte("finance-backfill-preview:" + cfg.JWT.Secret))
		key = digest[:]
	} else {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic(fmt.Sprintf("initialize finance backfill token key: %v", err))
		}
	}
	ownerBytes := make([]byte, 12)
	if _, err := rand.Read(ownerBytes); err != nil {
		panic(fmt.Sprintf("initialize finance backfill lease owner: %v", err))
	}
	return &FinanceBackfillService{
		repository: repository,
		ledger:     ledger,
		scanner:    scanner,
		timezone:   timezone,
		tokenKey:   key,
		leaseOwner: "finance-backfill-" + hex.EncodeToString(ownerBytes),
		now:        time.Now,
	}
}

func (s *FinanceBackfillService) Preview(ctx context.Context, input FinanceBackfillRequest) (*FinanceBackfillPreview, error) {
	request, err := normalizeFinanceBackfillRequest(input)
	if err != nil {
		return nil, err
	}
	total, err := s.repository.CountFinanceBackfillCandidates(ctx, request)
	if err != nil {
		return nil, err
	}
	sampleLimit := request.DryRunSampleSize
	if int64(sampleLimit) > total {
		sampleLimit = int(total)
	}
	candidates, err := s.repository.ListFinanceBackfillCandidates(ctx, request, FinanceBackfillCursor{}, sampleLimit)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(candidates))
	for i := range candidates {
		ids = append(ids, candidates[i].UsageLog.ID)
	}
	attemptsByUsage, err := s.ledger.LoadUsageAttempts(ctx, ids)
	if err != nil {
		return nil, err
	}
	launchAt, err := s.ledger.FinanceLaunchAt(ctx)
	if err != nil {
		return nil, err
	}
	var exact, estimated, unrepairable int64
	pendingModels := make(map[string]struct{})
	ambiguousAccounts := make(map[int64]struct{})
	blockers := make([]string, 0)
	for i := range candidates {
		candidate := &candidates[i]
		projection, buildErr := s.scanner.buildProjection(ctx, &candidate.UsageLog, attemptsByUsage[candidate.UsageLog.ID], launchAt)
		if buildErr != nil {
			blockers = append(blockers, fmt.Sprintf("usage_log_id=%d: %v", candidate.UsageLog.ID, buildErr))
			unrepairable++
			continue
		}
		switch projection.CostStatus {
		case FinanceCostStatusExact:
			exact++
		case FinanceCostStatusEstimated:
			estimated++
		default:
			unrepairable++
			model := strings.TrimSpace(projection.RequestedModel)
			if model != "" {
				pendingModels[model] = struct{}{}
			}
			if projection.CostStatus == FinanceCostStatusMissingProfile && projection.AccountID != nil {
				ambiguousAccounts[*projection.AccountID] = struct{}{}
			}
		}
	}
	exact = estimateFinanceBackfillCount(exact, int64(len(candidates)), total)
	estimated = estimateFinanceBackfillCount(estimated, int64(len(candidates)), total)
	if exact > total {
		exact = total
	}
	if estimated > total-exact {
		estimated = total - exact
	}
	unrepairable = total - exact - estimated
	if unrepairable < 0 {
		unrepairable = 0
	}
	checksum, err := financeBackfillRequestChecksum(request)
	if err != nil {
		return nil, err
	}
	expiresAt := s.now().UTC().Add(financeBackfillPreviewTTL)
	token, err := s.signPreviewToken(financeBackfillPreviewToken{Checksum: checksum, Expires: expiresAt.Unix(), Blocked: len(blockers) > 0})
	if err != nil {
		return nil, err
	}
	return &FinanceBackfillPreview{
		EstimatedRecords: total, ExactRepairable: exact, EstimatedOnly: estimated, Unrepairable: unrepairable,
		EstimatedScanBytes: total * 1024, SampleSize: len(candidates), PendingModels: sortedFinanceBackfillStrings(pendingModels),
		AmbiguousAccountIDs: sortedFinanceBackfillInt64s(ambiguousAccounts), Blockers: blockers, PreviewToken: token, ExpiresAt: expiresAt,
	}, nil
}

func (s *FinanceBackfillService) Run(ctx context.Context, input FinanceBackfillRequest, operatorID int64) (*FinanceBackfillJob, error) {
	if operatorID <= 0 {
		return nil, financeValidationError("finance backfill operator is required")
	}
	request, err := normalizeFinanceBackfillRequest(input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.PreviewToken) == "" {
		return nil, &FinanceBackfillError{Code: "BACKFILL_PRECONDITION_FAILED", Message: "preview_token is required"}
	}
	payload, err := s.verifyPreviewToken(input.PreviewToken)
	if err != nil {
		return nil, &FinanceBackfillError{Code: "BACKFILL_PRECONDITION_FAILED", Message: err.Error()}
	}
	checksum, err := financeBackfillRequestChecksum(request)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal([]byte(payload.Checksum), []byte(checksum)) {
		return nil, &FinanceBackfillError{Code: "BACKFILL_PRECONDITION_FAILED", Message: "preview_token does not match backfill conditions"}
	}
	if payload.Blocked {
		return nil, &FinanceBackfillError{Code: "BACKFILL_PRECONDITION_FAILED", Message: "backfill preview contains blocking risks"}
	}
	total, err := s.repository.CountFinanceBackfillCandidates(ctx, request)
	if err != nil {
		return nil, err
	}
	tokenHash := sha256.Sum256([]byte(input.PreviewToken))
	return s.repository.CreateFinanceBackfillJob(ctx, request, operatorID, checksum, hex.EncodeToString(tokenHash[:]), time.Unix(payload.Expires, 0).UTC(), total)
}

func (s *FinanceBackfillService) Get(ctx context.Context, jobID int64) (*FinanceBackfillJob, error) {
	if jobID <= 0 {
		return nil, financeValidationError("invalid finance backfill job id")
	}
	return s.repository.GetFinanceBackfillJob(ctx, jobID)
}

func (s *FinanceBackfillService) Pause(ctx context.Context, jobID int64) (*FinanceBackfillJob, error) {
	if jobID <= 0 {
		return nil, financeValidationError("invalid finance backfill job id")
	}
	return s.repository.PauseFinanceBackfillJob(ctx, jobID)
}

func (s *FinanceBackfillService) Resume(ctx context.Context, jobID int64) (*FinanceBackfillJob, error) {
	if jobID <= 0 {
		return nil, financeValidationError("invalid finance backfill job id")
	}
	return s.repository.ResumeFinanceBackfillJob(ctx, jobID)
}

// RunNextBatch claims one durable job and processes at most one bounded batch.
// Progress is committed only after every item in the batch completed; retries
// therefore replay from the last successful cursor and are idempotent.
func (s *FinanceBackfillService) RunNextBatch(ctx context.Context) error {
	job, err := s.repository.ClaimFinanceBackfillJob(ctx, s.leaseOwner, s.now().UTC())
	if err != nil || job == nil {
		return err
	}
	workCtx, stopHeartbeat, err := s.startLeaseHeartbeat(ctx, job.ID)
	if err != nil {
		return err
	}
	request := FinanceBackfillRequest{
		StartDate: job.StartDate, EndDate: job.EndDate, Scope: job.Scope,
		PricingPolicy: job.PricingPolicy, DryRunSampleSize: 1000, Reason: job.Reason,
	}
	candidates, err := s.repository.ListFinanceBackfillCandidates(workCtx, request, job.Cursor, financeBackfillBatchSize)
	if err != nil {
		if leaseErr := stopHeartbeat(); leaseErr != nil {
			return leaseErr
		}
		_ = s.repository.FailFinanceBackfillJob(ctx, job.ID, s.leaseOwner, err.Error(), s.now().UTC())
		return err
	}
	if len(candidates) == 0 {
		return s.finalizeBackfill(ctx, workCtx, job, stopHeartbeat)
	}
	ids := make([]int64, 0, len(candidates))
	for i := range candidates {
		ids = append(ids, candidates[i].UsageLog.ID)
	}
	attemptsByUsage, err := s.ledger.LoadUsageAttempts(workCtx, ids)
	if err != nil {
		if leaseErr := stopHeartbeat(); leaseErr != nil {
			return leaseErr
		}
		_ = s.repository.FailFinanceBackfillJob(ctx, job.ID, s.leaseOwner, err.Error(), s.now().UTC())
		return err
	}
	launchAt, err := s.ledger.FinanceLaunchAt(workCtx)
	if err != nil {
		if leaseErr := stopHeartbeat(); leaseErr != nil {
			return leaseErr
		}
		_ = s.repository.FailFinanceBackfillJob(ctx, job.ID, s.leaseOwner, err.Error(), s.now().UTC())
		return err
	}
	cursor := job.Cursor
	for i := range candidates {
		paused, pauseErr := s.acknowledgePauseIfRequested(ctx, job.ID, stopHeartbeat)
		if pauseErr != nil || paused {
			return pauseErr
		}
		candidate := &candidates[i]
		projection, buildErr := s.scanner.buildProjection(workCtx, &candidate.UsageLog, attemptsByUsage[candidate.UsageLog.ID], launchAt)
		if buildErr == nil {
			paused, pauseErr = s.acknowledgePauseIfRequested(ctx, job.ID, stopHeartbeat)
			if pauseErr != nil || paused {
				return pauseErr
			}
		}
		if buildErr == nil {
			if candidate.HasProjection {
				_, buildErr = s.ledger.ReviseFinanceProjection(workCtx, projection, FinanceRevisionMetadata{Reason: job.Reason, JobID: &job.ID, OperatorID: &job.OperatorID})
			} else {
				var created bool
				created, buildErr = s.ledger.CreateFinanceProjection(workCtx, projection)
				if buildErr == nil && !created {
					_, buildErr = s.ledger.ReviseFinanceProjection(workCtx, projection, FinanceRevisionMetadata{Reason: job.Reason, JobID: &job.ID, OperatorID: &job.OperatorID})
				}
			}
		}
		if buildErr != nil {
			if leaseErr := stopHeartbeat(); leaseErr != nil {
				return leaseErr
			}
			_ = s.repository.FailFinanceBackfillJob(ctx, job.ID, s.leaseOwner, fmt.Sprintf("usage_log_id=%d: %v", candidate.UsageLog.ID, buildErr), s.now().UTC())
			return buildErr
		}
		cursor = FinanceBackfillCursor{CreatedAt: candidate.UsageLog.CreatedAt, ID: candidate.UsageLog.ID}
	}
	paused, pauseErr := s.acknowledgePauseIfRequested(ctx, job.ID, stopHeartbeat)
	if pauseErr != nil || paused {
		return pauseErr
	}
	processed := int64(len(candidates))
	newProcessed := job.ProcessedCount + processed
	progress := decimal.Zero
	if job.EstimatedTotal > 0 {
		progress = decimal.NewFromInt(newProcessed).Div(decimal.NewFromInt(job.EstimatedTotal))
		if progress.GreaterThan(decimal.NewFromInt(1)) {
			progress = decimal.NewFromInt(1)
		}
	}
	if err = s.repository.UpdateFinanceBackfillProgress(workCtx, job.ID, s.leaseOwner, cursor, processed, processed, progress, s.now().UTC()); err != nil {
		_ = stopHeartbeat()
		if paused, pauseErr = s.acknowledgePausedLease(ctx, job.ID); paused {
			return pauseErr
		}
		return err
	}
	paused, pauseErr = s.acknowledgePauseIfRequested(ctx, job.ID, stopHeartbeat)
	if pauseErr != nil || paused {
		return pauseErr
	}
	if len(candidates) < financeBackfillBatchSize {
		return s.finalizeBackfill(ctx, workCtx, job, stopHeartbeat)
	}
	if leaseErr := stopHeartbeat(); leaseErr != nil {
		if paused, pauseErr = s.acknowledgePausedLease(ctx, job.ID); paused {
			return pauseErr
		}
		return leaseErr
	}
	if err = s.repository.ReleaseFinanceBackfillJob(ctx, job.ID, s.leaseOwner, s.now().UTC()); err != nil {
		if paused, pauseErr = s.acknowledgePausedLease(ctx, job.ID); paused {
			return pauseErr
		}
	}
	return err
}

func (s *FinanceBackfillService) acknowledgePauseIfRequested(ctx context.Context, jobID int64, stopHeartbeat func() error) (bool, error) {
	current, err := s.repository.GetFinanceBackfillJob(ctx, jobID)
	if err != nil {
		return false, err
	}
	if current.Status != "paused" {
		return false, nil
	}
	_ = stopHeartbeat()
	return s.acknowledgePausedLease(ctx, jobID)
}

func (s *FinanceBackfillService) acknowledgePausedLease(ctx context.Context, jobID int64) (bool, error) {
	current, err := s.repository.GetFinanceBackfillJob(ctx, jobID)
	if err != nil {
		return false, err
	}
	if current.Status != "paused" {
		return false, nil
	}
	if err = s.repository.AcknowledgeFinanceBackfillPause(ctx, jobID, s.leaseOwner, s.now().UTC()); err != nil {
		return true, err
	}
	return true, nil
}

func (s *FinanceBackfillService) finalizeBackfill(ctx, workCtx context.Context, job *FinanceBackfillJob, stopHeartbeat func() error) error {
	paused, pauseErr := s.acknowledgePauseIfRequested(ctx, job.ID, stopHeartbeat)
	if pauseErr != nil || paused {
		return pauseErr
	}
	var recognitionErr error
	if s.revenue != nil {
		start, startErr := time.Parse("2006-01-02", job.StartDate)
		end, endErr := time.Parse("2006-01-02", job.EndDate)
		if startErr != nil || endErr != nil {
			recognitionErr = financeValidationError("finance backfill date range is invalid")
		} else {
			maxDays := int(end.Sub(start).Hours()/24) + 1
			_, recognitionErr = s.revenue.RecognizeRange(workCtx, start, end, s.timezone, maxDays)
		}
	}
	paused, pauseErr = s.acknowledgePauseIfRequested(ctx, job.ID, stopHeartbeat)
	if pauseErr != nil || paused {
		return pauseErr
	}
	if leaseErr := stopHeartbeat(); leaseErr != nil {
		if paused, pauseErr = s.acknowledgePausedLease(ctx, job.ID); paused {
			return pauseErr
		}
		return leaseErr
	}
	if recognitionErr != nil {
		_ = s.repository.FailFinanceBackfillJob(ctx, job.ID, s.leaseOwner, "subscription revenue recalculation failed", s.now().UTC())
		return recognitionErr
	}
	if err := s.repository.CompleteFinanceBackfillJob(ctx, job.ID, s.leaseOwner, s.now().UTC()); err != nil {
		if paused, pauseErr = s.acknowledgePausedLease(ctx, job.ID); paused {
			return pauseErr
		}
		return err
	}
	return nil
}

func (s *FinanceBackfillService) startLeaseHeartbeat(ctx context.Context, jobID int64) (context.Context, func() error, error) {
	if err := s.repository.RenewFinanceBackfillLease(ctx, jobID, s.leaseOwner, s.now().UTC()); err != nil {
		return nil, nil, err
	}
	workCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(financeBackfillLeaseDuration / 4)
		defer ticker.Stop()
		for {
			select {
			case <-workCtx.Done():
				done <- nil
				return
			case <-ticker.C:
				if err := s.repository.RenewFinanceBackfillLease(workCtx, jobID, s.leaseOwner, s.now().UTC()); err != nil {
					cancel()
					done <- err
					return
				}
			}
		}
	}()
	return workCtx, func() error {
		cancel()
		return <-done
	}, nil
}

func normalizeFinanceBackfillRequest(input FinanceBackfillRequest) (FinanceBackfillRequest, error) {
	start, err := time.Parse("2006-01-02", strings.TrimSpace(input.StartDate))
	if err != nil {
		return FinanceBackfillRequest{}, financeValidationError("invalid start_date")
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(input.EndDate))
	if err != nil {
		return FinanceBackfillRequest{}, financeValidationError("invalid end_date")
	}
	if end.Before(start) {
		return FinanceBackfillRequest{}, financeValidationError("end_date must not be before start_date")
	}
	policy := strings.TrimSpace(input.PricingPolicy)
	if policy == "" {
		policy = FinanceBackfillPricingHistorical
	}
	if policy != FinanceBackfillPricingHistorical {
		return FinanceBackfillRequest{}, financeValidationError("pricing_policy must be historical_only")
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return FinanceBackfillRequest{}, financeValidationError("finance backfill reason is required")
	}
	sampleSize := input.DryRunSampleSize
	if sampleSize == 0 {
		sampleSize = 1000
	}
	if sampleSize < 1 || sampleSize > 5000 {
		return FinanceBackfillRequest{}, financeValidationError("dry_run_sample_size must be between 1 and 5000")
	}
	statuses := uniqueFinanceBackfillStrings(input.Scope.CostStatus)
	allowedStatuses := map[string]bool{
		string(FinanceCostStatusExact): true, string(FinanceCostStatusEstimated): true,
		string(FinanceCostStatusMissingProfile): true, string(FinanceCostStatusMissingPrice): true,
		string(FinanceCostStatusMissingMultiplier): true, string(FinanceCostStatusMissingUsage): true,
		string(FinanceCostStatusUnsupportedUsage): true, string(FinanceCostStatusNonBillable): true,
		string(FinanceCostStatusExcluded): true,
	}
	for _, status := range statuses {
		if !allowedStatuses[status] {
			return FinanceBackfillRequest{}, financeValidationErrorf("invalid finance cost_status: %s", status)
		}
	}
	return FinanceBackfillRequest{
		StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02"),
		Scope:         FinanceBackfillScope{CostStatus: statuses, AccountIDs: uniqueFinanceBackfillPositiveIDs(input.Scope.AccountIDs), WalletIDs: uniqueFinanceBackfillPositiveIDs(input.Scope.WalletIDs)},
		PricingPolicy: policy, DryRunSampleSize: sampleSize, Reason: reason,
	}, nil
}

func financeBackfillRequestChecksum(request FinanceBackfillRequest) (string, error) {
	request.PreviewToken = ""
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (s *FinanceBackfillService) signPreviewToken(payload financeBackfillPreviewToken) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.tokenKey)
	_, _ = mac.Write(encoded)
	return base64.RawURLEncoding.EncodeToString(encoded) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *FinanceBackfillService) verifyPreviewToken(token string) (financeBackfillPreviewToken, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return financeBackfillPreviewToken{}, errors.New("invalid preview_token")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return financeBackfillPreviewToken{}, errors.New("invalid preview_token")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return financeBackfillPreviewToken{}, errors.New("invalid preview_token")
	}
	mac := hmac.New(sha256.New, s.tokenKey)
	_, _ = mac.Write(payloadBytes)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return financeBackfillPreviewToken{}, errors.New("invalid preview_token signature")
	}
	var payload financeBackfillPreviewToken
	if err = json.Unmarshal(payloadBytes, &payload); err != nil {
		return financeBackfillPreviewToken{}, errors.New("invalid preview_token payload")
	}
	if !s.now().UTC().Before(time.Unix(payload.Expires, 0).UTC()) {
		return financeBackfillPreviewToken{}, errors.New("preview_token has expired")
	}
	return payload, nil
}

func estimateFinanceBackfillCount(sampleCount, sampleSize, total int64) int64 {
	if total == 0 || sampleSize == 0 {
		return 0
	}
	if sampleSize >= total {
		return sampleCount
	}
	return decimal.NewFromInt(sampleCount).Mul(decimal.NewFromInt(total)).Div(decimal.NewFromInt(sampleSize)).Round(0).IntPart()
}

func uniqueFinanceBackfillStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return sortedFinanceBackfillStrings(set)
}

func uniqueFinanceBackfillPositiveIDs(values []int64) []int64 {
	set := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value > 0 {
			set[value] = struct{}{}
		}
	}
	return sortedFinanceBackfillInt64s(set)
}

func sortedFinanceBackfillStrings(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortedFinanceBackfillInt64s(values map[int64]struct{}) []int64 {
	result := make([]int64, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
