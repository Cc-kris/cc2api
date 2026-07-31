//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type financeReconciliationRepoStub struct {
	input       FinanceReconciliationImportInput
	importCalls int
	status      string
	note        string
}

func (s *financeReconciliationRepoStub) ImportFinanceReconciliation(_ context.Context, input FinanceReconciliationImportInput, _ time.Time) (*FinanceReconciliationImportResult, error) {
	s.input = input
	s.importCalls++
	return &FinanceReconciliationImportResult{Reconciliation: FinanceReconciliation{ID: 1}}, nil
}

func (s *financeReconciliationRepoStub) ListFinanceReconciliations(context.Context, FinanceReconciliationListRequest) ([]FinanceReconciliation, int64, error) {
	return nil, 0, nil
}

func (s *financeReconciliationRepoStub) UpdateFinanceReconciliationStatus(_ context.Context, id int64, status, note string, _ int64, _ time.Time) (*FinanceReconciliation, error) {
	s.status = status
	s.note = note
	return &FinanceReconciliation{ID: id, Status: status}, nil
}

func TestFinanceReconciliationImportCSVUsesExactDecimalSumAndChecksum(t *testing.T) {
	repo := &financeReconciliationRepoStub{}
	service := NewFinanceReconciliationService(repo)
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	content := []byte("line_id,upstream_bill_amount\na,10.1234567891\nb,0.8765432109\n")

	result, err := service.ImportCSV(context.Background(), 12, start, end, "usd", "bill-1", "../bill.csv", content, 8)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Reconciliation.ID)
	require.Equal(t, "11", repo.input.UpstreamBillAmount.String())
	require.Equal(t, "USD", repo.input.Currency)
	require.Equal(t, "bill.csv", repo.input.SourceFileName)
	require.Len(t, repo.input.SourceFileChecksum, 64)
	require.Equal(t, int64(8), repo.input.OperatorID)
}

func TestFinanceReconciliationImportCSVRejectsWholeBatch(t *testing.T) {
	repo := &financeReconciliationRepoStub{}
	service := NewFinanceReconciliationService(repo)
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	content := []byte("amount\n10\nbad\n-2\n")

	_, err := service.ImportCSV(context.Background(), 12, start, start.AddDate(0, 1, 0), "USD", "", "bill.csv", content, 0)
	var validationErr *FinanceCSVValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Len(t, validationErr.Rows, 2)
	require.Zero(t, repo.importCalls)
	require.Contains(t, string(validationErr.CSV()), "upstream_bill_amount")
}

func TestFinanceReconciliationUpdateStatusOnlyAllowsManualStates(t *testing.T) {
	repo := &financeReconciliationRepoStub{}
	service := NewFinanceReconciliationService(repo)

	_, err := service.UpdateStatus(context.Background(), 1, FinanceReconciliationStatusUpdate{Status: FinanceReconciliationDifference, Note: "not allowed"}, 8)
	require.Error(t, err)
	require.True(t, IsFinanceValidationError(err))

	item, err := service.UpdateStatus(context.Background(), 1, FinanceReconciliationStatusUpdate{Status: FinanceReconciliationConfirmed, Note: " reviewed "}, 8)
	require.NoError(t, err)
	require.Equal(t, FinanceReconciliationConfirmed, item.Status)
	require.Equal(t, "reviewed", repo.note)
}

func TestFinanceReconciliationCSVRequiresAmountHeader(t *testing.T) {
	_, err := parseFinanceReconciliationCSV(strings.NewReader("cost\n10\n"))
	var validationErr *FinanceCSVValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Equal(t, "upstream_bill_amount", validationErr.Rows[0].Field)
}

func TestFinanceReconciliationRejectsNonUSDBillUntilFXIsProvided(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	_, err := NewFinanceReconciliationService(&financeReconciliationRepoStub{}).ImportCSV(
		context.Background(), 12, start, start.AddDate(0, 1, 0), "CNY", "", "bill.csv", []byte("amount\n10\n"), 0,
	)
	require.Error(t, err)
	require.True(t, IsFinanceValidationError(err))
	require.Contains(t, err.Error(), "USD")
}
