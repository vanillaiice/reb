//go:build js && wasm

package main

import "testing"

// Run with (needs Node; bin/ci does):
//
//	GOOS=js GOARCH=wasm go -C reb test -exec="bash $(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./cmd/wasm
//
// The same expressions as the server's golden cases (test/lib/reb_formula_test.rb).
func TestEvalFormulaFollowsTheOtherClients(t *testing.T) {
	cases := []struct {
		expr string
		row  map[string]any
		want float64
	}{
		{"qty*rate", map[string]any{"qty": "3", "rate": "2.5"}, 7.5},
		{"a+b*c", map[string]any{"a": "1", "b": "2", "c": "3"}, 7},
		{"(a+b)*c", map[string]any{"a": "1", "b": "2", "c": "3"}, 9},
		{"a/b", map[string]any{"a": "1", "b": "0"}, 0},
		{"qty * unknown + 4", map[string]any{"qty": "3"}, 4},
		{"q*qty", map[string]any{"q": "2", "qty": "5"}, 10},
		{"2*-3", map[string]any{}, -3},
	}
	for _, c := range cases {
		if got := evalFormula(c.expr, c.row); got != c.want {
			t.Errorf("%s with %v = %v, want %v", c.expr, c.row, got, c.want)
		}
	}
}
