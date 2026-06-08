package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

type fieldPermissionRole string

const (
	fieldPermissionOwner    fieldPermissionRole = "owner"
	fieldPermissionOps      fieldPermissionRole = "ops"
	fieldPermissionBusiness fieldPermissionRole = "business"
	fieldPermissionSupport  fieldPermissionRole = "support"
	fieldPermissionUnknown  fieldPermissionRole = "unknown"
)

func normalizeFieldPermissionRole(role string) fieldPermissionRole {
	role = strings.TrimSpace(role)
	if role == "" || role == RoleAdmin {
		return fieldPermissionOwner
	}
	switch strings.ToLower(role) {
	case "ops", "operation", "operator", "operations":
		return fieldPermissionOps
	case "business", "business_operator", "business-operator", "yunying", "运营":
		return fieldPermissionBusiness
	case "customer_service", "customer-service", "customerservice", "support", "service", "cs", "客服":
		return fieldPermissionSupport
	default:
		return fieldPermissionUnknown
	}
}

func canViewCacheStats(role string) bool {
	switch normalizeFieldPermissionRole(role) {
	case fieldPermissionOwner, fieldPermissionOps, fieldPermissionBusiness:
		return true
	default:
		return false
	}
}

func canExportCacheStats(role string) bool {
	switch normalizeFieldPermissionRole(role) {
	case fieldPermissionOwner, fieldPermissionBusiness:
		return true
	default:
		return false
	}
}

func canViewRevenueDashboard(role string) bool {
	switch normalizeFieldPermissionRole(role) {
	case fieldPermissionOwner, fieldPermissionBusiness:
		return true
	default:
		return false
	}
}

func viewerRoleFromCacheStatsFilter(filter *CacheStatsFilter) string {
	if filter == nil {
		return ""
	}
	return filter.ViewerRole
}

func (s *DashboardService) GetRevenueOverviewForRole(ctx context.Context, viewerRole string) (*DashboardRevenueOverview, error) {
	if !canViewRevenueDashboard(viewerRole) {
		return &DashboardRevenueOverview{}, nil
	}
	return s.GetRevenueOverview(ctx)
}

func (s *DashboardService) GetRepurchaseDistributionForRole(ctx context.Context, viewerRole string) (*DashboardRepurchaseDistribution, error) {
	if !canViewRevenueDashboard(viewerRole) {
		return &DashboardRepurchaseDistribution{Buckets: []DashboardRepurchaseBucket{}}, nil
	}
	return s.GetRepurchaseDistribution(ctx)
}

func viewerRoleFromOpsUnifiedErrorFilter(filter *OpsUnifiedErrorListFilter) string {
	if filter == nil {
		return ""
	}
	return filter.ViewerRole
}

func applyOpsUnifiedErrorListFieldPolicy(list *OpsUnifiedErrorList, viewerRole string) *OpsUnifiedErrorList {
	if list == nil {
		return nil
	}
	role := normalizeFieldPermissionRole(viewerRole)
	out := &OpsUnifiedErrorList{
		Items:    make([]*OpsUnifiedErrorItem, 0, len(list.Items)),
		Total:    list.Total,
		Page:     list.Page,
		PageSize: list.PageSize,
	}
	for _, item := range list.Items {
		out.Items = append(out.Items, applyOpsUnifiedErrorItemFieldPolicy(item, role))
	}
	return out
}

func applyOpsUnifiedErrorItemFieldPolicy(item *OpsUnifiedErrorItem, role fieldPermissionRole) *OpsUnifiedErrorItem {
	if item == nil {
		return nil
	}
	out := *item
	out.User = applyOpsEntityFieldPolicy(item.User, role, opsEntityUser)
	out.APIKey = applyOpsEntityFieldPolicy(item.APIKey, role, opsEntityAPIKey)
	out.Group = cloneOpsEntityRef(item.Group)
	out.UpstreamAccount = applyOpsEntityFieldPolicy(item.UpstreamAccount, role, opsEntityUpstreamAccount)
	if role == fieldPermissionSupport {
		out.Summary = logredact.RedactText(out.Summary)
	}
	return &out
}

