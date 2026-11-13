//go:build unit

package service

import (
	"context"
	"time"
)

// GatewayCache 新增 reasoning 缓存方法后，各测试桩补全空实现。
// 按桩类型所在测试文件的构建标签拆成两个文件。

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
