// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebpdf

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"github.com/vanillaiice/reb/internal/rebdoc"
)

// testdata/form.pdf is testdata/golden/fillable.fillable.html printed by Chromium (headless
// --print-to-pdf); testdata/filled.pdf is form.pdf through Fillable, then filled (the check box
// ticked) and saved incrementally by another program (pypdf), as a PDF reader saves a form.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	pdf, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

func TestFillableTurnsMarkersIntoFields(t *testing.T) {
	pdf, err := Fillable(fixture(t, "form.pdf"), map[string]any{"supplier": "Gulf Steel", "quantity": 12.5, "remarks": true, "crane": true})
	if err != nil {
		t.Fatal(err)
	}
	values, err := Values(pdf)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"supplier": "Gulf Steel", "quantity": "12.5", "delivery": "", "remarks": "", "crane": "true"}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("values = %q, want %q", values, want)
	}
	if states := checkBoxStates(t, pdf); !reflect.DeepEqual(states, []string{"Yes"}) {
		t.Errorf("check box shows %q, want its check mark", states)
	}
	unticked, err := Fillable(fixture(t, "form.pdf"), map[string]any{"crane": "false"})
	if err != nil {
		t.Fatal(err)
	}
	if values, err := Values(unticked); err != nil || values["crane"] != "false" {
		t.Errorf("crane = %q (%v), want false", values["crane"], err)
	}
	if bytes.Contains(pdf, []byte("reb-field:")) {
		t.Error("a marker link is left in the PDF")
	}
}

// checkBoxStates are the appearance states the check box widgets of a PDF show.
func checkBoxStates(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx, err := read(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var states []string
	pageDict, _, _, err := ctx.XRefTable.PageDict(context.Background(), 1, false)
	if err != nil {
		t.Fatal(err)
	}
	annots, _ := ctx.XRefTable.DereferenceArray(pageDict["Annots"])
	for _, entry := range annots {
		if annot, err := ctx.XRefTable.DereferenceDict(entry); err == nil && annot["AS"] != nil {
			states = append(states, string(annot["AS"].(types.Name)))
		}
	}
	return states
}

func TestValuesOfAFilledForm(t *testing.T) {
	values, err := Values(fixture(t, "filled.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"supplier": "Qatar Steel – Ras Laffan", "quantity": "40", "delivery": "2026-11-02",
		"remarks": "Gate 3 only.\nCall 30 min ahead.", "crane": "true"}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("values = %q, want %q", values, want)
	}
}

func TestAnswersKeepTheTemplatesFillableFields(t *testing.T) {
	compiled, err := rebdoc.Compile(`<reb-text name="supplier" label="Supplier" fillable></reb-text>
		<reb-number name="quantity" label="Quantity"></reb-number><reb-textarea name="remarks" label="Remarks" fillable></reb-textarea>
		<reb-checkbox name="crane" label="Crane needed" fillable></reb-checkbox>`)
	if err != nil {
		t.Fatal(err)
	}
	answers, err := Answers(fixture(t, "filled.pdf"), compiled.Fields)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"supplier": "Qatar Steel – Ras Laffan", "remarks": "Gate 3 only.\nCall 30 min ahead.", "crane": true}
	if !reflect.DeepEqual(answers, want) {
		t.Errorf("answers = %v, want %v", answers, want)
	}
}

func TestPDFsWithoutFields(t *testing.T) {
	plain, err := Fillable(fixture(t, "filled.pdf"), nil) // its markers are gone
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, fixture(t, "filled.pdf")) {
		t.Error("a PDF without markers changed")
	}
	if _, err := Values(fixture(t, "form.pdf")); !errors.Is(err, ErrNoForm) {
		t.Errorf("Values of a PDF without a form: %v, want ErrNoForm", err)
	}
	if _, err := Fillable([]byte("not a pdf"), nil); err == nil {
		t.Error("Fillable accepted something that is not a PDF")
	}
	if _, err := Values(nil); err == nil {
		t.Error("Values accepted an empty input")
	}
}
