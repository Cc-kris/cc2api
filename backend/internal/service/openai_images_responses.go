package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type openAIResponsesImageResult struct {
	Result        string
	RevisedPrompt string
	OutputFormat  string
	Size          string
	Background    string
	Quality       string
	Model         string
}

const (
	responsesImageBridgeIdempotencyRoute             = "/v1/responses:image_bridge"
	responsesImageBridgeIdempotencyTTL               = 15 * time.Minute
	responsesImageBridgeIdempotencyProcessingTimeout = 10 * time.Minute
	responsesImageBridgeDuplicateWaitTimeout         = 2 * time.Minute
	responsesImageBridgeDuplicatePollInterval        = 500 * time.Millisecond
)

var ErrResponsesImageBridgeDuplicate = errors.New("responses image bridge duplicate request")

type responsesImageBridgeIdempotencyClaim struct {
	repo      IdempotencyRepository
	recordID  int64
	expiresAt time.Time
	enabled   bool
}

type responsesImageBridgeReplayPayload struct {
	Kind        string `json:"kind"`
	Version     int    `json:"version"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
	Stream      bool   `json:"stream"`
	Body        string `json:"body"`
	ImageCount  int    `json:"image_count"`
	CreatedAt   int64  `json:"created_at"`
}

const (
	responsesImageBridgeReplayKind    = "responses_image_bridge_replay"
	responsesImageBridgeReplayVersion = 1
)

func (c *responsesImageBridgeIdempotencyClaim) markSucceeded(replay responsesImageBridgeReplayPayload) {
	if c == nil || !c.enabled || c.repo == nil || c.recordID == 0 {
		return
	}
	if replay.Kind == "" {
		replay.Kind = responsesImageBridgeReplayKind
	}
	if replay.Version == 0 {
		replay.Version = responsesImageBridgeReplayVersion
	}
	if replay.StatusCode == 0 {
		replay.StatusCode = http.StatusOK
	}
	if replay.CreatedAt == 0 {
		replay.CreatedAt = time.Now().Unix()
	}
	body, err := json.Marshal(replay)
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Responses image bridge replay marshal failed: %v", err)
		body = []byte(fmt.Sprintf(`{"status":"completed","image_count":%d}`, replay.ImageCount))
	}
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.repo.MarkSucceeded(persistCtx, c.recordID, replay.StatusCode, string(body), c.expiresAt); err != nil {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Responses image bridge idempotency mark succeeded failed: %v", err)
	}
}

func (c *responsesImageBridgeIdempotencyClaim) markFailedRetryable(reason string) {
	if c == nil || !c.enabled || c.repo == nil || c.recordID == 0 {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "IMAGE_BRIDGE_FAILED"
	}
	now := time.Now()
	lockedUntil := now.Add(15 * time.Second)
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.repo.MarkFailedRetryable(persistCtx, c.recordID, reason, lockedUntil, c.expiresAt); err != nil {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Responses image bridge idempotency mark failed failed: %v", err)
	}
}

type OpenAIImagesUpstreamError struct {
	StatusCode        int
	ErrorType         string
	Code              string
	Message           string
	Param             string
	UpstreamRequestID string
}

func (e *OpenAIImagesUpstreamError) Error() string {
	if e == nil {
		return ""
	}
	code := strings.TrimSpace(e.Code)
	if code == "" {
		code = strings.TrimSpace(e.ErrorType)
	}
	message := strings.TrimSpace(e.Message)
	if code != "" && message != "" {
		return fmt.Sprintf("openai images upstream error: %s: %s", code, message)
	}
	if message != "" {
		return "openai images upstream error: " + message
	}
	if code != "" {
		return "openai images upstream error: " + code
	}
	return "openai images upstream error"
}

func (e *OpenAIImagesUpstreamError) clientStatusCode() int {
	if e == nil {
		return http.StatusBadGateway
	}
	if e.StatusCode > 0 {
		return e.StatusCode
	}
	return http.StatusBadGateway
}

func (e *OpenAIImagesUpstreamError) clientErrorType() string {
	if e == nil {
		return "upstream_error"
	}
	if trimmed := strings.TrimSpace(e.ErrorType); trimmed != "" {
		return trimmed
	}
	return "upstream_error"
}

func (e *OpenAIImagesUpstreamError) clientMessage() string {
	if e == nil {
		return "Upstream request failed"
	}
	if trimmed := strings.TrimSpace(e.Message); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(e.Code); trimmed != "" {
		return trimmed
	}
	return "Upstream request failed"
}

func openAIResponsesImageResultKey(itemID string, result openAIResponsesImageResult) string {
	if strings.TrimSpace(result.Result) != "" {
		return strings.TrimSpace(result.OutputFormat) + "|" + strings.TrimSpace(result.Result)
	}
	return "item:" + strings.TrimSpace(itemID)
}

func appendOpenAIResponsesImageResultDedup(results *[]openAIResponsesImageResult, seen map[string]struct{}, itemID string, result openAIResponsesImageResult) bool {
	if results == nil {
		return false
	}
	key := openAIResponsesImageResultKey(itemID, result)
	if key != "" {
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
	}
	*results = append(*results, result)
	return true
}

func mergeOpenAIResponsesImageMeta(dst *openAIResponsesImageResult, src openAIResponsesImageResult) {
	if dst == nil {
		return
	}
	if trimmed := strings.TrimSpace(src.OutputFormat); trimmed != "" {
		dst.OutputFormat = trimmed
	}
	if trimmed := strings.TrimSpace(src.Size); trimmed != "" {
		dst.Size = trimmed
	}
	if trimmed := strings.TrimSpace(src.Background); trimmed != "" {
		dst.Background = trimmed
	}
	if trimmed := strings.TrimSpace(src.Quality); trimmed != "" {
		dst.Quality = trimmed
	}
	if trimmed := strings.TrimSpace(src.Model); trimmed != "" {
		dst.Model = trimmed
	}
}

func openAIResponsesImageResultSizes(results []openAIResponsesImageResult) []string {
	if len(results) == 0 {
		return nil
	}
	sizes := make([]string, 0, len(results))
	for _, result := range results {
		if size := strings.TrimSpace(result.Size); size != "" {
			sizes = append(sizes, size)
		}
	}
	if len(sizes) == 0 {
		return nil
	}
	return sizes
}

func extractOpenAIResponsesImageMetaFromLifecycleEvent(payload []byte) (openAIResponsesImageResult, int64, bool) {
	switch gjson.GetBytes(payload, "type").String() {
	case "response.created", "response.in_progress", "response.completed":
	default:
		return openAIResponsesImageResult{}, 0, false
	}

	response := gjson.GetBytes(payload, "response")
	if !response.Exists() {
		return openAIResponsesImageResult{}, 0, false
	}

	meta := openAIResponsesImageResult{
		OutputFormat: strings.TrimSpace(response.Get("tools.0.output_format").String()),
		Size:         strings.TrimSpace(response.Get("tools.0.size").String()),
		Background:   strings.TrimSpace(response.Get("tools.0.background").String()),
		Quality:      strings.TrimSpace(response.Get("tools.0.quality").String()),
		Model:        strings.TrimSpace(response.Get("tools.0.model").String()),
	}
	return meta, response.Get("created_at").Int(), true
}

func buildOpenAIImagesStreamPartialPayload(
	eventType string,
	b64 string,
	partialImageIndex int64,
	responseFormat string,
	createdAt int64,
	meta openAIResponsesImageResult,
) []byte {
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}

	payload := []byte(`{"type":"","created_at":0,"partial_image_index":0,"b64_json":""}`)
	payload, _ = sjson.SetBytes(payload, "type", eventType)
	payload, _ = sjson.SetBytes(payload, "created_at", createdAt)
	payload, _ = sjson.SetBytes(payload, "partial_image_index", partialImageIndex)
	payload, _ = sjson.SetBytes(payload, "b64_json", b64)
	if strings.EqualFold(strings.TrimSpace(responseFormat), "url") {
		payload, _ = sjson.SetBytes(payload, "url", "data:"+openAIImageOutputMIMEType(meta.OutputFormat)+";base64,"+b64)
	}
	if meta.Background != "" {
		payload, _ = sjson.SetBytes(payload, "background", meta.Background)
	}
	if meta.OutputFormat != "" {
		payload, _ = sjson.SetBytes(payload, "output_format", meta.OutputFormat)
	}
	if meta.Quality != "" {
		payload, _ = sjson.SetBytes(payload, "quality", meta.Quality)
	}
	if meta.Size != "" {
		payload, _ = sjson.SetBytes(payload, "size", meta.Size)
	}
	if meta.Model != "" {
		payload, _ = sjson.SetBytes(payload, "model", meta.Model)
	}
	return payload
}

func buildOpenAIImagesStreamCompletedPayload(
	eventType string,
	img openAIResponsesImageResult,
	responseFormat string,
	createdAt int64,
	usageRaw []byte,
) []byte {
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}

	payload := []byte(`{"type":"","created_at":0,"b64_json":""}`)
	payload, _ = sjson.SetBytes(payload, "type", eventType)
	payload, _ = sjson.SetBytes(payload, "created_at", createdAt)
	payload, _ = sjson.SetBytes(payload, "b64_json", img.Result)
	if strings.EqualFold(strings.TrimSpace(responseFormat), "url") {
		payload, _ = sjson.SetBytes(payload, "url", "data:"+openAIImageOutputMIMEType(img.OutputFormat)+";base64,"+img.Result)
	}
	if img.Background != "" {
		payload, _ = sjson.SetBytes(payload, "background", img.Background)
	}
	if img.OutputFormat != "" {
		payload, _ = sjson.SetBytes(payload, "output_format", img.OutputFormat)
	}
	if img.Quality != "" {
		payload, _ = sjson.SetBytes(payload, "quality", img.Quality)
	}
	if img.Size != "" {
		payload, _ = sjson.SetBytes(payload, "size", img.Size)
	}
	if img.Model != "" {
		payload, _ = sjson.SetBytes(payload, "model", img.Model)
	}
	if len(usageRaw) > 0 && gjson.ValidBytes(usageRaw) {
		payload, _ = sjson.SetRawBytes(payload, "usage", usageRaw)
	}
	return payload
}

func openAIImageOutputMIMEType(outputFormat string) string {
	if outputFormat == "" {
		return "image/png"
	}
	if strings.Contains(outputFormat, "/") {
		return outputFormat
	}
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func openAIImageUploadToDataURL(upload OpenAIImagesUpload) (string, error) {
	if len(upload.Data) == 0 {
		return "", fmt.Errorf("upload %q is empty", strings.TrimSpace(upload.FileName))
	}
	contentType := strings.TrimSpace(upload.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(upload.Data)
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(upload.Data), nil
}

func buildOpenAIImagesResponsesRequest(parsed *OpenAIImagesRequest, toolModel string) ([]byte, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	prompt := strings.TrimSpace(parsed.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	inputImages := make([]string, 0, len(parsed.InputImageURLs)+len(parsed.Uploads))
	for _, imageURL := range parsed.InputImageURLs {
		if trimmed := strings.TrimSpace(imageURL); trimmed != "" {
			inputImages = append(inputImages, trimmed)
		}
	}
	for _, upload := range parsed.Uploads {
		dataURL, err := openAIImageUploadToDataURL(upload)
		if err != nil {
			return nil, err
		}
		inputImages = append(inputImages, dataURL)
	}
	if parsed.IsEdits() && len(inputImages) == 0 {
		return nil, fmt.Errorf("image input is required")
	}

	req := []byte(`{"instructions":"","stream":true,"reasoning":{"effort":"medium","summary":"auto"},"parallel_tool_calls":true,"include":["reasoning.encrypted_content"],"model":"","store":false,"tool_choice":{"type":"image_generation"}}`)
	req, _ = sjson.SetBytes(req, "model", openAIImagesResponsesMainModel)

	input := []byte(`[{"type":"message","role":"user","content":[{"type":"input_text","text":""}]}]`)
	input, _ = sjson.SetBytes(input, "0.content.0.text", prompt)
	for index, imageURL := range inputImages {
		part := []byte(`{"type":"input_image","image_url":""}`)
		part, _ = sjson.SetBytes(part, "image_url", imageURL)
		input, _ = sjson.SetRawBytes(input, fmt.Sprintf("0.content.%d", index+1), part)
	}
	req, _ = sjson.SetRawBytes(req, "input", input)

	action := "generate"
	if parsed.IsEdits() {
		action = "edit"
	}
	tool := []byte(`{"type":"image_generation","action":"","model":""}`)
	tool, _ = sjson.SetBytes(tool, "action", action)
	tool, _ = sjson.SetBytes(tool, "model", strings.TrimSpace(toolModel))
	if shouldPassOpenAIImagesN(toolModel, parsed.N) {
		tool, _ = sjson.SetBytes(tool, "n", parsed.N)
	}

	for _, field := range []struct {
		path  string
		value string
	}{
		{path: "size", value: parsed.Size},
		{path: "quality", value: parsed.Quality},
		{path: "background", value: parsed.Background},
		{path: "output_format", value: parsed.OutputFormat},
		{path: "moderation", value: parsed.Moderation},
		{path: "style", value: parsed.Style},
	} {
		if trimmed := strings.TrimSpace(field.value); trimmed != "" {
			tool, _ = sjson.SetBytes(tool, field.path, trimmed)
		}
	}
	if parsed.OutputCompression != nil {
		tool, _ = sjson.SetBytes(tool, "output_compression", *parsed.OutputCompression)
	}
	if parsed.PartialImages != nil {
		tool, _ = sjson.SetBytes(tool, "partial_images", *parsed.PartialImages)
	}

	maskImageURL := strings.TrimSpace(parsed.MaskImageURL)
	if parsed.MaskUpload != nil {
		dataURL, err := openAIImageUploadToDataURL(*parsed.MaskUpload)
		if err != nil {
			return nil, err
		}
		maskImageURL = dataURL
	}
	if maskImageURL != "" {
		tool, _ = sjson.SetBytes(tool, "input_image_mask.image_url", maskImageURL)
	}

	req, _ = sjson.SetRawBytes(req, "tools", []byte(`[]`))
	req, _ = sjson.SetRawBytes(req, "tools.-1", tool)
	return req, nil
}

func shouldPassOpenAIImagesN(model string, n int) bool {
	if n <= 1 {
		return false
	}
	return !strings.EqualFold(strings.TrimSpace(model), "dall-e-3")
}

func extractOpenAIImagesFromResponsesCompleted(payload []byte) ([]openAIResponsesImageResult, int64, []byte, openAIResponsesImageResult, error) {
	if gjson.GetBytes(payload, "type").String() != "response.completed" {
		return nil, 0, nil, openAIResponsesImageResult{}, fmt.Errorf("unexpected event type")
	}

	createdAt := gjson.GetBytes(payload, "response.created_at").Int()
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}

	var (
		results   []openAIResponsesImageResult
		firstMeta openAIResponsesImageResult
	)
	output := gjson.GetBytes(payload, "response.output")
	if output.IsArray() {
		for _, item := range output.Array() {
			if item.Get("type").String() != "image_generation_call" {
				continue
			}
			result := strings.TrimSpace(item.Get("result").String())
			if result == "" {
				continue
			}
			entry := openAIResponsesImageResult{
				Result:        result,
				RevisedPrompt: strings.TrimSpace(item.Get("revised_prompt").String()),
				OutputFormat:  strings.TrimSpace(item.Get("output_format").String()),
				Size:          strings.TrimSpace(item.Get("size").String()),
				Background:    strings.TrimSpace(item.Get("background").String()),
				Quality:       strings.TrimSpace(item.Get("quality").String()),
			}
			if len(results) == 0 {
				firstMeta = entry
			}
			results = append(results, entry)
		}
	}

	var usageRaw []byte
	if usage := gjson.GetBytes(payload, "response.tool_usage.image_gen"); usage.Exists() && usage.IsObject() {
		usageRaw = []byte(usage.Raw)
	}
	return results, createdAt, usageRaw, firstMeta, nil
}

func extractOpenAIImageFromResponsesOutputItemDone(payload []byte) (openAIResponsesImageResult, string, bool, error) {
	if gjson.GetBytes(payload, "type").String() != "response.output_item.done" {
		return openAIResponsesImageResult{}, "", false, fmt.Errorf("unexpected event type")
	}

	item := gjson.GetBytes(payload, "item")
	if !item.Exists() || item.Get("type").String() != "image_generation_call" {
		return openAIResponsesImageResult{}, "", false, nil
	}

	result := strings.TrimSpace(item.Get("result").String())
	if result == "" {
		return openAIResponsesImageResult{}, "", false, nil
	}

	entry := openAIResponsesImageResult{
		Result:        result,
		RevisedPrompt: strings.TrimSpace(item.Get("revised_prompt").String()),
		OutputFormat:  strings.TrimSpace(item.Get("output_format").String()),
		Size:          strings.TrimSpace(item.Get("size").String()),
		Background:    strings.TrimSpace(item.Get("background").String()),
		Quality:       strings.TrimSpace(item.Get("quality").String()),
	}
	return entry, strings.TrimSpace(item.Get("id").String()), true, nil
}

func collectOpenAIImagesFromResponsesBody(body []byte) ([]openAIResponsesImageResult, int64, []byte, openAIResponsesImageResult, bool, error) {
	var (
		fallbackResults []openAIResponsesImageResult
		fallbackSeen    = make(map[string]struct{})
		finalResults    []openAIResponsesImageResult
		finalMeta       openAIResponsesImageResult
		collectErr      error
		createdAt       int64
		usageRaw        []byte
		foundFinal      bool
		responseMeta    openAIResponsesImageResult
	)

	forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
		if collectErr != nil || len(finalResults) > 0 {
			return
		}
		if !gjson.ValidBytes(payload) {
			return
		}
		if meta, eventCreatedAt, ok := extractOpenAIResponsesImageMetaFromLifecycleEvent(payload); ok {
			mergeOpenAIResponsesImageMeta(&responseMeta, meta)
			if eventCreatedAt > 0 {
				createdAt = eventCreatedAt
			}
		}

		switch gjson.GetBytes(payload, "type").String() {
		case "response.output_item.done":
			result, itemID, ok, err := extractOpenAIImageFromResponsesOutputItemDone(payload)
			if err != nil {
				collectErr = err
				return
			}
			if ok {
				mergeOpenAIResponsesImageMeta(&result, responseMeta)
				appendOpenAIResponsesImageResultDedup(&fallbackResults, fallbackSeen, itemID, result)
			}
		case "response.completed":
			results, completedAt, completedUsageRaw, firstMeta, err := extractOpenAIImagesFromResponsesCompleted(payload)
			if err != nil {
				collectErr = err
				return
			}
			foundFinal = true
			if completedAt > 0 {
				createdAt = completedAt
			}
			if len(completedUsageRaw) > 0 {
				usageRaw = completedUsageRaw
			}
			if len(results) > 0 {
				mergeOpenAIResponsesImageMeta(&firstMeta, responseMeta)
				finalResults = results
				finalMeta = firstMeta
				return
			}
			if len(fallbackResults) > 0 {
				firstMeta = fallbackResults[0]
				mergeOpenAIResponsesImageMeta(&firstMeta, responseMeta)
				finalResults = fallbackResults
				finalMeta = firstMeta
				return
			}
		}
	})
	if collectErr != nil {
		return nil, 0, nil, openAIResponsesImageResult{}, false, collectErr
	}
	if len(finalResults) > 0 {
		return finalResults, createdAt, usageRaw, finalMeta, true, nil
	}

	if len(fallbackResults) > 0 {
		firstMeta := fallbackResults[0]
		mergeOpenAIResponsesImageMeta(&firstMeta, responseMeta)
		return fallbackResults, createdAt, usageRaw, firstMeta, foundFinal, nil
	}
	return nil, createdAt, usageRaw, openAIResponsesImageResult{}, foundFinal, nil
}

func extractOpenAIImagesUpstreamError(body []byte) *OpenAIImagesUpstreamError {
	var upstreamErr *OpenAIImagesUpstreamError
	forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
		if upstreamErr != nil || !gjson.ValidBytes(payload) {
			return
		}
		upstreamErr = openAIImagesUpstreamErrorFromSSEPayload(payload)
	})
	return upstreamErr
}

func openAIImagesUpstreamErrorFromSSEPayload(payload []byte) *OpenAIImagesUpstreamError {
	if !gjson.ValidBytes(payload) {
		return nil
	}
	switch gjson.GetBytes(payload, "type").String() {
	case "error":
		return openAIImagesUpstreamErrorFromGJSON(gjson.GetBytes(payload, "error"), "")
	case "response.failed":
		response := gjson.GetBytes(payload, "response")
		return openAIImagesUpstreamErrorFromGJSON(response.Get("error"), response.Get("id").String())
	default:
		return nil
	}
}

func openAIImagesUpstreamErrorFromGJSON(errorObj gjson.Result, upstreamRequestID string) *OpenAIImagesUpstreamError {
	if !errorObj.Exists() {
		return nil
	}
	code := strings.TrimSpace(errorObj.Get("code").String())
	errType := strings.TrimSpace(errorObj.Get("type").String())
	message := strings.TrimSpace(errorObj.Get("message").String())
	param := strings.TrimSpace(errorObj.Get("param").String())
	statusCode := http.StatusBadGateway
	if strings.EqualFold(code, "moderation_blocked") || strings.EqualFold(errType, "image_generation_user_error") {
		statusCode = http.StatusBadRequest
	}
	if message == "" {
		message = "Upstream request failed"
	}
	return &OpenAIImagesUpstreamError{
		StatusCode:        statusCode,
		ErrorType:         errType,
		Code:              code,
		Message:           sanitizeUpstreamErrorMessage(message),
		Param:             param,
		UpstreamRequestID: strings.TrimSpace(upstreamRequestID),
	}
}

func buildOpenAIImagesFallbackRequestFromResponsesBody(body []byte, fallbackModel string, forceStream bool) (*OpenAIImagesRequest, []byte, error) {
	return buildOpenAIImagesRequestFromResponsesBody(body, fallbackModel, forceStream, false)
}

func BuildOpenAIImagesRequestFromResponsesBody(body []byte, fallbackModel string, forceStream bool) (*OpenAIImagesRequest, []byte, error) {
	return buildOpenAIImagesRequestFromResponsesBody(body, fallbackModel, forceStream, true)
}

func buildOpenAIImagesRequestFromResponsesBody(body []byte, fallbackModel string, forceStream bool, requireExplicitIntent bool) (*OpenAIImagesRequest, []byte, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil, nil, fmt.Errorf("responses image fallback requires a valid JSON body")
	}
	if requireExplicitIntent && !OpenAIResponsesBodyHasExplicitImageGenerationIntent(body, fallbackModel) {
		return nil, nil, fmt.Errorf("responses image bridge requires explicit image generation intent")
	}

	prompt := extractOpenAIResponsesImagePrompt(body)
	if prompt == "" {
		return nil, nil, fmt.Errorf("responses image fallback requires prompt text")
	}

	tool := gjson.GetBytes(body, `tools.#(type=="image_generation")`)
	imageModel := strings.TrimSpace(tool.Get("model").String())
	if imageModel == "" || !isOpenAIImageGenerationModel(imageModel) {
		imageModel = strings.TrimSpace(fallbackModel)
	}
	if imageModel == "" || !isOpenAIImageGenerationModel(imageModel) {
		imageModel = "gpt-image-2"
	}

	req := &OpenAIImagesRequest{
		Endpoint:       openAIImagesGenerationsEndpoint,
		ContentType:    "application/json",
		Model:          imageModel,
		ExplicitModel:  true,
		Prompt:         prompt,
		Stream:         forceStream,
		N:              1,
		ResponseFormat: "b64_json",
		Body:           body,
	}
	inputImageURLs, maskImageURL := extractOpenAIResponsesInputImageURLs(body)
	if len(inputImageURLs) > 0 {
		req.Endpoint = openAIImagesEditsEndpoint
		req.InputImageURLs = inputImageURLs
		req.MaskImageURL = maskImageURL
		req.HasMask = maskImageURL != ""
	}
	if n := tool.Get("n"); n.Exists() && n.Type == gjson.Number && n.Int() > 0 {
		req.N = int(n.Int())
	}
	copyString := func(field string, dst *string) {
		if value := strings.TrimSpace(tool.Get(field).String()); value != "" {
			*dst = value
		}
	}
	copyString("size", &req.Size)
	copyString("quality", &req.Quality)
	copyString("background", &req.Background)
	copyString("output_format", &req.OutputFormat)
	copyString("moderation", &req.Moderation)
	copyString("style", &req.Style)
	if outputCompression := tool.Get("output_compression"); outputCompression.Exists() && outputCompression.Type == gjson.Number {
		v := int(outputCompression.Int())
		req.OutputCompression = &v
	}
	if partialImages := tool.Get("partial_images"); partialImages.Exists() && partialImages.Type == gjson.Number {
		v := int(partialImages.Int())
		req.PartialImages = &v
	}
	req.SizeTier = normalizeOpenAIImageSizeTier(req.Size)
	req.RequiredCapability = classifyOpenAIImagesCapability(req)

	payload := map[string]any{
		"model":           req.Model,
		"prompt":          req.Prompt,
		"n":               req.N,
		"response_format": req.ResponseFormat,
	}
	if len(req.InputImageURLs) > 0 {
		images := make([]map[string]string, 0, len(req.InputImageURLs))
		for _, imageURL := range req.InputImageURLs {
			images = append(images, map[string]string{"image_url": imageURL})
		}
		payload["images"] = images
		if req.MaskImageURL != "" {
			payload["mask"] = map[string]string{"image_url": req.MaskImageURL}
		}
	}
	if req.Size != "" {
		payload["size"] = req.Size
	}
	if req.Quality != "" {
		payload["quality"] = req.Quality
	}
	if req.Background != "" {
		payload["background"] = req.Background
	}
	if req.OutputFormat != "" {
		payload["output_format"] = req.OutputFormat
	}
	if req.Moderation != "" {
		payload["moderation"] = req.Moderation
	}
	if req.Style != "" {
		payload["style"] = req.Style
	}
	if req.OutputCompression != nil {
		payload["output_compression"] = *req.OutputCompression
	}
	if req.PartialImages != nil {
		payload["partial_images"] = *req.PartialImages
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	req.Body = payloadBytes
	return req, payloadBytes, nil
}

func extractOpenAIResponsesInputImageURLs(body []byte) ([]string, string) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil, ""
	}
	seen := make(map[string]struct{})
	images := make([]string, 0)
	var mask string
	addImage := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		images = append(images, raw)
	}
	extractImageURL := func(item gjson.Result) string {
		if imageURL := strings.TrimSpace(item.Get("image_url").String()); imageURL != "" {
			return imageURL
		}
		imageURL := item.Get("image_url")
		if imageURL.IsObject() {
			if url := strings.TrimSpace(imageURL.Get("url").String()); url != "" {
				return url
			}
		}
		if url := strings.TrimSpace(item.Get("url").String()); url != "" {
			return url
		}
		if data := strings.TrimSpace(item.Get("data").String()); data != "" {
			return data
		}
		if b64 := strings.TrimSpace(item.Get("b64_json").String()); b64 != "" {
			return "data:image/png;base64," + normalizeOpenAIImageBase64(b64)
		}
		if b64 := strings.TrimSpace(item.Get("result").String()); b64 != "" {
			outputFormat := strings.TrimSpace(item.Get("output_format").String())
			return "data:" + openAIImageOutputMIMEType(outputFormat) + ";base64," + normalizeOpenAIImageBase64(b64)
		}
		return ""
	}
	var walk func(gjson.Result)
	walk = func(node gjson.Result) {
		if node.IsArray() {
			for _, item := range node.Array() {
				walk(item)
			}
			return
		}
		if !node.IsObject() {
			return
		}
		itemType := strings.TrimSpace(node.Get("type").String())
		switch itemType {
		case "input_image", "image_url":
			addImage(extractImageURL(node))
		case "image_generation_call":
			addImage(extractImageURL(node))
		case "input_image_mask":
			if mask == "" {
				mask = strings.TrimSpace(extractImageURL(node))
			}
		}
		if mask == "" {
			mask = strings.TrimSpace(node.Get("input_image_mask.image_url").String())
		}
		if content := node.Get("content"); content.Exists() {
			walk(content)
		}
		if input := node.Get("input"); input.Exists() {
			walk(input)
		}
		if output := node.Get("output"); output.Exists() {
			walk(output)
		}
	}
	walk(gjson.GetBytes(body, "input"))
	return images, mask
}

