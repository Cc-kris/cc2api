package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) GetIncidentOverview(ctx context.Context, filter *OpsDashboardFilter) (*OpsIncidentOverview, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if filter == nil {
		return nil, infraerrors.BadRequest("OPS_FILTER_REQUIRED", "filter is required")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}

	overview, err := s.GetDashboardOverview(ctx, filter)
	if err != nil {
		return nil, err
	}

	impact, err := s.opsRepo.GetIncidentImpact(ctx, filter)
	if err != nil {
		return nil, err
	}
	if overview == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_DASHBOARD_OVERVIEW_UNAVAILABLE", "Ops dashboard overview is unavailable")
	}
	if impact == nil {
		impact = &OpsIncidentImpact{}
	}
	if impact.AffectedModels == nil {
		impact.AffectedModels = []string{}
	}
	if impact.AffectedAccounts == nil {
		impact.AffectedAccounts = []*OpsIncidentAffectedAccount{}
	}

	alerts, err := s.loadOpsIncidentAlerts(ctx, filter)
	if err != nil {
		return nil, err
	}
	topAlert := pickTopOpsIncidentAlert(alerts)

	finalFailures := finalFailureCount(overview)
	finalRate := finalFailureRate(overview)
	recovered := overview.ErrorCountTotal - finalFailures - overview.BusinessLimitedCount
	if recovered < 0 {
		recovered = 0
	}

	score := overview.HealthScore
	level := opsIncidentScoreLevel(score)
	status := opsIncidentStatus(topAlert, score, finalFailures, finalRate)
	quickFilters := buildOpsIncidentQuickFilters(filter, overview, impact)

	// Query optional side panels for final_failed errors and latest generated AI analysis.
	errorCategoryCounts, _ := s.getErrorCategoryCounts(ctx, filter)
	latestAIAnalysis, _ := s.GetLatestAIAnalysisReportSummary(ctx)

	return &OpsIncidentOverview{
		Status:                status,
		HealthRiskScore:       score,
		ScoreLevel:            level,
		ScoreReasons:          opsIncidentScoreReasonStrings(overview.HealthScoreReasons),
		Summary:               buildOpsIncidentSummary(status, topAlert, finalFailures, finalRate, impact),
		FinalFailures:         finalFailures,
		FinalFailureRate:      roundIncidentFloat(finalRate),
		RecoveredFluctuations: recovered,
		TotalRequests:         effectiveRequestCount(overview),
		AffectedUsers:         impact.AffectedUsers,
		AffectedAPIKeys:       impact.AffectedAPIKeys,
		AffectedModels:        impact.AffectedModels,
		AffectedAccounts:      impact.AffectedAccounts,
		SystemMetrics:         overview.SystemMetrics,
		LatestAIAnalysis:      latestAIAnalysis,
		QuickFilters:          quickFilters,
		RecommendedActions:    buildOpsIncidentRecommendedActions(status, topAlert, overview),
		ErrorCategoryCounts:   errorCategoryCounts,
		UpdatedAt:             time.Now().UTC(),
	}, nil
}

func (s *OpsService) loadOpsIncidentAlerts(ctx context.Context, filter *OpsDashboardFilter) ([]*OpsAlertEvent, error) {
	active, err := s.opsRepo.ListAlertEvents(ctx, &OpsAlertEventFilter{
		Limit:    20,
		Status:   OpsAlertStatusFiring,
		EndTime:  &filter.EndTime,
		Platform: filter.Platform,
		Model:    filter.Model,
		GroupID:  filter.GroupID,
	})
	if err != nil {
		return nil, err
	}
	recent, err := s.opsRepo.ListAlertEvents(ctx, &OpsAlertEventFilter{
		Limit:     20,
		StartTime: &filter.StartTime,
		EndTime:   &filter.EndTime,
		Platform:  filter.Platform,
		Model:     filter.Model,
		GroupID:   filter.GroupID,
	})
	if err != nil {
		return nil, err
	}

	seen := map[int64]struct{}{}
	out := make([]*OpsAlertEvent, 0, len(active)+len(recent))
	for _, alert := range append(active, recent...) {
		if alert == nil {
			continue
		}
		if alert.ID > 0 {
			if _, ok := seen[alert.ID]; ok {
				continue
			}
			seen[alert.ID] = struct{}{}
		}
		out = append(out, alert)
	}
	return out, nil
}

func pickTopOpsIncidentAlert(alerts []*OpsAlertEvent) *OpsAlertEvent {
	var best *OpsAlertEvent
	bestRank := -1
	for _, alert := range alerts {
		if alert == nil {
			continue
		}
		rank := opsIncidentSeverityRank(alert.Severity)
		if alert.Status == OpsAlertStatusFiring {
			rank += 10
		}
		if best == nil || rank > bestRank || (rank == bestRank && alert.FiredAt.After(best.FiredAt)) {
			best = alert
			bestRank = rank
		}
	}
	return best
}

