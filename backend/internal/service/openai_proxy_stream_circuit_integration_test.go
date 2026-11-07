package service

import (
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

// V02-T06：代理断流熔断接入调度和流式转发。上游用例按 CCAI 的函数签名改写。

type proxyCircuitReadThenErrorCloser struct {
	reader *strings.Reader
	err    error
}

func (r *proxyCircuitReadThenErrorCloser) Read(p []byte) (int, error) {
	if r.reader != nil && r.reader.Len() > 0 {
		return r.reader.Read(p)
	}
	return 0, r.err
}

func (r *proxyCircuitReadThenErrorCloser) Close() error { return nil }

func newTestProxyStreamCircuit(threshold int) *openAIProxyStreamCircuit {
	// collapseInterval 0：测试里的连续断流间隔远小于 3 秒，必须当作独立事件计数。
	return newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: threshold,
		failureWindow:    time.Minute,
		quarantineTTL:    10 * time.Minute,
		maxEntries:       16,
	})
}

// AC41：拷自上游 47ad29db3，调度器第一轮排除被隔离代理下的账号。
func TestOpenAIGatewayService_SelectAccountWithScheduler_SkipsQuarantinedSharedProxy(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	proxyA := int64(4698)
	proxyB := int64(4699)
	accounts := []Account{
		{ID: 469801, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, ProxyID: &proxyA},
		{ID: 469802, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, ProxyID: &proxyA},
		{ID: 469803, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5, ProxyID: &proxyB},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:              schedulerTestOpenAIAccountRepo{accounts: accounts},
		cfg:                      cfg,
		concurrencyService:       NewConcurrencyService(schedulerTestConcurrencyCache{}),
		openaiProxyStreamCircuit: newTestProxyStreamCircuit(1),
	}
	svc.openaiProxyStreamCircuit.recordFailure(proxyA, time.Now())

	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), nil, "", "", "gpt-5.4", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(469803), selection.Account.ID)
}

// AC42：拷自上游 da49ce3f2，所有候选都在被隔离代理下时调度器路径 fail-open。
func TestOpenAIGatewayService_SelectAccountWithScheduler_FailsOpenWhenAllProxiesQuarantined(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()

	proxyA := int64(5056)
	accounts := []Account{
		{ID: 505601, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, ProxyID: &proxyA},
		{ID: 505602, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5, ProxyID: &proxyA},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:              schedulerTestOpenAIAccountRepo{accounts: accounts},
		cfg:                      cfg,
		concurrencyService:       NewConcurrencyService(schedulerTestConcurrencyCache{}),
		openaiProxyStreamCircuit: newTestProxyStreamCircuit(1),
	}
	tripped, _ := svc.openaiProxyStreamCircuit.recordFailure(proxyA, time.Now())
	require.True(t, tripped)

	selection, _, err := svc.SelectAccountWithScheduler(context.Background(), nil, "", "", "gpt-5.4", nil, OpenAIUpstreamTransportAny, false)
	require.NoError(t, err, "quarantine must fail open instead of returning no available accounts")
	require.NotNil(t, selection)
	require.Equal(t, proxyA, *selection.Account.ProxyID)
	require.True(t, svc.openaiProxyStreamCircuit.isBlocked(proxyA, time.Now()),
		"fail-open must not clear the quarantine; only a completed stream or TTL expiry does")
}

// AC42（CCAI 新增）：旧选号路径（SelectAccountForModelWithExclusions，Codex 模型清单走它）同样 fail-open；
// 有健康代理时仍优先健康代理。
func TestOpenAISelectAccountForModelWithExclusions_ProxyQuarantineFailsOpen(t *testing.T) {
	proxyA := int64(6001)
	proxyB := int64(6002)
	allQuarantined := []Account{
		{ID: 600101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, ProxyID: &proxyA},
	}
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: allQuarantined}, openaiProxyStreamCircuit: newTestProxyStreamCircuit(1)}
	svc.openaiProxyStreamCircuit.recordFailure(proxyA, time.Now())

	acc, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-5.4", nil)
	require.NoError(t, err)
	require.Equal(t, int64(600101), acc.ID)

	mixed := []Account{
		{ID: 600201, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, ProxyID: &proxyA},
		{ID: 600202, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5, ProxyID: &proxyB},
	}
	svc = &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: mixed}, openaiProxyStreamCircuit: newTestProxyStreamCircuit(1)}
	svc.openaiProxyStreamCircuit.recordFailure(proxyA, time.Now())
	acc, err = svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-5.4", nil)
	require.NoError(t, err)
	require.Equal(t, int64(600202), acc.ID, "第一轮必须排除被隔离代理下的账号")
}

