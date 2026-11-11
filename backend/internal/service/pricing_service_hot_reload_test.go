package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 移植自上游 fdc2ec17e pricing_service_hot_reload_test.go。CCAI 的 fallback_file 只作
// 灾备目录（远程和本地缓存都不可用时整份替代目录），不与远程目录合并，所以 fallback
// 相关用例改为灾备模式下验证；override 用例与上游一致。

const hotReloadCatalogJSON = `{
	"remote-model": {"litellm_provider": "test", "mode": "chat",
		"input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06},
	"gpt-5.5": {"litellm_provider": "openai", "mode": "chat",
		"input_cost_per_token": 5e-06, "output_cost_per_token": 3e-05}
}`

func hotReloadModelJSON(name string, input, output float64) string {
	return `"` + name + `": {"litellm_provider": "test", "mode": "chat",
		"input_cost_per_token": ` + strconv.FormatFloat(input, 'g', -1, 64) +
		`, "output_cost_per_token": ` + strconv.FormatFloat(output, 'g', -1, 64) + `}`
}

// newHotReloadPricingService 在数据目录里放好目录缓存与 override 文件并完成首次加载。
func newHotReloadPricingService(t *testing.T, overrideJSON string) *PricingService {
	t.Helper()
	dir := t.TempDir()
	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.DataDir = dir
	require.NoError(t, os.WriteFile(svc.getPricingFilePath(), []byte(hotReloadCatalogJSON), 0644))
	svc.cfg.Pricing.OverrideFile = filepath.Join(dir, "overrides.json")
	if overrideJSON != "" {
		require.NoError(t, os.WriteFile(svc.cfg.Pricing.OverrideFile, []byte(overrideJSON), 0644))
	}
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
	return svc
}

func TestPricingCustomFilesFingerprint(t *testing.T) {
	svc := &PricingService{cfg: &config.Config{}}
	require.Empty(t, svc.customPricingFilesFingerprint(), "未配置文件返回空串")

	dir := t.TempDir()
	svc.cfg.Pricing.OverrideFile = filepath.Join(dir, "overrides.json")
	missing := svc.customPricingFilesFingerprint()
	require.NotEmpty(t, missing)
	require.Equal(t, missing, svc.customPricingFilesFingerprint(), "同一状态下指纹稳定")

	require.NoError(t, os.WriteFile(svc.cfg.Pricing.OverrideFile, []byte(`{}`), 0644))
	present := svc.customPricingFilesFingerprint()
	require.NotEqual(t, missing, present, "文件出现即视为变化")

	svc.cfg.Pricing.FallbackFile = filepath.Join(dir, "fallback.json")
	require.NoError(t, os.WriteFile(svc.cfg.Pricing.FallbackFile, []byte(`{"x":{}}`), 0644))
	require.Equal(t, present, svc.customPricingFilesFingerprint(), "非灾备模式下 fallback 不参与指纹")
	svc.usingFallback = true
	require.NotEqual(t, present, svc.customPricingFilesFingerprint(), "灾备模式下 fallback 参与指纹")
}

func TestPricingHotReload_UnchangedFilesSkipRebuild(t *testing.T) {
	svc := newHotReloadPricingService(t, `{"remote-model": {"input_cost_per_token": 7e-06}}`)
	svc.pricingData["sentinel"] = &LiteLLMModelPricing{}

	svc.reloadIfCustomFilesChanged()

	require.Contains(t, svc.pricingData, "sentinel", "指纹未变时不得重建")
}

func TestPricingHotReload_OverrideChangePatchesCatalogAndAddsModels(t *testing.T) {
	svc := newHotReloadPricingService(t, `{"remote-model": {"input_cost_per_token": 7e-06}}`)
	require.InDelta(t, 7e-6, svc.pricingData["remote-model"].InputCostPerToken, 1e-12)
	anchor, updated := svc.localHash, svc.lastUpdated

	require.NoError(t, os.WriteFile(svc.cfg.Pricing.OverrideFile, []byte(`{
		"remote-model": {"input_cost_per_token": 9e-06},
		`+hotReloadModelJSON("override-new-model", 5e-6, 1e-5)+`}`), 0644))
	svc.reloadIfCustomFilesChanged()

	require.InDelta(t, 9e-6, svc.pricingData["remote-model"].InputCostPerToken, 1e-12)
	require.InDelta(t, 2e-6, svc.pricingData["remote-model"].OutputCostPerToken, 1e-12, "未覆盖字段保持目录值")
	require.NotNil(t, svc.pricingData["override-new-model"])
	require.Equal(t, anchor, svc.localHash, "热重载不得改动远程同步锚点")
	require.Equal(t, updated, svc.lastUpdated)
	require.Equal(t, svc.customPricingFilesFingerprint(), svc.customFilesHash)

	// 清空补丁：目录条目回到原价，补丁新增的模型随之消失。
	require.NoError(t, os.WriteFile(svc.cfg.Pricing.OverrideFile, []byte(`{}`), 0644))
	svc.reloadIfCustomFilesChanged()

	require.InDelta(t, 1e-6, svc.pricingData["remote-model"].InputCostPerToken, 1e-12)
	require.Nil(t, svc.pricingData["override-new-model"])
}

