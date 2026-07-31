package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
)

type financeJSONPathToken struct {
	field    string
	index    *int
	wildcard bool
}

func parseFinanceJSONPath(path string) ([]financeJSONPathToken, error) {
	path = strings.TrimSpace(path)
	if path == "$" {
		return nil, nil
	}
	if !strings.HasPrefix(path, "$") || strings.Contains(path, "..") || strings.ContainsAny(path, "?@()'\"") {
		return nil, fmt.Errorf("only restricted JSON paths are supported")
	}
	path = strings.TrimPrefix(path, "$")
	var tokens []financeJSONPathToken
	for len(path) > 0 {
		if path[0] == '.' {
			path = path[1:]
			end := 0
			for end < len(path) && (unicode.IsLetter(rune(path[end])) || unicode.IsDigit(rune(path[end])) || path[end] == '_' || path[end] == '-') {
				end++
			}
			if end == 0 {
				return nil, fmt.Errorf("invalid object field")
			}
			tokens = append(tokens, financeJSONPathToken{field: path[:end]})
			path = path[end:]
			continue
		}
		if path[0] == '[' {
			end := strings.IndexByte(path, ']')
			if end < 0 {
				return nil, fmt.Errorf("unclosed array selector")
			}
			raw := path[1:end]
			if raw == "*" {
				tokens = append(tokens, financeJSONPathToken{wildcard: true})
			} else {
				idx, err := strconv.Atoi(raw)
				if err != nil || idx < 0 || idx > 100000 {
					return nil, fmt.Errorf("array index must be a non-negative integer")
				}
				tokens = append(tokens, financeJSONPathToken{index: &idx})
			}
			path = path[end+1:]
			continue
		}
		return nil, fmt.Errorf("invalid JSON path token")
	}
	return tokens, nil
}

func FinanceJSONPath(root any, path string) ([]any, error) {
	tokens, err := parseFinanceJSONPath(path)
	if err != nil {
		return nil, err
	}
	values := []any{root}
	for _, token := range tokens {
		next := make([]any, 0)
		for _, value := range values {
			switch {
			case token.field != "":
				object, ok := value.(map[string]any)
				if ok {
					if child, exists := object[token.field]; exists {
						next = append(next, child)
					}
				}
			case token.index != nil:
				array, ok := value.([]any)
				if ok && *token.index < len(array) {
					next = append(next, array[*token.index])
				}
			case token.wildcard:
				array, ok := value.([]any)
				if ok {
					next = append(next, array...)
				}
			}
		}
		values = next
	}
	return values, nil
}

func FinanceDecimal(value any) (decimal.Decimal, error) {
	switch typed := value.(type) {
	case decimal.Decimal:
		return typed, nil
	case json.Number:
		return decimal.NewFromString(typed.String())
	case string:
		return decimal.NewFromString(strings.TrimSpace(typed))
	case float64:
		return decimal.NewFromString(strconv.FormatFloat(typed, 'g', -1, 64))
	case float32:
		return decimal.NewFromString(strconv.FormatFloat(float64(typed), 'g', -1, 32))
	case int:
		return decimal.NewFromInt(int64(typed)), nil
	case int64:
		return decimal.NewFromInt(typed), nil
	default:
		return decimal.Zero, fmt.Errorf("value is not a decimal")
	}
}

type financeExpressionParser struct {
	input     string
	pos       int
	variables map[string]decimal.Decimal
}

func EvaluateFinanceExpression(expression string, variables map[string]decimal.Decimal) (decimal.Decimal, error) {
	if len(expression) > 512 {
		return decimal.Zero, fmt.Errorf("expression is too long")
	}
	p := &financeExpressionParser{input: expression, variables: variables}
	value, err := p.parseExpression()
	if err != nil {
		return decimal.Zero, err
	}
	p.skipSpace()
	if p.pos != len(p.input) {
		return decimal.Zero, fmt.Errorf("unsupported expression token at position %d", p.pos)
	}
	return value, nil
}

func (p *financeExpressionParser) parseExpression() (decimal.Decimal, error) {
	left, err := p.parseTerm()
	if err != nil {
		return decimal.Zero, err
	}
	for {
		p.skipSpace()
		if !p.consume('+') && !p.consume('-') {
			return left, nil
		}
		op := p.input[p.pos-1]
		right, err := p.parseTerm()
		if err != nil {
			return decimal.Zero, err
		}
		if op == '+' {
			left = left.Add(right)
		} else {
			left = left.Sub(right)
		}
	}
}
func (p *financeExpressionParser) parseTerm() (decimal.Decimal, error) {
	left, err := p.parseFactor()
	if err != nil {
		return decimal.Zero, err
	}
	for {
		p.skipSpace()
		if !p.consume('*') && !p.consume('/') {
			return left, nil
		}
		op := p.input[p.pos-1]
		right, err := p.parseFactor()
		if err != nil {
			return decimal.Zero, err
		}
		if op == '*' {
			left = left.Mul(right)
		} else {
			if right.IsZero() {
				return decimal.Zero, fmt.Errorf("division by zero")
			}
			left = left.Div(right)
		}
	}
}
func (p *financeExpressionParser) parseFactor() (decimal.Decimal, error) {
	p.skipSpace()
	if p.consume('(') {
		value, err := p.parseExpression()
		if err != nil {
			return decimal.Zero, err
		}
		p.skipSpace()
		if !p.consume(')') {
			return decimal.Zero, fmt.Errorf("missing closing parenthesis")
		}
		return value, nil
	}
	if p.consume('-') {
		value, err := p.parseFactor()
		return value.Neg(), err
	}
	start := p.pos
	for p.pos < len(p.input) && (unicode.IsLetter(rune(p.input[p.pos])) || unicode.IsDigit(rune(p.input[p.pos])) || p.input[p.pos] == '_' || p.input[p.pos] == '.') {
		p.pos++
	}
	if start == p.pos {
		return decimal.Zero, fmt.Errorf("expected number or variable at position %d", p.pos)
	}
	token := p.input[start:p.pos]
	if value, ok := p.variables[token]; ok {
		return value, nil
	}
	value, err := decimal.NewFromString(token)
	if err != nil {
		return decimal.Zero, fmt.Errorf("unknown variable %q", token)
	}
	return value, nil
}
func (p *financeExpressionParser) skipSpace() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}
func (p *financeExpressionParser) consume(char byte) bool {
	if p.pos < len(p.input) && p.input[p.pos] == char {
		p.pos++
		return true
	}
	return false
}
