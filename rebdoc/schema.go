// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

// Package rebdoc is what a consumer needs around a compiled template to fill and render documents:
// the normalized schema (Normalize), answer validation and formulas (Prepare), show-if conditions,
// the render context (BuildContext) and sample answers for previews. Rebar's server (through rebc)
// and Rebar Studio (through the WebAssembly build) both use it, so a template behaves the same in
// both.
package rebdoc

import (
	"strconv"
	"strings"

	"github.com/vanillaiice/reb/rebcompiler"
)

// SchemaVersion is the version of the normalized schema below. The raw schema the compiler emits
// (rebcompiler.RebFieldSchema, read by older clients) is version 1.
const SchemaVersion = 2

// Limits every consumer enforces the same way.
const (
	TextLimit        = 10_000 // characters in a text answer
	TextareaLimit    = 50_000 // characters in a text area answer
	MaxRows          = 500    // rows in a table
	MaxImages        = 100    // files in a photo grid
	DefaultPrecision = 2      // decimals of a formula column without |N
)

// Field kinds. Aliases map onto them ("string" is text, "radio" a select shown as radio buttons,
// "photogrid", "attachments" and "image" are images); an unknown type is text.
const (
	KindSection   = "section"
	KindText      = "text"
	KindNumber    = "number"
	KindDate      = "date"
	KindTextarea  = "textarea"
	KindSelect    = "select"
	KindCheckbox  = "checkbox"
	KindImages    = "images"
	KindSignature = "signature"
	KindTable     = "table"
)

// Column kinds of a table.
const (
	ColumnText          = "text"
	ColumnNumber        = "number"
	ColumnCheckbox      = "checkbox"
	ColumnSelect        = "select"
	ColumnImage         = "image"
	ColumnSignature     = "signature"
	ColumnFormula       = "formula"
	ColumnAutoincrement = "autoincrement"
)

var fieldKinds = map[string]string{
	"section": KindSection, "text": KindText, "string": KindText, "number": KindNumber, "date": KindDate,
	"textarea": KindTextarea, "select": KindSelect, "radio": KindSelect, "checkbox": KindCheckbox,
	"photogrid": KindImages, "attachments": KindImages, "image": KindImages, "signature": KindSignature, "table": KindTable,
}

var columnKinds = map[string]string{
	"text": ColumnText, "number": ColumnNumber, "checkbox": ColumnCheckbox, "select": ColumnSelect, "photo": ColumnImage,
	"image": ColumnImage, "signature": ColumnSignature, "formula": ColumnFormula, "autoincrement": ColumnAutoincrement,
}

// Schema is a template's fields in order, sections included, table columns folded into their table.
type Schema struct {
	Version int     `json:"schemaVersion"`
	Fields  []Field `json:"fields"`
}

