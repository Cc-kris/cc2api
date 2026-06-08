package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/infraerrors"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const (
	SemanticCacheDecisionObserve = "observe"
	SemanticCacheDecisionHit     = "hit"
	SemanticCacheDecisionMiss    = "miss"
	SemanticCacheDecisionBlocked = "blocked"
	SemanticCacheDecisionRollback = "rollback"

	SemanticCacheReviewPending     = "pending"
	SemanticCacheReviewReusable    = "reusable"
	SemanticCacheReviewNotReusable = "not_reusable"
	SemanticCacheReviewDisputed    = "disputed"

	SemanticCacheFeedbackNone       = "none"
	SemanticCacheFeedbackWrongHit   = "wrong_hit"
	SemanticCacheFeedbackComplaint  = "complaint"
	SemanticCacheFeedbackManualMark = "manual_mark"
)

var (
	ErrSemanticCacheAuditUnavailable    = errors.New("semantic cache audit store unavailable")
	ErrInvalidSemanticCacheAuditList    = errors.New("invalid semantic cache audit list request")
	ErrInvalidSemanticCacheAuditReview  = errors.New("invalid semantic cache audit review request")
	ErrInvalidSemanticCacheAuditFeedback = errors.New("invalid semantic cache audit feedback request")
)

type SemanticCacheAuditListFilter struct {
	Page         int
	PageSize     int
	StartTime    *time.Time
	EndTime      *time.Time
	Platform     string
	Model        string
	APIKeyID     *int64
	ReviewStatus string
	Decision     string
	MinSimilarity *float64
	MaxSimilarity *float64
	ViewerRole   string
}

