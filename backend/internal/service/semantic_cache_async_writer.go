package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
)

const (
	semanticCacheWriteQueueSize    = 256
	semanticCacheWriteWorkerCount  = 2
	semanticCacheWriteTaskTimeout  = 5 * time.Second
	semanticCacheSystemFingerprint = "none"
)

type SemanticCacheEntry struct {
	Namespace            string
	Platform             string
	Model                string
	APIKeyID             *int64
	UserID               *int64
	GroupID              *int64
	SystemFingerprint    string
	RuleVersion          string
	EmbeddingModel       string
	EmbeddingDimension   int
	EmbeddingRef         json.RawMessage
	NormalizedPromptHash string
	ResponseCacheKey     string
	Status               string
	ExpiresAt            time.Time
}

type SemanticCacheEntryStore interface {
	UpsertSemanticCacheEntry(ctx context.Context, entry *SemanticCacheEntry) error
}

type SemanticCacheStoredCandidate struct {
	ID                   int64
	ResponseCacheKey     string
	NormalizedPromptHash string
	EmbeddingModel       string
	EmbeddingDimension   int
	EmbeddingRef         json.RawMessage
}

type SemanticCacheCandidateStore interface {
	ListSemanticCacheCandidates(ctx context.Context, namespace string, limit int) ([]SemanticCacheStoredCandidate, error)
}

type SemanticCacheAuditRecord struct {
	RequestID       string
	SemanticEntryID *int64
	Platform        string
	Model           string
	APIKeyID        *int64
	Similarity      float64
	Decision        string
	BlockReason     string
	ReviewStatus    string
	SourceSummary   string
	TargetSummary   string
}

type SemanticCacheAuditStore interface {
	CreateSemanticCacheAudit(ctx context.Context, record *SemanticCacheAuditRecord) error
}

type SemanticCacheWriteRequest struct {
	Protocol         string
	RequestBody      []byte
	ResponseCacheKey string
	Platform         string
	Model            string
	APIKeyID         *int64
	UserID           *int64
	GroupID          *int64
	TTL              time.Duration
	StoredAt         time.Time
}

type SemanticCacheLookupRequest struct {
	RequestBody []byte
	Platform    string
	Model       string
	APIKeyID    *int64
	UserID      *int64
	GroupID     *int64
}

type SemanticCacheLookupMatch struct {
	SemanticEntryID      *int64
	ResponseCacheKey     string
	NormalizedPromptHash string
	EmbeddingModel       string
	Similarity           float64
}

type SemanticCacheLookupResult struct {
	Namespace         string
	CandidateCount    int
	HighestSimilarity float64
	Match             *SemanticCacheLookupMatch
	Reusable          bool
	Decision          string
	BlockReason       string
	ReviewStatus      string
	SkipReason        string
}

type SemanticCacheAsyncWriter struct {
	settingService  *SettingService
	embeddingClient *semanticEmbeddingClient
	store           SemanticCacheEntryStore
	candidateStore  SemanticCacheCandidateStore
	auditStore      SemanticCacheAuditStore
	queue           chan SemanticCacheWriteRequest
	once            sync.Once
}

func NewSemanticCacheAsyncWriter(cache GatewayCache, settingService *SettingService) *SemanticCacheAsyncWriter {
	store, _ := cache.(SemanticCacheEntryStore)
	candidateStore, _ := cache.(SemanticCacheCandidateStore)
	auditStore, _ := cache.(SemanticCacheAuditStore)
	writer := &SemanticCacheAsyncWriter{
		settingService:  settingService,
		embeddingClient: NewSemanticEmbeddingClient(settingService),
		store:           store,
		candidateStore:  candidateStore,
		auditStore:      auditStore,
		queue:           make(chan SemanticCacheWriteRequest, semanticCacheWriteQueueSize),
	}
	writer.start()
	return writer
}

func (w *SemanticCacheAsyncWriter) start() {
	if w == nil || w.store == nil || w.settingService == nil {
		return
	}
	w.once.Do(func() {
		for i := 0; i < semanticCacheWriteWorkerCount; i++ {
			go w.worker()
		}
	})
}

