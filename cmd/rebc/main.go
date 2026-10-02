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
//	rebc render     {"html", "system", "answers", "assets", "fields"?} -> {"html"}
//	rebc normalize  {"schema"}                                         -> {"schemaVersion", "fields"}
//	rebc version                                                       -> {"version"}
//
// "schema" is the compiler's raw schema, "fields" the normalized one (rebdoc.Schema). On failure rebc
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
	"github.com/vanillaiice/reb/rebcompiler"
	"github.com/vanillaiice/reb/rebdoc"
	"github.com/vanillaiice/reb/rebrender"
)

// maxInput bounds what rebc reads, so a runaway caller cannot exhaust memory.
const maxInput = 32 << 20

func main() {
	if len(os.Args) != 2 {
		fail(errors.New("usage: rebc compile|prepare|render|normalize|version < input.json"))
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
	var coded *rebdoc.Error
	var out []byte
	if errors.As(err, &coded) {
		out, _ = json.Marshal(coded)
	} else {
		out, _ = json.Marshal(map[string]string{"error": err.Error()})
	}
	os.Stdout.Write(out)
	os.Exit(1)
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
	case "version":
		return json.Marshal(map[string]string{"version": reb.Version})
	default:
		return nil, fmt.Errorf("unknown command %q (use compile, prepare, render, normalize or version)", command)
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

// schemaInput accepts the normalized schema ("fields") or the raw one ("schema").
type schemaInput struct {
	Fields *rebdoc.Schema               `json:"fields"`
	Schema []rebcompiler.RebFieldSchema `json:"schema"`
}

func (s schemaInput) resolve() *rebdoc.Schema {
	if s.Fields != nil {
		return s.Fields
	}
	if s.Schema != nil {
		normalized := rebdoc.Normalize(s.Schema)
		return &normalized
	}
	return nil
}

func prepare(input []byte) ([]byte, error) {
	var in struct {
		schemaInput
		Answers map[string]any `json:"answers"`
	}
	if err := decode(input, &in); err != nil {
		return nil, err
	}
	schema := in.resolve()
	if schema == nil {
		return nil, errors.New("prepare needs fields or schema")
	}
	return json.Marshal(rebdoc.Prepare(*schema, in.Answers))
}

func render(input []byte) ([]byte, error) {
	var in struct {
		schemaInput
		HTML    string            `json:"html"`
		System  map[string]any    `json:"system"`
		Answers map[string]any    `json:"answers"`
		Assets  map[string]string `json:"assets"`
	}
	if err := decode(input, &in); err != nil {
		return nil, err
	}
	html, err := rebrender.CompileHTML(in.HTML, rebdoc.BuildContext(in.System, in.Answers, in.Assets, in.resolve()))
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
