package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 本文件提供 V03-R05 流中断计费的 handler 级测试夹具：
// 真实 GatewayHandler.Messages → GatewayService.Forward → RecordUsage → applyUsageBilling，
// 上游、计费仓库、用量记录仓库用桩替换，断言「每个客户端请求恰好扣费一次」。

// streamBillingUpstreamStep 描述某个账号某次上游调用返回的 SSE 片段。
// readErr 非 nil 时在 body 读完后返回该错误（模拟连接中断）；nil 时以 EOF 结束。
type streamBillingUpstreamStep struct {
	body    string
	readErr error
	status  int
}

type streamBillingUpstream struct {
	mu       sync.Mutex
	steps    map[int64][]streamBillingUpstreamStep
	calls    map[int64]int
	requests []streamBillingUpstreamRequest
}

type streamBillingUpstreamRequest struct {
	accountID int64
	body      []byte
}

func newStreamBillingUpstream(steps map[int64][]streamBillingUpstreamStep) *streamBillingUpstream {
	return &streamBillingUpstream{steps: steps, calls: map[int64]int{}}
}

func (u *streamBillingUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	var body []byte
	if req != nil && req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	u.requests = append(u.requests, streamBillingUpstreamRequest{accountID: accountID, body: body})
	idx := u.calls[accountID]
	u.calls[accountID] = idx + 1
	steps := u.steps[accountID]
	if len(steps) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	if idx >= len(steps) {
		idx = len(steps) - 1
	}
	step := steps[idx]
	status := step.status
	if status == 0 {
		status = http.StatusOK
	}
	header := http.Header{"Content-Type": []string{"text/event-stream"}}
	if status != http.StatusOK {
		header.Set("Content-Type", "application/json")
	}
	header.Set("X-Request-Id", "upstream-req")
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       &streamBillingBody{reader: strings.NewReader(step.body), err: step.readErr},
	}, nil
}

func (u *streamBillingUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *streamBillingUpstream) callCount(accountID int64) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.calls[accountID]
}

type streamBillingBody struct {
	reader *strings.Reader
	err    error
}

func (b *streamBillingBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if err == io.EOF && b.err != nil {
		return n, b.err
	}
	return n, err
}

func (b *streamBillingBody) Close() error { return nil }

// streamBillingRepo 记录每一次 UsageBillingRepository.Apply，并按 request_id 去重（模拟 usage_billing_dedup）。
type streamBillingRepo struct {
	mu      sync.Mutex
	applied map[string]*service.UsageBillingCommand
	calls   []*service.UsageBillingCommand
}

func newStreamBillingRepo() *streamBillingRepo {
	return &streamBillingRepo{applied: map[string]*service.UsageBillingCommand{}}
}

func (r *streamBillingRepo) Apply(_ context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := *cmd
	r.calls = append(r.calls, &copied)
	key := cmd.RequestID
	if _, ok := r.applied[key]; ok {
		return &service.UsageBillingApplyResult{Applied: false}, nil
	}
	r.applied[key] = &copied
	return &service.UsageBillingApplyResult{Applied: true, FinanceBusinessType: "balance"}, nil
}

func (r *streamBillingRepo) snapshot() (applied []*service.UsageBillingCommand, calls int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cmd := range r.applied {
		applied = append(applied, cmd)
	}
	return applied, len(r.calls)
}

type streamBillingUsageLogRepo struct {
	service.UsageLogRepository
	mu   sync.Mutex
	logs []*service.UsageLog
}

func (r *streamBillingUsageLogRepo) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, log)
	return true, nil
}

func (r *streamBillingUsageLogRepo) snapshot() []*service.UsageLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*service.UsageLog(nil), r.logs...)
}

type streamBillingHarness struct {
	handler  *GatewayHandler
	upstream *streamBillingUpstream
	billing  *streamBillingRepo
	usage    *streamBillingUsageLogRepo
	group    *service.Group
	apiKey   *service.APIKey
	accounts []*service.Account
	cleanup  func()
}

func newStreamBillingAccount(id int64, groupID int64, priority int) *service.Account {
	return &service.Account{
		ID:            id,
		Name:          "anthropic-apikey-" + string(rune('a'+id%26)),
		Platform:      service.PlatformAnthropic,
		Type:          service.AccountTypeAPIKey,
		Credentials:   map[string]any{"api_key": "sk-ant-test", "base_url": "https://api.anthropic.com"},
		Extra:         map[string]any{"anthropic_passthrough": false},
		Concurrency:   10,
		Priority:      priority,
		Status:        service.StatusActive,
		Schedulable:   true,
		AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
	}
}

// newStreamBillingHarness 构造 Messages 测试夹具。anyError=true 时开启全局 + 分组「任意错误换号」。
func newStreamBillingHarness(t *testing.T, anyError bool, accounts []*service.Account, steps map[int64][]streamBillingUpstreamStep) *streamBillingHarness {
	t.Helper()
	return newStreamBillingHarnessWithBillingCache(t, anyError, accounts, steps, nil)
}