func (w *SemanticCacheAsyncWriter) Enqueue(req SemanticCacheWriteRequest) bool {
	if w == nil || w.store == nil || w.settingService == nil || strings.TrimSpace(req.ResponseCacheKey) == "" || len(req.RequestBody) == 0 {
		return false
	}
	if req.StoredAt.IsZero() {
		req.StoredAt = time.Now()
	}
	req.Platform = strings.TrimSpace(req.Platform)
	req.Model = strings.TrimSpace(req.Model)
	req.Protocol = strings.TrimSpace(req.Protocol)
	req.ResponseCacheKey = strings.TrimSpace(req.ResponseCacheKey)
	req.RequestBody = append([]byte(nil), req.RequestBody...)
	select {
	case w.queue <- req:
		return true
	default:
		return false
	}
}

func (w *SemanticCacheAsyncWriter) worker() {
	for req := range w.queue {
		ctx, cancel := context.WithTimeout(context.Background(), semanticCacheWriteTaskTimeout)
		_ = w.process(ctx, req)
		cancel()
	}
}

func (w *SemanticCacheAsyncWriter) process(ctx context.Context, req SemanticCacheWriteRequest) error {
	if w == nil || w.store == nil || w.settingService == nil || w.embeddingClient == nil {
		return nil
	}
	cfg, err := w.settingService.loadSemanticCacheConfigForUpdate(ctx)
	if err != nil {
		return err
	}
	cfg = normalizeSemanticCacheConfig(cfg)
	if !semanticCacheWriteEnabled(cfg, req.Platform, req.Model) {
		return nil
	}
	prepared, ok := buildSemanticCacheWritePrepared(req, cfg)
	if !ok {
		return nil
	}
	result, err := w.embeddingClient.GenerateEmbedding(ctx, prepared.NormalizedPrompt)
	if err != nil || result == nil || result.Skipped || len(result.Vector) == 0 {
		return err
	}
	model := strings.TrimSpace(result.Model)
	if model == "" {
		model = cfg.SemanticModelName
	}
	embeddingRef, err := json.Marshal(map[string]any{
		"provider":     "inline",
		"model":        model,
		"vector":       result.Vector,
		"generated_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}
	entry := &SemanticCacheEntry{
		Namespace:            prepared.Namespace,
		Platform:             req.Platform,
		Model:                req.Model,
		APIKeyID:             req.APIKeyID,
		UserID:               req.UserID,
		GroupID:              req.GroupID,
		SystemFingerprint:    prepared.SystemFingerprint,
		RuleVersion:          cfg.RuleVersion,
		EmbeddingModel:       model,
		EmbeddingDimension:   len(result.Vector),
		EmbeddingRef:         embeddingRef,
		NormalizedPromptHash: prepared.PromptHash,
		ResponseCacheKey:     req.ResponseCacheKey,
		Status:               "active",
		ExpiresAt:            prepared.ExpiresAt,
	}
	return w.store.UpsertSemanticCacheEntry(ctx, entry)
}

type semanticCachePreparedWrite struct {
	NormalizedPrompt  string
	PromptHash        string
	SystemFingerprint string
	Namespace         string
	ExpiresAt         time.Time
}

type semanticCachePreparedLookup struct {
	NormalizedPrompt  string
	PromptHash        string
	SystemFingerprint string
	Namespace         string
}

func semanticCacheWriteEnabled(cfg SemanticCacheConfig, platform, model string) bool {
	if !cfg.Enabled || cfg.AutoClosed || cfg.Stage == "rollback" {
		return false
	}
	if strings.TrimSpace(cfg.SemanticModelBaseURL) == "" || strings.TrimSpace(cfg.SemanticModelName) == "" || strings.TrimSpace(cfg.SemanticAPIKeyEncrypted) == "" {
		return false
	}
	platform = strings.TrimSpace(platform)
	model = strings.TrimSpace(model)
	if len(cfg.Platforms) > 0 && !stringInServiceList(platform, cfg.Platforms) {
		return false
	}
	if len(cfg.ModelAllowlist) > 0 && !stringInServiceList(model, cfg.ModelAllowlist) {
		return false
	}
	return true
}

func buildSemanticCachePreparedLookup(req SemanticCacheLookupRequest, cfg SemanticCacheConfig) (*semanticCachePreparedLookup, bool) {
	if !json.Valid(req.RequestBody) {
		return nil, false
	}
	var payload map[string]any
	if err := json.Unmarshal(req.RequestBody, &payload); err != nil {
		return nil, false
	}
	if semanticCacheRequestHasUnsafeContent(payload) {
		return nil, false
	}
	prompt, ok := extractSemanticPromptFromRequest(payload)
	if !ok {
		return nil, false
	}
	systemFingerprint := semanticSystemFingerprintFromPayload(payload)
	return &semanticCachePreparedLookup{
		NormalizedPrompt:  prompt,
		PromptHash:        sha256Hex(prompt),
		SystemFingerprint: systemFingerprint,
		Namespace: semanticCacheNamespace(cfg, SemanticCacheWriteRequest{
			Platform: req.Platform,
			Model:    req.Model,
			APIKeyID: req.APIKeyID,
			UserID:   req.UserID,
			GroupID:  req.GroupID,
		}, systemFingerprint),
	}, true
}

func buildSemanticCacheWritePrepared(req SemanticCacheWriteRequest, cfg SemanticCacheConfig) (*semanticCachePreparedWrite, bool) {
	prepared, ok := buildSemanticCachePreparedLookup(SemanticCacheLookupRequest{
		RequestBody: req.RequestBody,
		Platform:    req.Platform,
		Model:       req.Model,
		APIKeyID:    req.APIKeyID,
		UserID:      req.UserID,
		GroupID:     req.GroupID,
	}, cfg)
	if !ok {
		return nil, false
	}
	storedAt := req.StoredAt
	if storedAt.IsZero() {
		storedAt = time.Now()
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = time.Duration(cfg.MaxReuseMinutes) * time.Minute
	}
	return &semanticCachePreparedWrite{
		NormalizedPrompt:  prepared.NormalizedPrompt,
		PromptHash:        prepared.PromptHash,
		SystemFingerprint: prepared.SystemFingerprint,
		Namespace:         prepared.Namespace,
		ExpiresAt:         storedAt.Add(ttl),
	}, true
}

func (w *SemanticCacheAsyncWriter) Probe(ctx context.Context, req SemanticCacheLookupRequest) *SemanticCacheLookupResult {
	result := &SemanticCacheLookupResult{}
	if ctx == nil {
		ctx = context.Background()
	}
	var cfg SemanticCacheConfig
	var prepared *semanticCachePreparedLookup
	defer w.recordSemanticAudit(ctx, cfg, req, prepared, result)
	if w == nil || w.settingService == nil || w.embeddingClient == nil || w.candidateStore == nil {
		result.SkipReason = SemanticEmbeddingSkipConfigIncomplete
		return result
	}
	loadedCfg, err := w.settingService.loadSemanticCacheConfigForUpdate(ctx)
	if err != nil {
		result.SkipReason = SemanticEmbeddingSkipConfigIncomplete
		return result
	}
	cfg = normalizeSemanticCacheConfig(loadedCfg)
	if !semanticCacheWriteEnabled(cfg, req.Platform, req.Model) {
		result.SkipReason = SemanticEmbeddingSkipDisabled
		return result
	}
	var ok bool
	prepared, ok = buildSemanticCachePreparedLookup(req, cfg)
	if !ok {
		result.SkipReason = SemanticEmbeddingSkipInvalidInput
		return result
	}
	result.Namespace = prepared.Namespace
	embedding, err := w.embeddingClient.GenerateEmbedding(ctx, prepared.NormalizedPrompt)
	if err != nil {
		result.SkipReason = SemanticEmbeddingSkipRequestFailed
		return result
	}
	if embedding == nil || embedding.Skipped || len(embedding.Vector) == 0 {
		if embedding != nil {
			result.SkipReason = embedding.SkipReason
		} else {
			result.SkipReason = SemanticEmbeddingSkipEmptyVector
		}
		return result
	}
	candidates, err := w.candidateStore.ListSemanticCacheCandidates(ctx, prepared.Namespace, cfg.MaxCandidates)
	if err != nil {
		result.SkipReason = "candidate_query_failed"
		return result
	}
	bestSimilarity := 0.0
	for _, candidate := range candidates {
		vector, ok := semanticCacheInlineVector(candidate.EmbeddingRef)
		if !ok || len(vector) == 0 || len(vector) != len(embedding.Vector) {
			continue
		}
		similarity, ok := semanticCacheCosineSimilarity(embedding.Vector, vector)
		if !ok {
			continue
		}
		result.CandidateCount++
		if similarity > result.HighestSimilarity {
			result.HighestSimilarity = similarity
		}
		if similarity >= cfg.SimilarityThreshold && similarity >= bestSimilarity {
			bestSimilarity = similarity
			entryID := candidate.ID
			result.Match = &SemanticCacheLookupMatch{
				SemanticEntryID:      &entryID,
				ResponseCacheKey:     candidate.ResponseCacheKey,
				NormalizedPromptHash: candidate.NormalizedPromptHash,
				EmbeddingModel:       candidate.EmbeddingModel,
				Similarity:           similarity,
			}
		}
	}
	w.applySemanticLookupDecision(cfg, result)
	return result
}

func (w *SemanticCacheAsyncWriter) applySemanticLookupDecision(cfg SemanticCacheConfig, result *SemanticCacheLookupResult) {
	if result == nil || result.Match == nil {
		return
	}
	result.Reusable = true
	result.Decision = "hit"
	result.ReviewStatus = "reusable"
	switch {
	case strings.TrimSpace(cfg.Stage) == "observe":
		result.Reusable = false
		result.Decision = "observe"
		result.ReviewStatus = "pending"
	case strings.TrimSpace(cfg.Stage) == "review" || cfg.ReviewMode:
		result.Reusable = false
		result.Decision = "blocked"
		result.BlockReason = "review_pending"
		result.ReviewStatus = "pending"
	}
}

func (w *SemanticCacheAsyncWriter) recordSemanticAudit(ctx context.Context, cfg SemanticCacheConfig, req SemanticCacheLookupRequest, prepared *semanticCachePreparedLookup, result *SemanticCacheLookupResult) {
	if w == nil || w.auditStore == nil || prepared == nil || result == nil {
		return
	}
	if strings.TrimSpace(result.SkipReason) != "" || result.Match == nil {
		return
	}
	if strings.TrimSpace(result.Decision) == "" {
		return
	}
	similarity := result.Match.Similarity
	semanticEntryID := result.Match.SemanticEntryID
	targetSummary := fmt.Sprintf(
		"cache_key=%s prompt_hash=%s embedding_model=%s",
		strings.TrimSpace(result.Match.ResponseCacheKey),
		strings.TrimSpace(result.Match.NormalizedPromptHash),
		strings.TrimSpace(result.Match.EmbeddingModel),
	)

	requestID := semanticCacheRequestIDFromContext(ctx)
	if strings.TrimSpace(requestID) == "" {
		requestID = "generated:" + generateRequestID()
	}

	sourceSummary := fmt.Sprintf(
		"namespace=%s prompt_hash=%s candidates=%d highest_similarity=%.4f",
		strings.TrimSpace(prepared.Namespace),
		strings.TrimSpace(prepared.PromptHash),
		result.CandidateCount,
		result.HighestSimilarity,
	)

	_ = w.auditStore.CreateSemanticCacheAudit(ctx, &SemanticCacheAuditRecord{
		RequestID:       requestID,
		SemanticEntryID: semanticEntryID,
		Platform:        strings.TrimSpace(req.Platform),
		Model:           strings.TrimSpace(req.Model),
		APIKeyID:        req.APIKeyID,
		Similarity:      similarity,
		Decision:        strings.TrimSpace(result.Decision),
		BlockReason:     strings.TrimSpace(result.BlockReason),
		ReviewStatus:    strings.TrimSpace(result.ReviewStatus),
		SourceSummary:   sourceSummary,
		TargetSummary:   targetSummary,
	})
}

func semanticCacheRequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if clientRequestID, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
		return "client:" + strings.TrimSpace(clientRequestID)
	}
	if requestID, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
		return "local:" + strings.TrimSpace(requestID)
	}
	return ""
}

