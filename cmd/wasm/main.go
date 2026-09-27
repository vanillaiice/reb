//go:build js && wasm

// SPDX-License-Identifier: BUSL-1.1 OR GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Dual-licensed: under the Business Source License 1.1 as part of the rebar
// server (see /server/LICENSE.txt), and under the GNU GPL v3.0-or-later as part
// of Rebar Studio (see /rebar-studio/LICENSE), into which this file is
// compiled (WebAssembly).

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
	"strconv"
	"strings"
	"syscall/js"
	"time"

	"github.com/vanillaiice/rebar-on-rails/reb/rebcompiler"
	"github.com/vanillaiice/rebar-on-rails/reb/rebrender"
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

// evalFormula evaluates a simple left-to-right arithmetic expression over the
// row's columns, e.g. "quantity*unit_price". Operands are column names or
// numeric literals; supported operators are + - * /. Unknown forms yield 0.
func evalFormula(expr string, row map[string]any) float64 {
	var tokens []string
	var cur strings.Builder
	for _, ch := range expr {
		switch ch {
		case '+', '-', '*', '/':
			if cur.Len() > 0 {
				tokens = append(tokens, strings.TrimSpace(cur.String()))
				cur.Reset()
			}
			tokens = append(tokens, string(ch))
		default:
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, strings.TrimSpace(cur.String()))
	}
	if len(tokens) == 0 {
		return 0
	}

	resolve := func(tok string) float64 {
		if v, ok := row[tok]; ok {
			return toFloat(v)
		}
		f, _ := strconv.ParseFloat(tok, 64)
		return f
	}

	acc := resolve(tokens[0])
	for i := 1; i+1 < len(tokens); i += 2 {
		rhs := resolve(tokens[i+1])
		switch tokens[i] {
		case "+":
			acc += rhs
		case "-":
			acc -= rhs
		case "*":
			acc *= rhs
		case "/":
			if rhs != 0 {
				acc /= rhs
			}
		}
	}
	return acc
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