func OpenAIResponsesBodyHasExplicitImageGenerationIntent(body []byte, requestedModel string) bool {
	if strings.TrimSpace(requestedModel) != "" && isOpenAIImageGenerationModel(requestedModel) {
		return true
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	if model := strings.TrimSpace(gjson.GetBytes(body, "model").String()); isOpenAIImageGenerationModel(model) {
		return true
	}
	return openAIJSONToolChoiceSelectsImageGeneration(gjson.GetBytes(body, "tool_choice"))
}

func (s *OpenAIGatewayService) acquireResponsesImageBridgeIdempotency(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	imageBody []byte,
	reqModel string,
	reqStream bool,
	upstreamModel string,
) (*responsesImageBridgeIdempotencyClaim, error) {
	claim := &responsesImageBridgeIdempotencyClaim{}
	if s == nil || s.idempotencyRepo == nil || c == nil || c.Request == nil || account == nil {
		return claim, nil
	}
	scope, rawKey, fingerprintPayload, ok := buildResponsesImageBridgeIdempotencyIdentity(c, account, imageBody, reqModel, upstreamModel)
	if !ok {
		return claim, nil
	}
	fingerprint, err := BuildIdempotencyFingerprint(http.MethodPost, responsesImageBridgeIdempotencyRoute, scope, fingerprintPayload)
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Responses image bridge idempotency fingerprint failed: %v", err)
		return claim, nil
	}

	now := time.Now()
	lockedUntil := now.Add(responsesImageBridgeIdempotencyProcessingTimeout)
	expiresAt := now.Add(responsesImageBridgeIdempotencyTTL)
	keyHash := HashIdempotencyKey(rawKey)
	record := &IdempotencyRecord{
		Scope:              scope,
		IdempotencyKeyHash: keyHash,
		RequestFingerprint: fingerprint,
		Status:             IdempotencyStatusProcessing,
		LockedUntil:        &lockedUntil,
		ExpiresAt:          expiresAt,
	}

	owner, err := s.idempotencyRepo.CreateProcessing(ctx, record)
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Responses image bridge idempotency create failed: %v", err)
		return claim, nil
	}
	if owner {
		claim.repo = s.idempotencyRepo
		claim.recordID = record.ID
		claim.expiresAt = expiresAt
		claim.enabled = true
		return claim, nil
	}

	existing, err := s.idempotencyRepo.GetByScopeAndKeyHash(ctx, scope, keyHash)
	if err != nil || existing == nil {
		if err != nil {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Responses image bridge idempotency lookup failed: %v", err)
		}
		return claim, nil
	}
	if existing.RequestFingerprint != fingerprint {
		writeResponsesImageBridgeDuplicateResponse(c, reqModel, reqStream, "duplicate image request key was reused with different payload")
		return nil, ErrResponsesImageBridgeDuplicate
	}

	if !existing.ExpiresAt.After(now) || (existing.LockedUntil != nil && !existing.LockedUntil.After(now)) {
		taken, reclaimErr := s.idempotencyRepo.TryReclaim(ctx, existing.ID, existing.Status, now, lockedUntil, expiresAt)
		if reclaimErr != nil {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Responses image bridge idempotency reclaim failed: %v", reclaimErr)
			return claim, nil
		}
		if taken {
			claim.repo = s.idempotencyRepo
			claim.recordID = existing.ID
			claim.expiresAt = expiresAt
			claim.enabled = true
			return claim, nil
		}
	}

	retryAfter := 0
	if existing.LockedUntil != nil && existing.LockedUntil.After(now) {
		retryAfter = int(time.Until(*existing.LockedUntil).Seconds())
		if retryAfter <= 0 {
			retryAfter = 1
		}
	}
	switch existing.Status {
	case IdempotencyStatusSucceeded:
		if writeResponsesImageBridgeReplayResponse(c, existing.ResponseBody) {
			return nil, ErrResponsesImageBridgeDuplicate
		}
		writeResponsesImageBridgeDuplicateResponse(c, reqModel, reqStream, "completed image generation replay is unavailable; upstream request was not repeated")
	case IdempotencyStatusProcessing:
		if s.waitAndReplayResponsesImageBridgeDuplicate(ctx, c, scope, keyHash, reqModel, reqStream, now) {
			return nil, ErrResponsesImageBridgeDuplicate
		}
		writeResponsesImageBridgeDuplicateResponse(c, reqModel, reqStream, "duplicate image generation request is still processing; upstream request was not repeated", retryAfter)
	case IdempotencyStatusFailedRetryable:
		writeResponsesImageBridgeDuplicateResponse(c, reqModel, reqStream, "duplicate image generation request is in retry backoff; upstream request was not repeated", retryAfter)
	default:
		writeResponsesImageBridgeDuplicateResponse(c, reqModel, reqStream, "duplicate image generation request was not repeated")
	}
	return nil, ErrResponsesImageBridgeDuplicate
}

