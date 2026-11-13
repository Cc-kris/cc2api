//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// 账号缓存键改为 ag:account:<id> 后，旧版 ag:<project_id> 键仍可能残留在缓存里，
// 失效时必须显式删除，否则旧 token 会被其他共用同一 project_id 的账号读到。
func TestCompositeTokenCacheInvalidator_Antigravity_DeletesLegacyProjectKey(t *testing.T) {
	cache := &geminiTokenCacheStub{}
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       7,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"project_id": "  shared-project  ",
		},
	}

	require.NoError(t, invalidator.InvalidateToken(context.Background(), account))
	require.Equal(t, []string{"ag:shared-project", "ag:account:7"}, cache.deletedKeys)
}

func TestCompositeTokenCacheInvalidator_Antigravity_NoProjectIDOnlyDeletesAccountKey(t *testing.T) {
	for name, creds := range map[string]map[string]any{
		"missing": {},
		"empty":   {"project_id": ""},
		"blank":   {"project_id": "   "},
	} {
		t.Run(name, func(t *testing.T) {
			cache := &geminiTokenCacheStub{}
			invalidator := NewCompositeTokenCacheInvalidator(cache)
			account := &Account{ID: 8, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Credentials: creds}

			require.NoError(t, invalidator.InvalidateToken(context.Background(), account))
			require.Equal(t, []string{"ag:account:8"}, cache.deletedKeys)
		})
	}
}

func TestAntigravityTokenCacheKey_TwoAccountsSameProjectDoNotCollide(t *testing.T) {
	a := &Account{ID: 1, Credentials: map[string]any{"project_id": "shared"}}
	b := &Account{ID: 2, Credentials: map[string]any{"project_id": "shared"}}
	require.NotEqual(t, AntigravityTokenCacheKey(a), AntigravityTokenCacheKey(b))
}
