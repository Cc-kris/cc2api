package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/shopspring/decimal"
)

const (
	FinanceExportJobType       = "finance_export"
	financeExportPageSize      = 200
	financeExportLeaseDuration = 2 * time.Minute
	financeExportFileTTL       = 24 * time.Hour
	financeExportDownloadTTL   = 15 * time.Minute
)

type FinanceExportFilters struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Dimension string `json:"dimension"`
	DataScope string `json:"data_scope"`
	SortBy    string `json:"sort_by,omitempty"`
	SortOrder string `json:"sort_order,omitempty"`
}

type FinanceExportRequest struct {
	Report   string               `json:"report"`
	Format   string               `json:"format"`
	Filters  FinanceExportFilters `json:"filters"`
	Timezone string               `json:"timezone"`
}

type FinanceExportJob struct {
	ID             int64                `json:"id"`
	Type           string               `json:"type"`
	Status         string               `json:"status"`
	Progress       string               `json:"progress"`
	ProcessedCount int64                `json:"processed_count"`
	SuccessCount   int64                `json:"success_count"`
	FailedCount    int64                `json:"failed_count"`
	Report         string               `json:"report"`
	Format         string               `json:"format"`
	Request        FinanceExportRequest `json:"-"`
	OperatorID     int64                `json:"-"`
	StorageKey     string               `json:"-"`
	FileSize       *int64               `json:"file_size"`
	RowCount       *int64               `json:"row_count"`
	ExpiresAt      *time.Time           `json:"expires_at"`
	CreatedAt      time.Time            `json:"created_at"`
	StartedAt      *time.Time           `json:"started_at"`
	FinishedAt     *time.Time           `json:"finished_at"`
	ErrorSummary   *string              `json:"error_summary"`
	DownloadURL    string               `json:"download_url,omitempty"`
}

type FinanceExportDownload struct {
	Path     string
	Filename string
}

type FinanceExportError struct {
	Code    string
	Message string
}

func (e *FinanceExportError) Error() string { return e.Message }

func IsFinanceExportError(err error, code string) bool {
	var target *FinanceExportError
	return errors.As(err, &target) && target.Code == code
}

