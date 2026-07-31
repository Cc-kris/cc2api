package repository

import (
	"context"
	"database/sql"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/usagefinancecostsegment"
	"github.com/Wei-Shaw/sub2api/ent/usagefinancerecord"
	"github.com/Wei-Shaw/sub2api/ent/usagerevenueallocation"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type financeDetailRepository struct {
	client *dbent.Client
	sql    *sql.DB
}

func NewFinanceDetailRepository(client *dbent.Client, sqlDB *sql.DB) service.FinanceDetailRepository {
	return &financeDetailRepository{client: client, sql: sqlDB}
}

func (r *financeDetailRepository) GetUsageFinanceDetailFacts(ctx context.Context, usageLogID int64) (*service.FinanceUsageDetailFacts, error) {
	row := r.sql.QueryRowContext(ctx, "SELECT "+usageLogSelectColumns+" FROM usage_logs WHERE id = $1", usageLogID)
	usage, err := scanUsageLog(row)
	if err == sql.ErrNoRows {
		return nil, service.ErrUsageLogNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get finance usage log: %w", err)
	}
	facts := &service.FinanceUsageDetailFacts{Usage: usage, AccountNames: map[int64]string{}}
	record, err := r.client.UsageFinanceRecord.Query().
		Where(usagefinancerecord.UsageLogIDEQ(usageLogID)).
		Only(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, fmt.Errorf("get usage finance projection: %w", err)
	}
	if record != nil {
		segments, queryErr := r.client.UsageFinanceCostSegment.Query().
			Where(usagefinancecostsegment.UsageFinanceRecordIDEQ(record.ID)).
			Order(dbent.Asc(usagefinancecostsegment.FieldAttemptNo)).
			All(ctx)
		if queryErr != nil {
			return nil, fmt.Errorf("get usage finance segments: %w", queryErr)
		}
		facts.Projection = financeEntityToProjection(record, segments)
		accountIDs := uniqueFinanceAccountIDs(segments)
		if len(accountIDs) > 0 {
			accounts, queryErr := r.client.Account.Query().
				Where(account.IDIn(accountIDs...)).
				Select(account.FieldID, account.FieldName).
				All(ctx)
			if queryErr != nil {
				return nil, fmt.Errorf("get finance segment account names: %w", queryErr)
			}
			for _, item := range accounts {
				facts.AccountNames[item.ID] = item.Name
			}
		}
	}
	allocations, err := r.client.UsageRevenueAllocation.Query().
		Where(
			usagerevenueallocation.UsageLogIDEQ(usageLogID),
			usagerevenueallocation.InvalidatedAtIsNil(),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("get usage revenue allocations: %w", err)
	}
	if len(allocations) > 0 {
		total := decimal.Zero
		for _, allocation := range allocations {
			total = total.Add(allocation.AllocatedAmount)
		}
		facts.Revenue = &total
	}
	return facts, nil
}

func uniqueFinanceAccountIDs(segments []*dbent.UsageFinanceCostSegment) []int64 {
	set := make(map[int64]struct{}, len(segments))
	for _, segment := range segments {
		if segment != nil && segment.AccountID > 0 {
			set[segment.AccountID] = struct{}{}
		}
	}
	result := make([]int64, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	return result
}
