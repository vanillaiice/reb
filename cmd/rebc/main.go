// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

// Command rebc is the .reb pipeline for server-side callers (Rebar calls it from Rails): the same
// compiler and renderer that Rebar Studio runs as WebAssembly, behind a JSON-in, JSON-out command
// line so any language can call it.
//
//	rebc compile    {"reb"}                                            -> {"schema", "fields", "html", "engineVersion", "warnings"}
//	rebc prepare    {"fields" | "schema", "answers"}                   -> {"answers", "errors"}
//	rebc render     {"html", "system", "answers", "assets", "fields"? | "schema"?, "fillable"?} -> {"html"}
//	rebc normalize  {"schema"}                                         -> {"schemaVersion", "fields"}
//	rebc fillable   {"pdf", "answers"?}                                -> {"pdf"}
//	rebc pdf-answers {"pdf", "fields" | "schema"}                      -> {"answers"}
//	rebc version                                                       -> {"version"}
//
// "schema" is the compiler's raw schema, "fields" the normalized one (rebdoc.Schema). PDFs travel
// base64-encoded: fillable turns a document printed with "fillable": true into a PDF form,
// pdf-answers reads the template's fillable fields back from a filled one. On failure rebc
// exits with status 1 and prints {"error", "code"?, "params"?}. Input always arrives on stdin, never
// as arguments.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/vanillaiice/reb"
	"github.com/vanillaiice/reb/internal/rebcompiler"
	"github.com/vanillaiice/reb/internal/rebdoc"
	"github.com/vanillaiice/reb/internal/rebpdf"
	"github.com/vanillaiice/reb/internal/rebrender"
)

// maxInput bounds what rebc reads, so a runaway caller cannot exhaust memory.
const maxInput = 32 << 20

func main() {
	if len(os.Args) != 2 {
		fail(errors.New("usage: rebc compile|prepare|render|normalize|fillable|pdf-answers|version < input.json"))
	}
	input, err := io.ReadAll(io.LimitReader(os.Stdin, maxInput+1))
	if err != nil {
		fail(err)
	}
	if len(input) > maxInput {
		fail(fmt.Errorf("input larger than %d bytes", maxInput))
	}

	output, err := run(os.Args[1], input)
	if err != nil {
		fail(err)
	}
	if _, err := os.Stdout.Write(output); err != nil {
		os.Exit(1)
	}
}

func fail(err error) {
	os.Stdout.Write(errorJSON(err))
	os.Exit(1)
}

// errorJSON is what rebc prints on failure: {"error", "code"?, "params"?}.
func errorJSON(err error) []byte {
	var coded *rebdoc.Error
	if errors.As(err, &coded) {
		out, _ := json.Marshal(coded)
		return out
	}
	out, _ := json.Marshal(map[string]string{"error": err.Error()})
	return out
}

func run(command string, input []byte) ([]byte, error) {
	switch command {
	case "compile":
		return compile(input)
	case "prepare":
		return prepare(input)
	case "render":
		return render(input)
	case "normalize":
		return normalize(input)
	case "fillable": // pdf-forms:fields
		return fillable(input)
	case "pdf-answers": // pdf-forms:fields
		return pdfAnswers(input)
	case "version":
		return json.Marshal(map[string]string{"version": reb.Version})
	default:
		return nil, fmt.Errorf("unknown command %q (use compile, prepare, render, normalize, fillable, pdf-answers or version)", command)
	}
}

func decode(input []byte, into any) error {
	if err := json.Unmarshal(input, into); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	return nil
}

func compile(input []byte) ([]byte, error) {
	var in struct {
		Reb string `json:"reb"`
	}
	if err := decode(input, &in); err != nil {
		return nil, err
	}
	compiled, err := rebdoc.Compile(in.Reb)
	if err != nil {
		return nil, err
	}
	return json.Marshal(compiled)
}

func prepare(input []byte) ([]byte, error) {
	var in struct {
		rebdoc.SchemaInput
		Answers map[string]any `json:"answers"`
	}
	if err := decode(input, &in); err != nil {
		return nil, err
	}
	schema := in.Resolve()
	if schema == nil {
		return nil, errors.New("prepare needs fields or schema")
	}
	return json.Marshal(rebdoc.Prepare(*schema, in.Answers))
}

func render(input []byte) ([]byte, error) {
	var in struct {
		rebdoc.SchemaInput
		HTML     string            `json:"html"`
		System   map[string]any    `json:"system"`
		Answers  map[string]any    `json:"answers"`
		Assets   map[string]string `json:"assets"`
		Fillable bool              `json:"fillable"` // pdf-forms:boxes
	}
	if err := decode(input, &in); err != nil {
		return nil, err
	}
	html, err := rebrender.CompileHTML(in.HTML, rebdoc.BuildContext(rebdoc.WithFillable(in.System, in.Fillable), in.Answers, in.Assets, in.Resolve()))
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"html": html})
}

func normalize(input []byte) ([]byte, error) {
	var in struct {
		Schema []rebcompiler.RebFieldSchema `json:"schema"`
	}
	if err := decode(input, &in); err != nil {
		return nil, err
	}
	return json.Marshal(rebdoc.Normalize(in.Schema))
}

// pdf-forms:fields
func fillable(input []byte) ([]byte, error) {
	var in struct {
		PDF     []byte         `json:"pdf"`
		Answers map[string]any `json:"answers"`
	}
	if err := decode(input, &in); err != nil {
		return nil, err
	}
	pdf, err := rebpdf.Fillable(in.PDF, in.Answers)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string][]byte{"pdf": pdf})
}

// pdf-forms:fields
func pdfAnswers(input []byte) ([]byte, error) {
	var in struct {
		rebdoc.SchemaInput
		PDF []byte `json:"pdf"`
	}
	if err := decode(input, &in); err != nil {
		return nil, err
	}
	schema := in.Resolve()
	if schema == nil {
		return nil, errors.New("pdf-answers needs fields or schema")
	}
	answers, err := rebpdf.Answers(in.PDF, *schema)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"answers": answers})
}
