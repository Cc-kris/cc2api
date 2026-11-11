//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// invitationRaceSettingRepo 内存设置：开放注册 + 开启邀请码，关闭邮件验证。不写共享的 settings 表。
type invitationRaceSettingRepo struct{ values map[string]string }

func (r *invitationRaceSettingRepo) Get(_ context.Context, key string) (*service.Setting, error) {
	if v, ok := r.values[key]; ok {
		return &service.Setting{Key: key, Value: v}, nil
	}
	return nil, service.ErrSettingNotFound
}
func (r *invitationRaceSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if v, ok := r.values[key]; ok {
		return v, nil
	}
	return "", service.ErrSettingNotFound
}
func (r *invitationRaceSettingRepo) Set(context.Context, string, string) error { return nil }
func (r *invitationRaceSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}
func (r *invitationRaceSettingRepo) SetMultiple(context.Context, map[string]string) error { return nil }
func (r *invitationRaceSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}
func (r *invitationRaceSettingRepo) Delete(context.Context, string) error { return nil }

func newInvitationRaceAuthService(t *testing.T) *service.AuthService {
	t.Helper()
	client := testEntClient(t)
	cfg := &config.Config{}
	cfg.JWT.Secret = "invitation-race-test-secret-0123456789abcdef"
	cfg.JWT.ExpireHour = 1
	settings := service.NewSettingService(&invitationRaceSettingRepo{values: map[string]string{
		service.SettingKeyRegistrationEnabled:   "true",
		service.SettingKeyInvitationCodeEnabled: "true",
		service.SettingKeyEmailVerifyEnabled:    "false",
	}}, cfg)
	return service.NewAuthService(
		client,
		NewUserRepository(client, integrationDB),
		NewRedeemCodeRepository(client),
		NewRefreshTokenCache(testRedis(t)),
		cfg, settings,
		nil, nil, nil, nil, nil, nil, nil,
	)
}

// V03-AC15：20 个并发邮箱注册用同一个未使用的邀请码，另起 OAuth 注册用同一码。
// 恰好 1 个用户创建成功、邀请码 used_by 是该用户；其余 19 个和 OAuth 都返回 INVITATION_CODE_INVALID，
// 且没有残留用户行。
func TestRegisterWithVerification_ConcurrentInvitationClaimOnlyOneWins(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	code := "INV-RACE-" + suffix
	emailPrefix := "inv-race-" + suffix + "-"
	oauthEmail := "inv-race-oauth-" + suffix + "@example.test"

	codeRow, err := client.RedeemCode.Create().
		SetCode(code).
		SetType(service.RedeemTypeInvitation).
		SetStatus(service.StatusUnused).
		SetValue(0).
		Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = client.RedeemCode.Delete().Where(redeemcode.IDEQ(codeRow.ID)).Exec(bg)
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM auth_identity_channels WHERE identity_id IN (SELECT id FROM auth_identities WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1 OR email=$2))`, emailPrefix+"%", oauthEmail)
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM auth_identities WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1 OR email=$2)`, emailPrefix+"%", oauthEmail)
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM users WHERE email LIKE $1 OR email=$2`, emailPrefix+"%", oauthEmail)
	})

	svc := newInvitationRaceAuthService(t)

	const workers = 20
	type outcome struct {
		user *service.User
		err  error
	}
	results := make([]outcome, workers)
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate
			email := fmt.Sprintf("%s%02d@example.test", emailPrefix, i)
			_, u, err := svc.RegisterWithVerification(context.Background(), email, "Passw0rd!x", "", "", code, "")
			results[i] = outcome{user: u, err: err}
		}(i)
	}
	close(gate)
	wg.Wait()

	var winner *service.User
	invalid := 0
	for i, r := range results {
		switch {
		case r.err == nil:
			require.Nil(t, winner, "only one registration may succeed (second winner at %d)", i)
			require.NotNil(t, r.user)
			winner = r.user
		case errors.Is(r.err, service.ErrInvitationCodeInvalid):
			invalid++
		default:
			t.Fatalf("registration %d returned unexpected error: %v", i, r.err)
		}
	}
	require.NotNil(t, winner, "exactly one registration must succeed")
	require.Equal(t, workers-1, invalid)

	// OAuth 注册同一码：码已用，拒绝且不建号。
	_, _, err = svc.LoginOrRegisterOAuthWithTokenPair(ctx, oauthEmail, "oauth-racer", code, "", "linuxdo")
	require.ErrorIs(t, err, service.ErrInvitationCodeInvalid)

	after, err := client.RedeemCode.Get(ctx, codeRow.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusUsed, after.Status)
	require.NotNil(t, after.UsedBy)
	require.Equal(t, winner.ID, *after.UsedBy)

	users, err := client.User.Query().
		Where(dbuser.Or(dbuser.EmailHasPrefix(emailPrefix), dbuser.EmailEQ(oauthEmail))).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, users, 1, "no orphan users may remain")
	require.Equal(t, winner.ID, users[0].ID)
}

// 外部事务回滚时，用户创建与邀请码占用一并撤销（user_repo 复用 ctx 中的事务，不再自行提交）。
func TestCreateUserJoinsOuterTransaction(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	userRepo := NewUserRepository(client, integrationDB)
	redeemRepo := NewRedeemCodeRepository(client)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	codeRow, err := client.RedeemCode.Create().
		SetCode("ITX-" + suffix).
		SetType(service.RedeemTypeInvitation).
		SetStatus(service.StatusUnused).
		SetValue(0).
		Save(ctx)
	require.NoError(t, err)
	email := "itx-rollback-" + suffix + "@example.test"
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = client.RedeemCode.Delete().Where(redeemcode.IDEQ(codeRow.ID)).Exec(bg)
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM users WHERE email=$1`, email)
	})

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	txCtx := dbent.NewTxContext(ctx, tx)
	u := &service.User{Email: email, PasswordHash: "hash", Role: service.RoleUser, Status: service.StatusActive, Concurrency: 1}
	require.NoError(t, userRepo.Create(txCtx, u))
	require.Positive(t, u.ID)
	require.NoError(t, redeemRepo.Use(txCtx, codeRow.ID, u.ID))
	require.NoError(t, tx.Rollback())

	exists, err := userRepo.ExistsByEmail(ctx, email)
	require.NoError(t, err)
	require.False(t, exists, "rolled-back outer transaction must not leave an orphan user")
	after, err := client.RedeemCode.Get(ctx, codeRow.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusUnused, after.Status)
}
