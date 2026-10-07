// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

// pdf-forms:fields: this whole package (see ../docs/pdf-forms.md in the Rebar folder to remove).
//
// Package rebpdf turns a document printed in fillable mode into a PDF form, and reads the answers
// typed into one (specification section 4.5).
//
// Chromium, which prints every Rebar PDF, flattens form inputs but keeps links: a fillable field
// renders as an empty link to "reb-field:NAME" (rebcompiler.FillableMarker), which the PDF holds as a link annotation
// over the field's box. Fillable replaces each such annotation with a text field named NAME at the
// same place, or a check box field for a checkbox; Values reads the fields back.
package rebpdf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"github.com/vanillaiice/reb/internal/rebcompiler"
	"github.com/vanillaiice/reb/internal/rebdoc"
)

// Field flags (PDF 32000-1, 12.7.4.2 and 12.7.4.3).
const (
	flagMultiline  = 1 << 12
	flagRadio      = 1 << 15
	flagPushbutton = 1 << 16
)

// A check box is on in its "Yes" appearance state and value, off in "Off".
const checkedState, uncheckedState = types.Name("Yes"), types.Name("Off")

func read(pdf []byte) (*model.Context, error) {
	// Stateless: pdfcpu would otherwise create a configuration folder in the user's home.
	conf := model.NewStatelessConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(pdf), conf)
	if err != nil {
		return nil, fmt.Errorf("not a readable PDF: %w", err)
	}
	if err := ctx.EnsurePageCount(); err != nil {
		return nil, fmt.Errorf("not a readable PDF: %w", err)
	}
	return ctx, nil
}

