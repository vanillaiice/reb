// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"sort"
	"strings"
	"text/template/parse"

	"golang.org/x/net/html"

	"github.com/vanillaiice/reb/internal/rebcompiler"
	"github.com/vanillaiice/reb/internal/rebrender"
)

// SystemValues are the names a consumer passes besides the answers (specification section 5.1).
var SystemValues = []string{
	"ID", "Name", "Number", "Reference", "ProjectName", "ReporterName", "TemplateName", "CreatedAt",
	"OrganizationName", "OrganizationLogo", "Attachments", "Photos", "Answers",
	"Fillable", // pdf-forms:boxes
}

// lint reads what a compiled template prints and reports (specification section 8.2):
//
//	unknown_binding  {{.name}} where no field, system value or (inside a table's rows) column is named so
//	unused_field     a field the template never prints, tests or reads from a show-if
//	duplicate_field  one name declared as fields of different kinds
//	fillable_ignored the fillable attribute on a tag that cannot be fillable
func lint(source, compiled string, schema Schema) []Warning {
	tree, err := rebrender.Tree(compiled)
	if err != nil || tree == nil || tree.Root == nil {
		return nil // Compile has already refused a template that does not parse
	}
	l := &linter{schema: schema, known: map[string]bool{}, tables: map[string]map[string]bool{}, used: map[string]bool{}, reported: map[string]bool{}}
	for _, name := range SystemValues {
		l.known[name] = true
	}
	for _, field := range schema.Fields {
		l.known[field.Key] = true
		if field.Kind == KindTable {
			columns := map[string]bool{}
			for _, column := range field.Columns {
				columns[column.Key] = true
			}
			l.tables[field.Key] = columns
		}
	}
	l.walk(tree.Root, scope{})

	for _, field := range schema.Fields {
		if field.ShowIf == "" {
			continue
		}
		if condition, err := ParseShowIf(field.ShowIf); err == nil {
			for _, name := range condition.Fields {
				l.used[name] = true
			}
		}
	}
	for _, field := range schema.Fields {
		if field.Stored() && !l.used[field.Key] {
			l.warn(Warning{Code: "unused_field", Params: map[string]string{"field": field.Key},
				Message: field.Key + " is never printed: the form asks for it, but the document does not show it"})
		}
	}
	for _, name := range conflictingDeclarations(source) {
		l.warn(Warning{Code: "duplicate_field", Params: map[string]string{"field": name},
			Message: name + " is declared more than once as different kinds of field; only the first counts"})
	}
	for _, tag := range ignoredFillable(source) { // pdf-forms:boxes
		l.warn(Warning{Code: "fillable_ignored", Params: map[string]string{"field": tag[0], "tag": tag[1]},
			Message: "fillable has no effect on <" + tag[1] + "> (" + tag[0] + "): only text, number, date and text area tags can be filled in a PDF"})
	}
	return l.warnings
}

// pdf-forms:boxes
// ignoredFillable finds the fillable attribute on tags that cannot be fillable, as [name, tag] pairs.
func ignoredFillable(source string) [][2]string {
	root, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return nil
	}
	var found [][2]string
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.HasPrefix(n.Data, "reb-") && !rebcompiler.CanBeFillable(strings.TrimPrefix(n.Data, "reb-")) {
			name, fillable := "", false
			for _, attr := range n.Attr {
				switch attr.Key {
				case "name":
					name = strings.TrimSpace(attr.Val)
				case "fillable":
					fillable = !strings.EqualFold(strings.TrimSpace(attr.Val), "false")
				}
			}
			if fillable {
				found = append(found, [2]string{name, n.Data})
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return found
}

// scope is what the dot holds: the root (answers and system values), the rows of one table, or
// something lint cannot know (a photo, a value from with).
type scope struct {
	table   string
	unknown bool
}

type linter struct {
	schema   Schema
	known    map[string]bool
	tables   map[string]map[string]bool
	used     map[string]bool
	reported map[string]bool
	warnings []Warning
}

func (l *linter) warn(w Warning) {
	key := w.Code + "|" + w.Params["field"] + "|" + w.Params["name"] + "|" + w.Params["table"]
	if l.reported[key] {
		return
	}
	l.reported[key] = true
	l.warnings = append(l.warnings, w)
}

func (l *linter) walk(node parse.Node, s scope) {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, child := range n.Nodes {
			l.walk(child, s)
		}
	case *parse.ActionNode:
		l.pipe(n.Pipe, s)
	case *parse.IfNode:
		l.pipe(n.Pipe, s)
		l.walk(n.List, s)
		l.walk(n.ElseList, s)
	case *parse.WithNode:
		l.pipe(n.Pipe, s)
		l.walk(n.List, scope{unknown: true})
		l.walk(n.ElseList, s)
	case *parse.RangeNode:
		l.pipe(n.Pipe, s)
		l.walk(n.List, l.rangeScope(n.Pipe, s))
		l.walk(n.ElseList, s)
	case *parse.TemplateNode:
		l.pipe(n.Pipe, s)
	}
}

