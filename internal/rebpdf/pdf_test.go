// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebpdf

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/vanillaiice/reb/internal/rebdoc"
)

// testdata/form.pdf is testdata/golden/fillable.fillable.html printed by Chromium (headless
// --print-to-pdf); testdata/filled.pdf is form.pdf through Fillable, then filled and saved
// incrementally by another program (pypdf), as a PDF reader saves a form.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	pdf, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

func TestFillableTurnsMarkersIntoTextFields(t *testing.T) {
	pdf, err := Fillable(fixture(t, "form.pdf"), map[string]any{"supplier": "Gulf Steel", "quantity": 12.5, "remarks": true})
	if err != nil {
		t.Fatal(err)
	}
	values, err := Values(pdf)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"supplier": "Gulf Steel", "quantity": "12.5", "delivery": "", "remarks": ""}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("values = %q, want %q", values, want)
	}
	if bytes.Contains(pdf, []byte("reb-field:")) {
		t.Error("a marker link is left in the PDF")
	}
}

func TestValuesOfAFilledForm(t *testing.T) {
	values, err := Values(fixture(t, "filled.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"supplier": "Qatar Steel – Ras Laffan", "quantity": "40", "delivery": "2026-11-02",
		"remarks": "Gate 3 only.\nCall 30 min ahead."}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("values = %q, want %q", values, want)
	}
}

func TestAnswersKeepTheTemplatesFillableFields(t *testing.T) {
	compiled, err := rebdoc.Compile(`<reb-text name="supplier" label="Supplier" fillable></reb-text>
		<reb-number name="quantity" label="Quantity"></reb-number><reb-textarea name="remarks" label="Remarks" fillable></reb-textarea>`)
	if err != nil {
		t.Fatal(err)
	}
	answers, err := Answers(fixture(t, "filled.pdf"), compiled.Fields)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"supplier": "Qatar Steel – Ras Laffan", "remarks": "Gate 3 only.\nCall 30 min ahead."}
	if !reflect.DeepEqual(answers, want) {
		t.Errorf("answers = %q, want %q", answers, want)
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
