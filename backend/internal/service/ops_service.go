package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrOpsDisabled = infraerrors.NotFound("OPS_DISABLED", "Ops monitoring is disabled")

const (
	opsMaxStoredErrorBodyBytes = 20 * 1024
)

// OpsService provides ingestion and query APIs for the Ops monitoring module.
type OpsService struct {
	opsRepo     OpsRepository
	settingRepo SettingRepository
	cfg         *config.Config

	accountRepo AccountRepository
	userRepo    UserRepository

	// getAccountAvailability is a unit-test hook for overriding account availability lookup.
	getAccountAvailability func(ctx context.Context, platformFilter string, groupIDFilter *int64) (*OpsAccountAvailability, error)

	concurrencyService        *ConcurrencyService
	gatewayService            *GatewayService
	openAIGatewayService      *OpenAIGatewayService
	geminiCompatService       *GeminiMessagesCompatService
	antigravityGatewayService *AntigravityGatewayService
	systemLogSink             *OpsSystemLogSink
	secretEncryptor           SecretEncryptor
	aiAnalysisHTTPClient      *http.Client

	aiWorkerStartOnce      sync.Once
	aiWorkerStopOnce       sync.Once
	aiWorkerRunning        int32
	aiWorkerCtx            context.Context
	aiWorkerCancel         context.CancelFunc
	aiExecutorMu           sync.Mutex
	aiAnalysisTaskExecutor OpsAIAnalysisTaskExecutor

	// cleanupReloader 由 wire 在 OpsCleanupService 构造完成后通过 SetCleanupReloader 注入。
	// 解耦避免 OpsService -> OpsCleanupService 的硬依赖（cleanup 也读 settings，会循环）。
	cleanupReloader CleanupReloader
}

// CleanupReloader 由 OpsCleanupService 实现。
// UpdateOpsAdvancedSettings 写入新配置后调用 Reload，让 schedule/enabled 改动立刻生效。
type CleanupReloader interface {
	Reload(ctx context.Context) error
}

// SetCleanupReloader 由 wire 注入 cleanup hook（构造期循环依赖的解耦点）。
func (s *OpsService) SetCleanupReloader(r CleanupReloader) {
	if s == nil {
		return
	}
	s.cleanupReloader = r
}

func (s *OpsService) SetAIAnalysisHTTPClient(client *http.Client) {
	if s == nil {
		return
	}
	s.aiAnalysisHTTPClient = client
}

func (s *OpsService) SetSecretEncryptor(encryptor SecretEncryptor) {
	if s == nil {
		return
	}
	s.secretEncryptor = encryptor
}

func NewOpsService(
	opsRepo OpsRepository,
	settingRepo SettingRepository,
	cfg *config.Config,
	accountRepo AccountRepository,
	userRepo UserRepository,
	concurrencyService *ConcurrencyService,
	gatewayService *GatewayService,
	openAIGatewayService *OpenAIGatewayService,
	geminiCompatService *GeminiMessagesCompatService,
	antigravityGatewayService *AntigravityGatewayService,
	systemLogSink *OpsSystemLogSink,
) *OpsService {
	svc := &OpsService{
		opsRepo:     opsRepo,
		settingRepo: settingRepo,
		cfg:         cfg,

		accountRepo: accountRepo,
		userRepo:    userRepo,

		concurrencyService:        concurrencyService,
		gatewayService:            gatewayService,
		openAIGatewayService:      openAIGatewayService,
		geminiCompatService:       geminiCompatService,
		antigravityGatewayService: antigravityGatewayService,
		systemLogSink:             systemLogSink,
	}
	svc.applyRuntimeLogConfigOnStartup(context.Background())
	return svc
}

func (s *OpsService) RequireMonitoringEnabled(ctx context.Context) error {
	if s.IsMonitoringEnabled(ctx) {
		return nil
	}
	return ErrOpsDisabled
}

func (s *OpsService) IsMonitoringEnabled(ctx context.Context) bool {
	// Hard switch: disable ops entirely.
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return false
	}
	if s.settingRepo == nil {
		return true
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyOpsMonitoringEnabled)
	if err != nil {
		// Default enabled when key is missing, and fail-open on transient errors
		// (ops should never block gateway traffic).
		if errors.Is(err, ErrSettingNotFound) {
			return true
		}
		return true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "disabled":
		return false
	default:
		return true
	}
}

func (s *OpsService) RecordError(ctx context.Context, entry *OpsInsertErrorLogInput) error {
	prepared, ok, err := s.prepareErrorLogInput(ctx, entry)
	if err != nil {
		log.Printf("[Ops] RecordError prepare failed: %v", err)
		return err
	}
	if !ok {
		return nil
	}

	if _, err := s.opsRepo.InsertErrorLog(ctx, prepared); err != nil {
		// Never bubble up to gateway; best-effort logging.
		log.Printf("[Ops] RecordError failed: %v", err)
		return err
	}
	return nil
}

