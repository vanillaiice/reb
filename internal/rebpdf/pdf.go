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
// same place; Values reads the fields back.
package rebpdf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"github.com/vanillaiice/reb/internal/rebcompiler"
	"github.com/vanillaiice/reb/internal/rebdoc"
)

// Text field flags (PDF 32000-1, 12.7.4.3).
const flagMultiline = 1 << 12

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
// given answers (by field name; text as it is, numbers as written, anything else left empty). Boxes of the same name become widgets of one field, so they share
// the value. A PDF without markers comes back unchanged.
func Fillable(pdf []byte, answers map[string]any) ([]byte, error) {
	ctx, err := read(pdf)
	if err != nil {
		return nil, err
	}
	xref := ctx.XRefTable

	type field struct {
		multiLine bool
		widgets   types.Array
		ref       *types.IndirectRef
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
			name, multiLine, ok := marker(xref, annot)
			if !ok {
				continue
			}
			f := fields[name]
			if f == nil {
				f = &field{}
				fields[name] = f
				order = append(order, name)
				if f.ref, err = xref.IndRefForNewObject(types.Dict{}); err != nil {
					return nil, err
				}
			}
			f.multiLine = f.multiLine || multiLine
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
		d["FT"] = types.Name("Tx")
		d["T"] = types.StringLiteral(types.EncodeUTF16String(name))
		d["Kids"] = f.widgets
		d["DA"] = types.StringLiteral("/Helv 0 Tf 0 g")
		if f.multiLine {
			d["Ff"] = types.Integer(flagMultiline)
		}
		if value := text(answers[name]); value != "" {
			d["V"] = types.StringLiteral(types.EncodeUTF16String(value))
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
	form, err := xref.IndRefForNewObject(types.Dict{
		"Fields":          refs,
		"NeedAppearances": types.Boolean(true), // the viewer draws the values
		"DA":              types.StringLiteral("/Helv 0 Tf 0 g"),
		"DR":              types.Dict{"Font": types.Dict{"Helv": *helvetica}},
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

// marker reads "reb-field:NAME[;multiline]" from a link annotation.
func marker(xref *model.XRefTable, annot types.Dict) (string, bool, bool) {
	if subtype, _ := annot["Subtype"].(types.Name); subtype != "Link" {
		return "", false, false
	}
	action, err := xref.DereferenceDict(annot["A"])
	if err != nil || action == nil {
		return "", false, false
	}
	uri, err := xref.DereferenceStringOrHexLiteral(action["URI"], model.V10, nil)
	if err != nil || !strings.HasPrefix(uri, rebcompiler.FillableScheme) {
		return "", false, false
	}
	name, multiLine := strings.CutSuffix(strings.TrimPrefix(uri, rebcompiler.FillableScheme), rebcompiler.FillableMultiline)
	if name == "" {
		return "", false, false
	}
	return name, multiLine, true
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

// Values reads a PDF form's text fields: their full names (parent.child) with the values typed in,
// empty ones included. Other kinds of field are left out.
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
		if fieldType != "Tx" || name == "" {
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
// takes back from a filled PDF. Fields the form has but the schema does not declare fillable are
// left out; an empty field is an empty answer.
func Answers(pdf []byte, schema rebdoc.Schema) (map[string]any, error) {
	values, err := Values(pdf)
	if err != nil {
		return nil, err
	}
	answers := map[string]any{}
	for _, field := range schema.Fields {
		if value, ok := values[field.Key]; ok && field.Fillable {
			answers[field.Key] = value
		}
	}
	return answers, nil
}
