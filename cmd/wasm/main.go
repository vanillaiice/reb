//go:build js && wasm

// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

// Command wasm is the WebAssembly build of the .reb engine, for browsers: Rebar Studio and Rebar's
// template editor preview run the same compiler, renderer and rebdoc rules as the server's rebc.
//
// Build: GOOS=js GOARCH=wasm go build -o rebcompiler.wasm ./cmd/wasm
//
// It registers global functions taking and returning JSON strings:
//
//	__rebCompile(reb)  -> {schema, fields, htmlSource, previewHtml, warnings, engineVersion,
//	                       error?, code?, params?, execError?}
//	                      (schema is the raw schema as a JSON string, as before; previewHtml is the
//	                      template rendered with sample answers)
//	__rebPrepare(json) {fields, answers}                         -> {answers, errors}
//	__rebRender(json)  {html, system, answers, assets, fields?}  -> {html} or {error}
//	__rebVersion()     -> the engine version
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/vanillaiice/reb"
	"github.com/vanillaiice/reb/rebdoc"
	"github.com/vanillaiice/reb/rebrender"
)

func main() {
	js.Global().Set("__rebCompile", js.FuncOf(func(_ js.Value, args []js.Value) any { return compile(argument(args)) }))
	js.Global().Set("__rebPrepare", js.FuncOf(func(_ js.Value, args []js.Value) any { return prepare(argument(args)) }))
	js.Global().Set("__rebRender", js.FuncOf(func(_ js.Value, args []js.Value) any { return render(argument(args)) }))
	js.Global().Set("__rebVersion", js.FuncOf(func(js.Value, []js.Value) any { return reb.Version }))
	// Keep the Go runtime alive so the exported functions stay callable.
	select {}
}

func argument(args []js.Value) string {
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return ""
	}
	return args[0].String()
}

// compile compiles the source and renders it with sample answers for the preview.
func compile(source string) string {
	compiled, err := rebdoc.Compile(source)
	if err != nil {
		if coded, ok := err.(*rebdoc.Error); ok {
			return marshal(map[string]any{"error": coded.Message, "code": coded.Code, "params": coded.Params})
		}
		return marshal(map[string]any{"error": err.Error()})
	}
	schemaJSON, _ := json.Marshal(compiled.Schema)
	out := map[string]any{
		"schema": string(schemaJSON), "fields": compiled.Fields, "htmlSource": compiled.HTML,
		"warnings": compiled.Warnings, "engineVersion": compiled.EngineVersion,
	}

	context := rebdoc.BuildContext(rebdoc.SampleSystem(), rebdoc.SampleAnswers(compiled.Fields), nil, &compiled.Fields)
	if preview, err := rebrender.CompileHTML(compiled.HTML, context); err != nil {
		// Fall back to the compiled HTML so the author still sees the structure.
		out["execError"] = err.Error()
		out["previewHtml"] = compiled.HTML
	} else {
		out["previewHtml"] = preview
	}
	return marshal(out)
}

func prepare(input string) string {
	var in struct {
		Fields  rebdoc.Schema  `json:"fields"`
		Answers map[string]any `json:"answers"`
	}
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return marshal(map[string]any{"error": "invalid input: " + err.Error()})
	}
	return marshal(rebdoc.Prepare(in.Fields, in.Answers))
}

func render(input string) string {
	var in struct {
		HTML    string            `json:"html"`
		System  map[string]any    `json:"system"`
		Answers map[string]any    `json:"answers"`
		Assets  map[string]string `json:"assets"`
		Fields  *rebdoc.Schema    `json:"fields"`
	}
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return marshal(map[string]any{"error": "invalid input: " + err.Error()})
	}
	html, err := rebrender.CompileHTML(in.HTML, rebdoc.BuildContext(in.System, in.Answers, in.Assets, in.Fields))
	if err != nil {
		return marshal(map[string]any{"error": err.Error()})
	}
	return marshal(map[string]any{"html": html})
}

func marshal(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return `{"error":"failed to encode result"}`
	}
	return string(b)
}
