package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type FinanceCSVError struct {
	Row     int
	Field   string
	Message string
}

type FinanceCSVValidationError struct{ Rows []FinanceCSVError }

func (e *FinanceCSVValidationError) Error() string { return "CSV validation failed" }

func (e *FinanceCSVValidationError) CSV() []byte {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	_ = writer.Write([]string{"row", "field", "error"})
	for _, item := range e.Rows {
		_ = writer.Write([]string{fmt.Sprintf("%d", item.Row), item.Field, item.Message})
	}
	writer.Flush()
	return buffer.Bytes()
}

type PaymentFeeImportRow struct {
	BillEventID string
	OrderNo     string
	GrossAmount decimal.Decimal
	FeeAmount   decimal.Decimal
	NetAmount   decimal.Decimal
	FXRateToUSD decimal.Decimal
	OccurredAt  time.Time
}

type PaymentFeeImportInput struct {
	Provider string
	Currency string
	Rows     []PaymentFeeImportRow
}

type PaymentFeeImportResult struct {
	ImportedCount  int `json:"imported_count"`
	DuplicateCount int `json:"duplicate_count"`
	UnmatchedCount int `json:"unmatched_count"`
}

type FinancePaymentFeeItem struct {
	ID             *int64    `json:"id"`
	PaymentOrderID *int64    `json:"payment_order_id"`
	OrderNo        string    `json:"order_no"`
	Provider       string    `json:"provider"`
	BillEventID    string    `json:"bill_event_id"`
	GrossAmount    *string   `json:"gross_amount"`
	FeeAmount      *string   `json:"fee_amount"`
	NetAmount      *string   `json:"net_amount"`
	Currency       string    `json:"currency"`
	FXRateToUSD    *string   `json:"fx_rate_to_usd"`
	FeeUSDAmount   *string   `json:"fee_usd_amount"`
	Status         string    `json:"status"`
	OccurredAt     time.Time `json:"occurred_at"`
}

type FinancePaymentFeeListRequest struct {
	OrderNo  string
	Provider string
	Status   string
	Page     int
	PageSize int
}

type FinancePaymentFeeRepository interface {
	ImportPaymentFees(ctx context.Context, input PaymentFeeImportInput) (*PaymentFeeImportResult, error)
	ListPaymentFees(ctx context.Context, filter FinanceReportFilter, request FinancePaymentFeeListRequest) ([]FinancePaymentFeeItem, int64, error)
}

type FinancePaymentFeeService struct{ repo FinancePaymentFeeRepository }

func NewFinancePaymentFeeService(repo FinancePaymentFeeRepository) *FinancePaymentFeeService {
	return &FinancePaymentFeeService{repo: repo}
}

func (s *FinancePaymentFeeService) ImportCSV(ctx context.Context, provider, currency string, reader io.Reader) (*PaymentFeeImportResult, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" || len(provider) > 50 {
		return nil, errors.New("provider is invalid")
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if !validFinanceCurrency(currency) {
		return nil, errors.New("currency is invalid")
	}
	rows, err := parsePaymentFeeCSV(reader, currency)
	if err != nil {
		return nil, err
	}
	return s.repo.ImportPaymentFees(ctx, PaymentFeeImportInput{Provider: provider, Currency: currency, Rows: rows})
}

func (s *FinancePaymentFeeService) List(ctx context.Context, filter FinanceReportFilter, request FinancePaymentFeeListRequest) ([]FinancePaymentFeeItem, int64, error) {
	request.OrderNo = strings.TrimSpace(request.OrderNo)
	request.Provider = strings.TrimSpace(request.Provider)
	request.Status = strings.TrimSpace(request.Status)
	if request.Status != "" && !financeAllowed(request.Status, "confirmed", "uncollected") {
		return nil, 0, errors.New("status is invalid")
	}
	normalizeFinancePage(&request.Page, &request.PageSize)
	return s.repo.ListPaymentFees(ctx, filter, request)
}

