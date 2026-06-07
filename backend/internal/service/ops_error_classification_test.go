package service

import "testing"

func TestClassifyOpsError_ClientSubcategories(t *testing.T) {
	tests := []struct {
		name    string
		input   OpsErrorClassificationInput
		wantSub string
	}{
		{
			name:    "auth",
			input:   clientInput(401, "invalid_api_key: key disabled"),
			wantSub: OpsClientErrorSubcategoryAuth,
		},
		{
			name:    "rate limit",
			input:   clientInput(429, "key rate limit exceeded"),
			wantSub: OpsClientErrorSubcategoryRateLimit,
		},
		{
			name:    "balance",
			input:   clientInput(402, "insufficient balance"),
			wantSub: OpsClientErrorSubcategoryBalance,
		},
		{
			name:    "parameter",
			input:   clientInput(400, "validation error: missing required field model"),
			wantSub: OpsClientErrorSubcategoryParameter,
		},
		{
			name:    "model",
			input:   clientInput(400, "model not found: no mapping for gpt-x"),
			wantSub: OpsClientErrorSubcategoryModel,
		},
		{
			name:    "path",
			input:   clientInput(404, "route not found"),
			wantSub: OpsClientErrorSubcategoryPath,
		},
		{
			name:    "context",
			input:   clientInput(400, "context length exceeds model context window"),
			wantSub: OpsClientErrorSubcategoryContext,
		},
		{
			name:    "disconnect",
			input:   clientInput(499, "context canceled by client"),
			wantSub: OpsClientErrorSubcategoryDisconnect,
		},
		{
			name:    "insufficient evidence",
			input:   clientInput(418, "client request failed"),
			wantSub: OpsClientErrorSubcategoryInsufficientEvidence,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyOpsError(tt.input)
			if got.ErrorCategory != OpsErrorCategoryClient {
				t.Fatalf("category = %q, want client", got.ErrorCategory)
			}
			if got.ErrorSubcategory != tt.wantSub {
				t.Fatalf("subcategory = %q, want %q", got.ErrorSubcategory, tt.wantSub)
			}
			if got.ClientErrorSubcategory != tt.wantSub {
				t.Fatalf("client subcategory = %q, want %q", got.ClientErrorSubcategory, tt.wantSub)
			}
		})
	}
}

func TestClassifyOpsError_MajorCategories(t *testing.T) {
	upstreamStatus429 := 429
	upstreamStatus403 := 403
	upstreamStatus503 := 503
	slow := int64(130000)
	tests := []struct {
		name     string
		input    OpsErrorClassificationInput
		category string
		sub      string
	}{
		{
			name:     "account pool",
			input:    OpsErrorClassificationInput{ErrorOwner: "platform", ErrorSource: "gateway", ErrorPhase: "routing", ErrorMessage: "no available accounts for model"},
			category: OpsErrorCategoryAccountPool,
			sub:      "account_pool_empty",
		},
		{
			name:     "upstream rate limit",
			input:    OpsErrorClassificationInput{StatusCode: 200, UpstreamStatusCode: &upstreamStatus429, ErrorOwner: "provider", ErrorSource: "upstream_http", UpstreamErrorMessage: "rate limit exceeded"},
			category: OpsErrorCategoryRateLimit,
			sub:      "upstream_rate_limit",
		},
		{
			name:     "permission",
			input:    OpsErrorClassificationInput{StatusCode: 200, UpstreamStatusCode: &upstreamStatus403, ErrorOwner: "provider", ErrorSource: "upstream_http", UpstreamErrorMessage: "forbidden"},
			category: OpsErrorCategoryPermission,
			sub:      "upstream_permission_error",
		},
		{
			name:     "balance",
			input:    OpsErrorClassificationInput{ErrorOwner: "provider", ErrorSource: "upstream_http", UpstreamErrorMessage: "insufficient balance"},
			category: OpsErrorCategoryBalance,
			sub:      "upstream_balance_error",
		},
		{
			name:     "config",
			input:    OpsErrorClassificationInput{ErrorOwner: "platform", ErrorSource: "gateway", ErrorMessage: "model mapping config missing"},
			category: OpsErrorCategoryConfig,
			sub:      "config_model_mapping_error",
		},
		{
			name:     "slow request",
			input:    OpsErrorClassificationInput{ErrorOwner: "platform", ErrorSource: "gateway", ResponseLatencyMs: &slow},
			category: OpsErrorCategorySlowRequest,
			sub:      "slow_response",
		},
		{
			name:     "upstream unavailable",
			input:    OpsErrorClassificationInput{StatusCode: 200, UpstreamStatusCode: &upstreamStatus503, ErrorOwner: "provider", ErrorSource: "upstream_http", UpstreamErrorMessage: "service unavailable"},
			category: OpsErrorCategoryUpstream,
			sub:      "upstream_unavailable",
		},
		{
			name:     "platform dependency",
			input:    OpsErrorClassificationInput{ErrorOwner: "platform", ErrorSource: "gateway", ErrorMessage: "redis dependency unavailable"},
			category: OpsErrorCategoryPlatform,
			sub:      "platform_dependency_error",
		},
		{
			name:     "unknown",
			input:    OpsErrorClassificationInput{ErrorMessage: ""},
			category: OpsErrorCategoryUnknown,
			sub:      "unknown_insufficient_evidence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyOpsError(tt.input)
			if got.ErrorCategory != tt.category || got.ErrorSubcategory != tt.sub {
				t.Fatalf("classification = %s/%s, want %s/%s (reason=%s)", got.ErrorCategory, got.ErrorSubcategory, tt.category, tt.sub, got.ClassificationReason)
			}
			if got.ErrorCategory != OpsErrorCategoryClient && got.ClientErrorSubcategory != "" {
				t.Fatalf("non-client category should not set client subcategory: %#v", got)
			}
		})
	}
}

func TestSetOpsErrorClassificationUsesNilClientSubcategoryForNonClient(t *testing.T) {
	log := &OpsErrorLog{}
	log.SetClassification(OpsErrorClassification{ErrorCategory: OpsErrorCategoryUpstream, ErrorSubcategory: "upstream_timeout"})
	if log.ClientErrorSubcategory != nil {
		t.Fatalf("client subcategory should be nil for non-client classification")
	}

	log.SetClassification(clientClassification(OpsClientErrorSubcategoryParameter, "参数错误", OpsClassificationConfidenceHigh))
	if log.ClientErrorSubcategory == nil || *log.ClientErrorSubcategory != OpsClientErrorSubcategoryParameter {
		t.Fatalf("client subcategory not applied: %#v", log.ClientErrorSubcategory)
	}
}

func clientInput(status int, message string) OpsErrorClassificationInput {
	return OpsErrorClassificationInput{
		StatusCode:   status,
		ErrorOwner:   "client",
		ErrorSource:  "client_request",
		ErrorPhase:   "request",
		ErrorMessage: message,
	}
}
