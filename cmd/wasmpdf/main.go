//go:build js && wasm

// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

// pdf-forms:fields: this whole command.
//
// Command wasmpdf is the WebAssembly build of the engine's PDF form functions (rebc fillable and
// pdf-answers), kept out of the main build because the PDF library makes it half as large again.
// A browser loads it only when it makes or reads a PDF form.
//
// Build: GOOS=js GOARCH=wasm go build -o rebpdf.wasm ./cmd/wasmpdf
//
// It registers global functions taking and returning JSON strings (PDFs base64-encoded):
//
//	__rebFillable(json)   {pdf, answers?}         -> {pdf} or {error}
//	__rebPdfAnswers(json) {pdf, fields | schema}  -> {answers} or {error}
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/vanillaiice/reb/internal/rebdoc"
	"github.com/vanillaiice/reb/internal/rebpdf"
)

func main() {
	js.Global().Set("__rebFillable", js.FuncOf(func(_ js.Value, args []js.Value) any { return fillable(argument(args)) }))
	js.Global().Set("__rebPdfAnswers", js.FuncOf(func(_ js.Value, args []js.Value) any { return pdfAnswers(argument(args)) }))
	// Keep the Go runtime alive so the exported functions stay callable.
	select {}
}

func argument(args []js.Value) string {
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return ""
	}
	return args[0].String()
}

func fillable(input string) string {
	var in struct {
		PDF     []byte         `json:"pdf"`
		Answers map[string]any `json:"answers"`
	}
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return marshal(map[string]any{"error": "invalid input: " + err.Error()})
	}
	pdf, err := rebpdf.Fillable(in.PDF, in.Answers)
	if err != nil {
		return marshal(map[string]any{"error": err.Error()})
	}
	return marshal(map[string]any{"pdf": pdf})
}

func pdfAnswers(input string) string {
	var in struct {
		rebdoc.SchemaInput
		PDF []byte `json:"pdf"`
	}
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return marshal(map[string]any{"error": "invalid input: " + err.Error()})
	}
	schema := in.Resolve()
	if schema == nil {
		return marshal(map[string]any{"error": "pdf-answers needs fields or schema"})
	}
	answers, err := rebpdf.Answers(in.PDF, *schema)
	if err != nil {
		return marshal(map[string]any{"error": err.Error()})
	}
	return marshal(map[string]any{"answers": answers})
}

func marshal(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return `{"error":"failed to encode result"}`
	}
	return string(b)
}