func (s *OpenAIGatewayService) waitAndReplayResponsesImageBridgeDuplicate(
	ctx context.Context,
	c *gin.Context,
	scope string,
	keyHash string,
	reqModel string,
	reqStream bool,
	start time.Time,
) bool {
	if s == nil || s.idempotencyRepo == nil || c == nil || c.Request == nil {
		return false
	}
	waitCtx := ctx
	if waitCtx == nil {
		waitCtx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(waitCtx, responsesImageBridgeDuplicateWaitTimeout)
	defer cancel()

	ticker := time.NewTicker(responsesImageBridgeDuplicatePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-waitCtx.Done():
			return false
		case <-ticker.C:
			existing, err := s.idempotencyRepo.GetByScopeAndKeyHash(waitCtx, scope, keyHash)
			if err != nil || existing == nil {
				return false
			}
			switch existing.Status {
			case IdempotencyStatusSucceeded:
				if writeResponsesImageBridgeReplayResponse(c, existing.ResponseBody) {
					logger.LegacyPrintf(
						"service.openai_gateway",
						"[OpenAI] Responses image bridge waited and replayed duplicate: scope=%s key_hash=%s wait_ms=%d",
						scope,
						truncateString(keyHash, 16),
						time.Since(start).Milliseconds(),
					)
					return true
				}
				return false
			case IdempotencyStatusFailedRetryable:
				writeResponsesImageBridgeDuplicateResponse(c, reqModel, reqStream, "duplicate image generation request is in retry backoff; upstream request was not repeated")
				return true
			}
		}
	}
}

