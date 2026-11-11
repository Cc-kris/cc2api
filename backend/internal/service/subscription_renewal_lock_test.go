//go:build unit

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// lockingRenewalRepo 取自上游 db725a775：stale 模拟事务外读到的旧行，current 是加锁重读的当前行。
type lockingRenewalRepo struct {
	userSubRepoNoop
	mu        sync.Mutex
	stale     UserSubscription
	current   UserSubscription
	lockReads int
}

func (r *lockingRenewalRepo) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	return true, nil
}

func (r *lockingRenewalRepo) GetByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	copy := r.stale
	return &copy, nil
}

func (r *lockingRenewalRepo) GetByID(_ context.Context, _ int64) (*UserSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := r.current
	return &copy, nil
}

func (r *lockingRenewalRepo) GetByIDForUpdate(_ context.Context, _ int64) (*UserSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lockReads++
	copy := r.current
	return &copy, nil
}

func (r *lockingRenewalRepo) ExtendExpiry(_ context.Context, _ int64, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current.ExpiresAt = expiresAt
	return nil
}

func (r *lockingRenewalRepo) UpdateStatus(_ context.Context, _ int64, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current.Status = status
	return nil
}

func (r *lockingRenewalRepo) UpdateNotes(_ context.Context, _ int64, notes string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current.Notes = notes
	return nil
}

func (r *lockingRenewalRepo) Update(_ context.Context, sub *UserSubscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current = *sub
	return nil
}

// 续期必须用加锁重读的当前行计算，而不是事务外读到的旧行。
func TestAssignOrExtendSubscriptionUsesLockedCurrentRow(t *testing.T) {
	lockedExpiry := time.Now().Add(20 * 24 * time.Hour).Truncate(time.Second)
	repo := &lockingRenewalRepo{
		stale:   UserSubscription{ID: 7, UserID: 11, GroupID: 13, ExpiresAt: time.Now().Add(5 * 24 * time.Hour), Status: SubscriptionStatusActive, Notes: "stale"},
		current: UserSubscription{ID: 7, UserID: 11, GroupID: 13, ExpiresAt: lockedExpiry, Status: SubscriptionStatusActive, Notes: "current"},
	}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{ID: 13, SubscriptionType: SubscriptionTypeSubscription}}, repo, nil, nil, nil)

	sub, extended, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{
		UserID: 11, GroupID: 13, ValidityDays: 5, Notes: "renewed",
	})
	require.NoError(t, err)
	require.True(t, extended)
	require.Equal(t, 1, repo.lockReads)
	require.Equal(t, lockedExpiry.AddDate(0, 0, 5), sub.ExpiresAt)
	require.Equal(t, "current\nrenewed", sub.Notes)
}

// 逐次续期累加（并发的真实验证在 repository 集成测试 TestAssignOrExtendSubscription_ConcurrentRenewalsAccumulate）。
func TestAssignOrExtendSubscriptionSerializedRenewalsAccumulateDays(t *testing.T) {
	initialExpiry := time.Now().Add(10 * 24 * time.Hour).Truncate(time.Second)
	stale := UserSubscription{ID: 17, UserID: 21, GroupID: 23, ExpiresAt: initialExpiry, Status: SubscriptionStatusActive}
	repo := &lockingRenewalRepo{stale: stale, current: stale}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{ID: 23, SubscriptionType: SubscriptionTypeSubscription}}, repo, nil, nil, nil)
	input := &AssignSubscriptionInput{UserID: 21, GroupID: 23, ValidityDays: 7}

	_, _, err := svc.AssignOrExtendSubscription(context.Background(), input)
	require.NoError(t, err)
	second, _, err := svc.AssignOrExtendSubscription(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 2, repo.lockReads)
	require.Equal(t, initialExpiry.AddDate(0, 0, 14), second.ExpiresAt)
}

// 退款扣天和管理员调整用的 ExtendSubscription 同样加锁重读。
func TestExtendSubscriptionUsesLockedCurrentRow(t *testing.T) {
	initialExpiry := time.Now().Add(10 * 24 * time.Hour).Truncate(time.Second)
	repo := &lockingRenewalRepo{current: UserSubscription{ID: 37, UserID: 41, GroupID: 43, ExpiresAt: initialExpiry, Status: SubscriptionStatusActive}}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	updated, err := svc.ExtendSubscription(context.Background(), 37, 5)
	require.NoError(t, err)
	require.Equal(t, 1, repo.lockReads)
	require.Equal(t, initialExpiry.AddDate(0, 0, 5), updated.ExpiresAt)

	// 扣到过期仍返回 ErrAdjustWouldExpire（退款据此撤销订阅），且不改到期时间。
	_, err = svc.ExtendSubscription(context.Background(), 37, -30)
	require.ErrorIs(t, err, ErrAdjustWouldExpire)
	require.Equal(t, initialExpiry.AddDate(0, 0, 5), repo.current.ExpiresAt)
}
