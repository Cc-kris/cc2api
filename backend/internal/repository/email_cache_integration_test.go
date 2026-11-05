//go:build integration

package repository

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type EmailCacheSuite struct {
	IntegrationRedisSuite
	cache service.EmailCache
}

func (s *EmailCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewEmailCache(s.rdb)
}

func (s *EmailCacheSuite) TestGetVerificationCode_Missing() {
	_, err := s.cache.GetVerificationCode(s.ctx, "nonexistent@example.com")
	require.True(s.T(), errors.Is(err, redis.Nil), "expected redis.Nil for missing verification code")
}

func (s *EmailCacheSuite) TestSetAndGetVerificationCode() {
	email := "a@example.com"
	emailTTL := 2 * time.Minute
	data := &service.VerificationCodeData{Code: "123456", Attempts: 1, CreatedAt: time.Now()}

	require.NoError(s.T(), s.cache.SetVerificationCode(s.ctx, email, data, emailTTL), "SetVerificationCode")

	got, err := s.cache.GetVerificationCode(s.ctx, email)
	require.NoError(s.T(), err, "GetVerificationCode")
	require.Equal(s.T(), "123456", got.Code)
	require.Equal(s.T(), 1, got.Attempts)
}

func (s *EmailCacheSuite) TestVerificationCode_TTL() {
	email := "ttl@example.com"
	emailTTL := 2 * time.Minute
	data := &service.VerificationCodeData{Code: "654321", Attempts: 0, CreatedAt: time.Now()}

	require.NoError(s.T(), s.cache.SetVerificationCode(s.ctx, email, data, emailTTL), "SetVerificationCode")

	emailKey := verifyCodeKeyPrefix + email
	ttl, err := s.rdb.TTL(s.ctx, emailKey).Result()
	require.NoError(s.T(), err, "TTL emailKey")
	s.AssertTTLWithin(ttl, 1*time.Second, emailTTL)
}

func (s *EmailCacheSuite) TestDeleteVerificationCode() {
	email := "delete@example.com"
	data := &service.VerificationCodeData{Code: "999999", Attempts: 0, CreatedAt: time.Now()}

	require.NoError(s.T(), s.cache.SetVerificationCode(s.ctx, email, data, 2*time.Minute), "SetVerificationCode")

	// Verify it exists
	_, err := s.cache.GetVerificationCode(s.ctx, email)
	require.NoError(s.T(), err, "GetVerificationCode before delete")

	// Delete
	require.NoError(s.T(), s.cache.DeleteVerificationCode(s.ctx, email), "DeleteVerificationCode")

	// Verify it's gone
	_, err = s.cache.GetVerificationCode(s.ctx, email)
	require.True(s.T(), errors.Is(err, redis.Nil), "expected redis.Nil after delete")
}

func (s *EmailCacheSuite) TestDeleteVerificationCode_NonExistent() {
	// Deleting a non-existent key should not error
	require.NoError(s.T(), s.cache.DeleteVerificationCode(s.ctx, "nonexistent@example.com"), "DeleteVerificationCode non-existent")
}

func (s *EmailCacheSuite) TestGetVerificationCode_JSONCorruption() {
	emailKey := verifyCodeKeyPrefix + "corrupted@example.com"

	require.NoError(s.T(), s.rdb.Set(s.ctx, emailKey, "not-json", 1*time.Minute).Err(), "Set invalid JSON")

	_, err := s.cache.GetVerificationCode(s.ctx, "corrupted@example.com")
	require.Error(s.T(), err, "expected error for corrupted JSON")
	require.False(s.T(), errors.Is(err, redis.Nil), "expected decoding error, not redis.Nil")
}

func (s *EmailCacheSuite) TestIncrVerificationCodeAttempts_ConcurrentCapAndTTL() {
	// V01-R08：真 Redis 上验证 Lua 原子递增（并发 20 次错误码不超过 5 次比较）和 TTL 跟随验证码。
	email := "atomic-int@example.com"
	require.NoError(s.T(), s.cache.SetVerificationCode(s.ctx, email, &service.VerificationCodeData{
		Code:      "123456",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}, 15*time.Minute))

	svc := service.NewEmailService(nil, s.cache)
	const workers = 20
	var invalid, maxed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch err := svc.VerifyCode(s.ctx, email, "000000"); {
			case errors.Is(err, service.ErrInvalidVerifyCode):
				invalid.Add(1)
			case errors.Is(err, service.ErrVerifyCodeMaxAttempts):
				maxed.Add(1)
			default:
				s.T().Errorf("unexpected result: %v", err)
			}
		}()
	}
	wg.Wait()
	require.LessOrEqual(s.T(), int(invalid.Load()), 4)
	require.Equal(s.T(), workers, int(invalid.Load()+maxed.Load()))
	require.ErrorIs(s.T(), svc.VerifyCode(s.ctx, email, "123456"), service.ErrVerifyCodeMaxAttempts)

	attemptsTTL, err := s.rdb.TTL(s.ctx, verifyCodeKey(email)+attemptsKeySuffix).Result()
	require.NoError(s.T(), err)
	s.AssertTTLWithin(attemptsTTL, 14*time.Minute, 15*time.Minute)

	// 重新发码：计数清零。
	require.NoError(s.T(), s.cache.SetVerificationCode(s.ctx, email, &service.VerificationCodeData{Code: "654321"}, time.Minute))
	data, err := s.cache.GetVerificationCode(s.ctx, email)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, data.Attempts)
	require.NoError(s.T(), svc.VerifyCode(s.ctx, email, "654321"))
	exists, err := s.rdb.Exists(s.ctx, verifyCodeKey(email), verifyCodeKey(email)+attemptsKeySuffix).Result()
	require.NoError(s.T(), err)
	require.Zero(s.T(), exists)
}

func (s *EmailCacheSuite) TestConsumePasswordResetToken_RealRedisCompareAndDelete() {
	email := "reset-int@example.com"
	require.NoError(s.T(), s.cache.SetPasswordResetToken(s.ctx, email, &service.PasswordResetTokenData{Token: "hash-a", CreatedAt: time.Now()}, time.Minute))

	ok, err := s.cache.ConsumePasswordResetToken(s.ctx, email, "hash-b")
	require.NoError(s.T(), err)
	require.False(s.T(), ok)

	const workers = 20
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := s.cache.ConsumePasswordResetToken(s.ctx, email, "hash-a"); err == nil && ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(s.T(), int32(1), wins.Load())

	// 损坏的 JSON 不报错、不删除。
	require.NoError(s.T(), s.rdb.Set(s.ctx, passwordResetKey(email), "not-json", time.Minute).Err())
	ok, err = s.cache.ConsumePasswordResetToken(s.ctx, email, "hash-a")
	require.NoError(s.T(), err)
	require.False(s.T(), ok)
}

func TestEmailCacheSuite(t *testing.T) {
	suite.Run(t, new(EmailCacheSuite))
}