// Fillable replaces the fillable-field markers of a printed PDF with text fields, filled with the
// given answers (by field name; text as it is, numbers as written, anything else left empty), and
// checkbox markers with check boxes, ticked when the answer is. Boxes of the same name become
// widgets of one field, so they share the value. A PDF without markers comes back unchanged.
func Fillable(pdf []byte, answers map[string]any) ([]byte, error) {
	ctx, err := read(pdf)
	if err != nil {
		return nil, err
	}
	xref := ctx.XRefTable

	type field struct {
		kind    string // "", rebcompiler.FillableMultiline or rebcompiler.FillableCheckbox
		widgets types.Array
		ref     *types.IndirectRef
	}
	fields := map[string]*field{}
	var order []string

	for page := 1; page <= xref.PageCount; page++ {
		pageDict, pageRef, _, err := xref.PageDict(context.Background(), page, false)
		if err != nil || pageDict == nil {
			continue
		}
		annots, err := xref.DereferenceArray(pageDict["Annots"])
		if err != nil {
			continue
		}
		for _, entry := range annots {
			ref, ok := entry.(types.IndirectRef)
			if !ok {
				continue // Chromium writes annotations as indirect objects
			}
			annot, err := xref.DereferenceDict(ref)
			if err != nil || annot == nil {
				continue
			}
			name, kind, ok := marker(xref, annot)
			if !ok {
				continue
			}
			f := fields[name]
			if f == nil {
				f = &field{kind: kind} // a name has one type, so its first box tells the kind
				fields[name] = f
				order = append(order, name)
				if f.ref, err = xref.IndRefForNewObject(types.Dict{}); err != nil {
					return nil, err
				}
			}
			f.widgets = append(f.widgets, ref)

			// The link annotation becomes a widget of the field, in place (the page and the
			// structure tree keep pointing at it).
			for _, key := range []string{"A", "Border", "H", "PA", "QuadPoints", "Dest", "StructParent"} {
				delete(annot, key)
			}
			annot["Subtype"] = types.Name("Widget")
			annot["F"] = types.Integer(4) // printed
			annot["P"] = *pageRef
			annot["Parent"] = *f.ref
			annot["MK"] = types.Dict{}
			if f.kind == rebcompiler.FillableCheckbox {
				if err := checkBoxWidget(xref, annot, ticked(answers[name])); err != nil {
					return nil, err
				}
			}
		}
	}
	if len(order) == 0 {
		return pdf, nil
	}

	var refs types.Array
	for _, name := range order {
		f := fields[name]
		d, err := xref.DereferenceDict(*f.ref)
		if err != nil {
			return nil, err
		}
		d["T"] = types.StringLiteral(types.EncodeUTF16String(name))
		d["Kids"] = f.widgets
		if f.kind == rebcompiler.FillableCheckbox {
			d["FT"] = types.Name("Btn")
			d["DA"] = types.StringLiteral("/ZaDb 0 Tf 0 g")
			d["V"] = uncheckedState
			if ticked(answers[name]) {
				d["V"] = checkedState
			}
		} else {
			d["FT"] = types.Name("Tx")
			d["DA"] = types.StringLiteral("/Helv 0 Tf 0 g")
			if f.kind == rebcompiler.FillableMultiline {
				d["Ff"] = types.Integer(flagMultiline)
			}
			if value := text(answers[name]); value != "" {
				d["V"] = types.StringLiteral(types.EncodeUTF16String(value))
			}
		}
		refs = append(refs, *f.ref)
	}

	helvetica, err := xref.IndRefForNewObject(types.Dict{
		"Type": types.Name("Font"), "Subtype": types.Name("Type1"), "BaseFont": types.Name("Helvetica"),
		"Encoding": types.Name("WinAnsiEncoding"),
	})
	if err != nil {
		return nil, err
	}
	zapfDingbats, err := xref.IndRefForNewObject(zapfDingbatsFont())
	if err != nil {
		return nil, err
	}
	form, err := xref.IndRefForNewObject(types.Dict{
		"Fields":          refs,
		"NeedAppearances": types.Boolean(true), // the viewer draws the values
		"DA":              types.StringLiteral("/Helv 0 Tf 0 g"),
		"DR":              types.Dict{"Font": types.Dict{"Helv": *helvetica, "ZaDb": *zapfDingbats}},
	})
	if err != nil {
		return nil, err
	}
	catalog, err := xref.Catalog()
	if err != nil {
		return nil, err
	}
	catalog["AcroForm"] = *form

	var out bytes.Buffer
	if err := api.WriteContext(context.Background(), ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// marker reads "reb-field:NAME[;multiline|;checkbox]" from a link annotation: the name, and the
// suffix as the kind.
func marker(xref *model.XRefTable, annot types.Dict) (string, string, bool) {
	if subtype, _ := annot["Subtype"].(types.Name); subtype != "Link" {
		return "", "", false
	}
	action, err := xref.DereferenceDict(annot["A"])
	if err != nil || action == nil {
		return "", "", false
	}
	uri, err := xref.DereferenceStringOrHexLiteral(action["URI"], model.V10, nil)
	if err != nil || !strings.HasPrefix(uri, rebcompiler.FillableScheme) {
		return "", "", false
	}
	name := strings.TrimPrefix(uri, rebcompiler.FillableScheme)
	kind := ""
	for _, suffix := range []string{rebcompiler.FillableMultiline, rebcompiler.FillableCheckbox} {
		if rest, ok := strings.CutSuffix(name, suffix); ok {
			name, kind = rest, suffix
			break
		}
	}
	if name == "" {
		return "", "", false
	}
	return name, kind, true
}

func zapfDingbatsFont() types.Dict {
	return types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("Type1"), "BaseFont": types.Name("ZapfDingbats")}
}

// checkBoxWidget gives a widget the two appearances of a check box, a check mark (ZapfDingbats "4")
// centred in its box and nothing, and shows the one for ticked. Readers draw check boxes from these,
// not from NeedAppearances.
func checkBoxWidget(xref *model.XRefTable, annot types.Dict, ticked bool) error {
	rect, err := xref.DereferenceArray(annot["Rect"])
	if err != nil || len(rect) != 4 {
		return fmt.Errorf("a checkbox box has no rectangle")
	}
	var r [4]float64
	for i, v := range rect {
		switch n := v.(type) {
		case types.Integer:
			r[i] = float64(n)
		case types.Float:
			r[i] = float64(n)
		default:
			return fmt.Errorf("a checkbox box has no rectangle")
		}
	}
	w, h := math.Abs(r[2]-r[0]), math.Abs(r[3]-r[1])
	side := math.Min(w, h)
	// The proportions of a check mark in a square box, as pdfcpu draws one.
	size, x, y := side*14.532/18, (w-side)/2+side*2.853/18, (h-side)/2+side*4.081/18
	appearance := func(content string) (*types.IndirectRef, error) {
		sd, err := xref.NewStreamDictForBuf([]byte(content))
		if err != nil {
			return nil, err
		}
		sd.InsertName("Type", "XObject")
		sd.InsertName("Subtype", "Form")
		sd.Insert("BBox", types.NewNumberArray(0, 0, w, h))
		sd.Insert("Resources", types.Dict{"Font": types.Dict{"ZaDb": zapfDingbatsFont()}})
		if err := sd.Encode(); err != nil {
			return nil, err
		}
		return xref.IndRefForNewObject(*sd)
	}
	on, err := appearance(fmt.Sprintf("q 0 g BT /ZaDb %.3f Tf %.3f %.3f Td (4) Tj ET Q", size, x, y))
	if err != nil {
		return err
	}
	off, err := appearance("")
	if err != nil {
		return err
	}
	annot["AP"] = types.Dict{"N": types.Dict{string(checkedState): *on, string(uncheckedState): *off}}
	annot["MK"] = types.Dict{"CA": types.StringLiteral("4")}
	annot["AS"] = uncheckedState
	if ticked {
		annot["AS"] = checkedState
	}
	return nil
}

// ticked is a checkbox answer as rebdoc reads one: true, 1, "true", "1" or "on".
func ticked(answer any) bool {
	switch v := answer.(type) {
	case bool:
		return v
	case float64:
		return v == 1
	case json.Number:
		return v.String() == "1"
	case string:
		return v == "true" || v == "1" || v == "on"
	}
	return false
}

func text(answer any) string {
	switch v := answer.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	}
	return ""
}

