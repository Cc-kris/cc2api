package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type financeRevenueRecognitionRepoStub struct {
	orders       []FinanceSubscriptionOrder
	usages       []FinanceSubscriptionUsage
	recognitions []FinanceRevenueRecognition
	allocations  [][]FinanceRevenueAllocation
	oldest       *time.Time
	listedDates  []time.Time
	locks        []time.Time
	events       []string
}

func (s *financeRevenueRecognitionRepoStub) AcquireSubscriptionRevenueDateLock(_ context.Context, date time.Time, _ string) (func() error, error) {
	s.locks = append(s.locks, date)
	s.events = append(s.events, "lock")
	return func() error {
		s.events = append(s.events, "unlock")
		return nil
	}, nil
}

func (s *financeRevenueRecognitionRepoStub) OldestUnrecognizedSubscriptionDate(context.Context, time.Time, string) (*time.Time, error) {
	return s.oldest, nil
}

func (s *financeRevenueRecognitionRepoStub) ListSubscriptionOrdersForDate(_ context.Context, date time.Time, _ string) ([]FinanceSubscriptionOrder, error) {
	s.listedDates = append(s.listedDates, date)
	s.events = append(s.events, "orders")
	return s.orders, nil
}
func (s *financeRevenueRecognitionRepoStub) ListSubscriptionUsageForDate(context.Context, FinanceSubscriptionOrder, time.Time, string) ([]FinanceSubscriptionUsage, error) {
	s.events = append(s.events, "usage")
	return s.usages, nil
}
func (s *financeRevenueRecognitionRepoStub) SaveSubscriptionRecognition(_ context.Context, recognition FinanceRevenueRecognition, allocations []FinanceRevenueAllocation) error {
	s.events = append(s.events, "save")
	s.recognitions = append(s.recognitions, recognition)
	s.allocations = append(s.allocations, allocations)
	return nil
}

func TestFinanceRevenueRecognitionUsesConfiguredTimezoneAndCalendarDaysAcrossDST(t *testing.T) {
	repo := &financeRevenueRecognitionRepoStub{}
	svc := NewFinanceRevenueRecognitionService(repo, &config.Config{Timezone: "America/New_York"})
	require.Equal(t, "America/New_York", svc.Timezone())

	days, err := svc.RecognizeRange(
		context.Background(),
		time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC),
		svc.Timezone(),
		3,
	)
	require.NoError(t, err)
	require.Equal(t, 3, days)
	require.Equal(t, []string{"2026-03-07", "2026-03-08", "2026-03-09"}, []string{
		repo.listedDates[0].Format("2006-01-02"),
		repo.listedDates[1].Format("2006-01-02"),
		repo.listedDates[2].Format("2006-01-02"),
	})
}

func TestFinanceRevenueRecognitionLockCoversReadCalculateAndSave(t *testing.T) {
	date := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	repo := &financeRevenueRecognitionRepoStub{orders: []FinanceSubscriptionOrder{{
		OrderID: 1, UserID: 2, Amount: decimal.NewFromInt(30), ServiceStartDate: date, ServiceDays: 30,
	}}}
	_, err := NewFinanceRevenueRecognitionService(repo, nil).RecognizeDate(context.Background(), date, "UTC")
	require.NoError(t, err)
	require.Equal(t, []string{"lock", "orders", "usage", "save", "unlock"}, repo.events)
}

func TestCalculateSubscriptionDailyRevenueStraightLineAndRefund(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	refundDate := start.AddDate(0, 0, 10)
	order := FinanceSubscriptionOrder{
		Amount: decimal.NewFromInt(300), RefundAmount: decimal.NewFromInt(60),
		ServiceStartDate: start, ServiceDays: 30, RefundDate: &refundDate,
	}

	before, reduction, ok := calculateSubscriptionDailyRevenue(order, start.AddDate(0, 0, 1), time.UTC)
	require.True(t, ok)
	require.True(t, before.Equal(decimal.NewFromInt(10)))
	require.True(t, reduction.IsZero())

	onRefund, reduction, ok := calculateSubscriptionDailyRevenue(order, refundDate, time.UTC)
	require.True(t, ok)
	require.True(t, onRefund.Equal(decimal.NewFromInt(-12)))
	require.True(t, reduction.Equal(decimal.NewFromInt(22)))

	after, reduction, ok := calculateSubscriptionDailyRevenue(order, refundDate.AddDate(0, 0, 1), time.UTC)
	require.True(t, ok)
	require.True(t, after.Equal(decimal.NewFromInt(8)))
	require.True(t, reduction.Equal(decimal.NewFromInt(2)))

	total := decimal.Zero
	for day := 0; day < 30; day++ {
		amount, _, included := calculateSubscriptionDailyRevenue(order, start.AddDate(0, 0, day), time.UTC)
		require.True(t, included)
		total = total.Add(amount)
	}
	require.True(t, total.Equal(decimal.NewFromInt(240)))
}