func (s *OpsService) RecordErrorBatch(ctx context.Context, entries []*OpsInsertErrorLogInput) error {
	if len(entries) == 0 {
		return nil
	}
	prepared := make([]*OpsInsertErrorLogInput, 0, len(entries))
	for _, entry := range entries {
		item, ok, err := s.prepareErrorLogInput(ctx, entry)
		if err != nil {
			log.Printf("[Ops] RecordErrorBatch prepare failed: %v", err)
			continue
		}
		if ok {
			prepared = append(prepared, item)
		}
	}
	if len(prepared) == 0 {
		return nil
	}
	if len(prepared) == 1 {
		_, err := s.opsRepo.InsertErrorLog(ctx, prepared[0])
		if err != nil {
			log.Printf("[Ops] RecordErrorBatch single insert failed: %v", err)
		}
		return err
	}

	if _, err := s.opsRepo.BatchInsertErrorLogs(ctx, prepared); err != nil {
		log.Printf("[Ops] RecordErrorBatch failed, fallback to single inserts: %v", err)
		var firstErr error
		for _, entry := range prepared {
			if _, insertErr := s.opsRepo.InsertErrorLog(ctx, entry); insertErr != nil {
				log.Printf("[Ops] RecordErrorBatch fallback insert failed: %v", insertErr)
				if firstErr == nil {
					firstErr = insertErr
				}
			}
		}
		return firstErr
	}
	return nil
}

func (s *OpsService) prepareErrorLogInput(ctx context.Context, entry *OpsInsertErrorLogInput) (*OpsInsertErrorLogInput, bool, error) {
	if entry == nil {
		return nil, false, nil
	}
	if !s.IsMonitoringEnabled(ctx) {
		return nil, false, nil
	}
	if s.opsRepo == nil {
		return nil, false, nil
	}

	// Ensure timestamps are always populated.
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	// Ensure required fields exist (DB has NOT NULL constraints).
	entry.ErrorPhase = strings.TrimSpace(entry.ErrorPhase)
	entry.ErrorType = strings.TrimSpace(entry.ErrorType)
	if entry.ErrorPhase == "" {
		entry.ErrorPhase = "internal"
	}
	if entry.ErrorType == "" {
		entry.ErrorType = "api_error"
	}

	// Sanitize + truncate error_body to avoid storing sensitive data.
	if strings.TrimSpace(entry.ErrorBody) != "" {
		sanitized, _ := sanitizeErrorBodyForStorage(entry.ErrorBody, opsMaxStoredErrorBodyBytes)
		entry.ErrorBody = sanitized
	}

	// Sanitize upstream error context if provided by gateway services.
	if entry.UpstreamStatusCode != nil && *entry.UpstreamStatusCode <= 0 {
		entry.UpstreamStatusCode = nil
	}
	if entry.UpstreamErrorMessage != nil {
		msg := strings.TrimSpace(*entry.UpstreamErrorMessage)
		msg = sanitizeUpstreamErrorMessage(msg)
		msg = truncateString(msg, 2048)
		if strings.TrimSpace(msg) == "" {
			entry.UpstreamErrorMessage = nil
		} else {
			entry.UpstreamErrorMessage = &msg
		}
	}
	if entry.UpstreamErrorDetail != nil {
		detail := strings.TrimSpace(*entry.UpstreamErrorDetail)
		if detail == "" {
			entry.UpstreamErrorDetail = nil
		} else {
			sanitized, _ := sanitizeErrorBodyForStorage(detail, opsMaxStoredErrorBodyBytes)
			if strings.TrimSpace(sanitized) == "" {
				entry.UpstreamErrorDetail = nil
			} else {
				entry.UpstreamErrorDetail = &sanitized
			}
		}
	}

	if err := sanitizeOpsUpstreamErrors(entry); err != nil {
		return nil, false, err
	}

	return entry, true, nil
}

