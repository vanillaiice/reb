// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"errors"
	"regexp"
	"strings"

	"github.com/vanillaiice/reb"
	"github.com/vanillaiice/reb/internal/rebcompiler"
	"github.com/vanillaiice/reb/internal/rebrender"
)

// Compiled is what a consumer keeps for a template version.
type Compiled struct {
	Schema        []rebcompiler.RebFieldSchema `json:"schema"` // raw (schemaVersion 1), for older clients
	Fields        Schema                       `json:"fields"` // normalized
	HTML          string                       `json:"html"`   // the Go template to render documents with
	EngineVersion string                       `json:"engineVersion"`
	Warnings      []Warning                    `json:"warnings"`
}

// Error is a template that cannot be used, with a code and parameters a consumer can translate:
//
//	invalid_field_name  a name that is not letters, digits and underscores {name, tag}
//	invalid_show_if     a show-if condition that does not parse {field, detail}
//	invalid_pattern     a pattern that is not a valid regular expression {field}
//	syntax              Go template syntax that could never render {detail}
//	invalid             anything else {detail}
type Error struct {
	Code    string            `json:"code"`
	Params  map[string]string `json:"params,omitempty"`
	Message string            `json:"error"`
}

func (e *Error) Error() string { return e.Message }

// Warning is something to fix that does not stop the template from working:
//
//	missing_label          a field without a label {field}
//	show_if_unknown_field  a show-if condition reading a field the template does not declare {field, name}
//	unknown_binding        {{.name}} that no field, system value or table column names {name, table?}
//	unused_field           a field the template never prints, tests or reads in a show-if {field}
//	duplicate_field        one name declared as fields of different kinds {field}
//	fillable_ignored       (pdf-forms:boxes) the fillable attribute on a tag that cannot be fillable {field, tag}
type Warning struct {
	Code    string            `json:"code"`
	Params  map[string]string `json:"params,omitempty"`
	Message string            `json:"message"`
}

// Compile compiles .reb source, normalizes its schema and checks what the compiler does not: show-if
// conditions and patterns parse, and the template's Go syntax could render.
func Compile(source string) (*Compiled, error) {
	raw, html, err := rebcompiler.Compile(source)
	if err != nil {
		var compileError *rebcompiler.Error
		if errors.As(err, &compileError) {
			return nil, &Error{Code: compileError.Code, Params: compileError.Params, Message: compileError.Message}
		}
		return nil, &Error{Code: "invalid", Params: map[string]string{"detail": err.Error()}, Message: err.Error()}
	}

	if err := rebrender.Check(html); err != nil {
		detail := strings.TrimPrefix(err.Error(), "template: dynamic_form:")
		return nil, &Error{Code: "syntax", Params: map[string]string{"detail": detail}, Message: "template syntax: " + detail}
	}

	fields := Normalize(raw)
	compiled := &Compiled{Schema: raw, Fields: fields, HTML: html, EngineVersion: reb.Version, Warnings: []Warning{}}
	declared := map[string]bool{}
	for _, entry := range raw {
		declared[entry.Key] = true
	}
	for _, field := range fields.Fields {
		if field.ShowIf != "" {
			condition, err := ParseShowIf(field.ShowIf)
			if err != nil {
				return nil, &Error{Code: "invalid_show_if", Params: map[string]string{"field": field.Key, "detail": err.Error()},
					Message: "invalid show-if on " + field.Key + ": " + err.Error()}
			}
			for _, name := range condition.Fields {
				if !declared[name] {
					compiled.Warnings = append(compiled.Warnings, Warning{Code: "show_if_unknown_field", Params: map[string]string{"field": field.Key, "name": name},
						Message: "the show-if of " + field.Key + " reads " + name + ", which the template does not declare"})
				}
			}
		}
		if field.Pattern != "" {
			if _, err := regexp.Compile(field.Pattern); err != nil {
				return nil, &Error{Code: "invalid_pattern", Params: map[string]string{"field": field.Key},
					Message: "invalid pattern on " + field.Key + ": " + err.Error()}
			}
		}
		if field.Stored() && strings.TrimSpace(field.Label) == "" {
			compiled.Warnings = append(compiled.Warnings, Warning{Code: "missing_label", Params: map[string]string{"field": field.Key},
				Message: field.Key + " has no label"})
		}
	}
	compiled.Warnings = append(compiled.Warnings, lint(source, html, fields)...)
	return compiled, nil
}
