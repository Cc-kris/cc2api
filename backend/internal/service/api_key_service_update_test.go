package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// 本文件不带 unit 标签，桩用嵌入接口实现：未覆写的方法被调用会 nil panic，便于发现意外调用。

type apiKeyUpdateRepoStub struct {
	APIKeyRepository
	apiKey  *APIKey
	updated []*APIKey
}

func (s *apiKeyUpdateRepoStub) GetByID(_ context.Context, id int64) (*APIKey, error) {
	if s.apiKey == nil || s.apiKey.ID != id {
		return nil, ErrAPIKeyNotFound
	}
	clone := *s.apiKey
	clone.IPWhitelist = append([]string(nil), s.apiKey.IPWhitelist...)
	clone.IPBlacklist = append([]string(nil), s.apiKey.IPBlacklist...)
	return &clone, nil
}

func (s *apiKeyUpdateRepoStub) Update(_ context.Context, key *APIKey) error {
	clone := *key
	s.updated = append(s.updated, &clone)
	return nil
}

type apiKeyUpdateUserRepoStub struct {
	UserRepository
	user *User
}

func (s *apiKeyUpdateUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, nil
}

type apiKeyUpdateGroupRepoStub struct {
	GroupRepository
	group *Group
}

func (s *apiKeyUpdateGroupRepoStub) GetByID(context.Context, int64) (*Group, error) {
	clone := *s.group
	return &clone, nil
}

func newAPIKeyUpdateTestService(t *testing.T) (*APIKeyService, *apiKeyUpdateRepoStub) {
	t.Helper()
	repo := &apiKeyUpdateRepoStub{apiKey: &APIKey{
		ID:          10,
		UserID:      7,
		Key:         "sk-update-test",
		Name:        "old",
		Status:      StatusActive,
		IPWhitelist: []string{"10.0.0.0/8", "192.168.1.1"},
		IPBlacklist: []string{"1.2.3.4"},
		QuotaUsed:   5,
	}}
	svc := &APIKeyService{
		apiKeyRepo: repo,
		userRepo:   &apiKeyUpdateUserRepoStub{user: &User{ID: 7}},
		groupRepo:  &apiKeyUpdateGroupRepoStub{group: &Group{ID: 3, Name: "g3"}},
	}
	return svc, repo
}

func TestAPIKeyServiceUpdate_GroupOnlyKeepsIPLists(t *testing.T) {
	svc, repo := newAPIKeyUpdateTestService(t)
	groupID := int64(3)

	got, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{GroupID: &groupID})
	require.NoError(t, err)
	require.Len(t, repo.updated, 1)
	require.Equal(t, []string{"10.0.0.0/8", "192.168.1.1"}, repo.updated[0].IPWhitelist)
	require.Equal(t, []string{"1.2.3.4"}, repo.updated[0].IPBlacklist)
	require.NotNil(t, got.GroupID)
	require.Equal(t, int64(3), *got.GroupID)
}

func TestAPIKeyServiceUpdate_ResetQuotaOnlyKeepsIPLists(t *testing.T) {
	svc, repo := newAPIKeyUpdateTestService(t)
	reset := true

	_, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{ResetQuota: &reset})
	require.NoError(t, err)
	require.Len(t, repo.updated, 1)
	require.Equal(t, float64(0), repo.updated[0].QuotaUsed)
	require.Equal(t, []string{"10.0.0.0/8", "192.168.1.1"}, repo.updated[0].IPWhitelist)
	require.Equal(t, []string{"1.2.3.4"}, repo.updated[0].IPBlacklist)
}

func TestAPIKeyServiceUpdate_EmptyWhitelistClearsOnlyWhitelist(t *testing.T) {
	svc, repo := newAPIKeyUpdateTestService(t)
	empty := []string{}

	_, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{IPWhitelist: &empty})
	require.NoError(t, err)
	require.Len(t, repo.updated, 1)
	require.Empty(t, repo.updated[0].IPWhitelist)
	require.Equal(t, []string{"1.2.3.4"}, repo.updated[0].IPBlacklist)
}

func TestAPIKeyServiceUpdate_ExplicitListsReplace(t *testing.T) {
	svc, repo := newAPIKeyUpdateTestService(t)
	wl := []string{"172.16.0.0/12"}
	bl := []string{}

	_, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{IPWhitelist: &wl, IPBlacklist: &bl})
	require.NoError(t, err)
	require.Len(t, repo.updated, 1)
	require.Equal(t, []string{"172.16.0.0/12"}, repo.updated[0].IPWhitelist)
	require.Empty(t, repo.updated[0].IPBlacklist)
}

func TestAPIKeyServiceUpdate_InvalidIPRejectedWithoutWrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  func(bad *[]string) UpdateAPIKeyRequest
	}{
		{"whitelist", func(bad *[]string) UpdateAPIKeyRequest { return UpdateAPIKeyRequest{IPWhitelist: bad} }},
		{"blacklist", func(bad *[]string) UpdateAPIKeyRequest { return UpdateAPIKeyRequest{IPBlacklist: bad} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newAPIKeyUpdateTestService(t)
			bad := []string{"not-an-ip"}

			_, err := svc.Update(context.Background(), 10, 7, tc.req(&bad))
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrInvalidIPPattern))
			require.Empty(t, repo.updated, "invalid IP must not write")
			require.Equal(t, []string{"10.0.0.0/8", "192.168.1.1"}, repo.apiKey.IPWhitelist)
			require.Equal(t, []string{"1.2.3.4"}, repo.apiKey.IPBlacklist)
		})
	}
}

// TestAPIKeyServiceUpdate_ForeignKeyIndistinguishableFromMissing：改别人的 Key 与改不存在的 Key
// 返回同一个错误，不写库（V01-R11，e2e 发现 PUT 仍返回 403）。
func TestAPIKeyServiceUpdate_ForeignKeyIndistinguishableFromMissing(t *testing.T) {
	svc, repo := newAPIKeyUpdateTestService(t)
	name := "hijack"

	_, foreignErr := svc.Update(context.Background(), 10, 8, UpdateAPIKeyRequest{Name: &name})
	require.ErrorIs(t, foreignErr, ErrAPIKeyNotFound)

	_, missingErr := svc.Update(context.Background(), 999, 7, UpdateAPIKeyRequest{Name: &name})
	require.ErrorIs(t, missingErr, ErrAPIKeyNotFound)

	require.Empty(t, repo.updated)
}
