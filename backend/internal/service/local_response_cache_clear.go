package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	LocalResponseCacheClearTypeAll        = "all"
	LocalResponseCacheClearTypeByPlatform = "by_platform"
	LocalResponseCacheClearTypeByModel    = "by_model"
	LocalResponseCacheClearTypeByGroup    = "by_group"
	LocalResponseCacheClearTypeByAPIKey   = "by_api_key"
	LocalResponseCacheClearTypeByTime     = "by_time"
	LocalResponseCacheClearTypeExpired    = "expired"

	LocalResponseCacheClearStatusSuccess        = "success"
	LocalResponseCacheClearStatusFailed         = "failed"
	LocalResponseCacheClearStatusPartialSuccess = "partial_success"

	LocalResponseCacheClearConfirmText = "确认清理"
)

var (
	ErrLocalResponseCacheClearUnavailable = errors.New("local response cache clear store unavailable")
	ErrInvalidLocalResponseCacheClear     = errors.New("invalid local response cache clear request")
	ErrInvalidLocalResponseCacheAuditList = errors.New("invalid local response cache clear audit list request")
)

type LocalResponseCacheClearScope struct {
	Platforms []string   `json:"platforms,omitempty"`
	Models    []string   `json:"models,omitempty"`
	GroupIDs  []int64    `json:"group_ids,omitempty"`
	APIKeyIDs []int64    `json:"api_key_ids,omitempty"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
}

type LocalResponseCacheClearRequest struct {
	ClearType      string                       `json:"clear_type"`
	Scope          LocalResponseCacheClearScope `json:"scope"`
	ConfirmText    string                       `json:"confirm_text"`
	OperatorUserID *int64                       `json:"-"`
	ViewerRole     string                       `json:"-"`
}

type LocalResponseCacheClearResult struct {
	ClearType    string                       `json:"clear_type"`
	Scope        LocalResponseCacheClearScope `json:"scope"`
	MatchedKeys  int64                        `json:"matched_keys"`
	DeletedKeys  int64                        `json:"deleted_keys"`
	Status       string                       `json:"status"`
	ErrorMessage string                       `json:"error_message,omitempty"`
}

type LocalResponseCacheClearAudit struct {
	OperatorUserID *int64
	ClearType      string
	Scope          LocalResponseCacheClearScope
	MatchedKeys    int64
	DeletedKeys    int64
	Status         string
	ErrorMessage   string
}

type LocalResponseCacheClearAuditRecord struct {
	ID             int64                        `json:"id"`
	OperatorUserID *int64                       `json:"operator_user_id,omitempty"`
	ClearType      string                       `json:"clear_type"`
	Scope          LocalResponseCacheClearScope `json:"scope"`
	MatchedKeys    int64                        `json:"matched_keys"`
	DeletedKeys    int64                        `json:"deleted_keys"`
	Status         string                       `json:"status"`
	ErrorMessage   string                       `json:"error_message,omitempty"`
	CreatedAt      time.Time                    `json:"created_at"`
}

type LocalResponseCacheClearAuditFilter struct {
	Page           int
	PageSize       int
	StartTime      *time.Time
	EndTime        *time.Time
	OperatorUserID *int64
	ClearType      string
	Status         string
	ViewerRole     string
}

type LocalResponseCacheClearAuditPage struct {
	Items    []LocalResponseCacheClearAuditRecord `json:"items"`
	Total    int64                                `json:"total"`
	Page     int                                  `json:"page"`
	PageSize int                                  `json:"page_size"`
}

type LocalResponseCacheClearStore interface {
	ClearLocalResponseCache(ctx context.Context, req LocalResponseCacheClearRequest) (*LocalResponseCacheClearResult, error)
	RecordLocalResponseCacheClearAudit(ctx context.Context, audit LocalResponseCacheClearAudit) error
}

type LocalResponseCacheClearAuditStore interface {
	ListLocalResponseCacheClearAudits(ctx context.Context, filter LocalResponseCacheClearAuditFilter) (*LocalResponseCacheClearAuditPage, error)
}

func (s *OpenAIGatewayService) ClearLocalResponseCache(ctx context.Context, req LocalResponseCacheClearRequest) (*LocalResponseCacheClearResult, error) {
	if !canClearCache(req.ViewerRole) {
		return nil, ErrInvalidLocalResponseCacheClear
	}
	req.ClearType = strings.TrimSpace(req.ClearType)
	if err := validateLocalResponseCacheClearRequest(req); err != nil {
		return nil, err
	}
	if s == nil || s.cache == nil {
		return nil, ErrLocalResponseCacheClearUnavailable
	}
	store, ok := s.cache.(LocalResponseCacheClearStore)
	if !ok {
		return nil, ErrLocalResponseCacheClearUnavailable
	}

	result, clearErr := store.ClearLocalResponseCache(ctx, req)
	if result == nil {
		result = &LocalResponseCacheClearResult{ClearType: req.ClearType, Scope: req.Scope}
	}
	if clearErr != nil {
		result.Status = LocalResponseCacheClearStatusFailed
		result.ErrorMessage = clearErr.Error()
	} else if result.Status == "" {
		result.Status = LocalResponseCacheClearStatusSuccess
	}

	auditErr := store.RecordLocalResponseCacheClearAudit(ctx, LocalResponseCacheClearAudit{
		OperatorUserID: req.OperatorUserID,
		ClearType:      req.ClearType,
		Scope:          req.Scope,
		MatchedKeys:    result.MatchedKeys,
		DeletedKeys:    result.DeletedKeys,
		Status:         result.Status,
		ErrorMessage:   result.ErrorMessage,
	})
	if clearErr != nil {
		return result, clearErr
	}
	if auditErr != nil {
		return result, fmt.Errorf("record local response cache clear audit: %w", auditErr)
	}
	return result, nil
}

func (s *OpenAIGatewayService) ListLocalResponseCacheClearAudits(ctx context.Context, filter LocalResponseCacheClearAuditFilter) (*LocalResponseCacheClearAuditPage, error) {
	if !canViewCacheClearAudits(filter.ViewerRole) {
		return &LocalResponseCacheClearAuditPage{Items: []LocalResponseCacheClearAuditRecord{}}, nil
	}
	filter.ClearType = strings.TrimSpace(filter.ClearType)
	filter.Status = strings.TrimSpace(filter.Status)
	normalized, err := normalizeLocalResponseCacheClearAuditFilter(filter)
	if err != nil {
		return nil, err
	}
	if s == nil || s.cache == nil {
		return nil, ErrLocalResponseCacheClearUnavailable
	}
	store, ok := s.cache.(LocalResponseCacheClearAuditStore)
	if !ok {
		return nil, ErrLocalResponseCacheClearUnavailable
	}
	return store.ListLocalResponseCacheClearAudits(ctx, normalized)
}

func validateLocalResponseCacheClearRequest(req LocalResponseCacheClearRequest) error {
	scope := req.Scope
	switch req.ClearType {
	case LocalResponseCacheClearTypeAll:
		if strings.TrimSpace(req.ConfirmText) != LocalResponseCacheClearConfirmText {
			return fmt.Errorf("%w: confirm_text must be %q", ErrInvalidLocalResponseCacheClear, LocalResponseCacheClearConfirmText)
		}
	case LocalResponseCacheClearTypeByPlatform:
		if len(cleanStringList(scope.Platforms)) == 0 {
			return fmt.Errorf("%w: platforms is required", ErrInvalidLocalResponseCacheClear)
		}
	case LocalResponseCacheClearTypeByModel:
		if len(cleanStringList(scope.Models)) == 0 {
			return fmt.Errorf("%w: models is required", ErrInvalidLocalResponseCacheClear)
		}
	case LocalResponseCacheClearTypeByGroup:
		if len(cleanPositiveInt64List(scope.GroupIDs)) == 0 {
			return fmt.Errorf("%w: group_ids is required", ErrInvalidLocalResponseCacheClear)
		}
	case LocalResponseCacheClearTypeByAPIKey:
		if len(cleanPositiveInt64List(scope.APIKeyIDs)) == 0 {
			return fmt.Errorf("%w: api_key_ids is required", ErrInvalidLocalResponseCacheClear)
		}
	case LocalResponseCacheClearTypeByTime:
		if scope.StartTime == nil || scope.EndTime == nil {
			return fmt.Errorf("%w: start_time and end_time are required", ErrInvalidLocalResponseCacheClear)
		}
		if scope.StartTime.After(*scope.EndTime) {
			return fmt.Errorf("%w: start_time must be before end_time", ErrInvalidLocalResponseCacheClear)
		}
		if scope.EndTime.Sub(*scope.StartTime) > 31*24*time.Hour {
			return fmt.Errorf("%w: time range cannot exceed 31 days", ErrInvalidLocalResponseCacheClear)
		}
	case LocalResponseCacheClearTypeExpired:
		// No extra scope is required.
	default:
		return fmt.Errorf("%w: clear_type is required", ErrInvalidLocalResponseCacheClear)
	}
	return nil
}

func normalizeLocalResponseCacheClearAuditFilter(filter LocalResponseCacheClearAuditFilter) (LocalResponseCacheClearAuditFilter, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	if filter.StartTime != nil && filter.EndTime != nil {
		if filter.StartTime.After(*filter.EndTime) {
			return filter, fmt.Errorf("%w: start_time must be before end_time", ErrInvalidLocalResponseCacheAuditList)
		}
		if filter.EndTime.Sub(*filter.StartTime) > 31*24*time.Hour {
			return filter, fmt.Errorf("%w: time range cannot exceed 31 days", ErrInvalidLocalResponseCacheAuditList)
		}
	}
	if filter.OperatorUserID != nil && *filter.OperatorUserID <= 0 {
		return filter, fmt.Errorf("%w: invalid operator_user_id", ErrInvalidLocalResponseCacheAuditList)
	}
	if filter.ClearType != "" && !isValidLocalResponseCacheClearType(filter.ClearType) {
		return filter, fmt.Errorf("%w: invalid clear_type", ErrInvalidLocalResponseCacheAuditList)
	}
	if filter.Status != "" && !isValidLocalResponseCacheClearStatus(filter.Status) {
		return filter, fmt.Errorf("%w: invalid status", ErrInvalidLocalResponseCacheAuditList)
	}
	return filter, nil
}

func isValidLocalResponseCacheClearType(clearType string) bool {
	switch clearType {
	case LocalResponseCacheClearTypeAll,
		LocalResponseCacheClearTypeByPlatform,
		LocalResponseCacheClearTypeByModel,
		LocalResponseCacheClearTypeByGroup,
		LocalResponseCacheClearTypeByAPIKey,
		LocalResponseCacheClearTypeByTime,
		LocalResponseCacheClearTypeExpired:
		return true
	default:
		return false
	}
}

func isValidLocalResponseCacheClearStatus(status string) bool {
	switch status {
	case LocalResponseCacheClearStatusSuccess,
		LocalResponseCacheClearStatusFailed,
		LocalResponseCacheClearStatusPartialSuccess:
		return true
	default:
		return false
	}
}

func cleanStringList(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func cleanPositiveInt64List(values []int64) []int64 {
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