func buildResponsesImageBridgeIdempotencyIdentity(
	c *gin.Context,
	account *Account,
	imageBody []byte,
	reqModel string,
	upstreamModel string,
) (string, string, map[string]any, bool) {
	if c == nil || c.Request == nil || account == nil {
		return "", "", nil, false
	}
	actorScope := responsesImageBridgeActorScope(c, account)
	sum := sha256.Sum256(imageBody)
	bodyHash := hex.EncodeToString(sum[:])
	rawKey := strings.Join([]string{
		"openai_responses_image_bridge",
		"body:" + bodyHash,
	}, "|")
	payload := map[string]any{
		"body_hash": bodyHash,
	}
	return "openai_responses_image_bridge:" + actorScope, rawKey, payload, true
}

func responsesImageBridgeActorScope(c *gin.Context, account *Account) string {
	if c != nil {
		if v, ok := c.Get("api_key"); ok {
			if apiKey, ok := v.(*APIKey); ok && apiKey != nil && apiKey.ID > 0 {
				userID := apiKey.UserID
				if userID == 0 && apiKey.User != nil {
					userID = apiKey.User.ID
				}
				return fmt.Sprintf("user:%d:api_key:%d", userID, apiKey.ID)
			}
		}
	}
	if account != nil {
		return fmt.Sprintf("account:%d", account.ID)
	}
	return "anonymous"
}

