package service

import (
	"bytes"
	"context"
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

// V03-T15：分组「按张计费」开关（移植上游 fd998129e、aa1a1b11c、3ce09c8af 的测试，改为 CCAI 口径：
// 只有 openai 平台，没有 composite 与分组级定价来源）。

func TestResolveBillableImageCount(t *testing.T) {
	tests := []struct {
		name          string
		reported      int
		requested     int
		fallbackToOne bool
		want          int
	}{
		{name: "实际返回多张", reported: 3, requested: 5, want: 3},
		{name: "开关开启兜底一张", reported: 0, requested: 3, fallbackToOne: true, want: 1},
		{name: "开关关闭保留请求 n", reported: 0, requested: 3, want: 3},
		{name: "流式开关关闭保持 0", reported: 0, requested: 0, want: 0},
		{name: "非法负数开关关闭保持 n", reported: -1, requested: 2, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, resolveBillableImageCount(tt.reported, tt.requested, tt.fallbackToOne))
		})
	}
}

func TestImageCountBillingApplies(t *testing.T) {
	enabled := &APIKey{Group: &Group{ImageCountBilling: true}}
	disabled := &APIKey{Group: &Group{ImageCountBilling: false}}

	require.True(t, imageCountBillingApplies(enabled, true))
	require.False(t, imageCountBillingApplies(enabled, false), "混合请求不能受开关影响")
	require.False(t, imageCountBillingApplies(disabled, true))
	require.False(t, imageCountBillingApplies(nil, true))
	require.False(t, imageCountBillingApplies(&APIKey{}, true))
}

func TestImageCountBillingAllowed(t *testing.T) {
	require.True(t, imageCountBillingAllowed(PlatformOpenAI, true))
	require.False(t, imageCountBillingAllowed(PlatformOpenAI, false), "依赖允许图片生成")
	require.False(t, imageCountBillingAllowed(PlatformGemini, true), "只支持 openai 分组")
	require.False(t, imageCountBillingAllowed(PlatformAnthropic, true))
}

// ---------------- AC27 through RecordUsage ----------------

const ac27Model = "gpt-image-2"

func newAC27Service(t *testing.T, withTokenChannel bool) (*OpenAIGatewayService, *openAIRecordUsageLogRepoStub, int64) {
	t.Helper()
	groupID := int64(927)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	if withTokenChannel {
		cache := newEmptyChannelCache()
		cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: ac27Model}] = &ChannelModelPricing{
			BillingMode: BillingModeToken,
			InputPrice:  f64(5e-6),
			OutputPrice: f64(3e-5),
		}
		cache.channelByGroupID[groupID] = &Channel{ID: groupID, Status: StatusActive}
		cache.groupPlatform[groupID] = ""
		cache.loadedAt = time.Now()
		channelService := &ChannelService{}
		channelService.cache.Store(cache)
		svc.resolver = NewModelPricingResolver(channelService, svc.billingService)
	}
	return svc, usageRepo, groupID
}

func ac27APIKey(groupID int64, switchOn bool) *APIKey {
	return &APIKey{ID: 1927, GroupID: i64p(groupID), Group: &Group{
		ID: groupID, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true,
		RateMultiplier: 1, AllowImageGeneration: true, ImageCountBilling: switchOn,
		ImagePrice1K: f64(0.04),
	}}
}

func recordAC27(t *testing.T, svc *OpenAIGatewayService, repo *openAIRecordUsageLogRepoStub, apiKey *APIKey, result *OpenAIForwardResult) *UsageLog {
	t.Helper()
	result.Model = ac27Model
	result.Duration = time.Second
	if result.RequestID == "" {
		result.RequestID = "img_ac27"
	}
	require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: result, APIKey: apiKey, User: &User{ID: 2927}, Account: &Account{ID: 3927, Platform: PlatformOpenAI},
	}))
	require.NotNil(t, repo.lastLog)
	return repo.lastLog
}