// ErrNoForm is a PDF without form fields.
var ErrNoForm = errors.New("the PDF has no form fields")

// Values reads a PDF form's text fields and check boxes: their full names (parent.child) with the
// values typed in, empty ones included, and "true" or "false" for a check box. Other kinds of field
// (push buttons, radio buttons, choices, signatures) are left out.
func Values(pdf []byte) (map[string]string, error) {
	ctx, err := read(pdf)
	if err != nil {
		return nil, err
	}
	xref := ctx.XRefTable
	catalog, err := xref.Catalog()
	if err != nil {
		return nil, err
	}
	form, err := xref.DereferenceDict(catalog["AcroForm"])
	if err != nil || form == nil {
		return nil, ErrNoForm
	}
	roots, err := xref.DereferenceArray(form["Fields"])
	if err != nil || len(roots) == 0 {
		return nil, ErrNoForm
	}

	values := map[string]string{}
	seen := map[int]bool{}
	var visit func(obj types.Object, prefix, inheritedType string, inheritedValue types.Object)
	visit = func(obj types.Object, prefix, inheritedType string, inheritedValue types.Object) {
		if ref, ok := obj.(types.IndirectRef); ok {
			if seen[ref.ObjectNumber.Value()] {
				return
			}
			seen[ref.ObjectNumber.Value()] = true
		}
		d, err := xref.DereferenceDict(obj)
		if err != nil || d == nil {
			return
		}
		name := prefix
		if t, err := xref.DereferenceStringOrHexLiteral(d["T"], model.V10, nil); err == nil && t != "" {
			if name != "" {
				name += "."
			}
			name += t
		}
		fieldType := inheritedType
		if ft, ok := d["FT"].(types.Name); ok {
			fieldType = string(ft)
		}
		value := inheritedValue
		if v, ok := d["V"]; ok {
			value = v
		}
		kids, _ := xref.DereferenceArray(d["Kids"])
		named := false
		for _, kid := range kids {
			if kd, err := xref.DereferenceDict(kid); err == nil && kd["T"] != nil {
				named = true
				break
			}
		}
		if named {
			for _, kid := range kids {
				visit(kid, name, fieldType, value)
			}
			return
		}
		// A terminal field (its kids, if any, are only widgets).
		if name == "" {
			return
		}
		if fieldType == "Btn" {
			flags, _ := d["Ff"].(types.Integer)
			if flags&(flagRadio|flagPushbutton) != 0 {
				return
			}
			// Any state but Off is on. The state is a name, but some writers (pypdf) save it as text.
			state := ""
			if n, ok := value.(types.Name); ok {
				state = string(n)
			} else if value != nil {
				state, _ = xref.DereferenceStringOrHexLiteral(value, model.V10, nil)
				state = strings.TrimPrefix(state, "/")
			}
			values[name] = strconv.FormatBool(state != "" && state != string(uncheckedState))
			return
		}
		if fieldType != "Tx" {
			return
		}
		text := ""
		if value != nil {
			text, _ = xref.DereferenceStringOrHexLiteral(value, model.V10, nil)
		}
		values[name] = text
	}
	for _, root := range roots {
		visit(root, "", "", nil)
	}
	return values, nil
}

// Answers are the values of a PDF form's fields that a template declares fillable: what a document
// takes back from a filled PDF, text as typed and a checkbox as true or false. Fields the form has
// but the schema does not declare fillable are left out; an empty field is an empty answer.
func Answers(pdf []byte, schema rebdoc.Schema) (map[string]any, error) {
	values, err := Values(pdf)
	if err != nil {
		return nil, err
	}
	answers := map[string]any{}
	for _, field := range schema.Fields {
		if value, ok := values[field.Key]; ok && field.Fillable {
			if field.Kind == rebdoc.KindCheckbox {
				answers[field.Key] = value == "true"
			} else {
				answers[field.Key] = value
			}
		}
	}
	return answers, nil
}