func extractCodexTurnID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if gjson.Valid(raw) {
		for _, path := range []string{"turn_id", "turnId", "id", "turn.id"} {
			if v := strings.TrimSpace(gjson.Get(raw, path).String()); v != "" {
				return v
			}
		}
	}
	return raw
}

func writeResponsesImageBridgeReplayResponse(c *gin.Context, rawReplay *string) bool {
	if c == nil || c.Writer == nil || c.Writer.Written() || rawReplay == nil || strings.TrimSpace(*rawReplay) == "" {
		return false
	}
	var replay responsesImageBridgeReplayPayload
	if err := json.Unmarshal([]byte(*rawReplay), &replay); err != nil {
		return false
	}
	if replay.Kind != responsesImageBridgeReplayKind || replay.Version != responsesImageBridgeReplayVersion || strings.TrimSpace(replay.Body) == "" {
		return false
	}
	statusCode := replay.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	contentType := strings.TrimSpace(replay.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	if replay.Stream {
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
	}
	c.Data(statusCode, contentType, []byte(replay.Body))
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return true
}

func writeResponsesImageBridgeDuplicateResponse(c *gin.Context, model string, stream bool, message string, retryAfter ...int) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "duplicate image generation request was not repeated"
	}
	if len(retryAfter) > 0 && retryAfter[0] > 0 {
		c.Header("Retry-After", strconv.Itoa(retryAfter[0]))
	}
	responseID := "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	errorCode := "image_generation_duplicate"
	statusCode := http.StatusConflict
	if strings.Contains(strings.ToLower(message), "processing") {
		errorCode = "image_generation_in_progress"
	}
	if strings.Contains(strings.ToLower(message), "different payload") {
		errorCode = "image_generation_idempotency_conflict"
	}
	response := map[string]any{
		"id":     responseID,
		"object": "response",
		"model":  model,
		"status": "failed",
		"output": []any{},
		"error": map[string]any{
			"code":    errorCode,
			"message": message,
		},
		"metadata": map[string]any{
			"duplicate_suppressed": true,
			"reason":               message,
		},
	}
	if !stream {
		c.JSON(statusCode, response)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(statusCode)
	createdPayload := map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":     responseID,
			"object": "response",
			"model":  model,
			"status": "in_progress",
			"output": []any{},
		},
	}
	failedPayload := map[string]any{
		"type":     "response.failed",
		"response": response,
	}
	createdRaw, _ := json.Marshal(createdPayload)
	failedRaw, _ := json.Marshal(failedPayload)
	_, _ = fmt.Fprintf(c.Writer, "event: response.created\ndata: %s\n\n", createdRaw)
	_, _ = fmt.Fprintf(c.Writer, "event: response.failed\ndata: %s\n\n", failedRaw)
	_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *OpenAIGatewayService) ForwardResponsesImageBridgeToImages(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	reqModel string,
	reqStream bool,
	imageBillingModel string,
	imageSizeTier string,
	imageInputSize string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	if account == nil || account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("responses image bridge requires an OpenAI API key account")
	}
	parsed, imageBody, err := BuildOpenAIImagesRequestFromResponsesBody(body, imageBillingModel, false)
	if err != nil {
		return nil, err
	}
	upstreamModel := account.GetMappedModel(parsed.Model)
	if upstreamModel == "" {
		upstreamModel = parsed.Model
	}
	if err := validateOpenAIImagesModel(upstreamModel); err != nil {
		return nil, err
	}
	if upstreamModel != parsed.Model {
		imageBody, _, err = rewriteOpenAIImagesModel(imageBody, parsed.ContentType, upstreamModel)
		if err != nil {
			return nil, err
		}
	}
	imageBodyHashBytes := sha256.Sum256(imageBody)
	imageBodyHash := hex.EncodeToString(imageBodyHashBytes[:])
	idempotencyClaim, err := s.acquireResponsesImageBridgeIdempotency(ctx, c, account, imageBody, reqModel, reqStream, upstreamModel)
	if err != nil {
		if errors.Is(err, ErrResponsesImageBridgeDuplicate) {
			return &OpenAIForwardResult{
				Model:               reqModel,
				UpstreamModel:       upstreamModel,
				Stream:              reqStream,
				Duration:            time.Since(startTime),
				BillingModel:        firstNonEmptyString(imageBillingModel, parsed.Model),
				DuplicateSuppressed: true,
			}, nil
		}
		return nil, err
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Responses image bridge direct to /images/generations: account=%d name=%s image_model=%s upstream_model=%s body_hash=%s",
		account.ID,
		account.Name,
		parsed.Model,
		upstreamModel,
		truncateString(imageBodyHash, 16),
	)

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAIImagesRequest(upstreamCtx, c, account, imageBody, parsed.ContentType, token, parsed.Endpoint)
	if err != nil {
		return nil, err
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		idempotencyClaim.markFailedRetryable("IMAGE_BRIDGE_UPSTREAM_REQUEST_FAILED")
		return nil, fmt.Errorf("images bridge upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, readErr := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if readErr != nil {
		idempotencyClaim.markFailedRetryable("IMAGE_BRIDGE_UPSTREAM_READ_FAILED")
		return nil, readErr
	}
	if resp.StatusCode >= 400 {
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, truncateString(string(respBody), 2048))
		idempotencyClaim.markFailedRetryable(fmt.Sprintf("IMAGE_BRIDGE_UPSTREAM_%d", resp.StatusCode))
		return nil, &UpstreamFailoverError{
			StatusCode:   resp.StatusCode,
			ResponseBody: respBody,
		}
	}

	results := make([]openAIResponsesImageResult, 0, parsed.N)
	for _, item := range gjson.GetBytes(respBody, "data").Array() {
		result := strings.TrimSpace(item.Get("b64_json").String())
		if result == "" {
			continue
		}
		results = append(results, openAIResponsesImageResult{
			Result:        result,
			RevisedPrompt: strings.TrimSpace(item.Get("revised_prompt").String()),
			OutputFormat:  firstNonEmptyString(gjson.GetBytes(imageBody, "output_format").String(), "png"),
			Size:          parsed.Size,
			Model:         upstreamModel,
		})
	}
	if len(results) == 0 {
		idempotencyClaim.markFailedRetryable("IMAGE_BRIDGE_EMPTY_RESULT")
		return nil, fmt.Errorf("images bridge returned no images")
	}
	usage, _ := extractOpenAIUsageFromJSONBytes(respBody)
	statusCode, contentType, responseBody, err := buildResponsesImageFallbackResponse(reqModel, reqStream, results, usage)
	if err != nil {
		idempotencyClaim.markFailedRetryable("IMAGE_BRIDGE_RESPONSE_BUILD_FAILED")
		return nil, err
	}
	idempotencyClaim.markSucceeded(responsesImageBridgeReplayPayload{
		StatusCode:  statusCode,
		ContentType: contentType,
		Stream:      reqStream,
		Body:        string(responseBody),
		ImageCount:  len(results),
	})
	if err := writeResponsesImageFallbackResponse(c, reqStream, statusCode, contentType, responseBody); err != nil {
		return nil, err
	}
	firstTokenMs := int(time.Since(startTime).Milliseconds())
	return &OpenAIForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            usage,
		Model:            reqModel,
		UpstreamModel:    upstreamModel,
		Stream:           reqStream,
		ResponseHeaders:  resp.Header.Clone(),
		Duration:         time.Since(startTime),
		FirstTokenMs:     &firstTokenMs,
		ImageCount:       len(results),
		ImageSize:        imageSizeTier,
		ImageInputSize:   imageInputSize,
		ImageOutputSizes: openAIResponsesImageResultSizes(results),
		BillingModel:     firstNonEmptyString(imageBillingModel, parsed.Model),
	}, nil
}

