//go:build js && wasm

// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

// Command wasm is the WebAssembly build of the .reb pipeline. It exposes the
// real Go compiler (pkg/rebcompiler) and template executor (pkg/rebrender) to
// the browser-based template editor so previews render with full fidelity,
// identical to the API/PDF backend.
//
// Build: GOOS=js GOARCH=wasm go build -o rebcompiler.wasm ./cmd/wasm
//
// It registers a single global JS function, __rebCompile(reb) -> jsonString,
// returning { schema, htmlSource, previewHtml, error?, execError? }.
package main

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"github.com/vanillaiice/reb/rebcompiler"
	"github.com/vanillaiice/reb/rebrender"
)

func main() {
	js.Global().Set("__rebCompile", js.FuncOf(compile))
	// Keep the Go runtime alive so the exported function stays callable.
	select {}
}

// compile runs the full .reb pipeline: tag compilation + template execution
// with auto-generated sample data, returning a JSON string for the JS caller.
func compile(_ js.Value, args []js.Value) any {
	out := map[string]any{}

	if len(args) < 1 {
		out["error"] = "missing reb source argument"
		return marshal(out)
	}
	reb := args[0].String()

	schemaJSON, htmlSrc, err := rebcompiler.Compile(reb)
	if err != nil {
		out["error"] = err.Error()
		return marshal(out)
	}
	out["schema"] = string(schemaJSON)
	out["htmlSource"] = htmlSrc

	var schema []rebcompiler.RebFieldSchema
	_ = json.Unmarshal(schemaJSON, &schema)

	preview, perr := rebrender.CompileHTML(htmlSrc, buildSampleData(schema))
	if perr != nil {
		// Fall back to the raw compiled HTML so the user still sees structure.
		out["execError"] = perr.Error()
		out["previewHtml"] = htmlSrc
	} else {
		out["previewHtml"] = preview
	}

	return marshal(out)
}

func marshal(m map[string]any) string {
	b, err := json.Marshal(m)
	if err != nil {
		return `{"error":"failed to encode result"}`
	}
	return string(b)
}

// 1x1 transparent-ish gray PNG used as a placeholder for image-bearing fields.
const placeholderImage = "data:image/svg+xml;utf8," +
	"<svg xmlns='http://www.w3.org/2000/svg' width='200' height='150'>" +
	"<rect width='100%25' height='100%25' fill='%23e2e8f0'/>" +
	"<text x='50%25' y='50%25' fill='%2394a3b8' font-family='sans-serif' font-size='14' " +
	"text-anchor='middle' dominant-baseline='middle'>Sample</text></svg>"

// buildSampleData generates representative values per field type so the executed
// template renders real output (bindings, formulas, totals) instead of blanks.
func buildSampleData(schema []rebcompiler.RebFieldSchema) map[string]any {
	data := map[string]any{}

	// First pass: tables. Their column names are also emitted as top-level
	// schema entries (from the inner reb-declare tags); collect them so we don't
	// shadow the per-row values with unused top-level scalars.
	tableCols := map[string]bool{}
	for _, f := range schema {
		if f.Type != "table" {
			continue
		}
		cols := parseColumns(f.Options)
		for _, c := range cols {
			tableCols[c.name] = true
		}
		data[f.Key] = sampleRows(cols)
	}

	// Second pass: scalar fields.
	for _, f := range schema {
		if f.Type == "table" || tableCols[f.Key] {
			continue
		}
		data[f.Key] = sampleScalar(f)
	}

	return data
}

func sampleScalar(f rebcompiler.RebFieldSchema) any {
	switch f.Type {
	case "number":
		// float64, like a JSON number in a real document, so the preview runs
		// the same code paths as the PDF export.
		return 42.0
	case "date":
		return time.Now().Format("2006-01-02")
	case "select", "radio":
		if len(f.Options) > 0 {
			return strings.TrimSpace(f.Options[0])
		}
		return ""
	case "signature":
		return placeholderImage
	case "photogrid", "attachments":
		return []any{placeholderImage, placeholderImage}
	case "textarea":
		return "<p>Sample paragraph text rendered for preview.</p>"
	default: // text, string, and anything else
		if f.Label != "" {
			return "Sample " + strings.ToLower(f.Label)
		}
		return "Sample text"
	}
}