// AC27：分组 1K 图片价 0.04，开关开启；/v1/images/generations 返回 3 张 → 0.12（渠道 token 定价被跳过）。
func TestImageCountBilling_AC27_ThreeImages(t *testing.T) {
	svc, repo, groupID := newAC27Service(t, true)
	log := recordAC27(t, svc, repo, ac27APIKey(groupID, true), &OpenAIForwardResult{
		ImageEndpoint: true, ImageCount: 3, ImageSize: "1K",
		Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 4000, ImageOutputTokens: 4000},
	})
	require.Equal(t, 3, log.ImageCount)
	require.NotNil(t, log.BillingMode)
	require.Equal(t, string(BillingModeImage), *log.BillingMode)
	require.InDelta(t, 0.12, log.TotalCost, 1e-12)
	require.InDelta(t, 0.12, log.ActualCost, 1e-12)
}

// AC27：响应解析不到张数但 200 → 开关开启按 1 张 0.04（ForwardImages 端到端）。
func TestImageCountBilling_AC27_UnparsableCountBillsOne(t *testing.T) {
	for _, tc := range []struct {
		name      string
		switchOn  bool
		wantCount int
	}{
		{name: "switch on falls back to one", switchOn: true, wantCount: 1},
		{name: "switch off keeps requested n", switchOn: false, wantCount: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","n":3,"size":"1024x1024","response_format":"b64_json"}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req
			groupID := int64(928)
			apiKey := ac27APIKey(groupID, tc.switchOn)
			c.Set("api_key", apiKey)

			svc := &OpenAIGatewayService{
				cfg: &config.Config{},
				httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					// 上游 200 但 data 不是数组：数不出张数。
					Body: io.NopCloser(strings.NewReader(`{"created":1710000007,"data":"unexpected"}`)),
				}},
			}
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			account := &Account{ID: 6, Name: "openai-apikey", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "k", "base_url": "https://image-upstream.example/v1"}}

			result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.ImageEndpoint)
			require.Equal(t, tc.wantCount, result.ImageCount)
			require.Equal(t, "1K", result.ImageSize)

			if tc.switchOn {
				recordSvc, repo, _ := newAC27Service(t, false)
				log := recordAC27(t, recordSvc, repo, apiKey, result)
				require.InDelta(t, 0.04, log.ActualCost, 1e-12)
			}
		})
	}
}

// AC27：同一分组走 /v1/responses 混合请求（非图片端点）仍按 token 计费。
func TestImageCountBilling_AC27_MixedResponsesRequestKeepsTokenBilling(t *testing.T) {
	svc, repo, groupID := newAC27Service(t, true)
	log := recordAC27(t, svc, repo, ac27APIKey(groupID, true), &OpenAIForwardResult{
		ImageEndpoint: false, ImageCount: 1, ImageSize: "1K",
		Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 200},
	})
	require.NotNil(t, log.BillingMode)
	require.Equal(t, string(BillingModeToken), *log.BillingMode)
	// 1000×5e-6 + 200×3e-5 = 0.011
	require.InDelta(t, 0.011, log.TotalCost, 1e-12)
}

// AC27：关闭开关保持现有口径（渠道 token 定价优先；无渠道定价时按分组图片价 × 张数）。
func TestImageCountBilling_AC27_SwitchOffKeepsCurrentBehavior(t *testing.T) {
	svc, repo, groupID := newAC27Service(t, true)
	log := recordAC27(t, svc, repo, ac27APIKey(groupID, false), &OpenAIForwardResult{
		ImageEndpoint: true, ImageCount: 3, ImageSize: "1K",
		Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 200},
	})
	require.Equal(t, string(BillingModeToken), *log.BillingMode)
	require.InDelta(t, 0.011, log.TotalCost, 1e-12)

	svc, repo, groupID = newAC27Service(t, false)
	log = recordAC27(t, svc, repo, ac27APIKey(groupID, false), &OpenAIForwardResult{
		ImageEndpoint: true, ImageCount: 3, ImageSize: "1K",
	})
	require.Equal(t, string(BillingModeImage), *log.BillingMode)
	require.InDelta(t, 0.12, log.TotalCost, 1e-12)
}

// 渠道按次/图片模式 + 开关开启：仍按渠道单价 × 张数（不被分组价覆盖）。
func TestImageCountBilling_ChannelImageModeStillWins(t *testing.T) {
	groupID := int64(929)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	svc.resolver = newOpenAIImageChannelPricingResolverForTest(t, groupID, ac27Model, 0.25)
	log := recordAC27(t, svc, usageRepo, ac27APIKey(groupID, true), &OpenAIForwardResult{
		ImageEndpoint: true, ImageCount: 2, ImageSize: "1K",
	})
	require.InDelta(t, 0.5, log.TotalCost, 1e-12)
}