func parsePaymentFeeCSV(reader io.Reader, currency string) ([]PaymentFeeImportRow, error) {
	csvReader := csv.NewReader(io.LimitReader(reader, 20<<20))
	csvReader.ReuseRecord = false
	header, err := csvReader.Read()
	if err != nil {
		return nil, errors.New("CSV header is invalid")
	}
	columns := make(map[string]int, len(header))
	for index, name := range header {
		columns[strings.ToLower(strings.TrimSpace(name))] = index
	}
	required := []string{"bill_event_id", "order_no", "gross_amount", "fee_amount", "net_amount", "occurred_at"}
	validationErrors := make([]FinanceCSVError, 0)
	for _, name := range required {
		if _, exists := columns[name]; !exists {
			validationErrors = append(validationErrors, FinanceCSVError{Row: 1, Field: name, Message: "required column is missing"})
		}
	}
	if len(validationErrors) > 0 {
		return nil, &FinanceCSVValidationError{Rows: validationErrors}
	}
	rows := make([]PaymentFeeImportRow, 0)
	seen := make(map[string]struct{})
	for line := 2; ; line++ {
		record, readErr := csvReader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			validationErrors = append(validationErrors, FinanceCSVError{Row: line, Field: "row", Message: readErr.Error()})
			continue
		}
		value := func(name string) string {
			index, exists := columns[name]
			if !exists {
				return ""
			}
			if index >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[index])
		}
		row := PaymentFeeImportRow{BillEventID: value("bill_event_id"), OrderNo: value("order_no")}
		if row.BillEventID == "" || len(row.BillEventID) > 200 {
			validationErrors = append(validationErrors, FinanceCSVError{Row: line, Field: "bill_event_id", Message: "value is invalid"})
		}
		if _, exists := seen[row.BillEventID]; row.BillEventID != "" && exists {
			validationErrors = append(validationErrors, FinanceCSVError{Row: line, Field: "bill_event_id", Message: "duplicate value in file"})
		}
		seen[row.BillEventID] = struct{}{}
		parseMoney := func(field string) decimal.Decimal {
			parsed, parseErr := decimal.NewFromString(value(field))
			if parseErr != nil || parsed.IsNegative() {
				validationErrors = append(validationErrors, FinanceCSVError{Row: line, Field: field, Message: "must be a non-negative decimal"})
				return decimal.Zero
			}
			return parsed
		}
		row.GrossAmount = parseMoney("gross_amount")
		row.FeeAmount = parseMoney("fee_amount")
		row.NetAmount = parseMoney("net_amount")
		if row.GrossAmount.Sub(row.FeeAmount).Sub(row.NetAmount).Abs().GreaterThan(decimal.New(1, -8)) {
			validationErrors = append(validationErrors, FinanceCSVError{Row: line, Field: "net_amount", Message: "gross_amount - fee_amount must equal net_amount"})
		}
		row.FXRateToUSD = decimal.NewFromInt(1)
		if rawFX := value("fx_rate_to_usd"); rawFX != "" {
			row.FXRateToUSD, err = decimal.NewFromString(rawFX)
			if err != nil || !row.FXRateToUSD.IsPositive() {
				validationErrors = append(validationErrors, FinanceCSVError{Row: line, Field: "fx_rate_to_usd", Message: "must be a positive decimal"})
			}
		} else if currency != "USD" {
			validationErrors = append(validationErrors, FinanceCSVError{Row: line, Field: "fx_rate_to_usd", Message: "required for non-USD currency"})
		}
		row.OccurredAt, err = time.Parse(time.RFC3339, value("occurred_at"))
		if err != nil {
			validationErrors = append(validationErrors, FinanceCSVError{Row: line, Field: "occurred_at", Message: "must be RFC3339"})
		}
		rows = append(rows, row)
		if len(rows) > 100000 {
			return nil, errors.New("CSV has too many rows")
		}
	}
	if len(validationErrors) > 0 {
		return nil, &FinanceCSVValidationError{Rows: validationErrors}
	}
	if len(rows) == 0 {
		return nil, errors.New("CSV has no data rows")
	}
	return rows, nil
}