func semanticCacheNamespace(cfg SemanticCacheConfig, req SemanticCacheWriteRequest, systemFingerprint string) string {
	return fmt.Sprintf(
		"%s|p=%s|k=%d|u=%d|g=%d|m=%s|s=%s|r=%s",
		strings.TrimSpace(cfg.Namespace),
		strings.TrimSpace(req.Platform),
		semanticInt64Value(req.APIKeyID),
		semanticInt64Value(req.UserID),
		semanticInt64Value(req.GroupID),
		strings.TrimSpace(req.Model),
		systemFingerprint,
		strings.TrimSpace(cfg.RuleVersion),
	)
}

func semanticInt64Value(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func SemanticCacheUserIDFromContext(c *gin.Context) *int64 {
	if c == nil {
		return nil
	}
	if value, ok := c.Get("api_key"); ok {
		if apiKey, ok := value.(*APIKey); ok && apiKey != nil {
			if apiKey.UserID > 0 {
				userID := apiKey.UserID
				return &userID
			}
			if apiKey.User != nil && apiKey.User.ID > 0 {
				userID := apiKey.User.ID
				return &userID
			}
		}
	}
	return nil
}

func extractSemanticPromptFromRequest(payload map[string]any) (string, bool) {
	if input, ok := payload["input"]; ok {
		if prompt, ok := extractSemanticPromptFromOpenAIInput(input); ok {
			return prompt, true
		}
	}
	if messages, ok := payload["messages"]; ok {
		if prompt, ok := extractSemanticPromptFromMessages(messages); ok {
			return prompt, true
		}
	}
	if contents, ok := payload["contents"]; ok {
		if prompt, ok := extractSemanticPromptFromGeminiContents(contents); ok {
			return prompt, true
		}
	}
	return "", false
}

func extractSemanticPromptFromOpenAIInput(input any) (string, bool) {
	switch v := input.(type) {
	case string:
		return normalizeSemanticPromptChunks([]string{v})
	case []any:
		chunks := make([]string, 0, len(v))
		for _, item := range v {
			switch typed := item.(type) {
			case string:
				chunks = append(chunks, typed)
			case map[string]any:
				if role, _ := typed["role"].(string); strings.EqualFold(strings.TrimSpace(role), "user") {
					if text, ok := extractSemanticTextFromContentValue(typed["content"]); ok {
						chunks = append(chunks, text)
					}
					continue
				}
				if itemType, _ := typed["type"].(string); strings.EqualFold(strings.TrimSpace(itemType), "message") {
					if role, _ := typed["role"].(string); strings.EqualFold(strings.TrimSpace(role), "user") {
						if text, ok := extractSemanticTextFromContentValue(typed["content"]); ok {
							chunks = append(chunks, text)
						}
					}
					continue
				}
				if text, ok := extractSemanticTextBlock(typed); ok {
					chunks = append(chunks, text)
				}
			}
		}
		return normalizeSemanticPromptChunks(chunks)
	case map[string]any:
		if role, _ := v["role"].(string); strings.EqualFold(strings.TrimSpace(role), "user") {
			return extractSemanticTextFromContentValue(v["content"])
		}
	}
	return "", false
}

func extractSemanticPromptFromMessages(messages any) (string, bool) {
	items, ok := messages.([]any)
	if !ok {
		return "", false
	}
	chunks := make([]string, 0, len(items))
	for _, item := range items {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if !strings.EqualFold(strings.TrimSpace(role), "user") {
			continue
		}
		if text, ok := extractSemanticTextFromContentValue(msg["content"]); ok {
			chunks = append(chunks, text)
		}
	}
	return normalizeSemanticPromptChunks(chunks)
}

func extractSemanticPromptFromGeminiContents(contents any) (string, bool) {
	items, ok := contents.([]any)
	if !ok {
		return "", false
	}
	chunks := make([]string, 0, len(items))
	for _, item := range items {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if !strings.EqualFold(strings.TrimSpace(role), "user") {
			continue
		}
		parts, _ := msg["parts"].([]any)
		for _, part := range parts {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := extractSemanticTextBlock(partMap); ok {
				chunks = append(chunks, text)
			}
		}
	}
	return normalizeSemanticPromptChunks(chunks)
}

func extractSemanticTextFromContentValue(content any) (string, bool) {
	switch v := content.(type) {
	case string:
		return normalizeSemanticPromptChunks([]string{v})
	case []any:
		chunks := make([]string, 0, len(v))
		for _, item := range v {
			switch typed := item.(type) {
			case string:
				chunks = append(chunks, typed)
			case map[string]any:
				if text, ok := extractSemanticTextBlock(typed); ok {
					chunks = append(chunks, text)
				}
			}
		}
		return normalizeSemanticPromptChunks(chunks)
	case map[string]any:
		if text, ok := extractSemanticTextBlock(v); ok {
			return normalizeSemanticPromptChunks([]string{text})
		}
	}
	return "", false
}

func extractSemanticTextBlock(block map[string]any) (string, bool) {
	if block == nil {
		return "", false
	}
	if text, ok := block["text"].(string); ok {
		return normalizeSemanticPromptChunks([]string{text})
	}
	if text, ok := block["input_text"].(string); ok {
		return normalizeSemanticPromptChunks([]string{text})
	}
	if text, ok := block["output_text"].(string); ok {
		return normalizeSemanticPromptChunks([]string{text})
	}
	if content, ok := block["content"].(string); ok {
		return normalizeSemanticPromptChunks([]string{content})
	}
	return "", false
}

func normalizeSemanticPromptChunks(chunks []string) (string, bool) {
	normalized := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, strings.Join(strings.Fields(trimmed), " "))
	}
	if len(normalized) == 0 {
		return "", false
	}
	return strings.Join(normalized, "\n"), true
}

