//go:build integration

package repository

// V03-R11 用量审计字段集成测试：三条写入路径（单条 tx、批量、best-effort）都能落库并读回；
// 未提供时保持 NULL（含三态 mismatch）。上游参考 1c0cb24c7 usage_log_session_id_integration_test.go。

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func usageAuditFieldsForTest(suffix string) service.UsageAuditFields {
	responseModel := "claude-sonnet-4-5-" + suffix
	mismatch := true
	sessionID := "sess-" + suffix
	effort := "max"
	upstreamRequestID := "req_" + suffix
	return service.UsageAuditFields{
		UpstreamResponseModel:    &responseModel,
		UpstreamModelMismatch:    &mismatch,
		SessionID:                &sessionID,
		RequestedReasoningEffort: &effort,
		UpstreamRequestID:        &upstreamRequestID,
	}
}

func requireUsageAuditFieldsEqual(t *testing.T, want, got service.UsageAuditFields) {
	t.Helper()
	require.Equal(t, want.UpstreamResponseModel, got.UpstreamResponseModel)
	require.Equal(t, want.UpstreamModelMismatch, got.UpstreamModelMismatch)
	require.Equal(t, want.SessionID, got.SessionID)
	require.Equal(t, want.RequestedReasoningEffort, got.RequestedReasoningEffort)
	require.Equal(t, want.UpstreamRequestID, got.UpstreamRequestID)
}

func (s *UsageLogRepoSuite) TestCreate_PersistsUsageAuditFields() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "audit-fields@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-audit-fields", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-audit-fields"})

	audit := usageAuditFieldsForTest("tx")
	log := &service.UsageLog{
		UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID,
		RequestID: uuid.NewString(), Model: "claude-sonnet-4",
		InputTokens: 10, OutputTokens: 5, TotalCost: 0.5, ActualCost: 0.5,
		UsageAuditFields: audit,
		CreatedAt:        time.Now().UTC(),
	}
	_, err := s.repo.Create(s.ctx, log)
	s.Require().NoError(err)

	got, err := s.repo.GetByID(s.ctx, log.ID)
	s.Require().NoError(err)
	requireUsageAuditFieldsEqual(s.T(), audit, got.UsageAuditFields)

	empty := &service.UsageLog{
		UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID,
		RequestID: uuid.NewString(), Model: "claude-sonnet-4",
		InputTokens: 10, OutputTokens: 5, TotalCost: 0.5, ActualCost: 0.5,
		CreatedAt: time.Now().UTC(),
	}
	_, err = s.repo.Create(s.ctx, empty)
	s.Require().NoError(err)
	var mismatchIsNull, sessionIsNull bool
	s.Require().NoError(scanSingleRow(s.ctx, s.tx,
		"SELECT upstream_model_mismatch IS NULL, session_id IS NULL FROM usage_logs WHERE id = $1",
		[]any{empty.ID}, &mismatchIsNull, &sessionIsNull))
	s.Require().True(mismatchIsNull, "absent mismatch must stay NULL, not false")
	s.Require().True(sessionIsNull)
	gotEmpty, err := s.repo.GetByID(s.ctx, empty.ID)
	s.Require().NoError(err)
	requireUsageAuditFieldsEqual(s.T(), service.UsageAuditFields{}, gotEmpty.UsageAuditFields)
}

func TestUsageLogRepository_AuditFieldsBatchAndBestEffortPaths(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{Email: fmt.Sprintf("usage-audit-%d@example.com", time.Now().UnixNano())})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-usage-audit-" + uuid.NewString(), Name: "k"})
	account := mustCreateAccount(t, client, &service.Account{Name: "acc-usage-audit-" + uuid.NewString()})

	newLog := func(suffix string) *service.UsageLog {
		return &service.UsageLog{
			UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID,
			RequestID: "audit-" + suffix + "-" + uuid.NewString(), Model: "gpt-5.1",
			InputTokens: 10, OutputTokens: 5, TotalCost: 0.5, ActualCost: 0.5,
			UsageAuditFields: usageAuditFieldsForTest(suffix),
			CreatedAt:        time.Now().UTC(),
		}
	}

	batched := newLog("batch")
	inserted, err := repo.Create(ctx, batched)
	require.NoError(t, err)
	require.True(t, inserted)
	got, err := repo.GetByID(ctx, batched.ID)
	require.NoError(t, err)
	requireUsageAuditFieldsEqual(t, batched.UsageAuditFields, got.UsageAuditFields)

	bestEffort := newLog("best")
	require.NoError(t, repo.CreateBestEffort(ctx, bestEffort))
	var (
		responseModel, sessionID, effort, upstreamRequestID string
		mismatch                                            bool
	)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT upstream_response_model, upstream_model_mismatch, session_id, requested_reasoning_effort, upstream_request_id
		FROM usage_logs WHERE request_id = $1 AND api_key_id = $2`, bestEffort.RequestID, apiKey.ID,
	).Scan(&responseModel, &mismatch, &sessionID, &effort, &upstreamRequestID))
	require.Equal(t, *bestEffort.UpstreamResponseModel, responseModel)
	require.True(t, mismatch)
	require.Equal(t, *bestEffort.SessionID, sessionID)
	require.Equal(t, "max", effort)
	require.Equal(t, *bestEffort.UpstreamRequestID, upstreamRequestID)
}

// AC31（后半）：管理员按 upstream_request_id 精确查询、按「模型不一致」筛选；统计与列表口径一致。
func (s *UsageLogRepoSuite) TestListWithFilters_UsageAuditFilters() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "audit-filter@test.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-audit-filter", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-audit-filter"})

	create := func(upstreamRequestID string, mismatch *bool, cost float64) {
		log := &service.UsageLog{
			UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID,
			RequestID: uuid.NewString(), Model: "gpt-5.1",
			InputTokens: 10, OutputTokens: 5, TotalCost: cost, ActualCost: cost,
			CreatedAt: time.Now().UTC(),
		}
		if upstreamRequestID != "" {
			log.UpstreamRequestID = &upstreamRequestID
		}
		log.UpstreamModelMismatch = mismatch
		_, err := s.repo.Create(s.ctx, log)
		s.Require().NoError(err)
	}
	yes, no := true, false
	create("req_abc", &yes, 0.4)
	create("req_other", &no, 0.2)
	create("", nil, 0.1)

	byRequestID, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, UsageLogFilters{UserID: user.ID, UpstreamRequestID: "req_abc"})
	s.Require().NoError(err)
	s.Require().Len(byRequestID, 1)
	s.Require().Equal(int64(1), page.Total)
	s.Require().Equal("req_abc", *byRequestID[0].UpstreamRequestID)

	mismatched, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, UsageLogFilters{UserID: user.ID, UpstreamModelMismatch: &yes})
	s.Require().NoError(err)
	s.Require().Len(mismatched, 1)
	s.Require().True(*mismatched[0].UpstreamModelMismatch)

	matched, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, UsageLogFilters{UserID: user.ID, UpstreamModelMismatch: &no})
	s.Require().NoError(err)
	s.Require().Len(matched, 1, "mismatch=false excludes unobserved (NULL) rows")

	all, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, UsageLogFilters{UserID: user.ID})
	s.Require().NoError(err)
	s.Require().Len(all, 3)

	stats, err := s.repo.GetStatsWithFilters(s.ctx, UsageLogFilters{UserID: user.ID, UpstreamModelMismatch: &yes})
	s.Require().NoError(err)
	s.Require().Equal(int64(1), stats.TotalRequests)
	s.Require().InDelta(0.4, stats.TotalActualCost, 1e-9)
}
