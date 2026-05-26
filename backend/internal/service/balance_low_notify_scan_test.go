//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeBalanceLowNotifyRepo struct {
	users     []BalanceLowNotifyUser
	marked    map[int64]bool
	resetRows int64
	markCalls []int64
	listedEx  []int64
	resetEx   []int64
}

func (r *fakeBalanceLowNotifyRepo) ListUsersBelowBalanceThreshold(_ context.Context, _ float64, excludedUserIDs []int64) ([]BalanceLowNotifyUser, error) {
	r.listedEx = append([]int64(nil), excludedUserIDs...)
	return append([]BalanceLowNotifyUser(nil), r.users...), nil
}

func (r *fakeBalanceLowNotifyRepo) ResetUsersAtOrAboveBalanceThreshold(_ context.Context, _ float64, excludedUserIDs []int64) (int64, error) {
	r.resetEx = append([]int64(nil), excludedUserIDs...)
	return r.resetRows, nil
}

func (r *fakeBalanceLowNotifyRepo) MarkBalanceLowNotified(_ context.Context, userID int64) (bool, error) {
	r.markCalls = append(r.markCalls, userID)
	if r.marked == nil {
		r.marked = make(map[int64]bool)
	}
	if r.marked[userID] {
		return false, nil
	}
	r.marked[userID] = true
	return true, nil
}

func newBalanceNotifyScanService() (*BalanceNotifyService, *mockSettingRepo, *fakeBalanceLowNotifyRepo) {
	settings := newMockSettingRepo()
	repo := &fakeBalanceLowNotifyRepo{}
	email := NewEmailService(settings, nil)
	svc := NewBalanceNotifyService(email, settings, nil, repo)
	return svc, settings, repo
}

func TestParseBalanceLowNotifyExcludedUserIDs(t *testing.T) {
	require.Equal(t, []int64{2, 5}, ParseBalanceLowNotifyExcludedUserIDs(`[5,2,5,0,-1]`))
	require.Nil(t, ParseBalanceLowNotifyExcludedUserIDs(`not json`))
	require.Equal(t, `[]`, MarshalBalanceLowNotifyExcludedUserIDs(nil))
	require.Equal(t, `[2,5]`, MarshalBalanceLowNotifyExcludedUserIDs([]int64{5, 2, 5, 0}))
}

func TestScanBalanceLowUsers_GlobalDisabled(t *testing.T) {
	svc, settings, repo := newBalanceNotifyScanService()
	settings.data[SettingKeyBalanceLowNotifyEnabled] = "false"
	settings.data[SettingKeyBalanceLowNotifyThreshold] = "10"

	stats, err := svc.ScanBalanceLowUsers(context.Background())
	require.NoError(t, err)
	require.Zero(t, stats)
	require.Empty(t, repo.markCalls)
}

func TestScanBalanceLowUsers_UsesGlobalEmailAndSendsOncePerEpisode(t *testing.T) {
	svc, settings, repo := newBalanceNotifyScanService()
	settings.data[SettingKeyBalanceLowNotifyEnabled] = "true"
	settings.data[SettingKeyBalanceLowNotifyThreshold] = "10"
	settings.data[SettingKeyBalanceLowNotifyExcludedUserIDs] = `[3,1,3]`
	repo.users = []BalanceLowNotifyUser{
		{ID: 2, Email: "low@example.com", Username: "low", Balance: 5},
		{ID: 4, Email: "", Username: "empty", Balance: 4},
	}

	stats, err := svc.ScanBalanceLowUsers(context.Background())
	require.NoError(t, err)
	require.Equal(t, BalanceLowScanStats{Matched: 2, Marked: 2, Sent: 1}, stats)
	require.Equal(t, []int64{1, 3}, repo.listedEx)
	require.Equal(t, []int64{1, 3}, repo.resetEx)
	require.Equal(t, []int64{2, 4}, repo.markCalls)

	stats, err = svc.ScanBalanceLowUsers(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, stats.Matched)
	require.Equal(t, 0, stats.Marked)
	require.Equal(t, 0, stats.Sent)
}