func semanticSystemFingerprintFromPayload(payload map[string]any) string {
	system := map[string]any{}
	for _, key := range []string{"system", "instructions", "system_instruction", "systemInstruction"} {
		if value, ok := payload[key]; ok {
			system[key] = value
		}
	}
	if len(system) == 0 {
		return semanticCacheSystemFingerprint
	}
	body, err := json.Marshal(system)
	if err != nil {
		return semanticCacheSystemFingerprint
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:8])
}

func sha256Hex(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func semanticCacheRequestHasUnsafeContent(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if items, ok := typed["tools"].([]any); ok && len(items) > 0 {
			return true
		}
		if items, ok := typed["functions"].([]any); ok && len(items) > 0 {
			return true
		}
		if typeValue, ok := typed["type"].(string); ok {
			switch strings.ToLower(strings.TrimSpace(typeValue)) {
			case "tool_use", "tool_result", "function_call", "function_result", "input_image", "output_image", "input_audio", "audio", "image", "file", "video":
				return true
			}
		}
		for _, key := range []string{"tool_use_id", "function_call", "function_response", "file_data", "inline_data", "image_url", "image", "audio", "video"} {
			if raw, ok := typed[key]; ok && semanticCacheNonEmptyValue(raw) {
				return true
			}
		}
		for _, nested := range typed {
			if semanticCacheRequestHasUnsafeContent(nested) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if semanticCacheRequestHasUnsafeContent(item) {
				return true
			}
		}
	}
	return false
}

func semanticCacheNonEmptyValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func semanticCacheInlineVector(raw json.RawMessage) ([]float64, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var payload struct {
		Vector []float64 `json:"vector"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	if len(payload.Vector) == 0 {
		return nil, false
	}
	return payload.Vector, true
}

func semanticCacheCosineSimilarity(left, right []float64) (float64, bool) {
	if len(left) == 0 || len(left) != len(right) {
		return 0, false
	}
	dot := 0.0
	leftNorm := 0.0
	rightNorm := 0.0
	for i := range left {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, false
	}
	similarity := dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
	if math.IsNaN(similarity) || math.IsInf(similarity, 0) {
		return 0, false
	}
	if similarity < 0 {
		similarity = 0
	}
	if similarity > 1 {
		similarity = 1
	}
	return similarity, true
}