type column struct {
	name string
	typ  string // "text", "number", or "formula"
	expr string // formula expression, e.g. "quantity*unit_price"
}

// parseColumns parses reb-table options like
// "description:text,quantity:number,amount:formula[quantity*unit_price|2]".
func parseColumns(options []string) []column {
	var cols []column
	for _, raw := range options {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		name, spec, _ := strings.Cut(raw, ":")
		name = strings.TrimSpace(name)
		spec = strings.TrimSpace(spec)

		c := column{name: name, typ: "text"}
		switch {
		case strings.HasPrefix(spec, "formula"):
			c.typ = "formula"
			if i := strings.Index(spec, "["); i != -1 {
				inner := strings.TrimSuffix(spec[i+1:], "]")
				// Drop the optional "|N" rounding hint; preview uses formatNumber.
				inner, _, _ = strings.Cut(inner, "|")
				c.expr = strings.TrimSpace(inner)
			}
		case spec == "number":
			c.typ = "number"
		}
		cols = append(cols, c)
	}
	return cols
}

// sampleRows builds two plausible rows, computing formula columns from the
// other sample values so totals (sumColumn) are non-zero.
func sampleRows(cols []column) []map[string]any {
	const rowCount = 2
	rows := make([]map[string]any, 0, rowCount)

	for r := 0; r < rowCount; r++ {
		row := map[string]any{}
		// Base values first (so formulas can reference them).
		numIdx := 0
		for _, c := range cols {
			switch c.typ {
			case "number":
				row[c.name] = float64((numIdx+1)*5) * float64(r+1)
				numIdx++
			case "formula":
				// computed below
			default:
				row[c.name] = "Sample " + c.name
			}
		}
		// Formula values, now that base columns exist.
		for _, c := range cols {
			if c.typ == "formula" {
				row[c.name] = evalFormula(c.expr, row)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// evalFormula evaluates a table formula over the row's columns the way the mobile app, the web form
// and the server do (client/mobile/utils/formula.ts, lib/reb/formula.rb): column names become the
// row's numbers, other names 0, then + - * / and parentheses with the usual precedence; dividing by
// zero gives 0.
func evalFormula(expr string, row map[string]any) float64 {
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, key := range keys {
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(key) + `\b`)
		expr = pattern.ReplaceAllString(expr, strconv.FormatFloat(toFloat(row[key]), 'f', -1, 64))
	}
	expr = regexp.MustCompile(`[a-zA-Z_]\w*`).ReplaceAllString(expr, "0")

	var tokens []string
	for i := 0; i < len(expr); {
		ch := expr[i]
		switch {
		case strings.ContainsRune("+-*/()", rune(ch)):
			tokens = append(tokens, string(ch))
			i++
		case ch == '.' || (ch >= '0' && ch <= '9'):
			j := i
			for j < len(expr) && (expr[j] == '.' || (expr[j] >= '0' && expr[j] <= '9')) {
				j++
			}
			tokens = append(tokens, expr[i:j])
			i = j
		default:
			i++
		}
	}

	precedence := map[string]int{"+": 1, "-": 1, "*": 2, "/": 2}
	isNumber := func(token string) bool {
		_, err := strconv.ParseFloat(strings.TrimRight(token, "."), 64)
		return err == nil && token != "."
	}
	var output, operators []string
	for _, token := range tokens {
		switch {
		case isNumber(token):
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
	for len(operators) > 0 {
		output = append(output, operators[len(operators)-1])
		operators = operators[:len(operators)-1]
	}

	var stack []float64
	pop := func() float64 {
		if len(stack) == 0 {
			return 0
		}
		value := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return value
	}
	for _, token := range output {
		if isNumber(token) {
			stack = append(stack, toFloat(token))
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
		}
	}
	if len(stack) == 0 || math.IsInf(stack[0], 0) || math.IsNaN(stack[0]) {
		return 0
	}
	return stack[0]
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}
