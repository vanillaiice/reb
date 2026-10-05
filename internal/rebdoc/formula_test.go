// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 hblabs
//
// Part of reb, the .reb template engine (see /LICENSE).

package rebdoc

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// testdata/formula_cases.json defines table formulas; every evaluator runs it (Rebar's
// app/javascript/reb/formula.js in its CI, the mobile app's utils/formula.ts).
func TestFormulaGoldenCases(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/formula_cases.json")
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
		{"a*2", map[string]any{"a": "0.0000001"}, 7, "0.0000002"},                      // values are not written into the expression as text
		{"-" + strings.Repeat("(", 100) + "1" + strings.Repeat(")", 100), nil, 0, "0"}, // too deep
		{strings.Repeat("-", 100) + "1", nil, 0, "0"},
	} {
		if got := Formula(c.expression, c.row, c.precision); got != c.want {
			t.Errorf("Formula(%q, %v) = %q, want %q", c.expression, c.row, got, c.want)
		}
	}
}