func sanitizeOpsUpstreamErrors(entry *OpsInsertErrorLogInput) error {
	if entry == nil || len(entry.UpstreamErrors) == 0 {
		return nil
	}

	const maxEvents = 32
	events := entry.UpstreamErrors
	if len(events) > maxEvents {
		events = events[len(events)-maxEvents:]
	}

	sanitized := make([]*OpsUpstreamErrorEvent, 0, len(events))
	for _, ev := range events {
		if ev == nil {
			continue
		}
		out := *ev

		out.Platform = strings.TrimSpace(out.Platform)
		out.UpstreamRequestID = truncateString(strings.TrimSpace(out.UpstreamRequestID), 128)
		out.Kind = truncateString(strings.TrimSpace(out.Kind), 64)

		if out.AccountID < 0 {
			out.AccountID = 0
		}
		if out.UpstreamStatusCode < 0 {
			out.UpstreamStatusCode = 0
		}
		if out.AtUnixMs < 0 {
			out.AtUnixMs = 0
		}

		msg := sanitizeUpstreamErrorMessage(strings.TrimSpace(out.Message))
		msg = truncateString(msg, 2048)
		out.Message = msg

		detail := strings.TrimSpace(out.Detail)
		if detail != "" {
			// Keep upstream detail small; request bodies are not stored here, only upstream error payloads.
			sanitizedDetail, _ := sanitizeErrorBodyForStorage(detail, opsMaxStoredErrorBodyBytes)
			out.Detail = sanitizedDetail
		} else {
			out.Detail = ""
		}

		// Drop fully-empty events (can happen if only status code was known).
		if out.UpstreamStatusCode == 0 && out.Message == "" && out.Detail == "" {
			continue
		}

		evCopy := out
		sanitized = append(sanitized, &evCopy)
	}

	entry.UpstreamErrorsJSON = marshalOpsUpstreamErrors(sanitized)
	entry.UpstreamErrors = nil
	return nil
}

func (s *OpsService) GetErrorLogs(ctx context.Context, filter *OpsErrorLogFilter) (*OpsErrorLogList, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return &OpsErrorLogList{Errors: []*OpsErrorLog{}, Total: 0, Page: 1, PageSize: 20}, nil
	}
	result, err := s.opsRepo.ListErrorLogs(ctx, filter)
	if err != nil {
		log.Printf("[Ops] GetErrorLogs failed: %v", err)
		return nil, err
	}

	return result, nil
}

func (s *OpsService) GetUnifiedErrors(ctx context.Context, filter *OpsUnifiedErrorListFilter) (*OpsUnifiedErrorList, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return &OpsUnifiedErrorList{Items: []*OpsUnifiedErrorItem{}, Total: 0, Page: 1, PageSize: 20}, nil
	}
	result, err := s.opsRepo.ListUnifiedErrors(ctx, filter)
	if err != nil {
		log.Printf("[Ops] GetUnifiedErrors failed: %v", err)
		return nil, err
	}
	return result, nil
}

func (s *OpsService) GetErrorLogByID(ctx context.Context, id int64) (*OpsErrorLogDetail, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
	}
	detail, err := s.opsRepo.GetErrorLogByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
		}
		return nil, infraerrors.InternalServer("OPS_ERROR_LOAD_FAILED", "Failed to load ops error log").WithCause(err)
	}
	return detail, nil
}

func (s *OpsService) ExportUnifiedErrors(ctx context.Context, filter *OpsUnifiedErrorListFilter, maxRows int) (*OpsUnifiedErrorExportResult, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if maxRows <= 0 || maxRows > 100000 {
		maxRows = 100000
	}
	if filter == nil {
		filter = &OpsUnifiedErrorListFilter{}
	}
	exportFilter := *filter
	exportFilter.Page = 1
	exportFilter.PageSize = 100
	exportFilter.SortBy = strings.TrimSpace(exportFilter.SortBy)
	if exportFilter.SortBy == "" {
		exportFilter.SortBy = "occurred_at"
	}
	exportFilter.SortOrder = strings.TrimSpace(exportFilter.SortOrder)
	if exportFilter.SortOrder == "" {
		exportFilter.SortOrder = "desc"
	}

	rows := [][]string{opsUnifiedErrorCSVHeader()}
	exported := 0
	total := 0
	for {
		page, err := s.GetUnifiedErrors(ctx, &exportFilter)
		if err != nil {
			return nil, err
		}
		if page == nil {
			break
		}
		if page.Total > total {
			total = page.Total
		}
		if page.Total > maxRows {
			return &OpsUnifiedErrorExportResult{Rows: rows, Total: page.Total, Truncated: true}, nil
		}
		if len(page.Items) == 0 {
			break
		}
		for _, item := range page.Items {
			if item == nil {
				continue
			}
			if exported >= maxRows {
				return &OpsUnifiedErrorExportResult{Rows: rows, Total: total, Truncated: true}, nil
			}
			rows = append(rows, opsUnifiedErrorCSVRow(item))
			exported++
		}
		if exported >= page.Total || len(page.Items) < exportFilter.PageSize {
			break
		}
		exportFilter.Page++
	}
	return &OpsUnifiedErrorExportResult{Rows: rows, Total: total, Truncated: total > maxRows}, nil
}

