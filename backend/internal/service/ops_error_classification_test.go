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
			input:   clientInput(403, "Insufficient account balance"),
			wantSub: OpsClientErrorSubcategoryBalance,
		},
		{
			name:    "group disabled",
			input:   OpsErrorClassificationInput{StatusCode: 403, ErrorOwner: "platform", ErrorSource: "gateway", ErrorPhase: "internal", ErrorMessage: "API Key 所属分组已停用"},
			wantSub: OpsClientErrorSubcategoryGroup,
		},
		{
			name:    "subscription",
			input:   clientInput(403, "SUBSCRIPTION_NOT_FOUND no active subscription found for this group"),
			wantSub: OpsClientErrorSubcategorySubscription,
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

func TestClassifyOpsError_DoesNotTreatEvery403AsAPIKey(t *testing.T) {
	tests := []struct {
		name   string
		input  OpsErrorClassificationInput
		want   string
		reason string
	}{
		{
			name:  "insufficient balance 403",
			input: clientInput(403, "Insufficient account balance"),
			want:  OpsClientErrorSubcategoryBalance,
		},
		{
			name:  "group disabled 403",
			input: OpsErrorClassificationInput{StatusCode: 403, ErrorOwner: "platform", ErrorSource: "gateway", ErrorPhase: "internal", ErrorMessage: "API Key 所属分组已停用", ErrorBody: `{"code":"GROUP_DISABLED"}`},
			want:  OpsClientErrorSubcategoryGroup,
		},
		{
			name:  "request body incomplete 400",
			input: clientInput(400, `Request body is incomplete {"diagnostics":{"kind":"incomplete_body","cause":"unexpected EOF"}}`),
			want:  OpsClientErrorSubcategoryDisconnect,
		},
		{
			name:  "true invalid key 401",
			input: OpsErrorClassificationInput{StatusCode: 401, ErrorOwner: "client", ErrorSource: "client_request", ErrorPhase: "auth", ErrorMessage: "INVALID_API_KEY Invalid API key"},
			want:  OpsClientErrorSubcategoryAuth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyOpsError(tt.input)
			if got.ErrorCategory != OpsErrorCategoryClient || got.ErrorSubcategory != tt.want {
				t.Fatalf("classification = %s/%s, want client/%s (reason=%s)", got.ErrorCategory, got.ErrorSubcategory, tt.want, got.ClassificationReason)
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
			name:     "provider no available account stays upstream",
			input:    OpsErrorClassificationInput{StatusCode: 503, UpstreamStatusCode: intPtrForOpsTest(503), ErrorOwner: "provider", ErrorSource: "upstream_http", ErrorPhase: "upstream", UpstreamErrorMessage: "no available accounts"},
			category: OpsErrorCategoryUpstream,
			sub:      "upstream_unavailable",
		},
		{
			name:     "client default model routing failure",
			input:    OpsErrorClassificationInput{StatusCode: 503, ErrorOwner: "platform", ErrorSource: "gateway", ErrorPhase: "routing", ErrorMessage: "Service temporarily unavailable", Model: "codex-current", RequestedModel: "codex-current"},
			category: OpsErrorCategoryClient,
			sub:      OpsClientErrorSubcategoryModel,
		},
		{
			name:     "generic platform routing unavailable",
			input:    OpsErrorClassificationInput{StatusCode: 503, ErrorOwner: "platform", ErrorSource: "gateway", ErrorPhase: "routing", ErrorMessage: "Service temporarily unavailable"},
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
			name:     "platform stream terminal event",
			input:    OpsErrorClassificationInput{StatusCode: 502, ErrorOwner: "platform", ErrorSource: "gateway", ErrorMessage: "Upstream stream ended without a terminal response event"},
			category: OpsErrorCategoryPlatform,
			sub:      "platform_internal_error",
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

func TestClassifyOpsError_OpenAIImageWebSocketHTTPFallback(t *testing.T) {
	got := ClassifyOpsError(OpsErrorClassificationInput{
		StatusCode:   426,
		ErrorType:    "api_error",
		ErrorPhase:   "internal",
		ErrorSource:  "gateway",
		ErrorOwner:   "platform",
		ErrorMessage: "Codex image channels require HTTPS Responses transport",
		ErrorBody:    `{"error":{"type":"websocket_transport_unsupported","message":"Codex image channels require HTTPS Responses transport"}}`,
		RequestPath:  "/openai/v1/responses",
	})

	if got.ErrorCategory != OpsErrorCategoryPlatform {
		t.Fatalf("category = %q, want %q", got.ErrorCategory, OpsErrorCategoryPlatform)
	}
	if got.ErrorSubcategory != "platform_internal_error" {
		t.Fatalf("subcategory = %q, want platform_internal_error", got.ErrorSubcategory)
	}
	if got.ClassificationReason != OpsClassificationReasonOpenAIImageWebSocketHTTPFallback {
		t.Fatalf("reason = %q, want %q", got.ClassificationReason, OpsClassificationReasonOpenAIImageWebSocketHTTPFallback)
	}
}

func TestClassifyOpsError_UsesSpecificClientReason(t *testing.T) {
	tests := []struct {
		name       string
		input      OpsErrorClassificationInput
		wantReason string
	}{
		{name: "balance", input: clientInput(403, "Insufficient account balance"), wantReason: "用户余额不足"},
		{name: "group rpm", input: clientInput(429, "group requests-per-minute limit exceeded"), wantReason: "分组 RPM 限流"},
		{name: "group rpm code", input: clientInput(429, "GROUP_RPM_EXCEEDED"), wantReason: "分组 RPM 限流"},
		{name: "user rpm code", input: clientInput(429, "USER_RPM_EXCEEDED"), wantReason: "用户 RPM 限流"},
		{name: "key rate window", input: clientInput(429, "API key 5小时限额已用完"), wantReason: "API Key 限流"},
		{name: "key quota", input: clientInput(429, "API_KEY_QUOTA_EXHAUSTED API key 额度已用完"), wantReason: "API Key 配额已用完"},
		{name: "generic quota", input: clientInput(429, "quota exhausted"), wantReason: "用户或订阅额度已用完"},
		{name: "key disabled", input: clientInput(401, "API_KEY_DISABLED API key is disabled"), wantReason: "API Key 已禁用"},
		{name: "image permission", input: OpsErrorClassificationInput{StatusCode: 403, ErrorOwner: "platform", ErrorSource: "gateway", ErrorPhase: "internal", ErrorMessage: "Image generation is not enabled for this group"}, wantReason: "当前分组未开启生图权限"},
		{name: "image permission recorded as upstream context", input: OpsErrorClassificationInput{StatusCode: 403, UpstreamStatusCode: intPtrForOpsTest(403), ErrorOwner: "provider", ErrorSource: "upstream_http", ErrorPhase: "upstream", ErrorMessage: "Image generation is not enabled for this group", UpstreamErrorMessage: "Image generation is not enabled for this group"}, wantReason: "当前分组未开启生图权限"},
		{name: "messages permission", input: OpsErrorClassificationInput{StatusCode: 403, ErrorOwner: "platform", ErrorSource: "gateway", ErrorPhase: "internal", ErrorMessage: "This group does not allow /v1/messages dispatch"}, wantReason: "当前分组未开启 /v1/messages 调度"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyOpsError(tt.input)
			if got.ClassificationReason != tt.wantReason {
				t.Fatalf("reason = %q, want %q (classification=%s/%s)", got.ClassificationReason, tt.wantReason, got.ErrorCategory, got.ErrorSubcategory)
			}
		})
	}
}

func TestBuildOpsErrorSummaryIncludesConcreteDetail(t *testing.T) {
	if got := BuildOpsErrorSummary("上游服务不可用或过载", "Upstream request failed", "openai_error", "Cloudflare 524: origin timed out"); got != "上游服务不可用或过载：Cloudflare 524: origin timed out" {
		t.Fatalf("summary = %q", got)
	}
	if got := BuildOpsErrorSummary("API Key 已禁用", "API key is disabled", "", ""); got != "API Key 已禁用：API key is disabled" {
		t.Fatalf("summary = %q", got)
	}
	cases := []struct {
		name     string
		reason   string
		message  string
		upstream string
		detail   string
		want     string
	}{
		{"json wrapper", "上游服务不可用或过载", `{"error":{"message":"Cloudflare 503: Service Unavailable"}}`, "", "", "上游服务不可用或过载：Cloudflare 503: Service Unavailable"},
		{"json generic message with concrete detail", "上游服务不可用或过载", `{"error":{"message":"upstream_error","detail":"Cloudflare 524: origin timed out"}}`, "", "", "上游服务不可用或过载：Cloudflare 524: origin timed out"},
		{"websocket pool", "上游账号池无可用账号", `1013 websocket close: upstream websocket proxy failed: no available account`, "", "", "上游账号池无可用账号：上游账号池暂无可用账号"},
		{"image format", "上游请求参数不兼容", "", `Failed to deserialize the JSON body into the target type: unknown variant 'image_url'`, "", "上游请求参数不兼容：上游不支持当前请求中的 image_url 内容格式"},
		{"generic only", "上游服务不可用或过载", "upstream_error", "openai_error", "", "上游服务不可用或过载"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildOpsErrorSummary(tc.reason, tc.message, tc.upstream, tc.detail); got != tc.want {
				t.Fatalf("summary = %q, want %q", got, tc.want)
			}
		})
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

func intPtrForOpsTest(value int) *int {
	return &value
}
