package service

import (
	"errors"
	"fmt"
)

// FinanceValidationError identifies caller-controlled report and alert input
// errors so handlers can distinguish HTTP 400 responses from repository or
// infrastructure failures.
type FinanceValidationError struct {
	message string
}

func (e *FinanceValidationError) Error() string { return e.message }

func financeValidationError(message string) error {
	return &FinanceValidationError{message: message}
}

func financeValidationErrorf(format string, args ...any) error {
	return &FinanceValidationError{message: fmt.Sprintf(format, args...)}
}

func IsFinanceValidationError(err error) bool {
	var target *FinanceValidationError
	return errors.As(err, &target)
}
