package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetUnifiedErrorDetailBuildsReadableSections(t *testing.T) {
	createdAt := time.Date(2026, 6, 8, 11, 0, 0, 0, time.UTC)
	userID := int64(6)
	apiKeyID := int64(12)
	accountID := int64(88)
	groupID := int64(3)
	clientSub := OpsClientErrorSubcategoryParameter
	repo := &opsRepoMock{
		GetErrorLogByIDFn: func(ctx context.Context, id int64) (*OpsErrorLogDetail, error) {
			require.Equal(t, int64(9), id)
			return &OpsErrorLogDetail{
				OpsErrorLog: OpsErrorLog{
					ID:                       9,
					CreatedAt:                createdAt,
					Phase:                    "request",
					Type:                     "invalid_request_error",
					Owner:                    "client",
					Source:                   "client_request",
					ErrorCategory:            OpsErrorCategoryClient,
					ErrorSubcategory:         OpsClientErrorSubcategoryParameter,
					ClientErrorSubcategory:   &clientSub,
					ClassificationConfidence: OpsClassificationConfidenceHigh,
					ClassificationReason:     "请求参数缺少必填字段 model",
					Severity:                 "P2",
					StatusCode:               400,
					ClientStatusCode:         400,
					Platform:                 "openai",
					Model:                    "gpt-5.5",
					RequestID:                "req-9",
					ClientRequestID:          "client-9",
					Message:                  "validation error",
					UserID:                   &userID,
					UserEmail:                "user@example.com",
					APIKeyID:                 &apiKeyID,
					AccountID:                &accountID,
					AccountName:              "上游账号A",
					GroupID:                  &groupID,
					GroupName:                "VIP",
					RequestPath:              "/v1/responses",
					InboundEndpoint:          "/v1/responses",
					RequestedModel:           "gpt-5.5",
				},
				ErrorBody:      strings.Repeat("敏", 520),
				UpstreamErrors: strings.Repeat("上", 520),
			}, nil
		},
		ListUnifiedErrorsFn: func(ctx context.Context, filter *OpsUnifiedErrorListFilter) (*OpsUnifiedErrorList, error) {
			require.Equal(t, []string{OpsUnifiedErrorResultFinalFailed}, filter.ErrorResults)
			require.Equal(t, []string{OpsErrorCategoryClient}, filter.ErrorCategories)
			require.Equal(t, []string{OpsClientErrorSubcategoryParameter}, filter.ErrorSubcategories)
			require.Equal(t, []string{clientSub}, filter.ClientErrorSubcategories)
			return &OpsUnifiedErrorList{Total: 2, Page: 1, PageSize: 20, Items: []*OpsUnifiedErrorItem{
				{ID: 9, ErrorCategory: OpsErrorCategoryClient, ErrorSubcategory: OpsClientErrorSubcategoryParameter, ClientErrorSubcategory: &clientSub, ErrorResult: OpsUnifiedErrorResultFinalFailed, User: &OpsUnifiedEntityRef{ID: userID}, APIKey: &OpsUnifiedEntityRef{ID: apiKeyID}, Group: &OpsUnifiedEntityRef{ID: groupID}, Model: "gpt-5.5", AIAnalysisStatus: "completed"},
				{ID: 10, ErrorCategory: OpsErrorCategoryClient, ErrorSubcategory: OpsClientErrorSubcategoryParameter, ClientErrorSubcategory: &clientSub, ErrorResult: OpsUnifiedErrorResultFinalFailed, User: &OpsUnifiedEntityRef{ID: 7}, APIKey: &OpsUnifiedEntityRef{ID: 13}, Group: &OpsUnifiedEntityRef{ID: groupID}, Model: "gpt-5.5", AIAnalysisStatus: OpsUnifiedAIAnalysisNotAnalyzed},
			}}, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	got, err := svc.GetUnifiedErrorDetail(context.Background(), 9)

	require.NoError(t, err)
	require.Equal(t, OpsUnifiedErrorResultFinalFailed, got.Conclusion.ErrorResult)
	require.True(t, got.Conclusion.FinalFailed)
	require.False(t, got.Recovery.Recovered)
	require.Equal(t, OpsErrorCategoryClient, got.Classification.ErrorCategory)
	require.Equal(t, OpsClientErrorSubcategoryParameter, *got.Classification.ClientErrorSubcategory)
	require.Equal(t, int64(6), got.RequestChain.User.ID)
	require.Equal(t, int64(88), got.RequestChain.UpstreamAccount.ID)
	require.Equal(t, 2, got.ImpactScope.SameKindCount)
	require.Equal(t, 2, got.ImpactScope.AffectedUsers)
	require.Equal(t, "completed", got.AIAnalysis.Status)
	require.NotEmpty(t, got.RawRecord.ErrorBodyPreview)
	require.Len(t, []rune(got.RawRecord.ErrorBodyPreview), 500)
	require.Len(t, []rune(got.RawRecord.ErrorLog.ErrorBody), 500)
	require.Len(t, []rune(got.RawRecord.ErrorLog.UpstreamErrors), 500)
	require.Len(t, []rune(got.RawRecord.UpstreamErrors), 500)
	require.Len(t, got.SameKindErrors, 2)
}

func TestGetUnifiedErrorDetailMarksRecoveredFromClientStatus(t *testing.T) {
	accountID := int64(88)
	repo := &opsRepoMock{
		GetErrorLogByIDFn: func(ctx context.Context, id int64) (*OpsErrorLogDetail, error) {
			return &OpsErrorLogDetail{OpsErrorLog: OpsErrorLog{ID: id, CreatedAt: time.Now(), ErrorCategory: OpsErrorCategoryRateLimit, ErrorSubcategory: "upstream_rate_limit", StatusCode: 429, ClientStatusCode: 200, AccountID: &accountID, AccountName: "Op01", Platform: "openai", Model: "gpt-5.5"}, UpstreamErrors: `[{"status":429}]`}, nil
		},
		ListUnifiedErrorsFn: func(ctx context.Context, filter *OpsUnifiedErrorListFilter) (*OpsUnifiedErrorList, error) {
			require.Equal(t, []string{OpsUnifiedErrorResultRecovered}, filter.ErrorResults)
			return &OpsUnifiedErrorList{Total: 1, Items: []*OpsUnifiedErrorItem{{ID: 11, ErrorResult: OpsUnifiedErrorResultRecovered}}}, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	got, err := svc.GetUnifiedErrorDetail(context.Background(), 11)

	require.NoError(t, err)
	require.Equal(t, OpsUnifiedErrorResultRecovered, got.Conclusion.ErrorResult)
	require.False(t, got.Conclusion.FinalFailed)
	require.True(t, got.Recovery.Recovered)
	require.Equal(t, "account_switch", got.Recovery.RecoveryMethod)
	require.Equal(t, 200, got.Classification.ClientStatusCode)
	require.Equal(t, 429, got.Classification.StatusCode)
}
