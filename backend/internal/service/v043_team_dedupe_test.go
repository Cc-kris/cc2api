package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestV043TeamDedupeConcurrencyFailureTTLRecovery(t *testing.T) {
	ctx := context.Background()
	body := []byte(`{"detail":{"code":"deactivated_workspace"}}`)
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "team-a"}}
	store := &openAITeamBlockStoreStub{blockErr: errors.New("database unavailable"), cleared: true}
	svc := NewRateLimitService(nil, nil, nil, nil, nil)
	svc.SetOpenAITeamBlockStore(store)
	// An unsuccessful DB write is never remembered as success.
	for range 2 {
		require.True(t, svc.HandleOpenAITeamWorkspaceDeactivation(ctx, account, 402, nil, body))
	}
	require.Equal(t, 2, store.calls)
	require.Empty(t, svc.teamDedupe)
	store.blockErr = nil
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() { defer wg.Done(); svc.HandleOpenAITeamWorkspaceDeactivation(ctx, account, 402, nil, body) }()
	}
	wg.Wait()
	require.Equal(t, 3, store.calls)
	require.WithinDuration(t, time.Now().Add(15*time.Minute), svc.teamDedupe["team-a"], time.Second)
	// An expired entry must permit persistence again, without waiting 15 minutes.
	svc.teamDedupe["team-a"] = time.Now().Add(-time.Second)
	require.True(t, svc.HandleOpenAITeamWorkspaceDeactivation(ctx, account, 402, nil, body))
	require.Equal(t, 4, store.calls)
	require.NoError(t, svc.CompleteOpenAITeamProbe(ctx, &OpenAITeamProbeLease{TeamID: "team-a"}, true))
	require.Empty(t, svc.teamDedupe)
	require.True(t, svc.HandleOpenAITeamWorkspaceDeactivation(ctx, account, 402, nil, body))
	require.Equal(t, 5, store.calls)
	// Ordinary payment/quota errors and wrong status never widen to a Team block.
	require.False(t, svc.HandleOpenAITeamWorkspaceDeactivation(ctx, account, http.StatusBadRequest, nil, body))
	require.False(t, svc.HandleOpenAITeamWorkspaceDeactivation(ctx, account, 402, nil, []byte(`{"detail":{"code":"insufficient_quota"}}`)))
	require.Equal(t, 5, store.calls)
}
