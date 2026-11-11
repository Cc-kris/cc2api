package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// V03-T15 / R09：移植上游 b6eb6c1ef、917dbdc70 的 gemini_image_output_accounting_test.go，
// 加上 CCAI 的已送达计数与 AC28。

const geminiTestPNG = "iVBORw0KGgoAAAANSUhEUg=="

func newGeminiImageTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/nana-banana-2:generateContent", strings.NewReader("{}"))
	return c
}

func geminiImageResponse(parts string) string {
	return `{"candidates":[{"content":{"role":"model","parts":[` + parts + `]},"finishReason":"STOP"}]}`
}

func geminiInlinePart(mime string) string {
	return `{"inlineData":{"mimeType":"` + mime + `","data":"` + geminiTestPNG + `"}}`
}

func TestCountGeminiImageOutputs(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    int
	}{
		{name: "camelCase inlineData", payload: geminiImageResponse(geminiInlinePart("image/png")), want: 1},
		{name: "snake_case inline_data", payload: geminiImageResponse(`{"inline_data":{"mime_type":"image/png","data":"` + geminiTestPNG + `"}}`), want: 1},
		{name: "text and image mixed", payload: geminiImageResponse(`{"text":"here you go"},` + geminiInlinePart("image/jpeg")), want: 1},
		{name: "multiple images", payload: geminiImageResponse(geminiInlinePart("image/png") + `,` + geminiInlinePart("image/webp")), want: 2},
		{name: "uppercase mime type", payload: geminiImageResponse(geminiInlinePart("IMAGE/PNG")), want: 1},
		{name: "text only", payload: geminiImageResponse(`{"text":"no image here"}`), want: 0},
		{name: "non image mime type", payload: geminiImageResponse(geminiInlinePart("audio/mpeg")), want: 0},
		{name: "empty data is not billable", payload: geminiImageResponse(`{"inlineData":{"mimeType":"image/png","data":""}}`), want: 0},
		{name: "camelCase fileData", payload: geminiImageResponse(`{"fileData":{"mimeType":"image/jpeg","fileUri":"https://example.test/image.jpg"}}`), want: 1},
		{name: "snake_case file_data", payload: geminiImageResponse(`{"file_data":{"mime_type":"image/png","file_uri":"gs://bucket/image.png"}}`), want: 1},
		{name: "mixed inline and referenced images", payload: geminiImageResponse(geminiInlinePart("image/png") + `,{"fileData":{"mimeType":"image/webp","fileUri":"https://example.test/image.webp"}}`), want: 2},
		{name: "a part is counted once", payload: geminiImageResponse(`{"inlineData":{"mimeType":"image/png","data":"` + geminiTestPNG + `"},"fileData":{"mimeType":"image/png","fileUri":"https://example.test/image.png"}}`), want: 1},
		{name: "non image file", payload: geminiImageResponse(`{"fileData":{"mimeType":"application/pdf","fileUri":"https://example.test/file.pdf"}}`), want: 0},
		{name: "empty file URI", payload: geminiImageResponse(`{"fileData":{"mimeType":"image/png","fileUri":"  "}}`), want: 0},
		{name: "missing file URI", payload: geminiImageResponse(`{"fileData":{"mimeType":"image/png"}}`), want: 0},
		{name: "non string file URI", payload: geminiImageResponse(`{"fileData":{"mimeType":"image/png","fileUri":123}}`), want: 0},
		{name: "empty payload", payload: "", want: 0},
		{name: "invalid json", payload: "not-json", want: 0},
		{name: "error response", payload: `{"error":{"code":429,"message":"quota"}}`, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, countGeminiImageOutputs([]byte(tc.payload)))
		})
	}
}

// 累积式 SSE 会把同一张图在后续 chunk 里整段重发；计数器取单个 payload 内的最大值。
func TestObserveGeminiImageOutputs_CumulativeChunksDoNotDoubleCount(t *testing.T) {
	c := newGeminiImageTestContext(t)
	beginGeminiImageOutputObservation(c)
	oneImage := []byte(geminiImageResponse(geminiInlinePart("image/png")))
	for range 4 {
		observeGeminiImageOutputs(c, oneImage)
	}
	require.Equal(t, 1, observedGeminiImageOutputs(c))
}

func TestObserveGeminiImageOutputs_KeepsLargestChunk(t *testing.T) {
	c := newGeminiImageTestContext(t)
	beginGeminiImageOutputObservation(c)
	observeGeminiImageOutputs(c, []byte(geminiImageResponse(`{"text":"working"}`)))
	observeGeminiImageOutputs(c, []byte(geminiImageResponse(geminiInlinePart("image/png")+`,`+geminiInlinePart("image/png"))))
	observeGeminiImageOutputs(c, []byte(`{"usageMetadata":{"promptTokenCount":9}}`))
	require.Equal(t, 2, observedGeminiImageOutputs(c))
}

