package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

const (
	FinanceReconciliationMatched    = "matched"
	FinanceReconciliationDifference = "difference"
	FinanceReconciliationConfirmed  = "confirmed"
	FinanceReconciliationIgnored    = "ignored"
	FinanceReconciliationPending    = "pending"
	financeReconciliationMaxCSVSize = 20 << 20
)

var ErrFinanceReconciliationNotFound = errors.New("finance reconciliation not found")

type FinanceReconciliation struct {
	ID                 int64      `json:"id"`
	WalletID           int64      `json:"wallet_id"`
	WalletName         string     `json:"wallet_name"`
	UpstreamID         int64      `json:"upstream_id"`
	UpstreamName       string     `json:"upstream_name"`
	PeriodStart        time.Time  `json:"period_start"`
	PeriodEnd          time.Time  `json:"period_end"`
	UpstreamBillAmount string     `json:"upstream_bill_amount"`
	SystemCostAmount   string     `json:"system_cost_amount"`
	DifferenceAmount   string     `json:"difference_amount"`
	DifferenceRate     *string    `json:"difference_rate"`
	Currency           string     `json:"currency"`
	SourceReference    string     `json:"source_reference,omitempty"`
	SourceFileName     string     `json:"source_file_name,omitempty"`
	SourceFileChecksum string     `json:"source_file_checksum"`
	Status             string     `json:"status"`
	HandledBy          *int64     `json:"handled_by"`
	HandledNote        string     `json:"handled_note,omitempty"`
	HandledAt          *time.Time `json:"handled_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type FinanceReconciliationListRequest struct {
	StartAt    *time.Time
	EndBefore  *time.Time
	UpstreamID *int64
	WalletID   *int64
	Status     string
	Page       int
	PageSize   int
}

type FinanceReconciliationImportInput struct {
	WalletID           int64
	PeriodStart        time.Time
	PeriodEnd          time.Time
	UpstreamBillAmount decimal.Decimal
	Currency           string
	SourceReference    string
	SourceFileName     string
	SourceFileChecksum string
	OperatorID         int64
}

type FinanceReconciliationImportResult struct {
	Reconciliation FinanceReconciliation `json:"reconciliation"`
	JobID          int64                 `json:"job_id"`
	JobStatus      string                `json:"job_status"`
	Duplicate      bool                  `json:"duplicate"`
}

type FinanceReconciliationStatusUpdate struct {
	Status string `json:"status" binding:"required"`
	Note   string `json:"note" binding:"required"`
}

type FinanceReconciliationRepository interface {
	ImportFinanceReconciliation(ctx context.Context, input FinanceReconciliationImportInput, now time.Time) (*FinanceReconciliationImportResult, error)
	ListFinanceReconciliations(ctx context.Context, request FinanceReconciliationListRequest) ([]FinanceReconciliation, int64, error)
	UpdateFinanceReconciliationStatus(ctx context.Context, id int64, status, note string, actorID int64, now time.Time) (*FinanceReconciliation, error)
}

type FinanceReconciliationService struct {
	repo FinanceReconciliationRepository
	now  func() time.Time
}

func NewFinanceReconciliationService(repo FinanceReconciliationRepository) *FinanceReconciliationService {
	return &FinanceReconciliationService{repo: repo, now: time.Now}
}

func (s *FinanceReconciliationService) ImportCSV(
	ctx context.Context,
	walletID int64,
	periodStart, periodEnd time.Time,
	currency, sourceReference, sourceFileName string,
	content []byte,
	operatorID int64,
) (*FinanceReconciliationImportResult, error) {
	if walletID <= 0 {
		return nil, financeValidationError("wallet_id is invalid")
	}
	if periodStart.IsZero() || periodEnd.IsZero() || !periodEnd.After(periodStart) {
		return nil, financeValidationError("billing period is invalid")
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if !validFinanceCurrency(currency) {
		return nil, financeValidationError("currency is invalid")
	}
	if currency != "USD" {
		return nil, financeValidationError("reconciliation currency must be USD")
	}
	sourceReference = strings.TrimSpace(sourceReference)
	if utf8.RuneCountInString(sourceReference) > 200 {
		return nil, financeValidationError("source_reference exceeds 200 characters")
	}
	sourceFileName = filepath.Base(strings.TrimSpace(sourceFileName))
	if sourceFileName == "." || sourceFileName == string(filepath.Separator) || utf8.RuneCountInString(sourceFileName) > 255 {
		return nil, financeValidationError("source file name is invalid")
	}
	if !strings.EqualFold(filepath.Ext(sourceFileName), ".csv") {
		return nil, financeValidationError("source file must be CSV")
	}
	if len(content) == 0 {
		return nil, financeValidationError("CSV file is empty")
	}
	if len(content) > financeReconciliationMaxCSVSize {
		return nil, financeValidationError("CSV file exceeds 20 MB")
	}
	amount, err := parseFinanceReconciliationCSV(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))
	return s.repo.ImportFinanceReconciliation(ctx, FinanceReconciliationImportInput{
		WalletID:           walletID,
		PeriodStart:        periodStart.UTC(),
		PeriodEnd:          periodEnd.UTC(),
		UpstreamBillAmount: amount,
		Currency:           currency,
		SourceReference:    sourceReference,
		SourceFileName:     sourceFileName,
		SourceFileChecksum: checksum,
		OperatorID:         operatorID,
	}, s.now().UTC())
}

func (s *FinanceReconciliationService) List(ctx context.Context, request FinanceReconciliationListRequest) ([]FinanceReconciliation, int64, error) {
	if request.Page <= 0 {
		request.Page = 1
	}
	if request.PageSize <= 0 || request.PageSize > 100 {
		request.PageSize = 20
	}
	request.Status = strings.TrimSpace(request.Status)
	if request.Status != "" && !financeAllowed(request.Status,
		FinanceReconciliationPending, FinanceReconciliationMatched, FinanceReconciliationDifference,
		FinanceReconciliationConfirmed, FinanceReconciliationIgnored) {
		return nil, 0, financeValidationError("status is invalid")
	}
	return s.repo.ListFinanceReconciliations(ctx, request)
}

func (s *FinanceReconciliationService) UpdateStatus(ctx context.Context, id int64, request FinanceReconciliationStatusUpdate, actorID int64) (*FinanceReconciliation, error) {
	if id <= 0 {
		return nil, financeValidationError("reconciliation id is invalid")
	}
	request.Status = strings.TrimSpace(request.Status)
	if !financeAllowed(request.Status, FinanceReconciliationConfirmed, FinanceReconciliationIgnored, FinanceReconciliationPending) {
		return nil, financeValidationError("status must be confirmed, ignored or pending")
	}
	request.Note = strings.TrimSpace(request.Note)
	if request.Note == "" || utf8.RuneCountInString(request.Note) > 2000 {
		return nil, financeValidationError("note is required and must not exceed 2000 characters")
	}
	return s.repo.UpdateFinanceReconciliationStatus(ctx, id, request.Status, request.Note, actorID, s.now().UTC())
}

func parseFinanceReconciliationCSV(reader io.Reader) (decimal.Decimal, error) {
	parser := csv.NewReader(reader)
	parser.FieldsPerRecord = -1
	parser.ReuseRecord = true
	header, err := parser.Read()
	if err != nil {
		return decimal.Zero, &FinanceCSVValidationError{Rows: []FinanceCSVError{{Row: 1, Field: "header", Message: "CSV header is required"}}}
	}
	amountIndex := -1
	for index, field := range header {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "amount", "bill_amount", "upstream_bill_amount":
			if amountIndex >= 0 {
				return decimal.Zero, &FinanceCSVValidationError{Rows: []FinanceCSVError{{Row: 1, Field: "upstream_bill_amount", Message: "amount column must be unique"}}}
			}
			amountIndex = index
		}
	}
	if amountIndex < 0 {
		return decimal.Zero, &FinanceCSVValidationError{Rows: []FinanceCSVError{{Row: 1, Field: "upstream_bill_amount", Message: "amount column is required"}}}
	}

	total := decimal.Zero
	validRows := 0
	errorsFound := make([]FinanceCSVError, 0)
	for rowNumber := 2; ; rowNumber++ {
		record, readErr := parser.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			errorsFound = append(errorsFound, FinanceCSVError{Row: rowNumber, Field: "row", Message: readErr.Error()})
			continue
		}
		if len(record) == 0 || (len(record) == 1 && strings.TrimSpace(record[0]) == "") {
			continue
		}
		if amountIndex >= len(record) {
			errorsFound = append(errorsFound, FinanceCSVError{Row: rowNumber, Field: "upstream_bill_amount", Message: "amount is required"})
			continue
		}
		amount, parseErr := decimal.NewFromString(strings.TrimSpace(record[amountIndex]))
		if parseErr != nil || amount.IsNegative() {
			errorsFound = append(errorsFound, FinanceCSVError{Row: rowNumber, Field: "upstream_bill_amount", Message: "amount must be a non-negative decimal"})
			continue
		}
		total = total.Add(amount)
		validRows++
	}
	if validRows == 0 && len(errorsFound) == 0 {
		errorsFound = append(errorsFound, FinanceCSVError{Row: 2, Field: "upstream_bill_amount", Message: "at least one data row is required"})
	}
	if len(errorsFound) > 0 {
		return decimal.Zero, &FinanceCSVValidationError{Rows: errorsFound}
	}
	return total, nil
}