func opsIncidentSeverityRank(severity string) int {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "P0", "CRITICAL":
		return 4
	case "P1", "HIGH":
		return 3
	case "P2", "MEDIUM", "WARN", "WARNING":
		return 2
	case "P3", "LOW", "OBSERVE", "INFO":
		return 1
	default:
		return 0
	}
}

func opsIncidentScoreLevel(score int) string {
	switch {
	case score < 50:
		return OpsIncidentScoreLevelIncident
	case score < 70:
		return OpsIncidentScoreLevelRisk
	case score < 90:
		return OpsIncidentScoreLevelObserving
	default:
		return OpsIncidentScoreLevelNormal
	}
}

func opsIncidentStatus(alert *OpsAlertEvent, score int, failures int64, failureRate float64) string {
	if alert != nil && alert.Status == OpsAlertStatusFiring {
		switch opsIncidentSeverityRank(alert.Severity) {
		case 4:
			return OpsIncidentStatusIncident
		case 3:
			return OpsIncidentStatusRisk
		case 2, 1:
			return OpsIncidentStatusObserving
		}
	}
	if score < 50 || failures >= 50 || failureRate >= 0.20 {
		return OpsIncidentStatusIncident
	}
	if score < 70 || failures >= 10 || failureRate >= 0.05 {
		return OpsIncidentStatusRisk
	}
	if score < 90 || failures > 0 || failureRate > 0 {
		return OpsIncidentStatusObserving
	}
	return OpsIncidentStatusNormal
}

func opsIncidentScoreReasonStrings(reasons []*OpsHealthScoreReason) []string {
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		if reason == nil {
			continue
		}
		message := strings.TrimSpace(reason.Message)
		value := strings.TrimSpace(reason.Value)
		if message == "" && value == "" {
			continue
		}
		text := ""
		switch {
		case message == "":
			text = value
		case value == "":
			text = message
		default:
			text = fmt.Sprintf("%s：%s", message, value)
		}
		if meaning := opsIncidentScoreReasonMeaning(reason); meaning != "" && !strings.Contains(text, "（") {
			text = fmt.Sprintf("%s（%s）", text, meaning)
		}
		out = append(out, text)
	}
	if len(out) == 0 {
		return []string{"暂无异常原因"}
	}
	return out
}

func opsIncidentScoreReasonMeaning(reason *OpsHealthScoreReason) string {
	if reason == nil {
		return ""
	}
	switch strings.TrimSpace(reason.Code) {
	case OpsHealthReasonFinalFailures:
		return "当前窗口内真正失败并影响用户的请求次数"
	case OpsHealthReasonFailureRate:
		return "最终失败占有效请求的比例，比例越高客户体感越差"
	case OpsHealthReasonEffectiveRequests:
		return "参与本次判断的有效请求量，样本越小分数越需要结合明细判断"
	case OpsHealthReasonImpactScope:
		return "错误主要分布在平台、上游或客户端，用来判断优先排查方向"
	case OpsHealthReasonDependencyStatus:
		return "数据库、Redis、后台任务等基础依赖的健康情况"
	default:
		return "该项参与健康风险分数计算"
	}
}

func buildOpsIncidentSummary(status string, alert *OpsAlertEvent, failures int64, failureRate float64, impact *OpsIncidentImpact) string {
	if failures == 0 && status == OpsIncidentStatusNormal {
		return "当前系统运行正常，最近窗口暂无最终失败。"
	}
	parts := make([]string, 0, 4)
	if alert != nil && strings.TrimSpace(alert.Title) != "" {
		parts = append(parts, "当前主要问题："+strings.TrimSpace(alert.Title))
	} else {
		parts = append(parts, "当前存在需要观察的错误波动")
	}
	parts = append(parts, fmt.Sprintf("最终失败 %d 次，失败率 %.2f%%", failures, failureRate*100))
	if impact != nil {
		parts = append(parts, fmt.Sprintf("影响用户 %d 个、API Key %d 个", impact.AffectedUsers, impact.AffectedAPIKeys))
		if len(impact.AffectedModels) > 0 {
			parts = append(parts, "涉及模型 "+strings.Join(impact.AffectedModels, "、"))
		}
	}
	return strings.Join(parts, "；") + "。"
}