// failover 会拿同一个 gin.Context 重跑 Forward，计数器必须按次重置。
func TestBeginGeminiImageOutputObservation_ResetsPerForward(t *testing.T) {
	c := newGeminiImageTestContext(t)
	oneImage := []byte(geminiImageResponse(geminiInlinePart("image/png")))
	beginGeminiImageOutputObservation(c)
	observeGeminiImageOutputs(c, oneImage)
	markGeminiImageOutputsDelivered(c, 1)
	require.Equal(t, 1, observedGeminiImageOutputs(c))

	beginGeminiImageOutputObservation(c)
	require.Zero(t, observedGeminiImageOutputs(c))
	require.Zero(t, deliveredGeminiImageOutputs(c))
}

func TestResolveGeminiImageCount(t *testing.T) {
	oneImage := []byte(geminiImageResponse(geminiInlinePart("image/png")))
	textOnly := []byte(geminiImageResponse(`{"text":"hello"}`))

	t.Run("custom model name bills by observed images", func(t *testing.T) {
		c := newGeminiImageTestContext(t)
		beginGeminiImageOutputObservation(c)
		observeGeminiImageOutputs(c, oneImage)
		require.False(t, isImageGenerationModel("nana-banana-2"))
		require.Equal(t, 1, resolveGeminiImageCount(c, "nana-banana-2", "nana-banana-2"))
	})
	t.Run("falls back to requested model name", func(t *testing.T) {
		c := newGeminiImageTestContext(t)
		beginGeminiImageOutputObservation(c)
		observeGeminiImageOutputs(c, textOnly)
		require.Equal(t, 1, resolveGeminiImageCount(c, "gemini-3-pro-image-preview", "gemini-3-pro-image-preview"))
	})
	t.Run("falls back to mapped upstream model name", func(t *testing.T) {
		c := newGeminiImageTestContext(t)
		beginGeminiImageOutputObservation(c)
		observeGeminiImageOutputs(c, textOnly)
		require.Equal(t, 1, resolveGeminiImageCount(c, "my-image-alias", "gemini-2.5-flash-image"))
	})
	t.Run("text model stays unbilled", func(t *testing.T) {
		c := newGeminiImageTestContext(t)
		beginGeminiImageOutputObservation(c)
		observeGeminiImageOutputs(c, textOnly)
		require.Zero(t, resolveGeminiImageCount(c, "gemini-2.5-pro", "gemini-2.5-pro"))
	})
	t.Run("no counter on context degrades to name heuristic", func(t *testing.T) {
		c := newGeminiImageTestContext(t)
		require.Zero(t, resolveGeminiImageCount(c, "nana-banana-2", "nana-banana-2"))
		require.Equal(t, 1, resolveGeminiImageCount(c, "gemini-3-pro-image", "gemini-3-pro-image"))
	})
}

// 客户端在结束分片前断开：只计已送达的图片；结束后断开或未断开按实际观测。
func TestResolveGeminiStreamImageCount_DisconnectBillsDeliveredOnly(t *testing.T) {
	c := newGeminiImageTestContext(t)
	beginGeminiImageOutputObservation(c)
	observeGeminiImageOutputs(c, []byte(geminiImageResponse(geminiInlinePart("image/png")+`,`+geminiInlinePart("image/png"))))
	markGeminiImageOutputsDelivered(c, 1)

	require.Equal(t, 1, resolveGeminiStreamImageCount(c, "gemini-3-pro-image", "gemini-3-pro-image", true, false))
	require.Equal(t, 2, resolveGeminiStreamImageCount(c, "gemini-3-pro-image", "gemini-3-pro-image", true, true))
	require.Equal(t, 2, resolveGeminiStreamImageCount(c, "gemini-3-pro-image", "gemini-3-pro-image", false, false))

	// 断开前一张都没送达：不按模型名兜底成 1 张。
	c2 := newGeminiImageTestContext(t)
	beginGeminiImageOutputObservation(c2)
	require.Zero(t, resolveGeminiStreamImageCount(c2, "gemini-3-pro-image", "gemini-3-pro-image", true, false))
}

