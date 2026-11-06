package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type apiKeyCreateRepoStub struct {
	APIKeyRepository
	created []*APIKey
}

func (s *apiKeyCreateRepoStub) Create(_ context.Context, key *APIKey) error {
	clone := *key
	s.created = append(s.created, &clone)
	return nil
}

const apiKeyXSSName = `<img src=x onerror=alert(1)>&"'`
const apiKeyXSSNameEscaped = `&lt;img src=x onerror=alert(1)&gt;&amp;&#34;&#39;`

// V01-AC38：新建 Key 时名称按 HTML 转义后存储。
func TestAPIKeyServiceCreate_EscapesName(t *testing.T) {
	repo := &apiKeyCreateRepoStub{}
	svc := &APIKeyService{
		apiKeyRepo: repo,
		userRepo:   &apiKeyUpdateUserRepoStub{user: &User{ID: 7}},
		cfg:        &config.Config{},
	}

	got, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: apiKeyXSSName})
	require.NoError(t, err)
	require.Len(t, repo.created, 1)
	require.Equal(t, apiKeyXSSNameEscaped, repo.created[0].Name)
	require.Equal(t, apiKeyXSSNameEscaped, got.Name)
}

// V01-AC38：改名时名称按 HTML 转义后存储；不改名时原名称不被二次转义。
func TestAPIKeyServiceUpdate_EscapesNameOnlyWhenProvided(t *testing.T) {
	svc, repo := newAPIKeyUpdateTestService(t)
	name := apiKeyXSSName

	_, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{Name: &name})
	require.NoError(t, err)
	require.Len(t, repo.updated, 1)
	require.Equal(t, apiKeyXSSNameEscaped, repo.updated[0].Name)

	repo.apiKey.Name = "a&b"
	reset := true
	_, err = svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{ResetQuota: &reset})
	require.NoError(t, err)
	require.Equal(t, "a&b", repo.updated[1].Name)
}

// e2e 发现：编辑弹窗把已转义的名称原样提交，重复保存不能逐层加深转义。
func TestAPIKeyServiceUpdate_NameEscapeIsIdempotent(t *testing.T) {
	svc, repo := newAPIKeyUpdateTestService(t)
	name := "A&B"
	for i := 0; i < 3; i++ {
		_, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{Name: &name})
		require.NoError(t, err)
		saved := repo.updated[len(repo.updated)-1].Name
		require.Equal(t, "A&amp;B", saved, "save #%d", i+1)
		name = saved // 模拟前端把已存储的名称原样提交回来
		repo.apiKey.Name = saved
	}
	require.Equal(t, apiKeyXSSNameEscaped, escapeAPIKeyName(apiKeyXSSNameEscaped))
}