func buildOpsIncidentRecommendedActions(status string, alert *OpsAlertEvent, overview *OpsDashboardOverview) []string {
	if status == OpsIncidentStatusNormal {
		return []string{"保持观察，无需立即处理。"}
	}
	out := make([]string, 0, 3)
	if alert != nil && strings.TrimSpace(alert.Description) != "" {
		out = append(out, strings.TrimSpace(alert.Description))
	}
	if overview != nil {
		switch {
		case overview.UpstreamErrorCount > 0 || overview.UpstreamLimitedCount > 0:
			out = append(out, "优先检查对应上游账号、渠道可用性和限流状态。")
		case overview.PlatformErrorCount > 0:
			out = append(out, "优先检查平台内部服务、数据库、Redis 和后台任务状态。")
		case overview.ClientErrorCount > 0:
			out = append(out, "优先查看客户端请求参数、认证、额度和调用方式。")
		}
	}
	out = append(out, "进入统一错误列表查看明细，并对当前筛选范围发起 AI 分析。")
	return dedupeNonEmptyStrings(out)
}

func buildOpsIncidentQuickFilters(filter *OpsDashboardFilter, overview *OpsDashboardOverview, impact *OpsIncidentImpact) []*OpsIncidentQuickFilter {
	base := map[string]string{}
	if filter != nil {
		base["start_time"] = filter.StartTime.UTC().Format(time.RFC3339)
		base["end_time"] = filter.EndTime.UTC().Format(time.RFC3339)
		if strings.TrimSpace(filter.Platform) != "" {
			base["platform"] = strings.TrimSpace(filter.Platform)
		}
		if strings.TrimSpace(filter.Model) != "" {
			base["model"] = strings.TrimSpace(filter.Model)
		}
		if filter.GroupID != nil && *filter.GroupID > 0 {
			base["group_id"] = strconv.FormatInt(*filter.GroupID, 10)
		}
	}

	out := make([]*OpsIncidentQuickFilter, 0, 6)
	if overview != nil && finalFailureCount(overview) > 0 {
		out = append(out, &OpsIncidentQuickFilter{Label: "最终失败", Params: copyIncidentFilterParams(base, map[string]string{"impact_platform_sla": "true"})})
	}
	if overview != nil && overview.UpstreamErrorCount+overview.UpstreamLimitedCount > 0 {
		out = append(out, &OpsIncidentQuickFilter{Label: "上游错误", Params: copyIncidentFilterParams(base, map[string]string{"category": "upstream_error"})})
	}
	if overview != nil && overview.PlatformErrorCount > 0 {
		out = append(out, &OpsIncidentQuickFilter{Label: "平台错误", Params: copyIncidentFilterParams(base, map[string]string{"category": "platform_error"})})
	}
	if overview != nil && overview.ClientErrorCount > 0 {
		out = append(out, &OpsIncidentQuickFilter{Label: "客户端错误", Params: copyIncidentFilterParams(base, map[string]string{"category": "client_error"})})
	}
	if impact != nil {
		for _, model := range impact.AffectedModels {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			out = append(out, &OpsIncidentQuickFilter{Label: "模型：" + model, Params: copyIncidentFilterParams(base, map[string]string{"model": model})})
			break
		}
		for _, account := range impact.AffectedAccounts {
			if account == nil || account.ID <= 0 {
				continue
			}
			label := "上游账号：" + strconv.FormatInt(account.ID, 10)
			if strings.TrimSpace(account.Name) != "" {
				label = "上游账号：" + strings.TrimSpace(account.Name)
			}
			out = append(out, &OpsIncidentQuickFilter{Label: label, Params: copyIncidentFilterParams(base, map[string]string{"account_id": strconv.FormatInt(account.ID, 10)})})
			break
		}
	}
	return out
}

func copyIncidentFilterParams(base map[string]string, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func dedupeNonEmptyStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func roundIncidentFloat(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func (s *OpsService) getErrorCategoryCounts(ctx context.Context, filter *OpsDashboardFilter) (map[string]int64, error) {
	if s.opsRepo == nil || filter == nil {
		return nil, nil
	}

	// Query error logs with error_result = 'final_failed'
	result, err := s.opsRepo.ListErrorLogs(ctx, &OpsErrorLogFilter{
		StartTime:   &filter.StartTime,
		EndTime:     &filter.EndTime,
		Platform:    filter.Platform,
		Model:       filter.Model,
		GroupID:     filter.GroupID,
		ErrorResult: "final_failed",
		Page:        1,
		PageSize:    10000, // Large page size to get all errors
		View:        "all",
	})
	if err != nil || result == nil {
		return nil, nil
	}

	counts := make(map[string]int64)
	for _, item := range result.Errors {
		if item == nil {
			continue
		}
		category := strings.TrimSpace(strings.ToLower(item.ErrorCategory))
		if category == "" {
			category = "unknown"
		}
		counts[category]++
	}

	return counts, nil
}