type SemanticCacheAuditListRecord struct {
	ID              int64      `json:"id"`
	RequestID       string     `json:"request_id"`
	SemanticEntryID *int64     `json:"semantic_entry_id,omitempty"`
	OccurredAt      time.Time  `json:"occurred_at"`
	Platform        string     `json:"platform"`
	Model           string     `json:"model"`
	APIKeyID        *int64     `json:"api_key_id,omitempty"`
	APIKey          string     `json:"api_key,omitempty"`
	Similarity      float64    `json:"similarity"`
	Decision        string     `json:"decision"`
	BlockReason     string     `json:"block_reason,omitempty"`
	ReviewStatus    string     `json:"review_status"`
	FeedbackType    string     `json:"feedback_type,omitempty"`
	FeedbackNote    string     `json:"feedback_note,omitempty"`
	OperatorUserID  *int64     `json:"operator_user_id,omitempty"`
	AutoCloseReason string     `json:"auto_close_reason,omitempty"`
	SourceSummary   string     `json:"source_summary,omitempty"`
	TargetSummary   string     `json:"target_summary,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type SemanticCacheAuditListPage struct {
	Items    []SemanticCacheAuditListRecord `json:"items"`
	Total    int64                          `json:"total"`
	Page     int                            `json:"page"`
	PageSize int                            `json:"page_size"`
}

type SemanticCacheAuditReviewRequest struct {
	ReviewStatus string `json:"review_status"`
	Note         string `json:"note"`
}

type SemanticCacheAuditFeedbackRequest struct {
	FeedbackType string `json:"feedback_type"`
	Note         string `json:"note"`
}

type SemanticCacheAuditReviewStore interface {
	ListSemanticCacheAudits(ctx context.Context, filter SemanticCacheAuditListFilter) (*SemanticCacheAuditListPage, error)
	UpdateSemanticCacheAuditReview(ctx context.Context, auditID int64, reviewStatus string, operatorUserID int64) (*SemanticCacheAuditListRecord, error)
	UpdateSemanticCacheAuditFeedback(ctx context.Context, auditID int64, feedbackType, feedbackNote string, operatorUserID int64) (*SemanticCacheAuditListRecord, error)
	SetSemanticCacheAuditAutoCloseReason(ctx context.Context, auditID int64, reason string) error
	GetSemanticCacheAudit24hQualityStats(ctx context.Context, since time.Time) (*SemanticCacheAuditQualityStats, error)
}

type SemanticCacheAuditQualityStats struct {
	HitCount           int64
	ComplaintCount     int64
	ErrorFeedbackCount int64
}

func (s *OpenAIGatewayService) ListSemanticCacheAudits(ctx context.Context, filter SemanticCacheAuditListFilter) (*SemanticCacheAuditListPage, error) {
	if !canViewSemanticCacheAudits(filter.ViewerRole) {
		return &SemanticCacheAuditListPage{Items: []SemanticCacheAuditListRecord{}}, nil
	}
	normalized, err := normalizeSemanticCacheAuditListFilter(filter)
	if err != nil {
		return nil, err
	}
	if s == nil || s.cache == nil {
		return nil, ErrSemanticCacheAuditUnavailable
	}
	store, ok := s.cache.(SemanticCacheAuditReviewStore)
	if !ok {
		return nil, ErrSemanticCacheAuditUnavailable
	}
	page, err := store.ListSemanticCacheAudits(ctx, normalized)
	if err != nil {
		return nil, err
	}
	return sanitizeSemanticCacheAuditListPage(page), nil
}

func (s *OpenAIGatewayService) ReviewSemanticCacheAudit(ctx context.Context, auditID int64, req SemanticCacheAuditReviewRequest, operatorUserID int64, viewerRole string) (*SemanticCacheAuditListRecord, error) {
	if !canManageSemanticCacheAudits(viewerRole) {
		return nil, ErrInvalidSemanticCacheAuditReview
	}
	if auditID <= 0 {
		return nil, infraerrors.BadRequest("SEMANTIC_CACHE_AUDIT_INVALID_ID", "invalid semantic audit id")
	}
	req.ReviewStatus = strings.TrimSpace(req.ReviewStatus)
	req.Note = strings.TrimSpace(req.Note)
	if !isValidSemanticCacheReviewStatus(req.ReviewStatus) || req.ReviewStatus == SemanticCacheReviewPending {
		return nil, fmt.Errorf("%w: invalid review_status", ErrInvalidSemanticCacheAuditReview)
	}
	if len([]rune(req.Note)) > 500 {
		return nil, fmt.Errorf("%w: note is too long", ErrInvalidSemanticCacheAuditReview)
	}
	if operatorUserID <= 0 {
		return nil, infraerrors.BadRequest("SEMANTIC_CACHE_AUDIT_INVALID_OPERATOR", "invalid operator user")
	}
	if s == nil || s.cache == nil {
		return nil, ErrSemanticCacheAuditUnavailable
	}
	store, ok := s.cache.(SemanticCacheAuditReviewStore)
	if !ok {
		return nil, ErrSemanticCacheAuditUnavailable
	}
	record, err := store.UpdateSemanticCacheAuditReview(ctx, auditID, req.ReviewStatus, operatorUserID)
	if err != nil {
		return nil, err
	}
	return sanitizeSemanticCacheAuditRecord(record), nil
}

func (s *OpenAIGatewayService) FeedbackSemanticCacheAudit(ctx context.Context, auditID int64, req SemanticCacheAuditFeedbackRequest, operatorUserID int64, viewerRole string) (*SemanticCacheAuditListRecord, error) {
	if !canManageSemanticCacheAudits(viewerRole) {
		return nil, ErrInvalidSemanticCacheAuditFeedback
	}
	if auditID <= 0 {
		return nil, infraerrors.BadRequest("SEMANTIC_CACHE_AUDIT_INVALID_ID", "invalid semantic audit id")
	}
	req.FeedbackType = strings.TrimSpace(req.FeedbackType)
	req.Note = strings.TrimSpace(req.Note)
	if !isValidSemanticCacheFeedbackType(req.FeedbackType) || req.FeedbackType == SemanticCacheFeedbackNone {
		return nil, fmt.Errorf("%w: invalid feedback_type", ErrInvalidSemanticCacheAuditFeedback)
	}
	if len([]rune(req.Note)) == 0 {
		return nil, fmt.Errorf("%w: note is required", ErrInvalidSemanticCacheAuditFeedback)
	}
	if len([]rune(req.Note)) > 500 {
		return nil, fmt.Errorf("%w: note is too long", ErrInvalidSemanticCacheAuditFeedback)
	}
	if operatorUserID <= 0 {
		return nil, infraerrors.BadRequest("SEMANTIC_CACHE_AUDIT_INVALID_OPERATOR", "invalid operator user")
	}
	if s == nil || s.cache == nil {
		return nil, ErrSemanticCacheAuditUnavailable
	}
	store, ok := s.cache.(SemanticCacheAuditReviewStore)
	if !ok {
		return nil, ErrSemanticCacheAuditUnavailable
	}
	record, err := store.UpdateSemanticCacheAuditFeedback(ctx, auditID, req.FeedbackType, req.Note, operatorUserID)
	if err != nil {
		return nil, err
	}
	if autoCloseReason, autoCloseErr := s.maybeAutoCloseSemanticCache(ctx, store); autoCloseErr == nil && strings.TrimSpace(autoCloseReason) != "" {
		record.AutoCloseReason = autoCloseReason
		_ = store.SetSemanticCacheAuditAutoCloseReason(ctx, auditID, autoCloseReason)
	} else if autoCloseErr != nil {
		return nil, autoCloseErr
	}
	return sanitizeSemanticCacheAuditRecord(record), nil
}

func (s *OpenAIGatewayService) maybeAutoCloseSemanticCache(ctx context.Context, store SemanticCacheAuditReviewStore) (string, error) {
	if s == nil || s.settingService == nil || store == nil {
		return "", nil
	}
	cfg, err := s.settingService.loadSemanticCacheConfigForUpdate(ctx)
	if err != nil {
		return "", err
	}
	cfg = normalizeSemanticCacheConfig(cfg)
	if cfg.AutoClosed || !cfg.Enabled {
		return "", nil
	}
	since := time.Now().UTC().Add(-24 * time.Hour)
	stats, err := store.GetSemanticCacheAudit24hQualityStats(ctx, since)
	if err != nil {
		return "", err
	}
	if stats == nil || stats.HitCount <= 0 {
		return "", nil
	}
	complaintRate := float64(stats.ComplaintCount) * 100 / float64(stats.HitCount)
	errorRate := float64(stats.ErrorFeedbackCount) * 100 / float64(stats.HitCount)
	threshold := cfg.QualityRollbackThresholdPercent
	if complaintRate < threshold && errorRate < threshold {
		return "", nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	reason := fmt.Sprintf(
		"24h semantic quality rollback: hits=%d complaints=%d complaint_rate=%.2f%% error_feedbacks=%d error_feedback_rate=%.2f%% threshold=%.2f%%",
		stats.HitCount,
		stats.ComplaintCount,
		complaintRate,
		stats.ErrorFeedbackCount,
		errorRate,
		threshold,
	)
	cfg.Enabled = false
	cfg.Stage = SemanticCacheDecisionRollback
	cfg.AutoClosed = true
	cfg.AutoCloseReason = cloneStringPtr(&reason)
	cfg.AutoClosedAt = cloneStringPtr(&now)
	payload, err := jsonMarshalSemanticCacheConfig(cfg)
	if err != nil {
		return "", err
	}
	if err := s.settingService.settingRepo.Set(ctx, SettingKeySemanticCacheConfig, payload); err != nil {
		return "", err
	}
	return reason, nil
}

func normalizeSemanticCacheAuditListFilter(filter SemanticCacheAuditListFilter) (SemanticCacheAuditListFilter, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	filter.Platform = strings.TrimSpace(filter.Platform)
	filter.Model = strings.TrimSpace(filter.Model)
	filter.ReviewStatus = strings.TrimSpace(filter.ReviewStatus)
	filter.Decision = strings.TrimSpace(filter.Decision)
	if filter.StartTime != nil && filter.EndTime != nil {
		if filter.StartTime.After(*filter.EndTime) {
			return filter, fmt.Errorf("%w: start_time must be before end_time", ErrInvalidSemanticCacheAuditList)
		}
		if filter.EndTime.Sub(*filter.StartTime) > 31*24*time.Hour {
			return filter, fmt.Errorf("%w: time range cannot exceed 31 days", ErrInvalidSemanticCacheAuditList)
		}
	}
	if filter.APIKeyID != nil && *filter.APIKeyID <= 0 {
		return filter, fmt.Errorf("%w: invalid api_key_id", ErrInvalidSemanticCacheAuditList)
	}
	if filter.ReviewStatus != "" && !isValidSemanticCacheReviewStatus(filter.ReviewStatus) {
		return filter, fmt.Errorf("%w: invalid review_status", ErrInvalidSemanticCacheAuditList)
	}
	if filter.Decision != "" && !isValidSemanticCacheDecision(filter.Decision) {
		return filter, fmt.Errorf("%w: invalid decision", ErrInvalidSemanticCacheAuditList)
	}
	if filter.MinSimilarity != nil {
		if *filter.MinSimilarity < 0 || *filter.MinSimilarity > 1 {
			return filter, fmt.Errorf("%w: invalid min_similarity", ErrInvalidSemanticCacheAuditList)
		}
	}
	if filter.MaxSimilarity != nil {
		if *filter.MaxSimilarity < 0 || *filter.MaxSimilarity > 1 {
			return filter, fmt.Errorf("%w: invalid max_similarity", ErrInvalidSemanticCacheAuditList)
		}
	}
	if filter.MinSimilarity != nil && filter.MaxSimilarity != nil && *filter.MinSimilarity > *filter.MaxSimilarity {
		return filter, fmt.Errorf("%w: min_similarity must be <= max_similarity", ErrInvalidSemanticCacheAuditList)
	}
	return filter, nil
}

func sanitizeSemanticCacheAuditListPage(page *SemanticCacheAuditListPage) *SemanticCacheAuditListPage {
	if page == nil {
		return &SemanticCacheAuditListPage{Items: []SemanticCacheAuditListRecord{}}
	}
	out := &SemanticCacheAuditListPage{
		Items:    make([]SemanticCacheAuditListRecord, 0, len(page.Items)),
		Total:    page.Total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}
	for _, item := range page.Items {
		record := item
		out.Items = append(out.Items, *sanitizeSemanticCacheAuditRecord(&record))
	}
	return out
}

func sanitizeSemanticCacheAuditRecord(record *SemanticCacheAuditListRecord) *SemanticCacheAuditListRecord {
	if record == nil {
		return nil
	}
	out := *record
	out.APIKey = logredact.RedactAPIKey(strings.TrimSpace(out.APIKey))
	out.SourceSummary = logredact.RedactRequestBody(strings.TrimSpace(out.SourceSummary), 300)
	out.TargetSummary = logredact.RedactResponseBody(strings.TrimSpace(out.TargetSummary), 300)
	out.FeedbackNote = logredact.RedactAIContext(strings.TrimSpace(out.FeedbackNote), 500)
	out.AutoCloseReason = logredact.RedactAIContext(strings.TrimSpace(out.AutoCloseReason), 500)
	return &out
}

func isValidSemanticCacheDecision(value string) bool {
	switch strings.TrimSpace(value) {
	case SemanticCacheDecisionObserve, SemanticCacheDecisionHit, SemanticCacheDecisionMiss, SemanticCacheDecisionBlocked, SemanticCacheDecisionRollback:
		return true
	default:
		return false
	}
}

func isValidSemanticCacheReviewStatus(value string) bool {
	switch strings.TrimSpace(value) {
	case SemanticCacheReviewPending, SemanticCacheReviewReusable, SemanticCacheReviewNotReusable, SemanticCacheReviewDisputed:
		return true
	default:
		return false
	}
}

func isValidSemanticCacheFeedbackType(value string) bool {
	switch strings.TrimSpace(value) {
	case SemanticCacheFeedbackNone, SemanticCacheFeedbackWrongHit, SemanticCacheFeedbackComplaint, SemanticCacheFeedbackManualMark:
		return true
	default:
		return false
	}
}

func jsonMarshalSemanticCacheConfig(cfg SemanticCacheConfig) (string, error) {
	payload, err := json.Marshal(semanticCacheConfigToStored(cfg))
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