func TestHandleNativeNonStreamingResponse_FeedsImageCounter(t *testing.T) {
	c := newGeminiImageTestContext(t)
	beginGeminiImageOutputObservation(c)
	body := geminiImageResponse(`{"file_data":{"mime_type":"image/jpeg","file_uri":"https://example.test/one.jpg"}},` + geminiInlinePart("image/png"))
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
	usage, err := (&GeminiMessagesCompatService{}).handleNativeNonStreamingResponse(c, resp, false)
	require.NoError(t, err)
	require.NotNil(t, usage)
	const model = "gemini-nano-banana-2.1"
	require.False(t, isImageGenerationModel(model))
	require.Equal(t, 2, resolveGeminiImageCount(c, model, model))
}

// ---------------- AC28 end to end ----------------

func newGeminiImageForwardService(resp *http.Response) *GeminiMessagesCompatService {
	return &GeminiMessagesCompatService{
		httpUpstream:         &geminiCompatHTTPUpstreamStub{response: resp},
		cfg:                  &config.Config{},
		responseHeaderFilter: compileResponseHeaderFilter(&config.Config{}),
	}
}

func geminiImageTestAccount() *Account {
	return &Account{ID: 281, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "gemini-api-key"}, Concurrency: 1}
}

const ac28RequestBody = `{"contents":[{"role":"user","parts":[{"text":"draw three cats"}]}],"generationConfig":{"imageConfig":{"imageSize":"1K"}}}`

// AC28：gemini-3-pro-image-preview，未配置分组图片价；响应含 2 个 inlineData 和 1 个 fileData
// → image_count=3，费用 3 × 0.134 = 0.402（修复前写死 1 张 0.134）。
func TestGeminiForwardNative_AC28_CountsInlineAndFileDataImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const model = "gemini-3-pro-image-preview"
	upstream := geminiImageResponse(geminiInlinePart("image/png") + `,` + geminiInlinePart("image/png") +
		`,{"fileData":{"mimeType":"image/png","fileUri":"https://example.test/three.png"}}`)
	svc := newGeminiImageForwardService(&http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(upstream)),
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/"+model+":generateContent", bytes.NewReader([]byte(ac28RequestBody)))

	result, err := svc.ForwardNative(context.Background(), c, geminiImageTestAccount(), model, "generateContent", false, []byte(ac28RequestBody))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 3, result.ImageCount)
	require.Equal(t, "1K", result.ImageSize)

	repo := &tierUsageLogRepo{}
	gw := newGatewayRecordUsageServiceForFastTest(repo)
	billing, _ := newShippedCatalogBillingService(t)
	gw.billingService = billing
	groupID := int64(28)
	require.NoError(t, gw.RecordUsage(context.Background(), &RecordUsageInput{
		Result:  result,
		APIKey:  &APIKey{ID: 1, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformGemini, RateMultiplier: 1, AllowImageGeneration: true}},
		User:    &User{ID: 1},
		Account: geminiImageTestAccount(),
	}))
	require.NotNil(t, repo.lastLog)
	require.Equal(t, 3, repo.lastLog.ImageCount)
	require.InDelta(t, 0.402, repo.lastLog.ActualCost, 1e-9)
}

// 原生流：写客户端失败前送达的图片计入 delivered，写失败那一块不算送达。
type geminiFailingWriter struct {
	gin.ResponseWriter
	failOn string
}

func (w *geminiFailingWriter) Write(b []byte) (int, error) {
	if strings.Contains(string(b), w.failOn) {
		return 0, errors.New("client gone")
	}
	return w.ResponseWriter.Write(b)
}

func (w *geminiFailingWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func TestGeminiNativeStream_MarksDeliveredImagesOnlyAfterSuccessfulWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	first := geminiImageResponse(geminiInlinePart("image/png"))
	second := strings.Replace(geminiImageResponse(geminiInlinePart("image/png")+`,`+geminiInlinePart("image/png")), "STOP", "STOP_FAILME", 1)
	sse := "data: " + first + "\n\n" + "data: " + second + "\n\n"
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(sse))}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-pro-image:streamGenerateContent?alt=sse", strings.NewReader("{}"))
	c.Writer = &geminiFailingWriter{ResponseWriter: c.Writer, failOn: "STOP_FAILME"}
	beginGeminiImageOutputObservation(c)

	svc := &GeminiMessagesCompatService{cfg: &config.Config{}}
	result, err := svc.handleNativeStreamingResponse(c, resp, time.Now(), false)
	require.NoError(t, err, "V03-T05 keeps draining upstream after a client write failure")
	require.True(t, result.clientDisconnect)
	require.Equal(t, 2, observedGeminiImageOutputs(c))
	require.Equal(t, 1, deliveredGeminiImageOutputs(c), "写失败的那一块不算送达")
	require.Equal(t, 1, resolveGeminiStreamImageCount(c, "gemini-3-pro-image", "gemini-3-pro-image", true, false))
}
