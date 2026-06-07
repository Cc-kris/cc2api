package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	opsAIManualMaxWindow        = 24 * time.Hour
	opsAIManualMaxActiveTasks   = 3
	opsAIManualSamplePageSize   = 1
	opsAIManualMaxFilterJSONLen = 64 * 1024
)

func (s *OpsService) CreateManualAIAnalysisTask(ctx context.Context, req *OpsAIAnalysisTaskCreateRequest, triggerUserID int64) (*OpsAIAnalysisTaskCreateResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s == nil || s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if triggerUserID <= 0 {
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_OPERATOR", "invalid operator")
	}
	cfg, err := s.loadOpsAIAnalysisConfigForUpdate(ctx)
	if err != nil {
		return nil, err
	}
	normalizeOpsAIAnalysisConfig(cfg)
	if !cfg.Enabled || !cfg.ManualEnabled || cfg.BaseURL == "" || cfg.Model == "" || cfg.APIKeyEncrypted == "" {
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_NOT_CONFIGURED", "请先配置 AI 分析服务")
	}

	input, filter, err := s.normalizeManualAIAnalysisTaskInput(req, triggerUserID, cfg)
	if err != nil {
		return nil, err
	}

	list, err := s.GetUnifiedErrors(ctx, filter)
	if err != nil {
		return nil, err
	}
	if list == nil || list.Total == 0 {
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_NO_ERRORS", "当前筛选条件下暂无可分析的错误")
	}
	task, result, err := s.opsRepo.CreateAIAnalysisTaskIfAllowed(ctx, input, opsAIManualMaxActiveTasks)
	if err != nil {
		return nil, err
	}
	switch result {
	case OpsAIAnalysisTaskCreateResultCreated:
		return &OpsAIAnalysisTaskCreateResponse{TaskID: task.ID, Status: task.Status, SampleCount: task.SampleCount, MatchedErrorCount: list.Total, Message: "AI 分析任务已提交"}, nil
	case OpsAIAnalysisTaskCreateResultDuplicate:
		return nil, infraerrors.Conflict("OPS_AI_ANALYSIS_TASK_DUPLICATE", "分析任务处理中，请稍后查看")
	case OpsAIAnalysisTaskCreateResultQueueBusy:
		return nil, infraerrors.TooManyRequests("OPS_AI_ANALYSIS_QUEUE_BUSY", "AI 分析队列繁忙，请稍后重试")
	default:
		return nil, infraerrors.ServiceUnavailable("OPS_AI_ANALYSIS_TASK_CREATE_FAILED", "AI analysis task create failed")
	}
}

func (s *OpsService) normalizeManualAIAnalysisTaskInput(req *OpsAIAnalysisTaskCreateRequest, triggerUserID int64, cfg *OpsAIAnalysisConfig) (*OpsAIAnalysisTaskCreateInput, *OpsUnifiedErrorListFilter, error) {
	if req == nil {
		return nil, nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_REQUEST", "Invalid request body")
	}
	sourceType := strings.TrimSpace(req.SourceType)
	if sourceType == "" {
		sourceType = OpsAIAnalysisSourceManualFilter
	}
	if sourceType != OpsAIAnalysisSourceUnifiedErrors && sourceType != OpsAIAnalysisSourceManualFilter {
		return nil, nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_SOURCE", "source_type must be unified_errors or manual_filter")
	}
	start, err := parseOpsAITaskTime(req.TimeStart, "time_start")
	if err != nil {
		return nil, nil, err
	}
	end, err := parseOpsAITaskTime(req.TimeEnd, "time_end")
	if err != nil {
		return nil, nil, err
	}
	if end.Before(start) || end.Equal(start) {
		return nil, nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_TIME_RANGE", "time_end must be after time_start")
	}
	if end.Sub(start) > opsAIManualMaxWindow {
		return nil, nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_TIME_RANGE_TOO_LARGE", "手动 AI 分析时间范围不能超过 24 小时")
	}
	filters := req.Filters
	if filters == nil {
		filters = map[string]any{}
	}
	filter, canonicalFilters, err := opsUnifiedFilterFromAIAnalysisFilters(filters, start, end)
	if err != nil {
		return nil, nil, err
	}
	filtersJSONBytes, err := json.Marshal(canonicalFilters)
	if err != nil {
		return nil, nil, err
	}
	if len(filtersJSONBytes) > opsAIManualMaxFilterJSONLen {
		return nil, nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_FILTER_TOO_LARGE", "filters is too large")
	}
	if req.SourceID != nil && *req.SourceID <= 0 {
		return nil, nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_SOURCE", "source_id must be positive integer")
	}
	triggerID := triggerUserID
	input := &OpsAIAnalysisTaskCreateInput{
		SourceType:    sourceType,
		SourceID:      req.SourceID,
		TriggerType:   OpsAIAnalysisTriggerManual,
		TriggerUserID: &triggerID,
		TimeStart:     start,
		TimeEnd:       end,
		FiltersJSON:   string(filtersJSONBytes),
		Status:        OpsAIAnalysisStatusPending,
		Provider:      cfg.InterfaceType,
		Model:         cfg.Model,
	}
	return input, filter, nil
}