func extractOpenAIResponsesImagePrompt(body []byte) string {
	if prompt := strings.TrimSpace(gjson.GetBytes(body, "prompt").String()); prompt != "" {
		return prompt
	}
	input := gjson.GetBytes(body, "input")
	if input.Type == gjson.String {
		return strings.TrimSpace(input.String())
	}
	lastUserText := ""
	lastAnyText := ""
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Type == gjson.String {
			if text := strings.TrimSpace(item.String()); text != "" {
				lastAnyText = text
			}
			return true
		}
		text := extractOpenAIResponsesImagePromptTextFromItem(item)
		if text == "" {
			return true
		}
		lastAnyText = text
		if strings.TrimSpace(item.Get("role").String()) == "user" {
			lastUserText = text
		}
		return true
	})
	if lastUserText != "" {
		return lastUserText
	}
	return lastAnyText
}

func extractOpenAIResponsesImagePromptTextFromItem(item gjson.Result) string {
	content := item.Get("content")
	if content.Type == gjson.String {
		return strings.TrimSpace(content.String())
	}
	parts := make([]string, 0, 4)
	content.ForEach(func(_, part gjson.Result) bool {
		partType := strings.TrimSpace(part.Get("type").String())
		if partType != "" && partType != "text" && partType != "input_text" && partType != "output_text" {
			return true
		}
		if text := strings.TrimSpace(part.Get("text").String()); text != "" {
			parts = append(parts, text)
			return true
		}
		if text := strings.TrimSpace(part.Get("input_text").String()); text != "" {
			parts = append(parts, text)
			return true
		}
		return true
	})
	if len(parts) > 0 {
		return strings.TrimSpace(strings.Join(parts, "\n"))
	}
	if text := strings.TrimSpace(item.Get("text").String()); text != "" {
		return text
	}
	return strings.TrimSpace(item.Get("input_text").String())
}

func openAIResponsesImageFallbackShouldRun(c *gin.Context, account *Account, body []byte, reqModel string) bool {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return false
	}
	if account == nil || account.Type != AccountTypeAPIKey {
		return false
	}
	return OpenAIResponsesBodyHasExplicitImageGenerationIntent(body, reqModel)
}

func (s *OpenAIGatewayService) tryFallbackResponsesImageGenerationToImagesAPI(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	reqModel string,
	reqStream bool,
	imageBillingModel string,
	imageSizeTier string,
	imageInputSize string,
	startTime time.Time,
	reason string,
) (*OpenAIForwardResult, bool, error) {
	if !openAIResponsesImageFallbackShouldRun(c, account, body, reqModel) {
		return nil, false, nil
	}

	parsed, fallbackBody, err := buildOpenAIImagesFallbackRequestFromResponsesBody(body, imageBillingModel, false)
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Responses image_generation fallback skipped: account=%d reason=%s err=%v", account.ID, reason, err)
		return nil, false, nil
	}
	upstreamModel := account.GetMappedModel(parsed.Model)
	if upstreamModel == "" {
		upstreamModel = parsed.Model
	}
	if err := validateOpenAIImagesModel(upstreamModel); err != nil {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Responses image_generation fallback skipped: account=%d reason=%s mapped_model=%s err=%v", account.ID, reason, upstreamModel, err)
		return nil, false, nil
	}
	if upstreamModel != parsed.Model {
		fallbackBody, _, err = rewriteOpenAIImagesModel(fallbackBody, parsed.ContentType, upstreamModel)
		if err != nil {
			return nil, true, err
		}
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Responses image_generation fallback to /images/generations: account=%d name=%s reason=%s image_model=%s upstream_model=%s",
		account.ID,
		account.Name,
		reason,
		parsed.Model,
		upstreamModel,
	)

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, true, err
	}
	upstreamReq, err := s.buildOpenAIImagesRequest(upstreamCtx, c, account, fallbackBody, parsed.ContentType, token, parsed.Endpoint)
	if err != nil {
		return nil, true, err
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		return nil, true, fmt.Errorf("images fallback upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, readErr := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if readErr != nil {
		return nil, true, readErr
	}
	if resp.StatusCode >= 400 {
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, truncateString(string(respBody), 2048))
		return nil, true, &UpstreamFailoverError{
			StatusCode:   resp.StatusCode,
			ResponseBody: respBody,
		}
	}

	results := make([]openAIResponsesImageResult, 0, parsed.N)
	for _, item := range gjson.GetBytes(respBody, "data").Array() {
		result := strings.TrimSpace(item.Get("b64_json").String())
		if result == "" {
			continue
		}
		results = append(results, openAIResponsesImageResult{
			Result:        result,
			RevisedPrompt: strings.TrimSpace(item.Get("revised_prompt").String()),
			OutputFormat:  firstNonEmptyString(gjson.GetBytes(fallbackBody, "output_format").String(), "png"),
			Size:          parsed.Size,
			Model:         upstreamModel,
		})
	}
	if len(results) == 0 {
		return nil, true, fmt.Errorf("images fallback returned no images")
	}
	usage, _ := extractOpenAIUsageFromJSONBytes(respBody)
	if err := s.writeResponsesImageFallbackResult(c, reqModel, reqStream, results, usage); err != nil {
		return nil, true, err
	}
	firstTokenMs := int(time.Since(startTime).Milliseconds())
	return &OpenAIForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            usage,
		Model:            reqModel,
		UpstreamModel:    upstreamModel,
		Stream:           reqStream,
		ResponseHeaders:  resp.Header.Clone(),
		Duration:         time.Since(startTime),
		FirstTokenMs:     &firstTokenMs,
		ImageCount:       len(results),
		ImageSize:        imageSizeTier,
		ImageInputSize:   imageInputSize,
		ImageOutputSizes: openAIResponsesImageResultSizes(results),
		BillingModel:     firstNonEmptyString(imageBillingModel, parsed.Model),
	}, true, nil
}

func buildResponsesImageFallbackResponse(model string, stream bool, results []openAIResponsesImageResult, usage OpenAIUsage) (int, string, []byte, error) {
	responseID := "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	output := make([]map[string]any, 0, len(results))
	for i, img := range results {
		item := map[string]any{
			"id":            fmt.Sprintf("ig_%s_%d", responseID, i),
			"type":          "image_generation_call",
			"status":        "completed",
			"result":        img.Result,
			"output_format": firstNonEmptyString(img.OutputFormat, "png"),
		}
		if img.RevisedPrompt != "" {
			item["revised_prompt"] = img.RevisedPrompt
		}
		if img.Size != "" {
			item["size"] = img.Size
		}
		output = append(output, item)
	}
	response := map[string]any{
		"id":     responseID,
		"object": "response",
		"model":  model,
		"status": "completed",
		"output": output,
		"usage":  usage,
	}
	if !stream {
		raw, err := json.Marshal(response)
		if err != nil {
			return 0, "", nil, err
		}
		return http.StatusOK, "application/json; charset=utf-8", raw, nil
	}

	var buf bytes.Buffer
	writeEvent := func(eventType string, payload any) error {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if eventType != "" {
			if _, err := fmt.Fprintf(&buf, "event: %s\n", eventType); err != nil {
				return err
			}
		}
		_, err = fmt.Fprintf(&buf, "data: %s\n\n", raw)
		return err
	}
	created := map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":     responseID,
			"object": "response",
			"model":  model,
			"status": "in_progress",
			"output": []any{},
		},
	}
	if err := writeEvent("response.created", created); err != nil {
		return 0, "", nil, err
	}
	for i, item := range output {
		payload := map[string]any{
			"type":         "response.output_item.done",
			"output_index": i,
			"item":         item,
		}
		if err := writeEvent("response.output_item.done", payload); err != nil {
			return 0, "", nil, err
		}
	}
	completed := map[string]any{
		"type":     "response.completed",
		"response": response,
	}
	if err := writeEvent("response.completed", completed); err != nil {
		return 0, "", nil, err
	}
	if _, err := fmt.Fprint(&buf, "data: [DONE]\n\n"); err != nil {
		return 0, "", nil, err
	}
	return http.StatusOK, "text/event-stream", buf.Bytes(), nil
}