func applyOpsUnifiedErrorDetailFieldPolicy(detail *OpsUnifiedErrorDetail, viewerRole string) *OpsUnifiedErrorDetail {
	if detail == nil {
		return nil
	}
	role := normalizeFieldPermissionRole(viewerRole)
	out := *detail
	out.RequestChain = detail.RequestChain
	out.RequestChain.User = applyOpsEntityFieldPolicy(detail.RequestChain.User, role, opsEntityUser)
	out.RequestChain.APIKey = applyOpsEntityFieldPolicy(detail.RequestChain.APIKey, role, opsEntityAPIKey)
	out.RequestChain.Group = cloneOpsEntityRef(detail.RequestChain.Group)
	out.RequestChain.UpstreamAccount = applyOpsEntityFieldPolicy(detail.RequestChain.UpstreamAccount, role, opsEntityUpstreamAccount)
	if role == fieldPermissionBusiness || role == fieldPermissionSupport {
		out.RequestChain.UpstreamEndpoint = ""
	}
	if role == fieldPermissionSupport {
		out.RequestChain.InboundEndpoint = ""
		out.RawRecord = OpsUnifiedErrorRawRecord{ErrorBodyPreview: logredact.RedactText(detail.RawRecord.ErrorBodyPreview)}
	} else if role == fieldPermissionBusiness {
		out.RawRecord = sanitizeOpsRawRecord(detail.RawRecord)
	}
	out.SameKindErrors = make([]*OpsUnifiedErrorItem, 0, len(detail.SameKindErrors))
	for _, item := range detail.SameKindErrors {
		out.SameKindErrors = append(out.SameKindErrors, applyOpsUnifiedErrorItemFieldPolicy(item, role))
	}
	if role == fieldPermissionSupport {
		out.AIAnalysis.Summary = logredact.RedactText(out.AIAnalysis.Summary)
	}
	return &out
}

type opsEntityKind string

const (
	opsEntityUser            opsEntityKind = "user"
	opsEntityAPIKey          opsEntityKind = "api_key"
	opsEntityUpstreamAccount opsEntityKind = "upstream_account"
)

func applyOpsEntityFieldPolicy(ref *OpsUnifiedEntityRef, role fieldPermissionRole, kind opsEntityKind) *OpsUnifiedEntityRef {
	out := cloneOpsEntityRef(ref)
	if out == nil {
		return nil
	}
	switch role {
	case fieldPermissionOwner, fieldPermissionOps:
		if kind == opsEntityAPIKey && out.Display == "" {
			out.Display = opsUnifiedAPIKeyDisplayFromID(out.ID)
		}
	case fieldPermissionBusiness:
		if kind == opsEntityUser {
			out.Email = maskEmailForExport(out.Email)
			out.Display = ""
		}
		if kind == opsEntityAPIKey {
			out.Name = ""
			out.Display = opsUnifiedAPIKeyDisplayFromID(out.ID)
		}
		if kind == opsEntityUpstreamAccount {
			out.Name = maskUpstreamAccountNameForExport(out.Name)
			out.Display = ""
		}
	case fieldPermissionSupport:
		if kind == opsEntityUpstreamAccount {
			return nil
		}
		out.Name = ""
		out.Email = maskEmailForExport(out.Email)
		if kind == opsEntityAPIKey {
			out.Display = opsUnifiedAPIKeyDisplayFromID(out.ID)
		} else {
			out.Display = ""
		}
	default:
		return nil
	}
	return out
}

func cloneOpsEntityRef(ref *OpsUnifiedEntityRef) *OpsUnifiedEntityRef {
	if ref == nil {
		return nil
	}
	out := *ref
	return &out
}

func opsUnifiedAPIKeyDisplayFromID(id int64) string {
	if id <= 0 {
		return ""
	}
	return "API Key #" + opsStrconvFormatInt(id)
}

func sanitizeOpsRawRecord(raw OpsUnifiedErrorRawRecord) OpsUnifiedErrorRawRecord {
	out := raw
	if out.ErrorLog != nil {
		log := *out.ErrorLog
		log.ErrorBody = logredact.RedactText(log.ErrorBody)
		log.UpstreamErrorDetail = ""
		log.UpstreamErrors = ""
		out.ErrorLog = &log
	}
	out.ErrorBodyPreview = logredact.RedactText(out.ErrorBodyPreview)
	out.UpstreamErrors = ""
	return out
}