func parseOpsAITaskTime(raw string, field string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_TIME", field+" is required")
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_TIME", field+" must be RFC3339")
	}
	return parsed, nil
}

func opsUnifiedFilterFromAIAnalysisFilters(filters map[string]any, start, end time.Time) (*OpsUnifiedErrorListFilter, map[string]any, error) {
	filter := &OpsUnifiedErrorListFilter{StartTime: &start, EndTime: &end, Page: 1, PageSize: opsAIManualSamplePageSize, SortBy: "occurred_at", SortOrder: "desc"}
	canonical := map[string]any{}
	var err error
	if filter.ErrorCategories, err = aiStringSliceFilter(filters, "error_categories", IsValidOpsErrorCategory); err != nil {
		return nil, nil, err
	}
	if len(filter.ErrorCategories) > 0 {
		canonical["error_categories"] = filter.ErrorCategories
	}
	if filter.ErrorSubcategories, err = aiStringSliceFilter(filters, "error_subcategories", nil); err != nil {
		return nil, nil, err
	}
	if len(filter.ErrorSubcategories) > 0 {
		canonical["error_subcategories"] = filter.ErrorSubcategories
	}
	if filter.ClientErrorSubcategories, err = aiStringSliceFilter(filters, "client_error_subcategories", IsValidOpsClientErrorSubcategory); err != nil {
		return nil, nil, err
	}
	if len(filter.ClientErrorSubcategories) > 0 {
		canonical["client_error_subcategories"] = filter.ClientErrorSubcategories
	}
	if filter.ErrorResults, err = aiStringSliceFilter(filters, "error_results", IsValidOpsUnifiedErrorResult); err != nil {
		return nil, nil, err
	}
	if len(filter.ErrorResults) > 0 {
		canonical["error_results"] = filter.ErrorResults
	}
	if filter.Severities, err = aiStringSliceFilter(filters, "severity", IsValidOpsUnifiedSeverity); err != nil {
		return nil, nil, err
	}
	if len(filter.Severities) > 0 {
		canonical["severity"] = filter.Severities
	}
	if filter.StatusCodes, err = aiStatusCodesFilter(filters, "status_code"); err != nil {
		return nil, nil, err
	}
	if len(filter.StatusCodes) > 0 {
		canonical["status_code"] = filter.StatusCodes
	}
	if filter.UserID, err = aiInt64PtrFilter(filters, "user_id"); err != nil {
		return nil, nil, err
	}
	if filter.UserID != nil {
		canonical["user_id"] = *filter.UserID
	}
	if filter.APIKeyID, err = aiInt64PtrFilter(filters, "api_key_id"); err != nil {
		return nil, nil, err
	}
	if filter.APIKeyID != nil {
		canonical["api_key_id"] = *filter.APIKeyID
	}
	if filter.GroupID, err = aiInt64PtrFilter(filters, "group_id"); err != nil {
		return nil, nil, err
	}
	if filter.GroupID != nil {
		canonical["group_id"] = *filter.GroupID
	}
	if filter.UpstreamAccountID, err = aiInt64PtrFilter(filters, "upstream_account_id"); err != nil {
		return nil, nil, err
	}
	if filter.UpstreamAccountID != nil {
		canonical["upstream_account_id"] = *filter.UpstreamAccountID
	}
	if filter.Platform, err = aiStringFilter(filters, "platform", 64); err != nil {
		return nil, nil, err
	}
	if filter.Platform != "" {
		canonical["platform"] = filter.Platform
	}
	if filter.Model, err = aiStringFilter(filters, "model", 128); err != nil {
		return nil, nil, err
	}
	if filter.Model != "" {
		canonical["model"] = filter.Model
	}
	if filter.RequestID, err = aiStringFilter(filters, "request_id", 128); err != nil {
		return nil, nil, err
	}
	if filter.RequestID != "" {
		canonical["request_id"] = filter.RequestID
	}
	if filter.Keyword, err = aiStringFilter(filters, "keyword", 100); err != nil {
		return nil, nil, err
	}
	if filter.Keyword != "" {
		if len([]rune(filter.Keyword)) < 2 {
			return nil, nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FILTER", "keyword must be 2-100 characters")
		}
		canonical["keyword"] = filter.Keyword
	}
	filter.AIAnalysis = OpsUnifiedAIAnalysisAll
	return filter, canonical, nil
}

