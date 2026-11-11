//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func createRenewalLockFixture(t *testing.T, ctx context.Context, expiresAt time.Time) (userID, groupID, subID int64) {
	t.Helper()
	client := testEntClient(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	user, err := client.User.Create().
		SetEmail("renewal-lock-" + suffix + "@example.test").
		SetPasswordHash("hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetName("renewal-lock-" + suffix).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)
	sub, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(expiresAt.AddDate(0, 0, -30)).
		SetExpiresAt(expiresAt).
		SetStatus(service.SubscriptionStatusActive).
		SetAssignedAt(time.Now()).
		Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM user_subscriptions WHERE id=$1`, sub.ID)
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM groups WHERE id=$1`, group.ID)
		_, _ = integrationDB.ExecContext(bg, `DELETE FROM users WHERE id=$1`, user.ID)
	})
	return user.ID, group.ID, sub.ID
}

func newRenewalLockSubscriptionService(t *testing.T) *service.SubscriptionService {
	t.Helper()
	client := testEntClient(t)
	return service.NewSubscriptionService(
		NewGroupRepository(client, integrationDB),
		NewUserSubscriptionRepository(client),
		nil, client, &config.Config{},
	)
}

func subscriptionExpiresAtForTest(t *testing.T, ctx context.Context, subID int64) time.Time {
	t.Helper()
	var expiresAt time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT expires_at FROM user_subscriptions WHERE id=$1`, subID).Scan(&expiresAt))
	return expiresAt.UTC()
}

// V03-AC14：未过期订阅到期 2026-12-01 00:00:00 UTC，10 个并发续期各 30 天，
// 到期时间准确为 2027-09-27 00:00:00 UTC（+300 天）。没有行锁时会少于 300 天。
func TestAssignOrExtendSubscription_ConcurrentRenewalsAccumulate(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	if !start.After(time.Now()) {
		t.Skip("fixture date 2026-12-01 must be in the future for the not-yet-expired renewal branch")
	}
	userID, groupID, subID := createRenewalLockFixture(t, ctx, start)
	svc := newRenewalLockSubscriptionService(t)

	const workers = 10
	var wg sync.WaitGroup
	errs := make([]error, workers)
	renewed := make([]bool, workers)
	gate := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate
			_, isRenewal, err := svc.AssignOrExtendSubscription(context.Background(), &service.AssignSubscriptionInput{
				UserID: userID, GroupID: groupID, ValidityDays: 30, Notes: "",
			})
			errs[i] = err
			renewed[i] = isRenewal
		}(i)
	}
	close(gate)
	wg.Wait()

	for i := range errs {
		require.NoError(t, errs[i], "renewal %d", i)
		require.True(t, renewed[i], "renewal %d must extend the existing subscription", i)
	}
	want := time.Date(2027, 9, 27, 0, 0, 0, 0, time.UTC)
	require.True(t, want.Equal(subscriptionExpiresAtForTest(t, ctx, subID)),
		"expires_at = %s, want %s", subscriptionExpiresAtForTest(t, ctx, subID), want)
}

// ExtendSubscription（管理员调整、退款扣天都用它）同样在事务内加锁重读。
func TestExtendSubscription_ConcurrentAdjustmentsAccumulate(t *testing.T) {
	ctx := context.Background()
	start := time.Now().UTC().Truncate(time.Second).AddDate(0, 0, 100)
	_, _, subID := createRenewalLockFixture(t, ctx, start)
	svc := newRenewalLockSubscriptionService(t)

	const workers = 10
	var wg sync.WaitGroup
	errs := make([]error, workers)
	gate := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate
			days := 7
			if i%2 == 1 {
				days = -3
			}
			_, errs[i] = svc.ExtendSubscription(context.Background(), subID, days)
		}(i)
	}
	close(gate)
	wg.Wait()
	for i := range errs {
		require.NoError(t, errs[i], "adjust %d", i)
	}
	// 5×(+7) + 5×(-3) = +20 天
	want := start.AddDate(0, 0, 20)
	require.True(t, want.Equal(subscriptionExpiresAtForTest(t, ctx, subID)),
		"expires_at = %s, want %s", subscriptionExpiresAtForTest(t, ctx, subID), want)
}
