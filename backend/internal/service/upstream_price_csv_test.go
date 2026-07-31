package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseUpstreamPriceCSVParsesMultipleBillingModes(t *testing.T) {
	input := `model_pattern,billing_mode,currency,effective_at,input,output,per_request,is_wildcard
gpt-test,token,USD,2026-07-27T08:00:00Z,1.25,5,,false
image-test,per_request,CNY,2026-07-27T09:00:00Z,,,0.03,true
`
	prices, err := ParseUpstreamPriceCSV(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, prices, 2)
	require.Equal(t, "1.25", prices[0].PriceDetail["input"])
	require.Equal(t, "per_request", prices[1].BillingMode)
	require.True(t, prices[1].IsWildcard)
	require.False(t, prices[1].EffectiveAt.IsZero())
}

func TestParseUpstreamPriceCSVReturnsAllRowErrors(t *testing.T) {
	input := `model_pattern,billing_mode,currency,effective_at,input
,token,US,not-a-time,
`
	_, err := ParseUpstreamPriceCSV(strings.NewReader(input))
	var validationErr *UpstreamPriceCSVValidationError
	require.True(t, errors.As(err, &validationErr))
	require.Len(t, validationErr.Rows, 4)
	require.ElementsMatch(t, []string{"effective_at", "model_pattern", "currency", "price"}, []string{
		validationErr.Rows[0].Field, validationErr.Rows[1].Field, validationErr.Rows[2].Field, validationErr.Rows[3].Field,
	})
}

func TestParseUpstreamPriceCSVRejectsMissingRequiredHeader(t *testing.T) {
	_, err := ParseUpstreamPriceCSV(strings.NewReader("billing_mode,currency,effective_at,input\ntoken,USD,2026-07-27T08:00:00Z,1\n"))
	var validationErr *UpstreamPriceCSVValidationError
	require.True(t, errors.As(err, &validationErr))
	require.Equal(t, "model_pattern", validationErr.Rows[0].Field)
}
