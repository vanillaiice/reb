// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"strings"
	"time"
)

// PlaceholderImage stands in for photos, signatures and logos in previews.
const PlaceholderImage = "data:image/svg+xml;utf8," +
	"<svg xmlns='http://www.w3.org/2000/svg' width='200' height='150'>" +
	"<rect width='100%25' height='100%25' fill='%23e2e8f0'/>" +
	"<text x='50%25' y='50%25' fill='%2394a3b8' font-family='sans-serif' font-size='14' " +
	"text-anchor='middle' dominant-baseline='middle'>Sample</text></svg>"

// SampleAnswers makes representative answers for a preview, so bindings, formulas and totals render
// real output instead of blanks: two table rows with numbers that make the totals non-zero, formula
// cells computed as in a real document, a field's default where it has one.
func SampleAnswers(schema Schema) map[string]any {
	answers := map[string]any{}
	for _, field := range schema.Fields {
		if field.Stored() {
			answers[field.Key] = sampleValue(field)
		}
	}
	return answers
}

// SampleSystem is the system values (spec section 5.1) a preview renders with.
func SampleSystem() map[string]any {
	return map[string]any{
		"ID": "00000000-0000-0000-0000-000000000000", "Name": "Sample document", "Number": 1, "Reference": "D-1",
		"ProjectName": "Sample project", "ReporterName": "Sample author", "TemplateName": "Sample template",
		"CreatedAt": time.Now().Format(time.RFC3339), "OrganizationName": "Sample organization",
		"OrganizationLogo": PlaceholderImage, "Attachments": []any{}, "Photos": []any{},
	}
}

func sampleValue(field Field) any {
	if field.Default != "" && field.Kind != KindTable {
		return defaultValue(field)
	}
	switch field.Kind {
	case KindNumber:
		return "42"
	case KindDate:
		return time.Now().Format("2006-01-02")
	case KindSelect:
		if len(field.Options) > 0 {
			return field.Options[0]
		}
		return ""
	case KindCheckbox:
		return true
	case KindSignature:
		return PlaceholderImage
	case KindImages:
		return []any{PlaceholderImage, PlaceholderImage}
	case KindTextarea:
		return "Sample paragraph text rendered for preview."
	case KindTable:
		return sampleRows(field.Columns)
	default:
		if field.Label != "" {
			return "Sample " + strings.ToLower(field.Label)
		}
		return "Sample text"
	}
}

// defaultValue is a field's default as a form would prefill it ("today" for dates, a ticked
// checkbox for "true").
func defaultValue(field Field) any {
	switch {
	case field.Kind == KindDate && strings.EqualFold(field.Default, "today"):
		return time.Now().Format("2006-01-02")
	case field.Kind == KindCheckbox:
		return checked(field.Default)
	}
	return field.Default
}

func sampleRows(columns []Column) []any {
	const rowCount = 2
	rows := make([]any, 0, rowCount)
	for r := range rowCount {
		row := map[string]any{}
		number := 0
		for _, column := range columns {
			switch column.Kind {
			case ColumnNumber:
				number++
				row[column.Key] = scalarText(float64(number*5) * float64(r+1))
			case ColumnCheckbox:
				row[column.Key] = true
			case ColumnSelect:
				if len(column.Options) > 0 {
					row[column.Key] = column.Options[0]
				}
			case ColumnImage, ColumnSignature:
				row[column.Key] = PlaceholderImage
			case ColumnFormula, ColumnAutoincrement:
				row[column.Key] = nil
			default:
				row[column.Key] = "Sample " + column.Key
			}
		}
		for _, column := range columns {
			switch column.Kind {
			case ColumnAutoincrement:
				row[column.Key] = scalarText(float64(r + 1))
			case ColumnFormula:
				precision := DefaultPrecision
				if column.Precision != nil {
					precision = *column.Precision
				}
				row[column.Key] = Formula(column.Expression, row, precision)
			}
		}
		rows = append(rows, row)
	}
	return rows
}