func (s *OpsService) GetUnifiedErrorDetail(ctx context.Context, id int64) (*OpsUnifiedErrorDetail, error) {
	detail, err := s.GetErrorLogByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if detail.ClientStatusCode == 0 {
		detail.ClientStatusCode = detail.StatusCode
	}
	rawDetail := *detail
	rawDetail.ErrorBody = truncateRunes(detail.ErrorBody, 500)
	rawDetail.UpstreamErrorDetail = truncateRunes(detail.UpstreamErrorDetail, 500)
	rawDetail.UpstreamErrors = truncateRunes(detail.UpstreamErrors, 500)

	result := opsUnifiedErrorResultFromDetail(detail)
	classification := OpsUnifiedErrorClassification{
		ErrorCategory:            detail.ErrorCategory,
		ErrorSubcategory:         detail.ErrorSubcategory,
		ClientErrorSubcategory:   detail.ClientErrorSubcategory,
		ClassificationConfidence: detail.ClassificationConfidence,
		ClassificationReason:     detail.ClassificationReason,
		MissingEvidence:          detail.ClassificationMissingEvidence,
		StatusCode:               detail.StatusCode,
		ClientStatusCode:         detail.ClientStatusCode,
		ErrorSource:              detail.Source,
		ErrorOwner:               detail.Owner,
	}

	start := detail.CreatedAt.Add(-30 * time.Minute)
	end := detail.CreatedAt.Add(30 * time.Minute)
	sameKindFilter := &OpsUnifiedErrorListFilter{
		StartTime:          &start,
		EndTime:            &end,
		ErrorCategories:    []string{detail.ErrorCategory},
		ErrorSubcategories: []string{detail.ErrorSubcategory},
		StatusCodes:        []int{detail.StatusCode},
		ErrorResults:       []string{result},
		Platform:           detail.Platform,
		Model:              detail.Model,
		SortBy:             "occurred_at",
		SortOrder:          "desc",
		Page:               1,
		PageSize:           20,
	}
	if detail.ClientErrorSubcategory != nil && *detail.ClientErrorSubcategory != "" {
		sameKindFilter.ClientErrorSubcategories = []string{*detail.ClientErrorSubcategory}
	}
	sameKind, _ := s.GetUnifiedErrors(ctx, sameKindFilter)
	var sameKindItems []*OpsUnifiedErrorItem
	impact := OpsUnifiedErrorImpactScope{SameKindCount: 1}
	aiStatus := OpsUnifiedAIAnalysisNotAnalyzed
	if sameKind != nil {
		sameKindItems = sameKind.Items
		if sameKind.Total > 0 {
			impact.SameKindCount = sameKind.Total
		}
		impact.AffectedUsers, impact.AffectedAPIKeys, impact.AffectedGroups, impact.AffectedModels, impact.AffectedUpstreamAccounts = opsUnifiedImpactCounts(sameKind.Items)
		for _, item := range sameKind.Items {
			if item != nil && item.ID == detail.ID && strings.TrimSpace(item.AIAnalysisStatus) != "" {
				aiStatus = item.AIAnalysisStatus
				break
			}
		}
	}
	if detail.UserID != nil && impact.AffectedUsers == 0 {
		impact.AffectedUsers = 1
	}
	if detail.APIKeyID != nil && impact.AffectedAPIKeys == 0 {
		impact.AffectedAPIKeys = 1
	}
	if detail.GroupID != nil && impact.AffectedGroups == 0 {
		impact.AffectedGroups = 1
	}
	if detail.Model != "" && impact.AffectedModels == 0 {
		impact.AffectedModels = 1
	}
	if detail.AccountID != nil && impact.AffectedUpstreamAccounts == 0 {
		impact.AffectedUpstreamAccounts = 1
	}

	return &OpsUnifiedErrorDetail{
		Conclusion: OpsUnifiedErrorConclusion{
			Title:              opsUnifiedDetailTitle(classification, result),
			Summary:            opsUnifiedDetailSummary(classification, detail),
			ErrorResult:        result,
			FinalFailed:        result == OpsUnifiedErrorResultFinalFailed,
			Recovered:          result == OpsUnifiedErrorResultRecovered,
			AffectsUser:        detail.UserID != nil || detail.APIKeyID != nil,
			RecommendedActions: opsUnifiedRecommendedActions(classification.ErrorCategory, classification.ErrorSubcategory),
		},
		RequestChain: OpsUnifiedErrorRequestChain{
			User:             opsUnifiedUserRef(detail.UserID, detail.UserEmail),
			APIKey:           opsUnifiedNameRef(detail.APIKeyID, "", opsUnifiedAPIKeyDisplayFromPtr(detail.APIKeyID)),
			Group:            opsUnifiedNameRef(detail.GroupID, detail.GroupName, ""),
			Platform:         detail.Platform,
			Model:            detail.Model,
			RequestedModel:   detail.RequestedModel,
			UpstreamModel:    detail.UpstreamModel,
			RequestPath:      detail.RequestPath,
			InboundEndpoint:  detail.InboundEndpoint,
			UpstreamEndpoint: detail.UpstreamEndpoint,
			UpstreamAccount:  opsUnifiedNameRef(detail.AccountID, detail.AccountName, ""),
			RequestID:        detail.RequestID,
			ClientRequestID:  detail.ClientRequestID,
		},
		Classification: classification,
		ImpactScope:    impact,
		Recovery: OpsUnifiedErrorRecovery{
			FinalFailed:    result == OpsUnifiedErrorResultFinalFailed,
			Recovered:      result == OpsUnifiedErrorResultRecovered,
			RecoveryMethod: opsUnifiedRecoveryMethod(detail, result),
			Resolved:       detail.Resolved,
			ResolvedAt:     detail.ResolvedAt,
		},
		AIAnalysis: OpsUnifiedErrorAIAnalysis{Status: aiStatus},
		RawRecord: OpsUnifiedErrorRawRecord{
			ErrorLog:         &rawDetail,
			ErrorBodyPreview: rawDetail.ErrorBody,
			UpstreamErrors:   rawDetail.UpstreamErrors,
		},
		SameKindErrors: sameKindItems,
	}, nil
}

