//go:build unit

package repository

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestFinanceReconciliationAlertThresholdUsesAmountOrRate(t *testing.T) {
	tests := []struct {
		name       string
		difference string
		rate       *decimal.Decimal
		want       bool
	}{
		{name: "below both thresholds", difference: "0.50", rate: reconciliationDecimal("0.005"), want: false},
		{name: "exact amount threshold", difference: "1.00", rate: reconciliationDecimal("0.005"), want: false},
		{name: "above amount threshold", difference: "1.00000001", rate: reconciliationDecimal("0.005"), want: true},
		{name: "negative difference uses absolute amount", difference: "-1.01", rate: nil, want: true},
		{name: "exact rate threshold", difference: "0.50", rate: reconciliationDecimal("0.01"), want: false},
		{name: "above rate threshold", difference: "0.50", rate: reconciliationDecimal("0.01000001"), want: true},
		{name: "missing rate and low amount", difference: "0.50", rate: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, financeReconciliationNeedsAlert(decimal.RequireFromString(tt.difference), tt.rate))
		})
	}
}

func reconciliationDecimal(value string) *decimal.Decimal {
	result := decimal.RequireFromString(value)
	return &result
}