func TestPricingHotReload_InvalidFileKeepsCurrentDataUntilFixed(t *testing.T) {
	svc := newHotReloadPricingService(t, `{"remote-model": {"input_cost_per_token": 4e-06}}`)
	before := svc.customFilesHash

	require.NoError(t, os.WriteFile(svc.cfg.Pricing.OverrideFile, []byte(`{"remote-model": {"input_cost_per_token": `), 0644))
	svc.reloadIfCustomFilesChanged()
	require.InDelta(t, 4e-6, svc.pricingData["remote-model"].InputCostPerToken, 1e-12, "半写文件不得替换数据")
	require.Equal(t, before, svc.customFilesHash, "指纹不更新，下一轮继续尝试")

	require.NoError(t, os.WriteFile(svc.cfg.Pricing.OverrideFile, []byte(`{"remote-model": {"input_cost_per_token": 6e-06}}`), 0644))
	svc.reloadIfCustomFilesChanged()
	require.InDelta(t, 6e-6, svc.pricingData["remote-model"].InputCostPerToken, 1e-12, "文件修好后正常重建")
	require.NotEqual(t, before, svc.customFilesHash)
}

// 删除文件等于清空该层，且缺失状态被记录，之后不会每轮重建。
func TestPricingHotReload_DeletedFileDropsItsLayer(t *testing.T) {
	svc := newHotReloadPricingService(t, `{"remote-model": {"input_cost_per_token": 7e-06}}`)
	require.InDelta(t, 7e-6, svc.pricingData["remote-model"].InputCostPerToken, 1e-12)

	require.NoError(t, os.Remove(svc.cfg.Pricing.OverrideFile))
	svc.reloadIfCustomFilesChanged()
	require.InDelta(t, 1e-6, svc.pricingData["remote-model"].InputCostPerToken, 1e-12, "删除 override 后目录条目回到原价")

	svc.pricingData["sentinel"] = &LiteLLMModelPricing{}
	svc.reloadIfCustomFilesChanged()
	require.Contains(t, svc.pricingData, "sentinel", "缺失状态已记录，不得每轮重建")
}

// 灾备模式：目录正文来自 fallback 文件，fallback 内容变化同样热重载。
func TestPricingHotReload_FallbackModeReloadsFallbackFile(t *testing.T) {
	dir := t.TempDir()
	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.DataDir = filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(svc.cfg.Pricing.DataDir, 0755))
	svc.cfg.Pricing.FallbackFile = filepath.Join(dir, "fallback.json")
	require.NoError(t, os.WriteFile(svc.cfg.Pricing.FallbackFile, []byte(`{`+hotReloadModelJSON("custom-a", 4e-6, 8e-6)+`}`), 0644))
	require.NoError(t, svc.useFallbackPricing())
	require.True(t, svc.usingFallback)
	anchor := svc.localHash

	require.NoError(t, os.WriteFile(svc.cfg.Pricing.FallbackFile, []byte(`{`+
		hotReloadModelJSON("custom-a", 5e-6, 8e-6)+`,`+
		hotReloadModelJSON("custom-b", 1e-6, 3e-6)+`}`), 0644))
	svc.reloadIfCustomFilesChanged()

	require.InDelta(t, 5e-6, svc.pricingData["custom-a"].InputCostPerToken, 1e-12, "改价即时生效")
	require.NotNil(t, svc.pricingData["custom-b"], "新模型即时并入")
	require.Equal(t, anchor, svc.localHash, "热重载不得改动远程同步锚点")
}

// 非灾备模式下 fallback 文件变化不触发重建，也不改变财务 checksum。
func TestPricingHotReload_FallbackIgnoredWhenCatalogLoaded(t *testing.T) {
	svc := newHotReloadPricingService(t, "")
	svc.cfg.Pricing.OverrideFile = ""
	svc.cfg.Pricing.FallbackFile = filepath.Join(t.TempDir(), "fallback.json")
	require.NoError(t, os.WriteFile(svc.cfg.Pricing.FallbackFile, []byte(`{`+hotReloadModelJSON("custom-a", 4e-6, 8e-6)+`}`), 0644))
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
	checksum := svc.Snapshot().Checksum
	require.Equal(t, svc.localHash, checksum)

	require.NoError(t, os.WriteFile(svc.cfg.Pricing.FallbackFile, []byte(`{`+hotReloadModelJSON("custom-a", 9e-6, 8e-6)+`}`), 0644))
	svc.pricingData["sentinel"] = &LiteLLMModelPricing{}
	svc.reloadIfCustomFilesChanged()
	require.Contains(t, svc.pricingData, "sentinel")
	require.Nil(t, svc.pricingData["custom-a"])
	require.Equal(t, checksum, svc.Snapshot().Checksum)
}

