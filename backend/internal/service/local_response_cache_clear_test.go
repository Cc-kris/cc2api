package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type localResponseCacheClearStoreStub struct {
	result *LocalResponseCacheClearResult
	err    error
	audit  LocalResponseCacheClearAudit
	page   *LocalResponseCacheClearAuditPage
	filter LocalResponseCacheClearAuditFilter
}

func (s *localResponseCacheClearStoreStub) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, nil
}
func (s *localResponseCacheClearStoreStub) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}
func (s *localResponseCacheClearStoreStub) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}
func (s *localResponseCacheClearStoreStub) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}
func (s *localResponseCacheClearStoreStub) ClearLocalResponseCache(context.Context, LocalResponseCacheClearRequest) (*LocalResponseCacheClearResult, error) {
	return s.result, s.err
}
func (s *localResponseCacheClearStoreStub) RecordLocalResponseCacheClearAudit(_ context.Context, audit LocalResponseCacheClearAudit) error {
	s.audit = audit
	return nil
}
func (s *localResponseCacheClearStoreStub) ListLocalResponseCacheClearAudits(_ context.Context, filter LocalResponseCacheClearAuditFilter) (*LocalResponseCacheClearAuditPage, error) {
	s.filter = filter
	return s.page, nil
}

func TestOpenAIGatewayServiceClearLocalResponseCacheValidatesAllConfirmText(t *testing.T) {
	svc := &OpenAIGatewayService{cache: &localResponseCacheClearStoreStub{}}
	_, err := svc.ClearLocalResponseCache(context.Background(), LocalResponseCacheClearRequest{ClearType: LocalResponseCacheClearTypeAll})
	require.ErrorIs(t, err, ErrInvalidLocalResponseCacheClear)

	res, err := svc.ClearLocalResponseCache(context.Background(), LocalResponseCacheClearRequest{ClearType: LocalResponseCacheClearTypeAll, ConfirmText: LocalResponseCacheClearConfirmText})
	require.NoError(t, err)
	require.Equal(t, LocalResponseCacheClearStatusSuccess, res.Status)
}

func TestOpenAIGatewayServiceClearLocalResponseCacheValidatesScopedInputs(t *testing.T) {
	svc := &OpenAIGatewayService{cache: &localResponseCacheClearStoreStub{}}
	_, err := svc.ClearLocalResponseCache(context.Background(), LocalResponseCacheClearRequest{ClearType: LocalResponseCacheClearTypeByModel})
	require.ErrorIs(t, err, ErrInvalidLocalResponseCacheClear)

	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(32 * 24 * time.Hour)
	_, err = svc.ClearLocalResponseCache(context.Background(), LocalResponseCacheClearRequest{ClearType: LocalResponseCacheClearTypeByTime, Scope: LocalResponseCacheClearScope{StartTime: &start, EndTime: &end}})
	require.ErrorIs(t, err, ErrInvalidLocalResponseCacheClear)
}

func TestOpenAIGatewayServiceClearLocalResponseCacheWritesAudit(t *testing.T) {
	operatorID := int64(9)
	store := &localResponseCacheClearStoreStub{result: &LocalResponseCacheClearResult{MatchedKeys: 3, DeletedKeys: 2, Status: LocalResponseCacheClearStatusPartialSuccess, ErrorMessage: "one key vanished"}}
	svc := &OpenAIGatewayService{cache: store}

	res, err := svc.ClearLocalResponseCache(context.Background(), LocalResponseCacheClearRequest{ClearType: LocalResponseCacheClearTypeByAPIKey, Scope: LocalResponseCacheClearScope{APIKeyIDs: []int64{12}}, OperatorUserID: &operatorID})

	require.NoError(t, err)
	require.Equal(t, int64(3), res.MatchedKeys)
	require.Equal(t, operatorID, *store.audit.OperatorUserID)
	require.Equal(t, LocalResponseCacheClearTypeByAPIKey, store.audit.ClearType)
	require.Equal(t, LocalResponseCacheClearStatusPartialSuccess, store.audit.Status)
}

func TestOpenAIGatewayServiceListLocalResponseCacheClearAuditsNormalizesFilter(t *testing.T) {
	store := &localResponseCacheClearStoreStub{page: &LocalResponseCacheClearAuditPage{}}
	svc := &OpenAIGatewayService{cache: store}

	got, err := svc.ListLocalResponseCacheClearAudits(context.Background(), LocalResponseCacheClearAuditFilter{PageSize: 200, ClearType: LocalResponseCacheClearTypeByModel, Status: LocalResponseCacheClearStatusSuccess})

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, 1, store.filter.Page)
	require.Equal(t, 100, store.filter.PageSize)
	require.Equal(t, LocalResponseCacheClearTypeByModel, store.filter.ClearType)
	require.Equal(t, LocalResponseCacheClearStatusSuccess, store.filter.Status)
}

func TestOpenAIGatewayServiceListLocalResponseCacheClearAuditsRejectsInvalidStatus(t *testing.T) {
	svc := &OpenAIGatewayService{cache: &localResponseCacheClearStoreStub{}}

	_, err := svc.ListLocalResponseCacheClearAudits(context.Background(), LocalResponseCacheClearAuditFilter{Status: "done"})

	require.ErrorIs(t, err, ErrInvalidLocalResponseCacheAuditList)
}