// fail-open 只在真有代理被隔离时才重选；普通无号错误原样返回，并且错误能被识别为 ErrNoAvailableAccounts。
func TestOpenAISelectionNoAccountsWithoutQuarantineDoesNotRetry(t *testing.T) {
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{}}
	_, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-5.4", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Equal(t, "no available OpenAI accounts supporting model: gpt-5.4", err.Error())
}

func proxyCircuitStreamResponse(body string, readErr error) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       &proxyCircuitReadThenErrorCloser{reader: strings.NewReader(body), err: readErr},
		Header:     http.Header{"X-Request-Id": []string{"rid-proxy-disconnect"}},
	}
}

const proxyCircuitPartialDelta = "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"

// AC41：拷自上游 47ad29db3，非透传流已有输出后断流计入熔断，下一次调度排除。
func TestOpenAIStreamingPostOutputDisconnectQuarantinesSharedProxyWithoutSameStreamFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxyID := int64(4698)
	account := &Account{ID: 469801, Name: "oauth-on-shared-proxy", Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxyID}
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	svc.openaiProxyStreamCircuit = newTestProxyStreamCircuit(2)

	for _, readErr := range []error{io.ErrUnexpectedEOF, errors.New("http2: client connection lost")} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		_, err := svc.handleStreamingResponse(c.Request.Context(), proxyCircuitStreamResponse(proxyCircuitPartialDelta, readErr), c, account, time.Now(), "gpt-5.4", "gpt-5.4")
		require.Error(t, err)
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverErr), "post-output disconnect must not fail over inside the same stream")
		require.Contains(t, rec.Body.String(), "partial")
	}

	scheduler := &defaultOpenAIAccountScheduler{service: svc}
	require.False(t, scheduler.isAccountRequestCompatible(context.Background(), account, OpenAIAccountScheduleRequest{}),
		"the next request must exclude accounts sharing the quarantined proxy")
}

// 拷自上游 47ad29db3：正常结束、客户端取消都不计入熔断。
func TestOpenAIStreamingTerminalAndClientCancellationDoNotQuarantineProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxyID := int64(4699)
	account := &Account{ID: 469901, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxyID}
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	svc.openaiProxyStreamCircuit = newTestProxyStreamCircuit(2)

	terminalRec := httptest.NewRecorder()
	terminalCtx, _ := gin.CreateTestContext(terminalRec)
	terminalCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	terminalBody := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":3,\"total_tokens\":8}}}\n\n"
	_, err := svc.handleStreamingResponse(terminalCtx.Request.Context(), proxyCircuitStreamResponse(terminalBody, io.ErrUnexpectedEOF), terminalCtx, account, time.Now(), "model", "model")
	require.NoError(t, err)

	for range 2 {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
		_, err = svc.handleStreamingResponse(c.Request.Context(), proxyCircuitStreamResponse("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n", context.Canceled), c, account, time.Now(), "model", "model")
		require.Error(t, err)
	}

	scheduler := &defaultOpenAIAccountScheduler{service: svc}
	require.True(t, scheduler.isAccountRequestCompatible(context.Background(), account, OpenAIAccountScheduleRequest{}))
}

