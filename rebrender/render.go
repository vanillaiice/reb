// SPDX-License-Identifier: BUSL-1.1 OR GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Dual-licensed: BUSL-1.1 as part of the rebar server (/server/LICENSE.txt) and
// GPL-3.0-or-later as part of Rebar Studio (/rebar-studio/LICENSE), which
// compiles this package to WebAssembly.

// Package rebrender executes compiled .reb template HTML against context data.
//
// It is a leaf package depending only on the standard library so it can be
// reused both by the API server and by the WebAssembly build of the editor.
package rebrender

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"
)

// toFloat converts a template value to a number. Answers reach the renderer as
// float64 (JSON numbers), string (table cells, preview data) or template.HTML
// (top-level answers after sanitizing in the document export), so all of them
// must be understood; anything unparseable counts as 0 so formulas never panic.
func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int8:
		return float64(val)
	case int16:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint8:
		return float64(val)
	case uint16:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	case string:
		return parseFloat(val)
	case template.HTML:
		return parseFloat(string(val))
	case template.HTMLAttr:
		return parseFloat(string(val))
	default:
		return 0
	}
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// isInt reports whether v is an integer literal or integer-typed value, which
// is how a template passes a constant like the 2 in {{formatNumber .val 2}}.
func isInt(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	}
	return false
}

// formatNumber renders v with a fixed number of decimals. It accepts both the
// pipeline form {{sumColumn .rows "amount" | formatNumber 2}} (decimals first)
// and the form documented in the .reb spec, {{formatNumber .val 2}} (decimals
// last), which used to abort the whole render with a type error.
func formatNumber(a, b any) string {
	decimals, value := a, b
	if isInt(b) && !isInt(a) {
		decimals, value = b, a
	}
	d := int(toFloat(decimals))
	if d < 0 {
		d = 0
	} else if d > 20 {
		d = 20
	}
	return strconv.FormatFloat(toFloat(value), 'f', d, 64)
}

// formatMoney renders an amount with thousands separators after its currency code, as in
// "QAR 12,500.00". Like formatNumber it accepts the documented form {{formatMoney .val "QAR" 2}}
// and the pipeline form {{sumColumn .rows "amount" | formatMoney "QAR" 2}}, where the value
// comes last; the pipeline form is recognized by its integer second argument.
func formatMoney(a, b, c any) string {
	value, currency, decimals := a, b, c
	if isInt(b) {
		currency, decimals, value = a, b, c
	}
	number := formatNumber(value, decimals)
	sign := ""
	if strings.HasPrefix(number, "-") {
		sign, number = "-", number[1:]
	}
	whole, fraction, hasFraction := strings.Cut(number, ".")
	var grouped strings.Builder
	for i, digit := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	out := sign + grouped.String()
	if hasFraction {
		out += "." + fraction
	}
	if code := strings.TrimSpace(fmt.Sprintf("%v", currency)); code != "" && currency != nil {
		out = code + " " + out
	}
	return out
}

// CompileHTML parses the raw template HTML markup and executes it with the provided custom context data, returning the final output string.
func CompileHTML(htmlContent string, data any) (string, error) {
	tmpl, err := template.New("dynamic_form").Funcs(template.FuncMap{
		"now": func() time.Time {
			return time.Now()
		},
		"safeHTML": func(s any) template.HTML {
			// A missing or empty answer must render as nothing, not "<nil>".
			if s == nil {
				return ""
			}
			return template.HTML(fmt.Sprintf("%v", s))
		},
		"toFloat64": toFloat,
		"multiply": func(a, b any) float64 {
			return toFloat(a) * toFloat(b)
		},
		"add": func(a, b any) float64 {
			return toFloat(a) + toFloat(b)
		},
		"subtract": func(a, b any) float64 {
			return toFloat(a) - toFloat(b)
		},
		"divide": func(a, b any) float64 {
			denom := toFloat(b)
			if denom == 0 {
				return 0
			}
			return toFloat(a) / denom
		},
		"sumColumn": func(rows any, colKey string) float64 {
			var sum float64

			// rows could be []any or []map[string]any
			switch slice := rows.(type) {
			case []any:
				for _, rowAny := range slice {
					if rowMap, ok := rowAny.(map[string]any); ok {
						if val, exists := rowMap[colKey]; exists {
							sum += toFloat(val)
						}
					}
				}
			case []map[string]any:
				for _, rowMap := range slice {
					if val, exists := rowMap[colKey]; exists {
						sum += toFloat(val)
					}
				}
			}
			return sum
		},
		"formatNumber": formatNumber,
		"formatMoney":  formatMoney,
		"formatDate": func(layout string, s any) string {
			if s == nil {
				return ""
			}
			var t time.Time
			var dateStr string

			switch val := s.(type) {
			case time.Time:
				t = val
			case string:
				dateStr = val
			case template.HTML:
				dateStr = string(val)
			case template.HTMLAttr:
				dateStr = string(val)
			default:
				dateStr = fmt.Sprintf("%v", s)
			}

			if !t.IsZero() {
				return t.Format(layout)
			}

			if dateStr == "" || dateStr == "<nil>" {
				return ""
			}

			var err error
			for _, f := range []string{"2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05", "2006-01-02", "02/01/2006", "02/01/06"} {
				t, err = time.Parse(f, dateStr)
				if err == nil {
					break
				}
			}
			if err != nil {
				return dateStr
			}
			return t.Format(layout)
		},
	}).Parse(htmlContent)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
