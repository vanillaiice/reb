// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the expected outputs in testdata/golden")

// TestGolden runs each testdata/golden/NAME.reb through rebc as a consumer would and compares the
// outputs with the files beside it, so any change to what a template compiles or renders to shows up
// in review:
//
//	NAME.golden.json     compile's output (html and engineVersion aside), or its error; with an
//	                     input, prepare's output too
//	NAME.compiled.html   the compiled template
//	NAME.rendered.html   with NAME.input.json ({"system", "answers", "assets", "fillable"?}): the
//	                     template rendered with the prepared answers
//	NAME.fillable.html   when the input says "fillable": true, the same rendered fillable
//
// After an intended change: go test ./cmd/rebc -update, and review the diff.
func TestGolden(t *testing.T) {
	sources, err := filepath.Glob("../../testdata/golden/*.reb")
	if err != nil || len(sources) == 0 {
		t.Fatalf("no golden cases: %v", err)
	}
	for _, source := range sources {
		base := strings.TrimSuffix(source, ".reb")
		t.Run(filepath.Base(base), func(t *testing.T) {
			for suffix, got := range goldenOutputs(t, source, base) {
				compareGolden(t, base+suffix, got)
			}
		})
	}
}

func goldenOutputs(t *testing.T, source, base string) map[string][]byte {
	reb, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string][]byte{}
	out, err := run("compile", mustJSON(t, map[string]string{"reb": string(reb)}))
	if err != nil {
		outputs[".golden.json"] = indentJSON(t, errorJSON(err))
		return outputs
	}
	var compiled map[string]any
	if err := json.Unmarshal(out, &compiled); err != nil {
		t.Fatal(err)
	}
	html, _ := compiled["html"].(string)
	outputs[".compiled.html"] = []byte(html + "\n")
	delete(compiled, "html")
	compiled["engineVersion"] = "(the engine version)"
	result := map[string]any{"compile": compiled}

	if input, err := os.ReadFile(base + ".input.json"); err == nil {
		var in struct {
			System   map[string]any    `json:"system"`
			Answers  map[string]any    `json:"answers"`
			Assets   map[string]string `json:"assets"`
			Fillable bool              `json:"fillable"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			t.Fatal(err)
		}
		out, err := run("prepare", mustJSON(t, map[string]any{"fields": compiled["fields"], "answers": in.Answers}))
		if err != nil {
			t.Fatalf("prepare: %v", err)
		}
		var prepared map[string]any
		if err := json.Unmarshal(out, &prepared); err != nil {
			t.Fatal(err)
		}
		result["prepare"] = prepared

		// pdf-forms:boxes: the .fillable.html output
		modes := map[bool]string{false: ".rendered.html"}
		if in.Fillable {
			modes[true] = ".fillable.html"
		}
		for fillable, suffix := range modes {
			out, err = run("render", mustJSON(t, map[string]any{"html": html, "system": in.System, "answers": prepared["answers"],
				"assets": in.Assets, "fields": compiled["fields"], "fillable": fillable}))
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			var rendered struct {
				HTML string `json:"html"`
			}
			if err := json.Unmarshal(out, &rendered); err != nil {
				t.Fatal(err)
			}
			outputs[suffix] = []byte(rendered.HTML + "\n")
		}
	}
	outputs[".golden.json"] = indentJSON(t, mustJSON(t, result))
	return outputs
}

func compareGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./cmd/rebc -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the output:\n%s\n(review it, then go test ./cmd/rebc -update)", filepath.Base(path), got)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func indentJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		t.Fatal(err)
	}
	out.WriteByte('\n')
	return out.Bytes()
}