func writeResponsesImageFallbackResponse(c *gin.Context, stream bool, statusCode int, contentType string, body []byte) error {
	if c == nil {
		return nil
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/json; charset=utf-8"
	}
	if stream {
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
	}
	c.Data(statusCode, contentType, body)
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func (s *OpenAIGatewayService) writeResponsesImageFallbackResult(c *gin.Context, model string, stream bool, results []openAIResponsesImageResult, usage OpenAIUsage) error {
	statusCode, contentType, body, err := buildResponsesImageFallbackResponse(model, stream, results, usage)
	if err != nil {
		return err
	}
	return writeResponsesImageFallbackResponse(c, stream, statusCode, contentType, body)
}

func buildOpenAIImagesAPIResponse(
	results []openAIResponsesImageResult,
	createdAt int64,
	usageRaw []byte,
	firstMeta openAIResponsesImageResult,
	responseFormat string,
) ([]byte, error) {
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	out := []byte(`{"created":0,"data":[]}`)
	out, _ = sjson.SetBytes(out, "created", createdAt)

	format := strings.ToLower(strings.TrimSpace(responseFormat))
	if format == "" {
		format = "b64_json"
	}
	for _, img := range results {
		item := []byte(`{}`)
		if format == "url" {
			item, _ = sjson.SetBytes(item, "url", "data:"+openAIImageOutputMIMEType(img.OutputFormat)+";base64,"+img.Result)
		} else {
			item, _ = sjson.SetBytes(item, "b64_json", img.Result)
		}
		if img.RevisedPrompt != "" {
			item, _ = sjson.SetBytes(item, "revised_prompt", img.RevisedPrompt)
		}
		out, _ = sjson.SetRawBytes(out, "data.-1", item)
	}
	if firstMeta.Background != "" {
		out, _ = sjson.SetBytes(out, "background", firstMeta.Background)
	}
	if firstMeta.OutputFormat != "" {
		out, _ = sjson.SetBytes(out, "output_format", firstMeta.OutputFormat)
	}
	if firstMeta.Quality != "" {
		out, _ = sjson.SetBytes(out, "quality", firstMeta.Quality)
	}
	if firstMeta.Size != "" {
		out, _ = sjson.SetBytes(out, "size", firstMeta.Size)
	}
	if firstMeta.Model != "" {
		out, _ = sjson.SetBytes(out, "model", firstMeta.Model)
	}
	if len(usageRaw) > 0 && gjson.ValidBytes(usageRaw) {
		out, _ = sjson.SetRawBytes(out, "usage", usageRaw)
	}
	return out, nil
}

func openAIImagesStreamPrefix(parsed *OpenAIImagesRequest) string {
	if parsed != nil && parsed.IsEdits() {
		return "image_edit"
	}
	return "image_generation"
}

func buildOpenAIImagesStreamErrorBody(message string) []byte {
	body := []byte(`{"type":"error","error":{"type":"upstream_error","message":""}}`)
	if strings.TrimSpace(message) == "" {
		message = "upstream request failed"
	}
	body, _ = sjson.SetBytes(body, "error.message", message)
	return body
}

func buildOpenAIImagesStreamErrorBodyFromUpstream(err *OpenAIImagesUpstreamError) []byte {
	if err == nil {
		return buildOpenAIImagesStreamErrorBody("")
	}
	body := buildOpenAIImagesStreamErrorBody(err.clientMessage())
	body, _ = sjson.SetBytes(body, "error.type", err.clientErrorType())
	if code := strings.TrimSpace(err.Code); code != "" {
		body, _ = sjson.SetBytes(body, "error.code", code)
	}
	if param := strings.TrimSpace(err.Param); param != "" {
		body, _ = sjson.SetBytes(body, "error.param", param)
	}
	return body
}

func writeOpenAIImagesUpstreamErrorResponse(c *gin.Context, err *OpenAIImagesUpstreamError) bool {
	if c == nil || c.Writer == nil || c.Writer.Written() || err == nil {
		return false
	}
	errorObj := gin.H{
		"type":    err.clientErrorType(),
		"message": err.clientMessage(),
	}
	if code := strings.TrimSpace(err.Code); code != "" {
		errorObj["code"] = code
	}
	if param := strings.TrimSpace(err.Param); param != "" {
		errorObj["param"] = param
	}
	c.JSON(err.clientStatusCode(), gin.H{
		"error": errorObj,
	})
	return true
}

func (s *OpenAIGatewayService) writeOpenAIImagesStreamEvent(c *gin.Context, flusher http.Flusher, eventName string, payload []byte) error {
	if strings.TrimSpace(eventName) != "" {
		if _, err := fmt.Fprintf(c.Writer, "event: %s\n", eventName); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (s *OpenAIGatewayService) tryWriteOpenAIImagesStreamEvent(
	c *gin.Context,
	flusher http.Flusher,
	clientDisconnected *bool,
	lastWriteAt *time.Time,
	eventName string,
	payload []byte,
) bool {
	if clientDisconnected != nil && *clientDisconnected {
		return false
	}
	if err := s.writeOpenAIImagesStreamEvent(c, flusher, eventName, payload); err != nil {
		if clientDisconnected != nil {
			*clientDisconnected = true
		}
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images stream client disconnected, continue draining upstream for billing")
		return false
	}
	if lastWriteAt != nil {
		*lastWriteAt = time.Now()
	}
	return true
}

func (s *OpenAIGatewayService) handleOpenAIImagesOAuthNonStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	responseFormat string,
	fallbackModel string,
) (OpenAIUsage, int, []string, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{}, 0, nil, err
	}

	var usage OpenAIUsage
	forEachOpenAISSEDataPayload(string(body), func(data []byte) {
		s.parseSSEUsageBytes(data, &usage)
	})
	results, createdAt, usageRaw, firstMeta, _, err := collectOpenAIImagesFromResponsesBody(body)
	if err != nil {
		return OpenAIUsage{}, 0, nil, err
	}
	if len(results) == 0 {
		if upstreamErr := extractOpenAIImagesUpstreamError(body); upstreamErr != nil {
			setOpsUpstreamError(c, upstreamErr.clientStatusCode(), upstreamErr.clientMessage(), "")
			writeOpenAIImagesUpstreamErrorResponse(c, upstreamErr)
			return OpenAIUsage{}, 0, nil, upstreamErr
		}
		return OpenAIUsage{}, 0, nil, fmt.Errorf("upstream did not return image output")
	}
	if strings.TrimSpace(firstMeta.Model) == "" {
		firstMeta.Model = strings.TrimSpace(fallbackModel)
	}

	responseBody, err := buildOpenAIImagesAPIResponse(results, createdAt, usageRaw, firstMeta, responseFormat)
	if err != nil {
		return OpenAIUsage{}, 0, nil, err
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Data(resp.StatusCode, "application/json; charset=utf-8", responseBody)
	return usage, len(results), openAIResponsesImageResultSizes(results), nil
}

func (s *OpenAIGatewayService) handleOpenAIImagesOAuthStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	startTime time.Time,
	responseFormat string,
	streamPrefix string,
	fallbackModel string,
) (OpenAIUsage, int, []string, *int, error) {
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return OpenAIUsage{}, 0, nil, nil, fmt.Errorf("streaming is not supported by response writer")
	}

	format := strings.ToLower(strings.TrimSpace(responseFormat))
	if format == "" {
		format = "b64_json"
	}

	usage := OpenAIUsage{}
	imageCount := 0
	var imageOutputSizes []string
	var firstTokenMs *int
	emitted := make(map[string]struct{})
	pendingResults := make([]openAIResponsesImageResult, 0, 1)
	pendingSeen := make(map[string]struct{})
	streamMeta := openAIResponsesImageResult{Model: strings.TrimSpace(fallbackModel)}
	var createdAt int64
	clientDisconnected := false
	lastDownstreamWriteAt := time.Now()
	var sseData openAISSEDataAccumulator
	var processDataErr error
	processDataDone := false

	processData := func(dataBytes []byte) {
		if processDataDone || processDataErr != nil {
			return
		}
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		s.parseSSEUsageBytes(dataBytes, &usage)
		if !gjson.ValidBytes(dataBytes) {
			return
		}
		if meta, eventCreatedAt, ok := extractOpenAIResponsesImageMetaFromLifecycleEvent(dataBytes); ok {
			mergeOpenAIResponsesImageMeta(&streamMeta, meta)
			if eventCreatedAt > 0 {
				createdAt = eventCreatedAt
			}
		}
		switch gjson.GetBytes(dataBytes, "type").String() {
		case "response.image_generation_call.partial_image":
			b64 := strings.TrimSpace(gjson.GetBytes(dataBytes, "partial_image_b64").String())
			if b64 == "" {
				return
			}
			eventName := streamPrefix + ".partial_image"
			partialMeta := streamMeta
			mergeOpenAIResponsesImageMeta(&partialMeta, openAIResponsesImageResult{
				OutputFormat: strings.TrimSpace(gjson.GetBytes(dataBytes, "output_format").String()),
				Background:   strings.TrimSpace(gjson.GetBytes(dataBytes, "background").String()),
			})
			payload := buildOpenAIImagesStreamPartialPayload(
				eventName,
				b64,
				gjson.GetBytes(dataBytes, "partial_image_index").Int(),
				format,
				createdAt,
				partialMeta,
			)
			s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, eventName, payload)
		case "response.output_item.done":
			img, itemID, ok, extractErr := extractOpenAIImageFromResponsesOutputItemDone(dataBytes)
			if extractErr != nil {
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(extractErr.Error()))
				processDataErr = extractErr
				processDataDone = true
				return
			}
			if !ok {
				return
			}
			mergeOpenAIResponsesImageMeta(&streamMeta, img)
			mergeOpenAIResponsesImageMeta(&img, streamMeta)
			key := openAIResponsesImageResultKey(itemID, img)
			if _, exists := emitted[key]; exists {
				return
			}
			if _, exists := pendingSeen[key]; exists {
				return
			}
			pendingSeen[key] = struct{}{}
			pendingResults = append(pendingResults, img)
		case "response.completed":
			results, _, usageRaw, firstMeta, extractErr := extractOpenAIImagesFromResponsesCompleted(dataBytes)
			if extractErr != nil {
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(extractErr.Error()))
				processDataErr = extractErr
				processDataDone = true
				return
			}
			mergeOpenAIResponsesImageMeta(&streamMeta, firstMeta)
			finalResults := make([]openAIResponsesImageResult, 0, len(results)+len(pendingResults))
			finalSeen := make(map[string]struct{})
			for _, img := range results {
				mergeOpenAIResponsesImageMeta(&img, streamMeta)
				appendOpenAIResponsesImageResultDedup(&finalResults, finalSeen, "", img)
			}
			for _, img := range pendingResults {
				mergeOpenAIResponsesImageMeta(&img, streamMeta)
				appendOpenAIResponsesImageResultDedup(&finalResults, finalSeen, "", img)
			}
			if len(finalResults) == 0 {
				outputErr := fmt.Errorf("upstream did not return image output")
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(outputErr.Error()))
				processDataErr = outputErr
				processDataDone = true
				return
			}
			eventName := streamPrefix + ".completed"
			for _, img := range finalResults {
				key := openAIResponsesImageResultKey("", img)
				if _, exists := emitted[key]; exists {
					continue
				}
				payload := buildOpenAIImagesStreamCompletedPayload(eventName, img, format, createdAt, usageRaw)
				emitted[key] = struct{}{}
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, eventName, payload)
			}
			imageCount = len(emitted)
			imageOutputSizes = openAIResponsesImageResultSizes(finalResults)
			processDataDone = true
		case "error", "response.failed":
			if upstreamErr := openAIImagesUpstreamErrorFromSSEPayload(dataBytes); upstreamErr != nil {
				if !clientDisconnected {
					s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBodyFromUpstream(upstreamErr))
				}
				setOpsUpstreamError(c, upstreamErr.clientStatusCode(), upstreamErr.clientMessage(), "")
				processDataErr = upstreamErr
				processDataDone = true
				return
			}
		}
	}

	processLine := func(line []byte) (bool, error) {
		if len(line) == 0 {
			return false, nil
		}
		sseData.AddLine(string(line), processData)
		if processDataErr != nil {
			return true, processDataErr
		}
		return processDataDone, nil
	}

	flushData := func() (bool, error) {
		sseData.Flush(processData)
		if processDataErr != nil {
			return true, processDataErr
		}
		return processDataDone, nil
	}

	finalizePending := func() error {
		if imageCount > 0 {
			return nil
		}
		if len(pendingResults) > 0 {
			eventName := streamPrefix + ".completed"
			for _, img := range pendingResults {
				mergeOpenAIResponsesImageMeta(&img, streamMeta)
				key := openAIResponsesImageResultKey("", img)
				if _, exists := emitted[key]; exists {
					continue
				}
				payload := buildOpenAIImagesStreamCompletedPayload(eventName, img, format, createdAt, nil)
				emitted[key] = struct{}{}
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, eventName, payload)
			}
			imageCount = len(emitted)
			imageOutputSizes = openAIResponsesImageResultSizes(pendingResults)
			return nil
		}

		streamErr := fmt.Errorf("stream disconnected before image generation completed")
		s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(streamErr.Error()))
		return streamErr
	}

	streamInterval := s.openAIImageStreamDataInterval()
	keepaliveInterval := s.openAIImageStreamKeepaliveInterval()
	if streamInterval <= 0 && keepaliveInterval <= 0 {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			done, processErr := processLine(line)
			if processErr != nil {
				return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
			}
			if done {
				return usage, imageCount, imageOutputSizes, firstTokenMs, nil
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				if done, processErr := flushData(); processErr != nil {
					return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
				} else if done {
					return usage, imageCount, imageOutputSizes, firstTokenMs, nil
				}
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(err.Error()))
				return usage, imageCount, imageOutputSizes, firstTokenMs, err
			}
		}
		if done, processErr := flushData(); processErr != nil {
			return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
		} else if done {
			return usage, imageCount, imageOutputSizes, firstTokenMs, nil
		}
		if err := finalizePending(); err != nil {
			return usage, imageCount, imageOutputSizes, firstTokenMs, err
		}
		return usage, imageCount, imageOutputSizes, firstTokenMs, nil
	}

	type readEvent struct {
		line []byte
		err  error
	}
	events := make(chan readEvent, 16)
	done := make(chan struct{})
	sendEvent := func(ev readEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func() {
		defer close(events)
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			}
			if len(line) > 0 && !sendEvent(readEvent{line: line}) {
				return
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				_ = sendEvent(readEvent{err: err})
				return
			}
		}
	}()
	defer close(done)

	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
	}

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if done, processErr := flushData(); processErr != nil {
					return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
				} else if done {
					return usage, imageCount, imageOutputSizes, firstTokenMs, nil
				}
				if err := finalizePending(); err != nil {
					return usage, imageCount, imageOutputSizes, firstTokenMs, err
				}
				return usage, imageCount, imageOutputSizes, firstTokenMs, nil
			}
			if ev.err != nil {
				if done, processErr := flushData(); processErr != nil {
					return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
				} else if done {
					return usage, imageCount, imageOutputSizes, firstTokenMs, nil
				}
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(ev.err.Error()))
				return usage, imageCount, imageOutputSizes, firstTokenMs, ev.err
			}
			done, processErr := processLine(ev.line)
			if processErr != nil {
				return usage, imageCount, imageOutputSizes, firstTokenMs, processErr
			}
			if done {
				return usage, imageCount, imageOutputSizes, firstTokenMs, nil
			}
		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return usage, imageCount, imageOutputSizes, firstTokenMs, fmt.Errorf("image stream incomplete after timeout")
			}
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images responses stream data interval timeout: interval=%s", streamInterval)
			s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(fmt.Sprintf("upstream image stream idle for %s", streamInterval)))
			return usage, imageCount, imageOutputSizes, firstTokenMs, fmt.Errorf("image stream data interval timeout")
		case <-keepaliveCh:
			if clientDisconnected || time.Since(lastDownstreamWriteAt) < keepaliveInterval {
				continue
			}
			if _, writeErr := io.WriteString(c.Writer, ":\n\n"); writeErr != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images responses stream client disconnected during keepalive, continue draining upstream for billing")
				continue
			}
			flusher.Flush()
			lastDownstreamWriteAt = time.Now()
		}
	}
}

