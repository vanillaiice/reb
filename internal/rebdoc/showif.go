// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"fmt"
	"strconv"
	"strings"
)

// show-if conditions (spec section 3.3): a field is shown only while its condition holds. The
// language is small on purpose, so every form evaluates it the same way (the cases in
// testdata/show_if_cases.json are run by each evaluator):
//
//	expression := or
//	or         := and ("or" and)*
//	and        := not ("and" not)*
//	not        := "not" not | comparison
//	comparison := operand (("==" | "!=") operand)?
//	operand    := field_name | 'text' | "text" | number | "(" expression ")"
//
// A field alone is true when answered: non-blank text other than "false", a ticked checkbox, a
// non-empty list, a number other than 0. == and != compare the answers as text (a checkbox reads
// "true" or "false", a missing answer ""), trimmed.

// ShowIf is a parsed condition.
type ShowIf struct {
	root   node
	Fields []string // the field names it reads, in order of appearance
}

// ParseShowIf parses a condition; an empty one is always true.
func ParseShowIf(expression string) (*ShowIf, error) {
	tokens, err := lex(expression)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	condition := &ShowIf{}
	if len(tokens) == 0 {
		return condition, nil
	}
	root, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.tokens) {
		return nil, fmt.Errorf("unexpected %q", p.tokens[p.pos].text)
	}
	condition.root = root
	condition.Fields = p.fields
	return condition, nil
}

// Holds evaluates the condition against the answers.
func (s *ShowIf) Holds(answers map[string]any) bool {
	if s == nil || s.root == nil {
		return true
	}
	return truthy(s.root.eval(answers))
}

type tokenKind int

const (
	tName tokenKind = iota
	tText
	tNumber
	tOp // == != ( )
	tKeyword
)

type token struct {
	kind tokenKind
	text string
}

func lex(expression string) ([]token, error) {
	var tokens []token
	for i := 0; i < len(expression); {
		ch := expression[i]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			i++
		case ch == '(' || ch == ')':
			tokens = append(tokens, token{tOp, string(ch)})
			i++
		case (ch == '=' || ch == '!') && i+1 < len(expression) && expression[i+1] == '=':
			tokens = append(tokens, token{tOp, expression[i : i+2]})
			i += 2
		case ch == '\'' || ch == '"':
			end := strings.IndexByte(expression[i+1:], ch)
			if end < 0 {
				return nil, fmt.Errorf("unterminated text starting at %d", i+1)
			}
			tokens = append(tokens, token{tText, expression[i+1 : i+1+end]})
			i += end + 2
		case ch >= '0' && ch <= '9':
			j := i
			for j < len(expression) && (expression[j] == '.' || (expression[j] >= '0' && expression[j] <= '9')) {
				j++
			}
			tokens = append(tokens, token{tNumber, expression[i:j]})
			i = j
		case ch == '_' || (ch|0x20 >= 'a' && ch|0x20 <= 'z'):
			j := i
			for j < len(expression) && (expression[j] == '_' || (expression[j]|0x20 >= 'a' && expression[j]|0x20 <= 'z') || (expression[j] >= '0' && expression[j] <= '9')) {
				j++
			}
			word := expression[i:j]
			if word == "and" || word == "or" || word == "not" {
				tokens = append(tokens, token{tKeyword, word})
			} else {
				tokens = append(tokens, token{tName, word})
			}
			i = j
		default:
			return nil, fmt.Errorf("unexpected %q at %d", string(ch), i+1)
		}
	}
	return tokens, nil
}

type parser struct {
	tokens []token
	pos    int
	fields []string
}

func (p *parser) peek(kind tokenKind, text string) bool {
	return p.pos < len(p.tokens) && p.tokens[p.pos].kind == kind && p.tokens[p.pos].text == text
}

func (p *parser) or() (node, error) {
	left, err := p.and()
	for err == nil && p.peek(tKeyword, "or") {
		p.pos++
		var right node
		if right, err = p.and(); err == nil {
			left = binary{"or", left, right}
		}
	}
	return left, err
}

func (p *parser) and() (node, error) {
	left, err := p.not()
	for err == nil && p.peek(tKeyword, "and") {
		p.pos++
		var right node
		if right, err = p.not(); err == nil {
			left = binary{"and", left, right}
		}
	}
	return left, err
}

func (p *parser) not() (node, error) {
	if p.peek(tKeyword, "not") {
		p.pos++
		operand, err := p.not()
		return negation{operand}, err
	}
	return p.comparison()
}

func (p *parser) comparison() (node, error) {
	left, err := p.operand()
	if err != nil {
		return nil, err
	}
	if p.peek(tOp, "==") || p.peek(tOp, "!=") {
		op := p.tokens[p.pos].text
		p.pos++
		right, err := p.operand()
		if err != nil {
			return nil, err
		}
		return binary{op, left, right}, nil
	}
	return left, nil
}

func (p *parser) operand() (node, error) {
	if p.pos >= len(p.tokens) {
		return nil, fmt.Errorf("the condition ends too early")
	}
	t := p.tokens[p.pos]
	p.pos++
	switch {
	case t.kind == tName:
		p.fields = append(p.fields, t.text)
		return name(t.text), nil
	case t.kind == tText || t.kind == tNumber:
		return literal(t.text), nil
	case t.kind == tOp && t.text == "(":
		inner, err := p.or()
		if err != nil {
			return nil, err
		}
		if !p.peek(tOp, ")") {
			return nil, fmt.Errorf("missing )")
		}
		p.pos++
		return inner, nil
	}
	return nil, fmt.Errorf("unexpected %q", t.text)
}

type node interface{ eval(map[string]any) any }

type name string
type literal string
type negation struct{ operand node }
type binary struct {
	op          string
	left, right node
}

func (n name) eval(answers map[string]any) any     { return answers[string(n)] }
func (l literal) eval(map[string]any) any          { return string(l) }
func (n negation) eval(answers map[string]any) any { return !truthy(n.operand.eval(answers)) }

func (b binary) eval(answers map[string]any) any {
	switch b.op {
	case "and":
		return truthy(b.left.eval(answers)) && truthy(b.right.eval(answers))
	case "or":
		return truthy(b.left.eval(answers)) || truthy(b.right.eval(answers))
	case "==":
		return text(b.left.eval(answers)) == text(b.right.eval(answers))
	default: // !=
		return text(b.left.eval(answers)) != text(b.right.eval(answers))
	}
}

func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		t := strings.TrimSpace(v)
		return t != "" && t != "false"
	case []any:
		return len(v) > 0
	}
	return true
}

func text(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return strings.TrimSpace(v)
	}
	return fmt.Sprint(value)
}
