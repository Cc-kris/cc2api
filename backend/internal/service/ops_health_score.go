package service

import (
	"fmt"
	"math"
	"time"
)

const (
	opsHealthSmallSampleMinScore = 70

	OpsHealthReasonFinalFailures     = "final_failures"
	OpsHealthReasonFailureRate       = "failure_rate"
	OpsHealthReasonEffectiveRequests = "effective_requests"
	OpsHealthReasonImpactScope       = "impact_scope"
	OpsHealthReasonDependencyStatus  = "dependency_status"
)

type OpsHealthScoreResult struct {
	Score   int
	Reasons []*OpsHealthScoreReason
}

func buildOpsHealthScoreReasons(now time.Time, overview *OpsDashboardOverview) []*OpsHealthScoreReason {
	if overview == nil {
		return []*OpsHealthScoreReason{{Code: OpsHealthReasonDependencyStatus, Message: "overview data unavailable", Value: "unavailable"}}
	}

	finalFailures := finalFailureCount(overview)
	effectiveRequests := effectiveRequestCount(overview)
	failureRate := finalFailureRate(overview)
	impactScope := impactScopeSummary(overview)
	dependencyStatus := dependencyStatusSummary(now, overview)

	return []*OpsHealthScoreReason{
		{Code: OpsHealthReasonFinalFailures, Message: "final failures in window", Value: fmt.Sprintf("%d", finalFailures)},
		{Code: OpsHealthReasonFailureRate, Message: "final failure rate", Value: fmt.Sprintf("%.2f%%", failureRate*100)},
		{Code: OpsHealthReasonEffectiveRequests, Message: "effective requests in window", Value: fmt.Sprintf("%d", effectiveRequests)},
		{Code: OpsHealthReasonImpactScope, Message: "impact scope", Value: impactScope},
		{Code: OpsHealthReasonDependencyStatus, Message: "dependency status", Value: dependencyStatus},
	}
}

func isSmallOneMinuteFailureWindow(overview *OpsDashboardOverview) bool {
	if overview == nil {
		return false
	}
	return isOneMinuteWindow(overview) && finalFailureCount(overview) <= 2
}

func isOneMinuteWindow(overview *OpsDashboardOverview) bool {
	if overview == nil || overview.StartTime.IsZero() || overview.EndTime.IsZero() {
		return false
	}
	d := overview.EndTime.Sub(overview.StartTime)
	return d > 0 && d <= time.Minute
}

func finalFailureCount(overview *OpsDashboardOverview) int64 {
	if overview == nil {
		return 0
	}
	if overview.ErrorCountSLA > 0 {
		return overview.ErrorCountSLA
	}
	return overview.ErrorCountTotal
}

func effectiveRequestCount(overview *OpsDashboardOverview) int64 {
	if overview == nil {
		return 0
	}
	if overview.RequestCountSLA > 0 {
		return overview.RequestCountSLA
	}
	return overview.RequestCountTotal
}

func finalFailureRate(overview *OpsDashboardOverview) float64 {
	requests := effectiveRequestCount(overview)
	if requests <= 0 {
		return 0
	}
	return clampFloat64(float64(finalFailureCount(overview))/float64(requests), 0, 1)
}

func impactScopeSummary(overview *OpsDashboardOverview) string {
	if overview == nil {
		return "none"
	}
	parts := make([]string, 0, 3)
	if overview.PlatformErrorCount > 0 {
		parts = append(parts, fmt.Sprintf("platform:%d", overview.PlatformErrorCount))
	}
	if overview.UpstreamErrorCount > 0 || overview.UpstreamLimitedCount > 0 {
		parts = append(parts, fmt.Sprintf("upstream:%d", overview.UpstreamErrorCount+overview.UpstreamLimitedCount))
	}
	if overview.ClientErrorCount > 0 {
		parts = append(parts, fmt.Sprintf("client:%d", overview.ClientErrorCount))
	}
	if len(parts) == 0 {
		return "none"
	}
	return joinStrings(parts, ",")
}

func dependencyStatusSummary(now time.Time, overview *OpsDashboardOverview) string {
	if overview == nil {
		return "unavailable"
	}
	states := make([]string, 0, 3)
	if overview.SystemMetrics != nil {
		if overview.SystemMetrics.DBOK != nil && !*overview.SystemMetrics.DBOK {
			states = append(states, "db:down")
		}
		if overview.SystemMetrics.RedisOK != nil && !*overview.SystemMetrics.RedisOK {
			states = append(states, "redis:down")
		}
	}
	failedJobs := 0
	for _, hb := range overview.JobHeartbeats {
		if hb == nil {
			continue
		}
		if hb.LastErrorAt != nil && (hb.LastSuccessAt == nil || hb.LastErrorAt.After(*hb.LastSuccessAt)) {
			failedJobs++
		} else if hb.LastSuccessAt != nil && now.Sub(*hb.LastSuccessAt) > 15*time.Minute {
			failedJobs++
		}
	}
	if failedJobs > 0 {
		states = append(states, fmt.Sprintf("jobs_failed:%d", failedJobs))
	}
	if len(states) == 0 {
		return "healthy"
	}
	return joinStrings(states, ",")
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, part := range parts[1:] {
		out += sep + part
	}
	return out
}

