// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebcompiler

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	// selfClosingRebTag matches a self-closing custom tag such as
	// <reb-text name="a" label="A" />. Attribute values may contain ">" or "/"
	// because quoted values are matched as a whole.
	selfClosingRebTag = regexp.MustCompile(`(?i)<(reb-[a-z0-9-]+)((?:\s+[^\s"'<>/=]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s"'<>]+))?)*)\s*/>`)

	// rebRowOpen matches <reb-row> with or without attributes.
	rebRowOpen  = regexp.MustCompile(`(?i)<reb-row(\s[^>]*)?>`)
	rebRowClose = regexp.MustCompile(`(?i)</reb-row\s*>`)

	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

	// fieldName is what a Go template can address as {{.name}}.
	fieldName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

	// What html.Parse does to Go template syntax, undone after rendering (see Compile).
	actionAsAttribute = regexp.MustCompile(`\{\{([a-zA-Z]+)="" `)
	actionAsEmptyAttr = regexp.MustCompile(`\{\{([^}]+)\}\}=""`)
	action            = regexp.MustCompile(`(?s)\{\{.*?\}\}`)
)

// pdf-forms:boxes (see ../docs/pdf-forms.md in the Rebar folder to remove).
// FillableScheme starts the link a fillable field prints as when a document is rendered fillable:
// "reb-field:NAME", "reb-field:NAME;multiline" for a text area, "reb-field:NAME;checkbox" for a
// checkbox. Chromium keeps it as a link annotation over the field's box, which rebpdf replaces with a
// text field (a checkbox: a check box field).
const (
	FillableScheme    = "reb-field:"
	FillableMultiline = ";multiline"
	FillableCheckbox  = ";checkbox"
)

// FillableMarker is the link target of a fillable <reb-TYPE> field.
func FillableMarker(name, rebType string) string {
	switch rebType {
	case "textarea":
		return FillableScheme + name + FillableMultiline
	case "checkbox":
		return FillableScheme + name + FillableCheckbox
	}
	return FillableScheme + name
}

// fillableTypes are the tags whose answer can be typed (or ticked) into a PDF field (the fillable attribute).
var fillableTypes = map[string]bool{"text": true, "number": true, "date": true, "textarea": true, "checkbox": true}

// CanBeFillable reports whether a <reb-TYPE> tag takes the fillable attribute.
func CanBeFillable(rebType string) bool { return fillableTypes[rebType] }

// fillableCSS sizes the boxes of fillable fields; :where() keeps it below any class the template sets.
const fillableCSS = `:where(.reb-fillable){display:inline-block;min-width:10em;height:1.3em;vertical-align:bottom;border-bottom:1px solid currentColor;color:inherit;text-decoration:none}:where(.reb-fillable-multiline){display:block;width:100%;height:5em;border:1px solid currentColor}:where(.reb-fillable-checkbox){min-width:0;width:1em;height:1em;vertical-align:middle;border:1px solid currentColor}`

