package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type financePaymentFeeRepoStub struct{ input PaymentFeeImportInput }

func (s *financePaymentFeeRepoStub) ImportPaymentFees(_ context.Context, input PaymentFeeImportInput) (*PaymentFeeImportResult, error) {
	s.input = input
	return &PaymentFeeImportResult{ImportedCount: len(input.Rows)}, nil
}
func (s *financePaymentFeeRepoStub) ListPaymentFees(context.Context, FinanceReportFilter, FinancePaymentFeeListRequest) ([]FinancePaymentFeeItem, int64, error) {
	return nil, 0, nil
}

func TestFinancePaymentFeeCSVImportParsesExactDecimals(t *testing.T) {
	repo := &financePaymentFeeRepoStub{}
	csvData := "bill_event_id,order_no,gross_amount,fee_amount,net_amount,occurred_at\n" +
		"evt-1,order-1,10.25,0.25,10.00,2026-07-27T08:00:00Z\n"
	result, err := NewFinancePaymentFeeService(repo).ImportCSV(context.Background(), "stripe", "usd", strings.NewReader(csvData))
	require.NoError(t, err)
	require.Equal(t, 1, result.ImportedCount)
	require.Equal(t, "USD", repo.input.Currency)
	require.Equal(t, "0.25", repo.input.Rows[0].FeeAmount.String())
	require.Equal(t, "1", repo.input.Rows[0].FXRateToUSD.String())
}

func TestFinancePaymentFeeCSVRejectsWholeBatchOnInvalidRow(t *testing.T) {
	repo := &financePaymentFeeRepoStub{}
	csvData := "bill_event_id,order_no,gross_amount,fee_amount,net_amount,occurred_at\n" +
		"evt-1,order-1,10,1,9,2026-07-27T08:00:00Z\n" +
		"evt-2,order-2,10,2,9,bad-time\n"
	_, err := NewFinancePaymentFeeService(repo).ImportCSV(context.Background(), "stripe", "USD", strings.NewReader(csvData))
	var validationErr *FinanceCSVValidationError
	require.ErrorAs(t, err, &validationErr)
	require.NotEmpty(t, validationErr.Rows)
	require.Empty(t, repo.input.Rows)
	require.Contains(t, string(validationErr.CSV()), "net_amount")
	require.Contains(t, string(validationErr.CSV()), "occurred_at")
}

func TestFinancePaymentFeeCSVRequiresFXForNonUSD(t *testing.T) {
	csvData := "bill_event_id,order_no,gross_amount,fee_amount,net_amount,occurred_at\n" +
		"evt-1,order-1,10,1,9,2026-07-27T08:00:00Z\n"
	_, err := NewFinancePaymentFeeService(&financePaymentFeeRepoStub{}).ImportCSV(context.Background(), "wechat", "CNY", strings.NewReader(csvData))
	var validationErr *FinanceCSVValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Equal(t, "fx_rate_to_usd", validationErr.Rows[0].Field)
}
