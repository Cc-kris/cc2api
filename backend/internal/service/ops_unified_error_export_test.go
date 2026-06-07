package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExportUnifiedErrorsCSVRowsAndMasking(t *testing.T) {
	clientSub := OpsClientErrorSubcategoryParameter
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	repo := &opsRepoMock{
		ListUnifiedErrorsFn: func(ctx context.Context, filter *OpsUnifiedErrorListFilter) (*OpsUnifiedErrorList, error) {
			require.Equal(t, 1, filter.Page)
			require.Equal(t, 100, filter.PageSize)
			return &OpsUnifiedErrorList{Total: 1, Page: 1, PageSize: 100, Items: []*OpsUnifiedErrorItem{{
				ID:                     1,
				OccurredAt:             now,
				ErrorCategory:          OpsErrorCategoryClient,
				ErrorSubcategory:       OpsClientErrorSubcategoryParameter,
				ClientErrorSubcategory: &clientSub,
				ErrorResult:            OpsUnifiedErrorResultFinalFailed,
				Severity:               "P2",
				StatusCode:             400,
				User:                   &OpsUnifiedEntityRef{ID: 6, Email: "kris@example.com"},
				APIKey:                 &OpsUnifiedEntityRef{ID: 12, Name: "prod-key", Display: "prod-key #12"},
				Group:                  &OpsUnifiedEntityRef{ID: 3, Name: "VIP"},
				Platform:               "openai",
				Model:                  "gpt-5.5",
				UpstreamAccount:        &OpsUnifiedEntityRef{ID: 88, Name: "OpenAIAccount01"},
				Summary:                strings.Repeat("摘", 520),
				SameKindCount:          2,
				AIAnalysisStatus:       "completed",
			}}}, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	got, err := svc.ExportUnifiedErrors(context.Background(), &OpsUnifiedErrorListFilter{SortBy: "occurred_at"}, 100000)

	require.NoError(t, err)
	require.False(t, got.Truncated)
	require.Equal(t, 1, got.Total)
	require.Len(t, got.Rows, 2)
	require.Contains(t, got.Rows[0], "error_category")
	require.Contains(t, got.Rows[0], "error_subcategory")
	require.Contains(t, got.Rows[0], "client_error_subcategory")
	row := got.Rows[1]
	require.Equal(t, "client", row[2])
	require.Equal(t, OpsClientErrorSubcategoryParameter, row[4])
	require.Equal(t, "k***@example.com", row[9])
	require.Equal(t, "Op***01", row[16])
	require.Len(t, []rune(row[17]), 500)
}

func TestExportUnifiedErrorsMarksTruncatedBeforeBuildingRows(t *testing.T) {
	calls := 0
	repo := &opsRepoMock{
		ListUnifiedErrorsFn: func(ctx context.Context, filter *OpsUnifiedErrorListFilter) (*OpsUnifiedErrorList, error) {
			calls++
			require.Equal(t, 1, filter.Page)
			return &OpsUnifiedErrorList{Total: 2, Page: 1, PageSize: 100, Items: []*OpsUnifiedErrorItem{{ID: 1}, {ID: 2}}}, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	got, err := svc.ExportUnifiedErrors(context.Background(), &OpsUnifiedErrorListFilter{}, 1)

	require.NoError(t, err)
	require.True(t, got.Truncated)
	require.Equal(t, 2, got.Total)
	require.Len(t, got.Rows, 1) // header only; no oversized file rows are built
	require.Equal(t, 1, calls)
}

func TestOpsUnifiedErrorCSVRowEscapesFormulaInjection(t *testing.T) {
	clientSub := OpsClientErrorSubcategoryParameter
	item := &OpsUnifiedErrorItem{
		ID:                     1,
		OccurredAt:             time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
		ErrorCategory:          OpsErrorCategoryClient,
		ErrorSubcategory:       OpsClientErrorSubcategoryParameter,
		ClientErrorSubcategory: &clientSub,
		APIKey:                 &OpsUnifiedEntityRef{Display: "=cmd"},
		Group:                  &OpsUnifiedEntityRef{Name: "+group"},
		Platform:               "-platform",
		Model:                  "@model",
		UpstreamAccount:        &OpsUnifiedEntityRef{Name: "=Account01"},
		Summary:                "=summary",
	}

	row := opsUnifiedErrorCSVRow(item)

	require.Equal(t, "'=cmd", row[10])
	require.Equal(t, "'+group", row[12])
	require.Equal(t, "'-platform", row[13])
	require.Equal(t, "'@model", row[14])
	require.Equal(t, "'=A***01", row[16])
	require.Equal(t, "'=summary", row[17])
}
