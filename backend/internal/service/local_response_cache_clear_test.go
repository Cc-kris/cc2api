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
