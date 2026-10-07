// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"html"
	"html/template"
	"regexp"
	"strings"
)

// pdf-forms:boxes
// WithFillable is the system values with Fillable set: true renders fillable fields as empty boxes
// for a PDF form (specification section 4.5), false prints their answers.
func WithFillable(system map[string]any, fillable bool) map[string]any {
	out := make(map[string]any, len(system)+1)
	for key, value := range system {
		out[key] = value
	}
	out["Fillable"] = fillable
	return out
}

// BuildContext is the data a compiled template executes against (spec section 5). It does not
// change the maps it is given.
//
//   - file references in the answers (top level, in lists and in table rows) are replaced by the
//     names the renderer receives the files under (assets: reference -> file name);
//   - text areas (when the schema is given) become paragraphs: blank lines separate paragraphs,
//     single line breaks become <br>, and the text is escaped (text areas hold plain text);
//   - the answers are flattened into the root without overriding system values, and stay
//     available as .Answers;
//   - every other answer stays as it is: text is plain text, escaped where the template prints it,
//     so {{if eq .dept "R&D"}} compares what was typed.
func BuildContext(system, answers map[string]any, assets map[string]string, schema *Schema) map[string]any {
	values := make(map[string]any, len(answers))
	for key, value := range answers {
		values[key] = withAssets(value, assets)
	}
	if schema != nil {
		for _, field := range schema.Fields {
			if s, ok := values[field.Key].(string); ok && field.Kind == KindTextarea {
				values[field.Key] = template.HTML(Paragraphs(s))
			}
		}
	}

	context := make(map[string]any, len(system)+len(values)+1)
	for key, value := range system {
		context[key] = value
	}
	if _, exists := context["Answers"]; !exists {
		context["Answers"] = values
	}
	for key, value := range values {
		if _, exists := context[key]; !exists {
			context[key] = value
		}
	}
	return context
}

// withAssets is an answer with its file references replaced by file names, copied where it changes.
func withAssets(value any, assets map[string]string) any {
	switch v := value.(type) {
	case string:
		if name, ok := assets[v]; ok {
			return name
		}
	case []any:
		list := make([]any, len(v))
		for i, item := range v {
			list[i] = withAssets(item, assets)
		}
		return list
	case map[string]any: // a table row
		row := make(map[string]any, len(v))
		for column, cell := range v {
			if s, ok := cell.(string); ok {
				row[column] = withAssets(s, assets)
			} else {
				row[column] = cell
			}
		}
		return row
	}
	return value
}

var paragraphBreak = regexp.MustCompile(`\r?\n\s*\r?\n`)
var lineBreak = regexp.MustCompile(`\r?\n`)

// Paragraphs renders plain text as HTML paragraphs: "a\nb\n\nc" -> "<p>a<br>b</p><p>c</p>".
func Paragraphs(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var out strings.Builder
	for _, paragraph := range paragraphBreak.Split(text, -1) {
		out.WriteString("<p>")
		out.WriteString(lineBreak.ReplaceAllString(html.EscapeString(paragraph), "<br>"))
		out.WriteString("</p>")
	}
	return out.String()
}