type FinanceExportRepository interface {
	CreateFinanceExportJob(ctx context.Context, request FinanceExportRequest, operatorID int64, idempotencyKey, requestChecksum string) (*FinanceExportJob, error)
	GetFinanceExportJob(ctx context.Context, jobID, operatorID int64) (*FinanceExportJob, error)
	ClaimFinanceExportJob(ctx context.Context, leaseOwner string, now time.Time) (*FinanceExportJob, error)
	RenewFinanceExportLease(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error
	UpdateFinanceExportProgress(ctx context.Context, jobID int64, leaseOwner string, processed int64, progress decimal.Decimal, now time.Time) error
	CompleteFinanceExportJob(ctx context.Context, jobID int64, leaseOwner, storageKey string, fileSize, rowCount int64, expiresAt, now time.Time) error
	FailFinanceExportJob(ctx context.Context, jobID int64, leaseOwner, message string, now time.Time) error
	ReleaseFinanceExportJob(ctx context.Context, jobID int64, leaseOwner string, now time.Time) error
	SetFinanceExportDownloadToken(ctx context.Context, jobID, operatorID int64, tokenHash string, expiresAt time.Time) error
	ConsumeFinanceExportDownloadToken(ctx context.Context, jobID, operatorID int64, tokenHash string, now time.Time) (*FinanceExportJob, error)
}

type FinanceExportService struct {
	repo       FinanceExportRepository
	reports    *FinanceReportService
	exportDir  string
	leaseOwner string
	now        func() time.Time
}

func NewFinanceExportService(repo FinanceExportRepository, reports *FinanceReportService, cfg *config.Config) *FinanceExportService {
	dataDir := "./data"
	if cfg != nil && strings.TrimSpace(cfg.Pricing.DataDir) != "" {
		dataDir = cfg.Pricing.DataDir
	}
	ownerBytes := make([]byte, 12)
	_, _ = rand.Read(ownerBytes)
	return &FinanceExportService{
		repo: repo, reports: reports, exportDir: filepath.Join(dataDir, "finance_exports"),
		leaseOwner: "finance-export-" + hex.EncodeToString(ownerBytes), now: time.Now,
	}
}

func (s *FinanceExportService) Create(ctx context.Context, request FinanceExportRequest, operatorID int64, idempotencyKey string) (*FinanceExportJob, error) {
	if operatorID <= 0 {
		return nil, financeValidationError("operator is required")
	}
	normalized, _, err := normalizeFinanceExportRequest(request)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	checksum := sha256.Sum256(payload)
	return s.repo.CreateFinanceExportJob(ctx, normalized, operatorID, strings.TrimSpace(idempotencyKey), hex.EncodeToString(checksum[:]))
}

func (s *FinanceExportService) Get(ctx context.Context, jobID, operatorID int64) (*FinanceExportJob, string, error) {
	job, err := s.repo.GetFinanceExportJob(ctx, jobID, operatorID)
	if err != nil {
		return nil, "", err
	}
	if job.Status != "completed" || job.StorageKey == "" || job.ExpiresAt == nil || !job.ExpiresAt.After(s.now()) {
		return job, "", nil
	}
	tokenBytes := make([]byte, 32)
	if _, err = rand.Read(tokenBytes); err != nil {
		return nil, "", err
	}
	token := hex.EncodeToString(tokenBytes)
	downloadExpiresAt := s.now().Add(financeExportDownloadTTL)
	if downloadExpiresAt.After(*job.ExpiresAt) {
		downloadExpiresAt = *job.ExpiresAt
	}
	if err = s.repo.SetFinanceExportDownloadToken(ctx, jobID, operatorID, hashFinanceExportToken(token), downloadExpiresAt); err != nil {
		return nil, "", err
	}
	job.ExpiresAt = &downloadExpiresAt
	return job, token, nil
}

func (s *FinanceExportService) Download(ctx context.Context, jobID, operatorID int64, token string) (*FinanceExportDownload, error) {
	if strings.TrimSpace(token) == "" {
		return nil, financeValidationError("download token is required")
	}
	job, err := s.repo.ConsumeFinanceExportDownloadToken(ctx, jobID, operatorID, hashFinanceExportToken(token), s.now())
	if err != nil {
		return nil, err
	}
	cleanDir, err := filepath.Abs(s.exportDir)
	if err != nil {
		return nil, err
	}
	cleanPath, err := filepath.Abs(filepath.Clean(job.StorageKey))
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(cleanDir, cleanPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return nil, errors.New("finance export storage path is invalid")
	}
	if _, err = os.Stat(cleanPath); err != nil {
		return nil, fmt.Errorf("finance export file unavailable: %w", err)
	}
	return &FinanceExportDownload{Path: cleanPath, Filename: fmt.Sprintf("finance-%s-%d.csv", job.Report, job.ID)}, nil
}

func (s *FinanceExportService) RunNext(ctx context.Context) error {
	job, err := s.repo.ClaimFinanceExportJob(ctx, s.leaseOwner, s.now())
	if err != nil || job == nil {
		return err
	}
	if err = s.writeBreakdown(ctx, job); err != nil {
		if ctx.Err() != nil {
			_ = s.repo.ReleaseFinanceExportJob(context.Background(), job.ID, s.leaseOwner, s.now())
			return ctx.Err()
		}
		_ = s.repo.FailFinanceExportJob(context.Background(), job.ID, s.leaseOwner, err.Error(), s.now())
		return err
	}
	return nil
}

func (s *FinanceExportService) writeBreakdown(ctx context.Context, job *FinanceExportJob) (err error) {
	workCtx, stopHeartbeat, err := s.startLeaseHeartbeat(ctx, job.ID)
	if err != nil {
		return err
	}
	heartbeatStopped := false
	defer func() {
		if heartbeatStopped {
			return
		}
		if heartbeatErr := stopHeartbeat(); err == nil && heartbeatErr != nil {
			err = heartbeatErr
		}
	}()
	if err = os.MkdirAll(s.exportDir, 0o750); err != nil {
		return err
	}
	owner := strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(s.leaseOwner)
	tempFile, err := os.CreateTemp(s.exportDir, fmt.Sprintf("finance-%d-%s-*.csv", job.ID, owner))
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err = tempFile.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	writer := csv.NewWriter(tempFile)
	generatedAt := s.now().UTC().Format(time.RFC3339)
	filtersJSON, _ := json.Marshal(job.Request.Filters)
	for _, metadata := range []string{"# generated_at=" + generatedAt, "# timezone=" + job.Request.Timezone, "# filters=" + string(filtersJSON)} {
		if err = writer.Write([]string{metadata}); err != nil {
			return err
		}
	}
	headers := []string{"dimension_key", "dimension_name", "revenue", "upstream_cost", "input_cost", "output_cost", "cache_cost", "fast_cost", "image_cost", "video_cost", "other_cost", "profit", "margin_rate", "loss_amount", "request_count", "exact_count", "estimated_count", "missing_count"}
	if err = writer.Write(headers); err != nil {
		return err
	}

	filter, _, err := normalizeFinanceExportRequest(job.Request)
	if err != nil {
		return err
	}
	reportFilter, err := financeExportReportFilter(filter)
	if err != nil {
		return err
	}
	var processed int64
	for page := 1; ; page++ {
		items, total, listErr := s.reports.Breakdown(workCtx, reportFilter, FinanceBreakdownRequest{
			Dimension: filter.Filters.Dimension, SortBy: filter.Filters.SortBy, SortOrder: filter.Filters.SortOrder,
			Page: page, PageSize: financeExportPageSize,
		})
		if listErr != nil {
			return listErr
		}
		for _, item := range items {
			margin := ""
			if item.MarginRate != nil {
				margin = *item.MarginRate
			}
			record := []string{
				safeFinanceCSVText(item.DimensionKey), safeFinanceCSVText(item.DimensionName), item.Revenue, item.UpstreamCost,
				item.InputCost, item.OutputCost, item.CacheCost, item.FastCost, item.ImageCost, item.VideoCost, item.OtherCost,
				item.Profit, margin, item.LossAmount, strconv.FormatInt(item.RequestCount, 10), strconv.FormatInt(item.ExactCount, 10),
				strconv.FormatInt(item.EstimatedCount, 10), strconv.FormatInt(item.MissingCount, 10),
			}
			if err = writer.Write(record); err != nil {
				return err
			}
		}
		writer.Flush()
		if err = writer.Error(); err != nil {
			return err
		}
		processed += int64(len(items))
		progress := decimal.NewFromInt(1)
		if total > 0 {
			progress = decimal.NewFromInt(processed).Div(decimal.NewFromInt(total))
			if progress.GreaterThan(decimal.NewFromInt(1)) {
				progress = decimal.NewFromInt(1)
			}
		}
		if err = s.repo.UpdateFinanceExportProgress(workCtx, job.ID, s.leaseOwner, int64(len(items)), progress, s.now()); err != nil {
			return err
		}
		if processed >= total || len(items) == 0 {
			break
		}
	}
	if err = tempFile.Sync(); err != nil {
		return err
	}
	if err = tempFile.Close(); err != nil {
		return err
	}
	stat, err := os.Stat(tempPath)
	if err != nil {
		return err
	}
	expiresAt := s.now().Add(financeExportFileTTL)
	heartbeatErr := stopHeartbeat()
	heartbeatStopped = true
	if heartbeatErr != nil {
		return heartbeatErr
	}
	if err = s.repo.CompleteFinanceExportJob(ctx, job.ID, s.leaseOwner, tempPath, stat.Size(), processed, expiresAt, s.now()); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func (s *FinanceExportService) startLeaseHeartbeat(ctx context.Context, jobID int64) (context.Context, func() error, error) {
	if err := s.repo.RenewFinanceExportLease(ctx, jobID, s.leaseOwner, s.now().UTC()); err != nil {
		return nil, nil, err
	}
	workCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(financeExportLeaseDuration / 4)
		defer ticker.Stop()
		for {
			select {
			case <-workCtx.Done():
				done <- nil
				return
			case <-ticker.C:
				if err := s.repo.RenewFinanceExportLease(workCtx, jobID, s.leaseOwner, s.now().UTC()); err != nil {
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

func normalizeFinanceExportRequest(request FinanceExportRequest) (FinanceExportRequest, FinanceReportFilter, error) {
	request.Report = strings.ToLower(strings.TrimSpace(request.Report))
	request.Format = strings.ToLower(strings.TrimSpace(request.Format))
	request.Timezone = strings.TrimSpace(request.Timezone)
	request.Filters.Dimension = strings.ToLower(strings.TrimSpace(request.Filters.Dimension))
	request.Filters.DataScope = strings.ToLower(strings.TrimSpace(request.Filters.DataScope))
	request.Filters.SortBy = strings.ToLower(strings.TrimSpace(request.Filters.SortBy))
	request.Filters.SortOrder = strings.ToLower(strings.TrimSpace(request.Filters.SortOrder))
	if request.Report != "breakdown" {
		return request, FinanceReportFilter{}, financeValidationError("report must be breakdown")
	}
	if request.Format == "" {
		request.Format = "csv"
	}
	if request.Format != "csv" {
		return request, FinanceReportFilter{}, financeValidationError("format must be csv")
	}
	if request.Filters.Dimension == "" {
		request.Filters.Dimension = "requested_model"
	}
	if request.Filters.DataScope == "" {
		request.Filters.DataScope = "all"
	}
	if request.Filters.SortBy == "" {
		request.Filters.SortBy = "profit"
	}
	if request.Filters.SortOrder == "" {
		request.Filters.SortOrder = "asc"
	}
	filter, err := financeExportReportFilter(request)
	if err != nil {
		return request, FinanceReportFilter{}, err
	}
	if !financeAllowed(request.Filters.Dimension, "user", "group", "channel", "upstream", "wallet", "account", "requested_model", "upstream_model", "billing_type", "business_type") {
		return request, FinanceReportFilter{}, financeValidationError("dimension is invalid")
	}
	if !financeAllowed(request.Filters.SortBy, "revenue", "upstream_cost", "profit", "loss_amount", "margin_rate", "request_count") {
		return request, FinanceReportFilter{}, financeValidationError("sort_by is invalid")
	}
	if !financeAllowed(request.Filters.SortOrder, "asc", "desc") {
		return request, FinanceReportFilter{}, financeValidationError("sort_order must be asc or desc")
	}
	return request, filter, nil
}

func financeExportReportFilter(request FinanceExportRequest) (FinanceReportFilter, error) {
	values := make(url.Values)
	values["start_date"] = []string{request.Filters.StartDate}
	values["end_date"] = []string{request.Filters.EndDate}
	values["timezone"] = []string{request.Timezone}
	values["data_scope"] = []string{request.Filters.DataScope}
	return ParseFinanceReportFilter(values)
}

func safeFinanceCSVText(value string) string {
	trimmed := strings.TrimLeft(value, " \n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	default:
		return value
	}
}

func hashFinanceExportToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
