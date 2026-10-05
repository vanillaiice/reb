// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Formula evaluates a table formula column, formula[expression|precision], over a row (specification
// section 4.3):
//
//	expression := term (("+" | "-") term)*
//	term       := factor (("*" | "/") factor)*
//	factor     := ("+" | "-") factor | number | name | "(" expression ")"
//
// A name is a column of the row, read like JavaScript's parseFloat (0 when not a number); any other
// name is 0. Dividing by zero gives 0, and so does an expression that does not parse. The result is
// formatted like JavaScript's Number#toFixed. Rebar's web form and mobile app evaluate formulas the
// same way: every evaluator runs testdata/formula_cases.json.
func Formula(expression string, row map[string]any, precision int) string {
	result := 0.0
	if tokens, ok := formulaTokens(expression); ok {
		f := &formulaParser{tokens: tokens, row: row}
		if value, ok := f.expression(0); ok && f.pos == len(tokens) {
			result = value
		}
	}
	if math.IsNaN(result) || math.IsInf(result, 0) {
		result = 0
	}
	return toFixed(result, precision)
}

// maxFormulaDepth bounds nested parentheses and signs, so a hostile formula cannot exhaust the stack.
const maxFormulaDepth = 64

var numberPrefix = regexp.MustCompile(`^\s*([+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?)`)

// parseNumber is JavaScript's parseFloat falling back to 0, as the mobile code's `|| 0`.
func parseNumber(value any) float64 {
	switch v := value.(type) {
	case nil, bool:
		return 0
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		if match := numberPrefix.FindStringSubmatch(v); match != nil {
			f, _ := strconv.ParseFloat(match[1], 64)
			return f
		}
	}
	return 0
}

type formulaToken struct {
	op     byte // + - * / ( ), or 0 for a number or a name
	name   string
	number float64
}

// formulaTokens splits an expression into operators, numbers (digits and dots, read like parseFloat:
// "5." and ".5" are numbers, "." is not) and names; anything else makes the formula invalid.
func formulaTokens(text string) ([]formulaToken, bool) {
	var tokens []formulaToken
	for i := 0; i < len(text); {
		ch := text[i]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			i++
		case strings.IndexByte("+-*/()", ch) >= 0:
			tokens = append(tokens, formulaToken{op: ch})
			i++
		case ch == '.' || (ch >= '0' && ch <= '9'):
			j := i
			for j < len(text) && (text[j] == '.' || (text[j] >= '0' && text[j] <= '9')) {
				j++
			}
			if !numberPrefix.MatchString(text[i:j]) {
				return nil, false
			}
			tokens = append(tokens, formulaToken{number: parseNumber(text[i:j])})
			i = j
		case ch == '_' || (ch|0x20 >= 'a' && ch|0x20 <= 'z'):
			j := i
			for j < len(text) && (text[j] == '_' || (text[j]|0x20 >= 'a' && text[j]|0x20 <= 'z') || (text[j] >= '0' && text[j] <= '9')) {
				j++
			}
			tokens = append(tokens, formulaToken{name: text[i:j]})
			i = j
		default:
			return nil, false
		}
	}
	return tokens, true
}

type formulaParser struct {
	tokens []formulaToken
	pos    int
	row    map[string]any
}

func (f *formulaParser) next(ops string) (byte, bool) {
	if f.pos < len(f.tokens) && f.tokens[f.pos].op != 0 && strings.IndexByte(ops, f.tokens[f.pos].op) >= 0 {
		f.pos++
		return f.tokens[f.pos-1].op, true
	}
	return 0, false
}

func (f *formulaParser) expression(depth int) (float64, bool) {
	left, ok := f.term(depth)
	for ok {
		op, more := f.next("+-")
		if !more {
			break
		}
		var right float64
		if right, ok = f.term(depth); op == '+' {
			left += right
		} else {
			left -= right
		}
	}
	return left, ok
}

func (f *formulaParser) term(depth int) (float64, bool) {
	left, ok := f.factor(depth)
	for ok {
		op, more := f.next("*/")
		if !more {
			break
		}
		var right float64
		right, ok = f.factor(depth)
		switch {
		case op == '*':
			left *= right
		case right == 0:
			left = 0
		default:
			left /= right
		}
	}
	return left, ok
}

func (f *formulaParser) factor(depth int) (float64, bool) {
	if depth > maxFormulaDepth || f.pos >= len(f.tokens) {
		return 0, false
	}
	if op, ok := f.next("+-"); ok {
		value, ok := f.factor(depth + 1)
		if op == '-' {
			value = -value
		}
		return value, ok
	}
	if _, ok := f.next("("); ok {
		value, ok := f.expression(depth + 1)
		if _, closed := f.next(")"); !ok || !closed {
			return 0, false
		}
		return value, true
	}
	t := f.tokens[f.pos]
	if t.op != 0 {
		return 0, false
	}
	f.pos++
	if t.name != "" {
		return parseNumber(f.row[t.name]), true
	}
	return t.number, true
}

// toFixed is Number#toFixed: the exact binary value rounded half away from zero, with "-" for any
// negative input (so -0.001 gives "-0.00", as in JavaScript).
func toFixed(value float64, precision int) string {
	exact := new(big.Rat).SetFloat64(math.Abs(value))
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(precision)), nil))
	scaled := new(big.Rat).Mul(exact, scale)
	// Round half up: floor(scaled + 1/2).
	scaled.Add(scaled, big.NewRat(1, 2))
	rounded := new(big.Int).Quo(scaled.Num(), scaled.Denom())

	digits := rounded.String()
	if precision > 0 {
		if len(digits) <= precision {
			digits = strings.Repeat("0", precision-len(digits)+1) + digits
		}
		digits = digits[:len(digits)-precision] + "." + digits[len(digits)-precision:]
	}
	if value < 0 {
		return "-" + digits
	}
	return digits
}