func (s *OpenAIGatewayService) forwardOpenAIImagesOAuth(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	if requestModel == "" {
		requestModel = "gpt-image-2"
	}
	if err := validateOpenAIImagesModel(requestModel); err != nil {
		return nil, err
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Images request routing request_model=%s endpoint=%s account_type=%s uploads=%d",
		requestModel,
		parsed.Endpoint,
		account.Type,
		len(parsed.Uploads),
	)
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}

	responsesBody, err := buildOpenAIImagesResponsesRequest(parsed, requestModel)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildUpstreamRequest(upstreamCtx, c, account, responsesBody, token, true, parsed.StickySessionSeed(), false)
	if err != nil {
		return nil, err
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "text/event-stream")

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			s.handleFailoverSideEffects(upstreamCtx, resp, account)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleErrorResponse(upstreamCtx, resp, c, account, responsesBody)
	}
	defer func() { _ = resp.Body.Close() }()

	var (
		usage            OpenAIUsage
		imageCount       int
		imageOutputSizes []string
		firstTokenMs     *int
	)
	if parsed.Stream {
		usage, imageCount, imageOutputSizes, firstTokenMs, err = s.handleOpenAIImagesOAuthStreamingResponse(resp, c, startTime, parsed.ResponseFormat, openAIImagesStreamPrefix(parsed), requestModel)
		if err != nil {
			if imageCount > 0 {
				return &OpenAIForwardResult{
					RequestID:        resp.Header.Get("x-request-id"),
					Usage:            usage,
					Model:            requestModel,
					UpstreamModel:    requestModel,
					Stream:           parsed.Stream,
					ResponseHeaders:  resp.Header.Clone(),
					Duration:         time.Since(startTime),
					FirstTokenMs:     firstTokenMs,
					ImageCount:       imageCount,
					ImageSize:        parsed.SizeTier,
					ImageInputSize:   parsed.Size,
					ImageOutputSizes: imageOutputSizes,
				}, err
			}
			return nil, err
		}
	} else {
		usage, imageCount, imageOutputSizes, err = s.handleOpenAIImagesOAuthNonStreamingResponse(resp, c, parsed.ResponseFormat, requestModel)
		if err != nil {
			return nil, err
		}
	}
	if imageCount <= 0 {
		imageCount = parsed.N
	}
	return &OpenAIForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            usage,
		Model:            requestModel,
		UpstreamModel:    requestModel,
		Stream:           parsed.Stream,
		ResponseHeaders:  resp.Header.Clone(),
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ImageCount:       imageCount,
		ImageSize:        parsed.SizeTier,
		ImageInputSize:   parsed.Size,
		ImageOutputSizes: imageOutputSizes,
	}, nil
}