func (s *OpsService) UpdateErrorResolution(ctx context.Context, errorID int64, resolved bool, resolvedByUserID *int64) error {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
	}
	if s.opsRepo == nil {
		return infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if errorID <= 0 {
		return infraerrors.BadRequest("OPS_ERROR_INVALID_ID", "invalid error id")
	}
	// Best-effort ensure the error exists
	if _, err := s.opsRepo.GetErrorLogByID(ctx, errorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.NotFound("OPS_ERROR_NOT_FOUND", "ops error log not found")
		}
		return infraerrors.InternalServer("OPS_ERROR_LOAD_FAILED", "Failed to load ops error log").WithCause(err)
	}
	return s.opsRepo.UpdateErrorResolution(ctx, errorID, resolved, resolvedByUserID, nil)
}

func sanitizeAndTrimJSONPayload(raw []byte, maxBytes int) (jsonString string, truncated bool, bytesLen int) {
	bytesLen = len(raw)
	if len(raw) == 0 {
		return "", false, 0
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		// If it is not valid JSON, fall back to the caller's non-JSON handling.
		return "", false, bytesLen
	}

	decoded = redactSensitiveJSON(decoded)

	encoded, err := json.Marshal(decoded)
	if err != nil {
		return "", false, bytesLen
	}
	if len(encoded) <= maxBytes {
		return string(encoded), false, bytesLen
	}

	// Trim conversation history to keep the most recent context.
	if root, ok := decoded.(map[string]any); ok {
		if trimmed, ok := trimConversationArrays(root, maxBytes); ok {
			encoded2, err2 := json.Marshal(trimmed)
			if err2 == nil && len(encoded2) <= maxBytes {
				return string(encoded2), true, bytesLen
			}
			// Fallthrough: keep shrinking.
			decoded = trimmed
		}

		essential := shrinkToEssentials(root)
		encoded3, err3 := json.Marshal(essential)
		if err3 == nil && len(encoded3) <= maxBytes {
			return string(encoded3), true, bytesLen
		}
	}

	// Last resort: keep JSON shape but drop big fields.
	// This avoids downstream code that expects certain top-level keys from crashing.
	if root, ok := decoded.(map[string]any); ok {
		placeholder := shallowCopyMap(root)
		placeholder["payload_truncated"] = true

		// Replace potentially huge arrays/strings, but keep the keys present.
		for _, k := range []string{"messages", "contents", "input", "prompt"} {
			if _, exists := placeholder[k]; exists {
				placeholder[k] = []any{}
			}
		}
		for _, k := range []string{"text"} {
			if _, exists := placeholder[k]; exists {
				placeholder[k] = ""
			}
		}

		encoded4, err4 := json.Marshal(placeholder)
		if err4 == nil {
			if len(encoded4) <= maxBytes {
				return string(encoded4), true, bytesLen
			}
		}
	}

	// Final fallback: minimal valid JSON.
	encoded4, err4 := json.Marshal(map[string]any{"payload_truncated": true})
	if err4 != nil {
		return "", true, bytesLen
	}
	return string(encoded4), true, bytesLen
}

func redactSensitiveJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			if isSensitiveKey(k) {
				out[k] = "[REDACTED]"
				continue
			}
			out[k] = redactSensitiveJSON(vv)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, vv := range t {
			out = append(out, redactSensitiveJSON(vv))
		}
		return out
	default:
		return v
	}
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}

	// Token 计数 / 预算字段不是凭据，应保留用于排错。
	// 白名单保持尽量窄，避免误把真实敏感信息"反脱敏"。
	switch k {
	case "max_tokens",
		"max_output_tokens",
		"max_input_tokens",
		"max_completion_tokens",
		"max_tokens_to_sample",
		"budget_tokens",
		"prompt_tokens",
		"completion_tokens",
		"input_tokens",
		"output_tokens",
		"total_tokens",
		"token_count",
		"cache_creation_input_tokens",
		"cache_read_input_tokens":
		return false
	}

	// Exact matches (common credential fields).
	switch k {
	case "authorization",
		"proxy-authorization",
		"x-api-key",
		"api_key",
		"apikey",
		"access_token",
		"refresh_token",
		"id_token",
		"session_token",
		"token",
		"password",
		"passwd",
		"passphrase",
		"secret",
		"client_secret",
		"private_key",
		"jwt",
		"signature",
		"accesskeyid",
		"secretaccesskey":
		return true
	}

	// Suffix matches.
	for _, suffix := range []string{
		"_secret",
		"_token",
		"_id_token",
		"_session_token",
		"_password",
		"_passwd",
		"_passphrase",
		"_key",
		"secret_key",
		"private_key",
	} {
		if strings.HasSuffix(k, suffix) {
			return true
		}
	}

	// Substring matches (conservative, but errs on the side of privacy).
	for _, sub := range []string{
		"secret",
		"token",
		"password",
		"passwd",
		"passphrase",
		"privatekey",
		"private_key",
		"apikey",
		"api_key",
		"accesskeyid",
		"secretaccesskey",
		"bearer",
		"cookie",
		"credential",
		"session",
		"jwt",
		"signature",
	} {
		if strings.Contains(k, sub) {
			return true
		}
	}

	return false
}

func trimConversationArrays(root map[string]any, maxBytes int) (map[string]any, bool) {
	// Supported: anthropic/openai: messages; gemini: contents.
	if out, ok := trimArrayField(root, "messages", maxBytes); ok {
		return out, true
	}
	if out, ok := trimArrayField(root, "contents", maxBytes); ok {
		return out, true
	}
	return root, false
}

func trimArrayField(root map[string]any, field string, maxBytes int) (map[string]any, bool) {
	raw, ok := root[field]
	if !ok {
		return nil, false
	}
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil, false
	}

	// Keep at least the last message/content. Use binary search so we don't marshal O(n) times.
	// We are dropping from the *front* of the array (oldest context first).
	lo := 0
	hi := len(arr) - 1 // inclusive; hi ensures at least one item remains

	var best map[string]any
	found := false

	for lo <= hi {
		mid := (lo + hi) / 2
		candidateArr := arr[mid:]
		if len(candidateArr) == 0 {
			lo = mid + 1
			continue
		}

		next := shallowCopyMap(root)
		next[field] = candidateArr
		encoded, err := json.Marshal(next)
		if err != nil {
			// If marshal fails, try dropping more.
			lo = mid + 1
			continue
		}

		if len(encoded) <= maxBytes {
			best = next
			found = true
			// Try to keep more context by dropping fewer items.
			hi = mid - 1
			continue
		}

		// Need to drop more.
		lo = mid + 1
	}

	if found {
		return best, true
	}

	// Nothing fit (even with only one element); return the smallest slice and let the
	// caller fall back to shrinkToEssentials().
	next := shallowCopyMap(root)
	next[field] = arr[len(arr)-1:]
	return next, true
}

func shrinkToEssentials(root map[string]any) map[string]any {
	out := make(map[string]any)
	for _, key := range []string{
		"model",
		"stream",
		"max_tokens",
		"max_output_tokens",
		"max_input_tokens",
		"max_completion_tokens",
		"thinking",
		"temperature",
		"top_p",
		"top_k",
	} {
		if v, ok := root[key]; ok {
			out[key] = v
		}
	}

	// Keep only the last element of the conversation array.
	if v, ok := root["messages"]; ok {
		if arr, ok := v.([]any); ok && len(arr) > 0 {
			out["messages"] = []any{arr[len(arr)-1]}
		}
	}
	if v, ok := root["contents"]; ok {
		if arr, ok := v.([]any); ok && len(arr) > 0 {
			out["contents"] = []any{arr[len(arr)-1]}
		}
	}
	return out
}

func shallowCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func sanitizeErrorBodyForStorage(raw string, maxBytes int) (sanitized string, truncated bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	// Prefer JSON-safe sanitization when possible.
	if out, trunc, _ := sanitizeAndTrimJSONPayload([]byte(raw), maxBytes); out != "" {
		return out, trunc
	}

	// Non-JSON: best-effort truncate.
	if maxBytes > 0 && len(raw) > maxBytes {
		return truncateString(raw, maxBytes), true
	}
	return raw, false
}

func opsUnifiedErrorResultFromDetail(detail *OpsErrorLogDetail) string {
	if detail == nil {
		return OpsUnifiedErrorResultUnknown
	}
	clientStatus := detail.ClientStatusCode
	if clientStatus == 0 {
		clientStatus = detail.StatusCode
	}
	if detail.ClientErrorSubcategory != nil && *detail.ClientErrorSubcategory == OpsClientErrorSubcategoryDisconnect {
		return OpsUnifiedErrorResultClientAborted
	}
	if clientStatus >= 400 {
		return OpsUnifiedErrorResultFinalFailed
	}
	if clientStatus > 0 && clientStatus < 400 {
		return OpsUnifiedErrorResultRecovered
	}
	return OpsUnifiedErrorResultUnknown
}

func opsUnifiedImpactCounts(items []*OpsUnifiedErrorItem) (users int, apiKeys int, groups int, models int, upstreamAccounts int) {
	userSet := map[int64]struct{}{}
	apiKeySet := map[int64]struct{}{}
	groupSet := map[int64]struct{}{}
	modelSet := map[string]struct{}{}
	accountSet := map[int64]struct{}{}
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.User != nil && item.User.ID > 0 {
			userSet[item.User.ID] = struct{}{}
		}
		if item.APIKey != nil && item.APIKey.ID > 0 {
			apiKeySet[item.APIKey.ID] = struct{}{}
		}
		if item.Group != nil && item.Group.ID > 0 {
			groupSet[item.Group.ID] = struct{}{}
		}
		if strings.TrimSpace(item.Model) != "" {
			modelSet[item.Model] = struct{}{}
		}
		if item.UpstreamAccount != nil && item.UpstreamAccount.ID > 0 {
			accountSet[item.UpstreamAccount.ID] = struct{}{}
		}
	}
	return len(userSet), len(apiKeySet), len(groupSet), len(modelSet), len(accountSet)
}

func opsUnifiedUserRef(id *int64, email string) *OpsUnifiedEntityRef {
	if id == nil || *id <= 0 {
		return nil
	}
	return &OpsUnifiedEntityRef{ID: *id, Email: email}
}

func opsUnifiedNameRef(id *int64, name string, display string) *OpsUnifiedEntityRef {
	if id == nil || *id <= 0 {
		return nil
	}
	return &OpsUnifiedEntityRef{ID: *id, Name: name, Display: display}
}

func opsUnifiedAPIKeyDisplayFromPtr(id *int64) string {
	if id == nil || *id <= 0 {
		return ""
	}
	return "API Key #" + opsStrconvFormatInt(*id)
}

func opsStrconvFormatInt(v int64) string {
	// Keep service layer free of fmt formatting for hot-path detail assembly.
	if v == 0 {
		return "0"
	}
	negative := v < 0
	if negative {
		v = -v
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func opsUnifiedDetailTitle(classification OpsUnifiedErrorClassification, result string) string {
	prefix := "错误"
	switch result {
	case OpsUnifiedErrorResultRecovered:
		prefix = "已恢复波动"
	case OpsUnifiedErrorResultFinalFailed:
		prefix = "最终失败"
	case OpsUnifiedErrorResultClientAborted:
		prefix = "客户端中断"
	}
	if classification.ErrorSubcategory != "" {
		return prefix + "：" + classification.ErrorSubcategory
	}
	return prefix
}

func opsUnifiedDetailSummary(classification OpsUnifiedErrorClassification, detail *OpsErrorLogDetail) string {
	for _, candidate := range []string{classification.ClassificationReason, detail.Message, detail.UpstreamErrorMessage} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return truncateRunes(candidate, 160)
		}
	}
	return "暂无摘要"
}

func opsUnifiedRecoveryMethod(detail *OpsErrorLogDetail, result string) string {
	if result != OpsUnifiedErrorResultRecovered {
		return "none"
	}
	if detail != nil && detail.AccountID != nil && strings.TrimSpace(detail.UpstreamErrors) != "" {
		return "account_switch"
	}
	if detail != nil && strings.Contains(strings.ToLower(detail.UpstreamErrors), "retry") {
		return "retry"
	}
	return "recovered"
}