type stubPricingRemoteClient struct{ body string }

func (c stubPricingRemoteClient) FetchPricingJSON(context.Context, string) ([]byte, error) {
	return []byte(c.body), nil
}

func (c stubPricingRemoteClient) FetchHashText(context.Context, string) (string, error) {
	return "", nil
}

// 远程下载重建后指纹必须同步到当前文件内容，否则下一轮定时比对会多做一次无意义重载。
func TestPricingHotReload_DownloadRefreshesFingerprint(t *testing.T) {
	svc := newHotReloadPricingService(t, `{"remote-model": {"input_cost_per_token": 7e-06}}`)
	svc.cfg.Pricing.RemoteURL = "https://example.com/pricing.json"
	svc.remoteClient = stubPricingRemoteClient{body: hotReloadCatalogJSON}
	require.NoError(t, os.WriteFile(svc.cfg.Pricing.OverrideFile, []byte(`{"remote-model": {"input_cost_per_token": 8e-06}}`), 0644))

	require.NoError(t, svc.downloadPricingData())

	require.InDelta(t, 8e-6, svc.pricingData["remote-model"].InputCostPerToken, 1e-12)
	require.Equal(t, svc.customPricingFilesFingerprint(), svc.customFilesHash)

	svc.pricingData["sentinel"] = &LiteLLMModelPricing{}
	svc.reloadIfCustomFilesChanged()
	require.Contains(t, svc.pricingData, "sentinel", "下载已消化文件变化，不得再次重建")
}

// AC30：override 热加载改价、非法 JSON 保留、删除恢复；每次生效的变化都产生新的财务 checksum，
// 生效时间晚于上一个版本。
func TestPricingHotReload_AC30OverrideLifecycleAndFinanceChecksum(t *testing.T) {
	svc := newHotReloadPricingService(t, "")
	billing := NewBillingService(&config.Config{}, svc)
	cost := func() float64 {
		c, err := billing.CalculateCost("gpt-5.5", UsageTokens{InputTokens: 1000, OutputTokens: 100}, 1)
		require.NoError(t, err)
		return c.TotalCost
	}
	require.InDelta(t, 0.008, cost(), 1e-12)
	snap0 := svc.Snapshot()

	time.Sleep(5 * time.Millisecond)
	require.NoError(t, os.WriteFile(svc.cfg.Pricing.OverrideFile, []byte(`{"gpt-5.5":{"input_cost_per_token":6e-6}}`), 0644))
	svc.reloadIfCustomFilesChanged()
	require.InDelta(t, 0.009, cost(), 1e-12)
	snap1 := svc.Snapshot()
	require.NotEqual(t, snap0.Checksum, snap1.Checksum, "改价后财务 checksum 变化")
	require.True(t, snap1.UpdatedAt.After(snap0.UpdatedAt), "新版本生效时间晚于旧版本")

	require.NoError(t, os.WriteFile(svc.cfg.Pricing.OverrideFile, []byte(`{"gpt-5.5":`), 0644))
	svc.reloadIfCustomFilesChanged()
	require.InDelta(t, 0.009, cost(), 1e-12, "非法 JSON 保留旧数据")
	require.Equal(t, snap1.Checksum, svc.Snapshot().Checksum, "非法 JSON 不产生新版本")

	time.Sleep(5 * time.Millisecond)
	require.NoError(t, os.Remove(svc.cfg.Pricing.OverrideFile))
	svc.reloadIfCustomFilesChanged()
	require.InDelta(t, 0.008, cost(), 1e-12, "删除后回到目录价")
	snap2 := svc.Snapshot()
	require.NotEqual(t, snap1.Checksum, snap2.Checksum)
	require.True(t, snap2.UpdatedAt.After(snap1.UpdatedAt))
}

// 未配置 override 时财务 checksum 与升级前一致（等于远程同步锚点）。
func TestPricingSnapshotChecksumUnchangedWithoutOverride(t *testing.T) {
	dir := t.TempDir()
	svc := &PricingService{cfg: &config.Config{}}
	svc.cfg.Pricing.DataDir = dir
	require.NoError(t, os.WriteFile(svc.getPricingFilePath(), []byte(hotReloadCatalogJSON), 0644))
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
	require.Equal(t, svc.localHash, svc.Snapshot().Checksum)
	require.Equal(t, svc.lastUpdated, svc.Snapshot().UpdatedAt)
}

// 只配置了 override 时调度器照常运行并监视文件。
func TestPricingSchedulerStartsForCustomFiles(t *testing.T) {
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{
		OverrideFile: filepath.Join(t.TempDir(), "overrides.json"),
	}}, nil)
	require.True(t, svc.hasCustomPricingFiles())

	svc.startUpdateScheduler()
	done := make(chan struct{})
	go func() {
		svc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("custom file watch must keep the scheduler running")
	case <-time.After(50 * time.Millisecond):
	}

	svc.Stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler must exit after Stop")
	}
}