func aiStringSliceFilter(filters map[string]any, key string, validate func(string) bool) ([]string, error) {
	raw, ok := filters[key]
	if !ok || raw == nil {
		return nil, nil
	}
	var values []string
	switch v := raw.(type) {
	case []string:
		values = v
	case []any:
		values = make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FILTER", key+" must be string array")
			}
			values = append(values, s)
		}
	default:
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FILTER", key+" must be string array")
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if validate != nil && !validate(value) {
			return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FILTER", "invalid "+key)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func aiStatusCodesFilter(filters map[string]any, key string) ([]int, error) {
	raw, ok := filters[key]
	if !ok || raw == nil {
		return nil, nil
	}
	var items []any
	switch v := raw.(type) {
	case []any:
		items = v
	case []int:
		items = make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, item)
		}
	default:
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FILTER", key+" must be number array")
	}
	out := make([]int, 0, len(items))
	seen := map[int]struct{}{}
	for _, item := range items {
		var code int
		switch v := item.(type) {
		case float64:
			if v != float64(int(v)) {
				return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FILTER", key+" must be number array")
			}
			code = int(v)
		case int:
			code = v
		default:
			return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FILTER", key+" must be number array")
		}
		if code < 100 || code > 599 {
			return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FILTER", "invalid status_code")
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	sort.Ints(out)
	return out, nil
}

func aiInt64PtrFilter(filters map[string]any, key string) (*int64, error) {
	raw, ok := filters[key]
	if !ok || raw == nil {
		return nil, nil
	}
	f, ok := raw.(float64)
	if !ok || f != float64(int64(f)) || f <= 0 {
		return nil, infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FILTER", key+" must be positive integer")
	}
	v := int64(f)
	return &v, nil
}

func aiStringFilter(filters map[string]any, key string, maxRunes int) (string, error) {
	raw, ok := filters[key]
	if !ok || raw == nil {
		return "", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FILTER", key+" must be string")
	}
	s = strings.TrimSpace(s)
	if len([]rune(s)) > maxRunes {
		return "", infraerrors.BadRequest("OPS_AI_ANALYSIS_INVALID_FILTER", key+" is too long")
	}
	return s, nil
}
