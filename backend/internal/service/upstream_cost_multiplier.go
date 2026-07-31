package service

import (
	"errors"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

var maxUpstreamCostMultiplier = decimal.RequireFromString("9999.9999")

var ErrAccountUpstreamMultiplierConflict = infraerrors.Conflict(
	"ACCOUNT_UPSTREAM_MULTIPLIER_CONFLICT",
	"upstream cost multiplier was changed by another request",
)

func ValidateUpstreamCostMultiplier(value decimal.Decimal) error {
	if value.IsNegative() || value.GreaterThan(maxUpstreamCostMultiplier) {
		return errors.New("upstream_cost_multiplier must be between 0 and 9999.9999")
	}
	if value.Exponent() < -4 {
		return errors.New("upstream_cost_multiplier supports at most 4 decimal places")
	}
	return nil
}

// CloneDecimalSnapshot returns an independent Decimal pointer so asynchronous
// usage persistence cannot observe later mutations of the source object.
func CloneDecimalSnapshot(value *decimal.Decimal) *decimal.Decimal {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

// ResolveUpstreamCostMultiplierSnapshot accepts only the immutable request-level
// value copied when the account was selected. Falling back to the account's current
// value would rewrite historical procurement evidence after an account edit.
func ResolveUpstreamCostMultiplierSnapshot(requestValue *decimal.Decimal, _ *Account) *decimal.Decimal {
	if requestValue != nil {
		return CloneDecimalSnapshot(requestValue)
	}
	return nil
}