// ---------------- group validation ----------------

type imageCountBillingGroupRepo struct {
	GroupRepository
	group   *Group
	updated *Group
}

func (r *imageCountBillingGroupRepo) GetByID(context.Context, int64) (*Group, error) {
	cp := *r.group
	return &cp, nil
}

func (r *imageCountBillingGroupRepo) Update(_ context.Context, g *Group) error {
	r.updated = g
	return nil
}

func TestGroupService_ImageCountBillingValidation(t *testing.T) {
	ctx := context.Background()
	on := true

	t.Run("创建入口固定 anthropic 平台，拒绝开关", func(t *testing.T) {
		svc := NewGroupService(nil, nil)
		_, err := svc.Create(ctx, CreateGroupRequest{Name: "g", ImageCountBilling: true})
		require.ErrorContains(t, err, "image_count_billing")
	})
	t.Run("更新时未开启图片生成则拒绝", func(t *testing.T) {
		svc := NewGroupService(&imageCountBillingGroupRepo{group: &Group{ID: 1, Platform: PlatformOpenAI}}, nil)
		_, err := svc.Update(ctx, 1, UpdateGroupRequest{ImageCountBilling: &on})
		require.ErrorContains(t, err, "image_count_billing")
	})
	t.Run("更新时不支持的平台拒绝", func(t *testing.T) {
		svc := NewGroupService(&imageCountBillingGroupRepo{group: &Group{ID: 2, Platform: PlatformGemini, AllowImageGeneration: true}}, nil)
		_, err := svc.Update(ctx, 2, UpdateGroupRequest{ImageCountBilling: &on})
		require.ErrorContains(t, err, "image_count_billing")
	})
	t.Run("合法开启", func(t *testing.T) {
		svc := NewGroupService(&imageCountBillingGroupRepo{group: &Group{ID: 3, Platform: PlatformOpenAI, AllowImageGeneration: true}}, nil)
		updated, err := svc.Update(ctx, 3, UpdateGroupRequest{ImageCountBilling: &on})
		require.NoError(t, err)
		require.True(t, updated.ImageCountBilling)
	})
	t.Run("关闭图片生成时自动清理开关", func(t *testing.T) {
		svc := NewGroupService(&imageCountBillingGroupRepo{group: &Group{ID: 4, Platform: PlatformOpenAI, AllowImageGeneration: false, ImageCountBilling: true}}, nil)
		updated, err := svc.Update(ctx, 4, UpdateGroupRequest{})
		require.NoError(t, err)
		require.False(t, updated.ImageCountBilling)
	})
}

func TestAdminUpdateGroup_ImageCountBillingClearedWhenNotAllowed(t *testing.T) {
	ctx := context.Background()
	on, off := true, false

	repo := &imageCountBillingGroupRepo{group: &Group{ID: 5, Platform: PlatformOpenAI, AllowImageGeneration: true, RateMultiplier: 1}}
	admin := &adminServiceImpl{groupRepo: repo}
	_, err := admin.UpdateGroup(ctx, 5, &UpdateGroupInput{ImageCountBilling: &on})
	require.NoError(t, err)
	require.True(t, repo.updated.ImageCountBilling)

	repo.group = repo.updated
	_, err = admin.UpdateGroup(ctx, 5, &UpdateGroupInput{AllowImageGeneration: &off})
	require.NoError(t, err)
	require.False(t, repo.updated.ImageCountBilling, "关闭图片生成后开关自动关闭")
}

func TestAPIKeyAuthSnapshotCarriesImageCountBilling(t *testing.T) {
	groupID := int64(930)
	svc := &APIKeyService{}
	apiKey := &APIKey{ID: 1, Key: "k", Status: StatusActive, GroupID: &groupID, User: &User{ID: 2, Status: StatusActive},
		Group: &Group{ID: groupID, Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true, ImageCountBilling: true}}
	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Group)
	require.True(t, snapshot.Group.ImageCountBilling)
	restored := svc.snapshotToAPIKey("k", snapshot)
	require.NotNil(t, restored.Group)
	require.True(t, restored.Group.ImageCountBilling)
	require.Equal(t, 13, apiKeyAuthSnapshotVersion)
}
