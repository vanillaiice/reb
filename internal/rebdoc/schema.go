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

	"github.com/vanillaiice/reb/internal/rebcompiler"
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
	MaxPrecision     = 10     // decimals of a formula column at most
)

// Budgets a schema given as JSON must fit (Schema.Validate); a compiled template is far below them.
const (
	MaxFields     = 2_000 // fields of a schema
	MaxColumns    = 200   // columns of a table
	MaxOptions    = 1_000 // choices of a select field or column
	MaxExpression = 1_000 // characters of a formula expression
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
	Fillable    bool     `json:"fillable,omitempty"` // pdf-forms:boxes: typed (or ticked) into a PDF field when rendered fillable
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

// SchemaInput is how rebc and the WebAssembly build receive a schema: the normalized one ("fields")
// or the raw one the compiler emits ("schema").
type SchemaInput struct {
	Fields *Schema                      `json:"fields"`
	Schema []rebcompiler.RebFieldSchema `json:"schema"`
}

// Resolve is the normalized schema given, else the raw one normalized, else nil; a schema that does
// not pass Validate is an invalid_schema error.
func (s SchemaInput) Resolve() (*Schema, error) {
	schema := s.Fields
	if schema == nil && s.Schema != nil {
		normalized := Normalize(s.Schema)
		schema = &normalized
	}
	if schema == nil {
		return nil, nil
	}
	if err := schema.Validate(); err != nil {
		return nil, err
	}
	return schema, nil
}

// Validate checks a schema that arrived as JSON rather than from Normalize: its version, every
// field and column kind, formula precision and the size budgets above.
func (s Schema) Validate() error {
	invalid := func(detail string) error {
		return &Error{Code: "invalid_schema", Params: map[string]string{"detail": detail}, Message: "invalid schema: " + detail}
	}
	if s.Version != SchemaVersion {
		return invalid("schemaVersion " + strconv.Itoa(s.Version) + " is not " + strconv.Itoa(SchemaVersion))
	}
	if len(s.Fields) > MaxFields {
		return invalid("more than " + strconv.Itoa(MaxFields) + " fields")
	}
	kinds := map[string]bool{}
	for _, kind := range fieldKinds {
		kinds[kind] = true
	}
	columnKindSet := map[string]bool{}
	for _, kind := range columnKinds {
		columnKindSet[kind] = true
	}
	for _, field := range s.Fields {
		if strings.TrimSpace(field.Key) == "" {
			return invalid("a field without a key")
		}
		if !kinds[field.Kind] {
			return invalid(field.Key + " has the unknown kind " + strconv.Quote(field.Kind))
		}
		if len(field.Options) > MaxOptions {
			return invalid(field.Key + " has more than " + strconv.Itoa(MaxOptions) + " options")
		}
		if len(field.Columns) > MaxColumns {
			return invalid(field.Key + " has more than " + strconv.Itoa(MaxColumns) + " columns")
		}
		for _, column := range field.Columns {
			where := field.Key + "." + column.Key
			switch {
			case strings.TrimSpace(column.Key) == "":
				return invalid("a column of " + field.Key + " without a key")
			case !columnKindSet[column.Kind]:
				return invalid(where + " has the unknown kind " + strconv.Quote(column.Kind))
			case len(column.Options) > MaxOptions:
				return invalid(where + " has more than " + strconv.Itoa(MaxOptions) + " options")
			case len(column.Expression) > MaxExpression:
				return invalid(where + " has a formula longer than " + strconv.Itoa(MaxExpression) + " characters")
			case column.Precision != nil && (*column.Precision < 0 || *column.Precision > MaxPrecision):
				return invalid(where + " has a precision outside 0.." + strconv.Itoa(MaxPrecision))
			}
		}
	}
	return nil
}

// Stored reports whether the field has an answer (sections do not).
func (f Field) Stored() bool { return f.Kind != KindSection }

// Computed reports whether the column's cells are computed rather than typed.
func (c Column) Computed() bool { return c.Kind == ColumnFormula || c.Kind == ColumnAutoincrement }

// Normalize types the compiler's raw schema. Entries without a key are ignored; a tag inside a table
// that declares one of its columns (marked with its table by the compiler) is answered per row, so it
// is dropped from the top level, while a top-level field of the same name stays. A raw schema from
// before v0.8.0 has no marks, so all its entries are top-level.
func Normalize(raw []rebcompiler.RebFieldSchema) Schema {
	schema := Schema{Version: SchemaVersion, Fields: []Field{}}
	for _, entry := range raw {
		if strings.TrimSpace(entry.Key) == "" || entry.Table != "" {
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
		Fillable: entry.Fillable, // pdf-forms:boxes
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
					precision = min(max(n, 0), MaxPrecision)
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