func opsUnifiedRecommendedActions(category string, subcategory string) []string {
	switch category {
	case OpsErrorCategoryAccountPool:
		return []string{"检查上游账号可用性", "补充可用账号或调整分组路由"}
	case OpsErrorCategoryPermission:
		return []string{"检查 API Key、上游账号和模型权限", "确认订阅或模型访问范围"}
	case OpsErrorCategoryBalance:
		return []string{"检查用户余额、Key 配额和上游额度", "补充余额或切换可用上游账号"}
	case OpsErrorCategoryRateLimit:
		return []string{"检查 RPM/TPM/并发限制", "降低请求速率或扩容上游账号"}
	case OpsErrorCategoryClient:
		return []string{"检查客户端请求参数、路径、模型和鉴权配置"}
	case OpsErrorCategoryConfig:
		return []string{"检查模型映射、渠道配置、分组配置和缓存配置"}
	case OpsErrorCategoryUpstream:
		return []string{"检查上游服务状态和账号健康", "必要时临时切换上游账号"}
	case OpsErrorCategoryPlatform:
		return []string{"检查 Sub2API 服务日志和依赖服务状态"}
	case OpsErrorCategorySlowRequest:
		return []string{"检查 TTFT、总耗时和上游响应延迟"}
	default:
		if strings.TrimSpace(subcategory) != "" {
			return []string{"根据错误子类继续排查：" + subcategory}
		}
		return []string{"补充日志证据后继续排查"}
	}
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func opsUnifiedErrorCSVHeader() []string {
	return []string{
		"id", "occurred_at", "error_category", "error_subcategory", "client_error_subcategory", "error_result", "severity", "status_code",
		"user_id", "user_email", "api_key", "group_id", "group_name", "platform", "model", "upstream_account_id", "upstream_account_name",
		"summary", "same_kind_count", "ai_analysis_status",
	}
}

func opsUnifiedErrorCSVRow(item *OpsUnifiedErrorItem) []string {
	clientSubcategory := ""
	if item.ClientErrorSubcategory != nil {
		clientSubcategory = *item.ClientErrorSubcategory
	}
	userID, userEmail := "", ""
	if item.User != nil {
		userID = opsStrconvFormatInt(item.User.ID)
		userEmail = maskEmailForExport(item.User.Email)
	}
	apiKey := ""
	if item.APIKey != nil {
		apiKey = item.APIKey.Display
		if apiKey == "" {
			apiKey = item.APIKey.Name
		}
		if apiKey == "" && item.APIKey.ID > 0 {
			apiKey = "API Key #" + opsStrconvFormatInt(item.APIKey.ID)
		}
	}
	groupID, groupName := "", ""
	if item.Group != nil {
		groupID = opsStrconvFormatInt(item.Group.ID)
		groupName = item.Group.Name
	}
	accountID, accountName := "", ""
	if item.UpstreamAccount != nil {
		accountID = opsStrconvFormatInt(item.UpstreamAccount.ID)
		accountName = maskUpstreamAccountNameForExport(item.UpstreamAccount.Name)
	}
	return opsCSVSafeRow([]string{
		opsStrconvFormatInt(item.ID),
		item.OccurredAt.Format("2006-01-02 15:04:05"),
		item.ErrorCategory,
		item.ErrorSubcategory,
		clientSubcategory,
		item.ErrorResult,
		item.Severity,
		opsStrconvFormatInt(int64(item.StatusCode)),
		userID,
		userEmail,
		apiKey,
		groupID,
		groupName,
		item.Platform,
		item.Model,
		accountID,
		accountName,
		truncateRunes(item.Summary, 500),
		opsStrconvFormatInt(int64(item.SameKindCount)),
		item.AIAnalysisStatus,
	})
}

func opsCSVSafeRow(row []string) []string {
	out := make([]string, len(row))
	for i, value := range row {
		out[i] = opsCSVSafeCell(value)
	}
	return out
}

func opsCSVSafeCell(value string) string {
	if value == "" {
		return ""
	}
	trimmedLeft := strings.TrimLeft(value, " \r\n")
	if trimmedLeft == "" {
		return value
	}
	switch trimmedLeft[0] {
	case '=', '+', '-', '@', '\t':
		return "'" + value
	}
	return value
}

func maskEmailForExport(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "***"
	}
	local := []rune(parts[0])
	return string(local[0]) + "***@" + parts[1]
}

func maskUpstreamAccountNameForExport(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	runes := []rune(name)
	if len(runes) <= 4 {
		return string(runes[:1]) + "***"
	}
	return string(runes[:2]) + "***" + string(runes[len(runes)-2:])
}