// RebFieldSchema is one declared field, as the tag wrote it (the "raw" schema, schemaVersion 1, which
// the Go API and the mobile app read). rebdoc.Normalize turns it into typed fields.
type RebFieldSchema struct {
	Key     string   `json:"key"`
	Type    string   `json:"type"`
	Label   string   `json:"label"`
	Options []string `json:"options,omitempty"`

	// Spec v1.1 attributes (docs/specification.md section 3), omitted when absent.
	Required    bool   `json:"required,omitempty"`
	Help        string `json:"help,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Default     string `json:"default,omitempty"`
	Min         string `json:"min,omitempty"`
	Max         string `json:"max,omitempty"`
	Step        string `json:"step,omitempty"`
	Pattern     string `json:"pattern,omitempty"`
	ShowIf      string `json:"showIf,omitempty"`
	Fillable    bool   `json:"fillable,omitempty"` // pdf-forms:boxes
}

// Error is a compile error a caller can translate: Code names the problem, Params fill the message.
type Error struct {
	Code    string
	Params  map[string]string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Compile takes raw HTML with custom <reb-*> tags, extracts the field schema (never nil),
// and replaces the custom tags with native go html/template bindings.
func Compile(rawHTML string) ([]RebFieldSchema, string, error) {
	// Strip HTML comments entirely
	rawHTML = htmlComment.ReplaceAllString(rawHTML, "")

	// HTML5 ignores "/>" on non-void elements, so a self-closing <reb-text ... />
	// would stay open and swallow its following siblings (which were then
	// dropped from both the schema and the output). Expand them to explicit
	// open/close pairs before parsing.
	rawHTML = selfClosingRebTag.ReplaceAllString(rawHTML, "<$1$2></$1>")

	// Pre-process raw HTML to convert <reb-row> to <tr reb-row> before the HTML5 parser hoists invalid tags out of tables
	rawHTML = rebRowOpen.ReplaceAllString(rawHTML, "<tr reb-row$1>")
	rawHTML = rebRowClose.ReplaceAllString(rawHTML, "</tr>")

	node, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return nil, "", err
	}

	schema := []RebFieldSchema{}
	var walkErr error
	fillableStyled := false // pdf-forms:boxes

	getAttr := func(n *html.Node, key string) string {
		for _, attr := range n.Attr {
			if attr.Key == key {
				return attr.Val
			}
		}
		return ""
	}
	// A boolean attribute: present (required, required="", required="required") unless "false".
	hasFlag := func(n *html.Node, key string) bool {
		for _, attr := range n.Attr {
			if attr.Key == key {
				return !strings.EqualFold(strings.TrimSpace(attr.Val), "false")
			}
		}
		return false
	}

	var walk func(*html.Node, string)
	walk = func(n *html.Node, currentTable string) {
		// Check for reb-row attribute (avoids HTML5 parser hoisting invalid tags out of tables)
		if n.Type == html.ElementNode {
			hasRebRow := false
			for i, attr := range n.Attr {
				if attr.Key == "reb-row" {
					hasRebRow = true
					// Strip the attribute
					n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
					break
				}
			}

			if hasRebRow && currentTable != "" {
				// Inject Go template range around this node
				rangeNode := &html.Node{Type: html.TextNode, Data: `{{range .` + currentTable + `}}`}
				n.Parent.InsertBefore(rangeNode, n)

				endNode := &html.Node{Type: html.TextNode, Data: `{{end}}`}
				if n.NextSibling != nil {
					n.Parent.InsertBefore(endNode, n.NextSibling)
				} else {
					n.Parent.AppendChild(endNode)
				}
			}
		}

		// Check for <reb-*> tags
		if n.Type == html.ElementNode && strings.HasPrefix(n.Data, "reb-") {
			rebType := strings.TrimPrefix(n.Data, "reb-")
			name := getAttr(n, "name")
			label := getAttr(n, "label")

			// The name becomes a Go template field ({{.name}}), so anything else
			// (e.g. "site-name") would compile here and only fail at render time.
			if name != "" && !fieldName.MatchString(name) && walkErr == nil {
				walkErr = &Error{
					Code:    "invalid_field_name",
					Params:  map[string]string{"name": name, "tag": n.Data},
					Message: fmt.Sprintf("invalid field name %q on <%s>: use letters, digits and underscores, starting with a letter or underscore", name, n.Data),
				}
			}

			if name != "" {
				schemaType := rebType
				if rebType == "declare" {
					if t := getAttr(n, "type"); t != "" {
						schemaType = t
					} else {
						schemaType = "text" // Default to text
					}
				}

				field := RebFieldSchema{
					Key:         name,
					Type:        schemaType,
					Label:       label,
					Required:    hasFlag(n, "required"),
					Help:        getAttr(n, "help"),
					Placeholder: getAttr(n, "placeholder"),
					Default:     getAttr(n, "default"),
					Min:         strings.TrimSpace(getAttr(n, "min")),
					Max:         strings.TrimSpace(getAttr(n, "max")),
					Step:        strings.TrimSpace(getAttr(n, "step")),
					Pattern:     getAttr(n, "pattern"),
					ShowIf:      strings.TrimSpace(getAttr(n, "show-if")),
					Fillable:    hasFlag(n, "fillable") && fillableTypes[rebType], // pdf-forms:boxes
				}

				if opts := getAttr(n, "options"); opts != "" {
					// options="High, Medium, Low" must yield "Medium", not " Medium".
					for _, opt := range strings.Split(opts, ",") {
						if opt = strings.TrimSpace(opt); opt != "" {
							field.Options = append(field.Options, opt)
						}
					}
				}

				// Deduplicate: only append if a field with this Key doesn't already exist
				exists := false
				for _, existingField := range schema {
					if existingField.Key == name {
						exists = true
						break
					}
				}
				if !exists {
					schema = append(schema, field)
				}
			}

			// If it's just a declare, we completely remove its visual presence
			// by turning it into an empty text node to avoid breaking tree traversal
			if rebType == "declare" {
				n.Type = html.TextNode
				n.Data = ""
				n.Attr = nil
				n.FirstChild = nil
				n.LastChild = nil
				// Skip the rest of the node processing
				goto WalkChildren
			}

			// If it's a tailwind directive, convert it to the local script tag
			if rebType == "tailwind" {
				n.Type = html.ElementNode
				n.Data = "script"
				n.Attr = []html.Attribute{{Key: "src", Val: "tailwindcss.js"}}
				n.FirstChild = nil
				n.LastChild = nil
				goto WalkChildren
			}

			// If it's a page break, inject standard CSS print breaking styles
			if rebType == "pagebreak" {
				n.Type = html.ElementNode
				n.Data = "div"
				n.Attr = []html.Attribute{{Key: "style", Val: "page-break-after: always; clear: both;"}}
				n.FirstChild = nil
				n.LastChild = nil
				goto WalkChildren
			}

			// A footer or header block is hidden: the PDF client moves it into the page margins
			// (Chromium's footer and header templates; paged.js running elements in a browser).
			if rebType == "footer" || rebType == "header" {
				n.Type = html.ElementNode
				n.Data = "rebar-pdf-" + rebType + "-extract"
				var newAttrs []html.Attribute
				newAttrs = append(newAttrs, html.Attribute{Key: "style", Val: "display:none;"})
				if c := getAttr(n, "class"); c != "" {
					newAttrs = append(newAttrs, html.Attribute{Key: "class", Val: c})
				}
				n.Attr = newAttrs
				goto WalkChildren // Keep its children intact!
			}

			if rebType == "table" {
				// We turn the <reb-table> container into a div and keep all children
				// (the manual table layout) intact so the user can structure it themselves.
				n.Type = html.ElementNode
				n.Data = "div"

				var newAttrs []html.Attribute
				if c := getAttr(n, "class"); c != "" {
					newAttrs = append(newAttrs, html.Attribute{Key: "class", Val: c})
				}
				n.Attr = newAttrs
				currentTable = name // Update context for children
				goto WalkChildren
			}

			class := getAttr(n, "class")
			// pdf-forms:boxes
			fillable := name != "" && hasFlag(n, "fillable") && fillableTypes[rebType]

			// Transform node in-place
			n.Data = "span"
			switch rebType {
			case "photogrid", "attachments", "textarea":
				n.Data = "div"
			case "signature":
				n.Data = "img"
			}

			n.Attr = nil
			if class != "" {
				n.Attr = append(n.Attr, html.Attribute{Key: "class", Val: class})
			}
			// Textarea content from Quill comes as HTML (e.g. <p>...</p>). Inside table cells,
			// the text won't wrap without explicit word-wrap styles, causing PDF text to overflow.
			if rebType == "textarea" {
				n.Attr = append(n.Attr, html.Attribute{Key: "style", Val: "word-wrap:break-word;overflow-wrap:break-word;white-space:normal;"})
			}
			n.FirstChild = nil
			n.LastChild = nil

			// Construct dynamic content based on the rebType
			if rebType == "photogrid" || rebType == "attachments" {
				n.AppendChild(&html.Node{Type: html.TextNode, Data: `{{range .` + name + `}}`})
				img := &html.Node{
					Type: html.ElementNode,
					Data: "img",
					Attr: []html.Attribute{
						{Key: "src", Val: `{{.}}`},
						{Key: "class", Val: "w-full object-cover rounded shadow-sm"},
					},
				}
				n.AppendChild(img)
				n.AppendChild(&html.Node{Type: html.TextNode, Data: `{{end}}`})
			} else if rebType == "signature" {
				// Base signature styles
				sigClass := "h-16 object-contain"
				if class != "" {
					sigClass = class
				}

				// Ensure src uses Go template
				n.Attr = []html.Attribute{
					{Key: "src", Val: `{{.` + name + `}}`},
					{Key: "class", Val: sigClass},
				}
			} else if name != "" {
				// Generic text/number/date
				if rebType == "textarea" {
					n.AppendChild(&html.Node{Type: html.TextNode, Data: `{{.` + name + ` | safeHTML}}`})
				} else {
					n.AppendChild(&html.Node{Type: html.TextNode, Data: `{{.` + name + `}}`})
				}
			}

			// pdf-forms:boxes: a fillable field prints, when the document is rendered fillable, an empty box linking
			// to its marker instead of the answer; rebpdf turns the link into a PDF field.
			if fillable {
				if !fillableStyled {
					fillableStyled = true
					style := &html.Node{Type: html.ElementNode, Data: "style", DataAtom: atom.Style}
					style.AppendChild(&html.Node{Type: html.RawNode, Data: fillableCSS})
					n.Parent.InsertBefore(style, n)
				}
				boxClass := "reb-fillable"
				switch rebType {
				case "textarea":
					boxClass += " reb-fillable-multiline"
				case "checkbox":
					boxClass += " reb-fillable-checkbox"
				}
				if class != "" {
					boxClass += " " + class
				}
				box := &html.Node{Type: html.ElementNode, Data: "a", DataAtom: atom.A, Attr: []html.Attribute{
					{Key: "href", Val: FillableMarker(name, rebType)},
					{Key: "class", Val: boxClass},
				}}
				n.Parent.InsertBefore(&html.Node{Type: html.TextNode, Data: `{{if $.Fillable}}`}, n)
				n.Parent.InsertBefore(box, n)
				n.Parent.InsertBefore(&html.Node{Type: html.TextNode, Data: `{{else}}`}, n)
				end := &html.Node{Type: html.TextNode, Data: `{{end}}`}
				if n.NextSibling != nil {
					n.Parent.InsertBefore(end, n.NextSibling)
				} else {
					n.Parent.AppendChild(end)
				}
			}
		}

	WalkChildren:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, currentTable)
		}
	}

	walk(node, "")
	if walkErr != nil {
		return nil, "", walkErr
	}

	var buf bytes.Buffer
	if hasBodyTag(rawHTML) {
		if err := html.Render(&buf, node); err != nil {
			return nil, "", err
		}
	} else {
		// html.Parse wraps snippets in <html><head></head><body> ... </body>. The
		// user didn't write a <body>, so emit the snippet without the wrapper. A
		// leading <style>, <link> or <meta> is moved into <head> by the parser, so
		// render the head's children before the body's instead of dropping them.
		for _, section := range []atom.Atom{atom.Head, atom.Body} {
			if el := findElement(node, section); el != nil {
				for c := el.FirstChild; c != nil; c = c.NextSibling {
					if err := html.Render(&buf, c); err != nil {
						return nil, "", err
					}
				}
			}
		}
	}

	outHTML := buf.String()

	// Fix html.Parse mangling of Go template syntaxes inside HTML attributes
	// <div {{if="" .cond}}> becomes <div {{if .cond}}>
	outHTML = actionAsAttribute.ReplaceAllString(outHTML, "{{$1 ")

	// <div {{end}}=""> becomes <div {{end}}>
	outHTML = actionAsEmptyAttr.ReplaceAllString(outHTML, "{{$1}}")

	// Fix html.Parse escaping quotes inside Go template directives in text nodes
	// {{if eq .severity &#34;C&#34;}} becomes {{if eq .severity "C"}}, also in an action written over
	// several lines ({{sumColumn .rows "amount"\n  | formatMoney "QAR" 2}}).
	outHTML = action.ReplaceAllStringFunc(outHTML, html.UnescapeString)

	return schema, strings.TrimSpace(outHTML), nil
}

// hasBodyTag reports whether the source has a <body> tag of its own (not "<body" inside an
// attribute, a comment or a <style>).
func hasBodyTag(source string) bool {
	z := html.NewTokenizer(strings.NewReader(source))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken, html.SelfClosingTagToken:
			if name, _ := z.TagName(); string(name) == "body" {
				return true
			}
		}
	}
}

// findElement returns the first element with the given atom in document order.
func findElement(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, a); found != nil {
			return found
		}
	}
	return nil
}