// computeDashboardHealthScore computes a 0-100 health score from the metrics returned by the dashboard overview.
//
// Design goals:
// - Backend-owned scoring (UI only displays).
// - Layered scoring: Business Health (70%) + Infrastructure Health (30%)
// - Avoids double-counting (e.g., DB failure affects both infra and business metrics)
// - Conservative + stable: penalize clear degradations; avoid overreacting to missing/idle data.
func computeDashboardHealthScoreResult(now time.Time, overview *OpsDashboardOverview) *OpsHealthScoreResult {
	if overview == nil {
		return &OpsHealthScoreResult{
			Score: 0,
			Reasons: []*OpsHealthScoreReason{
				{Code: OpsHealthReasonDependencyStatus, Message: "overview data unavailable", Value: "unavailable"},
			},
		}
	}

	reasons := buildOpsHealthScoreReasons(now, overview)

	// Idle/no-data: avoid showing a "bad" score when there is no traffic.
	// UI can still render a gray/idle state based on QPS + error rate.
	if overview.RequestCountSLA <= 0 && overview.RequestCountTotal <= 0 && overview.ErrorCountTotal <= 0 {
		return &OpsHealthScoreResult{Score: 100, Reasons: reasons}
	}

	businessHealth := computeBusinessHealth(overview)
	infraHealth := computeInfraHealth(now, overview)

	// Weighted combination: 70% business + 30% infrastructure. The score is auxiliary only;
	// alert events and notifications must be driven by explicit alert/incident rules elsewhere.
	score := int(math.Round(clampFloat64(businessHealth*0.7+infraHealth*0.3, 0, 100)))
	if isSmallOneMinuteFailureWindow(overview) && score < opsHealthSmallSampleMinScore {
		score = opsHealthSmallSampleMinScore
	}

	return &OpsHealthScoreResult{Score: score, Reasons: reasons}
}

// computeBusinessHealth calculates business health score (0-100)
// Components: Error Rate (50%) + TTFT (50%)
func computeBusinessHealth(overview *OpsDashboardOverview) float64 {
	// Error rate score: 1% → 100, 10% → 0 (linear)
	// Combines request errors and upstream errors
	errorScore := 100.0
	errorPct := clampFloat64(overview.ErrorRate*100, 0, 100)
	upstreamPct := clampFloat64(overview.UpstreamErrorRate*100, 0, 100)
	combinedErrorPct := math.Max(errorPct, upstreamPct) // Use worst case
	if combinedErrorPct > 1.0 {
		if combinedErrorPct <= 10.0 {
			errorScore = (10.0 - combinedErrorPct) / 9.0 * 100
		} else {
			errorScore = 0
		}
	}

	// TTFT score: 1s → 100, 3s → 0 (linear)
	// Time to first token is critical for user experience
	ttftScore := 100.0
	if overview.TTFT.P99 != nil {
		p99 := float64(*overview.TTFT.P99)
		if p99 > 1000 {
			if p99 <= 3000 {
				ttftScore = (3000 - p99) / 2000 * 100
			} else {
				ttftScore = 0
			}
		}
	}

	// Weighted combination: 50% error rate + 50% TTFT
	return errorScore*0.5 + ttftScore*0.5
}

// computeInfraHealth calculates infrastructure health score (0-100)
// Components: Storage (40%) + Compute Resources (30%) + Background Jobs (30%)
func computeInfraHealth(now time.Time, overview *OpsDashboardOverview) float64 {
	// Storage score: DB critical, Redis less critical
	storageScore := 100.0
	if overview.SystemMetrics != nil {
		if overview.SystemMetrics.DBOK != nil && !*overview.SystemMetrics.DBOK {
			storageScore = 0 // DB failure is critical
		} else if overview.SystemMetrics.RedisOK != nil && !*overview.SystemMetrics.RedisOK {
			storageScore = 50 // Redis failure is degraded but not critical
		}
	}

	// Compute resources score: CPU + Memory
	computeScore := 100.0
	if overview.SystemMetrics != nil {
		cpuScore := 100.0
		if overview.SystemMetrics.CPUUsagePercent != nil {
			cpuPct := clampFloat64(*overview.SystemMetrics.CPUUsagePercent, 0, 100)
			if cpuPct > 80 {
				if cpuPct <= 100 {
					cpuScore = (100 - cpuPct) / 20 * 100
				} else {
					cpuScore = 0
				}
			}
		}

		memScore := 100.0
		if overview.SystemMetrics.MemoryUsagePercent != nil {
			memPct := clampFloat64(*overview.SystemMetrics.MemoryUsagePercent, 0, 100)
			if memPct > 85 {
				if memPct <= 100 {
					memScore = (100 - memPct) / 15 * 100
				} else {
					memScore = 0
				}
			}
		}

		computeScore = (cpuScore + memScore) / 2
	}

	// Background jobs score
	jobScore := 100.0
	failedJobs := 0
	totalJobs := 0
	for _, hb := range overview.JobHeartbeats {
		if hb == nil {
			continue
		}
		totalJobs++
		if hb.LastErrorAt != nil && (hb.LastSuccessAt == nil || hb.LastErrorAt.After(*hb.LastSuccessAt)) {
			failedJobs++
		} else if hb.LastSuccessAt != nil && now.Sub(*hb.LastSuccessAt) > 15*time.Minute {
			failedJobs++
		}
	}
	if totalJobs > 0 && failedJobs > 0 {
		jobScore = (1 - float64(failedJobs)/float64(totalJobs)) * 100
	}

	// Weighted combination
	return storageScore*0.4 + computeScore*0.3 + jobScore*0.3
}

func clampFloat64(v float64, min float64, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