// rangeScope is the dot inside {{range .table}}: that table's rows.
func (l *linter) rangeScope(pipe *parse.PipeNode, s scope) scope {
	if s.table == "" && !s.unknown && pipe != nil && len(pipe.Cmds) == 1 && len(pipe.Cmds[0].Args) == 1 {
		if field, ok := pipe.Cmds[0].Args[0].(*parse.FieldNode); ok && len(field.Ident) == 1 {
			if _, isTable := l.tables[field.Ident[0]]; isTable {
				return scope{table: field.Ident[0]}
			}
		}
	}
	return scope{unknown: true}
}

func (l *linter) pipe(pipe *parse.PipeNode, s scope) {
	if pipe == nil {
		return
	}
	for _, command := range pipe.Cmds {
		for _, arg := range command.Args {
			l.arg(arg, s)
		}
	}
}

func (l *linter) arg(node parse.Node, s scope) {
	switch n := node.(type) {
	case *parse.FieldNode:
		l.field(n.Ident, s)
	case *parse.ChainNode:
		l.arg(n.Node, s)
	case *parse.VariableNode:
		// $.name reads the root whatever the dot holds.
		if len(n.Ident) > 1 && n.Ident[0] == "$" {
			l.field(n.Ident[1:], scope{})
		}
	case *parse.PipeNode:
		l.pipe(n, s)
	}
}

// field checks .a.b...: at the root, .Answers.name reads the answer name.
func (l *linter) field(idents []string, s scope) {
	l.name(idents[0], s)
	if len(idents) > 1 && idents[0] == "Answers" && s == (scope{}) {
		l.name(idents[1], s)
	}
}

func (l *linter) name(name string, s scope) {
	switch {
	case s.unknown:
	case s.table != "":
		if !l.tables[s.table][name] {
			l.warn(Warning{Code: "unknown_binding", Params: map[string]string{"name": name, "table": s.table},
				Message: "the rows of " + s.table + " print ." + name + ", which is not one of its columns"})
		}
	default:
		l.used[name] = true
		if !l.known[name] {
			l.warn(Warning{Code: "unknown_binding", Params: map[string]string{"name": name},
				Message: "the template prints ." + name + ", which no field declares and is not a system value"})
		}
	}
}

// conflictingDeclarations finds names declared by <reb-*> tags as different kinds of field. The same
// name declared twice as the same kind is fine: a template may print one answer in two places.
func conflictingDeclarations(source string) []string {
	root, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return nil
	}
	kinds := map[string]map[string]bool{}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.HasPrefix(n.Data, "reb-") {
			name, declared := "", strings.TrimPrefix(n.Data, "reb-")
			for _, attr := range n.Attr {
				switch attr.Key {
				case "name":
					name = strings.TrimSpace(attr.Val)
				case "type":
					if declared == "declare" {
						declared = strings.TrimSpace(attr.Val)
					}
				}
			}
			if declared == "declare" {
				declared = "text"
			}
			if name != "" && declared != "section" {
				kind, ok := fieldKinds[declared]
				if !ok {
					kind = KindText
				}
				if kinds[name] == nil {
					kinds[name] = map[string]bool{}
				}
				kinds[name][kind] = true
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	var conflicts []string
	for name, set := range kinds {
		if len(set) > 1 {
			conflicts = append(conflicts, name)
		}
	}
	sort.Strings(conflicts)
	return conflicts
}
