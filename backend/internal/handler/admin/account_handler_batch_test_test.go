package admin

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestFilterBatchTestEligibleAccountsSkipsUnschedulableAccounts(t *testing.T) {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	accounts := []service.Account{
		{ID: 1, Name: "eligible", Status: service.StatusActive, Schedulable: true},
		{ID: 2, Name: "manual-paused", Status: service.StatusActive, Schedulable: false},
		{ID: 3, Name: "inactive", Status: service.StatusDisabled, Schedulable: true},
		{ID: 4, Name: "temporary-unschedulable", Status: service.StatusActive, Schedulable: true, TempUnschedulableUntil: &future},
		{ID: 5, Name: "expired-temp-unschedulable", Status: service.StatusActive, Schedulable: true, TempUnschedulableUntil: &past},
	}

	filtered := filterBatchTestEligibleAccounts(accounts)

	require.Equal(t, []int64{1, 5}, accountIDsForBatchTest(filtered))
}

func TestNewBatchAccountTestContextIgnoresRequestCancellation(t *testing.T) {
	requestCtx, requestCancel := context.WithCancel(context.Background())
	testCtx, testCancel := newBatchAccountTestContext(requestCtx)
	defer testCancel()

	requestCancel()

	select {
	case <-testCtx.Done():
		require.Failf(t, "batch account test context was canceled by request context", "err=%v", testCtx.Err())
	default:
	}
}

func accountIDsForBatchTest(accounts []service.Account) []int64 {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids
}
