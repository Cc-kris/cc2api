//go:build unit

package service

import (
	"context"
	"time"
)

// GatewayCache 新增 reasoning 缓存方法后，各测试桩在此统一补全空实现，
// 避免逐个桩类型散落重复方法。

func (c *stubGatewayCache) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *stubGatewayCache) GetReasoningContent(context.Context, string) (string, error) {
	return "", ErrReasoningContentNotFound
}

func (s *stickyGatewayCacheHotpathStub) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (s *stickyGatewayCacheHotpathStub) GetReasoningContent(context.Context, string) (string, error) {
	return "", ErrReasoningContentNotFound
}

func (m *mockGatewayCacheForPlatform) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (m *mockGatewayCacheForPlatform) GetReasoningContent(context.Context, string) (string, error) {
	return "", ErrReasoningContentNotFound
}

func (m *mockGatewayCacheForGemini) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (m *mockGatewayCacheForGemini) GetReasoningContent(context.Context, string) (string, error) {
	return "", ErrReasoningContentNotFound
}

func (c *schedulerTestGatewayCache) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *schedulerTestGatewayCache) GetReasoningContent(context.Context, string) (string, error) {
	return "", ErrReasoningContentNotFound
}

func (c *schedulerClaimRaceCache) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *schedulerClaimRaceCache) GetReasoningContent(context.Context, string) (string, error) {
	return "", ErrReasoningContentNotFound
}

func (c *openAIWSStateStoreTimeoutProbeCache) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *openAIWSStateStoreTimeoutProbeCache) GetReasoningContent(context.Context, string) (string, error) {
	return "", ErrReasoningContentNotFound
}

func (s *localResponseCacheMemoryStore) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (s *localResponseCacheMemoryStore) GetReasoningContent(context.Context, string) (string, error) {
	return "", ErrReasoningContentNotFound
}

func (s *localResponseCacheClearStoreStub) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (s *localResponseCacheClearStoreStub) GetReasoningContent(context.Context, string) (string, error) {
	return "", ErrReasoningContentNotFound
}

func (s *localResponseCacheGatewayTestStore) SetReasoningContent(context.Context, string, string, time.Duration) error {
	return nil
}

func (s *localResponseCacheGatewayTestStore) GetReasoningContent(context.Context, string) (string, error) {
	return "", ErrReasoningContentNotFound
}
