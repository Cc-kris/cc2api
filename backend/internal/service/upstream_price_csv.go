package service

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const upstreamPriceCSVMaxRows = 10000

type UpstreamPriceCSVError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type UpstreamPriceCSVValidationError struct {
	Rows []UpstreamPriceCSVError
}

func (e *UpstreamPriceCSVValidationError) Error() string {
	return fmt.Sprintf("price CSV contains %d validation errors", len(e.Rows))
}

func ParseUpstreamPriceCSV(reader io.Reader) ([]UpstreamFinancePrice, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true
	csvReader.ReuseRecord = false
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("read price CSV header: %w", err)
	}
	indexes := make(map[string]int, len(header))
	for index, name := range header {
		indexes[strings.ToLower(strings.TrimSpace(name))] = index
	}
	required := []string{"model_pattern", "billing_mode", "currency", "effective_at"}
	for _, field := range required {
		if _, ok := indexes[field]; !ok {
			return nil, &UpstreamPriceCSVValidationError{Rows: []UpstreamPriceCSVError{{Row: 1, Field: field, Message: "required column is missing"}}}
		}
	}
	prices := make([]UpstreamFinancePrice, 0)
	validationErrors := make([]UpstreamPriceCSVError, 0)
	for rowNo := 2; ; rowNo++ {
		record, readErr := csvReader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			validationErrors = append(validationErrors, UpstreamPriceCSVError{Row: rowNo, Field: "row", Message: readErr.Error()})
			continue
		}
		if len(prices)+len(validationErrors) >= upstreamPriceCSVMaxRows {
			return nil, errors.New("price CSV exceeds 10000 rows")
		}
		value := func(name string) string {
			index, ok := indexes[name]
			if !ok || index >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[index])
		}
		price := UpstreamFinancePrice{
			ModelPattern: value("model_pattern"), BillingMode: value("billing_mode"),
			ServiceTier: value("service_tier"), Currency: strings.ToUpper(value("currency")),
			PriceDetail: map[string]any{}, Source: FinancePricingSourceManual,
		}
		if raw := strings.ToLower(value("is_wildcard")); raw != "" {
			parsed, parseErr := strconv.ParseBool(raw)
			if parseErr != nil {
				validationErrors = append(validationErrors, UpstreamPriceCSVError{Row: rowNo, Field: "is_wildcard", Message: "must be true or false"})
			} else {
				price.IsWildcard = parsed
			}
		}
		effectiveAt, parseErr := time.Parse(time.RFC3339, value("effective_at"))
		if parseErr != nil {
			validationErrors = append(validationErrors, UpstreamPriceCSVError{Row: rowNo, Field: "effective_at", Message: "must be RFC3339"})
		} else {
			price.EffectiveAt = effectiveAt.UTC()
		}
		if price.ModelPattern == "" {
			validationErrors = append(validationErrors, UpstreamPriceCSVError{Row: rowNo, Field: "model_pattern", Message: "is required"})
		}
		if len(price.Currency) != 3 {
			validationErrors = append(validationErrors, UpstreamPriceCSVError{Row: rowNo, Field: "currency", Message: "must be a 3-letter ISO code"})
		}
		for _, field := range []string{"input", "output", "cache_read", "cache_write_5m", "cache_write_1h", "per_request", "per_image", "per_second"} {
			if raw := value(field); raw != "" {
				price.PriceDetail[field] = raw
			}
		}
		if len(price.PriceDetail) == 0 {
			validationErrors = append(validationErrors, UpstreamPriceCSVError{Row: rowNo, Field: "price", Message: "at least one price field is required"})
		} else if _, parseErr = FinancePriceDetailFromMap(price.PriceDetail); parseErr != nil {
			validationErrors = append(validationErrors, UpstreamPriceCSVError{Row: rowNo, Field: "price", Message: parseErr.Error()})
		}
		prices = append(prices, price)
	}
	if len(validationErrors) > 0 {
		return nil, &UpstreamPriceCSVValidationError{Rows: validationErrors}
	}
	if len(prices) == 0 {
		return nil, errors.New("price CSV has no data rows")
	}
	return prices, nil
}
