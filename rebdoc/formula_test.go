// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"encoding/json"
	"os"
	"testing"
)

// testdata/formula_cases.json was produced under Node from the Rebar mobile and web evaluators; every
// consumer's evaluator runs it (Rebar's app/javascript/reb/formula.js in its CI).
func TestFormulaGoldenCases(t *testing.T) {
	raw, err := os.ReadFile("../testdata/formula_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Expression string         `json:"expression"`
		Row        map[string]any `json:"row"`
		Precision  int            `json:"precision"`
		Expected   string         `json:"expected"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 20 {
		t.Fatalf("expected the golden cases, got %d", len(cases))
	}
	for _, c := range cases {
		if got := Formula(c.Expression, c.Row, c.Precision); got != c.Expected {
			t.Errorf("Formula(%q, %v, %d) = %q, want %q", c.Expression, c.Row, c.Precision, got, c.Expected)
		}
	}
}

func TestFormulaEdges(t *testing.T) {
	for _, c := range []struct {
		expression string
		row        map[string]any
		precision  int
		want       string
	}{
		{"(a+b", map[string]any{"a": "1", "b": "2"}, 2, "0.00"},
		{"a*b", map[string]any{"a": 2.5, "b": true}, 1, "0.0"},
		{"a", map[string]any{"a": "-0.001"}, 2, "-0.00"},
		{"a", map[string]any{"a": "0.0000001"}, 2, "3.00"}, // "1e-7" becomes "10-7" once names turn to 0, as in JavaScript
	} {
		if got := Formula(c.expression, c.row, c.precision); got != c.want {
			t.Errorf("Formula(%q, %v) = %q, want %q", c.expression, c.row, got, c.want)
		}
	}
}
