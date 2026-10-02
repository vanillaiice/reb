// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Formula evaluates a table formula column, formula[expression|precision], over a row, exactly as
// the Rebar mobile app does (and the web form's evaluator, which runs the same golden cases,
// testdata/formula_cases.json): column names are replaced by the row's numbers (parsed like
// JavaScript's parseFloat, 0 when not a number), other names by 0, then + - * / and parentheses are
// evaluated with the usual precedence; dividing by zero gives 0. The result is formatted like
// JavaScript's Number#toFixed. A precision of 0 is honoured (the mobile app turns it into 2).
func Formula(expression string, row map[string]any, precision int) string {
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	// Longest names first, so "qty" is not replaced inside "qty_total".
	sort.SliceStable(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })

	text := expression
	for _, key := range keys {
		if key == "" {
			continue
		}
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(key) + `\b`)
		text = pattern.ReplaceAllLiteralString(text, jsNumberString(parseNumber(row[key])))
	}
	text = otherNames.ReplaceAllString(text, "0")

	result := evaluateRPN(toRPN(tokenize(text)))
	if math.IsNaN(result) || math.IsInf(result, 0) {
		result = 0
	}
	return toFixed(result, precision)
}

var (
	otherNames   = regexp.MustCompile(`[a-zA-Z_]\w*`)
	numberPrefix = regexp.MustCompile(`^\s*([+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?)`)
	precedence   = map[string]int{"+": 1, "-": 1, "*": 2, "/": 2}
)

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

func tokenize(text string) []string {
	var tokens []string
	for i := 0; i < len(text); {
		ch := text[i]
		switch {
		case strings.IndexByte("+-*/()", ch) >= 0:
			tokens = append(tokens, string(ch))
			i++
		case ch == '.' || (ch >= '0' && ch <= '9'):
			j := i
			for j < len(text) && (text[j] == '.' || (text[j] >= '0' && text[j] <= '9')) {
				j++
			}
			tokens = append(tokens, text[i:j])
			i = j
		default:
			i++ // spaces and anything else are skipped, as in the mobile tokenizer
		}
	}
	return tokens
}

// numericToken is !isNaN(parseFloat(token)): "5." and ".5" are numbers, "." and "..5" are not and
// are dropped.
func numericToken(token string) bool { return numberPrefix.MatchString(token) }

func toRPN(tokens []string) []string {
	var output, operators []string
	for _, token := range tokens {
		switch {
		case numericToken(token):
			output = append(output, token)
		case precedence[token] > 0:
			for len(operators) > 0 && precedence[operators[len(operators)-1]] >= precedence[token] {
				output = append(output, operators[len(operators)-1])
				operators = operators[:len(operators)-1]
			}
			operators = append(operators, token)
		case token == "(":
			operators = append(operators, token)
		case token == ")":
			for len(operators) > 0 && operators[len(operators)-1] != "(" {
				output = append(output, operators[len(operators)-1])
				operators = operators[:len(operators)-1]
			}
			if len(operators) > 0 {
				operators = operators[:len(operators)-1]
			}
		}
	}
	for i := len(operators) - 1; i >= 0; i-- {
		output = append(output, operators[i])
	}
	return output
}

func evaluateRPN(rpn []string) float64 {
	var stack []float64
	pop := func() float64 {
		if len(stack) == 0 {
			return 0
		}
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return v
	}
	for _, token := range rpn {
		if numericToken(token) {
			stack = append(stack, parseNumber(token))
			continue
		}
		right, left := pop(), pop()
		switch token {
		case "+":
			stack = append(stack, left+right)
		case "-":
			stack = append(stack, left-right)
		case "*":
			stack = append(stack, left*right)
		case "/":
			if right == 0 {
				stack = append(stack, 0)
			} else {
				stack = append(stack, left/right)
			}
		default: // an unmatched "(" left on the operator stack: the result is 0, as on the server before
			stack = append(stack, math.NaN())
		}
	}
	if len(stack) == 0 {
		return 0
	}
	return stack[0]
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

// jsNumberString is Number#toString for the values put into the expression: plain decimals between
// 1e-6 and 1e21, exponent notation outside ("1e-7", "1.5e+21").
func jsNumberString(value float64) string {
	abs := math.Abs(value)
	if abs == 0 || (abs >= 1e-6 && abs < 1e21) {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	text := strconv.FormatFloat(value, 'e', -1, 64) // "1e-07", "1.5e+21"
	mantissa, exponent, _ := strings.Cut(text, "e")
	sign := exponent[:1]
	exponent = strings.TrimLeft(exponent[1:], "0")
	return mantissa + "e" + sign + exponent
}