// AC41（CCAI 新增）：开了流式续写的请求断流时先计入熔断，再走 interruptForContinuation 换号续写。
func TestOpenAIStreamingContinuationDisconnectStillQuarantinesProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxyID := int64(4700)
	account := &Account{ID: 470001, Name: "oauth-continuation", Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxyID}
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	svc.openaiProxyStreamCircuit = newTestProxyStreamCircuit(2)

	cases := []struct {
		name    string
		readErr error
	}{
		{name: "scan error", readErr: errors.New("http2: client connection lost")},
		{name: "eof without terminal", readErr: io.EOF},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			sc := AttachStreamContinuation(c)

			_, err := svc.handleStreamingResponse(c.Request.Context(), proxyCircuitStreamResponse(proxyCircuitPartialDelta, tc.readErr), c, account, time.Now(), "gpt-5.4", "gpt-5.4")
			require.Error(t, err)
			require.True(t, sc.Active(), "测试前提：续写已激活（有内容推给客户端）")
			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr), "续写开启时断流应走换号续写")
			require.True(t, failoverErr.SafeToFailoverAfterWrite)
		})
	}

	require.True(t, svc.isOpenAIProxyStreamQuarantined(context.Background(), account),
		"续写路径的断流也必须计入熔断，否则开了续写的分组永远不会触发隔离")
}

// AC41：拷自上游 47ad29db3，透传流已有输出后断流计入熔断。
func TestOpenAIStreamingPassthroughPostOutputDisconnectQuarantinesSharedProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxyID := int64(4698)
	account := &Account{ID: 469804, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, ProxyID: &proxyID}
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	svc.openaiProxyStreamCircuit = newTestProxyStreamCircuit(2)

	for _, readErr := range []error{io.ErrUnexpectedEOF, errors.New("http2: client connection lost")} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), proxyCircuitStreamResponse("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n", readErr), c, account, time.Now(), "model", "model")
		require.Error(t, err)
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverErr), "post-output disconnect must not fail over inside the same stream")
		require.Contains(t, rec.Body.String(), "partial")
	}

	require.True(t, svc.isOpenAIProxyStreamQuarantined(context.Background(), account))
}

// AC43：disabled=true 时断流不隔离；配置读取走 resolveOpenAIProxyStreamCircuitSettings。
func TestOpenAIProxyStreamCircuitDisabledByConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxyID := int64(4701)
	account := &Account{ID: 470101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxyID}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	cfg.Gateway.OpenAIProxyStreamCircuit.Disabled = true
	cfg.Gateway.OpenAIProxyStreamCircuit.FailureThreshold = 1
	svc := &OpenAIGatewayService{cfg: cfg}

	for range 3 {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		_, _ = svc.handleStreamingResponse(c.Request.Context(), proxyCircuitStreamResponse(proxyCircuitPartialDelta, io.ErrUnexpectedEOF), c, account, time.Now(), "m", "m")
	}
	require.False(t, svc.isOpenAIProxyStreamQuarantined(context.Background(), account))
	require.Equal(t, 0, svc.getOpenAIProxyStreamCircuit().activeBlockCount(time.Now()))

	settings := resolveOpenAIProxyStreamCircuitSettings(&OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		OpenAIProxyStreamCircuit: config.GatewayOpenAIProxyStreamCircuitConfig{FailureThreshold: 5, WindowSeconds: 30, TTLSeconds: 120},
	}}})
	require.Equal(t, 5, settings.failureThreshold)
	require.Equal(t, 30*time.Second, settings.failureWindow)
	require.Equal(t, 2*time.Minute, settings.quarantineTTL)
}

// AC42：3 秒内 10 个并发断流只算 1 次（生产默认合并间隔），阈值 2 不触发隔离。
func TestOpenAIProxyStreamCircuitCollapsesTenConcurrentDisconnects(t *testing.T) {
	proxyID := int64(4702)
	account := &Account{ID: 470201, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxyID}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	done := make(chan struct{})
	for range 10 {
		go func() {
			svc.recordOpenAIProxyStreamDisconnect(account, io.ErrUnexpectedEOF, "rid")
			done <- struct{}{}
		}()
	}
	for range 10 {
		<-done
	}
	require.False(t, svc.isOpenAIProxyStreamQuarantined(context.Background(), account))
	circuit := svc.getOpenAIProxyStreamCircuit()
	circuit.mu.Lock()
	defer circuit.mu.Unlock()
	require.Equal(t, 1, circuit.entries[proxyID].failureCount)
}
