package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildOpsErrorLogsWhere_QueryUsesQualifiedColumns(t *testing.T) {
	filter := &service.OpsErrorLogFilter{
		Query: "ACCESS_DENIED",
	}

	where, args := buildOpsErrorLogsWhere(filter)
	if where == "" {
		t.Fatalf("where should not be empty")
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d, want 1", len(args))
	}
	if !strings.Contains(where, "e.request_id ILIKE $") {
		t.Fatalf("where should include qualified request_id condition: %s", where)
	}
	if !strings.Contains(where, "e.client_request_id ILIKE $") {
		t.Fatalf("where should include qualified client_request_id condition: %s", where)
	}
	if !strings.Contains(where, "e.error_message ILIKE $") {
		t.Fatalf("where should include qualified error_message condition: %s", where)
	}
}

func TestBuildOpsErrorLogsWhere_UserQueryUsesExistsSubquery(t *testing.T) {
	filter := &service.OpsErrorLogFilter{
		UserQuery: "admin@",
	}

	where, args := buildOpsErrorLogsWhere(filter)
	if where == "" {
		t.Fatalf("where should not be empty")
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d, want 1", len(args))
	}
	if !strings.Contains(where, "EXISTS (SELECT 1 FROM users u WHERE u.id = e.user_id AND u.email ILIKE $") {
		t.Fatalf("where should include EXISTS user email condition: %s", where)
	}
}

func TestBuildOpsErrorLogsWhere_SemanticFiltersDoNotAddGenericClientStatus(t *testing.T) {
	where, _ := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{Category: "upstream_error", View: "all"})
	if strings.Contains(where, "COALESCE(e.status_code, 0) >= 400") {
		t.Fatalf("category drill-down should not add generic client status filter: %s", where)
	}
	if !strings.Contains(where, "error_owner = 'provider'") {
		t.Fatalf("upstream category condition missing: %s", where)
	}

	impact := true
	where, _ = buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{ImpactPlatformSLA: &impact, View: "all"})
	if strings.Contains(where, "COALESCE(e.status_code, 0) >= 400 AND COALESCE(status_code, 0) >= 400") {
		t.Fatalf("SLA drill-down should not duplicate generic client status filter: %s", where)
	}
	if !strings.Contains(where, "COALESCE(status_code, 0) >= 400") {
		t.Fatalf("SLA condition should still enforce client-visible status: %s", where)
	}
}