// Field is one input of the form. Type keeps the declared type ("radio" and "select" share a kind).
type Field struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Kind        string   `json:"kind"`
	Label       string   `json:"label"`
	Options     []string `json:"options,omitempty"`
	Columns     []Column `json:"columns,omitempty"`
	MaxLength   int      `json:"maxLength,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Help        string   `json:"help,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Default     string   `json:"default,omitempty"`
	Min         string   `json:"min,omitempty"`
	Max         string   `json:"max,omitempty"`
	Step        string   `json:"step,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	ShowIf      string   `json:"showIf,omitempty"`
}

// Column is one column of a table field.
type Column struct {
	Key        string   `json:"key"`
	Kind       string   `json:"kind"`
	Label      string   `json:"label"`
	Options    []string `json:"options,omitempty"`
	Expression string   `json:"expression,omitempty"`
	Precision  *int     `json:"precision,omitempty"`
}

// Stored reports whether the field has an answer (sections do not).
func (f Field) Stored() bool { return f.Kind != KindSection }

// Computed reports whether the column's cells are computed rather than typed.
func (c Column) Computed() bool { return c.Kind == ColumnFormula || c.Kind == ColumnAutoincrement }

// Normalize types the compiler's raw schema. Entries without a key are ignored; columns declared
// with <reb-declare> inside a table also appear as top-level entries and are answered per row, so
// they are dropped from the top level.
func Normalize(raw []rebcompiler.RebFieldSchema) Schema {
	var entries []rebcompiler.RebFieldSchema
	for _, entry := range raw {
		if strings.TrimSpace(entry.Key) != "" {
			entries = append(entries, entry)
		}
	}

	columnKeys := map[string]bool{}
	for _, entry := range entries {
		if entry.Type == "table" {
			for _, column := range ParseColumns(entry.Options) {
				columnKeys[column.Key] = true
			}
		}
	}

	schema := Schema{Version: SchemaVersion, Fields: []Field{}}
	for _, entry := range entries {
		if entry.Type != "table" && columnKeys[entry.Key] {
			continue
		}
		schema.Fields = append(schema.Fields, build(entry))
	}
	return schema
}

func build(entry rebcompiler.RebFieldSchema) Field {
	kind, ok := fieldKinds[entry.Type]
	if !ok {
		kind = KindText
	}
	field := Field{
		Key: entry.Key, Type: entry.Type, Kind: kind, Label: entry.Label,
		Required: entry.Required, Help: entry.Help, Placeholder: entry.Placeholder, Default: entry.Default,
		Min: entry.Min, Max: entry.Max, Step: entry.Step, Pattern: entry.Pattern, ShowIf: entry.ShowIf,
	}
	switch kind {
	case KindTable:
		field.Columns = ParseColumns(entry.Options)
	case KindText:
		field.MaxLength = TextLimit
		field.Options = entry.Options
	case KindTextarea:
		field.MaxLength = TextareaLimit
	default:
		field.Options = entry.Options
	}
	return field
}

// ParseColumns reads a table's options the way the mobile app does:
// "qty:number,total:formula[qty*rate|2],ok:checkbox,grade:select[A|B]". The text before the first
// ":" is the key and the text up to a second ":" the type (text when missing or unknown).
func ParseColumns(options []string) []Column {
	var columns []Column
	for _, option := range options {
		if strings.TrimSpace(option) == "" {
			continue
		}
		parts := strings.Split(option, ":")
		key := strings.TrimSpace(parts[0])
		spec := ""
		if len(parts) > 1 {
			spec = strings.TrimSpace(parts[1])
		}
		if spec == "" {
			spec = "text"
		}
		column := Column{Key: key, Label: columnLabel(key)}
		switch {
		case strings.HasPrefix(spec, "formula[") && strings.HasSuffix(spec, "]"):
			expression, precisionText, hasPrecision := strings.Cut(spec[len("formula["):len(spec)-1], "|")
			precision := DefaultPrecision
			if hasPrecision {
				if n, err := strconv.Atoi(strings.TrimSpace(precisionText)); err == nil {
					precision = min(max(n, 0), 10)
				}
			}
			column.Kind, column.Expression, column.Precision = ColumnFormula, expression, &precision
		case strings.HasPrefix(spec, "select[") && strings.HasSuffix(spec, "]"):
			column.Kind = ColumnSelect
			for _, choice := range strings.Split(spec[len("select["):len(spec)-1], "|") {
				if choice = strings.TrimSpace(choice); choice != "" {
					column.Options = append(column.Options, choice)
				}
			}
		default:
			kind, ok := columnKinds[spec]
			if !ok {
				kind = ColumnText
			}
			column.Kind = kind
		}
		columns = append(columns, column)
	}
	return columns
}

// "unit_price" -> "Unit Price" (each word capitalized, the rest lower case, as the mobile app shows
// column headings).
func columnLabel(key string) string {
	words := strings.Split(key, "_")
	for i, word := range words {
		if word != "" {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}
