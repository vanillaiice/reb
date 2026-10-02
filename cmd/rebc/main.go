// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

// Command rebc is the .reb pipeline for server-side callers (Rebar calls it from Rails): the same
// compiler and renderer that Rebar Studio runs as WebAssembly, behind a JSON-in, JSON-out command
// line so any language can call it.
//
//	rebc compile   stdin {"reb": "..."}                               stdout {"schema": [...], "html": "..."}
//	rebc render    stdin {"html", "system", "answers", "assets"}      stdout {"html": "..."}
//
// On failure it exits with status 1 and prints {"error": "..."}. Input always arrives on stdin,
// never as arguments.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"

	"github.com/microcosm-cc/bluemonday"

	"github.com/vanillaiice/reb/rebcompiler"
	"github.com/vanillaiice/reb/rebrender"
)

// maxInput bounds what rebc reads, so a runaway caller cannot exhaust memory.
const maxInput = 32 << 20

func main() {
	if len(os.Args) != 2 {
		fail(errors.New("usage: rebc compile|render < input.json"))
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
	out, _ := json.Marshal(map[string]string{"error": err.Error()})
	os.Stdout.Write(out)
	os.Exit(1)
}

func run(command string, input []byte) ([]byte, error) {
	switch command {
	case "compile":
		return compile(input)
	case "render":
		return render(input)
	default:
		return nil, fmt.Errorf("unknown command %q (use compile or render)", command)
	}
}

type compileInput struct {
	Reb string `json:"reb"`
}

func compile(input []byte) ([]byte, error) {
	var in compileInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	schema, html, err := rebcompiler.Compile(in.Reb)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"schema": schema, "html": html})
}

type renderInput struct {
	HTML    string            `json:"html"`
	System  map[string]any    `json:"system"`
	Answers map[string]any    `json:"answers"`
	Assets  map[string]string `json:"assets"`
}

func render(input []byte) ([]byte, error) {
	var in renderInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	html, err := rebrender.CompileHTML(in.HTML, buildContext(in.System, in.Answers, in.Assets))
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"html": html})
}

// buildContext reproduces the Go API's document export (server/internal/documents, export
// handler): file references in the answers are replaced by the names the PDF request sends the
// files under, including inside table rows; answers are flattened into the root without
// overriding system keys, and remain available as .Answers; plain strings are sanitized with
// bluemonday's UGC policy and passed as HTML (rich textarea content), while replaced file
// references stay plain strings so templates can use them in src attributes.
func buildContext(system, answers map[string]any, assets map[string]string) map[string]any {
	if answers == nil {
		answers = map[string]any{}
	}
	isAsset := map[string]bool{}
	for key, value := range answers {
		switch val := value.(type) {
		case string:
			if name, ok := assets[val]; ok {
				answers[key] = name
				isAsset[key] = true
			}
		case []any:
			for i, item := range val {
				switch it := item.(type) {
				case string:
					if name, ok := assets[it]; ok {
						val[i] = name
					}
				case map[string]any:
					for column, cell := range it {
						if s, ok := cell.(string); ok {
							if name, ok := assets[s]; ok {
								it[column] = name
							}
						}
					}
				}
			}
		}
	}

	context := make(map[string]any, len(system)+len(answers)+1)
	for key, value := range system {
		context[key] = value
	}
	if _, exists := context["Answers"]; !exists {
		context["Answers"] = answers
	}

	sanitizer := bluemonday.UGCPolicy()
	for key, value := range answers {
		if _, exists := context[key]; exists {
			continue
		}
		if s, ok := value.(string); ok && !isAsset[key] {
			context[key] = template.HTML(sanitizer.Sanitize(s))
		} else {
			context[key] = value
		}
	}
	return context
}
