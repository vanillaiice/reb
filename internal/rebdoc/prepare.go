// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// FieldError is a refused answer: Key is the field, Code names the reason and Params fill the
// message, so each consumer words it in its own language. An error in a table cell also has the
// params row (from 0, in the rows Prepare returns) and column (its key).
//
//	invalid           not in the expected form (also a text not matching its pattern)
//	too_long          longer than {count} characters
//	not_a_number      not a number
//	invalid_date      not a date (YYYY-MM-DD)
//	not_an_option     not one of the choices
//	too_many_files    more than {count} files
//	too_many_rows     more than {count} rows
//	blank             required and left empty
//	too_small         below the minimum {count}
//	too_large         above the maximum {count}
type FieldError struct {
	Key    string         `json:"key"`
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// Prepared is the outcome of Prepare.
type Prepared struct {
	Answers map[string]any `json:"answers"`
	Errors  []FieldError   `json:"errors"`
}

var numberPattern = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$`)

// Prepare checks and cleans a document's answers against its schema:
//
//   - unknown keys and sections are dropped; text is limited; numbers and dates must parse (dates
//     come back as YYYY-MM-DD); a select answer must be one of its options; checkboxes become true
//     or false; table rows keep only their columns, with formula and row-number cells computed here
//     whatever was sent;
//   - fields whose show-if condition does not hold are dropped, and only visible fields are
//     checked for required, min, max and pattern;
//   - file answers (photo grids, signatures, photo and signature cells) are kept as the trimmed
//     references the caller sent: what a reference may point to is the caller's rule.
func Prepare(schema Schema, input map[string]any) Prepared {
	p := &preparer{errors: []FieldError{}}
	answers := map[string]any{}
	for _, field := range schema.Fields {
		if !field.Stored() {
			continue
		}
		if value, ok := input[field.Key]; ok {
			answers[field.Key] = p.value(field, value)
		}
	}

	visible := visibility(schema, answers)
	for _, field := range schema.Fields {
		if !field.Stored() {
			continue
		}
		if !visible[field.Key] {
			delete(answers, field.Key)
			continue
		}
		p.rules(field, answers[field.Key])
	}

	return Prepared{Answers: answers, Errors: p.errors}
}

// visibility evaluates the show-if conditions until nothing changes, so a field hidden by another
// hidden field is hidden too (a hidden field reads as unanswered).
func visibility(schema Schema, answers map[string]any) map[string]bool {
	conditions := map[string]*ShowIf{}
	visible := map[string]bool{}
	for _, field := range schema.Fields {
		visible[field.Key] = true
		if field.ShowIf != "" {
			if condition, err := ParseShowIf(field.ShowIf); err == nil {
				conditions[field.Key] = condition
			}
		}
	}
	for range len(schema.Fields) + 1 {
		seen := map[string]any{}
		for key, value := range answers {
			if visible[key] {
				seen[key] = value
			}
		}
		changed := false
		for key, condition := range conditions {
			if holds := condition.Holds(seen); holds != visible[key] {
				visible[key] = holds
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return visible
}

type preparer struct{ errors []FieldError }

func (p *preparer) fail(key, code string, params map[string]any) {
	p.errors = append(p.errors, FieldError{Key: key, Code: code, Params: params})
}

func (p *preparer) value(field Field, value any) any {
	switch field.Kind {
	case KindTextarea:
		return p.text(field.Key, value, TextareaLimit)
	case KindNumber:
		return p.number(field.Key, value)
	case KindDate:
		return p.date(field.Key, value)
	case KindSelect:
		return p.option(field.Key, value, field.Options)
	case KindCheckbox:
		return checked(value)
	case KindImages:
		return p.images(field.Key, value)
	case KindSignature:
		return reference(value)
	case KindTable:
		return p.rows(field, value)
	default:
		return p.text(field.Key, value, TextLimit)
	}
}

func (p *preparer) text(key string, value any, limit int) string {
	switch v := value.(type) {
	case nil:
		return ""
	case map[string]any, []any:
		p.fail(key, "invalid", nil)
		return ""
	default:
		s := scalarText(v)
		if utf8.RuneCountInString(s) > limit {
			p.fail(key, "too_long", map[string]any{"count": limit})
			s = string([]rune(s)[:limit])
		}
		return s
	}
}

func (p *preparer) number(key string, value any) string {
	s := strings.TrimSpace(scalarText(value))
	if s == "" {
		return ""
	}
	if !numberPattern.MatchString(s) {
		p.fail(key, "not_a_number", nil)
		return ""
	}
	return s
}

func (p *preparer) date(key string, value any) string {
	s := strings.TrimSpace(scalarText(value))
	if s == "" {
		return ""
	}
	for _, layout := range []string{"2006-01-02", "20060102"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	p.fail(key, "invalid_date", nil)
	return ""
}

func (p *preparer) option(key string, value any, options []string) string {
	s := strings.TrimSpace(scalarText(value))
	if s == "" || len(options) == 0 {
		return s
	}
	for _, option := range options {
		if option == s {
			return s
		}
	}
	p.fail(key, "not_an_option", nil)
	return ""
}

func (p *preparer) images(key string, value any) []any {
	list, ok := value.([]any)
	if !ok {
		list = []any{value}
	}
	references := []any{}
	for _, item := range list {
		if s := reference(item); s != "" {
			references = append(references, s)
		}
	}
	if len(references) > MaxImages {
		p.fail(key, "too_many_files", map[string]any{"count": MaxImages})
		references = references[:MaxImages]
	}
	return references
}

func (p *preparer) rows(field Field, value any) []any {
	var list []any
	switch v := value.(type) {
	case nil:
		return []any{}
	case string:
		if strings.TrimSpace(v) == "" { // a form's empty table sends ""
			return []any{}
		}
		p.fail(field.Key, "invalid", nil)
		return []any{}
	case []any:
		list = v
	case map[string]any: // a form's rows arrive keyed by index
		indexes := make([]string, 0, len(v))
		for index := range v {
			indexes = append(indexes, index)
		}
		sort.SliceStable(indexes, func(i, j int) bool { return leadingInt(indexes[i]) < leadingInt(indexes[j]) })
		for _, index := range indexes {
			list = append(list, v[index])
		}
	default:
		p.fail(field.Key, "invalid", nil)
		return []any{}
	}

	if len(list) > MaxRows {
		p.fail(field.Key, "too_many_rows", map[string]any{"count": MaxRows})
		list = list[:MaxRows]
	}
	rows := make([]any, 0, len(list))
	for index, item := range list {
		row, _ := item.(map[string]any)
		cells := map[string]any{}
		for _, column := range field.Columns {
			cells[column.Key] = p.cell(field.Key, index, column, row[column.Key])
		}
		for _, column := range field.Columns {
			switch column.Kind {
			case ColumnAutoincrement:
				cells[column.Key] = strconv.Itoa(index + 1)
			case ColumnFormula:
				precision := DefaultPrecision
				if column.Precision != nil {
					precision = *column.Precision
				}
				cells[column.Key] = Formula(column.Expression, cells, precision)
			}
		}
		rows = append(rows, cells)
	}
	return rows
}

// cell cleans one typed cell; its errors carry the row (from 0, in the rows returned) and the column.
func (p *preparer) cell(key string, row int, column Column, value any) any {
	before := len(p.errors)
	cleaned := p.cellValue(key, column, value)
	for i := before; i < len(p.errors); i++ {
		params := map[string]any{"row": row, "column": column.Key}
		for name, param := range p.errors[i].Params {
			params[name] = param
		}
		p.errors[i].Params = params
	}
	return cleaned
}

func (p *preparer) cellValue(key string, column Column, value any) any {
	switch column.Kind {
	case ColumnNumber:
		return p.number(key, value)
	case ColumnCheckbox:
		return checked(value)
	case ColumnSelect:
		return p.option(key, value, column.Options)
	case ColumnImage, ColumnSignature:
		return reference(value)
	case ColumnFormula, ColumnAutoincrement:
		return nil // computed once every typed cell is known
	default:
		return p.text(key, value, TextLimit)
	}
}

// rules checks a visible field's v1.1 constraints on its cleaned value.
func (p *preparer) rules(field Field, value any) {
	if field.Required && blank(value) {
		p.fail(field.Key, "blank", nil)
		return
	}
	s, isText := value.(string)
	if !isText || s == "" {
		return
	}
	switch field.Kind {
	case KindNumber:
		n, _ := strconv.ParseFloat(s, 64)
		if bound, err := strconv.ParseFloat(field.Min, 64); err == nil && n < bound {
			p.fail(field.Key, "too_small", map[string]any{"count": field.Min})
		} else if bound, err := strconv.ParseFloat(field.Max, 64); err == nil && n > bound {
			p.fail(field.Key, "too_large", map[string]any{"count": field.Max})
		}
	case KindDate: // YYYY-MM-DD compares as text
		if field.Min != "" && s < field.Min {
			p.fail(field.Key, "too_small", map[string]any{"count": field.Min})
		} else if field.Max != "" && s > field.Max {
			p.fail(field.Key, "too_large", map[string]any{"count": field.Max})
		}
	case KindText:
		if field.Pattern != "" {
			if pattern, err := regexp.Compile(`^(?:` + field.Pattern + `)$`); err == nil && !pattern.MatchString(s) {
				p.fail(field.Key, "invalid", nil)
			}
		}
	}
}

func blank(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case bool:
		return !v // a required checkbox must be ticked
	case []any:
		return len(v) == 0
	}
	return false
}

func checked(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case float64:
		return v == 1
	case string:
		return v == "true" || v == "1" || v == "on"
	}
	return false
}

// reference is a file answer as sent, trimmed; anything but text is no reference.
func reference(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// scalarText is a scalar answer as text (JSON numbers without a trailing ".0").
func scalarText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1e15 {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

// leadingInt is Ruby's String#to_i for row indexes ("2" -> 2, "x" -> 0).
func leadingInt(s string) int {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}
