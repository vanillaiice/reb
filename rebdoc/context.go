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

	"github.com/microcosm-cc/bluemonday"
)

// BuildContext is the data a compiled template executes against (spec section 5):
//
//   - file references in the answers (top level, in lists and in table rows) are replaced by the
//     names the renderer receives the files under (assets: reference -> file name);
//   - text areas (when the schema is given) become paragraphs: blank lines separate paragraphs,
//     single line breaks become <br>, and the text is escaped (text areas hold plain text);
//   - the answers are flattened into the root without overriding system values, and stay
//     available as .Answers;
//   - plain strings are sanitized with bluemonday's UGC policy and passed as HTML, while replaced
//     file references stay plain strings so templates can use them in src attributes.
func BuildContext(system, answers map[string]any, assets map[string]string, schema *Schema) map[string]any {
	if answers == nil {
		answers = map[string]any{}
	}
	if schema != nil {
		for _, field := range schema.Fields {
			if s, ok := answers[field.Key].(string); ok && field.Kind == KindTextarea {
				answers[field.Key] = Paragraphs(s)
			}
		}
	}

	isAsset := map[string]bool{}
	for key, value := range answers {
		switch val := value.(type) {
		case string:
			if name, ok := assets[val]; ok {
				answers[key] = name
				isAsset[key] = true
			}
		case []any:
			for i, item := range val {
				switch it := item.(type) {
				case string:
					if name, ok := assets[it]; ok {
						val[i] = name
					}
				case map[string]any:
					for column, cell := range it {
						if s, ok := cell.(string); ok {
							if name, ok := assets[s]; ok {
								it[column] = name
							}
						}
					}
				}
			}
		}
	}

	context := make(map[string]any, len(system)+len(answers)+1)
	for key, value := range system {
		context[key] = value
	}
	if _, exists := context["Answers"]; !exists {
		context["Answers"] = answers
	}

	sanitizer := bluemonday.UGCPolicy()
	for key, value := range answers {
		if _, exists := context[key]; exists {
			continue
		}
		if s, ok := value.(string); ok && !isAsset[key] {
			context[key] = template.HTML(sanitizer.Sanitize(s))
		} else {
			context[key] = value
		}
	}
	return context
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