func TestCalculateSubscriptionDailyRevenueUsesCivilDaysAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	start := time.Date(2026, 3, 7, 0, 0, 0, 0, location)
	refundDate := time.Date(2026, 3, 9, 0, 0, 0, 0, location)
	order := FinanceSubscriptionOrder{
		Amount: decimal.NewFromInt(300), RefundAmount: decimal.NewFromInt(60),
		ServiceStartDate: start, ServiceDays: 30, RefundDate: &refundDate,
	}

	recognized, reduction, ok := calculateSubscriptionDailyRevenue(order, refundDate, location)
	require.True(t, ok)
	require.True(t, recognized.Equal(decimal.NewFromInt(4)))
	require.True(t, reduction.Equal(decimal.NewFromInt(6)))

	total := decimal.Zero
	for day := 0; day < 30; day++ {
		amount, _, included := calculateSubscriptionDailyRevenue(order, start.AddDate(0, 0, day), location)
		require.True(t, included)
		total = total.Add(amount)
	}
	require.True(t, total.Equal(decimal.NewFromInt(240)))
}

func TestFinanceRevenueRecognitionBackfillsOldestMissingDatesInBoundedBatches(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	repo := &financeRevenueRecognitionRepoStub{oldest: &start}
	days, err := NewFinanceRevenueRecognitionService(repo, nil).BackfillUnrecognized(
		context.Background(), start.AddDate(0, 0, 10), "UTC", 3,
	)
	require.NoError(t, err)
	require.Equal(t, 3, days)
	require.Len(t, repo.listedDates, 3)
	require.Equal(t, "2026-07-01", repo.listedDates[0].Format("2006-01-02"))
	require.Equal(t, "2026-07-03", repo.listedDates[2].Format("2006-01-02"))
}

func TestAllocateSubscriptionRevenueByUsageValueAndRequestCount(t *testing.T) {
	weighted := allocateSubscriptionRevenue(decimal.NewFromInt(10), []FinanceSubscriptionUsage{
		{UsageLogID: 1, UsageListValue: decimal.NewFromInt(3)},
		{UsageLogID: 2, UsageListValue: decimal.NewFromInt(1)},
	})
	require.Len(t, weighted, 2)
	require.True(t, weighted[0].Amount.Equal(decimal.RequireFromString("7.5")))
	require.True(t, weighted[1].Amount.Equal(decimal.RequireFromString("2.5")))
	require.Equal(t, "usage_list_value", weighted[0].Method)

	equal := allocateSubscriptionRevenue(decimal.NewFromInt(9), []FinanceSubscriptionUsage{{UsageLogID: 1}, {UsageLogID: 2}, {UsageLogID: 3}})
	require.Len(t, equal, 3)
	for _, allocation := range equal {
		require.True(t, allocation.Amount.Equal(decimal.NewFromInt(3)))
		require.Equal(t, "request_count", allocation.Method)
	}

	mixed := allocateSubscriptionRevenue(decimal.NewFromInt(100), []FinanceSubscriptionUsage{
		{UsageLogID: 1, UsageListValue: decimal.NewFromInt(10)},
		{UsageLogID: 2, UsageListValue: decimal.Zero},
		{UsageLogID: 3, UsageListValue: decimal.NewFromInt(10)},
	})
	require.Equal(t, "50", mixed[0].Amount.String())
	require.True(t, mixed[1].Amount.IsZero())
	require.Equal(t, "50", mixed[2].Amount.String())
}

func TestFinanceRevenueRecognitionKeepsUnallocatedRevenue(t *testing.T) {
	date := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	repo := &financeRevenueRecognitionRepoStub{orders: []FinanceSubscriptionOrder{{
		OrderID: 1, UserID: 2, Amount: decimal.NewFromInt(30), ServiceStartDate: date, ServiceDays: 30,
	}}}
	processed, err := NewFinanceRevenueRecognitionService(repo, nil).RecognizeDate(context.Background(), date, "UTC")
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	require.Len(t, repo.recognitions, 1)
	require.Equal(t, "unallocated", repo.recognitions[0].AllocationStatus)
	require.True(t, repo.recognitions[0].UnallocatedRevenue.Equal(decimal.NewFromInt(1)))
	require.Empty(t, repo.allocations[0])
}

func TestFinanceRevenueRecognitionFreeSubscriptionStaysZero(t *testing.T) {
	date := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	repo := &financeRevenueRecognitionRepoStub{orders: []FinanceSubscriptionOrder{{
		OrderID: 1, UserID: 2, Amount: decimal.Zero, ServiceStartDate: date, ServiceDays: 30,
	}}}
	_, err := NewFinanceRevenueRecognitionService(repo, nil).RecognizeDate(context.Background(), date, "UTC")
	require.NoError(t, err)
	require.True(t, repo.recognitions[0].RecognizedRevenue.IsZero())
	require.Equal(t, "allocated", repo.recognitions[0].AllocationStatus)
}
