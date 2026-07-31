//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestAccountFinanceProfileVersionsAndReadiness(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var userID, accountID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, fmt.Sprintf("finance-profile-%d@example.test", suffix)).Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type,credentials,upstream_cost_multiplier,upstream_cost_multiplier_updated_at) VALUES($1,'openai','api_key','{}',0.22,NOW()) RETURNING id`, fmt.Sprintf("finance-profile-%d", suffix)).Scan(&accountID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `UPDATE accounts SET current_finance_profile_id=NULL WHERE id=$1`, accountID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_finance_profiles WHERE account_id=$1`, accountID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id=$1`, accountID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	repo := NewAccountFinanceProfileRepository(integrationDB, nil, nil)
	svc := service.NewAccountFinanceProfileService(repo)
	effectiveFrom := time.Now().UTC().Add(time.Second).Truncate(time.Microsecond)
	contract := decimal.RequireFromString("0.22")
	created, err := svc.Save(ctx, accountID, service.AccountFinanceProfileInput{
		CostMode: service.FinanceCostModeContractMultiplier, ContractType: stringPtrRepository("multiplier"), ContractMultiplier: &contract,
		EndpointSource: "account_base_url", CredentialSource: "account_api_key", CounterScope: service.FinanceCounterScopeAccount,
		BalanceUnitSemantics: service.FinanceUnitNone, EffectiveFrom: effectiveFrom, ExpectedVersion: 0,
		Reason: "初始化账号财务配置", OperatorID: userID,
	})
	require.NoError(t, err)
	require.Equal(t, 1, created.Version)
	require.Equal(t, service.AccountFinanceReadinessReadyContract, created.ReadinessStatus)

	updated, err := svc.Save(ctx, accountID, service.AccountFinanceProfileInput{
		CostMode: service.FinanceCostModeManual, EndpointSource: "account_base_url", CredentialSource: "account_api_key",
		CounterScope: service.FinanceCounterScopeAccount, BalanceUnitSemantics: service.FinanceUnitNone,
		EffectiveFrom: effectiveFrom.Add(time.Second), ExpectedVersion: 1, Reason: "切换为目录价格优先", OperatorID: userID,
	})
	require.NoError(t, err)
	require.Equal(t, 2, updated.Version)
	require.Equal(t, service.AccountFinanceReadinessReadyContract, updated.ReadinessStatus)

	_, err = svc.Save(ctx, accountID, service.AccountFinanceProfileInput{
		CostMode: service.FinanceCostModeManual, EndpointSource: "account_base_url", CredentialSource: "account_api_key",
		CounterScope: service.FinanceCounterScopeAccount, BalanceUnitSemantics: service.FinanceUnitNone,
		EffectiveFrom: effectiveFrom.Add(2 * time.Second), ExpectedVersion: 1, Reason: "使用过期版本提交", OperatorID: userID,
	})
	require.True(t, errors.Is(err, service.ErrAccountFinanceProfileConflict), "err=%v", err)

	readiness, err := svc.Readiness(ctx, accountID)
	require.NoError(t, err)
	require.Equal(t, service.AccountFinanceReadinessReadyContract, readiness.Status)
	require.Equal(t, 2, readiness.Profile.Version)
	var historicalCount, currentCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FILTER (WHERE effective_to IS NOT NULL),COUNT(*) FILTER (WHERE effective_to IS NULL) FROM account_finance_profiles WHERE account_id=$1`, accountID).Scan(&historicalCount, &currentCount))
	require.Equal(t, 1, historicalCount)
	require.Equal(t, 1, currentCount)
}

func TestAccountFinanceProfileBackfillNullMultiplierIsUnconfigured(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var accountID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO accounts(name,platform,type,credentials,upstream_cost_multiplier) VALUES($1,'openai','api_key','{}',NULL) RETURNING id`,
		fmt.Sprintf("finance-profile-backfill-%d", suffix),
	).Scan(&accountID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_finance_profiles WHERE account_id=$1`, accountID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id=$1`, accountID)
	})

	backfill, err := migrations.FS.ReadFile("197_backfill_account_finance_profiles.sql")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, string(backfill))
	require.NoError(t, err)

	var costMode, readiness string
	var contractType, contractMultiplier, contractChangeID any
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT cost_mode,readiness_status,contract_type,contract_multiplier,contract_multiplier_change_id
		FROM account_finance_profiles WHERE account_id=$1`, accountID).
		Scan(&costMode, &readiness, &contractType, &contractMultiplier, &contractChangeID))
	require.Equal(t, "contract_multiplier", costMode)
	require.Equal(t, "unconfigured", readiness)
	require.Nil(t, contractType)
	require.Nil(t, contractMultiplier)
	require.Nil(t, contractChangeID)
}

func stringPtrRepository(value string) *string { return &value }

func TestAccountMultiplierChangeCreatesProfileVersionAndFreezesUsageEvidence(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	suffix := uuid.NewString()
	user := mustCreateUser(t, client, &service.User{Email: "finance-evidence-" + suffix + "@example.test"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-finance-evidence-" + suffix, Name: "finance-evidence"})
	oldMultiplier := decimal.RequireFromString("0.22")
	initialEffectiveAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	account := &service.Account{
		Name: "finance-evidence-" + suffix, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{}, Extra: map[string]any{}, Concurrency: 1, Priority: 1,
		Status: service.StatusActive, Schedulable: true,
		UpstreamCostMultiplier: &oldMultiplier, UpstreamCostMultiplierUpdatedAt: &initialEffectiveAt,
	}
	accountRepo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	require.NoError(t, accountRepo.CreateWithUpstreamMultiplierAudit(ctx, account, nil, "初始化财务证据测试账号"))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM scheduler_outbox WHERE account_id=$1`, account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_upstream_attempts WHERE account_id=$1`, account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_logs WHERE account_id=$1`, account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM api_keys WHERE id=$1`, apiKey.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `UPDATE accounts SET current_finance_profile_id=NULL,upstream_cost_multiplier_change_id=NULL WHERE id=$1`, account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_finance_profiles WHERE account_id=$1`, account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM account_upstream_multiplier_changes WHERE account_id=$1`, account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id=$1`, account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, user.ID)
	})
	require.NotNil(t, account.UpstreamCostMultiplierChangeID)
	require.NotNil(t, account.CurrentFinanceProfileID)
	initialChangeID := *account.UpstreamCostMultiplierChangeID
	initialProfileID := *account.CurrentFinanceProfileID

	usageRepo := newUsageLogRepositoryWithSQL(client, integrationDB)
	createUsage := func(requestID string, selectedAccount *service.Account, createdAt time.Time) *service.UsageLog {
		log := &service.UsageLog{
			UserID: user.ID, APIKeyID: apiKey.ID, AccountID: selectedAccount.ID, RequestID: requestID,
			Model: "gpt-5", InputTokens: 100, OutputTokens: 20, TotalCost: 1, ActualCost: 1,
			UpstreamCostMultiplier: service.CloneDecimalSnapshot(selectedAccount.UpstreamCostMultiplier), CreatedAt: createdAt,
		}
		service.ApplyAccountFinanceEvidenceToUsageLog(log, selectedAccount)
		service.EnsureFinalUsageUpstreamAttempt(log)
		inserted, err := usageRepo.createWithAttempts(ctx, integrationDB, log)
		require.NoError(t, err)
		require.True(t, inserted)
		return log
	}
	first := createUsage("finance-evidence-a-"+suffix, account, initialEffectiveAt.Add(time.Second))

	newMultiplier := decimal.RequireFromString("0.31")
	changedAt := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, accountRepo.UpdateUpstreamMultiplierWithAudit(ctx, account.ID, &oldMultiplier, newMultiplier, changedAt, nil, "上游合同倍率调整"))
	currentAccount, err := accountRepo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, currentAccount.UpstreamCostMultiplierChangeID)
	require.NotNil(t, currentAccount.CurrentFinanceProfileID)
	require.NotEqual(t, initialChangeID, *currentAccount.UpstreamCostMultiplierChangeID)
	require.NotEqual(t, initialProfileID, *currentAccount.CurrentFinanceProfileID)
	second := createUsage("finance-evidence-b-"+suffix, currentAccount, changedAt.Add(time.Second))

	storedFirst, err := usageRepo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	storedSecond, err := usageRepo.GetByID(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, initialChangeID, *storedFirst.UpstreamMultiplierChangeID)
	require.Equal(t, initialProfileID, *storedFirst.AccountFinanceProfileID)
	require.True(t, oldMultiplier.Equal(*storedFirst.UpstreamCostMultiplier))
	require.Equal(t, *currentAccount.UpstreamCostMultiplierChangeID, *storedSecond.UpstreamMultiplierChangeID)
	require.Equal(t, *currentAccount.CurrentFinanceProfileID, *storedSecond.AccountFinanceProfileID)
	require.True(t, newMultiplier.Equal(*storedSecond.UpstreamCostMultiplier))

	var firstAttemptChangeID, firstAttemptProfileID, secondAttemptChangeID, secondAttemptProfileID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT upstream_multiplier_change_id,account_finance_profile_id FROM usage_upstream_attempts WHERE usage_log_id=$1`, first.ID).Scan(&firstAttemptChangeID, &firstAttemptProfileID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT upstream_multiplier_change_id,account_finance_profile_id FROM usage_upstream_attempts WHERE usage_log_id=$1`, second.ID).Scan(&secondAttemptChangeID, &secondAttemptProfileID))
	require.Equal(t, initialChangeID, firstAttemptChangeID)
	require.Equal(t, initialProfileID, firstAttemptProfileID)
	require.Equal(t, *currentAccount.UpstreamCostMultiplierChangeID, secondAttemptChangeID)
	require.Equal(t, *currentAccount.CurrentFinanceProfileID, secondAttemptProfileID)

	var profileCount int
	var currentSnapshot, currentContract decimal.Decimal
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*),MAX(account_multiplier_snapshot) FILTER (WHERE effective_to IS NULL),MAX(contract_multiplier) FILTER (WHERE effective_to IS NULL) FROM account_finance_profiles WHERE account_id=$1`, account.ID).Scan(&profileCount, &currentSnapshot, &currentContract))
	require.Equal(t, 2, profileCount)
	require.True(t, newMultiplier.Equal(currentSnapshot))
	require.True(t, newMultiplier.Equal(currentContract))

}