// newStreamBillingHarnessWithBillingCache 同 newStreamBillingHarness；billingCacheSvc 非 nil 时准入检查、
// 在途预占和计费后的余额缓存扣减都走它（在途预占用例使用）。
func newStreamBillingHarnessWithBillingCache(t *testing.T, anyError bool, accounts []*service.Account, steps map[int64][]streamBillingUpstreamStep, billingCacheSvc *service.BillingCacheService) *streamBillingHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)

	groupID := int64(7001)
	for _, account := range accounts {
		account.AccountGroups = []service.AccountGroup{{AccountID: account.ID, GroupID: groupID}}
	}
	group := &service.Group{
		ID:                 groupID,
		Hydrated:           true,
		Platform:           service.PlatformAnthropic,
		Status:             service.StatusActive,
		RateMultiplier:     1,
		FailoverOnAnyError: anyError,
	}

	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.MaxLineSize = 1 << 20
	cfg.Gateway.FailoverOnAnyError = anyError
	cfg.Gateway.FailoverSameAccountRetries = 0

	upstream := newStreamBillingUpstream(steps)
	billingRepo := newStreamBillingRepo()
	usageRepo := &streamBillingUsageLogRepo{}

	schedulerSnapshot := service.NewSchedulerSnapshotService(&streamBillingSchedulerCache{accounts: accounts}, nil, nil, nil, nil)
	// 准入检查不在本夹具的验证范围：给 BillingCacheService 单独一份简易模式配置跳过余额检查；
	// GatewayService 用标准模式配置，保证 RecordUsage 走 applyUsageBilling 真实扣费路径。
	cleanup := func() {}
	if billingCacheSvc == nil {
		eligibilityCfg := &config.Config{RunMode: config.RunModeSimple}
		billingCacheSvc = service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, eligibilityCfg, nil)
		cleanup = billingCacheSvc.Stop
	}
	gwSvc := service.NewGatewayService(
		nil, &streamBillingGroupRepo{group: group}, usageRepo, billingRepo,
		nil, nil, nil, nil, cfg, schedulerSnapshot, nil,
		service.NewBillingService(cfg, nil), &service.RateLimitService{}, billingCacheSvc,
		nil, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)

	h := &GatewayHandler{
		gatewayService:      gwSvc,
		billingCacheService: billingCacheSvc,
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&concurrencyCacheMock{
			acquireUserSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
			acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) {
				return true, nil
			},
		}), SSEPingFormatClaude, 0),
		maxAccountSwitches:       3,
		maxAccountSwitchesGemini: 3,
		cfg:                      cfg,
	}

	apiKey := &service.APIKey{
		ID:      8001,
		UserID:  9001,
		GroupID: &groupID,
		Status:  service.StatusActive,
		User:    &service.User{ID: 9001, Concurrency: 10, Balance: 10, Status: service.StatusActive},
		Group:   group,
	}
	return &streamBillingHarness{
		handler:  h,
		upstream: upstream,
		billing:  billingRepo,
		usage:    usageRepo,
		group:    group,
		apiKey:   apiKey,
		accounts: accounts,
		cleanup:  cleanup,
	}
}

// serve 以流式请求调用 Messages，返回响应记录器。
func (h *streamBillingHarness) serve(t *testing.T, model string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"` + model + `","max_tokens":1024,"stream":true,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), ctxkey.Group, h.group)
	ctx = context.WithValue(ctx, ctxkey.RequestID, "client-req-1")
	c.Request = req.WithContext(ctx)
	c.Set(string(middleware.ContextKeyAPIKey), h.apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: h.apiKey.UserID, Concurrency: 10})
	h.handler.Messages(c)
	return rec
}

// waitBilling 等待异步计费落库（无 worker 池时同步执行，这里只是兜底）。
func (h *streamBillingHarness) waitBilling(t *testing.T, wantLogs int) []*service.UsageLog {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if logs := h.usage.snapshot(); len(logs) >= wantLogs {
			return logs
		}
		time.Sleep(10 * time.Millisecond)
	}
	return h.usage.snapshot()
}

// streamBillingSchedulerCache 只实现 GetSnapshot/GetAccount，其余方法由嵌入接口兜底（不会被调用）。
type streamBillingSchedulerCache struct {
	service.SchedulerCache
	accounts []*service.Account
}

func (f *streamBillingSchedulerCache) GetSnapshot(_ context.Context, _ service.SchedulerBucket) ([]*service.Account, bool, error) {
	return f.accounts, true, nil
}

func (f *streamBillingSchedulerCache) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	for _, account := range f.accounts {
		if account != nil && account.ID == id {
			return account, nil
		}
	}
	return nil, nil
}

type streamBillingGroupRepo struct {
	service.GroupRepository
	group *service.Group
}

func (f *streamBillingGroupRepo) GetByID(context.Context, int64) (*service.Group, error) {
	return f.group, nil
}

func (f *streamBillingGroupRepo) GetByIDLite(context.Context, int64) (*service.Group, error) {
	return f.group, nil
}

func sseEvent(event, data string) string {
	return "event: " + event + "\ndata: " + data + "\n\n"
}

func requireSingleBalanceCharge(t *testing.T, h *streamBillingHarness, want float64) *service.UsageBillingCommand {
	t.Helper()
	applied, _ := h.billing.snapshot()
	require.Len(t, applied, 1, "一个客户端请求必须只扣费一次")
	require.InDelta(t, want, applied[0].BalanceCost, 1e-12)
	return applied[0]
}

// waitBillingNone 给异步计费留出时间，返回最终的用量记录（期望为空的场景用）。
func (h *streamBillingHarness) waitBillingNone(t *testing.T) []*service.UsageLog {
	t.Helper()
	time.Sleep(50 * time.Millisecond)
	return h.usage.snapshot()
}
